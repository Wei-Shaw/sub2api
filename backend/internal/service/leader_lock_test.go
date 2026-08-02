package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// fakeLeaderLockCache is an in-memory LeaderLockCache for unit tests. It models the
// compare-and-delete release semantics of the real Redis-backed implementation.
type fakeLeaderLockCache struct {
	mu         sync.Mutex
	owners     map[string]string
	acquireErr error
}

func (f *fakeLeaderLockCache) TryAcquireLeaderLock(_ context.Context, key, owner string, _ time.Duration) (bool, error) {
	if f.acquireErr != nil {
		return false, f.acquireErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.owners == nil {
		f.owners = map[string]string{}
	}
	if _, held := f.owners[key]; held {
		return false, nil
	}
	f.owners[key] = owner
	return true, nil
}

func (f *fakeLeaderLockCache) ReleaseLeaderLock(_ context.Context, key, owner string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.owners[key] == owner {
		delete(f.owners, key)
	}
	return nil
}

func (f *fakeLeaderLockCache) heldBy(key string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.owners[key]
}

func TestTryAcquireSingletonLeaderLock_NoBackendRunsUngated(t *testing.T) {
	release, ok := tryAcquireSingletonLeaderLock(context.Background(), nil, nil, "k", "inst", time.Minute)
	require.True(t, ok)
	require.NotNil(t, release)
	require.NotPanics(t, release)
}

func TestTryAcquireSingletonLeaderLock_ContendedThenReleased(t *testing.T) {
	cache := &fakeLeaderLockCache{}
	ctx := context.Background()
	const key = "leader:test:contended"

	releaseA, ok := tryAcquireSingletonLeaderLock(ctx, cache, nil, key, "A", time.Minute)
	require.True(t, ok, "first instance should acquire")
	require.Equal(t, "A", cache.heldBy(key))

	_, okB := tryAcquireSingletonLeaderLock(ctx, cache, nil, key, "B", time.Minute)
	require.False(t, okB, "peer must be locked out while the lock is held")

	releaseA()
	require.Empty(t, cache.heldBy(key), "release must free the lock")

	releaseB, okB := tryAcquireSingletonLeaderLock(ctx, cache, nil, key, "B", time.Minute)
	require.True(t, okB, "peer should acquire after the holder releases")
	releaseB()
}

// When Redis is configured but errors, skip — never fall through to ungated
// run or DB. Peers may still hold the Redis lock (avoids split-brain).
func TestTryAcquireSingletonLeaderLock_CacheErrorSkips(t *testing.T) {
	cache := &fakeLeaderLockCache{acquireErr: context.DeadlineExceeded}
	release, ok := tryAcquireSingletonLeaderLock(context.Background(), cache, nil, "k", "inst", time.Minute)
	require.False(t, ok, "cache error with live ctx must skip, not run ungated")
	require.Nil(t, release)
}

// When the cache errors AND the caller ctx is already canceled/deadline, still
// skip (same path as any cache error — no ungated fallthrough).
func TestTryAcquireSingletonLeaderLock_CacheErrorCanceledCtxSkips(t *testing.T) {
	cache := &fakeLeaderLockCache{acquireErr: context.DeadlineExceeded}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	release, ok := tryAcquireSingletonLeaderLock(ctx, cache, nil, "k", "inst", time.Minute)
	require.False(t, ok, "dead ctx + cache error must skip, not run ungated")
	require.Nil(t, release)
}

func TestSubscriptionExpiryService_ReminderSkipsScanWhenNotLeader(t *testing.T) {
	cache := &fakeLeaderLockCache{}
	// A peer already holds the reminder leader lock.
	_, _ = cache.TryAcquireLeaderLock(context.Background(), subscriptionExpiryReminderLeaderLockKey, "peer", time.Minute)

	repo := &subscriptionExpiryRepoStub{}
	settingRepo := &subscriptionExpirySettingRepoStub{values: map[string]string{}}
	svc := NewSubscriptionExpiryService(repo, time.Minute)
	svc.SetSettingRepository(settingRepo)
	svc.SetNotificationEmailService(NewNotificationEmailService(settingRepo, nil))
	svc.SetLeaderLock(cache, nil)

	svc.sendExpiryReminders(context.Background())

	require.Zero(t, repo.listCalls, "non-leader must not scan active subscriptions")
}

func TestSubscriptionExpiryService_ReminderScansWhenLeader(t *testing.T) {
	repo := &subscriptionExpiryRepoStub{}
	settingRepo := &subscriptionExpirySettingRepoStub{values: map[string]string{}}
	svc := NewSubscriptionExpiryService(repo, time.Minute)
	svc.SetSettingRepository(settingRepo)
	svc.SetNotificationEmailService(NewNotificationEmailService(settingRepo, nil))
	svc.SetLeaderLock(&fakeLeaderLockCache{}, nil)

	svc.sendExpiryReminders(context.Background())

	require.Equal(t, 1, repo.listCalls, "leader should scan active subscriptions once")
}

// Single-instance correctness: the lock is released at the end of each cycle, so
// the same instance must re-acquire it and run on every subsequent cycle (no
// self-lockout). Covers both the cache-backed path and the no-backend path.
func TestSubscriptionExpiryService_ReminderRunsEveryCycleSingleInstance(t *testing.T) {
	cases := map[string]LeaderLockCache{
		"with_cache": &fakeLeaderLockCache{},
		"no_backend": nil,
	}
	for name, cache := range cases {
		t.Run(name, func(t *testing.T) {
			repo := &subscriptionExpiryRepoStub{}
			settingRepo := &subscriptionExpirySettingRepoStub{values: map[string]string{}}
			svc := NewSubscriptionExpiryService(repo, time.Minute)
			svc.SetSettingRepository(settingRepo)
			svc.SetNotificationEmailService(NewNotificationEmailService(settingRepo, nil))
			svc.SetLeaderLock(cache, nil)

			// Three consecutive cycles, mimicking the ticker loop.
			svc.sendExpiryReminders(context.Background())
			svc.sendExpiryReminders(context.Background())
			svc.sendExpiryReminders(context.Background())

			require.Equal(t, 3, repo.listCalls, "single instance must run every cycle")
		})
	}
}

// fakeRefreshLeaderLock implements LeaderLockRefresher for heartbeat tests.
type fakeRefreshLeaderLock struct {
	fakeLeaderLockCache
	mu         sync.Mutex
	refreshOK  bool
	refreshErr error
	refreshes  int
}

func (f *fakeRefreshLeaderLock) RefreshLeaderLock(_ context.Context, _, _ string, _ time.Duration) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.refreshes++
	return f.refreshOK, f.refreshErr
}

func (f *fakeRefreshLeaderLock) refreshCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.refreshes
}

// H2: when Refresh returns false, derived context is canceled (after first tick).
func TestStartLeaderLockHeartbeat_CancelsOnRefreshFalse(t *testing.T) {
	cache := &fakeRefreshLeaderLock{refreshOK: false}
	// ttl=6s → interval=2s; first refresh after ~2s, not on start.
	ctx, stop := startLeaderLockHeartbeat(context.Background(), cache, "leader:hb:test", "owner-a", 6*time.Second)
	defer stop()

	require.Nil(t, ctx.Err(), "must not cancel before first refresh")
	require.Equal(t, 0, cache.refreshCount())

	select {
	case <-ctx.Done():
		// leadership lost after failed refresh
	case <-time.After(5 * time.Second):
		t.Fatal("expected heartbeat to cancel ctx after refresh returned false")
	}
	require.GreaterOrEqual(t, cache.refreshCount(), 1)
	stop() // idempotent
}

// H2: Refresh error also cancels.
func TestStartLeaderLockHeartbeat_CancelsOnRefreshError(t *testing.T) {
	cache := &fakeRefreshLeaderLock{refreshOK: false, refreshErr: context.DeadlineExceeded}
	ctx, stop := startLeaderLockHeartbeat(context.Background(), cache, "leader:hb:err", "owner-b", 6*time.Second)
	defer stop()

	select {
	case <-ctx.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("expected cancel on refresh error")
	}
}

// H2: no refresher → parent ctx returned, no-op stop.
func TestStartLeaderLockHeartbeat_NoRefresherPassthrough(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	cache := &fakeLeaderLockCache{} // no RefreshLeaderLock
	ctx, stop := startLeaderLockHeartbeat(parent, cache, "k", "o", time.Minute)
	require.Equal(t, parent, ctx)
	require.NotPanics(t, stop)
}
