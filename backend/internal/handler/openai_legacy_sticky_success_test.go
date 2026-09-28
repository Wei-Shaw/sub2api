//go:build unit

package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newOpenAILegacySuccessFixture(t *testing.T, upstream *stickyUpstream, cache *stickyGatewayTestCache, profit bool) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	group := &service.Group{ID: 7, Hydrated: true, Platform: service.PlatformOpenAI, Status: service.StatusActive,
		RateMultiplier: 1, SubscriptionType: service.SubscriptionTypeStandard, ProfitControlEnabled: profit}
	accounts := make([]service.Account, 2)
	for i, rate := range []float64{0.8, 1} {
		accounts[i] = service.Account{
			ID: int64(i + 1), Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey,
			Status: service.StatusActive, Schedulable: true, Concurrency: 4, Priority: 10 - i,
			RateMultiplier: &rate,
			Credentials:    map[string]any{"api_key": "test-key", "base_url": "https://upstream.invalid"},
			Extra: map[string]any{service.UpstreamBillingProbeExtraKey: map[string]any{
				"status": "ok", "received_at": time.Now().Add(-time.Minute).UTC().Format(time.RFC3339Nano),
				"fresh_until": time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano),
				"data":        map[string]any{"billing_scope": "token", "resolved_rate_multiplier": rate, "effective_rate_multiplier": rate, "peak_rate_enabled": false},
			}},
		}
	}
	cfg := &config.Config{RunMode: config.RunModeSimple}
	cfg.Gateway.Scheduling.FallbackWaitTimeout = time.Second
	cfg.Gateway.Scheduling.FallbackMaxWaiting = 5
	repo := &openAIImagesFailoverAccountRepo{accounts: accounts}
	settings := service.NewSettingService(&contentModerationHandlerSettingRepo{values: map[string]string{
		"openai_advanced_scheduler_enabled":                    "false",
		service.SettingKeyOpenAILowUpstreamRatePriorityEnabled: "true",
	}}, cfg)
	stored, err := settings.GetAllSettings(context.Background())
	require.NoError(t, err)
	require.NoError(t, settings.UpdateSettings(context.Background(), stored))
	t.Cleanup(func() {
		stored.OpenAILowUpstreamRatePriorityEnabled = false
		// A request may still be finishing an asynchronous settings refresh.
		reset := service.NewSettingService(&contentModerationHandlerSettingRepo{}, cfg)
		require.NoError(t, reset.UpdateSettings(context.Background(), stored))
	})
	rateLimit := service.NewRateLimitService(repo, nil, cfg, nil, nil)
	rateLimit.SetSettingService(settings)
	concurrency := service.NewConcurrencyService(&fakeConcurrencyCache{})
	billing := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billing.Stop)
	gateway := service.NewOpenAIGatewayService(repo, &stickyUsageLogRepo{}, nil, nil, nil, nil, cache, cfg,
		nil, concurrency, service.NewBillingService(cfg, nil), rateLimit, billing, upstream,
		nil, nil, nil, nil, nil, nil, settings, nil)
	h := NewOpenAIGatewayHandler(gateway, concurrency, billing,
		service.NewAPIKeyService(nil, nil, nil, nil, nil, nil, cfg), nil, nil, nil, nil, cfg)
	h.maxAccountSwitches = 3
	apiKey := &service.APIKey{ID: 3, UserID: 4, GroupID: &group.ID, Group: group, Status: service.StatusActive,
		User: &service.User{ID: 4, Concurrency: 4, Balance: 100, Status: service.StatusActive}}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), ctxkey.Group, group))
		c.Set(string(middleware2.ContextKeyAPIKey), apiKey)
		c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 4, Concurrency: 4})
		c.Next()
	})
	router.POST("/v1/responses", h.Responses)
	router.POST("/v1/chat/completions", h.ChatCompletions)
	return router
}

func requestOpenAILegacySuccess(router *gin.Engine, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("session_id", "synthetic-sticky-session")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestOpenAILegacyStickySuccessHTTPFailover(t *testing.T) {
	for _, tc := range []struct{ name, path, body, response string }{
		{"responses", "/v1/responses", `{"model":"gpt-test","input":"hello","stream":false}`, `{"id":"resp_test","status":"completed","model":"gpt-test","output":[],"usage":{"input_tokens":1,"output_tokens":1}}`},
		{"chat", "/v1/chat/completions", `{"model":"gpt-test","messages":[{"role":"user","content":"hello"}],"stream":false}`, `{"id":"chat_test","model":"gpt-test","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ok := stickyScript{statusCode: http.StatusOK, jsonBody: tc.response}
			if tc.name == "chat" {
				ok = stickyScript{statusCode: http.StatusOK, streamBody: "data: " +
					`{"type":"response.completed","response":{"id":"resp_test","status":"completed","model":"gpt-test","output":[],"usage":{"input_tokens":1,"output_tokens":1}}}` + "\n\n"}
			}
			upstream := &stickyUpstream{scripts: map[int64][]stickyScript{1: {stickyFailScript(), ok}, 2: {ok}}}
			cache := newStickyGatewayTestCache()
			router := newOpenAILegacySuccessFixture(t, upstream, cache, true)
			first := requestOpenAILegacySuccess(router, tc.path, tc.body)
			require.Equal(t, http.StatusOK, first.Code, first.Body.String())
			require.Equal(t, []int64{1, 2}, upstream.hitOrder(), "low rate precedes account priority")
			require.EqualValues(t, 2, cache.preference(t).AccountID)
			upstream.resetHits()
			second := requestOpenAILegacySuccess(router, tc.path, tc.body)
			require.Equal(t, http.StatusOK, second.Code, second.Body.String())
			require.Equal(t, []int64{2}, upstream.hitOrder(), "successful fallback wins over recovered cheaper account")
			for key := range cache.bindings {
				require.True(t, strings.HasPrefix(key, "binding:7:openai:response:") ||
					strings.HasPrefix(key, "binding:7:openai:http-response-owner:"),
					"only the existing response ownership keys may be written: %s", key)
			}
		})
	}
}

func TestOpenAILegacyStickySuccessHTTPAllFail(t *testing.T) {
	upstream := &stickyUpstream{scripts: map[int64][]stickyScript{1: {stickyFailScript()}, 2: {stickyFailScript()}}}
	cache := newStickyGatewayTestCache()
	router := newOpenAILegacySuccessFixture(t, upstream, cache, true)
	rec := requestOpenAILegacySuccess(router, "/v1/responses", `{"model":"gpt-test","input":"hello"}`)
	require.GreaterOrEqual(t, rec.Code, 400)
	require.Equal(t, []int64{1, 2}, upstream.hitOrder())
	require.Empty(t, cache.keys())
}

// previous_response_id 绑定的请求不接入软偏好：那条归属链有自己的账号解析规则。
// 未开启利润控制的普通分组不再是边界，见 openai_legacy_normal_group_sticky_test.go。
func TestOpenAILegacyStickySuccessHTTPBoundary(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		profit     bool
	}{
		{name: "previous response", profit: true, body: `{"model":"gpt-test","input":"hello","previous_response_id":"resp_parent"}`},
		{name: "previous response without profit control", body: `{"model":"gpt-test","input":"hello","previous_response_id":"resp_parent"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream := &stickyUpstream{scripts: map[int64][]stickyScript{1: {{statusCode: 200, jsonBody: `{"id":"resp_test","model":"gpt-test","output":[]}`}}}}
			cache := newStickyGatewayTestCache()
			router := newOpenAILegacySuccessFixture(t, upstream, cache, tc.profit)
			requestOpenAILegacySuccess(router, "/v1/responses", tc.body)
			require.Empty(t, cache.successes)
		})
	}
}
