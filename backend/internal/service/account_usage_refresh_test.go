package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/adminaccess"
	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

type refreshUsageFetcher struct {
	calls atomic.Int32
	fetch func(context.Context) (*ClaudeUsageResponse, error)
}

func (f *refreshUsageFetcher) FetchUsage(ctx context.Context, _, _ string) (*ClaudeUsageResponse, error) {
	f.calls.Add(1)
	return f.fetch(ctx)
}
func (f *refreshUsageFetcher) FetchUsageWithOptions(ctx context.Context, _ *ClaudeUsageFetchOptions) (*ClaudeUsageResponse, error) {
	return f.FetchUsage(ctx, "", "")
}

type refreshUsageRepo struct {
	stubQuotaAccountRepo
	mu         sync.Mutex
	writes     int
	updates    map[string]any
	writeErr   error
	windowErr  error
	entered    chan struct{}
	release    chan struct{}
	writeCtx   context.Context
	scopeMoved *atomic.Bool
}

func (r *refreshUsageRepo) GetByID(ctx context.Context, id int64) (*Account, error) {
	a, err := r.stubQuotaAccountRepo.GetByID(ctx, id)
	if err == nil && r.scopeMoved != nil && r.scopeMoved.Load() {
		copy := *a
		copy.GroupIDs = []int64{2}
		return &copy, nil
	}
	return a, err
}

func (r *refreshUsageRepo) GetByIDs(ctx context.Context, ids []int64) ([]*Account, error) {
	var accounts []*Account
	for _, id := range ids {
		if a, err := r.GetByID(ctx, id); err == nil {
			accounts = append(accounts, a)
		}
	}
	return accounts, nil
}
func (r *refreshUsageRepo) UpdateExtra(ctx context.Context, _ int64, updates map[string]any) error {
	if r.entered != nil {
		r.entered <- struct{}{}
	}
	if r.release != nil {
		select {
		case <-r.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.writes++
	r.updates = updates
	r.writeCtx = ctx
	return r.writeErr
}
func (r *refreshUsageRepo) UpdateSessionWindowEnd(context.Context, int64, time.Time) error {
	return r.windowErr
}

func anthropicRefreshFixture() (*AccountUsageService, *Account, *refreshUsageFetcher, *refreshUsageRepo) {
	a := &Account{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeOAuth, Credentials: map[string]any{"access_token": "token"}}
	r := &refreshUsageRepo{stubQuotaAccountRepo: stubQuotaAccountRepo{accounts: map[int64]*Account{1: a}}}
	f := &refreshUsageFetcher{fetch: func(context.Context) (*ClaudeUsageResponse, error) {
		resp := &ClaudeUsageResponse{}
		resp.FiveHour.Utilization = 73
		resp.FiveHour.ResetsAt = time.Now().Add(time.Hour).Format(time.RFC3339)
		return resp, nil
	}}
	s := &AccountUsageService{accountRepo: r, usageLogRepo: &usageBatchLogRepoStub{}, usageFetcher: f, cache: NewUsageCache()}
	return s, a, f, r
}

func TestAccountUsageRefresh_AnthropicForceBypassesSuccessNotNegativeCache(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s, a, f, r := anthropicRefreshFixture()
		old := &ClaudeUsageResponse{}
		old.FiveHour.Utilization = 12
		s.cache.apiCache.Store(a.ID, &apiUsageCache{response: old, timestamp: time.Now()})
		u, err := s.GetUsageForAccount(t.Context(), a)
		require.NoError(t, err)
		require.Equal(t, 12.0, u.FiveHour.Utilization)
		require.Zero(t, f.calls.Load())
		u, err = s.GetUsageForAccount(t.Context(), a, true)
		require.NoError(t, err)
		require.Equal(t, 73.0, u.FiveHour.Utilization)
		require.EqualValues(t, 1, f.calls.Load())
		require.Equal(t, 0.73, r.updates["session_window_utilization"])

		limited := errors.New("upstream rate limited")
		s.cache.apiCache.Store(a.ID, &apiUsageCache{err: limited, timestamp: time.Now()})
		u, err = s.GetUsageForAccount(t.Context(), a, true)
		require.Nil(t, u)
		require.ErrorIs(t, err, limited)
		require.EqualValues(t, 1, f.calls.Load(), "force cannot bypass negative caching")
		limitedWithRetry := infraerrors.New(429, "UPSTREAM_ERROR", "rate limited").WithMetadata(map[string]string{"retry_after": "300"})
		s.cache.apiCache.Store(a.ID, &apiUsageCache{err: limitedWithRetry, timestamp: time.Now().Add(-2 * time.Minute)})
		_, err = s.GetUsageForAccount(t.Context(), a, true)
		require.ErrorIs(t, err, limitedWithRetry)
		require.EqualValues(t, 1, f.calls.Load(), "Retry-After outlives the default negative TTL")
	})
}

func TestAccountUsageRefresh_AnthropicForceSingleflightActuallyDeduplicates(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s, a, f, _ := anthropicRefreshFixture()
		old := &ClaudeUsageResponse{}
		old.FiveHour.Utilization = 12
		s.cache.apiCache.Store(a.ID, &apiUsageCache{response: old, timestamp: time.Now()})
		release := make(chan struct{})
		entered := make(chan struct{}, 1)
		fetch := f.fetch
		f.fetch = func(ctx context.Context) (*ClaudeUsageResponse, error) {
			entered <- struct{}{}
			<-release
			return fetch(ctx)
		}
		var wg sync.WaitGroup
		for range 12 {
			wg.Go(func() {
				u, err := s.GetUsageForAccount(t.Context(), a, true)
				if err != nil || u == nil || u.FiveHour.Utilization != 73 {
					t.Errorf("forced shared result: usage=%v err=%v", u, err)
				}
			})
		}
		<-entered // Let virtual time advance through the flight's jitter.
		synctest.Wait()
		require.EqualValues(t, 1, f.calls.Load(), "all callers must share the blocked upstream fetch")
		close(release)
		wg.Wait()
		require.EqualValues(t, 1, f.calls.Load())
	})
}

func TestAccountUsageRefresh_AnthropicFailureAndPersistence(t *testing.T) {
	for _, scenario := range []string{"nil", "empty", "fetch_error", "write_error", "window_error", "blocked_write"} {
		t.Run(scenario, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				s, a, f, r := anthropicRefreshFixture()
				switch scenario {
				case "nil":
					f.fetch = func(context.Context) (*ClaudeUsageResponse, error) { return nil, nil }
				case "empty":
					f.fetch = func(context.Context) (*ClaudeUsageResponse, error) { return &ClaudeUsageResponse{}, nil }
				case "fetch_error":
					f.fetch = func(context.Context) (*ClaudeUsageResponse, error) { return nil, errors.New("upstream failed") }
				case "write_error":
					r.writeErr = errors.New("write failed")
				case "window_error":
					r.windowErr = errors.New("window failed")
				case "blocked_write":
					r.entered, r.release = make(chan struct{}, 1), make(chan struct{})
				}
				done := make(chan struct{})
				var usage *UsageInfo
				var err error
				go func() { usage, err = s.GetUsageForAccount(t.Context(), a, true); close(done) }()
				if scenario == "blocked_write" {
					<-r.entered
					synctest.Wait()
					select {
					case <-done:
						t.Fatal("active success returned before UpdateExtra completed")
					default:
					}
					close(r.release)
					<-done
					require.NoError(t, err)
					require.NotNil(t, usage)
				} else {
					<-done
					require.Error(t, err)
					require.Nil(t, usage)
				}
			})
		})
	}
}

func TestAccountUsageRefresh_AnthropicBatchForceOnlyActivatesOAuth(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s, a, f, r := anthropicRefreshFixture()
		setup := &Account{ID: 2, Platform: PlatformAnthropic, Type: AccountTypeSetupToken}
		r.accounts[2] = setup
		for _, force := range []bool{false, true} {
			usage, failures, err := s.GetUsageBatch(t.Context(), []int64{a.ID, setup.ID}, force)
			require.NoError(t, err)
			require.Empty(t, failures)
			require.Equal(t, "passive", usage[setup.ID].Source)
			if force {
				require.Equal(t, 73.0, usage[a.ID].FiveHour.Utilization)
				require.EqualValues(t, 1, f.calls.Load())
			} else {
				require.Equal(t, "passive", usage[a.ID].Source)
				require.Zero(t, f.calls.Load())
			}
		}
	})
}

func quotaRefreshFixture(t *testing.T, account *Account, body string, status int, onUsage ...func()) (*AccountUsageService, *refreshUsageRepo, *atomic.Int32) {
	t.Helper()
	if account.Credentials == nil {
		account.Credentials = map[string]any{"chatgpt_account_id": "quota-account"}
	}
	r := &refreshUsageRepo{stubQuotaAccountRepo: stubQuotaAccountRepo{accounts: map[int64]*Account{account.ID: account}}}
	calls := &atomic.Int32{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		calls.Add(1)
		if strings.HasSuffix(req.URL.Path, "/usage") && len(onUsage) > 0 {
			onUsage[0]()
		}
		if req.Method != http.MethodGet || strings.Contains(req.URL.Path, "responses") || strings.Contains(req.URL.Path, "redeem") {
			t.Errorf("forced usage sent a non-read request: %s %s", req.Method, req.URL.Path)
		}
		if account.IsOpenAIAgentIdentity() && !strings.HasPrefix(req.Header.Get("Authorization"), "AgentAssertion ") {
			t.Error("Agent Identity quota read lost its assertion")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	tokens := &stubQuotaTokenCache{tokens: map[string]string{OpenAITokenCacheKey(account): "quota-token"}}
	q := NewOpenAIQuotaService(r, nil, NewOpenAITokenProvider(r, tokens, nil), newQuotaRedirectingFactory(srv), nil)
	return &AccountUsageService{accountRepo: r, openAIQuotaService: q, cache: NewUsageCache()}, r, calls
}

const refreshQuotaBody = `{"rate_limit":{"primary_window":{"used_percent":73,"reset_after_seconds":3600,"limit_window_seconds":18000},"secondary_window":{"used_percent":31,"reset_after_seconds":86400,"limit_window_seconds":604800}},"additional_rate_limits":[{"metered_feature":"codex_bengalfox","rate_limit":{"primary_window":{"used_percent":42,"reset_after_seconds":3600,"limit_window_seconds":18000}}}]}`

func TestAccountUsageRefresh_OpenAIForcePureReadPersistsWithoutReset(t *testing.T) {
	notifier := NewOpenAIQuotaAutoResetService(nil, nil, nil, nil, nil, nil, nil)
	openAIAutoResetNotifierRegistry.Lock()
	previous := openAIAutoResetNotifierRegistry.service
	openAIAutoResetNotifierRegistry.service = notifier
	openAIAutoResetNotifierRegistry.Unlock()
	t.Cleanup(func() {
		openAIAutoResetNotifierRegistry.Lock()
		openAIAutoResetNotifierRegistry.service = previous
		openAIAutoResetNotifierRegistry.Unlock()
		notifier.cancel()
	})
	for _, identity := range []string{"oauth", "agent_identity", "shadow"} {
		for _, restricted := range []bool{false, true} {
			t.Run(identity+"/restricted="+map[bool]string{false: "false", true: "true"}[restricted], func(t *testing.T) {
				a := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, GroupIDs: []int64{1}, Credentials: map[string]any{"chatgpt_account_id": "account"}, Extra: map[string]any{"codex_5h_used_percent": 12.0}}
				if identity == "agent_identity" {
					_, key := newTestAgentIdentityKey(t)
					a.Credentials["auth_mode"] = OpenAIAuthModeAgentIdentity
					a.Credentials["agent_private_key"] = key
					a.Credentials["agent_runtime_id"] = "runtime"
					a.Credentials["task_id"] = "task"
				}
				s, r, calls := quotaRefreshFixture(t, a, refreshQuotaBody, http.StatusOK)
				if identity == "shadow" {
					parentID := int64(2)
					parent := &Account{ID: parentID, Platform: PlatformOpenAI, Type: AccountTypeOAuth, GroupIDs: []int64{1}, Credentials: map[string]any{"access_token": "parent-token", "expires_at": time.Now().Add(time.Hour).Format(time.RFC3339), "chatgpt_account_id": "parent-account"}}
					r.accounts[parentID] = parent
					a.ParentAccountID, a.QuotaDimension = &parentID, QuotaDimensionSpark
				}
				ctx := t.Context()
				if restricted {
					ctx = adminaccess.WithPrincipal(ctx, &adminaccess.Principal{Policy: adminaccess.Policy{Rules: []adminaccess.Rule{{Permissions: []string{"accounts.usage.read"}, GroupIDs: []int64{1}, Platforms: []string{PlatformOpenAI}, AccountTypes: []string{AccountTypeOAuth}}}}})
				}
				s.cache.openAIProbeCache.Store(a.ID, time.Now())
				u, err := s.GetUsageForAccount(ctx, a, true)
				require.NoError(t, err)
				require.NotNil(t, u.FiveHour)
				expected := 73.0
				if identity == "shadow" {
					expected = 42
				}
				require.Equal(t, expected, u.FiveHour.Utilization)
				require.Equal(t, 1, r.writes)
				require.Equal(t, expected, r.updates["codex_5h_used_percent"])
				require.Equal(t, adminaccess.FromContext(ctx), adminaccess.FromContext(r.writeCtx))
				require.Positive(t, calls.Load())
				require.Empty(t, notifier.queue, "forced reads must not notify parent or account auto-reset")
			})
		}
	}
}

func TestAccountUsageRefresh_OpenAIForceFailuresDoNotReturnOldSuccess(t *testing.T) {
	for _, scenario := range []string{"upstream", "empty", "empty_rate_limit", "write"} {
		t.Run(scenario, func(t *testing.T) {
			a := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{"codex_5h_used_percent": 12.0}}
			body, status := refreshQuotaBody, http.StatusOK
			if scenario == "upstream" {
				status = http.StatusBadGateway
			}
			if scenario == "empty" {
				body = `{}`
			}
			if scenario == "empty_rate_limit" {
				body = `{"rate_limit":{}}`
			}
			s, r, _ := quotaRefreshFixture(t, a, body, status)
			if scenario == "write" {
				r.writeErr = errors.New("database write failed")
			}
			u, err := s.GetUsageForAccount(t.Context(), a, true)
			require.Error(t, err)
			require.Nil(t, u)
			require.Equal(t, 12.0, a.Extra["codex_5h_used_percent"], "failed persistence must not mutate old snapshot")
			if scenario != "write" {
				require.Zero(t, r.writes)
			}
		})
	}
}

func TestAccountUsageRefresh_OpenAIWaitsForWrite(t *testing.T) {
	a := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	s, r, _ := quotaRefreshFixture(t, a, refreshQuotaBody, http.StatusOK)
	r.entered, r.release = make(chan struct{}, 1), make(chan struct{})
	done := make(chan error, 1)
	go func() { _, err := s.GetUsageForAccount(t.Context(), a, true); done <- err }()
	select {
	case <-r.entered:
	case err := <-done:
		t.Fatalf("query failed before reaching persistence: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("write was never reached")
	}
	select {
	case err := <-done:
		t.Fatalf("returned before persistence completed: %v", err)
	default:
	}
	close(r.release)
	require.NoError(t, <-done)
	require.Equal(t, 1, r.writes)
}

func TestAccountUsageRefresh_NonForceCompatibility(t *testing.T) {
	t.Run("anthropic write failure remains best effort", func(t *testing.T) {
		s, a, f, r := anthropicRefreshFixture()
		r.writeErr = errors.New("database write failed")
		resp := &ClaudeUsageResponse{}
		resp.FiveHour.Utilization = 12
		s.cache.apiCache.Store(a.ID, &apiUsageCache{response: resp, timestamp: time.Now()})
		u, err := s.GetUsageForAccount(t.Context(), a)
		require.NoError(t, err)
		require.Equal(t, 12.0, u.FiveHour.Utilization)
		require.Zero(t, f.calls.Load())
	})
	t.Run("openai shadow failure keeps old snapshot", func(t *testing.T) {
		parentID := int64(2)
		a := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, ParentAccountID: &parentID, QuotaDimension: QuotaDimensionSpark, Extra: map[string]any{"codex_5h_used_percent": 12.0}}
		s, r, calls := quotaRefreshFixture(t, a, `{}`, http.StatusBadGateway)
		r.accounts[parentID] = &Account{ID: parentID, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"chatgpt_account_id": "parent", "access_token": "parent-token"}}
		u, err := s.GetUsageForAccount(t.Context(), a)
		require.NoError(t, err)
		require.Equal(t, 12.0, u.FiveHour.Utilization)
		require.Positive(t, calls.Load())
		require.Zero(t, r.writes)
	})
}

func TestAccountUsageRefresh_OpenAIForcePreservesScopeUntilWrite(t *testing.T) {
	for _, scenario := range []string{"denied_before_query", "denied_before_write"} {
		t.Run(scenario, func(t *testing.T) {
			a := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, GroupIDs: []int64{1}, Extra: map[string]any{"codex_5h_used_percent": 12.0}}
			var moved atomic.Bool
			s, r, calls := quotaRefreshFixture(t, a, refreshQuotaBody, http.StatusOK, func() { moved.Store(true) })
			r.scopeMoved = &moved
			if scenario == "denied_before_query" {
				a.GroupIDs = []int64{2}
			}
			ctx := adminaccess.WithPrincipal(t.Context(), &adminaccess.Principal{Policy: adminaccess.Policy{Rules: []adminaccess.Rule{{Permissions: []string{"accounts.usage.read"}, GroupIDs: []int64{1}, Platforms: []string{PlatformOpenAI}, AccountTypes: []string{AccountTypeOAuth}}}}})
			u, err := s.GetUsageForAccount(ctx, a, true)
			require.Nil(t, u)
			require.Equal(t, http.StatusNotFound, infraerrors.Code(err))
			require.Zero(t, r.writes)
			require.Equal(t, 12.0, a.Extra["codex_5h_used_percent"])
			if scenario == "denied_before_query" {
				require.Zero(t, calls.Load())
			} else {
				require.Positive(t, calls.Load())
			}
		})
	}
}

func TestAccountUsageRefresh_AntigravityForceCacheAndFlight(t *testing.T) {
	var fetches atomic.Int32
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(req.URL.Path, "fetchAvailableModels") {
			fetches.Add(1)
			<-release
			_, _ = w.Write([]byte(`{"models":{"claude-sonnet-4":{"quotaInfo":{"remainingFraction":0.25}}}}`))
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	oldURLs, oldAvailability := antigravity.BaseURLs, antigravity.DefaultURLAvailability
	antigravity.BaseURLs = []string{srv.URL}
	antigravity.DefaultURLAvailability = antigravity.NewURLAvailability(time.Minute)
	defer func() { antigravity.BaseURLs, antigravity.DefaultURLAvailability = oldURLs, oldAvailability }()
	a := &Account{ID: 1, Platform: PlatformAntigravity, Type: AccountTypeOAuth, Credentials: map[string]any{"access_token": "token"}}
	s := &AccountUsageService{cache: NewUsageCache(), antigravityQuotaFetcher: NewAntigravityQuotaFetcher(nil, nil)}
	s.cache.antigravityCache.Store(a.ID, &antigravityUsageCache{usageInfo: &UsageInfo{FiveHour: &UsageProgress{Utilization: 12}}, timestamp: time.Now()})
	u, err := s.getAntigravityUsage(t.Context(), a)
	require.NoError(t, err)
	require.Equal(t, 12.0, u.FiveHour.Utilization)
	require.Zero(t, fetches.Load())
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			u, err := s.getAntigravityUsage(t.Context(), a, true)
			if err != nil || u == nil || u.FiveHour == nil || u.FiveHour.Utilization != 75 {
				t.Errorf("forced Antigravity result: usage=%v err=%v", u, err)
			}
		})
	}
	require.Eventually(t, func() bool { return fetches.Load() > 0 }, 5*time.Second, time.Millisecond)
	// Keep the upstream blocked long enough for callers to join; the final count
	// checks real upstream requests rather than asserting a singleflight key.
	time.Sleep(30 * time.Millisecond)
	close(release)
	wg.Wait()
	require.EqualValues(t, 1, fetches.Load())
	negative := &UsageInfo{ErrorCode: errorCodeRateLimited, Error: "HTTP 429"}
	s.cache.antigravityCache.Store(a.ID, &antigravityUsageCache{usageInfo: negative, timestamp: time.Now()})
	u, err = s.getAntigravityUsage(t.Context(), a, true)
	require.NoError(t, err)
	require.Same(t, negative, u)
	require.EqualValues(t, 1, fetches.Load())
}
