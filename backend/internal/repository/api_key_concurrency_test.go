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

func TestAPIKeyConcurrencyAtomicAdmission(t *testing.T) {
	r := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: r.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	cache, ok := NewConcurrencyCache(client, 1, 60).(*concurrencyCache)
	require.True(t, ok)
	ctx := context.Background()
	var admitted atomic.Int64
	var failures atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ok, err := cache.AcquireAPIKeySlot(ctx, 7, 2, fmt.Sprintf("request-%d", i))
			if err != nil {
				failures.Add(1)
			}
			if ok {
				admitted.Add(1)
			}
		}(i)
	}
	wg.Wait()
	require.Zero(t, failures.Load())
	require.EqualValues(t, 2, admitted.Load())
	counts, err := cache.GetAPIKeyConcurrencyBatch(ctx, []int64{7})
	require.NoError(t, err)
	require.Equal(t, 2, counts[7])
	// A different key has independent capacity.
	ok, err = cache.AcquireAPIKeySlot(ctx, 8, 1, "another-key")
	require.NoError(t, err)
	require.True(t, ok)
	// Zero disables the limit, without disabling tracking.
	for i := 0; i < 4; i++ {
		ok, err = cache.AcquireAPIKeySlot(ctx, 7, 0, fmt.Sprintf("unlimited-%d", i))
		require.NoError(t, err)
		require.True(t, ok)
	}
	// Lowering the limit does not remove existing requests.
	ok, err = cache.AcquireAPIKeySlot(ctx, 7, 1, "after-lowering")
	require.NoError(t, err)
	require.False(t, ok)
	counts, err = cache.GetAPIKeyConcurrencyBatch(ctx, []int64{7})
	require.NoError(t, err)
	require.Equal(t, 6, counts[7])
}

func TestAPIKeyConcurrencyRenewalAndCrashRecovery(t *testing.T) {
	r := miniredis.RunT(t)
	now := time.Now()
	r.SetTime(now)
	client := redis.NewClient(&redis.Options{Addr: r.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	cache, ok := NewConcurrencyCache(client, 1, 60).(*concurrencyCache)
	require.True(t, ok)
	ctx := context.Background()
	ok, err := cache.AcquireAPIKeySlot(ctx, 7, 1, "long-request")
	require.NoError(t, err)
	require.True(t, ok)
	for i := 1; i <= 4; i++ {
		r.SetTime(now.Add(time.Duration(i) * 40 * time.Second))
		ok, err = cache.RefreshAPIKeySlot(ctx, 7, "long-request")
		require.NoError(t, err)
		require.True(t, ok)
		ok, err = cache.AcquireAPIKeySlot(ctx, 7, 1, "blocked")
		require.NoError(t, err)
		require.False(t, ok)
	}
	require.NoError(t, cache.ReleaseAPIKeySlot(ctx, 7, "long-request"))
	ok, err = cache.RefreshAPIKeySlot(ctx, 7, "long-request")
	require.NoError(t, err)
	require.False(t, ok, "renewal must never resurrect a released request")
	ok, err = cache.AcquireAPIKeySlot(ctx, 7, 1, "crashed")
	require.NoError(t, err)
	require.True(t, ok)
	r.SetTime(now.Add(221 * time.Second))
	ok, err = cache.AcquireAPIKeySlot(ctx, 7, 1, "replacement")
	require.NoError(t, err)
	require.True(t, ok, "a dead process must not leak capacity indefinitely")
}
