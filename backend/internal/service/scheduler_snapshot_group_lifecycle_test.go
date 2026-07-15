//go:build unit

package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type groupLifecycleTestCache struct {
	*retirementRaceCache

	stateMu sync.Mutex

	leaseHeld       bool
	lease           SchedulerGroupLifecycleLease
	leaseSequence   int
	leaseBusy       bool
	leaseAcquireErr error
	leaseReleaseErr error
	acquireCalls    int
	releaseCalls    int
	acquireTTL      time.Duration
	acquireDeadline bool
	releaseDeadline bool
	releaseCtxErr   error

	listErr   error
	listCalls int

	retireCalls  []SchedulerBucket
	reopenTokens []SchedulerBucketWriteToken
	retireHeld   []bool
	reopenHeld   []bool
	retireErr    error
	retireErrAt  int
	reopenErr    error
	reopenErrAt  int

	bucketLockBusy bool
	bucketLockErr  error
	bucketLockTTLs []time.Duration
	unlockCalls    int
	setErr         error
REDACTED

func newGroupLifecycleTestCache(buckets ...SchedulerBucket) *groupLifecycleTestCache {
	return &groupLifecycleTestCache{retirementRaceCache: newRetirementRaceCache(buckets...)REDACTED
REDACTED

func (c *groupLifecycleTestCache) TryAcquireGroupLifecycleLease(ctx context.Context, groupID int64, ttl time.Duration) (SchedulerGroupLifecycleLease, bool, error) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.acquireCalls++
	c.acquireTTL = ttl
	_, c.acquireDeadline = ctx.Deadline()
	if c.leaseAcquireErr != nil {
		return SchedulerGroupLifecycleLease{REDACTED, false, c.leaseAcquireErr
REDACTED
	if c.leaseBusy || c.leaseHeld {
		return SchedulerGroupLifecycleLease{REDACTED, false, nil
REDACTED
	c.leaseSequence++
	c.lease = SchedulerGroupLifecycleLease{GroupID: groupID, OwnerToken: fmt.Sprintf("owner-%d", c.leaseSequence)REDACTED
	c.leaseHeld = true
	return c.lease, true, nil
REDACTED

func (c *groupLifecycleTestCache) ReleaseGroupLifecycleLease(ctx context.Context, lease SchedulerGroupLifecycleLease) error {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.releaseCalls++
	_, c.releaseDeadline = ctx.Deadline()
	c.releaseCtxErr = ctx.Err()
	if c.leaseReleaseErr != nil {
		return c.leaseReleaseErr
REDACTED
	if !c.leaseHeld || lease != c.lease {
		return ErrSchedulerGroupLifecycleLeaseLost
REDACTED
	c.leaseHeld = false
	return nil
REDACTED

func (c *groupLifecycleTestCache) RetireBucket(ctx context.Context, bucket SchedulerBucket) error {
	c.stateMu.Lock()
	c.retireCalls = append(c.retireCalls, bucket)
	c.retireHeld = append(c.retireHeld, c.leaseHeld)
	held := c.leaseHeld
	call := len(c.retireCalls)
	err := c.retireErr
	errAt := c.retireErrAt
	c.stateMu.Unlock()
	if !held {
		return errors.New("retire called outside group lifecycle lease")
REDACTED
	if err != nil && (errAt <= 0 || call == errAt) {
		return err
REDACTED
	return c.retirementRaceCache.RetireBucket(ctx, bucket)
REDACTED

func (c *groupLifecycleTestCache) ReopenBucket(ctx context.Context, bucket SchedulerBucket) (SchedulerBucketWriteToken, error) {
	if err := ctx.Err(); err != nil {
		return SchedulerBucketWriteToken{REDACTED, err
REDACTED
	c.stateMu.Lock()
	c.reopenHeld = append(c.reopenHeld, c.leaseHeld)
	held := c.leaseHeld
	call := len(c.reopenHeld)
	reopenErr := c.reopenErr
	reopenErrAt := c.reopenErrAt
	c.stateMu.Unlock()
	if !held {
		return SchedulerBucketWriteToken{REDACTED, errors.New("reopen called outside group lifecycle lease")
REDACTED
	if reopenErr != nil && (reopenErrAt <= 0 || call == reopenErrAt) {
		return SchedulerBucketWriteToken{REDACTED, reopenErr
REDACTED
	token, err := c.retirementRaceCache.ReopenBucket(ctx, bucket)
	if err != nil {
		return SchedulerBucketWriteToken{REDACTED, err
REDACTED
	c.stateMu.Lock()
	c.reopenTokens = append(c.reopenTokens, token)
	c.stateMu.Unlock()
	return token, nil
REDACTED

func (c *groupLifecycleTestCache) ListBuckets(ctx context.Context) ([]SchedulerBucket, error) {
	c.stateMu.Lock()
	c.listCalls++
	err := c.listErr
	c.stateMu.Unlock()
	if err != nil {
		return nil, err
REDACTED
	return c.retirementRaceCache.ListBuckets(ctx)
REDACTED

func (c *groupLifecycleTestCache) TryLockBucket(_ context.Context, _ SchedulerBucket, ttl time.Duration) (bool, error) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.bucketLockTTLs = append(c.bucketLockTTLs, ttl)
	if c.bucketLockErr != nil {
		return false, c.bucketLockErr
REDACTED
	return !c.bucketLockBusy, nil
REDACTED

func (c *groupLifecycleTestCache) UnlockBucket(context.Context, SchedulerBucket) error {
	c.stateMu.Lock()
	c.unlockCalls++
	c.stateMu.Unlock()
	return nil
REDACTED

func (c *groupLifecycleTestCache) SetSnapshot(ctx context.Context, bucket SchedulerBucket, token SchedulerBucketWriteToken, accounts []Account) error {
	c.stateMu.Lock()
	err := c.setErr
	c.stateMu.Unlock()
	if err != nil {
		return err
REDACTED
	return c.retirementRaceCache.SetSnapshot(ctx, bucket, token, accounts)
REDACTED

func (c *groupLifecycleTestCache) lifecycleCounts() (acquires, releases, listCalls int) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	return c.acquireCalls, c.releaseCalls, c.listCalls
REDACTED

func (c *groupLifecycleTestCache) retiredBuckets() []SchedulerBucket {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	return append([]SchedulerBucket(nil), c.retireCalls...)
REDACTED

func (c *groupLifecycleTestCache) tokens() []SchedulerBucketWriteToken {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	return append([]SchedulerBucketWriteToken(nil), c.reopenTokens...)
REDACTED

func (c *groupLifecycleTestCache) leaseHeldAndTokenCount() (bool, int) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	return c.leaseHeld, len(c.reopenTokens)
REDACTED

func (c *groupLifecycleTestCache) lockStats() ([]time.Duration, int) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	return append([]time.Duration(nil), c.bucketLockTTLs...), c.unlockCalls
REDACTED

func (c *groupLifecycleTestCache) lifecycleMutationLeaseStates() (retire, reopen []bool) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	return append([]bool(nil), c.retireHeld...), append([]bool(nil), c.reopenHeld...)
REDACTED

type groupLifecycleTestGroupRepo struct {
	GroupRepository

	mu       sync.Mutex
	group    *Group
	err      error
	calls    int
	afterGet func()
REDACTED

func (r *groupLifecycleTestGroupRepo) GetByIDLite(context.Context, int64) (*Group, error) {
	r.mu.Lock()
	r.calls++
	if r.err != nil {
		err := r.err
		r.mu.Unlock()
		return nil, err
REDACTED
	if r.group == nil {
		r.mu.Unlock()
		return nil, ErrGroupNotFound
REDACTED
	copyGroup := *r.group
	afterGet := r.afterGet
	r.mu.Unlock()
	if afterGet != nil {
		afterGet()
REDACTED
	return &copyGroup, nil
REDACTED

func (r *groupLifecycleTestGroupRepo) set(group *Group, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.group = group
	r.err = err
REDACTED

func (r *groupLifecycleTestGroupRepo) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
REDACTED

type groupLifecycleTestAccountRepo struct {
	AccountRepository

	mu              sync.Mutex
	calls           int
	callsByPlatform map[string]int
	err             error
	started         chan struct{REDACTED
	release         chan struct{REDACTED
	once            sync.Once
	beforeLoad      func()
	beforeLoadOnce  sync.Once
REDACTED

func (r *groupLifecycleTestAccountRepo) load(ctx context.Context, platform string) ([]Account, error) {
	r.mu.Lock()
	r.calls++
	if r.callsByPlatform == nil {
		r.callsByPlatform = make(map[string]int)
REDACTED
	r.callsByPlatform[platform]++
	err := r.err
	started := r.started
	release := r.release
	r.mu.Unlock()
	if started != nil {
		r.once.Do(func() { close(started) REDACTED)
REDACTED
	if r.beforeLoad != nil {
		r.beforeLoadOnce.Do(r.beforeLoad)
REDACTED
	if release != nil {
		select {
		case <-release:
		case <-ctx.Done():
			return nil, ctx.Err()
	REDACTED
REDACTED
	if err != nil {
		return nil, err
REDACTED
	return []Account{{ID: 9001, Platform: platform, Status: StatusActive, Schedulable: trueREDACTEDREDACTED, nil
REDACTED

func (r *groupLifecycleTestAccountRepo) ListSchedulableByGroupIDAndPlatform(ctx context.Context, _ int64, platform string) ([]Account, error) {
	return r.load(ctx, platform)
REDACTED

func (r *groupLifecycleTestAccountRepo) ListSchedulableByGroupIDAndPlatforms(ctx context.Context, _ int64, platforms []string) ([]Account, error) {
	platform := "mixed"
	if len(platforms) > 0 {
		platform = platforms[0]
REDACTED
	return r.load(ctx, platform)
REDACTED

func (r *groupLifecycleTestAccountRepo) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
REDACTED

func (r *groupLifecycleTestAccountRepo) platformCallCount(platform string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.callsByPlatform[platform]
REDACTED

func newGroupLifecycleTestService(cache SchedulerCache, accounts AccountRepository, groups GroupRepository, runMode string) *SchedulerSnapshotService {
	return NewSchedulerSnapshotService(cache, nil, accounts, groups, &config.Config{RunMode: runModeREDACTED)
REDACTED

func expectedGroupLifecycleBuckets(groupID int64) []SchedulerBucket {
	platforms := []string{PlatformAnthropic, PlatformGemini, PlatformOpenAI, PlatformAntigravity, PlatformGrokREDACTED
	buckets := make([]SchedulerBucket, 0, 12)
	for _, platform := range platforms {
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

func bucketStrings(buckets []SchedulerBucket) map[string]struct{REDACTED {
	out := make(map[string]struct{REDACTED, len(buckets))
	for _, bucket := range buckets {
		out[bucket.String()] = struct{REDACTED{REDACTED
REDACTED
	return out
REDACTED

func requireLifecycleSeen(t *testing.T, seen map[batchSeenKey]struct{REDACTED, groupID int64) {
REDACTED
	_, ok := seen[batchSeenKey{groupID: groupID, lifecycle: trueREDACTED]
	require.True(t, ok)
	for _, platform := range schedulerSnapshotPlatforms() {
		_, ok = seen[batchSeenKey{groupID: groupID, platform: platformREDACTED]
		require.True(t, ok)
REDACTED
REDACTED

func requireLifecycleNotSeen(t *testing.T, seen map[batchSeenKey]struct{REDACTED, groupID int64) {
REDACTED
	_, ok := seen[batchSeenKey{groupID: groupID, lifecycle: trueREDACTED]
	require.False(t, ok)
	for _, platform := range schedulerSnapshotPlatforms() {
		_, ok = seen[batchSeenKey{groupID: groupID, platform: platformREDACTED]
		require.False(t, ok)
REDACTED
REDACTED

func TestSchedulerGroupLifecycleInactiveAndMissingRetireAllHistoricalBucketsWithoutAccountReads(t *testing.T) {
	for _, tc := range []struct {
		name  string
		group *Group
		err   error
REDACTED{
		{name: "inactive", group: &Group{ID: 81, Status: StatusDisabled, Hydrated: trueREDACTEDREDACTED,
		{name: "missing", err: ErrGroupNotFoundREDACTED,
REDACTED {
		t.Run(tc.name, func(t *testing.T) {
			const groupID int64 = 81
			current := expectedGroupLifecycleBuckets(groupID)
			historical := SchedulerBucket{GroupID: groupID, Platform: "legacy", Mode: "obsolete"REDACTED
			other := SchedulerBucket{GroupID: groupID + 1, Platform: PlatformOpenAI, Mode: SchedulerModeForcedREDACTED
			groupZero := SchedulerBucket{GroupID: 0, Platform: PlatformOpenAI, Mode: SchedulerModeForcedREDACTED
			cache := newGroupLifecycleTestCache(current[0], historical, other, groupZero)
			groups := &groupLifecycleTestGroupRepo{group: tc.group, err: tc.errREDACTED
			accounts := &groupLifecycleTestAccountRepo{REDACTED
			svc := newGroupLifecycleTestService(cache, accounts, groups, config.RunModeStandard)
			seen := make(map[batchSeenKey]struct{REDACTED)

			require.NoError(t, svc.handleGroupEvent(context.Background(), ptrInt64(groupID), seen))

			expected := bucketStrings(append(current, historical))
			got := bucketStrings(cache.retiredBuckets())
			require.Equal(t, expected, got)
			retireHeld, _ := cache.lifecycleMutationLeaseStates()
			require.Len(t, retireHeld, len(expected))
			for _, held := range retireHeld {
				require.True(t, held)
		REDACTED
			require.NotContains(t, got, other.String())
			require.NotContains(t, got, groupZero.String())
			require.Zero(t, accounts.callCount())
			require.Equal(t, 1, groups.callCount())
			_, _, listCalls := cache.lifecycleCounts()
			require.Equal(t, 1, listCalls)
			requireLifecycleSeen(t, seen, groupID)
	REDACTED)
REDACTED
REDACTED

func TestSchedulerPrepareGroupLifecycleUsesKnownHistoricalBucketsWithoutListingRegistry(t *testing.T) {
	const groupID int64 = 811
	historical := SchedulerBucket{GroupID: groupID, Platform: "legacy", Mode: "obsolete"REDACTED
	cache := newGroupLifecycleTestCache()
	cache.listErr = errors.New("registry must not be listed")
	groups := &groupLifecycleTestGroupRepo{group: &Group{ID: groupID, Status: StatusDisabled, Hydrated: trueREDACTEDREDACTED
	accounts := &groupLifecycleTestAccountRepo{REDACTED
	svc := newGroupLifecycleTestService(cache, accounts, groups, config.RunModeStandard)

	plan, err := svc.prepareGroupLifecycle(context.Background(), groupID, []SchedulerBucket{historicalREDACTED)
REDACTED
	require.False(t, plan.active)
	require.Empty(t, plan.tasks)
	_, _, listCalls := cache.lifecycleCounts()
	require.Zero(t, listCalls)
	require.Contains(t, bucketStrings(cache.retiredBuckets()), historical.String())
	require.Zero(t, accounts.callCount())
REDACTED

func TestSchedulerGroupLifecycleActiveReopensAndRebuildsAllCurrentBuckets(t *testing.T) {
	const groupID int64 = 82
	current := expectedGroupLifecycleBuckets(groupID)
	historical := SchedulerBucket{GroupID: groupID, Platform: "legacy", Mode: "obsolete"REDACTED
	cache := newGroupLifecycleTestCache(historical)
	for _, bucket := range current {
		require.NoError(t, cache.retirementRaceCache.RetireBucket(context.Background(), bucket))
REDACTED
	groups := &groupLifecycleTestGroupRepo{group: &Group{ID: groupID, Platform: PlatformOpenAI, Status: StatusActive, Hydrated: trueREDACTEDREDACTED
	accounts := &groupLifecycleTestAccountRepo{REDACTED
	accounts.beforeLoad = func() {
		held, tokenCount := cache.leaseHeldAndTokenCount()
		require.False(t, held, "the group lifecycle lease must be released before the first account query")
		require.Equal(t, 12, tokenCount, "all reopen tokens must be prepared before the first account query")
REDACTED
	svc := newGroupLifecycleTestService(cache, accounts, groups, config.RunModeStandard)
	seen := make(map[batchSeenKey]struct{REDACTED)

	require.NoError(t, svc.handleGroupEvent(context.Background(), ptrInt64(groupID), seen))

	require.Equal(t, bucketStrings(current), bucketStrings(cache.reopens))
	require.Empty(t, cache.retiredBuckets())
	registered, err := cache.retirementRaceCache.ListBuckets(context.Background())
REDACTED
	require.Contains(t, bucketStrings(registered), historical.String())
	require.Len(t, cache.tokens(), 12)
	require.Equal(t, 7, accounts.callCount())
	require.Equal(t, 1, accounts.platformCallCount(PlatformOpenAI))
	for _, bucket := range current {
		_, published := cache.counts(bucket)
		require.Equal(t, 1, published, bucket.String())
REDACTED
	require.Contains(t, bucketStrings(current), SchedulerBucket{GroupID: groupID, Platform: PlatformAntigravity, Mode: SchedulerModeForcedREDACTED.String())
	require.Contains(t, bucketStrings(current), SchedulerBucket{GroupID: groupID, Platform: PlatformAnthropic, Mode: SchedulerModeMixedREDACTED.String())
	require.Contains(t, bucketStrings(current), SchedulerBucket{GroupID: groupID, Platform: PlatformGemini, Mode: SchedulerModeMixedREDACTED.String())
	acquires, releases, listCalls := cache.lifecycleCounts()
	require.Equal(t, 1, acquires)
	require.Equal(t, 1, releases)
	require.Zero(t, listCalls)
	require.Equal(t, schedulerGroupLifecycleLeaseTTL, cache.acquireTTL)
	require.True(t, cache.acquireDeadline)
	require.True(t, cache.releaseDeadline)
	require.NoError(t, cache.releaseCtxErr)
	_, reopenHeld := cache.lifecycleMutationLeaseStates()
	require.Len(t, reopenHeld, 12)
	for _, held := range reopenHeld {
		require.True(t, held)
REDACTED
	lockTTLs, unlockCalls := cache.lockStats()
	require.Len(t, lockTTLs, 12)
	for _, ttl := range lockTTLs {
		require.Equal(t, 30*time.Second, ttl)
REDACTED
	require.Equal(t, 12, unlockCalls)
	requireLifecycleSeen(t, seen, groupID)
REDACTED

func TestSchedulerGroupLifecycleInactiveThenActiveAuthoritativelyReopens(t *testing.T) {
	const groupID int64 = 83
	cache := newGroupLifecycleTestCache()
	groups := &groupLifecycleTestGroupRepo{group: &Group{ID: groupID, Status: StatusDisabled, Hydrated: trueREDACTEDREDACTED
	accounts := &groupLifecycleTestAccountRepo{REDACTED
	svc := newGroupLifecycleTestService(cache, accounts, groups, config.RunModeStandard)

	require.NoError(t, svc.handleGroupEvent(context.Background(), ptrInt64(groupID), make(map[batchSeenKey]struct{REDACTED)))
	require.Zero(t, accounts.callCount())
	groups.set(&Group{ID: groupID, Status: StatusActive, Hydrated: trueREDACTED, nil)
	require.NoError(t, svc.handleGroupEvent(context.Background(), ptrInt64(groupID), make(map[batchSeenKey]struct{REDACTED)))

	require.Len(t, cache.tokens(), 12)
	require.Equal(t, 7, accounts.callCount())
	for _, bucket := range expectedGroupLifecycleBuckets(groupID) {
		_, published := cache.counts(bucket)
		require.Equal(t, 1, published, bucket.String())
REDACTED
REDACTED

func TestSchedulerGroupLifecycleLaterInactiveFencesLongActiveRebuild(t *testing.T) {
	const groupID int64 = 84
	cache := newGroupLifecycleTestCache()
	groups := &groupLifecycleTestGroupRepo{group: &Group{ID: groupID, Status: StatusActive, Hydrated: trueREDACTEDREDACTED
	started := make(chan struct{REDACTED)
	release := make(chan struct{REDACTED)
	accounts := &groupLifecycleTestAccountRepo{started: started, release: releaseREDACTED
	svc := newGroupLifecycleTestService(cache, accounts, groups, config.RunModeStandard)
	activeSeen := make(map[batchSeenKey]struct{REDACTED)
	inactiveSeen := make(map[batchSeenKey]struct{REDACTED)
	activeResult := make(chan error, 1)

	go func() {
		activeResult <- svc.handleGroupEvent(context.Background(), ptrInt64(groupID), activeSeen)
REDACTED()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("active rebuild did not reach the account load")
REDACTED

	groups.set(&Group{ID: groupID, Status: StatusDisabled, Hydrated: trueREDACTED, nil)
	require.NoError(t, svc.handleGroupEvent(context.Background(), ptrInt64(groupID), inactiveSeen))
	close(release)
	err := <-activeResult
	require.ErrorIs(t, err, ErrSchedulerBucketRetired)
	requireLifecycleNotSeen(t, activeSeen, groupID)
	requireLifecycleSeen(t, inactiveSeen, groupID)
REDACTED

func TestSchedulerGroupLifecycleEpochPreventsABA(t *testing.T) {
	const groupID int64 = 85
	cache := newGroupLifecycleTestCache()
	groups := &groupLifecycleTestGroupRepo{group: &Group{ID: groupID, Status: StatusDisabled, Hydrated: trueREDACTEDREDACTED
	accounts := &groupLifecycleTestAccountRepo{REDACTED
	svc := newGroupLifecycleTestService(cache, accounts, groups, config.RunModeStandard)

	require.NoError(t, svc.handleGroupEvent(context.Background(), ptrInt64(groupID), make(map[batchSeenKey]struct{REDACTED)))
	groups.set(&Group{ID: groupID, Status: StatusActive, Hydrated: trueREDACTED, nil)
	require.NoError(t, svc.handleGroupEvent(context.Background(), ptrInt64(groupID), make(map[batchSeenKey]struct{REDACTED)))
	firstActiveTokens := cache.tokens()
	require.Len(t, firstActiveTokens, 12)

	groups.set(&Group{ID: groupID, Status: StatusDisabled, Hydrated: trueREDACTED, nil)
	require.NoError(t, svc.handleGroupEvent(context.Background(), ptrInt64(groupID), make(map[batchSeenKey]struct{REDACTED)))
	groups.set(&Group{ID: groupID, Status: StatusActive, Hydrated: trueREDACTED, nil)
	require.NoError(t, svc.handleGroupEvent(context.Background(), ptrInt64(groupID), make(map[batchSeenKey]struct{REDACTED)))
	allTokens := cache.tokens()
	require.Len(t, allTokens, 24)
	require.Greater(t, allTokens[12].Epoch, firstActiveTokens[0].Epoch)
	require.ErrorIs(t, cache.SetSnapshot(context.Background(), firstActiveTokens[0].Bucket, firstActiveTokens[0], nil), ErrSchedulerBucketWriteFenced)
REDACTED

func TestSchedulerGroupLifecycleSeenIsIndependentAndDeduplicatesGroupEvents(t *testing.T) {
	const groupID int64 = 86
	cache := newGroupLifecycleTestCache()
	groups := &groupLifecycleTestGroupRepo{group: &Group{ID: groupID, Status: StatusActive, Hydrated: trueREDACTEDREDACTED
	accounts := &groupLifecycleTestAccountRepo{REDACTED
	svc := newGroupLifecycleTestService(cache, accounts, groups, config.RunModeStandard)
	seen := make(map[batchSeenKey]struct{REDACTED)
	for _, platform := range schedulerSnapshotPlatforms() {
		seen[batchSeenKey{groupID: groupID, platform: platformREDACTED] = struct{REDACTED{REDACTED
REDACTED

	require.NoError(t, svc.handleGroupEvent(context.Background(), ptrInt64(groupID), seen))
	require.Equal(t, 1, groups.callCount())
	require.Equal(t, 7, accounts.callCount())
	requireLifecycleSeen(t, seen, groupID)
	require.NoError(t, svc.handleGroupEvent(context.Background(), ptrInt64(groupID), seen))
	require.Equal(t, 1, groups.callCount())
	require.Equal(t, 7, accounts.callCount())
REDACTED

func TestSchedulerGroupLifecycleFailuresDoNotMarkSeen(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(*groupLifecycleTestCache, *groupLifecycleTestGroupRepo, *groupLifecycleTestAccountRepo)
		check   func(*testing.T, error)
REDACTED{
		{
			name: "lease busy",
			prepare: func(cache *groupLifecycleTestCache, _ *groupLifecycleTestGroupRepo, _ *groupLifecycleTestAccountRepo) {
				cache.leaseBusy = true
		REDACTED,
			check: func(t *testing.T, err error) { require.ErrorIs(t, err, ErrSchedulerGroupLifecycleLeaseBusy) REDACTED,
	REDACTED,
		{
			name: "lease error",
			prepare: func(cache *groupLifecycleTestCache, _ *groupLifecycleTestGroupRepo, _ *groupLifecycleTestAccountRepo) {
				cache.leaseAcquireErr = errors.New("lease failed")
		REDACTED,
			check: func(t *testing.T, err error) { require.EqualError(t, err, "lease failed") REDACTED,
	REDACTED,
		{
			name: "release lost",
			prepare: func(cache *groupLifecycleTestCache, _ *groupLifecycleTestGroupRepo, _ *groupLifecycleTestAccountRepo) {
				cache.leaseReleaseErr = ErrSchedulerGroupLifecycleLeaseLost
		REDACTED,
			check: func(t *testing.T, err error) { require.ErrorIs(t, err, ErrSchedulerGroupLifecycleLeaseLost) REDACTED,
	REDACTED,
		{
			name: "release error",
			prepare: func(cache *groupLifecycleTestCache, _ *groupLifecycleTestGroupRepo, _ *groupLifecycleTestAccountRepo) {
				cache.leaseReleaseErr = errors.New("release failed")
		REDACTED,
			check: func(t *testing.T, err error) { require.EqualError(t, err, "release failed") REDACTED,
	REDACTED,
		{
			name: "group query error",
			prepare: func(_ *groupLifecycleTestCache, groups *groupLifecycleTestGroupRepo, _ *groupLifecycleTestAccountRepo) {
				groups.err = errors.New("group query failed")
		REDACTED,
			check: func(t *testing.T, err error) { require.EqualError(t, err, "group query failed") REDACTED,
	REDACTED,
		{
			name: "list buckets error",
			prepare: func(cache *groupLifecycleTestCache, groups *groupLifecycleTestGroupRepo, _ *groupLifecycleTestAccountRepo) {
				groups.group.Status = StatusDisabled
				cache.listErr = errors.New("list buckets failed")
		REDACTED,
			check: func(t *testing.T, err error) { require.EqualError(t, err, "list buckets failed") REDACTED,
	REDACTED,
		{
			name: "retire bucket error",
			prepare: func(cache *groupLifecycleTestCache, groups *groupLifecycleTestGroupRepo, _ *groupLifecycleTestAccountRepo) {
				groups.group.Status = StatusDisabled
				cache.retireErr = errors.New("retire bucket failed")
				cache.retireErrAt = 2
		REDACTED,
			check: func(t *testing.T, err error) { require.EqualError(t, err, "retire bucket failed") REDACTED,
	REDACTED,
		{
			name: "reopen bucket error",
			prepare: func(cache *groupLifecycleTestCache, _ *groupLifecycleTestGroupRepo, _ *groupLifecycleTestAccountRepo) {
				cache.reopenErr = errors.New("reopen bucket failed")
				cache.reopenErrAt = 2
		REDACTED,
			check: func(t *testing.T, err error) { require.EqualError(t, err, "reopen bucket failed") REDACTED,
	REDACTED,
		{
			name: "account rebuild error",
			prepare: func(_ *groupLifecycleTestCache, _ *groupLifecycleTestGroupRepo, accounts *groupLifecycleTestAccountRepo) {
				accounts.err = errors.New("account load failed")
		REDACTED,
			check: func(t *testing.T, err error) { require.EqualError(t, err, "account load failed") REDACTED,
	REDACTED,
		{
			name: "bucket lock busy",
			prepare: func(cache *groupLifecycleTestCache, _ *groupLifecycleTestGroupRepo, _ *groupLifecycleTestAccountRepo) {
				cache.bucketLockBusy = true
		REDACTED,
			check: func(t *testing.T, err error) { require.ErrorIs(t, err, ErrSchedulerBucketRebuildBusy) REDACTED,
	REDACTED,
		{
			name: "bucket lock error",
			prepare: func(cache *groupLifecycleTestCache, _ *groupLifecycleTestGroupRepo, _ *groupLifecycleTestAccountRepo) {
				cache.bucketLockErr = errors.New("bucket lock failed")
		REDACTED,
			check: func(t *testing.T, err error) { require.EqualError(t, err, "bucket lock failed") REDACTED,
	REDACTED,
		{
			name: "set snapshot error",
			prepare: func(cache *groupLifecycleTestCache, _ *groupLifecycleTestGroupRepo, _ *groupLifecycleTestAccountRepo) {
				cache.setErr = errors.New("set snapshot failed")
		REDACTED,
			check: func(t *testing.T, err error) { require.EqualError(t, err, "set snapshot failed") REDACTED,
	REDACTED,
REDACTED

	for index, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			groupID := int64(870 + index)
			cache := newGroupLifecycleTestCache()
			groups := &groupLifecycleTestGroupRepo{group: &Group{ID: groupID, Status: StatusActive, Hydrated: trueREDACTEDREDACTED
			accounts := &groupLifecycleTestAccountRepo{REDACTED
			tc.prepare(cache, groups, accounts)
			svc := newGroupLifecycleTestService(cache, accounts, groups, config.RunModeStandard)
			seen := make(map[batchSeenKey]struct{REDACTED)

			err := svc.handleGroupEvent(context.Background(), ptrInt64(groupID), seen)
			tc.check(t, err)
			requireLifecycleNotSeen(t, seen, groupID)
			if tc.name == "release lost" || tc.name == "release error" {
				require.Zero(t, accounts.callCount())
		REDACTED
			if tc.name == "retire bucket error" || tc.name == "reopen bucket error" {
				_, releases, _ := cache.lifecycleCounts()
				require.Equal(t, 1, releases)
				require.Zero(t, accounts.callCount())
		REDACTED
			if tc.name == "account rebuild error" || tc.name == "set snapshot error" {
				lockTTLs, unlockCalls := cache.lockStats()
				require.Len(t, lockTTLs, 1)
				require.Equal(t, 1, unlockCalls)
				require.Equal(t, 1, accounts.callCount())
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestSchedulerGroupLifecycleOperationAndReleaseErrorsPreserveBothCauses(t *testing.T) {
	const groupID int64 = 880
	operationErr := errors.New("group query failed")
	cache := newGroupLifecycleTestCache()
	cache.leaseReleaseErr = ErrSchedulerGroupLifecycleLeaseLost
	groups := &groupLifecycleTestGroupRepo{err: operationErrREDACTED
	accounts := &groupLifecycleTestAccountRepo{REDACTED
	svc := newGroupLifecycleTestService(cache, accounts, groups, config.RunModeStandard)
	seen := make(map[batchSeenKey]struct{REDACTED)

	err := svc.handleGroupEvent(context.Background(), ptrInt64(groupID), seen)
	require.ErrorIs(t, err, operationErr)
	require.ErrorIs(t, err, ErrSchedulerGroupLifecycleLeaseLost)
	requireLifecycleNotSeen(t, seen, groupID)
	require.Zero(t, accounts.callCount())
REDACTED

func TestSchedulerGroupLifecycleUntrustedGroupStateFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		name  string
		group *Group
REDACTED{
		{name: "not hydrated", group: &Group{ID: 88, Status: StatusActiveREDACTEDREDACTED,
		{name: "mismatched id", group: &Group{ID: 89, Status: StatusActive, Hydrated: trueREDACTEDREDACTED,
REDACTED {
		t.Run(tc.name, func(t *testing.T) {
			const eventGroupID int64 = 88
			cache := newGroupLifecycleTestCache()
			groups := &groupLifecycleTestGroupRepo{group: tc.groupREDACTED
			accounts := &groupLifecycleTestAccountRepo{REDACTED
			svc := newGroupLifecycleTestService(cache, accounts, groups, config.RunModeStandard)
			seen := make(map[batchSeenKey]struct{REDACTED)

			err := svc.handleGroupEvent(context.Background(), ptrInt64(eventGroupID), seen)
		REDACTED
			require.Empty(t, cache.retiredBuckets())
			require.Empty(t, cache.tokens())
			require.Zero(t, accounts.callCount())
			requireLifecycleNotSeen(t, seen, eventGroupID)
			acquires, releases, listCalls := cache.lifecycleCounts()
			require.Equal(t, 1, acquires)
			require.Equal(t, 1, releases)
			require.Zero(t, listCalls)
	REDACTED)
REDACTED
REDACTED

func TestSchedulerGroupLifecycleCanceledAfterFreshQueryUsesIndependentReleaseContext(t *testing.T) {
	const groupID int64 = 89
	ctx, cancel := context.WithCancel(context.Background())
	cache := newGroupLifecycleTestCache()
	groups := &groupLifecycleTestGroupRepo{
		group:    &Group{ID: groupID, Status: StatusActive, Hydrated: trueREDACTED,
		afterGet: cancel,
REDACTED
	accounts := &groupLifecycleTestAccountRepo{REDACTED
	svc := newGroupLifecycleTestService(cache, accounts, groups, config.RunModeStandard)
	seen := make(map[batchSeenKey]struct{REDACTED)

	err := svc.handleGroupEvent(ctx, ptrInt64(groupID), seen)
	require.ErrorIs(t, err, context.Canceled)
	requireLifecycleNotSeen(t, seen, groupID)
	require.Empty(t, cache.tokens())
	require.Zero(t, accounts.callCount())
	acquires, releases, _ := cache.lifecycleCounts()
	require.Equal(t, 1, acquires)
	require.Equal(t, 1, releases)
	require.True(t, cache.releaseDeadline)
	require.NoError(t, cache.releaseCtxErr)
REDACTED

func TestSchedulerGroupLifecycleGroupZeroAndSimpleModeAreNoOps(t *testing.T) {
	cache := newGroupLifecycleTestCache()
	groups := &groupLifecycleTestGroupRepo{group: &Group{ID: 88, Status: StatusActive, Hydrated: trueREDACTEDREDACTED
	accounts := &groupLifecycleTestAccountRepo{REDACTED
	standard := newGroupLifecycleTestService(cache, accounts, groups, config.RunModeStandard)
	simple := newGroupLifecycleTestService(cache, accounts, groups, config.RunModeSimple)

	require.NoError(t, standard.handleGroupEvent(context.Background(), nil, make(map[batchSeenKey]struct{REDACTED)))
	require.NoError(t, standard.handleGroupEvent(context.Background(), ptrInt64(0), make(map[batchSeenKey]struct{REDACTED)))
	require.NoError(t, simple.handleGroupEvent(context.Background(), ptrInt64(88), make(map[batchSeenKey]struct{REDACTED)))

	acquires, releases, listCalls := cache.lifecycleCounts()
	require.Zero(t, acquires)
	require.Zero(t, releases)
	require.Zero(t, listCalls)
	require.Zero(t, groups.callCount())
	require.Zero(t, accounts.callCount())
REDACTED
