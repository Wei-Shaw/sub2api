package handler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIWSHandshakeFailover(t *testing.T) {
	for _, mode := range []string{service.OpenAIWSIngressModeCtxPool, service.OpenAIWSIngressModePassthrough} {
		t.Run(mode, func(t *testing.T) {
			for _, tc := range []struct {
				name             string
				statuses         []int
				body             string
				poolMode         bool
				retryCount       int
				wantAccounts     []int64
				wantClose        bool
				cancelAfterFirst bool
				earlyNextFrame   bool
			}{
				{name: "unauthorized", statuses: []int{401, 101}, wantAccounts: []int64{801, 802}},
				{name: "forbidden", statuses: []int{403, 101}, wantAccounts: []int64{801, 802}},
				{name: "html_forbidden", statuses: []int{403, 101}, body: "<html>Access denied</html>", wantAccounts: []int64{801, 802}},
				{name: "internal_error", statuses: []int{500, 101}, wantAccounts: []int64{801, 802}},
				{name: "bad_gateway", statuses: []int{502, 101}, wantAccounts: []int64{801, 802}},
				{name: "unavailable", statuses: []int{503, 101}, wantAccounts: []int64{801, 802}},
				{name: "gateway_timeout", statuses: []int{504, 101}, wantAccounts: []int64{801, 802}},
				{name: "quota", statuses: []int{429, 101}, body: `{"error":{"code":"insufficient_quota","message":"quota exhausted"}}`, wantAccounts: []int64{801, 802}},
				{name: "same_account_recovers", statuses: []int{503, 503, 101}, poolMode: true, retryCount: 2, wantAccounts: []int64{801, 801, 801}},
				{name: "same_account_exhausts_before_switch", statuses: []int{503, 503, 503, 101}, poolMode: true, retryCount: 2, wantAccounts: []int64{801, 801, 801, 802}},
				{name: "zero_disables_same_account_retry", statuses: []int{503, 101}, poolMode: true, wantAccounts: []int64{801, 802}},
				{name: "switch_budget_exhausted", statuses: []int{503, 503}, wantAccounts: []int64{801, 802}, wantClose: true},
				{name: "client_cancels_during_retry", statuses: []int{503}, poolMode: true, retryCount: 2, wantAccounts: []int64{801}, cancelAfterFirst: true},
				{name: "early_next_frame_survives_retry", statuses: []int{503, 101}, poolMode: true, retryCount: 2, wantAccounts: []int64{801, 801}, earlyNextFrame: true},
				{name: "invalid_request", statuses: []int{400}, body: `{"error":{"code":"invalid_request_error","message":"bad parameter"}}`, wantAccounts: []int64{801}, wantClose: true},
				{name: "cyber_policy_wrapped_in_503", statuses: []int{503}, body: `{"error":{"code":"cyber_policy","message":"blocked by cyber policy"}}`, wantAccounts: []int64{801}, wantClose: true},
			} {
				t.Run(tc.name, func(t *testing.T) {
					gin.SetMode(gin.TestMode)
					var mu sync.Mutex
					var accounts []int64
					var requests [][]byte
					firstHandshake := make(chan struct{}, 1)
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						var id int64
						if _, err := fmt.Sscanf(r.Header.Get("Authorization"), "Bearer sk-%d", &id); err != nil {
							t.Error(err)
							w.WriteHeader(http.StatusUnauthorized)
							return
						}
						mu.Lock()
						accounts = append(accounts, id)
						attempt := len(accounts)
						mu.Unlock()
						if attempt > len(tc.statuses) {
							t.Errorf("unexpected handshake attempt %d", attempt)
							w.WriteHeader(http.StatusBadRequest)
							return
						}
						status := tc.statuses[attempt-1]
						if status != http.StatusSwitchingProtocols {
							w.Header().Set("Content-Type", "application/json")
							w.WriteHeader(status)
							body := tc.body
							if body == "" {
								body = fmt.Sprintf(`{"error":{"message":%q}}`, http.StatusText(status))
							}
							_, _ = w.Write([]byte(body))
							if attempt == 1 {
								firstHandshake <- struct{}{}
							}
							return
						}
						conn, err := coderws.Accept(w, r, nil)
						if err != nil {
							t.Error(err)
							return
						}
						defer func() { _ = conn.CloseNow() }()
						_, payload, err := conn.Read(r.Context())
						if err != nil {
							t.Error(err)
							return
						}
						mu.Lock()
						requests = append(requests, payload)
						mu.Unlock()
						_ = conn.Write(r.Context(), coderws.MessageText, []byte(`{"type":"response.completed","response":{"id":"resp_handshake_ok","model":"gpt-5.1","output":[],"usage":{"input_tokens":1,"output_tokens":1}}}`))
						if tc.earlyNextFrame {
							_, payload, err = conn.Read(r.Context())
							if err != nil {
								t.Error(err)
								return
							}
							mu.Lock()
							requests = append(requests, payload)
							mu.Unlock()
							_ = conn.Write(r.Context(), coderws.MessageText, []byte(`{"type":"response.completed","response":{"id":"resp_handshake_next","model":"gpt-5.1","output":[],"usage":{"input_tokens":1,"output_tokens":1}}}`))
						}
						_, _, _ = conn.Read(r.Context())
					}))
					t.Cleanup(upstream.Close)
					repo := &openAIWSTurnBudgetAccountRepo{}
					for i := 0; i < 3; i++ {
						id := int64(801 + i)
						repo.accounts = append(repo.accounts, service.Account{
							ID: id, Name: fmt.Sprintf("ws-handshake-%d", id), Platform: service.PlatformOpenAI,
							Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true, Concurrency: 1, Priority: i + 1,
							Credentials: map[string]any{"api_key": fmt.Sprintf("sk-%d", id), "base_url": upstream.URL, "pool_mode": tc.poolMode, "pool_mode_retry_count": tc.retryCount, "pool_mode_retry_status_codes": []any{503}},
							Extra: map[string]any{
								"openai_apikey_responses_websockets_v2_enabled": true,
								"openai_apikey_responses_websockets_v2_mode":    mode,
							},
						})
					}
					conn, finish := newOpenAIWSHandshakeTestClient(t, repo, 1)
					ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
					defer cancel()
					require.NoError(t, conn.Write(ctx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.1","instructions":"handshake-retry","input":"hello"}`)))
					if tc.cancelAfterFirst || tc.earlyNextFrame {
						select {
						case <-firstHandshake:
						case <-ctx.Done():
							t.Fatal("first handshake did not arrive")
						}
					}
					if tc.cancelAfterFirst {
						finish()
						mu.Lock()
						defer mu.Unlock()
						require.Equal(t, tc.wantAccounts, accounts)
						return
					}
					if tc.earlyNextFrame {
						require.NoError(t, conn.Write(ctx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.1","input":"next turn"}`)))
					}
					_, payload, err := conn.Read(ctx)
					if tc.wantClose {
						var closeErr coderws.CloseError
						require.ErrorAs(t, err, &closeErr)
					} else {
						require.NoError(t, err)
						require.Equal(t, "resp_handshake_ok", gjson.GetBytes(payload, "response.id").String())
					}
					if tc.earlyNextFrame {
						_, payload, err = conn.Read(ctx)
						require.NoError(t, err)
						require.Equal(t, "resp_handshake_next", gjson.GetBytes(payload, "response.id").String())
					}
					finish()
					mu.Lock()
					defer mu.Unlock()
					require.Equal(t, tc.wantAccounts, accounts)
					if !tc.wantClose {
						wantRequests := 1
						if tc.earlyNextFrame {
							wantRequests = 2
						}
						require.Len(t, requests, wantRequests, "rejected handshakes must not send the model request")
						require.Equal(t, "handshake-retry", gjson.GetBytes(requests[0], "instructions").String())
						if tc.earlyNextFrame {
							require.Equal(t, "next turn", gjson.GetBytes(requests[1], "input").String())
						}
					}
				})
			}
		})
	}
}

func newOpenAIWSHandshakeTestClient(t *testing.T, repo service.AccountRepository, maxSwitches int, configure ...func(*config.Config)) (*coderws.Conn, func()) {
	t.Helper()
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
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
	cfg.Gateway.MaxAccountSwitches = maxSwitches
	for _, fn := range configure {
		fn(cfg)
	}
	billing := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billing.Stop)
	gateway := service.NewOpenAIGatewayService(repo, nil, nil, nil, nil, nil, nil, cfg, nil, nil,
		service.NewBillingService(cfg, nil), nil, billing, openAIWSTurnBudgetHTTPClient{},
		&service.DeferredService{}, nil, nil, nil, nil, nil, nil, nil)
	t.Cleanup(gateway.CloseOpenAIWSPool)
	var userAcquires, accountAcquires atomic.Int32
	cache := &concurrencyCacheMock{
		acquireUserSlotFn: func(context.Context, int64, int, string) (bool, error) {
			userAcquires.Add(1)
			return true, nil
		},
		acquireAccountSlotFn: func(context.Context, int64, int, string) (bool, error) {
			accountAcquires.Add(1)
			return true, nil
		},
	}
	h := NewOpenAIGatewayHandler(gateway, service.NewConcurrencyService(cache), billing, &service.APIKeyService{}, nil, nil, nil, nil, cfg)
	groupID := int64(4202)
	apiKey := &service.APIKey{
		ID: 1802, GroupID: &groupID, User: &service.User{ID: 1702, Status: service.StatusActive},
		Group: &service.Group{ID: groupID, Platform: service.PlatformOpenAI, Status: service.StatusActive},
	}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyAPIKey), apiKey)
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: apiKey.User.ID, Concurrency: 1})
		c.Next()
	})
	finished := make(chan struct{})
	router.GET("/openai/v1/responses", func(c *gin.Context) {
		h.ResponsesWebSocket(c)
		close(finished)
	})
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, _, err := coderws.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/openai/v1/responses", nil)
	require.NoError(t, err)
	finish := func() {
		_ = conn.CloseNow()
		select {
		case <-finished:
		case <-time.After(3 * time.Second):
			t.Fatal("websocket handler did not exit")
		}
		require.Equal(t, userAcquires.Load(), atomic.LoadInt32(&cache.releaseUserCalled), "user slots must be released")
		require.Equal(t, accountAcquires.Load(), atomic.LoadInt32(&cache.releaseAccountCalled), "account slots must be released")
	}
	t.Cleanup(finish)
	return conn, finish
}
