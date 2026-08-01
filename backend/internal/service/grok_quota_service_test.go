//go:build unit

package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type grokQuotaAccountRepo struct {
	*mockAccountRepoForPlatform
	updates               map[int64]map[string]any
	updateCalls           int
	rateLimitedCalls      int
	lastRateLimitedID     int64
	lastRateLimitResetAt  time.Time
	tempUnschedCalls      int
	lastTempUnschedID     int64
	lastTempUnschedUntil  time.Time
	lastTempUnschedReason string
	recoveryClearCalls    int
	recoveryObservedAt    time.Time
	recoveryObservedReset time.Time
	recoveryClearResult   bool
}

func (r *grokQuotaAccountRepo) UpdateExtra(_ context.Context, id int64, updates map[string]any) error {
	r.updateCalls++
	if r.updates == nil {
		r.updates = make(map[int64]map[string]any)
	}
	r.updates[id] = updates
	if r.mockAccountRepoForPlatform != nil {
		account := r.accountsByID[id]
		if account == nil {
			return nil
		}
		if account.Extra == nil {
			account.Extra = make(map[string]any)
		}
		for key, value := range updates {
			account.Extra[key] = value
		}
	}
	return nil
}

func (r *grokQuotaAccountRepo) SetRateLimited(_ context.Context, id int64, resetAt time.Time) error {
	r.rateLimitedCalls++
	r.lastRateLimitedID = id
	r.lastRateLimitResetAt = resetAt
	return nil
}

func (r *grokQuotaAccountRepo) SetRateLimitedIfLater(ctx context.Context, id int64, resetAt time.Time) error {
	return r.SetRateLimited(ctx, id, resetAt)
}

func (r *grokQuotaAccountRepo) ClearRateLimitIfObserved(_ context.Context, _ int64, observedLimitedAt, observedResetAt time.Time) (bool, error) {
	r.recoveryClearCalls++
	r.recoveryObservedAt = observedLimitedAt
	r.recoveryObservedReset = observedResetAt
	return r.recoveryClearResult, nil
}

func (r *grokQuotaAccountRepo) SetTempUnschedulable(_ context.Context, id int64, until time.Time, reason string) error {
	r.tempUnschedCalls++
	r.lastTempUnschedID = id
	r.lastTempUnschedUntil = until
	r.lastTempUnschedReason = reason
	return nil
}

type grokQuotaProxyRepo struct {
	proxyRepoStub
	proxies map[int64]*Proxy
	calls   int
}

type grokQuotaUsageLogRepo struct {
	UsageLogRepository
	stats      *usagestats.AccountStats
	err        error
	calls      int
	startTimes []time.Time
}

func (r *grokQuotaUsageLogRepo) GetAccountWindowStats(_ context.Context, _ int64, start time.Time) (*usagestats.AccountStats, error) {
	r.calls++
	r.startTimes = append(r.startTimes, start)
	return r.stats, r.err
}

func (r *grokQuotaUsageLogRepo) GetAccountTodayStats(context.Context, int64) (*usagestats.AccountStats, error) {
	return nil, nil
}

type grokHybridUpstream struct {
	httpUpstreamRecorder
	mu                   sync.Mutex
	requests             []*http.Request
	bodies               [][]byte
	weeklyUsagePercent   *float64
	monthlyLimitCents    *float64
	activeStatus         int
	activeHeaders        http.Header
	billingStarted       chan struct{}
	billingRelease       <-chan struct{}
	billingStartOnce     sync.Once
	billingStatus        int
	weeklyBillingStatus  int
	monthlyBillingStatus int
	billingHeaders       http.Header
}

type grokQuotaUpstreamStep struct {
	status int
	body   string
	err    error
}

type grokQuotaSequenceUpstream struct {
	httpUpstreamRecorder
	mu       sync.Mutex
	steps    []grokQuotaUpstreamStep
	requests []*http.Request
}

func (u *grokQuotaSequenceUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.requests = append(u.requests, req)
	index := len(u.requests) - 1
	if index >= len(u.steps) {
		return nil, errors.New("unexpected upstream request")
	}
	step := u.steps[index]
	if step.err != nil {
		return nil, step.err
	}
	return &http.Response{
		StatusCode: step.status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(step.body)),
	}, nil
}

func (u *grokQuotaSequenceUpstream) snapshotRequests() []*http.Request {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]*http.Request(nil), u.requests...)
}

func (u *grokHybridUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	var body []byte
	if req != nil && req.Body != nil {
		body, _ = io.ReadAll(req.Body)
	}
	u.mu.Lock()
	u.requests = append(u.requests, req)
	u.bodies = append(u.bodies, body)
	u.mu.Unlock()

	if req.URL.Path == "/v1/responses" {
		status := u.activeStatus
		if status == 0 {
			status = http.StatusOK
		}
		headers := u.activeHeaders
		if headers == nil {
			headers = http.Header{
				"X-Ratelimit-Limit-Tokens":     []string{"2000000"},
				"X-Ratelimit-Remaining-Tokens": []string{"1500000"},
			}
		}
		return &http.Response{StatusCode: status, Header: headers, Body: io.NopCloser(strings.NewReader(`{"id":"resp_probe"}`))}, nil
	}
	if u.billingStarted != nil {
		u.billingStartOnce.Do(func() { close(u.billingStarted) })
	}
	if u.billingRelease != nil {
		select {
		case <-u.billingRelease:
		case <-req.Context().Done():
			return nil, req.Context().Err()
		}
	}
	billingStatus := u.billingStatus
	if req.URL.RawQuery == "format=credits" && u.weeklyBillingStatus != 0 {
		billingStatus = u.weeklyBillingStatus
	}
	if req.URL.RawQuery != "format=credits" && u.monthlyBillingStatus != 0 {
		billingStatus = u.monthlyBillingStatus
	}
	if billingStatus != 0 && billingStatus != http.StatusOK {
		return &http.Response{
			StatusCode: billingStatus,
			Header:     u.billingHeaders,
			Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"billing limited"}}`)),
		}, nil
	}

	if req.URL.RawQuery == "format=credits" {
		usage := ""
		if u.weeklyUsagePercent != nil {
			usage = `,"creditUsagePercent":` + strconv.FormatFloat(*u.weeklyUsagePercent, 'f', -1, 64)
		}
		payload := `{"config":{"currentPeriod":{"type":"WEEKLY","start":"2026-07-09T03:25:00Z","end":"2026-07-16T03:25:00Z"}` + usage + `}}`
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(payload))}, nil
	}
	monthlyLimit := ""
	if u.monthlyLimitCents != nil {
		monthlyLimit = `,"monthlyLimit":{"val":` + strconv.FormatFloat(*u.monthlyLimitCents, 'f', -1, 64) + `}`
	}
	monthlyPayload := `{"config":{"billingPeriodStart":"2026-07-01T00:00:00Z","billingPeriodEnd":"2026-08-01T00:00:00Z"` + monthlyLimit + `}}`
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(monthlyPayload)),
	}, nil
}

func (u *grokHybridUpstream) snapshot() ([]*http.Request, [][]byte) {
	u.mu.Lock()
	defer u.mu.Unlock()
	requests := append([]*http.Request(nil), u.requests...)
	bodies := make([][]byte, len(u.bodies))
	for i := range u.bodies {
		bodies[i] = append([]byte(nil), u.bodies[i]...)
	}
	return requests, bodies
}

func (r *grokQuotaProxyRepo) GetByID(_ context.Context, id int64) (*Proxy, error) {
	r.calls++
	return r.proxies[id], nil
}

func healthyGrokQuotaOAuthAccount(id int64) *Account {
	return &Account{
		ID:          id,
		Platform:    PlatformGrok,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":  "access-token",
			"refresh_token": "refresh-token",
			"expires_at":    time.Now().Add(2 * grokTokenRefreshSkew).UTC().Format(time.RFC3339),
		},
	}
}

func TestGrokQuotaServiceFetchBillingRetries502ThenSucceeds(t *testing.T) {
	account := healthyGrokQuotaOAuthAccount(401)
	upstream := &grokQuotaSequenceUpstream{steps: []grokQuotaUpstreamStep{
		{status: http.StatusBadGateway, body: `The origin web server returned an invalid or incomplete response to Cloudflare.`},
		{status: http.StatusOK, body: `{"config":{"currentPeriod":{"type":"WEEKLY","start":"2026-07-09T03:25:00Z","end":"2026-07-16T03:25:00Z"},"creditUsagePercent":12}}`},
	}}
	svc := &GrokQuotaService{httpUpstream: upstream}

	summary, status, err := svc.fetchBilling(context.Background(), account, "access-token", "", true)

	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status)
	require.NotNil(t, summary)
	require.NotNil(t, summary.UsagePercent)
	require.Equal(t, 12.0, *summary.UsagePercent)
	requests := upstream.snapshotRequests()
	require.Len(t, requests, 2)
	require.Equal(t, http.MethodGet, requests[0].Method)
	require.Equal(t, "/v1/billing", requests[0].URL.Path)
	require.Equal(t, "format=credits", requests[0].URL.RawQuery)
}

func TestGrokQuotaServiceFetchBillingRetriesTransportErrorThenSucceeds(t *testing.T) {
	account := healthyGrokQuotaOAuthAccount(402)
	upstream := &grokQuotaSequenceUpstream{steps: []grokQuotaUpstreamStep{
		{err: errors.New("temporary transport failure")},
		{status: http.StatusOK, body: `{"config":{"currentPeriod":{"type":"WEEKLY"},"creditUsagePercent":8}}`},
	}}
	svc := &GrokQuotaService{httpUpstream: upstream}

	summary, status, err := svc.fetchBilling(context.Background(), account, "access-token", "", true)

	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status)
	require.NotNil(t, summary)
	require.Len(t, upstream.snapshotRequests(), 2)
}

func TestGrokQuotaServiceFetchBillingStopsAfterSingleTransientRetry(t *testing.T) {
	account := healthyGrokQuotaOAuthAccount(403)
	upstream := &grokQuotaSequenceUpstream{steps: []grokQuotaUpstreamStep{
		{status: http.StatusBadGateway, body: `cloudflare failure`},
		{status: http.StatusBadGateway, body: `cloudflare failure`},
		{status: http.StatusOK, body: `{"config":{"currentPeriod":{"type":"WEEKLY"}}}`},
	}}
	svc := &GrokQuotaService{httpUpstream: upstream}

	summary, status, err := svc.fetchBilling(context.Background(), account, "access-token", "", true)

	require.Error(t, err)
	require.Nil(t, summary)
	require.Equal(t, http.StatusBadGateway, status)
	require.Equal(t, "GROK_QUOTA_PROBE_UPSTREAM_ERROR", infraerrors.Reason(err))
	require.Contains(t, infraerrors.Message(err), "billing returned 502: cloudflare failure")
	require.Len(t, upstream.snapshotRequests(), 2)
}

func TestGrokQuotaServiceFetchBillingDoesNotRetryNonTransientStatuses(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		wantErr bool
	}{
		{name: "unauthorized", status: http.StatusUnauthorized, wantErr: true},
		{name: "forbidden", status: http.StatusForbidden, wantErr: true},
		{name: "rate limited", status: http.StatusTooManyRequests, wantErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := healthyGrokQuotaOAuthAccount(404)
			upstream := &grokQuotaSequenceUpstream{steps: []grokQuotaUpstreamStep{
				{status: tt.status, body: `{"error":{"message":"rejected"}}`},
				{status: http.StatusOK, body: `{"config":{"currentPeriod":{"type":"WEEKLY"}}}`},
			}}
			svc := &GrokQuotaService{httpUpstream: upstream}

			summary, status, err := svc.fetchBilling(context.Background(), account, "access-token", "", true)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Nil(t, summary)
			require.Equal(t, tt.status, status)
			require.Len(t, upstream.snapshotRequests(), 1)
		})
	}
}

func TestIsRetryableGrokBillingStatus(t *testing.T) {
	tests := []struct {
		status int
		want   bool
	}{
		{status: http.StatusBadGateway, want: true},
		{status: http.StatusServiceUnavailable, want: true},
		{status: http.StatusGatewayTimeout, want: true},
		{status: http.StatusUnauthorized, want: false},
		{status: http.StatusForbidden, want: false},
		{status: http.StatusTooManyRequests, want: false},
	}

	for _, tt := range tests {
		t.Run(strconv.Itoa(tt.status), func(t *testing.T) {
			require.Equal(t, tt.want, isRetryableGrokBillingStatus(tt.status))
		})
	}
}

func TestGrokQuotaServiceProbeUsageDoesNotRetryResponsesPost(t *testing.T) {
	account := healthyGrokQuotaOAuthAccount(405)
	repo := &grokQuotaAccountRepo{mockAccountRepoForPlatform: &mockAccountRepoForPlatform{
		accountsByID: map[int64]*Account{account.ID: account},
	}}
	upstream := &grokQuotaSequenceUpstream{steps: []grokQuotaUpstreamStep{
		{status: http.StatusBadGateway, body: `cloudflare failure`},
		{status: http.StatusOK, body: `{"id":"unexpected_retry"}`},
	}}
	svc := NewGrokQuotaService(repo, nil, NewGrokTokenProvider(repo, nil), upstream, nil)

	result, err := svc.ProbeUsage(context.Background(), account.ID)

	require.Error(t, err)
	require.Nil(t, result)
	require.Equal(t, "GROK_QUOTA_PROBE_UPSTREAM_ERROR", infraerrors.Reason(err))
	requests := upstream.snapshotRequests()
	require.Len(t, requests, 1)
	require.Equal(t, http.MethodPost, requests[0].Method)
	require.Equal(t, "/v1/responses", requests[0].URL.Path)
}

func TestGrokQuotaServiceProbeUsageStoresHeaders(t *testing.T) {
	t.Parallel()

	account := healthyGrokQuotaOAuthAccount(42)
	repo := &grokQuotaAccountRepo{
		mockAccountRepoForPlatform: &mockAccountRepoForPlatform{
			accountsByID: map[int64]*Account{42: account},
		},
	}
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"X-Ratelimit-Limit-Requests":     []string{"10"},
			"X-Ratelimit-Remaining-Requests": []string{"7"},
			"X-Ratelimit-Reset-Requests":     []string{"2000000000"},
			"X-Ratelimit-Limit-Tokens":       []string{"1000"},
			"X-Ratelimit-Remaining-Tokens":   []string{"900"},
		},
		Body: io.NopCloser(strings.NewReader(`{"id":"resp_probe"}`)),
	}}
	svc := NewGrokQuotaService(repo, nil, NewGrokTokenProvider(repo, nil), upstream, nil)

	result, err := svc.ProbeUsage(context.Background(), 42)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, result.StatusCode)
	require.Equal(t, "grok-4.5", result.Model)
	require.True(t, result.HeadersObserved)
	require.NotNil(t, result.Snapshot)
	require.True(t, result.Snapshot.HeadersObserved)
	require.Equal(t, "active_probe", result.Snapshot.ObservationSource)
	require.NotEmpty(t, result.Snapshot.LastProbeAt)
	require.NotEmpty(t, result.Snapshot.LastHeadersSeenAt)
	require.NotNil(t, result.Snapshot.Requests)
	require.EqualValues(t, 10, *result.Snapshot.Requests.Limit)
	require.EqualValues(t, 7, *result.Snapshot.Requests.Remaining)
	require.Equal(t, "https://cli-chat-proxy.grok.com/v1/responses", upstream.lastReq.URL.String())
	require.Equal(t, "Bearer access-token", upstream.lastReq.Header.Get("Authorization"))
	require.Equal(t, grokCLIVersion, upstream.lastReq.Header.Get("X-Grok-Client-Version"))
	require.Equal(t, "application/json, text/event-stream", upstream.lastReq.Header.Get("Accept"))
	require.Equal(t, "grok-4.5", gjson.GetBytes(upstream.lastBody, "model").String())
	require.Equal(t, grokQuotaProbeInput, gjson.GetBytes(upstream.lastBody, "input").String())
	require.True(t, gjson.GetBytes(upstream.lastBody, "stream").Bool())
	require.False(t, gjson.GetBytes(upstream.lastBody, "max_output_tokens").Exists())
	require.False(t, gjson.GetBytes(upstream.lastBody, "store").Exists())
	require.NotNil(t, repo.updates[42][grokQuotaSnapshotExtraKey])
}

func TestGrokQuotaServiceProbeUsageIgnoresAccountGrokMapping(t *testing.T) {
	t.Parallel()

	account := healthyGrokQuotaOAuthAccount(47)
	account.Credentials["model_mapping"] = map[string]any{
		"grok":          "grok-composer",
		"grok-composer": "grok-composer-2.5-fast",
	}
	repo := &grokQuotaAccountRepo{
		mockAccountRepoForPlatform: &mockAccountRepoForPlatform{
			accountsByID: map[int64]*Account{47: account},
		},
	}
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader(`{"id":"resp_probe"}`)),
	}}
	svc := NewGrokQuotaService(repo, nil, NewGrokTokenProvider(repo, nil), upstream, nil)

	result, err := svc.ProbeUsage(context.Background(), 47)
	require.NoError(t, err)
	require.Equal(t, "grok-4.5", result.Model)
	require.Equal(t, "grok-4.5", gjson.GetBytes(upstream.lastBody, "model").String())
	require.NotContains(t, string(upstream.lastBody), "grok-composer")
}

func TestGrokQuotaServiceProbeUsageReportsProbeModelOnUpstreamError(t *testing.T) {
	t.Parallel()

	account := healthyGrokQuotaOAuthAccount(48)
	repo := &grokQuotaAccountRepo{
		mockAccountRepoForPlatform: &mockAccountRepoForPlatform{
			accountsByID: map[int64]*Account{48: account},
		},
	}
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader(`{"code":"invalid-argument","error":"Model not found"}`)),
	}}
	svc := NewGrokQuotaService(repo, nil, NewGrokTokenProvider(repo, nil), upstream, nil)

	_, err := svc.ProbeUsage(context.Background(), 48)
	require.Error(t, err)
	require.Equal(t, "GROK_QUOTA_PROBE_UPSTREAM_ERROR", infraerrors.Reason(err))
	require.Contains(t, infraerrors.Message(err), `probe model "grok-4.5"`)
}

func TestGrokQuotaServiceProbeUsageRedactsUpstreamErrorBodyFromErrorAndLogs(t *testing.T) {
	const upstreamSecret = "upstream-secret-refresh-token"
	account := healthyGrokQuotaOAuthAccount(49)
	repo := &grokQuotaAccountRepo{
		mockAccountRepoForPlatform: &mockAccountRepoForPlatform{
			accountsByID: map[int64]*Account{49: account},
		},
	}
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     http.Header{},
		Body: io.NopCloser(strings.NewReader(
			`{"error":"` + upstreamSecret + `","detail":"credential rejected"}`,
		)),
	}}
	svc := NewGrokQuotaService(
		repo,
		nil,
		NewGrokTokenProvider(repo, nil),
		upstream,
		nil,
	)

	var logs bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	defer slog.SetDefault(previousLogger)

	_, err := svc.ProbeUsage(context.Background(), account.ID)
	require.Error(t, err)
	require.Equal(t, "GROK_QUOTA_PROBE_UPSTREAM_ERROR", infraerrors.Reason(err))
	require.Contains(t, infraerrors.Message(err), `probe model "grok-4.5"`)
	require.NotContains(t, err.Error(), upstreamSecret)
	require.NotContains(t, infraerrors.Message(err), upstreamSecret)
	require.Contains(t, logs.String(), "GROK_QUOTA_PROBE_UPSTREAM_ERROR")
	require.NotContains(t, logs.String(), upstreamSecret)
	require.NotContains(t, logs.String(), "credential rejected")
	require.Equal(t, xai.DefaultCLIBaseURL+"/responses", upstream.lastReq.URL.String())
}

func TestGrokQuotaServiceProbeUsageLoadsProxyWhenAccountEdgeMissing(t *testing.T) {
	t.Parallel()

	proxyID := int64(7)
	account := healthyGrokQuotaOAuthAccount(46)
	account.ProxyID = &proxyID
	repo := &grokQuotaAccountRepo{
		mockAccountRepoForPlatform: &mockAccountRepoForPlatform{
			accountsByID: map[int64]*Account{46: account},
		},
	}
	proxyRepo := &grokQuotaProxyRepo{
		proxies: map[int64]*Proxy{
			proxyID: {
				ID:       proxyID,
				Protocol: "http",
				Host:     "proxy.test",
				Port:     3128,
			},
		},
	}
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader(`{"id":"resp_probe"}`)),
	}}
	svc := NewGrokQuotaService(repo, proxyRepo, NewGrokTokenProvider(repo, nil), upstream, nil)

	_, err := svc.ProbeUsage(context.Background(), 46)
	require.NoError(t, err)
	require.Equal(t, 1, proxyRepo.calls)
	require.Equal(t, "http://proxy.test:3128", upstream.lastProxyURL)
}

func TestGrokQuotaServiceProbeUsageStoresNoHeadersState(t *testing.T) {
	t.Parallel()

	account := healthyGrokQuotaOAuthAccount(45)
	observedResetAt := time.Now().Add(-time.Second).UTC().Truncate(time.Second)
	observedLimitedAt := observedResetAt.Add(-grokRateLimitRepeatCooldown)
	account.RateLimitedAt = &observedLimitedAt
	account.RateLimitResetAt = &observedResetAt
	repo := &grokQuotaAccountRepo{
		mockAccountRepoForPlatform: &mockAccountRepoForPlatform{
			accountsByID: map[int64]*Account{45: account},
		},
		recoveryClearResult: true,
	}
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader(`{"id":"resp_probe"}`)),
	}}
	svc := NewGrokQuotaService(repo, nil, NewGrokTokenProvider(repo, nil), upstream, nil)

	result, err := svc.ProbeUsage(context.Background(), 45)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, result.StatusCode)
	require.False(t, result.HeadersObserved)
	require.NotNil(t, result.Snapshot)
	require.False(t, result.Snapshot.HeadersObserved)
	require.Equal(t, "active_probe", result.Snapshot.ObservationSource)
	require.NotEmpty(t, result.Snapshot.LastProbeAt)
	require.Empty(t, result.Snapshot.LastHeadersSeenAt)

	stored, ok := repo.updates[45][grokQuotaSnapshotExtraKey].(*xai.QuotaSnapshot)
	require.True(t, ok)
	require.False(t, stored.HeadersObserved)
	require.Equal(t, http.StatusOK, stored.StatusCode)
	require.Equal(t, 1, repo.recoveryClearCalls)
	require.Equal(t, observedLimitedAt, repo.recoveryObservedAt)
	require.Equal(t, observedResetAt, repo.recoveryObservedReset)
}

func TestGrokQuotaServiceProbeUsageReturnsRateLimitedSnapshot(t *testing.T) {
	t.Parallel()

	account := healthyGrokQuotaOAuthAccount(43)
	repo := &grokQuotaAccountRepo{
		mockAccountRepoForPlatform: &mockAccountRepoForPlatform{
			accountsByID: map[int64]*Account{43: account},
		},
	}
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     http.Header{"Retry-After": []string{"45"}},
		Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"rate limited"}}`)),
	}}
	svc := NewGrokQuotaService(repo, nil, NewGrokTokenProvider(repo, nil), upstream, nil)

	result, err := svc.ProbeUsage(context.Background(), 43)
	require.NoError(t, err)
	require.Equal(t, http.StatusTooManyRequests, result.StatusCode)
	require.NotNil(t, result.Snapshot)
	require.NotNil(t, result.Snapshot.RetryAfterSeconds)
	require.Equal(t, 45, *result.Snapshot.RetryAfterSeconds)
	require.Equal(t, 1, repo.rateLimitedCalls)
	require.Equal(t, account.ID, repo.lastRateLimitedID)
	require.WithinDuration(t, time.Now().Add(45*time.Second), repo.lastRateLimitResetAt, time.Second)
	require.Zero(t, repo.tempUnschedCalls)
}

func TestGrokQuotaServiceQueryQuotaFreeFallsBackToGrok45(t *testing.T) {
	t.Parallel()

	account := healthyGrokQuotaOAuthAccount(51)
	repo := &grokQuotaAccountRepo{mockAccountRepoForPlatform: &mockAccountRepoForPlatform{
		accountsByID: map[int64]*Account{account.ID: account},
	}}
	upstream := &grokHybridUpstream{}
	usageRepo := &grokQuotaUsageLogRepo{stats: &usagestats.AccountStats{Tokens: 1_000_000}}
	svc := NewGrokQuotaService(repo, nil, NewGrokTokenProvider(repo, nil), upstream, nil, usageRepo)

	result, err := svc.QueryQuota(context.Background(), account.ID)
	require.NoError(t, err)
	require.Equal(t, "hybrid_probe", result.Source)
	require.Equal(t, "grok-4.5", result.Model)
	require.NotNil(t, result.Billing)
	require.Nil(t, result.Billing.UsagePercent)
	require.NotNil(t, result.LocalUsage24h)
	require.EqualValues(t, 1_000_000, result.LocalUsage24h.Tokens)
	require.Equal(t, 1, usageRepo.calls)
	require.WithinDuration(t, time.Now().UTC().Add(-24*time.Hour), usageRepo.startTimes[0], time.Second)
	require.NotNil(t, result.Snapshot)
	require.NotNil(t, result.Snapshot.Tokens)
	require.EqualValues(t, 2_000_000, *result.Snapshot.Tokens.Limit)
	require.True(t, result.HeadersObserved)

	requests, bodies := upstream.snapshot()
	require.Len(t, requests, 3)
	responseCalls := 0
	for i, req := range requests {
		if req.URL.Path != "/v1/responses" {
			continue
		}
		responseCalls++
		require.Equal(t, http.MethodPost, req.Method)
		require.Equal(t, "application/json, text/event-stream", req.Header.Get("Accept"))
		require.Equal(t, "grok-4.5", gjson.GetBytes(bodies[i], "model").String())
		require.Equal(t, grokQuotaProbeInput, gjson.GetBytes(bodies[i], "input").String())
		require.True(t, gjson.GetBytes(bodies[i], "stream").Bool())
		require.False(t, gjson.GetBytes(bodies[i], "max_output_tokens").Exists())
		require.False(t, gjson.GetBytes(bodies[i], "store").Exists())
	}
	require.Equal(t, 1, responseCalls)
}

func TestGrokQuotaServiceQueryQuotaPaidBillingSkipsActiveProbe(t *testing.T) {
	t.Parallel()

	account := healthyGrokQuotaOAuthAccount(52)
	repo := &grokQuotaAccountRepo{mockAccountRepoForPlatform: &mockAccountRepoForPlatform{
		accountsByID: map[int64]*Account{account.ID: account},
	}}
	usagePercent := 25.0
	upstream := &grokHybridUpstream{weeklyUsagePercent: &usagePercent}
	usageRepo := &grokQuotaUsageLogRepo{stats: &usagestats.AccountStats{Tokens: 1_000_000}}
	svc := NewGrokQuotaService(repo, nil, NewGrokTokenProvider(repo, nil), upstream, nil, usageRepo)

	result, err := svc.QueryQuota(context.Background(), account.ID)
	require.NoError(t, err)
	require.Equal(t, "billing_probe", result.Source)
	require.NotNil(t, result.Billing)
	require.InDelta(t, usagePercent, *result.Billing.UsagePercent, 1e-9)
	require.Nil(t, result.Snapshot)
	require.Empty(t, result.Model)
	require.Nil(t, result.LocalUsage24h)

	requests, _ := upstream.snapshot()
	require.Len(t, requests, 2)
	for _, req := range requests {
		require.Equal(t, "/v1/billing", req.URL.Path)
	}
}

func TestGrokQuotaServiceQueryQuotaCustomPaidMonthlyLimitSkipsActiveProbe(t *testing.T) {
	t.Parallel()

	account := healthyGrokQuotaOAuthAccount(57)
	repo := &grokQuotaAccountRepo{mockAccountRepoForPlatform: &mockAccountRepoForPlatform{
		accountsByID: map[int64]*Account{account.ID: account},
	}}
	monthlyLimit := 25_000.0
	upstream := &grokHybridUpstream{monthlyLimitCents: &monthlyLimit}
	svc := NewGrokQuotaService(repo, nil, NewGrokTokenProvider(repo, nil), upstream, nil)

	result, err := svc.QueryQuota(context.Background(), account.ID)
	require.NoError(t, err)
	require.Equal(t, "billing_probe", result.Source)
	require.NotNil(t, result.Billing)
	require.InDelta(t, monthlyLimit, *result.Billing.MonthlyLimitCents, 1e-9)
	require.Nil(t, result.Snapshot)

	requests, _ := upstream.snapshot()
	require.Len(t, requests, 2)
	for _, req := range requests {
		require.Equal(t, "/v1/billing", req.URL.Path)
	}
}

func TestGrokLocalUsage24hUsesRollingUTCWindow(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 14, 20, 30, 0, 0, time.FixedZone("UTC+8", 8*60*60))

	t.Run("returns usage from exact rolling window", func(t *testing.T) {
		repo := &grokQuotaUsageLogRepo{stats: &usagestats.AccountStats{Tokens: 1_250_000}}
		stats := grokLocalUsage24h(context.Background(), repo, 57, now)

		require.NotNil(t, stats)
		require.EqualValues(t, 1_250_000, stats.Tokens)
		require.Equal(t, []time.Time{now.UTC().Add(-24 * time.Hour)}, repo.startTimes)
	})

	t.Run("query failure returns no stats", func(t *testing.T) {
		repo := &grokQuotaUsageLogRepo{err: context.DeadlineExceeded}
		stats := grokLocalUsage24h(context.Background(), repo, 57, now)

		require.Nil(t, stats)
		require.Equal(t, []time.Time{now.UTC().Add(-24 * time.Hour)}, repo.startTimes)
	})

	t.Run("missing repository returns no stats", func(t *testing.T) {
		require.Nil(t, grokLocalUsage24h(context.Background(), nil, 57, now))
	})

	t.Run("invalid account returns no stats without query", func(t *testing.T) {
		repo := &grokQuotaUsageLogRepo{}
		require.Nil(t, grokLocalUsage24h(context.Background(), repo, 0, now))
		require.Zero(t, repo.calls)
	})
}

func TestGrokLocalUsageForQuotaSelectsFreeOrPaidWindows(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	billing := &xai.BillingSummary{
		PeriodType:         "weekly",
		PeriodStart:        now.Add(-4 * 24 * time.Hour).Format(time.RFC3339),
		PeriodEnd:          now.Add(3 * 24 * time.Hour).Format(time.RFC3339),
		BillingPeriodStart: now.Add(-13 * 24 * time.Hour).Format(time.RFC3339),
		BillingPeriodEnd:   now.Add(17 * 24 * time.Hour).Format(time.RFC3339),
	}

	t.Run("free queries only rolling 24h", func(t *testing.T) {
		repo := &grokQuotaUsageLogRepo{stats: &usagestats.AccountStats{Tokens: 500_000}}
		rolling, weekly, monthly := grokLocalUsageForQuota(context.Background(), repo, 57, billing, now)

		require.NotNil(t, rolling)
		require.Nil(t, weekly)
		require.Nil(t, monthly)
		require.Equal(t, []time.Time{now.Add(-24 * time.Hour)}, repo.startTimes)
	})

	t.Run("paid queries only billing windows", func(t *testing.T) {
		usagePercent := 25.0
		paidBilling := *billing
		paidBilling.UsagePercent = &usagePercent
		repo := &grokQuotaUsageLogRepo{stats: &usagestats.AccountStats{Tokens: 500_000}}
		rolling, weekly, monthly := grokLocalUsageForQuota(context.Background(), repo, 57, &paidBilling, now)

		require.Nil(t, rolling)
		require.NotNil(t, weekly)
		require.NotNil(t, monthly)
		require.Equal(t, []time.Time{
			now.Add(-4 * 24 * time.Hour),
			now.Add(-13 * 24 * time.Hour),
		}, repo.startTimes)
	})
}

func TestGrokLocalUsageForBillingOnlyReturnsAvailableWindows(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	billing := &xai.BillingSummary{
		PeriodType:  "weekly",
		PeriodStart: now.Add(-4 * 24 * time.Hour).Format(time.RFC3339),
		PeriodEnd:   now.Add(3 * 24 * time.Hour).Format(time.RFC3339),
	}

	t.Run("valid weekly window", func(t *testing.T) {
		repo := &grokQuotaUsageLogRepo{stats: &usagestats.AccountStats{Tokens: 1_500_000}}
		weekly, monthly := grokLocalUsageForBilling(context.Background(), repo, 57, billing, now)
		require.NotNil(t, weekly)
		require.EqualValues(t, 1_500_000, weekly.Tokens)
		require.Nil(t, monthly)
		require.Equal(t, 1, repo.calls)
	})

	t.Run("query failure", func(t *testing.T) {
		repo := &grokQuotaUsageLogRepo{err: context.DeadlineExceeded}
		weekly, monthly := grokLocalUsageForBilling(context.Background(), repo, 57, billing, now)
		require.Nil(t, weekly)
		require.Nil(t, monthly)
		require.Equal(t, 1, repo.calls)
	})

	t.Run("missing billing window", func(t *testing.T) {
		repo := &grokQuotaUsageLogRepo{}
		weekly, monthly := grokLocalUsageForBilling(context.Background(), repo, 57, nil, now)
		require.Nil(t, weekly)
		require.Nil(t, monthly)
		require.Zero(t, repo.calls)
	})
}

func TestAccountUsageServiceGrokRefreshUsesBillingOnly(t *testing.T) {
	t.Parallel()

	account := healthyGrokQuotaOAuthAccount(54)
	repo := &grokQuotaAccountRepo{mockAccountRepoForPlatform: &mockAccountRepoForPlatform{
		accountsByID: map[int64]*Account{account.ID: account},
	}}
	upstream := &grokHybridUpstream{}
	usageRepo := &grokQuotaUsageLogRepo{stats: &usagestats.AccountStats{Tokens: 750_000}}
	quotaService := NewGrokQuotaService(repo, nil, NewGrokTokenProvider(repo, nil), upstream, nil, usageRepo)
	usageService := &AccountUsageService{
		grokQuotaFetcher: NewGrokQuotaFetcher(),
		grokQuotaService: quotaService,
		usageLogRepo:     usageRepo,
		cache:            NewUsageCache(),
	}

	usage, err := usageService.getGrokUsage(context.Background(), account, false)
	require.NoError(t, err)
	require.NotNil(t, usage.GrokBilling)
	require.Nil(t, usage.GrokBilling.UsagePercent)
	require.NotNil(t, usage.GrokLocalUsage24h)
	require.EqualValues(t, 750_000, usage.GrokLocalUsage24h.Tokens)
	require.Equal(t, 1, usageRepo.calls)
	require.Len(t, usageRepo.startTimes, 1)
	require.WithinDuration(t, time.Now().UTC().Add(-24*time.Hour), usageRepo.startTimes[0], time.Second)

	requests, _ := upstream.snapshot()
	require.Len(t, requests, 2)
	for _, req := range requests {
		require.Equal(t, http.MethodGet, req.Method)
		require.Equal(t, "/v1/billing", req.URL.Path)
	}
}

func TestGrokQuotaServiceProbeFlightsDeduplicateBillingAndSeparateActive(t *testing.T) {
	t.Parallel()

	account := healthyGrokQuotaOAuthAccount(55)
	repo := &grokQuotaAccountRepo{mockAccountRepoForPlatform: &mockAccountRepoForPlatform{
		accountsByID: map[int64]*Account{account.ID: account},
	}}
	billingStarted := make(chan struct{})
	billingRelease := make(chan struct{})
	upstream := &grokHybridUpstream{billingStarted: billingStarted, billingRelease: billingRelease}
	svc := NewGrokQuotaService(repo, nil, NewGrokTokenProvider(repo, nil), upstream, nil)

	type probeOutcome struct {
		result *GrokQuotaProbeResult
		err    error
	}
	billingOutcomes := make(chan probeOutcome, 2)
	go func() {
		result, err := svc.ProbeBilling(context.Background(), account.ID)
		billingOutcomes <- probeOutcome{result: result, err: err}
	}()
	<-billingStarted
	secondStarted := make(chan struct{})
	go func() {
		close(secondStarted)
		result, err := svc.ProbeBilling(context.Background(), account.ID)
		billingOutcomes <- probeOutcome{result: result, err: err}
	}()
	<-secondStarted
	time.Sleep(25 * time.Millisecond)

	activeResult, err := svc.ProbeUsage(context.Background(), account.ID)
	require.NoError(t, err)
	require.NotNil(t, activeResult.Snapshot)
	close(billingRelease)
	for range 2 {
		outcome := <-billingOutcomes
		require.NoError(t, outcome.err)
		require.NotNil(t, outcome.result.Billing)
	}

	requests, _ := upstream.snapshot()
	billingCalls := 0
	activeCalls := 0
	for _, req := range requests {
		switch req.URL.Path {
		case "/v1/billing":
			billingCalls++
		case "/v1/responses":
			activeCalls++
		}
	}
	require.Equal(t, 2, billingCalls)
	require.Equal(t, 1, activeCalls)
}

func TestGrokQuotaServiceBilling429DoesNotPauseModelScheduling(t *testing.T) {
	t.Parallel()

	account := healthyGrokQuotaOAuthAccount(56)
	repo := &grokQuotaAccountRepo{mockAccountRepoForPlatform: &mockAccountRepoForPlatform{
		accountsByID: map[int64]*Account{account.ID: account},
	}}
	upstream := &grokHybridUpstream{
		billingStatus:  http.StatusTooManyRequests,
		billingHeaders: http.Header{"Retry-After": []string{"45"}},
	}
	svc := NewGrokQuotaService(repo, nil, NewGrokTokenProvider(repo, nil), upstream, nil)

	result, err := svc.ProbeBilling(context.Background(), account.ID)

	require.Error(t, err)
	require.Nil(t, result)
	require.Zero(t, repo.rateLimitedCalls)
}

func TestGrokQuotaServiceBilling403PersistsMediaEligibilitySignal(t *testing.T) {
	t.Parallel()

	account := healthyGrokQuotaOAuthAccount(58)
	repo := &grokQuotaAccountRepo{mockAccountRepoForPlatform: &mockAccountRepoForPlatform{
		accountsByID: map[int64]*Account{account.ID: account},
	}}
	upstream := &grokHybridUpstream{billingStatus: http.StatusForbidden}
	svc := NewGrokQuotaService(repo, nil, NewGrokTokenProvider(repo, nil), upstream, nil)

	result, err := svc.ProbeBilling(context.Background(), account.ID)

	require.Error(t, err)
	require.Nil(t, result)
	require.Equal(t, 1, repo.updateCalls)
	raw := repo.updates[account.ID][grokBillingExtraKey]
	billing, ok := raw.(*xai.BillingSummary)
	require.True(t, ok)
	require.Equal(t, http.StatusForbidden, billing.StatusCode)
	require.Equal(t, http.StatusForbidden, billing.WeeklyStatusCode)
	require.Equal(t, http.StatusForbidden, billing.MonthlyStatusCode)
	require.True(t, billing.Partial)

	account.Extra = map[string]any{grokBillingExtraKey: billing}
	eligible, reason := account.GrokMediaGenerationEligibility()
	require.False(t, eligible)
	require.Equal(t, "billing_forbidden", reason)
}

func TestGrokQuotaServicePartialBilling403PersistsMediaEligibilitySignal(t *testing.T) {
	t.Parallel()

	account := healthyGrokQuotaOAuthAccount(59)
	repo := &grokQuotaAccountRepo{mockAccountRepoForPlatform: &mockAccountRepoForPlatform{
		accountsByID: map[int64]*Account{account.ID: account},
	}}
	upstream := &grokHybridUpstream{
		weeklyBillingStatus:  http.StatusForbidden,
		monthlyBillingStatus: http.StatusOK,
	}
	svc := NewGrokQuotaService(repo, nil, NewGrokTokenProvider(repo, nil), upstream, nil)

	result, err := svc.ProbeBilling(context.Background(), account.ID)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.Billing)
	require.Equal(t, http.StatusOK, result.StatusCode)
	require.Equal(t, http.StatusForbidden, result.Billing.WeeklyStatusCode)
	require.Equal(t, http.StatusOK, result.Billing.MonthlyStatusCode)
	require.True(t, result.Billing.Partial)
	require.Contains(t, result.Billing.FailedWindows, "weekly")
	require.Equal(t, 1, repo.updateCalls)

	account.Extra = map[string]any{grokBillingExtraKey: result.Billing}
	eligible, reason := account.GrokMediaGenerationEligibility()
	require.False(t, eligible)
	require.Equal(t, "billing_forbidden", reason)
}

func TestGrokQuotaServiceProbeMediaEligibility(t *testing.T) {
	t.Run("positive paid evidence enables media", func(t *testing.T) {
		usagePercent := 10.0
		monthlyLimit := 15_000.0
		account := healthyGrokQuotaOAuthAccount(60)
		repo := &grokQuotaAccountRepo{mockAccountRepoForPlatform: &mockAccountRepoForPlatform{
			accountsByID: map[int64]*Account{account.ID: account},
		}}
		upstream := &grokHybridUpstream{weeklyUsagePercent: &usagePercent, monthlyLimitCents: &monthlyLimit}
		svc := NewGrokQuotaService(repo, nil, NewGrokTokenProvider(repo, nil), upstream, nil)

		eligible, reason, err := svc.ProbeMediaEligibility(context.Background(), account.ID)

		require.NoError(t, err)
		require.True(t, eligible)
		require.Equal(t, "eligible", reason)
	})

	t.Run("successful empty billing identifies free account", func(t *testing.T) {
		account := healthyGrokQuotaOAuthAccount(61)
		repo := &grokQuotaAccountRepo{mockAccountRepoForPlatform: &mockAccountRepoForPlatform{
			accountsByID: map[int64]*Account{account.ID: account},
		}}
		svc := NewGrokQuotaService(repo, nil, NewGrokTokenProvider(repo, nil), &grokHybridUpstream{}, nil)

		eligible, reason, err := svc.ProbeMediaEligibility(context.Background(), account.ID)

		require.NoError(t, err)
		require.False(t, eligible)
		require.Equal(t, "billing_free_tier", reason)
	})

	t.Run("forbidden billing is deterministic ineligibility", func(t *testing.T) {
		account := healthyGrokQuotaOAuthAccount(62)
		repo := &grokQuotaAccountRepo{mockAccountRepoForPlatform: &mockAccountRepoForPlatform{
			accountsByID: map[int64]*Account{account.ID: account},
		}}
		svc := NewGrokQuotaService(repo, nil, NewGrokTokenProvider(repo, nil), &grokHybridUpstream{billingStatus: http.StatusForbidden}, nil)

		eligible, reason, err := svc.ProbeMediaEligibility(context.Background(), account.ID)

		require.NoError(t, err)
		require.False(t, eligible)
		require.Equal(t, "billing_forbidden", reason)
	})
}

func TestPreferBillingObservationStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		weeklyStatus  int
		monthlyStatus int
		want          int
	}{
		{name: "weekly forbidden wins", weeklyStatus: http.StatusForbidden, monthlyStatus: http.StatusBadGateway, want: http.StatusForbidden},
		{name: "monthly forbidden wins", weeklyStatus: http.StatusBadGateway, monthlyStatus: http.StatusForbidden, want: http.StatusForbidden},
		{name: "weekly observation otherwise wins", weeklyStatus: http.StatusTooManyRequests, monthlyStatus: http.StatusBadGateway, want: http.StatusTooManyRequests},
		{name: "monthly observation is fallback", monthlyStatus: http.StatusBadGateway, want: http.StatusBadGateway},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, preferBillingObservationStatus(tt.weeklyStatus, tt.monthlyStatus))
		})
	}
}

func TestGrokQuotaServiceQueryQuotaFree429PersistsLimitAndKeepsBilling(t *testing.T) {
	t.Parallel()

	account := healthyGrokQuotaOAuthAccount(53)
	repo := &grokQuotaAccountRepo{mockAccountRepoForPlatform: &mockAccountRepoForPlatform{
		accountsByID: map[int64]*Account{account.ID: account},
	}}
	upstream := &grokHybridUpstream{
		activeStatus:  http.StatusTooManyRequests,
		activeHeaders: http.Header{"Retry-After": []string{"45"}},
	}
	svc := NewGrokQuotaService(repo, nil, NewGrokTokenProvider(repo, nil), upstream, nil)

	result, err := svc.QueryQuota(context.Background(), account.ID)
	require.NoError(t, err)
	require.Equal(t, http.StatusTooManyRequests, result.StatusCode)
	require.NotNil(t, result.Billing)
	require.NotNil(t, result.Snapshot)
	require.Equal(t, 45, *result.Snapshot.RetryAfterSeconds)
	require.Equal(t, 1, repo.rateLimitedCalls)
	require.Equal(t, account.ID, repo.lastRateLimitedID)
	require.WithinDuration(t, time.Now().Add(45*time.Second), repo.lastRateLimitResetAt, time.Second)
}

func TestGrokQuotaServiceResetQuotaUnsupported(t *testing.T) {
	t.Parallel()

	account := &Account{
		ID:       44,
		Platform: PlatformGrok,
		Type:     AccountTypeOAuth,
	}
	repo := &grokQuotaAccountRepo{
		mockAccountRepoForPlatform: &mockAccountRepoForPlatform{
			accountsByID: map[int64]*Account{44: account},
		},
	}
	svc := NewGrokQuotaService(repo, nil, nil, nil, nil)

	_, err := svc.ResetQuota(context.Background(), 44)
	require.Error(t, err)
	require.Equal(t, http.StatusNotImplemented, infraerrors.Code(err))
	require.Equal(t, "GROK_QUOTA_RESET_UNSUPPORTED", infraerrors.Reason(err))
}

func TestShouldAutoPauseGrokAccountByQuota(t *testing.T) {
	t.Parallel()

	zero := int64(0)
	limit := int64(10)
	resetFuture := time.Now().Add(time.Minute).Unix()
	retryAfter := 30
	tests := []struct {
		name     string
		snapshot xai.QuotaSnapshot
		want     bool
	}{
		{
			name: "remaining requests exhausted",
			snapshot: xai.QuotaSnapshot{
				Requests:  &xai.QuotaWindow{Limit: &limit, Remaining: &zero, ResetUnix: &resetFuture},
				UpdatedAt: time.Now().UTC().Format(time.RFC3339),
			},
			want: true,
		},
		{
			name: "retry after active",
			snapshot: xai.QuotaSnapshot{
				RetryAfterSeconds: &retryAfter,
				UpdatedAt:         time.Now().UTC().Format(time.RFC3339),
			},
			want: true,
		},
		{
			name: "retry after expired",
			snapshot: xai.QuotaSnapshot{
				RetryAfterSeconds: &retryAfter,
				UpdatedAt:         time.Now().Add(-time.Duration(retryAfter+1) * time.Second).UTC().Format(time.RFC3339),
			},
			want: false,
		},
		{
			name: "stale snapshot ignored",
			snapshot: xai.QuotaSnapshot{
				Requests:  &xai.QuotaWindow{Limit: &limit, Remaining: &zero, ResetUnix: &resetFuture},
				UpdatedAt: time.Now().Add(-3 * time.Hour).UTC().Format(time.RFC3339),
			},
			want: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			account := &Account{
				Platform: PlatformGrok,
				Type:     AccountTypeOAuth,
				Extra: map[string]any{
					grokQuotaSnapshotExtraKey: tt.snapshot,
				},
			}
			got, _ := shouldAutoPauseGrokAccountByQuota(account)
			require.Equal(t, tt.want, got)
		})
	}
}
