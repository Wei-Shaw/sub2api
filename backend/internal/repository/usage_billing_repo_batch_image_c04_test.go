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
	incrementAPIKeyQuotaSQL     = `(?s)UPDATE api_keys\s+SET quota_used = quota_used \+ \$1,`
	incrementAPIKeyRateLimitSQL = `(?s)UPDATE api_keys SET\s+usage_5h = CASE`
)

func expectBatchImageCaptureOutstanding(mock sqlmock.Sqlmock, batchID string, apiKeyID int64) {
	mock.ExpectQuery(dedupClaimExistsSQL).
		WithArgs(service.BatchImageHoldRequestID(batchID), apiKeyID).
		WillReturnRows(sqlmock.NewRows([]string{"?column?"}).AddRow(1))
	mock.ExpectQuery(dedupClaimExistsSQL).
		WithArgs(service.BatchImageReleaseRequestID(batchID), apiKeyID).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(dedupArchiveClaimExistsSQL).
		WithArgs(service.BatchImageReleaseRequestID(batchID), apiKeyID).
		WillReturnError(sql.ErrNoRows)
}

// TestCaptureUsageBillingBatchImageBalance_IncrementsAPIKeyLimitsInSameTx 钉住 C-04：
// 批量生图核销必须在扣款的同一事务里累加 api_keys.quota_used 与 5h/1d/7d 窗口，
// 否则 Key 额度与限速窗口对批量生图完全失效。
func TestCaptureUsageBillingBatchImageBalance_IncrementsAPIKeyLimitsInSameTx(t *testing.T) {
	ctx := context.Background()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectBegin()
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	expectBatchImageCaptureOutstanding(mock, "imgbatch_c04", 7)
	mock.ExpectQuery(captureBatchImageHoldSQL).
		WithArgs(1.0, 0.25, int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"balance", "frozen_balance"}).AddRow(9.75, 0.0))
	mock.ExpectQuery(incrementAPIKeyQuotaSQL).
		WithArgs(0.25, int64(7), service.StatusAPIKeyActive, service.StatusAPIKeyQuotaExhausted).
		WillReturnRows(sqlmock.NewRows([]string{"exhausted"}).AddRow(true))
	mock.ExpectExec(incrementAPIKeyRateLimitSQL).
		WithArgs(0.25, int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	result, err := captureUsageBillingBatchImageBalance(ctx, tx, &service.BatchImageBalanceHoldCommand{
		UserID: 42, APIKeyID: 7, BatchID: "imgbatch_c04", HoldAmount: 1, ActualAmount: 0.25,
		APIKeyQuotaCost: 0.25, APIKeyRateLimitCost: 0.25,
	})
	require.NoError(t, err)
	require.True(t, result.Captured)
	require.True(t, result.APIKeyQuotaExhausted)
	require.InDelta(t, 9.75, *result.NewBalance, 0.000001)
	require.NoError(t, tx.Commit())
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestCaptureUsageBillingBatchImageBalance_SkipsAPIKeyLimitsWhenHoldNotOutstanding
// 冻结额已不在押（被 release 归还）时核销是空操作，也不得累加 Key 额度。
func TestCaptureUsageBillingBatchImageBalance_SkipsAPIKeyLimitsWhenHoldNotOutstanding(t *testing.T) {
	ctx := context.Background()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectBegin()
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	mock.ExpectQuery(dedupClaimExistsSQL).
		WithArgs(service.BatchImageHoldRequestID("imgbatch_c04_released"), int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"?column?"}).AddRow(1))
	mock.ExpectQuery(dedupClaimExistsSQL).
		WithArgs(service.BatchImageReleaseRequestID("imgbatch_c04_released"), int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"?column?"}).AddRow(1))
	mock.ExpectCommit()

	result, err := captureUsageBillingBatchImageBalance(ctx, tx, &service.BatchImageBalanceHoldCommand{
		UserID: 42, APIKeyID: 7, BatchID: "imgbatch_c04_released", HoldAmount: 1, ActualAmount: 0.25,
		APIKeyQuotaCost: 0.25, APIKeyRateLimitCost: 0.25,
	})
	require.NoError(t, err)
	require.False(t, result.Captured)
	require.NoError(t, tx.Commit())
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestBatchImageHoldFingerprintIgnoresAPIKeyLimitCosts 指纹不含 Key 限额成本：
// Key 配置在 capture 重试之间变化不能让同一 request id 被判为指纹冲突。
func TestBatchImageHoldFingerprintIgnoresAPIKeyLimitCosts(t *testing.T) {
	base := service.BatchImageBalanceHoldCommand{RequestID: "batch_image_capture:x", UserID: 1, APIKeyID: 2, BatchID: "x", HoldAmount: 1, ActualAmount: 0.5}
	withCosts := base
	withCosts.APIKeyQuotaCost = 0.5
	withCosts.APIKeyRateLimitCost = 0.5
	base.Normalize()
	withCosts.Normalize()
	require.Equal(t, base.RequestFingerprint, withCosts.RequestFingerprint)
}
