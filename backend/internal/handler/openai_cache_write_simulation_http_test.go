package handler

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// This is an HTTP integration simulation, not a production/SQL test. It runs
// the real Responses handler, OAuth SSE forwarding/parser, inference, pricing,
// RecordUsage and async worker pool. Authentication, settings, account storage,
// usage persistence and the balance ledger are in-memory fixtures. All outbound
// requests are rerouted to the localhost fake upstream below.
func TestOpenAICacheWriteSimulationHTTP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name           string
		enabled        bool
		disableOnTurn2 bool
		officialZero   bool
		divergent      bool
		wantInferred   bool
	}{
		{name: "missing_write_bills_prior_turn", enabled: true, wantInferred: true},
		{name: "toggle_off_does_not_infer"},
		{name: "toggle_off_between_turns_clears_lineage", enabled: true, disableOnTurn2: true},
		{name: "official_zero_is_authoritative", enabled: true, officialZero: true},
		{name: "same_session_divergent_prompt_skips", enabled: true, divergent: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const (
				userID    int64 = 91001
				keyID     int64 = 91002
				groupID   int64 = 91003
				accountID int64 = 91004
			)
			cfg := &config.Config{}
			cfg.Default.RateMultiplier = 1
			cfg.Security.URLAllowlist.Enabled = false
			cfg.Gateway.MaxAccountSwitches = 1

			settings := service.NewSettingService(&contentModerationHandlerSettingRepo{}, cfg)
			settingValues, err := settings.GetAllSettings(context.Background())
			require.NoError(t, err)
			setInference := func(enabled bool) {
				t.Helper()
				settingValues.OpenAICacheWriteInferenceEnabled = enabled
				require.NoError(t, settings.UpdateSettings(context.Background(), settingValues))
				require.Equal(t, enabled, settings.IsOpenAICacheWriteInferenceEnabled(context.Background()))
			}
			setInference(tc.enabled)
			t.Cleanup(func() { setInference(false) })

			ledger := &cacheWriteHTTPTestLedger{
				balance: 10,
				logs:    make(map[string]service.UsageLog),
				seen:    make(map[string]string),
			}
			userRepo := &cacheWriteHTTPTestUserRepo{ledger: ledger, userID: userID}
			usageRepo := &cacheWriteHTTPTestUsageRepo{ledger: ledger}
			billingRepo := &cacheWriteHTTPTestBillingRepo{ledger: ledger}
			billingCache := service.NewBillingCacheService(nil, userRepo, nil, nil, nil, nil, cfg, nil)
			t.Cleanup(billingCache.Stop)

			var upstreamCalls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				turn := upstreamCalls.Add(1)
				body, readErr := io.ReadAll(r.Body)
				if readErr != nil || r.Method != http.MethodPost ||
					gjson.GetBytes(body, "model").String() != "gpt-6-astra" ||
					r.Header.Get("Authorization") != "Bearer simulation-only-oauth-token" || turn > 2 {
					http.Error(w, "invalid simulation request", http.StatusBadRequest)
					return
				}
				cached := 0
				if turn == 2 {
					cached = 9984
				}
				writeField := ""
				if tc.officialZero && turn == 1 {
					writeField = `,"cache_write_tokens":0`
				}
				w.Header().Set("Content-Type", "text/event-stream")
				w.Header().Set("X-Request-Id", fmt.Sprintf("req-cache-http-%d", turn))
				_, _ = fmt.Fprintf(w,
					"event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp-cache-http-%d\",\"model\":\"gpt-6-astra\",\"status\":\"completed\",\"output\":[],\"usage\":{\"input_tokens\":10818,\"output_tokens\":0,\"total_tokens\":10818,\"input_tokens_details\":{\"cached_tokens\":%d%s}}}}\n\ndata: [DONE]\n\n",
					turn, cached, writeField)
			}))
			t.Cleanup(upstream.Close)
			upstreamURL, err := url.Parse(upstream.URL)
			require.NoError(t, err)
			transport := &cacheWriteHTTPLocalUpstream{target: upstreamURL, client: upstream.Client()}
			// Redirects cannot escape the local fake server, even if a fixture changes.
			transport.client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

			accountRepo := &openAIWSUsageHandlerAccountRepoStub{account: service.Account{
				ID: accountID, Name: "cache-write-http-simulation", Platform: service.PlatformOpenAI,
				Type: service.AccountTypeOAuth, Status: service.StatusActive, Schedulable: true, GroupIDs: []int64{groupID},
				Credentials: map[string]any{"access_token": "simulation-only-oauth-token", "chatgpt_account_id": "simulation-only-account"},
				Extra:       map[string]any{"openai_passthrough": true},
			}}
			billing := service.NewBillingService(cfg, nil)
			gateway := service.NewOpenAIGatewayService(
				accountRepo, usageRepo, billingRepo, userRepo, nil, nil, nil, cfg, nil, nil,
				billing, nil, billingCache, transport, &service.DeferredService{}, nil, nil,
				service.NewModelPricingResolver(nil, billing), nil, nil, settings, nil,
			)
			t.Cleanup(gateway.CloseOpenAIWSPool)
			pool := service.NewUsageRecordWorkerPoolWithOptions(service.UsageRecordWorkerPoolOptions{
				WorkerCount: 1, QueueSize: 8, TaskTimeout: 5 * time.Second,
				OverflowPolicy: config.UsageRecordOverflowPolicySync,
			})
			t.Cleanup(pool.Stop)
			h := NewOpenAIGatewayHandler(gateway, service.NewConcurrencyService(nil), billingCache,
				service.NewAPIKeyService(nil, nil, nil, nil, nil, nil, cfg), pool, nil, nil, nil, cfg)
			apiKey := &service.APIKey{
				ID: keyID, UserID: userID, GroupID: ptrCacheWriteHTTP(groupID),
				User:  &service.User{ID: userID, Status: service.StatusActive, Balance: 10},
				Group: &service.Group{ID: groupID, Platform: service.PlatformOpenAI, Status: service.StatusActive, RateMultiplier: 1},
			}
			router := gin.New()
			// Bypass only external authentication. The real handler still performs
			// balance eligibility, account selection, forwarding and billing.
			router.Use(func(c *gin.Context) {
				c.Set(string(middleware.ContextKeyAPIKey), apiKey)
				c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: userID})
				c.Next()
			})
			router.POST("/v1/responses", h.Responses)
			server := httptest.NewServer(router)
			t.Cleanup(server.Close)
			client := server.Client()
			client.Timeout = 5 * time.Second

			postTurn := func(turn int) {
				t.Helper()
				promptTurn := 1
				if tc.divergent {
					promptTurn = turn
				}
				payload := fmt.Sprintf(`{"model":"gpt-6-astra","stream":true,"instructions":"Local simulation only","prompt_cache_key":"cache-http-same-session","input":[{"role":"user","content":"simulation turn %d"}]}`, promptTurn)
				response, err := client.Post(server.URL+"/v1/responses", "application/json", strings.NewReader(payload))
				require.NoError(t, err)
				body, readErr := io.ReadAll(response.Body)
				require.NoError(t, response.Body.Close())
				require.NoError(t, readErr)
				require.Equal(t, http.StatusOK, response.StatusCode, string(body))
				require.Contains(t, string(body), `"type":"response.completed"`)
				require.Contains(t, string(body), fmt.Sprintf(`"id":"resp-cache-http-%d"`, turn))
				require.Eventually(t, func() bool {
					return pool.Stats().CompletedTasks >= uint64(turn)
				}, 5*time.Second, time.Millisecond, "async RecordUsage did not finish")
			}

			postTurn(1)
			first, ok := ledger.log("req-cache-http-1", keyID)
			require.True(t, ok, "first HTTP request must persist its original usage row")
			require.Equal(t, 10818, first.InputTokens)
			require.Zero(t, first.CacheCreationTokens)
			require.InDelta(t, 0.10818, first.ActualCost, 1e-12)
			beforeSecond := ledger.currentBalance()
			require.InDelta(t, 10-0.10818, beforeSecond, 1e-12)

			if tc.disableOnTurn2 {
				setInference(false)
			}
			postTurn(2)
			pool.Stop()
			require.Equal(t, int32(2), upstreamCalls.Load())
			require.Equal(t, uint64(2), pool.Stats().SubmittedTasks)
			require.Zero(t, pool.Stats().FailedTasks)
			require.Zero(t, pool.Stats().SyncFallbackTasks, "usage must run through the async pool")

			first, ok = ledger.log("req-cache-http-1", keyID)
			require.True(t, ok)
			second, ok := ledger.log("req-cache-http-2", keyID)
			require.True(t, ok)
			require.Equal(t, 834, second.InputTokens)
			require.Equal(t, 9984, second.CacheReadTokens)
			require.Zero(t, second.CacheCreationTokens)
			// Astra standard: $10/MTok input and $1/MTok cached input.
			require.InDelta(t, 0.018324, second.ActualCost, 1e-12)

			adjustments := ledger.adjustments()
			if tc.wantInferred {
				require.Len(t, adjustments, 1)
				require.Equal(t, 834, first.InputTokens)
				require.Equal(t, 9984, first.CacheCreationTokens)
				require.Zero(t, first.CacheReadTokens)
				require.InDelta(t, 0.00834, first.InputCost, 1e-12)
				require.InDelta(t, 0.1248, first.CacheCreationCost, 1e-12)
				require.InDelta(t, 0.13314, first.ActualCost, 1e-12)
				require.InDelta(t, 0.02496, adjustments[0].BalanceCost, 1e-12)
				require.Equal(t, "req-cache-http-1", adjustments[0].CacheWriteCorrection.RequestID)
				require.InDelta(t, 0.02496, beforeSecond-ledger.currentBalance()-second.ActualCost, 1e-12,
					"the inferred write surcharge must actually debit the test balance")
			} else {
				require.Empty(t, adjustments)
				require.Equal(t, 10818, first.InputTokens)
				require.Zero(t, first.CacheCreationTokens)
				require.InDelta(t, 0.10818, first.ActualCost, 1e-12)
				require.InDelta(t, second.ActualCost, beforeSecond-ledger.currentBalance(), 1e-12)
			}
			require.InDelta(t, first.ActualCost+second.ActualCost, 10-ledger.currentBalance(), 1e-12)
		})
	}
}

func ptrCacheWriteHTTP(v int64) *int64 { return &v }

// OAuth URLs are intentionally constructed by production code. This test-only
// adapter retains the request/path/body but rewrites its destination before any
// network access; it never uses the original host or a real credential.
type cacheWriteHTTPLocalUpstream struct {
	target *url.URL
	client *http.Client
}

func (u *cacheWriteHTTPLocalUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	local := req.Clone(req.Context())
	local.URL.Scheme = u.target.Scheme
	local.URL.Host = u.target.Host
	local.Host = u.target.Host
	return u.client.Do(local)
}

func (u *cacheWriteHTTPLocalUpstream) DoWithTLS(req *http.Request, proxy string, accountID int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxy, accountID, concurrency)
}

type cacheWriteHTTPTestLedger struct {
	mu       sync.Mutex
	balance  float64
	logs     map[string]service.UsageLog
	seen     map[string]string
	commands []service.UsageBillingCommand
}

func cacheWriteHTTPLogKey(requestID string, apiKeyID int64) string {
	return fmt.Sprintf("%s|%d", requestID, apiKeyID)
}

func (l *cacheWriteHTTPTestLedger) currentBalance() float64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.balance
}

func (l *cacheWriteHTTPTestLedger) log(requestID string, apiKeyID int64) (service.UsageLog, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	value, ok := l.logs[cacheWriteHTTPLogKey(requestID, apiKeyID)]
	return value, ok
}

func (l *cacheWriteHTTPTestLedger) adjustments() []service.UsageBillingCommand {
	l.mu.Lock()
	defer l.mu.Unlock()
	var result []service.UsageBillingCommand
	for _, command := range l.commands {
		if command.CacheWriteCorrection != nil {
			result = append(result, command)
		}
	}
	return result
}

type cacheWriteHTTPTestUserRepo struct {
	service.UserRepository
	ledger *cacheWriteHTTPTestLedger
	userID int64
}

func (r *cacheWriteHTTPTestUserRepo) GetByID(_ context.Context, id int64) (*service.User, error) {
	if id != r.userID {
		return nil, fmt.Errorf("unexpected simulation user %d", id)
	}
	return &service.User{ID: id, Status: service.StatusActive, Balance: r.ledger.currentBalance()}, nil
}

type cacheWriteHTTPTestUsageRepo struct {
	service.UsageLogRepository
	ledger *cacheWriteHTTPTestLedger
}

func (r *cacheWriteHTTPTestUsageRepo) Create(_ context.Context, log *service.UsageLog) (bool, error) {
	r.ledger.mu.Lock()
	defer r.ledger.mu.Unlock()
	key := cacheWriteHTTPLogKey(log.RequestID, log.APIKeyID)
	if _, exists := r.ledger.logs[key]; exists {
		return false, nil
	}
	r.ledger.logs[key] = *log
	return true, nil
}

type cacheWriteHTTPTestBillingRepo struct {
	service.UsageBillingRepository
	ledger *cacheWriteHTTPTestLedger
}

// This in-memory implementation consumes production-calculated commands. It
// deliberately does not claim to verify PostgreSQL transactions or migrations.
func (r *cacheWriteHTTPTestBillingRepo) Apply(_ context.Context, command *service.UsageBillingCommand) (*service.UsageBillingApplyResult, error) {
	r.ledger.mu.Lock()
	defer r.ledger.mu.Unlock()
	key := cacheWriteHTTPLogKey(command.RequestID, command.APIKeyID)
	if fingerprint, exists := r.ledger.seen[key]; exists {
		if fingerprint != command.RequestFingerprint {
			return nil, service.ErrUsageBillingRequestConflict
		}
		return &service.UsageBillingApplyResult{}, nil
	}
	if correction := command.CacheWriteCorrection; correction != nil {
		originalKey := cacheWriteHTTPLogKey(correction.RequestID, correction.APIKeyID)
		original, exists := r.ledger.logs[originalKey]
		if !exists || original.InputTokens != correction.OriginalInputTokens ||
			original.CacheCreationTokens != correction.OriginalCacheCreationTokens {
			return nil, fmt.Errorf("simulation correction target missing or stale: %s", originalKey)
		}
		original.InputTokens = correction.CorrectedInputTokens
		original.CacheCreationTokens = correction.CorrectedCacheCreationTokens
		original.InputCost = correction.CorrectedInputCost
		original.CacheCreationCost = correction.CorrectedCacheCreationCost
		original.TotalCost = correction.CorrectedTotalCost
		original.ActualCost = correction.CorrectedActualCost
		original.AccountStatsCost = correction.CorrectedAccountStatsCost
		r.ledger.logs[originalKey] = original
	}
	r.ledger.balance = service.QuantizeUsageBillingAmount(r.ledger.balance - command.BalanceCost)
	r.ledger.commands = append(r.ledger.commands, *command)
	r.ledger.seen[key] = command.RequestFingerprint
	balance := r.ledger.balance
	return &service.UsageBillingApplyResult{Applied: true, NewBalance: &balance}, nil
}
