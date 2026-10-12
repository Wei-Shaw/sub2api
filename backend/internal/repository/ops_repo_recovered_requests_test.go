//go:build unit

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestListRequestDetailsIncludesRecoveredUpstreamErrorsWhenRequested(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	start := time.Date(2026, time.September, 6, 5, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	requestID := "recovered-request"

	queryPattern := `(?s)WITH combined AS \(.*COALESCE\(o\.status_code, 0\) >= 400 OR o\.error_phase IN \('upstream', 'account_auth'\).*\)`
	mock.ExpectQuery(queryPattern).
		WithArgs(start, end, string(service.OpsRequestKindError), requestID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))

	mock.ExpectQuery(queryPattern).
		WithArgs(start, end, string(service.OpsRequestKindError), requestID, 10, 0).
		WillReturnRows(sqlmock.NewRows([]string{
			"kind", "created_at", "request_id", "platform", "model", "duration_ms", "first_token_ms", "status_code", "error_id",
			"phase", "severity", "message", "user_id", "api_key_id", "account_id", "group_id", "stream",
		}).AddRow(
			"error", end.Add(-time.Minute), requestID, "gemini", "gemini-2.5-pro", 1200, nil, 200, int64(42),
			"upstream", "P2", "Recovered upstream error 503", nil, nil, int64(7), int64(3), false,
		))

	repo := &opsRepository{db: db}
	items, total, err := repo.ListRequestDetails(context.Background(), &service.OpsRequestDetailFilter{
		StartTime:                &start,
		EndTime:                  &end,
		Kind:                     string(service.OpsRequestKindError),
		RequestID:                requestID,
		IncludeRecoveredUpstream: true,
		Page:                     1,
		PageSize:                 10,
	})
	if err != nil {
		t.Fatalf("ListRequestDetails: %v", err)
	}
	if total != 1 || len(items) != 1 {
		t.Fatalf("ListRequestDetails returned total=%d items=%d, want 1/1", total, len(items))
	}
	if items[0].StatusCode == nil || *items[0].StatusCode != 200 {
		t.Fatalf("recovered row status_code = %v, want 200", items[0].StatusCode)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestListRequestDetailsKeepsRecoveredUpstreamErrorsExcludedByDefault(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	start := time.Date(2026, time.September, 6, 5, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)

	queryPattern := `(?s)WITH combined AS \(.*AND COALESCE\(o\.status_code, 0\) >= 400\s*\)`
	mock.ExpectQuery(queryPattern).
		WithArgs(start, end).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(0)))
	mock.ExpectQuery(queryPattern).
		WithArgs(start, end, 10, 0).
		WillReturnRows(sqlmock.NewRows([]string{
			"kind", "created_at", "request_id", "platform", "model", "duration_ms", "status_code", "error_id",
			"phase", "severity", "message", "user_id", "api_key_id", "account_id", "group_id", "stream",
		}))

	repo := &opsRepository{db: db}
	items, total, err := repo.ListRequestDetails(context.Background(), &service.OpsRequestDetailFilter{
		StartTime: &start,
		EndTime:   &end,
		Page:      1,
		PageSize:  10,
	})
	if err != nil {
		t.Fatalf("ListRequestDetails: %v", err)
	}
	if total != 0 || len(items) != 0 {
		t.Fatalf("ListRequestDetails returned total=%d items=%d, want 0/0", total, len(items))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}
