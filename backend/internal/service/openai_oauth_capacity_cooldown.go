package service

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	coderws "github.com/coder/websocket"
	"go.uber.org/zap"
)

const openAIOAuthCapacityCooldownReason = "openai_oauth_capacity_cooldown"

type openAIOAuthCapacityRetryContextKey struct{}

type openAIOAuthCapacityRetry struct {
	accountID       int64
	attemptSequence int64
	tripSequence    int64
	overloadUntil   time.Time
}

// WithOpenAIOAuthCapacityCooldownRetry marks the capacity failure that entered
// the current request's existing same-account retry loop. The scheduler resolves
// the matching distributed cooldown marker immediately before selecting again.
func WithOpenAIOAuthCapacityCooldownRetry(ctx context.Context, account *Account) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if account == nil || !account.IsOpenAIOAuth() || account.OpenAIOAuthCapacityAttemptSequence <= 0 {
		return ctx
	}
	retry := openAIOAuthCapacityRetry{accountID: account.ID, attemptSequence: account.OpenAIOAuthCapacityAttemptSequence}
	if account.OverloadUntil != nil && time.Now().Before(*account.OverloadUntil) {
		retry.tripSequence = retry.attemptSequence
		retry.overloadUntil = *account.OverloadUntil
	}
	return context.WithValue(ctx, openAIOAuthCapacityRetryContextKey{}, retry)
}

func (s *OpenAIGatewayService) resolveOpenAIOAuthCapacityRetry(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	retry, ok := ctx.Value(openAIOAuthCapacityRetryContextKey{}).(openAIOAuthCapacityRetry)
	if !ok || retry.accountID <= 0 || retry.attemptSequence <= 0 || s == nil || s.rateLimitService == nil || s.rateLimitService.settingService == nil || s.rateLimitService.openAIOAuthCapacityFailures == nil {
		return ctx
	}
	if !s.rateLimitService.settingService.GetOpenAIOAuthCapacitySettings(ctx).OpenAIOAuthCapacityEnabled {
		return context.WithValue(ctx, openAIOAuthCapacityRetryContextKey{}, openAIOAuthCapacityRetry{})
	}
	tripSequence, until, active, err := s.rateLimitService.openAIOAuthCapacityFailures.GetOpenAIOAuthCapacityCooldown(ctx, retry.accountID)
	if err != nil {
		logger.L().Warn("openai.oauth_capacity_cooldown_resolve_failed", zap.Int64("account_id", retry.accountID), zap.Error(err))
		return ctx
	}
	if !active || tripSequence < retry.attemptSequence {
		return ctx
	}
	retry.tripSequence = tripSequence
	retry.overloadUntil = until
	return context.WithValue(ctx, openAIOAuthCapacityRetryContextKey{}, retry)
}

func openAIOAuthCapacityRetryForAccount(ctx context.Context, account *Account) (openAIOAuthCapacityRetry, bool) {
	if ctx == nil || account == nil || !account.IsOpenAIOAuth() {
		return openAIOAuthCapacityRetry{}, false
	}
	retry, ok := ctx.Value(openAIOAuthCapacityRetryContextKey{}).(openAIOAuthCapacityRetry)
	if !ok || retry.accountID != account.ID || retry.attemptSequence <= 0 || retry.tripSequence < retry.attemptSequence || retry.overloadUntil.IsZero() || !time.Now().Before(retry.overloadUntil) {
		return openAIOAuthCapacityRetry{}, false
	}
	if account.OverloadUntil == nil || !account.OverloadUntil.Equal(retry.overloadUntil) {
		return openAIOAuthCapacityRetry{}, false
	}
	return retry, true
}

func isOpenAIAccountSchedulableForContext(ctx context.Context, account *Account) bool {
	_, ignoreCapacityCooldown := openAIOAuthCapacityRetryForAccount(ctx, account)
	return account != nil && account.isSchedulableAt(time.Now(), ignoreCapacityCooldown)
}

func (s *RateLimitService) beginOpenAIOAuthCapacityAttempt(ctx context.Context, account *Account) int64 {
	if s == nil || s.openAIOAuthCapacityFailures == nil || s.settingService == nil || account == nil || !account.IsOpenAIOAuth() || account.ID <= 0 {
		return 0
	}
	settings := s.settingService.GetOpenAIOAuthCapacitySettings(ctx)
	if !settings.OpenAIOAuthCapacityEnabled {
		return 0
	}
	sequence, err := s.openAIOAuthCapacityFailures.BeginOpenAIOAuthCapacityAttempt(ctx, account.ID)
	if err != nil {
		logger.L().Warn("openai.oauth_capacity_attempt_begin_failed", zap.Int64("account_id", account.ID), zap.Error(err))
		return 0
	}
	return sequence
}

func (s *OpenAIGatewayService) attachOpenAIOAuthCapacityAttempt(ctx context.Context, selection *AccountSelectionResult) {
	if s == nil || s.rateLimitService == nil || selection == nil || selection.Account == nil {
		return
	}
	sequence := s.rateLimitService.beginOpenAIOAuthCapacityAttempt(ctx, selection.Account)
	if sequence <= 0 {
		return
	}
	accountCopy := *selection.Account
	accountCopy.OpenAIOAuthCapacityAttemptSequence = sequence
	selection.Account = &accountCopy
}

func (h *OpenAIWSIngressHooks) openAIOAuthCapacityAccount(account *Account) *Account {
	if h == nil || h.openAIOAuthCapacityAccountForTurn == nil || account == nil {
		return account
	}
	return h.openAIOAuthCapacityAccountForTurn(account)
}

func (h *OpenAIWSIngressHooks) openAIOAuthCapacityAttemptSequence() int64 {
	if h == nil || h.openAIOAuthCapacitySequenceForTurn == nil {
		return 0
	}
	return h.openAIOAuthCapacitySequenceForTurn()
}

func (s *OpenAIGatewayService) openAIOAuthCapacityCooldownActive(ctx context.Context, account *Account) bool {
	if s == nil || s.rateLimitService == nil || s.rateLimitService.settingService == nil || account == nil || !account.IsOpenAIOAuth() {
		return false
	}
	settings := s.rateLimitService.settingService.GetOpenAIOAuthCapacitySettings(ctx)
	if !settings.OpenAIOAuthCapacityEnabled {
		return false
	}
	cache := s.rateLimitService.openAIOAuthCapacityFailures
	if cache != nil {
		_, _, active, err := cache.GetOpenAIOAuthCapacityCooldown(ctx, account.ID)
		if err == nil {
			return active
		}
		logger.L().Warn("openai.oauth_capacity_cooldown_check_failed", zap.Int64("account_id", account.ID), zap.Error(err))
	}
	return s.openAIOAuthCapacityRuntimeCooldownActive(account.ID, time.Now())
}

func (s *OpenAIGatewayService) withOpenAIOAuthCapacityTurnAttempts(ctx context.Context, account *Account, hooks *OpenAIWSIngressHooks) *OpenAIWSIngressHooks {
	if s == nil || s.rateLimitService == nil || account == nil || !account.IsOpenAIOAuth() {
		return hooks
	}

	wrapped := OpenAIWSIngressHooks{}
	if hooks != nil {
		wrapped = *hooks
	}
	var sequence atomic.Int64
	sequence.Store(account.OpenAIOAuthCapacityAttemptSequence)
	wrapped.openAIOAuthCapacityAccountForTurn = func(base *Account) *Account {
		current := sequence.Load()
		if base == nil || current <= 0 {
			return base
		}
		accountCopy := *base
		accountCopy.OpenAIOAuthCapacityAttemptSequence = current
		return &accountCopy
	}
	wrapped.openAIOAuthCapacitySequenceForTurn = sequence.Load

	beforeTurn := wrapped.BeforeTurn
	wrapped.BeforeTurn = func(turn int) error {
		if beforeTurn != nil {
			if err := beforeTurn(turn); err != nil {
				return err
			}
		}
		if turn > 1 && s.openAIOAuthCapacityCooldownActive(ctx, account) {
			return NewOpenAIWSClientCloseError(coderws.StatusTryAgainLater, "account is temporarily unavailable; please reconnect", nil)
		}

		current := sequence.Load()
		if turn > 1 || current <= 0 {
			current = s.rateLimitService.beginOpenAIOAuthCapacityAttempt(ctx, account)
			sequence.Store(current)
		}
		return nil
	}
	return &wrapped
}

func (s *RateLimitService) observeOpenAIOAuthCapacityOutcome(ctx context.Context, account *Account, failed bool, statusCode int) bool {
	if s == nil || s.openAIOAuthCapacityFailures == nil || s.settingService == nil || s.accountRepo == nil || account == nil || !account.IsOpenAIOAuth() || account.ID <= 0 {
		return false
	}
	settings := s.settingService.GetOpenAIOAuthCapacitySettings(ctx)
	if !settings.OpenAIOAuthCapacityEnabled {
		return false
	}
	var err error
	sequence := account.OpenAIOAuthCapacityAttemptSequence
	if sequence <= 0 {
		sequence, err = s.openAIOAuthCapacityFailures.BeginOpenAIOAuthCapacityAttempt(ctx, account.ID)
		if err != nil {
			logger.L().Warn("openai.oauth_capacity_attempt_begin_failed", zap.Int64("account_id", account.ID), zap.Error(err))
			return false
		}
		account.OpenAIOAuthCapacityAttemptSequence = sequence
	}
	count, tripSequence, tripped, err := s.openAIOAuthCapacityFailures.RecordOpenAIOAuthCapacityOutcome(ctx, account.ID, sequence, failed, settings.OpenAIOAuthCapacityWindowMinutes, settings.OpenAIOAuthCapacityFailureThreshold, settings.OpenAIOAuthCapacityCooldownMinutes)
	if err != nil {
		logger.L().Warn("openai.oauth_capacity_cooldown_record_failed", zap.Int64("account_id", account.ID), zap.Error(err))
		return false
	}
	if !tripped {
		return false
	}

	// PostgreSQL timestamptz is microsecond-precise. Canonicalize before the
	// Redis marker, database write, runtime block, and retry context diverge.
	desiredUntil := time.Now().Add(time.Duration(settings.OpenAIOAuthCapacityCooldownMinutes) * time.Minute).Truncate(time.Microsecond)
	baseCtx := context.Background()
	if ctx != nil {
		baseCtx = context.WithoutCancel(ctx)
	}
	persistCtx, cancel := context.WithTimeout(baseCtx, 3*time.Second)
	defer cancel()
	preparedUntil, publicationID, owned, err := s.openAIOAuthCapacityFailures.PrepareOpenAIOAuthCapacityCooldown(persistCtx, account.ID, tripSequence, desiredUntil)
	if err != nil {
		logger.L().Warn("openai.oauth_capacity_cooldown_prepare_failed", zap.Int64("account_id", account.ID), zap.Error(err))
		return false
	}
	if !owned {
		return false
	}
	desiredUntil = preparedUntil
	if err := s.accountRepo.SetOverloaded(persistCtx, account.ID, desiredUntil); err != nil {
		logger.L().Warn("openai.oauth_capacity_cooldown_persist_failed", zap.Int64("account_id", account.ID), zap.Error(err))
		return false
	}
	if err := s.openAIOAuthCapacityFailures.AcknowledgeOpenAIOAuthCapacityCooldown(persistCtx, account.ID, tripSequence, desiredUntil, publicationID); err != nil {
		logger.L().Warn("openai.oauth_capacity_cooldown_ack_failed", zap.Int64("account_id", account.ID), zap.Error(err))
	}
	actualUntil := desiredUntil
	if account.OverloadUntil != nil && account.OverloadUntil.After(actualUntil) {
		actualUntil = *account.OverloadUntil
	}
	account.OverloadUntil = &actualUntil
	s.notifyAccountSchedulingBlocked(account, desiredUntil, openAIOAuthCapacityCooldownReason)
	logger.L().Warn("openai.oauth_capacity_cooldown_tripped",
		zap.Int64("account_id", account.ID), zap.Int64("failure_count", count), zap.Int64("trip_sequence", tripSequence),
		zap.Int("failure_threshold", settings.OpenAIOAuthCapacityFailureThreshold), zap.Int("window_minutes", settings.OpenAIOAuthCapacityWindowMinutes),
		zap.Int("cooldown_minutes", settings.OpenAIOAuthCapacityCooldownMinutes), zap.Int("upstream_status", statusCode), zap.Time("until", actualUntil))
	return true
}

// ObserveOpenAIOAuthCapacityFailure records one terminal capacity outcome. The
// attempt sequence prevents duplicate protocol signals and late completions from
// corrupting the consecutive-outcome order.
func (s *RateLimitService) ObserveOpenAIOAuthCapacityFailure(ctx context.Context, account *Account, statusCode int, responseBody []byte, message string) bool {
	if account == nil || !isOpenAIRequestScopedCapacityShed(message, responseBody) {
		return false
	}
	return s.observeOpenAIOAuthCapacityOutcome(ctx, account, true, statusCode)
}

func (s *RateLimitService) ObserveOpenAIOAuthCapacityNonFailure(ctx context.Context, account *Account) {
	_ = s.observeOpenAIOAuthCapacityOutcome(ctx, account, false, 0)
}
