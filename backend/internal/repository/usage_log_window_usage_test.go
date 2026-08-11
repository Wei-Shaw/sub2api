package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestGetAccountWindowUsageUsesHalfOpenBatchQuery(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer func() { _ = db.Close() }()

	start := time.Date(2026, 8, 11, 8, 0, 0, 0, time.UTC)
	middle := start.Add(5 * time.Hour)
	end := middle.Add(5 * time.Hour)
	queries := []service.AccountWindowUsageQuery{
		{WindowKey: service.AccountWindowKeyFiveHour, Period: service.AccountWindowPeriodPrevious, StartTime: start, EndTime: middle},
		{WindowKey: service.AccountWindowKeyFiveHour, Period: service.AccountWindowPeriodCurrent, StartTime: middle, EndTime: end},
	}

	queryPattern := `(?s)FROM jsonb_to_recordset.*ul\.created_at >= t\.start_time.*ul\.created_at < t\.end_time.*error_log\.created_at >= t\.start_time.*error_log\.created_at < t\.end_time.*is_count_tokens.*error_log\.error_type = 'cyber_policy'.*error_log\.stream.*NOT LIKE 'Recovered upstream error%'.*ORDER BY t\.target_index`
	rows := sqlmock.NewRows([]string{
		"window_key", "period", "start_time", "end_time", "success_calls", "failure_calls",
		"total_tokens", "account_cost", "standard_cost", "user_cost",
	}).
		AddRow(service.AccountWindowKeyFiveHour, service.AccountWindowPeriodPrevious, start, middle, int64(3), int64(1), int64(1200), 2.5, 2.0, 3.0).
		AddRow(service.AccountWindowKeyFiveHour, service.AccountWindowPeriodCurrent, middle, end, int64(8), int64(2), int64(4800), 7.25, 6.5, 8.0)
	mock.ExpectQuery(queryPattern).
		WithArgs(int64(42), sqlmock.AnyArg()).
		WillReturnRows(rows)

	repo := newUsageLogRepositoryWithSQL(nil, db)
	result, err := repo.GetAccountWindowUsage(context.Background(), 42, queries)
	if err != nil {
		t.Fatalf("GetAccountWindowUsage() error = %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("result length = %d, want 2", len(result))
	}
	if result[0].SuccessCalls != 3 || result[0].FailureCalls != 1 || result[0].TotalTokens != 1200 {
		t.Fatalf("previous aggregate = %#v", result[0])
	}
	if result[1].AccountCost != 7.25 || result[1].StandardCost != 6.5 || result[1].UserCost != 8.0 {
		t.Fatalf("current costs = %#v", result[1])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("SQL expectations: %v", err)
	}
}

func TestGetAccountWindowUsageEmptyAndQueryError(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer func() { _ = db.Close() }()
	repo := newUsageLogRepositoryWithSQL(nil, db)

	empty, err := repo.GetAccountWindowUsage(context.Background(), 1, nil)
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty result = %#v, %v", empty, err)
	}

	queryErr := errors.New("database unavailable")
	mock.ExpectQuery(`(?s)FROM jsonb_to_recordset`).WillReturnError(queryErr)
	now := time.Now().UTC()
	_, err = repo.GetAccountWindowUsage(context.Background(), 1, []service.AccountWindowUsageQuery{{
		WindowKey: service.AccountWindowKeySevenDay,
		Period:    service.AccountWindowPeriodCurrent,
		StartTime: now.Add(-time.Hour),
		EndTime:   now,
	}})
	if !errors.Is(err, queryErr) {
		t.Fatalf("query error = %v, want %v", err, queryErr)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("SQL expectations: %v", err)
	}
}
