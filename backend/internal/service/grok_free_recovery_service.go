package service

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/google/uuid"
)

const (
	grokFreeRecoveryScanInterval  = time.Minute
	grokFreeRecoveryProbeInterval = 5 * time.Minute
	grokFreeRecoveryLeaseDuration = 10 * time.Minute
	grokFreeRecoveryRunTimeout    = 4 * time.Minute
	grokFreeRecoveryLeaderLockTTL = 5 * time.Minute
	grokFreeRecoveryMaxWorkers    = 3
	// Cap consecutive-failure backoff so Free accounts still recover within a
	// business day once quota resets, without re-probing every base interval.
	grokFreeRecoveryMaxBackoff           = 6 * time.Hour
	grokFreeRecoveryLeaderLockKey        = "sub2api:grok-free-recovery"
	grokFreeProactiveProbeTokenThreshold = xai.GrokFreeRolling24hTokenLimit * 99 / 100
)

type grokFreeRecoveryAccountStore interface {
	ListByPlatform(ctx context.Context, platform string) ([]Account, error)
	GetByID(ctx context.Context, id int64) (*Account, error)
	SetRateLimited(ctx context.Context, id int64, resetAt time.Time) error
	UpdateExtra(ctx context.Context, id int64, updates map[string]any) error
}

type grokFreeRecoveryRateLimitExtender interface {
	SetRateLimitedIfLater(ctx context.Context, id int64, resetAt time.Time) error
}

type grokFreeRecoveryCandidateStore interface {
	ClaimDueGrokFreeRecoveryCandidates(ctx context.Context, now, nextProbeAt, leaseUntil time.Time, limit int) ([]Account, error)
	ListGrokFreeProactiveCandidates(ctx context.Context, now time.Time, afterID int64, limit int) ([]Account, error)
	ClaimGrokFreeProactiveCandidates(ctx context.Context, ids []int64, now, nextProbeAt, leaseUntil time.Time) ([]Account, error)
}

type grokFreeRecoveryResultRecorder interface {
	RecordGrokFreeRecoveryProbeResult(ctx context.Context, id int64, expectedNextProbeAt time.Time, result string, completedAt time.Time) (bool, error)
}

type grokFreeRecoveryProber interface {
	probeUsage(ctx context.Context, accountID int64) (*GrokQuotaProbeResult, error)
}

type grokFreeRecoveryUsageReader interface {
	grokFreeRollingUsage(ctx context.Context, accountIDs []int64, now time.Time) (map[int64]*WindowStats, error)
}

type grokFreeRecoveryStateRecoverer interface {
	RecoverGrokFreeAfterSuccessfulProbe(ctx context.Context, accountID int64, probeStartedAt, nextProbeAt time.Time) (bool, error)
}

// GrokFreeRecoveryService keeps pending accounts out of scheduling until a
// minimal direct xAI request proves that they have recovered.
type GrokFreeRecoveryService struct {
	accountStore grokFreeRecoveryAccountStore
	prober       grokFreeRecoveryProber
	recoverer    grokFreeRecoveryStateRecoverer
	lockCache    LeaderLockCache
	db           *sql.DB
	instanceID   string

	enabled               bool
	scanInterval          time.Duration
	probeInterval         time.Duration
	runTimeout            time.Duration
	candidatePageSize     int
	maxCandidatesPerCycle int
	maxWorkers            int
	now                   func() time.Time
	metrics               grokFreeRecoveryMetrics
	proactiveCursor       atomic.Int64
	running               atomic.Bool
	startOnce             sync.Once
	stopOnce              sync.Once
	cancel                context.CancelFunc
	wg                    sync.WaitGroup
}

type grokFreeRecoveryMetrics struct {
	cycles           atomic.Int64
	claimErrors      atomic.Int64
	pendingClaimed   atomic.Int64
	proactiveScanned atomic.Int64
	proactiveClaimed atomic.Int64
	probes           atomic.Int64
	healthy          atomic.Int64
	limited          atomic.Int64
	failed           atomic.Int64
	casRejected      atomic.Int64
}

type GrokFreeRecoveryMetricsSnapshot struct {
	Cycles           int64
	ClaimErrors      int64
	PendingClaimed   int64
	ProactiveScanned int64
	ProactiveClaimed int64
	Probes           int64
	Healthy          int64
	Limited          int64
	Failed           int64
	CASRejected      int64
}

func newGrokFreeRecoveryService(
	accountStore grokFreeRecoveryAccountStore,
	prober grokFreeRecoveryProber,
	recoverer grokFreeRecoveryStateRecoverer,
	lockCache LeaderLockCache,
	db *sql.DB,
) *GrokFreeRecoveryService {
	return &GrokFreeRecoveryService{
		accountStore:          accountStore,
		prober:                prober,
		recoverer:             recoverer,
		lockCache:             lockCache,
		db:                    db,
		instanceID:            uuid.NewString(),
		enabled:               true,
		scanInterval:          grokFreeRecoveryScanInterval,
		probeInterval:         grokFreeRecoveryProbeInterval,
		runTimeout:            grokFreeRecoveryRunTimeout,
		candidatePageSize:     100,
		maxCandidatesPerCycle: 300,
		maxWorkers:            grokFreeRecoveryMaxWorkers,
		now:                   time.Now,
	}
}

func (s *GrokFreeRecoveryService) configure(
	enabled bool,
	scanInterval, probeInterval, runTimeout time.Duration,
	candidatePageSize, maxCandidatesPerCycle, maxWorkers int,
) {
	if s == nil {
		return
	}
	s.enabled = enabled
	if scanInterval > 0 {
		s.scanInterval = scanInterval
	}
	if probeInterval > 0 {
		s.probeInterval = probeInterval
	}
	if runTimeout > 0 {
		s.runTimeout = runTimeout
	}
	if candidatePageSize > 0 {
		s.candidatePageSize = candidatePageSize
	}
	if maxCandidatesPerCycle > 0 {
		s.maxCandidatesPerCycle = maxCandidatesPerCycle
	}
	if maxWorkers > 0 {
		s.maxWorkers = maxWorkers
	}
}

func (s *GrokFreeRecoveryService) Metrics() GrokFreeRecoveryMetricsSnapshot {
	if s == nil {
		return GrokFreeRecoveryMetricsSnapshot{}
	}
	return GrokFreeRecoveryMetricsSnapshot{
		Cycles: s.metrics.cycles.Load(), ClaimErrors: s.metrics.claimErrors.Load(),
		PendingClaimed: s.metrics.pendingClaimed.Load(), ProactiveScanned: s.metrics.proactiveScanned.Load(),
		ProactiveClaimed: s.metrics.proactiveClaimed.Load(), Probes: s.metrics.probes.Load(),
		Healthy: s.metrics.healthy.Load(), Limited: s.metrics.limited.Load(),
		Failed: s.metrics.failed.Load(), CASRejected: s.metrics.casRejected.Load(),
	}
}

func (s *GrokFreeRecoveryService) Start() {
	if s == nil {
		return
	}
	if !s.enabled {
		// Visible kill-switch signal: silent disable made production outages look
		// like "next probe shown but never probed".
		logger.LegacyPrintf("service.grok_free_recovery", "[GrokFreeRecovery] disabled by config (grok_free_recovery.enabled=false)")
		return
	}
	if s.accountStore == nil || s.prober == nil || s.recoverer == nil {
		logger.LegacyPrintf(
			"service.grok_free_recovery",
			"[GrokFreeRecovery] not started: missing deps accountStore=%v prober=%v recoverer=%v",
			s.accountStore != nil,
			s.prober != nil,
			s.recoverer != nil,
		)
		return
	}
	s.startOnce.Do(func() {
		ctx, cancel := context.WithCancel(context.Background())
		s.cancel = cancel
		s.wg.Add(1)
		go s.run(ctx)
		logger.LegacyPrintf(
			"service.grok_free_recovery",
			"[GrokFreeRecovery] started (scan=%s probe=%s workers=%d page=%d max_per_cycle=%d)",
			s.scanInterval,
			s.probeInterval,
			s.maxWorkers,
			s.candidatePageSize,
			s.maxCandidatesPerCycle,
		)
	})
}

func (s *GrokFreeRecoveryService) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		if s.cancel != nil {
			s.cancel()
		}
		s.wg.Wait()
	})
}

func (s *GrokFreeRecoveryService) run(ctx context.Context) {
	defer s.wg.Done()
	defer func() {
		if recovered := recover(); recovered != nil {
			logger.LegacyPrintf("service.grok_free_recovery", "[GrokFreeRecovery] panic in worker loop: %v", recovered)
		}
	}()
	s.runCycle(ctx)

	interval := s.scanInterval
	if interval <= 0 {
		interval = grokFreeRecoveryScanInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runCycle(ctx)
		}
	}
}

func (s *GrokFreeRecoveryService) runCycle(parent context.Context) {
	if s == nil || !s.enabled || s.accountStore == nil || s.prober == nil || s.recoverer == nil {
		return
	}
	if !s.running.CompareAndSwap(false, true) {
		// Previous cycle still running (long probe batch). Skip without
		// starving forever: the in-flight cycle owns the next probe advances.
		return
	}
	defer s.running.Store(false)

	s.metrics.cycles.Add(1)
	startedAt := s.now()
	acquiredLeader := false
	defer func() {
		metrics := s.Metrics()
		slog.Info("grok_free_recovery_cycle_metrics",
			"duration_ms", s.now().Sub(startedAt).Milliseconds(),
			"leader_acquired", acquiredLeader,
			"cycles_total", metrics.Cycles,
			"claim_errors_total", metrics.ClaimErrors,
			"pending_claimed_total", metrics.PendingClaimed,
			"proactive_scanned_total", metrics.ProactiveScanned,
			"proactive_claimed_total", metrics.ProactiveClaimed,
			"probes_total", metrics.Probes,
			"healthy_total", metrics.Healthy,
			"limited_total", metrics.Limited,
			"failed_total", metrics.Failed,
			"cas_rejected_total", metrics.CASRejected,
		)
	}()

	runTimeout := s.runTimeout
	if runTimeout <= 0 {
		runTimeout = grokFreeRecoveryRunTimeout
	}
	ctx, cancel := context.WithTimeout(parent, runTimeout)
	defer cancel()
	release, acquired := tryAcquireSingletonLeaderLock(
		ctx,
		s.lockCache,
		s.db,
		grokFreeRecoveryLeaderLockKey,
		s.instanceID,
		grokFreeRecoveryLeaderLockTTL,
	)
	if !acquired {
		slog.Info("grok_free_recovery_leader_not_acquired")
		return
	}
	acquiredLeader = true
	if release != nil {
		defer release()
	}
	if claimStore, ok := s.accountStore.(grokFreeRecoveryCandidateStore); ok {
		s.runClaimedCycle(ctx, claimStore)
		return
	}
	slog.Warn("grok_free_recovery_using_legacy_cycle",
		"reason", "accountStore does not implement ClaimDueGrokFreeRecoveryCandidates")
	s.runLegacyCycle(ctx)
}

func (s *GrokFreeRecoveryService) runClaimedCycle(ctx context.Context, store grokFreeRecoveryCandidateStore) {
	remaining := s.maxCandidatesPerCycle
	if remaining <= 0 {
		remaining = 300
	}
	pageSize := s.candidatePageSize
	if pageSize <= 0 {
		pageSize = 100
	}

	// Drain overdue pending accounts first. Proactive token scans only start
	// after the pending queue is empty, so a large pool cannot starve recovery.
	for remaining > 0 && ctx.Err() == nil {
		limit := min(pageSize, remaining)
		claimedAt := s.now()
		claimed, err := store.ClaimDueGrokFreeRecoveryCandidates(
			ctx,
			claimedAt,
			claimedAt.Add(s.effectiveProbeInterval()),
			claimedAt.Add(s.effectiveLeaseDuration()),
			limit,
		)
		if err != nil {
			s.metrics.claimErrors.Add(1)
			// Error-level: ops_runtime_log_config often sets level=error in prod,
			// and silent claim failures left Free accounts latched forever.
			slog.Error("grok_free_recovery_claim_pending_failed", "error", err)
			return
		}
		s.metrics.pendingClaimed.Add(int64(len(claimed)))
		if len(claimed) == 0 {
			break
		}
		s.probeCandidates(ctx, claimed, true)
		remaining -= len(claimed)
		if len(claimed) < limit {
			break
		}
	}
	if remaining <= 0 || ctx.Err() != nil {
		return
	}

	usageReader, ok := s.prober.(grokFreeRecoveryUsageReader)
	if !ok {
		return
	}
	afterID := s.proactiveCursor.Load()
	scanRemaining := remaining
	for remaining > 0 && scanRemaining > 0 && ctx.Err() == nil {
		pageLimit := min(pageSize, scanRemaining)
		page, err := store.ListGrokFreeProactiveCandidates(ctx, s.now(), afterID, pageLimit)
		if err != nil {
			s.metrics.claimErrors.Add(1)
			slog.Warn("grok_free_proactive_list_failed", "error", err)
			return
		}
		if len(page) == 0 {
			s.proactiveCursor.Store(0)
			return
		}
		afterID = page[len(page)-1].ID
		s.proactiveCursor.Store(afterID)
		scanRemaining -= len(page)
		s.metrics.proactiveScanned.Add(int64(len(page)))

		eligible := make([]Account, 0, len(page))
		ids := make([]int64, 0, len(page))
		now := s.now()
		for i := range page {
			if grokFreeProactiveUsageCandidate(&page[i], now) {
				eligible = append(eligible, page[i])
				ids = append(ids, page[i].ID)
			}
		}
		if len(ids) > 0 {
			usageByAccount, usageErr := usageReader.grokFreeRollingUsage(ctx, ids, now)
			if usageErr != nil {
				slog.Warn("grok_free_proactive_usage_query_failed", "error", usageErr)
				return
			}
			selected := make([]int64, 0, min(len(eligible), remaining))
			for i := range eligible {
				usage := usageByAccount[eligible[i].ID]
				if usage != nil && usage.Tokens >= grokFreeProactiveProbeTokenThreshold {
					selected = append(selected, eligible[i].ID)
					if len(selected) >= remaining {
						break
					}
				}
			}
			if len(selected) > 0 {
				claimedAt := s.now()
				claimed, claimErr := store.ClaimGrokFreeProactiveCandidates(
					ctx,
					selected,
					claimedAt,
					claimedAt.Add(s.effectiveProbeInterval()),
					claimedAt.Add(s.effectiveLeaseDuration()),
				)
				if claimErr != nil {
					s.metrics.claimErrors.Add(1)
					slog.Warn("grok_free_recovery_claim_proactive_failed", "error", claimErr)
					return
				}
				s.metrics.proactiveClaimed.Add(int64(len(claimed)))
				s.probeCandidates(ctx, claimed, true)
				remaining -= len(claimed)
			}
		}
		if len(page) < pageLimit {
			s.proactiveCursor.Store(0)
			return
		}
	}
}

func (s *GrokFreeRecoveryService) runLegacyCycle(ctx context.Context) {

	accounts, err := s.accountStore.ListByPlatform(ctx, PlatformGrok)
	if err != nil {
		slog.Warn("grok_free_recovery_list_failed", "error", err)
		return
	}
	now := s.now()
	candidates := make([]Account, 0, len(accounts))
	proactiveAccounts := make([]Account, 0, len(accounts))
	proactiveSelected := make([]Account, 0, len(accounts))
	for i := range accounts {
		if grokFreeRecoveryCandidate(&accounts[i], now) {
			candidates = append(candidates, accounts[i])
			continue
		}
		if grokFreeProactiveUsageCandidate(&accounts[i], now) {
			proactiveAccounts = append(proactiveAccounts, accounts[i])
		}
	}
	if usageReader, ok := s.prober.(grokFreeRecoveryUsageReader); ok && len(proactiveAccounts) > 0 {
		accountIDs := make([]int64, len(proactiveAccounts))
		for i := range proactiveAccounts {
			accountIDs[i] = proactiveAccounts[i].ID
		}
		usageByAccount, usageErr := usageReader.grokFreeRollingUsage(ctx, accountIDs, now)
		if usageErr != nil {
			slog.Warn("grok_free_proactive_usage_query_failed", "error", usageErr)
		} else {
			for i := range proactiveAccounts {
				usage := usageByAccount[proactiveAccounts[i].ID]
				if usage != nil && usage.Tokens >= grokFreeProactiveProbeTokenThreshold {
					proactiveSelected = append(proactiveSelected, proactiveAccounts[i])
				}
			}
		}
	}
	// Pending accounts always precede proactive candidates in the fallback
	// path used by lightweight tests and non-SQL repository implementations.
	sortGrokFreeRecoveryCandidates(candidates)
	sortGrokFreeRecoveryCandidates(proactiveSelected)
	candidates = append(candidates, proactiveSelected...)
	s.probeCandidates(ctx, candidates, false)
}

func (s *GrokFreeRecoveryService) effectiveProbeInterval() time.Duration {
	if s.probeInterval > 0 {
		return s.probeInterval
	}
	return grokFreeRecoveryProbeInterval
}

func (s *GrokFreeRecoveryService) effectiveLeaseDuration() time.Duration {
	leaseDuration := 2 * s.effectiveProbeInterval()
	if leaseDuration < grokFreeRecoveryLeaseDuration {
		return grokFreeRecoveryLeaseDuration
	}
	return leaseDuration
}

// grokFreeRollingUsage reuses the rolling 24-hour usage source shown by the
// account quota UI. Production uses the batch reader when available so a
// large Grok pool does not turn each scan into an N+1 query burst.
func (s *GrokQuotaService) grokFreeRollingUsage(
	ctx context.Context,
	accountIDs []int64,
	now time.Time,
) (map[int64]*WindowStats, error) {
	usageByAccount := make(map[int64]*WindowStats, len(accountIDs))
	if s == nil || s.usageLogRepo == nil || len(accountIDs) == 0 {
		return usageByAccount, nil
	}
	windowStart := now.UTC().Add(-grokFreeQuotaWindow)
	if batchReader, ok := s.usageLogRepo.(accountWindowStatsBatchReader); ok {
		statsByAccount, err := batchReader.GetAccountWindowStatsBatch(ctx, accountIDs, windowStart)
		if err != nil {
			return nil, err
		}
		for accountID, stats := range statsByAccount {
			if usage := windowStatsFromAccountStats(stats); usage != nil {
				usageByAccount[accountID] = usage
			}
		}
		return usageByAccount, nil
	}
	for _, accountID := range accountIDs {
		stats, err := s.usageLogRepo.GetAccountWindowStats(ctx, accountID, windowStart)
		if err != nil {
			return nil, err
		}
		if usage := windowStatsFromAccountStats(stats); usage != nil {
			usageByAccount[accountID] = usage
		}
	}
	return usageByAccount, nil
}

func sortGrokFreeRecoveryCandidates(accounts []Account) {
	sort.SliceStable(accounts, func(i, j int) bool {
		left := accounts[i].GrokFreeRecoveryNextProbeAt()
		right := accounts[j].GrokFreeRecoveryNextProbeAt()
		switch {
		case left.Equal(right):
			return accounts[i].ID < accounts[j].ID
		case left.IsZero():
			return true
		case right.IsZero():
			return false
		default:
			return left.Before(right)
		}
	})
}

func grokFreeRecoveryCandidate(account *Account, now time.Time) bool {
	if account == nil || !account.IsGrokFreeRecoveryPending() || !account.IsActive() || !account.Schedulable {
		return false
	}
	if account.AutoPauseOnExpired && account.ExpiresAt != nil && !now.Before(*account.ExpiresAt) {
		return false
	}
	nextProbeAt := account.GrokFreeRecoveryNextProbeAt()
	return nextProbeAt.IsZero() || !nextProbeAt.After(now)
}

func grokFreeProactiveUsageCandidate(account *Account, now time.Time) bool {
	if account == nil || account.IsGrokFreeRecoveryPending() || !account.IsGrokFreeOrUnknownOAuth() ||
		!account.IsActive() || !account.Schedulable {
		return false
	}
	if account.AutoPauseOnExpired && account.ExpiresAt != nil && !now.Before(*account.ExpiresAt) {
		return false
	}
	nextProbeAt := account.getExtraTime(GrokFreeProactiveNextProbeAtExtraKey)
	return nextProbeAt.IsZero() || !nextProbeAt.After(now)
}

func (s *GrokFreeRecoveryService) probeCandidates(ctx context.Context, accounts []Account, alreadyClaimed bool) {
	if len(accounts) == 0 {
		return
	}
	workerLimit := s.maxWorkers
	if workerLimit <= 0 {
		workerLimit = grokFreeRecoveryMaxWorkers
	}
	workerCount := min(workerLimit, len(accounts))
	jobs := make(chan Account)
	var workers sync.WaitGroup
	workers.Add(workerCount)
	for range workerCount {
		go func() {
			defer workers.Done()
			for account := range jobs {
				s.probeOne(ctx, &account, alreadyClaimed)
			}
		}()
	}
	for i := range accounts {
		select {
		case <-ctx.Done():
			close(jobs)
			workers.Wait()
			return
		case jobs <- accounts[i]:
		}
	}
	close(jobs)
	workers.Wait()
}

func (s *GrokFreeRecoveryService) probeOne(ctx context.Context, account *Account, alreadyClaimed bool) {
	if account == nil {
		return
	}
	now := s.now()
	// claimedNextProbeAt is the claim-generation CAS token written by the
	// atomic claim (or the legacy latch path). Unsuccessful probes keep that
	// token for CAS, then rewrite next_probe_at with exponential backoff.
	claimedNextProbeAt := account.GrokFreeRecoveryNextProbeAt()
	if !alreadyClaimed || claimedNextProbeAt.IsZero() {
		claimedNextProbeAt = now.Add(s.effectiveProbeInterval())
	}
	if !alreadyClaimed {
		leaseUntil := now.Add(s.effectiveLeaseDuration())
		if err := s.accountStore.UpdateExtra(ctx, account.ID, map[string]any{
			GrokFreeRecoveryPendingExtraKey:         true,
			GrokFreeRecoveryNextProbeAtExtraKey:     claimedNextProbeAt.UTC().Format(time.RFC3339Nano),
			GrokFreeRecoveryLastProbeAtExtraKey:     now.UTC().Format(time.RFC3339Nano),
			GrokFreeRecoveryLastProbeResultExtraKey: "running",
			GrokFreeProactiveNextProbeAtExtraKey:    claimedNextProbeAt.UTC().Format(time.RFC3339Nano),
		}); err != nil {
			s.metrics.failed.Add(1)
			slog.Warn("grok_free_recovery_schedule_probe_failed", "account_id", account.ID, "error", err)
			return
		}
		var err error
		if extender, ok := s.accountStore.(grokFreeRecoveryRateLimitExtender); ok {
			err = extender.SetRateLimitedIfLater(ctx, account.ID, leaseUntil)
		} else {
			err = s.accountStore.SetRateLimited(ctx, account.ID, leaseUntil)
		}
		if err != nil {
			s.metrics.failed.Add(1)
			slog.Warn("grok_free_recovery_rearm_failed", "account_id", account.ID, "error", err)
			return
		}
	}

	probeStartedAt := s.now()
	s.metrics.probes.Add(1)
	result, err := s.prober.probeUsage(ctx, account.ID)
	if err != nil {
		s.metrics.failed.Add(1)
		s.recordUnsuccessfulProbe(ctx, account, claimedNextProbeAt, "transport_error")
		slog.Warn("grok_free_recovery_probe_failed", "account_id", account.ID, "error", err)
		return
	}
	if result == nil {
		s.metrics.failed.Add(1)
		s.recordUnsuccessfulProbe(ctx, account, claimedNextProbeAt, "empty_result")
		return
	}
	if result.StatusCode < 200 || result.StatusCode >= 300 {
		s.metrics.limited.Add(1)
		s.recordUnsuccessfulProbe(ctx, account, claimedNextProbeAt, fmt.Sprintf("http_%d", result.StatusCode))
		return
	}
	if _, limited := grokRateLimitResetAt(result.Snapshot, s.now()); limited {
		s.metrics.limited.Add(1)
		s.recordUnsuccessfulProbe(ctx, account, claimedNextProbeAt, "quota_exhausted")
		return
	}
	if !s.recordProbeResult(ctx, account.ID, claimedNextProbeAt, "healthy") {
		s.metrics.casRejected.Add(1)
		return
	}

	recovered, err := s.recoverer.RecoverGrokFreeAfterSuccessfulProbe(ctx, account.ID, probeStartedAt, claimedNextProbeAt)
	if err != nil {
		s.metrics.failed.Add(1)
		slog.Warn("grok_free_recovery_clear_failed", "account_id", account.ID, "error", err)
		return
	}
	if !recovered {
		s.metrics.casRejected.Add(1)
		slog.Debug("grok_free_recovery_state_changed_during_probe", "account_id", account.ID)
		return
	}
	s.metrics.healthy.Add(1)
	slog.Info("grok_free_recovery_succeeded", "account_id", account.ID)
}

// grokFreeRecoveryBackoff returns how long to wait before the next probe after
// streak consecutive unsuccessful probes. streak is 1-based.
//
//	streak 1 → base
//	streak 2 → 2*base
//	streak 3 → 4*base
//	...
//
// capped at grokFreeRecoveryMaxBackoff so Free daily resets still get probed.
func grokFreeRecoveryBackoff(streak int, base time.Duration) time.Duration {
	if base <= 0 {
		base = grokFreeRecoveryProbeInterval
	}
	if streak <= 1 {
		return base
	}
	// base * 2^(streak-1) without float; stop once we hit the cap.
	delay := base
	for i := 1; i < streak; i++ {
		if delay >= grokFreeRecoveryMaxBackoff/2 {
			return grokFreeRecoveryMaxBackoff
		}
		delay *= 2
	}
	if delay > grokFreeRecoveryMaxBackoff {
		return grokFreeRecoveryMaxBackoff
	}
	return delay
}

func (s *GrokFreeRecoveryService) recordUnsuccessfulProbe(
	ctx context.Context,
	account *Account,
	claimedNextProbeAt time.Time,
	result string,
) {
	if account == nil {
		return
	}
	if !s.recordProbeResult(ctx, account.ID, claimedNextProbeAt, result) {
		s.metrics.casRejected.Add(1)
		return
	}
	// CAS accepted: this probe generation still owns the latch. Stretch the
	// next probe using the limited streak so a large Free pool that remains
	// exhausted does not re-enter the claim queue every base interval.
	streak := account.GrokFreeRecoveryLimitedStreak() + 1
	delay := grokFreeRecoveryBackoff(streak, s.effectiveProbeInterval())
	now := s.now()
	nextProbeAt := now.Add(delay)
	// Keep rate-limit coverage until at least the next probe (and never shorter
	// than the base lease) so schedulers cannot re-select a latched account.
	leaseUntil := nextProbeAt
	if minLease := now.Add(s.effectiveLeaseDuration()); minLease.After(leaseUntil) {
		leaseUntil = minLease
	}
	if err := s.accountStore.UpdateExtra(ctx, account.ID, map[string]any{
		GrokFreeRecoveryNextProbeAtExtraKey:     nextProbeAt.UTC().Format(time.RFC3339Nano),
		GrokFreeRecoveryLimitedStreakExtraKey:   streak,
		GrokFreeProactiveNextProbeAtExtraKey:    nextProbeAt.UTC().Format(time.RFC3339Nano),
		GrokFreeRecoveryLastProbeResultExtraKey: result,
	}); err != nil {
		slog.Warn("grok_free_recovery_backoff_reschedule_failed", "account_id", account.ID, "error", err)
		return
	}
	var err error
	if extender, ok := s.accountStore.(grokFreeRecoveryRateLimitExtender); ok {
		err = extender.SetRateLimitedIfLater(ctx, account.ID, leaseUntil)
	} else {
		err = s.accountStore.SetRateLimited(ctx, account.ID, leaseUntil)
	}
	if err != nil {
		slog.Warn("grok_free_recovery_backoff_lease_failed", "account_id", account.ID, "error", err)
	}
}

func (s *GrokFreeRecoveryService) recordProbeResult(
	ctx context.Context,
	accountID int64,
	nextProbeAt time.Time,
	result string,
) bool {
	completedAt := s.now()
	if recorder, ok := s.accountStore.(grokFreeRecoveryResultRecorder); ok {
		recorded, err := recorder.RecordGrokFreeRecoveryProbeResult(ctx, accountID, nextProbeAt, result, completedAt)
		if err != nil {
			slog.Warn("grok_free_recovery_record_result_failed", "account_id", accountID, "error", err)
			return false
		}
		return recorded
	}
	if err := s.accountStore.UpdateExtra(ctx, accountID, map[string]any{
		GrokFreeRecoveryLastProbeResultExtraKey: result,
		GrokFreeRecoveryLastResultAtExtraKey:    completedAt.UTC().Format(time.RFC3339Nano),
	}); err != nil {
		slog.Warn("grok_free_recovery_record_result_failed", "account_id", accountID, "error", err)
		return false
	}
	return true
}
