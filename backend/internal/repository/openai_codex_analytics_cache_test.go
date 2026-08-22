package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestCodexAnalyticsRedisCacheMissTranslation(t *testing.T) {
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	cache := NewCodexAnalyticsRedisCache(client)

	value, ttl, err := cache.Get(context.Background(), "missing")

	require.Nil(t, value)
	require.Zero(t, ttl)
	require.ErrorIs(t, err, service.ErrCodexAnalyticsCacheMiss)
}

func TestCodexAnalyticsRedisCacheReadWriteTTLAndDelete(t *testing.T) {
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	cache := NewCodexAnalyticsRedisCache(client)
	ctx := context.Background()
	const key = "codex_analytics:v2:100:recent:7"
	value := []byte(`{"account_id":100}`)
	wantTTL := 4 * time.Minute

	require.NoError(t, cache.Set(ctx, key, value, wantTTL))
	got, ttl, err := cache.Get(ctx, key)
	require.NoError(t, err)
	require.Equal(t, value, got)
	require.Equal(t, wantTTL, ttl)

	mini.FastForward(time.Minute)
	_, ttl, err = cache.Get(ctx, key)
	require.NoError(t, err)
	require.Equal(t, 3*time.Minute, ttl)

	require.NoError(t, cache.Delete(ctx, key))
	_, _, err = cache.Get(ctx, key)
	require.True(t, errors.Is(err, service.ErrCodexAnalyticsCacheMiss))
}
