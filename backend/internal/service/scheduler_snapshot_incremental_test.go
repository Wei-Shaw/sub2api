//go:build unit

package service

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type incrementalAccountRepo struct {
	*batchAccountQueryRepo
	account *Account
	getErr  error
}

func newIncrementalAccountRepo(account *Account) *incrementalAccountRepo {
	return &incrementalAccountRepo{batchAccountQueryRepo: newBatchAccountQueryRepo(), account: account}
}

func (r *incrementalAccountRepo) GetByID(context.Context, int64) (*Account, error) {
	if r.getErr != nil {
		return nil, r.getErr
	}
	if r.account == nil {
		return nil, ErrAccountNotFound
	}
	copy := *r.account
	copy.GroupIDs = append([]int64(nil), r.account.GroupIDs...)
	return &copy, nil
}

type incrementalRemoval struct {
	bucket    SchedulerBucket
	accountID int64
}

type incrementalSnapshotCache struct {
	*bulkEventSnapshotCache

	removeMu    sync.Mutex
	removals    []incrementalRemoval
	removeError map[SchedulerBucket]error
}

func newIncrementalSnapshotCache() *incrementalSnapshotCache {
	return &incrementalSnapshotCache{
		bulkEventSnapshotCache: newBulkEventSnapshotCache(),
		removeError:            make(map[SchedulerBucket]error),
	}
}

func (c *incrementalSnapshotCache) RemoveAccountFromBucket(_ context.Context, bucket SchedulerBucket, token SchedulerBucketWriteToken, accountID int64) (bool, error) {
	c.batchSnapshotCache.mu.Lock()
	valid := token.ValidFor(bucket) && token == c.batchSnapshotCache.captured[bucket]
	c.batchSnapshotCache.mu.Unlock()
	if !valid {
		return false, ErrSchedulerBucketWriteFenced
	}
	c.removeMu.Lock()
	defer c.removeMu.Unlock()
	if err := c.removeError[bucket]; err != nil {
		return false, err
	}
	c.removals = append(c.removals, incrementalRemoval{bucket: bucket, accountID: accountID})
	return true, nil
}

func (c *incrementalSnapshotCache) removedBuckets() []SchedulerBucket {
	c.removeMu.Lock()
	defer c.removeMu.Unlock()
	buckets := make([]SchedulerBucket, 0, len(c.removals))
	for _, removal := range c.removals {
		buckets = append(buckets, removal.bucket)
	}
	return buckets
}

func newIncrementalTestService(cache SchedulerCache, repo AccountRepository) *SchedulerSnapshotService {
	cfg := &config.Config{RunMode: config.RunModeStandard}
	cfg.Gateway.Scheduling.IncrementalBucketUpdateEnabled = true
	return NewSchedulerSnapshotService(cache, nil, repo, nil, cfg)
}

func TestSchedulerAccountEventDisabledAccountOnlyRemovesMembers(t *testing.T) {
	cache := newIncrementalSnapshotCache()
	repo := newIncrementalAccountRepo(&Account{ID: 91, Platform: PlatformOpenAI, Status: StatusDisabled, GroupIDs: []int64{7}})
	svc := newIncrementalTestService(cache, repo)
	id := int64(91)

	err := svc.handleAccountEvent(context.Background(), &id, nil, make(map[batchSeenKey]struct{}), false)

	require.NoError(t, err)
	require.ElementsMatch(t, schedulerCanonicalBuckets(7), cache.removedBuckets())
	require.Empty(t, cache.batchSnapshotCache.writes)
	set, deleted := cache.accountWrites()
	require.Equal(t, []int64{id, id}, set)
	require.Empty(t, deleted)
}

func TestSchedulerAccountEventRebuildsOnlyCurrentPlatformAfterCleanup(t *testing.T) {
	cache := newIncrementalSnapshotCache()
	repo := newIncrementalAccountRepo(&Account{ID: 92, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, GroupIDs: []int64{8}})
	svc := newIncrementalTestService(cache, repo)
	id := int64(92)

	err := svc.handleAccountEvent(context.Background(), &id, map[string]any{"group_ids": []any{float64(8)}}, make(map[batchSeenKey]struct{}), false)

	require.NoError(t, err)
	require.ElementsMatch(t, schedulerCanonicalBuckets(8), cache.removedBuckets())
	written := make([]SchedulerBucket, 0, len(cache.batchSnapshotCache.writes))
	for bucket := range cache.batchSnapshotCache.writes {
		written = append(written, bucket)
	}
	require.ElementsMatch(t, schedulerBucketsForTest([]int64{8}, PlatformOpenAI), written)
}

func TestSchedulerAccountGroupEventAlsoCleansUngroupedBuckets(t *testing.T) {
	cache := newIncrementalSnapshotCache()
	repo := newIncrementalAccountRepo(&Account{ID: 93, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, GroupIDs: []int64{9}})
	svc := newIncrementalTestService(cache, repo)
	id := int64(93)

	err := svc.handleAccountEvent(context.Background(), &id, map[string]any{"group_ids": []any{float64(9)}}, make(map[batchSeenKey]struct{}), true)

	require.NoError(t, err)
	wantRemoved := append(schedulerCanonicalBuckets(0), schedulerCanonicalBuckets(9)...)
	require.ElementsMatch(t, wantRemoved, cache.removedBuckets())
}

func TestSchedulerAccountDeleteRemovesPayloadBucketsAndDeletesMetadataLast(t *testing.T) {
	cache := newIncrementalSnapshotCache()
	repo := newIncrementalAccountRepo(nil)
	repo.getErr = ErrAccountNotFound
	svc := newIncrementalTestService(cache, repo)
	id := int64(94)

	err := svc.handleAccountEvent(context.Background(), &id, map[string]any{"group_ids": []any{float64(10)}}, make(map[batchSeenKey]struct{}), false)

	require.NoError(t, err)
	require.ElementsMatch(t, schedulerCanonicalBuckets(10), cache.removedBuckets())
	set, deleted := cache.accountWrites()
	require.Empty(t, set)
	require.Equal(t, []int64{id, id}, deleted)
}

func TestSchedulerAccountEventSkipsRemovalForBucketRebuiltEarlierInBatch(t *testing.T) {
	cache := newIncrementalSnapshotCache()
	repo := newIncrementalAccountRepo(&Account{ID: 95, Platform: PlatformOpenAI, Status: StatusDisabled, GroupIDs: []int64{11}})
	svc := newIncrementalTestService(cache, repo)
	id := int64(95)
	seen := map[batchSeenKey]struct{}{{groupID: 11, platform: PlatformOpenAI}: {}}

	err := svc.handleAccountEvent(context.Background(), &id, nil, seen, false)

	require.NoError(t, err)
	removed := cache.removedBuckets()
	for _, bucket := range removed {
		require.NotEqual(t, PlatformOpenAI, bucket.Platform)
	}
	require.Len(t, removed, len(schedulerCanonicalBuckets(11))-2)
}

func TestSchedulerAccountEventStopsBeforeRebuildOnRemovalError(t *testing.T) {
	cache := newIncrementalSnapshotCache()
	failedBucket := SchedulerBucket{GroupID: 12, Platform: PlatformAnthropic, Mode: SchedulerModeSingle}
	cache.removeError[failedBucket] = errors.New("remove failed")
	repo := newIncrementalAccountRepo(&Account{ID: 96, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, GroupIDs: []int64{12}})
	svc := newIncrementalTestService(cache, repo)
	id := int64(96)

	err := svc.handleAccountEvent(context.Background(), &id, nil, make(map[batchSeenKey]struct{}), false)

	require.EqualError(t, err, "remove failed")
	require.Empty(t, cache.batchSnapshotCache.writes)
}

func TestSchedulerAccountEventFallsBackToFullBucketRebuildWhenIncrementalDisabled(t *testing.T) {
	cache := newIncrementalSnapshotCache()
	repo := newIncrementalAccountRepo(&Account{ID: 97, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, GroupIDs: []int64{13}})
	svc := newIncrementalTestService(cache, repo)
	svc.cfg.Gateway.Scheduling.IncrementalBucketUpdateEnabled = false
	id := int64(97)
	before := GetSchedulerOptimizationMetricsSnapshot()

	err := svc.handleAccountEvent(context.Background(), &id, nil, make(map[batchSeenKey]struct{}), false)

	require.NoError(t, err)
	require.Empty(t, cache.removedBuckets())
	written := make([]SchedulerBucket, 0, len(cache.batchSnapshotCache.writes))
	for bucket := range cache.batchSnapshotCache.writes {
		written = append(written, bucket)
	}
	require.ElementsMatch(t, schedulerCanonicalBuckets(13), written)
	after := GetSchedulerOptimizationMetricsSnapshot()
	require.Equal(t, before.Incremental.FallbackRebuildTotal+1, after.Incremental.FallbackRebuildTotal)
}

func TestDerefAccountsClonesMutableSnapshotFields(t *testing.T) {
	group := &Group{ID: 7}
	accountGroup := AccountGroup{AccountID: 1, GroupID: 7}
	source := &Account{
		ID: 1,
		Credentials: map[string]any{
			"project_id": "cached-project",
			"nested":     map[string]any{"value": "cached-credential"},
			"nested_list": []any{
				map[string]any{"value": "cached-list-item"},
			},
		},
		Extra: map[string]any{
			"plan_type":         "cached-plan",
			"model_rate_limits": map[string]any{"AICredits": "cached-limit"},
		},
		AccountGroups: []AccountGroup{accountGroup},
		GroupIDs:      []int64{7},
		Groups:        []*Group{group},
	}

	result := derefAccounts([]*Account{source})
	require.Len(t, result, 1)
	result[0].Credentials["project_id"] = "request-project"
	result[0].Credentials["nested"].(map[string]any)["value"] = "request-credential"
	result[0].Credentials["nested_list"].([]any)[0].(map[string]any)["value"] = "request-list-item"
	result[0].Extra["plan_type"] = "request-plan"
	result[0].Extra["model_rate_limits"].(map[string]any)["AICredits"] = "request-limit"
	result[0].AccountGroups[0].GroupID = 8
	result[0].GroupIDs[0] = 8
	result[0].Groups[0] = nil

	require.Equal(t, "cached-project", source.Credentials["project_id"])
	require.Equal(t, "cached-credential", source.Credentials["nested"].(map[string]any)["value"])
	require.Equal(t, "cached-list-item", source.Credentials["nested_list"].([]any)[0].(map[string]any)["value"])
	require.Equal(t, "cached-plan", source.Extra["plan_type"])
	require.Equal(t, "cached-limit", source.Extra["model_rate_limits"].(map[string]any)["AICredits"])
	require.Equal(t, int64(7), source.AccountGroups[0].GroupID)
	require.Equal(t, int64(7), source.GroupIDs[0])
	require.Same(t, group, source.Groups[0])
}
