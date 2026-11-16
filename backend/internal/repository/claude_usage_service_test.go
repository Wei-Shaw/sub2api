package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type ClaudeUsageServiceSuite struct {
	suite.Suite
	srv     *httptest.Server
	fetcher *claudeUsageService
}

func (s *ClaudeUsageServiceSuite) TearDownTest() {
	if s.srv != nil {
		s.srv.Close()
		s.srv = nil
	}
}

// usageRequestCapture holds captured request data for assertions in the main goroutine.
type usageRequestCapture struct {
	authorization string
	anthropicBeta string
}

func (s *ClaudeUsageServiceSuite) TestFetchUsage_Success() {
	var captured usageRequestCapture

	s.srv = newLocalTestServer(s.T(), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured.authorization = r.Header.Get("Authorization")
		captured.anthropicBeta = r.Header.Get("anthropic-beta")

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
  "five_hour": {"utilization": 12.5, "resets_at": "2025-01-01T00:00:00Z"},
  "seven_day": {"utilization": 34.0, "resets_at": "2025-01-08T00:00:00Z"},
  "seven_day_sonnet": {"utilization": 56.0, "resets_at": "2025-01-08T00:00:00Z"}
}`)
	}))

	s.fetcher = &claudeUsageService{
		usageURL:          s.srv.URL,
		allowPrivateHosts: true,
	}

	resp, err := s.fetcher.FetchUsage(context.Background(), "at", "")
	require.NoError(s.T(), err, "FetchUsage")
	require.Equal(s.T(), 12.5, resp.FiveHour.Utilization, "FiveHour utilization mismatch")
	require.Equal(s.T(), 34.0, resp.SevenDay.Utilization, "SevenDay utilization mismatch")
	require.Equal(s.T(), 56.0, resp.SevenDaySonnet.Utilization, "SevenDaySonnet utilization mismatch")

	// Assertions on captured request data
	require.Equal(s.T(), "Bearer at", captured.authorization, "Authorization header mismatch")
	require.Equal(s.T(), "oauth-2025-04-20", captured.anthropicBeta, "anthropic-beta header mismatch")
}

type claudeUsageWindowAccountRepo struct {
	service.AccountRepository
	account *service.Account
}

func (r *claudeUsageWindowAccountRepo) GetByID(context.Context, int64) (*service.Account, error) {
	return r.account, nil
}

func (r *claudeUsageWindowAccountRepo) UpdateExtra(_ context.Context, _ int64, updates map[string]any) error {
	for key, value := range updates {
		r.account.Extra[key] = value
	}
	return nil
}

type claudeUsageWindowLogRepo struct {
	service.UsageLogRepository
}

func (*claudeUsageWindowLogRepo) GetAccountWindowStats(context.Context, int64, time.Time) (*usagestats.AccountStats, error) {
	return &usagestats.AccountStats{}, nil
}

func TestClaudeUsageFetcher_ScopedFableRoundTrip(t *testing.T) {
	resetAt := time.Now().Add(72 * time.Hour).UTC().Truncate(time.Microsecond)
	for _, tc := range []struct {
		name      string
		percent   string
		legacy    string
		wantUsage float64
	}{
		{name: "scoped", percent: "5.25", wantUsage: 5.25},
		{name: "zero overrides passive", percent: "0", wantUsage: 0},
		{name: "missing falls back to passive", percent: "null", wantUsage: 87},
		{name: "legacy takes precedence", percent: "5.25", legacy: fmt.Sprintf(`{"utilization":56,"resets_at":%q}`, resetAt.Format(time.RFC3339Nano)), wantUsage: 56},
	} {
		t.Run(tc.name, func(t *testing.T) {
			legacy := tc.legacy
			if legacy == "" {
				legacy = "null"
			}
			requests := make(chan [2]string, 2)
			srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests <- [2]string{r.Method, r.URL.Path}
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprintf(w, `{"five_hour":{"utilization":12},"seven_day_overage_included":%s,"limits":[{"kind":"weekly_scoped","group":"weekly","percent":%s,"resets_at":%q,"scope":{"model":{"display_name":"Fable"},"surface":null},"is_active":false}]}`, legacy, tc.percent, resetAt.Format(time.RFC3339Nano))
			}))
			defer srv.Close()
			fetcher := &claudeUsageService{usageURL: srv.URL + "/api/oauth/usage", allowPrivateHosts: true}
			account := &service.Account{
				ID: 1, Platform: service.PlatformAnthropic, Type: service.AccountTypeOAuth,
				Status: service.StatusActive, Credentials: map[string]any{"access_token": "test-token"},
				Extra: map[string]any{
					"passive_usage_7d_oi_utilization": 0.87,
					"passive_usage_7d_oi_reset":       resetAt.Unix(),
				},
			}
			repo := &claudeUsageWindowAccountRepo{account: account}
			svc := service.NewAccountUsageService(repo, &claudeUsageWindowLogRepo{}, fetcher,
				nil, nil, nil, nil, nil, service.NewUsageCache(), nil, nil)
			for range 2 {
				usage, err := svc.GetUsageForAccount(t.Context(), account)
				require.NoError(t, err)
				require.NotNil(t, usage.SevenDayFable)
				require.Equal(t, tc.wantUsage, usage.SevenDayFable.Utilization)
				require.NotNil(t, usage.SevenDayFable.ResetsAt)
				require.Equal(t, resetAt.Unix(), usage.SevenDayFable.ResetsAt.Unix())
			}
			passive, err := svc.GetPassiveUsage(t.Context(), account.ID)
			require.NoError(t, err)
			require.NotNil(t, passive.SevenDayFable)
			require.InDelta(t, tc.wantUsage, passive.SevenDayFable.Utilization, 1e-9)
			require.Equal(t, resetAt.Unix(), passive.SevenDayFable.ResetsAt.Unix())
			require.InDelta(t, tc.wantUsage/100, account.Extra["passive_usage_7d_oi_utilization"], 1e-9)
			require.Len(t, requests, 1, "the second query must reuse the decoded API response")
			require.Equal(t, [2]string{"GET", "/api/oauth/usage"}, <-requests)
		})
	}
}

func (s *ClaudeUsageServiceSuite) TestFetchUsage_NonOK() {
	s.srv = newLocalTestServer(s.T(), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, "nope")
	}))

	s.fetcher = &claudeUsageService{
		usageURL:          s.srv.URL,
		allowPrivateHosts: true,
	}

	_, err := s.fetcher.FetchUsage(context.Background(), "at", "")
	require.Error(s.T(), err)
	require.ErrorContains(s.T(), err, "status 401")
	require.ErrorContains(s.T(), err, "nope")
}

func (s *ClaudeUsageServiceSuite) TestFetchUsage_BadJSON() {
	s.srv = newLocalTestServer(s.T(), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, "not-json")
	}))

	s.fetcher = &claudeUsageService{
		usageURL:          s.srv.URL,
		allowPrivateHosts: true,
	}

	_, err := s.fetcher.FetchUsage(context.Background(), "at", "")
	require.Error(s.T(), err)
	require.ErrorContains(s.T(), err, "decode response failed")
}

func (s *ClaudeUsageServiceSuite) TestFetchUsage_ContextCancel() {
	s.srv = newLocalTestServer(s.T(), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Never respond - simulate slow server
		<-r.Context().Done()
	}))

	s.fetcher = &claudeUsageService{
		usageURL:          s.srv.URL,
		allowPrivateHosts: true,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := s.fetcher.FetchUsage(ctx, "at", "")
	require.Error(s.T(), err, "expected error for cancelled context")
}

func (s *ClaudeUsageServiceSuite) TestFetchUsage_InvalidProxyReturnsError() {
	s.fetcher = &claudeUsageService{
		usageURL:          "http://example.com",
		allowPrivateHosts: true,
	}

	_, err := s.fetcher.FetchUsage(context.Background(), "at", "://bad-proxy-url")
	require.Error(s.T(), err)
	require.ErrorContains(s.T(), err, "create http client failed")
}

func TestClaudeUsageServiceSuite(t *testing.T) {
	suite.Run(t, new(ClaudeUsageServiceSuite))
}

type claudeUsageRecoveryRepo struct {
	service.AccountRepository
	mu         sync.Mutex
	account    *service.Account
	clearCalls int
	clearErr   error
	requests   atomic.Int32
}

func (r *claudeUsageRecoveryRepo) GetByID(context.Context, int64) (*service.Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	copy := *r.account
	raw, err := json.Marshal(r.account.Extra)
	if err != nil {
		return nil, err
	}
	copy.Extra = nil
	if err := json.Unmarshal(raw, &copy.Extra); err != nil {
		return nil, err
	}
	return &copy, nil
}

func (r *claudeUsageRecoveryRepo) UpdateExtra(_ context.Context, _ int64, updates map[string]any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for key, value := range updates {
		r.account.Extra[key] = value
	}
	r.account.UpdatedAt = time.Now()
	return nil
}

func (r *claudeUsageRecoveryRepo) ClearModelRateLimit(_ context.Context, _ int64, scope string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.clearCalls++
	if r.clearErr != nil {
		return r.clearErr
	}
	limits, ok := r.account.Extra["model_rate_limits"].(map[string]any)
	if !ok {
		return errors.New("model rate limits missing from recovery fixture")
	}
	delete(limits, scope)
	r.account.UpdatedAt = time.Now()
	return nil
}

func TestClaudeUsageFetcher_RecoversFableModelQuota(t *testing.T) {
	reset := time.Now().Add(72 * time.Hour).UTC().Truncate(time.Second)
	scoped := func(percent string, reset time.Time, model, surface string) string {
		return fmt.Sprintf(`{"limits":[{"kind":"weekly_scoped","group":"weekly","percent":%s,"resets_at":%q,"scope":{"model":{"display_name":%q},"surface":%s}}]}`, percent, reset.Format(time.RFC3339), model, surface)
	}
	for _, tc := range []struct {
		name, response, reason string
		wantClear              bool
	}{
		{name: "scoped zero", response: scoped("0", reset, "Fable", "null"), wantClear: true},
		{name: "available quota", response: scoped("15", reset, "Fable", "null"), wantClear: true},
		{name: "below recovery threshold", response: scoped("94", reset, "Fable", "null"), wantClear: true},
		{name: "at recovery threshold", response: scoped("95", reset, "Fable", "null")},
		{name: "legacy zero", response: fmt.Sprintf(`{"seven_day_overage_included":{"utilization":0,"resets_at":%q}}`, reset.Format(time.RFC3339)), wantClear: true},
		{name: "missing percent", response: scoped("null", reset, "Fable", "null")},
		{name: "missing legacy utilization", response: fmt.Sprintf(`{"seven_day_overage_included":{"resets_at":%q}}`, reset.Format(time.RFC3339))},
		{name: "null legacy utilization", response: fmt.Sprintf(`{"seven_day_overage_included":{"utilization":null,"resets_at":%q}}`, reset.Format(time.RFC3339))},
		{name: "no active window", response: `{}`},
		{name: "expired active window", response: scoped("0", time.Now().Add(-time.Hour), "Fable", "null")},
		{name: "still exhausted", response: scoped("100", reset, "Fable", "null")},
		{name: "other model", response: scoped("0", reset, "Sonnet", "null")},
		{name: "restricted surface", response: scoped("0", reset, "Fable", `"claude_code"`)},
		{name: "credits required", response: scoped("0", reset, "Fable", "null"), reason: "anthropic_fable_credits_required"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo, svc := newClaudeUsageRecoveryFixture(t, tc.response, nil)
			if tc.reason != "" {
				limits, ok := repo.account.Extra["model_rate_limits"].(map[string]any)
				require.True(t, ok)
				limit, ok := limits["claude-fable-5"].(map[string]any)
				require.True(t, ok)
				limit["reason"] = tc.reason
			}
			usage := getClaudeRecoveryUsage(t, repo, svc)
			current, err := repo.GetByID(t.Context(), 1)
			require.NoError(t, err)
			if tc.wantClear {
				require.NotNil(t, usage.SevenDayFable)
				require.Less(t, usage.SevenDayFable.Utilization, 100.0)
				require.Zero(t, current.GetModelRateLimitRemainingTime("claude-fable-5-2"), "fresh upstream quota must recover the locally blocked model")
				require.Equal(t, 1, repo.clearCalls)
			} else {
				require.Positive(t, current.GetModelRateLimitRemainingTime("claude-fable-5-2"))
				require.Zero(t, repo.clearCalls)
			}
			require.Positive(t, current.GetModelRateLimitRemainingTime("claude-sonnet-4-6"), "other model limits must survive")
		})
	}
}

func newClaudeUsageRecoveryFixture(t *testing.T, response string, duringFetch func(*claudeUsageRecoveryRepo)) (*claudeUsageRecoveryRepo, *service.AccountUsageService) {
	t.Helper()
	reset := time.Now().Add(72 * time.Hour).UTC().Truncate(time.Second)
	repo := &claudeUsageRecoveryRepo{account: &service.Account{
		ID: 1, Platform: service.PlatformAnthropic, Type: service.AccountTypeOAuth,
		Status: service.StatusActive, Credentials: map[string]any{"access_token": "test-token"},
		UpdatedAt: time.Now().Add(-time.Hour),
		Extra: map[string]any{
			"passive_usage_7d_oi_utilization": 1.0,
			"passive_usage_7d_oi_reset":       reset.Unix(),
			"model_rate_limits": map[string]any{
				"claude-fable-5": map[string]any{
					"reason": "anthropic_7d_oi_window_exhausted", "rate_limit_reset_at": reset.Format(time.RFC3339),
				},
				"claude-sonnet-4-6": map[string]any{"rate_limit_reset_at": reset.Format(time.RFC3339)},
			},
		},
	}}
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		repo.requests.Add(1)
		if duringFetch != nil {
			duringFetch(repo)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, response)
	}))
	t.Cleanup(srv.Close)
	fetcher := &claudeUsageService{usageURL: srv.URL, allowPrivateHosts: true}
	svc := service.NewAccountUsageService(repo, &claudeUsageWindowLogRepo{}, fetcher,
		nil, nil, nil, nil, nil, service.NewUsageCache(), nil, nil)
	return repo, svc
}

func getClaudeRecoveryUsage(t *testing.T, repo *claudeUsageRecoveryRepo, svc *service.AccountUsageService) *service.UsageInfo {
	t.Helper()
	snapshot, err := repo.GetByID(t.Context(), 1)
	require.NoError(t, err)
	usage, err := svc.GetUsageForAccount(t.Context(), snapshot)
	require.NoError(t, err)
	return usage
}

func TestClaudeUsageFetcher_CachedQuotaPreservesNewFableLimit(t *testing.T) {
	reset := time.Now().Add(72 * time.Hour).UTC().Format(time.RFC3339)
	response := fmt.Sprintf(`{"seven_day_overage_included":{"utilization":0,"resets_at":%q}}`, reset)
	repo, svc := newClaudeUsageRecoveryFixture(t, response, nil)
	getClaudeRecoveryUsage(t, repo, svc)
	require.Equal(t, 1, repo.clearCalls)
	repo.mu.Lock()
	limits, ok := repo.account.Extra["model_rate_limits"].(map[string]any)
	if ok {
		limits["claude-fable-5"] = map[string]any{
			"reason": "anthropic_7d_oi_window_exhausted", "rate_limit_reset_at": reset,
		}
		repo.account.UpdatedAt = time.Now()
	}
	repo.mu.Unlock()
	require.True(t, ok)
	getClaudeRecoveryUsage(t, repo, svc)
	current, err := repo.GetByID(t.Context(), 1)
	require.NoError(t, err)
	require.EqualValues(t, 1, repo.requests.Load())
	require.Positive(t, current.GetModelRateLimitRemainingTime("claude-fable-5-2"))
	require.Equal(t, 1, repo.clearCalls, "a cached response must not trigger another recovery")
	require.EqualValues(t, 1, repo.requests.Load())
}

func TestClaudeUsageFetcher_ConcurrentFableLimitSurvives(t *testing.T) {
	reset := time.Now().Add(72 * time.Hour).UTC().Format(time.RFC3339)
	response := fmt.Sprintf(`{"seven_day_overage_included":{"utilization":0,"resets_at":%q}}`, reset)
	repo, svc := newClaudeUsageRecoveryFixture(t, response, func(r *claudeUsageRecoveryRepo) {
		r.mu.Lock()
		defer r.mu.Unlock()
		// A new 429 may have the exact same reset and reason. Its row version
		// must still distinguish it from the generation read before the fetch.
		r.account.UpdatedAt = time.Now()
	})
	getClaudeRecoveryUsage(t, repo, svc)
	current, err := repo.GetByID(t.Context(), 1)
	require.NoError(t, err)
	require.Positive(t, current.GetModelRateLimitRemainingTime("claude-fable-5-2"))
	require.Equal(t, 1, repo.clearCalls)
}

func TestClaudeUsageFetcher_RecoveryErrorPreservesFableLimit(t *testing.T) {
	reset := time.Now().Add(72 * time.Hour).UTC().Format(time.RFC3339)
	response := fmt.Sprintf(`{"seven_day_overage_included":{"utilization":0,"resets_at":%q}}`, reset)
	repo, svc := newClaudeUsageRecoveryFixture(t, response, nil)
	repo.clearErr = errors.New("persistence unavailable")
	getClaudeRecoveryUsage(t, repo, svc)
	current, err := repo.GetByID(t.Context(), 1)
	require.NoError(t, err)
	require.Positive(t, current.GetModelRateLimitRemainingTime("claude-fable-5-2"))
	require.Equal(t, 1, repo.clearCalls)
}

func (r *claudeUsageRecoveryRepo) RecoverModelRateLimitIfUnchanged(_ context.Context, _ int64, scope string, expected json.RawMessage, expectedUpdatedAt time.Time, updates map[string]any) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.clearCalls++
	if r.clearErr != nil {
		return false, r.clearErr
	}
	limits, ok := r.account.Extra["model_rate_limits"].(map[string]any)
	if !ok {
		return false, nil
	}
	actual, err := json.Marshal(limits[scope])
	if err != nil {
		return false, err
	}
	if !r.account.UpdatedAt.Equal(expectedUpdatedAt) || string(actual) != string(expected) {
		return false, nil
	}
	for key, value := range updates {
		r.account.Extra[key] = value
	}
	delete(limits, scope)
	r.account.UpdatedAt = time.Now()
	return true, nil
}
