package service

import (
	"context"
	"fmt"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type claudeAutoIntegrationRepo struct {
	AccountRepository
	account *Account
}

func (r *claudeAutoIntegrationRepo) GetByID(context.Context, int64) (*Account, error) {
	return r.account, nil
}
func (r *claudeAutoIntegrationRepo) ListByPlatform(context.Context, string) ([]Account, error) {
	return []Account{*r.account}, nil
}
func (r *claudeAutoIntegrationRepo) UpdateExtra(context.Context, int64, map[string]any) error {
	return nil
}

type claudeAutoIntegrationLease struct {
	mu   sync.Mutex
	keys map[string]bool
}

func (l *claudeAutoIntegrationLease) TryAcquireLeaderLock(_ context.Context, k, o string, _ time.Duration) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.keys == nil {
		l.keys = map[string]bool{}
	}
	if l.keys[k] {
		return false, nil
	}
	l.keys[k] = true
	return true, nil
}
func (l *claudeAutoIntegrationLease) ReleaseLeaderLock(_ context.Context, k, o string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.keys, k)
	return nil
}

func newClaudeAutoIntegration(t *testing.T, enabled bool, used float64) (*claudeQuotaAutoReset, *atomic.Int32) {
	t.Helper()
	repo := &claudeAutoIntegrationRepo{account: &Account{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Credentials: map[string]any{"scope": "user:profile"}, Extra: map[string]any{
		"claude_auto_reset_credit_enabled": enabled, "claude_auto_reset_credit_mode": "expiring", "claude_auto_reset_credit_expiry_horizon_seconds": 3600, "claude_auto_reset_credit_expiry_min_utilization": 0.5,
	}}}
	s := &ClaudeResetCreditService{accounts: repo, writer: repo, tokens: resetTokenStub{}, now: time.Now, idempotency: NewIdempotencyCoordinator(newInMemoryIdempotencyRepo(), DefaultIdempotencyConfig()), locks: &claudeAutoIntegrationLease{}}
	count := &atomic.Int32{}
	ends := time.Now().Add(30 * time.Minute).UTC().Format(time.RFC3339)
	s.do = func(r *http.Request, _ string) (*http.Response, error) {
		body := fmt.Sprintf(`{"cedar_ember":{"eligible":true,"next_grant_id":"grant","grants":[{"id":"grant","resets_left":1,"usable_now":true,"use_requires_limit":false,"ends_at":%q,"clears":["five_hour"],"percent_used":{"five_hour":%v}}]}}`, ends, used)
		if strings.HasSuffix(r.URL.Path, "/profile") {
			body = `{"organization":{"uuid":"11111111-1111-4111-8111-111111111111"}}`
		}
		if r.Method == "POST" {
			count.Add(1)
			body = `{"result":"reset"}`
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
	}
	return &claudeQuotaAutoReset{service: s, accounts: repo}, count
}
func TestClaudeAutoIntegrationDisabledAndUsefulExpiry(t *testing.T) {
	for _, tc := range []struct {
		name    string
		enabled bool
		used    float64
		want    int32
	}{{"disabled", false, 90, 0}, {"unused", true, 0, 0}, {"below-useful", true, 49, 0}, {"useful", true, 75, 1}} {
		t.Run(tc.name, func(t *testing.T) {
			w, count := newClaudeAutoIntegration(t, tc.enabled, tc.used)
			w.scan(context.Background())
			require.Equal(t, tc.want, count.Load())
		})
	}
}
func TestClaudeAutoIntegrationConcurrentScansSpendOnce(t *testing.T) {
	w, count := newClaudeAutoIntegration(t, true, 75)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); w.scan(context.Background()) }()
	}
	wg.Wait()
	require.Equal(t, int32(1), count.Load())
}

type claudeAutoIntegrationUsage struct{}

func (claudeAutoIntegrationUsage) FetchUsage(context.Context, string, string) (*ClaudeUsageResponse, error) {
	u := &ClaudeUsageResponse{}
	u.SevenDay.Utilization = 100
	u.SevenDay.ResetsAt = time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	return u, nil
}
func (f claudeAutoIntegrationUsage) FetchUsageWithOptions(c context.Context, _ *ClaudeUsageFetchOptions) (*ClaudeUsageResponse, error) {
	return f.FetchUsage(c, "", "")
}
func TestClaudeAutoIntegrationExhaustedWindowMustBeCleared(t *testing.T) {
	w, count := newClaudeAutoIntegration(t, true, 75)
	r := w.accounts.(*claudeAutoIntegrationRepo)
	r.account.Extra["claude_auto_reset_credit_mode"] = "exhausted"
	w.fetcher = claudeAutoIntegrationUsage{}
	w.scopes.Store("scope", claudeResetScope{model: "claude-sonnet-4", gateway: &GatewayService{}, expires: time.Now().Add(time.Minute)})
	// Native quota is blocked by seven_day, but offered reset only clears five_hour.
	w.scan(context.Background())
	require.Zero(t, count.Load())
}
