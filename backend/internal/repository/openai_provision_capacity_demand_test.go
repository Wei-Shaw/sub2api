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

func TestOpenAIProvisionCapacityDemandStoreTracksDistinctRecentUsers(t *testing.T) {
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	store := NewOpenAIProvisionCapacityDemandStore(client)
	ctx := context.Background()

	require.NoError(t, store.RecordOpenAICapacityDenied(ctx, 11))
	require.NoError(t, store.RecordOpenAICapacityDenied(ctx, 11))
	require.NoError(t, store.RecordOpenAICapacityDenied(ctx, 29))

	count, err := store.GetOpenAICapacityDeniedUsers(ctx, time.Now().UTC())
	require.NoError(t, err)
	require.Equal(t, int64(2), count)

	count, err = store.GetOpenAICapacityDeniedUsers(ctx, time.Now().UTC().Add(service.OpenAIProvisionCapacitySignalWindow+time.Second))
	require.NoError(t, err)
	require.Zero(t, count)
}
