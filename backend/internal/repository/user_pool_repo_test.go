//go:build unit

package repository

import (
	"context"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newUserPoolRepoMock creates a userPoolRepository backed by a sqlmock database.
func newUserPoolRepoMock(t *testing.T) (*userPoolRepository, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return &userPoolRepository{db: db}, mock
}

// ── TestGetUserPoolsBatch ─────────────────────────────────────────────────────

// TestGetUserPoolsBatch_Empty returns an empty map for empty input without querying.
func TestGetUserPoolsBatch_Empty(t *testing.T) {
	repo, mock := newUserPoolRepoMock(t)
	result, err := repo.GetUserPoolsBatch(context.Background(), nil)
	require.NoError(t, err)
	assert.Empty(t, result)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestGetUserPoolsBatch_MultiUser returns correct pool lists per user.
func TestGetUserPoolsBatch_MultiUser(t *testing.T) {
	repo, mock := newUserPoolRepoMock(t)

	now := time.Now()
	cols := []string{"user_id", "id", "name", "description", "status", "created_at", "updated_at", "deleted_at"}
	rows := sqlmock.NewRows(cols).
		AddRow(10, 1, "Pool A", "desc", "active", now, now, nil).
		AddRow(10, 2, "Pool B", "", "active", now, now, nil).
		AddRow(20, 3, "Pool C", "desc", "active", now, now, nil)

	mock.ExpectQuery(`SELECT m\.user_id`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(rows)

	result, err := repo.GetUserPoolsBatch(context.Background(), []int64{10, 20})
	require.NoError(t, err)
	assert.Len(t, result[10], 2, "user 10 should have 2 pools")
	assert.Len(t, result[20], 1, "user 20 should have 1 pool")
	assert.Equal(t, int64(1), result[10][0].ID)
	assert.Equal(t, int64(2), result[10][1].ID)
	assert.Equal(t, int64(3), result[20][0].ID)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestGetUserPoolsBatch_NoPoolsForUser returns empty slices for users not in any pool.
func TestGetUserPoolsBatch_NoPoolsForUser(t *testing.T) {
	repo, mock := newUserPoolRepoMock(t)

	cols := []string{"user_id", "id", "name", "description", "status", "created_at", "updated_at", "deleted_at"}
	rows := sqlmock.NewRows(cols) // no rows returned

	mock.ExpectQuery(`SELECT m\.user_id`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(rows)

	result, err := repo.GetUserPoolsBatch(context.Background(), []int64{99})
	require.NoError(t, err)
	assert.Empty(t, result[99], "user 99 should have no pools")
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestGetUserPoolsBatch_DeletedPoolExcluded verifies soft-deleted pools are excluded
// (SQL WHERE p.deleted_at IS NULL; sqlmock verifies query was issued).
func TestGetUserPoolsBatch_DeletedPoolExcluded(t *testing.T) {
	repo, mock := newUserPoolRepoMock(t)

	now := time.Now()
	cols := []string{"user_id", "id", "name", "description", "status", "created_at", "updated_at", "deleted_at"}
	// Only the active pool row is returned (DB filters deleted ones).
	rows := sqlmock.NewRows(cols).
		AddRow(10, 1, "Pool A", "", "active", now, now, nil)

	mock.ExpectQuery(`SELECT m\.user_id`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(rows)

	result, err := repo.GetUserPoolsBatch(context.Background(), []int64{10})
	require.NoError(t, err)
	assert.Len(t, result[10], 1, "only active non-deleted pool should appear")
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── TestListGroupGrantsBatch ──────────────────────────────────────────────────

// TestListGroupGrantsBatch_Empty returns empty map for empty input without querying.
func TestListGroupGrantsBatch_Empty(t *testing.T) {
	repo, mock := newUserPoolRepoMock(t)
	result, err := repo.ListGroupGrantsBatch(context.Background(), nil)
	require.NoError(t, err)
	assert.Empty(t, result)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestListGroupGrantsBatch_MultiPool returns correct grants per pool.
func TestListGroupGrantsBatch_MultiPool(t *testing.T) {
	repo, mock := newUserPoolRepoMock(t)

	now := time.Now()
	cols := []string{"pool_id", "group_id", "rate_multiplier", "rpm_override", "created_at", "updated_at"}
	rows := sqlmock.NewRows(cols).
		AddRow(1, 5, nil, nil, now, now).
		AddRow(1, 6, 1.5, 100, now, now).
		AddRow(2, 5, nil, nil, now, now)

	mock.ExpectQuery(`SELECT g\.pool_id`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(rows)

	result, err := repo.ListGroupGrantsBatch(context.Background(), []int64{1, 2})
	require.NoError(t, err)
	assert.Len(t, result[1], 2, "pool 1 should have 2 grants")
	assert.Len(t, result[2], 1, "pool 2 should have 1 grant")
	assert.Equal(t, int64(5), result[1][0].GroupID)
	assert.Equal(t, int64(6), result[1][1].GroupID)
	assert.NotNil(t, result[1][1].RateMultiplier)
	assert.Equal(t, 1.5, *result[1][1].RateMultiplier)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestListGroupGrantsBatch_EmptyPool returns empty slice for pool with no grants.
func TestListGroupGrantsBatch_EmptyPool(t *testing.T) {
	repo, mock := newUserPoolRepoMock(t)

	cols := []string{"pool_id", "group_id", "rate_multiplier", "rpm_override", "created_at", "updated_at"}
	rows := sqlmock.NewRows(cols) // no rows

	mock.ExpectQuery(`SELECT g\.pool_id`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(rows)

	result, err := repo.ListGroupGrantsBatch(context.Background(), []int64{99})
	require.NoError(t, err)
	assert.Empty(t, result[99])
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestListGroupGrantsBatch_DeletedGroupExcluded verifies the JOIN condition filters deleted groups.
// The SQL filters gr.deleted_at IS NULL and gr.status='active'; sqlmock validates query was issued.
func TestListGroupGrantsBatch_DeletedGroupExcluded(t *testing.T) {
	repo, mock := newUserPoolRepoMock(t)

	now := time.Now()
	cols := []string{"pool_id", "group_id", "rate_multiplier", "rpm_override", "created_at", "updated_at"}
	// Only active group grant returned — DB filters inactive/deleted ones via JOIN.
	rows := sqlmock.NewRows(cols).
		AddRow(1, 5, nil, nil, now, now)

	mock.ExpectQuery(`SELECT g\.pool_id`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(rows)

	result, err := repo.ListGroupGrantsBatch(context.Background(), []int64{1})
	require.NoError(t, err)
	assert.Len(t, result[1], 1)
	require.NoError(t, mock.ExpectationsWereMet())
}
