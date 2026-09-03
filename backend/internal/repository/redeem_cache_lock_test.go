//go:build unit

package repository

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func newRedeemCacheForTest(t *testing.T) (*redeemCache, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return &redeemCache{rdb: rdb}, mr
}

// 锁 TTL 到期后被下一个请求接管时，原持有者的 defer release 不得删除新持有者的锁。
func TestRedeemLock_ReleaseIsScopedToOwnerToken(t *testing.T) {
	ctx := context.Background()
	cache, mr := newRedeemCacheForTest(t)
	key := redeemLockKey("CODE-A")

	ok, err := cache.AcquireRedeemLock(ctx, "CODE-A", "token-first", 50*time.Millisecond)
	require.NoError(t, err)
	require.True(t, ok)

	// 第一个请求的事务超过了锁 TTL。
	mr.FastForward(100 * time.Millisecond)
	require.False(t, mr.Exists(key))

	ok, err = cache.AcquireRedeemLock(ctx, "CODE-A", "token-second", time.Minute)
	require.NoError(t, err)
	require.True(t, ok, "锁过期后第二个请求应能抢到")

	// 第一个请求此时才走到 defer release。
	require.NoError(t, cache.ReleaseRedeemLock(ctx, "CODE-A", "token-first"))

	value, err := cache.rdb.Get(ctx, key).Result()
	require.NoError(t, err, "第二个持有者的锁被误删了")
	require.Equal(t, "token-second", value)

	require.NoError(t, cache.ReleaseRedeemLock(ctx, "CODE-A", "token-second"))
	require.False(t, mr.Exists(key))
}

// 空 token（Redis 故障降级路径）不得删除别人持有的锁。
func TestRedeemLock_EmptyTokenNeverReleases(t *testing.T) {
	ctx := context.Background()
	cache, mr := newRedeemCacheForTest(t)
	key := redeemLockKey("CODE-B")

	ok, err := cache.AcquireRedeemLock(ctx, "CODE-B", "token-holder", time.Minute)
	require.NoError(t, err)
	require.True(t, ok)

	require.NoError(t, cache.ReleaseRedeemLock(ctx, "CODE-B", ""))
	require.True(t, mr.Exists(key), "降级路径不应删除他人的锁")
}

// 真实并发：多个 goroutine 同时抢同一把锁，只有一个能成功；
// 每个 goroutine 都在结束时用自己的 token 释放，互不干扰，
// 且临界区在任何时刻只允许一个持有者。
func TestRedeemLock_ConcurrentHoldersAreMutuallyExclusive(t *testing.T) {
	ctx := context.Background()
	cache, _ := newRedeemCacheForTest(t)

	const goroutines = 24
	const rounds = 8

	var (
		start     sync.WaitGroup
		done      sync.WaitGroup
		acquired  int64
		inSection int64
		overlaps  int64
	)
	start.Add(1)
	done.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer done.Done()
			start.Wait()
			for r := 0; r < rounds; r++ {
				token := fmt.Sprintf("g%d-r%d", id, r)
				ok, err := cache.AcquireRedeemLock(ctx, "RACE", token, time.Minute)
				if err != nil {
					t.Errorf("acquire: %v", err)
					return
				}
				if !ok {
					continue
				}
				atomic.AddInt64(&acquired, 1)
				if atomic.AddInt64(&inSection, 1) != 1 {
					atomic.AddInt64(&overlaps, 1)
				}
				// 临界区
				atomic.AddInt64(&inSection, -1)
				if err := cache.ReleaseRedeemLock(ctx, "RACE", token); err != nil {
					t.Errorf("release: %v", err)
					return
				}
			}
		}(i)
	}

	start.Done()
	done.Wait()

	require.Zero(t, atomic.LoadInt64(&overlaps), "同一时刻不应有多个锁持有者")
	require.Positive(t, atomic.LoadInt64(&acquired), "应至少有一次成功抢锁")
}
