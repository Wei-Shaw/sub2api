package service

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/kirocooldown"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"go.uber.org/zap"
)

var errKiroCooldownStoreUnavailable = errors.New("kiro cooldown store unavailable")

type KiroCooldownStore interface {
	ReserveRequest(ctx context.Context, tokenKey string) (time.Duration, error)
	MarkSuccess(ctx context.Context, tokenKey string) error
	Mark429(ctx context.Context, tokenKey string) (time.Duration, error)
	MarkSuspended(ctx context.Context, tokenKey string) (time.Duration, error)
	GetState(ctx context.Context, tokenKey string) (*kirocooldown.State, error)
	ClearEarliestTransientCooldown(ctx context.Context, tokenKeys []string) (bool, error)
}

func asKiroCooldownFailoverError(err error) *UpstreamFailoverError {
	if err == nil {
		return nil
	}
	var cooldownErr *kirocooldown.Error
	if !errors.As(err, &cooldownErr) {
		return nil
	}
	return &UpstreamFailoverError{
		StatusCode:   http.StatusTooManyRequests,
		ResponseBody: []byte(cooldownErr.Error()),
	}
}

func (s *GatewayService) checkAndWaitKiroCooldown(ctx context.Context, tokenKey string) error {
	if s == nil || s.kiroCooldownStore == nil {
		return errKiroCooldownStoreUnavailable
	}
	waitFor, err := s.kiroCooldownStore.ReserveRequest(ctx, tokenKey)
	if err != nil {
		return err
	}
	if waitFor <= 0 {
		return nil
	}
	timer := time.NewTimer(waitFor)
	select {
	case <-ctx.Done():
		if !timer.Stop() {
			<-timer.C
		}
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// markKiroSuccess records a successful Kiro response. Pair with markKiro429:
// because markKiro429 writes account.rate_limit_reset_at into DB, an account
// that recovers (success) must also clear that DB field, otherwise the
// scheduler keeps filtering the now-healthy account until the stale
// rate_limit_reset_at naturally expires (up to 5min). accountID may be 0 for
// callers that don't have it (Redis-only clear).
func (s *GatewayService) markKiroSuccess(ctx context.Context, accountID int64, tokenKey string) error {
	if s == nil || s.kiroCooldownStore == nil {
		return errKiroCooldownStoreUnavailable
	}
	if err := s.kiroCooldownStore.MarkSuccess(ctx, tokenKey); err != nil {
		return err
	}
	if s.accountRepo != nil && accountID > 0 {
		if dbErr := s.accountRepo.ClearRateLimit(ctx, accountID); dbErr != nil {
			logger.L().Warn("kiro.mark_success_db_clear_failed",
				zap.Int64("account_id", accountID),
				zap.Error(dbErr),
			)
		}
	}
	return nil
}

// markKiro429 records a Kiro 429 in both Redis (kiroCooldownStore) and DB
// (account.rate_limit_reset_at). Syncing to DB is critical: without it,
// ListSchedulable* still returns this account as schedulable, so the failover
// loop keeps re-picking it just to bounce off the Redis gate (asKiroCooldownFailoverError)
// — burning failover slots and amplifying retry storms. accountID may be 0 for
// callers that don't have it; we fall back to Redis-only.
func (s *GatewayService) markKiro429(ctx context.Context, accountID int64, tokenKey string) (time.Duration, error) {
	if s == nil || s.kiroCooldownStore == nil {
		return 0, errKiroCooldownStoreUnavailable
	}
	cooldown, err := s.kiroCooldownStore.Mark429(ctx, tokenKey)
	if err != nil {
		return 0, err
	}
	if s.accountRepo != nil && accountID > 0 && cooldown > 0 {
		resetAt := time.Now().Add(cooldown)
		if dbErr := s.accountRepo.SetRateLimited(ctx, accountID, resetAt); dbErr != nil {
			logger.L().Warn("kiro.mark_429_db_sync_failed",
				zap.Int64("account_id", accountID),
				zap.Duration("cooldown", cooldown),
				zap.Error(dbErr),
			)
		}
	}
	return cooldown, nil
}

func (s *GatewayService) markKiroSuspended(ctx context.Context, tokenKey string) (time.Duration, error) {
	if s == nil || s.kiroCooldownStore == nil {
		return 0, errKiroCooldownStoreUnavailable
	}
	return s.kiroCooldownStore.MarkSuspended(ctx, tokenKey)
}

func (s *GatewayService) getKiroCooldownState(ctx context.Context, tokenKey string) (*kirocooldown.State, error) {
	if s == nil || s.kiroCooldownStore == nil {
		return nil, errKiroCooldownStoreUnavailable
	}
	return s.kiroCooldownStore.GetState(ctx, tokenKey)
}

func kiroRuntimeStateSnapshot(state *kirocooldown.State) (string, string, *time.Time) {
	if state == nil || !state.Active {
		return "", "", nil
	}
	resetAt := state.CooldownUntil
	switch state.Reason {
	case kirocooldown.CooldownReasonSuspended:
		return "suspended", state.Reason, &resetAt
	default:
		return "cooldown", state.Reason, &resetAt
	}
}
