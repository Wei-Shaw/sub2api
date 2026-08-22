package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/stretchr/testify/require"
)

type codexAnalyticsUsageRepoStub struct {
	stats      *usagestats.UsageStats
	trend      []usagestats.TrendDataPoint
	models     []usagestats.ModelStat
	statsCalls int
	trendCalls int
	modelCalls int
	statsStart time.Time
	statsEnd   time.Time
	trendStart time.Time
	trendEnd   time.Time
	modelStart time.Time
	modelEnd   time.Time
}

func (s *codexAnalyticsUsageRepoStub) GetAccountStatsAggregated(_ context.Context, _ int64, start, end time.Time) (*usagestats.UsageStats, error) {
	s.statsCalls++
	s.statsStart = start
	s.statsEnd = end
	return s.stats, nil
}

func (s *codexAnalyticsUsageRepoStub) GetUsageTrendWithFilters(_ context.Context, start, end time.Time, _ string, _, _, _, _ int64, _ string, _ *int16, _ *bool, _ *int8) ([]usagestats.TrendDataPoint, error) {
	s.trendCalls++
	s.trendStart = start
	s.trendEnd = end
	return s.trend, nil
}

func (s *codexAnalyticsUsageRepoStub) GetModelStatsWithFilters(_ context.Context, start, end time.Time, _, _, _, _ int64, _ *int16, _ *bool, _ *int8) ([]usagestats.ModelStat, error) {
	s.modelCalls++
	s.modelStart = start
	s.modelEnd = end
	return s.models, nil
}

type codexAnalyticsDoneSignalingContext struct {
	context.Context
	once   sync.Once
	signal chan struct{}
}

func (c *codexAnalyticsDoneSignalingContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.signal) })
	return c.Context.Done()
}

type codexAnalyticsCacheFakeEntry struct {
	value     []byte
	expiresAt time.Time
}

type codexAnalyticsCacheFake struct {
	mu        sync.Mutex
	now       time.Time
	entries   map[string]codexAnalyticsCacheFakeEntry
	getErr    error
	setErr    error
	deleteErr error
}

func newCodexAnalyticsCacheFake() *codexAnalyticsCacheFake {
	return &codexAnalyticsCacheFake{
		now:     time.Unix(0, 0),
		entries: make(map[string]codexAnalyticsCacheFakeEntry),
	}
}

func (f *codexAnalyticsCacheFake) Get(_ context.Context, key string) ([]byte, time.Duration, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.getErr != nil {
		return nil, 0, f.getErr
	}
	entry, ok := f.entries[key]
	if !ok || !entry.expiresAt.After(f.now) {
		delete(f.entries, key)
		return nil, 0, ErrCodexAnalyticsCacheMiss
	}
	return append([]byte(nil), entry.value...), entry.expiresAt.Sub(f.now), nil
}

func (f *codexAnalyticsCacheFake) Set(_ context.Context, key string, value []byte, ttl time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.setErr != nil {
		return f.setErr
	}
	f.entries[key] = codexAnalyticsCacheFakeEntry{
		value:     append([]byte(nil), value...),
		expiresAt: f.now.Add(ttl),
	}
	return nil
}

func (f *codexAnalyticsCacheFake) Delete(_ context.Context, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.deleteErr != nil {
		return f.deleteErr
	}
	delete(f.entries, key)
	return nil
}

func (f *codexAnalyticsCacheFake) seed(key string, value []byte, ttl time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entries[key] = codexAnalyticsCacheFakeEntry{
		value:     append([]byte(nil), value...),
		expiresAt: f.now.Add(ttl),
	}
}

func (f *codexAnalyticsCacheFake) advance(elapsed time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = f.now.Add(elapsed)
}

func (f *codexAnalyticsCacheFake) failGet(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.getErr = err
}

func (f *codexAnalyticsCacheFake) failSet(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.setErr = err
}

func newCodexAnalyticsTestService(t *testing.T, upstream *httptest.Server, usageRepo codexAnalyticsUsageRepository, analyticsCache CodexAnalyticsCache) *OpenAIQuotaService {
	t.Helper()
	account := &Account{
		ID:       100,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Status:   StatusActive,
		Credentials: map[string]any{
			"chatgpt_account_id": "chatgpt-account-100",
		},
	}
	accountRepo := &stubQuotaAccountRepo{accounts: map[int64]*Account{account.ID: account}}
	tokenCache := &stubQuotaTokenCache{tokens: map[string]string{OpenAITokenCacheKey(account): "test-access-token"}}
	tokenProvider := NewOpenAITokenProvider(accountRepo, tokenCache, nil)
	return NewOpenAIQuotaService(accountRepo, nil, tokenProvider, newQuotaRedirectingFactory(upstream), usageRepo, analyticsCache)
}

func newCodexAnalyticsPeriodUpstream(t *testing.T, sevenDay *OpenAIRateLimitWindow, calls map[string]int) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls != nil {
			calls[r.URL.Path]++
		}
		w.Header().Set("content-type", "application/json")
		switch r.URL.Path {
		case "/backend-api/wham/profiles/me":
			_ = json.NewEncoder(w).Encode(map[string]any{"stats": map[string]any{"daily_usage_buckets": []any{}}})
		case "/backend-api/wham/usage":
			_ = json.NewEncoder(w).Encode(OpenAIQuotaUsage{RateLimit: &OpenAIRateLimit{Allowed: true, PrimaryWindow: sevenDay}})
		case "/backend-api/wham/rate-limit-reset-credits":
			_, _ = w.Write([]byte(`{"available_count":0,"credits":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func requireCodexAnalyticsWarning(t *testing.T, warnings []OpenAICodexAnalyticsWarning, code string) {
	t.Helper()
	for _, warning := range warnings {
		if warning.Code == code {
			return
		}
	}
	require.Failf(t, "missing analytics warning", "code %q was not present in %#v", code, warnings)
}

func TestQueryCodexAnalyticsCachesMissThenHit(t *testing.T) {
	today := time.Now().UTC().Format("2006-01-02")
	calls := map[string]int{}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls[r.URL.Path]++
		w.Header().Set("content-type", "application/json")
		switch r.URL.Path {
		case "/backend-api/wham/profiles/me":
			_ = json.NewEncoder(w).Encode(map[string]any{"stats": map[string]any{
				"lifetime_tokens":          1000,
				"peak_daily_tokens":        400,
				"longest_running_turn_sec": 12,
				"current_streak_days":      3,
				"longest_streak_days":      8,
				"daily_usage_buckets":      []map[string]any{{"start_date": today, "tokens": 300}},
			}})
		case "/backend-api/wham/usage":
			_ = json.NewEncoder(w).Encode(OpenAIQuotaUsage{RateLimit: &OpenAIRateLimit{
				Allowed:         false,
				LimitReached:    true,
				PrimaryWindow:   &OpenAIRateLimitWindow{UsedPercent: 65, LimitWindowSeconds: 604800},
				SecondaryWindow: &OpenAIRateLimitWindow{UsedPercent: 25, LimitWindowSeconds: 18000},
			}})
		case "/backend-api/wham/rate-limit-reset-credits":
			_, _ = w.Write([]byte(`{"available_count":0,"credits":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	cache := newCodexAnalyticsCacheFake()
	usageRepo := &codexAnalyticsUsageRepoStub{
		stats: &usagestats.UsageStats{
			TotalRequests: 2, TotalInputTokens: 100, TotalOutputTokens: 50,
			TotalCacheTokens: 20, TotalCacheReadTokens: 15, TotalTokens: 170, TotalCost: 0.42,
		},
		trend: []usagestats.TrendDataPoint{{
			Date: today, Requests: 2, InputTokens: 100, OutputTokens: 50,
			CacheCreationTokens: 5, CacheReadTokens: 15, TotalTokens: 170, Cost: 0.42,
		}},
		models: []usagestats.ModelStat{{
			Model: "gpt-5-codex", Requests: 2, InputTokens: 100, OutputTokens: 50,
			CacheCreationTokens: 5, CacheReadTokens: 15, TotalTokens: 170, Cost: 0.42,
		}},
	}
	service := newCodexAnalyticsTestService(t, upstream, usageRepo, cache)

	first, err := service.QueryCodexAnalytics(context.Background(), 100, OpenAICodexAnalyticsQuery{Period: OpenAICodexAnalyticsCurrent7Days})
	require.NoError(t, err)
	require.False(t, first.Cache.Hit)
	require.Equal(t, int64(240), first.Cache.TTLSeconds)
	require.Equal(t, int64(300), *first.Summary.OfficialTotalTokens)
	require.Equal(t, int64(1000), *first.Profile.LifetimeTokens)
	require.Equal(t, int64(170), first.Summary.ManagedTotalTokens)
	require.Equal(t, float64(65), first.Summary.CurrentLimitUsedPercent)
	require.NotNil(t, first.RateLimits.FiveHour)
	require.Equal(t, int64(18000), first.RateLimits.FiveHour.LimitWindowSeconds)
	require.False(t, first.RateLimits.FiveHour.Allowed)
	require.True(t, first.RateLimits.FiveHour.LimitReached)
	require.NotNil(t, first.RateLimits.SevenDay)
	require.True(t, first.RateLimits.SevenDay.LimitReached)

	cache.advance(time.Minute)
	second, err := service.QueryCodexAnalytics(context.Background(), 100, OpenAICodexAnalyticsQuery{Period: OpenAICodexAnalyticsCurrent7Days})
	require.NoError(t, err)
	require.True(t, second.Cache.Hit)
	require.Equal(t, int64(180), second.Cache.TTLSeconds)
	require.Equal(t, 1, usageRepo.statsCalls)
	require.Equal(t, 1, usageRepo.trendCalls)
	require.Equal(t, 1, usageRepo.modelCalls)
	require.Equal(t, 1, calls["/backend-api/wham/profiles/me"])
	require.Equal(t, 1, calls["/backend-api/wham/usage"])
}

func TestQueryCodexAnalyticsLeaderCancellationDoesNotCancelFollower(t *testing.T) {
	usageStarted := make(chan struct{})
	releaseUsage := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseUsage) }) }
	defer release()

	var usageCalls atomic.Int32
	var profileCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		switch r.URL.Path {
		case "/backend-api/wham/usage":
			if usageCalls.Add(1) == 1 {
				close(usageStarted)
			}
			select {
			case <-releaseUsage:
				_ = json.NewEncoder(w).Encode(OpenAIQuotaUsage{})
			case <-r.Context().Done():
			}
		case "/backend-api/wham/rate-limit-reset-credits":
			_, _ = w.Write([]byte(`{"available_count":0,"credits":[]}`))
		case "/backend-api/wham/profiles/me":
			profileCalls.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{"stats": map[string]any{"daily_usage_buckets": []any{}}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(upstream.Close)

	service := newCodexAnalyticsTestService(t, upstream, &codexAnalyticsUsageRepoStub{}, nil)
	query := OpenAICodexAnalyticsQuery{Period: OpenAICodexAnalyticsCurrent7Days}
	type queryResult struct {
		analytics *OpenAICodexAnalytics
		err       error
	}

	leaderCtx, cancelLeader := context.WithCancel(context.Background())
	defer cancelLeader()
	leaderDone := make(chan queryResult, 1)
	go func() {
		analytics, err := service.QueryCodexAnalytics(leaderCtx, 100, query)
		leaderDone <- queryResult{analytics: analytics, err: err}
	}()

	select {
	case <-usageStarted:
	case <-time.After(time.Second):
		t.Fatal("leader did not start the shared analytics fetch")
	}

	followerWaiting := make(chan struct{})
	followerCtx := &codexAnalyticsDoneSignalingContext{Context: context.Background(), signal: followerWaiting}
	followerDone := make(chan queryResult, 1)
	go func() {
		analytics, err := service.QueryCodexAnalytics(followerCtx, 100, query)
		followerDone <- queryResult{analytics: analytics, err: err}
	}()

	select {
	case <-followerWaiting:
	case <-time.After(time.Second):
		t.Fatal("follower did not join the shared analytics fetch")
	}
	cancelLeader()

	var leader queryResult
	select {
	case leader = <-leaderDone:
	case <-time.After(time.Second):
		t.Fatal("canceled leader did not return promptly")
	}
	require.Nil(t, leader.analytics)
	require.ErrorIs(t, leader.err, context.Canceled)
	release()

	select {
	case follower := <-followerDone:
		require.NoError(t, follower.err)
		require.NotNil(t, follower.analytics)
	case <-time.After(time.Second):
		t.Fatal("follower did not receive the shared analytics result")
	}
	require.Equal(t, int32(1), usageCalls.Load())
	require.Equal(t, int32(1), profileCalls.Load())
}

func TestCodexOfficialPeriodTotalFiltersAndSumsBuckets(t *testing.T) {
	start := time.Date(2026, time.August, 10, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 7)
	profile := &OpenAICodexAnalyticsProfile{
		LifetimeTokens: func() *int64 { value := int64(9999); return &value }(),
		DailyUsageBuckets: []OpenAICodexDailyUsageBucket{
			{StartDate: "2026-08-09", Tokens: 50},
			{StartDate: "2026-08-10", Tokens: 100},
			{StartDate: "2026-08-16T12:00:00Z", Tokens: 200},
			{StartDate: "2026-08-17", Tokens: 400},
			{StartDate: "invalid", Tokens: 800},
		},
	}

	filterCodexProfileBuckets(profile, start, end)
	total := sumCodexOfficialPeriodTokens(profile, true)

	require.Equal(t, []OpenAICodexDailyUsageBucket{
		{StartDate: "2026-08-10", Tokens: 100},
		{StartDate: "2026-08-16", Tokens: 200},
	}, profile.DailyUsageBuckets)
	require.NotNil(t, total)
	require.Equal(t, int64(300), *total)
	require.Nil(t, sumCodexOfficialPeriodTokens(profile, false))
	emptyTotal := sumCodexOfficialPeriodTokens(&OpenAICodexAnalyticsProfile{}, true)
	require.NotNil(t, emptyTotal)
	require.Zero(t, *emptyTotal)
}

func TestQueryCodexAnalyticsUsesExactOfficialCurrentWindow(t *testing.T) {
	resetAt := time.Now().UTC().Truncate(time.Second).Add(36 * time.Hour)
	windowSeconds := int64((7 * 24 * time.Hour) / time.Second)
	upstream := newCodexAnalyticsPeriodUpstream(t, &OpenAIRateLimitWindow{
		LimitWindowSeconds: windowSeconds,
		ResetAt:            resetAt.Unix(),
	}, nil)
	usageRepo := &codexAnalyticsUsageRepoStub{}
	service := newCodexAnalyticsTestService(t, upstream, usageRepo, nil)

	result, err := service.QueryCodexAnalytics(context.Background(), 100, OpenAICodexAnalyticsQuery{Period: OpenAICodexAnalyticsCurrent7Days})

	require.NoError(t, err)
	wantStart := resetAt.Add(-7 * 24 * time.Hour)
	require.Equal(t, OpenAICodexAnalyticsCurrent7Days, result.PeriodMode)
	require.Equal(t, wantStart.Unix(), result.PeriodStart)
	require.Equal(t, result.FetchedAt, result.PeriodEnd)
	require.Equal(t, wantStart, usageRepo.statsStart)
	require.Equal(t, time.Unix(result.PeriodEnd, 0).UTC(), usageRepo.statsEnd)
	require.Equal(t, usageRepo.statsStart, usageRepo.trendStart)
	require.Equal(t, usageRepo.statsEnd, usageRepo.trendEnd)
	require.Equal(t, usageRepo.statsStart, usageRepo.modelStart)
	require.Equal(t, usageRepo.statsEnd, usageRepo.modelEnd)
	requireCodexAnalyticsWarning(t, result.Warnings, "official_daily_buckets_approximate_period")
}

func TestQueryCodexAnalyticsFallsBackToExactRecentSevenDays(t *testing.T) {
	upstream := newCodexAnalyticsPeriodUpstream(t, &OpenAIRateLimitWindow{
		LimitWindowSeconds: int64((7 * 24 * time.Hour) / time.Second),
		ResetAt:            time.Now().Add(-time.Minute).Unix(),
	}, nil)
	usageRepo := &codexAnalyticsUsageRepoStub{}
	service := newCodexAnalyticsTestService(t, upstream, usageRepo, nil)

	result, err := service.QueryCodexAnalytics(context.Background(), 100, OpenAICodexAnalyticsQuery{Period: OpenAICodexAnalyticsCurrent7Days})

	require.NoError(t, err)
	require.Equal(t, OpenAICodexAnalyticsCurrent7Days, result.PeriodMode)
	require.Equal(t, int64((7*24*time.Hour)/time.Second), result.PeriodEnd-result.PeriodStart)
	require.Equal(t, time.Unix(result.PeriodStart, 0).UTC(), usageRepo.statsStart)
	require.Equal(t, time.Unix(result.PeriodEnd, 0).UTC(), usageRepo.statsEnd)
	requireCodexAnalyticsWarning(t, result.Warnings, "current_7d_window_unavailable")
}
func TestQueryCodexAnalyticsRejectsNonSevenDayOfficialWindow(t *testing.T) {
	upstream := newCodexAnalyticsPeriodUpstream(t, &OpenAIRateLimitWindow{
		LimitWindowSeconds: int64((24 * time.Hour) / time.Second),
		ResetAt:            time.Now().Add(12 * time.Hour).Unix(),
	}, nil)
	usageRepo := &codexAnalyticsUsageRepoStub{}
	service := newCodexAnalyticsTestService(t, upstream, usageRepo, nil)

	result, err := service.QueryCodexAnalytics(context.Background(), 100, OpenAICodexAnalyticsQuery{Period: OpenAICodexAnalyticsCurrent7Days})

	require.NoError(t, err)
	require.Nil(t, result.RateLimits.SevenDay)
	require.Equal(t, openAICodexAnalyticsSevenDaySeconds, result.PeriodEnd-result.PeriodStart)
	require.Equal(t, time.Unix(result.PeriodStart, 0).UTC(), usageRepo.statsStart)
	require.Equal(t, time.Unix(result.PeriodEnd, 0).UTC(), usageRepo.statsEnd)
	requireCodexAnalyticsWarning(t, result.Warnings, "current_7d_window_unavailable")
}

func TestQueryCodexAnalyticsSeparatesCurrentAndRecentCaches(t *testing.T) {
	calls := map[string]int{}
	upstream := newCodexAnalyticsPeriodUpstream(t, &OpenAIRateLimitWindow{LimitWindowSeconds: int64((7 * 24 * time.Hour) / time.Second)}, calls)
	cache := newCodexAnalyticsCacheFake()
	usageRepo := &codexAnalyticsUsageRepoStub{}
	service := newCodexAnalyticsTestService(t, upstream, usageRepo, cache)

	current, err := service.QueryCodexAnalytics(context.Background(), 100, OpenAICodexAnalyticsQuery{Period: OpenAICodexAnalyticsCurrent7Days})
	require.NoError(t, err)
	recent, err := service.QueryCodexAnalytics(context.Background(), 100, OpenAICodexAnalyticsQuery{Period: OpenAICodexAnalyticsRecent, Days: 7})
	require.NoError(t, err)
	cachedCurrent, err := service.QueryCodexAnalytics(context.Background(), 100, OpenAICodexAnalyticsQuery{Period: OpenAICodexAnalyticsCurrent7Days})
	require.NoError(t, err)

	require.False(t, current.Cache.Hit)
	require.False(t, recent.Cache.Hit)
	require.True(t, cachedCurrent.Cache.Hit)
	require.Equal(t, OpenAICodexAnalyticsRecent, recent.PeriodMode)
	require.Equal(t, 2, usageRepo.statsCalls)
	require.Equal(t, 2, calls["/backend-api/wham/usage"])
}

func TestQueryCodexAnalyticsInvalidatesCurrentCacheAfterReset(t *testing.T) {
	resetAt := time.Now().UTC().Truncate(time.Second).Add(24 * time.Hour)
	upstream := newCodexAnalyticsPeriodUpstream(t, &OpenAIRateLimitWindow{
		LimitWindowSeconds: int64((7 * 24 * time.Hour) / time.Second),
		ResetAt:            resetAt.Unix(),
	}, nil)
	cache := newCodexAnalyticsCacheFake()
	usageRepo := &codexAnalyticsUsageRepoStub{}
	service := newCodexAnalyticsTestService(t, upstream, usageRepo, cache)
	query := OpenAICodexAnalyticsQuery{Period: OpenAICodexAnalyticsCurrent7Days}
	stale := OpenAICodexAnalytics{
		AccountID:  100,
		PeriodMode: OpenAICodexAnalyticsCurrent7Days,
		RateLimits: OpenAICodexAnalyticsRateLimits{SevenDay: &OpenAICodexAnalyticsRateLimitWindow{ResetAt: time.Now().Add(-time.Second).Unix()}},
	}
	raw, err := json.Marshal(stale)
	require.NoError(t, err)
	cache.seed(openAICodexAnalyticsCacheKey(100, query), raw, openAICodexAnalyticsTTL)

	result, err := service.QueryCodexAnalytics(context.Background(), 100, query)

	require.NoError(t, err)
	require.False(t, result.Cache.Hit)
	require.Equal(t, resetAt.Unix(), result.RateLimits.SevenDay.ResetAt)
	require.Equal(t, 1, usageRepo.statsCalls)
}

func TestCodexAnalyticsCacheWarnings(t *testing.T) {
	query := OpenAICodexAnalyticsQuery{Period: OpenAICodexAnalyticsCurrent7Days}
	key := openAICodexAnalyticsCacheKey(100, query)

	t.Run("read failure", func(t *testing.T) {
		cache := newCodexAnalyticsCacheFake()
		cache.failGet(errors.New("cache unavailable"))
		service := &OpenAIQuotaService{analyticsCache: cache}

		cached, warning := service.readCodexAnalyticsCache(context.Background(), key, query)

		require.Nil(t, cached)
		require.NotNil(t, warning)
		require.Equal(t, "cache_read_failed", warning.Code)
	})

	t.Run("invalid cached value", func(t *testing.T) {
		cache := newCodexAnalyticsCacheFake()
		cache.seed(key, []byte("invalid json"), openAICodexAnalyticsTTL)
		service := &OpenAIQuotaService{analyticsCache: cache}

		cached, warning := service.readCodexAnalyticsCache(context.Background(), key, query)

		require.Nil(t, cached)
		require.NotNil(t, warning)
		require.Equal(t, "cache_read_failed", warning.Code)
		_, _, err := cache.Get(context.Background(), key)
		require.ErrorIs(t, err, ErrCodexAnalyticsCacheMiss)
	})

	t.Run("write failure", func(t *testing.T) {
		cache := newCodexAnalyticsCacheFake()
		cache.failSet(errors.New("cache unavailable"))
		service := &OpenAIQuotaService{analyticsCache: cache}
		result := &OpenAICodexAnalytics{PeriodMode: OpenAICodexAnalyticsRecent}

		warning := service.writeCodexAnalyticsCache(context.Background(), key, result)

		require.NotNil(t, warning)
		require.Equal(t, "cache_write_failed", warning.Code)
		require.Equal(t, OpenAICodexAnalyticsCache{}, result.Cache)
	})
}

func TestQueryCodexAnalyticsPreservesProfile401And429(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusTooManyRequests} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("content-type", "application/json")
				switch r.URL.Path {
				case "/backend-api/wham/usage":
					_ = json.NewEncoder(w).Encode(OpenAIQuotaUsage{})
				case "/backend-api/wham/rate-limit-reset-credits":
					_, _ = w.Write([]byte(`{"available_count":0,"credits":[]}`))
				case "/backend-api/wham/profiles/me":
					w.WriteHeader(status)
					_, _ = w.Write([]byte(`{"access_token":"secret-upstream-token"}`))
				default:
					http.NotFound(w, r)
				}
			}))
			defer upstream.Close()

			service := newCodexAnalyticsTestService(t, upstream, &codexAnalyticsUsageRepoStub{}, nil)
			result, err := service.QueryCodexAnalytics(context.Background(), 100, OpenAICodexAnalyticsQuery{Period: OpenAICodexAnalyticsCurrent7Days})

			require.Nil(t, result)
			require.Error(t, err)
			gotStatus, _ := infraerrors.ToHTTP(err)
			require.Equal(t, status, gotStatus)
			require.NotContains(t, err.Error(), "secret-upstream-token")
		})
	}
}
