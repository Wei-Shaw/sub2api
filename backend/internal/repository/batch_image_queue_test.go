//go:build unit

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

func TestBatchImageQueue_DuplicateEnqueueReturnsAlreadyQueued(t *testing.T) {
	ctx := context.Background()
	queue, _ := newBatchImageQueueTest(t)
	batchID := "imgbatch_duplicate"

	require.NoError(t, queue.Enqueue(ctx, batchID))
	err := queue.Enqueue(ctx, batchID)
	require.Error(t, err)
	require.True(t, errors.Is(err, service.ErrBatchImageAlreadyQueued))
}

func TestBatchImageQueue_RequeueAfterMovesJobFromActiveToDelayed(t *testing.T) {
	ctx := context.Background()
	queue, _ := newBatchImageQueueTest(t)
	batchID := "imgbatch_requeue_after"
	require.NoError(t, queue.rdb.ZAdd(ctx, queue.activeKey, redis.Z{
		Score:  float64(time.Now().UnixMilli()),
		Member: batchID,
	}).Err())

	require.NoError(t, queue.RequeueAfter(ctx, batchID, time.Minute))
	require.ErrorIs(t, queue.rdb.ZScore(ctx, queue.activeKey, batchID).Err(), redis.Nil)
	score, err := queue.rdb.ZScore(ctx, queue.delayedKey, batchID).Result()
	require.NoError(t, err)
	require.Greater(t, score, float64(time.Now().UnixMilli()))
}

func TestBatchImageQueue_MoveDueDelayedToReadyMovesDueJobs(t *testing.T) {
	ctx := context.Background()
	queue, _ := newBatchImageQueueTest(t)
	dueBatchID := "imgbatch_due"
	futureBatchID := "imgbatch_future"
	now := time.Now()
	require.NoError(t, queue.rdb.ZAdd(ctx, queue.delayedKey,
		redis.Z{Score: float64(now.Add(-time.Second).UnixMilli()), Member: dueBatchID},
		redis.Z{Score: float64(now.Add(time.Hour).UnixMilli()), Member: futureBatchID},
	).Err())

	moved, err := queue.MoveDueDelayedToReady(ctx, 10)
	require.NoError(t, err)
	require.Equal(t, 1, moved)
	require.ErrorIs(t, queue.rdb.ZScore(ctx, queue.delayedKey, dueBatchID).Err(), redis.Nil)
	require.NoError(t, queue.rdb.ZScore(ctx, queue.delayedKey, futureBatchID).Err())

	reserved, err := queue.Reserve(ctx, time.Millisecond)
	require.NoError(t, err)
	require.Equal(t, dueBatchID, reserved.BatchID)
}

func TestBatchImageQueue_RecoverStaleActiveMovesStaleJobsToReady(t *testing.T) {
	ctx := context.Background()
	queue, _ := newBatchImageQueueTest(t)
	staleBatchID := "imgbatch_stale"
	recentBatchID := "imgbatch_recent"
	now := time.Now()
	require.NoError(t, queue.rdb.ZAdd(ctx, queue.activeKey,
		redis.Z{Score: float64(now.Add(-time.Hour).UnixMilli()), Member: staleBatchID},
		redis.Z{Score: float64(now.UnixMilli()), Member: recentBatchID},
	).Err())

	moved, err := queue.RecoverStaleActive(ctx, 10*time.Minute, 10)
	require.NoError(t, err)
	require.Equal(t, 1, moved)
	require.ErrorIs(t, queue.rdb.ZScore(ctx, queue.activeKey, staleBatchID).Err(), redis.Nil)
	require.NoError(t, queue.rdb.ZScore(ctx, queue.activeKey, recentBatchID).Err())

	reserved, err := queue.Reserve(ctx, time.Millisecond)
	require.NoError(t, err)
	require.Equal(t, staleBatchID, reserved.BatchID)
}

func TestBatchImageQueue_JobLockReleaseOnlyDeletesMatchingToken(t *testing.T) {
	ctx := context.Background()
	queue, _ := newBatchImageQueueTest(t)
	batchID := "imgbatch_lock"

	lock, ok, err := queue.TryAcquireJobLock(ctx, batchID, time.Minute)
	require.NoError(t, err)
	require.True(t, ok)

	require.NoError(t, queue.rdb.Set(ctx, queue.lockKey(batchID), "other-token", time.Minute).Err())
	require.NoError(t, lock.Release(ctx))
	got, err := queue.rdb.Get(ctx, queue.lockKey(batchID)).Result()
	require.NoError(t, err)
	require.Equal(t, "other-token", got)

	require.NoError(t, queue.rdb.Del(ctx, queue.lockKey(batchID)).Err())
	lock, ok, err = queue.TryAcquireJobLock(ctx, batchID, time.Minute)
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, lock.Release(ctx))
	require.ErrorIs(t, queue.rdb.Get(ctx, queue.lockKey(batchID)).Err(), redis.Nil)
}

func TestBatchImageQueue_ReserveAtomicallyMovesJobToActive(t *testing.T) {
	ctx := context.Background()
	queue, _ := newBatchImageQueueTest(t)
	batchID := "imgbatch_reserve"
	require.NoError(t, queue.Enqueue(ctx, batchID))

	reserved, err := queue.Reserve(ctx, time.Second)
	require.NoError(t, err)
	require.Equal(t, batchID, reserved.BatchID)

	// 弹出与写入 active 必须原子完成：ready 已空，active 中有该 job。
	require.Equal(t, int64(0), queue.rdb.LLen(ctx, queue.readyKey).Val())
	score, err := queue.rdb.ZScore(ctx, queue.activeKey, batchID).Result()
	require.NoError(t, err)
	require.Positive(t, score)
}

func TestBatchImageQueue_ReserveReturnsEmptyAfterTimeout(t *testing.T) {
	ctx := context.Background()
	queue, _ := newBatchImageQueueTest(t)

	start := time.Now()
	_, err := queue.Reserve(ctx, 50*time.Millisecond)
	require.ErrorIs(t, err, service.ErrBatchImageQueueEmpty)
	require.Less(t, time.Since(start), 5*time.Second)
}

func TestBatchImageQueue_ReserveDropsInvalidPayload(t *testing.T) {
	ctx := context.Background()
	queue, _ := newBatchImageQueueTest(t)
	require.NoError(t, queue.rdb.LPush(ctx, queue.readyKey, "not-a-batch-id").Err())

	_, err := queue.Reserve(ctx, 10*time.Millisecond)
	require.ErrorIs(t, err, service.ErrInvalidBatchImageQueuePayload)
	// 非法 payload 不得残留在 active zset，否则 stale 恢复会无限重投。
	require.ErrorIs(t, queue.rdb.ZScore(ctx, queue.activeKey, "not-a-batch-id").Err(), redis.Nil)
}

func TestBatchImageQueue_HeartbeatOnlyRefreshesExistingActiveMember(t *testing.T) {
	ctx := context.Background()
	queue, _ := newBatchImageQueueTest(t)
	batchID := "imgbatch_heartbeat"

	// 不在 active 中：心跳不得创建幽灵成员。
	require.NoError(t, queue.Heartbeat(ctx, batchID))
	require.ErrorIs(t, queue.rdb.ZScore(ctx, queue.activeKey, batchID).Err(), redis.Nil)

	require.NoError(t, queue.rdb.ZAdd(ctx, queue.activeKey, redis.Z{Score: 1, Member: batchID}).Err())
	require.NoError(t, queue.Heartbeat(ctx, batchID))
	score, err := queue.rdb.ZScore(ctx, queue.activeKey, batchID).Result()
	require.NoError(t, err)
	require.Greater(t, score, float64(1))
}

func TestBatchImageQueue_JobLockRefreshExtendsTTLOnlyForHolder(t *testing.T) {
	ctx := context.Background()
	queue, mr := newBatchImageQueueTest(t)
	batchID := "imgbatch_lock_refresh"

	lock, ok, err := queue.TryAcquireJobLock(ctx, batchID, time.Minute)
	require.NoError(t, err)
	require.True(t, ok)
	refresher, isRefresher := lock.(service.BatchImageJobLockRefresher)
	require.True(t, isRefresher)

	require.NoError(t, refresher.Refresh(ctx, 10*time.Minute))
	ttl := mr.TTL(queue.lockKey(batchID))
	require.Greater(t, ttl, 5*time.Minute)

	// token 不匹配时不得续期他人持有的锁。
	require.NoError(t, queue.rdb.Set(ctx, queue.lockKey(batchID), "other-token", time.Minute).Err())
	require.NoError(t, refresher.Refresh(ctx, 10*time.Minute))
	ttl = mr.TTL(queue.lockKey(batchID))
	require.LessOrEqual(t, ttl, time.Minute)
}

func newBatchImageQueueTest(t *testing.T) (*batchImageQueue, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = rdb.Close()
	})
	queue := newBatchImageQueueWithOptions(rdb, batchImageQueueOptions{
		InflightTTL: time.Hour,
		LockTTL:     time.Minute,
	})
	return queue, mr
}
