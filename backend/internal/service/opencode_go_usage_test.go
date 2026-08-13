package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
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
	mu                 sync.Mutex
	accounts           map[int64]*Account
	due                []Account
	groupResolveCalls  atomic.Int64
	getByIDCalls       atomic.Int64
	disableAutoCalls   atomic.Int64
	disableAutoAttempt atomic.Int64
}

func (r *openCodeGoUsageTestRepo) GetByID(_ context.Context, id int64) (*Account, error) {
	r.getByIDCalls.Add(1)
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

func (r *openCodeGoUsageTestRepo) ListOpenCodeGoUsageGroupAccounts(_ context.Context, anchors []*Account) ([]Account, error) {
	r.groupResolveCalls.Add(1)
	r.mu.Lock()
	defer r.mu.Unlock()
	wanted := make(map[string]struct{}, len(anchors))
	for _, anchor := range anchors {
		if fingerprint, ok := openCodeGoUsageGroupFingerprint(anchor); ok {
			wanted[fingerprint] = struct{}{}
		}
	}
	result := make([]Account, 0, len(r.accounts))
	for _, account := range r.accounts {
		fingerprint, ok := openCodeGoUsageGroupFingerprint(account)
		if _, match := wanted[fingerprint]; !ok || !match {
			continue
		}
		result = append(result, cloneOpenCodeGoUsageTestAccount(*account))
	}
	return result, nil
}

func (r *openCodeGoUsageTestRepo) SetOpenCodeGoUsageAutoRefresh(_ context.Context, expected *Account, enabled bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	members, err := r.openCodeGoGroupMembersLocked(expected)
	if err != nil {
		return err
	}
	for _, account := range members {
		applyOpenCodeGoUsageTestManagedExtra(account, expected)
		account.Extra[OpenCodeGoUsageAutoRefreshExtraKey] = enabled
	}
	return nil
}

func (r *openCodeGoUsageTestRepo) UpdateOpenCodeGoUsageSnapshot(_ context.Context, expected *Account, snapshot *OpenCodeGoUsageSnapshot) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	members, err := r.openCodeGoGroupMembersLocked(expected)
	if err != nil {
		return err
	}
	for _, account := range members {
		applyOpenCodeGoUsageTestManagedExtra(account, expected)
		account.Extra[OpenCodeGoUsageSnapshotExtraKey] = snapshot
	}
	return nil
}

func (r *openCodeGoUsageTestRepo) DisableOpenCodeGoUsageAutoRefresh(_ context.Context, expected *Account) error {
	r.disableAutoAttempt.Add(1)
	r.mu.Lock()
	defer r.mu.Unlock()
	members, err := r.openCodeGoGroupMembersLocked(expected)
	if err != nil {
		return err
	}
	for _, account := range members {
		applyOpenCodeGoUsageTestManagedExtra(account, expected)
		account.Extra[OpenCodeGoUsageAutoRefreshExtraKey] = false
		delete(account.Extra, OpenCodeGoUsageSnapshotExtraKey)
	}
	r.disableAutoCalls.Add(1)
	return nil
}

func (r *openCodeGoUsageTestRepo) openCodeGoGroupMembersLocked(expected *Account) ([]*Account, error) {
	anchor := r.accounts[expected.ID]
	if !sameOpenCodeGoUsageTestIdentity(anchor, expected) {
		return nil, ErrOpenCodeGoUsageIdentityChanged
	}
	fingerprint, ok := openCodeGoUsageGroupFingerprint(expected)
	if !ok {
		return nil, ErrOpenCodeGoUsageAccountInvalid
	}
	members := make([]*Account, 0, len(r.accounts))
	for _, account := range r.accounts {
		candidate, valid := openCodeGoUsageGroupFingerprint(account)
		if valid && candidate == fingerprint {
			if account.Extra == nil {
				account.Extra = make(map[string]any)
			}
			members = append(members, account)
		}
	}
	return members, nil
}

func applyOpenCodeGoUsageTestManagedExtra(account, source *Account) {
	for _, key := range []string{OpenCodeGoUsageAutoRefreshExtraKey, OpenCodeGoUsageSnapshotExtraKey} {
		delete(account.Extra, key)
		if value, ok := source.Extra[key]; ok {
			account.Extra[key] = value
		}
	}
}

func sameOpenCodeGoUsageTestIdentity(left, right *Account) bool {
	return left != nil && right != nil && left.Platform == right.Platform && left.Type == right.Type &&
		reflect.DeepEqual(left.Credentials, right.Credentials) && reflect.DeepEqual(left.ProxyID, right.ProxyID)
}

func (r *openCodeGoUsageTestRepo) ListDueOpenCodeGoUsageAccounts(_ context.Context, _ time.Time, _, _ time.Duration, limit int) ([]Account, error) {
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
	status         int
	body           []byte
	header         http.Header
	calls          atomic.Int64
	beforeResponse func(*http.Request)
	lastRequest    *http.Request
	lastProxy      string
	mu             sync.Mutex
}

func (s *openCodeGoUsageHTTPStub) Do(req *http.Request, proxyURL string, _ int64, _ int) (*http.Response, error) {
	s.calls.Add(1)
	s.mu.Lock()
	s.lastRequest = req
	s.lastProxy = proxyURL
	s.mu.Unlock()
	if s.beforeResponse != nil {
		s.beforeResponse(req)
	}
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
	debounce := time.Minute
	maxWait := time.Hour
	now := time.Date(2026, time.July, 25, 12, 0, 0, 0, time.UTC)
	fetched := now.Add(-30 * time.Minute)
	ptr := func(ts time.Time) *time.Time { return &ts }

	require.True(t, openCodeGoUsageIsAutoRefreshDue(nil, nil, now, debounce, maxWait), "missing snapshot first due")
	require.True(t, openCodeGoUsageIsAutoRefreshDue(&OpenCodeGoUsageSnapshot{Status: "bogus"}, nil, now, debounce, maxWait), "invalid status first due")

	okSnap := &OpenCodeGoUsageSnapshot{
		Status: OpenCodeGoUsageStatusOK, FetchedAt: ptr(fetched),
		LastAttemptAt: fetched, NextRefreshAt: fetched.Add(maxWait),
	}
	require.False(t, openCodeGoUsageIsAutoRefreshDue(okSnap, nil, now, debounce, maxWait), "no request after success")
	require.False(t, openCodeGoUsageIsAutoRefreshDue(okSnap, ptr(fetched), now, debounce, maxWait), "request not after fetched_at")
	require.False(t, openCodeGoUsageIsAutoRefreshDue(okSnap, ptr(now.Add(-30*time.Second)), now, debounce, maxWait), "debounce not elapsed")
	require.True(t, openCodeGoUsageIsAutoRefreshDue(okSnap, ptr(now.Add(-time.Minute)), now, debounce, maxWait), "single request quiet for debounce")

	// Continuous requests: last used is now, but max-wait from old fetch forces due.
	oldFetched := now.Add(-2 * time.Hour)
	oldSnap := &OpenCodeGoUsageSnapshot{
		Status: OpenCodeGoUsageStatusOK, FetchedAt: ptr(oldFetched),
		LastAttemptAt: oldFetched, NextRefreshAt: oldFetched.Add(maxWait),
	}
	require.True(t, openCodeGoUsageIsAutoRefreshDue(oldSnap, ptr(now), now, debounce, maxWait), "max-wait forces due while requests continue")
	// First request after a very old snapshot is immediately due because fetched+maxWait is past.
	require.True(t, openCodeGoUsageIsAutoRefreshDue(oldSnap, ptr(now.Add(-time.Second)), now, debounce, maxWait), "stale snapshot first request immediate")

	failSnap := &OpenCodeGoUsageSnapshot{
		Status: OpenCodeGoUsageStatusFailed, FetchedAt: ptr(fetched),
		LastAttemptAt: now.Add(-10 * time.Minute), NextRefreshAt: now.Add(20 * time.Minute),
	}
	require.False(t, openCodeGoUsageIsAutoRefreshDue(failSnap, nil, now, debounce, maxWait), "failure without new request")
	require.False(t, openCodeGoUsageIsAutoRefreshDue(failSnap, ptr(now.Add(-time.Minute)), now, debounce, maxWait), "failure blocked by backoff")
	failSnap.NextRefreshAt = now.Add(-time.Second)
	require.True(t, openCodeGoUsageIsAutoRefreshDue(failSnap, ptr(now.Add(-time.Minute)), now, debounce, maxWait), "failure after backoff with new request")

	require.True(t, openCodeGoUsageIsAutoRefreshDue(&OpenCodeGoUsageSnapshot{
		Status: OpenCodeGoUsageStatusOK, LastAttemptAt: now,
	}, nil, now, debounce, maxWait), "ok without fetched_at fails open")
}

// The success path stopped consulting next_refresh_at, which is where
// nextOpenCodeGoUsageDelay used to apply the minimum interval. Activity may pull
// a refresh forward only as far as that floor, otherwise request traffic spaced
// just wider than the debounce drives the group's outbound rate far above the
// pre-existing minimum.
func TestOpenCodeGoUsageAutoRefreshDueAtHonoursMinFetchInterval(t *testing.T) {
	debounce := time.Minute
	maxWait := time.Hour
	now := time.Date(2026, time.July, 25, 12, 0, 0, 0, time.UTC)
	ptr := func(ts time.Time) *time.Time { return &ts }

	// Debounce elapsed, but the last successful fetch is inside the floor.
	recent := now.Add(-2 * time.Minute)
	recentSnap := &OpenCodeGoUsageSnapshot{
		Status: OpenCodeGoUsageStatusOK, FetchedAt: ptr(recent), LastAttemptAt: recent,
	}
	dueAt, ok := openCodeGoUsageAutoRefreshDueAt(recentSnap, ptr(now.Add(-time.Minute)), debounce, maxWait)
	require.True(t, ok)
	require.Equal(t, recent.Add(OpenCodeGoUsageMinFetchInterval), dueAt,
		"due time must be clamped to fetched_at + min fetch interval")
	require.False(t, openCodeGoUsageIsAutoRefreshDue(recentSnap, ptr(now.Add(-time.Minute)), now, debounce, maxWait),
		"debounce alone must not refresh within the min fetch interval")

	// Once the floor has passed the debounce governs again.
	atFloor := now.Add(-OpenCodeGoUsageMinFetchInterval)
	floorSnap := &OpenCodeGoUsageSnapshot{
		Status: OpenCodeGoUsageStatusOK, FetchedAt: ptr(atFloor), LastAttemptAt: atFloor,
	}
	require.True(t, openCodeGoUsageIsAutoRefreshDue(floorSnap, ptr(now.Add(-2*time.Minute)), now, debounce, maxWait),
		"past the floor a quiet debounce window is due")

	// The floor never delays a refresh that max-wait has already forced.
	oldFetched := now.Add(-2 * time.Hour)
	oldSnap := &OpenCodeGoUsageSnapshot{
		Status: OpenCodeGoUsageStatusOK, FetchedAt: ptr(oldFetched), LastAttemptAt: oldFetched,
	}
	dueAt, ok = openCodeGoUsageAutoRefreshDueAt(oldSnap, ptr(now), debounce, maxWait)
	require.True(t, ok)
	require.Equal(t, oldFetched.Add(maxWait), dueAt, "max-wait due time must not be pushed out by the floor")
}

func TestOpenCodeGoUsageGroupSharesStateAcrossSiblings(t *testing.T) {
	source := openCodeGoUsageAccount(71)
	source.Credentials["api_key"] = "shared-key"
	source.Extra[OpenCodeGoUsageAutoRefreshExtraKey] = true
	source.Extra[OpenCodeGoUsageSnapshotExtraKey] = &OpenCodeGoUsageSnapshot{
		Status: OpenCodeGoUsageStatusOK,
		Data:   &OpenCodeGoUsageData{Rolling: OpenCodeGoUsageWindow{Status: "ok", Percent: 6}},
	}
	source.UpdatedAt = time.Now().Add(-time.Minute)
	sibling := openCodeGoUsageAccount(72)
	sibling.Credentials = map[string]any{"base_url": "HTTPS://OPENCODE.AI/ZEN/GO/V1/", "api_key": "shared-key"}
	sibling.Extra[OpenCodeGoUsageAutoRefreshExtraKey] = true
	sibling.UpdatedAt = time.Now()
	different := openCodeGoUsageAccount(73)
	different.Credentials["api_key"] = "different-key"
	repo := &openCodeGoUsageTestRepo{accounts: map[int64]*Account{
		source.ID: source, sibling.ID: sibling, different.ID: different,
	}}
	svc := newOpenCodeGoUsageTestService(t, repo, &openCodeGoUsageHTTPStub{}, &upstreamBillingProbeSettingRepo{})

	state, err := svc.GetState(context.Background(), sibling.ID)
	require.NoError(t, err)
	require.True(t, state.Eligible)
	require.True(t, state.AutoRefreshEnabled)
	require.Equal(t, 6.0, state.Snapshot.Data.Rolling.Percent)

	differentState, err := svc.GetState(context.Background(), different.ID)
	require.NoError(t, err)
	require.False(t, differentState.AutoRefreshEnabled)
	require.Nil(t, differentState.Snapshot)

	newSibling := openCodeGoUsageAccount(74)
	newSibling.Credentials = map[string]any{"base_url": "https://opencode.ai/zen/go/v1", "api_key": "shared-key"}
	repo.mu.Lock()
	repo.accounts[newSibling.ID] = newSibling
	repo.mu.Unlock()
	newState, err := svc.GetState(context.Background(), newSibling.ID)
	require.NoError(t, err)
	require.True(t, newState.AutoRefreshEnabled)
	require.Equal(t, state.Snapshot, newState.Snapshot)

	before := repo.groupResolveCalls.Load()
	require.NoError(t, svc.ResolveOpenCodeGoUsageAccounts(context.Background(), []*Account{source, sibling, different, newSibling}))
	require.Equal(t, before+1, repo.groupResolveCalls.Load(), "one list batch must issue one group lookup")
}

func TestOpenCodeGoUsageSetAutoRefreshAndSnapshotAreGroupScoped(t *testing.T) {
	first := openCodeGoUsageAccount(81)
	first.Credentials["api_key"] = "shared-key"
	second := openCodeGoUsageAccount(82)
	second.Credentials = map[string]any{"base_url": "https://opencode.ai/zen/go/v1/", "api_key": "shared-key"}
	different := openCodeGoUsageAccount(83)
	different.Credentials["api_key"] = "different-key"
	repo := &openCodeGoUsageTestRepo{accounts: map[int64]*Account{
		first.ID: first, second.ID: second, different.ID: different,
	}}
	svc := newOpenCodeGoUsageTestService(t, repo, &openCodeGoUsageHTTPStub{}, &upstreamBillingProbeSettingRepo{})

	state, err := svc.SetAutoRefresh(context.Background(), second.ID, true)
	require.NoError(t, err)
	require.True(t, state.AutoRefreshEnabled)
	require.Equal(t, true, first.Extra[OpenCodeGoUsageAutoRefreshExtraKey])
	require.Equal(t, true, second.Extra[OpenCodeGoUsageAutoRefreshExtraKey])
	require.NotContains(t, different.Extra, OpenCodeGoUsageAutoRefreshExtraKey)

	// A snapshot write must not wipe the auto-refresh switch (pure merge).
	now := time.Now().UTC()
	_, err = svc.Refresh(context.Background(), first.ID)
	require.NoError(t, err)
	require.Equal(t, true, first.Extra[OpenCodeGoUsageAutoRefreshExtraKey], "snapshot write must preserve auto_refresh")
	require.Equal(t, true, second.Extra[OpenCodeGoUsageAutoRefreshExtraKey], "snapshot write must preserve auto_refresh on siblings")
	require.NotNil(t, decodeOpenCodeGoUsageSnapshot(second.Extra), "snapshot must be shared with siblings")
	require.Equal(t, decodeOpenCodeGoUsageSnapshot(first.Extra), decodeOpenCodeGoUsageSnapshot(second.Extra))
	require.NotNil(t, now)
}

func TestOpenCodeGoUsageRefreshSingleflightAndRunnerDeduplicateSharedGroup(t *testing.T) {
	first := openCodeGoUsageAccount(91)
	first.Credentials["api_key"] = "shared-key"
	first.Extra[OpenCodeGoUsageAutoRefreshExtraKey] = true
	second := openCodeGoUsageAccount(92)
	second.Credentials = map[string]any{"base_url": "https://opencode.ai/zen/go/v1/", "api_key": "shared-key"}
	second.Extra[OpenCodeGoUsageAutoRefreshExtraKey] = true
	repo := &openCodeGoUsageTestRepo{
		accounts: map[int64]*Account{first.ID: first, second.ID: second},
		due:      []Account{*first, *second},
	}
	settingsRepo := &upstreamBillingProbeSettingRepo{values: map[string]string{
		SettingKeyOpenCodeGoUsageSettings: `{"enabled":true,"interval_minutes":15}`,
	}}
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	upstream := &openCodeGoUsageHTTPStub{body: []byte(openCodeGoUsageFixture), beforeResponse: func(*http.Request) {
		once.Do(func() { close(started) })
		<-release
	}}
	svc := newOpenCodeGoUsageTestService(t, repo, upstream, settingsRepo)

	errs := make(chan error, 2)
	go func() { _, err := svc.Refresh(context.Background(), first.ID); errs <- err }()
	<-started
	// The first caller is now parked in the stub, having loaded the account twice
	// (once to build the group key, once inside the singleflight function).
	loadsBeforeSecond := repo.getByIDCalls.Load()
	go func() { _, err := svc.Refresh(context.Background(), second.ID); errs <- err }()
	// Only release the first caller once the second one has loaded its own
	// account, which happens immediately before it joins the singleflight group.
	require.Eventually(t, func() bool {
		return repo.getByIDCalls.Load() > loadsBeforeSecond
	}, 5*time.Second, time.Millisecond, "the second caller must reach the singleflight group before the first is released")
	close(release)
	require.NoError(t, <-errs)
	require.NoError(t, <-errs)
	require.Equal(t, int64(1), upstream.calls.Load())
	require.NotNil(t, decodeOpenCodeGoUsageSnapshot(first.Extra))
	require.Equal(t, decodeOpenCodeGoUsageSnapshot(first.Extra), decodeOpenCodeGoUsageSnapshot(second.Extra))

	delete(first.Extra, OpenCodeGoUsageSnapshotExtraKey)
	delete(second.Extra, OpenCodeGoUsageSnapshotExtraKey)
	upstream.beforeResponse = nil
	require.NoError(t, svc.RunDue(context.Background()))
	require.Equal(t, int64(2), upstream.calls.Load(), "RunDue must issue one request for the shared group")
}

func TestOpenCodeGoUsageRefreshRejectsGroupChangeBeforeUpstreamRequest(t *testing.T) {
	account := openCodeGoUsageAccount(94)
	base := &openCodeGoUsageTestRepo{accounts: map[int64]*Account{account.ID: account}}
	repo := &openCodeGoRefreshPreflightIdentityChangeRepo{openCodeGoUsageTestRepo: base}
	upstream := &openCodeGoUsageHTTPStub{body: []byte(openCodeGoUsageFixture)}
	svc := NewOpenCodeGoUsageService(repo, upstream, NewSettingService(&upstreamBillingProbeSettingRepo{}, nil))
	t.Cleanup(svc.Stop)

	_, err := svc.Refresh(context.Background(), account.ID)

	require.ErrorIs(t, err, ErrOpenCodeGoUsageIdentityChanged)
	require.Zero(t, upstream.calls.Load())
	require.NotContains(t, account.Extra, OpenCodeGoUsageSnapshotExtraKey)
}

type openCodeGoRefreshPreflightIdentityChangeRepo struct {
	*openCodeGoUsageTestRepo
	getCalls atomic.Int64
}

func (r *openCodeGoRefreshPreflightIdentityChangeRepo) GetByID(ctx context.Context, id int64) (*Account, error) {
	if r.getCalls.Add(1) == 2 {
		r.mu.Lock()
		r.accounts[id].Credentials["api_key"] = "rotated-before-refresh"
		r.mu.Unlock()
	}
	return r.openCodeGoUsageTestRepo.GetByID(ctx, id)
}

func TestOpenCodeGoUsageRunnerDisablesAutoRefreshAfterIdentityError(t *testing.T) {
	account := openCodeGoUsageAccount(14)
	account.Extra[OpenCodeGoUsageAutoRefreshExtraKey] = true
	missingProxyID := int64(99)
	account.ProxyID = &missingProxyID
	account.Proxy = nil
	repo := &openCodeGoUsageTestRepo{accounts: map[int64]*Account{14: account}}
	settingsRepo := &upstreamBillingProbeSettingRepo{values: map[string]string{
		SettingKeyOpenCodeGoUsageSettings: `{"enabled":true,"interval_minutes":15}`,
	}}
	upstream := &openCodeGoUsageHTTPStub{body: []byte(openCodeGoUsageFixture)}
	svc := newOpenCodeGoUsageTestService(t, repo, upstream, settingsRepo)

	require.NoError(t, svc.RunDue(context.Background()))
	require.Equal(t, int64(1), repo.disableAutoCalls.Load())
	require.Equal(t, false, account.Extra[OpenCodeGoUsageAutoRefreshExtraKey])
	require.Zero(t, upstream.calls.Load())

	require.NoError(t, svc.RunDue(context.Background()))
	require.Equal(t, int64(1), repo.disableAutoCalls.Load())
	require.Zero(t, upstream.calls.Load())
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

func TestOpenCodeGoUsageGroupFingerprint(t *testing.T) {
	first := openCodeGoUsageAccount(1)
	first.Credentials["api_key"] = "shared-key"
	second := openCodeGoUsageAccount(2)
	second.Credentials = map[string]any{"base_url": "HTTPS://OPENCODE.AI/ZEN/GO/V1/", "api_key": "shared-key"}
	third := openCodeGoUsageAccount(3)
	third.Credentials["api_key"] = "other-key"

	firstFP, firstOK := openCodeGoUsageGroupFingerprint(first)
	secondFP, secondOK := openCodeGoUsageGroupFingerprint(second)
	thirdFP, thirdOK := openCodeGoUsageGroupFingerprint(third)
	require.True(t, firstOK)
	require.True(t, secondOK)
	require.True(t, thirdOK)
	require.Equal(t, firstFP, secondFP, "same api_key across base_url variants must share a group")
	require.NotEqual(t, firstFP, thirdFP, "different api_key must not share a group")

	// ineligible accounts have no fingerprint
	account := openCodeGoUsageAccount(4)
	account.Credentials["base_url"] = "https://opencode.ai/v1"
	_, ok := openCodeGoUsageGroupFingerprint(account)
	require.False(t, ok)
}

func TestScheduleOpenCodeGoUsageActivityOnlyForOpenCode(t *testing.T) {
	deferred := &DeferredService{}
	openCode := openCodeGoUsageAccount(1)
	openCode.Credentials["api_key"] = "k"
	other := openCodeGoUsageAccount(2)
	other.Credentials["base_url"] = "https://opencode.ai/v1"

	scheduleOpenCodeGoUsageActivity(deferred, openCode)
	scheduleOpenCodeGoUsageActivity(deferred, other)
	scheduleOpenCodeGoUsageActivity(nil, openCode)
	scheduleOpenCodeGoUsageActivity(deferred, nil)
}

func TestOpenCodeGoUsageSettingsDefaultOffAndValidation(t *testing.T) {
	repo := &upstreamBillingProbeSettingRepo{}
	settingsService := NewSettingService(repo, nil)
	settings, err := settingsService.GetOpenCodeGoUsageSettings(context.Background())
	require.NoError(t, err)
	require.False(t, settings.Enabled)
	require.Equal(t, 15, settings.IntervalMinutes)
	require.Equal(t, 1, settings.DebounceMinutes)

	// below the minimum
	err = settingsService.SetOpenCodeGoUsageSettings(context.Background(), &OpenCodeGoUsageSettings{Enabled: true, IntervalMinutes: 1})
	require.Error(t, err)
	// above the maximum
	err = settingsService.SetOpenCodeGoUsageSettings(context.Background(), &OpenCodeGoUsageSettings{Enabled: true, IntervalMinutes: 2000})
	require.Error(t, err)
	// debounce out of range
	err = settingsService.SetOpenCodeGoUsageSettings(context.Background(), &OpenCodeGoUsageSettings{Enabled: true, IntervalMinutes: 30, DebounceMinutes: 61})
	require.Error(t, err)
	// DebounceMinutes=0 (legacy omit) defaults to 1 on write.
	err = settingsService.SetOpenCodeGoUsageSettings(context.Background(), &OpenCodeGoUsageSettings{Enabled: true, IntervalMinutes: 30, DebounceMinutes: 0})
	require.NoError(t, err)
	settings, err = settingsService.GetOpenCodeGoUsageSettings(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, settings.DebounceMinutes)
	// valid update round-trips
	err = settingsService.SetOpenCodeGoUsageSettings(context.Background(), &OpenCodeGoUsageSettings{Enabled: true, IntervalMinutes: 30, DebounceMinutes: 2})
	require.NoError(t, err)
	settings, err = settingsService.GetOpenCodeGoUsageSettings(context.Background())
	require.NoError(t, err)
	require.True(t, settings.Enabled)
	require.Equal(t, 30, settings.IntervalMinutes)
	require.Equal(t, 2, settings.DebounceMinutes)

	// debounce >= interval would make the debounce term unreachable in
	// min(lastUsed+debounce, fetchedAt+maxWait), silently ignoring the operator's
	// setting, so it is rejected rather than accepted and dropped.
	err = settingsService.SetOpenCodeGoUsageSettings(context.Background(), &OpenCodeGoUsageSettings{Enabled: true, IntervalMinutes: 5, DebounceMinutes: 5})
	require.Error(t, err, "debounce equal to interval must be rejected")
	err = settingsService.SetOpenCodeGoUsageSettings(context.Background(), &OpenCodeGoUsageSettings{Enabled: true, IntervalMinutes: 5, DebounceMinutes: 60})
	require.Error(t, err, "debounce greater than interval must be rejected")
	err = settingsService.SetOpenCodeGoUsageSettings(context.Background(), &OpenCodeGoUsageSettings{Enabled: true, IntervalMinutes: 6, DebounceMinutes: 5})
	require.NoError(t, err, "debounce below interval stays valid")

	// Legacy JSON without debounce_minutes defaults to 1.
	repo.values[SettingKeyOpenCodeGoUsageSettings] = `{"enabled":true,"interval_minutes":45}`
	settings, err = settingsService.GetOpenCodeGoUsageSettings(context.Background())
	require.NoError(t, err)
	require.Equal(t, 45, settings.IntervalMinutes)
	require.Equal(t, 1, settings.DebounceMinutes)
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
