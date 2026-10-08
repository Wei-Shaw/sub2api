package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestUserSubscriptionLimits_UserOverrideTakesPriority(t *testing.T) {
	groupDaily := 100.0
	userDaily := 10.0
	sub := &UserSubscription{
		DailyUsageUSD: 10,
		DailyLimitUSD: &userDaily,
	}
	group := &Group{DailyLimitUSD: &groupDaily}

	require.False(t, sub.CheckDailyLimit(group, 0.01))
	require.Equal(t, userDaily, *sub.EffectiveDailyLimitUSD(group))
}

func TestUserSubscriptionLimits_FallBackToGroup(t *testing.T) {
	groupWeekly := 50.0
	sub := &UserSubscription{WeeklyUsageUSD: 49}
	group := &Group{WeeklyLimitUSD: &groupWeekly}

	require.True(t, sub.CheckWeeklyLimit(group, 1))
	require.False(t, sub.CheckWeeklyLimit(group, 1.01))
	require.Equal(t, groupWeekly, *sub.EffectiveWeeklyLimitUSD(group))
}

func TestUpdateSubscriptionLimits_SetAndClearOverrides(t *testing.T) {
	daily := 12.0
	weekly := 60.0
	repo := newSubscriptionUserSubRepoStub()
	repo.seed(&UserSubscription{
		ID:        10,
		UserID:    20,
		GroupID:   30,
		Status:    SubscriptionStatusActive,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	})
	svc := NewSubscriptionService(groupRepoNoop{}, repo, nil, nil, nil)

	updated, err := svc.UpdateSubscriptionLimits(context.Background(), 10, UpdateSubscriptionLimitsInput{
		Daily:  SubscriptionLimitUpdate{Set: true, Value: &daily},
		Weekly: SubscriptionLimitUpdate{Set: true, Value: &weekly},
	})
	require.NoError(t, err)
	require.Equal(t, daily, *updated.DailyLimitUSD)
	require.Equal(t, weekly, *updated.WeeklyLimitUSD)
	require.Nil(t, updated.MonthlyLimitUSD)

	cleared, err := svc.UpdateSubscriptionLimits(context.Background(), 10, UpdateSubscriptionLimitsInput{
		Daily: SubscriptionLimitUpdate{Set: true, Value: nil},
	})
	require.NoError(t, err)
	require.Nil(t, cleared.DailyLimitUSD)
	require.Equal(t, weekly, *cleared.WeeklyLimitUSD)
}

func TestUpdateSubscriptionLimits_RejectsNonPositiveValue(t *testing.T) {
	zero := 0.0
	repo := newSubscriptionUserSubRepoStub()
	repo.seed(&UserSubscription{ID: 1, UserID: 2, GroupID: 3})
	svc := NewSubscriptionService(groupRepoNoop{}, repo, nil, nil, nil)

	_, err := svc.UpdateSubscriptionLimits(context.Background(), 1, UpdateSubscriptionLimitsInput{
		Daily: SubscriptionLimitUpdate{Set: true, Value: &zero},
	})
	require.ErrorIs(t, err, ErrInvalidSubscriptionLimit)
}

type subscriptionLimitCacheStub struct {
	BillingCache
	data *SubscriptionCacheData
}

func (s *subscriptionLimitCacheStub) GetSubscriptionCache(context.Context, int64, int64) (*SubscriptionCacheData, error) {
	return s.data, nil
}

func TestBillingEligibility_UsesCachedUserLimitBeforeGroupLimit(t *testing.T) {
	groupDaily := 100.0
	userDaily := 10.0
	cache := &subscriptionLimitCacheStub{
		data: &SubscriptionCacheData{
			Status:        SubscriptionStatusActive,
			ExpiresAt:     time.Now().Add(24 * time.Hour),
			DailyUsage:    10,
			DailyLimitUSD: &userDaily,
		},
	}
	svc := &BillingCacheService{cache: cache}

	err := svc.checkSubscriptionEligibility(
		context.Background(),
		1,
		&Group{ID: 2, DailyLimitUSD: &groupDaily},
		&UserSubscription{DailyLimitUSD: &userDaily},
	)
	require.True(t, errors.Is(err, ErrDailyLimitExceeded))
}

func TestBillingEligibility_FallsBackToGroupLimit(t *testing.T) {
	groupDaily := 100.0
	cache := &subscriptionLimitCacheStub{
		data: &SubscriptionCacheData{
			Status:     SubscriptionStatusActive,
			ExpiresAt:  time.Now().Add(24 * time.Hour),
			DailyUsage: 99,
		},
	}
	svc := &BillingCacheService{cache: cache}

	err := svc.checkSubscriptionEligibility(
		context.Background(),
		1,
		&Group{ID: 2, DailyLimitUSD: &groupDaily},
		&UserSubscription{},
	)
	require.NoError(t, err)
}

func TestBillingEligibility_NewCachePreservesClearedOverride(t *testing.T) {
	groupDaily := 100.0
	staleUserDaily := 10.0
	cache := &subscriptionLimitCacheStub{
		data: &SubscriptionCacheData{
			Status:             SubscriptionStatusActive,
			ExpiresAt:          time.Now().Add(24 * time.Hour),
			DailyUsage:         10,
			LimitSchemaVersion: SubscriptionLimitCacheSchemaVersion,
		},
	}
	svc := &BillingCacheService{cache: cache}

	err := svc.checkSubscriptionEligibility(
		context.Background(),
		1,
		&Group{ID: 2, DailyLimitUSD: &groupDaily},
		&UserSubscription{DailyLimitUSD: &staleUserDaily},
	)
	require.NoError(t, err)
}

func TestBillingEligibility_OldCacheUsesSubscriptionOverride(t *testing.T) {
	groupDaily := 100.0
	userDaily := 10.0
	cache := &subscriptionLimitCacheStub{
		data: &SubscriptionCacheData{
			Status:     SubscriptionStatusActive,
			ExpiresAt:  time.Now().Add(24 * time.Hour),
			DailyUsage: 10,
		},
	}
	svc := &BillingCacheService{cache: cache}

	err := svc.checkSubscriptionEligibility(
		context.Background(),
		1,
		&Group{ID: 2, DailyLimitUSD: &groupDaily},
		&UserSubscription{DailyLimitUSD: &userDaily},
	)
	require.ErrorIs(t, err, ErrDailyLimitExceeded)
}
