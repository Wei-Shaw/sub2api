package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestQueryUsageCountsExcludesCyberFromSuccessButKeepsTokens(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	repo := &opsRepository{db: db}
	start := time.Date(2026, 9, 2, 1, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)

	mock.ExpectQuery(`(?s)COUNT\(\*\) FILTER \(WHERE COALESCE\(ul\.request_type, 0\) <> 4\).*SUM\(input_tokens \+ output_tokens \+ cache_creation_tokens \+ cache_read_tokens\).*FROM usage_logs ul`).
		WithArgs(start, end).
		WillReturnRows(sqlmock.NewRows([]string{"success_count", "token_consumed"}).AddRow(int64(2), int64(345)))

	successCount, tokenConsumed, err := repo.queryUsageCounts(context.Background(), nil, start, end)
	require.NoError(t, err)
	require.Equal(t, int64(2), successCount)
	require.Equal(t, int64(345), tokenConsumed)
	require.NoError(t, mock.ExpectationsWereMet())
}
