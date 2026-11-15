package service

import (
	"context"
	"log/slog"
	"time"
)

// emptyResponseCooldownReason marks the short cooldown an account gets after
// its same-account retries all returned an empty stream.
const emptyResponseCooldownReason = "empty stream response (auto temp-unschedule 1m)"

// ReleaseEmptyStreamTempUnschedule clears an account's cooldown when it is
// still the empty-stream cooldown. The failover loop calls it once the same
// request has come back empty from a second account too: the request, not the
// account, is the likely cause, so the shared account should not stay offline
// for everyone else. Returns true when a cooldown was cleared.
func (s *GatewayService) ReleaseEmptyStreamTempUnschedule(ctx context.Context, accountID int64) bool {
	if s.accountRepo == nil || accountID <= 0 {
		return false
	}
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil || account == nil {
		return false
	}
	if account.TempUnschedulableReason != emptyResponseCooldownReason ||
		account.TempUnschedulableUntil == nil || !time.Now().Before(*account.TempUnschedulableUntil) {
		return false
	}
	if err := s.accountRepo.ClearTempUnschedulable(ctx, accountID); err != nil {
		slog.Warn("empty_stream_cooldown_release_failed", "account_id", accountID, "error", err)
		return false
	}
	slog.Info("empty_stream_cooldown_released", "account_id", accountID)
	return true
}
