package repository

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/web3deposit"
	"github.com/stretchr/testify/require"
)

func TestWeb3AccountingCreditRollsBackWhenBalanceUpdateFails(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	now := time.Date(2026, time.August, 8, 13, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT d.id, d.user_id").WithArgs(int64(7)).WillReturnRows(sqlmock.NewRows([]string{
		"id", "user_id", "token_amount", "credited_amount", "status", "retry_count", "user_status", "deleted_at",
	}).AddRow(int64(7), int64(42), "10.000000", "", "crediting", int32(3), "active", nil))
	mock.ExpectExec("INSERT INTO web3_user_balances").WithArgs(int64(42), web3deposit.AssetKeyUSDT).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT id, available_amount").WithArgs(int64(42), web3deposit.AssetKeyUSDT).WillReturnRows(sqlmock.NewRows([]string{"id", "available_amount", "total_deposited"}).AddRow(int64(9), "0.00000000", "0.00000000"))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE web3_user_balances")).WithArgs(int64(9), "10.00000000", "10.00000000", now).WillReturnError(errors.New("write failed"))
	mock.ExpectRollback()

	repo := NewWeb3AccountingRepository(db)
	_, err = repo.CreditDeposit(context.Background(), web3deposit.CreditDepositRequest{DepositID: 7, ClaimVersion: 3, Now: now})

	require.ErrorContains(t, err, "increase web3 user balance")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestWeb3AccountingTransferRejectsAnotherUsersIdempotencyKey(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id,user_id,web3_balance_id").WithArgs("shared-key").WillReturnRows(sqlmock.NewRows([]string{
		"id", "user_id", "web3_balance_id", "amount", "web3_balance_before", "web3_balance_after",
		"user_balance_before", "user_balance_after", "idempotency_key", "metadata", "created_at",
	}).AddRow(int64(7), int64(99), int64(10), "1.00000000", "2.00000000", "1.00000000",
		"3.00000000", "4.00000000", "shared-key", []byte(`{}`), time.Now()))
	mock.ExpectRollback()

	repo := NewWeb3AccountingRepository(db)
	_, err = repo.TransferToMainBalance(context.Background(), web3deposit.TransferToMainBalanceRequest{
		UserID: 42, AssetKey: web3deposit.AssetKeyUSDT, Amount: "1", IdempotencyKey: "shared-key",
	})

	require.ErrorIs(t, err, web3deposit.ErrTransferAlreadyExists)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestWeb3CreditJobRetryRejectsStaleClaim(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectExec("UPDATE web3_deposits").WillReturnResult(sqlmock.NewResult(0, 0))

	err = NewWeb3CreditJobRepository(db).RetryCreditJob(context.Background(), web3deposit.CreditJob{DepositID: 7, ClaimVersion: 2}, time.Now(), errors.New("failed"))

	require.ErrorIs(t, err, web3deposit.ErrCreditClaimLost)
	require.NoError(t, mock.ExpectationsWereMet())
}
