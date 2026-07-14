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
)

// batchSeenKey tracks completed per-platform rebuilds and group lifecycle work
// within one pollOutbox call.
type batchSeenKey struct {
	groupID   int64
	platform  string
	lifecycle bool
REDACTED

type schedulerBucketWriteTask struct {
	bucket SchedulerBucket
	token  SchedulerBucketWriteToken
REDACTED

type schedulerGroupLifecyclePlan struct {
	active bool
	tasks  []schedulerBucketWriteTask
REDACTED

type schedulerActiveGroupIDLister interface {
	ListActiveIDs(ctx context.Context) ([]int64, error)
REDACTED

type SchedulerSnapshotService struct {
	cache         SchedulerCache
	outboxRepo    SchedulerOutboxRepository
	accountRepo   AccountRepository
	groupRepo     GroupRepository
	cfg           *config.Config
	stopCh        chan struct{REDACTED
	stopOnce      sync.Once
	wg            sync.WaitGroup
	fallbackLimit *fallbackLimiter
	lagMu         sync.Mutex
	lagFailures   int

	fullRebuildRunMu     sync.Mutex
	fullRebuildStateMu   sync.Mutex
	fullRebuildRequested uint64
	fullRebuildCompleted uint64
	fullRebuildLastErr   error
REDACTED

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
REDACTED
	return &SchedulerSnapshotService{
		cache:         cache,
		outboxRepo:    outboxRepo,
		accountRepo:   accountRepo,
		groupRepo:     groupRepo,
		cfg:           cfg,
		stopCh:        make(chan struct{REDACTED),
		fallbackLimit: newFallbackLimiter(maxQPS),
REDACTED
REDACTED

func (s *SchedulerSnapshotService) Start() {
	if s == nil || s.cache == nil {
		return
REDACTED

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.runInitialRebuild()
REDACTED()

	interval := s.outboxPollInterval()
	if s.outboxRepo != nil && interval > 0 {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.runOutboxWorker(interval)
	REDACTED()
REDACTED

	fullInterval := s.fullRebuildInterval()
	if fullInterval > 0 {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.runFullRebuildWorker(fullInterval)
	REDACTED()
REDACTED
REDACTED

func (s *SchedulerSnapshotService) Stop() {
	if s == nil {
		return
REDACTED
	s.stopOnce.Do(func() {
		close(s.stopCh)
REDACTED)
	s.wg.Wait()
REDACTED

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
	REDACTED else if hit {
			return derefAccounts(cached), useMixed, nil
	REDACTED
		token, err := s.cache.CaptureBucketWriteToken(ctx, bucket)
		if err != nil {
			if errors.Is(err, ErrSchedulerBucketRetired) || errors.Is(err, ErrSchedulerBucketWriteFenced) {
				slog.Debug("[Scheduler] cache publish fenced", "bucket", bucket.String())
		REDACTED else {
				logger.LegacyPrintf("service.scheduler_snapshot", "[Scheduler] cache publish token failed: bucket=%s err=%v", bucket.String(), err)
		REDACTED
	REDACTED else {
			writeToken = token
			canPublish = true
	REDACTED
REDACTED

	if err := s.guardFallback(ctx); err != nil {
		return nil, useMixed, err
REDACTED

	fallbackCtx, cancel := s.withFallbackTimeout(ctx)
	defer cancel()

	accounts, err := s.loadAccountsFromDB(fallbackCtx, bucket, useMixed)
	if err != nil {
		return nil, useMixed, err
REDACTED

	if s.cache != nil && canPublish {
		if err := s.cache.SetSnapshot(fallbackCtx, bucket, writeToken, accounts); err != nil {
			if errors.Is(err, ErrSchedulerBucketRetired) || errors.Is(err, ErrSchedulerBucketWriteFenced) {
				slog.Debug("[Scheduler] cache publish fenced", "bucket", bucket.String())
		REDACTED else {
				logger.LegacyPrintf("service.scheduler_snapshot", "[Scheduler] cache write failed: bucket=%s err=%v", bucket.String(), err)
		REDACTED
	REDACTED
REDACTED

	return accounts, useMixed, nil
REDACTED

func (s *SchedulerSnapshotService) GetAccount(ctx context.Context, accountID int64) (*Account, error) {
	if accountID <= 0 {
		return nil, nil
REDACTED
	if s.cache != nil {
		account, err := s.cache.GetAccount(ctx, accountID)
		if err != nil {
			logger.LegacyPrintf("service.scheduler_snapshot", "[Scheduler] account cache read failed: id=%d err=%v", accountID, err)
	REDACTED else if account != nil {
			return account, nil
	REDACTED
REDACTED

	if err := s.guardFallback(ctx); err != nil {
		return nil, err
REDACTED
	fallbackCtx, cancel := s.withFallbackTimeout(ctx)
	defer cancel()
	return s.accountRepo.GetByID(fallbackCtx, accountID)
REDACTED

// GetGroupByID 获取分组信息（供调度器使用）
func (s *SchedulerSnapshotService) GetGroupByID(ctx context.Context, groupID int64) (*Group, error) {
	if s.groupRepo == nil {
		return nil, nil
REDACTED
	return s.groupRepo.GetByID(ctx, groupID)
REDACTED

// UpdateAccountInCache 立即更新 Redis 中单个账号的数据（用于模型限流后立即生效）
func (s *SchedulerSnapshotService) UpdateAccountInCache(ctx context.Context, account *Account) error {
	if s.cache == nil || account == nil {
		return nil
REDACTED
	return s.cache.SetAccount(ctx, account)
REDACTED

func (s *SchedulerSnapshotService) runInitialRebuild() {
	if s.cache == nil {
		return
REDACTED
	_ = s.coalesceFullRebuild(func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if err := s.rebuildFullSnapshot(ctx, "startup"); err != nil {
			logger.LegacyPrintf("service.scheduler_snapshot", "[Scheduler] rebuild startup failed: %v", err)
			return err
	REDACTED
		return nil
REDACTED)
REDACTED

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
	REDACTED
REDACTED
REDACTED

func (s *SchedulerSnapshotService) runFullRebuildWorker(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := s.triggerFullRebuild("interval"); err != nil {
				logger.LegacyPrintf("service.scheduler_snapshot", "[Scheduler] full rebuild failed: %v", err)
		REDACTED
		case <-s.stopCh:
			return
	REDACTED
REDACTED
REDACTED

func (s *SchedulerSnapshotService) pollOutbox() {
	if s.outboxRepo == nil || s.cache == nil {
		return
REDACTED
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	watermark, err := s.cache.GetOutboxWatermark(ctx)
	if err != nil {
		logger.LegacyPrintf("service.scheduler_snapshot", "[Scheduler] outbox watermark read failed: %v", err)
		return
REDACTED

	events, err := s.outboxRepo.ListAfterAndReleaseDedup(ctx, watermark, 200)
	if err != nil {
		logger.LegacyPrintf("service.scheduler_snapshot", "[Scheduler] outbox poll failed: %v", err)
		return
REDACTED
	if len(events) == 0 {
		return
REDACTED

	seen := make(map[batchSeenKey]struct{REDACTED)
	for _, event := range events {
		eventCtx, cancel := context.WithTimeout(context.Background(), outboxEventTimeout)
		err := s.handleOutboxEvent(eventCtx, event, seen)
		cancel()
		if err != nil {
			logger.LegacyPrintf("service.scheduler_snapshot", "[Scheduler] outbox handle failed: id=%d type=%s err=%v", event.ID, event.EventType, err)
			return
	REDACTED
REDACTED

	lastID := events[len(events)-1].ID
	var wmErr error
	for i := range 3 {
		wmCtx, wmCancel := context.WithTimeout(context.Background(), 5*time.Second)
		wmErr = s.cache.SetOutboxWatermark(wmCtx, lastID)
		wmCancel()
		if wmErr == nil {
			break
	REDACTED
		if i < 2 {
			time.Sleep(200 * time.Millisecond)
	REDACTED
REDACTED
	if wmErr != nil {
		logger.LegacyPrintf("service.scheduler_snapshot", "[Scheduler] outbox watermark write failed: %v", wmErr)
		return
REDACTED
	s.cleanupConsumedOutbox(lastID)

	// 只有 watermark 成功推进后，当前批次才算已消费。延迟必须按下一条待消费事件计算，
	// 否则本批次处理越慢，越容易误触发一次更慢的全量重建，形成正反馈。
	lagCtx, lagCancel := context.WithTimeout(context.Background(), 5*time.Second)
	s.checkOutboxLag(lagCtx, lastID)
	lagCancel()
REDACTED

func (s *SchedulerSnapshotService) cleanupConsumedOutbox(watermark int64) {
	if s == nil || s.outboxRepo == nil || watermark <= 0 {
		return
REDACTED

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	lease, acquired, err := s.outboxRepo.TryAcquireCleanupLock(ctx)
	if err != nil {
		logger.LegacyPrintf("service.scheduler_snapshot", "[Scheduler] outbox cleanup lock failed: %v", err)
		return
REDACTED
	if !acquired {
		return
REDACTED
	defer lease.Release()

	for {
		deleted, err := s.outboxRepo.DeleteConsumedUpTo(ctx, watermark, schedulerOutboxCleanupBatch)
		if err != nil {
			logger.LegacyPrintf("service.scheduler_snapshot", "[Scheduler] outbox cleanup failed: watermark=%d err=%v", watermark, err)
			return
	REDACTED
		if deleted == 0 || deleted < schedulerOutboxCleanupBatch {
			return
	REDACTED
REDACTED
REDACTED

func (s *SchedulerSnapshotService) handleOutboxEvent(ctx context.Context, event SchedulerOutboxEvent, seen map[batchSeenKey]struct{REDACTED) error {
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
REDACTED
REDACTED

func (s *SchedulerSnapshotService) handleLastUsedEvent(ctx context.Context, payload map[string]any) error {
	if s.cache == nil || payload == nil {
		return nil
REDACTED
	raw, ok := payload["last_used"].(map[string]any)
	if !ok || len(raw) == 0 {
		return nil
REDACTED
	updates := make(map[int64]time.Time, len(raw))
	for key, value := range raw {
		id, err := strconv.ParseInt(key, 10, 64)
		if err != nil || id <= 0 {
			continue
	REDACTED
		sec, ok := toInt64(value)
		if !ok || sec <= 0 {
			continue
	REDACTED
		updates[id] = time.Unix(sec, 0)
REDACTED
	if len(updates) == 0 {
		return nil
REDACTED
	return s.cache.UpdateLastUsed(ctx, updates)
REDACTED

func (s *SchedulerSnapshotService) handleBulkAccountEvent(ctx context.Context, payload map[string]any, seen map[batchSeenKey]struct{REDACTED) error {
	if payload == nil {
		return nil
REDACTED
	if s.accountRepo == nil {
		return nil
REDACTED

	rawIDs := parseInt64Slice(payload["account_ids"])
	if len(rawIDs) == 0 {
		return nil
REDACTED

	ids := make([]int64, 0, len(rawIDs))
	seenIDs := make(map[int64]struct{REDACTED, len(rawIDs))
	for _, id := range rawIDs {
		if id <= 0 {
			continue
	REDACTED
		if _, exists := seenIDs[id]; exists {
			continue
	REDACTED
		seenIDs[id] = struct{REDACTED{REDACTED
		ids = append(ids, id)
REDACTED
	if len(ids) == 0 {
		return nil
REDACTED

	preloadGroupIDs := parseInt64Slice(payload["group_ids"])
	accounts, err := s.accountRepo.GetByIDs(ctx, ids)
	if err != nil {
		return err
REDACTED

	found := make(map[int64]struct{REDACTED, len(accounts))
	rebuildGroupSet := make(map[int64]struct{REDACTED, len(preloadGroupIDs))
	for _, gid := range preloadGroupIDs {
		if gid > 0 {
			rebuildGroupSet[gid] = struct{REDACTED{REDACTED
	REDACTED
REDACTED

	for _, account := range accounts {
		if account == nil || account.ID <= 0 {
			continue
	REDACTED
		found[account.ID] = struct{REDACTED{REDACTED
		if s.cache != nil {
			if err := s.cache.SetAccount(ctx, account); err != nil {
				return err
		REDACTED
	REDACTED
		for _, gid := range account.GroupIDs {
			if gid > 0 {
				rebuildGroupSet[gid] = struct{REDACTED{REDACTED
		REDACTED
	REDACTED
REDACTED

	if s.cache != nil {
		for _, id := range ids {
			if _, ok := found[id]; ok {
				continue
		REDACTED
			if err := s.cache.DeleteAccount(ctx, id); err != nil {
				return err
		REDACTED
	REDACTED
REDACTED

	rebuildGroupIDs := make([]int64, 0, len(rebuildGroupSet))
	for gid := range rebuildGroupSet {
		rebuildGroupIDs = append(rebuildGroupIDs, gid)
REDACTED
	return s.rebuildByGroupIDs(ctx, rebuildGroupIDs, "account_bulk_change", seen)
REDACTED

func (s *SchedulerSnapshotService) handleAccountEvent(ctx context.Context, accountID *int64, payload map[string]any, seen map[batchSeenKey]struct{REDACTED) error {
	if accountID == nil || *accountID <= 0 {
		return nil
REDACTED
	if s.accountRepo == nil {
		return nil
REDACTED

	var groupIDs []int64
	if payload != nil {
		groupIDs = parseInt64Slice(payload["group_ids"])
REDACTED

	account, err := s.accountRepo.GetByID(ctx, *accountID)
	if err != nil {
		if errors.Is(err, ErrAccountNotFound) {
			if s.cache != nil {
				if err := s.cache.DeleteAccount(ctx, *accountID); err != nil {
					return err
			REDACTED
		REDACTED
			return s.rebuildByGroupIDs(ctx, groupIDs, "account_miss", seen)
	REDACTED
		return err
REDACTED
	if s.cache != nil {
		if err := s.cache.SetAccount(ctx, account); err != nil {
			return err
	REDACTED
REDACTED
	if len(groupIDs) == 0 {
		groupIDs = account.GroupIDs
REDACTED
	return s.rebuildByAccount(ctx, account, groupIDs, "account_change", seen)
REDACTED

func (s *SchedulerSnapshotService) handleGroupEvent(ctx context.Context, groupID *int64, seen map[batchSeenKey]struct{REDACTED) error {
	if groupID == nil || *groupID <= 0 || s.isRunModeSimple() {
		return nil
REDACTED
	if seen != nil {
		if _, ok := seen[batchSeenKey{groupID: *groupID, lifecycle: trueREDACTED]; ok {
			return nil
	REDACTED
REDACTED
	return s.reconcileGroupLifecycle(ctx, *groupID, seen)
REDACTED

func (s *SchedulerSnapshotService) reconcileGroupLifecycle(ctx context.Context, groupID int64, seen map[batchSeenKey]struct{REDACTED) error {
	plan, err := s.prepareGroupLifecycle(ctx, groupID, nil)
	if err != nil {
		return err
REDACTED
	if plan.active {
		for _, task := range plan.tasks {
			if err := s.rebuildBucketWithTokenPolicy(ctx, task, "group_change", true); err != nil {
				return err
		REDACTED
	REDACTED
REDACTED
	markGroupLifecycleSeen(seen, groupID)
	return nil
REDACTED

func (s *SchedulerSnapshotService) prepareGroupLifecycle(ctx context.Context, groupID int64, knownHistorical []SchedulerBucket) (plan schedulerGroupLifecyclePlan, retErr error) {
	if groupID <= 0 || s.isRunModeSimple() {
		return schedulerGroupLifecyclePlan{REDACTED, nil
REDACTED
	if s.cache == nil || s.groupRepo == nil {
		return schedulerGroupLifecyclePlan{REDACTED, ErrSchedulerCacheNotReady
REDACTED

	lifecycleCtx, cancel := context.WithTimeout(ctx, schedulerGroupLifecycleTimeout)
	defer cancel()
	lease, acquired, err := s.cache.TryAcquireGroupLifecycleLease(lifecycleCtx, groupID, schedulerGroupLifecycleLeaseTTL)
	if err != nil {
		return schedulerGroupLifecyclePlan{REDACTED, err
REDACTED
	if !acquired {
		return schedulerGroupLifecyclePlan{REDACTED, fmt.Errorf("%w: group=%d", ErrSchedulerGroupLifecycleLeaseBusy, groupID)
REDACTED
	leaseHeld := true
	defer func() {
		if leaseHeld {
			retErr = errors.Join(retErr, s.releaseGroupLifecycleLease(lease))
	REDACTED
REDACTED()

	group, err := s.groupRepo.GetByIDLite(lifecycleCtx, groupID)
	missing := errors.Is(err, ErrGroupNotFound)
	if err != nil && !missing {
		return schedulerGroupLifecyclePlan{REDACTED, err
REDACTED
	if err == nil && (group == nil || group.ID != groupID || !group.Hydrated) {
		return schedulerGroupLifecyclePlan{REDACTED, fmt.Errorf("untrusted scheduler group lifecycle state: group=%d", groupID)
REDACTED

	plan = schedulerGroupLifecyclePlan{active: !missing && group.IsActive()REDACTED
	if plan.active {
		buckets := schedulerBucketsForGroup(groupID)
		plan.tasks = make([]schedulerBucketWriteTask, 0, len(buckets))
		for _, bucket := range buckets {
			token, err := s.cache.ReopenBucket(lifecycleCtx, bucket)
			if err != nil {
				return schedulerGroupLifecyclePlan{REDACTED, err
		REDACTED
			plan.tasks = append(plan.tasks, schedulerBucketWriteTask{bucket: bucket, token: tokenREDACTED)
	REDACTED
REDACTED else {
		registered := knownHistorical
		if registered == nil {
			registered, err = s.cache.ListBuckets(lifecycleCtx)
			if err != nil {
				return schedulerGroupLifecyclePlan{REDACTED, err
		REDACTED
	REDACTED
		buckets := schedulerBucketsForGroup(groupID)
		for _, bucket := range registered {
			if bucket.GroupID == groupID {
				buckets = append(buckets, bucket)
		REDACTED
	REDACTED
		for _, bucket := range dedupeBuckets(buckets) {
			if err := s.cache.RetireBucket(lifecycleCtx, bucket); err != nil {
				return schedulerGroupLifecyclePlan{REDACTED, err
		REDACTED
	REDACTED
REDACTED

	releaseErr := s.releaseGroupLifecycleLease(lease)
	leaseHeld = false
	if releaseErr != nil {
		return schedulerGroupLifecyclePlan{REDACTED, releaseErr
REDACTED
	return plan, nil
REDACTED

func (s *SchedulerSnapshotService) releaseGroupLifecycleLease(lease SchedulerGroupLifecycleLease) error {
	releaseCtx, cancel := context.WithTimeout(context.Background(), schedulerGroupLifecycleReleaseTimeout)
	defer cancel()
	return s.cache.ReleaseGroupLifecycleLease(releaseCtx, lease)
REDACTED

func markGroupLifecycleSeen(seen map[batchSeenKey]struct{REDACTED, groupID int64) {
	if seen == nil {
		return
REDACTED
	seen[batchSeenKey{groupID: groupID, lifecycle: trueREDACTED] = struct{REDACTED{REDACTED
	for _, platform := range schedulerSnapshotPlatforms() {
		seen[batchSeenKey{groupID: groupID, platform: platformREDACTED] = struct{REDACTED{REDACTED
REDACTED
REDACTED

func (s *SchedulerSnapshotService) rebuildByAccount(ctx context.Context, account *Account, groupIDs []int64, reason string, seen map[batchSeenKey]struct{REDACTED) error {
	if account == nil {
		return nil
REDACTED
	groupIDs = s.normalizeGroupIDs(groupIDs)
	if len(groupIDs) == 0 {
		return nil
REDACTED

	buckets := s.bucketsForPlatform(account.Platform, groupIDs, seen)
	if account.Platform == PlatformAntigravity && account.IsMixedSchedulingEnabled() {
		buckets = append(buckets, s.bucketsForPlatform(PlatformAnthropic, groupIDs, seen)...)
		buckets = append(buckets, s.bucketsForPlatform(PlatformGemini, groupIDs, seen)...)
REDACTED
	return s.rebuildBuckets(ctx, buckets, reason)
REDACTED

func schedulerSnapshotPlatforms() [5]string {
	return [5]string{PlatformAnthropic, PlatformGemini, PlatformOpenAI, PlatformAntigravity, PlatformGrokREDACTED
REDACTED

func schedulerBucketsForGroup(groupID int64) []SchedulerBucket {
	if groupID <= 0 {
		return nil
REDACTED
	return schedulerCanonicalBuckets(groupID)
REDACTED

func schedulerCanonicalBuckets(groupID int64) []SchedulerBucket {
	buckets := make([]SchedulerBucket, 0, 12)
	for _, platform := range schedulerSnapshotPlatforms() {
		buckets = append(buckets,
			SchedulerBucket{GroupID: groupID, Platform: platform, Mode: SchedulerModeSingleREDACTED,
			SchedulerBucket{GroupID: groupID, Platform: platform, Mode: SchedulerModeForcedREDACTED,
		)
		if platform == PlatformAnthropic || platform == PlatformGemini {
			buckets = append(buckets, SchedulerBucket{GroupID: groupID, Platform: platform, Mode: SchedulerModeMixedREDACTED)
	REDACTED
REDACTED
	return buckets
REDACTED

func (s *SchedulerSnapshotService) rebuildByGroupIDs(ctx context.Context, groupIDs []int64, reason string, seen map[batchSeenKey]struct{REDACTED) error {
	groupIDs = s.normalizeGroupIDs(groupIDs)
	if len(groupIDs) == 0 {
		return nil
REDACTED
	buckets := make([]SchedulerBucket, 0, len(groupIDs)*12)
	for _, platform := range schedulerSnapshotPlatforms() {
		buckets = append(buckets, s.bucketsForPlatform(platform, groupIDs, seen)...)
REDACTED
	return s.rebuildBuckets(ctx, buckets, reason)
REDACTED

func (s *SchedulerSnapshotService) bucketsForPlatform(platform string, groupIDs []int64, seen map[batchSeenKey]struct{REDACTED) []SchedulerBucket {
	if platform == "" {
		return nil
REDACTED
	buckets := make([]SchedulerBucket, 0, len(groupIDs)*3)
	for _, gid := range groupIDs {
		// Within a single poll batch, skip (groupID, platform) pairs that were
		// already rebuilt. The first rebuild loads fresh DB data for all accounts
		// in the group, so subsequent rebuilds for the same group+platform within
		// the same batch are redundant.
		if seen != nil {
			key := batchSeenKey{groupID: gid, platform: platformREDACTED
			if _, exists := seen[key]; exists {
				continue
		REDACTED
			seen[key] = struct{REDACTED{REDACTED
	REDACTED
		buckets = append(buckets, SchedulerBucket{GroupID: gid, Platform: platform, Mode: SchedulerModeSingleREDACTED)
		buckets = append(buckets, SchedulerBucket{GroupID: gid, Platform: platform, Mode: SchedulerModeForcedREDACTED)
		if platform == PlatformAnthropic || platform == PlatformGemini {
			buckets = append(buckets, SchedulerBucket{GroupID: gid, Platform: platform, Mode: SchedulerModeMixedREDACTED)
	REDACTED
REDACTED
	return buckets
REDACTED

func (s *SchedulerSnapshotService) rebuildBuckets(ctx context.Context, buckets []SchedulerBucket, reason string) error {
	tasks, firstErr := s.prepareBucketWriteTasks(ctx, buckets)
	if err := s.rebuildPreparedBucketTasks(ctx, tasks, reason, false); err != nil && firstErr == nil {
		firstErr = err
REDACTED
	return firstErr
REDACTED

func (s *SchedulerSnapshotService) prepareBucketWriteTasks(ctx context.Context, buckets []SchedulerBucket) ([]schedulerBucketWriteTask, error) {
	if s.cache == nil {
		return nil, ErrSchedulerCacheNotReady
REDACTED
	tasks := make([]schedulerBucketWriteTask, 0, len(buckets))
	var firstErr error
	for _, bucket := range buckets {
		token, err := s.cache.CaptureBucketWriteToken(ctx, bucket)
		if err != nil {
			if errors.Is(err, ErrSchedulerBucketRetired) || errors.Is(err, ErrSchedulerBucketWriteFenced) {
				continue
		REDACTED
			if firstErr == nil {
				firstErr = err
		REDACTED
			continue
	REDACTED
		tasks = append(tasks, schedulerBucketWriteTask{bucket: bucket, token: tokenREDACTED)
REDACTED
	return tasks, firstErr
REDACTED

func (s *SchedulerSnapshotService) rebuildPreparedBucketTasks(ctx context.Context, tasks []schedulerBucketWriteTask, reason string, strict bool) error {
	var firstErr error
	for _, task := range tasks {
		if err := s.rebuildBucketWithTokenPolicy(ctx, task, reason, strict); err != nil && firstErr == nil {
			firstErr = err
	REDACTED
REDACTED
	return firstErr
REDACTED

func (s *SchedulerSnapshotService) rebuildBucketWithTokenPolicy(ctx context.Context, task schedulerBucketWriteTask, reason string, strict bool) error {
	if s.cache == nil {
		return ErrSchedulerCacheNotReady
REDACTED
	bucket := task.bucket
	ok, err := s.cache.TryLockBucket(ctx, bucket, 30*time.Second)
	if err != nil {
		return err
REDACTED
	if !ok {
		if strict {
			return fmt.Errorf("%w: bucket=%s", ErrSchedulerBucketRebuildBusy, bucket.String())
	REDACTED
		return nil
REDACTED
	defer func() {
		_ = s.cache.UnlockBucket(ctx, bucket)
REDACTED()

	rebuildCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	accounts, err := s.loadAccountsFromDB(rebuildCtx, bucket, bucket.Mode == SchedulerModeMixed)
	if err != nil {
		logger.LegacyPrintf("service.scheduler_snapshot", "[Scheduler] rebuild failed: bucket=%s reason=%s err=%v", bucket.String(), reason, err)
		return err
REDACTED
	if err := s.cache.SetSnapshot(rebuildCtx, bucket, task.token, accounts); err != nil {
		if errors.Is(err, ErrSchedulerBucketRetired) || errors.Is(err, ErrSchedulerBucketWriteFenced) {
			slog.Debug("[Scheduler] rebuild fenced", "bucket", bucket.String(), "reason", reason)
			if strict {
				return err
		REDACTED
			return nil
	REDACTED
		logger.LegacyPrintf("service.scheduler_snapshot", "[Scheduler] rebuild cache failed: bucket=%s reason=%s err=%v", bucket.String(), reason, err)
		return err
REDACTED
	slog.Debug("[Scheduler] rebuild ok", "bucket", bucket.String(), "reason", reason, "size", len(accounts))
	return nil
REDACTED

func (s *SchedulerSnapshotService) triggerFullRebuild(reason string) error {
	if s.cache == nil {
		return ErrSchedulerCacheNotReady
REDACTED
	return s.coalesceFullRebuild(func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		return s.rebuildFullSnapshot(ctx, reason)
REDACTED)
REDACTED

func (s *SchedulerSnapshotService) rebuildFullSnapshot(ctx context.Context, reason string) error {
	if s.cache == nil {
		return ErrSchedulerCacheNotReady
REDACTED

	registered, err := s.cache.ListBuckets(ctx)
	if err != nil {
		return err
REDACTED
	registered = dedupeBuckets(registered)

	if s.isRunModeSimple() {
		canonical := schedulerCanonicalBuckets(0)
		captured, err := s.captureFullRebuildCanonicalTasks(ctx, canonical)
		if err != nil {
			return err
	REDACTED
		ordinary := appendBucketsExcept(nil, registered, canonical)
		return s.prepareAndRebuildFullSnapshot(ctx, captured, nil, ordinary, reason)
REDACTED

	activeGroupIDs, err := s.listActiveSchedulerGroupIDs(ctx)
	if err != nil {
		return err
REDACTED
	activeGroups := make(map[int64]struct{REDACTED, len(activeGroupIDs))
	for _, groupID := range activeGroupIDs {
		activeGroups[groupID] = struct{REDACTED{REDACTED
REDACTED

	registeredByGroup := make(map[int64][]SchedulerBucket)
	for _, bucket := range registered {
		registeredByGroup[bucket.GroupID] = append(registeredByGroup[bucket.GroupID], bucket)
REDACTED

	groupZeroCanonical := schedulerCanonicalBuckets(0)
	capturedTasks, err := s.captureFullRebuildCanonicalTasks(ctx, groupZeroCanonical)
	if err != nil {
		return err
REDACTED
	ordinaryBuckets := appendBucketsExcept(nil, registeredByGroup[0], groupZeroCanonical)
	for groupID, buckets := range registeredByGroup {
		if groupID < 0 {
			ordinaryBuckets = append(ordinaryBuckets, buckets...)
	REDACTED
REDACTED

	reopenedTasks := make([]schedulerBucketWriteTask, 0)
	for _, groupID := range activeGroupIDs {
		canonical := schedulerBucketsForGroup(groupID)
		canonicalTasks, captureErr := s.captureFullRebuildCanonicalTasks(ctx, canonical)
		if captureErr == nil {
			capturedTasks = append(capturedTasks, canonicalTasks...)
			ordinaryBuckets = appendBucketsExcept(ordinaryBuckets, registeredByGroup[groupID], canonical)
			continue
	REDACTED
		if !errors.Is(captureErr, ErrSchedulerBucketRetired) && !errors.Is(captureErr, ErrSchedulerBucketWriteFenced) {
			return captureErr
	REDACTED

		// A prior full_rebuild event can observe the active state committed for a
		// later group_changed event. Recover here under fresh authority so the
		// earlier event cannot block the outbox watermark before that event runs.
		knownHistorical := registeredByGroup[groupID]
		if knownHistorical == nil {
			knownHistorical = []SchedulerBucket{REDACTED
	REDACTED
		plan, err := s.prepareGroupLifecycle(ctx, groupID, knownHistorical)
		if err != nil {
			return err
	REDACTED
		if plan.active {
			reopenedTasks = append(reopenedTasks, plan.tasks...)
			ordinaryBuckets = appendBucketsExcept(ordinaryBuckets, registeredByGroup[groupID], canonical)
	REDACTED
REDACTED

	staleGroupIDs := make([]int64, 0)
	for groupID := range registeredByGroup {
		if groupID <= 0 {
			continue
	REDACTED
		if _, active := activeGroups[groupID]; !active {
			staleGroupIDs = append(staleGroupIDs, groupID)
	REDACTED
REDACTED
	sort.Slice(staleGroupIDs, func(i, j int) bool { return staleGroupIDs[i] < staleGroupIDs[j] REDACTED)

	for _, groupID := range staleGroupIDs {
		plan, err := s.prepareGroupLifecycle(ctx, groupID, registeredByGroup[groupID])
		if err != nil {
			return err
	REDACTED
		if plan.active {
			reopenedTasks = append(reopenedTasks, plan.tasks...)
			ordinaryBuckets = appendBucketsExcept(ordinaryBuckets, registeredByGroup[groupID], schedulerBucketsForGroup(groupID))
	REDACTED
REDACTED

	return s.prepareAndRebuildFullSnapshot(ctx, capturedTasks, reopenedTasks, ordinaryBuckets, reason)
REDACTED

func (s *SchedulerSnapshotService) listActiveSchedulerGroupIDs(ctx context.Context) ([]int64, error) {
	if s.groupRepo == nil {
		return nil, ErrSchedulerCacheNotReady
REDACTED

	var groupIDs []int64
	if lister, ok := s.groupRepo.(schedulerActiveGroupIDLister); ok {
		ids, err := lister.ListActiveIDs(ctx)
		if err != nil {
			return nil, err
	REDACTED
		groupIDs = ids
REDACTED else {
		groups, err := s.groupRepo.ListActive(ctx)
		if err != nil {
			return nil, err
	REDACTED
		groupIDs = make([]int64, 0, len(groups))
		for _, group := range groups {
			groupIDs = append(groupIDs, group.ID)
	REDACTED
REDACTED

	seen := make(map[int64]struct{REDACTED, len(groupIDs))
	normalized := make([]int64, 0, len(groupIDs))
	for _, groupID := range groupIDs {
		if groupID <= 0 {
			continue
	REDACTED
		if _, ok := seen[groupID]; ok {
			continue
	REDACTED
		seen[groupID] = struct{REDACTED{REDACTED
		normalized = append(normalized, groupID)
REDACTED
	sort.Slice(normalized, func(i, j int) bool { return normalized[i] < normalized[j] REDACTED)
	return normalized, nil
REDACTED

func (s *SchedulerSnapshotService) prepareAndRebuildFullSnapshot(
	ctx context.Context,
	captured []schedulerBucketWriteTask,
	reopened []schedulerBucketWriteTask,
	ordinaryBuckets []SchedulerBucket,
	reason string,
) error {
	preparedBuckets := make(map[SchedulerBucket]struct{REDACTED, len(captured)+len(reopened))
	for _, task := range captured {
		preparedBuckets[task.bucket] = struct{REDACTED{REDACTED
REDACTED
	for _, task := range reopened {
		preparedBuckets[task.bucket] = struct{REDACTED{REDACTED
REDACTED

	ordinaryBuckets = dedupeBuckets(ordinaryBuckets)
	toCapture := make([]SchedulerBucket, 0, len(ordinaryBuckets))
	for _, bucket := range ordinaryBuckets {
		if _, ok := preparedBuckets[bucket]; !ok {
			toCapture = append(toCapture, bucket)
	REDACTED
REDACTED
	ordinary, firstErr := s.prepareBucketWriteTasks(ctx, toCapture)
	if firstErr != nil {
		return firstErr
REDACTED
	captured = append(captured, ordinary...)
	if err := s.rebuildPreparedBucketTasks(ctx, reopened, reason, true); err != nil {
		firstErr = err
REDACTED
	if err := s.rebuildPreparedBucketTasks(ctx, captured, reason, false); err != nil && firstErr == nil {
		firstErr = err
REDACTED
	return firstErr
REDACTED

func (s *SchedulerSnapshotService) captureFullRebuildCanonicalTasks(ctx context.Context, buckets []SchedulerBucket) ([]schedulerBucketWriteTask, error) {
	if s.cache == nil {
		return nil, ErrSchedulerCacheNotReady
REDACTED
	tasks := make([]schedulerBucketWriteTask, 0, len(buckets))
	for _, bucket := range buckets {
		token, err := s.cache.CaptureBucketWriteToken(ctx, bucket)
		if err != nil {
			return nil, err
	REDACTED
		tasks = append(tasks, schedulerBucketWriteTask{bucket: bucket, token: tokenREDACTED)
REDACTED
	return tasks, nil
REDACTED

func appendBucketsExcept(dst, buckets, excluded []SchedulerBucket) []SchedulerBucket {
	excludedKeys := make(map[SchedulerBucket]struct{REDACTED, len(excluded))
	for _, bucket := range excluded {
		excludedKeys[bucket] = struct{REDACTED{REDACTED
REDACTED
	for _, bucket := range buckets {
		if _, ok := excludedKeys[bucket]; !ok {
			dst = append(dst, bucket)
	REDACTED
REDACTED
	return dst
REDACTED

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
REDACTED
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
REDACTED

func (s *SchedulerSnapshotService) checkOutboxLag(ctx context.Context, watermark int64) {
	if s.cfg == nil || s.outboxRepo == nil {
		return
REDACTED
	oldestCreatedAt, ok, err := s.outboxRepo.FirstCreatedAtAfter(ctx, watermark)
	if err != nil {
		logger.LegacyPrintf("service.scheduler_snapshot", "[Scheduler] outbox pending event read failed: %v", err)
		return
REDACTED
	if !ok || oldestCreatedAt.IsZero() {
		s.lagMu.Lock()
		s.lagFailures = 0
		s.lagMu.Unlock()
		return
REDACTED

	lag := time.Since(oldestCreatedAt)
	if lagSeconds := int(lag.Seconds()); lagSeconds >= s.cfg.Gateway.Scheduling.OutboxLagWarnSeconds && s.cfg.Gateway.Scheduling.OutboxLagWarnSeconds > 0 {
		logger.LegacyPrintf("service.scheduler_snapshot", "[Scheduler] outbox lag warning: %ds", lagSeconds)
REDACTED

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
		REDACTED
	REDACTED
REDACTED else {
		s.lagMu.Lock()
		s.lagFailures = 0
		s.lagMu.Unlock()
REDACTED

	threshold := s.cfg.Gateway.Scheduling.OutboxBacklogRebuildRows
	if threshold <= 0 {
		return
REDACTED
	maxID, err := s.outboxRepo.MaxID(ctx)
	if err != nil {
		return
REDACTED
	if maxID-watermark >= int64(threshold) {
		logger.LegacyPrintf("service.scheduler_snapshot", "[Scheduler] outbox backlog rebuild triggered: backlog=%d", maxID-watermark)
		if err := s.triggerFullRebuild("outbox_backlog"); err != nil {
			logger.LegacyPrintf("service.scheduler_snapshot", "[Scheduler] outbox backlog rebuild failed: %v", err)
	REDACTED
REDACTED
REDACTED

func (s *SchedulerSnapshotService) loadAccountsFromDB(ctx context.Context, bucket SchedulerBucket, useMixed bool) ([]Account, error) {
	if s.accountRepo == nil {
		return nil, ErrSchedulerCacheNotReady
REDACTED
	groupID := bucket.GroupID
	if s.isRunModeSimple() {
		groupID = 0
REDACTED

	if useMixed {
		platforms := []string{bucket.Platform, PlatformAntigravityREDACTED
		var accounts []Account
		var err error
		if groupID > 0 {
			accounts, err = s.accountRepo.ListSchedulableByGroupIDAndPlatforms(ctx, groupID, platforms)
	REDACTED else if s.isRunModeSimple() {
			accounts, err = s.accountRepo.ListSchedulableByPlatforms(ctx, platforms)
	REDACTED else {
			accounts, err = s.accountRepo.ListSchedulableUngroupedByPlatforms(ctx, platforms)
	REDACTED
		if err != nil {
			return nil, err
	REDACTED
		filtered := make([]Account, 0, len(accounts))
		for _, acc := range accounts {
			if acc.Platform == PlatformAntigravity && !acc.IsMixedSchedulingEnabled() {
				continue
		REDACTED
			filtered = append(filtered, acc)
	REDACTED
		return filtered, nil
REDACTED

	if groupID > 0 {
		return s.accountRepo.ListSchedulableByGroupIDAndPlatform(ctx, groupID, bucket.Platform)
REDACTED
	if s.isRunModeSimple() {
		return s.accountRepo.ListSchedulableByPlatform(ctx, bucket.Platform)
REDACTED
	return s.accountRepo.ListSchedulableUngroupedByPlatform(ctx, bucket.Platform)
REDACTED

func (s *SchedulerSnapshotService) bucketFor(groupID *int64, platform string, mode string) SchedulerBucket {
	return SchedulerBucket{
		GroupID:  s.normalizeGroupID(groupID),
		Platform: platform,
		Mode:     mode,
REDACTED
REDACTED

func (s *SchedulerSnapshotService) normalizeGroupID(groupID *int64) int64 {
	if s.isRunModeSimple() {
		return 0
REDACTED
	if groupID == nil || *groupID <= 0 {
		return 0
REDACTED
	return *groupID
REDACTED

func (s *SchedulerSnapshotService) normalizeGroupIDs(groupIDs []int64) []int64 {
	if s.isRunModeSimple() {
		return []int64{0REDACTED
REDACTED
	if len(groupIDs) == 0 {
		return []int64{0REDACTED
REDACTED
	seen := make(map[int64]struct{REDACTED, len(groupIDs))
	out := make([]int64, 0, len(groupIDs))
	for _, id := range groupIDs {
		if id <= 0 {
			continue
	REDACTED
		if _, ok := seen[id]; ok {
			continue
	REDACTED
		seen[id] = struct{REDACTED{REDACTED
		out = append(out, id)
REDACTED
	if len(out) == 0 {
		return []int64{0REDACTED
REDACTED
	return out
REDACTED

func (s *SchedulerSnapshotService) resolveMode(platform string, hasForcePlatform bool) string {
	if hasForcePlatform {
		return SchedulerModeForced
REDACTED
	if platform == PlatformAnthropic || platform == PlatformGemini {
		return SchedulerModeMixed
REDACTED
	return SchedulerModeSingle
REDACTED

func (s *SchedulerSnapshotService) guardFallback(ctx context.Context) error {
	if s.cfg == nil || s.cfg.Gateway.Scheduling.DbFallbackEnabled {
		if s.fallbackLimit == nil || s.fallbackLimit.Allow() {
			return nil
	REDACTED
		return ErrSchedulerFallbackLimited
REDACTED
	return ErrSchedulerCacheNotReady
REDACTED

func (s *SchedulerSnapshotService) withFallbackTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if s.cfg == nil || s.cfg.Gateway.Scheduling.DbFallbackTimeoutSeconds <= 0 {
		return context.WithCancel(ctx)
REDACTED
	timeout := time.Duration(s.cfg.Gateway.Scheduling.DbFallbackTimeoutSeconds) * time.Second
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return context.WithCancel(ctx)
	REDACTED
		if remaining < timeout {
			timeout = remaining
	REDACTED
REDACTED
	return context.WithTimeout(ctx, timeout)
REDACTED

func (s *SchedulerSnapshotService) isRunModeSimple() bool {
	return s.cfg != nil && s.cfg.RunMode == config.RunModeSimple
REDACTED

func (s *SchedulerSnapshotService) outboxPollInterval() time.Duration {
	if s.cfg == nil {
		return time.Second
REDACTED
	sec := s.cfg.Gateway.Scheduling.OutboxPollIntervalSeconds
	if sec <= 0 {
		return time.Second
REDACTED
	return time.Duration(sec) * time.Second
REDACTED

func (s *SchedulerSnapshotService) fullRebuildInterval() time.Duration {
	if s.cfg == nil {
		return 0
REDACTED
	sec := s.cfg.Gateway.Scheduling.FullRebuildIntervalSeconds
	if sec <= 0 {
		return 0
REDACTED
	return time.Duration(sec) * time.Second
REDACTED

func dedupeBuckets(in []SchedulerBucket) []SchedulerBucket {
	seen := make(map[string]struct{REDACTED, len(in))
	out := make([]SchedulerBucket, 0, len(in))
	for _, bucket := range in {
		key := bucket.String()
		if _, ok := seen[key]; ok {
			continue
	REDACTED
		seen[key] = struct{REDACTED{REDACTED
		out = append(out, bucket)
REDACTED
	return out
REDACTED

func derefAccounts(accounts []*Account) []Account {
	if len(accounts) == 0 {
		return []Account{REDACTED
REDACTED
	out := make([]Account, 0, len(accounts))
	for _, account := range accounts {
		if account == nil {
			continue
	REDACTED
		out = append(out, *account)
REDACTED
	return out
REDACTED

func parseInt64Slice(value any) []int64 {
	raw, ok := value.([]any)
	if !ok {
		return nil
REDACTED
	out := make([]int64, 0, len(raw))
	for _, item := range raw {
		if v, ok := toInt64(item); ok && v > 0 {
			out = append(out, v)
	REDACTED
REDACTED
	return out
REDACTED

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
REDACTED
REDACTED

type fallbackLimiter struct {
	maxQPS int
	mu     sync.Mutex
	window time.Time
	count  int
REDACTED

func newFallbackLimiter(maxQPS int) *fallbackLimiter {
	if maxQPS <= 0 {
		return nil
REDACTED
	return &fallbackLimiter{
		maxQPS: maxQPS,
		window: time.Now(),
REDACTED
REDACTED

func (l *fallbackLimiter) Allow() bool {
	if l == nil || l.maxQPS <= 0 {
		return true
REDACTED
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	if now.Sub(l.window) >= time.Second {
		l.window = now
		l.count = 0
REDACTED
	if l.count >= l.maxQPS {
		return false
REDACTED
	l.count++
	return true
REDACTED
