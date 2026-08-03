package admin

import (
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// TestFXAPIRequest 测试汇率 API 请求（v4.6.2 currency separation, task 3）
// 前端"立即获取汇率"按钮调用，验证用户填的 FX URL 是否正确。
type TestFXAPIRequest struct {
	URL          string  `json:"url"`           // 待测试的汇率 API URL
	FallbackRate float64 `json:"fallback_rate"` // 备用（暂未用，保留）
}

// TestFXAPI 测试单个汇率 API 地址
// POST /api/v1/admin/settings/test-fx
// 返回 { ok, base, rate_usd_cny, latency_ms, error }
func (h *SettingHandler) TestFXAPI(c *gin.Context) {
	var req TestFXAPIRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if req.URL == "" {
		response.BadRequest(c, "url is required")
		return
	}

	start := time.Now()
	// 用 FXService 直接拉取单 URL（复用 fetchAPI，不落缓存）
	rate, base, ok := fetchFXAPIForTest(req.URL)
	latency := time.Since(start).Milliseconds()

	if !ok {
		response.Error(c, http.StatusBadGateway, "FX_API_TEST_FAILED")
		return
	}
	response.Success(c, gin.H{
		"ok":            true,
		"base":          base,
		"rate_usd_cny":  rate,
		"latency_ms":    latency,
	})
}

// fetchFXAPIForTest 复用 FXService 的 fetchAPI（包内不可直接调私有方法，这里独立实现）。
func fetchFXAPIForTest(url string) (float64, string, bool) {
	svc := service.NewFXService(nil)
	return svc.TestFetchAPI(url)
}
