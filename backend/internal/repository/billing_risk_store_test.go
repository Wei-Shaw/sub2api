package repository

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func newBillingRiskStoreTest(t *testing.T) (service.BillingRiskStore, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return NewBillingRiskStore(rdb), mr
}

func billingRiskAcquire(userID int64, leaseID string, riskMicros int64) service.BillingRiskAcquireRequest {
	return service.BillingRiskAcquireRequest{
		UserID:                   userID,
		LeaseID:                  leaseID,
		RiskMicros:               riskMicros,
		BalanceMicros:            1_000_000,
		MinimumReserveMicros:     0,
		OverdraftAllowanceMicros: 0,
		LeaseTTL:                 time.Minute,
		IdleTTL:                  2 * time.Minute,
	}
}

func TestBillingRiskRedisStoreAllowsWeightedConcurrencyUntilBudgetIsFull(t *testing.T) {
	store, _ := newBillingRiskStoreTest(t)
	ctx := context.Background()

	for i := range 5 {
		result, err := store.Acquire(ctx, billingRiskAcquire(11, fmt.Sprintf("lease-%d", i), 200_000))
		require.NoError(t, err)
		require.True(t, result.Acquired)
		require.False(t, result.WouldReject)
		require.Equal(t, int64((i+1)*200_000), result.ReservedTotalMicros)
	}

	result, err := store.Acquire(ctx, billingRiskAcquire(11, "lease-over-budget", 1))
	require.NoError(t, err)
	require.False(t, result.Acquired)
	require.True(t, result.WouldReject)
	require.Equal(t, int64(1_000_000), result.ReservedTotalMicros)
}

func TestBillingRiskRedisStoreAsyncAndSyncImageShareBudget(t *testing.T) {
	store, _ := newBillingRiskStoreTest(t)
	ctx := context.Background()

	asyncResult, err := store.Acquire(ctx, billingRiskAcquire(25, "async-image", 800_000))
	require.NoError(t, err)
	require.True(t, asyncResult.Acquired)

	syncResult, err := store.Acquire(ctx, billingRiskAcquire(25, "sync-image", 800_000))
	require.NoError(t, err)
	require.False(t, syncResult.Acquired)
	require.True(t, syncResult.WouldReject)
	require.Equal(t, int64(800_000), syncResult.ReservedTotalMicros)
}

func TestBillingRiskRedisStoreConcurrentAcquireNeverExceedsBudget(t *testing.T) {
	store, _ := newBillingRiskStoreTest(t)
	ctx := context.Background()
	var admitted atomic.Int64
	var wg sync.WaitGroup
	errors := make(chan error, 20)

	for i := range 20 {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			result, err := store.Acquire(ctx, billingRiskAcquire(12, fmt.Sprintf("lease-%d", index), 200_000))
			if err != nil {
				errors <- err
				return
			}
			if result.Acquired {
				admitted.Add(1)
			}
		}(i)
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
	require.Equal(t, int64(5), admitted.Load())
}

func TestBillingRiskRedisStoreAcquireCommitAndReleaseAreIdempotent(t *testing.T) {
	store, _ := newBillingRiskStoreTest(t)
	ctx := context.Background()
	request := billingRiskAcquire(14, "same", 400_000)

	first, err := store.Acquire(ctx, request)
	require.NoError(t, err)
	second, err := store.Acquire(ctx, request)
	require.NoError(t, err)
	require.True(t, first.Acquired)
	require.True(t, second.Acquired)
	require.Equal(t, int64(400_000), second.ReservedTotalMicros)

	committed, err := store.Commit(ctx, 14, "same", 600_000, 2*time.Minute)
	require.NoError(t, err)
	require.True(t, committed)
	committed, err = store.Commit(ctx, 14, "same", 800_000, 2*time.Minute)
	require.NoError(t, err)
	require.False(t, committed)

	probe := billingRiskAcquire(14, "probe", 1)
	probe.BalanceMicros = 900_000
	result, err := store.Acquire(ctx, probe)
	require.NoError(t, err)
	require.True(t, result.Acquired)
	require.Equal(t, int64(600_000), result.KnownBalanceMicros)

	released, err := store.Release(ctx, 14, "probe", 2*time.Minute)
	require.NoError(t, err)
	require.True(t, released)
	released, err = store.Release(ctx, 14, "probe", 2*time.Minute)
	require.NoError(t, err)
	require.False(t, released)
}

func TestBillingRiskRedisStoreOutOfOrderCommitsOnlyLowerKnownBalance(t *testing.T) {
	store, _ := newBillingRiskStoreTest(t)
	ctx := context.Background()

	_, err := store.Acquire(ctx, billingRiskAcquire(15, "first", 100_000))
	require.NoError(t, err)
	_, err = store.Acquire(ctx, billingRiskAcquire(15, "second", 100_000))
	require.NoError(t, err)
	require.NoError(t, commitBillingRiskLease(ctx, store, 15, "second", 800_000))
	require.NoError(t, commitBillingRiskLease(ctx, store, 15, "first", 900_000))

	probe := billingRiskAcquire(15, "probe", 1)
	probe.BalanceMicros = 950_000
	result, err := store.Acquire(ctx, probe)
	require.NoError(t, err)
	require.Equal(t, int64(800_000), result.KnownBalanceMicros)
}

func TestBillingRiskRedisStoreRejectsStaleDatabaseBalanceResetAfterNewerCommit(t *testing.T) {
	store, _ := newBillingRiskStoreTest(t)
	ctx := context.Background()
	const userID = int64(23)

	_, err := store.Acquire(ctx, billingRiskAcquire(userID, "in-flight", 100_000))
	require.NoError(t, err)
	version, err := store.GetBalanceVersion(ctx, userID)
	require.NoError(t, err)

	committed, err := store.Commit(ctx, userID, "in-flight", 400_000, 2*time.Minute)
	require.NoError(t, err)
	require.True(t, committed)

	reset, err := store.ResetBalance(ctx, userID, 900_000, version, 2*time.Minute)
	require.NoError(t, err)
	require.False(t, reset.Accepted)
	require.Equal(t, int64(400_000), reset.KnownBalanceMicros)

	probe := billingRiskAcquire(userID, "must-stay-blocked", 500_000)
	probe.BalanceMicros = 900_000
	result, err := store.Acquire(ctx, probe)
	require.NoError(t, err)
	require.False(t, result.Acquired)
	require.Equal(t, int64(400_000), result.KnownBalanceMicros)
}

func TestBillingRiskRedisStoreAcceptsHigherBalanceWhenNoLeaseIsOutstanding(t *testing.T) {
	store, _ := newBillingRiskStoreTest(t)
	ctx := context.Background()

	_, err := store.Acquire(ctx, billingRiskAcquire(19, "spent", 100_000))
	require.NoError(t, err)
	require.NoError(t, commitBillingRiskLease(ctx, store, 19, "spent", 100_000))
	version, err := store.GetBalanceVersion(ctx, 19)
	require.NoError(t, err)
	reset, err := store.ResetBalance(ctx, 19, 500_000, version, 2*time.Minute)
	require.NoError(t, err)
	require.True(t, reset.Accepted)
	require.Equal(t, int64(500_000), reset.KnownBalanceMicros)

	afterRecharge := billingRiskAcquire(19, "after-recharge", 200_000)
	afterRecharge.BalanceMicros = 500_000
	result, err := store.Acquire(ctx, afterRecharge)

	require.NoError(t, err)
	require.True(t, result.Acquired)
	require.Equal(t, int64(500_000), result.KnownBalanceMicros)
}

func TestBillingRiskRedisStoreDoesNotRaiseKnownBalanceWhileLeaseIsOutstanding(t *testing.T) {
	store, _ := newBillingRiskStoreTest(t)
	ctx := context.Background()

	_, err := store.Acquire(ctx, billingRiskAcquire(20, "spent", 100_000))
	require.NoError(t, err)
	require.NoError(t, commitBillingRiskLease(ctx, store, 20, "spent", 100_000))

	inFlight := billingRiskAcquire(20, "in-flight", 50_000)
	inFlight.BalanceMicros = 100_000
	result, err := store.Acquire(ctx, inFlight)
	require.NoError(t, err)
	require.True(t, result.Acquired)

	staleHigherBalance := billingRiskAcquire(20, "must-stay-blocked", 100_000)
	staleHigherBalance.BalanceMicros = 500_000
	result, err = store.Acquire(ctx, staleHigherBalance)
	require.NoError(t, err)
	require.False(t, result.Acquired)
	require.True(t, result.WouldReject)
	require.Equal(t, int64(100_000), result.KnownBalanceMicros)
}

func TestBillingRiskRedisStoreOverdraftAllowanceIsCumulativeAfterBalanceTurnsNegative(t *testing.T) {
	store, _ := newBillingRiskStoreTest(t)
	ctx := context.Background()

	first := billingRiskAcquire(18, "first", 200_000)
	first.BalanceMicros = 100_000
	first.OverdraftAllowanceMicros = 200_000
	result, err := store.Acquire(ctx, first)
	require.NoError(t, err)
	require.True(t, result.Acquired)

	committed, err := store.Commit(ctx, 18, "first", -100_000, 2*time.Minute)
	require.NoError(t, err)
	require.True(t, committed)

	second := billingRiskAcquire(18, "second", 150_000)
	second.BalanceMicros = 100_000 // 模拟认证余额快照尚未刷新。
	second.OverdraftAllowanceMicros = 200_000
	result, err = store.Acquire(ctx, second)
	require.NoError(t, err)
	require.Equal(t, int64(-100_000), result.KnownBalanceMicros)
	require.False(t, result.Acquired)
	require.True(t, result.WouldReject)
}

func commitBillingRiskLease(ctx context.Context, store service.BillingRiskStore, userID int64, leaseID string, balance int64) error {
	_, err := store.Commit(ctx, userID, leaseID, balance, 2*time.Minute)
	return err
}

func TestBillingRiskRedisStoreExpiredLeaseIsCleanedBeforeAcquire(t *testing.T) {
	store, mr := newBillingRiskStoreTest(t)
	ctx := context.Background()
	now := time.Unix(2_000_000_000, 0)
	mr.SetTime(now)
	request := billingRiskAcquire(16, "expired", 1_000_000)
	request.LeaseTTL = time.Second

	result, err := store.Acquire(ctx, request)
	require.NoError(t, err)
	require.True(t, result.Acquired)
	mr.SetTime(now.Add(2 * time.Second))

	result, err = store.Acquire(ctx, billingRiskAcquire(16, "replacement", 1_000_000))
	require.NoError(t, err)
	require.True(t, result.Acquired)
	require.Equal(t, int64(1_000_000), result.ReservedTotalMicros)
}

func TestBillingRiskRedisStoreCleansAllExpiredLeasesBeforeBudgetCheck(t *testing.T) {
	store, mr := newBillingRiskStoreTest(t)
	ctx := context.Background()
	now := time.Unix(2_000_000_000, 0)
	mr.SetTime(now)

	for i := range 257 {
		request := billingRiskAcquire(21, fmt.Sprintf("expired-%d", i), 3_000)
		request.LeaseTTL = time.Second
		result, err := store.Acquire(ctx, request)
		require.NoError(t, err)
		require.True(t, result.Acquired)
	}
	mr.SetTime(now.Add(2 * time.Second))

	result, err := store.Acquire(ctx, billingRiskAcquire(21, "replacement", 1_000_000))
	require.NoError(t, err)
	require.True(t, result.Acquired)
	require.Equal(t, int64(1_000_000), result.ReservedTotalMicros)
}

func TestBillingRiskRedisStoreRefreshAndUncertainCooldownExtendLease(t *testing.T) {
	store, mr := newBillingRiskStoreTest(t)
	ctx := context.Background()
	now := time.Unix(2_000_000_000, 0)
	mr.SetTime(now)
	request := billingRiskAcquire(17, "long-running", 800_000)
	request.LeaseTTL = time.Second

	_, err := store.Acquire(ctx, request)
	require.NoError(t, err)
	refreshed, err := store.Refresh(ctx, 17, "long-running", 3*time.Second, 2*time.Minute)
	require.NoError(t, err)
	require.True(t, refreshed)
	mr.SetTime(now.Add(2 * time.Second))

	blocked, err := store.Acquire(ctx, billingRiskAcquire(17, "blocked", 300_000))
	require.NoError(t, err)
	require.False(t, blocked.Acquired)

	marked, err := store.MarkUncertain(ctx, 17, "long-running", 800_000, 10*time.Second, 2*time.Minute)
	require.NoError(t, err)
	require.True(t, marked)
	mr.SetTime(now.Add(6 * time.Second))
	blocked, err = store.Acquire(ctx, billingRiskAcquire(17, "still-blocked", 300_000))
	require.NoError(t, err)
	require.False(t, blocked.Acquired)

	mr.SetTime(now.Add(13 * time.Second))
	result, err := store.Acquire(ctx, billingRiskAcquire(17, "after-cooldown", 1_000_000))
	require.NoError(t, err)
	require.True(t, result.Acquired)
}

func TestBillingRiskRedisStoreMarkUncertainRestoresLostLeaseBudget(t *testing.T) {
	store, mr := newBillingRiskStoreTest(t)
	ctx := context.Background()
	request := billingRiskAcquire(18, "lost", 800_000)

	result, err := store.Acquire(ctx, request)
	require.NoError(t, err)
	require.True(t, result.Acquired)
	keys := billingRiskRedisKeys(18)
	mr.Del(keys.leases)
	mr.Del(keys.costs)
	mr.Del(keys.meta)

	marked, err := store.MarkUncertain(ctx, 18, "lost", request.RiskMicros, 5*time.Minute, request.IdleTTL)
	require.NoError(t, err)
	require.True(t, marked)

	blocked, err := store.Acquire(ctx, billingRiskAcquire(18, "next", 300_000))
	require.NoError(t, err)
	require.False(t, blocked.Acquired)
	require.True(t, blocked.WouldReject)
	require.Equal(t, int64(800_000), blocked.ReservedTotalMicros)
}

func TestBillingRiskRedisStoreMarkUncertainRebuildsMissingMetaFromExistingCosts(t *testing.T) {
	store, mr := newBillingRiskStoreTest(t)
	ctx := context.Background()
	request := billingRiskAcquire(19, "partial-loss", 800_000)

	result, err := store.Acquire(ctx, request)
	require.NoError(t, err)
	require.True(t, result.Acquired)
	keys := billingRiskRedisKeys(19)
	mr.Del(keys.leases)
	mr.Del(keys.meta)

	marked, err := store.MarkUncertain(ctx, 19, "partial-loss", request.RiskMicros, 5*time.Minute, request.IdleTTL)
	require.NoError(t, err)
	require.True(t, marked)

	blocked, err := store.Acquire(ctx, billingRiskAcquire(19, "next", 300_000))
	require.NoError(t, err)
	require.False(t, blocked.Acquired)
	require.True(t, blocked.WouldReject)
	require.Equal(t, int64(800_000), blocked.ReservedTotalMicros)
}

func TestBillingRiskRedisStoreUsesClusterHashTagPerUser(t *testing.T) {
	keys := billingRiskRedisKeys(123)
	require.Contains(t, keys.leases, "{123}")
	require.Contains(t, keys.costs, "{123}")
	require.Contains(t, keys.meta, "{123}")
	require.Contains(t, keys.balance, "{123}")
}
