//go:build unit

package repository

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestRPMReservationConcurrentHardLimit(t *testing.T) {
	server := miniredis.RunT(t)
	server.SetTime(time.Date(2026, 10, 13, 22, 21, 59, 0, time.UTC))
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	cache := &RPMCacheImpl{rdb: client}
	ctx := context.Background()
	var admitted atomic.Int32
	var failures atomic.Int32
	var wg sync.WaitGroup
	for range 200 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := cache.ReserveRPM(ctx, 1, 100)
			if err != nil {
				failures.Add(1)
			} else if ok {
				admitted.Add(1)
			}
		}()
	}
	wg.Wait()
	require.Zero(t, failures.Load())
	require.EqualValues(t, 100, admitted.Load())
	count, err := cache.GetRPM(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, 100, count)
	batch, err := cache.GetRPMBatch(ctx, []int64{1, 2})
	require.NoError(t, err)
	require.Equal(t, map[int64]int{1: 100, 2: 0}, batch)
}

func TestRPMReservationRollingBoundaryAndRejectedRequests(t *testing.T) {
	server := miniredis.RunT(t)
	start := time.Date(2026, 10, 13, 22, 21, 59, 0, time.UTC)
	server.SetTime(start)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	cache := &RPMCacheImpl{rdb: client}
	ctx := context.Background()
	ok, err := cache.ReserveRPM(ctx, 1, 1)
	require.NoError(t, err)
	require.True(t, ok)
	server.SetTime(start.Add(time.Second)) // calendar minute changes, quota does not
	ok, err = cache.ReserveRPM(ctx, 1, 1)
	require.NoError(t, err)
	require.False(t, ok)
	server.SetTime(start.Add(59 * time.Second))
	for range 50 {
		ok, err = cache.ReserveRPM(ctx, 1, 1)
		require.NoError(t, err)
		require.False(t, ok)
	}
	server.SetTime(start.Add(time.Minute))
	ok, err = cache.ReserveRPM(ctx, 1, 1)
	require.NoError(t, err)
	require.True(t, ok, "rejected traffic must not keep the rolling window full")
	ok, err = cache.ReserveRPM(ctx, 2, 1)
	require.NoError(t, err)
	require.True(t, ok, "account quotas must be isolated")
}

func TestRPMReservationCacheFailureDoesNotAdmit(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	require.NoError(t, client.Close())
	cache := &RPMCacheImpl{rdb: client}
	ok, err := cache.ReserveRPM(context.Background(), 1, 100)
	require.Error(t, err)
	require.False(t, ok)
}

func TestRPMReservationPreservesLegacyCounter(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	cache := &RPMCacheImpl{rdb: client}
	ctx := context.Background()
	_, err := cache.IncrementRPM(ctx, 4)
	require.NoError(t, err)
	count, err := cache.GetRPM(ctx, 4)
	require.NoError(t, err)
	require.Equal(t, 1, count)
	batch, err := cache.GetRPMBatch(ctx, []int64{4, 5})
	require.NoError(t, err)
	require.Equal(t, map[int64]int{4: 1, 5: 0}, batch)
}
