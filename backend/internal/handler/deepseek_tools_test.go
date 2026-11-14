package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// emptyDeepseekAccountRepo 只 stub 选号会用到的列表方法，其余方法不指望被调用。
type emptyDeepseekAccountRepo struct {
	service.AccountRepository
}

func (emptyDeepseekAccountRepo) ListSchedulableByPlatform(ctx context.Context, platform string) ([]service.Account, error) {
	return nil, nil
}

func newDeepseekToolsTestHandler(t *testing.T) *OpenAIGatewayHandler {
	t.Helper()
	cfg := &config.Config{RunMode: config.RunModeSimple}
	billing := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billing.Stop)
	gatewayService := service.NewOpenAIGatewayService(
		emptyDeepseekAccountRepo{}, nil, nil, nil, nil, nil, nil, cfg,
		nil, nil, nil, nil, billing, nil, nil, nil, nil, nil, nil, nil, nil, nil,
	)
	return &OpenAIGatewayHandler{
		gatewayService:      gatewayService,
		billingCacheService: billing,
		apiKeyService:       &service.APIKeyService{},
		concurrencyHelper:   &ConcurrencyHelper{concurrencyService: service.NewConcurrencyService(nil)},
		cfg:                 cfg,
	}
}

func newDeepseekToolsRouter(h *OpenAIGatewayHandler, platform string, endpoint string) *gin.Engine {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{
			Group: &service.Group{Platform: platform},
			User:  &service.User{ID: 1},
		})
		c.Next()
	})
	router.POST("/v1/tools/search", func(c *gin.Context) { h.DeepseekTools(c, "search") })
	router.GET("/v1/files", func(c *gin.Context) { h.DeepseekTools(c, "files_list") })
	return router
}

// 非DeepSeek组一律404；DeepSeek组无可用账号时503。
func TestDeepseekToolsGate(t *testing.T) {
	h := newDeepseekToolsTestHandler(t)

	t.Run("non deepseek platform 404", func(t *testing.T) {
		for _, platform := range []string{service.PlatformGrok, service.PlatformOpenAI, "anthropic", ""} {
			router := newDeepseekToolsRouter(h, platform, "search")
			req := httptest.NewRequest(http.MethodPost, "/v1/tools/search", strings.NewReader(`{"query":"golang"}`))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			require.Equal(t, http.StatusNotFound, rec.Code, "platform %q: %s", platform, rec.Body.String())
			require.Contains(t, rec.Body.String(), "not_found_error")
		}
	})

	t.Run("deepseek platform without accounts 503", func(t *testing.T) {
		router := newDeepseekToolsRouter(h, service.PlatformDeepseek, "search")
		req := httptest.NewRequest(http.MethodPost, "/v1/tools/search", strings.NewReader(`{"query":"golang"}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		require.Equal(t, http.StatusServiceUnavailable, rec.Code, rec.Body.String())
		require.Contains(t, rec.Body.String(), "No available DeepSeek accounts")
	})
}

// files_list 等 files_* 端点同样受平台门禁约束。
func TestDeepseekToolsGateFilesRoutes(t *testing.T) {
	h := newDeepseekToolsTestHandler(t)
	router := newDeepseekToolsRouter(h, service.PlatformGrok, "files_list")
	req := httptest.NewRequest(http.MethodGet, "/v1/files", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
}

// 计费量映射：per_request → SearchCount，ocr/asr/tts → AudioUsage（对应
// CalculateAudioCost 的 deepseek_* mode），files_* 不产生计费回执。
func TestDeepseekToolsBillingResultMapping(t *testing.T) {
	r := deepseekToolsBillingResult("search", service.DeepseekToolsUsage{Kind: service.DeepseekToolsUsagePerRequest, Count: 1}, "id-search")
	require.NotNil(t, r)
	require.Equal(t, "deepseek-tools-search", r.Model)
	require.Equal(t, 1, r.SearchCount)
	require.Nil(t, r.AudioUsage)

	r = deepseekToolsBillingResult("fetch", service.DeepseekToolsUsage{Kind: service.DeepseekToolsUsagePerRequest, Count: 1}, "id-fetch")
	require.NotNil(t, r)
	require.Equal(t, "deepseek-tools-fetch", r.Model)
	require.Equal(t, 1, r.SearchCount)

	r = deepseekToolsBillingResult("ocr", service.DeepseekToolsUsage{Kind: service.DeepseekToolsUsageOCRPages, Count: 3}, "id-ocr")
	require.NotNil(t, r)
	require.Equal(t, "deepseek-tools-ocr", r.Model)
	require.Equal(t, 0, r.SearchCount)
	require.NotNil(t, r.AudioUsage)
	require.Equal(t, "deepseek_ocr", r.AudioUsage.Mode)
	require.Equal(t, 3.0, r.AudioUsage.DurationOrUnits)

	r = deepseekToolsBillingResult("asr", service.DeepseekToolsUsage{Kind: service.DeepseekToolsUsageASRSeconds, Count: 60}, "id-asr")
	require.NotNil(t, r)
	require.Equal(t, "deepseek_asr", r.AudioUsage.Mode)
	require.Equal(t, 60.0, r.AudioUsage.DurationOrUnits)

	r = deepseekToolsBillingResult("tts", service.DeepseekToolsUsage{Kind: service.DeepseekToolsUsageTTSChars, Count: 100}, "id-tts")
	require.NotNil(t, r)
	require.Equal(t, "deepseek_tts", r.AudioUsage.Mode)
	require.Equal(t, 100.0, r.AudioUsage.DurationOrUnits)

	// files_*（以及上游失败时 Kind 为空）不落账。
	for _, usage := range []service.DeepseekToolsUsage{
		{},
		{Kind: "unknown_kind", Count: 1},
	} {
		require.Nil(t, deepseekToolsBillingResult("files_get", usage, "id-files"), "usage=%#v", usage)
	}
}

// 金额断言按官方牌价口径：search ¥5/千次；OCR ¥1.5/千页、ASR ¥0.78/小时、
// TTS ¥3.0/万字符（货币口径与 defaultSearchPricePer1k 一致）。
func TestDeepseekToolsCostsAtOfficialPrices(t *testing.T) {
	bs := &service.BillingService{}

	search := bs.CalculateSearchCost(1, nil, 1.0)
	require.NotNil(t, search)
	require.InDelta(t, 5.0/1000.0, search.TotalCost, 1e-12)
	require.Equal(t, string(service.BillingModePerRequest), search.BillingMode)

	ocr := bs.CalculateAudioCost("deepseek_ocr", 3, nil, 1.0)
	require.NotNil(t, ocr)
	require.InDelta(t, 3*1.5/1000.0, ocr.TotalCost, 1e-12)
	require.Equal(t, string(service.BillingModePerRequest), ocr.BillingMode)

	asr := bs.CalculateAudioCost("deepseek_asr", 60, nil, 1.0)
	require.NotNil(t, asr)
	require.InDelta(t, 60*0.78/3600.0, asr.TotalCost, 1e-12)

	tts := bs.CalculateAudioCost("deepseek_tts", 100, nil, 1.0)
	require.NotNil(t, tts)
	require.InDelta(t, 100*3.0/10000.0, tts.TotalCost, 1e-12)

	// 零/负量不计费。
	require.Equal(t, 0.0, bs.CalculateAudioCost("deepseek_ocr", 0, nil, 1.0).TotalCost)
}

// 请求 ID 幂等：同 endpoint+payload 指纹（同时间窗）生成同一 ID，不同指纹不同 ID，
// 且带 deepseek_tools: 强前缀（不会被客户端复用的 request id 折叠）。
func TestDeepseekToolsBillingRequestIDStable(t *testing.T) {
	first := service.StableDeepseekToolsBillingRequestID("search", "hash-a")
	second := service.StableDeepseekToolsBillingRequestID("search", "hash-a")
	require.Equal(t, first, second)
	require.True(t, strings.HasPrefix(first, "deepseek_tools:"))

	other := service.StableDeepseekToolsBillingRequestID("search", "hash-b")
	require.NotEqual(t, first, other)
}

// body 读取失败分类：POST 缺 body 是 400 校验错误（共享 errGrokVoiceBodyRequired 通道）。
func TestDeepseekToolsBodyRequired(t *testing.T) {
	h := newDeepseekToolsTestHandler(t)
	router := newDeepseekToolsRouter(h, service.PlatformDeepseek, "search")
	req := httptest.NewRequest(http.MethodPost, "/v1/tools/search", nil)
	req.Body = nil
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	require.Contains(t, rec.Body.String(), "request body is required")
}
