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
}

func newGroupLifecycleTestCache(buckets ...SchedulerBucket) *groupLifecycleTestCache {
	return &groupLifecycleTestCache{retirementRaceCache: newRetirementRaceCache(buckets...)}
}

func (c *groupLifecycleTestCache) TryAcquireGroupLifecycleLease(ctx context.Context, groupID int64, ttl time.Duration) (SchedulerGroupLifecycleLease, bool, error) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.acquireCalls++
	c.acquireTTL = ttl
	_, c.acquireDeadline = ctx.Deadline()
	if c.leaseAcquireErr != nil {
		return SchedulerGroupLifecycleLease{}, false, c.leaseAcquireErr
	}
	if c.leaseBusy || c.leaseHeld {
		return SchedulerGroupLifecycleLease{}, false, nil
	}
	c.leaseSequence++
	c.lease = SchedulerGroupLifecycleLease{GroupID: groupID, OwnerToken: fmt.Sprintf("owner-%d", c.leaseSequence)}
	c.leaseHeld = true
	return c.lease, true, nil
}

func (c *groupLifecycleTestCache) ReleaseGroupLifecycleLease(ctx context.Context, lease SchedulerGroupLifecycleLease) error {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.releaseCalls++
	_, c.releaseDeadline = ctx.Deadline()
	c.releaseCtxErr = ctx.Err()
	if c.leaseReleaseErr != nil {
		return c.leaseReleaseErr
	}
	if !c.leaseHeld || lease != c.lease {
		return ErrSchedulerGroupLifecycleLeaseLost
	}
	c.leaseHeld = false
	return nil
}

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
	}
	if err != nil && (errAt <= 0 || call == errAt) {
		return err
	}
	return c.retirementRaceCache.RetireBucket(ctx, bucket)
}

func (c *groupLifecycleTestCache) ReopenBucket(ctx context.Context, bucket SchedulerBucket) (SchedulerBucketWriteToken, error) {
	if err := ctx.Err(); err != nil {
		return SchedulerBucketWriteToken{}, err
	}
	c.stateMu.Lock()
	c.reopenHeld = append(c.reopenHeld, c.leaseHeld)
	held := c.leaseHeld
	call := len(c.reopenHeld)
	reopenErr := c.reopenErr
	reopenErrAt := c.reopenErrAt
	c.stateMu.Unlock()
	if !held {
		return SchedulerBucketWriteToken{}, errors.New("reopen called outside group lifecycle lease")
	}
	if reopenErr != nil && (reopenErrAt <= 0 || call == reopenErrAt) {
		return SchedulerBucketWriteToken{}, reopenErr
	}
	token, err := c.retirementRaceCache.ReopenBucket(ctx, bucket)
	if err != nil {
		return SchedulerBucketWriteToken{}, err
	}
	c.stateMu.Lock()
	c.reopenTokens = append(c.reopenTokens, token)
	c.stateMu.Unlock()
	return token, nil
}

func (c *groupLifecycleTestCache) ListBuckets(ctx context.Context) ([]SchedulerBucket, error) {
	c.stateMu.Lock()
	c.listCalls++
	err := c.listErr
	c.stateMu.Unlock()
	if err != nil {
		return nil, err
	}
	return c.retirementRaceCache.ListBuckets(ctx)
}

func (c *groupLifecycleTestCache) TryLockBucket(_ context.Context, _ SchedulerBucket, ttl time.Duration) (bool, error) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.bucketLockTTLs = append(c.bucketLockTTLs, ttl)
	if c.bucketLockErr != nil {
		return false, c.bucketLockErr
	}
	return !c.bucketLockBusy, nil
}

func (c *groupLifecycleTestCache) UnlockBucket(context.Context, SchedulerBucket) error {
	c.stateMu.Lock()
	c.unlockCalls++
	c.stateMu.Unlock()
	return nil
}

func (c *groupLifecycleTestCache) SetSnapshot(ctx context.Context, bucket SchedulerBucket, token SchedulerBucketWriteToken, accounts []Account) error {
	c.stateMu.Lock()
	err := c.setErr
	c.stateMu.Unlock()
	if err != nil {
		return err
	}
	return c.retirementRaceCache.SetSnapshot(ctx, bucket, token, accounts)
}

func (c *groupLifecycleTestCache) lifecycleCounts() (acquires, releases, listCalls int) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	return c.acquireCalls, c.releaseCalls, c.listCalls
}

func (c *groupLifecycleTestCache) retiredBuckets() []SchedulerBucket {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	return append([]SchedulerBucket(nil), c.retireCalls...)
}

func (c *groupLifecycleTestCache) tokens() []SchedulerBucketWriteToken {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	return append([]SchedulerBucketWriteToken(nil), c.reopenTokens...)
}

func (c *groupLifecycleTestCache) leaseHeldAndTokenCount() (bool, int) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	return c.leaseHeld, len(c.reopenTokens)
}

func (c *groupLifecycleTestCache) lockStats() ([]time.Duration, int) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	return append([]time.Duration(nil), c.bucketLockTTLs...), c.unlockCalls
}

func (c *groupLifecycleTestCache) lifecycleMutationLeaseStates() (retire, reopen []bool) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	return append([]bool(nil), c.retireHeld...), append([]bool(nil), c.reopenHeld...)
}

type groupLifecycleTestGroupRepo struct {
	GroupRepository

	mu       sync.Mutex
	group    *Group
	err      error
	calls    int
	afterGet func()
}

func (r *groupLifecycleTestGroupRepo) GetByIDLite(context.Context, int64) (*Group, error) {
	r.mu.Lock()
	r.calls++
	if r.err != nil {
		err := r.err
		r.mu.Unlock()
		return nil, err
	}
	if r.group == nil {
		r.mu.Unlock()
		return nil, ErrGroupNotFound
	}
	copyGroup := *r.group
	afterGet := r.afterGet
	r.mu.Unlock()
	if afterGet != nil {
		afterGet()
	}
	return &copyGroup, nil
}

func (r *groupLifecycleTestGroupRepo) set(group *Group, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.group = group
	r.err = err
}

func (r *groupLifecycleTestGroupRepo) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

type groupLifecycleTestAccountRepo struct {
	AccountRepository

	mu              sync.Mutex
	calls           int
	callsByPlatform map[string]int
	err             error
	started         chan struct{}
	release         chan struct{}
	once            sync.Once
	beforeLoad      func()
	beforeLoadOnce  sync.Once
}

func (r *groupLifecycleTestAccountRepo) load(ctx context.Context, platform string) ([]Account, error) {
	r.mu.Lock()
	r.calls++
	if r.callsByPlatform == nil {
		r.callsByPlatform = make(map[string]int)
	}
	r.callsByPlatform[platform]++
	err := r.err
	started := r.started
	release := r.release
	r.mu.Unlock()
	if started != nil {
		r.once.Do(func() { close(started) })
	}
	if r.beforeLoad != nil {
		r.beforeLoadOnce.Do(r.beforeLoad)
	}
	if release != nil {
		select {
		case <-release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if err != nil {
		return nil, err
	}
	return []Account{{ID: 9001, Platform: platform, Status: StatusActive, Schedulable: true}}, nil
}

func (r *groupLifecycleTestAccountRepo) ListSchedulableByGroupIDAndPlatform(ctx context.Context, _ int64, platform string) ([]Account, error) {
	return r.load(ctx, platform)
}

func (r *groupLifecycleTestAccountRepo) ListSchedulableByGroupIDAndPlatforms(ctx context.Context, _ int64, platforms []string) ([]Account, error) {
	platform := "mixed"
	if len(platforms) > 0 {
		platform = platforms[0]
	}
	return r.load(ctx, platform)
}

func (r *groupLifecycleTestAccountRepo) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

func (r *groupLifecycleTestAccountRepo) platformCallCount(platform string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.callsByPlatform[platform]
}

func newGroupLifecycleTestService(cache SchedulerCache, accounts AccountRepository, groups GroupRepository, runMode string) *SchedulerSnapshotService {
	return NewSchedulerSnapshotService(cache, nil, accounts, groups, &config.Config{RunMode: runMode})
}

func expectedGroupLifecycleBuckets(groupID int64) []SchedulerBucket {
	platforms := []string{PlatformAnthropic, PlatformGemini, PlatformOpenAI, PlatformAntigravity, PlatformGrok}
	buckets := make([]SchedulerBucket, 0, 12)
	for _, platform := range platforms {
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

func bucketStrings(buckets []SchedulerBucket) map[string]struct{} {
	out := make(map[string]struct{}, len(buckets))
	for _, bucket := range buckets {
		out[bucket.String()] = struct{}{}
	}
	return out
}

func requireLifecycleSeen(t *testing.T, seen map[batchSeenKey]struct{}, groupID int64) {
	t.Helper()
	_, ok := seen[batchSeenKey{groupID: groupID, lifecycle: true}]
	require.True(t, ok)
	for _, platform := range schedulerSnapshotPlatforms() {
		_, ok = seen[batchSeenKey{groupID: groupID, platform: platform}]
		require.True(t, ok)
	}
}

func requireLifecycleNotSeen(t *testing.T, seen map[batchSeenKey]struct{}, groupID int64) {
	t.Helper()
	_, ok := seen[batchSeenKey{groupID: groupID, lifecycle: true}]
	require.False(t, ok)
	for _, platform := range schedulerSnapshotPlatforms() {
		_, ok = seen[batchSeenKey{groupID: groupID, platform: platform}]
		require.False(t, ok)
	}
}

func TestSchedulerGroupLifecycleInactiveAndMissingRetireAllHistoricalBucketsWithoutAccountReads(t *testing.T) {
	for _, tc := range []struct {
		name  string
		group *Group
		err   error
	}{
		{name: "inactive", group: &Group{ID: 81, Status: StatusDisabled, Hydrated: true}},
		{name: "missing", err: ErrGroupNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const groupID int64 = 81
			current := expectedGroupLifecycleBuckets(groupID)
			historical := SchedulerBucket{GroupID: groupID, Platform: "legacy", Mode: "obsolete"}
			other := SchedulerBucket{GroupID: groupID + 1, Platform: PlatformOpenAI, Mode: SchedulerModeForced}
			groupZero := SchedulerBucket{GroupID: 0, Platform: PlatformOpenAI, Mode: SchedulerModeForced}
			cache := newGroupLifecycleTestCache(current[0], historical, other, groupZero)
			groups := &groupLifecycleTestGroupRepo{group: tc.group, err: tc.err}
			accounts := &groupLifecycleTestAccountRepo{}
			svc := newGroupLifecycleTestService(cache, accounts, groups, config.RunModeStandard)
			seen := make(map[batchSeenKey]struct{})

			require.NoError(t, svc.handleGroupEvent(context.Background(), ptrInt64(groupID), seen))

			expected := bucketStrings(append(current, historical))
			got := bucketStrings(cache.retiredBuckets())
			require.Equal(t, expected, got)
			retireHeld, _ := cache.lifecycleMutationLeaseStates()
			require.Len(t, retireHeld, len(expected))
			for _, held := range retireHeld {
				require.True(t, held)
			}
			require.NotContains(t, got, other.String())
			require.NotContains(t, got, groupZero.String())
			require.Zero(t, accounts.callCount())
			require.Equal(t, 1, groups.callCount())
			_, _, listCalls := cache.lifecycleCounts()
			require.Equal(t, 1, listCalls)
			requireLifecycleSeen(t, seen, groupID)
		})
	}
}

func TestSchedulerPrepareGroupLifecycleUsesKnownHistoricalBucketsWithoutListingRegistry(t *testing.T) {
	const groupID int64 = 811
	historical := SchedulerBucket{GroupID: groupID, Platform: "legacy", Mode: "obsolete"}
	cache := newGroupLifecycleTestCache()
	cache.listErr = errors.New("registry must not be listed")
	groups := &groupLifecycleTestGroupRepo{group: &Group{ID: groupID, Status: StatusDisabled, Hydrated: true}}
	accounts := &groupLifecycleTestAccountRepo{}
	svc := newGroupLifecycleTestService(cache, accounts, groups, config.RunModeStandard)

	plan, err := svc.prepareGroupLifecycle(context.Background(), groupID, []SchedulerBucket{historical})
	require.NoError(t, err)
	require.False(t, plan.active)
	require.Empty(t, plan.tasks)
	_, _, listCalls := cache.lifecycleCounts()
	require.Zero(t, listCalls)
	require.Contains(t, bucketStrings(cache.retiredBuckets()), historical.String())
	require.Zero(t, accounts.callCount())
}

func TestSchedulerGroupLifecycleActiveReopensAndRebuildsAllCurrentBuckets(t *testing.T) {
	const groupID int64 = 82
	current := expectedGroupLifecycleBuckets(groupID)
	historical := SchedulerBucket{GroupID: groupID, Platform: "legacy", Mode: "obsolete"}
	cache := newGroupLifecycleTestCache(historical)
	for _, bucket := range current {
		require.NoError(t, cache.retirementRaceCache.RetireBucket(context.Background(), bucket))
	}
	groups := &groupLifecycleTestGroupRepo{group: &Group{ID: groupID, Platform: PlatformOpenAI, Status: StatusActive, Hydrated: true}}
	accounts := &groupLifecycleTestAccountRepo{}
	accounts.beforeLoad = func() {
		held, tokenCount := cache.leaseHeldAndTokenCount()
		require.False(t, held, "the group lifecycle lease must be released before the first account query")
		require.Equal(t, 12, tokenCount, "all reopen tokens must be prepared before the first account query")
	}
	svc := newGroupLifecycleTestService(cache, accounts, groups, config.RunModeStandard)
	seen := make(map[batchSeenKey]struct{})

	require.NoError(t, svc.handleGroupEvent(context.Background(), ptrInt64(groupID), seen))

	require.Equal(t, bucketStrings(current), bucketStrings(cache.reopens))
	require.Empty(t, cache.retiredBuckets())
	registered, err := cache.retirementRaceCache.ListBuckets(context.Background())
	require.NoError(t, err)
	require.Contains(t, bucketStrings(registered), historical.String())
	require.Len(t, cache.tokens(), 12)
	require.Equal(t, 7, accounts.callCount())
	require.Equal(t, 1, accounts.platformCallCount(PlatformOpenAI))
	for _, bucket := range current {
		_, published := cache.counts(bucket)
		require.Equal(t, 1, published, bucket.String())
	}
	require.Contains(t, bucketStrings(current), SchedulerBucket{GroupID: groupID, Platform: PlatformAntigravity, Mode: SchedulerModeForced}.String())
	require.Contains(t, bucketStrings(current), SchedulerBucket{GroupID: groupID, Platform: PlatformAnthropic, Mode: SchedulerModeMixed}.String())
	require.Contains(t, bucketStrings(current), SchedulerBucket{GroupID: groupID, Platform: PlatformGemini, Mode: SchedulerModeMixed}.String())
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
	}
	lockTTLs, unlockCalls := cache.lockStats()
	require.Len(t, lockTTLs, 12)
	for _, ttl := range lockTTLs {
		require.Equal(t, 30*time.Second, ttl)
	}
	require.Equal(t, 12, unlockCalls)
	requireLifecycleSeen(t, seen, groupID)
}

func TestSchedulerGroupLifecycleInactiveThenActiveAuthoritativelyReopens(t *testing.T) {
	const groupID int64 = 83
	cache := newGroupLifecycleTestCache()
	groups := &groupLifecycleTestGroupRepo{group: &Group{ID: groupID, Status: StatusDisabled, Hydrated: true}}
	accounts := &groupLifecycleTestAccountRepo{}
	svc := newGroupLifecycleTestService(cache, accounts, groups, config.RunModeStandard)

	require.NoError(t, svc.handleGroupEvent(context.Background(), ptrInt64(groupID), make(map[batchSeenKey]struct{})))
	require.Zero(t, accounts.callCount())
	groups.set(&Group{ID: groupID, Status: StatusActive, Hydrated: true}, nil)
	require.NoError(t, svc.handleGroupEvent(context.Background(), ptrInt64(groupID), make(map[batchSeenKey]struct{})))

	require.Len(t, cache.tokens(), 12)
	require.Equal(t, 7, accounts.callCount())
	for _, bucket := range expectedGroupLifecycleBuckets(groupID) {
		_, published := cache.counts(bucket)
		require.Equal(t, 1, published, bucket.String())
	}
}

func TestSchedulerGroupLifecycleLaterInactiveFencesLongActiveRebuild(t *testing.T) {
	const groupID int64 = 84
	cache := newGroupLifecycleTestCache()
	groups := &groupLifecycleTestGroupRepo{group: &Group{ID: groupID, Status: StatusActive, Hydrated: true}}
	started := make(chan struct{})
	release := make(chan struct{})
	accounts := &groupLifecycleTestAccountRepo{started: started, release: release}
	svc := newGroupLifecycleTestService(cache, accounts, groups, config.RunModeStandard)
	activeSeen := make(map[batchSeenKey]struct{})
	inactiveSeen := make(map[batchSeenKey]struct{})
	activeResult := make(chan error, 1)

	go func() {
		activeResult <- svc.handleGroupEvent(context.Background(), ptrInt64(groupID), activeSeen)
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("active rebuild did not reach the account load")
	}

	groups.set(&Group{ID: groupID, Status: StatusDisabled, Hydrated: true}, nil)
	require.NoError(t, svc.handleGroupEvent(context.Background(), ptrInt64(groupID), inactiveSeen))
	close(release)
	err := <-activeResult
	require.ErrorIs(t, err, ErrSchedulerBucketRetired)
	requireLifecycleNotSeen(t, activeSeen, groupID)
	requireLifecycleSeen(t, inactiveSeen, groupID)
}

func TestSchedulerGroupLifecycleEpochPreventsABA(t *testing.T) {
	const groupID int64 = 85
	cache := newGroupLifecycleTestCache()
	groups := &groupLifecycleTestGroupRepo{group: &Group{ID: groupID, Status: StatusDisabled, Hydrated: true}}
	accounts := &groupLifecycleTestAccountRepo{}
	svc := newGroupLifecycleTestService(cache, accounts, groups, config.RunModeStandard)

	require.NoError(t, svc.handleGroupEvent(context.Background(), ptrInt64(groupID), make(map[batchSeenKey]struct{})))
	groups.set(&Group{ID: groupID, Status: StatusActive, Hydrated: true}, nil)
	require.NoError(t, svc.handleGroupEvent(context.Background(), ptrInt64(groupID), make(map[batchSeenKey]struct{})))
	firstActiveTokens := cache.tokens()
	require.Len(t, firstActiveTokens, 12)

	groups.set(&Group{ID: groupID, Status: StatusDisabled, Hydrated: true}, nil)
	require.NoError(t, svc.handleGroupEvent(context.Background(), ptrInt64(groupID), make(map[batchSeenKey]struct{})))
	groups.set(&Group{ID: groupID, Status: StatusActive, Hydrated: true}, nil)
	require.NoError(t, svc.handleGroupEvent(context.Background(), ptrInt64(groupID), make(map[batchSeenKey]struct{})))
	allTokens := cache.tokens()
	require.Len(t, allTokens, 24)
	require.Greater(t, allTokens[12].Epoch, firstActiveTokens[0].Epoch)
	require.ErrorIs(t, cache.SetSnapshot(context.Background(), firstActiveTokens[0].Bucket, firstActiveTokens[0], nil), ErrSchedulerBucketWriteFenced)
}

func TestSchedulerGroupLifecycleSeenIsIndependentAndDeduplicatesGroupEvents(t *testing.T) {
	const groupID int64 = 86
	cache := newGroupLifecycleTestCache()
	groups := &groupLifecycleTestGroupRepo{group: &Group{ID: groupID, Status: StatusActive, Hydrated: true}}
	accounts := &groupLifecycleTestAccountRepo{}
	svc := newGroupLifecycleTestService(cache, accounts, groups, config.RunModeStandard)
	seen := make(map[batchSeenKey]struct{})
	for _, platform := range schedulerSnapshotPlatforms() {
		seen[batchSeenKey{groupID: groupID, platform: platform}] = struct{}{}
	}

	require.NoError(t, svc.handleGroupEvent(context.Background(), ptrInt64(groupID), seen))
	require.Equal(t, 1, groups.callCount())
	require.Equal(t, 7, accounts.callCount())
	requireLifecycleSeen(t, seen, groupID)
	require.NoError(t, svc.handleGroupEvent(context.Background(), ptrInt64(groupID), seen))
	require.Equal(t, 1, groups.callCount())
	require.Equal(t, 7, accounts.callCount())
}

func TestSchedulerGroupLifecycleFailuresDoNotMarkSeen(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(*groupLifecycleTestCache, *groupLifecycleTestGroupRepo, *groupLifecycleTestAccountRepo)
		check   func(*testing.T, error)
	}{
		{
			name: "lease busy",
			prepare: func(cache *groupLifecycleTestCache, _ *groupLifecycleTestGroupRepo, _ *groupLifecycleTestAccountRepo) {
				cache.leaseBusy = true
			},
			check: func(t *testing.T, err error) { require.ErrorIs(t, err, ErrSchedulerGroupLifecycleLeaseBusy) },
		},
		{
			name: "lease error",
			prepare: func(cache *groupLifecycleTestCache, _ *groupLifecycleTestGroupRepo, _ *groupLifecycleTestAccountRepo) {
				cache.leaseAcquireErr = errors.New("lease failed")
			},
			check: func(t *testing.T, err error) { require.EqualError(t, err, "lease failed") },
		},
		{
			name: "release lost",
			prepare: func(cache *groupLifecycleTestCache, _ *groupLifecycleTestGroupRepo, _ *groupLifecycleTestAccountRepo) {
				cache.leaseReleaseErr = ErrSchedulerGroupLifecycleLeaseLost
			},
			check: func(t *testing.T, err error) { require.ErrorIs(t, err, ErrSchedulerGroupLifecycleLeaseLost) },
		},
		{
			name: "release error",
			prepare: func(cache *groupLifecycleTestCache, _ *groupLifecycleTestGroupRepo, _ *groupLifecycleTestAccountRepo) {
				cache.leaseReleaseErr = errors.New("release failed")
			},
			check: func(t *testing.T, err error) { require.EqualError(t, err, "release failed") },
		},
		{
			name: "group query error",
			prepare: func(_ *groupLifecycleTestCache, groups *groupLifecycleTestGroupRepo, _ *groupLifecycleTestAccountRepo) {
				groups.err = errors.New("group query failed")
			},
			check: func(t *testing.T, err error) { require.EqualError(t, err, "group query failed") },
		},
		{
			name: "list buckets error",
			prepare: func(cache *groupLifecycleTestCache, groups *groupLifecycleTestGroupRepo, _ *groupLifecycleTestAccountRepo) {
				groups.group.Status = StatusDisabled
				cache.listErr = errors.New("list buckets failed")
			},
			check: func(t *testing.T, err error) { require.EqualError(t, err, "list buckets failed") },
		},
		{
			name: "retire bucket error",
			prepare: func(cache *groupLifecycleTestCache, groups *groupLifecycleTestGroupRepo, _ *groupLifecycleTestAccountRepo) {
				groups.group.Status = StatusDisabled
				cache.retireErr = errors.New("retire bucket failed")
				cache.retireErrAt = 2
			},
			check: func(t *testing.T, err error) { require.EqualError(t, err, "retire bucket failed") },
		},
		{
			name: "reopen bucket error",
			prepare: func(cache *groupLifecycleTestCache, _ *groupLifecycleTestGroupRepo, _ *groupLifecycleTestAccountRepo) {
				cache.reopenErr = errors.New("reopen bucket failed")
				cache.reopenErrAt = 2
			},
			check: func(t *testing.T, err error) { require.EqualError(t, err, "reopen bucket failed") },
		},
		{
			name: "account rebuild error",
			prepare: func(_ *groupLifecycleTestCache, _ *groupLifecycleTestGroupRepo, accounts *groupLifecycleTestAccountRepo) {
				accounts.err = errors.New("account load failed")
			},
			check: func(t *testing.T, err error) { require.EqualError(t, err, "account load failed") },
		},
		{
			name: "bucket lock busy",
			prepare: func(cache *groupLifecycleTestCache, _ *groupLifecycleTestGroupRepo, _ *groupLifecycleTestAccountRepo) {
				cache.bucketLockBusy = true
			},
			check: func(t *testing.T, err error) { require.ErrorIs(t, err, ErrSchedulerBucketRebuildBusy) },
		},
		{
			name: "bucket lock error",
			prepare: func(cache *groupLifecycleTestCache, _ *groupLifecycleTestGroupRepo, _ *groupLifecycleTestAccountRepo) {
				cache.bucketLockErr = errors.New("bucket lock failed")
			},
			check: func(t *testing.T, err error) { require.EqualError(t, err, "bucket lock failed") },
		},
		{
			name: "set snapshot error",
			prepare: func(cache *groupLifecycleTestCache, _ *groupLifecycleTestGroupRepo, _ *groupLifecycleTestAccountRepo) {
				cache.setErr = errors.New("set snapshot failed")
			},
			check: func(t *testing.T, err error) { require.EqualError(t, err, "set snapshot failed") },
		},
	}

	for index, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			groupID := int64(870 + index)
			cache := newGroupLifecycleTestCache()
			groups := &groupLifecycleTestGroupRepo{group: &Group{ID: groupID, Status: StatusActive, Hydrated: true}}
			accounts := &groupLifecycleTestAccountRepo{}
			tc.prepare(cache, groups, accounts)
			svc := newGroupLifecycleTestService(cache, accounts, groups, config.RunModeStandard)
			seen := make(map[batchSeenKey]struct{})

			err := svc.handleGroupEvent(context.Background(), ptrInt64(groupID), seen)
			tc.check(t, err)
			requireLifecycleNotSeen(t, seen, groupID)
			if tc.name == "release lost" || tc.name == "release error" {
				require.Zero(t, accounts.callCount())
			}
			if tc.name == "retire bucket error" || tc.name == "reopen bucket error" {
				_, releases, _ := cache.lifecycleCounts()
				require.Equal(t, 1, releases)
				require.Zero(t, accounts.callCount())
			}
			if tc.name == "account rebuild error" || tc.name == "set snapshot error" {
				lockTTLs, unlockCalls := cache.lockStats()
				require.Len(t, lockTTLs, 1)
				require.Equal(t, 1, unlockCalls)
				require.Equal(t, 1, accounts.callCount())
			}
		})
	}
}

func TestSchedulerGroupLifecycleOperationAndReleaseErrorsPreserveBothCauses(t *testing.T) {
	const groupID int64 = 880
	operationErr := errors.New("group query failed")
	cache := newGroupLifecycleTestCache()
	cache.leaseReleaseErr = ErrSchedulerGroupLifecycleLeaseLost
	groups := &groupLifecycleTestGroupRepo{err: operationErr}
	accounts := &groupLifecycleTestAccountRepo{}
	svc := newGroupLifecycleTestService(cache, accounts, groups, config.RunModeStandard)
	seen := make(map[batchSeenKey]struct{})

	err := svc.handleGroupEvent(context.Background(), ptrInt64(groupID), seen)
	require.ErrorIs(t, err, operationErr)
	require.ErrorIs(t, err, ErrSchedulerGroupLifecycleLeaseLost)
	requireLifecycleNotSeen(t, seen, groupID)
	require.Zero(t, accounts.callCount())
}

func TestSchedulerGroupLifecycleUntrustedGroupStateFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		name  string
		group *Group
	}{
		{name: "not hydrated", group: &Group{ID: 88, Status: StatusActive}},
		{name: "mismatched id", group: &Group{ID: 89, Status: StatusActive, Hydrated: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const eventGroupID int64 = 88
			cache := newGroupLifecycleTestCache()
			groups := &groupLifecycleTestGroupRepo{group: tc.group}
			accounts := &groupLifecycleTestAccountRepo{}
			svc := newGroupLifecycleTestService(cache, accounts, groups, config.RunModeStandard)
			seen := make(map[batchSeenKey]struct{})

			err := svc.handleGroupEvent(context.Background(), ptrInt64(eventGroupID), seen)
			require.Error(t, err)
			require.Empty(t, cache.retiredBuckets())
			require.Empty(t, cache.tokens())
			require.Zero(t, accounts.callCount())
			requireLifecycleNotSeen(t, seen, eventGroupID)
			acquires, releases, listCalls := cache.lifecycleCounts()
			require.Equal(t, 1, acquires)
			require.Equal(t, 1, releases)
			require.Zero(t, listCalls)
		})
	}
}

func TestSchedulerGroupLifecycleCanceledAfterFreshQueryUsesIndependentReleaseContext(t *testing.T) {
	const groupID int64 = 89
	ctx, cancel := context.WithCancel(context.Background())
	cache := newGroupLifecycleTestCache()
	groups := &groupLifecycleTestGroupRepo{
		group:    &Group{ID: groupID, Status: StatusActive, Hydrated: true},
		afterGet: cancel,
	}
	accounts := &groupLifecycleTestAccountRepo{}
	svc := newGroupLifecycleTestService(cache, accounts, groups, config.RunModeStandard)
	seen := make(map[batchSeenKey]struct{})

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
}

func TestSchedulerGroupLifecycleGroupZeroAndSimpleModeAreNoOps(t *testing.T) {
	cache := newGroupLifecycleTestCache()
	groups := &groupLifecycleTestGroupRepo{group: &Group{ID: 88, Status: StatusActive, Hydrated: true}}
	accounts := &groupLifecycleTestAccountRepo{}
	standard := newGroupLifecycleTestService(cache, accounts, groups, config.RunModeStandard)
	simple := newGroupLifecycleTestService(cache, accounts, groups, config.RunModeSimple)

	require.NoError(t, standard.handleGroupEvent(context.Background(), nil, make(map[batchSeenKey]struct{})))
	require.NoError(t, standard.handleGroupEvent(context.Background(), ptrInt64(0), make(map[batchSeenKey]struct{})))
	require.NoError(t, simple.handleGroupEvent(context.Background(), ptrInt64(88), make(map[batchSeenKey]struct{})))

	acquires, releases, listCalls := cache.lifecycleCounts()
	require.Zero(t, acquires)
	require.Zero(t, releases)
	require.Zero(t, listCalls)
	require.Zero(t, groups.callCount())
	require.Zero(t, accounts.callCount())
}
