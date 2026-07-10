package handler

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const testOpenAIAttestationEnvelope = `{"v":1,"s":0,"t":"v1.handler-opaque"}`

func TestOpenAIResponsesAttestationStopsCrossAccountFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name        string
		attestation string
		wantCalls   int
		wantStatus  int
	}{
		{name: "携带证明时停止跨账号切换", attestation: testOpenAIAttestationEnvelope, wantCalls: 1, wantStatus: http.StatusTooManyRequests},
		{name: "普通请求保持既有故障转移", wantCalls: 2, wantStatus: http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			accounts := []service.Account{
				{
					ID:          9911,
					Name:        "oauth-rate-limited",
					Platform:    service.PlatformOpenAI,
					Type:        service.AccountTypeOAuth,
					Status:      service.StatusActive,
					Schedulable: true,
					Concurrency: 1,
					Priority:    1,
					Credentials: map[string]any{"access_token": "oauth-first", "chatgpt_account_id": "account-first"},
				},
				{
					ID:          9912,
					Name:        "oauth-healthy",
					Platform:    service.PlatformOpenAI,
					Type:        service.AccountTypeOAuth,
					Status:      service.StatusActive,
					Schedulable: true,
					Concurrency: 1,
					Priority:    2,
					Credentials: map[string]any{"access_token": "oauth-second", "chatgpt_account_id": "account-second"},
				},
			}
			accountRepo := &openAIWSFailoverHandlerAccountRepoStub{accounts: accounts}
			upstream := &openAIAttestationFailoverHTTPUpstream{}
			cfg := &config.Config{RunMode: config.RunModeSimple}
			cfg.Default.RateMultiplier = 1
			cfg.Security.URLAllowlist.Enabled = false
			cfg.Gateway.MaxAccountSwitches = 3

			rateLimitSvc := service.NewRateLimitService(accountRepo, nil, cfg, nil, nil)
			billingCacheSvc := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
			gatewaySvc := service.NewOpenAIGatewayService(
				accountRepo,
				nil,
				nil,
				nil,
				nil,
				nil,
				nil,
				cfg,
				nil,
				nil,
				service.NewBillingService(cfg, nil),
				rateLimitSvc,
				billingCacheSvc,
				upstream,
				&service.DeferredService{},
				nil,
				nil,
				nil,
				nil,
				nil,
				nil,
				nil,
			)

			cache := &concurrencyCacheMock{
				acquireUserSlotFn:    func(context.Context, int64, int, string) (bool, error) { return true, nil },
				acquireAccountSlotFn: func(context.Context, int64, int, string) (bool, error) { return true, nil },
			}
			h := &OpenAIGatewayHandler{
				gatewayService:      gatewaySvc,
				billingCacheService: billingCacheSvc,
				apiKeyService:       &service.APIKeyService{},
				concurrencyHelper:   NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatNone, time.Second),
				maxAccountSwitches:  3,
			}

			groupID := int64(4211)
			apiKey := &service.APIKey{
				ID:      1811,
				GroupID: &groupID,
				User:    &service.User{ID: 1711, Status: service.StatusActive},
				Group:   &service.Group{ID: groupID, Platform: service.PlatformOpenAI, Status: service.StatusActive},
			}
			router := gin.New()
			router.Use(func(c *gin.Context) {
				c.Set(string(middleware.ContextKeyAPIKey), apiKey)
				c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: apiKey.User.ID, Concurrency: 1})
				c.Next()
			})
			router.POST("/openai/v1/responses", h.Responses)

			req := httptest.NewRequest(http.MethodPost, "/openai/v1/responses", strings.NewReader(`{"model":"gpt-5.1","stream":false,"input":"test"}`))
			req.Header.Set("Content-Type", "application/json")
			if tt.attestation != "" {
				req.Header["X-OAI-Attestation"] = []string{tt.attestation}
			}
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, req)

			require.Equal(t, tt.wantStatus, recorder.Code)
			require.Equal(t, tt.wantCalls, upstream.CallCount())
			requests := upstream.Requests()
			require.Len(t, requests, tt.wantCalls)
			if tt.attestation != "" {
				require.Equal(t, []string{tt.attestation}, requests[0].Header.Values("X-OAI-Attestation"))
			} else {
				for _, upstreamReq := range requests {
					require.Empty(t, upstreamReq.Header.Values("X-OAI-Attestation"))
				}
				require.Contains(t, recorder.Body.String(), "resp_second_account")
			}
		})
	}
}

type openAIAttestationFailoverHTTPUpstream struct {
	mu       sync.Mutex
	requests []*http.Request
}

func (u *openAIAttestationFailoverHTTPUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	u.mu.Lock()
	cloned := req.Clone(req.Context())
	cloned.Header = req.Header.Clone()
	u.requests = append(u.requests, cloned)
	call := len(u.requests)
	u.mu.Unlock()

	if call == 1 {
		return &http.Response{
			StatusCode: http.StatusTooManyRequests,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"error":{"code":"rate_limit_exceeded","message":"rate limited"}}`)),
		}, nil
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(
			`{"id":"resp_second_account","object":"response","model":"gpt-5.1","output":[],"usage":{"input_tokens":1,"output_tokens":1}}`,
		)),
	}, nil
}

func (u *openAIAttestationFailoverHTTPUpstream) DoWithTLS(
	req *http.Request,
	proxyURL string,
	accountID int64,
	accountConcurrency int,
	_ *tlsfingerprint.Profile,
) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, accountConcurrency)
}

func (u *openAIAttestationFailoverHTTPUpstream) CallCount() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return len(u.requests)
}

func (u *openAIAttestationFailoverHTTPUpstream) Requests() []*http.Request {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]*http.Request(nil), u.requests...)
}
