package handler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type quotaAdmissionRPMCache struct {
	service.UserRPMCache
	calls atomic.Int32
}

func (c *quotaAdmissionRPMCache) IncrementUserRPM(context.Context, int64) (int, error) {
	return int(c.calls.Add(1)), nil
}

func TestOpenAIResponsesWebSocketQuotaAdmissionAtFinalSlotAndNextTurn(t *testing.T) {
	for _, mode := range []string{service.OpenAIWSIngressModePassthrough, service.OpenAIWSIngressModeCtxPool} {
		for _, denyAt := range []int32{2, 3, 4} {
			t.Run(fmt.Sprintf("%s/deny_at_%d", mode, denyAt), func(t *testing.T) {
				gin.SetMode(gin.TestMode)
				var upstreamFrames atomic.Int32
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					conn, err := coderws.Accept(w, r, nil)
					if err != nil {
						return
					}
					defer func() { _ = conn.CloseNow() }()
					for {
						ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
						_, _, err := conn.Read(ctx)
						cancel()
						if err != nil {
							return
						}
						turn := upstreamFrames.Add(1)
						ctx, cancel = context.WithTimeout(r.Context(), 5*time.Second)
						err = conn.Write(ctx, coderws.MessageText, []byte(fmt.Sprintf(`{"type":"response.completed","response":{"id":"resp_quota_%d","model":"gpt-5.4","usage":{"input_tokens":2,"output_tokens":1}}}`, turn)))
						cancel()
						if err != nil {
							return
						}
					}
				}))
				defer upstream.Close()

				cfg := &config.Config{RunMode: config.RunModeSimple}
				cfg.Default.RateMultiplier = 1
				cfg.Security.URLAllowlist.AllowInsecureHTTP = true
				cfg.Gateway.OpenAIWS.Enabled = true
				cfg.Gateway.OpenAIWS.APIKeyEnabled = true
				cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
				cfg.Gateway.OpenAIWS.ModeRouterV2Enabled = true
				cfg.Gateway.OpenAIWS.DialTimeoutSeconds = 3
				cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 3
				cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 3
				// Exercise real billing admission while keeping settlement outside
				// this transport test (the gateway's simple mode records usage only).
				billingCfg := *cfg
				billingCfg.RunMode = "standard"
				rpm := &quotaAdmissionRPMCache{}
				billing := service.NewBillingCacheService(nil, nil, nil, nil, rpm, nil, &billingCfg, nil)
				defer billing.Stop()
				var admissions, userSlots, accountSlots atomic.Int32
				base := quotaTurnSubscription(10)
				billing.SetSubscriptionQuotaAdmission(func(_ context.Context, id int64, group *service.Group) (*service.UserSubscription, error) {
					count := admissions.Add(1)
					if count >= 2 && (userSlots.Load() == 0 || accountSlots.Load() == 0) {
						return nil, fmt.Errorf("final admission ran before concurrency slots")
					}
					if count == denyAt {
						return nil, service.ErrSubscriptionInvalid
					}
					fresh := service.CloneSubscriptionForRequest(base)
					fresh.Quota.DailyBucketID = int64(count * 100)
					return fresh, nil
				})
				cache := &concurrencyCacheMock{
					acquireUserSlotFn:    func(context.Context, int64, int, string) (bool, error) { userSlots.Add(1); return true, nil },
					acquireAccountSlotFn: func(context.Context, int64, int, string) (bool, error) { accountSlots.Add(1); return true, nil },
				}
				accountRepo := &openAIWSUsageHandlerAccountRepoStub{account: service.Account{
					ID: 9901, Name: "quota admission fixture", Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey,
					Status: service.StatusActive, Schedulable: true, Concurrency: 1,
					Credentials: map[string]any{"api_key": "fixture", "base_url": upstream.URL},
					Extra:       map[string]any{"openai_apikey_responses_websockets_v2_enabled": true, "openai_apikey_responses_websockets_v2_mode": mode},
				}}
				usageRepo := &openAIWSUsageHandlerUsageLogRepoStub{created: make(chan *service.UsageLog, 4)}
				gateway := service.NewOpenAIGatewayService(accountRepo, usageRepo, nil, nil, nil, nil, nil, cfg, nil, service.NewConcurrencyService(cache),
					service.NewBillingService(cfg, nil), nil, billing, nil, &service.DeferredService{}, nil, nil, nil, nil, nil, nil, nil)
				h := &OpenAIGatewayHandler{gatewayService: gateway, billingCacheService: billing, apiKeyService: &service.APIKeyService{},
					concurrencyHelper: NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatNone, time.Second)}
				group := &service.Group{ID: base.GroupID, Platform: service.PlatformOpenAI, SubscriptionType: service.SubscriptionTypeSubscription}
				key := &service.APIKey{ID: 101, GroupID: &group.ID, Group: group, User: &service.User{ID: base.UserID, Status: service.StatusActive, RPMLimit: 100}}
				router := gin.New()
				router.Use(func(c *gin.Context) {
					c.Set(string(middleware.ContextKeyAPIKey), key)
					c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: base.UserID, Concurrency: 1})
					c.Set(string(middleware.ContextKeySubscription), base)
					c.Next()
				})
				router.GET("/openai/v1/responses", h.ResponsesWebSocket)
				server := httptest.NewServer(router)
				defer server.Close()
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				client, _, err := coderws.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/openai/v1/responses", nil)
				require.NoError(t, err)
				defer func() { _ = client.CloseNow() }()
				payload := []byte(`{"type":"response.create","model":"gpt-5.4","stream":false}`)
				require.NoError(t, client.Write(ctx, coderws.MessageText, payload))
				for completed := int32(0); completed < denyAt-2; completed++ {
					_, event, err := client.Read(ctx)
					require.NoError(t, err)
					require.Contains(t, string(event), "response.completed")
					require.NoError(t, client.Write(ctx, coderws.MessageText, payload))
				}
				_, _, err = client.Read(ctx)
				require.Error(t, err)
				var closeErr coderws.CloseError
				require.ErrorAs(t, err, &closeErr)
				require.Equal(t, coderws.StatusPolicyViolation, closeErr.Code)
				require.Contains(t, closeErr.Reason, "billing check failed")
				require.Equal(t, denyAt, admissions.Load())
				require.Equal(t, denyAt-2, upstreamFrames.Load(), "a rejected turn must never reach the upstream")
				require.Equal(t, max(int32(1), denyAt-2), rpm.calls.Load(), "first-frame refresh must not increment RPM again; later admitted turns must")
			})
		}
	}
}
