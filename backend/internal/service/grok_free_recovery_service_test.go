//go:build unit

package service

import (
	"context"
	"errors"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/stretchr/testify/require"
)

type atomicGrokFreeRecoveryStoreStub struct {
	*grokFreeRecoveryStoreStub
	pendingClaimCalls  int
	proactiveListCalls int
	maxPageRequested   int
}

func (s *atomicGrokFreeRecoveryStoreStub) ClaimDueGrokFreeRecoveryCandidates(
	_ context.Context,
	now, nextProbeAt, leaseUntil time.Time,
	limit int,
) ([]Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pendingClaimCalls++
	if limit > s.maxPageRequested {
		s.maxPageRequested = limit
	}
	ids := make([]int64, 0, len(s.accounts))
	for id, account := range s.accounts {
		if grokFreeRecoveryCandidate(&account, now) {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	if len(ids) > limit {
		ids = ids[:limit]
	}
	out := make([]Account, 0, len(ids))
	for _, id := range ids {
		account := s.accounts[id]
		if account.Extra == nil {
			account.Extra = map[string]any{}
		}
		account.Extra[GrokFreeRecoveryNextProbeAtExtraKey] = nextProbeAt.Format(time.RFC3339Nano)
		account.Extra[GrokFreeRecoveryLastProbeAtExtraKey] = now.Format(time.RFC3339Nano)
		account.Extra[GrokFreeRecoveryLastProbeResultExtraKey] = "running"
		account.RateLimitResetAt = &leaseUntil
		s.accounts[id] = account
		out = append(out, account)
	}
	return out, nil
}

func (s *atomicGrokFreeRecoveryStoreStub) ListGrokFreeProactiveCandidates(
	_ context.Context,
	now time.Time,
	afterID int64,
	limit int,
) ([]Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.proactiveListCalls++
	if limit > s.maxPageRequested {
		s.maxPageRequested = limit
	}
	ids := make([]int64, 0)
	for id, account := range s.accounts {
		if id > afterID && grokFreeProactiveUsageCandidate(&account, now) {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	if len(ids) > limit {
		ids = ids[:limit]
	}
	out := make([]Account, 0, len(ids))
	for _, id := range ids {
		out = append(out, s.accounts[id])
	}
	return out, nil
}

func (s *atomicGrokFreeRecoveryStoreStub) ClaimGrokFreeProactiveCandidates(
	_ context.Context,
	ids []int64,
	now, nextProbeAt, leaseUntil time.Time,
) ([]Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Account, 0, len(ids))
	for _, id := range ids {
		account, ok := s.accounts[id]
		if !ok || !grokFreeProactiveUsageCandidate(&account, now) {
			continue
		}
		if account.Extra == nil {
			account.Extra = map[string]any{}
		}
		account.Extra[GrokFreeRecoveryPendingExtraKey] = true
		account.Extra[GrokFreeRecoveryNextProbeAtExtraKey] = nextProbeAt.Format(time.RFC3339Nano)
		account.Extra[GrokFreeRecoveryLastProbeAtExtraKey] = now.Format(time.RFC3339Nano)
		account.Extra[GrokFreeRecoveryLastProbeResultExtraKey] = "running"
		account.Extra[GrokFreeProactiveNextProbeAtExtraKey] = nextProbeAt.Format(time.RFC3339Nano)
		account.RateLimitResetAt = &leaseUntil
		s.accounts[id] = account
		out = append(out, account)
	}
	return out, nil
}

func (s *atomicGrokFreeRecoveryStoreStub) RecordGrokFreeRecoveryProbeResult(
	_ context.Context,
	id int64,
	expectedNextProbeAt time.Time,
	result string,
	completedAt time.Time,
) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	account, ok := s.accounts[id]
	if !ok || !account.GrokFreeRecoveryNextProbeAt().Equal(expectedNextProbeAt) {
		return false, nil
	}
	account.Extra[GrokFreeRecoveryLastProbeResultExtraKey] = result
	account.Extra[GrokFreeRecoveryLastResultAtExtraKey] = completedAt.Format(time.RFC3339Nano)
	s.accounts[id] = account
	return true, nil
}

type grokFreeRecoveryStoreStub struct {
	mu        sync.Mutex
	accounts  map[int64]Account
	rearms    int
	updates   int
	updateErr error
	events    []string
}

func (s *grokFreeRecoveryStoreStub) ListByPlatform(context.Context, string) ([]Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Account, 0, len(s.accounts))
	for _, account := range s.accounts {
		out = append(out, account)
	}
	return out, nil
}

func (s *grokFreeRecoveryStoreStub) GetByID(_ context.Context, id int64) (*Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	account, ok := s.accounts[id]
	if !ok {
		return nil, ErrAccountNotFound
	}
	clone := account
	return &clone, nil
}

func (s *grokFreeRecoveryStoreStub) SetRateLimited(_ context.Context, id int64, resetAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, "lease")
	account := s.accounts[id]
	now := time.Now()
	account.RateLimitedAt = &now
	account.RateLimitResetAt = &resetAt
	s.accounts[id] = account
	s.rearms++
	return nil
}

func (s *grokFreeRecoveryStoreStub) SetRateLimitedIfLater(_ context.Context, id int64, resetAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, "lease")
	account := s.accounts[id]
	if account.RateLimitResetAt == nil || resetAt.After(*account.RateLimitResetAt) {
		now := time.Now()
		account.RateLimitedAt = &now
		account.RateLimitResetAt = &resetAt
		s.accounts[id] = account
	}
	s.rearms++
	return nil
}

func (s *grokFreeRecoveryStoreStub) UpdateExtra(_ context.Context, id int64, updates map[string]any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, "latch")
	if s.updateErr != nil {
		return s.updateErr
	}
	account := s.accounts[id]
	if account.Extra == nil {
		account.Extra = map[string]any{}
	}
	for key, value := range updates {
		account.Extra[key] = value
	}
	s.accounts[id] = account
	s.updates++
	return nil
}

type grokFreeRecoveryProberFunc func(context.Context, int64) (*GrokQuotaProbeResult, error)

func (f grokFreeRecoveryProberFunc) probeUsage(ctx context.Context, id int64) (*GrokQuotaProbeResult, error) {
	return f(ctx, id)
}

type grokFreeRecoveryUsageProberStub struct {
	mu         sync.Mutex
	usage      map[int64]*WindowStats
	result     *GrokQuotaProbeResult
	probeErr   error
	usageErr   error
	probeCalls []int64
	usageCalls int
	queriedIDs []int64
}

func (s *grokFreeRecoveryUsageProberStub) probeUsage(_ context.Context, id int64) (*GrokQuotaProbeResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.probeCalls = append(s.probeCalls, id)
	return s.result, s.probeErr
}

func (s *grokFreeRecoveryUsageProberStub) grokFreeRollingUsage(
	_ context.Context,
	accountIDs []int64,
	_ time.Time,
) (map[int64]*WindowStats, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.usageCalls++
	s.queriedIDs = append([]int64(nil), accountIDs...)
	if s.usageErr != nil {
		return nil, s.usageErr
	}
	result := make(map[int64]*WindowStats, len(s.usage))
	for id, usage := range s.usage {
		result[id] = usage
	}
	return result, nil
}

type grokFreeRecoveryRecovererStub struct {
	mu               sync.Mutex
	calls            []int64
	recoverResult    bool
	recoverResultSet bool
}

func (s *grokFreeRecoveryRecovererStub) RecoverGrokFreeAfterSuccessfulProbe(_ context.Context, id int64, _, _ time.Time) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, id)
	if s.recoverResultSet {
		return s.recoverResult, nil
	}
	return true, nil
}

func pendingGrokRecoveryAccount(id int64) Account {
	limitedAt := time.Now().Add(-time.Hour)
	return Account{
		ID:            id,
		Platform:      PlatformGrok,
		Type:          AccountTypeOAuth,
		Status:        StatusActive,
		Schedulable:   true,
		RateLimitedAt: &limitedAt,
		Extra: map[string]any{
			GrokFreeRecoveryPendingExtraKey: true,
		},
	}
}

func newGrokFreeRecoveryServiceForTest(
	store *grokFreeRecoveryStoreStub,
	prober grokFreeRecoveryProber,
	recoverer *grokFreeRecoveryRecovererStub,
) *GrokFreeRecoveryService {
	svc := newGrokFreeRecoveryService(store, prober, recoverer, nil, nil)
	svc.now = time.Now
	return svc
}

func TestGrokFreeRecoveryServiceRecoversOnlyAfterHealthyProbe(t *testing.T) {
	store := &grokFreeRecoveryStoreStub{accounts: map[int64]Account{1: pendingGrokRecoveryAccount(1)}}
	recoverer := &grokFreeRecoveryRecovererStub{}
	svc := newGrokFreeRecoveryServiceForTest(store, grokFreeRecoveryProberFunc(func(context.Context, int64) (*GrokQuotaProbeResult, error) {
		return &GrokQuotaProbeResult{StatusCode: 200, Snapshot: &xai.QuotaSnapshot{StatusCode: 200}}, nil
	}), recoverer)

	svc.runCycle(context.Background())

	require.Equal(t, []int64{1}, recoverer.calls)
	require.Equal(t, 1, store.rearms)
	require.Equal(t, 2, store.updates)
}

func TestGrokFreeRecoveryServiceKeepsFailuresPending(t *testing.T) {
	zero := int64(0)
	tests := []struct {
		name   string
		result *GrokQuotaProbeResult
		err    error
	}{
		{name: "429", result: &GrokQuotaProbeResult{StatusCode: 429, Snapshot: &xai.QuotaSnapshot{StatusCode: 429}}},
		{name: "transport error", err: errors.New("network down")},
		{name: "200 but exhausted", result: &GrokQuotaProbeResult{StatusCode: 200, Snapshot: &xai.QuotaSnapshot{StatusCode: 200, Tokens: &xai.QuotaWindow{Remaining: &zero}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &grokFreeRecoveryStoreStub{accounts: map[int64]Account{2: pendingGrokRecoveryAccount(2)}}
			recoverer := &grokFreeRecoveryRecovererStub{}
			svc := newGrokFreeRecoveryServiceForTest(store, grokFreeRecoveryProberFunc(func(context.Context, int64) (*GrokQuotaProbeResult, error) {
				return tt.result, tt.err
			}), recoverer)

			svc.runCycle(context.Background())

			require.Empty(t, recoverer.calls)
			account, err := store.GetByID(context.Background(), 2)
			require.NoError(t, err)
			require.True(t, account.IsGrokFreeRecoveryPending())
			require.True(t, account.GrokFreeRecoveryNextProbeAt().After(time.Now()))
		})
	}
}

func TestGrokFreeRecoveryServiceLegacyPathPersistsLatchBeforeLease(t *testing.T) {
	t.Run("latch precedes lease", func(t *testing.T) {
		store := &grokFreeRecoveryStoreStub{accounts: map[int64]Account{21: pendingGrokRecoveryAccount(21)}}
		svc := newGrokFreeRecoveryServiceForTest(store, grokFreeRecoveryProberFunc(func(context.Context, int64) (*GrokQuotaProbeResult, error) {
			return &GrokQuotaProbeResult{StatusCode: 429, Snapshot: &xai.QuotaSnapshot{StatusCode: 429}}, nil
		}), &grokFreeRecoveryRecovererStub{})

		svc.runCycle(context.Background())

		require.GreaterOrEqual(t, len(store.events), 2)
		require.Equal(t, []string{"latch", "lease"}, store.events[:2])
	})

	t.Run("latch failure does not write lease or probe", func(t *testing.T) {
		probeCalls := 0
		store := &grokFreeRecoveryStoreStub{
			accounts:  map[int64]Account{22: pendingGrokRecoveryAccount(22)},
			updateErr: errors.New("latch write failed"),
		}
		svc := newGrokFreeRecoveryServiceForTest(store, grokFreeRecoveryProberFunc(func(context.Context, int64) (*GrokQuotaProbeResult, error) {
			probeCalls++
			return nil, nil
		}), &grokFreeRecoveryRecovererStub{})

		svc.runCycle(context.Background())

		require.Equal(t, []string{"latch"}, store.events)
		require.Zero(t, store.rearms)
		require.Zero(t, probeCalls)
	})
}

func TestGrokFreeRecoveryServiceHonorsNextProbeAndCASFailure(t *testing.T) {
	t.Run("future next probe is skipped", func(t *testing.T) {
		account := pendingGrokRecoveryAccount(3)
		account.Extra[GrokFreeRecoveryNextProbeAtExtraKey] = time.Now().Add(time.Minute).UTC().Format(time.RFC3339Nano)
		store := &grokFreeRecoveryStoreStub{accounts: map[int64]Account{3: account}}
		recoverer := &grokFreeRecoveryRecovererStub{}
		svc := newGrokFreeRecoveryServiceForTest(store, grokFreeRecoveryProberFunc(func(context.Context, int64) (*GrokQuotaProbeResult, error) {
			t.Fatal("probe should not run before next_probe_at")
			return nil, nil
		}), recoverer)

		svc.runCycle(context.Background())

		require.Zero(t, store.rearms)
		require.Empty(t, recoverer.calls)
	})

	t.Run("CAS failure does not report recovery", func(t *testing.T) {
		store := &grokFreeRecoveryStoreStub{accounts: map[int64]Account{4: pendingGrokRecoveryAccount(4)}}
		recoverer := &grokFreeRecoveryRecovererStub{recoverResultSet: true, recoverResult: false}
		svc := newGrokFreeRecoveryServiceForTest(store, grokFreeRecoveryProberFunc(func(context.Context, int64) (*GrokQuotaProbeResult, error) {
			return &GrokQuotaProbeResult{StatusCode: 200, Snapshot: &xai.QuotaSnapshot{StatusCode: 200}}, nil
		}), recoverer)

		svc.runCycle(context.Background())

		require.Equal(t, []int64{4}, recoverer.calls)
		account, err := store.GetByID(context.Background(), 4)
		require.NoError(t, err)
		require.True(t, account.IsGrokFreeRecoveryPending())
	})
}

func TestGrokFreeRecoveryServiceProactivelyProbesRollingUsageAtLimit(t *testing.T) {
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	account := Account{
		ID:          5,
		Platform:    PlatformGrok,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Extra:       map[string]any{},
	}
	store := &grokFreeRecoveryStoreStub{accounts: map[int64]Account{account.ID: account}}
	prober := &grokFreeRecoveryUsageProberStub{
		usage: map[int64]*WindowStats{
			account.ID: {Tokens: grokFreeProactiveProbeTokenThreshold},
		},
		result: &GrokQuotaProbeResult{
			StatusCode: 429,
			Snapshot:   &xai.QuotaSnapshot{StatusCode: 429},
		},
	}
	recoverer := &grokFreeRecoveryRecovererStub{}
	svc := newGrokFreeRecoveryServiceForTest(store, prober, recoverer)
	svc.now = func() time.Time { return now }

	svc.runCycle(context.Background())

	require.Equal(t, []int64{account.ID}, prober.probeCalls)
	require.Equal(t, 1, prober.usageCalls)
	require.Equal(t, []int64{account.ID}, prober.queriedIDs)
	require.Equal(t, 1, store.rearms)
	require.Equal(t, 2, store.updates)
	require.Empty(t, recoverer.calls)

	updated, err := store.GetByID(context.Background(), account.ID)
	require.NoError(t, err)
	require.True(t, updated.IsGrokFreeRecoveryPending())
	require.Equal(t, now.Add(grokFreeRecoveryProbeInterval), updated.GrokFreeRecoveryNextProbeAt())
	require.Equal(
		t,
		now.Add(grokFreeRecoveryProbeInterval),
		updated.getExtraTime(GrokFreeProactiveNextProbeAtExtraKey),
	)
}

func TestGrokFreeRecoveryServiceProactiveScanFiltersUsageAndCooldown(t *testing.T) {
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	belowLimit := Account{
		ID:          6,
		Platform:    PlatformGrok,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Extra:       map[string]any{},
	}
	paid := belowLimit
	paid.ID = 7
	paid.Credentials = map[string]any{"subscription_tier": "supergrok"}
	coolingDown := belowLimit
	coolingDown.ID = 8
	coolingDown.Extra = map[string]any{
		GrokFreeProactiveNextProbeAtExtraKey: now.Add(time.Minute).Format(time.RFC3339Nano),
	}
	store := &grokFreeRecoveryStoreStub{accounts: map[int64]Account{
		belowLimit.ID:  belowLimit,
		paid.ID:        paid,
		coolingDown.ID: coolingDown,
	}}
	prober := &grokFreeRecoveryUsageProberStub{
		usage: map[int64]*WindowStats{
			belowLimit.ID:  {Tokens: grokFreeProactiveProbeTokenThreshold - 1},
			paid.ID:        {Tokens: grokFreeRolling24hTokenLimit},
			coolingDown.ID: {Tokens: grokFreeRolling24hTokenLimit},
		},
		result: &GrokQuotaProbeResult{StatusCode: 200, Snapshot: &xai.QuotaSnapshot{StatusCode: 200}},
	}
	recoverer := &grokFreeRecoveryRecovererStub{}
	svc := newGrokFreeRecoveryServiceForTest(store, prober, recoverer)
	svc.now = func() time.Time { return now }

	svc.runCycle(context.Background())

	require.Equal(t, 1, prober.usageCalls)
	require.Equal(t, []int64{belowLimit.ID}, prober.queriedIDs)
	require.Empty(t, prober.probeCalls)
	require.Zero(t, store.rearms)
	require.Zero(t, store.updates)
	require.Empty(t, recoverer.calls)
}

func TestGrokFreeRecoveryServiceAtomicClaimPreventsDuplicateMultiInstanceProbe(t *testing.T) {
	now := time.Date(2026, 7, 18, 2, 0, 0, 0, time.UTC)
	base := &grokFreeRecoveryStoreStub{accounts: map[int64]Account{90: pendingGrokRecoveryAccount(90)}}
	store := &atomicGrokFreeRecoveryStoreStub{grokFreeRecoveryStoreStub: base}
	var mu sync.Mutex
	probeCalls := 0
	prober := grokFreeRecoveryProberFunc(func(context.Context, int64) (*GrokQuotaProbeResult, error) {
		mu.Lock()
		probeCalls++
		mu.Unlock()
		return &GrokQuotaProbeResult{StatusCode: 429, Snapshot: &xai.QuotaSnapshot{StatusCode: 429}}, nil
	})
	recoverer := &grokFreeRecoveryRecovererStub{}
	first := newGrokFreeRecoveryService(store, prober, recoverer, nil, nil)
	second := newGrokFreeRecoveryService(store, prober, recoverer, nil, nil)
	first.now = func() time.Time { return now }
	second.now = func() time.Time { return now }

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); first.runCycle(context.Background()) }()
	go func() { defer wg.Done(); second.runCycle(context.Background()) }()
	wg.Wait()

	mu.Lock()
	require.Equal(t, 1, probeCalls)
	mu.Unlock()
	require.Equal(t, int64(1), first.Metrics().Probes+second.Metrics().Probes)
}

func TestGrokFreeRecoveryServicePrioritizesPendingBeforeProactive(t *testing.T) {
	now := time.Date(2026, 7, 18, 2, 0, 0, 0, time.UTC)
	proactive := Account{
		ID: 92, Platform: PlatformGrok, Type: AccountTypeOAuth,
		Status: StatusActive, Schedulable: true, Extra: map[string]any{},
	}
	base := &grokFreeRecoveryStoreStub{accounts: map[int64]Account{
		91: pendingGrokRecoveryAccount(91),
		92: proactive,
	}}
	store := &atomicGrokFreeRecoveryStoreStub{grokFreeRecoveryStoreStub: base}
	prober := &grokFreeRecoveryUsageProberStub{
		usage:  map[int64]*WindowStats{92: {Tokens: grokFreeProactiveProbeTokenThreshold}},
		result: &GrokQuotaProbeResult{StatusCode: 429, Snapshot: &xai.QuotaSnapshot{StatusCode: 429}},
	}
	svc := newGrokFreeRecoveryService(store, prober, &grokFreeRecoveryRecovererStub{}, nil, nil)
	svc.now = func() time.Time { return now }
	svc.maxCandidatesPerCycle = 1

	svc.runCycle(context.Background())

	require.Equal(t, []int64{91}, prober.probeCalls)
	require.Zero(t, store.proactiveListCalls)
	require.Equal(t, int64(1), svc.Metrics().PendingClaimed)
	require.Zero(t, svc.Metrics().ProactiveClaimed)
}

func TestGrokFreeRecoveryServicePaginatesLargePoolWithinConfiguredCapacity(t *testing.T) {
	now := time.Date(2026, 7, 18, 2, 0, 0, 0, time.UTC)
	const poolSize = 5000
	accounts := make(map[int64]Account, poolSize)
	usage := make(map[int64]*WindowStats, poolSize)
	for id := int64(1); id <= poolSize; id++ {
		accounts[id] = Account{
			ID: id, Platform: PlatformGrok, Type: AccountTypeOAuth,
			Status: StatusActive, Schedulable: true, Extra: map[string]any{},
		}
		usage[id] = &WindowStats{Tokens: 1}
	}
	usage[poolSize] = &WindowStats{Tokens: grokFreeProactiveProbeTokenThreshold}
	store := &atomicGrokFreeRecoveryStoreStub{
		grokFreeRecoveryStoreStub: &grokFreeRecoveryStoreStub{accounts: accounts},
	}
	prober := &grokFreeRecoveryUsageProberStub{
		usage:  usage,
		result: &GrokQuotaProbeResult{StatusCode: 429, Snapshot: &xai.QuotaSnapshot{StatusCode: 429}},
	}
	svc := newGrokFreeRecoveryService(store, prober, &grokFreeRecoveryRecovererStub{}, nil, nil)
	svc.now = func() time.Time { return now }
	svc.candidatePageSize = 128
	svc.maxCandidatesPerCycle = 256

	for range (poolSize + svc.maxCandidatesPerCycle - 1) / svc.maxCandidatesPerCycle {
		svc.runCycle(context.Background())
	}

	require.LessOrEqual(t, store.maxPageRequested, 128)
	require.Greater(t, store.proactiveListCalls, 1)
	require.Equal(t, int64(poolSize), svc.Metrics().ProactiveScanned)
	require.Equal(t, int64(1), svc.Metrics().ProactiveClaimed)
	require.Equal(t, []int64{poolSize}, prober.probeCalls)
}

func TestGrokFreeRecoveryServiceFeatureFlagDisablesWorkerAndCycles(t *testing.T) {
	store := &grokFreeRecoveryStoreStub{accounts: map[int64]Account{93: pendingGrokRecoveryAccount(93)}}
	probes := 0
	svc := newGrokFreeRecoveryService(store, grokFreeRecoveryProberFunc(func(context.Context, int64) (*GrokQuotaProbeResult, error) {
		probes++
		return nil, nil
	}), &grokFreeRecoveryRecovererStub{}, nil, nil)
	svc.configure(false, time.Second, time.Minute, time.Minute, 10, 10, 1)

	svc.Start()
	svc.runCycle(context.Background())
	svc.Stop()

	require.Zero(t, probes)
	require.Zero(t, svc.Metrics().Cycles)
}
