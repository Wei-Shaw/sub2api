package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestUserSubscriptionNeedsMonthlyReset_MonthWindowAnchoredAtStartsAt(t *testing.T) {
	startsAt := time.Date(2026, 6, 11, 15, 20, 32, 0, time.UTC)
	sub := &UserSubscription{
		StartsAt:           startsAt,
		ExpiresAt:          startsAt.AddDate(0, 0, 60),
		MonthlyWindowStart: &startsAt,
		MonthlyUsageUSD:    1190,
	}

	require.False(t, sub.NeedsMonthlyResetAt(startsAt.Add(30*24*time.Hour-time.Nanosecond)))
	require.True(t, sub.NeedsMonthlyResetAt(startsAt.Add(30*24*time.Hour)))
	require.Equal(t, startsAt.Add(30*24*time.Hour), sub.CurrentMonthlyWindowStart(startsAt.Add(30*24*time.Hour)))
}

func TestCheckAndResetWindows_MonthCardDoesNotResetMonthlyUsageBeforeStartsAtPlus30d(t *testing.T) {
	now := time.Now()
	startsAt := now.Add(-29*24*time.Hour - 23*time.Hour)
	repo := &dailyResetTrackingUserSubRepo{}
	svc := NewSubscriptionService(groupRepoNoop{}, repo, nil, nil, nil)
	sub := &UserSubscription{
		ID:                 1,
		UserID:             10,
		GroupID:            20,
		StartsAt:           startsAt,
		ExpiresAt:          startsAt.AddDate(0, 0, 30),
		MonthlyUsageUSD:    1190,
		MonthlyWindowStart: &startsAt,
	}

	err := svc.CheckAndResetWindows(context.Background(), sub, nil)

	require.NoError(t, err)
	require.False(t, repo.resetMonthlyCalled)
	require.Equal(t, 1190.0, sub.MonthlyUsageUSD)
}

func TestValidateAndCheckLimits_MonthCardDoesNotAllowSecondQuotaBeforeStartsAtPlus30d(t *testing.T) {
	now := time.Now()
	startsAt := now.Add(-29*24*time.Hour - 23*time.Hour)
	monthlyLimit := 1200.0
	sub := &UserSubscription{
		Status:             SubscriptionStatusActive,
		StartsAt:           startsAt,
		ExpiresAt:          startsAt.AddDate(0, 0, 30),
		MonthlyWindowStart: &startsAt,
		MonthlyUsageUSD:    monthlyLimit + 0.01,
	}
	group := &Group{
		SubscriptionType: SubscriptionTypeSubscription,
		MonthlyLimitUSD:  &monthlyLimit,
	}
	svc := NewSubscriptionService(groupRepoNoop{}, userSubRepoNoop{}, nil, nil, nil)

	needsMaintenance, err := svc.ValidateAndCheckLimits(sub, group)

	require.False(t, needsMaintenance)
	require.True(t, errors.Is(err, ErrMonthlyLimitExceeded))
	require.Equal(t, monthlyLimit+0.01, sub.MonthlyUsageUSD)
}

func TestUserSubscriptionCurrentWindowStart_UsesStartsAtAnchors(t *testing.T) {
	startsAt := time.Date(2026, 6, 11, 15, 20, 32, 0, time.UTC)
	now := startsAt.Add(29*24*time.Hour + 12*time.Hour)
	sub := &UserSubscription{StartsAt: startsAt}

	require.Equal(t, startsAt.Add(29*24*time.Hour), sub.CurrentDailyWindowStart(now))
	require.Equal(t, startsAt.Add(28*24*time.Hour), sub.CurrentWeeklyWindowStart(now))
	require.Equal(t, startsAt, sub.CurrentMonthlyWindowStart(now))
}

func TestUserSubscriptionNeedsWeeklyReset_WeeklyWindowAnchoredAtStartsAt(t *testing.T) {
	startsAt := time.Date(2026, 6, 11, 15, 20, 32, 0, time.UTC)
	sub := &UserSubscription{
		StartsAt:          startsAt,
		ExpiresAt:         startsAt.AddDate(0, 0, 30),
		WeeklyWindowStart: &startsAt,
	}

	require.False(t, sub.NeedsWeeklyResetAt(startsAt.Add(7*24*time.Hour-time.Nanosecond)))
	require.True(t, sub.NeedsWeeklyResetAt(startsAt.Add(7*24*time.Hour)))
	require.Equal(t, startsAt.Add(7*24*time.Hour), sub.CurrentWeeklyWindowStart(startsAt.Add(7*24*time.Hour)))
}

func TestCheckAndResetWindows_WeeklyWindowResetsToCurrentAnchoredWindow(t *testing.T) {
	now := time.Now()
	startsAt := now.Add(-15 * 24 * time.Hour)
	weeklyWindowStart := startsAt.Add(7 * 24 * time.Hour)
	expectedWindowStart := startsAt.Add(14 * 24 * time.Hour)
	repo := &dailyResetTrackingUserSubRepo{}
	svc := NewSubscriptionService(groupRepoNoop{}, repo, nil, nil, nil)
	sub := &UserSubscription{
		ID:                1,
		UserID:            10,
		GroupID:           20,
		StartsAt:          startsAt,
		ExpiresAt:         startsAt.AddDate(0, 0, 30),
		WeeklyUsageUSD:    10,
		WeeklyWindowStart: &weeklyWindowStart,
	}

	err := svc.CheckAndResetWindows(context.Background(), sub, nil)

	require.NoError(t, err)
	require.True(t, repo.resetWeeklyCalled)
	require.Equal(t, 0.0, sub.WeeklyUsageUSD)
	require.Equal(t, expectedWindowStart, *sub.WeeklyWindowStart)
}
