package service

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

var (
	ErrSchedulerCacheNotReady    = errors.New("scheduler cache not ready")
	ErrSchedulerCacheUnavailable = errors.New("scheduler cache unavailable")
	ErrSchedulerFallbackLimited  = errors.New("scheduler db fallback limited")
	schedulerV2ActivationBucket  = SchedulerBucket{GroupID: -1, Platform: "engine", Mode: "activation"}
)

const (
	outboxEventTimeout          = 2 * time.Minute
	schedulerOutboxCleanupBatch = 5000
	schedulerV2FullRebuildFloor = 6 * time.Hour
)

// batchSeenKey tracks which (groupID, platform) bucket sets have already been
// rebuilt within a single pollOutbox call, to avoid redundant work when multiple
// account_changed events share the same groups.
type batchSeenKey struct {
	groupID  int64
	platform string
}

type SchedulerSnapshotService struct {
	cache            SchedulerCache
	outboxRepo       SchedulerOutboxRepository
	accountRepo      AccountRepository
	groupRepo        GroupRepository
	settingRepo      SettingRepository
	cfg              *config.Config
	stopCh           chan struct{}
	stopOnce         sync.Once
	wg               sync.WaitGroup
	fallbackLimit    *fallbackLimiter
	lagMu            sync.Mutex
	lagFailures      int
	engineMu         sync.RWMutex
	engineState      SchedulerEngineState
	engineRecoveryMu sync.Mutex
	v2LimitsMu       sync.RWMutex
	v2CandidateLimit int
	v2ScanLimit      int
	activationMu     sync.Mutex
	v2RebuildMu      sync.Mutex
	v2LastRebuild    time.Time
}

func NewSchedulerSnapshotService(
	cache SchedulerCache,
	outboxRepo SchedulerOutboxRepository,
	accountRepo AccountRepository,
	groupRepo GroupRepository,
	cfg *config.Config,
) *SchedulerSnapshotService {
	maxQPS := 0
	if cfg != nil {
		maxQPS = cfg.Gateway.Scheduling.DbFallbackMaxQPS
	}
	return &SchedulerSnapshotService{
		cache:            cache,
		outboxRepo:       outboxRepo,
		accountRepo:      accountRepo,
		groupRepo:        groupRepo,
		cfg:              cfg,
		stopCh:           make(chan struct{}),
		fallbackLimit:    newFallbackLimiter(maxQPS),
		v2CandidateLimit: DefaultSchedulerCandidateFetchLimit,
		v2ScanLimit:      DefaultSchedulerCandidateScanLimit,
		engineState: SchedulerEngineState{
			Engine: SchedulerEngineLegacy,
			Status: SchedulerEngineStatusDisabled,
		},
	}
}

func (s *SchedulerSnapshotService) Start() {
	if s == nil || s.cache == nil {
		return
	}

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.runInitialRebuild()
	}()

	interval := s.outboxPollInterval()
	if s.outboxRepo != nil && interval > 0 {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.runOutboxWorker(interval)
		}()
	}

	fullInterval := s.fullRebuildInterval()
	if fullInterval > 0 {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.runFullRebuildWorker(fullInterval)
		}()
	}
}

func (s *SchedulerSnapshotService) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		close(s.stopCh)
	})
	s.wg.Wait()
}

func (s *SchedulerSnapshotService) ListSchedulableAccounts(ctx context.Context, groupID *int64, platform string, hasForcePlatform bool) ([]Account, bool, error) {
	useMixed := (platform == PlatformAnthropic || platform == PlatformGemini) && !hasForcePlatform
	mode := s.resolveMode(platform, hasForcePlatform)
	bucket := s.bucketFor(groupID, platform, mode)
	if s.schedulerV2Enabled(ctx) {
		accounts, err := s.listSchedulerV2Accounts(ctx, bucket)
		return accounts, useMixed, err
	}

	if s.cache != nil {
		cached, hit, err := s.cache.GetSnapshot(ctx, bucket)
		if err != nil {
			logger.LegacyPrintf("service.scheduler_snapshot", "[Scheduler] cache read failed: bucket=%s err=%v", bucket.String(), err)
		} else if hit {
			return derefAccounts(cached), useMixed, nil
		}
	}

	if err := s.guardFallback(ctx); err != nil {
		return nil, useMixed, err
	}

	fallbackCtx, cancel := s.withFallbackTimeout(ctx)
	defer cancel()

	accounts, err := s.loadAccountsFromDB(fallbackCtx, bucket, useMixed)
	if err != nil {
		return nil, useMixed, err
	}

	if s.cache != nil {
		if err := s.cache.SetSnapshot(fallbackCtx, bucket, accounts); err != nil {
			logger.LegacyPrintf("service.scheduler_snapshot", "[Scheduler] cache write failed: bucket=%s err=%v", bucket.String(), err)
		}
	}

	return accounts, useMixed, nil
}

func (s *SchedulerSnapshotService) SchedulerEngineState(ctx context.Context) SchedulerEngineState {
	if cache, ok := s.cache.(SchedulerV2Cache); ok {
		state, err := cache.GetSchedulerEngineState(ctx)
		if err == nil && validSchedulerEngineState(state) {
			s.setLocalEngineState(state)
			return state
		}
	}
	if state, recovered := s.recoverSchedulerEngineState(ctx); recovered {
		return state
	}
	s.engineMu.RLock()
	defer s.engineMu.RUnlock()
	return s.engineState
}

func validSchedulerEngineState(state SchedulerEngineState) bool {
	if ValidateSchedulerV2Limits(state.CandidateLimit, state.ScanLimit) != nil {
		return false
	}
	switch state.Engine {
	case SchedulerEngineLegacy:
		return state.Status == SchedulerEngineStatusDisabled
	case SchedulerEngineV2:
		return state.Status == SchedulerEngineStatusBuilding ||
			state.Status == SchedulerEngineStatusActive ||
			state.Status == SchedulerEngineStatusFailed
	default:
		return false
	}
}

func (s *SchedulerSnapshotService) recoverSchedulerEngineState(ctx context.Context) (SchedulerEngineState, bool) {
	cache, cacheOK := s.cache.(SchedulerV2Cache)
	if !cacheOK || s.settingRepo == nil {
		return SchedulerEngineState{}, false
	}
	s.engineRecoveryMu.Lock()
	defer s.engineRecoveryMu.Unlock()

	if state, err := cache.GetSchedulerEngineState(ctx); err == nil && validSchedulerEngineState(state) {
		s.setLocalEngineState(state)
		return state, true
	}
	value, err := s.settingRepo.GetValue(ctx, SettingKeySchedulerV2Enabled)
	if err != nil && !errors.Is(err, ErrSettingNotFound) {
		return SchedulerEngineState{}, false
	}
	limitValues, err := s.settingRepo.GetMultiple(ctx, []string{
		SettingKeySchedulerV2CandidateLimit,
		SettingKeySchedulerV2ScanLimit,
	})
	if err != nil {
		return SchedulerEngineState{}, false
	}
	candidateLimit, scanLimit := parseSchedulerV2Limits(
		limitValues[SettingKeySchedulerV2CandidateLimit],
		limitValues[SettingKeySchedulerV2ScanLimit],
	)
	state := SchedulerEngineState{Engine: SchedulerEngineLegacy, Status: SchedulerEngineStatusDisabled}
	if value == "true" {
		state = SchedulerEngineState{Engine: SchedulerEngineV2, Status: SchedulerEngineStatusBuilding}
	}
	state.CandidateLimit = candidateLimit
	state.ScanLimit = scanLimit
	s.setLocalEngineState(state)
	if err := cache.SetSchedulerV2Limits(ctx, candidateLimit, scanLimit); err != nil {
		return state, true
	}
	if err := cache.SetSchedulerEngineState(ctx, state); err != nil {
		return state, true
	}
	if state.V2Enabled() {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.activateSchedulerV2("engine_state_recovery")
		}()
	}
	return state, true
}

func (s *SchedulerSnapshotService) SetSchedulerEngineSettingRepository(repo SettingRepository) {
	s.settingRepo = repo
}

func (s *SchedulerSnapshotService) SetSchedulerV2Enabled(ctx context.Context, enabled bool, candidateLimit, scanLimit int) error {
	if err := ValidateSchedulerV2Limits(candidateLimit, scanLimit); err != nil {
		return err
	}
	cache, ok := s.cache.(SchedulerV2Cache)
	if !ok {
		if enabled {
			return ErrSchedulerCacheUnavailable
		}
		return nil
	}
	if !enabled {
		// Old snapshots were intentionally not maintained while v2 was active.
		// Invalidate them before publishing the legacy mode switch.
		if err := cache.InvalidateLegacySnapshots(ctx); err != nil {
			return err
		}
		state := SchedulerEngineState{
			Engine:         SchedulerEngineLegacy,
			Status:         SchedulerEngineStatusDisabled,
			CandidateLimit: candidateLimit,
			ScanLimit:      scanLimit,
		}
		if err := cache.SetSchedulerEngineState(ctx, state); err != nil {
			return err
		}
		s.setLocalEngineState(state)
		return nil
	}

	state := SchedulerEngineState{
		Engine:         SchedulerEngineV2,
		Status:         SchedulerEngineStatusBuilding,
		CandidateLimit: candidateLimit,
		ScanLimit:      scanLimit,
	}
	if err := cache.SetSchedulerEngineState(ctx, state); err != nil {
		return err
	}
	s.setLocalEngineState(state)
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.activateSchedulerV2("admin_enable")
	}()
	return nil
}

func (s *SchedulerSnapshotService) ConfigureSchedulerEngineState(state SchedulerEngineState) {
	if state.Engine != SchedulerEngineV2 {
		state = SchedulerEngineState{Engine: SchedulerEngineLegacy, Status: SchedulerEngineStatusDisabled}
	} else if state.Status == "" {
		state.Status = SchedulerEngineStatusBuilding
	}
	s.setLocalEngineState(state)
}

func (s *SchedulerSnapshotService) ConfigureSchedulerV2Limits(ctx context.Context, candidateLimit, scanLimit int) error {
	if err := ValidateSchedulerV2Limits(candidateLimit, scanLimit); err != nil {
		return err
	}
	if cache, ok := s.cache.(SchedulerV2Cache); ok {
		if err := cache.SetSchedulerV2Limits(ctx, candidateLimit, scanLimit); err != nil {
			return err
		}
	}
	s.setLocalSchedulerV2Limits(candidateLimit, scanLimit)
	return nil
}

func (s *SchedulerSnapshotService) setLocalSchedulerV2Limits(candidateLimit, scanLimit int) {
	s.v2LimitsMu.Lock()
	s.v2CandidateLimit = candidateLimit
	s.v2ScanLimit = scanLimit
	s.v2LimitsMu.Unlock()
}

func (s *SchedulerSnapshotService) SchedulerV2Limits() (candidateLimit, scanLimit int) {
	s.v2LimitsMu.RLock()
	defer s.v2LimitsMu.RUnlock()
	return s.v2CandidateLimit, s.v2ScanLimit
}

func (s *SchedulerSnapshotService) setLocalEngineState(state SchedulerEngineState) {
	s.engineMu.Lock()
	s.engineState = state
	s.engineMu.Unlock()
	if ValidateSchedulerV2Limits(state.CandidateLimit, state.ScanLimit) == nil {
		s.setLocalSchedulerV2Limits(state.CandidateLimit, state.ScanLimit)
	}
}

func (s *SchedulerSnapshotService) schedulerV2Enabled(ctx context.Context) bool {
	return s.SchedulerEngineState(ctx).V2Enabled()
}

func (s *SchedulerSnapshotService) listSchedulerV2Accounts(ctx context.Context, bucket SchedulerBucket) ([]Account, error) {
	cache, ok := s.cache.(SchedulerV2Cache)
	if !ok {
		return nil, ErrSchedulerCacheUnavailable
	}
	limit, scanLimit := s.SchedulerV2Limits()
	accounts := make([]Account, 0, limit)
	seen := make(map[int64]struct{}, limit)
	appendCandidates := func(candidates []*Account) {
		if len(candidates) == 0 {
			return
		}
		eligible := make([]*Account, 0, len(candidates))
		for _, account := range candidates {
			if account == nil {
				continue
			}
			if _, exists := seen[account.ID]; exists {
				continue
			}
			if !s.schedulerV2CandidateMatchesBucket(account, bucket) || !account.IsSchedulable() ||
				schedulerCandidateExcluded(ctx, account.ID) || !schedulerCandidateMatchesRequest(ctx, account) {
				continue
			}
			eligible = append(eligible, account)
		}
		matches := schedulerCandidateBatchMatches(ctx, eligible)
		for i, account := range eligible {
			if i < len(matches) && matches[i] {
				if len(accounts) >= limit {
					return
				}
				seen[account.ID] = struct{}{}
				accounts = append(accounts, *account)
			}
		}
	}
	priorityIDs := schedulerCandidatePriorityIDs(ctx)
	if len(priorityIDs) > 0 {
		if _, hit, err := cache.GetCandidatePage(ctx, bucket, 0, MinSchedulerCandidateFetchLimit); err != nil {
			return nil, ErrSchedulerCacheUnavailable
		} else if !hit {
			if err := s.ensureSchedulerV2Bucket(ctx, bucket); err != nil {
				return nil, err
			}
		}
	}
	if len(priorityIDs) > scanLimit {
		priorityIDs = priorityIDs[:scanLimit]
	}
	priorityAccounts := make([]*Account, 0, len(priorityIDs))
	for _, accountID := range priorityIDs {
		account, err := cache.GetAccount(ctx, accountID)
		if err != nil {
			return nil, ErrSchedulerCacheUnavailable
		}
		priorityAccounts = append(priorityAccounts, account)
	}
	appendCandidates(priorityAccounts)
	rawScanned := len(priorityAccounts)
	var offset int64
	for len(accounts) < limit && rawScanned < scanLimit {
		pageLimit := limit
		if remaining := scanLimit - rawScanned; pageLimit > remaining {
			pageLimit = remaining
		}
		page, hit, err := cache.GetCandidatePage(ctx, bucket, offset, pageLimit)
		if err != nil {
			return nil, ErrSchedulerCacheUnavailable
		}
		if !hit {
			if err := s.ensureSchedulerV2Bucket(ctx, bucket); err != nil {
				return nil, err
			}
			page, hit, err = cache.GetCandidatePage(ctx, bucket, offset, limit)
			if err != nil {
				return nil, ErrSchedulerCacheUnavailable
			}
			if !hit {
				return nil, ErrSchedulerCacheNotReady
			}
		}
		rawScanned += len(page.Accounts)
		appendCandidates(page.Accounts)
		if page.Done || page.NextOffset <= offset {
			break
		}
		offset = page.NextOffset
	}
	return accounts, nil
}

func (s *SchedulerSnapshotService) schedulerV2CandidateMatchesBucket(account *Account, bucket SchedulerBucket) bool {
	if account == nil || !schedulerV2AccountBelongsToBucket(account, bucket) {
		return false
	}
	if s.isRunModeSimple() {
		return true
	}
	if bucket.GroupID <= 0 {
		return len(account.GroupIDs) == 0
	}
	for _, groupID := range account.GroupIDs {
		if groupID == bucket.GroupID {
			return true
		}
	}
	return false
}

func (s *SchedulerSnapshotService) ensureSchedulerV2Bucket(ctx context.Context, bucket SchedulerBucket) error {
	if err := s.rebuildBucket(ctx, bucket, "candidate_miss"); err != nil {
		return err
	}
	return s.waitSchedulerV2BucketReady(ctx, bucket)
}

func (s *SchedulerSnapshotService) waitSchedulerV2BucketReady(ctx context.Context, bucket SchedulerBucket) error {
	cache, ok := s.cache.(SchedulerV2Cache)
	if !ok {
		return ErrSchedulerCacheUnavailable
	}
	waitCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		_, hit, err := cache.GetCandidatePage(waitCtx, bucket, 0, MinSchedulerCandidateFetchLimit)
		if err != nil {
			return ErrSchedulerCacheUnavailable
		}
		if hit {
			return nil
		}
		select {
		case <-waitCtx.Done():
			return ErrSchedulerCacheNotReady
		case <-ticker.C:
		}
	}
}

func (s *SchedulerSnapshotService) GetAccount(ctx context.Context, accountID int64) (*Account, error) {
	if accountID <= 0 {
		return nil, nil
	}
	if s.cache != nil {
		account, err := s.cache.GetAccount(ctx, accountID)
		if err != nil {
			logger.LegacyPrintf("service.scheduler_snapshot", "[Scheduler] account cache read failed: id=%d err=%v", accountID, err)
		} else if account != nil {
			return account, nil
		}
	}

	if err := s.guardFallback(ctx); err != nil {
		return nil, err
	}
	fallbackCtx, cancel := s.withFallbackTimeout(ctx)
	defer cancel()
	return s.accountRepo.GetByID(fallbackCtx, accountID)
}

// GetGroupByID 获取分组信息（供调度器使用）
func (s *SchedulerSnapshotService) GetGroupByID(ctx context.Context, groupID int64) (*Group, error) {
	if s.groupRepo == nil {
		return nil, nil
	}
	return s.groupRepo.GetByID(ctx, groupID)
}

// UpdateAccountInCache 立即更新 Redis 中单个账号的数据（用于模型限流后立即生效）
func (s *SchedulerSnapshotService) UpdateAccountInCache(ctx context.Context, account *Account) error {
	if s.cache == nil || account == nil {
		return nil
	}
	return s.cache.SetAccount(ctx, account)
}

func (s *SchedulerSnapshotService) runInitialRebuild() {
	if s.cache == nil {
		return
	}
	if s.schedulerV2Enabled(context.Background()) {
		s.activateSchedulerV2("startup")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	buckets, err := s.cache.ListBuckets(ctx)
	if err != nil {
		logger.LegacyPrintf("service.scheduler_snapshot", "[Scheduler] list buckets failed: %v", err)
	}
	if len(buckets) == 0 {
		buckets, err = s.defaultBuckets(ctx)
		if err != nil {
			logger.LegacyPrintf("service.scheduler_snapshot", "[Scheduler] default buckets failed: %v", err)
			return
		}
	}
	if err := s.rebuildBuckets(ctx, buckets, "startup"); err != nil {
		logger.LegacyPrintf("service.scheduler_snapshot", "[Scheduler] rebuild startup failed: %v", err)
	}
}

func (s *SchedulerSnapshotService) runOutboxWorker(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	s.pollOutbox()
	for {
		select {
		case <-ticker.C:
			s.pollOutbox()
		case <-s.stopCh:
			return
		}
	}
}

func (s *SchedulerSnapshotService) runFullRebuildWorker(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if s.schedulerV2Enabled(context.Background()) && !s.schedulerV2FullRebuildDue(interval) {
				continue
			}
			if err := s.triggerFullRebuild("interval"); err != nil {
				logger.LegacyPrintf("service.scheduler_snapshot", "[Scheduler] full rebuild failed: %v", err)
			}
		case <-s.stopCh:
			return
		}
	}
}

func (s *SchedulerSnapshotService) pollOutbox() {
	if s.outboxRepo == nil || s.cache == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	watermark, err := s.cache.GetOutboxWatermark(ctx)
	if err != nil {
		logger.LegacyPrintf("service.scheduler_snapshot", "[Scheduler] outbox watermark read failed: %v", err)
		return
	}

	events, err := s.outboxRepo.ListAfterAndReleaseDedup(ctx, watermark, 200)
	if err != nil {
		logger.LegacyPrintf("service.scheduler_snapshot", "[Scheduler] outbox poll failed: %v", err)
		return
	}
	if len(events) == 0 {
		return
	}

	watermarkForCheck := watermark
	seen := make(map[batchSeenKey]struct{})
	for _, event := range events {
		eventCtx, cancel := context.WithTimeout(context.Background(), outboxEventTimeout)
		err := s.handleOutboxEvent(eventCtx, event, seen)
		cancel()
		if err != nil {
			logger.LegacyPrintf("service.scheduler_snapshot", "[Scheduler] outbox handle failed: id=%d type=%s err=%v", event.ID, event.EventType, err)
			return
		}
	}

	lastID := events[len(events)-1].ID
	var wmErr error
	for i := range 3 {
		wmCtx, wmCancel := context.WithTimeout(context.Background(), 5*time.Second)
		wmErr = s.cache.SetOutboxWatermark(wmCtx, lastID)
		wmCancel()
		if wmErr == nil {
			break
		}
		if i < 2 {
			time.Sleep(200 * time.Millisecond)
		}
	}
	if wmErr != nil {
		logger.LegacyPrintf("service.scheduler_snapshot", "[Scheduler] outbox watermark write failed: %v", wmErr)
	} else {
		watermarkForCheck = lastID
		s.cleanupConsumedOutbox(lastID)
	}

	s.checkOutboxLag(ctx, events[0], watermarkForCheck)
}

func (s *SchedulerSnapshotService) cleanupConsumedOutbox(watermark int64) {
	if s == nil || s.outboxRepo == nil || watermark <= 0 {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	lease, acquired, err := s.outboxRepo.TryAcquireCleanupLock(ctx)
	if err != nil {
		logger.LegacyPrintf("service.scheduler_snapshot", "[Scheduler] outbox cleanup lock failed: %v", err)
		return
	}
	if !acquired {
		return
	}
	defer lease.Release()

	for {
		deleted, err := s.outboxRepo.DeleteConsumedUpTo(ctx, watermark, schedulerOutboxCleanupBatch)
		if err != nil {
			logger.LegacyPrintf("service.scheduler_snapshot", "[Scheduler] outbox cleanup failed: watermark=%d err=%v", watermark, err)
			return
		}
		if deleted == 0 || deleted < schedulerOutboxCleanupBatch {
			return
		}
	}
}

func (s *SchedulerSnapshotService) handleOutboxEvent(ctx context.Context, event SchedulerOutboxEvent, seen map[batchSeenKey]struct{}) error {
	switch event.EventType {
	case SchedulerOutboxEventAccountLastUsed:
		return s.handleLastUsedEvent(ctx, event.Payload)
	case SchedulerOutboxEventAccountBulkChanged:
		return s.handleBulkAccountEvent(ctx, event.Payload, seen)
	case SchedulerOutboxEventAccountGroupsChanged:
		return s.handleAccountEvent(ctx, event.AccountID, event.Payload, seen)
	case SchedulerOutboxEventAccountChanged:
		return s.handleAccountEvent(ctx, event.AccountID, event.Payload, seen)
	case SchedulerOutboxEventGroupChanged:
		return s.handleGroupEvent(ctx, event.GroupID, seen)
	case SchedulerOutboxEventFullRebuild:
		return s.triggerFullRebuild("outbox")
	default:
		return nil
	}
}

func (s *SchedulerSnapshotService) handleLastUsedEvent(ctx context.Context, payload map[string]any) error {
	if s.cache == nil || payload == nil {
		return nil
	}
	raw, ok := payload["last_used"].(map[string]any)
	if !ok || len(raw) == 0 {
		return nil
	}
	updates := make(map[int64]time.Time, len(raw))
	for key, value := range raw {
		id, err := strconv.ParseInt(key, 10, 64)
		if err != nil || id <= 0 {
			continue
		}
		sec, ok := toInt64(value)
		if !ok || sec <= 0 {
			continue
		}
		updates[id] = time.Unix(sec, 0)
	}
	if len(updates) == 0 {
		return nil
	}
	if s.schedulerV2Enabled(ctx) {
		cache, ok := s.cache.(SchedulerV2Cache)
		if !ok {
			return ErrSchedulerCacheUnavailable
		}
		return cache.UpdateCandidateLastUsed(ctx, updates)
	}
	return s.cache.UpdateLastUsed(ctx, updates)
}

func (s *SchedulerSnapshotService) handleBulkAccountEvent(ctx context.Context, payload map[string]any, seen map[batchSeenKey]struct{}) error {
	if payload == nil {
		return nil
	}
	if s.accountRepo == nil {
		return nil
	}

	rawIDs := parseInt64Slice(payload["account_ids"])
	if len(rawIDs) == 0 {
		return nil
	}

	ids := make([]int64, 0, len(rawIDs))
	seenIDs := make(map[int64]struct{}, len(rawIDs))
	for _, id := range rawIDs {
		if id <= 0 {
			continue
		}
		if _, exists := seenIDs[id]; exists {
			continue
		}
		seenIDs[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil
	}

	preloadGroupIDs := parseInt64Slice(payload["group_ids"])
	accounts, err := s.accountRepo.GetByIDs(ctx, ids)
	if err != nil {
		return err
	}
	if s.schedulerV2Enabled(ctx) {
		cache, ok := s.cache.(SchedulerV2Cache)
		if !ok {
			return ErrSchedulerCacheUnavailable
		}
		found := make(map[int64]struct{}, len(accounts))
		for _, account := range accounts {
			if account == nil || account.ID <= 0 {
				continue
			}
			found[account.ID] = struct{}{}
			if err := s.reconcileSchedulerV2Account(ctx, account); err != nil {
				return err
			}
		}
		for _, id := range ids {
			if _, exists := found[id]; exists {
				continue
			}
			if err := cache.DeleteCandidateAccount(ctx, id); err != nil {
				return err
			}
		}
		return nil
	}

	found := make(map[int64]struct{}, len(accounts))
	rebuildGroupSet := make(map[int64]struct{}, len(preloadGroupIDs))
	for _, gid := range preloadGroupIDs {
		if gid > 0 {
			rebuildGroupSet[gid] = struct{}{}
		}
	}

	for _, account := range accounts {
		if account == nil || account.ID <= 0 {
			continue
		}
		found[account.ID] = struct{}{}
		if s.cache != nil {
			if err := s.cache.SetAccount(ctx, account); err != nil {
				return err
			}
		}
		for _, gid := range account.GroupIDs {
			if gid > 0 {
				rebuildGroupSet[gid] = struct{}{}
			}
		}
	}

	if s.cache != nil {
		for _, id := range ids {
			if _, ok := found[id]; ok {
				continue
			}
			if err := s.cache.DeleteAccount(ctx, id); err != nil {
				return err
			}
		}
	}

	rebuildGroupIDs := make([]int64, 0, len(rebuildGroupSet))
	for gid := range rebuildGroupSet {
		rebuildGroupIDs = append(rebuildGroupIDs, gid)
	}
	return s.rebuildByGroupIDs(ctx, rebuildGroupIDs, "account_bulk_change", seen)
}

func (s *SchedulerSnapshotService) handleAccountEvent(ctx context.Context, accountID *int64, payload map[string]any, seen map[batchSeenKey]struct{}) error {
	if accountID == nil || *accountID <= 0 {
		return nil
	}
	if s.accountRepo == nil {
		return nil
	}

	var groupIDs []int64
	if payload != nil {
		groupIDs = parseInt64Slice(payload["group_ids"])
	}

	account, err := s.accountRepo.GetByID(ctx, *accountID)
	if err != nil {
		if errors.Is(err, ErrAccountNotFound) {
			if s.schedulerV2Enabled(ctx) {
				cache, ok := s.cache.(SchedulerV2Cache)
				if !ok {
					return ErrSchedulerCacheUnavailable
				}
				return cache.DeleteCandidateAccount(ctx, *accountID)
			}
			if s.cache != nil {
				if err := s.cache.DeleteAccount(ctx, *accountID); err != nil {
					return err
				}
			}
			return s.rebuildByGroupIDs(ctx, groupIDs, "account_miss", seen)
		}
		return err
	}
	if s.schedulerV2Enabled(ctx) {
		return s.reconcileSchedulerV2Account(ctx, account)
	}
	if s.cache != nil {
		if err := s.cache.SetAccount(ctx, account); err != nil {
			return err
		}
	}
	if len(groupIDs) == 0 {
		groupIDs = account.GroupIDs
	}
	return s.rebuildByAccount(ctx, account, groupIDs, "account_change", seen)
}

func (s *SchedulerSnapshotService) reconcileSchedulerV2Account(ctx context.Context, account *Account) error {
	cache, ok := s.cache.(SchedulerV2Cache)
	if !ok {
		return ErrSchedulerCacheUnavailable
	}
	buckets := s.schedulerV2BucketsForAccount(account)
	return cache.ReplaceAccountCandidates(ctx, account, buckets)
}

func (s *SchedulerSnapshotService) schedulerV2BucketsForAccount(account *Account) []SchedulerBucket {
	if !schedulerV2PotentialAccount(account) {
		return nil
	}
	if s.isRunModeSimple() || len(account.GroupIDs) == 0 {
		return schedulerV2BucketsForAccountGroup(account, 0, account.Platform)
	}
	buckets := make([]SchedulerBucket, 0, len(account.GroupIDs)*3)
	for _, groupID := range account.GroupIDs {
		if groupID <= 0 {
			continue
		}
		platform := schedulerV2AccountGroupPlatform(account, groupID)
		if platform == "" {
			platform = account.Platform
		}
		buckets = append(buckets, schedulerV2BucketsForAccountGroup(account, groupID, platform)...)
	}
	return dedupeBuckets(buckets)
}

func schedulerV2BucketsForAccountGroup(account *Account, groupID int64, groupPlatform string) []SchedulerBucket {
	if account == nil {
		return nil
	}
	buckets := []SchedulerBucket{
		{GroupID: groupID, Platform: account.Platform, Mode: SchedulerModeForced},
	}
	if groupPlatform == account.Platform {
		buckets = append(buckets, SchedulerBucket{GroupID: groupID, Platform: groupPlatform, Mode: SchedulerModeSingle})
		if groupPlatform == PlatformAnthropic || groupPlatform == PlatformGemini {
			buckets = append(buckets, SchedulerBucket{GroupID: groupID, Platform: groupPlatform, Mode: SchedulerModeMixed})
		}
	}
	if account.Platform == PlatformAntigravity && account.IsMixedSchedulingEnabled() &&
		(groupPlatform == PlatformAnthropic || groupPlatform == PlatformGemini) {
		buckets = append(buckets, SchedulerBucket{GroupID: groupID, Platform: groupPlatform, Mode: SchedulerModeMixed})
	}
	if groupID == 0 && account.Platform == PlatformAntigravity && account.IsMixedSchedulingEnabled() {
		buckets = append(buckets,
			SchedulerBucket{GroupID: 0, Platform: PlatformAnthropic, Mode: SchedulerModeMixed},
			SchedulerBucket{GroupID: 0, Platform: PlatformGemini, Mode: SchedulerModeMixed},
		)
	}
	return dedupeBuckets(buckets)
}

func schedulerV2AccountGroupPlatform(account *Account, groupID int64) string {
	if account == nil || groupID <= 0 {
		return ""
	}
	for _, membership := range account.AccountGroups {
		if membership.GroupID == groupID && membership.Group != nil {
			return membership.Group.Platform
		}
	}
	for _, group := range account.Groups {
		if group != nil && group.ID == groupID {
			return group.Platform
		}
	}
	return ""
}

func (s *SchedulerSnapshotService) handleGroupEvent(ctx context.Context, groupID *int64, seen map[batchSeenKey]struct{}) error {
	if groupID == nil || *groupID <= 0 {
		return nil
	}
	groupIDs := []int64{*groupID}
	return s.rebuildByGroupIDs(ctx, groupIDs, "group_change", seen)
}

func (s *SchedulerSnapshotService) rebuildByAccount(ctx context.Context, account *Account, groupIDs []int64, reason string, seen map[batchSeenKey]struct{}) error {
	if account == nil {
		return nil
	}
	groupIDs = s.normalizeGroupIDs(groupIDs)
	if len(groupIDs) == 0 {
		return nil
	}

	var firstErr error
	if err := s.rebuildBucketsForPlatform(ctx, account.Platform, groupIDs, reason, seen); err != nil && firstErr == nil {
		firstErr = err
	}
	if account.Platform == PlatformAntigravity && account.IsMixedSchedulingEnabled() {
		if err := s.rebuildBucketsForPlatform(ctx, PlatformAnthropic, groupIDs, reason, seen); err != nil && firstErr == nil {
			firstErr = err
		}
		if err := s.rebuildBucketsForPlatform(ctx, PlatformGemini, groupIDs, reason, seen); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (s *SchedulerSnapshotService) rebuildByGroupIDs(ctx context.Context, groupIDs []int64, reason string, seen map[batchSeenKey]struct{}) error {
	groupIDs = s.normalizeGroupIDs(groupIDs)
	if len(groupIDs) == 0 {
		return nil
	}
	platforms := []string{PlatformAnthropic, PlatformGemini, PlatformOpenAI, PlatformAntigravity, PlatformGrok}
	var firstErr error
	for _, platform := range platforms {
		if err := s.rebuildBucketsForPlatform(ctx, platform, groupIDs, reason, seen); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (s *SchedulerSnapshotService) rebuildBucketsForPlatform(ctx context.Context, platform string, groupIDs []int64, reason string, seen map[batchSeenKey]struct{}) error {
	if platform == "" {
		return nil
	}
	var firstErr error
	for _, gid := range groupIDs {
		// Within a single poll batch, skip (groupID, platform) pairs that were
		// already rebuilt. The first rebuild loads fresh DB data for all accounts
		// in the group, so subsequent rebuilds for the same group+platform within
		// the same batch are redundant.
		if seen != nil {
			key := batchSeenKey{gid, platform}
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
		}
		if err := s.rebuildBucket(ctx, SchedulerBucket{GroupID: gid, Platform: platform, Mode: SchedulerModeSingle}, reason); err != nil && firstErr == nil {
			firstErr = err
		}
		if err := s.rebuildBucket(ctx, SchedulerBucket{GroupID: gid, Platform: platform, Mode: SchedulerModeForced}, reason); err != nil && firstErr == nil {
			firstErr = err
		}
		if platform == PlatformAnthropic || platform == PlatformGemini {
			if err := s.rebuildBucket(ctx, SchedulerBucket{GroupID: gid, Platform: platform, Mode: SchedulerModeMixed}, reason); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

func (s *SchedulerSnapshotService) rebuildBuckets(ctx context.Context, buckets []SchedulerBucket, reason string) error {
	var firstErr error
	for _, bucket := range buckets {
		if err := s.rebuildBucket(ctx, bucket, reason); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (s *SchedulerSnapshotService) rebuildBucket(ctx context.Context, bucket SchedulerBucket, reason string) error {
	if s.schedulerV2Enabled(ctx) {
		return s.rebuildSchedulerV2Bucket(ctx, bucket, reason)
	}
	if s.cache == nil {
		return ErrSchedulerCacheNotReady
	}
	ok, err := s.cache.TryLockBucket(ctx, bucket, 30*time.Second)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	defer func() {
		_ = s.cache.UnlockBucket(ctx, bucket)
	}()

	rebuildCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	accounts, err := s.loadAccountsFromDB(rebuildCtx, bucket, bucket.Mode == SchedulerModeMixed)
	if err != nil {
		logger.LegacyPrintf("service.scheduler_snapshot", "[Scheduler] rebuild failed: bucket=%s reason=%s err=%v", bucket.String(), reason, err)
		return err
	}
	if err := s.cache.SetSnapshot(rebuildCtx, bucket, accounts); err != nil {
		logger.LegacyPrintf("service.scheduler_snapshot", "[Scheduler] rebuild cache failed: bucket=%s reason=%s err=%v", bucket.String(), reason, err)
		return err
	}
	slog.Debug("[Scheduler] rebuild ok", "bucket", bucket.String(), "reason", reason, "size", len(accounts))
	return nil
}

func (s *SchedulerSnapshotService) rebuildSchedulerV2Bucket(ctx context.Context, bucket SchedulerBucket, reason string) error {
	cache, ok := s.cache.(SchedulerV2Cache)
	if !ok {
		return ErrSchedulerCacheUnavailable
	}
	locked, err := s.cache.TryLockBucket(ctx, bucket, 3*time.Minute)
	if err != nil {
		return err
	}
	if !locked {
		return s.waitSchedulerV2BucketReady(ctx, bucket)
	}
	defer func() { _ = s.cache.UnlockBucket(context.Background(), bucket) }()

	rebuildCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	accounts, err := s.loadSchedulerV2CandidatesFromDB(rebuildCtx, bucket)
	if err != nil {
		logger.LegacyPrintf("service.scheduler_snapshot", "[SchedulerV2] rebuild failed: bucket=%s reason=%s err=%v", bucket.String(), reason, err)
		return err
	}
	if err := cache.SetCandidateIndex(rebuildCtx, bucket, accounts); err != nil {
		logger.LegacyPrintf("service.scheduler_snapshot", "[SchedulerV2] cache write failed: bucket=%s reason=%s err=%v", bucket.String(), reason, err)
		return err
	}
	slog.Debug("scheduler_v2_rebuild_ok", "bucket", bucket.String(), "reason", reason, "size", len(accounts))
	return nil
}

func (s *SchedulerSnapshotService) activateSchedulerV2(reason string) {
	s.activationMu.Lock()
	defer s.activationMu.Unlock()
	cache, ok := s.cache.(SchedulerV2Cache)
	if !ok {
		s.failSchedulerV2Activation(context.Background(), ErrSchedulerCacheUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	locked, err := s.cache.TryLockBucket(ctx, schedulerV2ActivationBucket, 16*time.Minute)
	if err != nil {
		s.failSchedulerV2Activation(context.Background(), err)
		return
	}
	if !locked {
		return
	}
	defer func() { _ = s.cache.UnlockBucket(context.Background(), schedulerV2ActivationBucket) }()

	initialState := s.SchedulerEngineState(ctx)
	if !initialState.V2Enabled() {
		return
	}
	building := SchedulerEngineState{Engine: SchedulerEngineV2, Status: SchedulerEngineStatusBuilding}
	if initialState.Status == SchedulerEngineStatusActive {
		changed, err := cache.CompareAndSetSchedulerEngineState(ctx, SchedulerEngineV2, SchedulerEngineStatusActive, building)
		if err != nil {
			s.failSchedulerV2Activation(context.Background(), err)
			return
		}
		if !changed {
			s.refreshLocalSchedulerEngineState(ctx)
			return
		}
	} else if initialState.Status != SchedulerEngineStatusBuilding {
		return
	}
	s.setLocalEngineState(building)

	buckets, err := s.cache.ListBuckets(ctx)
	if err != nil {
		s.failSchedulerV2Activation(context.Background(), err)
		return
	}
	defaults, err := s.defaultBuckets(ctx)
	if err == nil {
		buckets = append(buckets, defaults...)
	}
	buckets = dedupeBuckets(buckets)
	for _, bucket := range buckets {
		if !s.schedulerV2Enabled(ctx) {
			return
		}
		if reason == "startup" && initialState.Status == SchedulerEngineStatusActive {
			if _, hit, readyErr := cache.GetCandidatePage(ctx, bucket, 0, MinSchedulerCandidateFetchLimit); readyErr == nil && hit {
				continue
			}
		}
		if err := s.rebuildSchedulerV2Bucket(ctx, bucket, reason); err != nil {
			s.failSchedulerV2Activation(context.Background(), err)
			return
		}
	}
	active := SchedulerEngineState{Engine: SchedulerEngineV2, Status: SchedulerEngineStatusActive}
	changed, err := cache.CompareAndSetSchedulerEngineState(ctx, SchedulerEngineV2, SchedulerEngineStatusBuilding, active)
	if err != nil {
		s.failSchedulerV2Activation(context.Background(), err)
		return
	}
	if !changed {
		s.refreshLocalSchedulerEngineState(ctx)
		return
	}
	s.setLocalEngineState(active)
	s.markSchedulerV2FullRebuild()
	slog.Info("scheduler_v2_activated", "reason", reason, "bucket_count", len(buckets))
}

func (s *SchedulerSnapshotService) schedulerV2FullRebuildDue(configured time.Duration) bool {
	interval := configured
	if interval < schedulerV2FullRebuildFloor {
		interval = schedulerV2FullRebuildFloor
	}
	s.v2RebuildMu.Lock()
	defer s.v2RebuildMu.Unlock()
	return s.v2LastRebuild.IsZero() || time.Since(s.v2LastRebuild) >= interval
}

func (s *SchedulerSnapshotService) markSchedulerV2FullRebuild() {
	s.v2RebuildMu.Lock()
	s.v2LastRebuild = time.Now()
	s.v2RebuildMu.Unlock()
}

func (s *SchedulerSnapshotService) failSchedulerV2Activation(ctx context.Context, activationErr error) {
	if activationErr == nil {
		activationErr = ErrSchedulerCacheNotReady
	}
	state := SchedulerEngineState{
		Engine:    SchedulerEngineV2,
		Status:    SchedulerEngineStatusFailed,
		LastError: activationErr.Error(),
	}
	if cache, ok := s.cache.(SchedulerV2Cache); ok {
		changed, err := cache.CompareAndSetSchedulerEngineState(ctx, SchedulerEngineV2, SchedulerEngineStatusBuilding, state)
		if err != nil {
			s.setLocalEngineState(state)
			slog.Warn("scheduler_v2_activation_failed", "error", activationErr, "state_publish_error", err)
			return
		}
		if !changed {
			s.refreshLocalSchedulerEngineState(ctx)
			return
		}
	}
	s.setLocalEngineState(state)
	slog.Warn("scheduler_v2_activation_failed", "error", activationErr)
}

func (s *SchedulerSnapshotService) refreshLocalSchedulerEngineState(ctx context.Context) {
	cache, ok := s.cache.(SchedulerV2Cache)
	if !ok {
		return
	}
	state, err := cache.GetSchedulerEngineState(ctx)
	if err == nil && (state.Engine == SchedulerEngineLegacy || state.Engine == SchedulerEngineV2) {
		s.setLocalEngineState(state)
	}
}

func (s *SchedulerSnapshotService) triggerFullRebuild(reason string) error {
	if s.cache == nil {
		return ErrSchedulerCacheNotReady
	}
	timeout := 2 * time.Minute
	if s.schedulerV2Enabled(context.Background()) {
		timeout = 15 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	buckets, err := s.cache.ListBuckets(ctx)
	if err != nil {
		logger.LegacyPrintf("service.scheduler_snapshot", "[Scheduler] list buckets failed: %v", err)
		return err
	}
	if s.schedulerV2Enabled(ctx) {
		defaults, defaultErr := s.defaultBuckets(ctx)
		if defaultErr != nil {
			return defaultErr
		}
		buckets = dedupeBuckets(append(buckets, defaults...))
	} else if len(buckets) == 0 {
		buckets, err = s.defaultBuckets(ctx)
		if err != nil {
			logger.LegacyPrintf("service.scheduler_snapshot", "[Scheduler] default buckets failed: %v", err)
			return err
		}
	}
	if err := s.rebuildBuckets(ctx, buckets, reason); err != nil {
		return err
	}
	if s.schedulerV2Enabled(ctx) {
		cache, ok := s.cache.(SchedulerV2Cache)
		if !ok {
			return ErrSchedulerCacheUnavailable
		}
		current, err := cache.GetSchedulerEngineState(ctx)
		if err != nil {
			return err
		}
		if !current.V2Enabled() {
			s.setLocalEngineState(current)
			return nil
		}
		state := SchedulerEngineState{Engine: SchedulerEngineV2, Status: SchedulerEngineStatusActive}
		changed, err := cache.CompareAndSetSchedulerEngineState(ctx, current.Engine, current.Status, state)
		if err != nil {
			return err
		}
		if !changed {
			s.refreshLocalSchedulerEngineState(ctx)
			return nil
		}
		s.setLocalEngineState(state)
		s.markSchedulerV2FullRebuild()
	}
	return nil
}

func (s *SchedulerSnapshotService) loadSchedulerV2CandidatesFromDB(ctx context.Context, bucket SchedulerBucket) ([]Account, error) {
	if s.accountRepo == nil {
		return nil, ErrSchedulerCacheNotReady
	}
	var accounts []Account
	var err error
	switch {
	case bucket.GroupID > 0 && !s.isRunModeSimple():
		accounts, err = s.accountRepo.ListByGroup(ctx, bucket.GroupID)
	case s.isRunModeSimple():
		accounts, err = s.accountRepo.ListActive(ctx)
	default:
		accounts, err = s.accountRepo.ListAllWithFilters(ctx, "", "", StatusActive, "", AccountListGroupUngrouped, "")
	}
	if err != nil {
		return nil, err
	}
	filtered := make([]Account, 0, len(accounts))
	for _, account := range accounts {
		if !schedulerV2PotentialAccount(&account) || !schedulerV2AccountBelongsToBucket(&account, bucket) {
			continue
		}
		filtered = append(filtered, account)
	}
	return filtered, nil
}

func schedulerV2PotentialAccount(account *Account) bool {
	return account != nil && account.ID > 0 && account.IsActive() && account.Schedulable
}

func schedulerV2AccountBelongsToBucket(account *Account, bucket SchedulerBucket) bool {
	if account == nil {
		return false
	}
	if bucket.Mode == SchedulerModeMixed && (bucket.Platform == PlatformAnthropic || bucket.Platform == PlatformGemini) {
		return account.Platform == bucket.Platform ||
			(account.Platform == PlatformAntigravity && account.IsMixedSchedulingEnabled())
	}
	return account.Platform == bucket.Platform
}

func (s *SchedulerSnapshotService) checkOutboxLag(ctx context.Context, oldest SchedulerOutboxEvent, watermark int64) {
	if oldest.CreatedAt.IsZero() || s.cfg == nil {
		return
	}

	lag := time.Since(oldest.CreatedAt)
	if lagSeconds := int(lag.Seconds()); lagSeconds >= s.cfg.Gateway.Scheduling.OutboxLagWarnSeconds && s.cfg.Gateway.Scheduling.OutboxLagWarnSeconds > 0 {
		logger.LegacyPrintf("service.scheduler_snapshot", "[Scheduler] outbox lag warning: %ds", lagSeconds)
	}

	if s.cfg.Gateway.Scheduling.OutboxLagRebuildSeconds > 0 && int(lag.Seconds()) >= s.cfg.Gateway.Scheduling.OutboxLagRebuildSeconds {
		s.lagMu.Lock()
		s.lagFailures++
		failures := s.lagFailures
		s.lagMu.Unlock()

		if failures >= s.cfg.Gateway.Scheduling.OutboxLagRebuildFailures {
			logger.LegacyPrintf("service.scheduler_snapshot", "[Scheduler] outbox lag rebuild triggered: lag=%s failures=%d", lag, failures)
			s.lagMu.Lock()
			s.lagFailures = 0
			s.lagMu.Unlock()
			if err := s.triggerFullRebuild("outbox_lag"); err != nil {
				logger.LegacyPrintf("service.scheduler_snapshot", "[Scheduler] outbox lag rebuild failed: %v", err)
			}
		}
	} else {
		s.lagMu.Lock()
		s.lagFailures = 0
		s.lagMu.Unlock()
	}

	threshold := s.cfg.Gateway.Scheduling.OutboxBacklogRebuildRows
	if threshold <= 0 || s.outboxRepo == nil {
		return
	}
	maxID, err := s.outboxRepo.MaxID(ctx)
	if err != nil {
		return
	}
	if maxID-watermark >= int64(threshold) {
		logger.LegacyPrintf("service.scheduler_snapshot", "[Scheduler] outbox backlog rebuild triggered: backlog=%d", maxID-watermark)
		if err := s.triggerFullRebuild("outbox_backlog"); err != nil {
			logger.LegacyPrintf("service.scheduler_snapshot", "[Scheduler] outbox backlog rebuild failed: %v", err)
		}
	}
}

func (s *SchedulerSnapshotService) loadAccountsFromDB(ctx context.Context, bucket SchedulerBucket, useMixed bool) ([]Account, error) {
	if s.accountRepo == nil {
		return nil, ErrSchedulerCacheNotReady
	}
	groupID := bucket.GroupID
	if s.isRunModeSimple() {
		groupID = 0
	}

	if useMixed {
		platforms := []string{bucket.Platform, PlatformAntigravity}
		var accounts []Account
		var err error
		if groupID > 0 {
			accounts, err = s.accountRepo.ListSchedulableByGroupIDAndPlatforms(ctx, groupID, platforms)
		} else if s.isRunModeSimple() {
			accounts, err = s.accountRepo.ListSchedulableByPlatforms(ctx, platforms)
		} else {
			accounts, err = s.accountRepo.ListSchedulableUngroupedByPlatforms(ctx, platforms)
		}
		if err != nil {
			return nil, err
		}
		filtered := make([]Account, 0, len(accounts))
		for _, acc := range accounts {
			if acc.Platform == PlatformAntigravity && !acc.IsMixedSchedulingEnabled() {
				continue
			}
			filtered = append(filtered, acc)
		}
		return filtered, nil
	}

	if groupID > 0 {
		return s.accountRepo.ListSchedulableByGroupIDAndPlatform(ctx, groupID, bucket.Platform)
	}
	if s.isRunModeSimple() {
		return s.accountRepo.ListSchedulableByPlatform(ctx, bucket.Platform)
	}
	return s.accountRepo.ListSchedulableUngroupedByPlatform(ctx, bucket.Platform)
}

func (s *SchedulerSnapshotService) bucketFor(groupID *int64, platform string, mode string) SchedulerBucket {
	return SchedulerBucket{
		GroupID:  s.normalizeGroupID(groupID),
		Platform: platform,
		Mode:     mode,
	}
}

func (s *SchedulerSnapshotService) normalizeGroupID(groupID *int64) int64 {
	if s.isRunModeSimple() {
		return 0
	}
	if groupID == nil || *groupID <= 0 {
		return 0
	}
	return *groupID
}

func (s *SchedulerSnapshotService) normalizeGroupIDs(groupIDs []int64) []int64 {
	if s.isRunModeSimple() {
		return []int64{0}
	}
	if len(groupIDs) == 0 {
		return []int64{0}
	}
	seen := make(map[int64]struct{}, len(groupIDs))
	out := make([]int64, 0, len(groupIDs))
	for _, id := range groupIDs {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	if len(out) == 0 {
		return []int64{0}
	}
	return out
}

func (s *SchedulerSnapshotService) resolveMode(platform string, hasForcePlatform bool) string {
	if hasForcePlatform {
		return SchedulerModeForced
	}
	if platform == PlatformAnthropic || platform == PlatformGemini {
		return SchedulerModeMixed
	}
	return SchedulerModeSingle
}

func (s *SchedulerSnapshotService) guardFallback(ctx context.Context) error {
	if s.cfg == nil || s.cfg.Gateway.Scheduling.DbFallbackEnabled {
		if s.fallbackLimit == nil || s.fallbackLimit.Allow() {
			return nil
		}
		return ErrSchedulerFallbackLimited
	}
	return ErrSchedulerCacheNotReady
}

func (s *SchedulerSnapshotService) withFallbackTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if s.cfg == nil || s.cfg.Gateway.Scheduling.DbFallbackTimeoutSeconds <= 0 {
		return context.WithCancel(ctx)
	}
	timeout := time.Duration(s.cfg.Gateway.Scheduling.DbFallbackTimeoutSeconds) * time.Second
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return context.WithCancel(ctx)
		}
		if remaining < timeout {
			timeout = remaining
		}
	}
	return context.WithTimeout(ctx, timeout)
}

func (s *SchedulerSnapshotService) isRunModeSimple() bool {
	return s.cfg != nil && s.cfg.RunMode == config.RunModeSimple
}

func (s *SchedulerSnapshotService) outboxPollInterval() time.Duration {
	if s.cfg == nil {
		return time.Second
	}
	sec := s.cfg.Gateway.Scheduling.OutboxPollIntervalSeconds
	if sec <= 0 {
		return time.Second
	}
	return time.Duration(sec) * time.Second
}

func (s *SchedulerSnapshotService) fullRebuildInterval() time.Duration {
	if s.cfg == nil {
		return 0
	}
	sec := s.cfg.Gateway.Scheduling.FullRebuildIntervalSeconds
	if sec <= 0 {
		return 0
	}
	return time.Duration(sec) * time.Second
}

func (s *SchedulerSnapshotService) defaultBuckets(ctx context.Context) ([]SchedulerBucket, error) {
	buckets := make([]SchedulerBucket, 0)
	platforms := []string{PlatformAnthropic, PlatformGemini, PlatformOpenAI, PlatformAntigravity, PlatformGrok}
	for _, platform := range platforms {
		buckets = append(buckets, SchedulerBucket{GroupID: 0, Platform: platform, Mode: SchedulerModeSingle})
		buckets = append(buckets, SchedulerBucket{GroupID: 0, Platform: platform, Mode: SchedulerModeForced})
		if platform == PlatformAnthropic || platform == PlatformGemini {
			buckets = append(buckets, SchedulerBucket{GroupID: 0, Platform: platform, Mode: SchedulerModeMixed})
		}
	}

	if s.isRunModeSimple() || s.groupRepo == nil {
		return dedupeBuckets(buckets), nil
	}

	groups, err := s.groupRepo.ListActive(ctx)
	if err != nil {
		return dedupeBuckets(buckets), nil
	}
	for _, group := range groups {
		if group.Platform == "" {
			continue
		}
		buckets = append(buckets, SchedulerBucket{GroupID: group.ID, Platform: group.Platform, Mode: SchedulerModeSingle})
		buckets = append(buckets, SchedulerBucket{GroupID: group.ID, Platform: group.Platform, Mode: SchedulerModeForced})
		if group.Platform == PlatformAnthropic || group.Platform == PlatformGemini {
			buckets = append(buckets, SchedulerBucket{GroupID: group.ID, Platform: group.Platform, Mode: SchedulerModeMixed})
		}
	}
	return dedupeBuckets(buckets), nil
}

func dedupeBuckets(in []SchedulerBucket) []SchedulerBucket {
	seen := make(map[string]struct{}, len(in))
	out := make([]SchedulerBucket, 0, len(in))
	for _, bucket := range in {
		key := bucket.String()
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, bucket)
	}
	return out
}

func derefAccounts(accounts []*Account) []Account {
	if len(accounts) == 0 {
		return []Account{}
	}
	out := make([]Account, 0, len(accounts))
	for _, account := range accounts {
		if account == nil {
			continue
		}
		out = append(out, *account)
	}
	return out
}

func parseInt64Slice(value any) []int64 {
	raw, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]int64, 0, len(raw))
	for _, item := range raw {
		if v, ok := toInt64(item); ok && v > 0 {
			out = append(out, v)
		}
	}
	return out
}

func toInt64(value any) (int64, bool) {
	switch v := value.(type) {
	case float64:
		return int64(v), true
	case int64:
		return v, true
	case int:
		return int64(v), true
	case json.Number:
		parsed, err := strconv.ParseInt(v.String(), 10, 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

type fallbackLimiter struct {
	maxQPS int
	mu     sync.Mutex
	window time.Time
	count  int
}

func newFallbackLimiter(maxQPS int) *fallbackLimiter {
	if maxQPS <= 0 {
		return nil
	}
	return &fallbackLimiter{
		maxQPS: maxQPS,
		window: time.Now(),
	}
}

func (l *fallbackLimiter) Allow() bool {
	if l == nil || l.maxQPS <= 0 {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	if now.Sub(l.window) >= time.Second {
		l.window = now
		l.count = 0
	}
	if l.count >= l.maxQPS {
		return false
	}
	l.count++
	return true
}
