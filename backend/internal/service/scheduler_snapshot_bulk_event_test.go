//go:build unit

package service

import (
	"context"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type bulkEventAccountRepo struct {
	*batchAccountQueryRepo
	accounts []*Account
}

func newBulkEventAccountRepo(accounts ...*Account) *bulkEventAccountRepo {
	return &bulkEventAccountRepo{
		batchAccountQueryRepo: newBatchAccountQueryRepo(),
		accounts:              accounts,
	}
}

func (r *bulkEventAccountRepo) GetByIDs(context.Context, []int64) ([]*Account, error) {
	return append([]*Account(nil), r.accounts...), nil
}

type bulkEventSnapshotCache struct {
	*batchSnapshotCache

	accountMu        sync.Mutex
	setAccountIDs    []int64
	deleteAccountIDs []int64
}

func newBulkEventSnapshotCache() *bulkEventSnapshotCache {
	return &bulkEventSnapshotCache{batchSnapshotCache: newBatchSnapshotCache()}
}

func (c *bulkEventSnapshotCache) SetAccount(_ context.Context, account *Account) error {
	c.accountMu.Lock()
	defer c.accountMu.Unlock()
	c.setAccountIDs = append(c.setAccountIDs, account.ID)
	return nil
}

func (c *bulkEventSnapshotCache) DeleteAccount(_ context.Context, accountID int64) error {
	c.accountMu.Lock()
	defer c.accountMu.Unlock()
	c.deleteAccountIDs = append(c.deleteAccountIDs, accountID)
	return nil
}

func (c *bulkEventSnapshotCache) accountWrites() (set []int64, deleted []int64) {
	c.accountMu.Lock()
	defer c.accountMu.Unlock()
	return append([]int64(nil), c.setAccountIDs...), append([]int64(nil), c.deleteAccountIDs...)
}

func (c *bulkEventSnapshotCache) capturedBuckets() []SchedulerBucket {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]SchedulerBucket(nil), c.captures...)
}

func newBulkEventTestService(cache SchedulerCache, accounts AccountRepository) *SchedulerSnapshotService {
	return NewSchedulerSnapshotService(cache, nil, accounts, nil, &config.Config{RunMode: config.RunModeStandard})
}

func bulkEventPayload(accountIDs []int64, groupIDs []int64) map[string]any {
	accountValues := make([]any, 0, len(accountIDs))
	for _, id := range accountIDs {
		accountValues = append(accountValues, id)
	}
	groupValues := make([]any, 0, len(groupIDs))
	for _, id := range groupIDs {
		groupValues = append(groupValues, id)
	}
	return map[string]any{
		"account_ids": accountValues,
		"group_ids":   groupValues,
	}
}

func schedulerBucketsForTest(groupIDs []int64, platforms ...string) []SchedulerBucket {
	buckets := make([]SchedulerBucket, 0, len(groupIDs)*len(platforms)*3)
	for _, platform := range platforms {
		for _, groupID := range groupIDs {
			buckets = append(buckets,
				SchedulerBucket{GroupID: groupID, Platform: platform, Mode: SchedulerModeSingle},
				SchedulerBucket{GroupID: groupID, Platform: platform, Mode: SchedulerModeForced},
			)
			if platform == PlatformAnthropic || platform == PlatformGemini {
				buckets = append(buckets, SchedulerBucket{GroupID: groupID, Platform: platform, Mode: SchedulerModeMixed})
			}
		}
	}
	return buckets
}

func TestSchedulerBulkAccountEventScopesOpenAIRebuildToFreshPlatform(t *testing.T) {
	cache := newBulkEventSnapshotCache()
	repo := newBulkEventAccountRepo(&Account{ID: 1, Platform: PlatformOpenAI, GroupIDs: []int64{12}})
	svc := newBulkEventTestService(cache, repo)

	err := svc.handleBulkAccountEvent(context.Background(), bulkEventPayload([]int64{1}, []int64{11}), make(map[batchSeenKey]struct{}))

	require.NoError(t, err)
	require.ElementsMatch(t, schedulerBucketsForTest([]int64{11, 12}, PlatformOpenAI), cache.capturedBuckets())
	set, deleted := cache.accountWrites()
	require.Equal(t, []int64{1}, set)
	require.Empty(t, deleted)
}

func TestSchedulerBulkAccountEventRebuildsOpenAIUngroupedBucket(t *testing.T) {
	cache := newBulkEventSnapshotCache()
	repo := newBulkEventAccountRepo(&Account{ID: 6, Platform: PlatformOpenAI})
	svc := newBulkEventTestService(cache, repo)

	err := svc.handleBulkAccountEvent(context.Background(), bulkEventPayload([]int64{6}, nil), make(map[batchSeenKey]struct{}))

	require.NoError(t, err)
	require.ElementsMatch(t, schedulerBucketsForTest([]int64{0}, PlatformOpenAI), cache.capturedBuckets())
}

func TestSchedulerBulkAccountEventKeepsGroupedAndUngroupedBuckets(t *testing.T) {
	cache := newBulkEventSnapshotCache()
	repo := newBulkEventAccountRepo(
		&Account{ID: 7, Platform: PlatformOpenAI, GroupIDs: []int64{51}},
		&Account{ID: 8, Platform: PlatformOpenAI},
	)
	svc := newBulkEventTestService(cache, repo)

	err := svc.handleBulkAccountEvent(context.Background(), bulkEventPayload([]int64{7, 8}, nil), make(map[batchSeenKey]struct{}))

	require.NoError(t, err)
	require.ElementsMatch(t, schedulerBucketsForTest([]int64{0, 51}, PlatformOpenAI), cache.capturedBuckets())
}

func TestSchedulerBulkAccountEventDoesNotCrossCurrentGroupsBetweenPlatforms(t *testing.T) {
	cache := newBulkEventSnapshotCache()
	repo := newBulkEventAccountRepo(
		&Account{ID: 9, Platform: PlatformOpenAI, GroupIDs: []int64{61}},
		&Account{ID: 10, Platform: PlatformGrok, GroupIDs: []int64{62}},
	)
	svc := newBulkEventTestService(cache, repo)

	err := svc.handleBulkAccountEvent(context.Background(), bulkEventPayload([]int64{9, 10}, []int64{63}), make(map[batchSeenKey]struct{}))

	require.NoError(t, err)
	want := append(
		schedulerBucketsForTest([]int64{61, 63}, PlatformOpenAI),
		schedulerBucketsForTest([]int64{62, 63}, PlatformGrok)...,
	)
	require.ElementsMatch(t, want, cache.capturedBuckets())
}

func TestSchedulerBulkAccountEventUsesGroupZeroInSimpleMode(t *testing.T) {
	cache := newBulkEventSnapshotCache()
	repo := newBulkEventAccountRepo(&Account{ID: 11, Platform: PlatformOpenAI, GroupIDs: []int64{71}})
	svc := NewSchedulerSnapshotService(cache, nil, repo, nil, &config.Config{RunMode: config.RunModeSimple})

	err := svc.handleBulkAccountEvent(context.Background(), bulkEventPayload([]int64{11}, []int64{72}), make(map[batchSeenKey]struct{}))

	require.NoError(t, err)
	require.ElementsMatch(t, schedulerBucketsForTest([]int64{0}, PlatformOpenAI), cache.capturedBuckets())
}

func TestSchedulerBulkAccountEventConservativelyExpandsAntigravityPlatforms(t *testing.T) {
	cache := newBulkEventSnapshotCache()
	// fresh 值可能已经关闭 mixed_scheduling，兼容平台仍要重建以清理旧快照。
	repo := newBulkEventAccountRepo(&Account{ID: 2, Platform: PlatformAntigravity, GroupIDs: []int64{22}})
	svc := newBulkEventTestService(cache, repo)

	err := svc.handleBulkAccountEvent(context.Background(), bulkEventPayload([]int64{2}, []int64{21}), make(map[batchSeenKey]struct{}))

	require.NoError(t, err)
	require.ElementsMatch(t,
		schedulerBucketsForTest([]int64{21, 22}, PlatformAnthropic, PlatformGemini, PlatformAntigravity),
		cache.capturedBuckets(),
	)
}

func TestSchedulerBulkAccountEventMissingAccountFallsBackToAllPlatforms(t *testing.T) {
	cache := newBulkEventSnapshotCache()
	repo := newBulkEventAccountRepo(&Account{ID: 3, Platform: PlatformOpenAI, GroupIDs: []int64{32}})
	svc := newBulkEventTestService(cache, repo)

	err := svc.handleBulkAccountEvent(context.Background(), bulkEventPayload([]int64{3, 4}, []int64{31}), make(map[batchSeenKey]struct{}))

	require.NoError(t, err)
	platforms := schedulerSnapshotPlatforms()
	require.ElementsMatch(t, schedulerBucketsForTest([]int64{31, 32}, platforms[:]...), cache.capturedBuckets())
	set, deleted := cache.accountWrites()
	require.Equal(t, []int64{3}, set)
	require.Equal(t, []int64{4}, deleted)
}

func TestSchedulerBulkAccountEventUnknownPlatformFallsBackToAllPlatforms(t *testing.T) {
	cache := newBulkEventSnapshotCache()
	repo := newBulkEventAccountRepo(&Account{ID: 5, GroupIDs: []int64{42}})
	svc := newBulkEventTestService(cache, repo)

	err := svc.handleBulkAccountEvent(context.Background(), bulkEventPayload([]int64{5}, []int64{41}), make(map[batchSeenKey]struct{}))

	require.NoError(t, err)
	platforms := schedulerSnapshotPlatforms()
	require.ElementsMatch(t, schedulerBucketsForTest([]int64{41, 42}, platforms[:]...), cache.capturedBuckets())
}
