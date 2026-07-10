package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type subscriptionEligibilityCacheStub struct {
	BillingCache
	data        *SubscriptionCacheData
	invalidated bool
}

func (c *subscriptionEligibilityCacheStub) GetSubscriptionCache(context.Context, int64, int64) (*SubscriptionCacheData, error) {
	return c.data, nil
}

func (c *subscriptionEligibilityCacheStub) SetSubscriptionCache(context.Context, int64, int64, *SubscriptionCacheData) error {
	return nil
}

func (c *subscriptionEligibilityCacheStub) InvalidateSubscriptionCache(context.Context, int64, int64) error {
	c.invalidated = true
	c.data = nil
	return nil
}

type subscriptionEligibilityRepoStub struct {
	userSubRepoNoop
	sub                *UserSubscription
	resetMonthlyCalled bool
}

func (r *subscriptionEligibilityRepoStub) GetActiveByUserIDAndGroupID(context.Context, int64, int64) (*UserSubscription, error) {
	return r.sub, nil
}

func (r *subscriptionEligibilityRepoStub) ResetMonthlyUsage(_ context.Context, _ int64, _ *time.Time, windowStart time.Time) error {
	r.resetMonthlyCalled = true
	r.sub.MonthlyUsageUSD = 0
	r.sub.MonthlyWindowStart = &windowStart
	return nil
}

func TestCheckBillingEligibility_GroupRollingMonthlyBeforeWindowEndRejects(t *testing.T) {
	startsAt := time.Now().Add(-29 * 24 * time.Hour).Truncate(time.Second)
	sub := &UserSubscription{
		ID:                 1,
		UserID:             10,
		GroupID:            20,
		Status:             SubscriptionStatusActive,
		StartsAt:           startsAt,
		ExpiresAt:          startsAt.Add(31 * 24 * time.Hour),
		MonthlyUsageUSD:    1200,
		MonthlyWindowStart: &startsAt,
	}
	group := &Group{
		ID:                    20,
		SubscriptionType:      SubscriptionTypeSubscription,
		Status:                StatusActive,
		MonthlyLimitUSD:       ptrFloat64(1200),
		QuotaMonthlyResetMode: QuotaResetModeRolling,
	}
	cache := &subscriptionEligibilityCacheStub{data: &SubscriptionCacheData{
		Status:       sub.Status,
		ExpiresAt:    sub.ExpiresAt,
		MonthlyUsage: sub.MonthlyUsageUSD,
	}}
	repo := &subscriptionEligibilityRepoStub{sub: sub}
	svc := &BillingCacheService{cache: cache, subRepo: repo, cfg: &config.Config{}}

	err := svc.CheckBillingEligibility(context.Background(), &User{ID: 10}, nil, group, sub, "anthropic")

	require.ErrorIs(t, err, ErrMonthlyLimitExceeded)
	require.False(t, repo.resetMonthlyCalled)
	require.False(t, cache.invalidated)
}

func TestCheckBillingEligibility_GroupRollingMonthlyAfterWindowEndResetsAndAllows(t *testing.T) {
	startsAt := time.Now().Add(-31 * 24 * time.Hour).Truncate(time.Second)
	sub := &UserSubscription{
		ID:                 1,
		UserID:             10,
		GroupID:            20,
		Status:             SubscriptionStatusActive,
		StartsAt:           startsAt,
		ExpiresAt:          startsAt.Add(40 * 24 * time.Hour),
		MonthlyUsageUSD:    1200,
		MonthlyWindowStart: &startsAt,
	}
	group := &Group{
		ID:                    20,
		SubscriptionType:      SubscriptionTypeSubscription,
		Status:                StatusActive,
		MonthlyLimitUSD:       ptrFloat64(1200),
		QuotaMonthlyResetMode: QuotaResetModeRolling,
	}
	cache := &subscriptionEligibilityCacheStub{data: &SubscriptionCacheData{
		Status:       sub.Status,
		ExpiresAt:    sub.ExpiresAt,
		MonthlyUsage: sub.MonthlyUsageUSD,
	}}
	repo := &subscriptionEligibilityRepoStub{sub: sub}
	svc := &BillingCacheService{cache: cache, subRepo: repo, cfg: &config.Config{}}

	err := svc.CheckBillingEligibility(context.Background(), &User{ID: 10}, nil, group, sub, "anthropic")

	require.NoError(t, err)
	require.True(t, repo.resetMonthlyCalled)
	require.True(t, cache.invalidated)
	require.Zero(t, sub.MonthlyUsageUSD)
}

func TestCheckBillingEligibility_GroupFixedMonthlyAfterBoundaryResetsAndAllows(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	now := time.Now().In(loc)
	thisMonthReset := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
	if now.Before(thisMonthReset) {
		thisMonthReset = thisMonthReset.AddDate(0, -1, 0)
	}
	windowStart := thisMonthReset.AddDate(0, -1, 0).UTC()
	sub := &UserSubscription{
		ID:                 1,
		UserID:             10,
		GroupID:            20,
		Status:             SubscriptionStatusActive,
		StartsAt:           windowStart.Add(-24 * time.Hour),
		ExpiresAt:          now.Add(30 * 24 * time.Hour).UTC(),
		MonthlyUsageUSD:    1200,
		MonthlyWindowStart: &windowStart,
	}
	group := &Group{
		ID:                    20,
		SubscriptionType:      SubscriptionTypeSubscription,
		Status:                StatusActive,
		MonthlyLimitUSD:       ptrFloat64(1200),
		QuotaMonthlyResetMode: QuotaResetModeFixed,
		QuotaMonthlyResetDay:  1,
		QuotaMonthlyResetHour: 0,
		QuotaResetTimezone:    "Asia/Shanghai",
	}
	cache := &subscriptionEligibilityCacheStub{data: &SubscriptionCacheData{
		Status:       sub.Status,
		ExpiresAt:    sub.ExpiresAt,
		MonthlyUsage: sub.MonthlyUsageUSD,
	}}
	repo := &subscriptionEligibilityRepoStub{sub: sub}
	svc := &BillingCacheService{cache: cache, subRepo: repo, cfg: &config.Config{}}

	err = svc.CheckBillingEligibility(context.Background(), &User{ID: 10}, nil, group, sub, "anthropic")

	require.NoError(t, err)
	require.True(t, repo.resetMonthlyCalled)
	require.True(t, cache.invalidated)
	require.Equal(t, thisMonthReset.UTC(), sub.MonthlyWindowStart.UTC())
	require.Zero(t, sub.MonthlyUsageUSD)
}
