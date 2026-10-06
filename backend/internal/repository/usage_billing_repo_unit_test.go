//go:build unit

package repository

import (
	"context"
	"database/sql"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

const (
	conditionalBalanceDeductSQL = `(?s)UPDATE users\s+SET balance = balance - \$1,\s+updated_at = NOW\(\)\s+WHERE id = \$2 AND deleted_at IS NULL AND balance >= \$1\s+RETURNING balance`
	overdraftBalanceDeductSQL   = `(?s)UPDATE users\s+SET balance = balance - \$1,\s+updated_at = NOW\(\)\s+WHERE id = \$2 AND deleted_at IS NULL\s+RETURNING balance`
	reserveBatchImageHoldSQL    = `(?s)UPDATE users\s+SET balance = balance - \$1,\s+frozen_balance = COALESCE\(frozen_balance, 0\) \+ \$1,\s+updated_at = NOW\(\)\s+WHERE id = \$2 AND deleted_at IS NULL AND balance >= \$1\s+RETURNING balance, frozen_balance`
	captureBatchImageHoldSQL    = `(?s)UPDATE users\s+SET balance = balance\s+\+ CASE WHEN \$1 > \$2 THEN \$1 - \$2 ELSE 0 END\s+- CASE WHEN \$2 > \$1 THEN \$2 - \$1 ELSE 0 END,\s+frozen_balance = COALESCE\(frozen_balance, 0\) - \$1,\s+updated_at = NOW\(\)\s+WHERE id = \$3 AND deleted_at IS NULL AND COALESCE\(frozen_balance, 0\) >= \$1\s+RETURNING balance, frozen_balance`
	releaseBatchImageHoldSQL    = `(?s)UPDATE users\s+SET balance = balance \+ \$1,\s+frozen_balance = COALESCE\(frozen_balance, 0\) - \$1,\s+updated_at = NOW\(\)\s+WHERE id = \$2 AND deleted_at IS NULL AND COALESCE\(frozen_balance, 0\) >= \$1\s+RETURNING balance, frozen_balance`
	userExistsForBillingSQL     = `(?s)SELECT 1\s+FROM users\s+WHERE id = \$1 AND deleted_at IS NULL`
	apiKeyQuotaIncrementSQL     = `(?s)UPDATE api_keys\s+SET quota_used = quota_used \+ \$1,.*WHERE id = \$2 AND deleted_at IS NULL\s+RETURNING`
	apiKeyRateLimitIncrementSQL = `(?s)UPDATE api_keys SET\s+usage_5h = .*WHERE id = \$2 AND deleted_at IS NULL`
)

func TestDeductUsageBillingBalance_UsesSufficientBalanceGuard(t *testing.T) {
	ctx := context.Background()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectBegin()
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	mock.ExpectQuery(conditionalBalanceDeductSQL).
		WithArgs(2.5, int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"balance"}).AddRow(7.5))
	mock.ExpectCommit()

	newBalance, sufficient, err := deductUsageBillingBalance(ctx, tx, 42, 2.5)
	require.NoError(t, err)
	require.True(t, sufficient)
	require.InDelta(t, 7.5, newBalance, 0.000001)
	require.NoError(t, tx.Commit())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeductUsageBillingBalance_RecordsOverdraftWhenGuardMisses(t *testing.T) {
	ctx := context.Background()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectBegin()
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	mock.ExpectQuery(conditionalBalanceDeductSQL).
		WithArgs(10.0, int64(42)).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(overdraftBalanceDeductSQL).
		WithArgs(10.0, int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"balance"}).AddRow(-5.0))
	mock.ExpectCommit()

	newBalance, sufficient, err := deductUsageBillingBalance(ctx, tx, 42, 10)
	require.NoError(t, err)
	require.False(t, sufficient)
	require.InDelta(t, -5.0, newBalance, 0.000001)
	require.NoError(t, tx.Commit())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestApplyUsageBillingEffects_FlagsBalanceOverdraft(t *testing.T) {
	ctx := context.Background()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectBegin()
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	mock.ExpectQuery(conditionalBalanceDeductSQL).
		WithArgs(10.0, int64(42)).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(overdraftBalanceDeductSQL).
		WithArgs(10.0, int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"balance"}).AddRow(-5.0))
	mock.ExpectCommit()

	result := &service.UsageBillingApplyResult{Applied: true}
	err = (&usageBillingRepository{}).applyUsageBillingEffects(ctx, tx, &service.UsageBillingCommand{
		UserID:      42,
		BalanceCost: 10,
	}, result)
	require.NoError(t, err)
	require.NotNil(t, result.NewBalance)
	require.InDelta(t, -5.0, *result.NewBalance, 0.000001)
	require.True(t, result.BalanceOverdrafted)
	require.NoError(t, tx.Commit())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestApplyUsageBillingEffects_DeletedAPIKeyStillBillsBalance(t *testing.T) {
	ctx := context.Background()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectBegin()
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	mock.ExpectQuery(conditionalBalanceDeductSQL).
		WithArgs(10.0, int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"balance"}).AddRow(90.0))
	mock.ExpectQuery(apiKeyQuotaIncrementSQL).
		WithArgs(10.0, int64(7), service.StatusAPIKeyActive, service.StatusAPIKeyQuotaExhausted).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectExec(apiKeyRateLimitIncrementSQL).
		WithArgs(10.0, int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	result := &service.UsageBillingApplyResult{Applied: true}
	err = (&usageBillingRepository{}).applyUsageBillingEffects(ctx, tx, &service.UsageBillingCommand{
		UserID:              42,
		APIKeyID:            7,
		BalanceCost:         10,
		APIKeyQuotaCost:     10,
		APIKeyRateLimitCost: 10,
	}, result)
	require.NoError(t, err)
	require.NotNil(t, result.NewBalance)
	require.InDelta(t, 90.0, *result.NewBalance, 0.000001)
	require.False(t, result.APIKeyQuotaExhausted)
	require.NoError(t, tx.Commit())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestApplyUsageBillingEffects_APIKeyCounterErrorStillFails(t *testing.T) {
	ctx := context.Background()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectBegin()
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	mock.ExpectExec(apiKeyRateLimitIncrementSQL).
		WithArgs(10.0, int64(7)).
		WillReturnError(sql.ErrConnDone)
	mock.ExpectRollback()

	err = (&usageBillingRepository{}).applyUsageBillingEffects(ctx, tx, &service.UsageBillingCommand{
		UserID:              42,
		APIKeyID:            7,
		APIKeyRateLimitCost: 10,
	}, &service.UsageBillingApplyResult{Applied: true})
	require.ErrorIs(t, err, sql.ErrConnDone)
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeductUsageBillingBalance_ReturnsUserNotFoundWhenNoUserUpdated(t *testing.T) {
	ctx := context.Background()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectBegin()
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	mock.ExpectQuery(conditionalBalanceDeductSQL).
		WithArgs(10.0, int64(42)).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(overdraftBalanceDeductSQL).
		WithArgs(10.0, int64(42)).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()

	_, _, err = deductUsageBillingBalance(ctx, tx, 42, 10)
	require.ErrorIs(t, err, service.ErrUserNotFound)
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestReserveUsageBillingBatchImageBalance_MovesAvailableToFrozen(t *testing.T) {
	ctx := context.Background()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectBegin()
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	mock.ExpectQuery(reserveBatchImageHoldSQL).
		WithArgs(2.5, int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"balance", "frozen_balance"}).AddRow(7.5, 2.5))
	mock.ExpectCommit()

	result, err := reserveUsageBillingBatchImageBalance(ctx, tx, &service.BatchImageBalanceHoldCommand{UserID: 42, HoldAmount: 2.5})
	require.NoError(t, err)
	require.NotNil(t, result.NewBalance)
	require.NotNil(t, result.FrozenBalance)
	require.InDelta(t, 7.5, *result.NewBalance, 0.000001)
	require.InDelta(t, 2.5, *result.FrozenBalance, 0.000001)
	require.NoError(t, tx.Commit())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestReserveUsageBillingBatchImageBalance_InsufficientBalance(t *testing.T) {
	ctx := context.Background()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectBegin()
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	mock.ExpectQuery(reserveBatchImageHoldSQL).
		WithArgs(10.0, int64(42)).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(userExistsForBillingSQL).
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"?column?"}).AddRow(1))
	mock.ExpectRollback()

	_, err = reserveUsageBillingBatchImageBalance(ctx, tx, &service.BatchImageBalanceHoldCommand{UserID: 42, HoldAmount: 10})
	require.ErrorIs(t, err, service.ErrBatchImageInsufficientBalance)
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCaptureUsageBillingBatchImageBalance_ReleasesRemainder(t *testing.T) {
	ctx := context.Background()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectBegin()
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	mock.ExpectQuery(captureBatchImageHoldSQL).
		WithArgs(1.0, 0.25, int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"balance", "frozen_balance"}).AddRow(9.75, 0.0))
	mock.ExpectCommit()

	result, err := captureUsageBillingBatchImageBalance(ctx, tx, &service.BatchImageBalanceHoldCommand{UserID: 42, HoldAmount: 1, ActualAmount: 0.25})
	require.NoError(t, err)
	require.InDelta(t, 9.75, *result.NewBalance, 0.000001)
	require.InDelta(t, 0.0, *result.FrozenBalance, 0.000001)
	require.NoError(t, tx.Commit())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCaptureUsageBillingBatchImageBalance_RejectsActualCostOverHold(t *testing.T) {
	ctx := context.Background()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectBegin()
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	mock.ExpectRollback()

	_, err = captureUsageBillingBatchImageBalance(ctx, tx, &service.BatchImageBalanceHoldCommand{UserID: 42, HoldAmount: 0.5, ActualAmount: 1})
	require.ErrorIs(t, err, service.ErrBatchImageSettlementCostExceedsHold)
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestReleaseUsageBillingBatchImageBalance_ReturnsFrozenToAvailable(t *testing.T) {
	ctx := context.Background()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectBegin()
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	mock.ExpectQuery(`SELECT 1\s+FROM usage_billing_dedup\s+WHERE request_id = \$1 AND api_key_id = \$2`).
		WithArgs(service.BatchImageHoldRequestID("imgbatch_release"), int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"?column?"}).AddRow(1))
	mock.ExpectQuery(releaseBatchImageHoldSQL).
		WithArgs(1.0, int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"balance", "frozen_balance"}).AddRow(10.0, 0.0))
	mock.ExpectCommit()

	result, err := releaseUsageBillingBatchImageBalance(ctx, tx, &service.BatchImageBalanceHoldCommand{UserID: 42, APIKeyID: 7, BatchID: "imgbatch_release", HoldAmount: 1})
	require.NoError(t, err)
	require.InDelta(t, 10.0, *result.NewBalance, 0.000001)
	require.InDelta(t, 0.0, *result.FrozenBalance, 0.000001)
	require.NoError(t, tx.Commit())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestReleaseUsageBillingBatchImageBalance_SkipsWhenHoldNeverReserved(t *testing.T) {
	ctx := context.Background()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectBegin()
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	// dedup 与归档表均无 hold claim：说明该 job 从未成功冻结，
	// 释放必须跳过，不得从他人冻结资金池中凭空生成余额。
	mock.ExpectQuery(`SELECT 1\s+FROM usage_billing_dedup\s+WHERE request_id = \$1 AND api_key_id = \$2`).
		WithArgs(service.BatchImageHoldRequestID("imgbatch_phantom"), int64(7)).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(`SELECT 1\s+FROM usage_billing_dedup_archive\s+WHERE request_id = \$1 AND api_key_id = \$2`).
		WithArgs(service.BatchImageHoldRequestID("imgbatch_phantom"), int64(7)).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectCommit()

	result, err := releaseUsageBillingBatchImageBalance(ctx, tx, &service.BatchImageBalanceHoldCommand{UserID: 42, APIKeyID: 7, BatchID: "imgbatch_phantom", HoldAmount: 1})
	require.NoError(t, err)
	require.Nil(t, result.NewBalance)
	require.Nil(t, result.FrozenBalance)
	require.NoError(t, tx.Commit())
	require.NoError(t, mock.ExpectationsWereMet())
}

func testInferredCacheWriteCorrection() *service.OpenAICacheWriteUsageCorrection {
	accountStats := 0.42
	return &service.OpenAICacheWriteUsageCorrection{
		RequestID:                    "req-cache-write",
		APIKeyID:                     77,
		OriginalInputTokens:          2268,
		OriginalCacheCreationTokens:  0,
		CorrectedInputTokens:         68,
		CorrectedCacheCreationTokens: 2200,
		CorrectedInputCost:           0.00068,
		CorrectedCacheCreationCost:   0.0275,
		CorrectedTotalCost:           0.18058,
		CorrectedActualCost:          0.036116,
		CorrectedAccountStatsCost:    &accountStats,
	}
}

func expectInferredCacheWriteUsageUpdate(
	mock sqlmock.Sqlmock,
	correction *service.OpenAICacheWriteUsageCorrection,
	result sql.Result,
) {
	mock.ExpectExec(`(?s)UPDATE usage_logs\s+SET\s+input_tokens = \$3,.*cache_creation_tokens = \$4`).
		WithArgs(
			correction.RequestID,
			correction.APIKeyID,
			correction.CorrectedInputTokens,
			correction.CorrectedCacheCreationTokens,
			correction.CorrectedInputCost,
			correction.CorrectedCacheCreationCost,
			correction.CorrectedTotalCost,
			correction.CorrectedActualCost,
			true,
			*correction.CorrectedAccountStatsCost,
			correction.OriginalInputTokens,
			correction.OriginalCacheCreationTokens,
		).
		WillReturnResult(result)
}

func TestApplyUsageBillingEffects_CacheWriteCorrectionCommitsWithBalanceDelta(t *testing.T) {
	ctx := context.Background()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	correction := testInferredCacheWriteCorrection()
	mock.ExpectBegin()
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)

	mock.ExpectQuery(conditionalBalanceDeductSQL).
		WithArgs(0.0011, int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"balance"}).AddRow(9.9989))
	expectInferredCacheWriteUsageUpdate(mock, correction, sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	result := &service.UsageBillingApplyResult{Applied: true}
	err = (&usageBillingRepository{}).applyUsageBillingEffects(ctx, tx, &service.UsageBillingCommand{
		UserID:               42,
		BalanceCost:          0.0011,
		CacheWriteCorrection: correction,
	}, result)
	require.NoError(t, err)
	require.NotNil(t, result.NewBalance)
	require.InDelta(t, 9.9989, *result.NewBalance, 1e-12)
	require.NoError(t, tx.Commit())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestApplyUsageBillingEffects_CacheWriteCorrectionFailureRollsBackBalanceDelta(t *testing.T) {
	ctx := context.Background()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	correction := testInferredCacheWriteCorrection()
	mock.ExpectBegin()
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)

	mock.ExpectQuery(conditionalBalanceDeductSQL).
		WithArgs(0.0011, int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"balance"}).AddRow(9.9989))
	mock.ExpectExec(`(?s)UPDATE usage_logs\s+SET\s+input_tokens = \$3,.*cache_creation_tokens = \$4`).
		WithArgs(
			correction.RequestID,
			correction.APIKeyID,
			correction.CorrectedInputTokens,
			correction.CorrectedCacheCreationTokens,
			correction.CorrectedInputCost,
			correction.CorrectedCacheCreationCost,
			correction.CorrectedTotalCost,
			correction.CorrectedActualCost,
			true,
			*correction.CorrectedAccountStatsCost,
			correction.OriginalInputTokens,
			correction.OriginalCacheCreationTokens,
		).
		WillReturnError(sql.ErrConnDone)
	mock.ExpectRollback()

	err = (&usageBillingRepository{}).applyUsageBillingEffects(ctx, tx, &service.UsageBillingCommand{
		UserID:               42,
		BalanceCost:          0.0011,
		CacheWriteCorrection: correction,
	}, &service.UsageBillingApplyResult{Applied: true})
	require.ErrorIs(t, err, sql.ErrConnDone)
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestReconcileInferredCacheWriteUsageLogTx_RejectsConflictingSplit(t *testing.T) {
	ctx := context.Background()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	correction := testInferredCacheWriteCorrection()
	mock.ExpectBegin()
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)

	expectInferredCacheWriteUsageUpdate(mock, correction, sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`(?s)SELECT input_tokens, cache_creation_tokens\s+FROM usage_logs`).
		WithArgs(correction.RequestID, correction.APIKeyID).
		WillReturnRows(sqlmock.NewRows([]string{"input_tokens", "cache_creation_tokens"}).AddRow(999, 123))
	mock.ExpectRollback()

	err = reconcileInferredCacheWriteUsageLogTx(ctx, tx, correction)
	require.Error(t, err)
	require.Contains(t, err.Error(), "reconciliation conflict")
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}
