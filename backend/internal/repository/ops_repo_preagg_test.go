package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestCountDistinctHourlyMetricBucketsIgnoresDimensionDuplicates(t *testing.T) {
	first := time.Date(2026, 7, 27, 1, 0, 0, 0, time.UTC)
	second := first.Add(time.Hour)
	rows := []opsHourlyMetricsRow{
		{bucketStart: second},
		{bucketStart: first},
		{bucketStart: second},
	}

	if got := countDistinctHourlyMetricBuckets(rows); got != 2 {
		t.Fatalf("countDistinctHourlyMetricBuckets() = %d, want 2", got)
	}
}

func TestInvalidatePreaggregatedMetricVersionsMakesRowsUnreadableAsV2(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New(): %v", err)
	}
	defer db.Close()

	mock.ExpectExec(`(?s)UPDATE ops_metrics_hourly SET metric_definition_version = 1.*metric_definition_version = \$3`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), service.OpsMetricDefinitionVersion).
		WillReturnResult(sqlmock.NewResult(0, 24))
	mock.ExpectExec(`(?s)UPDATE ops_metrics_daily SET metric_definition_version = 1.*metric_definition_version = \$3`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), service.OpsMetricDefinitionVersion).
		WillReturnResult(sqlmock.NewResult(0, 2))

	repo := &opsRepository{db: db}
	start := time.Date(2026, 7, 25, 0, 0, 0, 0, time.UTC)
	end := start.Add(48 * time.Hour)
	if err := repo.InvalidateHourlyMetricsVersion(context.Background(), start, end, service.OpsMetricDefinitionVersion); err != nil {
		t.Fatalf("InvalidateHourlyMetricsVersion(): %v", err)
	}
	if err := repo.InvalidateDailyMetricsVersion(context.Background(), start, end, service.OpsMetricDefinitionVersion); err != nil {
		t.Fatalf("InvalidateDailyMetricsVersion(): %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestUpsertHourlyMetricsWritesMetricDefinitionVersion(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New(): %v", err)
	}
	defer db.Close()

	mock.ExpectExec(`(?s)INSERT INTO ops_metrics_hourly .*metric_definition_version.*ON CONFLICT .*metric_definition_version = EXCLUDED.metric_definition_version`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))

	repo := &opsRepository{db: db}
	start := time.Date(2026, 7, 27, 1, 0, 0, 0, time.UTC)
	if err := repo.UpsertHourlyMetrics(context.Background(), start, start.Add(time.Hour)); err != nil {
		t.Fatalf("UpsertHourlyMetrics(): %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestUpsertDailyMetricsOnlyReadsAndWritesMetricDefinitionV2(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New(): %v", err)
	}
	defer db.Close()

	mock.ExpectExec(`(?s)INSERT INTO ops_metrics_daily .*metric_definition_version.*FROM ops_metrics_hourly.*metric_definition_version = 2.*ON CONFLICT .*metric_definition_version = EXCLUDED.metric_definition_version`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))

	repo := &opsRepository{db: db}
	start := time.Date(2026, 7, 26, 0, 0, 0, 0, time.UTC)
	if err := repo.UpsertDailyMetrics(context.Background(), start, start.Add(24*time.Hour)); err != nil {
		t.Fatalf("UpsertDailyMetrics(): %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestLatestPreaggregatedBucketsOnlyReadMetricDefinitionV2(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New(): %v", err)
	}
	defer db.Close()

	hour := time.Date(2026, 7, 27, 1, 0, 0, 0, time.UTC)
	day := time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC)
	mock.ExpectQuery(`SELECT MAX\(bucket_start\) FROM ops_metrics_hourly WHERE metric_definition_version = 2`).
		WillReturnRows(sqlmock.NewRows([]string{"max"}).AddRow(hour))
	mock.ExpectQuery(`SELECT MAX\(bucket_date\) FROM ops_metrics_daily WHERE metric_definition_version = 2`).
		WillReturnRows(sqlmock.NewRows([]string{"max"}).AddRow(day))

	repo := &opsRepository{db: db}
	if got, ok, err := repo.GetLatestHourlyBucketStart(context.Background()); err != nil || !ok || !got.Equal(hour) {
		t.Fatalf("GetLatestHourlyBucketStart() = %s, %v, %v", got, ok, err)
	}
	if got, ok, err := repo.GetLatestDailyBucketDate(context.Background()); err != nil || !ok || !got.Equal(day) {
		t.Fatalf("GetLatestDailyBucketDate() = %s, %v, %v", got, ok, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
