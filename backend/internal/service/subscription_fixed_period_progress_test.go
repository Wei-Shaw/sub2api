package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCalculateProgress_MonthCardUsesStartsAtPlus30DaysAsMonthlyResetTime(t *testing.T) {
	svc := newTestSubscriptionService()
	startsAt := time.Date(2026, 6, 11, 15, 20, 32, 0, time.UTC)
	monthlyWindowStart := startsAt
	expiresAt := startsAt.AddDate(0, 0, 30)

	sub := &UserSubscription{
		ID:                 1,
		StartsAt:           startsAt,
		ExpiresAt:          expiresAt,
		MonthlyUsageUSD:    1190.0,
		MonthlyWindowStart: ptrTime(monthlyWindowStart),
	}
	group := &Group{
		Name:            "Monthly",
		MonthlyLimitUSD: ptrFloat64(1200.0),
	}

	progress := svc.calculateProgress(sub, group)

	require.NotNil(t, progress.Monthly)
	assert.Equal(t, expiresAt, progress.Monthly.ResetsAt, "30-day subscriptions should show monthly quota ending at expires_at")
}

func TestCalculateProgress_GroupFixedMonthlyUsesConfiguredResetTime(t *testing.T) {
	svc := newTestSubscriptionService()
	loc, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	windowStart := time.Date(2026, 6, 1, 0, 0, 0, 0, loc)
	expectedReset := time.Date(2026, 7, 1, 0, 0, 0, 0, loc)

	sub := &UserSubscription{
		ID:                 1,
		StartsAt:           windowStart.Add(-24 * time.Hour),
		ExpiresAt:          expectedReset.Add(24 * time.Hour),
		MonthlyUsageUSD:    100,
		MonthlyWindowStart: ptrTime(windowStart),
	}
	group := &Group{
		Name:                  "Fixed Monthly",
		MonthlyLimitUSD:       ptrFloat64(1200),
		QuotaMonthlyResetMode: QuotaResetModeFixed,
		QuotaMonthlyResetDay:  1,
		QuotaMonthlyResetHour: 0,
		QuotaResetTimezone:    "Asia/Shanghai",
	}

	progress := svc.calculateProgress(sub, group)

	require.NotNil(t, progress.Monthly)
	assert.Equal(t, expectedReset.UTC(), progress.Monthly.ResetsAt.UTC())
}
