package repository

import (
	"context"
	"fmt"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestUserAPIKeySlotsEnforceKeyAndUserLimitsAtomically(t *testing.T) {
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	cache := NewConcurrencyCache(client, 15, 900)
	limiter, ok := cache.(service.APIKeyConcurrencyLimiterCache)
	require.True(t, ok)
	keyStats, ok := cache.(service.APIKeyConcurrencyCache)
	require.True(t, ok)
	ctx := context.Background()

	userID := int64(100)
	keyA := int64(201)
	keyB := int64(202)
	keyC := int64(203)

	acquire := func(keyID int64, keyMax int, requestID string) {
		t.Helper()
		acquired, blockedSlotType, err := limiter.AcquireUserAPIKeySlot(
			ctx,
			userID,
			10,
			keyID,
			keyMax,
			requestID,
		)
		require.NoError(t, err)
		require.True(t, acquired)
		require.Empty(t, blockedSlotType)
	}

	for i := 0; i < 4; i++ {
		acquire(keyA, 4, fmt.Sprintf("a-%d", i))
	}
	acquired, blockedSlotType, err := limiter.AcquireUserAPIKeySlot(ctx, userID, 10, keyA, 4, "a-blocked")
	require.NoError(t, err)
	require.False(t, acquired)
	require.Equal(t, "api_key", blockedSlotType)

	for i := 0; i < 3; i++ {
		acquire(keyB, 3, fmt.Sprintf("b-%d", i))
		acquire(keyC, 3, fmt.Sprintf("c-%d", i))
	}

	userConcurrency, err := cache.GetUserConcurrency(ctx, userID)
	require.NoError(t, err)
	require.Equal(t, 10, userConcurrency)
	keyConcurrency, err := keyStats.GetAPIKeyConcurrencyBatch(ctx, []int64{keyA, keyB, keyC})
	require.NoError(t, err)
	require.Equal(t, map[int64]int{keyA: 4, keyB: 3, keyC: 3}, keyConcurrency)

	acquired, blockedSlotType, err = limiter.AcquireUserAPIKeySlot(ctx, userID, 10, int64(204), 2, "user-blocked")
	require.NoError(t, err)
	require.False(t, acquired)
	require.Equal(t, "user", blockedSlotType)

	require.NoError(t, limiter.ReleaseUserAPIKeySlot(ctx, userID, keyA, "a-0"))
	acquire(int64(204), 2, "after-release")
}
