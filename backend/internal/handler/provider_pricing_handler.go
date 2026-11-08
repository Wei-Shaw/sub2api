package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// ProviderPricingHandler 对外公开（无需登录）的模型价格接口。
//
// ⚠️ 该接口是唯一刻意不走 internal/pkg/response 响应信封的对外接口：
// 第三方已经依赖旧 companion 输出的裸 {schema_version, success, message, data}
// 形状，套上 {code,message,data} 信封会直接改变第三方可见的 JSON 结构。
type ProviderPricingHandler struct {
	// pricingService 价格数据来源：读取并解析 Hvoy schema 1.0 价格文件
	pricingService *service.ProviderPricingService
}

// NewProviderPricingHandler 构造公开价格接口 handler。
func NewProviderPricingHandler(pricingService *service.ProviderPricingService) *ProviderPricingHandler {
	return &ProviderPricingHandler{pricingService: pricingService}
}

// GetPricing 返回公开的模型价格配置。
// GET /api/provider/pricing
func (h *ProviderPricingHandler) GetPricing(c *gin.Context) {
	doc, err := h.pricingService.Load()
	if err != nil {
		if errors.Is(err, service.ErrProviderPricingInvalid) {
			writeProviderPricingJSON(c, http.StatusInternalServerError, map[string]any{
				"success": false,
				"message": "invalid pricing configuration",
			})
			return
		}
		// 文件不存在/不可读：对外只暴露 503，不泄露容器内路径。
		writeProviderPricingJSON(c, http.StatusServiceUnavailable, map[string]any{
			"success": false,
			"message": "pricing unavailable",
		})
		return
	}

	writeProviderPricingJSON(c, http.StatusOK, map[string]any{
		"schema_version": doc.SchemaVersion,
		"success":        true,
		"message":        "",
		"data": map[string]any{
			"currency":    doc.Currency,
			"price_unit":  doc.PriceUnit,
			"site_name":   doc.SiteName,
			"site_domain": doc.SiteDomain,
			// updated_at 每次请求现取当前时间（UTC，RFC3339），与旧实现一致
			"updated_at": time.Now().UTC().Format(time.RFC3339),
			"models":     doc.Models,
		},
	})
}

// writeProviderPricingJSON 按旧 companion 的字节形态输出公开价格响应。
//
// 顶层刻意用 map 承载而非结构体：Go 的 json 对 map 键做字典序排序，
// 这样产出与旧实现（writeJSON 里 json.NewEncoder(w).Encode(map)）一致，
// 并补上 Encoder 特有的结尾换行。第三方若做了字符串级比对也不会翻车。
func writeProviderPricingJSON(c *gin.Context, status int, payload map[string]any) {
	body, err := json.Marshal(payload)
	if err != nil {
		// 数据全部来自已解析的 JSON，理论上不可达；真发生时不返回半截响应。
		c.Status(http.StatusInternalServerError)
		return
	}
	c.Data(status, "application/json; charset=utf-8", append(body, '\n'))
}
