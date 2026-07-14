package repository

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func newUsageBillingQueueTestRepository(t *testing.T) (*queuedUsageBillingRepository, *redis.Client, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return &queuedUsageBillingRepository{
		rdb:                    rdb,
		dedupRetention:         time.Hour,
		globalDBMaxConcurrency: 1,
		commandTimeout:         100 * time.Millisecond,
	}, rdb, mr
}

func TestUsageBillingQueueSharedRedisDeduplicatesAcrossInstances(t *testing.T) {
	repoA, rdb, _ := newUsageBillingQueueTestRepository(t)
	repoB := &queuedUsageBillingRepository{rdb: rdb, dedupRetention: time.Hour}
	ctx := context.Background()

	require.NoError(t, rdb.Set(ctx, billingBalanceKey(11), 10.0, billingCacheTTL).Err())
	require.NoError(t, rdb.HSet(ctx, billingSubKey(11, 22), map[string]any{
		subFieldStatus:       service.SubscriptionStatusActive,
		subFieldDailyUsage:   1.0,
		subFieldWeeklyUsage:  1.0,
		subFieldMonthlyUsage: 1.0,
	}).Err())

	cmd := &service.UsageBillingCommand{
		RequestID:           "shared-request",
		APIKeyID:            33,
		UserID:              11,
		GroupID:             22,
		BalanceCost:         2.5,
		SubscriptionCost:    3.5,
		APIKeyQuotaCost:     4.5,
		APIKeyRateLimitCost: 5.5,
	}
	cmd.Normalize()
	payload, err := json.Marshal(cmd)
	require.NoError(t, err)

	enqueued, err := repoA.enqueue(ctx, cmd, payload)
	require.NoError(t, err)
	require.True(t, enqueued)

	enqueued, err = repoB.enqueue(ctx, cmd, payload)
	require.NoError(t, err)
	require.False(t, enqueued)
	require.EqualValues(t, 1, rdb.XLen(ctx, usageBillingStreamKey).Val())
	require.InDelta(t, 7.5, mustRedisFloat(t, rdb, billingBalanceKey(11)), 1e-9)
	require.InDelta(t, 4.5, mustRedisFloat(t, rdb, usageBillingPendingAPIKeyQuotaKey(33)), 1e-9)
	require.InDelta(t, 5.5, mustRedisFloat(t, rdb, usageBillingPendingAPIKeyRateLimitKey(33)), 1e-9)

	sub, err := rdb.HGetAll(ctx, billingSubKey(11, 22)).Result()
	require.NoError(t, err)
	require.Equal(t, "4.5", sub[subFieldDailyUsage])
}

func TestUsageBillingQueueRejectsCrossInstanceFingerprintConflict(t *testing.T) {
	repoA, rdb, _ := newUsageBillingQueueTestRepository(t)
	repoB := &queuedUsageBillingRepository{rdb: rdb, dedupRetention: time.Hour}
	ctx := context.Background()

	first := &service.UsageBillingCommand{RequestID: "conflicting-request", APIKeyID: 8, UserID: 1, BalanceCost: 1}
	first.Normalize()
	firstPayload, err := json.Marshal(first)
	require.NoError(t, err)
	enqueued, err := repoA.enqueue(ctx, first, firstPayload)
	require.NoError(t, err)
	require.True(t, enqueued)

	conflict := &service.UsageBillingCommand{RequestID: first.RequestID, APIKeyID: first.APIKeyID, UserID: 1, BalanceCost: 2}
	conflict.Normalize()
	conflictPayload, err := json.Marshal(conflict)
	require.NoError(t, err)
	_, err = repoB.enqueue(ctx, conflict, conflictPayload)
	require.ErrorIs(t, err, service.ErrUsageBillingRequestConflict)
	require.EqualValues(t, 1, rdb.XLen(ctx, usageBillingStreamKey).Val())
}

func TestUsageBillingQueueColdBalanceUsesPendingOverlay(t *testing.T) {
	repo, rdb, _ := newUsageBillingQueueTestRepository(t)
	ctx := context.Background()
	cmd := &service.UsageBillingCommand{RequestID: "cold-balance", APIKeyID: 7, UserID: 9, BalanceCost: 2.25}
	cmd.Normalize()
	payload, err := json.Marshal(cmd)
	require.NoError(t, err)

	enqueued, err := repo.enqueue(ctx, cmd, payload)
	require.NoError(t, err)
	require.True(t, enqueued)

	cache := &billingCache{rdb: rdb}
	require.NoError(t, cache.SetUserBalance(ctx, 9, 10))
	balance, err := cache.GetUserBalance(ctx, 9)
	require.NoError(t, err)
	require.InDelta(t, 7.75, balance, 1e-9)
}

func TestUsageBillingQueueColdSubscriptionAndRateLimitUsePendingOverlay(t *testing.T) {
	repo, rdb, _ := newUsageBillingQueueTestRepository(t)
	ctx := context.Background()
	cmd := &service.UsageBillingCommand{
		RequestID:           "cold-subscription-rate",
		APIKeyID:            17,
		UserID:              15,
		GroupID:             16,
		SubscriptionCost:    2.5,
		APIKeyRateLimitCost: 3.5,
	}
	cmd.Normalize()
	payload, err := json.Marshal(cmd)
	require.NoError(t, err)
	enqueued, err := repo.enqueue(ctx, cmd, payload)
	require.NoError(t, err)
	require.True(t, enqueued)

	cache := &billingCache{rdb: rdb}
	require.NoError(t, cache.SetSubscriptionCache(ctx, 15, 16, &service.SubscriptionCacheData{
		Status:       service.SubscriptionStatusActive,
		DailyUsage:   1,
		WeeklyUsage:  2,
		MonthlyUsage: 3,
	}))
	subscription, err := cache.GetSubscriptionCache(ctx, 15, 16)
	require.NoError(t, err)
	require.InDelta(t, 3.5, subscription.DailyUsage, 1e-9)
	require.InDelta(t, 4.5, subscription.WeeklyUsage, 1e-9)
	require.InDelta(t, 5.5, subscription.MonthlyUsage, 1e-9)

	now := time.Now().Unix()
	require.NoError(t, cache.SetAPIKeyRateLimit(ctx, 17, &service.APIKeyRateLimitCacheData{
		Usage5h:  1,
		Usage1d:  2,
		Usage7d:  3,
		Window5h: now,
		Window1d: now,
		Window7d: now,
	}))
	rateLimit, err := cache.GetAPIKeyRateLimit(ctx, 17)
	require.NoError(t, err)
	require.InDelta(t, 4.5, rateLimit.Usage5h, 1e-9)
	require.InDelta(t, 5.5, rateLimit.Usage1d, 1e-9)
	require.InDelta(t, 6.5, rateLimit.Usage7d, 1e-9)
}

func TestUsageBillingQueueAmbiguousEnqueueFallbackDoesNotApplyCacheDeltaTwice(t *testing.T) {
	repo, rdb, _ := newUsageBillingQueueTestRepository(t)
	repo.direct = usageBillingDirectStub{result: &service.UsageBillingApplyResult{Applied: true}}
	repo.fallbackDBSemaphore = make(chan struct{}, 1)
	ctx := context.Background()

	// A non-numeric balance makes the Lua script fail before XADD. The same
	// fallback behavior is used for ambiguous network failures after execution.
	require.NoError(t, rdb.Set(ctx, billingBalanceKey(5), "corrupt", billingCacheTTL).Err())
	cmd := &service.UsageBillingCommand{RequestID: "fallback", APIKeyID: 6, UserID: 5, BalanceCost: 1}
	result, err := repo.Apply(ctx, cmd)
	require.NoError(t, err)
	require.True(t, result.Applied)
	require.True(t, result.Deferred)
	require.Zero(t, rdb.Exists(ctx, billingBalanceKey(5)).Val())
	require.Positive(t, mustRedisInt(t, rdb, usageBillingBalanceMutationKey(5)))
}

func TestUsageBillingQueueCompleteIsIdempotentAcrossConsumers(t *testing.T) {
	repo, rdb, _ := newUsageBillingQueueTestRepository(t)
	ctx := context.Background()
	require.NoError(t, repo.ensureConsumerGroup(ctx))

	cmd := &service.UsageBillingCommand{
		RequestID:           "complete-once",
		APIKeyID:            4,
		UserID:              2,
		GroupID:             3,
		BalanceCost:         1.25,
		APIKeyQuotaCost:     2.5,
		APIKeyRateLimitCost: 3.75,
	}
	cmd.Normalize()
	payload, err := json.Marshal(cmd)
	require.NoError(t, err)
	enqueued, err := repo.enqueue(ctx, cmd, payload)
	require.NoError(t, err)
	require.True(t, enqueued)
	require.Equal(t, time.Duration(-1), rdb.TTL(ctx, usageBillingEnqueueDedupKey(cmd.RequestID, cmd.APIKeyID)).Val())

	streams, err := rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    usageBillingConsumerGroup,
		Consumer: "host-a",
		Streams:  []string{usageBillingStreamKey, ">"},
		Count:    1,
		Block:    -1,
	}).Result()
	require.NoError(t, err)
	require.Len(t, streams, 1)
	require.Len(t, streams[0].Messages, 1)
	message := streams[0].Messages[0]

	require.NoError(t, repo.complete(ctx, message, cmd, true))
	require.NoError(t, repo.complete(ctx, message, cmd, true))
	require.Zero(t, rdb.XLen(ctx, usageBillingStreamKey).Val())
	require.Zero(t, mustRedisFloat(t, rdb, usageBillingPendingBalanceKey(2)))
	require.Zero(t, mustRedisFloat(t, rdb, usageBillingPendingAPIKeyQuotaKey(4)))
	require.Zero(t, mustRedisFloat(t, rdb, usageBillingPendingAPIKeyRateLimitKey(4)))
	require.Positive(t, rdb.TTL(ctx, usageBillingEnqueueDedupKey(cmd.RequestID, cmd.APIKeyID)).Val())

	payload, err = json.Marshal(cmd)
	require.NoError(t, err)
	enqueued, err = repo.enqueue(ctx, cmd, payload)
	require.NoError(t, err)
	require.False(t, enqueued)
	require.Zero(t, rdb.XLen(ctx, usageBillingStreamKey).Val())
}

func TestUsageBillingQueueDuplicateDBResultInvalidatesOptimisticCaches(t *testing.T) {
	repo, rdb, _ := newUsageBillingQueueTestRepository(t)
	ctx := context.Background()
	require.NoError(t, repo.ensureConsumerGroup(ctx))
	require.NoError(t, rdb.Set(ctx, billingBalanceKey(12), 10.0, billingCacheTTL).Err())
	require.NoError(t, rdb.HSet(ctx, billingSubKey(12, 13), map[string]any{
		subFieldStatus:       service.SubscriptionStatusActive,
		subFieldDailyUsage:   1.0,
		subFieldWeeklyUsage:  1.0,
		subFieldMonthlyUsage: 1.0,
	}).Err())

	cmd := &service.UsageBillingCommand{
		RequestID:        "db-duplicate",
		APIKeyID:         14,
		UserID:           12,
		GroupID:          13,
		BalanceCost:      2,
		SubscriptionCost: 3,
	}
	cmd.Normalize()
	payload, err := json.Marshal(cmd)
	require.NoError(t, err)
	enqueued, err := repo.enqueue(ctx, cmd, payload)
	require.NoError(t, err)
	require.True(t, enqueued)
	require.InDelta(t, 8, mustRedisFloat(t, rdb, billingBalanceKey(12)), 1e-9)

	streams, err := rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    usageBillingConsumerGroup,
		Consumer: "host-duplicate",
		Streams:  []string{usageBillingStreamKey, ">"},
		Count:    1,
		Block:    -1,
	}).Result()
	require.NoError(t, err)
	message := streams[0].Messages[0]
	require.NoError(t, repo.complete(ctx, message, cmd, false))
	require.Zero(t, rdb.Exists(ctx, billingBalanceKey(12), billingSubKey(12, 13)).Val())
	require.Positive(t, rdb.TTL(ctx, usageBillingEnqueueDedupKey(cmd.RequestID, cmd.APIKeyID)).Val())
}

func TestUsageBillingQueueRejectsStaleCacheFillAfterCompletion(t *testing.T) {
	repo, rdb, _ := newUsageBillingQueueTestRepository(t)
	ctx := context.Background()
	require.NoError(t, repo.ensureConsumerGroup(ctx))

	cmd := &service.UsageBillingCommand{
		RequestID:           "stale-cache-fill",
		APIKeyID:            23,
		UserID:              21,
		GroupID:             22,
		BalanceCost:         1,
		SubscriptionCost:    2,
		APIKeyRateLimitCost: 3,
	}
	cmd.Normalize()
	payload, err := json.Marshal(cmd)
	require.NoError(t, err)
	enqueued, err := repo.enqueue(ctx, cmd, payload)
	require.NoError(t, err)
	require.True(t, enqueued)

	cache := &billingCache{rdb: rdb}
	balanceVersion, err := cache.GetUserBalanceMutationVersion(ctx, cmd.UserID)
	require.NoError(t, err)
	subscriptionVersion, err := cache.GetSubscriptionMutationVersion(ctx, cmd.UserID, cmd.GroupID)
	require.NoError(t, err)
	rateLimitVersion, err := cache.GetAPIKeyRateLimitMutationVersion(ctx, cmd.APIKeyID)
	require.NoError(t, err)
	require.EqualValues(t, 1, balanceVersion)
	require.EqualValues(t, 1, subscriptionVersion)
	require.EqualValues(t, 1, rateLimitVersion)

	streams, err := rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    usageBillingConsumerGroup,
		Consumer: "host-stale-fill",
		Streams:  []string{usageBillingStreamKey, ">"},
		Count:    1,
		Block:    -1,
	}).Result()
	require.NoError(t, err)
	require.NoError(t, repo.complete(ctx, streams[0].Messages[0], cmd, true))

	written, err := cache.SetUserBalanceIfMutationVersion(ctx, cmd.UserID, 100, balanceVersion)
	require.NoError(t, err)
	require.False(t, written)
	written, err = cache.SetSubscriptionCacheIfMutationVersion(ctx, cmd.UserID, cmd.GroupID, &service.SubscriptionCacheData{
		Status: service.SubscriptionStatusActive,
	}, subscriptionVersion)
	require.NoError(t, err)
	require.False(t, written)
	written, err = cache.SetAPIKeyRateLimitIfMutationVersion(ctx, cmd.APIKeyID, &service.APIKeyRateLimitCacheData{}, rateLimitVersion)
	require.NoError(t, err)
	require.False(t, written)
	require.Zero(t, rdb.Exists(ctx,
		billingBalanceKey(cmd.UserID),
		billingSubKey(cmd.UserID, cmd.GroupID),
		billingRateLimitKey(cmd.APIKeyID),
	).Val())
}

func TestUsageBillingQueueGlobalDBSemaphoreSharedAcrossInstances(t *testing.T) {
	repoA, rdb, _ := newUsageBillingQueueTestRepository(t)
	repoB := &queuedUsageBillingRepository{
		rdb:                    rdb,
		globalDBMaxConcurrency: 1,
		commandTimeout:         100 * time.Millisecond,
	}

	require.NoError(t, repoA.acquireGlobalDBSlot(context.Background(), "host-a:1"))
	waitCtx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	err := repoB.acquireGlobalDBSlot(waitCtx, "host-b:1")
	require.Error(t, err)
	require.True(t, errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled))

	repoA.releaseGlobalDBSlot("host-a:1")
	require.NoError(t, repoB.acquireGlobalDBSlot(context.Background(), "host-b:2"))
	repoB.releaseGlobalDBSlot("host-b:2")
}

func mustRedisFloat(t *testing.T, rdb *redis.Client, key string) float64 {
	t.Helper()
	value, err := rdb.Get(context.Background(), key).Float64()
	if errors.Is(err, redis.Nil) {
		return 0
	}
	require.NoError(t, err)
	return value
}

func mustRedisInt(t *testing.T, rdb *redis.Client, key string) int64 {
	t.Helper()
	value, err := rdb.Get(context.Background(), key).Int64()
	if errors.Is(err, redis.Nil) {
		return 0
	}
	require.NoError(t, err)
	return value
}

type usageBillingDirectStub struct {
	result *service.UsageBillingApplyResult
	err    error
}

func (s usageBillingDirectStub) Apply(context.Context, *service.UsageBillingCommand) (*service.UsageBillingApplyResult, error) {
	return s.result, s.err
}

func (s usageBillingDirectStub) ReserveBatchImageBalance(context.Context, *service.BatchImageBalanceHoldCommand) (*service.BatchImageBalanceHoldResult, error) {
	return nil, s.err
}

func (s usageBillingDirectStub) CaptureBatchImageBalance(context.Context, *service.BatchImageBalanceHoldCommand) (*service.BatchImageBalanceHoldResult, error) {
	return nil, s.err
}

func (s usageBillingDirectStub) ReleaseBatchImageBalance(context.Context, *service.BatchImageBalanceHoldCommand) (*service.BatchImageBalanceHoldResult, error) {
	return nil, s.err
}
