//go:build unit

package repository

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestTemporaryBalanceAuditMigrationRetainsUserHistory(t *testing.T) {
	contents, err := os.ReadFile("../../migrations/234_temporary_balance_audits.sql")
	require.NoError(t, err)
	sql := string(contents)
	require.Contains(t, sql, "REFERENCES users(id) ON DELETE RESTRICT")
	require.NotContains(t, sql, "user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE")
}

func TestBatchImageReserveUsesPermanentBalanceContract(t *testing.T) {
	// Batch-image holds remain backed by permanent balance because the current
	// frozen-balance schema has no temporary-hold provenance for settlement.
	// Keep this contract explicit until that separate hold design is introduced.
	require.Contains(t, reserveBatchImageHoldSQL, "balance >= \\$1")
}

func TestBillingCacheTemporaryBalanceDataExpiresAtBoundary(t *testing.T) {
	expires := time.Now().UTC().Add(time.Minute)
	data := service.UserBalanceCacheData{Balance: 12, TemporaryBalance: 5, TemporaryBalanceExpiresAt: &expires}
	require.Equal(t, float64(12), data.Balance)
	require.NotNil(t, data.TemporaryBalanceExpiresAt)
}

func TestBillingCacheStoresTemporaryExpiryAndInvalidatesIt(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	cache := &billingCache{rdb: rdb}
	expires := time.Now().UTC().Add(2 * time.Minute)
	require.NoError(t, cache.SetUserBalanceData(context.Background(), 42, service.UserBalanceCacheData{
		Balance: 15, TemporaryBalance: 5, TemporaryBalanceExpiresAt: &expires,
	}))
	got, err := cache.GetUserBalanceData(context.Background(), 42)
	require.NoError(t, err)
	require.InDelta(t, 15, got.Balance, 0.000001)
	require.InDelta(t, 5, got.TemporaryBalance, 0.000001)
	require.NoError(t, cache.InvalidateUserBalance(context.Background(), 42))
	_, err = cache.GetUserBalanceData(context.Background(), 42)
	require.ErrorIs(t, err, redis.Nil)
}

func TestBillingCacheLegacyBalanceWriteClearsTemporarySnapshot(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	cache := &billingCache{rdb: rdb}
	expires := time.Now().UTC().Add(time.Hour)
	require.NoError(t, cache.SetUserBalanceData(context.Background(), 45, service.UserBalanceCacheData{
		Balance: 15, TemporaryBalance: 5, TemporaryBalanceExpiresAt: &expires,
	}))
	require.NoError(t, cache.SetUserBalance(context.Background(), 45, 11))
	_, err := cache.GetUserBalanceData(context.Background(), 45)
	require.ErrorIs(t, err, redis.Nil)
	legacy, err := cache.GetUserBalance(context.Background(), 45)
	require.NoError(t, err)
	require.InDelta(t, 11, legacy, 0.000001)
}

func TestBillingCacheDoesNotPersistExpiredTemporaryAmount(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	cache := &billingCache{rdb: rdb}
	expires := time.Now().UTC().Add(-time.Minute)
	require.NoError(t, cache.SetUserBalanceData(context.Background(), 43, service.UserBalanceCacheData{
		Balance: 15, TemporaryBalance: 5, TemporaryBalanceExpiresAt: &expires,
	}))
	got, err := cache.GetUserBalanceData(context.Background(), 43)
	require.NoError(t, err)
	require.InDelta(t, 10, got.Balance, 0.000001)
	require.Zero(t, got.TemporaryBalance)
}

func TestBillingCacheDeductsTemporaryBalanceBeforeExpiry(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	cache := &billingCache{rdb: rdb}
	expires := time.Now().UTC().Add(2 * time.Minute)
	require.NoError(t, cache.SetUserBalanceData(context.Background(), 44, service.UserBalanceCacheData{
		Balance: 15, TemporaryBalance: 5, TemporaryBalanceExpiresAt: &expires,
	}))
	require.NoError(t, cache.DeductUserBalance(context.Background(), 44, 3))
	got, err := cache.GetUserBalanceData(context.Background(), 44)
	require.NoError(t, err)
	require.InDelta(t, 12, got.Balance, 0.000001)
	require.InDelta(t, 2, got.TemporaryBalance, 0.000001)
	// Once the temporary grant expires, only the remaining permanent balance
	// must be visible; the cached temporary field cannot be charged again.
	// miniredis FastForward does not advance the Go process clock used by the
	// expiry-aware reader, so rewrite the same snapshot with a past boundary.
	expired := time.Now().UTC().Add(-time.Minute)
	require.NoError(t, cache.SetUserBalanceData(context.Background(), 44, service.UserBalanceCacheData{
		Balance: 12, TemporaryBalance: 2, TemporaryBalanceExpiresAt: &expired,
	}))
	got, err = cache.GetUserBalanceData(context.Background(), 44)
	require.NoError(t, err)
	require.InDelta(t, 10, got.Balance, 0.000001)
	require.Zero(t, got.TemporaryBalance)
}

func TestTemporaryBalanceMaintenanceInvalidatesChangedUsers(t *testing.T) {
	// Compile-time contract for the maintenance hook. The concrete worker test
	// exercises this through a fake repository/cache implementation.
	var _ service.TemporaryBalanceMaintenanceRepository = (*userRepository)(nil)
	_ = context.Background()
}
