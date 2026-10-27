package repository

import (
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestApplyUserSubscriptionLimitFieldsClearsNilLimits(t *testing.T) {
	client := dbent.NewClient()
	builder := client.UserSubscription.UpdateOneID(1)

	mutation := applyUserSubscriptionLimitFields(builder, &service.UserSubscription{}).Mutation()

	require.True(t, mutation.DailyLimitUsdCleared())
	require.True(t, mutation.WeeklyLimitUsdCleared())
	require.True(t, mutation.MonthlyLimitUsdCleared())
}

func TestApplyUserSubscriptionLimitFieldsSetsConfiguredLimits(t *testing.T) {
	daily := 10.0
	weekly := 40.0
	monthly := 800.0
	client := dbent.NewClient()
	builder := client.UserSubscription.UpdateOneID(1)

	mutation := applyUserSubscriptionLimitFields(builder, &service.UserSubscription{
		DailyLimitUSD:   &daily,
		WeeklyLimitUSD:  &weekly,
		MonthlyLimitUSD: &monthly,
	}).Mutation()

	actualDaily, dailySet := mutation.DailyLimitUsd()
	actualWeekly, weeklySet := mutation.WeeklyLimitUsd()
	actualMonthly, monthlySet := mutation.MonthlyLimitUsd()
	require.True(t, dailySet)
	require.True(t, weeklySet)
	require.True(t, monthlySet)
	require.Equal(t, daily, actualDaily)
	require.Equal(t, weekly, actualWeekly)
	require.Equal(t, monthly, actualMonthly)
	require.False(t, mutation.DailyLimitUsdCleared())
	require.False(t, mutation.WeeklyLimitUsdCleared())
	require.False(t, mutation.MonthlyLimitUsdCleared())
}
