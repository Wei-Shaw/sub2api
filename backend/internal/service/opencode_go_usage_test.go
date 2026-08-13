package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
)

// openCodeGoUsageFixture is the verified upstream 200 response (2026-08-13).
const openCodeGoUsageFixture = `{"usage":{"rolling":{"status":"ok","percent":6,"resetsAt":"2026-08-13T18:26:39.281Z"},"weekly":{"status":"ok","percent":2,"resetsAt":"2026-08-17T00:00:00.281Z"},"monthly":{"status":"ok","percent":1,"resetsAt":"2026-09-13T13:24:47.281Z"}}}`

type openCodeGoUsageTestRepo struct {
	AccountRepository
	mu       sync.Mutex
	accounts map[int64]*Account
	due      []Account
}

func (r *openCodeGoUsageTestRepo) GetByID(_ context.Context, id int64) (*Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	account := r.accounts[id]
	if account == nil {
		return nil, ErrAccountNotFound
	}
	clone := *account
	clone.Credentials = mergeMap(nil, account.Credentials)
	clone.Extra = mergeMap(nil, account.Extra)
	return &clone, nil
}

func (r *openCodeGoUsageTestRepo) GetByIDs(_ context.Context, ids []int64) ([]*Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]*Account, 0, len(ids))
	for _, id := range ids {
		if account := r.accounts[id]; account != nil {
			result = append(result, account)
		}
	}
	return result, nil
}

func (r *openCodeGoUsageTestRepo) SetOpenCodeGoUsageAutoRefresh(_ context.Context, expected *Account, enabled bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	account := r.accounts[expected.ID]
	if account == nil {
		return ErrAccountNotFound
	}
	if account.Extra == nil {
		account.Extra = make(map[string]any)
	}
	account.Extra[OpenCodeGoUsageAutoRefreshExtraKey] = enabled
	return nil
}

func (r *openCodeGoUsageTestRepo) UpdateOpenCodeGoUsageSnapshot(_ context.Context, expected *Account, snapshot *OpenCodeGoUsageSnapshot) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	account := r.accounts[expected.ID]
	if account == nil {
		return ErrAccountNotFound
	}
	if account.Extra == nil {
		account.Extra = make(map[string]any)
	}
	account.Extra[OpenCodeGoUsageSnapshotExtraKey] = snapshot
	return nil
}

func (r *openCodeGoUsageTestRepo) ListDueOpenCodeGoUsageAccounts(_ context.Context, _ time.Time, limit int) ([]Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.due) > 0 {
		out := make([]Account, 0, min(limit, len(r.due)))
		for _, account := range r.due[:min(limit, len(r.due))] {
			out = append(out, cloneOpenCodeGoUsageTestAccount(account))
		}
		return out, nil
	}
	out := make([]Account, 0, len(r.accounts))
	for _, account := range r.accounts {
		out = append(out, cloneOpenCodeGoUsageTestAccount(*account))
		if len(out) == limit {
			break
		}
	}
	return out, nil
}

func cloneOpenCodeGoUsageTestAccount(account Account) Account {
	account.Credentials = mergeMap(nil, account.Credentials)
	account.Extra = mergeMap(nil, account.Extra)
	return account
}

type openCodeGoUsageHTTPStub struct {
	status      int
	body        []byte
	header      http.Header
	calls       atomic.Int64
	lastRequest *http.Request
	lastProxy   string
	mu          sync.Mutex
}

func (s *openCodeGoUsageHTTPStub) Do(req *http.Request, proxyURL string, _ int64, _ int) (*http.Response, error) {
	s.calls.Add(1)
	s.mu.Lock()
	s.lastRequest = req
	s.lastProxy = proxyURL
	s.mu.Unlock()
	status := s.status
	if status == 0 {
		status = http.StatusOK
	}
	header := s.header
	if header == nil {
		header = http.Header{"Content-Type": []string{"application/json"}}
	}
	return &http.Response{StatusCode: status, Header: header, Body: io.NopCloser(bytes.NewReader(s.body)), Request: req}, nil
}

func (s *openCodeGoUsageHTTPStub) DoWithTLS(req *http.Request, proxyURL string, accountID int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return s.Do(req, proxyURL, accountID, concurrency)
}

func openCodeGoUsageAccount(id int64) *Account {
	return &Account{
		ID: id, Name: fmt.Sprintf("opencode-%d", id), Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"base_url": "https://opencode.ai/zen/go/v1", "api_key": fmt.Sprintf("key-%d", id)},
		Extra:       map[string]any{}, Status: StatusActive, Schedulable: true, Concurrency: 1,
	}
}

func newOpenCodeGoUsageTestService(t *testing.T, repo *openCodeGoUsageTestRepo, upstream HTTPUpstream, settingsRepo SettingRepository) *OpenCodeGoUsageService {
	t.Helper()
	svc := NewOpenCodeGoUsageService(repo, upstream, NewSettingService(settingsRepo, nil))
	t.Cleanup(svc.Stop)
	return svc
}

func TestOpenCodeGoUsageParseJSON200(t *testing.T) {
	data, err := parseOpenCodeGoUsageJSON([]byte(openCodeGoUsageFixture))
	require.NoError(t, err)
	require.Equal(t, "ok", data.Rolling.Status)
	require.Equal(t, 6.0, data.Rolling.Percent)
	require.Equal(t, "ok", data.Weekly.Status)
	require.Equal(t, 2.0, data.Weekly.Percent)
	require.Equal(t, "ok", data.Monthly.Status)
	require.Equal(t, 1.0, data.Monthly.Percent)
	rollingReset, err := time.Parse(time.RFC3339, "2026-08-13T18:26:39.281Z")
	require.NoError(t, err)
	require.Equal(t, rollingReset.UTC(), data.Rolling.ResetsAt)
	weeklyReset, err := time.Parse(time.RFC3339, "2026-08-17T00:00:00.281Z")
	require.NoError(t, err)
	require.Equal(t, weeklyReset.UTC(), data.Weekly.ResetsAt)
}

func TestOpenCodeGoUsageParseJSONLenient(t *testing.T) {
	// top-level windows without a usage wrapper
	data, err := parseOpenCodeGoUsageJSON([]byte(`{"rolling":{"status":"ok","percent":6,"resetsAt":"2026-08-13T18:26:39.281Z"}}`))
	require.NoError(t, err)
	require.Equal(t, 6.0, data.Rolling.Percent)
	require.Zero(t, data.Weekly.Percent)

	// missing windows/fields degrade to zero values instead of failing
	data, err = parseOpenCodeGoUsageJSON([]byte(`{"usage":{"rolling":{"status":"ok"}}}`))
	require.NoError(t, err)
	require.Equal(t, "ok", data.Rolling.Status)
	require.Zero(t, data.Rolling.Percent)
	require.True(t, data.Rolling.ResetsAt.IsZero())

	// integer percent parses as float
	data, err = parseOpenCodeGoUsageJSON([]byte(`{"usage":{"weekly":{"status":"ok","percent":2}}}`))
	require.NoError(t, err)
	require.Equal(t, 2.0, data.Weekly.Percent)

	// malformed JSON is an error
	_, err = parseOpenCodeGoUsageJSON([]byte(`{"usage": {broken`))
	require.Error(t, err)
}

func TestOpenCodeGoUsageRefresh200Success(t *testing.T) {
	account := openCodeGoUsageAccount(7)
	repo := &openCodeGoUsageTestRepo{accounts: map[int64]*Account{7: account}}
	stub := &openCodeGoUsageHTTPStub{body: []byte(openCodeGoUsageFixture)}
	svc := newOpenCodeGoUsageTestService(t, repo, stub, &upstreamBillingProbeSettingRepo{})

	state, err := svc.Refresh(context.Background(), 7)
	require.NoError(t, err)
	require.True(t, state.Eligible)
	require.Equal(t, OpenCodeGoUsageStatusOK, state.Snapshot.Status)
	require.Equal(t, 6.0, state.Snapshot.Data.Rolling.Percent)
	require.Equal(t, 2.0, state.Snapshot.Data.Weekly.Percent)
	require.Equal(t, 1.0, state.Snapshot.Data.Monthly.Percent)
	require.Equal(t, http.StatusOK, state.Snapshot.HTTPStatus)
	require.Equal(t, 0, state.Snapshot.FailureCount)
	require.NotNil(t, state.Snapshot.FetchedAt)
	require.False(t, state.Snapshot.NextRefreshAt.IsZero())
	require.Equal(t, "Bearer key-7", stub.lastRequest.Header.Get("Authorization"))
	require.Equal(t, "https://opencode.ai/zen/go/v1/usage", stub.lastRequest.URL.String())
	require.Equal(t, "application/json", stub.lastRequest.Header.Get("Accept"))
}

func TestOpenCodeGoUsageRefresh401Unauthorized(t *testing.T) {
	account := openCodeGoUsageAccount(7)
	account.Extra[OpenCodeGoUsageAutoRefreshExtraKey] = true
	repo := &openCodeGoUsageTestRepo{accounts: map[int64]*Account{7: account}}
	stub := &openCodeGoUsageHTTPStub{status: http.StatusUnauthorized}
	svc := newOpenCodeGoUsageTestService(t, repo, stub, &upstreamBillingProbeSettingRepo{})

	state, err := svc.Refresh(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, OpenCodeGoUsageStatusUnauthorized, state.Snapshot.Status)
	require.Equal(t, http.StatusUnauthorized, state.Snapshot.HTTPStatus)
	require.Equal(t, "unauthorized", state.Snapshot.LastError)
	require.Equal(t, 1, state.Snapshot.FailureCount)
	require.False(t, state.Snapshot.NextRefreshAt.IsZero())
	// data is kept and auto-refresh stays enabled
	require.True(t, openCodeGoUsageAutoRefreshEnabled(account))
}

func TestOpenCodeGoUsageRefresh403SubscriptionRequired(t *testing.T) {
	account := openCodeGoUsageAccount(7)
	repo := &openCodeGoUsageTestRepo{accounts: map[int64]*Account{7: account}}
	stub := &openCodeGoUsageHTTPStub{status: http.StatusForbidden}
	svc := newOpenCodeGoUsageTestService(t, repo, stub, &upstreamBillingProbeSettingRepo{})

	state, err := svc.Refresh(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, OpenCodeGoUsageStatusFailed, state.Snapshot.Status)
	require.Equal(t, http.StatusForbidden, state.Snapshot.HTTPStatus)
	require.Equal(t, "OpenCode Go subscription required (403)", state.Snapshot.LastError)
	require.Equal(t, 1, state.Snapshot.FailureCount)
}

func TestOpenCodeGoUsageRefreshMalformedJSON(t *testing.T) {
	account := openCodeGoUsageAccount(7)
	repo := &openCodeGoUsageTestRepo{accounts: map[int64]*Account{7: account}}
	stub := &openCodeGoUsageHTTPStub{status: http.StatusOK, body: []byte(`{"usage": {broken`)}
	svc := newOpenCodeGoUsageTestService(t, repo, stub, &upstreamBillingProbeSettingRepo{})

	state, err := svc.Refresh(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, OpenCodeGoUsageStatusFailed, state.Snapshot.Status)
	require.Equal(t, http.StatusOK, state.Snapshot.HTTPStatus)
	require.Equal(t, "invalid_json", state.Snapshot.LastError)
	require.Equal(t, 1, state.Snapshot.FailureCount)
}

func TestOpenCodeGoUsageRefreshFailureKeepsPreviousData(t *testing.T) {
	now := time.Now().UTC()
	account := openCodeGoUsageAccount(7)
	account.Extra[OpenCodeGoUsageSnapshotExtraKey] = &OpenCodeGoUsageSnapshot{
		Status: OpenCodeGoUsageStatusOK, Data: &OpenCodeGoUsageData{Rolling: OpenCodeGoUsageWindow{Status: "ok", Percent: 6}},
		FetchedAt: &now, LastAttemptAt: now.Add(-30 * time.Second), NextRefreshAt: now.Add(time.Hour), FailureCount: 1,
	}
	repo := &openCodeGoUsageTestRepo{accounts: map[int64]*Account{7: account}}
	stub := &openCodeGoUsageHTTPStub{status: http.StatusInternalServerError}
	svc := newOpenCodeGoUsageTestService(t, repo, stub, &upstreamBillingProbeSettingRepo{})

	state, err := svc.Refresh(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, OpenCodeGoUsageStatusFailed, state.Snapshot.Status)
	require.Equal(t, "http_error", state.Snapshot.LastError)
	require.Equal(t, 2, state.Snapshot.FailureCount)
	require.NotNil(t, state.Snapshot.Data, "previous data must be retained on failure")
	require.Equal(t, 6.0, state.Snapshot.Data.Rolling.Percent)
}

func TestNextOpenCodeGoUsageDelayBackoff(t *testing.T) {
	// success: base interval with ±10% jitter
	delay := nextOpenCodeGoUsageDelay(15, 0, 0)
	require.GreaterOrEqual(t, delay, 13*time.Minute)
	require.LessOrEqual(t, delay, 17*time.Minute)

	// failure 1: 1x interval (2^min(0,6))
	delay = nextOpenCodeGoUsageDelay(15, 1, 0)
	require.GreaterOrEqual(t, delay, 13*time.Minute)
	require.LessOrEqual(t, delay, 17*time.Minute)

	// failure 2: 2x interval
	delay = nextOpenCodeGoUsageDelay(15, 2, 0)
	require.GreaterOrEqual(t, delay, 27*time.Minute)
	require.LessOrEqual(t, delay, 33*time.Minute)

	// failure 3: 4x interval
	delay = nextOpenCodeGoUsageDelay(15, 3, 0)
	require.GreaterOrEqual(t, delay, 55*time.Minute)
	require.LessOrEqual(t, delay, 65*time.Minute)

	// failure 8: exponent capped at 6 → 64x interval (16h)
	delay = nextOpenCodeGoUsageDelay(15, 8, 0)
	require.GreaterOrEqual(t, delay, 15*time.Hour)
	require.LessOrEqual(t, delay, 17*time.Hour)

	// hard cap at 24h even for huge intervals
	delay = nextOpenCodeGoUsageDelay(1440, 8, 0)
	require.GreaterOrEqual(t, delay, 23*time.Hour)
	require.LessOrEqual(t, delay, 25*time.Hour)

	// Retry-After hint wins over the computed backoff
	delay = nextOpenCodeGoUsageDelay(15, 0, 2*time.Hour)
	require.GreaterOrEqual(t, delay, 2*time.Hour)

	// floor of one minute
	delay = nextOpenCodeGoUsageDelay(5, 0, 0)
	require.GreaterOrEqual(t, delay, time.Minute)
}

func TestOpenCodeGoUsageManualRefreshThrottle(t *testing.T) {
	now := time.Now().UTC()
	account := openCodeGoUsageAccount(7)
	account.Extra[OpenCodeGoUsageSnapshotExtraKey] = &OpenCodeGoUsageSnapshot{
		Status: OpenCodeGoUsageStatusOK, LastAttemptAt: now.Add(-2 * time.Second),
	}
	repo := &openCodeGoUsageTestRepo{accounts: map[int64]*Account{7: account}}
	stub := &openCodeGoUsageHTTPStub{body: []byte(openCodeGoUsageFixture)}
	svc := newOpenCodeGoUsageTestService(t, repo, stub, &upstreamBillingProbeSettingRepo{})
	svc.now = func() time.Time { return now }

	_, err := svc.Refresh(context.Background(), 7)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrOpenCodeGoUsageRefreshRateLimited))
	require.Equal(t, int64(0), stub.calls.Load(), "throttled refresh must not hit upstream")
	var appErr *infraerrors.ApplicationError
	require.ErrorAs(t, err, &appErr)
	require.NotEmpty(t, appErr.Metadata["retry_after_seconds"])
}

func TestOpenCodeGoUsageManualRefreshNotThrottledAfterWindow(t *testing.T) {
	now := time.Now().UTC()
	account := openCodeGoUsageAccount(7)
	account.Extra[OpenCodeGoUsageSnapshotExtraKey] = &OpenCodeGoUsageSnapshot{
		Status: OpenCodeGoUsageStatusOK, LastAttemptAt: now.Add(-30 * time.Second),
	}
	repo := &openCodeGoUsageTestRepo{accounts: map[int64]*Account{7: account}}
	stub := &openCodeGoUsageHTTPStub{body: []byte(openCodeGoUsageFixture)}
	svc := newOpenCodeGoUsageTestService(t, repo, stub, &upstreamBillingProbeSettingRepo{})
	svc.now = func() time.Time { return now }

	state, err := svc.Refresh(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, OpenCodeGoUsageStatusOK, state.Snapshot.Status)
	require.Equal(t, int64(1), stub.calls.Load())
}

func TestOpenCodeGoUsageIsAutoRefreshDue(t *testing.T) {
	now := time.Now().UTC()
	require.True(t, openCodeGoUsageIsAutoRefreshDue(nil, now))
	require.True(t, openCodeGoUsageIsAutoRefreshDue(&OpenCodeGoUsageSnapshot{}, now))
	require.True(t, openCodeGoUsageIsAutoRefreshDue(&OpenCodeGoUsageSnapshot{NextRefreshAt: now.Add(-time.Minute)}, now))
	require.False(t, openCodeGoUsageIsAutoRefreshDue(&OpenCodeGoUsageSnapshot{NextRefreshAt: now.Add(time.Minute)}, now))
}

func TestOpenCodeGoUsageRunDueRefreshesDueAccounts(t *testing.T) {
	account := openCodeGoUsageAccount(7)
	account.Extra[OpenCodeGoUsageAutoRefreshExtraKey] = true
	repo := &openCodeGoUsageTestRepo{accounts: map[int64]*Account{7: account}}
	stub := &openCodeGoUsageHTTPStub{body: []byte(openCodeGoUsageFixture)}
	settingsRepo := &upstreamBillingProbeSettingRepo{}
	require.NoError(t, settingsRepo.Set(context.Background(), SettingKeyOpenCodeGoUsageSettings, `{"enabled":true,"interval_minutes":15}`))
	svc := newOpenCodeGoUsageTestService(t, repo, stub, settingsRepo)

	require.NoError(t, svc.RunDue(context.Background()))
	require.Equal(t, int64(1), stub.calls.Load())
	require.Equal(t, OpenCodeGoUsageStatusOK, decodeOpenCodeGoUsageSnapshot(account.Extra).Status)
}

func TestOpenCodeGoUsageRunDueSkipsWhenDisabled(t *testing.T) {
	account := openCodeGoUsageAccount(7)
	account.Extra[OpenCodeGoUsageAutoRefreshExtraKey] = true
	repo := &openCodeGoUsageTestRepo{accounts: map[int64]*Account{7: account}}
	stub := &openCodeGoUsageHTTPStub{body: []byte(openCodeGoUsageFixture)}
	svc := newOpenCodeGoUsageTestService(t, repo, stub, &upstreamBillingProbeSettingRepo{})

	require.NoError(t, svc.RunDue(context.Background()))
	require.Equal(t, int64(0), stub.calls.Load())
}

func TestIsOpenCodeGoUsageAccount(t *testing.T) {
	base := func() *Account {
		return &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": "https://opencode.ai/zen/go/v1", "api_key": "k"}}
	}
	require.True(t, IsOpenCodeGoUsageAccount(base()))

	// trailing slash on the path is allowed
	account := base()
	account.Credentials["base_url"] = "https://opencode.ai/zen/go/v1/"
	require.True(t, IsOpenCodeGoUsageAccount(account))

	// scheme/host/path are case-insensitive
	account = base()
	account.Credentials["base_url"] = "HTTPS://OPENCODE.AI/ZEN/GO/V1"
	require.True(t, IsOpenCodeGoUsageAccount(account))

	// wrong path
	account = base()
	account.Credentials["base_url"] = "https://opencode.ai/v1"
	require.False(t, IsOpenCodeGoUsageAccount(account))

	// wrong host
	account = base()
	account.Credentials["base_url"] = "https://ollama.com/zen/go/v1"
	require.False(t, IsOpenCodeGoUsageAccount(account))

	// wrong platform
	account = base()
	account.Platform = PlatformAnthropic
	require.False(t, IsOpenCodeGoUsageAccount(account))

	// non-apikey type
	account = base()
	account.Type = "oauth"
	require.False(t, IsOpenCodeGoUsageAccount(account))

	// query string rejected
	account = base()
	account.Credentials["base_url"] = "https://opencode.ai/zen/go/v1?x=1"
	require.False(t, IsOpenCodeGoUsageAccount(account))

	// missing base_url
	account = base()
	account.Credentials = map[string]any{"api_key": "k"}
	require.False(t, IsOpenCodeGoUsageAccount(account))
}

func TestOpenCodeGoUsageSettingsDefaultOffAndValidation(t *testing.T) {
	repo := &upstreamBillingProbeSettingRepo{}
	settingsService := NewSettingService(repo, nil)
	settings, err := settingsService.GetOpenCodeGoUsageSettings(context.Background())
	require.NoError(t, err)
	require.False(t, settings.Enabled)
	require.Equal(t, 15, settings.IntervalMinutes)

	// below the minimum
	err = settingsService.SetOpenCodeGoUsageSettings(context.Background(), &OpenCodeGoUsageSettings{Enabled: true, IntervalMinutes: 1})
	require.Error(t, err)
	// above the maximum
	err = settingsService.SetOpenCodeGoUsageSettings(context.Background(), &OpenCodeGoUsageSettings{Enabled: true, IntervalMinutes: 2000})
	require.Error(t, err)
	// valid update round-trips
	err = settingsService.SetOpenCodeGoUsageSettings(context.Background(), &OpenCodeGoUsageSettings{Enabled: true, IntervalMinutes: 30})
	require.NoError(t, err)
	settings, err = settingsService.GetOpenCodeGoUsageSettings(context.Background())
	require.NoError(t, err)
	require.True(t, settings.Enabled)
	require.Equal(t, 30, settings.IntervalMinutes)
}

func TestOpenCodeGoUsageStateFromAccount(t *testing.T) {
	account := openCodeGoUsageAccount(7)
	state := OpenCodeGoUsageStateFromAccount(account)
	require.True(t, state.Eligible)
	require.False(t, state.AutoRefreshEnabled)
	require.Nil(t, state.Snapshot)

	account.Extra[OpenCodeGoUsageAutoRefreshExtraKey] = true
	account.Extra[OpenCodeGoUsageSnapshotExtraKey] = &OpenCodeGoUsageSnapshot{Status: OpenCodeGoUsageStatusOK}
	state = OpenCodeGoUsageStateFromAccount(account)
	require.True(t, state.AutoRefreshEnabled)
	require.NotNil(t, state.Snapshot)
	require.Equal(t, OpenCodeGoUsageStatusOK, state.Snapshot.Status)

	// ineligible account exposes no managed state
	account.Platform = PlatformAnthropic
	state = OpenCodeGoUsageStateFromAccount(account)
	require.False(t, state.Eligible)
	require.Nil(t, state.Snapshot)
}

func TestOpenCodeGoUsageRefreshRejectsIneligibleAccount(t *testing.T) {
	account := openCodeGoUsageAccount(7)
	account.Credentials["base_url"] = "https://opencode.ai/v1"
	repo := &openCodeGoUsageTestRepo{accounts: map[int64]*Account{7: account}}
	stub := &openCodeGoUsageHTTPStub{body: []byte(openCodeGoUsageFixture)}
	svc := newOpenCodeGoUsageTestService(t, repo, stub, &upstreamBillingProbeSettingRepo{})

	_, err := svc.Refresh(context.Background(), 7)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrOpenCodeGoUsageAccountInvalid))
	require.Equal(t, int64(0), stub.calls.Load())
}
