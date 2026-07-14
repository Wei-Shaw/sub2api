//go:build unit

package service

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/stretchr/testify/require"
)

type grokFreeRecoveryStoreStub struct {
	mu       sync.Mutex
	accounts map[int64]Account
	rearms   int
	updates  int
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

func (s *grokFreeRecoveryStoreStub) markNewer429(id int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	account := s.accounts[id]
	now := time.Now().Add(time.Second)
	account.RateLimitedAt = &now
	s.accounts[id] = account
}

type grokFreeRecoveryProberFunc func(context.Context, int64) (*GrokQuotaProbeResult, error)

func (f grokFreeRecoveryProberFunc) probeUsage(ctx context.Context, id int64) (*GrokQuotaProbeResult, error) {
	return f(ctx, id)
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
	service := newGrokFreeRecoveryService(store, prober, recoverer, nil, nil)
	service.now = time.Now
	return service
}

func TestGrokFreeRecoveryServiceRecoversOnlyAfterHealthyProbe(t *testing.T) {
	store := &grokFreeRecoveryStoreStub{accounts: map[int64]Account{1: pendingGrokRecoveryAccount(1)}}
	recoverer := &grokFreeRecoveryRecovererStub{}
	service := newGrokFreeRecoveryServiceForTest(store, grokFreeRecoveryProberFunc(func(context.Context, int64) (*GrokQuotaProbeResult, error) {
		return &GrokQuotaProbeResult{StatusCode: 200, Snapshot: &xai.QuotaSnapshot{StatusCode: 200}}, nil
	}), recoverer)

	service.runCycle(context.Background())

	require.Equal(t, []int64{1}, recoverer.calls)
	require.Equal(t, 1, store.rearms)
	require.Equal(t, 1, store.updates)
}

func TestGrokFreeRecoveryServiceKeeps429AndExhaustedAccountsPending(t *testing.T) {
	zero := int64(0)
	tests := []struct {
		name   string
		result *GrokQuotaProbeResult
		err    error
	}{
		{name: "429", result: &GrokQuotaProbeResult{StatusCode: 429, Snapshot: &xai.QuotaSnapshot{StatusCode: 429}}},
		{name: "transport error", err: errors.New("network down")},
		{name: "200 but quota remains exhausted", result: &GrokQuotaProbeResult{StatusCode: 200, Snapshot: &xai.QuotaSnapshot{StatusCode: 200, Tokens: &xai.QuotaWindow{Remaining: &zero}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &grokFreeRecoveryStoreStub{accounts: map[int64]Account{2: pendingGrokRecoveryAccount(2)}}
			recoverer := &grokFreeRecoveryRecovererStub{}
			service := newGrokFreeRecoveryServiceForTest(store, grokFreeRecoveryProberFunc(func(context.Context, int64) (*GrokQuotaProbeResult, error) {
				return tt.result, tt.err
			}), recoverer)

			service.runCycle(context.Background())

			require.Empty(t, recoverer.calls)
			require.Equal(t, 1, store.rearms)
			account, getErr := store.GetByID(context.Background(), 2)
			require.NoError(t, getErr)
			require.True(t, account.IsGrokFreeRecoveryPending())
			require.True(t, account.GrokFreeRecoveryNextProbeAt().After(time.Now()))
		})
	}
}

func TestGrokFreeRecoveryServiceHonorsNextProbeAndNewer429(t *testing.T) {
	t.Run("future next probe is skipped", func(t *testing.T) {
		account := pendingGrokRecoveryAccount(3)
		account.Extra[GrokFreeRecoveryNextProbeAtExtraKey] = time.Now().Add(time.Minute).UTC().Format(time.RFC3339Nano)
		store := &grokFreeRecoveryStoreStub{accounts: map[int64]Account{3: account}}
		recoverer := &grokFreeRecoveryRecovererStub{}
		service := newGrokFreeRecoveryServiceForTest(store, grokFreeRecoveryProberFunc(func(context.Context, int64) (*GrokQuotaProbeResult, error) {
			t.Fatal("probe should not run before next_probe_at")
			return nil, nil
		}), recoverer)

		service.runCycle(context.Background())

		require.Zero(t, store.rearms)
		require.Empty(t, recoverer.calls)
	})

	t.Run("newer 429 wins over probe success", func(t *testing.T) {
		store := &grokFreeRecoveryStoreStub{accounts: map[int64]Account{4: pendingGrokRecoveryAccount(4)}}
		recoverer := &grokFreeRecoveryRecovererStub{recoverResultSet: true}
		service := newGrokFreeRecoveryServiceForTest(store, grokFreeRecoveryProberFunc(func(context.Context, int64) (*GrokQuotaProbeResult, error) {
			store.markNewer429(4)
			return &GrokQuotaProbeResult{StatusCode: 200, Snapshot: &xai.QuotaSnapshot{StatusCode: 200}}, nil
		}), recoverer)

		service.runCycle(context.Background())

		require.Equal(t, []int64{4}, recoverer.calls)
	})
}

func TestGrokFreeRecoveryServiceStopCancelsProbe(t *testing.T) {
	store := &grokFreeRecoveryStoreStub{accounts: map[int64]Account{5: pendingGrokRecoveryAccount(5)}}
	started := make(chan struct{})
	service := newGrokFreeRecoveryServiceForTest(store, grokFreeRecoveryProberFunc(func(ctx context.Context, _ int64) (*GrokQuotaProbeResult, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	}), &grokFreeRecoveryRecovererStub{})

	service.Start()
	<-started
	done := make(chan struct{})
	go func() {
		service.Stop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Stop did not cancel the in-flight recovery probe")
	}
}

func TestGrokFreeRecoveryServiceSkipsOverlappingCycleInSameInstance(t *testing.T) {
	store := &grokFreeRecoveryStoreStub{accounts: map[int64]Account{6: pendingGrokRecoveryAccount(6)}}
	started := make(chan struct{})
	release := make(chan struct{})
	var probeCalls atomic.Int32
	service := newGrokFreeRecoveryServiceForTest(store, grokFreeRecoveryProberFunc(func(context.Context, int64) (*GrokQuotaProbeResult, error) {
		probeCalls.Add(1)
		close(started)
		<-release
		return &GrokQuotaProbeResult{StatusCode: 429, Snapshot: &xai.QuotaSnapshot{StatusCode: 429}}, nil
	}), &grokFreeRecoveryRecovererStub{})

	firstDone := make(chan struct{})
	go func() {
		service.runCycle(context.Background())
		close(firstDone)
	}()
	<-started

	secondDone := make(chan struct{})
	go func() {
		service.runCycle(context.Background())
		close(secondDone)
	}()
	select {
	case <-secondDone:
	case <-time.After(time.Second):
		t.Fatal("overlapping cycle did not return immediately")
	}

	close(release)
	<-firstDone
	require.Equal(t, int32(1), probeCalls.Load())
	require.Equal(t, 1, store.rearms)
}

func TestGrokFreeRecoveryServiceLeaderLockAllowsOnlyOneInstanceToProbe(t *testing.T) {
	store := &grokFreeRecoveryStoreStub{accounts: map[int64]Account{7: pendingGrokRecoveryAccount(7)}}
	lockCache := &fakeLeaderLockCache{}
	started := make(chan struct{})
	release := make(chan struct{})
	var probeCalls atomic.Int32
	prober := grokFreeRecoveryProberFunc(func(context.Context, int64) (*GrokQuotaProbeResult, error) {
		probeCalls.Add(1)
		close(started)
		<-release
		return &GrokQuotaProbeResult{StatusCode: 429, Snapshot: &xai.QuotaSnapshot{StatusCode: 429}}, nil
	})
	serviceA := newGrokFreeRecoveryService(store, prober, &grokFreeRecoveryRecovererStub{}, lockCache, nil)
	serviceB := newGrokFreeRecoveryService(store, prober, &grokFreeRecoveryRecovererStub{}, lockCache, nil)

	firstDone := make(chan struct{})
	go func() {
		serviceA.runCycle(context.Background())
		close(firstDone)
	}()
	<-started

	serviceB.runCycle(context.Background())
	require.Equal(t, int32(1), probeCalls.Load())
	require.Equal(t, 1, store.rearms)

	close(release)
	<-firstDone
	require.Empty(t, lockCache.heldBy(grokFreeRecoveryLeaderLockKey))
}

func TestGrokFreeRecoveryServiceBoundsProbeConcurrency(t *testing.T) {
	accounts := make(map[int64]Account, 10)
	for id := int64(1); id <= 10; id++ {
		accounts[id] = pendingGrokRecoveryAccount(id)
	}
	store := &grokFreeRecoveryStoreStub{accounts: accounts}
	started := make(chan int64, len(accounts))
	release := make(chan struct{})
	var active atomic.Int32
	var maximum atomic.Int32
	var calls atomic.Int32
	service := newGrokFreeRecoveryServiceForTest(store, grokFreeRecoveryProberFunc(func(_ context.Context, id int64) (*GrokQuotaProbeResult, error) {
		calls.Add(1)
		current := active.Add(1)
		for {
			observed := maximum.Load()
			if current <= observed || maximum.CompareAndSwap(observed, current) {
				break
			}
		}
		started <- id
		<-release
		active.Add(-1)
		return &GrokQuotaProbeResult{StatusCode: 429, Snapshot: &xai.QuotaSnapshot{StatusCode: 429}}, nil
	}), &grokFreeRecoveryRecovererStub{})

	done := make(chan struct{})
	go func() {
		service.runCycle(context.Background())
		close(done)
	}()
	for range grokFreeRecoveryMaxWorkers {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("worker pool did not start the expected probes")
		}
	}
	select {
	case id := <-started:
		t.Fatalf("probe %d exceeded worker bound", id)
	case <-time.After(50 * time.Millisecond):
	}

	close(release)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("bounded probe cycle did not finish")
	}
	require.Equal(t, int32(len(accounts)), calls.Load())
	require.Equal(t, int32(grokFreeRecoveryMaxWorkers), maximum.Load())
}

func TestSortGrokFreeRecoveryCandidatesPrioritizesOldestAndNeverProbed(t *testing.T) {
	oldest := pendingGrokRecoveryAccount(3)
	oldest.Extra[GrokFreeRecoveryNextProbeAtExtraKey] = time.Now().Add(-time.Hour).UTC().Format(time.RFC3339Nano)
	newer := pendingGrokRecoveryAccount(2)
	newer.Extra[GrokFreeRecoveryNextProbeAtExtraKey] = time.Now().Add(-time.Minute).UTC().Format(time.RFC3339Nano)
	neverProbed := pendingGrokRecoveryAccount(1)
	accounts := []Account{newer, oldest, neverProbed}

	sortGrokFreeRecoveryCandidates(accounts)

	require.Equal(t, []int64{1, 3, 2}, []int64{accounts[0].ID, accounts[1].ID, accounts[2].ID})
}

func TestGrokFreeRecoveryServiceLeaseDoesNotShortenLaterUpstreamReset(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	laterReset := now.Add(24 * time.Hour)
	account := pendingGrokRecoveryAccount(8)
	account.RateLimitResetAt = &laterReset
	store := &grokFreeRecoveryStoreStub{accounts: map[int64]Account{8: account}}
	service := newGrokFreeRecoveryServiceForTest(store, grokFreeRecoveryProberFunc(func(context.Context, int64) (*GrokQuotaProbeResult, error) {
		return &GrokQuotaProbeResult{StatusCode: 429, Snapshot: &xai.QuotaSnapshot{StatusCode: 429}}, nil
	}), &grokFreeRecoveryRecovererStub{})
	service.now = func() time.Time { return now }

	service.runCycle(context.Background())

	got, err := store.GetByID(context.Background(), 8)
	require.NoError(t, err)
	require.NotNil(t, got.RateLimitResetAt)
	require.Equal(t, laterReset, *got.RateLimitResetAt)
	require.Equal(t, 1, store.rearms)
}
