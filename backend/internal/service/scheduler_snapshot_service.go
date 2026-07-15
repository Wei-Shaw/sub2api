package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

var (
	ErrSchedulerCacheNotReady           = errors.New("scheduler cache not ready")
	ErrSchedulerFallbackLimited         = errors.New("scheduler db fallback limited")
	ErrSchedulerGroupLifecycleLeaseBusy = errors.New("scheduler group lifecycle lease busy")
	ErrSchedulerBucketRebuildBusy       = errors.New("scheduler bucket rebuild busy")
)

const (
	outboxEventTimeout                    = 2 * time.Minute
	schedulerOutboxCleanupBatch           = 5000
	schedulerGroupLifecycleTimeout        = 30 * time.Second
	schedulerGroupLifecycleLeaseTTL       = 60 * time.Second
	schedulerGroupLifecycleReleaseTimeout = 2 * time.Second
	outboxRebuildRetryBaseDelay           = 5 * time.Second
	outboxRebuildRetryMaxDelay            = 5 * time.Minute
	outboxMaxIDErrorLogSampleInterval     = time.Minute
)

// batchSeenKey tracks completed per-platform rebuilds and group lifecycle work
// within one pollOutbox call.
type batchSeenKey struct {
	groupID   int64
	platform  string
	lifecycle bool
}

type schedulerBucketWriteTask struct {
	bucket SchedulerBucket
	token  SchedulerBucketWriteToken
}

type schedulerAccountQueryKey struct {
	groupID  int64
	platform string
}

// 查询结果只在一次 rebuild batch 内，按原始 groupID+platform 复用成功的 single/forced 查询；
// mixed 与历史模式保持独立。每个 task 都用 defer 消费 remaining，最后一个消费者会立即释放结果，
// 避免把账号切片的生命周期扩大到整轮 full rebuild。
type schedulerAccountQueryCache struct {
	remaining map[schedulerAccountQueryKey]int
	accounts  map[schedulerAccountQueryKey][]Account
}

func newSchedulerAccountQueryCache(taskSets ...[]schedulerBucketWriteTask) *schedulerAccountQueryCache {
	queries := &schedulerAccountQueryCache{
		remaining: make(map[schedulerAccountQueryKey]int),
		accounts:  make(map[schedulerAccountQueryKey][]Account),
	}
	for _, tasks := range taskSets {
		for _, task := range tasks {
			if key, ok := schedulerAccountQueryKeyForBucket(task.bucket); ok {
				queries.remaining[key]++
			}
		}
	}
	return queries
}

func schedulerAccountQueryKeyForBucket(bucket SchedulerBucket) (schedulerAccountQueryKey, bool) {
	if bucket.Mode != SchedulerModeSingle && bucket.Mode != SchedulerModeForced {
		return schedulerAccountQueryKey{}, false
	}
	return schedulerAccountQueryKey{groupID: bucket.GroupID, platform: bucket.Platform}, true
}

func (c *schedulerAccountQueryCache) release(bucket SchedulerBucket) {
	if c == nil {
		return
	}
	key, ok := schedulerAccountQueryKeyForBucket(bucket)
	if !ok {
		return
	}
	remaining := c.remaining[key] - 1
	if remaining <= 0 {
		delete(c.remaining, key)
		delete(c.accounts, key)
		return
	}
	c.remaining[key] = remaining
}

type schedulerGroupLifecyclePlan struct {
	active bool
	tasks  []schedulerBucketWriteTask
}

type schedulerActiveGroupIDLister interface {
	ListActiveIDs(ctx context.Context) ([]int64, error)
}

type SchedulerSnapshotService struct {
	cache                        SchedulerCache
	outboxRepo                   SchedulerOutboxRepository
	accountRepo                  AccountRepository
	groupRepo                    GroupRepository
	cfg                          *config.Config
	stopCh                       chan struct{}
	stopOnce                     sync.Once
	wg                           sync.WaitGroup
	fallbackLimit                *fallbackLimiter
	lagMu                        sync.Mutex
	lagFailures                  int
	outboxRebuildLatched         bool
	outboxRebuildRunning         bool
	outboxRebuildFailures        int
	outboxRebuildRetryAt         time.Time
	outboxRebuildRetryReason     string
	outboxLagWarningActive       bool
	outboxMaxIDErrorLastLoggedAt time.Time

	fullRebuildRunMu     sync.Mutex
	fullRebuildStateMu   sync.Mutex
	fullRebuildRequested uint64
	fullRebuildCompleted uint64
	fullRebuildLastErr   error
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
		cache:         cache,
		outboxRepo:    outboxRepo,
		accountRepo:   accountRepo,
		groupRepo:     groupRepo,
		cfg:           cfg,
		stopCh:        make(chan struct{}),
		fallbackLimit: newFallbackLimiter(maxQPS),
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
	var writeToken SchedulerBucketWriteToken
	canPublish := false

	if s.cache != nil {
		cached, hit, err := s.cache.GetSnapshot(ctx, bucket)
		if err != nil {
			logger.LegacyPrintf("service.scheduler_snapshot", "[Scheduler] cache read failed: bucket=%s err=%v", bucket.String(), err)
		} else if hit {
			return derefAccounts(cached), useMixed, nil
		}
		token, err := s.cache.CaptureBucketWriteToken(ctx, bucket)
		if err != nil {
			if errors.Is(err, ErrSchedulerBucketRetired) || errors.Is(err, ErrSchedulerBucketWriteFenced) {
				slog.Debug("[Scheduler] cache publish fenced", "bucket", bucket.String())
			} else {
				logger.LegacyPrintf("service.scheduler_snapshot", "[Scheduler] cache publish token failed: bucket=%s err=%v", bucket.String(), err)
			}
		} else {
			writeToken = token
			canPublish = true
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

	if s.cache != nil && canPublish {
		if err := s.cache.SetSnapshot(fallbackCtx, bucket, writeToken, accounts); err != nil {
			if errors.Is(err, ErrSchedulerBucketRetired) || errors.Is(err, ErrSchedulerBucketWriteFenced) {
				slog.Debug("[Scheduler] cache publish fenced", "bucket", bucket.String())
			} else {
				logger.LegacyPrintf("service.scheduler_snapshot", "[Scheduler] cache write failed: bucket=%s err=%v", bucket.String(), err)
			}
		}
	}

	return accounts, useMixed, nil
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
	_ = s.coalesceFullRebuild(func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if err := s.rebuildFullSnapshot(ctx, "startup"); err != nil {
			logger.LegacyPrintf("service.scheduler_snapshot", "[Scheduler] rebuild startup failed: %v", err)
			return err
		}
		return nil
	})
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
		// The outbox query itself proves there is no event after the watermark.
		// Clear degraded/retry state without adding two more repository queries to
		// the healthy one-second poll path.
		s.clearOutboxDegradedEpisode()
		return
	}

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
		return
	}
	s.cleanupConsumedOutbox(lastID)

	// 只有 watermark 成功推进后，当前批次才算已消费。延迟必须按下一条待消费事件计算，
	// 否则本批次处理越慢，越容易误触发一次更慢的全量重建，形成正反馈。
	lagCtx, lagCancel := context.WithTimeout(context.Background(), 5*time.Second)
	s.checkOutboxLag(lagCtx, lastID)
	lagCancel()
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
			if s.cache != nil {
				if err := s.cache.DeleteAccount(ctx, *accountID); err != nil {
					return err
				}
			}
			return s.rebuildByGroupIDs(ctx, groupIDs, "account_miss", seen)
		}
		return err
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

func (s *SchedulerSnapshotService) handleGroupEvent(ctx context.Context, groupID *int64, seen map[batchSeenKey]struct{}) error {
	if groupID == nil || *groupID <= 0 || s.isRunModeSimple() {
		return nil
	}
	if seen != nil {
		if _, ok := seen[batchSeenKey{groupID: *groupID, lifecycle: true}]; ok {
			return nil
		}
	}
	return s.reconcileGroupLifecycle(ctx, *groupID, seen)
}

func (s *SchedulerSnapshotService) reconcileGroupLifecycle(ctx context.Context, groupID int64, seen map[batchSeenKey]struct{}) error {
	plan, err := s.prepareGroupLifecycle(ctx, groupID, nil)
	if err != nil {
		return err
	}
	if plan.active {
		queries := newSchedulerAccountQueryCache(plan.tasks)
		for _, task := range plan.tasks {
			if err := s.rebuildBucketWithTokenPolicyAndQueryCache(ctx, task, "group_change", true, queries); err != nil {
				return err
			}
		}
	}
	markGroupLifecycleSeen(seen, groupID)
	return nil
}

// 生命周期决策必须在所有者安全的租约内读取 fresh 且完整的分组权威状态。
// active 仅 Reopen canonical bucket；missing/inactive 同时 Retire canonical 与已登记历史 bucket；
// group event 路径只有在权威决策和后续重建全部成功后才会标记 seen。
func (s *SchedulerSnapshotService) prepareGroupLifecycle(ctx context.Context, groupID int64, knownHistorical []SchedulerBucket) (plan schedulerGroupLifecyclePlan, retErr error) {
	if groupID <= 0 || s.isRunModeSimple() {
		return schedulerGroupLifecyclePlan{}, nil
	}
	if s.cache == nil || s.groupRepo == nil {
		return schedulerGroupLifecyclePlan{}, ErrSchedulerCacheNotReady
	}

	lifecycleCtx, cancel := context.WithTimeout(ctx, schedulerGroupLifecycleTimeout)
	defer cancel()
	lease, acquired, err := s.cache.TryAcquireGroupLifecycleLease(lifecycleCtx, groupID, schedulerGroupLifecycleLeaseTTL)
	if err != nil {
		return schedulerGroupLifecyclePlan{}, err
	}
	if !acquired {
		return schedulerGroupLifecyclePlan{}, fmt.Errorf("%w: group=%d", ErrSchedulerGroupLifecycleLeaseBusy, groupID)
	}
	leaseHeld := true
	defer func() {
		if leaseHeld {
			retErr = errors.Join(retErr, s.releaseGroupLifecycleLease(lease))
		}
	}()

	group, err := s.groupRepo.GetByIDLite(lifecycleCtx, groupID)
	missing := errors.Is(err, ErrGroupNotFound)
	if err != nil && !missing {
		return schedulerGroupLifecyclePlan{}, err
	}
	if err == nil && (group == nil || group.ID != groupID || !group.Hydrated) {
		return schedulerGroupLifecyclePlan{}, fmt.Errorf("untrusted scheduler group lifecycle state: group=%d", groupID)
	}

	plan = schedulerGroupLifecyclePlan{active: !missing && group.IsActive()}
	if plan.active {
		buckets := schedulerBucketsForGroup(groupID)
		plan.tasks = make([]schedulerBucketWriteTask, 0, len(buckets))
		for _, bucket := range buckets {
			token, err := s.cache.ReopenBucket(lifecycleCtx, bucket)
			if err != nil {
				return schedulerGroupLifecyclePlan{}, err
			}
			plan.tasks = append(plan.tasks, schedulerBucketWriteTask{bucket: bucket, token: token})
		}
	} else {
		registered := knownHistorical
		if registered == nil {
			registered, err = s.cache.ListBuckets(lifecycleCtx)
			if err != nil {
				return schedulerGroupLifecyclePlan{}, err
			}
		}
		buckets := schedulerBucketsForGroup(groupID)
		for _, bucket := range registered {
			if bucket.GroupID == groupID {
				buckets = append(buckets, bucket)
			}
		}
		for _, bucket := range dedupeBuckets(buckets) {
			if err := s.cache.RetireBucket(lifecycleCtx, bucket); err != nil {
				return schedulerGroupLifecyclePlan{}, err
			}
		}
	}

	releaseErr := s.releaseGroupLifecycleLease(lease)
	leaseHeld = false
	if releaseErr != nil {
		return schedulerGroupLifecyclePlan{}, releaseErr
	}
	return plan, nil
}

func (s *SchedulerSnapshotService) releaseGroupLifecycleLease(lease SchedulerGroupLifecycleLease) error {
	// 请求取消后仍需尝试释放自己的租约，因此使用独立且有界的后台上下文。
	releaseCtx, cancel := context.WithTimeout(context.Background(), schedulerGroupLifecycleReleaseTimeout)
	defer cancel()
	return s.cache.ReleaseGroupLifecycleLease(releaseCtx, lease)
}

func markGroupLifecycleSeen(seen map[batchSeenKey]struct{}, groupID int64) {
	if seen == nil {
		return
	}
	seen[batchSeenKey{groupID: groupID, lifecycle: true}] = struct{}{}
	for _, platform := range schedulerSnapshotPlatforms() {
		seen[batchSeenKey{groupID: groupID, platform: platform}] = struct{}{}
	}
}

func (s *SchedulerSnapshotService) rebuildByAccount(ctx context.Context, account *Account, groupIDs []int64, reason string, seen map[batchSeenKey]struct{}) error {
	if account == nil {
		return nil
	}
	groupIDs = s.normalizeGroupIDs(groupIDs)
	if len(groupIDs) == 0 {
		return nil
	}

	buckets := s.bucketsForPlatform(account.Platform, groupIDs, seen)
	if account.Platform == PlatformAntigravity && account.IsMixedSchedulingEnabled() {
		buckets = append(buckets, s.bucketsForPlatform(PlatformAnthropic, groupIDs, seen)...)
		buckets = append(buckets, s.bucketsForPlatform(PlatformGemini, groupIDs, seen)...)
	}
	return s.rebuildBuckets(ctx, buckets, reason)
}

func schedulerSnapshotPlatforms() [5]string {
	return [5]string{PlatformAnthropic, PlatformGemini, PlatformOpenAI, PlatformAntigravity, PlatformGrok}
}

// 生命周期辅助函数有意排除 group0；full rebuild 构造 group0 canonical 集时必须显式调用 canonical helper。
func schedulerBucketsForGroup(groupID int64) []SchedulerBucket {
	if groupID <= 0 {
		return nil
	}
	return schedulerCanonicalBuckets(groupID)
}

func schedulerCanonicalBuckets(groupID int64) []SchedulerBucket {
	buckets := make([]SchedulerBucket, 0, 12)
	for _, platform := range schedulerSnapshotPlatforms() {
		buckets = append(buckets,
			SchedulerBucket{GroupID: groupID, Platform: platform, Mode: SchedulerModeSingle},
			SchedulerBucket{GroupID: groupID, Platform: platform, Mode: SchedulerModeForced},
		)
		if platform == PlatformAnthropic || platform == PlatformGemini {
			buckets = append(buckets, SchedulerBucket{GroupID: groupID, Platform: platform, Mode: SchedulerModeMixed})
		}
	}
	return buckets
}

func (s *SchedulerSnapshotService) rebuildByGroupIDs(ctx context.Context, groupIDs []int64, reason string, seen map[batchSeenKey]struct{}) error {
	groupIDs = s.normalizeGroupIDs(groupIDs)
	if len(groupIDs) == 0 {
		return nil
	}
	buckets := make([]SchedulerBucket, 0, len(groupIDs)*12)
	for _, platform := range schedulerSnapshotPlatforms() {
		buckets = append(buckets, s.bucketsForPlatform(platform, groupIDs, seen)...)
	}
	return s.rebuildBuckets(ctx, buckets, reason)
}

func (s *SchedulerSnapshotService) bucketsForPlatform(platform string, groupIDs []int64, seen map[batchSeenKey]struct{}) []SchedulerBucket {
	if platform == "" {
		return nil
	}
	buckets := make([]SchedulerBucket, 0, len(groupIDs)*3)
	for _, gid := range groupIDs {
		// Within a single poll batch, skip (groupID, platform) pairs that were
		// already rebuilt. The first rebuild loads fresh DB data for all accounts
		// in the group, so subsequent rebuilds for the same group+platform within
		// the same batch are redundant.
		if seen != nil {
			key := batchSeenKey{groupID: gid, platform: platform}
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
		}
		buckets = append(buckets, SchedulerBucket{GroupID: gid, Platform: platform, Mode: SchedulerModeSingle})
		buckets = append(buckets, SchedulerBucket{GroupID: gid, Platform: platform, Mode: SchedulerModeForced})
		if platform == PlatformAnthropic || platform == PlatformGemini {
			buckets = append(buckets, SchedulerBucket{GroupID: gid, Platform: platform, Mode: SchedulerModeMixed})
		}
	}
	return buckets
}

func (s *SchedulerSnapshotService) rebuildBuckets(ctx context.Context, buckets []SchedulerBucket, reason string) error {
	tasks, firstErr := s.prepareBucketWriteTasks(ctx, buckets)
	queries := newSchedulerAccountQueryCache(tasks)
	if err := s.rebuildPreparedBucketTasks(ctx, tasks, reason, false, queries); err != nil && firstErr == nil {
		firstErr = err
	}
	return firstErr
}

func (s *SchedulerSnapshotService) prepareBucketWriteTasks(ctx context.Context, buckets []SchedulerBucket) ([]schedulerBucketWriteTask, error) {
	if s.cache == nil {
		return nil, ErrSchedulerCacheNotReady
	}
	tasks := make([]schedulerBucketWriteTask, 0, len(buckets))
	var firstErr error
	for _, bucket := range buckets {
		token, err := s.cache.CaptureBucketWriteToken(ctx, bucket)
		if err != nil {
			if errors.Is(err, ErrSchedulerBucketRetired) || errors.Is(err, ErrSchedulerBucketWriteFenced) {
				continue
			}
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		tasks = append(tasks, schedulerBucketWriteTask{bucket: bucket, token: token})
	}
	return tasks, firstErr
}

func (s *SchedulerSnapshotService) rebuildPreparedBucketTasks(
	ctx context.Context,
	tasks []schedulerBucketWriteTask,
	reason string,
	strict bool,
	queries *schedulerAccountQueryCache,
) error {
	var firstErr error
	for _, task := range tasks {
		if err := s.rebuildBucketWithTokenPolicyAndQueryCache(ctx, task, reason, strict, queries); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (s *SchedulerSnapshotService) rebuildBucketWithTokenPolicyAndQueryCache(
	ctx context.Context,
	task schedulerBucketWriteTask,
	reason string,
	strict bool,
	queries *schedulerAccountQueryCache,
) error {
	if queries != nil {
		defer queries.release(task.bucket)
	}
	if s.cache == nil {
		return ErrSchedulerCacheNotReady
	}
	bucket := task.bucket
	ok, err := s.cache.TryLockBucket(ctx, bucket, 30*time.Second)
	if err != nil {
		return err
	}
	if !ok {
		if strict {
			return fmt.Errorf("%w: bucket=%s", ErrSchedulerBucketRebuildBusy, bucket.String())
		}
		return nil
	}
	defer func() {
		_ = s.cache.UnlockBucket(ctx, bucket)
	}()

	rebuildCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	accounts, err := s.loadAccountsForRebuild(rebuildCtx, bucket, queries)
	if err != nil {
		logger.LegacyPrintf("service.scheduler_snapshot", "[Scheduler] rebuild failed: bucket=%s reason=%s err=%v", bucket.String(), reason, err)
		return err
	}
	if err := s.cache.SetSnapshot(rebuildCtx, bucket, task.token, accounts); err != nil {
		if errors.Is(err, ErrSchedulerBucketRetired) || errors.Is(err, ErrSchedulerBucketWriteFenced) {
			slog.Debug("[Scheduler] rebuild fenced", "bucket", bucket.String(), "reason", reason)
			if strict {
				return err
			}
			return nil
		}
		logger.LegacyPrintf("service.scheduler_snapshot", "[Scheduler] rebuild cache failed: bucket=%s reason=%s err=%v", bucket.String(), reason, err)
		return err
	}
	slog.Debug("[Scheduler] rebuild ok", "bucket", bucket.String(), "reason", reason, "size", len(accounts))
	return nil
}

func (s *SchedulerSnapshotService) triggerFullRebuild(reason string) error {
	if s.cache == nil {
		return ErrSchedulerCacheNotReady
	}
	return s.coalesceFullRebuild(func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		return s.rebuildFullSnapshot(ctx, reason)
	})
}

func (s *SchedulerSnapshotService) rebuildFullSnapshot(ctx context.Context, reason string) error {
	if s.cache == nil {
		return ErrSchedulerCacheNotReady
	}

	// 当前模式所需的全局读取必须先成功：桶注册表始终必需，standard 还需活跃分组 ID；
	// 失败时不执行 Capture/Retire/Reopen 或 DB 查询。
	// simple 模式不获取分组生命周期权威；standard 的 stale candidate 仍须在租约内 fresh 确认后才能退休。
	registered, err := s.cache.ListBuckets(ctx)
	if err != nil {
		return err
	}
	registered = dedupeBuckets(registered)

	if s.isRunModeSimple() {
		canonical := schedulerCanonicalBuckets(0)
		captured, err := s.captureFullRebuildCanonicalTasks(ctx, canonical)
		if err != nil {
			return err
		}
		ordinary := appendBucketsExcept(nil, registered, canonical)
		return s.prepareAndRebuildFullSnapshot(ctx, captured, nil, ordinary, reason)
	}

	activeGroupIDs, err := s.listActiveSchedulerGroupIDs(ctx)
	if err != nil {
		return err
	}
	activeGroups := make(map[int64]struct{}, len(activeGroupIDs))
	for _, groupID := range activeGroupIDs {
		activeGroups[groupID] = struct{}{}
	}

	registeredByGroup := make(map[int64][]SchedulerBucket)
	for _, bucket := range registered {
		registeredByGroup[bucket.GroupID] = append(registeredByGroup[bucket.GroupID], bucket)
	}

	groupZeroCanonical := schedulerCanonicalBuckets(0)
	capturedTasks, err := s.captureFullRebuildCanonicalTasks(ctx, groupZeroCanonical)
	if err != nil {
		return err
	}
	ordinaryBuckets := appendBucketsExcept(nil, registeredByGroup[0], groupZeroCanonical)
	for groupID, buckets := range registeredByGroup {
		if groupID < 0 {
			ordinaryBuckets = append(ordinaryBuckets, buckets...)
		}
	}

	reopenedTasks := make([]schedulerBucketWriteTask, 0)
	for _, groupID := range activeGroupIDs {
		canonical := schedulerBucketsForGroup(groupID)
		canonicalTasks, captureErr := s.captureFullRebuildCanonicalTasks(ctx, canonical)
		if captureErr == nil {
			capturedTasks = append(capturedTasks, canonicalTasks...)
			ordinaryBuckets = appendBucketsExcept(ordinaryBuckets, registeredByGroup[groupID], canonical)
			continue
		}
		if !errors.Is(captureErr, ErrSchedulerBucketRetired) && !errors.Is(captureErr, ErrSchedulerBucketWriteFenced) {
			return captureErr
		}

		// A prior full_rebuild event can observe the active state committed for a
		// later group_changed event. Recover here under fresh authority so the
		// earlier event cannot block the outbox watermark before that event runs.
		knownHistorical := registeredByGroup[groupID]
		if knownHistorical == nil {
			knownHistorical = []SchedulerBucket{}
		}
		plan, err := s.prepareGroupLifecycle(ctx, groupID, knownHistorical)
		if err != nil {
			return err
		}
		if plan.active {
			reopenedTasks = append(reopenedTasks, plan.tasks...)
			ordinaryBuckets = appendBucketsExcept(ordinaryBuckets, registeredByGroup[groupID], canonical)
		}
	}

	staleGroupIDs := make([]int64, 0)
	for groupID := range registeredByGroup {
		if groupID <= 0 {
			continue
		}
		if _, active := activeGroups[groupID]; !active {
			staleGroupIDs = append(staleGroupIDs, groupID)
		}
	}
	sort.Slice(staleGroupIDs, func(i, j int) bool { return staleGroupIDs[i] < staleGroupIDs[j] })

	for _, groupID := range staleGroupIDs {
		plan, err := s.prepareGroupLifecycle(ctx, groupID, registeredByGroup[groupID])
		if err != nil {
			return err
		}
		if plan.active {
			reopenedTasks = append(reopenedTasks, plan.tasks...)
			ordinaryBuckets = appendBucketsExcept(ordinaryBuckets, registeredByGroup[groupID], schedulerBucketsForGroup(groupID))
		}
	}

	return s.prepareAndRebuildFullSnapshot(ctx, capturedTasks, reopenedTasks, ordinaryBuckets, reason)
}

func (s *SchedulerSnapshotService) listActiveSchedulerGroupIDs(ctx context.Context) ([]int64, error) {
	if s.groupRepo == nil {
		return nil, ErrSchedulerCacheNotReady
	}

	// 轻量接口一旦实现，其错误直接失败；只有仓储不支持该接口时才回退完整 ListActive。
	var groupIDs []int64
	if lister, ok := s.groupRepo.(schedulerActiveGroupIDLister); ok {
		ids, err := lister.ListActiveIDs(ctx)
		if err != nil {
			return nil, err
		}
		groupIDs = ids
	} else {
		groups, err := s.groupRepo.ListActive(ctx)
		if err != nil {
			return nil, err
		}
		groupIDs = make([]int64, 0, len(groups))
		for _, group := range groups {
			groupIDs = append(groupIDs, group.ID)
		}
	}

	seen := make(map[int64]struct{}, len(groupIDs))
	normalized := make([]int64, 0, len(groupIDs))
	for _, groupID := range groupIDs {
		if groupID <= 0 {
			continue
		}
		if _, ok := seen[groupID]; ok {
			continue
		}
		seen[groupID] = struct{}{}
		normalized = append(normalized, groupID)
	}
	sort.Slice(normalized, func(i, j int) bool { return normalized[i] < normalized[j] })
	return normalized, nil
}

func (s *SchedulerSnapshotService) prepareAndRebuildFullSnapshot(
	ctx context.Context,
	captured []schedulerBucketWriteTask,
	reopened []schedulerBucketWriteTask,
	ordinaryBuckets []SchedulerBucket,
	reason string,
) error {
	// 首个 DB 查询前必须完成全部普通 bucket 的 token 预备；任何预备错误都不会留下部分发布。
	// fresh Reopen task 保持严格锁与 fencing 语义，普通 captured task 继续沿用 lock busy/fence 跳过语义。
	preparedBuckets := make(map[SchedulerBucket]struct{}, len(captured)+len(reopened))
	for _, task := range captured {
		preparedBuckets[task.bucket] = struct{}{}
	}
	for _, task := range reopened {
		preparedBuckets[task.bucket] = struct{}{}
	}

	ordinaryBuckets = dedupeBuckets(ordinaryBuckets)
	toCapture := make([]SchedulerBucket, 0, len(ordinaryBuckets))
	for _, bucket := range ordinaryBuckets {
		if _, ok := preparedBuckets[bucket]; !ok {
			toCapture = append(toCapture, bucket)
		}
	}
	ordinary, firstErr := s.prepareBucketWriteTasks(ctx, toCapture)
	if firstErr != nil {
		return firstErr
	}
	captured = append(captured, ordinary...)
	queries := newSchedulerAccountQueryCache(reopened, captured)
	if err := s.rebuildPreparedBucketTasks(ctx, reopened, reason, true, queries); err != nil {
		firstErr = err
	}
	if err := s.rebuildPreparedBucketTasks(ctx, captured, reason, false, queries); err != nil && firstErr == nil {
		firstErr = err
	}
	return firstErr
}

func (s *SchedulerSnapshotService) captureFullRebuildCanonicalTasks(ctx context.Context, buckets []SchedulerBucket) ([]schedulerBucketWriteTask, error) {
	if s.cache == nil {
		return nil, ErrSchedulerCacheNotReady
	}
	tasks := make([]schedulerBucketWriteTask, 0, len(buckets))
	for _, bucket := range buckets {
		token, err := s.cache.CaptureBucketWriteToken(ctx, bucket)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, schedulerBucketWriteTask{bucket: bucket, token: token})
	}
	return tasks, nil
}

func appendBucketsExcept(dst, buckets, excluded []SchedulerBucket) []SchedulerBucket {
	excludedKeys := make(map[SchedulerBucket]struct{}, len(excluded))
	for _, bucket := range excluded {
		excludedKeys[bucket] = struct{}{}
	}
	for _, bucket := range buckets {
		if _, ok := excludedKeys[bucket]; !ok {
			dst = append(dst, bucket)
		}
	}
	return dst
}

func (s *SchedulerSnapshotService) coalesceFullRebuild(run func() error) error {
	s.fullRebuildStateMu.Lock()
	s.fullRebuildRequested++
	requestID := s.fullRebuildRequested
	s.fullRebuildStateMu.Unlock()

	s.fullRebuildRunMu.Lock()
	defer s.fullRebuildRunMu.Unlock()

	s.fullRebuildStateMu.Lock()
	if s.fullRebuildCompleted >= requestID {
		err := s.fullRebuildLastErr
		s.fullRebuildStateMu.Unlock()
		return err
	}
	// 当前轮重建可能早于新 outbox 事件对应事务的提交，不能让后到请求直接复用当前轮。
	// 每轮开始前记录可覆盖的请求代次，执行期间登记的请求统一合并到下一轮。
	coveredThrough := s.fullRebuildRequested
	s.fullRebuildStateMu.Unlock()

	err := run()

	s.fullRebuildStateMu.Lock()
	s.fullRebuildCompleted = coveredThrough
	s.fullRebuildLastErr = err
	s.fullRebuildStateMu.Unlock()
	return err
}

func (s *SchedulerSnapshotService) checkOutboxLag(ctx context.Context, watermark int64) {
	if s.cfg == nil || s.outboxRepo == nil {
		return
	}
	now := time.Now()
	oldestCreatedAt, ok, err := s.outboxRepo.FirstCreatedAtAfter(ctx, watermark)
	if err != nil {
		logger.LegacyPrintf("service.scheduler_snapshot", "[Scheduler] outbox pending event read failed: %v", err)
		return
	}
	var lag time.Duration
	if ok && !oldestCreatedAt.IsZero() {
		lag = now.Sub(oldestCreatedAt)
	}
	lagSeconds := int(lag.Seconds())
	lagWarning := ok && !oldestCreatedAt.IsZero() &&
		s.cfg.Gateway.Scheduling.OutboxLagWarnSeconds > 0 &&
		lagSeconds >= s.cfg.Gateway.Scheduling.OutboxLagWarnSeconds

	lagDegraded := ok && !oldestCreatedAt.IsZero() &&
		s.cfg.Gateway.Scheduling.OutboxLagRebuildSeconds > 0 &&
		lagSeconds >= s.cfg.Gateway.Scheduling.OutboxLagRebuildSeconds

	backlogThreshold := s.cfg.Gateway.Scheduling.OutboxBacklogRebuildRows
	backlogKnown := true
	var backlog int64
	if backlogThreshold > 0 {
		maxID, maxErr := s.outboxRepo.MaxID(ctx)
		if maxErr != nil {
			backlogKnown = false
			if s.shouldLogOutboxMaxIDError(now) {
				logger.LegacyPrintf("service.scheduler_snapshot", "[Scheduler] outbox max id read failed: %v", maxErr)
			}
		} else {
			backlog = maxID - watermark
		}
	}
	backlogDegraded := backlogKnown && backlogThreshold > 0 && backlog >= int64(backlogThreshold)

	// A successful rebuild latches the degraded episode until recovery. A failed
	// rebuild remains retryable, but only after an exponentially backed-off
	// cooldown so a one-second poll cannot create a rebuild storm.
	logLagWarning := s.shouldLogOutboxLagWarning(lagWarning)
	s.lagMu.Lock()
	fullyRecovered := !lagDegraded && backlogKnown && !backlogDegraded
	if fullyRecovered {
		s.lagFailures = 0
		s.outboxRebuildLatched = false
		s.outboxRebuildFailures = 0
		s.outboxRebuildRetryAt = time.Time{}
		s.outboxRebuildRetryReason = ""
	}

	if s.outboxRebuildRetryReason != "" {
		retryReasonActive := (s.outboxRebuildRetryReason == "outbox_lag" && lagDegraded) ||
			(s.outboxRebuildRetryReason == "outbox_backlog" && (!backlogKnown || backlogDegraded))
		if !retryReasonActive {
			s.outboxRebuildFailures = 0
			s.outboxRebuildRetryAt = time.Time{}
			s.outboxRebuildRetryReason = ""
		}
	}

	lagRetryPending := s.outboxRebuildRetryReason == "outbox_lag" && !s.outboxRebuildRetryAt.IsZero()
	if lagDegraded {
		if !s.outboxRebuildLatched && !s.outboxRebuildRunning && !lagRetryPending {
			s.lagFailures++
		}
	} else {
		s.lagFailures = 0
	}
	failures := s.lagFailures
	lagReady := lagDegraded && failures >= s.cfg.Gateway.Scheduling.OutboxLagRebuildFailures
	retryDue := s.outboxRebuildRetryReason != "" &&
		!s.outboxRebuildRetryAt.IsZero() && !now.Before(s.outboxRebuildRetryAt)

	reason := ""
	lagCanPreemptRetry := lagReady && s.outboxRebuildRetryReason != "outbox_lag"
	if !s.outboxRebuildLatched && !s.outboxRebuildRunning &&
		(s.outboxRebuildRetryAt.IsZero() || retryDue || lagCanPreemptRetry) {
		switch {
		case lagReady || (retryDue && s.outboxRebuildRetryReason == "outbox_lag" && lagDegraded):
			if s.outboxRebuildRetryReason != "" && s.outboxRebuildRetryReason != "outbox_lag" {
				s.outboxRebuildFailures = 0
				s.outboxRebuildRetryAt = time.Time{}
				s.outboxRebuildRetryReason = ""
			}
			reason = "outbox_lag"
			s.lagFailures = 0
		case backlogDegraded && (s.outboxRebuildRetryReason == "" ||
			(retryDue && s.outboxRebuildRetryReason == "outbox_backlog")):
			reason = "outbox_backlog"
		}
		if reason != "" {
			s.outboxRebuildRunning = true
		}
	}
	s.lagMu.Unlock()

	if logLagWarning {
		logger.LegacyPrintf("service.scheduler_snapshot", "[Scheduler] outbox lag warning: %ds", lagSeconds)
	}

	if reason == "" {
		return
	}

	var rebuildErr error
	switch reason {
	case "outbox_lag":
		logger.LegacyPrintf("service.scheduler_snapshot", "[Scheduler] outbox lag rebuild triggered: lag=%s failures=%d", lag, failures)
		rebuildErr = s.triggerFullRebuild(reason)
	case "outbox_backlog":
		logger.LegacyPrintf("service.scheduler_snapshot", "[Scheduler] outbox backlog rebuild triggered: backlog=%d", backlog)
		rebuildErr = s.triggerFullRebuild(reason)
	}

	s.lagMu.Lock()
	s.outboxRebuildRunning = false
	if rebuildErr == nil {
		s.outboxRebuildLatched = true
		s.outboxRebuildFailures = 0
		s.outboxRebuildRetryAt = time.Time{}
		s.outboxRebuildRetryReason = ""
	} else {
		s.outboxRebuildLatched = false
		s.outboxRebuildFailures++
		s.outboxRebuildRetryAt = time.Now().Add(outboxRebuildRetryDelay(s.outboxRebuildFailures))
		s.outboxRebuildRetryReason = reason
	}
	s.lagMu.Unlock()

	if rebuildErr != nil {
		logger.LegacyPrintf("service.scheduler_snapshot", "[Scheduler] %s rebuild failed: %v", reason, rebuildErr)
	}
}

func outboxRebuildRetryDelay(failures int) time.Duration {
	delay := outboxRebuildRetryBaseDelay
	for i := 1; i < failures && delay < outboxRebuildRetryMaxDelay; i++ {
		delay *= 2
		if delay >= outboxRebuildRetryMaxDelay {
			return outboxRebuildRetryMaxDelay
		}
	}
	return delay
}

func (s *SchedulerSnapshotService) clearOutboxDegradedEpisode() {
	if s == nil {
		return
	}
	s.lagMu.Lock()
	if s.lagFailures != 0 || s.outboxRebuildLatched || s.outboxRebuildRunning ||
		s.outboxRebuildFailures != 0 || !s.outboxRebuildRetryAt.IsZero() ||
		s.outboxRebuildRetryReason != "" || s.outboxLagWarningActive {
		s.lagFailures = 0
		s.outboxRebuildLatched = false
		s.outboxRebuildFailures = 0
		s.outboxRebuildRetryAt = time.Time{}
		s.outboxRebuildRetryReason = ""
		s.outboxLagWarningActive = false
	}
	s.lagMu.Unlock()
}

func (s *SchedulerSnapshotService) shouldLogOutboxMaxIDError(now time.Time) bool {
	s.lagMu.Lock()
	defer s.lagMu.Unlock()
	if !s.outboxMaxIDErrorLastLoggedAt.IsZero() && now.Sub(s.outboxMaxIDErrorLastLoggedAt) < outboxMaxIDErrorLogSampleInterval {
		return false
	}
	s.outboxMaxIDErrorLastLoggedAt = now
	return true
}

func (s *SchedulerSnapshotService) shouldLogOutboxLagWarning(active bool) bool {
	s.lagMu.Lock()
	defer s.lagMu.Unlock()
	shouldLog := active && !s.outboxLagWarningActive
	s.outboxLagWarningActive = active
	return shouldLog
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

func (s *SchedulerSnapshotService) loadAccountsForRebuild(
	ctx context.Context,
	bucket SchedulerBucket,
	queries *schedulerAccountQueryCache,
) ([]Account, error) {
	key, cacheable := schedulerAccountQueryKeyForBucket(bucket)
	if queries == nil || !cacheable {
		return s.loadAccountsFromDB(ctx, bucket, bucket.Mode == SchedulerModeMixed)
	}

	if accounts, ok := queries.accounts[key]; ok {
		return accounts, nil
	}
	if queries.remaining[key] <= 1 {
		return s.loadAccountsFromDB(ctx, bucket, false)
	}
	accounts, err := s.loadAccountsFromDB(ctx, bucket, false)
	if err != nil {
		return nil, err
	}
	queries.accounts[key] = accounts
	return accounts, nil
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
