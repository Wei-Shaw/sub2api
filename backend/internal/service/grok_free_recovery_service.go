package service

import (
	"context"
	"database/sql"
	"log/slog"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

const (
	grokFreeRecoveryScanInterval         = time.Minute
	grokFreeRecoveryProbeInterval        = 5 * time.Minute
	grokFreeRecoveryLeaseDuration        = 10 * time.Minute
	grokFreeRecoveryRunTimeout           = 4 * time.Minute
	grokFreeRecoveryLeaderLockTTL        = 5 * time.Minute
	grokFreeRecoveryMaxWorkers           = 3
	grokFreeRecoveryLeaderLockKey        = "sub2api:grok-free-recovery"
	grokFreeProactiveProbeTokenThreshold = grokFreeRolling24hTokenLimit * 99 / 100

	// Successful recovery clears the latch keys; this timestamp prevents a
	// local 2M estimate from causing another probe on every scan cycle.
	grokFreeProactiveNextProbeAtExtraKey = "grok_free_proactive_next_probe_at"
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

	scanInterval time.Duration
	now          func() time.Time
	running      atomic.Bool
	startOnce    sync.Once
	stopOnce     sync.Once
	cancel       context.CancelFunc
	wg           sync.WaitGroup
}

func newGrokFreeRecoveryService(
	accountStore grokFreeRecoveryAccountStore,
	prober grokFreeRecoveryProber,
	recoverer grokFreeRecoveryStateRecoverer,
	lockCache LeaderLockCache,
	db *sql.DB,
) *GrokFreeRecoveryService {
	return &GrokFreeRecoveryService{
		accountStore: accountStore,
		prober:       prober,
		recoverer:    recoverer,
		lockCache:    lockCache,
		db:           db,
		instanceID:   uuid.NewString(),
		scanInterval: grokFreeRecoveryScanInterval,
		now:          time.Now,
	}
}

func (s *GrokFreeRecoveryService) Start() {
	if s == nil {
		return
	}
	s.startOnce.Do(func() {
		ctx, cancel := context.WithCancel(context.Background())
		s.cancel = cancel
		s.wg.Add(1)
		go s.run(ctx)
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
	if s == nil || s.accountStore == nil || s.prober == nil || s.recoverer == nil {
		return
	}
	if !s.running.CompareAndSwap(false, true) {
		return
	}
	defer s.running.Store(false)

	ctx, cancel := context.WithTimeout(parent, grokFreeRecoveryRunTimeout)
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
		return
	}
	if release != nil {
		defer release()
	}

	accounts, err := s.accountStore.ListByPlatform(ctx, PlatformGrok)
	if err != nil {
		slog.Warn("grok_free_recovery_list_failed", "error", err)
		return
	}
	now := s.now()
	candidates := make([]Account, 0, len(accounts))
	proactiveAccounts := make([]Account, 0, len(accounts))
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
					candidates = append(candidates, proactiveAccounts[i])
				}
			}
		}
	}
	sortGrokFreeRecoveryCandidates(candidates)
	s.probeCandidates(ctx, candidates)
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
	nextProbeAt := account.getExtraTime(grokFreeProactiveNextProbeAtExtraKey)
	return nextProbeAt.IsZero() || !nextProbeAt.After(now)
}

func (s *GrokFreeRecoveryService) probeCandidates(ctx context.Context, accounts []Account) {
	if len(accounts) == 0 {
		return
	}
	workerCount := min(grokFreeRecoveryMaxWorkers, len(accounts))
	jobs := make(chan Account)
	var workers sync.WaitGroup
	workers.Add(workerCount)
	for range workerCount {
		go func() {
			defer workers.Done()
			for account := range jobs {
				s.probeOne(ctx, &account)
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

func (s *GrokFreeRecoveryService) probeOne(ctx context.Context, account *Account) {
	if account == nil {
		return
	}
	now := s.now()
	leaseUntil := now.Add(grokFreeRecoveryLeaseDuration)
	nextProbeAt := now.Add(grokFreeRecoveryProbeInterval)
	var err error
	if extender, ok := s.accountStore.(grokFreeRecoveryRateLimitExtender); ok {
		err = extender.SetRateLimitedIfLater(ctx, account.ID, leaseUntil)
	} else {
		err = s.accountStore.SetRateLimited(ctx, account.ID, leaseUntil)
	}
	if err != nil {
		slog.Warn("grok_free_recovery_rearm_failed", "account_id", account.ID, "error", err)
		return
	}
	if err := s.accountStore.UpdateExtra(ctx, account.ID, map[string]any{
		GrokFreeRecoveryPendingExtraKey:      true,
		GrokFreeRecoveryNextProbeAtExtraKey:  nextProbeAt.UTC().Format(time.RFC3339Nano),
		GrokFreeRecoveryLastProbeAtExtraKey:  now.UTC().Format(time.RFC3339Nano),
		grokFreeProactiveNextProbeAtExtraKey: nextProbeAt.UTC().Format(time.RFC3339Nano),
	}); err != nil {
		slog.Warn("grok_free_recovery_schedule_probe_failed", "account_id", account.ID, "error", err)
		return
	}

	probeStartedAt := s.now()
	result, err := s.prober.probeUsage(ctx, account.ID)
	if err != nil {
		slog.Warn("grok_free_recovery_probe_failed", "account_id", account.ID, "error", err)
		return
	}
	if result == nil || result.StatusCode < 200 || result.StatusCode >= 300 {
		return
	}
	if _, limited := grokRateLimitResetAt(result.Snapshot, time.Now()); limited {
		return
	}

	recovered, err := s.recoverer.RecoverGrokFreeAfterSuccessfulProbe(ctx, account.ID, probeStartedAt, nextProbeAt)
	if err != nil {
		slog.Warn("grok_free_recovery_clear_failed", "account_id", account.ID, "error", err)
		return
	}
	if !recovered {
		slog.Debug("grok_free_recovery_state_changed_during_probe", "account_id", account.ID)
		return
	}
	slog.Info("grok_free_recovery_succeeded", "account_id", account.ID)
}
