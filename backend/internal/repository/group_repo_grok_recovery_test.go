package repository

import (
	"context"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestGroupRepository_GetAccountCount_GrokFreeRecoveryPredicate(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	var capturedSQL string
	mock.ExpectQuery(`(?s)SELECT.*FROM account_groups`).
		WithArgs(int64(17), service.GrokFreeRecoveryPendingExtraKey).
		WillReturnRows(sqlmock.NewRows([]string{"total", "active", "rate_limited"}).AddRow(3, 2, 1))

	repo := newGroupRepositoryWithSQL(nil, captureQuerySQL{db: db, captured: &capturedSQL})
	total, active, err := repo.GetAccountCount(context.Background(), 17)
	require.NoError(t, err)
	require.Equal(t, int64(3), total)
	require.Equal(t, int64(2), active)

	normalized := normalizeSQLWhitespace(capturedSQL)
	require.Contains(t, normalized, "COALESCE(a.extra ->> $2, 'false') <> 'true'")
	require.Contains(t, normalized, "COALESCE(a.extra ->> $2, 'false') = 'true'")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGroupRepository_LoadAccountCounts_BindsGrokFreeRecoveryMarker(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	var capturedSQL string
	mock.ExpectQuery(`(?s)SELECT ag.group_id.*FROM account_groups`).
		WithArgs(sqlmock.AnyArg(), service.GrokFreeRecoveryPendingExtraKey).
		WillReturnRows(sqlmock.NewRows([]string{"group_id", "total", "active", "rate_limited"}).AddRow(17, 3, 2, 1))

	repo := newGroupRepositoryWithSQL(nil, captureQuerySQL{db: db, captured: &capturedSQL})
	counts, err := repo.loadAccountCounts(context.Background(), []int64{17})
	require.NoError(t, err)
	require.Equal(t, groupAccountCounts{Total: 3, Active: 2, RateLimited: 1}, counts[17])

	normalized := normalizeSQLWhitespace(capturedSQL)
	require.Contains(t, normalized, "COALESCE(a.extra ->> $2, 'false') <> 'true'")
	require.Contains(t, normalized, "COALESCE(a.extra ->> $2, 'false') = 'true'")
	require.NoError(t, mock.ExpectationsWereMet())
}
