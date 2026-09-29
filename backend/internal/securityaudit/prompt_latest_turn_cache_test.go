package securityaudit

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestRedisPayloadStoreLatestTurnDecisionRoundTripAndTTL(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	store := NewRedisPayloadStore(client)
	key := latestTurnDecisionCacheKey(7, "prompt-hash")
	want := &PromptDecision{Kind: DecisionFlag, AllowNextStage: true, Result: &NormalizedResult{Action: ActionWarn, RiskLevel: RiskMedium}}

	require.NoError(t, store.SetLatestTurnDecision(context.Background(), key, want, 2*LatestTurnDecisionCacheTTL))
	got, err := store.GetLatestTurnDecision(context.Background(), key)
	require.NoError(t, err)
	require.Equal(t, want, got)
	ttl, err := client.TTL(context.Background(), key).Result()
	require.NoError(t, err)
	require.Greater(t, ttl, time.Duration(0))
	require.LessOrEqual(t, ttl, LatestTurnDecisionCacheTTL)

	server.FastForward(LatestTurnDecisionCacheTTL)
	got, err = store.GetLatestTurnDecision(context.Background(), key)
	require.NoError(t, err)
	require.Nil(t, got)
}

func TestRedisPayloadStoreLatestTurnDecisionSkipsNonContinuableResults(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	store := NewRedisPayloadStore(client)
	key := latestTurnDecisionCacheKey(7, "blocked-turn")

	require.NoError(t, store.SetLatestTurnDecision(context.Background(), key, &PromptDecision{Kind: DecisionBlock}, LatestTurnDecisionCacheTTL))
	require.Zero(t, client.Exists(context.Background(), key).Val())
	require.NoError(t, store.SetLatestTurnDecision(context.Background(), key, &PromptDecision{Kind: DecisionBlock, AllowNextStage: true}, LatestTurnDecisionCacheTTL))
	require.Zero(t, client.Exists(context.Background(), key).Val())
}
