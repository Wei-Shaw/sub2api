package service

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func newTestRedis(t *testing.T) *redis.Client {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return rdb
}

func TestTryAcquireSingletonLeaderLock_NoBackendRunsUngated(t *testing.T) {
	release, ok := tryAcquireSingletonLeaderLock(context.Background(), nil, nil, "k", "inst", time.Minute)
	require.True(t, ok)
	require.NotNil(t, release)
	require.NotPanics(t, release)
}

func TestTryAcquireSingletonLeaderLock_RedisContendedThenReleased(t *testing.T) {
	rdb := newTestRedis(t)
	ctx := context.Background()
	const key = "leader:test:contended"

	releaseA, ok := tryAcquireSingletonLeaderLock(ctx, rdb, nil, key, "A", time.Minute)
	require.True(t, ok, "first instance should acquire")

	_, okB := tryAcquireSingletonLeaderLock(ctx, rdb, nil, key, "B", time.Minute)
	require.False(t, okB, "peer must be locked out while the lock is held")

	releaseA()

	releaseB, okB := tryAcquireSingletonLeaderLock(ctx, rdb, nil, key, "B", time.Minute)
	require.True(t, okB, "peer should acquire after the holder releases")
	releaseB()

	exists, err := rdb.Exists(ctx, key).Result()
	require.NoError(t, err)
	require.Equal(t, int64(0), exists, "lock key should be deleted after release")
}

// A stale holder whose lock already expired and was re-acquired by a peer must not
// delete the peer's lock when its (late) release fires.
func TestTryAcquireSingletonLeaderLock_ReleaseIsCompareAndDelete(t *testing.T) {
	rdb := newTestRedis(t)
	ctx := context.Background()
	const key = "leader:test:cad"

	releaseA, ok := tryAcquireSingletonLeaderLock(ctx, rdb, nil, key, "A", time.Minute)
	require.True(t, ok)

	// Simulate A's lock expiring and peer B taking ownership.
	require.NoError(t, rdb.Set(ctx, key, "B", time.Minute).Err())

	releaseA() // stale release from A must be a no-op against B's lock.

	val, err := rdb.Get(ctx, key).Result()
	require.NoError(t, err)
	require.Equal(t, "B", val, "stale holder must not delete the new owner's lock")
}

func TestSubscriptionExpiryService_ReminderSkipsScanWhenNotLeader(t *testing.T) {
	rdb := newTestRedis(t)
	// A peer already holds the reminder leader lock.
	require.NoError(t, rdb.Set(context.Background(), subscriptionExpiryReminderLeaderLockKey, "peer", time.Minute).Err())

	repo := &subscriptionExpiryRepoStub{}
	settingRepo := &subscriptionExpirySettingRepoStub{values: map[string]string{}}
	svc := NewSubscriptionExpiryService(repo, time.Minute)
	svc.SetSettingRepository(settingRepo)
	svc.SetNotificationEmailService(NewNotificationEmailService(settingRepo, nil))
	svc.SetLeaderLockBackends(rdb, nil)

	svc.sendExpiryReminders(context.Background())

	require.Zero(t, repo.listCalls, "non-leader must not scan active subscriptions")
}

func TestSubscriptionExpiryService_ReminderScansWhenLeader(t *testing.T) {
	rdb := newTestRedis(t)

	repo := &subscriptionExpiryRepoStub{}
	settingRepo := &subscriptionExpirySettingRepoStub{values: map[string]string{}}
	svc := NewSubscriptionExpiryService(repo, time.Minute)
	svc.SetSettingRepository(settingRepo)
	svc.SetNotificationEmailService(NewNotificationEmailService(settingRepo, nil))
	svc.SetLeaderLockBackends(rdb, nil)

	svc.sendExpiryReminders(context.Background())

	require.Equal(t, 1, repo.listCalls, "leader should scan active subscriptions once")
}

// Single-instance correctness: the lock is released at the end of each cycle, so
// the same instance must re-acquire it and run on every subsequent cycle (no
// self-lockout). Covers both the Redis path and the no-backend path.
func TestSubscriptionExpiryService_ReminderRunsEveryCycleSingleInstance(t *testing.T) {
	cases := map[string]*redis.Client{
		"with_redis": newTestRedis(t),
		"no_backend": nil,
	}
	for name, rdb := range cases {
		t.Run(name, func(t *testing.T) {
			repo := &subscriptionExpiryRepoStub{}
			settingRepo := &subscriptionExpirySettingRepoStub{values: map[string]string{}}
			svc := NewSubscriptionExpiryService(repo, time.Minute)
			svc.SetSettingRepository(settingRepo)
			svc.SetNotificationEmailService(NewNotificationEmailService(settingRepo, nil))
			svc.SetLeaderLockBackends(rdb, nil)

			// Three consecutive cycles, mimicking the ticker loop.
			svc.sendExpiryReminders(context.Background())
			svc.sendExpiryReminders(context.Background())
			svc.sendExpiryReminders(context.Background())

			require.Equal(t, 3, repo.listCalls, "single instance must run every cycle")
		})
	}
}
