//go:build unit

package repository

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestCompleteManualVideoBillingReconcilesHoldInSingleTransaction(t *testing.T) {
	tests := []struct {
		name      string
		heldCost  float64
		finalCost float64
		delta     float64
	}{
		{name: "refunds excess hold", heldCost: 3, finalCost: 2, delta: -1},
		{name: "collects shortfall", heldCost: 1, finalCost: 2.5, delta: 1.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock := newSQLMock(t)
			repo := &asyncVideoTaskRepository{sql: db, db: db}

			mock.ExpectBegin()
			mock.ExpectQuery(`(?s)SELECT held_cost, billing_type, user_id, payer_user_id, organization_id, balance_source.*FOR UPDATE`).
				WithArgs(int64(42)).
				WillReturnRows(sqlmock.NewRows([]string{"held_cost", "billing_type", "user_id", "payer_user_id", "organization_id", "balance_source"}).
					AddRow(tt.heldCost, service.BillingTypeBalance, int64(7), int64(9), nil, service.BalanceSourceSelf))
			mock.ExpectQuery(`(?s)SELECT id, COALESCE\(billing_status, ''\).*FOR UPDATE`).
				WithArgs(int64(42)).
				WillReturnRows(sqlmock.NewRows([]string{"id", "billing_status"}).AddRow(int64(88), service.BillingStatusFailed))
			mock.ExpectExec(`UPDATE users SET balance = balance - \$1`).
				WithArgs(tt.delta, int64(9)).
				WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectExec(`UPDATE async_video_tasks SET final_cost = \$2`).
				WithArgs(int64(42), tt.finalCost).
				WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectExec(`UPDATE usage_logs SET total_cost = \$2`).
				WithArgs(int64(88), tt.finalCost, service.BillingStatusCharged).
				WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectCommit()

			applied, err := repo.CompleteManualBilling(context.Background(), 42, tt.finalCost)

			require.NoError(t, err)
			require.True(t, applied)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestMarkSucceededReconcilesBalanceBeforeCommittingTask(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &asyncVideoTaskRepository{sql: db, db: db}

	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)SELECT held_cost, billing_type, user_id, payer_user_id, organization_id, balance_source, status.*FOR UPDATE`).
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"held_cost", "billing_type", "user_id", "payer_user_id", "organization_id", "balance_source", "status"}).
			AddRow(1.0, service.BillingTypeBalance, int64(7), int64(9), nil, service.BalanceSourceSelf, service.AsyncVideoStatusRunning))
	mock.ExpectExec(`(?s)UPDATE users SET balance = balance - \$1.*balance >= \$1`).
		WithArgs(1.5, int64(9)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE async_video_tasks`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	applied, err := repo.MarkSucceeded(context.Background(), 42, []string{"https://example.test/result.mp4"}, nil, map[string]any{"duration": 25}, 2.5, 25, 0)

	require.NoError(t, err)
	require.True(t, applied)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMarkSucceededDoesNotReconcileTerminalTaskAgain(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &asyncVideoTaskRepository{sql: db, db: db}

	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)SELECT held_cost, billing_type, user_id, payer_user_id, organization_id, balance_source, status.*FOR UPDATE`).
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"held_cost", "billing_type", "user_id", "payer_user_id", "organization_id", "balance_source", "status"}).
			AddRow(1.0, service.BillingTypeBalance, int64(7), int64(9), nil, service.BalanceSourceSelf, service.AsyncVideoStatusSucceeded))
	mock.ExpectRollback()

	applied, err := repo.MarkSucceeded(context.Background(), 42, nil, nil, map[string]any{"duration": 25}, 2.5, 25, 0)

	require.NoError(t, err)
	require.False(t, applied)
	require.NoError(t, mock.ExpectationsWereMet())
}
