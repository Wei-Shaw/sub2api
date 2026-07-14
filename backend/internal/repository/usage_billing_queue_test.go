package repository

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func newDurableUsageBillingRedisTestRepository(t *testing.T) (*queuedUsageBillingRepository, *redis.Client) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return &queuedUsageBillingRepository{rdb: rdb}, rdb
}

func TestDurableUsageBillingOverlayIsIdempotentAndCompletes(t *testing.T) {
	repo, rdb := newDurableUsageBillingRedisTestRepository(t)
	ctx := context.Background()
	cmd := &service.UsageBillingCommand{
		RequestID:           "durable-overlay",
		APIKeyID:            17,
		UserID:              15,
		GroupID:             16,
		BalanceCost:         2.25,
		SubscriptionCost:    3.5,
		APIKeyQuotaCost:     4.75,
		APIKeyRateLimitCost: 5.25,
	}
	cmd.Normalize()

	repo.reconcilePendingOverlay(cmd)
	repo.reconcilePendingOverlay(cmd)
	require.InDelta(t, 2.25, mustRedisFloat(t, rdb, usageBillingPendingBalanceKey(15)), 1e-9)
	require.InDelta(t, 3.5, mustRedisFloat(t, rdb, usageBillingPendingSubscriptionKey(15, 16)), 1e-9)
	require.InDelta(t, 4.75, mustRedisFloat(t, rdb, usageBillingPendingAPIKeyQuotaKey(17)), 1e-9)
	require.InDelta(t, 5.25, mustRedisFloat(t, rdb, usageBillingPendingAPIKeyRateLimitKey(17)), 1e-9)
	require.EqualValues(t, 1, rdb.Exists(ctx, usageBillingOverlayKey(cmd.RequestID, cmd.APIKeyID)).Val())

	require.NoError(t, rdb.Set(ctx, billingBalanceKey(15), 100, time.Hour).Err())
	require.NoError(t, rdb.HSet(ctx, billingSubKey(15, 16), "status", "active").Err())
	require.NoError(t, rdb.HSet(ctx, billingRateLimitKey(17), "usage_5h", 1).Err())
	repo.completePendingOverlay(cmd)

	require.Zero(t, mustRedisFloat(t, rdb, usageBillingPendingBalanceKey(15)))
	require.Zero(t, mustRedisFloat(t, rdb, usageBillingPendingSubscriptionKey(15, 16)))
	require.Zero(t, mustRedisFloat(t, rdb, usageBillingPendingAPIKeyQuotaKey(17)))
	require.Zero(t, mustRedisFloat(t, rdb, usageBillingPendingAPIKeyRateLimitKey(17)))
	require.Zero(t, rdb.Exists(ctx, usageBillingOverlayKey(cmd.RequestID, cmd.APIKeyID)).Val())
	require.Zero(t, rdb.Exists(ctx, billingBalanceKey(15), billingSubKey(15, 16), billingRateLimitKey(17)).Val())
}

func TestDurableUsageBillingOverlayCanRebuildAfterRedisLoss(t *testing.T) {
	repo, rdb := newDurableUsageBillingRedisTestRepository(t)
	ctx := context.Background()
	cmd := &service.UsageBillingCommand{RequestID: "redis-rebuild", APIKeyID: 7, UserID: 9, BalanceCost: 1.5}
	cmd.Normalize()

	repo.reconcilePendingOverlay(cmd)
	require.NoError(t, rdb.FlushDB(ctx).Err())
	repo.reconcilePendingOverlay(cmd)
	require.InDelta(t, 1.5, mustRedisFloat(t, rdb, usageBillingPendingBalanceKey(9)), 1e-9)
}

func TestDurableUsageBillingInsertBatchReturnsStatuses(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := &queuedUsageBillingRepository{db: db}

	rows := sqlmock.NewRows([]string{"request_id", "api_key_id", "job_id", "status"}).
		AddRow("inserted", int64(1), int64(11), "inserted").
		AddRow("pending", int64(2), int64(12), "pending").
		AddRow("applied", int64(3), int64(0), "applied").
		AddRow("conflict", int64(4), int64(0), "conflict")
	mock.ExpectQuery(regexp.QuoteMeta("WITH input AS")).WithArgs(sqlmock.AnyArg()).WillReturnRows(rows)

	results, err := repo.insertEnqueueBatch(context.Background(), []byte(`[]`))
	require.NoError(t, err)
	require.Equal(t, usageBillingEnqueueInserted, results[usageBillingRequestKey("inserted", 1)].status)
	require.Equal(t, usageBillingEnqueuePending, results[usageBillingRequestKey("pending", 2)].status)
	require.Equal(t, usageBillingEnqueueApplied, results[usageBillingRequestKey("applied", 3)].status)
	require.Equal(t, usageBillingEnqueueConflict, results[usageBillingRequestKey("conflict", 4)].status)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDurableUsageBillingFlushBatchDeduplicatesInMemory(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := &queuedUsageBillingRepository{
		db:             db,
		commandTimeout: time.Second,
		consumerCount:  1,
		wakeCh:         make(chan struct{}, 1),
	}
	first := service.UsageBillingCommand{RequestID: "same", APIKeyID: 5, UserID: 1, BalanceCost: 1}
	first.Normalize()
	conflict := first
	conflict.BalanceCost = 2
	conflict.RequestFingerprint = ""
	conflict.Normalize()
	payload, err := json.Marshal(&first)
	require.NoError(t, err)

	rows := sqlmock.NewRows([]string{"request_id", "api_key_id", "job_id", "status"}).
		AddRow("same", int64(5), int64(9), "inserted")
	mock.ExpectQuery(regexp.QuoteMeta("WITH input AS")).WithArgs(sqlmock.AnyArg()).WillReturnRows(rows)

	batch := []usageBillingEnqueueRequest{
		{cmd: first, payload: payload, resultCh: make(chan usageBillingEnqueueResult, 1)},
		{cmd: first, payload: payload, resultCh: make(chan usageBillingEnqueueResult, 1)},
		{cmd: conflict, payload: payload, resultCh: make(chan usageBillingEnqueueResult, 1)},
	}
	repo.flushEnqueueBatch(batch)
	require.Equal(t, usageBillingEnqueueInserted, (<-batch[0].resultCh).status)
	require.Equal(t, usageBillingEnqueuePending, (<-batch[1].resultCh).status)
	require.Equal(t, usageBillingEnqueueConflict, (<-batch[2].resultCh).status)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUsageBillingRetryDelayIsBounded(t *testing.T) {
	require.Equal(t, time.Second, usageBillingRetryDelay(1, 30*time.Second))
	require.Equal(t, 8*time.Second, usageBillingRetryDelay(4, 30*time.Second))
	require.Equal(t, 30*time.Second, usageBillingRetryDelay(20, 30*time.Second))
}

func TestTruncateUsageBillingError(t *testing.T) {
	require.Empty(t, truncateUsageBillingError(nil))
	require.Len(t, truncateUsageBillingError(errors.New(string(make([]byte, 3000)))), 2000)
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
