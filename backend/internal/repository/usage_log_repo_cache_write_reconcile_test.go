package repository

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func testCacheWriteCorrection() *service.OpenAICacheWriteUsageCorrection {
	accountStats := 0.42
	return &service.OpenAICacheWriteUsageCorrection{
		RequestID:                       "req-cache-write",
		APIKeyID:                        77,
		OriginalInputTokens:             2268,
		OriginalCacheCreationTokens:     0,
		CorrectedInputTokens:            68,
		CorrectedCacheCreationTokens:    2200,
		CorrectedInputCost:              0.00068,
		CorrectedCacheCreationCost:      0.0275,
		CorrectedTotalCost:              0.18058,
		CorrectedActualCost:             0.036116,
		CorrectedAccountStatsCost:       &accountStats,
	}
}

func TestUsageLogRepositoryReconcileInferredCacheWrite_UpdatesOriginalRow(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := newUsageLogRepositoryWithSQL(nil, db)
	correction := testCacheWriteCorrection()

	mock.ExpectExec(regexp.QuoteMeta("UPDATE usage_logs")).
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
		WillReturnResult(sqlmock.NewResult(0, 1))

	ready, err := repo.ReconcileInferredCacheWrite(context.Background(), correction)
	require.NoError(t, err)
	require.True(t, ready)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUsageLogRepositoryReconcileInferredCacheWrite_RetriesWhenRowNotInsertedYet(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := newUsageLogRepositoryWithSQL(nil, db)
	correction := testCacheWriteCorrection()

	mock.ExpectExec(regexp.QuoteMeta("UPDATE usage_logs")).
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
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT input_tokens, cache_creation_tokens")).
		WithArgs(correction.RequestID, correction.APIKeyID).
		WillReturnRows(sqlmock.NewRows([]string{"input_tokens", "cache_creation_tokens"}))

	ready, err := repo.ReconcileInferredCacheWrite(context.Background(), correction)
	require.NoError(t, err)
	require.False(t, ready)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUsageLogRepositoryReconcileInferredCacheWrite_RejectsConflictingTokenSplit(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := newUsageLogRepositoryWithSQL(nil, db)
	correction := testCacheWriteCorrection()

	mock.ExpectExec(regexp.QuoteMeta("UPDATE usage_logs")).
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
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT input_tokens, cache_creation_tokens")).
		WithArgs(correction.RequestID, correction.APIKeyID).
		WillReturnRows(sqlmock.NewRows([]string{"input_tokens", "cache_creation_tokens"}).
			AddRow(999, 123))

	ready, err := repo.ReconcileInferredCacheWrite(context.Background(), correction)
	require.Error(t, err)
	require.False(t, ready)
	require.Contains(t, err.Error(), "reconciliation conflict")
	require.NoError(t, mock.ExpectationsWereMet())
}
