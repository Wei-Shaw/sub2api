package service

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	grokActiveProbeDefaultInterval   = 15 * time.Minute
	grokActiveProbeScanInterval      = 30 * time.Second
	grokActiveProbeConcurrency       = 4
	grokActiveProbeDefaultMaxPerScan = 180
	grokActiveProbeMaxPerScanLimit   = grokActiveProbeDefaultMaxPerScan
	grokActiveProbeAccountTimeout    = 45 * time.Second
	grokActiveProbeRunTimeout        = 40 * time.Minute

	grokActiveProbeEnabledEnv     = "GROK_ACTIVE_PROBE_ENABLED"
	grokActiveProbeIntervalEnv    = "GROK_ACTIVE_PROBE_INTERVAL"
	grokActiveProbeMaxPerScanEnv  = "GROK_ACTIVE_PROBE_MAX_PER_SCAN"
	grokActiveProbeRunLockKey     = "grok:active-probe:run"
	grokActiveProbeCadenceLockKey = "grok:active-probe:cadence"
	grokActiveProbeRunLockTTL     = 45 * time.Minute
	grokActiveProbeCadenceLockTTL = 2 * grokActiveProbeScanInterval
)

type grokActiveProbeAccountLister interface {
	ListByPlatform(ctx context.Context, platform string) ([]Account, error)
}

type grokActiveProbeProber interface {
	probeUsage(ctx context.Context, accountID int64) (*GrokQuotaProbeResult, error)
}

type grokActiveProbeCandidate struct {
	account *Account
	score   uint64
}

// GrokActiveProbeService periodically verifies eligible Grok OAuth accounts
// and lets GrokQuotaService persist the observed upstream scheduling state.
type GrokActiveProbeService struct {
	accountLister grokActiveProbeAccountLister
	prober        grokActiveProbeProber

	parentCtx    context.Context
	parentCancel context.CancelFunc
	wg           sync.WaitGroup
	lifecycleMu  sync.Mutex
	cycleMu      sync.Mutex
	started      bool
	stopped      bool

	attemptMu      sync.Mutex
	lastAttempts   map[int64]time.Time
	initialDueAt   map[int64]time.Time
	interval       time.Duration
	maxPerScan     int
	accountTimeout time.Duration
	runTimeout     time.Duration
	runLockTTL     time.Duration
	enabled        bool
	now            func() time.Time
	lockCache      LeaderLockCache
	db             *sql.DB
	instanceID     string
}

func NewGrokActiveProbeService(
	accountRepo AccountRepository,
	quotaService *GrokQuotaService,
) *GrokActiveProbeService {
	return newGrokActiveProbeService(accountRepo, quotaService)
}

func newGrokActiveProbeService(
	accountLister grokActiveProbeAccountLister,
	prober grokActiveProbeProber,
) *GrokActiveProbeService {
	ctx, cancel := context.WithCancel(context.Background())
	return &GrokActiveProbeService{
		accountLister:  accountLister,
		prober:         prober,
		parentCtx:      ctx,
		parentCancel:   cancel,
		lastAttempts:   make(map[int64]time.Time),
		initialDueAt:   make(map[int64]time.Time),
		interval:       grokProbeIntervalFromEnv(grokActiveProbeIntervalEnv, grokActiveProbeDefaultInterval),
		maxPerScan:     grokProbeMaxPerScanFromEnv(),
		accountTimeout: grokActiveProbeAccountTimeout,
		runTimeout:     grokActiveProbeRunTimeout,
		runLockTTL:     grokActiveProbeRunLockTTL,
		enabled:        grokProbeEnabledFromEnv(),
		now:            time.Now,
		instanceID:     uuid.NewString(),
	}
}

func (s *GrokActiveProbeService) SetLeaderLock(lockCache LeaderLockCache, db *sql.DB) {
	if s == nil {
		return
	}
	s.lockCache = lockCache
	s.db = db
}

func (s *GrokActiveProbeService) Start() {
	if s == nil || !s.enabled {
		return
	}
	s.lifecycleMu.Lock()
	if s.started || s.stopped {
		s.lifecycleMu.Unlock()
		return
	}
	s.started = true
	s.wg.Add(1)
	s.lifecycleMu.Unlock()

	go s.runLoop()
}

func (s *GrokActiveProbeService) Stop() {
	if s == nil {
		return
	}
	s.lifecycleMu.Lock()
	if s.stopped {
		s.lifecycleMu.Unlock()
		return
	}
	s.stopped = true
	s.parentCancel()
	s.lifecycleMu.Unlock()
	s.wg.Wait()
}

func (s *GrokActiveProbeService) runLoop() {
	defer s.wg.Done()
	if err := s.runOnce(s.parentCtx, true); err != nil && s.parentCtx.Err() == nil {
		slog.Warn("grok_active_probe_scan_failed", "error", err)
	}

	ticker := time.NewTicker(grokActiveProbeScanInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.parentCtx.Done():
			return
		case <-ticker.C:
			if err := s.runOnce(s.parentCtx, true); err != nil && s.parentCtx.Err() == nil {
				slog.Warn("grok_active_probe_scan_failed", "error", err)
			}
		}
	}
}

// RunOnce runs one bounded health-check cycle. The background loop additionally
// paces dispatches across each scan window to avoid request bursts.
func (s *GrokActiveProbeService) RunOnce(ctx context.Context) error {
	return s.runOnce(ctx, false)
}

func (s *GrokActiveProbeService) runOnce(ctx context.Context, paced bool) error {
	if s == nil || s.accountLister == nil || s.prober == nil {
		return nil
	}
	s.cycleMu.Lock()
	defer s.cycleMu.Unlock()

	now := s.currentTime()
	runRelease, acquired := tryAcquireSingletonLeaderLock(
		ctx, s.lockCache, s.db, grokActiveProbeRunLockKey, s.instanceID, s.runLockTTL,
	)
	if !acquired {
		return nil
	}
	defer runRelease()

	runCtx, cancelRun := context.WithTimeout(ctx, s.runTimeout)
	defer cancelRun()
	ctx = runCtx
	lockKey := fmt.Sprintf("%s:%d", grokActiveProbeCadenceLockKey, now.Unix()/int64(grokActiveProbeScanInterval/time.Second))
	release, acquired := tryAcquireSingletonLeaderLock(
		ctx, s.lockCache, s.db, lockKey, s.instanceID, grokActiveProbeCadenceLockTTL,
	)
	if !acquired {
		return nil
	}
	if paced {
		defer releaseGrokActiveProbeLeaderLock(release, now.Truncate(grokActiveProbeScanInterval).Add(grokActiveProbeScanInterval))
	} else {
		defer release()
	}

	accounts, err := s.accountLister.ListByPlatform(ctx, PlatformGrok)
	if err != nil {
		return err
	}
	scanBucket := uint64(now.UnixNano() / int64(grokActiveProbeScanInterval))
	due := make([]grokActiveProbeCandidate, 0, len(accounts))
	for i := range accounts {
		account := &accounts[i]
		if !grokActiveProbeAccountEnabled(account, now) || !s.probeDue(account, now) {
			continue
		}
		due = append(due, grokActiveProbeCandidate{
			account: account,
			score:   grokProbeOrderScore(account.ID, scanBucket),
		})
	}
	if len(due) == 0 {
		return nil
	}
	if len(due) > s.maxPerScan {
		slog.Warn("grok_active_probe_backlog", "due", len(due), "max_per_scan", s.maxPerScan)
	}
	sort.Slice(due, func(i, j int) bool {
		if due[i].score != due[j].score {
			return due[i].score < due[j].score
		}
		return due[i].account.ID < due[j].account.ID
	})
	if len(due) > s.maxPerScan {
		due = due[:s.maxPerScan]
	}

	accountIDs := make([]int64, 0, len(due))
	for i := range due {
		if s.reserveProbe(due[i].account, now) {
			accountIDs = append(accountIDs, due[i].account.ID)
		}
	}
	if len(accountIDs) == 0 {
		return nil
	}

	workerCount := grokActiveProbeConcurrency
	if len(accountIDs) < workerCount {
		workerCount = len(accountIDs)
	}
	jobs := make(chan int64)
	var workers sync.WaitGroup
	workers.Add(workerCount)
	for range workerCount {
		go func() {
			defer workers.Done()
			for accountID := range jobs {
				probeCtx, cancelProbe := context.WithTimeout(ctx, s.accountTimeout)
				s.runProbe(probeCtx, accountID)
				cancelProbe()
			}
		}()
	}

enqueue:
	for index, accountID := range accountIDs {
		if paced && index > 0 {
			pace := grokActiveProbeScanInterval / time.Duration(maxInt(s.maxPerScan, 1))
			timer := time.NewTimer(pace)
			select {
			case <-ctx.Done():
				timer.Stop()
				break enqueue
			case <-timer.C:
			}
		}
		select {
		case <-ctx.Done():
			break enqueue
		case jobs <- accountID:
		}
	}
	close(jobs)
	workers.Wait()
	return nil
}

func (s *GrokActiveProbeService) runProbe(ctx context.Context, accountID int64) {
	defer func() {
		if recovered := recover(); recovered != nil {
			slog.Error("grok_active_probe_panic", "account_id", accountID, "panic", recovered)
		}
	}()

	result, probeErr := s.prober.probeUsage(ctx, accountID)
	if probeErr != nil {
		statusCode := 0
		if result != nil {
			statusCode = result.StatusCode
		}
		slog.Warn("grok_active_probe_failed", "account_id", accountID, "status", statusCode, "error", probeErr)
		return
	}
	if result != nil && result.StatusCode == http.StatusTooManyRequests {
		slog.Info("grok_active_probe_rate_limited", "account_id", accountID)
	}
}

func grokActiveProbeAccountEnabled(account *Account, now time.Time) bool {
	if account == nil || !account.IsGrokOAuth() || !account.IsActive() || !account.Schedulable {
		return false
	}
	if account.TempUnschedulableUntil != nil && now.Before(*account.TempUnschedulableUntil) {
		return false
	}
	if account.RateLimitResetAt != nil && now.Before(*account.RateLimitResetAt) {
		return false
	}
	if account.OverloadUntil != nil && now.Before(*account.OverloadUntil) {
		return false
	}
	return !account.AutoPauseOnExpired || account.ExpiresAt == nil || now.Before(*account.ExpiresAt)
}

func (s *GrokActiveProbeService) probeDue(account *Account, now time.Time) bool {
	if account == nil || account.ID <= 0 || s.interval <= 0 {
		return false
	}
	lastAttempt := grokActiveProbePersistedAt(account)
	s.attemptMu.Lock()
	defer s.attemptMu.Unlock()
	if inMemory := s.lastAttempts[account.ID]; inMemory.After(lastAttempt) {
		lastAttempt = inMemory
	}
	if lastAttempt.IsZero() {
		dueAt, exists := s.initialDueAt[account.ID]
		if !exists {
			delay := time.Duration(grokProbeMix64(uint64(account.ID)) % uint64(s.interval))
			dueAt = now.Add(delay)
			s.initialDueAt[account.ID] = dueAt
		}
		return !now.Before(dueAt)
	}
	return !now.Before(lastAttempt.Add(s.interval))
}

func (s *GrokActiveProbeService) reserveProbe(account *Account, now time.Time) bool {
	if !s.probeDue(account, now) {
		return false
	}
	s.attemptMu.Lock()
	defer s.attemptMu.Unlock()
	s.lastAttempts[account.ID] = now
	delete(s.initialDueAt, account.ID)
	return true
}

func grokProbeOrderScore(accountID int64, scanBucket uint64) uint64 {
	return grokProbeMix64(uint64(accountID) ^ grokProbeMix64(scanBucket))
}

func grokProbeMix64(value uint64) uint64 {
	value ^= value >> 30
	value *= 0xbf58476d1ce4e5b9
	value ^= value >> 27
	value *= 0x94d049bb133111eb
	return value ^ (value >> 31)
}

func releaseGrokActiveProbeLeaderLock(release func(), releaseAt time.Time) {
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

func grokProbeIntervalFromEnv(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed < grokActiveProbeDefaultInterval {
		slog.Warn("grok_active_probe_invalid_interval", "key", key, "value", value, "fallback", fallback)
		return fallback
	}
	return parsed
}

func grokProbeMaxPerScanFromEnv() int {
	value := strings.TrimSpace(os.Getenv(grokActiveProbeMaxPerScanEnv))
	if value == "" {
		return grokActiveProbeDefaultMaxPerScan
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 || parsed > grokActiveProbeMaxPerScanLimit {
		slog.Warn("grok_active_probe_invalid_max_per_scan", "value", value, "fallback", grokActiveProbeDefaultMaxPerScan)
		return grokActiveProbeDefaultMaxPerScan
	}
	return parsed
}

func grokProbeEnabledFromEnv() bool {
	value := strings.TrimSpace(os.Getenv(grokActiveProbeEnabledEnv))
	if value == "" {
		return true
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		slog.Warn("grok_active_probe_invalid_enabled", "value", value, "fallback", true)
		return true
	}
	return parsed
}

func grokActiveProbePersistedAt(account *Account) time.Time {
	if account == nil {
		return time.Time{}
	}
	snapshot, err := grokQuotaSnapshotFromExtra(account.Extra)
	if err != nil || snapshot == nil {
		return time.Time{}
	}
	for _, value := range []string{snapshot.LastProbeAt, snapshot.UpdatedAt} {
		if probedAt, parseErr := parseTime(value); parseErr == nil {
			return probedAt
		}
	}
	return time.Time{}
}

func (s *GrokActiveProbeService) currentTime() time.Time {
	if s != nil && s.now != nil {
		return s.now()
	}
	return time.Now()
}
