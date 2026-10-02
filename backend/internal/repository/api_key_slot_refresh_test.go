package repository

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestAPIKeySlotRefreshKeepsLiveMemberWithoutRecreation(t *testing.T) {
	server := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	cache := &concurrencyCache{rdb: rdb, slotTTLSeconds: 6}
	require.Equal(t, 2*time.Second, cache.APIKeySlotRefreshInterval())
	ctx := context.Background()
	now := time.Unix(1700000000, 0)
	server.SetTime(now)
	require.NoError(t, cache.TrackAPIKeySlot(ctx, 8, "live"))
	require.NoError(t, cache.TrackAPIKeySlot(ctx, 8, "crashed"))
	for i := 0; i < 6; i++ {
		now = now.Add(2 * time.Second)
		server.FastForward(2 * time.Second)
		server.SetTime(now)
		ok, err := cache.RefreshAPIKeySlot(ctx, 8, "live")
		require.NoError(t, err)
		require.True(t, ok)
	}
	counts, err := cache.GetAPIKeyConcurrencyBatch(ctx, []int64{8})
	require.NoError(t, err)
	require.Equal(t, 1, counts[8], "live score survives original TTL, crashed member expires")
	require.NoError(t, cache.ReleaseAPIKeySlot(ctx, 8, "live"))
	ok, err := cache.RefreshAPIKeySlot(ctx, 8, "live")
	require.NoError(t, err)
	require.False(t, ok)
	require.False(t, server.Exists(apiKeySlotKey(8)))
	// A missing member inside a still-existing key is not recreated either.
	require.NoError(t, cache.TrackAPIKeySlot(ctx, 8, "other"))
	ok, err = cache.RefreshAPIKeySlot(ctx, 8, "live")
	require.NoError(t, err)
	require.False(t, ok)
	require.NoError(t, rdb.Del(ctx, apiKeySlotKey(8)).Err())
	ok, err = cache.RefreshAPIKeySlot(ctx, 8, "other")
	require.NoError(t, err)
	require.False(t, ok)
	require.False(t, server.Exists(apiKeySlotKey(8)))
}
