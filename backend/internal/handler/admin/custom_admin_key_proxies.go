package admin

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/adminaccess"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// scopedProxyOption 显式列出可公开字段，不能复用含用户名、密码的管理 DTO。
type scopedProxyOption struct {
	ID        int64      `json:"id"`
	Name      string     `json:"name"`
	Protocol  string     `json:"protocol"`
	Host      string     `json:"host"`
	Port      int        `json:"port"`
	Status    string     `json:"status"`
	ExpiresAt *time.Time `json:"expires_at"`
}

func scopedProxyDTO(p service.Proxy) scopedProxyOption {
	return scopedProxyOption{ID: p.ID, Name: p.Name, Protocol: p.Protocol, Host: p.Host, Port: p.Port, Status: p.Status, ExpiresAt: p.ExpiresAt}
}

// scopedAdminProxyRead 对代理读取限定数量；无分页的 /all 从全池抽样。
func (h *ProxyHandler) scopedAdminProxyRead(c *gin.Context) bool {
	principal := adminaccess.FromContext(c.Request.Context())
	if principal == nil {
		return false
	}
	if !slices.Contains(principal.Policy.GlobalPermissions, "proxies.read") {
		response.Error(c, 403, "代理读取未授权")
		return true
	}
	if raw := c.Param("id"); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id < 1 {
			response.BadRequest(c, "代理 ID 无效")
			return true
		}
		p, err := h.adminService.GetProxy(c.Request.Context(), id)
		if err != nil || p == nil {
			response.Error(c, 404, "代理不存在")
			return true
		}
		response.Success(c, scopedProxyDTO(*p))
		return true
	}
	page, size := 1, 50
	for key, dest := range map[string]*int{"page": &page, "page_size": &size} {
		if raw, ok := c.GetQuery(key); ok {
			n, err := strconv.Atoi(raw)
			if err != nil || n < 1 || n > 1000000 {
				response.BadRequest(c, "分页参数必须为正整数且不超过 1000000")
				return true
			}
			*dest = n
		}
	}
	if size > 100 {
		size = 100
	}
	var proxies []service.Proxy
	var total int64
	var err error
	if raw, ok := c.Request.URL.Query()["ids"]; ok {
		ids := []int64{}
		for _, part := range strings.Split(strings.Join(raw, ","), ",") {
			if strings.TrimSpace(part) == "" {
				continue
			}
			id, e := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
			if e != nil || id < 1 {
				response.BadRequest(c, "代理 ID 无效")
				return true
			}
			if !slices.Contains(ids, id) {
				ids = append(ids, id)
			}
			if len(ids) > 100 {
				response.BadRequest(c, "一次最多读取 100 个已选代理")
				return true
			}
		}
		if len(ids) > 0 {
			proxies, err = h.adminService.GetProxiesByIDs(c.Request.Context(), ids)
		}
		// 最多 100 个 ID 的回显按实际存在的记录分页，保留请求顺序。
		byID := make(map[int64]service.Proxy, len(proxies))
		for _, p := range proxies {
			byID[p.ID] = p
		}
		proxies = nil
		for _, id := range ids {
			if p, ok := byID[id]; ok {
				proxies = append(proxies, p)
			}
		}
		total = int64(len(proxies))
		start := (page - 1) * size
		if start < len(proxies) {
			proxies = proxies[start:min(start+size, len(proxies))]
		} else {
			proxies = nil
		}
	} else {
		search := strings.TrimSpace(c.Query("search"))
		if len(search) > 100 {
			response.BadRequest(c, "搜索关键词不能超过 100 字节")
			return true
		}
		query := c.Request.URL.Query()
		if strings.HasSuffix(c.FullPath(), "/proxies/all") && !query.Has("page") && !query.Has("page_size") {
			sampler, ok := h.adminService.(interface {
				SampleProxyOptions(context.Context, string) ([]service.Proxy, int64, error)
			})
			if !ok {
				response.Error(c, 503, "代理查询暂不可用")
				return true
			}
			proxies, total, err = sampler.SampleProxyOptions(c.Request.Context(), search)
			// 旧客户端重复加载时必须重新抽样，不能复用上一次缓存的候选。
			c.Header("Cache-Control", "no-store")
		} else {
			proxies, total, err = scopedProxyOptions(c.Request.Context(), h.adminService, page, size, search)
		}
	}
	if err != nil {
		response.Error(c, 503, "代理查询暂不可用")
		return true
	}
	out := make([]scopedProxyOption, 0, min(len(proxies), size))
	for _, p := range proxies[:min(len(proxies), size)] {
		out = append(out, scopedProxyDTO(p))
	}
	if strings.HasSuffix(c.FullPath(), "/proxies/all") {
		c.Header("X-Total-Count", strconv.FormatInt(total, 10))
		c.Header("X-Page", strconv.Itoa(page))
		c.Header("X-Page-Size", strconv.Itoa(size))
		c.Header("X-Total-Pages", strconv.FormatInt((total+int64(size)-1)/int64(size), 10))
		response.Success(c, out)
	} else {
		response.Paginated(c, out, total, page, size)
	}
	return true
}

func scopedProxyOptions(ctx context.Context, admin service.AdminService, page, size int, search string) ([]service.Proxy, int64, error) {
	api, ok := admin.(interface {
		ListProxyOptions(context.Context, int, int, string) ([]service.Proxy, int64, error)
	})
	if !ok {
		return nil, 0, fmt.Errorf("代理查询不可用")
	}
	return api.ListProxyOptions(ctx, page, size, search)
}
