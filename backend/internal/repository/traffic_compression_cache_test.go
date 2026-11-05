package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/compresslevel"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func newTrafficCompressionCacheTest(t *testing.T) (*miniredis.Miniredis, service.TrafficCompressionCache) {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr(), MaxRetries: -1})
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	return server, NewTrafficCompressionCache(client)
}

func TestTrafficCompressionCacheRoundTripAndExpiry(t *testing.T) {
	server, cache := newTrafficCompressionCacheTest(t)
	ctx := context.Background()
	got, err := cache.Get(ctx)
	require.NoError(t, err)
	require.Nil(t, got)

	levels := compresslevel.Settings{Brotli: 7, Zstd: 9, Gzip: 4, Deflate: 2}
	require.NoError(t, cache.Set(ctx, levels))
	got, err = cache.Get(ctx)
	require.NoError(t, err)
	require.Equal(t, &levels, got)
	ttl := server.TTL(trafficCompressionRedisKey)
	require.Greater(t, ttl, 4*time.Minute)
	require.LessOrEqual(t, ttl, trafficCompressionRedisTTL)

	server.FastForward(trafficCompressionRedisTTL + time.Second)
	got, err = cache.Get(ctx)
	require.NoError(t, err)
	require.Nil(t, got)
}

func TestTrafficCompressionCacheRejectsInvalidValues(t *testing.T) {
	for _, raw := range []string{
		"not-json",
		`{"brotli":12,"zstd":3,"gzip":6,"deflate":6}`,
	} {
		t.Run(raw, func(t *testing.T) {
			server, cache := newTrafficCompressionCacheTest(t)
			require.NoError(t, server.Set(trafficCompressionRedisKey, raw))
			got, err := cache.Get(context.Background())
			require.Error(t, err)
			require.Nil(t, got)
		})
	}
}

func TestTrafficCompressionCachePropagatesRedisErrors(t *testing.T) {
	server, cache := newTrafficCompressionCacheTest(t)
	server.SetError("ERR cache unavailable")
	got, err := cache.Get(context.Background())
	require.Error(t, err)
	require.Nil(t, got)
	require.Error(t, cache.Set(context.Background(), compresslevel.Default()))
}

func TestTrafficCompressionCacheWithoutRedis(t *testing.T) {
	cache := NewTrafficCompressionCache(nil)
	got, err := cache.Get(context.Background())
	require.NoError(t, err)
	require.Nil(t, got)
	require.NoError(t, cache.Set(context.Background(), compresslevel.Default()))
}
