//go:build unit

package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type fullRebuildLifecycleCache struct {
	*groupLifecycleTestCache

	mu              sync.Mutex
	captureAttempts []SchedulerBucket
	captureErrors   map[string]error
	lockBusyOnce    map[string]bool
	watermark       int64
	watermarkWrites []int64
REDACTED

func newFullRebuildLifecycleCache(buckets ...SchedulerBucket) *fullRebuildLifecycleCache {
	return &fullRebuildLifecycleCache{
		groupLifecycleTestCache: newGroupLifecycleTestCache(buckets...),
		captureErrors:           make(map[string]error),
		lockBusyOnce:            make(map[string]bool),
REDACTED
REDACTED

func (c *fullRebuildLifecycleCache) CaptureBucketWriteToken(ctx context.Context, bucket SchedulerBucket) (SchedulerBucketWriteToken, error) {
	c.mu.Lock()
	c.captureAttempts = append(c.captureAttempts, bucket)
	err := c.captureErrors[bucket.String()]
	c.mu.Unlock()
	if err != nil {
		return SchedulerBucketWriteToken{REDACTED, err
REDACTED
	return c.retirementRaceCache.CaptureBucketWriteToken(ctx, bucket)
REDACTED

func (c *fullRebuildLifecycleCache) ListBuckets(ctx context.Context) ([]SchedulerBucket, error) {
	buckets, err := c.groupLifecycleTestCache.ListBuckets(ctx)
	if err != nil {
		return nil, err
REDACTED
	c.retirementRaceCache.mu.Lock()
	defer c.retirementRaceCache.mu.Unlock()
	registered := make([]SchedulerBucket, 0, len(buckets))
	for _, bucket := range buckets {
		if !c.retirementRaceCache.retired[bucket.String()] {
			registered = append(registered, bucket)
	REDACTED
REDACTED
	return registered, nil
REDACTED

func (c *fullRebuildLifecycleCache) TryLockBucket(ctx context.Context, bucket SchedulerBucket, ttl time.Duration) (bool, error) {
	c.mu.Lock()
	busy := c.lockBusyOnce[bucket.String()]
	if busy {
		delete(c.lockBusyOnce, bucket.String())
REDACTED
	c.mu.Unlock()
	if busy {
		return false, nil
REDACTED
	return c.groupLifecycleTestCache.TryLockBucket(ctx, bucket, ttl)
REDACTED

func (c *fullRebuildLifecycleCache) GetOutboxWatermark(context.Context) (int64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.watermark, nil
REDACTED

func (c *fullRebuildLifecycleCache) SetOutboxWatermark(_ context.Context, id int64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.watermark = id
	c.watermarkWrites = append(c.watermarkWrites, id)
	return nil
REDACTED

func (c *fullRebuildLifecycleCache) captureAttemptCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.captureAttempts)
REDACTED

func (c *fullRebuildLifecycleCache) currentWatermark() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.watermark
REDACTED

func (c *fullRebuildLifecycleCache) totalSetAttempts() int {
	c.retirementRaceCache.mu.Lock()
	defer c.retirementRaceCache.mu.Unlock()
	var total int
	for _, attempts := range c.retirementRaceCache.setAttempts {
		total += attempts
REDACTED
	return total
REDACTED

type fullRebuildLifecycleGroupRepo struct {
	GroupRepository

	mu              sync.Mutex
	activeIDs       []int64
	activeIDsErr    error
	listActiveErr   error
	fresh           map[int64]*Group
	freshErr        map[int64]error
	activeIDCalls   int
	listActiveCalls int
	freshCalls      []int64
REDACTED

func (r *fullRebuildLifecycleGroupRepo) ListActiveIDs(context.Context) ([]int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.activeIDCalls++
	return append([]int64(nil), r.activeIDs...), r.activeIDsErr
REDACTED

func (r *fullRebuildLifecycleGroupRepo) ListActive(context.Context) ([]Group, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.listActiveCalls++
	return nil, r.listActiveErr
REDACTED

func (r *fullRebuildLifecycleGroupRepo) GetByIDLite(_ context.Context, id int64) (*Group, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.freshCalls = append(r.freshCalls, id)
	if err := r.freshErr[id]; err != nil {
		return nil, err
REDACTED
	group := r.fresh[id]
	if group == nil {
		return nil, ErrGroupNotFound
REDACTED
	copyGroup := *group
	return &copyGroup, nil
REDACTED

func (r *fullRebuildLifecycleGroupRepo) stats() (activeIDs, listActive int, fresh []int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.activeIDCalls, r.listActiveCalls, append([]int64(nil), r.freshCalls...)
REDACTED

type fullRebuildFallbackGroupRepo struct {
	GroupRepository

	mu        sync.Mutex
	groups    []Group
	err       error
	listCalls int
REDACTED

func (r *fullRebuildFallbackGroupRepo) ListActive(context.Context) ([]Group, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.listCalls++
	return append([]Group(nil), r.groups...), r.err
REDACTED

type fullRebuildAccountCall struct {
	groupID  int64
	platform string
REDACTED

type fullRebuildAccountRepo struct {
	AccountRepository

	mu          sync.Mutex
	calls       []fullRebuildAccountCall
	beforeFirst func()
	once        sync.Once
REDACTED

func (r *fullRebuildAccountRepo) record(groupID int64, platform string) ([]Account, error) {
	r.mu.Lock()
	r.calls = append(r.calls, fullRebuildAccountCall{groupID: groupID, platform: platformREDACTED)
	beforeFirst := r.beforeFirst
	r.mu.Unlock()
	if beforeFirst != nil {
		r.once.Do(beforeFirst)
REDACTED
	return []Account{{ID: 1, Platform: platform, Status: StatusActive, Schedulable: trueREDACTEDREDACTED, nil
REDACTED

func (r *fullRebuildAccountRepo) ListSchedulableByGroupIDAndPlatform(_ context.Context, groupID int64, platform string) ([]Account, error) {
	return r.record(groupID, platform)
REDACTED

func (r *fullRebuildAccountRepo) ListSchedulableByGroupIDAndPlatforms(_ context.Context, groupID int64, platforms []string) ([]Account, error) {
	return r.record(groupID, firstPlatform(platforms))
REDACTED

func (r *fullRebuildAccountRepo) ListSchedulableUngroupedByPlatform(_ context.Context, platform string) ([]Account, error) {
	return r.record(0, platform)
REDACTED

func (r *fullRebuildAccountRepo) ListSchedulableUngroupedByPlatforms(_ context.Context, platforms []string) ([]Account, error) {
	return r.record(0, firstPlatform(platforms))
REDACTED

func (r *fullRebuildAccountRepo) ListSchedulableByPlatform(_ context.Context, platform string) ([]Account, error) {
	return r.record(0, platform)
REDACTED

func (r *fullRebuildAccountRepo) ListSchedulableByPlatforms(_ context.Context, platforms []string) ([]Account, error) {
	return r.record(0, firstPlatform(platforms))
REDACTED

func (r *fullRebuildAccountRepo) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.calls)
REDACTED

func (r *fullRebuildAccountRepo) groupCallCount(groupID int64) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	var count int
	for _, call := range r.calls {
		if call.groupID == groupID {
			count++
	REDACTED
REDACTED
	return count
REDACTED

func firstPlatform(platforms []string) string {
	if len(platforms) == 0 {
		return ""
REDACTED
	return platforms[0]
REDACTED

func newFullRebuildLifecycleService(
	cache SchedulerCache,
	outbox SchedulerOutboxRepository,
	accounts AccountRepository,
	groups GroupRepository,
	runMode string,
) *SchedulerSnapshotService {
	return NewSchedulerSnapshotService(cache, outbox, accounts, groups, &config.Config{RunMode: runModeREDACTED)
REDACTED

func TestSchedulerFullRebuildActiveTombstoneDoesNotBlockFollowingGroupEvent(t *testing.T) {
	const groupID int64 = 101
	canonical := schedulerBucketsForGroup(groupID)
	cache := newFullRebuildLifecycleCache()
	require.NoError(t, cache.retirementRaceCache.RetireBucket(context.Background(), canonical[0]))
	groups := &fullRebuildLifecycleGroupRepo{
		activeIDs: []int64{groupIDREDACTED,
		fresh: map[int64]*Group{
			groupID: {ID: groupID, Status: StatusActive, Hydrated: trueREDACTED,
	REDACTED,
		freshErr: make(map[int64]error),
REDACTED
	accounts := &fullRebuildAccountRepo{REDACTED
	outbox := &outboxCleanupRepo{events: []SchedulerOutboxEvent{
		{ID: 1, EventType: SchedulerOutboxEventFullRebuildREDACTED,
		{ID: 2, EventType: SchedulerOutboxEventGroupChanged, GroupID: ptrInt64(groupID)REDACTED,
REDACTEDREDACTED
	svc := newFullRebuildLifecycleService(cache, outbox, accounts, groups, config.RunModeStandard)

	svc.pollOutbox()

	require.Equal(t, int64(2), cache.currentWatermark())
	cache.mu.Lock()
	require.Equal(t, []int64{2REDACTED, cache.watermarkWrites)
	cache.mu.Unlock()
	activeCalls, fallbackCalls, freshCalls := groups.stats()
	require.Equal(t, 1, activeCalls)
	require.Zero(t, fallbackCalls)
	require.Equal(t, []int64{groupID, groupIDREDACTED, freshCalls)
	require.Len(t, cache.tokens(), 24, "full rebuild and the following group event must each run fresh authority")
	_, reopenHeld := cache.lifecycleMutationLeaseStates()
	require.Len(t, reopenHeld, 24)
	for _, held := range reopenHeld {
		require.True(t, held)
REDACTED
	require.Equal(t, 36, accounts.callCount())
REDACTED

func TestSchedulerFullRebuildGlobalReadErrorsFailBeforeMutationOrDB(t *testing.T) {
	t.Run("list buckets", func(t *testing.T) {
		cache := newFullRebuildLifecycleCache()
		cache.listErr = errors.New("registry failed")
		groups := &fullRebuildLifecycleGroupRepo{fresh: make(map[int64]*Group), freshErr: make(map[int64]error)REDACTED
		accounts := &fullRebuildAccountRepo{REDACTED
		svc := newFullRebuildLifecycleService(cache, nil, accounts, groups, config.RunModeStandard)

		err := svc.rebuildFullSnapshot(context.Background(), "test")
		require.ErrorIs(t, err, cache.listErr)
		activeCalls, fallbackCalls, freshCalls := groups.stats()
		require.Zero(t, activeCalls)
		require.Zero(t, fallbackCalls)
		require.Empty(t, freshCalls)
		requireFullRebuildNoMutationOrDB(t, cache, accounts)
REDACTED)

	t.Run("list active ids without fallback", func(t *testing.T) {
		cache := newFullRebuildLifecycleCache()
		groups := &fullRebuildLifecycleGroupRepo{
			activeIDsErr:  errors.New("active ids failed"),
			listActiveErr: errors.New("fallback must not run"),
			fresh:         make(map[int64]*Group),
			freshErr:      make(map[int64]error),
	REDACTED
		accounts := &fullRebuildAccountRepo{REDACTED
		svc := newFullRebuildLifecycleService(cache, nil, accounts, groups, config.RunModeStandard)

		err := svc.rebuildFullSnapshot(context.Background(), "test")
		require.ErrorIs(t, err, groups.activeIDsErr)
		activeCalls, fallbackCalls, freshCalls := groups.stats()
		require.Equal(t, 1, activeCalls)
		require.Zero(t, fallbackCalls)
		require.Empty(t, freshCalls)
		requireFullRebuildNoMutationOrDB(t, cache, accounts)
REDACTED)

	t.Run("list active fallback", func(t *testing.T) {
		cache := newFullRebuildLifecycleCache()
		groups := &fullRebuildFallbackGroupRepo{err: errors.New("active groups failed")REDACTED
		accounts := &fullRebuildAccountRepo{REDACTED
		svc := newFullRebuildLifecycleService(cache, nil, accounts, groups, config.RunModeStandard)

		err := svc.rebuildFullSnapshot(context.Background(), "test")
		require.ErrorIs(t, err, groups.err)
		groups.mu.Lock()
		require.Equal(t, 1, groups.listCalls)
		groups.mu.Unlock()
		requireFullRebuildNoMutationOrDB(t, cache, accounts)
REDACTED)
REDACTED

func TestSchedulerFullRebuildFreshActivePreparesEveryTokenBeforeFirstDB(t *testing.T) {
	const groupID int64 = 102
	historical := SchedulerBucket{GroupID: groupID, Platform: "legacy", Mode: "unknown"REDACTED
	cache := newFullRebuildLifecycleCache(historical)
	groups := &fullRebuildLifecycleGroupRepo{
		fresh: map[int64]*Group{
			groupID: {ID: groupID, Status: StatusActive, Hydrated: trueREDACTED,
	REDACTED,
		freshErr: make(map[int64]error),
REDACTED
	accounts := &fullRebuildAccountRepo{REDACTED
	var capturesAtFirstDB int
	accounts.beforeFirst = func() {
		capturesAtFirstDB = cache.captureAttemptCount()
		held, reopenCount := cache.leaseHeldAndTokenCount()
		require.False(t, held)
		require.Equal(t, 12, reopenCount)
		require.Equal(t, 13, capturesAtFirstDB, "C(0) and the historical bucket must be captured before DB")
REDACTED
	svc := newFullRebuildLifecycleService(cache, nil, accounts, groups, config.RunModeStandard)

	require.NoError(t, svc.rebuildFullSnapshot(context.Background(), "test"))
	require.Equal(t, capturesAtFirstDB, cache.captureAttemptCount())
	require.Equal(t, 25, accounts.callCount())
	require.Equal(t, 13, accounts.groupCallCount(groupID))
	_, historicalPublished := cache.counts(historical)
	require.Equal(t, 1, historicalPublished)
	activeCalls, fallbackCalls, freshCalls := groups.stats()
	require.Equal(t, 1, activeCalls)
	require.Zero(t, fallbackCalls)
	require.Equal(t, []int64{groupIDREDACTED, freshCalls)
	_, _, listCalls := cache.lifecycleCounts()
	require.Equal(t, 1, listCalls, "known Rg must prevent a second registry read")
REDACTED

func TestSchedulerFullRebuildOrdinaryCaptureErrorReturnsBeforeFirstDB(t *testing.T) {
	first := SchedulerBucket{GroupID: 0, Platform: "legacy-a", Mode: "unknown"REDACTED
	last := SchedulerBucket{GroupID: 0, Platform: "legacy-b", Mode: "unknown"REDACTED
	cache := newFullRebuildLifecycleCache(first, last)
	wantErr := errors.New("capture failed")
	cache.captureErrors[last.String()] = wantErr
	groups := &fullRebuildLifecycleGroupRepo{fresh: make(map[int64]*Group), freshErr: make(map[int64]error)REDACTED
	accounts := &fullRebuildAccountRepo{REDACTED
	svc := newFullRebuildLifecycleService(cache, nil, accounts, groups, config.RunModeStandard)

	err := svc.rebuildFullSnapshot(context.Background(), "test")
	require.ErrorIs(t, err, wantErr)
	require.Equal(t, 14, cache.captureAttemptCount(), "all canonical and ordinary captures must be attempted before returning")
	require.Zero(t, accounts.callCount())
	require.Zero(t, cache.totalSetAttempts())
REDACTED

func TestSchedulerFullRebuildPreservesGroupZeroActiveHistoricalAndInvalidRegistryBuckets(t *testing.T) {
	const groupID int64 = 103
	groupZeroHistorical := SchedulerBucket{GroupID: 0, Platform: "legacy-zero", Mode: "unknown"REDACTED
	activeHistorical := SchedulerBucket{GroupID: groupID, Platform: "legacy-active", Mode: "unknown"REDACTED
	activeForced := SchedulerBucket{GroupID: groupID, Platform: PlatformOpenAI, Mode: SchedulerModeForcedREDACTED
	activeMixed := SchedulerBucket{GroupID: groupID, Platform: PlatformAnthropic, Mode: SchedulerModeMixedREDACTED
	invalidHistorical := SchedulerBucket{GroupID: -7, Platform: "legacy-invalid", Mode: "unknown"REDACTED
	cache := newFullRebuildLifecycleCache(groupZeroHistorical, activeHistorical, activeForced, activeMixed, invalidHistorical)
	groups := &fullRebuildFallbackGroupRepo{groups: []Group{{ID: groupID, Status: StatusActiveREDACTEDREDACTEDREDACTED
	accounts := &fullRebuildAccountRepo{REDACTED
	svc := newFullRebuildLifecycleService(cache, nil, accounts, groups, config.RunModeStandard)

	require.NoError(t, svc.rebuildFullSnapshot(context.Background(), "test"))
	require.Equal(t, 27, cache.captureAttemptCount())
	require.Equal(t, 27, accounts.callCount())
	groups.mu.Lock()
	require.Equal(t, 1, groups.listCalls)
	groups.mu.Unlock()
	require.Empty(t, cache.retiredBuckets())
	require.Empty(t, cache.tokens())
	for _, bucket := range []SchedulerBucket{groupZeroHistorical, activeHistorical, activeForced, activeMixed, invalidHistoricalREDACTED {
		_, published := cache.counts(bucket)
		require.Equal(t, 1, published, bucket.String())
REDACTED
REDACTED

func TestSchedulerFullRebuildActiveTombstoneFreshInactiveOrMissingFiltersAllGroupTasks(t *testing.T) {
	for _, tc := range []struct {
		name  string
		group *Group
REDACTED{
		{name: "inactive", group: &Group{ID: 104, Status: StatusDisabled, Hydrated: trueREDACTEDREDACTED,
		{name: "missing"REDACTED,
REDACTED {
		t.Run(tc.name, func(t *testing.T) {
			const groupID int64 = 104
			canonical := schedulerBucketsForGroup(groupID)
			historical := SchedulerBucket{GroupID: groupID, Platform: "legacy", Mode: "unknown"REDACTED
			cache := newFullRebuildLifecycleCache(historical)
			require.NoError(t, cache.retirementRaceCache.RetireBucket(context.Background(), canonical[5]))
			groups := &fullRebuildLifecycleGroupRepo{
				activeIDs: []int64{groupIDREDACTED,
				fresh:     make(map[int64]*Group),
				freshErr:  make(map[int64]error),
		REDACTED
			if tc.group != nil {
				groups.fresh[groupID] = tc.group
		REDACTED
			accounts := &fullRebuildAccountRepo{REDACTED
			svc := newFullRebuildLifecycleService(cache, nil, accounts, groups, config.RunModeStandard)

			require.NoError(t, svc.rebuildFullSnapshot(context.Background(), "test"))
			require.Zero(t, accounts.groupCallCount(groupID))
			require.Equal(t, 12, accounts.groupCallCount(0))
			require.Empty(t, cache.tokens())
			require.Equal(t, bucketStrings(append(canonical, historical)), bucketStrings(cache.retiredBuckets()))
			for _, bucket := range append(canonical, historical) {
				attempts, published := cache.counts(bucket)
				require.Zero(t, attempts, bucket.String())
				require.Zero(t, published, bucket.String())
		REDACTED
			_, _, listCalls := cache.lifecycleCounts()
			require.Equal(t, 1, listCalls)
	REDACTED)
REDACTED
REDACTED

func TestSchedulerFullRebuildStaleCandidatesAreSortedAndNeverReopenedAcrossRounds(t *testing.T) {
	historical := []SchedulerBucket{
		{GroupID: 3, Platform: "legacy", Mode: "unknown"REDACTED,
		{GroupID: 1, Platform: "legacy", Mode: "unknown"REDACTED,
		{GroupID: 2, Platform: "legacy", Mode: "unknown"REDACTED,
REDACTED
	cache := newFullRebuildLifecycleCache(historical...)
	groups := &fullRebuildLifecycleGroupRepo{
		fresh: map[int64]*Group{
			1: {ID: 1, Status: StatusDisabled, Hydrated: trueREDACTED,
			3: {ID: 3, Status: StatusDisabled, Hydrated: trueREDACTED,
	REDACTED,
		freshErr: make(map[int64]error),
REDACTED
	accounts := &fullRebuildAccountRepo{REDACTED
	svc := newFullRebuildLifecycleService(cache, nil, accounts, groups, config.RunModeStandard)

	require.NoError(t, svc.rebuildFullSnapshot(context.Background(), "first"))
	retireCallsAfterFirst := len(cache.retiredBuckets())
	require.NoError(t, svc.rebuildFullSnapshot(context.Background(), "second"))
	_, _, freshCalls := groups.stats()
	require.Equal(t, []int64{1, 2, 3REDACTED, freshCalls)
	require.Equal(t, retireCallsAfterFirst, len(cache.retiredBuckets()))
	require.Empty(t, cache.tokens())
	require.Zero(t, accounts.groupCallCount(1))
	require.Zero(t, accounts.groupCallCount(2))
	require.Zero(t, accounts.groupCallCount(3))
	_, _, listCalls := cache.lifecycleCounts()
	require.Equal(t, 2, listCalls, "each round must use only its one global registry snapshot")
	for _, bucket := range historical {
		require.Contains(t, bucketStrings(cache.retiredBuckets()), bucket.String())
REDACTED
REDACTED

func TestSchedulerFullRebuildPartialLifecycleFailureReturnsBeforeDBAndRetries(t *testing.T) {
	historical := []SchedulerBucket{
		{GroupID: 1, Platform: "legacy", Mode: "unknown"REDACTED,
		{GroupID: 2, Platform: "legacy", Mode: "unknown"REDACTED,
		{GroupID: 3, Platform: "legacy", Mode: "unknown"REDACTED,
REDACTED
	wantErr := errors.New("fresh group query failed")
	cache := newFullRebuildLifecycleCache(historical...)
	groups := &fullRebuildLifecycleGroupRepo{
		fresh: map[int64]*Group{
			1: {ID: 1, Status: StatusDisabled, Hydrated: trueREDACTED,
			3: {ID: 3, Status: StatusDisabled, Hydrated: trueREDACTED,
	REDACTED,
		freshErr: map[int64]error{2: wantErrREDACTED,
REDACTED
	accounts := &fullRebuildAccountRepo{REDACTED
	svc := newFullRebuildLifecycleService(cache, nil, accounts, groups, config.RunModeStandard)

	err := svc.triggerFullRebuild("first")
	require.ErrorIs(t, err, wantErr)
	_, _, freshCalls := groups.stats()
	require.Equal(t, []int64{1, 2REDACTED, freshCalls)
	require.Zero(t, accounts.callCount())
	require.Zero(t, cache.totalSetAttempts())
	require.Equal(t, 13, len(cache.retiredBuckets()))

	groups.mu.Lock()
	delete(groups.freshErr, 2)
	groups.fresh[2] = &Group{ID: 2, Status: StatusDisabled, Hydrated: trueREDACTED
	groups.mu.Unlock()
	require.NoError(t, svc.triggerFullRebuild("retry"))
	_, _, freshCalls = groups.stats()
	require.Equal(t, []int64{1, 2, 2, 3REDACTED, freshCalls)
	require.Equal(t, 39, len(cache.retiredBuckets()))
	require.Equal(t, 12, accounts.callCount())
	require.Empty(t, cache.tokens())
REDACTED

func TestSchedulerFullRebuildActiveTombstoneLazyRecoveryDiscardsPartialCaptureTasks(t *testing.T) {
	const groupID int64 = 105
	canonical := schedulerBucketsForGroup(groupID)
	historical := SchedulerBucket{GroupID: groupID, Platform: "legacy", Mode: "unknown"REDACTED
	cache := newFullRebuildLifecycleCache(canonical[0], canonical[4], historical)
	require.NoError(t, cache.retirementRaceCache.RetireBucket(context.Background(), canonical[5]))
	groups := &fullRebuildLifecycleGroupRepo{
		activeIDs: []int64{groupIDREDACTED,
		fresh: map[int64]*Group{
			groupID: {ID: groupID, Status: StatusActive, Hydrated: trueREDACTED,
	REDACTED,
		freshErr: make(map[int64]error),
REDACTED
	accounts := &fullRebuildAccountRepo{REDACTED
	var capturesAtFirstDB int
	accounts.beforeFirst = func() {
		capturesAtFirstDB = cache.captureAttemptCount()
		held, reopenCount := cache.leaseHeldAndTokenCount()
		require.False(t, held)
		require.Equal(t, 12, reopenCount)
		require.Equal(t, 19, capturesAtFirstDB)
REDACTED
	svc := newFullRebuildLifecycleService(cache, nil, accounts, groups, config.RunModeStandard)

	require.NoError(t, svc.rebuildFullSnapshot(context.Background(), "test"))
	require.Equal(t, capturesAtFirstDB, cache.captureAttemptCount())
	require.Equal(t, 25, accounts.callCount())
	for _, bucket := range canonical {
		attempts, published := cache.counts(bucket)
		require.Equal(t, 1, attempts, "discarded pre-recovery tokens must never publish: %s", bucket.String())
		require.Equal(t, 1, published, bucket.String())
REDACTED
	_, published := cache.counts(historical)
	require.Equal(t, 1, published)
	_, _, listCalls := cache.lifecycleCounts()
	require.Equal(t, 1, listCalls)
REDACTED

func TestSchedulerFullRebuildSimpleModePreservesRegistryWithoutLifecycleAuthority(t *testing.T) {
	registered := []SchedulerBucket{
		{GroupID: 0, Platform: "legacy-zero", Mode: "unknown"REDACTED,
		{GroupID: 106, Platform: "legacy-positive", Mode: "unknown"REDACTED,
		{GroupID: -8, Platform: "legacy-negative", Mode: "unknown"REDACTED,
REDACTED
	cache := newFullRebuildLifecycleCache(registered...)
	groups := &fullRebuildLifecycleGroupRepo{
		activeIDsErr: errors.New("simple mode must not query groups"),
		fresh:        make(map[int64]*Group),
		freshErr:     make(map[int64]error),
REDACTED
	accounts := &fullRebuildAccountRepo{REDACTED
	svc := newFullRebuildLifecycleService(cache, nil, accounts, groups, config.RunModeSimple)

	require.NoError(t, svc.rebuildFullSnapshot(context.Background(), "test"))
	activeCalls, fallbackCalls, freshCalls := groups.stats()
	require.Zero(t, activeCalls)
	require.Zero(t, fallbackCalls)
	require.Empty(t, freshCalls)
	require.Equal(t, 15, cache.captureAttemptCount())
	require.Equal(t, 15, accounts.callCount())
	require.Equal(t, 15, accounts.groupCallCount(0))
	require.Empty(t, cache.retiredBuckets())
	require.Empty(t, cache.tokens())
	for _, bucket := range registered {
		_, published := cache.counts(bucket)
		require.Equal(t, 1, published, bucket.String())
REDACTED
REDACTED

func TestSchedulerFullRebuildFreshReopenLockBusyRetriesWithoutBlockingOrdinaryTasks(t *testing.T) {
	const groupID int64 = 107
	canonical := schedulerBucketsForGroup(groupID)
	cache := newFullRebuildLifecycleCache()
	require.NoError(t, cache.retirementRaceCache.RetireBucket(context.Background(), canonical[0]))
	cache.lockBusyOnce[canonical[0].String()] = true
	groups := &fullRebuildLifecycleGroupRepo{
		activeIDs: []int64{groupIDREDACTED,
		fresh: map[int64]*Group{
			groupID: {ID: groupID, Status: StatusActive, Hydrated: trueREDACTED,
	REDACTED,
		freshErr: make(map[int64]error),
REDACTED
	accounts := &fullRebuildAccountRepo{REDACTED
	outbox := &outboxCleanupRepo{events: []SchedulerOutboxEvent{{ID: 1, EventType: SchedulerOutboxEventFullRebuildREDACTEDREDACTEDREDACTED
	svc := newFullRebuildLifecycleService(cache, outbox, accounts, groups, config.RunModeStandard)

	svc.pollOutbox()
	require.Zero(t, cache.currentWatermark())
	_, groupZeroPublished := cache.counts(schedulerCanonicalBuckets(0)[0])
	require.Equal(t, 1, groupZeroPublished, "ordinary tasks must still run when one strict Reopen task is busy")
	require.Equal(t, 23, accounts.callCount())

	svc.pollOutbox()
	require.Equal(t, int64(1), cache.currentWatermark())
	require.Equal(t, 47, accounts.callCount())
	_, busyBucketPublished := cache.counts(canonical[0])
	require.Equal(t, 1, busyBucketPublished)
	activeCalls, fallbackCalls, freshCalls := groups.stats()
	require.Equal(t, 2, activeCalls)
	require.Zero(t, fallbackCalls)
	require.Equal(t, []int64{groupIDREDACTED, freshCalls)
REDACTED

func TestSchedulerFullRebuildOrdinaryLockBusyKeepsExistingSkipSemantics(t *testing.T) {
	busyBucket := schedulerCanonicalBuckets(0)[0]
	cache := newFullRebuildLifecycleCache()
	cache.lockBusyOnce[busyBucket.String()] = true
	groups := &fullRebuildLifecycleGroupRepo{fresh: make(map[int64]*Group), freshErr: make(map[int64]error)REDACTED
	accounts := &fullRebuildAccountRepo{REDACTED
	svc := newFullRebuildLifecycleService(cache, nil, accounts, groups, config.RunModeStandard)

	require.NoError(t, svc.rebuildFullSnapshot(context.Background(), "test"))
	require.Equal(t, 11, accounts.callCount())
	attempts, published := cache.counts(busyBucket)
	require.Zero(t, attempts)
	require.Zero(t, published)
REDACTED

func requireFullRebuildNoMutationOrDB(t *testing.T, cache *fullRebuildLifecycleCache, accounts *fullRebuildAccountRepo) {
REDACTED
	require.Zero(t, cache.captureAttemptCount())
	require.Empty(t, cache.retiredBuckets())
	require.Empty(t, cache.tokens())
	require.Zero(t, accounts.callCount())
	require.Zero(t, cache.totalSetAttempts())
REDACTED
