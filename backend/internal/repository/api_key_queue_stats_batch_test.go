package repository

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestAPIKeyQueueStatsBatch(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr(), ContextTimeoutEnabled: true})
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	cache, ok := NewConcurrencyCache(client, 15, 60).(*concurrencyCache)
	require.True(t, ok, "expected Redis concurrency cache")
	ctx := context.Background()
	now := time.Now()
	for _, id := range []int64{1, 2} {
		require.NoError(t, client.ZAdd(ctx, apiKeySlotKey(id), redis.Z{Score: float64(now.Unix()), Member: "regular"}).Err())
		require.NoError(t, client.ZAdd(ctx, liveAPIKeySlotKey(id), redis.Z{Score: float64(now.Unix()), Member: "live"}).Err())
		require.NoError(t, client.ZAdd(ctx, apiKeyWaitKey(id), redis.Z{Score: float64(now.Add(time.Minute).UnixMilli()), Member: "waiting"}, redis.Z{Score: 1, Member: "expired"}).Err())
	}
	// Exercise cold cache, warm cache, then loss of the script cache.
	for attempt := 0; attempt < 3; attempt++ {
		if attempt == 2 {
			require.NoError(t, client.ScriptFlush(ctx).Err())
		}
		counts, err := cache.GetAPIKeyQueueStatsBatch(ctx, []int64{1, 2, 3})
		require.NoError(t, err)
		require.Len(t, counts, 3)
		require.Equal(t, 2, counts[1].Active)
		require.Equal(t, 1, counts[1].Waiting)
		require.Equal(t, counts[1], counts[2])
		require.Zero(t, counts[3].Active)
	}
	// A failed key invalidates the entire snapshot; no partial or fake zeros.
	require.NoError(t, client.Set(ctx, apiKeySlotKey(2), "wrong type", 0).Err())
	counts, err := cache.GetAPIKeyQueueStatsBatch(ctx, []int64{1, 2})
	require.Error(t, err)
	require.Nil(t, counts)
	require.NoError(t, client.ScriptFlush(ctx).Err())
	counts, err = cache.GetAPIKeyQueueStatsBatch(ctx, []int64{1, 2})
	require.Error(t, err)
	require.Nil(t, counts)
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	counts, err = cache.GetAPIKeyQueueStatsBatch(canceled, []int64{1})
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, counts)
}
