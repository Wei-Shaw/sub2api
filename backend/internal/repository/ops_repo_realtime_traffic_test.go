package repository

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestRealtimeBucketSeconds(t *testing.T) {
	require.Equal(t, 5, realtimeBucketSeconds(time.Minute))
	require.Equal(t, 15, realtimeBucketSeconds(5*time.Minute))
	require.Equal(t, 60, realtimeBucketSeconds(30*time.Minute))
	require.Equal(t, 60, realtimeBucketSeconds(time.Hour))
}

func TestGetRealtimeTrafficSummaryReturnsRealBucketsAndActualCost(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	start := time.Date(2026, 8, 2, 10, 0, 0, 0, time.UTC)
	end := start.Add(time.Minute)
	rows := sqlmock.NewRows([]string{"bucket", "request_total", "token_total", "actual_cost"}).
		AddRow(start, int64(1), int64(5_000), 0.01).
		AddRow(start.Add(5*time.Second), int64(2), int64(10_000), 0.02)
	mock.ExpectQuery(regexp.QuoteMeta("WITH buckets AS (")).
		WithArgs(start, end, start, end).
		WillReturnRows(rows)

	repo := &opsRepository{db: db}
	got, err := repo.GetRealtimeTrafficSummary(context.Background(), &service.OpsDashboardFilter{
		StartTime: start,
		EndTime:   end,
	})
	require.NoError(t, err)
	require.Equal(t, 5, got.BucketSeconds)
	require.Len(t, got.Points, 2)
	require.Equal(t, 12.0, got.Points[0].RPM)
	require.Equal(t, 1000.0, got.Points[0].TokensPerSecond)
	require.Equal(t, 24.0, got.Points[1].RPM)
	require.InDelta(t, 0.03, got.ActualCostTotal, 0.0000001)
	require.InDelta(t, 0.4, got.QPS.Peak, 0.0001)
	require.NoError(t, mock.ExpectationsWereMet())
}
