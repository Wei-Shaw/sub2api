package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type fiveHourResetRepoStub struct {
	userSubRepoNoop

	activateFiveHourCalled bool
	activateWindowStart    time.Time
	resetFiveHourCalled    bool
	resetWindowStart       time.Time
}

func (r *fiveHourResetRepoStub) ActivateFiveHourWindow(_ context.Context, _ int64, windowStart time.Time) error {
	r.activateFiveHourCalled = true
	r.activateWindowStart = windowStart
	return nil
}

func (r *fiveHourResetRepoStub) ResetFiveHourUsage(_ context.Context, _ int64, windowStart time.Time) error {
	r.resetFiveHourCalled = true
	r.resetWindowStart = windowStart
	return nil
}

func TestValidateAndCheckLimits_MissingFiveHourWindowRequiresMaintenance(t *testing.T) {
	dailyStart := time.Now().Add(-time.Hour)
	group := &Group{FiveHourLimitUSD: ptrFloat64(10)}
	sub := &UserSubscription{
		ID:                 1,
		Status:             SubscriptionStatusActive,
		ExpiresAt:          time.Now().Add(24 * time.Hour),
		FiveHourUsageUSD:   99,
		DailyWindowStart:   &dailyStart,
		DailyUsageUSD:      1,
		WeeklyWindowStart:  &dailyStart,
		MonthlyWindowStart: &dailyStart,
	}

	needsMaintenance, err := (&SubscriptionService{}).ValidateAndCheckLimits(sub, group)

	require.NoError(t, err)
	require.True(t, needsMaintenance)
	require.Zero(t, sub.FiveHourUsageUSD)
	require.Same(t, group, sub.Group)
}

func TestCheckAndResetWindows_BackfillsMissingFiveHourWindow(t *testing.T) {
	windowStart := time.Now()
	group := &Group{FiveHourLimitUSD: ptrFloat64(10)}
	repo := &fiveHourResetRepoStub{}
	svc := NewSubscriptionService(groupRepoNoop{}, repo, nil, nil, nil)
	sub := &UserSubscription{
		ID:                 1,
		UserID:             2,
		GroupID:            3,
		Group:              group,
		FiveHourUsageUSD:   4,
		DailyWindowStart:   &windowStart,
		WeeklyWindowStart:  &windowStart,
		MonthlyWindowStart: &windowStart,
	}

	err := svc.CheckAndResetWindows(context.Background(), sub)

	require.NoError(t, err)
	require.True(t, repo.activateFiveHourCalled)
	require.False(t, repo.resetFiveHourCalled)
	require.NotNil(t, sub.FiveHourWindowStart)
	require.Equal(t, repo.activateWindowStart, *sub.FiveHourWindowStart)
	require.Equal(t, float64(4), sub.FiveHourUsageUSD)
}
