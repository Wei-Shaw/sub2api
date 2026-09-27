//go:build unit

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func opsRequestDetailEmptyRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"kind", "created_at", "request_id", "platform", "model", "duration_ms", "first_token_ms",
		"status_code", "error_id", "phase", "severity", "message", "user_id", "api_key_id", "account_id", "group_id", "stream",
	})
}

func TestOpsRepositoryListRequestDetails_CapsCountBeyond24h(t *testing.T) {
	for _, tc := range []struct {
		name       string
		page       int
		countLimit int
	}{
		{name: "first page", page: 1, countLimit: opsRequestDetailsCountCap + 1},
		{name: "page past the cap stays reachable", page: 300, countLimit: 300*50 + 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock := newSQLMock(t)
			repo := &opsRepository{db: db}
			start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
			end := start.Add(7 * 24 * time.Hour)
			filter := &service.OpsRequestDetailFilter{StartTime: &start, EndTime: &end, Kind: "error", Page: tc.page, PageSize: 50}

			mock.ExpectQuery(`(?s)SELECT COUNT\(1\) FROM \(SELECT 1 FROM combined WHERE kind = \$3 LIMIT \$4\) capped`).
				WithArgs(start, end, "error", tc.countLimit).
				WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(tc.countLimit))
			mock.ExpectQuery(`(?s)FROM combined\s+WHERE kind = \$3\s+ORDER BY created_at DESC\s+LIMIT \$4 OFFSET \$5`).
				WithArgs(start, end, "error", 50, (tc.page-1)*50).
				WillReturnRows(opsRequestDetailEmptyRows())

			_, total, err := repo.ListRequestDetails(context.Background(), filter)
			require.NoError(t, err)
			require.EqualValues(t, tc.countLimit, total)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestOpsRepositoryListRequestDetails_ExactCountWithin24h(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &opsRepository{db: db}
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	filter := &service.OpsRequestDetailFilter{StartTime: &start, EndTime: &end}

	mock.ExpectQuery(`(?s)\) SELECT COUNT\(1\) FROM combined\s*$`).
		WithArgs(start, end).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(123456))
	mock.ExpectQuery(`LIMIT \$3 OFFSET \$4`).
		WithArgs(start, end, 50, 0).
		WillReturnRows(opsRequestDetailEmptyRows())

	_, total, err := repo.ListRequestDetails(context.Background(), filter)
	require.NoError(t, err)
	require.EqualValues(t, 123456, total)
	require.NoError(t, mock.ExpectationsWereMet())
}
