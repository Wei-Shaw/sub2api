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

func TestOpsRequestDetailsOrderBy(t *testing.T) {
	tests := []struct {
		name      string
		sort      string
		want      string
		wantError bool
	}{
		{name: "default", sort: "", want: "ORDER BY created_at DESC"},
		{name: "duration descending", sort: "duration_desc", want: "ORDER BY duration_ms DESC NULLS LAST, created_at DESC"},
		{name: "ttft descending", sort: "ttft_desc", want: "ORDER BY first_token_ms DESC NULLS LAST, created_at DESC"},
		{name: "ttft ascending", sort: "ttft_asc", want: "ORDER BY first_token_ms ASC NULLS LAST, created_at DESC"},
		{name: "normalizes whitespace and case", sort: " TTFT_DESC ", want: "ORDER BY first_token_ms DESC NULLS LAST, created_at DESC"},
		{name: "rejects unknown sort", sort: "ttft_sideways", wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := opsRequestDetailsOrderBy(tt.sort)
			if tt.wantError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestOpsRepositoryListRequestDetailsIncludesTTFTAndSortsBeforePagination(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &opsRepository{db: db}

	start := time.Date(2026, 8, 2, 10, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	hasTTFT := true
	filter := &service.OpsRequestDetailFilter{
		StartTime: &start,
		EndTime:   &end,
		Kind:      "success",
		HasTTFT:   &hasTTFT,
		Sort:      "ttft_desc",
		Page:      1,
		PageSize:  10,
	}

	mock.ExpectQuery(`SELECT COUNT\(1\) FROM combined WHERE kind = \$3 AND first_token_ms IS NOT NULL`).
		WithArgs(start, end, "success").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))

	rows := sqlmock.NewRows([]string{
		"kind", "created_at", "request_id", "platform", "model", "duration_ms", "first_token_ms",
		"status_code", "error_id", "phase", "severity", "message", "user_id", "api_key_id",
		"account_id", "group_id", "stream",
	}).AddRow(
		"success", start, "req-ttft", "openai", "gpt-test", 2500, 800,
		nil, nil, nil, nil, nil, int64(1), int64(2), int64(3), int64(4), true,
	)

	mock.ExpectQuery(`ORDER BY first_token_ms DESC NULLS LAST, created_at DESC\s+LIMIT \$4 OFFSET \$5`).
		WithArgs(start, end, "success", 10, 0).
		WillReturnRows(rows)

	items, total, err := repo.ListRequestDetails(context.Background(), filter)
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, items, 1)
	require.Equal(t, "req-ttft", items[0].RequestID)
	require.NotNil(t, items[0].FirstTokenMs)
	require.Equal(t, 800, *items[0].FirstTokenMs)
	require.NotNil(t, items[0].DurationMs)
	require.Equal(t, 2500, *items[0].DurationMs)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestOpsRepositoryListRequestDetailsSLAOnlyExcludesIgnoredErrorsAndKeepsStreamFailures(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &opsRepository{db: db}

	start := time.Date(2026, 8, 9, 10, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	filter := &service.OpsRequestDetailFilter{
		StartTime: &start,
		EndTime:   &end,
		Kind:      "error",
		SLAOnly:   true,
		Page:      1,
		PageSize:  10,
	}

	slaScope := `(?s)COALESCE\(o\.counts_toward_sla, NOT COALESCE\(o\.is_business_limited, false\)\) = TRUE AND o\.is_count_tokens = FALSE`
	mock.ExpectQuery(slaScope+`.*SELECT COUNT\(1\) FROM combined WHERE kind = \$3`).
		WithArgs(start, end, "error").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(0)))
	mock.ExpectQuery(slaScope+`.*WHERE kind = \$3.*ORDER BY created_at DESC.*LIMIT \$4 OFFSET \$5`).
		WithArgs(start, end, "error", 10, 0).
		WillReturnRows(sqlmock.NewRows([]string{
			"kind", "created_at", "request_id", "platform", "model", "duration_ms", "first_token_ms",
			"status_code", "error_id", "phase", "severity", "message", "user_id", "api_key_id",
			"account_id", "group_id", "stream",
		}))

	items, total, err := repo.ListRequestDetails(context.Background(), filter)
	require.NoError(t, err)
	require.Zero(t, total)
	require.Empty(t, items)
	require.NoError(t, mock.ExpectationsWereMet())
}
