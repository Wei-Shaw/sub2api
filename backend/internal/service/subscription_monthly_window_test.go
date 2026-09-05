//go:build unit

package service

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/stretchr/testify/require"
)

type activateWindowUserSubRepo struct {
	userSubRepoNoop
	dailyStart    time.Time
	periodicStart time.Time
}

type monthlyResetUserSubRepo struct {
	userSubRepoNoop
	resetCalled bool
	resetAt     time.Time
}

func (r *monthlyResetUserSubRepo) ResetMonthlyUsage(_ context.Context, _ int64, _ *time.Time, resetAt time.Time) error {
	r.resetCalled = true
	r.resetAt = resetAt
	return nil
}

func (r *activateWindowUserSubRepo) ActivateWindows(_ context.Context, _ int64, dailyStart, periodicStart time.Time) error {
	r.dailyStart = dailyStart
	r.periodicStart = periodicStart
	return nil
}

func TestDelayedFirstUseAnchorsMonthlyWindowAtActivation(t *testing.T) {
	repo := &activateWindowUserSubRepo{}
	svc := NewSubscriptionService(groupRepoNoop{}, repo, nil, nil, nil)
	startsAt := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	activatedAt := time.Date(2026, 7, 10, 23, 30, 0, 0, time.UTC)
	svc.now = func() time.Time { return activatedAt }
	sub := &UserSubscription{
		ID:        1,
		StartsAt:  startsAt,
		ExpiresAt: startsAt.Add(45 * 24 * time.Hour),
	}

	require.NoError(t, svc.CheckAndActivateWindow(context.Background(), sub))

	require.Equal(t, activatedAt, repo.periodicStart)
	require.Equal(t, timezone.StartOfDay(activatedAt), repo.dailyStart)
	monthlyWindowStart := repo.periodicStart
	resetAt, ok := sub.automaticWindowStartAt(&monthlyWindowStart, 30*24*time.Hour, activatedAt.Add(30*24*time.Hour))
	require.True(t, ok)
	require.Equal(t, activatedAt.Add(30*24*time.Hour), resetAt)
	require.NotEqual(t, startsAt.Add(30*24*time.Hour), resetAt)
}

func TestThirtyDaySubscriptionDoesNotResetMonthlyQuotaBeforeExpiry(t *testing.T) {
	startsAt := time.Date(2026, 7, 1, 23, 30, 0, 0, time.UTC)
	expiresAt := startsAt.Add(30 * 24 * time.Hour)
	renewed := renewedSubscriptionTerm(&UserSubscription{}, "", startsAt, expiresAt)

	require.Equal(t, startsAt, *renewed.MonthlyWindowStart)
	require.False(t, renewed.NeedsMonthlyResetAt(expiresAt.Add(-time.Second)))
	require.True(t, renewed.NeedsMonthlyResetAt(expiresAt))
	require.False(t, renewed.canAutomaticallyResetMonthlyAt(expiresAt))
	require.Equal(t, expiresAt, *renewed.MonthlyResetTime())
}

func TestCheckAndResetWindowsDoesNotResetExactThirtyDayLegacyMonthlyWindow(t *testing.T) {
	windowStart := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	startsAt := time.Date(2026, 7, 1, 23, 30, 0, 0, time.UTC)
	now := startsAt.Add(30 * 24 * time.Hour)
	repo := &monthlyResetUserSubRepo{}
	svc := NewSubscriptionService(groupRepoNoop{}, repo, nil, nil, nil)
	svc.now = func() time.Time { return now }
	sub := &UserSubscription{
		ID:                 1,
		StartsAt:           startsAt,
		ExpiresAt:          startsAt.Add(30 * 24 * time.Hour),
		MonthlyWindowStart: &windowStart,
		MonthlyUsageUSD:    12,
	}

	require.NoError(t, svc.CheckAndResetWindows(context.Background(), sub))
	require.False(t, repo.resetCalled)
	require.Equal(t, 12.0, sub.MonthlyUsageUSD)
	require.Equal(t, windowStart, *sub.MonthlyWindowStart)
}

func TestCheckAndResetWindowsResetsPartialFinalMonthlySubscriptions(t *testing.T) {
	for _, durationDays := range []int{31, 45} {
		t.Run(strconv.Itoa(durationDays)+"_days", func(t *testing.T) {
			legacyWindowStart := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
			startsAt := time.Date(2026, 7, 1, 23, 30, 0, 0, time.UTC)
			now := startsAt.Add(30 * 24 * time.Hour)
			repo := &monthlyResetUserSubRepo{}
			svc := NewSubscriptionService(groupRepoNoop{}, repo, nil, nil, nil)
			svc.now = func() time.Time { return now }
			sub := &UserSubscription{
				ID:                 2,
				StartsAt:           startsAt,
				ExpiresAt:          startsAt.Add(time.Duration(durationDays) * 24 * time.Hour),
				MonthlyWindowStart: &legacyWindowStart,
				MonthlyUsageUSD:    12,
			}

			require.NoError(t, svc.CheckAndResetWindows(context.Background(), sub))
			require.True(t, repo.resetCalled)
			require.Equal(t, startsAt.Add(30*24*time.Hour), repo.resetAt)
			require.Zero(t, sub.MonthlyUsageUSD)
		})
	}
}

func TestNormalizeExpiredWindowsKeepsLegacyMonthlyUsageBeforeExpiry(t *testing.T) {
	windowStart := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	startsAt := windowStart.Add(23*time.Hour + 30*time.Minute)
	now := windowStart.Add(30 * 24 * time.Hour)
	subs := []UserSubscription{{
		StartsAt:           startsAt,
		ExpiresAt:          startsAt.Add(30 * 24 * time.Hour),
		MonthlyWindowStart: &windowStart,
		MonthlyUsageUSD:    12,
	}}

	normalizeExpiredWindowsAt(subs, now)

	require.Equal(t, 12.0, subs[0].MonthlyUsageUSD)
	require.Equal(t, windowStart, *subs[0].MonthlyWindowStart)
}

func TestNormalizeExpiredWindowsResetsMonthlyUsageWithPartialFinalPeriod(t *testing.T) {
	startsAt := time.Date(2026, 7, 1, 23, 30, 0, 0, time.UTC)
	windowStart := startsAt.Add(-23*time.Hour - 30*time.Minute)
	now := startsAt.Add(30 * 24 * time.Hour)
	subs := []UserSubscription{{
		StartsAt:           startsAt,
		ExpiresAt:          startsAt.Add(31 * 24 * time.Hour),
		MonthlyWindowStart: &windowStart,
		MonthlyUsageUSD:    12,
	}}

	normalizeExpiredWindowsAt(subs, now)

	require.Zero(t, subs[0].MonthlyUsageUSD)
	require.Nil(t, subs[0].MonthlyWindowStart)
}

func TestValidateAndCheckLimitsKeepsLegacyMonthlyUsageBeforeExpiry(t *testing.T) {
	windowStart := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	startsAt := windowStart.Add(23*time.Hour + 30*time.Minute)
	now := windowStart.Add(30 * 24 * time.Hour)
	limit := 10.0
	sub := &UserSubscription{
		Status:             SubscriptionStatusActive,
		StartsAt:           startsAt,
		ExpiresAt:          startsAt.Add(30 * 24 * time.Hour),
		MonthlyWindowStart: &windowStart,
		MonthlyUsageUSD:    12,
	}
	svc := NewSubscriptionService(groupRepoNoop{}, userSubRepoNoop{}, nil, nil, nil)
	svc.now = func() time.Time { return now }

	needsMaintenance, err := svc.ValidateAndCheckLimits(sub, &Group{MonthlyLimitUSD: &limit})

	require.ErrorIs(t, err, ErrMonthlyLimitExceeded)
	require.False(t, needsMaintenance)
	require.Equal(t, 12.0, sub.MonthlyUsageUSD)
}

func TestValidateAndCheckLimitsResetsMonthlyUsageWithPartialFinalPeriod(t *testing.T) {
	startsAt := time.Date(2026, 7, 1, 23, 30, 0, 0, time.UTC)
	windowStart := startsAt.Add(-23*time.Hour - 30*time.Minute)
	now := startsAt.Add(30 * 24 * time.Hour)
	limit := 10.0
	sub := &UserSubscription{
		Status:             SubscriptionStatusActive,
		StartsAt:           startsAt,
		ExpiresAt:          startsAt.Add(45 * 24 * time.Hour),
		MonthlyWindowStart: &windowStart,
		MonthlyUsageUSD:    12,
	}
	svc := NewSubscriptionService(groupRepoNoop{}, userSubRepoNoop{}, nil, nil, nil)
	svc.now = func() time.Time { return now }

	needsMaintenance, err := svc.ValidateAndCheckLimits(sub, &Group{MonthlyLimitUSD: &limit})

	require.NoError(t, err)
	require.True(t, needsMaintenance)
	require.Zero(t, sub.MonthlyUsageUSD)
}

func TestValidateAndCheckLimitsRejectsExactExpiry(t *testing.T) {
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	sub := &UserSubscription{Status: SubscriptionStatusActive, ExpiresAt: now}
	svc := NewSubscriptionService(groupRepoNoop{}, userSubRepoNoop{}, nil, nil, nil)
	svc.now = func() time.Time { return now }

	needsMaintenance, err := svc.ValidateAndCheckLimits(sub, &Group{})

	require.ErrorIs(t, err, ErrSubscriptionExpired)
	require.False(t, needsMaintenance)
}

func TestAutomaticWindowsAllowPartialFinalDailyAndWeeklyPeriods(t *testing.T) {
	startsAt := time.Date(2026, 7, 1, 15, 45, 0, 0, time.UTC)
	legacyWindowStart := startOfDay(startsAt)
	tests := []struct {
		name      string
		period    time.Duration
		expiresAt time.Time
	}{
		{name: "daily", period: 24 * time.Hour, expiresAt: startsAt.Add(36 * time.Hour)},
		{name: "weekly", period: 7 * 24 * time.Hour, expiresAt: startsAt.Add(10 * 24 * time.Hour)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sub := &UserSubscription{StartsAt: startsAt, ExpiresAt: tt.expiresAt}
			resetAt, ok := sub.automaticWindowStartAt(&legacyWindowStart, tt.period, startsAt.Add(tt.period))

			require.True(t, ok)
			require.Equal(t, startsAt.Add(tt.period), resetAt)
		})
	}
}

func TestAutomaticWindowPreservesPersistedManualAnchorAndAdvancesWholePeriods(t *testing.T) {
	startsAt := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	manualAnchor := startsAt.Add(5 * 24 * time.Hour)
	sub := &UserSubscription{StartsAt: startsAt, ExpiresAt: startsAt.Add(100 * 24 * time.Hour)}

	resetAt, ok := sub.automaticWindowStartAt(&manualAnchor, 30*24*time.Hour, manualAnchor.Add(65*24*time.Hour))

	require.True(t, ok)
	require.Equal(t, manualAnchor.Add(60*24*time.Hour), resetAt)
}

func TestAutomaticWindowPreservesPeriodAlignedLaterMidnightManualAnchor(t *testing.T) {
	startsAt := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	manualAnchor := startOfDay(startsAt).Add(30 * 24 * time.Hour)
	sub := &UserSubscription{StartsAt: startsAt, ExpiresAt: startsAt.Add(100 * 24 * time.Hour)}

	resetAt, ok := sub.automaticWindowStartAt(&manualAnchor, 30*24*time.Hour, startsAt.Add(60*24*time.Hour))

	require.True(t, ok)
	require.Equal(t, manualAnchor.Add(30*24*time.Hour), resetAt)
}

func TestAutomaticWindowPreservesExactMidnightManualAnchor(t *testing.T) {
	startsAt := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	manualAnchor := time.Date(2026, 1, 20, 0, 0, 0, 0, time.UTC)
	sub := &UserSubscription{StartsAt: startsAt, ExpiresAt: startsAt.Add(100 * 24 * time.Hour)}

	resetAt, ok := sub.automaticWindowStartAt(&manualAnchor, 30*24*time.Hour, manualAnchor.Add(30*24*time.Hour))

	require.True(t, ok)
	require.Equal(t, manualAnchor.Add(30*24*time.Hour), resetAt)
}
