package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/dgraph-io/ristretto"
	"github.com/stretchr/testify/require"
)

type authInvalidationRepoStub struct {
	mu         sync.Mutex
	events     []AuthCacheInvalidationEvent
	claimLimit int
	scheduled  []int64
	deleted    []int64
	retried    []int64
	retryError string
	stats      AuthCacheInvalidationOutboxStats
	statsErr   error
}

func (r *authInvalidationRepoStub) Claim(_ context.Context, _ string, limit int, _ time.Duration) ([]AuthCacheInvalidationEvent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.claimLimit = limit
	return append([]AuthCacheInvalidationEvent(nil), r.events...), nil
}
func (r *authInvalidationRepoStub) DeleteClaimed(_ context.Context, id int64, _ string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.deleted = append(r.deleted, id)
	return nil
}
func (r *authInvalidationRepoStub) ScheduleSecondPass(_ context.Context, id int64, _ string, _ time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.scheduled = append(r.scheduled, id)
	return nil
}
func (r *authInvalidationRepoStub) RetryClaimed(_ context.Context, id int64, _ string, _ time.Time, lastError string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.retried = append(r.retried, id)
	r.retryError = lastError
	return nil
}
func (r *authInvalidationRepoStub) Stats(context.Context) (AuthCacheInvalidationOutboxStats, error) {
	return r.stats, r.statsErr
}

type authInvalidationCacheStub struct {
	mu          sync.Mutex
	deleteFn    func(context.Context, string) error
	publishFn   func(context.Context, string) error
	subscribeFn func(context.Context, func(string)) error
	deleted     []string
	published   []string
}

func (*authInvalidationCacheStub) GetCreateAttemptCount(context.Context, int64) (int, error) {
	return 0, nil
}
func (*authInvalidationCacheStub) IncrementCreateAttemptCount(context.Context, int64) error {
	return nil
}
func (*authInvalidationCacheStub) DeleteCreateAttemptCount(context.Context, int64) error { return nil }
func (*authInvalidationCacheStub) IncrementDailyUsage(context.Context, string) error     { return nil }
func (*authInvalidationCacheStub) SetDailyUsageExpiry(context.Context, string, time.Duration) error {
	return nil
}
func (*authInvalidationCacheStub) GetAuthCache(context.Context, string) (*APIKeyAuthCacheEntry, error) {
	return nil, errors.New("miss")
}
func (*authInvalidationCacheStub) SetAuthCache(context.Context, string, *APIKeyAuthCacheEntry, time.Duration) error {
	return nil
}
func (c *authInvalidationCacheStub) DeleteAuthCache(ctx context.Context, key string) error {
	c.mu.Lock()
	c.deleted = append(c.deleted, key)
	c.mu.Unlock()
	if c.deleteFn != nil {
		return c.deleteFn(ctx, key)
	}
	return nil
}
func (c *authInvalidationCacheStub) PublishAuthCacheInvalidation(ctx context.Context, key string) error {
	c.mu.Lock()
	c.published = append(c.published, key)
	c.mu.Unlock()
	if c.publishFn != nil {
		return c.publishFn(ctx, key)
	}
	return nil
}
func (c *authInvalidationCacheStub) SubscribeAuthCacheInvalidation(ctx context.Context, handler func(string)) error {
	if c.subscribeFn != nil {
		return c.subscribeFn(ctx, handler)
	}
	return nil
}

func TestAuthCacheInvalidationWorker_FirstPassSchedulesSafetyPass(t *testing.T) {
	repo := &authInvalidationRepoStub{}
	cache := &authInvalidationCacheStub{}
	worker := NewAuthCacheInvalidationWorker(repo, cache)
	worker.processEvent(context.Background(), AuthCacheInvalidationEvent{ID: 7, CacheKey: "hash", Stage: 0})
	require.Equal(t, []string{"hash"}, cache.deleted)
	require.Equal(t, []string{"hash"}, cache.published)
	require.Equal(t, []int64{7}, repo.scheduled)
	require.Empty(t, repo.deleted)
}

func TestAuthCacheInvalidationWorker_SecondPassCleansEvent(t *testing.T) {
	repo := &authInvalidationRepoStub{}
	cache := &authInvalidationCacheStub{}
	worker := NewAuthCacheInvalidationWorker(repo, cache)
	worker.processEvent(context.Background(), AuthCacheInvalidationEvent{ID: 8, CacheKey: "hash", Stage: 1})
	require.Equal(t, []int64{8}, repo.deleted)
	require.Equal(t, uint64(1), worker.Health(context.Background()).Processed)
}

func TestAuthCacheInvalidationWorker_RetriesRedisAndPublishFailures(t *testing.T) {
	for _, tc := range []struct {
		name       string
		deleteErr  error
		publishErr error
		published  int
	}{
		{name: "redis down", deleteErr: errors.New("redis unavailable")},
		{name: "publish failure after delete", publishErr: errors.New("publish failed"), published: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &authInvalidationRepoStub{}
			cache := &authInvalidationCacheStub{
				deleteFn:  func(context.Context, string) error { return tc.deleteErr },
				publishFn: func(context.Context, string) error { return tc.publishErr },
			}
			worker := NewAuthCacheInvalidationWorker(repo, cache)
			worker.processEvent(context.Background(), AuthCacheInvalidationEvent{ID: 9, CacheKey: "hash"})
			require.Equal(t, []int64{9}, repo.retried)
			require.Len(t, cache.published, tc.published)
			require.NotEmpty(t, repo.retryError)
			require.Empty(t, repo.deleted)
			require.Equal(t, uint64(1), worker.Health(context.Background()).Failures)
		})
	}
}

func TestAuthCacheInvalidationWorker_RedisSlowIsTimedOut(t *testing.T) {
	repo := &authInvalidationRepoStub{}
	cache := &authInvalidationCacheStub{deleteFn: func(ctx context.Context, _ string) error {
		<-ctx.Done()
		return ctx.Err()
	}}
	worker := NewAuthCacheInvalidationWorker(repo, cache)
	started := time.Now()
	worker.processEvent(context.Background(), AuthCacheInvalidationEvent{ID: 10, CacheKey: "hash"})
	require.Less(t, time.Since(started), 3*time.Second)
	require.Equal(t, []int64{10}, repo.retried)
	require.Contains(t, repo.retryError, "deadline")
}

func TestAuthCacheInvalidationWorker_BoundedBatchAndHealth(t *testing.T) {
	oldest := time.Now().Add(-time.Minute)
	repo := &authInvalidationRepoStub{stats: AuthCacheInvalidationOutboxStats{
		Pending: 12, OldestCreatedAt: &oldest, MaxAttempts: 4, LastError: "redis down",
	}}
	worker := NewAuthCacheInvalidationWorker(repo, &authInvalidationCacheStub{})
	require.NoError(t, worker.processBatch(context.Background()))
	require.Equal(t, authInvalidationBatchSize, repo.claimLimit)
	health := worker.Health(context.Background())
	require.Equal(t, int64(12), health.Pending)
	require.Equal(t, 4, health.MaxAttempts)
	require.Equal(t, "redis down", health.LastError)
	require.GreaterOrEqual(t, health.OldestLag, time.Minute)
	require.Equal(t, 35*time.Second, health.HealthySLA)
	require.Equal(t, 6*time.Minute, health.RecoverySLA)
}

func TestAuthCacheInvalidationWorker_ProcessesClaimedBatchConcurrently(t *testing.T) {
	events := make([]AuthCacheInvalidationEvent, 32)
	for i := range events {
		events[i] = AuthCacheInvalidationEvent{ID: int64(i + 1), CacheKey: "hash", Stage: 1}
	}
	repo := &authInvalidationRepoStub{events: events}
	cache := &authInvalidationCacheStub{deleteFn: func(context.Context, string) error {
		time.Sleep(100 * time.Millisecond)
		return nil
	}}
	worker := NewAuthCacheInvalidationWorker(repo, cache)
	started := time.Now()
	require.NoError(t, worker.processBatch(context.Background()))
	require.Less(t, time.Since(started), time.Second)
	require.Len(t, repo.deleted, 32)
}

func TestAuthCacheInvalidationWorker_LifecycleIsManagedAndIdempotent(t *testing.T) {
	worker := NewAuthCacheInvalidationWorker(&authInvalidationRepoStub{}, &authInvalidationCacheStub{})
	worker.Start()
	require.Eventually(t, func() bool { return worker.Health(context.Background()).Running }, time.Second, 10*time.Millisecond)
	require.NotPanics(t, func() { worker.Stop(); worker.Stop() })
	require.False(t, worker.Health(context.Background()).Running)
}

func TestAuthInvalidationRetryDelayIsBoundedAndJittered(t *testing.T) {
	for attempt := 1; attempt <= 20; attempt++ {
		delay := authInvalidationRetryDelay(attempt)
		require.GreaterOrEqual(t, delay, 800*time.Millisecond)
		require.LessOrEqual(t, delay, 308*time.Second)
	}
}

func TestAuthCacheInvalidationSubscriber_RetriesInitialFailureAndStops(t *testing.T) {
	ready := make(chan struct{})
	var calls int
	cache := &authInvalidationCacheStub{subscribeFn: func(ctx context.Context, _ func(string)) error {
		calls++
		if calls == 1 {
			return errors.New("redis starting")
		}
		NotifyAuthCacheSubscriptionReady(ctx)
		close(ready)
		<-ctx.Done()
		return ctx.Err()
	}}
	svc := NewAPIKeyService(nil, nil, nil, nil, nil, cache, nil)
	localCache, err := ristretto.NewCache(&ristretto.Config{NumCounters: 10, MaxCost: 1, BufferItems: 64})
	require.NoError(t, err)
	defer localCache.Close()
	svc.authNegativeCacheL1 = localCache
	svc.StartAuthCacheInvalidationSubscriber(context.Background())
	select {
	case <-ready:
	case <-time.After(2 * time.Second):
		t.Fatal("subscriber did not retry")
	}
	require.Eventually(t, func() bool { return svc.AuthCacheInvalidationSubscriberHealth().Connected }, time.Second, 10*time.Millisecond)
	require.Equal(t, uint64(1), svc.AuthCacheInvalidationSubscriberHealth().Failures)
	require.NotPanics(t, func() { svc.StopAuthCacheInvalidationSubscriber(); svc.StopAuthCacheInvalidationSubscriber() })
}

func TestAuthCacheInvalidationSubscriber_ReconnectsAfterRuntimeDisconnect(t *testing.T) {
	ready := make(chan int, 2)
	var calls int
	cache := &authInvalidationCacheStub{subscribeFn: func(ctx context.Context, _ func(string)) error {
		calls++
		NotifyAuthCacheSubscriptionReady(ctx)
		ready <- calls
		if calls == 1 {
			return errors.New("connection dropped")
		}
		<-ctx.Done()
		return ctx.Err()
	}}
	svc := NewAPIKeyService(nil, nil, nil, nil, nil, cache, nil)
	localCache, err := ristretto.NewCache(&ristretto.Config{NumCounters: 10, MaxCost: 1, BufferItems: 64})
	require.NoError(t, err)
	defer localCache.Close()
	svc.authNegativeCacheL1 = localCache
	svc.StartAuthCacheInvalidationSubscriber(context.Background())

	select {
	case call := <-ready:
		require.Equal(t, 1, call)
	case <-time.After(time.Second):
		t.Fatal("initial subscription did not start")
	}
	select {
	case call := <-ready:
		require.Equal(t, 2, call)
	case <-time.After(2 * time.Second):
		t.Fatal("subscriber did not reconnect after runtime disconnect")
	}
	require.Eventually(t, func() bool { return svc.AuthCacheInvalidationSubscriberHealth().Connected }, time.Second, 10*time.Millisecond)
	require.Equal(t, uint64(1), svc.AuthCacheInvalidationSubscriberHealth().Failures)
	svc.StopAuthCacheInvalidationSubscriber()
}
