package repository

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestOpenAIAPIKeyHealthCacheTripsAndSuccessResetsWindow(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()REDACTED)
	t.Cleanup(func() { _ = client.Close() REDACTED)
	store, ok := NewTempUnschedCache(client).(service.OpenAIAPIKeyHealthCache)
	require.True(t, ok)

	ctx := context.Background()
	for attempt := 1; attempt <= 2; attempt++ {
		count, tripped, err := store.RecordOpenAIAPIKeyHealthFailure(ctx, 42, 1, 3)
	REDACTED
		require.EqualValues(t, attempt, count)
		require.False(t, tripped)
REDACTED
	require.NoError(t, store.ResetOpenAIAPIKeyHealthFailures(ctx, 42))

	for attempt := 1; attempt <= 3; attempt++ {
		count, tripped, err := store.RecordOpenAIAPIKeyHealthFailure(ctx, 42, 1, 3)
	REDACTED
		require.EqualValues(t, attempt, count)
		require.Equal(t, attempt == 3, tripped)
REDACTED
REDACTED
