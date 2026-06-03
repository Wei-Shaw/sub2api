package service

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/stretchr/testify/require"
)

func TestApplyUsageStatsTodayOverride(t *testing.T) {
	todayRequests := int64(12)
	todayTokens := int64(3456)
	todayActualCost := 7.89
	totalTokens := int64(9999)
	totalActualCost := 88.88

	stats := &UsageStats{
		TotalRequests:   1,
		TotalTokens:     2,
		TotalCost:       3,
		TotalActualCost: 4,
	}

	applyUsageStatsTodayOverride(stats, &usagestats.UserUsageOverride{
		TodayRequests:   &todayRequests,
		TodayTokens:     &todayTokens,
		TodayActualCost: &todayActualCost,
		TotalTokens:     &totalTokens,
		TotalActualCost: &totalActualCost,
	})

	require.Equal(t, todayRequests, stats.TotalRequests)
	require.Equal(t, todayTokens, stats.TotalTokens)
	require.Equal(t, todayActualCost, stats.TotalActualCost)
	require.Equal(t, todayActualCost, stats.TotalCost)
}

func TestApplyUsageStatsRangeTodayOverrideAddsDeltaToSelectedRange(t *testing.T) {
	todayRequests := int64(12)
	todayTokens := int64(350)
	todayActualCost := 7.5

	stats := &UsageStats{
		TotalRequests:   20,
		TotalTokens:     500,
		TotalCost:       10,
		TotalActualCost: 10,
	}
	actualToday := &UsageStats{
		TotalRequests:   2,
		TotalTokens:     50,
		TotalCost:       1.5,
		TotalActualCost: 1.5,
	}

	applyUsageStatsRangeTodayOverride(stats, actualToday, &usagestats.UserUsageOverride{
		TodayRequests:   &todayRequests,
		TodayTokens:     &todayTokens,
		TodayActualCost: &todayActualCost,
	})

	require.Equal(t, int64(30), stats.TotalRequests)
	require.Equal(t, int64(800), stats.TotalTokens)
	require.Equal(t, 16.0, stats.TotalActualCost)
	require.Equal(t, 16.0, stats.TotalCost)
}

func TestApplyUsageStatsTotalOverride(t *testing.T) {
	todayRequests := int64(12)
	todayTokens := int64(3456)
	todayActualCost := 7.89
	totalTokens := int64(9999)
	totalActualCost := 88.88

	stats := &UsageStats{
		TotalRequests:   1,
		TotalTokens:     2,
		TotalCost:       3,
		TotalActualCost: 4,
	}

	applyUsageStatsTotalOverride(stats, &usagestats.UserUsageOverride{
		TodayRequests:   &todayRequests,
		TodayTokens:     &todayTokens,
		TodayActualCost: &todayActualCost,
		TotalTokens:     &totalTokens,
		TotalActualCost: &totalActualCost,
	})

	require.Equal(t, int64(1), stats.TotalRequests)
	require.Equal(t, totalTokens, stats.TotalTokens)
	require.Equal(t, totalActualCost, stats.TotalActualCost)
	require.Equal(t, totalActualCost, stats.TotalCost)
}

func TestApplyUserDashboardStatsOverrideAddsTodayDeltaToTotals(t *testing.T) {
	todayRequests := int64(12)
	todayTokens := int64(350)
	todayActualCost := 7.5

	stats := &usagestats.UserDashboardStats{
		TodayRequests:   2,
		TodayTokens:     50,
		TodayActualCost: 1.5,
		TodayCost:       1.5,
		TotalRequests:   20,
		TotalTokens:     500,
		TotalActualCost: 10,
		TotalCost:       10,
	}

	applyUserDashboardStatsOverride(stats, &usagestats.UserUsageOverride{
		TodayRequests:   &todayRequests,
		TodayTokens:     &todayTokens,
		TodayActualCost: &todayActualCost,
	})

	require.Equal(t, todayRequests, stats.TodayRequests)
	require.Equal(t, int64(30), stats.TotalRequests)
	require.Equal(t, todayTokens, stats.TodayTokens)
	require.Equal(t, int64(800), stats.TotalTokens)
	require.Equal(t, todayActualCost, stats.TodayActualCost)
	require.Equal(t, todayActualCost, stats.TodayCost)
	require.Equal(t, 16.0, stats.TotalActualCost)
	require.Equal(t, 16.0, stats.TotalCost)
	require.True(t, stats.UsageOverridden)
}

func TestApplyUserDashboardStatsOverrideUsesExplicitTotals(t *testing.T) {
	todayTokens := int64(350)
	todayActualCost := 7.5
	totalTokens := int64(999)
	totalActualCost := 88.8

	stats := &usagestats.UserDashboardStats{
		TodayTokens:     50,
		TodayActualCost: 1.5,
		TotalTokens:     500,
		TotalActualCost: 10,
		TotalCost:       10,
	}

	applyUserDashboardStatsOverride(stats, &usagestats.UserUsageOverride{
		TodayTokens:     &todayTokens,
		TodayActualCost: &todayActualCost,
		TotalTokens:     &totalTokens,
		TotalActualCost: &totalActualCost,
	})

	require.Equal(t, todayTokens, stats.TodayTokens)
	require.Equal(t, totalTokens, stats.TotalTokens)
	require.Equal(t, todayActualCost, stats.TodayActualCost)
	require.Equal(t, totalActualCost, stats.TotalActualCost)
	require.Equal(t, totalActualCost, stats.TotalCost)
	require.True(t, stats.UsageOverridden)
}

func TestApplyAPIKeyUsageStatsUserOverride(t *testing.T) {
	todayActualCost := 7.5
	totalActualCost := 88.8

	t.Run("today_delta_adjusts_total_when_total_not_explicit", func(t *testing.T) {
		stats := &usagestats.BatchAPIKeyUsageStats{
			APIKeyID:        10,
			TodayActualCost: 1.5,
			TotalActualCost: 10,
		}

		applyAPIKeyUsageStatsUserOverride(stats, &usagestats.UserUsageOverride{
			TodayActualCost: &todayActualCost,
		})

		require.Equal(t, todayActualCost, stats.TodayActualCost)
		require.Equal(t, 16.0, stats.TotalActualCost)
		require.True(t, stats.UsageOverridden)
	})

	t.Run("explicit_total_wins", func(t *testing.T) {
		stats := &usagestats.BatchAPIKeyUsageStats{
			APIKeyID:        10,
			TodayActualCost: 1.5,
			TotalActualCost: 10,
		}

		applyAPIKeyUsageStatsUserOverride(stats, &usagestats.UserUsageOverride{
			TodayActualCost: &todayActualCost,
			TotalActualCost: &totalActualCost,
		})

		require.Equal(t, todayActualCost, stats.TodayActualCost)
		require.Equal(t, totalActualCost, stats.TotalActualCost)
		require.True(t, stats.UsageOverridden)
	})
}

func TestUsageRangeCoversToday(t *testing.T) {
	require.NoError(t, timezone.Init("Asia/Shanghai"))

	now := timezone.Now()
	todayStart := timezone.StartOfDay(now)

	require.True(t, usageRangeCoversToday(todayStart, now))
	require.False(t, usageRangeCoversToday(now.AddDate(0, 0, -7), now))
	require.False(t, usageRangeCoversToday(time.Time{}, now))
}

func TestUsageTodayWindow(t *testing.T) {
	require.NoError(t, timezone.Init("Asia/Shanghai"))

	now := timezone.Now()
	todayStart := timezone.StartOfDay(now)

	start, end, ok := usageTodayWindow(now.AddDate(0, 0, -7), now)
	require.True(t, ok)
	require.Equal(t, todayStart, start)
	require.Equal(t, now, end)

	_, _, ok = usageTodayWindow(now.AddDate(0, 0, -7), todayStart)
	require.False(t, ok)
}
