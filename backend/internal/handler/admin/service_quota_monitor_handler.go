package admin

import (
	"strconv"
	"strings"

	pkgerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// ServiceQuotaMonitorHandler 是 admin 端"服务限额运行时监控"接口的入口。
//
// 单一职责：接受 admin filter（rule_id/user_id/channel_id/group_id/account_id/platform）
// 透传给 ServiceQuotaMonitorService.Snapshot，把结果序列化为 JSON。
// 错误语义统一走 response.ErrorFrom（业务层用 ApplicationError 表达）；
// 解析阶段的非数字 query 直接返回 400 + code=INVALID_QUERY_PARAM。
type ServiceQuotaMonitorHandler struct {
	svc service.ServiceQuotaMonitorService
}

func NewServiceQuotaMonitorHandler(svc service.ServiceQuotaMonitorService) *ServiceQuotaMonitorHandler {
	return &ServiceQuotaMonitorHandler{svc: svc}
}

// Snapshot 处理 GET /api/v1/admin/service-quotas/monitor。
//
// 所有 query 都是可选的，留空则不过滤。filter 字段一律解析为指针，让 service 层
// 区分"没指定"与"指定为 0"。任何字段解析失败立即 400，不再调用 service。
func (h *ServiceQuotaMonitorHandler) Snapshot(c *gin.Context) {
	if h == nil || h.svc == nil {
		response.ErrorFrom(c, pkgerrors.NotFound(
			"SERVICE_QUOTA_MONITOR_UNAVAILABLE",
			"service quota monitor unavailable",
		))
		return
	}
	filter, err := parseMonitorFilter(c)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	snap, err := h.svc.Snapshot(c.Request.Context(), filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, snap)
}

// parseMonitorFilter 解析 6 个 admin query。任何非数字字段直接返回 BadRequest，
// 触发 response.ErrorFrom 输出 {code:400, reason:"INVALID_QUERY_PARAM", details:{field, value}}。
func parseMonitorFilter(c *gin.Context) (service.MonitorSnapshotFilter, error) {
	filter := service.MonitorSnapshotFilter{}
	if v, ok, err := parseInt64Query(c, "rule_id"); err != nil {
		return filter, err
	} else if ok {
		filter.RuleID = &v
	}
	if v, ok, err := parseInt64Query(c, "user_id"); err != nil {
		return filter, err
	} else if ok {
		filter.UserID = &v
	}
	if v, ok, err := parseInt64Query(c, "channel_id"); err != nil {
		return filter, err
	} else if ok {
		filter.ChannelID = &v
	}
	if v, ok, err := parseInt64Query(c, "group_id"); err != nil {
		return filter, err
	} else if ok {
		filter.GroupID = &v
	}
	if v, ok, err := parseInt64Query(c, "account_id"); err != nil {
		return filter, err
	} else if ok {
		filter.AccountID = &v
	}
	if v, ok := parseStringQuery(c, "platform"); ok {
		filter.Platform = &v
	}
	return filter, nil
}

// parseInt64Query 读取并解析单个 int64 query。返回 (value, hasValue, err)：
//   - 空字符串 → (0, false, nil)，调用方跳过
//   - 非数字 → (0, false, BadRequest)
//   - 合法数字 → (v, true, nil)
func parseInt64Query(c *gin.Context, key string) (int64, bool, error) {
	raw := strings.TrimSpace(c.Query(key))
	if raw == "" {
		return 0, false, nil
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, false, pkgerrors.BadRequest(
			"INVALID_QUERY_PARAM",
			"query parameter must be an integer",
		).WithMetadata(map[string]string{
			"field": key,
			"value": raw,
		})
	}
	return v, true, nil
}

// parseStringQuery 读取单个字符串 query；trim 后空字符串视为未提供。
func parseStringQuery(c *gin.Context, key string) (string, bool) {
	raw := strings.TrimSpace(c.Query(key))
	if raw == "" {
		return "", false
	}
	return raw, true
}
