package repository

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestGatewayCacheOpenAIHealthCircuitReportsOnlyStateTransition(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	cache := &gatewayCache{rdb: client}
	ctx := context.Background()

	count, until, tripped, err := cache.RecordOpenAIAccountFailure(ctx, 601, time.Minute, 30*time.Second, 2)
	require.NoError(t, err)
	require.Equal(t, int64(1), count)
	require.Equal(t, int64(0), until.Unix())
	require.False(t, tripped)

	count, firstUntil, tripped, err := cache.RecordOpenAIAccountFailure(ctx, 601, time.Minute, 30*time.Second, 2)
	require.NoError(t, err)
	require.Equal(t, int64(2), count)
	require.True(t, tripped)
	require.True(t, firstUntil.After(time.Now()))

	count, extendedUntil, tripped, err := cache.RecordOpenAIAccountFailure(ctx, 601, time.Minute, 30*time.Second, 2)
	require.NoError(t, err)
	require.Equal(t, int64(3), count)
	require.False(t, tripped)
	require.False(t, extendedUntil.Before(firstUntil))

	storedUntil, err := cache.GetOpenAIAccountCircuit(ctx, 601)
	require.NoError(t, err)
	require.Equal(t, extendedUntil.Unix(), storedUntil.Unix())
	require.NoError(t, cache.ClearOpenAIAccountFailure(ctx, 601))
	storedUntil, err = cache.GetOpenAIAccountCircuit(ctx, 601)
	require.NoError(t, err)
	require.True(t, storedUntil.IsZero())
}
