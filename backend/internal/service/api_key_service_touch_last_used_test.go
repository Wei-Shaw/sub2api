//go:build unit

package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAPIKeyService_TouchLastUsed_InvalidKeyID(t *testing.T) {
	repo := &apiKeyRepoStub{
		updateLastUsed: func(ctx context.Context, id int64, usedAt time.Time) error {
			return errors.New("should not be called")
		},
	}
	svc := &APIKeyService{apiKeyRepo: repo}

	require.NoError(t, svc.TouchLastUsed(context.Background(), 0))
	require.NoError(t, svc.TouchLastUsed(context.Background(), -1))
	require.Empty(t, repo.touchedIDs)
}

func TestAPIKeyService_TouchLastUsed_FirstTouchSucceeds(t *testing.T) {
	repo := &apiKeyRepoStub{}
	svc := &APIKeyService{apiKeyRepo: repo}

	err := svc.TouchLastUsed(context.Background(), 123)
	require.NoError(t, err)
	require.Equal(t, []int64{123}, repo.touchedIDs)
	require.Len(t, repo.touchedUsedAts, 1)
	require.False(t, repo.touchedUsedAts[0].IsZero())

	cached, ok := svc.lastUsedTouchL1.Load(int64(123))
	require.True(t, ok, "successful touch should update debounce cache")
	_, isTime := cached.(time.Time)
	require.True(t, isTime)
}

func TestAPIKeyService_TouchLastUsed_DebouncedWithinWindow(t *testing.T) {
	repo := &apiKeyRepoStub{}
	svc := &APIKeyService{apiKeyRepo: repo}

	require.NoError(t, svc.TouchLastUsed(context.Background(), 123))
	require.NoError(t, svc.TouchLastUsed(context.Background(), 123))

	require.Equal(t, []int64{123}, repo.touchedIDs, "second touch within debounce window should not hit repository")
}

func TestAPIKeyService_TouchLastUsed_ExpiredDebounceTouchesAgain(t *testing.T) {
	repo := &apiKeyRepoStub{}
	svc := &APIKeyService{apiKeyRepo: repo}

	require.NoError(t, svc.TouchLastUsed(context.Background(), 123))

	// 强制将 debounce 时间回拨到窗口之外，触发第二次写库。
	svc.lastUsedTouchL1.Store(int64(123), time.Now().Add(-apiKeyLastUsedMinTouch-time.Second))

	require.NoError(t, svc.TouchLastUsed(context.Background(), 123))
	require.Len(t, repo.touchedIDs, 2)
	require.Equal(t, int64(123), repo.touchedIDs[0])
	require.Equal(t, int64(123), repo.touchedIDs[1])
}

func TestAPIKeyService_TouchLastUsed_RepoError(t *testing.T) {
	repo := &apiKeyRepoStub{
		updateLastUsed: func(ctx context.Context, id int64, usedAt time.Time) error {
			return errors.New("db write failed")
		},
	}
	svc := &APIKeyService{apiKeyRepo: repo}

	err := svc.TouchLastUsed(context.Background(), 123)
	require.Error(t, err)
	require.ErrorContains(t, err, "touch api key last used")
	require.Equal(t, []int64{123}, repo.touchedIDs)

	cached, ok := svc.lastUsedTouchL1.Load(int64(123))
	require.True(t, ok, "failed touch should still update retry debounce cache")
	_, isTime := cached.(time.Time)
	require.True(t, isTime)
}

func TestAPIKeyService_TouchLastUsed_RepoErrorDebounced(t *testing.T) {
	repo := &apiKeyRepoStub{
		updateLastUsed: func(ctx context.Context, id int64, usedAt time.Time) error {
			return errors.New("db write failed")
		},
	}
	svc := &APIKeyService{apiKeyRepo: repo}

	firstErr := svc.TouchLastUsed(context.Background(), 456)
	require.Error(t, firstErr)
	require.ErrorContains(t, firstErr, "touch api key last used")

	secondErr := svc.TouchLastUsed(context.Background(), 456)
	require.NoError(t, secondErr, "failed touch should be debounced and skip immediate retry")
	require.Equal(t, []int64{456}, repo.touchedIDs, "debounced retry should not hit repository again")
}

type touchSingleflightRepo struct {
	*apiKeyRepoStub
	mu      sync.Mutex
	calls   int
	blockCh chan struct{}
}

func (r *touchSingleflightRepo) UpdateLastUsed(ctx context.Context, id int64, usedAt time.Time) error {
	r.mu.Lock()
	r.calls++
	r.mu.Unlock()
	<-r.blockCh
	return nil
}

func TestAPIKeyService_TouchLastUsed_ConcurrentFirstTouchDeduplicated(t *testing.T) {
	repo := &touchSingleflightRepo{
		apiKeyRepoStub: &apiKeyRepoStub{},
		blockCh:        make(chan struct{}),
	}
	svc := &APIKeyService{apiKeyRepo: repo}

	const workers = 20
	startCh := make(chan struct{})
	errCh := make(chan error, workers)
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-startCh
			errCh <- svc.TouchLastUsed(context.Background(), 321)
		}()
	}

	close(startCh)

	require.Eventually(t, func() bool {
		repo.mu.Lock()
		defer repo.mu.Unlock()
		return repo.calls >= 1
	}, time.Second, 10*time.Millisecond)

	close(repo.blockCh)
	wg.Wait()
	close(errCh)

	for err := range errCh {
		require.NoError(t, err)
	}

	repo.mu.Lock()
	defer repo.mu.Unlock()
	require.Equal(t, 1, repo.calls, "并发首次 touch 只应写库一次")
}

func TestAPIKeyService_TouchLastUsedAsync_DoesNotBlockAndCoalesces(t *testing.T) {
	repo := &touchSingleflightRepo{
		apiKeyRepoStub: &apiKeyRepoStub{},
		blockCh:        make(chan struct{}),
	}
	svc := &APIKeyService{apiKeyRepo: repo}
	calls := func() int {
		repo.mu.Lock()
		defer repo.mu.Unlock()
		return repo.calls
	}

	// The repository write hangs until blockCh closes; the caller must not wait for it.
	done := make(chan struct{})
	go func() {
		for i := 0; i < 50; i++ {
			svc.TouchLastUsedAsync(context.Background(), 321)
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("TouchLastUsedAsync blocked on the repository write")
	}
	require.Eventually(t, func() bool { return calls() == 1 }, time.Second, 5*time.Millisecond)

	close(repo.blockCh)
	require.Eventually(t, func() bool { return svc.lastUsedTouchInFlight.Load() == 0 }, time.Second, 5*time.Millisecond)

	// Inside the debounce window the fast path returns without scheduling another write.
	for i := 0; i < 50; i++ {
		svc.TouchLastUsedAsync(context.Background(), 321)
	}
	require.Zero(t, svc.lastUsedTouchInFlight.Load())
	require.Equal(t, 1, calls(), "repeated touches for one key within the interval should write once")
}

func TestAPIKeyService_TouchLastUsedAsync_DetachesFromRequestContext(t *testing.T) {
	type writeCtx struct {
		err         error
		hasDeadline bool
	}
	seen := make(chan writeCtx, 1)
	repo := &apiKeyRepoStub{
		updateLastUsed: func(ctx context.Context, id int64, usedAt time.Time) error {
			_, hasDeadline := ctx.Deadline()
			seen <- writeCtx{err: ctx.Err(), hasDeadline: hasDeadline}
			return nil
		},
	}
	svc := &APIKeyService{apiKeyRepo: repo}

	reqCtx, cancel := context.WithCancel(context.Background())
	cancel() // the request is already gone by the time the write runs

	svc.TouchLastUsedAsync(reqCtx, 42)

	select {
	case got := <-seen:
		require.NoError(t, got.err, "write must not inherit the request cancellation")
		require.True(t, got.hasDeadline, "write must still be bounded by a timeout")
	case <-time.After(time.Second):
		t.Fatal("expected an async last_used write")
	}
}

func TestAPIKeyService_TouchLastUsedAsync_BoundsInFlightWrites(t *testing.T) {
	repo := &touchSingleflightRepo{
		apiKeyRepoStub: &apiKeyRepoStub{},
		blockCh:        make(chan struct{}),
	}
	svc := &APIKeyService{apiKeyRepo: repo}
	calls := func() int {
		repo.mu.Lock()
		defer repo.mu.Unlock()
		return repo.calls
	}

	for id := int64(1); id <= apiKeyLastUsedTouchMaxInFlight*2; id++ {
		svc.TouchLastUsedAsync(context.Background(), id)
	}
	require.EqualValues(t, apiKeyLastUsedTouchMaxInFlight, svc.lastUsedTouchInFlight.Load())
	require.Eventually(t, func() bool { return calls() == apiKeyLastUsedTouchMaxInFlight }, time.Second, 5*time.Millisecond)

	close(repo.blockCh)
	require.Eventually(t, func() bool { return svc.lastUsedTouchInFlight.Load() == 0 }, time.Second, 5*time.Millisecond)
	require.Equal(t, apiKeyLastUsedTouchMaxInFlight, calls())

	// A key skipped at the cap was not debounced, so its next request writes it.
	svc.TouchLastUsedAsync(context.Background(), apiKeyLastUsedTouchMaxInFlight*2)
	require.Eventually(t, func() bool { return calls() == apiKeyLastUsedTouchMaxInFlight+1 }, time.Second, 5*time.Millisecond)
}

type slowTouchRepo struct {
	*apiKeyRepoStub
}

func (r *slowTouchRepo) UpdateLastUsed(context.Context, int64, time.Time) error {
	time.Sleep(time.Millisecond)
	return nil
}

// BenchmarkTouchLastUsedDueKey measures what the auth path pays per request when a key's
// debounce window has expired and the DB write takes ~1ms (sync = the old middleware path).
func BenchmarkTouchLastUsedDueKey(b *testing.B) {
	b.Run("sync", func(b *testing.B) {
		svc := &APIKeyService{apiKeyRepo: &slowTouchRepo{apiKeyRepoStub: &apiKeyRepoStub{}}}
		for i := 0; i < b.N; i++ {
			_ = svc.TouchLastUsed(context.Background(), int64(i+1))
		}
	})
	b.Run("async", func(b *testing.B) {
		svc := &APIKeyService{apiKeyRepo: &slowTouchRepo{apiKeyRepoStub: &apiKeyRepoStub{}}}
		for i := 0; i < b.N; i++ {
			svc.TouchLastUsedAsync(context.Background(), int64(i+1))
		}
	})
}
