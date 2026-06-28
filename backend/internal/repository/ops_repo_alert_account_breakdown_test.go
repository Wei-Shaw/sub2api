package repository

import (
	"context"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestListAlertAccountBreakdownMapsAccountRows(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	repo := &opsRepository{db: db}
	start := time.Date(2026, 6, 28, 15, 40, 0, 0, time.UTC)
	end := start.Add(5 * time.Minute)

	rows := sqlmock.NewRows([]string{
		"account_id",
		"account_name",
		"platform",
		"group_id",
		"group_name",
		"success_count",
		"error_count_total",
		"error_count_sla",
		"business_limited_count",
		"upstream_error_count",
		"request_count_sla",
		"error_rate",
		"duration_p95_ms",
		"duration_p99_ms",
		"duration_avg_ms",
		"duration_max_ms",
		"last_error_status_code",
		"last_error_type",
		"last_error_message",
	}).AddRow(
		int64(42),
		"acc-prod-1",
		service.PlatformOpenAI,
		int64(7),
		"main",
		int64(80),
		int64(21),
		int64(20),
		int64(1),
		int64(18),
		int64(100),
		20.0,
		56000.0,
		70000.0,
		41000.0,
		int64(71000),
		int64(500),
		"provider_error",
		"server overloaded",
	)

	mock.ExpectQuery(`(?s)WITH usage_agg.*FROM combined`).
		WithArgs(start, end, start, end, 5).
		WillReturnRows(rows)

	got, err := repo.ListAlertAccountBreakdown(context.Background(), &service.OpsAlertAccountBreakdownFilter{
		StartTime:  start,
		EndTime:    end,
		MetricType: "error_rate",
		Limit:      5,
	})
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.NoError(t, mock.ExpectationsWereMet())

	item := got[0]
	require.NotNil(t, item.AccountID)
	require.Equal(t, int64(42), *item.AccountID)
	require.Equal(t, "acc-prod-1", item.AccountName)
	require.Equal(t, service.PlatformOpenAI, item.Platform)
	require.NotNil(t, item.GroupID)
	require.Equal(t, int64(7), *item.GroupID)
	require.Equal(t, int64(20), item.ErrorCountSLA)
	require.InDelta(t, 20.0, item.ErrorRate, 0.0001)
	require.NotNil(t, item.DurationP95Ms)
	require.Equal(t, 56000, *item.DurationP95Ms)
	require.NotNil(t, item.LastErrorStatusCode)
	require.Equal(t, 500, *item.LastErrorStatusCode)
	require.Equal(t, "provider_error", item.LastErrorType)
}
