package repository

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
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
