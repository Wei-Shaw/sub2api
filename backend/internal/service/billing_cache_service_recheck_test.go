//go:build unit

package service

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type billingRecheckCache struct {
	balanceEligibilityCacheStub
	subscription SubscriptionCacheData
	keyUsage     APIKeyRateLimitCacheData
	platforms    []string
	quota        UserPlatformQuotaCacheEntry
}

func (s *billingRecheckCache) GetSubscriptionCache(context.Context, int64, int64) (*SubscriptionCacheData, error) {
	return &s.subscription, nil
}

func (s *billingRecheckCache) GetAPIKeyRateLimit(context.Context, int64) (*APIKeyRateLimitCacheData, error) {
	return &s.keyUsage, nil
}

func (s *billingRecheckCache) GetUserPlatformQuotaCache(_ context.Context, _ int64, platform string) (*UserPlatformQuotaCacheEntry, bool, error) {
	s.platforms = append(s.platforms, platform)
	return &s.quota, true, nil
}

func TestRecheckBillingEligibilityPreservesGuardsWithoutRPM(t *testing.T) {
	for _, guard := range []string{"balance", "subscription", "key", "platform"} {
		t.Run(guard, func(t *testing.T) {
			now := time.Now()
			limit := 10.0
			cache := &billingRecheckCache{
				balanceEligibilityCacheStub: balanceEligibilityCacheStub{balance: 100},
				subscription:                SubscriptionCacheData{Status: SubscriptionStatusActive, ExpiresAt: now.Add(time.Hour)},
				keyUsage:                    APIKeyRateLimitCacheData{Window5h: now.Unix(), Window1d: now.Unix(), Window7d: now.Unix()},
				quota:                       UserPlatformQuotaCacheEntry{SchemaVersion: UserPlatformQuotaCacheSchemaV1, DailyLimitUSD: &limit, DailyWindowStart: &now, WeeklyWindowStart: &now, MonthlyWindowStart: &now},
			}
			rpm := &userRPMCacheStub{userCounts: []int{1, 2}}
			svc := NewBillingCacheService(cache, nil, nil, nil, rpm, nil, &config.Config{}, nil)
			t.Cleanup(svc.Stop)
			svc.userPlatformQuotaRepo = &fakeQuotaRepo{}
			user := &User{ID: 1, RPMLimit: 1}
			group := &Group{ID: 2}
			key := &APIKey{ID: 3, RateLimit5h: limit}
			var subscription *UserSubscription
			if guard == "subscription" {
				group.SubscriptionType, group.DailyLimitUSD = SubscriptionTypeSubscription, &limit
				subscription = &UserSubscription{ID: 4}
			}
			ctx := context.Background()
			require.NoError(t, svc.CheckBillingEligibility(ctx, user, key, group, subscription, PlatformOpenCodeGo))
			require.NoError(t, svc.RecheckBillingEligibility(ctx, user, key, group, subscription, PlatformOpenCodeGo))
			var want error
			switch guard {
			case "balance":
				cache.balance = 0
				want = ErrInsufficientBalance
			case "subscription":
				cache.subscription.DailyUsage = limit
				want = ErrDailyLimitExceeded
			case "key":
				cache.keyUsage.Usage5h = limit
				want = ErrAPIKeyRateLimit5hExceeded
			case "platform":
				cache.quota.DailyUsageUSD = limit
				want = ErrUserPlatformDailyQuotaExhausted
			}
			require.ErrorIs(t, svc.RecheckBillingEligibility(ctx, user, key, group, subscription, PlatformDeepseek), want)
			require.EqualValues(t, 1, atomic.LoadInt32(&rpm.userCalls))
			if guard == "platform" {
				require.Equal(t, []string{PlatformOpenCodeGo, PlatformOpenCodeGo, PlatformDeepseek}, cache.platforms)
			}
			cache.balance, cache.subscription.DailyUsage, cache.keyUsage.Usage5h, cache.quota.DailyUsageUSD = 100, 0, 0, 0
			require.ErrorIs(t, svc.CheckBillingEligibility(ctx, user, key, group, subscription, PlatformDeepseek), ErrUserRPMExceeded, "new requests must still enforce RPM")
		})
	}
}
