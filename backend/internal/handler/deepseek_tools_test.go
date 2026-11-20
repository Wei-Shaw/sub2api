package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 1})
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

	// composite 组（多平台账号池）放行进 handler，由调度器按 deepseek 选号；
	// 组内无 deepseek 账号时 503 而非 404（对齐 Grok video 的 composite 放行先例）。
	t.Run("composite platform allowed, falls through to account selection", func(t *testing.T) {
		router := newDeepseekToolsRouter(h, service.PlatformComposite, "search")
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

// deepseekToolsAccountRepo 返回固定账号列表，供调度器选号测试。
type deepseekToolsAccountRepo struct {
	service.AccountRepository
	accounts []service.Account
}

func (r deepseekToolsAccountRepo) ListSchedulableByPlatform(ctx context.Context, platform string) ([]service.Account, error) {
	return r.accounts, nil
}

func (r deepseekToolsAccountRepo) ListSchedulableByPlatforms(ctx context.Context, platforms []string) ([]service.Account, error) {
	return r.accounts, nil
}

// 上游能力不支持的工具账号（非官方 base_url、未探测）必须被选号循环跳过，
// 最终表现为无可用账号 503，而不是把请求发给必然 404 的第三方中转。
func TestDeepseekToolsSkipsAccountWithoutToolsCapability(t *testing.T) {
	account := service.Account{
		ID:          950,
		Platform:    service.PlatformDeepseek,
		Type:        service.AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "https://www.crazyhub.cn/v1",
		},
	}
	cfg := &config.Config{RunMode: config.RunModeSimple}
	billing := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billing.Stop)
	gatewayService := service.NewOpenAIGatewayService(
		deepseekToolsAccountRepo{accounts: []service.Account{account}},
		nil, nil, nil, nil, nil, nil, cfg, nil, nil, nil, nil, billing, nil, nil, nil, nil, nil, nil, nil, nil, nil,
	)
	h := &OpenAIGatewayHandler{
		gatewayService:      gatewayService,
		billingCacheService: billing,
		apiKeyService:       &service.APIKeyService{},
		concurrencyHelper:   &ConcurrencyHelper{concurrencyService: service.NewConcurrencyService(nil)},
		cfg:                 cfg,
	}
	router := newDeepseekToolsRouter(h, service.PlatformDeepseek, "search")
	req := httptest.NewRequest(http.MethodPost, "/v1/tools/search", strings.NewReader(`{"query":"golang"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusServiceUnavailable, rec.Code, rec.Body.String())
	require.Contains(t, rec.Body.String(), "No available DeepSeek accounts")
}

// deepseekToolsSlotCache 只实现账号槽位的抢/放，用于断言选号阶段抢到的槽位
// 是否被归还（其余方法由内嵌接口占位，调用即 panic，便于暴露意外依赖）。
type deepseekToolsSlotCache struct {
	service.ConcurrencyCache
	mu       sync.Mutex
	held     map[string]struct{}
	acquires int
	releases int
}

func (c *deepseekToolsSlotCache) AcquireAccountSlot(_ context.Context, _ int64, maxConcurrency int, requestID string) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.held) >= maxConcurrency {
		return false, nil
	}
	c.held[requestID] = struct{}{}
	c.acquires++
	return true, nil
}

func (c *deepseekToolsSlotCache) ReleaseAccountSlot(_ context.Context, _ int64, requestID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.releases++
	delete(c.held, requestID)
	return nil
}

func (c *deepseekToolsSlotCache) outstanding() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.held)
}

func (c *deepseekToolsSlotCache) releaseCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.releases
}

func TestDeepseekToolsSchedulerFiltersBeforeSlotAcquisition(t *testing.T) {
	account := service.Account{
		ID:          951,
		Platform:    service.PlatformDeepseek,
		Type:        service.AccountTypeAPIKey,
		Status:      service.StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "https://www.crazyhub.cn/v1",
		},
	}
	require.False(t, account.SupportsOpenAIEndpointCapability(service.OpenAIEndpointCapabilityDeepseekTools),
		"测试账号必须是「已选号但不支持工具端点」的中转账号")

	cfg := &config.Config{RunMode: config.RunModeSimple}
	billing := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billing.Stop)
	slotCache := &deepseekToolsSlotCache{held: map[string]struct{}{}}
	concurrencyService := service.NewConcurrencyService(slotCache)
	gatewayService := service.NewOpenAIGatewayService(
		deepseekToolsAccountRepo{accounts: []service.Account{account}},
		nil, nil, nil, nil, nil, nil, cfg,
		nil, concurrencyService, nil, nil, billing, nil, nil, nil, nil, nil, nil, nil, nil, nil,
	)
	h := &OpenAIGatewayHandler{
		gatewayService:      gatewayService,
		billingCacheService: billing,
		apiKeyService:       &service.APIKeyService{},
		concurrencyHelper:   &ConcurrencyHelper{concurrencyService: concurrencyService},
		cfg:                 cfg,
	}
	router := newDeepseekToolsRouter(h, service.PlatformDeepseek, "search")
	req := httptest.NewRequest(http.MethodPost, "/v1/tools/search", strings.NewReader(`{"query":"golang"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusServiceUnavailable, rec.Code, rec.Body.String())
	require.Contains(t, rec.Body.String(), "No available DeepSeek accounts")
	require.Zero(t, slotCache.acquires)
	require.Zero(t, slotCache.releaseCount())
	require.Equal(t, 0, slotCache.outstanding(), "槽位不得滞留")
}
