package repository

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAccountRepository_ResetQuotaUsed_ClearsExtraAndDeletesUsageLogs(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	accountID := int64(123)

	mock.ExpectExec(regexp.QuoteMeta(`UPDATE accounts SET extra = (
			COALESCE(extra, '{}'::jsonb)
			|| '{"quota_used": 0, "quota_daily_used": 0, "quota_weekly_used": 0}'::jsonb
		) - 'quota_daily_start' - 'quota_weekly_start' - 'quota_daily_reset_at' - 'quota_weekly_reset_at', updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL`)).
		WithArgs(accountID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM usage_logs WHERE account_id = $1`)).
		WithArgs(accountID).
		WillReturnResult(sqlmock.NewResult(0, 2))

	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO scheduler_outbox (event_type, account_id, group_id, payload, dedup_key)")).
		WithArgs(service.SchedulerOutboxEventAccountChanged, accountID, nil, nil, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	repo := newAccountRepositoryWithSQL(nil, db, nil)
	err = repo.ResetQuotaUsed(context.Background(), accountID)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAccountRepository_ResetQuotaUsed_UsageLogDeleteFailureDoesNotAbort(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	accountID := int64(456)

	mock.ExpectExec(regexp.QuoteMeta(`UPDATE accounts SET extra = (`)).
		WithArgs(accountID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM usage_logs WHERE account_id = $1`)).
		WithArgs(accountID).
		WillReturnError(errors.New("foreign key constraint violation"))

	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO scheduler_outbox (event_type, account_id, group_id, payload, dedup_key)")).
		WithArgs(service.SchedulerOutboxEventAccountChanged, accountID, nil, nil, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	repo := newAccountRepositoryWithSQL(nil, db, nil)
	err = repo.ResetQuotaUsed(context.Background(), accountID)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAccountRepository_ResetQuotaUsed_ExtraUpdateFailureReturnsError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	accountID := int64(789)

	mock.ExpectExec(regexp.QuoteMeta(`UPDATE accounts SET extra = (`)).
		WithArgs(accountID).
		WillReturnError(errors.New("connection refused"))

	repo := newAccountRepositoryWithSQL(nil, db, nil)
	err = repo.ResetQuotaUsed(context.Background(), accountID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "connection refused")
}
