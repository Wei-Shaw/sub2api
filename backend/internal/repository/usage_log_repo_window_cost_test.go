//go:build unit

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestGetAccountWindowCostsBatchReadsInitializedState(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := newUsageLogRepositoryWithSQL(nil, db)
	start := time.Date(2026, 8, 31, 8, 0, 0, 0, time.UTC)

	mock.ExpectQuery(`(?s)SELECT account_id, standard_cost.*FROM account_window_cost_state`).
		WithArgs("{3,9}", start).
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "standard_cost"}).
			AddRow(int64(3), 1.25).
			AddRow(int64(9), 8.75))

	costs, err := repo.GetAccountWindowCostsBatch(context.Background(), []int64{9, 3, 9, 0}, start)
	require.NoError(t, err)
	require.Equal(t, map[int64]float64{3: 1.25, 9: 8.75}, costs)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetAccountWindowCostsBatchInitializesMissingWindowOnce(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := newUsageLogRepositoryWithSQL(nil, db)
	start := time.Date(2026, 8, 31, 8, 0, 0, 0, time.UTC)
	oldStart := start.Add(-5 * time.Hour)

	mock.ExpectQuery(`(?s)SELECT account_id, standard_cost.*FROM account_window_cost_state`).
		WithArgs("{7}", start).
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "standard_cost"}))
	mock.ExpectBegin()
	mock.ExpectExec(`(?s)INSERT INTO account_window_cost_state`).
		WithArgs("{7}", start).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`(?s)SELECT account_id, window_start, standard_cost, initialized.*FOR UPDATE`).
		WithArgs("{7}").
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "window_start", "standard_cost", "initialized"}).
			AddRow(int64(7), oldStart, 99.0, false))
	mock.ExpectExec(`(?s)UPDATE account_window_cost_state.*standard_cost = 0`).
		WithArgs("{7}", start).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`(?s)WITH targets AS.*SUM\(usage_logs.total_cost\).*UPDATE account_window_cost_state`).
		WithArgs("{7}", start).
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "standard_cost"}).AddRow(int64(7), 12.5))
	mock.ExpectCommit()

	costs, err := repo.GetAccountWindowCostsBatch(context.Background(), []int64{7}, start)
	require.NoError(t, err)
	require.Equal(t, map[int64]float64{7: 12.5}, costs)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestInitializeAccountWindowCostReusesConcurrentInitializerResult(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := newUsageLogRepositoryWithSQL(nil, db)
	start := time.Date(2026, 8, 31, 8, 0, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectExec(`(?s)INSERT INTO account_window_cost_state`).
		WithArgs("{11}", start).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`(?s)SELECT account_id, window_start, standard_cost, initialized.*FOR UPDATE`).
		WithArgs("{11}").
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "window_start", "standard_cost", "initialized"}).
			AddRow(int64(11), start, 4.75, true))
	mock.ExpectCommit()

	costs, err := repo.initializeAccountWindowCosts(context.Background(), []int64{11}, start)
	require.NoError(t, err)
	require.Equal(t, map[int64]float64{11: 4.75}, costs)
	require.NoError(t, mock.ExpectationsWereMet())
}
