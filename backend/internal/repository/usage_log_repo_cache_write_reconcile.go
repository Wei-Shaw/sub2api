package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// ReconcileInferredCacheWrite rewrites the original usage row after a later
// cache hit proves that ordinary input from that request was actually admitted
// into the prompt cache.
//
// The update is absolute and idempotent. A missing row is reported as
// ready=false because best-effort usage-log insertion may still be queued even
// though the money transaction has already committed.
func (r *usageLogRepository) ReconcileInferredCacheWrite(
	ctx context.Context,
	correction *service.OpenAICacheWriteUsageCorrection,
) (bool, error) {
	if correction == nil {
		return false, nil
	}
	if r == nil || r.sql == nil {
		return false, errors.New("usage log repository sql executor is nil")
	}
	if correction.RequestID == "" || correction.APIKeyID <= 0 {
		return false, errors.New("cache write correction requires request_id and api_key_id")
	}
	if correction.CorrectedInputTokens < 0 || correction.CorrectedCacheCreationTokens < 0 {
		return false, errors.New("cache write correction contains negative token counts")
	}

	hasAccountStatsCost := correction.CorrectedAccountStatsCost != nil
	var accountStatsCost any
	if hasAccountStatsCost {
		accountStatsCost = *correction.CorrectedAccountStatsCost
	}

	result, err := r.sql.ExecContext(ctx, `
		UPDATE usage_logs
		SET
			input_tokens = $3,
			cache_creation_tokens = $4,
			input_cost = $5,
			cache_creation_cost = $6,
			total_cost = $7,
			actual_cost = $8,
			account_stats_cost = CASE WHEN $9 THEN $10 ELSE account_stats_cost END
		WHERE request_id = $1
			AND api_key_id = $2
			AND (
				(input_tokens = $11 AND cache_creation_tokens = $12)
				OR
				(input_tokens = $3 AND cache_creation_tokens = $4)
			)
	`,
		correction.RequestID,
		correction.APIKeyID,
		correction.CorrectedInputTokens,
		correction.CorrectedCacheCreationTokens,
		correction.CorrectedInputCost,
		correction.CorrectedCacheCreationCost,
		correction.CorrectedTotalCost,
		correction.CorrectedActualCost,
		hasAccountStatsCost,
		accountStatsCost,
		correction.OriginalInputTokens,
		correction.OriginalCacheCreationTokens,
	)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if affected > 0 {
		return true, nil
	}

	// Distinguish "best-effort insert has not landed yet" from a conflicting
	// row that was already changed by another accounting path.
	var existingInput, existingCacheCreation int
	err = scanSingleRow(
		ctx,
		r.sql,
		`SELECT input_tokens, cache_creation_tokens
		 FROM usage_logs
		 WHERE request_id = $1 AND api_key_id = $2`,
		[]any{correction.RequestID, correction.APIKeyID},
		&existingInput,
		&existingCacheCreation,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return false, fmt.Errorf(
		"cache write usage reconciliation conflict: request_id=%s api_key_id=%d input=%d cache_write=%d",
		correction.RequestID,
		correction.APIKeyID,
		existingInput,
		existingCacheCreation,
	)
}
