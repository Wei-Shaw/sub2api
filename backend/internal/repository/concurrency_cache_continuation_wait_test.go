package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func newContinuationWaitTestCache(t *testing.T) (*concurrencyCache, *redis.Client) {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	cache, ok := NewConcurrencyCache(client, 15, 900).(*concurrencyCache)
	require.True(t, ok)
	return cache, client
}

func TestContinuationWaitCountIsIndependentFromLegacyWaitCount(t *testing.T) {
	cache, _ := newContinuationWaitTestCache(t)
	ctx := context.Background()
	for range 3 {
		ok, err := cache.IncrementAccountWaitCount(ctx, 7, 100)
		require.NoError(t, err)
		require.True(t, ok)
	}
	ok, err := cache.IncrementAccountContinuationWaitCount(ctx, 7, 3)
	require.NoError(t, err)
	require.True(t, ok, "3 个旧键等待者不应挡住续聊入队")

	cont, err := cache.GetAccountContinuationWaitingCount(ctx, 7)
	require.NoError(t, err)
	require.Equal(t, 1, cont)
	legacy, err := cache.GetAccountWaitingCount(ctx, 7)
	require.NoError(t, err)
	require.Equal(t, 3, legacy)

	loads, err := cache.GetAccountsLoadBatch(ctx, []service.AccountWithConcurrency{{ID: 7, MaxConcurrency: 5}})
	require.NoError(t, err)
	require.Equal(t, 4, loads[7].WaitingCount, "WaitingCount 是两类之和")
	require.Equal(t, 1, loads[7].ContinuationWaiting)
	require.Equal(t, (0+4)*100/5, loads[7].LoadRate)

	require.NoError(t, cache.DecrementAccountContinuationWaitCount(ctx, 7))
	cont, err = cache.GetAccountContinuationWaitingCount(ctx, 7)
	require.NoError(t, err)
	require.Equal(t, 0, cont)
}

func TestContinuationWaitCountRespectsOwnLimit(t *testing.T) {
	cache, _ := newContinuationWaitTestCache(t)
	ctx := context.Background()
	for range 3 {
		ok, err := cache.IncrementAccountContinuationWaitCount(ctx, 8, 3)
		require.NoError(t, err)
		require.True(t, ok)
	}
	ok, err := cache.IncrementAccountContinuationWaitCount(ctx, 8, 3)
	require.NoError(t, err)
	require.False(t, ok, "续聊上限只数续聊等待者")
	ok, err = cache.IncrementAccountWaitCount(ctx, 8, 100)
	require.NoError(t, err)
	require.True(t, ok, "续聊队列满不影响旧键")
}

func TestActiveIndexKeepsAccountWithOnlyContinuationWaiters(t *testing.T) {
	cache, client := newContinuationWaitTestCache(t)
	ctx := context.Background()
	ok, err := cache.IncrementAccountContinuationWaitCount(ctx, 9, 3)
	require.NoError(t, err)
	require.True(t, ok)
	_, err = client.ZScore(ctx, accountActiveIndexKey, "9").Result()
	require.NoError(t, err, "只有续聊等待者的账号必须留在活跃索引")

	cache.refreshAccountActiveIndex(ctx, 9)
	_, err = client.ZScore(ctx, accountActiveIndexKey, "9").Result()
	require.NoError(t, err, "以真实负载重建索引时续聊等待者要被计入")

	require.NoError(t, cache.DecrementAccountContinuationWaitCount(ctx, 9))
	_, err = client.ZScore(ctx, accountActiveIndexKey, "9").Result()
	require.ErrorIs(t, err, redis.Nil, "两类等待都为 0 且无槽位时索引成员应删除")
}

func newRefreshTestCache(t *testing.T) (*concurrencyCache, *redis.Client, *miniredis.Miniredis) {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	cache, ok := NewConcurrencyCache(client, 15, 900).(*concurrencyCache)
	require.True(t, ok)
	return cache, client, server
}

func TestRefreshAccountSlotOnlyTouchesExistingMember(t *testing.T) {
	cache, client, _ := newRefreshTestCache(t)
	ctx := context.Background()
	ok, err := cache.AcquireAccountSlot(ctx, 11, 1, "req-a")
	require.NoError(t, err)
	require.True(t, ok)

	refreshed, err := cache.RefreshAccountSlot(ctx, 11, "req-a")
	require.NoError(t, err)
	require.True(t, refreshed)
	require.Equal(t, int64(1), client.ZCard(ctx, accountSlotKey(11)).Val())

	require.NoError(t, cache.ReleaseAccountSlot(ctx, 11, "req-a"))
	refreshed, err = cache.RefreshAccountSlot(ctx, 11, "req-a")
	require.NoError(t, err)
	require.False(t, refreshed, "已释放的成员不得被续租重建")
	require.Equal(t, int64(0), client.ZCard(ctx, accountSlotKey(11)).Val(), "取消释放先完成、续租后到达时账号计数必须为 0")
}

func TestRefreshAccountSlotFailsAfterTTLExpiry(t *testing.T) {
	cache, client, server := newRefreshTestCache(t)
	ctx := context.Background()
	ok, err := cache.AcquireAccountSlot(ctx, 12, 1, "req-b")
	require.NoError(t, err)
	require.True(t, ok)

	server.SetTime(time.Now().Add(16 * time.Minute))
	refreshed, err := cache.RefreshAccountSlot(ctx, 12, "req-b")
	require.NoError(t, err)
	require.False(t, refreshed, "跨过槽 TTL 后成员已被清理，续租返回失租")
	require.Equal(t, int64(0), client.ZCard(ctx, accountSlotKey(12)).Val())
}

func TestCleanupStaleProcessSlotsDeletesContinuationWaitKey(t *testing.T) {
	cache, client := newContinuationWaitTestCache(t)
	ctx := context.Background()

	// 预置迁移 marker，确保等待计数删除来自索引驱动路径而非一次性清扫。
	require.NoError(t, client.Set(ctx, legacyWaitSweepMarkerKey, "1", 0).Err())

	ok, err := cache.IncrementAccountContinuationWaitCount(ctx, 14, 3)
	require.NoError(t, err)
	require.True(t, ok)
	_, err = client.ZScore(ctx, accountActiveIndexKey, "14").Result()
	require.NoError(t, err, "只有续聊等待者的账号在清理前必须留在活跃索引")

	require.NoError(t, cache.CleanupStaleProcessSlots(ctx, "keep-"))

	_, err = client.Get(ctx, accountContinuationWaitKey(14)).Result()
	require.ErrorIs(t, err, redis.Nil, "启动清理必须删除续聊等待键，否则会幽灵积压到 TTL 自然过期")
	cont, err := cache.GetAccountContinuationWaitingCount(ctx, 14)
	require.NoError(t, err)
	require.Equal(t, 0, cont)
	_, err = client.ZScore(ctx, accountActiveIndexKey, "14").Result()
	require.ErrorIs(t, err, redis.Nil, "续聊等待键清空后无槽位的账号应从活跃索引移除")

	// 同一场景下旧键既有行为不变：本就没有旧键等待，清理后仍是 0。
	legacy, err := cache.GetAccountWaitingCount(ctx, 14)
	require.NoError(t, err)
	require.Equal(t, 0, legacy)
}

func TestSweepLegacyWaitKeysOnceSkipsContinuationWaitKey(t *testing.T) {
	cache, client := newContinuationWaitTestCache(t)
	ctx := context.Background()

	legacyKey := accountWaitKey(15)
	continuationKey := accountContinuationWaitKey(16)
	require.NoError(t, client.Set(ctx, legacyKey, 5, time.Minute).Err())
	require.NoError(t, client.Set(ctx, continuationKey, 2, time.Minute).Err())

	require.NoError(t, cache.CleanupStaleProcessSlots(ctx, "keep-"))

	_, err := client.Get(ctx, legacyKey).Result()
	require.ErrorIs(t, err, redis.Nil, "一次性清扫仍要删掉未入索引的遗留旧键")
	waiting, err := client.Get(ctx, continuationKey).Int()
	require.NoError(t, err, "续聊等待键是在线计数，wait:account:* 通配不得连带删除")
	require.Equal(t, 2, waiting)
}
