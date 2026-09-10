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

func TestOpenAIAPIKeyHealthCacheTripsWithinRollingWindow(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	store, ok := NewTempUnschedCache(client).(service.OpenAIAPIKeyHealthCache)
	require.True(t, ok)

	ctx := context.Background()
	for attempt := 1; attempt <= 3; attempt++ {
		count, tripped, err := store.RecordOpenAIAPIKeyHealthFailure(ctx, 42, 1, 3)
		require.NoError(t, err)
		require.EqualValues(t, attempt, count)
		require.Equal(t, attempt == 3, tripped)
	}
}

func TestOpenAIAPIKeyHealthCacheDropsFailuresOutsideRollingWindow(t *testing.T) {
	server := miniredis.RunT(t)
	now := time.Unix(1_700_000_000, 0)
	server.SetTime(now)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	store, ok := NewTempUnschedCache(client).(service.OpenAIAPIKeyHealthCache)
	require.True(t, ok)

	ctx := context.Background()
	count, tripped, err := store.RecordOpenAIAPIKeyHealthFailure(ctx, 42, 1, 3)
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	require.False(t, tripped)

	server.SetTime(now.Add(61 * time.Second))
	count, tripped, err = store.RecordOpenAIAPIKeyHealthFailure(ctx, 42, 1, 3)
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	require.False(t, tripped)
}

func TestOpenAIOAuthCapacityCacheTracksConsecutiveOrderedOutcomes(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	cache := NewTempUnschedCache(client)
	apiKeyStore, apiKeyOK := cache.(service.OpenAIAPIKeyHealthCache)
	oauthStore, oauthOK := cache.(service.OpenAIOAuthCapacityFailureCache)
	require.True(t, apiKeyOK)
	require.True(t, oauthOK)

	ctx := context.Background()
	apiKeyCount, apiKeyTripped, err := apiKeyStore.RecordOpenAIAPIKeyHealthFailure(ctx, 42, 1, 2)
	require.NoError(t, err)
	require.EqualValues(t, 1, apiKeyCount)
	require.False(t, apiKeyTripped)

	first, err := oauthStore.BeginOpenAIOAuthCapacityAttempt(ctx, 42)
	require.NoError(t, err)
	count, _, tripped, err := oauthStore.RecordOpenAIOAuthCapacityOutcome(ctx, 42, first, true, 1, 2, 5)
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	require.False(t, tripped)

	success, err := oauthStore.BeginOpenAIOAuthCapacityAttempt(ctx, 42)
	require.NoError(t, err)
	count, _, tripped, err = oauthStore.RecordOpenAIOAuthCapacityOutcome(ctx, 42, success, false, 1, 2, 5)
	require.NoError(t, err)
	require.Zero(t, count)
	require.False(t, tripped)

	third, err := oauthStore.BeginOpenAIOAuthCapacityAttempt(ctx, 42)
	require.NoError(t, err)
	count, _, tripped, err = oauthStore.RecordOpenAIOAuthCapacityOutcome(ctx, 42, third, true, 1, 2, 5)
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	require.False(t, tripped)

	fourth, err := oauthStore.BeginOpenAIOAuthCapacityAttempt(ctx, 42)
	require.NoError(t, err)
	count, tripSequence, tripped, err := oauthStore.RecordOpenAIOAuthCapacityOutcome(ctx, 42, fourth, true, 1, 2, 5)
	require.NoError(t, err)
	require.EqualValues(t, 2, count)
	require.Equal(t, fourth, tripSequence)
	require.True(t, tripped)
}

func TestOpenAIOAuthCapacityCacheIgnoresLateSuccessBeforeNewerFailures(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	store := NewTempUnschedCache(client).(service.OpenAIOAuthCapacityFailureCache)
	ctx := context.Background()

	first, err := store.BeginOpenAIOAuthCapacityAttempt(ctx, 42)
	require.NoError(t, err)
	second, err := store.BeginOpenAIOAuthCapacityAttempt(ctx, 42)
	require.NoError(t, err)
	third, err := store.BeginOpenAIOAuthCapacityAttempt(ctx, 42)
	require.NoError(t, err)

	_, _, tripped, err := store.RecordOpenAIOAuthCapacityOutcome(ctx, 42, second, true, 1, 2, 5)
	require.NoError(t, err)
	require.False(t, tripped)
	count, tripSequence, tripped, err := store.RecordOpenAIOAuthCapacityOutcome(ctx, 42, third, true, 1, 2, 5)
	require.NoError(t, err)
	require.EqualValues(t, 2, count)
	require.Equal(t, third, tripSequence)
	require.True(t, tripped)

	count, tripSequence, tripped, err = store.RecordOpenAIOAuthCapacityOutcome(ctx, 42, first, false, 1, 2, 5)
	require.NoError(t, err)
	require.EqualValues(t, 2, count)
	require.Equal(t, third, tripSequence)
	require.True(t, tripped)
}

func TestOpenAIOAuthCapacityCacheRetriesPendingTransitionAcrossInstances(t *testing.T) {
	server := miniredis.RunT(t)
	clientA := redis.NewClient(&redis.Options{Addr: server.Addr()})
	clientB := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = clientA.Close(); _ = clientB.Close() })
	storeA := NewTempUnschedCache(clientA).(service.OpenAIOAuthCapacityFailureCache)
	storeB := NewTempUnschedCache(clientB).(service.OpenAIOAuthCapacityFailureCache)
	ctx := context.Background()

	first, err := storeA.BeginOpenAIOAuthCapacityAttempt(ctx, 42)
	require.NoError(t, err)
	second, err := storeB.BeginOpenAIOAuthCapacityAttempt(ctx, 42)
	require.NoError(t, err)
	_, _, tripped, err := storeA.RecordOpenAIOAuthCapacityOutcome(ctx, 42, first, true, 1, 2, 5)
	require.NoError(t, err)
	require.False(t, tripped)
	_, tripSequence, tripped, err := storeB.RecordOpenAIOAuthCapacityOutcome(ctx, 42, second, true, 1, 2, 5)
	require.NoError(t, err)
	require.True(t, tripped)

	until := time.Now().Add(5 * time.Minute).Round(0)
	require.NoError(t, storeB.PrepareOpenAIOAuthCapacityCooldown(ctx, 42, tripSequence, until))
	markerSequence, markerUntil, active, err := storeA.GetOpenAIOAuthCapacityCooldown(ctx, 42)
	require.NoError(t, err)
	require.True(t, active)
	require.Equal(t, tripSequence, markerSequence)
	require.Equal(t, until, markerUntil)

	third, err := storeA.BeginOpenAIOAuthCapacityAttempt(ctx, 42)
	require.NoError(t, err)
	_, retriedTrip, tripped, err := storeA.RecordOpenAIOAuthCapacityOutcome(ctx, 42, third, false, 1, 2, 5)
	require.NoError(t, err)
	require.True(t, tripped, "pending publication must survive a later outcome")
	require.Equal(t, tripSequence, retriedTrip)

	require.NoError(t, storeA.AcknowledgeOpenAIOAuthCapacityCooldown(ctx, 42, tripSequence))
	_, _, tripped, err = storeB.RecordOpenAIOAuthCapacityOutcome(ctx, 42, first, true, 1, 2, 5)
	require.NoError(t, err)
	require.False(t, tripped, "acknowledged attempts must not recreate the transition")
}
