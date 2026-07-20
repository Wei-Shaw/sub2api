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
	resetAt = normalizeGrokRateLimitResetAt(account, resetAt, time.Now())

	stateCtx, cancel := openAIAccountStateContext(ctx)
	defer cancel()
	if atomicRepo, ok := repo.(GrokFreeRecoveryStateRepository); ok {
		return atomicRepo.SetGrokFreeRecoveryPending(stateCtx, account.ID, pendingUpdates, resetAt)
	}

	if err := repo.UpdateExtra(stateCtx, account.ID, pendingUpdates); err != nil {
		return err
	}
	if extendingRepo, ok := repo.(grokRateLimitExtendingRepository); ok {
		return extendingRepo.SetRateLimitedIfLater(stateCtx, account.ID, resetAt)
	}
	return repo.SetRateLimited(stateCtx, account.ID, resetAt)
}
