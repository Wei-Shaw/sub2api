//go:build unit

package handler

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type compositeAdmissionCache struct {
	service.BillingCache
	service.UserRPMCache
	calls atomic.Int32
}

func (*compositeAdmissionCache) GetUserBalance(context.Context, int64) (float64, error) {
	return 100, nil
}

func (s *compositeAdmissionCache) IncrementUserRPM(context.Context, int64) (int, error) {
	return int(s.calls.Add(1)), nil
}

type compositeAdmissionUpstream struct {
	service.HTTPUpstream
	ids []int64
}

func (u *compositeAdmissionUpstream) Do(_ *http.Request, _ string, id int64, _ int) (*http.Response, error) {
	u.ids = append(u.ids, id)
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(bytes.NewBufferString(`{"id":"chatcmpl_test","object":"chat.completion","model":"deepseek-flash","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))}, nil
}

func TestCompositeHTTPFallbackAdmissionAndMixedLoadBatch(t *testing.T) {
	for _, endpoint := range []string{"chat/completions", "responses", "messages"} {
		for _, mode := range []string{"primary RPM1", "empty RPM2", "mixed cooldown RPM1"} {
			t.Run(endpoint+"/"+mode, func(t *testing.T) {
				h, repo, _, _, cleanup := newGrokCredentialFailoverHandler(t, "unused")
				defer cleanup()
				h.cfg.RunMode = config.RunModeStandard
				h.cfg.Gateway.Scheduling.LoadBatchEnabled = true
				cache := &compositeAdmissionCache{}
				billing := service.NewBillingCacheService(cache, nil, nil, nil, cache, nil, h.cfg, nil)
				defer billing.Stop()
				h.billingCacheService = billing
				concurrency := service.NewConcurrencyService(&concurrencyCacheMock{acquireAccountSlotFn: func(context.Context, int64, int, string) (bool, error) { return true, nil }})
				upstream := &compositeAdmissionUpstream{}
				logs := &openAIWSUsageHandlerUsageLogRepoStub{created: make(chan *service.UsageLog, 8)}
				h.gatewayService = service.NewOpenAIGatewayService(repo, logs, nil, nil, nil, nil, nil, h.cfg, nil, concurrency, service.NewBillingService(h.cfg, nil), nil, billing, upstream, &service.DeferredService{}, nil, nil, nil, nil, nil, nil, nil)
				repo.accounts = []service.Account{
					{ID: 801, Platform: service.PlatformOpenCodeGo, Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: mode != "empty RPM2", Concurrency: 1, Credentials: map[string]any{"api_key": "test", "base_url": "https://example.com/v1", "model_mapping": map[string]any{"deepseek-v4.1-flash": "deepseek-v4.1-flash"}}},
					{ID: 802, Platform: service.PlatformDeepseek, Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true, Concurrency: 1, Credentials: map[string]any{"api_key": "test", "base_url": "https://example.com/v1", "model_mapping": map[string]any{"deepseek-flash": "deepseek-flash"}}},
				}
				if mode == "mixed cooldown RPM1" {
					until := time.Now().Add(time.Hour)
					repo.accounts[0].RateLimitResetAt = &until
					repo.accounts = append(repo.accounts, service.Account{ID: 803, Platform: service.PlatformOpenCodeGo, Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true, Concurrency: 1, Credentials: map[string]any{"api_key": "test", "model_mapping": map[string]any{"mimo-v2.5-pro": "mimo-v2.5-pro"}}})
				}
				rpm := 1
				if mode == "empty RPM2" {
					rpm = 2
				}
				groupID := int64(901)
				if mode == "mixed cooldown RPM1" {
					ctx := service.WithCompositeRouteDecision(context.Background(), compositeHTTPTestDecision())
					_, _, err := h.gatewayService.SelectAccountWithSchedulerForCapability(ctx, &groupID, "", "", "deepseek-v4.1-flash", nil, service.OpenAIUpstreamTransportAny, service.OpenAIEndpointCapabilityChatCompletions, false, false, false, service.PlatformOpenCodeGo)
					require.ErrorIs(t, err, service.ErrNoAvailableAccounts)
					require.Contains(t, err.Error(), "model_not_supported")
				}
				key := &service.APIKey{ID: 902, GroupID: &groupID, User: &service.User{ID: 903, Status: service.StatusActive, RPMLimit: rpm}, Group: &service.Group{ID: groupID, Platform: service.PlatformComposite, Status: service.StatusActive}}
				router := gin.New()
				router.Use(func(c *gin.Context) {
					c.Set(string(middleware.ContextKeyAPIKey), key)
					c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: key.User.ID, Concurrency: 1})
					c.Request = c.Request.WithContext(service.WithCompositeRouteDecision(c.Request.Context(), compositeHTTPTestDecision()))
				})
				router.POST("/v1/chat/completions", h.ChatCompletions)
				router.POST("/v1/responses", h.Responses)
				router.POST("/v1/messages", h.Messages)
				payload := `{"model":"deepseek-v4.1-flash","messages":[{"role":"user","content":"hello"}],"max_tokens":10}`
				if endpoint == "responses" {
					payload = `{"model":"deepseek-v4.1-flash","input":"hello"}`
				}
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, httptest.NewRequest("POST", "/v1/"+endpoint, bytes.NewBufferString(payload)))
				require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
				want := int64(802)
				if mode == "primary RPM1" {
					want = 801
				}
				require.Equal(t, []int64{want}, upstream.ids)
				require.EqualValues(t, 1, cache.calls.Load())
			})
		}
	}
}
