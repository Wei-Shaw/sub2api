package service

import (
	"context"
	"time"
)

// persistGrokFreeRecoveryPendingState keeps the durable latch ahead of the
// finite scheduling lease. Production repositories commit both in one SQL
// statement; the fallback preserves compatibility with focused test doubles
// without ever writing a lease after a failed latch write.
func persistGrokFreeRecoveryPendingState(
	ctx context.Context,
	repo AccountRepository,
	account *Account,
	updates map[string]any,
	resetAt time.Time,
) error {
	if repo == nil || account == nil || account.ID <= 0 {
		return ErrAccountNilInput
	}

	pendingUpdates := make(map[string]any, len(updates)+1)
	for key, value := range updates {
		pendingUpdates[key] = value
	}
	pendingUpdates[GrokFreeRecoveryPendingExtraKey] = true
	now := time.Now()
	resetAt = normalizeGrokRateLimitResetAt(account, resetAt, now)
	horizon := now.Add(grokRateLimitResetHorizon)

	stateCtx, cancel := openAIAccountStateContext(ctx)
	defer cancel()
	if atomicRepo, ok := repo.(GrokFreeRecoveryStateRepository); ok {
		if err := atomicRepo.SetGrokFreeRecoveryPending(stateCtx, account.ID, pendingUpdates, resetAt, horizon); err != nil {
			return err
		}
		// SQL LEAST(..., horizon) may have clamped a dirty multi-week value down
		// to resetAt (already horizon-capped). Keep memory coherent when we knew
		// the stored reset was over-long.
		if grokRateLimitResetRequiresForce(account, resetAt) || account.RateLimitResetAt == nil || account.RateLimitResetAt.Before(resetAt) {
			account.RateLimitResetAt = cloneTimePtr(&resetAt)
			if account.RateLimitedAt == nil {
				limitedAt := now
				account.RateLimitedAt = &limitedAt
			}
		}
		return nil
	}

	if err := repo.UpdateExtra(stateCtx, account.ID, pendingUpdates); err != nil {
		return err
	}
	return writeGrokRateLimitReset(stateCtx, repo, account, resetAt)
}
