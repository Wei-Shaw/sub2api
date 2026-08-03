package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
)

const (
	openAIRateLimitReconcileLeaderLockKey  = "openai:rate-limit:reconcile:leader"
	openAIRateLimitReconcileOverscanFactor = 4
	openAIRateLimitReconcileLockGrace      = 30 * time.Second
)

var errOpenAIRateLimitReconcileRepositoryUnavailable = errors.New("openai rate-limit reconciliation repository is unavailable")

// OpenAIQuotaUsageQuerier is the narrow read-only quota surface required by the
// reconciler. The concrete implementation calls ChatGPT's /backend-api/wham/usage.
type OpenAIQuotaUsageQuerier interface {
	QueryUsageForReconciliation(ctx context.Context, accountID int64) (*OpenAIQuotaUsage, error)
}

// OpenAIRateLimitRecoveryRepository exposes bounded candidate selection and an
// exact-generation compare-and-clear mutation.
type OpenAIRateLimitRecoveryRepository interface {
	ListOpenAIRateLimitRecoveryCandidates(ctx context.Context, now time.Time, limit int) ([]Account, error)
	ClearOpenAIRateLimitIfObserved(
		ctx context.Context,
		accountID int64,
		observedRateLimitedAt time.Time,
		observedResetAt time.Time,
	) (bool, error)
}

// OpenAIRateLimitRecoveryRuntimeBlocker protects the process-local scheduling
// fast path with an opaque generation, so a concurrent 429 always wins.
type OpenAIRateLimitRecoveryRuntimeBlocker interface {
	AccountSchedulingBlockGeneration(accountID int64) uint64
	ClearAccountSchedulingBlockIfGeneration(accountID int64, observedGeneration uint64) bool
}

// OpenAIRateLimitReconcilerService periodically verifies future-dated,
// account-level OpenAI OAuth rate limits against the authoritative quota
// endpoint. It never sends model traffic and never clears independent blocks.
type OpenAIRateLimitReconcilerService struct {
	accountRepo    AccountRepository
	quotaQuerier   OpenAIQuotaUsageQuerier
	runtimeBlocker OpenAIRateLimitRecoveryRuntimeBlocker
	cfg            config.GatewayOpenAIRateLimitReconcileConfig

	parentCtx    context.Context
	parentCancel context.CancelFunc
	wg           sync.WaitGroup
	mu           sync.Mutex
	cycleMu      sync.Mutex
	started      bool
	stopped      bool

	lockCache  LeaderLockCache
	db         *sql.DB
	instanceID string
	now        func() time.Time
}

func NewOpenAIRateLimitReconcilerService(
	accountRepo AccountRepository,
	quotaQuerier OpenAIQuotaUsageQuerier,
	runtimeBlocker OpenAIRateLimitRecoveryRuntimeBlocker,
	cfg *config.Config,
) *OpenAIRateLimitReconcilerService {
	reconcileCfg := config.GatewayOpenAIRateLimitReconcileConfig{
		IntervalSeconds:     300,
		MaxAccountsPerCycle: 20,
		Concurrency:         2,
	}
	if cfg != nil {
		reconcileCfg = cfg.Gateway.OpenAIRateLimitReconcile
	}
	normalizeOpenAIRateLimitReconcileConfig(&reconcileCfg)
	ctx, cancel := context.WithCancel(context.Background())
	return &OpenAIRateLimitReconcilerService{
		accountRepo:    accountRepo,
		quotaQuerier:   quotaQuerier,
		runtimeBlocker: runtimeBlocker,
		cfg:            reconcileCfg,
		parentCtx:      ctx,
		parentCancel:   cancel,
		instanceID:     uuid.NewString(),
		now:            time.Now,
	}
}

func normalizeOpenAIRateLimitReconcileConfig(cfg *config.GatewayOpenAIRateLimitReconcileConfig) {
	if cfg.IntervalSeconds <= 0 {
		cfg.IntervalSeconds = 300
	}
	if cfg.MaxAccountsPerCycle <= 0 {
		cfg.MaxAccountsPerCycle = 20
	}
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 2
	}
}

func (s *OpenAIRateLimitReconcilerService) SetLeaderLock(lockCache LeaderLockCache, db *sql.DB) {
	if s == nil {
		return
	}
	s.lockCache = lockCache
	s.db = db
}

func (s *OpenAIRateLimitReconcilerService) Start() {
	if s == nil || !s.cfg.Enabled {
		return
	}
	s.mu.Lock()
	if s.started || s.stopped {
		s.mu.Unlock()
		return
	}
	s.started = true
	s.wg.Add(1)
	s.mu.Unlock()

	go s.runLoop()
	slog.Info("openai_rate_limit_reconciler_started",
		"interval_seconds", s.cfg.IntervalSeconds,
		"max_accounts_per_cycle", s.cfg.MaxAccountsPerCycle,
		"concurrency", s.cfg.Concurrency,
	)
}

func (s *OpenAIRateLimitReconcilerService) Stop() {
	if s == nil {
		return
	}
	s.mu.Lock()
	if !s.stopped {
		s.stopped = true
		s.parentCancel()
	}
	s.mu.Unlock()
	s.wg.Wait()
}

func (s *OpenAIRateLimitReconcilerService) runLoop() {
	defer s.wg.Done()
	if err := s.RunOnce(s.parentCtx); err != nil && s.parentCtx.Err() == nil {
		slog.Warn("openai_rate_limit_reconcile_cycle_failed", "error", err)
	}

	ticker := time.NewTicker(s.interval())
	defer ticker.Stop()
	for {
		select {
		case <-s.parentCtx.Done():
			return
		case <-ticker.C:
			if err := s.RunOnce(s.parentCtx); err != nil && s.parentCtx.Err() == nil {
				slog.Warn("openai_rate_limit_reconcile_cycle_failed", "error", err)
			}
		}
	}
}

// RunOnce executes one bounded scheduled cycle. It is exported for deterministic
// tests and operational diagnostics; the feature switch still applies.
func (s *OpenAIRateLimitReconcilerService) RunOnce(ctx context.Context) error {
	if s == nil || !s.cfg.Enabled || s.accountRepo == nil || s.quotaQuerier == nil {
		return nil
	}
	recoveryRepo, ok := s.accountRepo.(OpenAIRateLimitRecoveryRepository)
	if !ok {
		return errOpenAIRateLimitReconcileRepositoryUnavailable
	}

	s.cycleMu.Lock()
	defer s.cycleMu.Unlock()

	now := s.currentTime()
	release, acquired := s.acquireCycleLeadership(ctx, now)
	if !acquired {
		return nil
	}
	defer release()

	queryLimit := s.cfg.MaxAccountsPerCycle * openAIRateLimitReconcileOverscanFactor
	accounts, err := recoveryRepo.ListOpenAIRateLimitRecoveryCandidates(ctx, now, queryLimit)
	if err != nil {
		return fmt.Errorf("list OpenAI rate-limit recovery candidates: %w", err)
	}

	eligible := make([]Account, 0, min(len(accounts), s.cfg.MaxAccountsPerCycle))
	for i := range accounts {
		if shouldProbeOpenAIRateLimitRecovery(&accounts[i], now) {
			eligible = append(eligible, accounts[i])
			if len(eligible) == s.cfg.MaxAccountsPerCycle {
				break
			}
		}
	}

	var group errgroup.Group
	group.SetLimit(s.cfg.Concurrency)
	for i := range eligible {
		account := eligible[i]
		group.Go(func() error {
			s.reconcileAccount(ctx, recoveryRepo, &account)
			return nil
		})
	}
	return group.Wait()
}

func (s *OpenAIRateLimitReconcilerService) acquireCycleLeadership(ctx context.Context, now time.Time) (func(), bool) {
	lockCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	runRelease, acquired := tryAcquireSingletonLeaderLock(
		lockCtx,
		s.lockCache,
		s.db,
		openAIRateLimitReconcileLeaderLockKey,
		s.instanceID,
		s.lockTTL(),
	)
	if !acquired {
		return nil, false
	}

	cadenceKey := fmt.Sprintf("%s:%d", openAIRateLimitReconcileLeaderLockKey, now.Unix()/int64(s.interval()/time.Second))
	cadenceRelease, acquired := tryAcquireSingletonLeaderLock(
		lockCtx,
		s.lockCache,
		s.db,
		cadenceKey,
		s.instanceID,
		s.lockTTL(),
	)
	if !acquired {
		runRelease()
		return nil, false
	}

	nextBoundary := now.Truncate(s.interval()).Add(s.interval())
	return func() {
		runRelease()
		releaseOpenAIRateLimitCadenceLock(cadenceRelease, nextBoundary)
	}, true
}

func releaseOpenAIRateLimitCadenceLock(release func(), releaseAt time.Time) {
	if release == nil {
		return
	}
	delay := time.Until(releaseAt)
	if delay <= 0 {
		release()
		return
	}
	time.AfterFunc(delay, release)
}

func (s *OpenAIRateLimitReconcilerService) reconcileAccount(
	ctx context.Context,
	recoveryRepo OpenAIRateLimitRecoveryRepository,
	observed *Account,
) {
	if observed == nil || observed.RateLimitedAt == nil || observed.RateLimitResetAt == nil {
		return
	}
	runtimeGeneration := uint64(0)
	if s.runtimeBlocker != nil {
		runtimeGeneration = s.runtimeBlocker.AccountSchedulingBlockGeneration(observed.ID)
	}

	usage, err := s.quotaQuerier.QueryUsageForReconciliation(ctx, observed.ID)
	if err != nil {
		if ctx.Err() == nil {
			slog.Debug("openai_rate_limit_reconcile_probe_failed", "account_id", observed.ID, "error", err)
		}
		return
	}
	if !openAIQuotaUsageConfirmsAvailable(usage) {
		return
	}

	cleared, err := recoveryRepo.ClearOpenAIRateLimitIfObserved(
		ctx,
		observed.ID,
		*observed.RateLimitedAt,
		*observed.RateLimitResetAt,
	)
	if err != nil {
		if ctx.Err() == nil {
			slog.Warn("openai_rate_limit_reconcile_clear_failed", "account_id", observed.ID, "error", err)
		}
		return
	}
	if !cleared {
		return
	}

	if s.runtimeBlocker != nil &&
		!s.runtimeBlocker.ClearAccountSchedulingBlockIfGeneration(observed.ID, runtimeGeneration) {
		slog.Info("openai_rate_limit_reconcile_runtime_clear_skipped_newer_block", "account_id", observed.ID)
		return
	}
	slog.Info("openai_rate_limit_reconciled",
		"account_id", observed.ID,
		"previous_reset_at", observed.RateLimitResetAt.UTC(),
		"quota_fetched_at", usage.FetchedAt,
	)
}

func (s *OpenAIRateLimitReconcilerService) interval() time.Duration {
	return time.Duration(s.cfg.IntervalSeconds) * time.Second
}

func (s *OpenAIRateLimitReconcilerService) lockTTL() time.Duration {
	batches := (s.cfg.MaxAccountsPerCycle + s.cfg.Concurrency - 1) / s.cfg.Concurrency
	worstCase := time.Duration(batches) * openaiQuotaUpstreamTimeout
	ttl := worstCase + openAIRateLimitReconcileLockGrace
	if minimum := s.interval() + openAIRateLimitReconcileLockGrace; ttl < minimum {
		ttl = minimum
	}
	return ttl
}

func (s *OpenAIRateLimitReconcilerService) currentTime() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

func shouldProbeOpenAIRateLimitRecovery(account *Account, now time.Time) bool {
	if account == nil ||
		account.Platform != PlatformOpenAI ||
		account.Type != AccountTypeOAuth ||
		account.IsShadow() ||
		account.Status != StatusActive ||
		!account.Schedulable ||
		account.RateLimitedAt == nil ||
		account.RateLimitResetAt == nil ||
		!now.Before(*account.RateLimitResetAt) {
		return false
	}
	if account.AutoPauseOnExpired && account.ExpiresAt != nil && !now.Before(*account.ExpiresAt) {
		return false
	}
	if account.OverloadUntil != nil && now.Before(*account.OverloadUntil) {
		return false
	}
	if account.TempUnschedulableUntil != nil && now.Before(*account.TempUnschedulableUntil) {
		return false
	}
	return !hasActiveModelRateLimitAt(account, now)
}

func hasActiveModelRateLimitAt(account *Account, now time.Time) bool {
	if account == nil || account.Extra == nil {
		return false
	}
	rawLimits, ok := account.Extra[modelRateLimitsKey]
	if !ok || rawLimits == nil {
		return false
	}
	limits, ok := rawLimits.(map[string]any)
	if !ok {
		// Unknown persisted shapes fail closed instead of weakening a separate
		// scheduler guard.
		return true
	}
	for _, rawLimit := range limits {
		limit, ok := rawLimit.(map[string]any)
		if !ok {
			return true
		}
		resetAtRaw, ok := limit["rate_limit_reset_at"].(string)
		if !ok {
			return true
		}
		resetAt, err := time.Parse(time.RFC3339, resetAtRaw)
		if err != nil {
			return true
		}
		if now.Before(resetAt) {
			return true
		}
	}
	return false
}

func openAIQuotaUsageConfirmsAvailable(usage *OpenAIQuotaUsage) bool {
	if usage == nil || usage.RateLimit == nil {
		return false
	}
	limit := usage.RateLimit
	if !limit.Allowed || limit.LimitReached {
		return false
	}

	windowCount := 0
	for _, window := range []*OpenAIRateLimitWindow{limit.PrimaryWindow, limit.SecondaryWindow} {
		if window == nil {
			continue
		}
		windowCount++
		if window.UsedPercent < 0 || window.UsedPercent >= 100 {
			return false
		}
	}
	return windowCount > 0
}
