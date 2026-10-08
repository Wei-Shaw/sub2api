//go:build integration

package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestOpenAICacheWriteHTTPPostgres joins the real HTTP and SQL paths in one test.
// It shares the repository TestMain's migrated PostgreSQL testcontainer; there
// is no second application/container boot. API-key authentication, account
// request IDs, selection, Responses/SSE parsing, inference, pricing, async RecordUsage,
// usage-row persistence and transactional billing all use production code.
//
// Boundaries: the upstream is localhost-only with synthetic credentials/usage;
// settings are an isolated in-memory repository and prices are a local fixture.
// Redis concurrency uses the existing harness; auth/billing/scheduler caches
// are disabled (production database fallbacks). No subscription, notification
// or deferred account-last-used worker is started.
// This proves the balance/API-key billing path, not a live provider's usage
// reporting, deployed wiring, distributed cache invalidation, window rollover
// or crash recovery. Fresh HTTP requests have distinct production billing IDs;
// only reapplying the identical captured billing command proves SQL deduplication.
func TestOpenAICacheWriteHTTPPostgres(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name           string
		rate           float64
		enabled        bool
		disableOnTurn2 bool
		officialZero   bool
		conflictingRow bool
		wantCorrection bool
	}{
		{name: "enabled", rate: 1, enabled: true, wantCorrection: true},
		{name: "enabled_non_unit_rate", rate: 2, enabled: true, wantCorrection: true},
		{name: "disabled", rate: 2},
		{name: "disabled_between_turns", rate: 2, enabled: true, disableOnTurn2: true},
		{name: "authoritative_zero", rate: 2, enabled: true, officialZero: true},
		{name: "conflicting_row_rolls_back_then_next_http_request_retries", rate: 2, enabled: true, conflictingRow: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			f := newCacheWriteHTTPPGFixture(t, tc.rate)
			cfg := &config.Config{}
			cfg.Default.RateMultiplier = 1
			cfg.Gateway.MaxAccountSwitches = 1
			settings := service.NewSettingService(&cacheWriteHTTPPGSettings{values: map[string]string{}}, cfg)
			values, err := settings.GetAllSettings(ctx)
			require.NoError(t, err)
			setInference := func(enabled bool) {
				t.Helper()
				values.OpenAICacheWriteInferenceEnabled = enabled
				require.NoError(t, settings.UpdateSettings(ctx, values))
				require.Equal(t, enabled, settings.IsOpenAICacheWriteInferenceEnabled(ctx))
			}
			setInference(tc.enabled)
			t.Cleanup(func() { setInference(false) })

			userRepo := NewUserRepository(integrationEntClient, integrationDB)
			keyRepo := NewAPIKeyRepository(integrationEntClient, integrationDB)
			accountRepo := NewAccountRepository(integrationEntClient, integrationDB, nil)
			logs := NewUsageLogRepository(integrationEntClient, integrationDB)
			// This observer delegates every command to the real repository. It
			// does not calculate, suppress or simulate any monetary operation.
			billingRepo := &cacheWriteHTTPPGBillingObserver{UsageBillingRepository: NewUsageBillingRepository(integrationEntClient, integrationDB)}
			apiKeys := service.NewAPIKeyService(keyRepo, userRepo, NewGroupRepository(integrationEntClient, integrationDB), nil, nil, nil, cfg)
			billingCache := service.NewBillingCacheService(nil, userRepo, nil, keyRepo, nil, nil, cfg, nil)
			t.Cleanup(billingCache.Stop)
			billing := cacheWriteHTTPPGPricing(t, cfg)
			concurrencyCache := NewConcurrencyCache(testRedis(t), 15, 60)
			concurrency := service.NewConcurrencyService(concurrencyCache)

			var upstreamCalls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				call := upstreamCalls.Add(1)
				body, readErr := io.ReadAll(r.Body)
				if readErr != nil || r.Method != http.MethodPost || call > 3 ||
					gjson.GetBytes(body, "model").String() != "gpt-6-astra" ||
					r.Header.Get("Authorization") != "Bearer postgres-test-only-oauth-token" {
					http.Error(w, "invalid test upstream request", http.StatusBadRequest)
					return
				}
				// The third HTTP call repeats the second provider completion and
				// upstream ID. It is still a separately billable HTTP request; the
				// duplicate completion must not infer another cache-write surcharge.
				turn, cached := 1, 0
				if call >= 2 {
					turn, cached = 2, 9984
				}
				writeField := ""
				if tc.officialZero && turn == 1 {
					writeField = `,"cache_write_tokens":0`
				}
				w.Header().Set("Content-Type", "text/event-stream")
				w.Header().Set("X-Request-Id", fmt.Sprintf("%s-upstream-%d", f.prefix, turn))
				_, _ = fmt.Fprintf(w, "event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp-pg-%d\",\"model\":\"gpt-6-astra\",\"status\":\"completed\",\"output\":[],\"usage\":{\"input_tokens\":10818,\"output_tokens\":0,\"total_tokens\":10818,\"input_tokens_details\":{\"cached_tokens\":%d%s}}}}\n\ndata: [DONE]\n\n", turn, cached, writeField)
			}))
			t.Cleanup(upstream.Close)
			upstreamURL, err := url.Parse(upstream.URL)
			require.NoError(t, err)
			local := &cacheWriteHTTPPGUpstream{target: upstreamURL, client: upstream.Client()}
			local.client.Timeout = 10 * time.Second
			local.client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
			gateway := service.NewOpenAIGatewayService(accountRepo, logs, billingRepo, userRepo, nil, nil, nil, cfg, nil, concurrency,
				billing, nil, billingCache, local, &service.DeferredService{}, nil, nil,
				service.NewModelPricingResolver(nil, billing), nil, nil, settings, nil)
			t.Cleanup(gateway.CloseOpenAIWSPool)
			pool := service.NewUsageRecordWorkerPoolWithOptions(service.UsageRecordWorkerPoolOptions{
				WorkerCount: 1, QueueSize: 8, TaskTimeout: 10 * time.Second, OverflowPolicy: config.UsageRecordOverflowPolicySync,
			})
			t.Cleanup(pool.Stop)
			h := handler.NewOpenAIGatewayHandler(gateway, concurrency, billingCache, apiKeys, pool, nil, nil, nil, cfg)
			router := gin.New()
			router.Use(middleware.ClientRequestID())
			router.Use(gin.HandlerFunc(middleware.NewAPIKeyAuthMiddleware(apiKeys, nil, cfg)))
			router.POST("/v1/responses", h.Responses)
			server := httptest.NewServer(router)
			t.Cleanup(server.Close)
			client := server.Client()
			client.Timeout = 15 * time.Second
			payload := fmt.Sprintf(`{"model":"gpt-6-astra","stream":true,"instructions":"PostgreSQL integration fixture only","prompt_cache_key":%q,"input":[{"role":"user","content":"same fixture prompt"}]}`, f.prefix)
			post := func(key string, status int, turn int) {
				t.Helper()
				req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/responses", strings.NewReader(payload))
				require.NoError(t, err)
				req.Header.Set("Content-Type", "application/json")
				if key != "" {
					req.Header.Set("Authorization", "Bearer "+key)
				}
				resp, err := client.Do(req)
				require.NoError(t, err)
				body, readErr := io.ReadAll(resp.Body)
				require.NoError(t, resp.Body.Close())
				require.NoError(t, readErr)
				require.Equal(t, status, resp.StatusCode, string(body))
				if status == http.StatusOK {
					clientID := resp.Header.Get("X-Client-Request-ID")
					_, err := uuid.Parse(clientID)
					require.NoError(t, err, "production middleware must return its generated request ID")
					billingID := "client:" + clientID
					for _, previousID := range f.requestIDs {
						require.NotEqual(t, previousID, billingID, "fresh HTTP requests must get distinct billing IDs")
					}
					f.requestIDs[turn] = billingID
					require.Contains(t, string(body), `"type":"response.completed"`)
					require.Contains(t, string(body), fmt.Sprintf(`"id":"resp-pg-%d"`, min(turn, 2)))
					require.Eventually(t, func() bool { return pool.Stats().CompletedTasks >= uint64(turn) }, 15*time.Second, 5*time.Millisecond)
				}
			}

			post("", http.StatusUnauthorized, 0)
			post("sk-nonexistent-postgres-fixture", http.StatusUnauthorized, 0)
			require.Zero(t, upstreamCalls.Load(), "authentication must reject before forwarding")
			f.requireState(t, 0, 0)
			post(f.key.Key, http.StatusOK, 1)
			first := f.log(t, logs, 1)
			require.Equal(t, 10818, first.InputTokens)
			require.Zero(t, first.CacheCreationTokens)
			require.Zero(t, first.CacheReadTokens)
			require.InDelta(t, 0.10818, first.TotalCost, 1e-8)
			require.InDelta(t, 0.10818*tc.rate, first.ActualCost, 1e-8)
			before := f.requireState(t, 0.10818*tc.rate, 1)
			var lastUsed sql.NullTime
			require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT last_used_at FROM api_keys WHERE id=$1", f.key.ID).Scan(&lastUsed))
			require.True(t, lastUsed.Valid, "production authentication must touch the persisted key")

			if tc.disableOnTurn2 {
				setInference(false)
			}
			if tc.conflictingRow {
				_, err := integrationDB.ExecContext(ctx, "UPDATE usage_logs SET input_tokens=10817, cache_creation_tokens=1 WHERE id=$1", first.ID)
				require.NoError(t, err)
			}
			post(f.key.Key, http.StatusOK, 2)
			first = f.log(t, logs, 1)
			second := f.log(t, logs, 2)
			require.Equal(t, 834, second.InputTokens)
			require.Equal(t, 9984, second.CacheReadTokens)
			require.Zero(t, second.CacheCreationTokens)
			require.InDelta(t, 0.00834, second.InputCost, 1e-8)
			require.InDelta(t, 0.009984, second.CacheReadCost, 1e-8)
			require.InDelta(t, 0.018324, second.TotalCost, 1e-8)
			require.InDelta(t, 0.018324*tc.rate, second.ActualCost, 1e-8)
			wantCost, wantDedup := 0.126504*tc.rate, 2
			if tc.wantCorrection {
				wantCost, wantDedup = 0.151464*tc.rate, 3
				f.requireCorrectedLog(t, first, tc.rate)
			} else {
				wantInput, wantWrite := 10818, 0
				if tc.conflictingRow {
					wantInput, wantWrite = 10817, 1
				}
				require.Equal(t, wantInput, first.InputTokens)
				require.Equal(t, wantWrite, first.CacheCreationTokens)
				require.InDelta(t, 0.10818*tc.rate, first.ActualCost, 1e-8)
			}
			after := f.requireState(t, wantCost, wantDedup)
			require.Equal(t, before.Window5h, after.Window5h)
			require.Equal(t, before.Window1d, after.Window1d)
			require.Equal(t, before.Window7d, after.Window7d)
			wantDelta := 0.0
			if tc.wantCorrection {
				wantDelta = 0.02496 * tc.rate
			}
			require.InDelta(t, wantDelta, before.Balance-after.Balance-second.ActualCost, 1e-8,
				"correction must debit only the write premium, never the full write price")
			corrections := billingRepo.corrections()
			if tc.wantCorrection || tc.conflictingRow {
				require.Len(t, corrections, 1)
				correction := corrections[0]
				require.Equal(t, f.requestID(1), correction.command.CacheWriteCorrection.RequestID)
				require.InDelta(t, 0.02496*tc.rate, correction.command.BalanceCost, 1e-8)
				require.InDelta(t, correction.command.BalanceCost, correction.command.APIKeyQuotaCost, 1e-8)
				require.InDelta(t, correction.command.BalanceCost, correction.command.APIKeyRateLimitCost, 1e-8)
				if tc.conflictingRow {
					require.ErrorContains(t, correction.err, "cache write usage reconciliation conflict")
					require.Nil(t, correction.result)
					// Repair only the deliberately corrupted fixture. The next
					// HTTP request must retry the retained correction while paying
					// its own normal bill under a fresh production request ID.
					_, err := integrationDB.ExecContext(ctx, "UPDATE usage_logs SET input_tokens=10818, cache_creation_tokens=0 WHERE id=$1", first.ID)
					require.NoError(t, err)
				} else {
					require.NoError(t, correction.err)
					require.True(t, correction.result.Applied)
				}
			} else {
				require.Empty(t, corrections)
			}

			post(f.key.Key, http.StatusOK, 3)
			pool.Stop()
			var originalCalls []cacheWriteHTTPPGCall
			for _, call := range billingRepo.snapshot() {
				if call.command.CacheWriteCorrection == nil {
					originalCalls = append(originalCalls, call)
				}
			}
			require.Len(t, originalCalls, 3, "every successful HTTP response must reach real billing")
			for i, call := range originalCalls {
				require.NoError(t, call.err, "each distinct HTTP request must bill normally")
				require.NotNil(t, call.result)
				require.Equal(t, f.requestID(i+1), call.command.RequestID)
				require.True(t, call.result.Applied)
			}
			wantCost += 0.018324 * tc.rate
			wantDedup++
			if tc.conflictingRow {
				wantCost, wantDedup = 0.169788*tc.rate, 4
				f.requireCorrectedLog(t, f.log(t, logs, 1), tc.rate)
				corrections = billingRepo.corrections()
				require.Len(t, corrections, 2)
				require.NoError(t, corrections[1].err)
				require.True(t, corrections[1].result.Applied)
				require.Equal(t, corrections[0].command.RequestID, corrections[1].command.RequestID,
					"rollback must release the same correction dedup key for retry")
			} else {
				require.Len(t, billingRepo.corrections(), len(corrections), "duplicate provider completion must not add another correction")
				require.Equal(t, first, f.log(t, logs, 1), "duplicate provider completion must not rewrite the prior row again")
			}
			require.Equal(t, second, f.log(t, logs, 2), "a fresh HTTP request must not replace R2")
			third := f.log(t, logs, 3)
			require.NotEqual(t, second.ID, third.ID)
			require.Equal(t, 834, third.InputTokens)
			require.Equal(t, 9984, third.CacheReadTokens)
			require.Zero(t, third.CacheCreationTokens)
			require.InDelta(t, 0.018324, third.TotalCost, 1e-8)
			require.InDelta(t, 0.018324*tc.rate, third.ActualCost, 1e-8)
			finalState := f.requireState(t, wantCost, wantDedup)
			require.Equal(t, int32(3), upstreamCalls.Load())
			require.Equal(t, uint64(3), pool.Stats().SubmittedTasks)
			require.Zero(t, pool.Stats().FailedTasks)
			require.Zero(t, pool.Stats().SyncFallbackTasks)
			require.Eventually(t, func() bool {
				users, userErr := concurrencyCache.GetUserConcurrency(ctx, f.user.ID)
				accounts, accountErr := concurrencyCache.GetAccountConcurrency(ctx, f.account.ID)
				return userErr == nil && accountErr == nil && users == 0 && accounts == 0
			}, 5*time.Second, 5*time.Millisecond, "real Redis slots must be released")

			beforeSQLReplayFirst := f.log(t, logs, 1)
			// Reapply the exact commands captured from HTTP, including the
			// original R1 bill after its usage row was reconciled. SQL must
			// retain their dedup claims independently of the in-memory tracker.
			for _, call := range billingRepo.snapshot() {
				if call.err != nil {
					continue
				}
				cmd := call.command
				result, err := billingRepo.UsageBillingRepository.Apply(ctx, &cmd)
				require.NoError(t, err)
				require.False(t, result.Applied)
			}
			require.Equal(t, finalState, f.requireState(t, wantCost, wantDedup))
			finalFirst := f.log(t, logs, 1)
			require.Equal(t, beforeSQLReplayFirst, finalFirst, "SQL replay must preserve corrected R1")
			require.Equal(t, second, f.log(t, logs, 2), "SQL replay must preserve R2")
			require.Equal(t, third, f.log(t, logs, 3), "SQL replay must preserve R3")
			require.InDelta(t, wantCost, finalFirst.ActualCost+second.ActualCost+third.ActualCost, 1e-8)
			t.Logf("PostgreSQL receipt: R1 input=%d/write=%d total=%.8f actual=%.8f; R2 input=%d/read=%d total=%.8f actual=%.8f; balance=%.8f quota=%.8f windows=%.8f/%.8f/%.8f dedup=%d; R3 actual=%.8f; three distinct HTTP bills, identical SQL-command replays applied=false",
				finalFirst.InputTokens, finalFirst.CacheCreationTokens, finalFirst.TotalCost, finalFirst.ActualCost,
				second.InputTokens, second.CacheReadTokens, second.TotalCost, second.ActualCost,
				finalState.Balance, finalState.QuotaUsed, finalState.Usage5h, finalState.Usage1d, finalState.Usage7d, wantDedup, third.ActualCost)
		})
	}
}

// The real service loads fixed test prices from disk; remote pricing is disabled.
func cacheWriteHTTPPGPricing(t *testing.T, cfg *config.Config) *service.BillingService {
	t.Helper()
	cfg.Pricing.DataDir = t.TempDir()
	cfg.Pricing.UpdateIntervalHours = 24
	fixture := []byte(`{
		"gpt-6-astra":{"input_cost_per_token":0.00001,"output_cost_per_token":0.00005,
		"cache_creation_input_token_cost":0.0000125,"cache_read_input_token_cost":0.000001,
		"litellm_provider":"openai","mode":"chat","supports_prompt_caching":true}
	}`)
	require.NoError(t, os.WriteFile(filepath.Join(cfg.Pricing.DataDir, "model_pricing.json"), fixture, 0600))
	pricing := service.NewPricingService(cfg, nil)
	require.NoError(t, pricing.Initialize())
	t.Cleanup(pricing.Stop)
	// Rates match today's fallback, so numeric assertions alone cannot prove
	// this fixture was loaded. Pin its published content hash and model count.
	status := pricing.GetStatus()
	hash := fmt.Sprintf("%x", sha256.Sum256(fixture))
	require.Equal(t, hash[:8], status["local_hash"])
	require.Equal(t, 1, status["model_count"])
	require.NotNil(t, pricing.GetIdentifiedModelPricing("gpt-6-astra"))
	t.Logf("Pricing fixture loaded: models=%v sha256-prefix=%v", status["model_count"], status["local_hash"])
	billing := service.NewBillingService(cfg, pricing)
	prices, err := billing.GetModelPricing("gpt-6-astra")
	require.NoError(t, err)
	require.InDelta(t, 10e-6, prices.InputPricePerToken, 1e-15)
	require.InDelta(t, 12.5e-6, prices.CacheCreationPricePerToken, 1e-15)
	require.InDelta(t, 1e-6, prices.CacheReadPricePerToken, 1e-15)
	return billing
}

type cacheWriteHTTPPGFixture struct {
	prefix      string
	requestIDs  map[int]string
	windowStart time.Time
	user        *service.User
	key         *service.APIKey
	group       *service.Group
	account     *service.Account
}

func newCacheWriteHTTPPGFixture(t *testing.T, rate float64) *cacheWriteHTTPPGFixture {
	t.Helper()
	f := &cacheWriteHTTPPGFixture{prefix: "cache-http-pg-" + uuid.NewString(), requestIDs: make(map[int]string)}
	ctx := context.Background()
	// Usage-row triggers may rewind this shared watermark. Preserve the
	// harness's pre-test value; these subtests intentionally do not run parallel.
	var closedBefore, retainedFrom, updatedAt time.Time
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT closed_before, retained_from, updated_at FROM usage_group_rollup_state WHERE id=1").Scan(&closedBefore, &retainedFrom, &updatedAt))
	t.Cleanup(func() {
		cleanup := func(query string, args ...any) {
			if _, err := integrationDB.ExecContext(ctx, query, args...); err != nil {
				t.Errorf("clean up HTTP PostgreSQL fixture: %s: %v", query, err)
			}
		}
		if f.key != nil {
			cleanup("DELETE FROM usage_logs WHERE api_key_id=$1", f.key.ID)
			cleanup("DELETE FROM usage_billing_dedup WHERE api_key_id=$1", f.key.ID)
			cleanup("DELETE FROM usage_billing_dedup_archive WHERE api_key_id=$1", f.key.ID)
			cleanup("DELETE FROM api_keys WHERE id=$1", f.key.ID)
		}
		if f.account != nil {
			cleanup("DELETE FROM account_groups WHERE account_id=$1", f.account.ID)
			cleanup("DELETE FROM accounts WHERE id=$1", f.account.ID)
			cleanup("DELETE FROM scheduler_outbox WHERE account_id=$1", f.account.ID)
		}
		if f.group != nil {
			cleanup("DELETE FROM usage_group_daily_rollups WHERE group_id=$1", f.group.ID)
			cleanup("DELETE FROM groups WHERE id=$1", f.group.ID)
			cleanup("DELETE FROM scheduler_outbox WHERE group_id=$1", f.group.ID)
		}
		if f.user != nil {
			cleanup("DELETE FROM users WHERE id=$1", f.user.ID)
		}
		if f.key != nil {
			cleanup("DELETE FROM auth_cache_invalidation_outbox WHERE cache_key=encode(sha256(convert_to($1, 'UTF8')), 'hex')", f.key.Key)
		}
		cleanup("UPDATE usage_group_rollup_state SET closed_before=$1, retained_from=$2, updated_at=$3 WHERE id=1", closedBefore, retainedFrom, updatedAt)
	})
	f.user = mustCreateUser(t, integrationEntClient, &service.User{Email: f.prefix + "@example.com", Balance: 10})
	f.group = mustCreateGroup(t, integrationEntClient, &service.Group{Name: f.prefix, Platform: service.PlatformOpenAI, RateMultiplier: rate})
	// Seed active windows at the database's current time, rather than letting
	// first billing initialize a 1d window at midnight and race a day rollover.
	// This test covers increments/reconciliation inside active windows.
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT NOW()").Scan(&f.windowStart))
	f.key = mustCreateApiKey(t, integrationEntClient, &service.APIKey{UserID: f.user.ID, GroupID: &f.group.ID,
		Key: "sk-" + f.prefix, Quota: 100, RateLimit5h: 100, RateLimit1d: 100, RateLimit7d: 100,
		Window5hStart: &f.windowStart, Window1dStart: &f.windowStart, Window7dStart: &f.windowStart})
	f.account = mustCreateAccount(t, integrationEntClient, &service.Account{Name: f.prefix, Platform: service.PlatformOpenAI,
		Type: service.AccountTypeOAuth, Credentials: map[string]any{"access_token": "postgres-test-only-oauth-token", "chatgpt_account_id": "postgres-test-only-account"},
		Extra: map[string]any{"openai_passthrough": true}})
	mustBindAccountToGroup(t, integrationEntClient, f.account.ID, f.group.ID, 1)
	return f
}

func (f *cacheWriteHTTPPGFixture) requestID(turn int) string {
	return f.requestIDs[turn]
}

func (f *cacheWriteHTTPPGFixture) log(t *testing.T, logs service.UsageLogRepository, turn int) *service.UsageLog {
	t.Helper()
	var id int64
	require.NoError(t, integrationDB.QueryRowContext(context.Background(), "SELECT id FROM usage_logs WHERE request_id=$1 AND api_key_id=$2", f.requestID(turn), f.key.ID).Scan(&id))
	row, err := logs.GetByID(context.Background(), id)
	require.NoError(t, err)
	require.Equal(t, f.user.ID, row.UserID)
	require.Equal(t, f.key.ID, row.APIKeyID)
	require.Equal(t, f.account.ID, row.AccountID)
	require.Equal(t, &f.group.ID, row.GroupID)
	require.Equal(t, "gpt-6-astra", row.Model)
	require.Zero(t, row.OutputTokens)
	require.Equal(t, service.BillingTypeBalance, row.BillingType)
	require.InDelta(t, f.group.RateMultiplier, row.RateMultiplier, 1e-8)
	return row
}

type cacheWriteHTTPPGState struct {
	Balance, QuotaUsed, Usage5h, Usage1d, Usage7d float64
	Status                                        string
	Window5h, Window1d, Window7d                  sql.NullTime
}

func (f *cacheWriteHTTPPGFixture) requireState(t *testing.T, cost float64, dedup int) cacheWriteHTTPPGState {
	t.Helper()
	ctx := context.Background()
	var state cacheWriteHTTPPGState
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT u.balance, k.quota_used, k.usage_5h, k.usage_1d, k.usage_7d,
		k.status, k.window_5h_start, k.window_1d_start, k.window_7d_start FROM users u JOIN api_keys k ON k.user_id=u.id WHERE k.id=$1`, f.key.ID).
		Scan(&state.Balance, &state.QuotaUsed, &state.Usage5h, &state.Usage1d, &state.Usage7d, &state.Status, &state.Window5h, &state.Window1d, &state.Window7d))
	require.InDelta(t, 10-cost, state.Balance, 1e-8)
	for _, actual := range []float64{state.QuotaUsed, state.Usage5h, state.Usage1d, state.Usage7d} {
		require.InDelta(t, cost, actual, 1e-8)
	}
	require.Equal(t, service.StatusActive, state.Status)
	for _, window := range []sql.NullTime{state.Window5h, state.Window1d, state.Window7d} {
		require.True(t, window.Valid)
		require.True(t, window.Time.Equal(f.windowStart), "billing must preserve active window starts")
	}
	var count int
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM usage_billing_dedup WHERE api_key_id=$1", f.key.ID).Scan(&count))
	require.Equal(t, dedup, count)
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM usage_logs WHERE api_key_id=$1", f.key.ID).Scan(&count))
	require.Equal(t, len(f.requestIDs), count, "each HTTP request owns one usage row; correction must only rewrite R1")
	return state
}

func (f *cacheWriteHTTPPGFixture) requireCorrectedLog(t *testing.T, row *service.UsageLog, rate float64) {
	t.Helper()
	require.Equal(t, 834, row.InputTokens)
	require.Equal(t, 9984, row.CacheCreationTokens)
	require.Zero(t, row.CacheReadTokens)
	require.Equal(t, 10818, row.InputTokens+row.CacheCreationTokens)
	require.InDelta(t, 0.00834, row.InputCost, 1e-8)
	require.InDelta(t, 0.1248, row.CacheCreationCost, 1e-8)
	require.InDelta(t, 0.13314, row.TotalCost, 1e-8)
	require.InDelta(t, 0.13314*rate, row.ActualCost, 1e-8)
}

// The destination is overwritten before any network call. Redirects are not
// followed, so neither production URL construction nor a redirect can escape.
type cacheWriteHTTPPGUpstream struct {
	target *url.URL
	client *http.Client
}

func (u *cacheWriteHTTPPGUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	local := req.Clone(req.Context())
	local.URL.Scheme, local.URL.Host, local.Host = u.target.Scheme, u.target.Host, u.target.Host
	return u.client.Do(local)
}
func (u *cacheWriteHTTPPGUpstream) DoWithTLS(req *http.Request, proxy string, accountID int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxy, accountID, concurrency)
}

type cacheWriteHTTPPGCall struct {
	command service.UsageBillingCommand
	result  *service.UsageBillingApplyResult
	err     error
}

// Embed the complete production interface so unobserved operations (including
// batch-image balance holds/captures/releases) also delegate to the real SQL
// repository, rather than becoming test stubs.
type cacheWriteHTTPPGBillingObserver struct {
	service.UsageBillingRepository
	mu    sync.Mutex
	calls []cacheWriteHTTPPGCall
}

var _ service.UsageBillingRepository = (*cacheWriteHTTPPGBillingObserver)(nil)

func (r *cacheWriteHTTPPGBillingObserver) Apply(ctx context.Context, cmd *service.UsageBillingCommand) (*service.UsageBillingApplyResult, error) {
	result, err := r.UsageBillingRepository.Apply(ctx, cmd)
	copy := *cmd
	if cmd.CacheWriteCorrection != nil {
		correction := *cmd.CacheWriteCorrection
		copy.CacheWriteCorrection = &correction
	}
	r.mu.Lock()
	r.calls = append(r.calls, cacheWriteHTTPPGCall{command: copy, result: result, err: err})
	r.mu.Unlock()
	return result, err
}
func (r *cacheWriteHTTPPGBillingObserver) snapshot() []cacheWriteHTTPPGCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]cacheWriteHTTPPGCall(nil), r.calls...)
}
func (r *cacheWriteHTTPPGBillingObserver) corrections() []cacheWriteHTTPPGCall {
	var corrections []cacheWriteHTTPPGCall
	for _, call := range r.snapshot() {
		if call.command.CacheWriteCorrection != nil {
			corrections = append(corrections, call)
		}
	}
	return corrections
}

// Settings only are in memory to avoid changing unrelated tests' shared SQL
// settings. All money, token rows, quota counters and dedup keys live in SQL.
type cacheWriteHTTPPGSettings struct {
	mu     sync.RWMutex
	values map[string]string
}

func (r *cacheWriteHTTPPGSettings) Get(ctx context.Context, key string) (*service.Setting, error) {
	value, err := r.GetValue(ctx, key)
	if err != nil {
		return nil, err
	}
	return &service.Setting{Key: key, Value: value}, nil
}
func (r *cacheWriteHTTPPGSettings) GetValue(_ context.Context, key string) (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if value, ok := r.values[key]; ok {
		return value, nil
	}
	return "", service.ErrSettingNotFound
}
func (r *cacheWriteHTTPPGSettings) Set(_ context.Context, key, value string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.values[key] = value
	return nil
}
func (r *cacheWriteHTTPPGSettings) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]string)
	for _, key := range keys {
		if value, ok := r.values[key]; ok {
			out[key] = value
		}
	}
	return out, nil
}
func (r *cacheWriteHTTPPGSettings) SetMultiple(_ context.Context, values map[string]string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	maps.Copy(r.values, values)
	return nil
}
func (r *cacheWriteHTTPPGSettings) GetAll(_ context.Context) (map[string]string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return maps.Clone(r.values), nil
}
func (r *cacheWriteHTTPPGSettings) Delete(_ context.Context, key string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.values, key)
	return nil
}
