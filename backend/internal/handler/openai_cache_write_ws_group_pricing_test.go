package handler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// Exercise the merged handler hooks together: a new turn refreshes its group
// pricing and profit gate, while a later cache hit reconciles the prior turn
// using the pricing captured by its original bill. Only authentication,
// persistence and the upstream are fixtures; prompt/output evidence, inference,
// pricing, billing commands and the asynchronous usage worker are production.
func TestOpenAICacheWriteWebSocket_GroupPricingRefreshKeepsOriginalBill(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// These cases change the process-wide inference setting and default HTTP
	// client used by the OAuth WS dialer. Do not run them in parallel.
	for _, mode := range []string{service.OpenAIWSIngressModePassthrough, service.OpenAIWSIngressModeCtxPool} {
		for _, tc := range []struct {
			name                string
			disabled            bool
			rejectSecond        bool
			retryFirstDial      bool
			stripFirstReference bool
			resetEpoch          bool
		}{
			{name: "reconcile_at_original_rate"},
			{name: "refreshed_profit_gate_rejects", rejectSecond: true},
			{name: "disabled_does_not_reconcile", disabled: true},
			{name: "first_dial_failure_retries_with_fresh_admission", retryFirstDial: true},
			{name: "first_reference_stripped_before_admission", stripFirstReference: true},
			{name: "first_reference_stays_stripped_after_dial_retry", stripFirstReference: true, retryFirstDial: true},
			{name: "off_on_does_not_bridge_first_turn", resetEpoch: true},
		} {
			t.Run(mode+"/"+tc.name, func(t *testing.T) {
				const (
					initialBalance  = 10.0
					originalRate    = 3.0
					refreshedRate   = 0.3
					firstRequestID  = "resp_cache_write_ws_1"
					secondRequestID = "resp_cache_write_ws_2"
					payload         = `{"type":"response.create","model":"gpt-6-astra","stream":true,"instructions":"Local simulation only","prompt_cache_key":"cache-write-ws-group-pricing","input":[{"role":"user","content":"same explicit prompt"}]}`
				)
				cfg := &config.Config{}
				cfg.Default.RateMultiplier = 1
				cfg.Security.URLAllowlist.Enabled = false
				cfg.Gateway.MaxAccountSwitches = 1
				cfg.Gateway.OpenAIWS.Enabled = true
				cfg.Gateway.OpenAIWS.OAuthEnabled = true
				cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
				cfg.Gateway.OpenAIWS.ModeRouterV2Enabled = true
				cfg.Gateway.OpenAIWS.DialTimeoutSeconds = 3
				cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 3
				cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 3

				settings := service.NewSettingService(&contentModerationHandlerSettingRepo{}, cfg)
				values, err := settings.GetAllSettings(context.Background())
				require.NoError(t, err)
				originalEnabled := values.OpenAICacheWriteInferenceEnabled
				values.OpenAICacheWriteInferenceEnabled = !tc.disabled
				require.NoError(t, settings.UpdateSettings(context.Background(), values))
				require.Equal(t, !tc.disabled, settings.IsOpenAICacheWriteInferenceEnabled(context.Background()))
				t.Cleanup(func() {
					values.OpenAICacheWriteInferenceEnabled = originalEnabled
					require.NoError(t, settings.UpdateSettings(context.Background(), values))
				})

				keyRepo := newWSGroupPricingAPIKeyRepoStub(originalRate)
				keyRepo.apiKey.User.Balance = initialBalance
				keyRepo.group.ProfitControlEnabled = true
				keyRepo.group.ProfitMinMargin = 0.1
				keyService := newWSGroupPricingAPIKeyService(keyRepo)
				apiKey, err := keyService.GetByKey(context.Background(), wsGroupPricingKey)
				require.NoError(t, err)
				ledger := &cacheWriteHTTPTestLedger{
					balance: initialBalance,
					logs:    make(map[string]service.UsageLog),
					seen:    make(map[string]string),
				}
				userRepo := &cacheWriteHTTPTestUserRepo{ledger: ledger, userID: apiKey.User.ID}
				billingCache := service.NewBillingCacheService(nil, userRepo, nil, nil, nil, nil, cfg, nil)
				t.Cleanup(billingCache.Stop)

				var upstreamTurns, upstreamDials atomic.Int32
				upstreamErrors := make(chan error, 4)
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Header.Get("Authorization") != "Bearer cache-write-ws-test-token" {
						upstreamErrors <- fmt.Errorf("unexpected upstream authorization")
						http.Error(w, "invalid fixture request", http.StatusBadRequest)
						return
					}
					if upstreamDials.Add(1) == 1 && tc.retryFirstDial {
						// No AfterTurn runs for a failed handshake. A retry must
						// discard this attempt's receipt and admit the first turn anew.
						http.Error(w, "transient rate limit", http.StatusTooManyRequests)
						return
					}
					conn, acceptErr := coderws.Accept(w, r, &coderws.AcceptOptions{CompressionMode: coderws.CompressionContextTakeover})
					if acceptErr != nil {
						upstreamErrors <- acceptErr
						return
					}
					defer func() { _ = conn.CloseNow() }()
					for {
						_, body, readErr := conn.Read(r.Context())
						if readErr != nil {
							return // Handler/pool cleanup closes the local connection.
						}
						turn := upstreamTurns.Add(1)
						if turn > 2 || gjson.GetBytes(body, "type").String() != "response.create" ||
							gjson.GetBytes(body, "model").String() != "gpt-6-astra" ||
							!gjson.GetBytes(body, "input").IsArray() {
							upstreamErrors <- fmt.Errorf("unexpected upstream turn %d: %s", turn, body)
							return
						}
						if tc.stripFirstReference && turn == 1 && gjson.GetBytes(body, "previous_response_id").Exists() {
							upstreamErrors <- fmt.Errorf("first turn retained a foreign previous_response_id: %s", body)
							return
						}
						cached := 0
						if turn == 2 {
							cached = 9984
						}
						// Explicit completed output and full prompt history are real
						// evidence. An absent write field must remain absent, not zero.
						response := fmt.Sprintf(`{"type":"response.completed","response":{"id":"resp_cache_write_ws_%d","model":"gpt-6-astra","status":"completed","output":[],"usage":{"input_tokens":10818,"output_tokens":0,"total_tokens":10818,"input_tokens_details":{"cached_tokens":%d}}}}`, turn, cached)
						writeCtx, cancelWrite := context.WithTimeout(r.Context(), 3*time.Second)
						writeErr := conn.Write(writeCtx, coderws.MessageText, []byte(response))
						cancelWrite()
						if writeErr != nil {
							upstreamErrors <- writeErr
							return
						}
					}
				}))
				t.Cleanup(upstream.Close)
				target, err := url.Parse(upstream.URL)
				require.NoError(t, err)
				localTransport := &http.Transport{Proxy: nil}
				previousClient := http.DefaultClient
				http.DefaultClient = &http.Client{
					Transport:     &cacheWriteWSLocalTransport{target: target, transport: localTransport},
					CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
				}
				t.Cleanup(func() {
					http.DefaultClient = previousClient
					localTransport.CloseIdleConnections()
				})

				accountRate := 0.1 // Below the refreshed threshold: 0.3 * (1 - 0.1).
				if tc.rejectSecond {
					accountRate = 1 // Admitted at 3.0, rejected at 0.3.
				}
				accountRepo := &openAIWSUsageHandlerAccountRepoStub{account: service.Account{
					ID: 9901, Name: "cache-write-ws-group-pricing", Platform: service.PlatformOpenAI,
					Type: service.AccountTypeOAuth, Status: service.StatusActive, Schedulable: true,
					Concurrency: 1, GroupIDs: []int64{wsGroupPricingGroupID}, RateMultiplier: &accountRate,
					Credentials: map[string]any{"access_token": "cache-write-ws-test-token", "chatgpt_account_id": "cache-write-ws-test-account"},
					Extra: map[string]any{
						"openai_oauth_responses_websockets_v2_enabled": true,
						"openai_oauth_responses_websockets_v2_mode":    mode,
					},
				}}
				billing := service.NewBillingService(cfg, nil)
				gateway := service.NewOpenAIGatewayService(
					accountRepo, &cacheWriteHTTPTestUsageRepo{ledger: ledger}, &cacheWriteHTTPTestBillingRepo{ledger: ledger},
					userRepo, nil, nil, nil, cfg, nil, nil, billing, nil, billingCache, nil,
					&service.DeferredService{}, nil, nil, service.NewModelPricingResolver(nil, billing), nil, nil, settings, nil,
				)
				t.Cleanup(gateway.CloseOpenAIWSPool)
				pool := service.NewUsageRecordWorkerPoolWithOptions(service.UsageRecordWorkerPoolOptions{
					WorkerCount: 1, QueueSize: 8, TaskTimeout: 5 * time.Second,
					OverflowPolicy: config.UsageRecordOverflowPolicySync,
				})
				t.Cleanup(pool.Stop)
				cache := &concurrencyCacheMock{
					acquireUserSlotFn:    func(context.Context, int64, int, string) (bool, error) { return true, nil },
					acquireAccountSlotFn: func(context.Context, int64, int, string) (bool, error) { return true, nil },
				}
				h := NewOpenAIGatewayHandler(gateway, service.NewConcurrencyService(cache), billingCache, keyService, pool, nil, nil, nil, cfg)
				router := gin.New()
				router.Use(func(c *gin.Context) {
					c.Set(string(middleware.ContextKeyAPIKey), apiKey)
					c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: apiKey.User.ID, Concurrency: 1})
					c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), ctxkey.Group, apiKey.Group))
					c.Next()
				})
				handlerDone := make(chan struct{})
				router.GET("/openai/v1/responses", func(c *gin.Context) {
					defer close(handlerDone)
					h.ResponsesWebSocket(c)
				})
				server := httptest.NewServer(router)
				t.Cleanup(server.Close)
				dialCtx, cancelDial := context.WithTimeout(context.Background(), 3*time.Second)
				client, _, err := coderws.Dial(dialCtx, "ws"+strings.TrimPrefix(server.URL, "http")+"/openai/v1/responses",
					&coderws.DialOptions{HTTPClient: server.Client(), CompressionMode: coderws.CompressionContextTakeover})
				cancelDial()
				require.NoError(t, err)
				waitForHandler := func() {
					t.Helper()
					select {
					case <-handlerDone:
					case <-time.After(5 * time.Second):
						t.Error("websocket handler did not finish after client close")
					}
				}
				t.Cleanup(func() {
					_ = client.CloseNow()
					waitForHandler()
				})
				writeTurn := func(body string) {
					t.Helper()
					ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
					defer cancel()
					require.NoError(t, client.Write(ctx, coderws.MessageText, []byte(body)))
				}
				readCompleted := func(turn int, requestID string) {
					t.Helper()
					ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
					defer cancel()
					_, event, readErr := client.Read(ctx)
					require.NoError(t, readErr)
					require.Equal(t, "response.completed", gjson.GetBytes(event, "type").String())
					require.Equal(t, requestID, gjson.GetBytes(event, "response.id").String())
					// AfterTurn observes completion and saves the original billing
					// snapshot before the next request starts. No timing-only sleeps.
					require.Eventually(t, func() bool { return pool.Stats().CompletedTasks >= uint64(turn) },
						5*time.Second, time.Millisecond, "usage worker did not complete")
				}

				firstPayload := payload
				if tc.stripFirstReference {
					firstPayload = strings.TrimSuffix(payload, "}") + `,"previous_response_id":"resp_foreign_account"}`
				}
				writeTurn(firstPayload)
				readCompleted(1, firstRequestID)
				first, ok := ledger.log(firstRequestID, apiKey.ID)
				require.True(t, ok)
				require.Equal(t, 10818, first.InputTokens)
				require.Zero(t, first.CacheCreationTokens)
				require.InDelta(t, originalRate, first.RateMultiplier, 1e-12)
				require.InDelta(t, 0.10818*originalRate, first.ActualCost, 1e-12)
				beforeSecond := ledger.currentBalance()
				require.InDelta(t, initialBalance-first.ActualCost, beforeSecond, 1e-12)
				require.NoError(t, keyRepo.adminUpdate(keyService, func(_ *service.APIKey, group *service.Group) {
					group.RateMultiplier = refreshedRate
				}))
				if tc.resetEpoch {
					values.OpenAICacheWriteInferenceEnabled = false
					require.NoError(t, settings.UpdateSettings(context.Background(), values))
					values.OpenAICacheWriteInferenceEnabled = true
					require.NoError(t, settings.UpdateSettings(context.Background(), values))
				}

				writeTurn(payload) // The existing client connection must see the new group.
				if tc.rejectSecond {
					ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
					_, _, readErr := client.Read(ctx)
					cancel()
					var closeErr coderws.CloseError
					require.ErrorAs(t, readErr, &closeErr)
					require.Equal(t, coderws.StatusTryAgainLater, closeErr.Code)
					require.Contains(t, closeErr.Reason, "no longer eligible for this connection")
					waitForHandler()
					pool.Stop()
					require.Equal(t, int32(1), upstreamTurns.Load(), "rejected second turn must not reach upstream")
					require.Equal(t, uint64(1), pool.Stats().SubmittedTasks)
					require.Empty(t, ledger.adjustments(), "a rejected turn supplies no cache-hit evidence")
					require.InDelta(t, beforeSecond, ledger.currentBalance(), 1e-12)
				} else {
					readCompleted(2, secondRequestID)
					first, ok = ledger.log(firstRequestID, apiKey.ID)
					require.True(t, ok)
					second, ok := ledger.log(secondRequestID, apiKey.ID)
					require.True(t, ok)
					require.Equal(t, int32(2), upstreamTurns.Load())
					require.Equal(t, uint64(2), pool.Stats().SubmittedTasks)
					require.InDelta(t, refreshedRate, second.RateMultiplier, 1e-12)
					require.Equal(t, 834, second.InputTokens)
					require.Equal(t, 9984, second.CacheReadTokens)
					require.Zero(t, second.CacheCreationTokens)
					require.InDelta(t, 0.018324*refreshedRate, second.ActualCost, 1e-12,
						"R2 normal billing must use its newly refreshed group price")
					adjustments := ledger.adjustments()
					if tc.disabled || tc.resetEpoch {
						require.Empty(t, adjustments, "disabled or stale-epoch first turns must not be reconciled")
						require.Equal(t, 10818, first.InputTokens)
						require.Zero(t, first.CacheCreationTokens)
						require.InDelta(t, 0.10818*originalRate, first.ActualCost, 1e-12)
					} else {
						require.Len(t, adjustments, 1)
						require.Equal(t, firstRequestID, adjustments[0].CacheWriteCorrection.RequestID)
						require.Equal(t, apiKey.ID, adjustments[0].CacheWriteCorrection.APIKeyID)
						require.Equal(t, 834, first.InputTokens)
						require.Equal(t, 9984, first.CacheCreationTokens)
						require.Zero(t, first.CacheReadTokens)
						require.InDelta(t, originalRate, first.RateMultiplier, 1e-12)
						require.InDelta(t, 0.00834, first.InputCost, 1e-12)
						require.InDelta(t, 0.1248, first.CacheCreationCost, 1e-12)
						require.InDelta(t, 0.13314*originalRate, first.ActualCost, 1e-12)
						require.InDelta(t, 0.02496*originalRate, adjustments[0].BalanceCost, 1e-12,
							"R1 inferred surcharge must use R1's frozen price, not R2's refreshed price")
						require.InDelta(t, adjustments[0].BalanceCost, beforeSecond-ledger.currentBalance()-second.ActualCost, 1e-12)
					}
					require.InDelta(t, first.ActualCost+second.ActualCost, initialBalance-ledger.currentBalance(), 1e-12)
				}
				if tc.retryFirstDial {
					require.Equal(t, int32(2), upstreamDials.Load(), "the failed first handshake must retry once")
				}
				require.GreaterOrEqual(t, keyRepo.lookupCount(), 2, "the second turn must refresh the authentication snapshot")
				require.Zero(t, pool.Stats().FailedTasks)
				require.Zero(t, pool.Stats().SyncFallbackTasks)
				select {
				case upstreamErr := <-upstreamErrors:
					require.NoError(t, upstreamErr)
				default:
				}
			})
		}
	}
}

// OAuth WS endpoints are fixed in production. Redirect their actual HTTP
// upgrade to the local fixture before dialing, without replacing the WS parser,
// admission hooks or billing. Unknown destinations fail closed.
type cacheWriteWSLocalTransport struct {
	target    *url.URL
	transport *http.Transport
}

func (t *cacheWriteWSLocalTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Scheme != "https" || req.URL.Host != "chatgpt.com" || req.URL.Path != "/backend-api/codex/responses" {
		return nil, fmt.Errorf("unexpected cache-write WS destination: %s", req.URL)
	}
	local := req.Clone(req.Context())
	local.URL.Scheme = t.target.Scheme
	local.URL.Host = t.target.Host
	local.Host = t.target.Host
	return t.transport.RoundTrip(local)
}
