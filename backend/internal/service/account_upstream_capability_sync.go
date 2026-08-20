package service

import (
	"context"
	"database/sql"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	upstreamCapabilitySyncInterval      = time.Hour
	upstreamCapabilitySyncCycle         = 2 * time.Minute
	upstreamCapabilitySyncTimeout       = 45 * time.Second
	upstreamCapabilitySyncMaxPerCycle   = 15
	upstreamCapabilitySyncConcurrency   = 3
	upstreamCapabilitySyncLeaderLockKey = "upstream:capability:sync:leader"
	upstreamCapabilitySyncLeaderLockTTL = 10 * time.Minute
)

type upstreamCapabilityAccountLister interface {
	ListActive(ctx context.Context) ([]Account, error)
}

type upstreamModelSnapshotStore interface {
	FetchUpstreamSupportedModels(ctx context.Context, account *Account) ([]string, error)
	StoreUpstreamModelSnapshot(ctx context.Context, account *Account, models []string) error
}

// UpstreamCapabilitySyncService periodically refreshes each account's last
// successful upstream model list. Failures keep the old snapshot.
type UpstreamCapabilitySyncService struct {
	accounts  upstreamCapabilityAccountLister
	store     upstreamModelSnapshotStore
	lockCache LeaderLockCache
	db        *sql.DB

	interval    time.Duration
	cycle       time.Duration
	maxPerCycle int
	now         func() time.Time
	instanceID  string

	parentCtx    context.Context
	parentCancel context.CancelFunc
	wg           sync.WaitGroup
	mu           sync.Mutex
	started      bool
	stopped      bool
}

func NewUpstreamCapabilitySyncService(
	accounts upstreamCapabilityAccountLister,
	store upstreamModelSnapshotStore,
) *UpstreamCapabilitySyncService {
	ctx, cancel := context.WithCancel(context.Background())
	return &UpstreamCapabilitySyncService{
		accounts:     accounts,
		store:        store,
		interval:     upstreamCapabilitySyncInterval,
		cycle:        upstreamCapabilitySyncCycle,
		maxPerCycle:  upstreamCapabilitySyncMaxPerCycle,
		now:          time.Now,
		instanceID:   uuid.NewString(),
		parentCtx:    ctx,
		parentCancel: cancel,
	}
}

func (s *UpstreamCapabilitySyncService) SetLeaderLock(lockCache LeaderLockCache, db *sql.DB) {
	if s == nil {
		return
	}
	s.lockCache = lockCache
	s.db = db
}

func ProvideUpstreamCapabilitySyncService(
	accountRepo AccountRepository,
	accountTestService *AccountTestService,
	lockCache LeaderLockCache,
	db *sql.DB,
) *UpstreamCapabilitySyncService {
	svc := NewUpstreamCapabilitySyncService(accountRepo, accountTestService)
	svc.SetLeaderLock(lockCache, db)
	startProcessBackground("upstream_capability_sync", svc.Start)
	return svc
}

func (s *UpstreamCapabilitySyncService) Start() {
	if s == nil {
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
}

func (s *UpstreamCapabilitySyncService) Stop() {
	if s == nil {
		return
	}
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	s.stopped = true
	s.parentCancel()
	s.mu.Unlock()
	s.wg.Wait()
}

func (s *UpstreamCapabilitySyncService) runLoop() {
	defer s.wg.Done()
	_ = s.RunDue(s.parentCtx)
	ticker := time.NewTicker(s.cycleDuration())
	defer ticker.Stop()
	for {
		select {
		case <-s.parentCtx.Done():
			return
		case <-ticker.C:
			if err := s.RunDue(s.parentCtx); err != nil {
				slog.Warn("upstream_capability_sync_run_due_failed", "error", err)
			}
		}
	}
}

func (s *UpstreamCapabilitySyncService) cycleDuration() time.Duration {
	if s != nil && s.cycle > 0 {
		return s.cycle
	}
	return upstreamCapabilitySyncCycle
}

// RunDue refreshes a bounded batch of stale or never-synced accounts.
func (s *UpstreamCapabilitySyncService) RunDue(ctx context.Context) error {
	if s == nil || s.accounts == nil || s.store == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	release, acquired := tryAcquireSingletonLeaderLock(
		ctx,
		s.lockCache,
		s.db,
		upstreamCapabilitySyncLeaderLockKey,
		s.instanceID,
		upstreamCapabilitySyncLeaderLockTTL,
	)
	if !acquired {
		return nil
	}
	defer release()

	listCtx, cancel := context.WithTimeout(ctx, upstreamCapabilitySyncTimeout)
	accounts, err := s.accounts.ListActive(listCtx)
	cancel()
	if err != nil {
		return err
	}

	due := selectDueUpstreamCapabilityAccounts(accounts, s.currentTime(), s.freshnessInterval(), s.batchLimit())
	if len(due) == 0 {
		return nil
	}

	slots := make(chan struct{}, upstreamCapabilitySyncConcurrency)
	var wg sync.WaitGroup
	for i := range due {
		account := due[i]
		if ctx.Err() != nil {
			break
		}
		slots <- struct{}{}
		wg.Add(1)
		go func(account Account) {
			defer wg.Done()
			defer func() { <-slots }()
			s.syncAccount(ctx, &account)
		}(account)
	}
	wg.Wait()
	return ctx.Err()
}

func (s *UpstreamCapabilitySyncService) syncAccount(ctx context.Context, account *Account) {
	if account == nil {
		return
	}
	syncCtx, cancel := context.WithTimeout(ctx, upstreamCapabilitySyncTimeout)
	defer cancel()

	models, err := s.store.FetchUpstreamSupportedModels(syncCtx, account)
	if err != nil {
		slog.Warn("upstream_capability_sync_fetch_failed",
			"account_id", account.ID,
			"platform", account.Platform,
			"error", err,
		)
		return
	}
	if err := s.store.StoreUpstreamModelSnapshot(syncCtx, account, models); err != nil {
		slog.Warn("upstream_capability_sync_store_failed",
			"account_id", account.ID,
			"platform", account.Platform,
			"error", err,
		)
	}
}

func (s *UpstreamCapabilitySyncService) currentTime() time.Time {
	if s != nil && s.now != nil {
		return s.now()
	}
	return time.Now()
}

func (s *UpstreamCapabilitySyncService) freshnessInterval() time.Duration {
	if s != nil && s.interval > 0 {
		return s.interval
	}
	return upstreamCapabilitySyncInterval
}

func (s *UpstreamCapabilitySyncService) batchLimit() int {
	if s != nil && s.maxPerCycle > 0 {
		return s.maxPerCycle
	}
	return upstreamCapabilitySyncMaxPerCycle
}

func selectDueUpstreamCapabilityAccounts(accounts []Account, now time.Time, interval time.Duration, limit int) []Account {
	if limit <= 0 {
		return nil
	}
	type ranked struct {
		account  Account
		syncedAt time.Time
	}
	due := make([]ranked, 0, len(accounts))
	for _, account := range accounts {
		if !supportsUpstreamModelSync(&account) {
			continue
		}
		snapshot := account.UpstreamModelSnapshot()
		syncedAt := snapshotSyncedAt(snapshot)
		if !syncedAt.IsZero() && interval > 0 && now.Sub(syncedAt) < interval {
			continue
		}
		due = append(due, ranked{account: account, syncedAt: syncedAt})
	}
	sort.SliceStable(due, func(i, j int) bool {
		if due[i].syncedAt.IsZero() != due[j].syncedAt.IsZero() {
			return due[i].syncedAt.IsZero()
		}
		if !due[i].syncedAt.Equal(due[j].syncedAt) {
			return due[i].syncedAt.Before(due[j].syncedAt)
		}
		return due[i].account.ID < due[j].account.ID
	})
	if len(due) > limit {
		due = due[:limit]
	}
	out := make([]Account, 0, len(due))
	for _, item := range due {
		out = append(out, item.account)
	}
	return out
}
