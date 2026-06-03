package repository

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/stretchr/testify/require"
)

func TestApplyUserUsageOverrideToBatch(t *testing.T) {
	todayRequests := int64(12)
	todayTokens := int64(3456)
	todayActualCost := 7.89
	totalTokens := int64(9999)
	totalActualCost := 88.88

	stats := &BatchUserUsageStats{
		UserID:          1,
		TodayRequests:   1,
		TodayTokens:     2,
		TodayActualCost: 3,
		TotalTokens:     4,
		TotalActualCost: 5,
	}

	applyUserUsageOverrideToBatch(stats, &usagestats.UserUsageOverride{
		TodayRequests:   &todayRequests,
		TodayTokens:     &todayTokens,
		TodayActualCost: &todayActualCost,
		TotalTokens:     &totalTokens,
		TotalActualCost: &totalActualCost,
	})

	require.Equal(t, todayRequests, stats.TodayRequests)
	require.Equal(t, todayTokens, stats.TodayTokens)
	require.Equal(t, todayActualCost, stats.TodayActualCost)
	require.Equal(t, totalTokens, stats.TotalTokens)
	require.Equal(t, totalActualCost, stats.TotalActualCost)
	require.True(t, stats.UsageOverridden)
}

func TestApplyUserUsageOverrideToBatchAddsTodayDeltaToTotals(t *testing.T) {
	todayTokens := int64(350)
	todayActualCost := 7.5

	stats := &BatchUserUsageStats{
		UserID:          1,
		TodayTokens:     50,
		TodayActualCost: 1.5,
		TotalTokens:     500,
		TotalActualCost: 10,
	}

	applyUserUsageOverrideToBatch(stats, &usagestats.UserUsageOverride{
		TodayTokens:     &todayTokens,
		TodayActualCost: &todayActualCost,
	})

	require.Equal(t, todayTokens, stats.TodayTokens)
	require.Equal(t, int64(800), stats.TotalTokens)
	require.Equal(t, todayActualCost, stats.TodayActualCost)
	require.Equal(t, 16.0, stats.TotalActualCost)
	require.True(t, stats.UsageOverridden)
}

func TestApplyUserUsageOverrideToDashboard(t *testing.T) {
	todayRequests := int64(12)
	todayTokens := int64(3456)
	todayActualCost := 7.89
	totalTokens := int64(9999)
	totalActualCost := 88.88

	stats := &UserDashboardStats{
		TodayRequests:   1,
		TodayTokens:     2,
		TodayCost:       3,
		TodayActualCost: 4,
		TotalTokens:     5,
		TotalCost:       6,
		TotalActualCost: 7,
	}

	applyUserUsageOverrideToDashboard(stats, &usagestats.UserUsageOverride{
		TodayRequests:   &todayRequests,
		TodayTokens:     &todayTokens,
		TodayActualCost: &todayActualCost,
		TotalTokens:     &totalTokens,
		TotalActualCost: &totalActualCost,
	})

	require.Equal(t, todayRequests, stats.TodayRequests)
	require.Equal(t, todayTokens, stats.TodayTokens)
	require.Equal(t, todayActualCost, stats.TodayActualCost)
	require.Equal(t, todayActualCost, stats.TodayCost)
	require.Equal(t, totalTokens, stats.TotalTokens)
	require.Equal(t, totalActualCost, stats.TotalActualCost)
	require.Equal(t, totalActualCost, stats.TotalCost)
	require.True(t, stats.UsageOverridden)
}

func TestApplyUserUsageOverrideToDashboardAddsTodayDeltaToTotals(t *testing.T) {
	todayRequests := int64(12)
	todayTokens := int64(350)
	todayActualCost := 7.5

	stats := &UserDashboardStats{
		TodayRequests:   2,
		TodayTokens:     50,
		TodayActualCost: 1.5,
		TodayCost:       1.5,
		TotalRequests:   20,
		TotalTokens:     500,
		TotalActualCost: 10,
		TotalCost:       10,
	}

	applyUserUsageOverrideToDashboard(stats, &usagestats.UserUsageOverride{
		TodayRequests:   &todayRequests,
		TodayTokens:     &todayTokens,
		TodayActualCost: &todayActualCost,
	})

	require.Equal(t, todayRequests, stats.TodayRequests)
	require.Equal(t, int64(30), stats.TotalRequests)
	require.Equal(t, todayTokens, stats.TodayTokens)
	require.Equal(t, int64(800), stats.TotalTokens)
	require.Equal(t, todayActualCost, stats.TodayActualCost)
	require.Equal(t, 16.0, stats.TotalActualCost)
	require.Equal(t, 16.0, stats.TotalCost)
	require.True(t, stats.UsageOverridden)
}
