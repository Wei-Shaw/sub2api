//go:build unit

package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/stretchr/testify/require"
)

type grokActiveProbeAccountListerStub struct {
	accounts []Account
	calls    chan struct{}
}

func (s *grokActiveProbeAccountListerStub) ListByPlatform(context.Context, string) ([]Account, error) {
	if s.calls != nil {
		select {
		case s.calls <- struct{}{}:
		default:
		}
	}
	return append([]Account(nil), s.accounts...), nil
}

type grokActiveProbeIntegratedRepo struct {
	*grokQuotaAccountRepo
	accounts []Account
}

func (r *grokActiveProbeIntegratedRepo) ListByPlatform(context.Context, string) ([]Account, error) {
	return append([]Account(nil), r.accounts...), nil
}

type grokActiveProberStub struct {
	mu         sync.Mutex
	calls      []int64
	statusCode int
	probeErr   error
}

func (s *grokActiveProberStub) probeUsage(_ context.Context, accountID int64) (*GrokQuotaProbeResult, error) {
	s.mu.Lock()
	s.calls = append(s.calls, accountID)
	s.mu.Unlock()
	return &GrokQuotaProbeResult{StatusCode: s.statusCode}, s.probeErr
}

func (s *grokActiveProberStub) accountIDs() []int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := append([]int64(nil), s.calls...)
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

type grokProbeBlockingLister struct {
	done chan error
}

func (s *grokProbeBlockingLister) ListByPlatform(ctx context.Context, _ string) ([]Account, error) {
	<-ctx.Done()
	s.done <- ctx.Err()
	return nil, ctx.Err()
}

type grokProbeBlockingProber struct {
	done chan error
}

func (s *grokProbeBlockingProber) probeUsage(ctx context.Context, _ int64) (*GrokQuotaProbeResult, error) {
	<-ctx.Done()
	s.done <- ctx.Err()
	return nil, ctx.Err()
}

type grokProbeConcurrencyRecorder struct {
	mu      sync.Mutex
	current int
	maximum int
	started chan struct{}
	release chan struct{}
}

func (s *grokProbeConcurrencyRecorder) probeUsage(ctx context.Context, _ int64) (*GrokQuotaProbeResult, error) {
	s.mu.Lock()
	s.current++
	if s.current > s.maximum {
		s.maximum = s.current
	}
	s.mu.Unlock()
	s.started <- struct{}{}
	select {
	case <-ctx.Done():
	case <-s.release:
	}
	s.mu.Lock()
	s.current--
	s.mu.Unlock()
	return &GrokQuotaProbeResult{StatusCode: http.StatusOK}, nil
}

type grokProbeLeaderLockCache struct {
	mu       sync.Mutex
	keys     []string
	ttls     []time.Duration
	releases []string
}

func (s *grokProbeLeaderLockCache) TryAcquireLeaderLock(_ context.Context, key, _ string, ttl time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.keys = append(s.keys, key)
	s.ttls = append(s.ttls, ttl)
	return true, nil
}

func (s *grokProbeLeaderLockCache) ReleaseLeaderLock(_ context.Context, key, _ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.releases = append(s.releases, key)
	return nil
}

func schedulableGrokOAuthAccount(id int64) Account {
	return Account{
		ID:          id,
		Platform:    PlatformGrok,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
	}
}

func markGrokAccountsDue(svc *GrokActiveProbeService, accounts []Account, now time.Time) {
	for i := range accounts {
		svc.lastAttempts[accounts[i].ID] = now.Add(-svc.interval)
	}
}

func TestGrokActiveProbeAccountEligibilityAndCooldowns(t *testing.T) {
	now := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)
	future := now.Add(time.Hour)
	past := now.Add(-time.Hour)
	accounts := []Account{
		schedulableGrokOAuthAccount(1),
		{ID: 2, Platform: PlatformGrok, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true},
		{ID: 3, Platform: PlatformGrok, Type: AccountTypeOAuth, Status: StatusDisabled, Schedulable: true},
		{ID: 4, Platform: PlatformGrok, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: false},
		schedulableGrokOAuthAccount(5),
		schedulableGrokOAuthAccount(6),
		schedulableGrokOAuthAccount(7),
		schedulableGrokOAuthAccount(8),
		schedulableGrokOAuthAccount(9),
	}
	accounts[4].TempUnschedulableUntil = &future
	accounts[5].RateLimitResetAt = &future
	accounts[6].OverloadUntil = &future
	accounts[7].AutoPauseOnExpired = true
	accounts[7].ExpiresAt = &past
	accounts[8].RateLimitResetAt = &past
	prober := &grokActiveProberStub{}
	svc := newGrokActiveProbeService(&grokActiveProbeAccountListerStub{accounts: accounts}, prober)
	svc.now = func() time.Time { return now }
	markGrokAccountsDue(svc, accounts, now)

	require.NoError(t, svc.RunOnce(context.Background()))
	require.Equal(t, []int64{1, 9}, prober.accountIDs())
}

func TestGrokActiveProbeChecksEachAccountEveryFifteenMinutes(t *testing.T) {
	now := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)
	account := schedulableGrokOAuthAccount(11)
	prober := &grokActiveProberStub{}
	svc := newGrokActiveProbeService(&grokActiveProbeAccountListerStub{accounts: []Account{account}}, prober)
	svc.now = func() time.Time { return now }
	svc.lastAttempts[account.ID] = now.Add(-grokActiveProbeDefaultInterval)

	require.NoError(t, svc.RunOnce(context.Background()))
	now = now.Add(grokActiveProbeDefaultInterval - time.Second)
	require.NoError(t, svc.RunOnce(context.Background()))
	require.Equal(t, []int64{account.ID}, prober.accountIDs())

	now = now.Add(time.Second)
	require.NoError(t, svc.RunOnce(context.Background()))
	require.Equal(t, []int64{account.ID, account.ID}, prober.accountIDs())
}

func TestGrokActiveProbeInitialAccountsAreSpreadAndBudgeted(t *testing.T) {
	now := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)
	accounts := make([]Account, 250)
	for i := range accounts {
		accounts[i] = schedulableGrokOAuthAccount(int64(i + 1))
	}
	prober := &grokActiveProberStub{}
	svc := newGrokActiveProbeService(&grokActiveProbeAccountListerStub{accounts: accounts}, prober)
	svc.now = func() time.Time { return now }
	svc.maxPerScan = 20

	require.NoError(t, svc.RunOnce(context.Background()))
	initial := len(prober.accountIDs())
	require.LessOrEqual(t, initial, 20)
	now = now.Add(grokActiveProbeDefaultInterval / 2)
	require.NoError(t, svc.RunOnce(context.Background()))
	first := len(prober.accountIDs())
	require.Positive(t, first-initial)
	require.LessOrEqual(t, first-initial, 20)
	require.Less(t, first, len(accounts))

	require.NoError(t, svc.RunOnce(context.Background()))
	second := len(prober.accountIDs())
	require.Greater(t, second, first)
	require.LessOrEqual(t, second, initial+40)
}

func TestGrokActiveProbeInitialDueTimeSurvivesIntervalBoundary(t *testing.T) {
	now := time.Date(2026, 7, 22, 12, 14, 50, 0, time.UTC)
	account := schedulableGrokOAuthAccount(77)
	svc := newGrokActiveProbeService(nil, nil)

	require.False(t, svc.probeDue(&account, now))
	dueAt := svc.initialDueAt[account.ID]
	require.True(t, dueAt.After(now))

	now = dueAt.Add(time.Second)
	require.True(t, svc.probeDue(&account, now))
	require.Equal(t, dueAt, svc.initialDueAt[account.ID])
}

func TestGrokActiveProbeGlobalScanBudget(t *testing.T) {
	now := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)
	accounts := make([]Account, 25)
	for i := range accounts {
		accounts[i] = schedulableGrokOAuthAccount(int64(i + 101))
	}
	prober := &grokActiveProberStub{}
	svc := newGrokActiveProbeService(&grokActiveProbeAccountListerStub{accounts: accounts}, prober)
	svc.now = func() time.Time { return now }
	svc.maxPerScan = 7
	markGrokAccountsDue(svc, accounts, now)

	require.NoError(t, svc.RunOnce(context.Background()))
	require.Len(t, prober.accountIDs(), 7)
}

func TestGrokActiveProbeConcurrencyIsBoundedToFour(t *testing.T) {
	now := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)
	accounts := make([]Account, 8)
	for i := range accounts {
		accounts[i] = schedulableGrokOAuthAccount(int64(i + 201))
	}
	prober := &grokProbeConcurrencyRecorder{
		started: make(chan struct{}, len(accounts)),
		release: make(chan struct{}),
	}
	svc := newGrokActiveProbeService(&grokActiveProbeAccountListerStub{accounts: accounts}, prober)
	svc.now = func() time.Time { return now }
	markGrokAccountsDue(svc, accounts, now)
	done := make(chan error, 1)
	go func() { done <- svc.RunOnce(context.Background()) }()

	for range grokActiveProbeConcurrency {
		select {
		case <-prober.started:
		case <-time.After(time.Second):
			t.Fatal("expected four concurrent probes")
		}
	}
	select {
	case <-prober.started:
		t.Fatal("active probe exceeded the concurrency limit")
	case <-time.After(50 * time.Millisecond):
	}
	close(prober.release)
	require.NoError(t, <-done)
	prober.mu.Lock()
	defer prober.mu.Unlock()
	require.Equal(t, grokActiveProbeConcurrency, prober.maximum)
}

func TestGrokActiveProbeAutomaticallyAppliesUpstreamStatusPolicy(t *testing.T) {
	for _, tt := range []struct {
		status        int
		wantPermanent int
		wantTemporary int
	}{
		{status: http.StatusPaymentRequired, wantPermanent: 1},
		{status: http.StatusForbidden, wantPermanent: 1},
		{status: http.StatusBadGateway, wantTemporary: 1},
	} {
		t.Run(http.StatusText(tt.status), func(t *testing.T) {
			account := healthyGrokQuotaOAuthAccount(int64(1000 + tt.status))
			baseRepo := &grokQuotaAccountRepo{
				mockAccountRepoForPlatform: &mockAccountRepoForPlatform{accountsByID: map[int64]*Account{account.ID: account}},
				paymentRequiredResult:      true,
				probeTempResult:            true,
			}
			repo := &grokActiveProbeIntegratedRepo{grokQuotaAccountRepo: baseRepo, accounts: []Account{*account}}
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: tt.status,
				Header:     http.Header{},
				Body:       io.NopCloser(strings.NewReader("upstream failure")),
			}}
			quotaService := NewGrokQuotaService(repo, nil, NewGrokTokenProvider(repo, nil), upstream, nil)
			svc := newGrokActiveProbeService(repo, quotaService)
			now := time.Now()
			svc.now = func() time.Time { return now }
			svc.lastAttempts[account.ID] = now.Add(-svc.interval)

			require.NoError(t, svc.RunOnce(context.Background()))
			require.Equal(t, tt.wantPermanent, baseRepo.paymentRequiredCalls)
			require.Equal(t, tt.wantTemporary, baseRepo.probeTempCalls)
			stored, ok := baseRepo.updates[account.ID][grokQuotaSnapshotExtraKey].(*xai.QuotaSnapshot)
			require.True(t, ok)
			require.Equal(t, tt.status, stored.StatusCode)
		})
	}
}

func TestGrokActiveProbeRetainsThrottleAfterFailure(t *testing.T) {
	now := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)
	account := schedulableGrokOAuthAccount(21)
	prober := &grokActiveProberStub{statusCode: http.StatusBadGateway, probeErr: errors.New("upstream returned 502")}
	svc := newGrokActiveProbeService(&grokActiveProbeAccountListerStub{accounts: []Account{account}}, prober)
	svc.now = func() time.Time { return now }
	svc.lastAttempts[account.ID] = now.Add(-svc.interval)

	require.NoError(t, svc.RunOnce(context.Background()))
	require.NoError(t, svc.RunOnce(context.Background()))
	require.Equal(t, []int64{account.ID}, prober.accountIDs())
}

func TestGrokActiveProbeEnvironmentConfigurationAndFallback(t *testing.T) {
	t.Setenv(grokActiveProbeEnabledEnv, "false")
	t.Setenv(grokActiveProbeIntervalEnv, "45m")
	t.Setenv(grokActiveProbeMaxPerScanEnv, "23")
	svc := newGrokActiveProbeService(nil, nil)
	require.Equal(t, 45*time.Minute, svc.interval)
	require.Equal(t, 23, svc.maxPerScan)
	require.False(t, svc.enabled)

	t.Setenv(grokActiveProbeEnabledEnv, "invalid")
	t.Setenv(grokActiveProbeIntervalEnv, "invalid")
	for _, value := range []string{"181", "1000"} {
		t.Setenv(grokActiveProbeMaxPerScanEnv, value)
		svc = newGrokActiveProbeService(nil, nil)
		require.Equal(t, grokActiveProbeDefaultInterval, svc.interval)
		require.Equal(t, grokActiveProbeDefaultMaxPerScan, svc.maxPerScan)
		require.True(t, svc.enabled)
	}

	t.Setenv(grokActiveProbeIntervalEnv, "5m")
	svc = newGrokActiveProbeService(nil, nil)
	require.Equal(t, grokActiveProbeDefaultInterval, svc.interval)
}

func TestGrokActiveProbeDefaults(t *testing.T) {
	svc := newGrokActiveProbeService(nil, nil)
	require.True(t, svc.enabled)
	require.Equal(t, 15*time.Minute, svc.interval)
	require.Equal(t, 180, svc.maxPerScan)
}

func TestGrokActiveProbeTimeoutBudgetFitsLeaderLock(t *testing.T) {
	waves := (grokActiveProbeMaxPerScanLimit + grokActiveProbeConcurrency - 1) / grokActiveProbeConcurrency
	worstWorkerDuration := time.Duration(waves)*grokActiveProbeAccountTimeout + grokActiveProbeScanInterval
	require.Less(t, worstWorkerDuration, grokActiveProbeRunTimeout)
	require.Less(t, grokActiveProbeRunTimeout, grokActiveProbeRunLockTTL)
}

func TestGrokActiveProbeUsesBoundedRunAndAccountContexts(t *testing.T) {
	t.Run("run deadline covers account loading", func(t *testing.T) {
		lister := &grokProbeBlockingLister{done: make(chan error, 1)}
		svc := newGrokActiveProbeService(lister, &grokActiveProberStub{})
		svc.runTimeout = 20 * time.Millisecond

		err := svc.RunOnce(context.Background())
		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.ErrorIs(t, <-lister.done, context.DeadlineExceeded)
	})

	t.Run("account deadline cancels a blocked probe", func(t *testing.T) {
		now := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)
		account := schedulableGrokOAuthAccount(90)
		prober := &grokProbeBlockingProber{done: make(chan error, 1)}
		svc := newGrokActiveProbeService(&grokActiveProbeAccountListerStub{accounts: []Account{account}}, prober)
		svc.now = func() time.Time { return now }
		svc.lastAttempts[account.ID] = now.Add(-svc.interval)
		svc.accountTimeout = 20 * time.Millisecond
		svc.runTimeout = time.Second

		require.NoError(t, svc.RunOnce(context.Background()))
		require.ErrorIs(t, <-prober.done, context.DeadlineExceeded)
	})
}

func TestGrokActiveProbeAcquiresRunLockWithFullCleanupBudget(t *testing.T) {
	lockCache := &grokProbeLeaderLockCache{}
	svc := newGrokActiveProbeService(&grokActiveProbeAccountListerStub{}, &grokActiveProberStub{})
	svc.lockCache = lockCache

	require.NoError(t, svc.RunOnce(context.Background()))
	lockCache.mu.Lock()
	defer lockCache.mu.Unlock()
	require.NotEmpty(t, lockCache.keys)
	require.Equal(t, grokActiveProbeRunLockKey, lockCache.keys[0])
	require.Equal(t, grokActiveProbeRunLockTTL, lockCache.ttls[0])
	require.Contains(t, lockCache.releases, grokActiveProbeRunLockKey)
}

func TestGrokActiveProbeDisabledDoesNotStart(t *testing.T) {
	lister := &grokActiveProbeAccountListerStub{calls: make(chan struct{}, 1)}
	svc := newGrokActiveProbeService(lister, &grokActiveProberStub{})
	svc.enabled = false

	svc.Start()
	select {
	case <-lister.calls:
		t.Fatal("disabled active probe unexpectedly started")
	case <-time.After(50 * time.Millisecond):
	}
	require.NotPanics(t, svc.Stop)
}

func TestGrokActiveProbeUsesSnapshotUpdatedAtWhenLastProbeAtMissing(t *testing.T) {
	updatedAt := "2026-07-22T12:00:00Z"
	account := schedulableGrokOAuthAccount(55)
	account.Extra = map[string]any{grokQuotaSnapshotExtraKey: &xai.QuotaSnapshot{UpdatedAt: updatedAt}}

	require.Equal(t, time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC), grokActiveProbePersistedAt(&account))
}

func TestGrokActiveProbeStartAndStopAreIdempotent(t *testing.T) {
	lister := &grokActiveProbeAccountListerStub{calls: make(chan struct{}, 1)}
	svc := newGrokActiveProbeService(lister, &grokActiveProberStub{})

	svc.Start()
	svc.Start()
	select {
	case <-lister.calls:
	case <-time.After(time.Second):
		t.Fatal("initial active probe scan did not start")
	}
	require.NotPanics(t, svc.Stop)
	require.NotPanics(t, svc.Stop)
}
