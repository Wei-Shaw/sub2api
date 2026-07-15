package repository

import (
	"context"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestFillDashboardEntityStats_GrokFreeRecoveryPredicate(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	today := time.Date(2026, 7, 14, 0, 0, 0, 0, time.UTC)
	now := today.Add(12 * time.Hour)
	mock.ExpectQuery(`(?s)SELECT.*FROM users`).
		WithArgs(today).
		WillReturnRows(sqlmock.NewRows([]string{"total", "today"}).AddRow(5, 1))
	mock.ExpectQuery(`(?s)SELECT.*FROM api_keys`).
		WithArgs(service.StatusActive).
		WillReturnRows(sqlmock.NewRows([]string{"total", "active"}).AddRow(4, 3))

	var capturedSQL string
	mock.ExpectQuery(`(?s)SELECT.*FROM accounts`).
		WithArgs(service.StatusActive, service.StatusError, service.GrokFreeRecoveryPendingExtraKey, now, now).
		WillReturnRows(sqlmock.NewRows([]string{"total", "normal", "error", "rate_limited", "overload"}).AddRow(6, 3, 1, 1, 1))

	repo := newUsageLogRepositoryWithSQL(nil, captureQuerySQL{db: db, captured: &capturedSQL})
	stats := &DashboardStats{}
	require.NoError(t, repo.fillDashboardEntityStats(context.Background(), stats, today, now))
	require.Equal(t, int64(3), stats.NormalAccounts)
	require.Equal(t, int64(1), stats.RateLimitAccounts)

	normalized := normalizeSQLWhitespace(capturedSQL)
	require.Contains(t, normalized, "COALESCE(extra ->> $3, 'false') <> 'true'")
	require.Contains(t, normalized, "OR COALESCE(extra ->> $3, 'false') = 'true'")
	require.NoError(t, mock.ExpectationsWereMet())
}
