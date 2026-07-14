//go:build unit

package repository

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	appTimezone "github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestFillDashboardEntityStatsUsesOneQuery(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	today := time.Date(2026, 7, 14, 0, 0, 0, 0, time.UTC)
	now := today.Add(12 * time.Hour)
	mock.ExpectQuery(regexp.QuoteMeta("WITH user_stats AS (")).
		WithArgs(today, service.StatusActive, service.StatusError, now).
		WillReturnRows(sqlmock.NewRows([]string{
			"total_users", "today_new_users", "total_api_keys", "active_api_keys",
			"total_accounts", "normal_accounts", "error_accounts", "ratelimit_accounts", "overload_accounts",
		}).AddRow(10, 2, 8, 7, 6, 4, 1, 1, 0))

	stats := &usagestats.DashboardStats{}
	repo := &usageLogRepository{sql: db}
	require.NoError(t, repo.fillDashboardEntityStats(context.Background(), stats, today, now))
	require.Equal(t, int64(10), stats.TotalUsers)
	require.Equal(t, int64(7), stats.ActiveAPIKeys)
	require.Equal(t, int64(4), stats.NormalAccounts)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestFillDashboardUsageStatsAggregatedUsesOneQuery(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	today := time.Date(2026, 7, 14, 0, 0, 0, 0, time.UTC)
	now := today.Add(12*time.Hour + 34*time.Minute)
	hourStart := now.In(appTimezone.Location()).Truncate(time.Hour)
	mock.ExpectQuery(regexp.QuoteMeta("COALESCE(SUM(total_requests), 0) AS total_requests")).
		WithArgs(today, hourStart).
		WillReturnRows(sqlmock.NewRows([]string{
			"total_requests", "total_input_tokens", "total_output_tokens", "total_cache_creation_tokens", "total_cache_read_tokens",
			"total_cost", "total_actual_cost", "total_account_cost", "total_duration_ms",
			"today_requests", "today_input_tokens", "today_output_tokens", "today_cache_creation_tokens", "today_cache_read_tokens",
			"today_cost", "today_actual_cost", "today_account_cost", "active_users", "hourly_active_users",
		}).AddRow(10, 100, 50, 20, 10, 3.0, 2.0, 1.0, 500,
			4, 40, 20, 8, 4, 1.2, 0.8, 0.4, 3, 2))

	stats := &usagestats.DashboardStats{}
	repo := &usageLogRepository{sql: db}
	require.NoError(t, repo.fillDashboardUsageStatsAggregated(context.Background(), stats, today, now))
	require.Equal(t, int64(180), stats.TotalTokens)
	require.Equal(t, int64(72), stats.TodayTokens)
	require.Equal(t, 50.0, stats.AverageDurationMs)
	require.Equal(t, int64(2), stats.HourlyActiveUsers)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetDashboardUserInsightsUsesOneUsageScanQuery(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	rows := sqlmock.NewRows([]string{
		"row_kind", "date", "user_id", "email", "username", "requests", "tokens", "cost", "actual_cost",
		"total_actual_cost", "total_requests", "total_tokens", "row_order",
	}).
		AddRow("trend", "2026-07-01", 7, "u@example.com", "user", 3, 30, 2.5, 1.5, 0, 0, 0, 1).
		AddRow("ranking", "", 7, "u@example.com", "", 3, 30, 0, 1.5, 4.5, 9, 90, 1)
	mock.ExpectQuery(regexp.QuoteMeta("WITH user_buckets AS MATERIALIZED (")).
		WithArgs(start, end, 12, 8).
		WillReturnRows(rows)

	repo := &usageLogRepository{sql: db}
	got, err := repo.GetDashboardUserInsights(context.Background(), start, end, "day", 12, 8)
	require.NoError(t, err)
	require.Len(t, got.Trend, 1)
	require.Equal(t, int64(30), got.Trend[0].Tokens)
	require.Len(t, got.Ranking.Ranking, 1)
	require.Equal(t, 4.5, got.Ranking.TotalActualCost)
	require.Equal(t, int64(90), got.Ranking.TotalTokens)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestFillSubjectDashboardUsageStatsCombinesTotalAndToday(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	today := time.Date(2026, 7, 14, 0, 0, 0, 0, time.UTC)
	mock.ExpectQuery(regexp.QuoteMeta("COUNT(*) FILTER (WHERE created_at >= $2) AS today_requests")).
		WithArgs(int64(17), today).
		WillReturnRows(sqlmock.NewRows([]string{
			"total_requests", "total_input_tokens", "total_output_tokens", "total_cache_creation_tokens", "total_cache_read_tokens",
			"total_cost", "total_actual_cost", "avg_duration_ms",
			"today_requests", "today_input_tokens", "today_output_tokens", "today_cache_creation_tokens", "today_cache_read_tokens",
			"today_cost", "today_actual_cost",
		}).AddRow(5, 50, 25, 10, 5, 2.5, 2.0, 80.0, 2, 20, 10, 4, 2, 1.0, 0.8))

	stats := &usagestats.UserDashboardStats{}
	repo := &usageLogRepository{sql: db}
	require.NoError(t, repo.fillSubjectDashboardUsageStats(context.Background(), stats, "user_id", 17, today))
	require.Equal(t, int64(90), stats.TotalTokens)
	require.Equal(t, int64(36), stats.TodayTokens)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestFillSubjectDashboardUsageStatsRejectsUnknownColumn(t *testing.T) {
	repo := &usageLogRepository{}
	err := repo.fillSubjectDashboardUsageStats(context.Background(), &usagestats.UserDashboardStats{}, "unsafe", 1, time.Now())
	require.ErrorContains(t, err, "unsupported dashboard subject column")
}
