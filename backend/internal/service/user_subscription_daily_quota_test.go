package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type dailyResetTrackingUserSubRepo struct {
	userSubRepoNoop

	resetDailyCalled   bool
	resetWeeklyCalled  bool
	resetMonthlyCalled bool
}

func (r *dailyResetTrackingUserSubRepo) ResetDailyUsage(context.Context, int64, *time.Time, time.Time) error {
	r.resetDailyCalled = true
	return nil
}

func (r *dailyResetTrackingUserSubRepo) ResetWeeklyUsage(context.Context, int64, *time.Time, time.Time) error {
	r.resetWeeklyCalled = true
	return nil
}

func (r *dailyResetTrackingUserSubRepo) ResetMonthlyUsage(context.Context, int64, *time.Time, time.Time) error {
	r.resetMonthlyCalled = true
	return nil
}

func TestAssignOrExtendSubscription_ExpiredDailyCardStartsNewOneTimeQuota(t *testing.T) {
	groupRepo := &subscriptionGroupRepoStub{
		group: &Group{ID: 1, SubscriptionType: SubscriptionTypeSubscription},
	}
	subRepo := newSubscriptionUserSubRepoStub()
	oldStart := time.Now().AddDate(0, 0, -3)
	oldWindowStart := startOfDay(oldStart)
	subRepo.seed(&UserSubscription{
		ID:                 100,
		UserID:             200,
		GroupID:            1,
		StartsAt:           oldStart,
		ExpiresAt:          oldStart.AddDate(0, 0, 1),
		Status:             SubscriptionStatusExpired,
		DailyWindowStart:   &oldWindowStart,
		WeeklyWindowStart:  &oldWindowStart,
		MonthlyWindowStart: &oldWindowStart,
		DailyUsageUSD:      10,
		WeeklyUsageUSD:     20,
		MonthlyUsageUSD:    30,
		Notes:              "old",
	})
	svc := NewSubscriptionService(groupRepo, subRepo, nil, nil, nil)

	renewed, reused, err := svc.AssignOrExtendSubscription(context.Background(), &AssignSubscriptionInput{
		UserID:       200,
		GroupID:      1,
		ValidityDays: 1,
		Notes:        "new",
	})

	require.NoError(t, err)
	require.True(t, reused)
	require.Equal(t, SubscriptionStatusActive, renewed.Status)
	require.True(t, renewed.StartsAt.After(oldStart), "重新购买过期订阅时应重置当前周期 StartsAt")
	require.False(t, renewed.ExpiresAt.After(renewed.StartsAt.AddDate(0, 0, 1)))
	require.NotNil(t, renewed.DailyWindowStart)
	require.Equal(t, renewed.StartsAt, *renewed.DailyWindowStart)
	require.Equal(t, 0.0, renewed.DailyUsageUSD)
	require.Equal(t, 0.0, renewed.WeeklyUsageUSD)
	require.Equal(t, 0.0, renewed.MonthlyUsageUSD)
	require.Equal(t, "old\nnew", renewed.Notes)
}

func TestAssignOrExtendSubscription_ExpiredSubscriptionAppendsMatchingNotes(t *testing.T) {
	groupRepo := &subscriptionGroupRepoStub{
		group: &Group{ID: 1, SubscriptionType: SubscriptionTypeSubscription},
	}
	subRepo := newSubscriptionUserSubRepoStub()
	oldStart := time.Now().AddDate(0, 0, -3)
	subRepo.seed(&UserSubscription{
		ID:        101,
		UserID:    201,
		GroupID:   1,
		StartsAt:  oldStart,
		ExpiresAt: oldStart.AddDate(0, 0, 1),
		Status:    SubscriptionStatusExpired,
		Notes:     "same",
	})
	svc := NewSubscriptionService(groupRepo, subRepo, nil, nil, nil)

	renewed, reused, err := svc.AssignOrExtendSubscription(context.Background(), &AssignSubscriptionInput{
		UserID:       201,
		GroupID:      1,
		ValidityDays: 1,
		Notes:        "same",
	})

	require.NoError(t, err)
	require.True(t, reused)
	require.Equal(t, "same\nsame", renewed.Notes)
}

func TestUserSubscriptionNeedsDailyReset_DailyWindowAnchoredAtStartsAt(t *testing.T) {
	start := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	sub := &UserSubscription{
		StartsAt:         start,
		ExpiresAt:        start.Add(24 * time.Hour),
		DailyWindowStart: &start,
		DailyUsageUSD:    10,
	}

	require.False(t, sub.NeedsDailyResetAt(start.Add(23*time.Hour)), "daily window should not reset before starts_at+24h")
	require.False(t, sub.NeedsDailyResetAt(start.Add(24*time.Hour)), "subscription expires when the first 24h window ends")
}

func TestUserSubscriptionNeedsDailyReset_MultiDaySubscriptionRefreshesAtStartsAtPlus24h(t *testing.T) {
	start := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	sub := &UserSubscription{
		StartsAt:         start,
		ExpiresAt:        start.AddDate(0, 0, 2),
		DailyWindowStart: &start,
	}

	require.False(t, sub.NeedsDailyResetAt(start.Add(24*time.Hour-time.Nanosecond)), "daily window should stay active until starts_at+24h")
	require.True(t, sub.NeedsDailyResetAt(start.Add(24*time.Hour)), "multi-day subscriptions should refresh at starts_at+24h")
}

func TestUserSubscriptionDailyResetTime_ReturnsWindowEndCappedByExpiry(t *testing.T) {
	start := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	expiresAt := start.Add(24 * time.Hour)
	sub := &UserSubscription{
		StartsAt:         start,
		ExpiresAt:        expiresAt,
		DailyWindowStart: &start,
	}

	resetAt := sub.DailyResetTime()
	require.NotNil(t, resetAt)
	require.Equal(t, expiresAt, *resetAt)
}

func TestCheckAndResetWindows_DailyWindowBeforeStartsAtPlus24hDoesNotReset(t *testing.T) {
	now := time.Now()
	startsAt := now.Add(-23 * time.Hour)
	repo := &dailyResetTrackingUserSubRepo{}
	svc := NewSubscriptionService(groupRepoNoop{}, repo, nil, nil, nil)
	sub := &UserSubscription{
		ID:               1,
		UserID:           10,
		GroupID:          20,
		StartsAt:         startsAt,
		ExpiresAt:        startsAt.Add(24 * time.Hour),
		DailyUsageUSD:    10,
		DailyWindowStart: &startsAt,
	}

	err := svc.CheckAndResetWindows(context.Background(), sub, nil)

	require.NoError(t, err)
	require.False(t, repo.resetDailyCalled)
	require.Equal(t, 10.0, sub.DailyUsageUSD)
}

func TestCheckAndResetWindows_MultiDaySubscriptionResetsToCurrentAnchoredDailyWindow(t *testing.T) {
	now := time.Now()
	startsAt := now.Add(-49 * time.Hour)
	dailyWindowStart := startsAt.Add(24 * time.Hour)
	expectedWindowStart := startsAt.Add(48 * time.Hour)
	repo := &dailyResetTrackingUserSubRepo{}
	svc := NewSubscriptionService(groupRepoNoop{}, repo, nil, nil, nil)
	sub := &UserSubscription{
		ID:               1,
		UserID:           10,
		GroupID:          20,
		StartsAt:         startsAt,
		ExpiresAt:        startsAt.AddDate(0, 0, 4),
		DailyUsageUSD:    10,
		DailyWindowStart: &dailyWindowStart,
	}

	err := svc.CheckAndResetWindows(context.Background(), sub, nil)

	require.NoError(t, err)
	require.True(t, repo.resetDailyCalled)
	require.Equal(t, 0.0, sub.DailyUsageUSD)
	require.Equal(t, expectedWindowStart, *sub.DailyWindowStart)
}

func TestValidateAndCheckLimits_DailyCardDoesNotResetBeforeStartsAtPlus24h(t *testing.T) {
	start := time.Now().Add(-23 * time.Hour)
	dailyLimit := 10.0
	sub := &UserSubscription{
		Status:           SubscriptionStatusActive,
		StartsAt:         start,
		ExpiresAt:        start.Add(24 * time.Hour),
		DailyWindowStart: &start,
		DailyUsageUSD:    dailyLimit + 0.01,
	}
	group := &Group{
		SubscriptionType: SubscriptionTypeSubscription,
		DailyLimitUSD:    &dailyLimit,
	}
	svc := NewSubscriptionService(groupRepoNoop{}, userSubRepoNoop{}, nil, nil, nil)

	needsMaintenance, err := svc.ValidateAndCheckLimits(sub, group)

	require.False(t, needsMaintenance)
	require.True(t, errors.Is(err, ErrDailyLimitExceeded))
	require.Equal(t, dailyLimit+0.01, sub.DailyUsageUSD)
}
