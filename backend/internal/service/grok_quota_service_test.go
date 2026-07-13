//go:build unit

package service

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/stretchr/testify/require"
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
}

func (r *grokQuotaAccountRepo) UpdateExtra(_ context.Context, id int64, updates map[string]any) error {
	r.updateCalls++
	if r.updates == nil {
		r.updates = make(map[int64]map[string]any)
	}
	r.updates[id] = updates
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

func (r *grokQuotaProxyRepo) GetByID(_ context.Context, id int64) (*Proxy, error) {
	r.calls++
	return r.proxies[id], nil
}

// billingUpstreamMock returns different bodies for weekly/monthly billing URLs.
type billingUpstreamMock struct {
	mu           sync.Mutex
	requests     []*http.Request
	lastProxyURL string
	byURL        map[string]*http.Response
	err          error
}

func (u *billingUpstreamMock) Do(req *http.Request, proxyURL string, _ int64, _ int) (*http.Response, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.requests = append(u.requests, req)
	u.lastProxyURL = proxyURL
	if u.err != nil {
		return nil, u.err
	}
	if req == nil || req.URL == nil {
		return nil, io.EOF
	}
	key := req.URL.String()
	resp, ok := u.byURL[key]
	if !ok {
		// Fallback: match by path+query suffix
		for k, v := range u.byURL {
			if strings.HasSuffix(key, k) || strings.Contains(key, k) {
				resp = v
				ok = true
				break
			}
		}
	}
	if !ok || resp == nil {
		return &http.Response{
			StatusCode: http.StatusNotFound,
			Body:       io.NopCloser(strings.NewReader(`{"error":"not found"}`)),
		}, nil
	}
	// Re-wrap body so concurrent readers each get a fresh copy.
	bodyBytes, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	resp.Body = io.NopCloser(strings.NewReader(string(bodyBytes)))
	return &http.Response{
		StatusCode: resp.StatusCode,
		Header:     resp.Header.Clone(),
		Body:       io.NopCloser(strings.NewReader(string(bodyBytes))),
	}, nil
}

func (u *billingUpstreamMock) DoWithTLS(req *http.Request, proxyURL string, accountID int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, concurrency)
}

func weeklyBillingBody() string {
	return `{"config":{"currentPeriod":{"type":"weekly","start":"2026-07-01T00:00:00Z","end":"2026-07-08T00:00:00Z"},"creditUsagePercent":42.5,"productUsage":[{"product":"Grok","usagePercent":40}]}}`
}

func monthlyBillingBody() string {
	return `{"config":{"monthlyLimit":{"val":20000},"used":{"val":3000},"onDemandCap":{"val":5000},"billingPeriodEnd":"2026-07-31T00:00:00Z"}}`
}

func newBillingUpstreamOK() *billingUpstreamMock {
	return &billingUpstreamMock{
		byURL: map[string]*http.Response{
			xai.BillingWeeklyURL: {
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(weeklyBillingBody())),
			},
			xai.BillingMonthlyURL: {
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(monthlyBillingBody())),
			},
		},
	}
}

func TestGrokQuotaServiceProbeUsageStoresBilling(t *testing.T) {
	t.Parallel()

	account := &Account{
		ID:          42,
		Platform:    PlatformGrok,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token": "access-token",
			"expires_at":   time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
			"email":        "user@example.com",
		},
	}
	repo := &grokQuotaAccountRepo{
		mockAccountRepoForPlatform: &mockAccountRepoForPlatform{
			accountsByID: map[int64]*Account{42: account},
		},
	}
	upstream := newBillingUpstreamOK()
	svc := NewGrokQuotaService(repo, nil, NewGrokTokenProvider(repo, nil), upstream)

	result, err := svc.ProbeUsage(context.Background(), 42)
	require.NoError(t, err)
	require.Equal(t, "billing_probe", result.Source)
	require.True(t, result.BillingObserved)
	require.NotNil(t, result.Billing)
	require.Equal(t, "weekly", result.Billing.PeriodType)
	require.NotNil(t, result.Billing.UsagePercent)
	require.Equal(t, 42.5, *result.Billing.UsagePercent)
	require.NotNil(t, result.Billing.MonthlyLimitCents)
	require.EqualValues(t, 20000, *result.Billing.MonthlyLimitCents)
	require.Equal(t, "supergrok", result.Billing.PlanLabel)
	require.False(t, result.ResetSupported)

	require.Len(t, upstream.requests, 2)
	urls := []string{upstream.requests[0].URL.String(), upstream.requests[1].URL.String()}
	require.Contains(t, urls, xai.BillingWeeklyURL)
	require.Contains(t, urls, xai.BillingMonthlyURL)
	for _, req := range upstream.requests {
		require.Equal(t, http.MethodGet, req.Method)
		require.Equal(t, "Bearer access-token", req.Header.Get("Authorization"))
		require.Equal(t, "user@example.com", req.Header.Get("x-userid"))
		require.Equal(t, grokCLIVersion, req.Header.Get("X-Grok-Client-Version"))
	}

	stored, ok := repo.updates[42][grokBillingExtraKey].(*xai.BillingSummary)
	require.True(t, ok)
	require.NotNil(t, stored)
	require.Equal(t, "supergrok", stored.PlanLabel)
	require.NotEmpty(t, repo.updates[42][grokBillingUpdatedAtKey])
}

func TestGrokQuotaServiceProbeUsageUsesIDTokenSubject(t *testing.T) {
	t.Parallel()

	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"user-sub-1","email":"a@b.com"}`))
	idToken := "eyJhbGciOiJub25lIn0." + payload + ".x"
	account := &Account{
		ID:          49,
		Platform:    PlatformGrok,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token": "access-token",
			"id_token":     idToken,
			"expires_at":   time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		},
	}
	repo := &grokQuotaAccountRepo{
		mockAccountRepoForPlatform: &mockAccountRepoForPlatform{
			accountsByID: map[int64]*Account{49: account},
		},
	}
	upstream := newBillingUpstreamOK()
	svc := NewGrokQuotaService(repo, nil, NewGrokTokenProvider(repo, nil), upstream)

	_, err := svc.ProbeUsage(context.Background(), 49)
	require.NoError(t, err)
	require.NotEmpty(t, upstream.requests)
	require.Equal(t, "user-sub-1", upstream.requests[0].Header.Get("x-userid"))
}

func TestGrokQuotaServiceProbeUsageLoadsProxyWhenAccountEdgeMissing(t *testing.T) {
	t.Parallel()

	proxyID := int64(7)
	account := &Account{
		ID:          46,
		Platform:    PlatformGrok,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		ProxyID:     &proxyID,
		Credentials: map[string]any{
			"access_token": "access-token",
			"expires_at":   time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		},
	}
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
	upstream := newBillingUpstreamOK()
	svc := NewGrokQuotaService(repo, proxyRepo, NewGrokTokenProvider(repo, nil), upstream)

	_, err := svc.ProbeUsage(context.Background(), 46)
	require.NoError(t, err)
	require.Equal(t, 1, proxyRepo.calls)
	require.Equal(t, "http://proxy.test:3128", upstream.lastProxyURL)
}

func TestGrokQuotaServiceProbeUsageReportsUpstreamError(t *testing.T) {
	t.Parallel()

	account := &Account{
		ID:          48,
		Platform:    PlatformGrok,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token": "access-token",
			"expires_at":   time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		},
	}
	repo := &grokQuotaAccountRepo{
		mockAccountRepoForPlatform: &mockAccountRepoForPlatform{
			accountsByID: map[int64]*Account{48: account},
		},
	}
	upstream := &billingUpstreamMock{
		byURL: map[string]*http.Response{
			xai.BillingWeeklyURL: {
				StatusCode: http.StatusUnauthorized,
				Body:       io.NopCloser(strings.NewReader(`{"error":"unauthorized"}`)),
			},
			xai.BillingMonthlyURL: {
				StatusCode: http.StatusUnauthorized,
				Body:       io.NopCloser(strings.NewReader(`{"error":"unauthorized"}`)),
			},
		},
	}
	svc := NewGrokQuotaService(repo, nil, NewGrokTokenProvider(repo, nil), upstream)

	_, err := svc.ProbeUsage(context.Background(), 48)
	require.Error(t, err)
	require.Equal(t, "GROK_QUOTA_PROBE_UPSTREAM_ERROR", infraerrors.Reason(err))
	require.Contains(t, infraerrors.Message(err), "billing probe failed")
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
	svc := NewGrokQuotaService(repo, nil, nil, nil)

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
