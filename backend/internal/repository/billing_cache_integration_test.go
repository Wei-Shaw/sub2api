//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service/ports"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type BillingCacheSuite struct {
	IntegrationRedisSuite
}

func (s *BillingCacheSuite) TestUserBalance() {
	tests := []struct {
		name string
		fn   func(ctx context.Context, rdb *redis.Client, cache ports.BillingCache)
	}{
		{
			name: "missing_key_returns_redis_nil",
			fn: func(ctx context.Context, rdb *redis.Client, cache ports.BillingCache) {
				_, err := cache.GetUserBalance(ctx, 1)
				require.ErrorIs(s.T(), err, redis.Nil, "expected redis.Nil for missing balance key")
			},
		},
		{
			name: "deduct_on_nonexistent_is_noop",
			fn: func(ctx context.Context, rdb *redis.Client, cache ports.BillingCache) {
				userID := int64(1)
				balanceKey := fmt.Sprintf("%s%d", billingBalanceKeyPrefix, userID)

				require.NoError(s.T(), cache.DeductUserBalance(ctx, userID, 1), "DeductUserBalance should not error")

				_, err := rdb.Get(ctx, balanceKey).Result()
				require.ErrorIs(s.T(), err, redis.Nil, "expected missing key after deduct on non-existent")
			},
		},
		{
			name: "set_and_get_with_ttl",
			fn: func(ctx context.Context, rdb *redis.Client, cache ports.BillingCache) {
				userID := int64(2)
				balanceKey := fmt.Sprintf("%s%d", billingBalanceKeyPrefix, userID)

				require.NoError(s.T(), cache.SetUserBalance(ctx, userID, 10.5), "SetUserBalance")

				got, err := cache.GetUserBalance(ctx, userID)
				require.NoError(s.T(), err, "GetUserBalance")
				require.Equal(s.T(), 10.5, got, "balance mismatch")

				ttl, err := rdb.TTL(ctx, balanceKey).Result()
				require.NoError(s.T(), err, "TTL")
				s.AssertTTLWithin(ttl, 1*time.Second, billingCacheTTL)
			},
		},
		{
			name: "deduct_reduces_balance",
			fn: func(ctx context.Context, rdb *redis.Client, cache ports.BillingCache) {
				userID := int64(3)

				require.NoError(s.T(), cache.SetUserBalance(ctx, userID, 10.5), "SetUserBalance")
				require.NoError(s.T(), cache.DeductUserBalance(ctx, userID, 2.25), "DeductUserBalance")

				got, err := cache.GetUserBalance(ctx, userID)
				require.NoError(s.T(), err, "GetUserBalance after deduct")
				require.Equal(s.T(), 8.25, got, "deduct mismatch")
			},
		},
		{
			name: "invalidate_removes_key",
			fn: func(ctx context.Context, rdb *redis.Client, cache ports.BillingCache) {
				userID := int64(100)
				balanceKey := fmt.Sprintf("%s%d", billingBalanceKeyPrefix, userID)

				require.NoError(s.T(), cache.SetUserBalance(ctx, userID, 50.0), "SetUserBalance")

				exists, err := rdb.Exists(ctx, balanceKey).Result()
				require.NoError(s.T(), err, "Exists")
				require.Equal(s.T(), int64(1), exists, "expected balance key to exist")

				require.NoError(s.T(), cache.InvalidateUserBalance(ctx, userID), "InvalidateUserBalance")

				exists, err = rdb.Exists(ctx, balanceKey).Result()
				require.NoError(s.T(), err, "Exists after invalidate")
				require.Equal(s.T(), int64(0), exists, "expected balance key to be removed after invalidate")

				_, err = cache.GetUserBalance(ctx, userID)
				require.ErrorIs(s.T(), err, redis.Nil, "expected redis.Nil after invalidate")
			},
		},
		{
			name: "deduct_refreshes_ttl",
			fn: func(ctx context.Context, rdb *redis.Client, cache ports.BillingCache) {
				userID := int64(103)
				balanceKey := fmt.Sprintf("%s%d", billingBalanceKeyPrefix, userID)

				require.NoError(s.T(), cache.SetUserBalance(ctx, userID, 100.0), "SetUserBalance")

				ttl1, err := rdb.TTL(ctx, balanceKey).Result()
				require.NoError(s.T(), err, "TTL before deduct")
				s.AssertTTLWithin(ttl1, 1*time.Second, billingCacheTTL)

				require.NoError(s.T(), cache.DeductUserBalance(ctx, userID, 25.0), "DeductUserBalance")

				balance, err := cache.GetUserBalance(ctx, userID)
				require.NoError(s.T(), err, "GetUserBalance")
				require.Equal(s.T(), 75.0, balance, "expected balance 75.0")

				ttl2, err := rdb.TTL(ctx, balanceKey).Result()
				require.NoError(s.T(), err, "TTL after deduct")
				s.AssertTTLWithin(ttl2, 1*time.Second, billingCacheTTL)
			},
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			rdb := testRedis(s.T())
			cache := NewBillingCache(rdb)
			ctx := context.Background()

			tt.fn(ctx, rdb, cache)
		})
	}
}

func (s *BillingCacheSuite) TestSubscriptionCache() {
	tests := []struct {
		name string
		fn   func(ctx context.Context, rdb *redis.Client, cache ports.BillingCache)
	}{
		{
			name: "missing_key_returns_redis_nil",
			fn: func(ctx context.Context, rdb *redis.Client, cache ports.BillingCache) {
				userID := int64(10)
				groupID := int64(20)

				_, err := cache.GetSubscriptionCache(ctx, userID, groupID)
				require.ErrorIs(s.T(), err, redis.Nil, "expected redis.Nil for missing subscription key")
			},
		},
		{
			name: "update_usage_on_nonexistent_is_noop",
			fn: func(ctx context.Context, rdb *redis.Client, cache ports.BillingCache) {
				userID := int64(11)
				groupID := int64(21)
				subKey := fmt.Sprintf("%s%d:%d", billingSubKeyPrefix, userID, groupID)

				require.NoError(s.T(), cache.UpdateSubscriptionUsage(ctx, userID, groupID, 1.0), "UpdateSubscriptionUsage should not error")

				exists, err := rdb.Exists(ctx, subKey).Result()
				require.NoError(s.T(), err, "Exists")
				require.Equal(s.T(), int64(0), exists, "expected missing subscription key after UpdateSubscriptionUsage on non-existent")
			},
		},
		{
			name: "set_and_get_with_ttl",
			fn: func(ctx context.Context, rdb *redis.Client, cache ports.BillingCache) {
				userID := int64(12)
				groupID := int64(22)
				subKey := fmt.Sprintf("%s%d:%d", billingSubKeyPrefix, userID, groupID)

				data := &ports.SubscriptionCacheData{
					Status:       "active",
					ExpiresAt:    time.Now().Add(1 * time.Hour),
					DailyUsage:   1.0,
					WeeklyUsage:  2.0,
					MonthlyUsage: 3.0,
					Version:      7,
				}
				require.NoError(s.T(), cache.SetSubscriptionCache(ctx, userID, groupID, data), "SetSubscriptionCache")

				gotSub, err := cache.GetSubscriptionCache(ctx, userID, groupID)
				require.NoError(s.T(), err, "GetSubscriptionCache")
				require.Equal(s.T(), "active", gotSub.Status)
				require.Equal(s.T(), int64(7), gotSub.Version)
				require.Equal(s.T(), 1.0, gotSub.DailyUsage)

				ttl, err := rdb.TTL(ctx, subKey).Result()
				require.NoError(s.T(), err, "TTL subKey")
				s.AssertTTLWithin(ttl, 1*time.Second, billingCacheTTL)
			},
		},
		{
			name: "update_usage_increments_all_fields",
			fn: func(ctx context.Context, rdb *redis.Client, cache ports.BillingCache) {
				userID := int64(13)
				groupID := int64(23)

				data := &ports.SubscriptionCacheData{
					Status:       "active",
					ExpiresAt:    time.Now().Add(1 * time.Hour),
					DailyUsage:   1.0,
					WeeklyUsage:  2.0,
					MonthlyUsage: 3.0,
					Version:      1,
				}
				require.NoError(s.T(), cache.SetSubscriptionCache(ctx, userID, groupID, data), "SetSubscriptionCache")

				require.NoError(s.T(), cache.UpdateSubscriptionUsage(ctx, userID, groupID, 0.5), "UpdateSubscriptionUsage")

				gotSub, err := cache.GetSubscriptionCache(ctx, userID, groupID)
				require.NoError(s.T(), err, "GetSubscriptionCache after update")
				require.Equal(s.T(), 1.5, gotSub.DailyUsage)
				require.Equal(s.T(), 2.5, gotSub.WeeklyUsage)
				require.Equal(s.T(), 3.5, gotSub.MonthlyUsage)
			},
		},
		{
			name: "invalidate_removes_key",
			fn: func(ctx context.Context, rdb *redis.Client, cache ports.BillingCache) {
				userID := int64(101)
				groupID := int64(10)
				subKey := fmt.Sprintf("%s%d:%d", billingSubKeyPrefix, userID, groupID)

				data := &ports.SubscriptionCacheData{
					Status:       "active",
					ExpiresAt:    time.Now().Add(1 * time.Hour),
					DailyUsage:   1.0,
					WeeklyUsage:  2.0,
					MonthlyUsage: 3.0,
					Version:      1,
				}
				require.NoError(s.T(), cache.SetSubscriptionCache(ctx, userID, groupID, data), "SetSubscriptionCache")

				exists, err := rdb.Exists(ctx, subKey).Result()
				require.NoError(s.T(), err, "Exists")
				require.Equal(s.T(), int64(1), exists, "expected subscription key to exist")

				require.NoError(s.T(), cache.InvalidateSubscriptionCache(ctx, userID, groupID), "InvalidateSubscriptionCache")

				exists, err = rdb.Exists(ctx, subKey).Result()
				require.NoError(s.T(), err, "Exists after invalidate")
				require.Equal(s.T(), int64(0), exists, "expected subscription key to be removed after invalidate")

				_, err = cache.GetSubscriptionCache(ctx, userID, groupID)
				require.ErrorIs(s.T(), err, redis.Nil, "expected redis.Nil after invalidate")
			},
		},
		{
			name: "missing_status_returns_parsing_error",
			fn: func(ctx context.Context, rdb *redis.Client, cache ports.BillingCache) {
				userID := int64(102)
				groupID := int64(11)
				subKey := fmt.Sprintf("%s%d:%d", billingSubKeyPrefix, userID, groupID)

				fields := map[string]any{
					"expires_at":    time.Now().Add(1 * time.Hour).Unix(),
					"daily_usage":   1.0,
					"weekly_usage":  2.0,
					"monthly_usage": 3.0,
					"version":       1,
				}
				require.NoError(s.T(), rdb.HSet(ctx, subKey, fields).Err(), "HSet")

				_, err := cache.GetSubscriptionCache(ctx, userID, groupID)
				require.Error(s.T(), err, "expected error for missing status field")
				require.NotErrorIs(s.T(), err, redis.Nil, "expected parsing error, not redis.Nil")
				require.Equal(s.T(), "invalid cache: missing status", err.Error())
			},
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			rdb := testRedis(s.T())
			cache := NewBillingCache(rdb)
			ctx := context.Background()

			tt.fn(ctx, rdb, cache)
		})
	}
}

func TestBillingCacheSuite(t *testing.T) {
	suite.Run(t, new(BillingCacheSuite))
}
