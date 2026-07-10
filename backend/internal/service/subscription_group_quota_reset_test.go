package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCheckAndResetWindows_GroupRollingMonthlyDoesNotResetAtCalendarMidnight(t *testing.T) {
	startsAt := time.Now().Add(-29*24*time.Hour - 23*time.Hour).Truncate(time.Second)
	repo := &dailyResetTrackingUserSubRepo{}
	svc := NewSubscriptionService(groupRepoNoop{}, repo, nil, nil, nil)
	sub := &UserSubscription{
		ID:                 1,
		UserID:             10,
		GroupID:            20,
		StartsAt:           startsAt,
		ExpiresAt:          startsAt.Add(30 * 24 * time.Hour),
		MonthlyUsageUSD:    1100,
		MonthlyWindowStart: &startsAt,
	}
	group := &Group{
		SubscriptionType:      SubscriptionTypeSubscription,
		QuotaMonthlyResetMode: QuotaResetModeRolling,
		QuotaResetTimezone:    "Asia/Shanghai",
		QuotaMonthlyResetDay:  1,
		QuotaMonthlyResetHour: 0,
	}

	err := svc.CheckAndResetWindows(context.Background(), sub, group)

	require.NoError(t, err)
	require.False(t, repo.resetMonthlyCalled)
	require.Equal(t, 1100.0, sub.MonthlyUsageUSD)
	require.Equal(t, startsAt, *sub.MonthlyWindowStart)
}

func TestCheckAndResetWindows_GroupFixedMonthlyResetsAtConfiguredBoundary(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	now := time.Now().In(loc)
	thisMonthReset := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
	if now.Before(thisMonthReset) {
		thisMonthReset = thisMonthReset.AddDate(0, -1, 0)
	}
	lastWindow := thisMonthReset.AddDate(0, -1, 0).UTC()
	expectedWindowStart := thisMonthReset.UTC()
	startsAt := lastWindow.Add(-12 * time.Hour)
	repo := &dailyResetTrackingUserSubRepo{}
	svc := NewSubscriptionService(groupRepoNoop{}, repo, nil, nil, nil)
	sub := &UserSubscription{
		ID:                 1,
		UserID:             10,
		GroupID:            20,
		StartsAt:           startsAt,
		ExpiresAt:          now.Add(30 * 24 * time.Hour).UTC(),
		MonthlyUsageUSD:    1100,
		MonthlyWindowStart: &lastWindow,
	}
	group := &Group{
		SubscriptionType:      SubscriptionTypeSubscription,
		QuotaMonthlyResetMode: QuotaResetModeFixed,
		QuotaMonthlyResetDay:  1,
		QuotaMonthlyResetHour: 0,
		QuotaResetTimezone:    "Asia/Shanghai",
	}

	err = svc.CheckAndResetWindows(context.Background(), sub, group)

	require.NoError(t, err)
	require.True(t, repo.resetMonthlyCalled)
	require.Equal(t, 0.0, sub.MonthlyUsageUSD)
	require.Equal(t, expectedWindowStart, sub.MonthlyWindowStart.UTC())
}
