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
REDACTED

func newBulkEventAccountRepo(accounts ...*Account) *bulkEventAccountRepo {
	return &bulkEventAccountRepo{
		batchAccountQueryRepo: newBatchAccountQueryRepo(),
		accounts:              accounts,
REDACTED
REDACTED

func (r *bulkEventAccountRepo) GetByIDs(context.Context, []int64) ([]*Account, error) {
	return append([]*Account(nil), r.accounts...), nil
REDACTED

type bulkEventSnapshotCache struct {
	*batchSnapshotCache

	accountMu        sync.Mutex
	setAccountIDs    []int64
	deleteAccountIDs []int64
REDACTED

func newBulkEventSnapshotCache() *bulkEventSnapshotCache {
	return &bulkEventSnapshotCache{batchSnapshotCache: newBatchSnapshotCache()REDACTED
REDACTED

func (c *bulkEventSnapshotCache) SetAccount(_ context.Context, account *Account) error {
	c.accountMu.Lock()
	defer c.accountMu.Unlock()
	c.setAccountIDs = append(c.setAccountIDs, account.ID)
	return nil
REDACTED

func (c *bulkEventSnapshotCache) DeleteAccount(_ context.Context, accountID int64) error {
	c.accountMu.Lock()
	defer c.accountMu.Unlock()
	c.deleteAccountIDs = append(c.deleteAccountIDs, accountID)
	return nil
REDACTED

func (c *bulkEventSnapshotCache) accountWrites() (set []int64, deleted []int64) {
	c.accountMu.Lock()
	defer c.accountMu.Unlock()
	return append([]int64(nil), c.setAccountIDs...), append([]int64(nil), c.deleteAccountIDs...)
REDACTED

func (c *bulkEventSnapshotCache) capturedBuckets() []SchedulerBucket {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]SchedulerBucket(nil), c.captures...)
REDACTED

func newBulkEventTestService(cache SchedulerCache, accounts AccountRepository) *SchedulerSnapshotService {
	return NewSchedulerSnapshotService(cache, nil, accounts, nil, &config.Config{RunMode: config.RunModeStandardREDACTED)
REDACTED

func bulkEventPayload(accountIDs []int64, groupIDs []int64) map[string]any {
	accountValues := make([]any, 0, len(accountIDs))
	for _, id := range accountIDs {
		accountValues = append(accountValues, id)
REDACTED
	groupValues := make([]any, 0, len(groupIDs))
	for _, id := range groupIDs {
		groupValues = append(groupValues, id)
REDACTED
	return map[string]any{
		"account_ids": accountValues,
		"group_ids":   groupValues,
REDACTED
REDACTED

func schedulerBucketsForTest(groupIDs []int64, platforms ...string) []SchedulerBucket {
	buckets := make([]SchedulerBucket, 0, len(groupIDs)*len(platforms)*3)
	for _, platform := range platforms {
		for _, groupID := range groupIDs {
			buckets = append(buckets,
				SchedulerBucket{GroupID: groupID, Platform: platform, Mode: SchedulerModeSingleREDACTED,
				SchedulerBucket{GroupID: groupID, Platform: platform, Mode: SchedulerModeForcedREDACTED,
			)
			if platform == PlatformAnthropic || platform == PlatformGemini {
				buckets = append(buckets, SchedulerBucket{GroupID: groupID, Platform: platform, Mode: SchedulerModeMixedREDACTED)
		REDACTED
	REDACTED
REDACTED
	return buckets
REDACTED

func TestSchedulerBulkAccountEventScopesOpenAIRebuildToFreshPlatform(t *testing.T) {
	cache := newBulkEventSnapshotCache()
	repo := newBulkEventAccountRepo(&Account{ID: 1, Platform: PlatformOpenAI, GroupIDs: []int64{12REDACTEDREDACTED)
	svc := newBulkEventTestService(cache, repo)

	err := svc.handleBulkAccountEvent(context.Background(), bulkEventPayload([]int64{1REDACTED, []int64{11REDACTED), make(map[batchSeenKey]struct{REDACTED))

REDACTED
	require.ElementsMatch(t, schedulerBucketsForTest([]int64{11, 12REDACTED, PlatformOpenAI), cache.capturedBuckets())
	set, deleted := cache.accountWrites()
	require.Equal(t, []int64{1REDACTED, set)
	require.Empty(t, deleted)
REDACTED

func TestSchedulerBulkAccountEventRebuildsOpenAIUngroupedBucket(t *testing.T) {
	cache := newBulkEventSnapshotCache()
	repo := newBulkEventAccountRepo(&Account{ID: 6, Platform: PlatformOpenAIREDACTED)
	svc := newBulkEventTestService(cache, repo)

	err := svc.handleBulkAccountEvent(context.Background(), bulkEventPayload([]int64{6REDACTED, nil), make(map[batchSeenKey]struct{REDACTED))

REDACTED
	require.ElementsMatch(t, schedulerBucketsForTest([]int64{0REDACTED, PlatformOpenAI), cache.capturedBuckets())
REDACTED

func TestSchedulerBulkAccountEventKeepsGroupedAndUngroupedBuckets(t *testing.T) {
	cache := newBulkEventSnapshotCache()
	repo := newBulkEventAccountRepo(
		&Account{ID: 7, Platform: PlatformOpenAI, GroupIDs: []int64{51REDACTEDREDACTED,
		&Account{ID: 8, Platform: PlatformOpenAIREDACTED,
	)
	svc := newBulkEventTestService(cache, repo)

	err := svc.handleBulkAccountEvent(context.Background(), bulkEventPayload([]int64{7, 8REDACTED, nil), make(map[batchSeenKey]struct{REDACTED))

REDACTED
	require.ElementsMatch(t, schedulerBucketsForTest([]int64{0, 51REDACTED, PlatformOpenAI), cache.capturedBuckets())
REDACTED

func TestSchedulerBulkAccountEventDoesNotCrossCurrentGroupsBetweenPlatforms(t *testing.T) {
	cache := newBulkEventSnapshotCache()
	repo := newBulkEventAccountRepo(
		&Account{ID: 9, Platform: PlatformOpenAI, GroupIDs: []int64{61REDACTEDREDACTED,
		&Account{ID: 10, Platform: PlatformGrok, GroupIDs: []int64{62REDACTEDREDACTED,
	)
	svc := newBulkEventTestService(cache, repo)

	err := svc.handleBulkAccountEvent(context.Background(), bulkEventPayload([]int64{9, 10REDACTED, []int64{63REDACTED), make(map[batchSeenKey]struct{REDACTED))

REDACTED
	want := append(
		schedulerBucketsForTest([]int64{61, 63REDACTED, PlatformOpenAI),
		schedulerBucketsForTest([]int64{62, 63REDACTED, PlatformGrok)...,
	)
	require.ElementsMatch(t, want, cache.capturedBuckets())
REDACTED

func TestSchedulerBulkAccountEventUsesGroupZeroInSimpleMode(t *testing.T) {
	cache := newBulkEventSnapshotCache()
	repo := newBulkEventAccountRepo(&Account{ID: 11, Platform: PlatformOpenAI, GroupIDs: []int64{71REDACTEDREDACTED)
	svc := NewSchedulerSnapshotService(cache, nil, repo, nil, &config.Config{RunMode: config.RunModeSimpleREDACTED)

	err := svc.handleBulkAccountEvent(context.Background(), bulkEventPayload([]int64{11REDACTED, []int64{72REDACTED), make(map[batchSeenKey]struct{REDACTED))

REDACTED
	require.ElementsMatch(t, schedulerBucketsForTest([]int64{0REDACTED, PlatformOpenAI), cache.capturedBuckets())
REDACTED

func TestSchedulerBulkAccountEventConservativelyExpandsAntigravityPlatforms(t *testing.T) {
	cache := newBulkEventSnapshotCache()
	// fresh 值可能已经关闭 mixed_scheduling，兼容平台仍要重建以清理旧快照。
	repo := newBulkEventAccountRepo(&Account{ID: 2, Platform: PlatformAntigravity, GroupIDs: []int64{22REDACTEDREDACTED)
	svc := newBulkEventTestService(cache, repo)

	err := svc.handleBulkAccountEvent(context.Background(), bulkEventPayload([]int64{2REDACTED, []int64{21REDACTED), make(map[batchSeenKey]struct{REDACTED))

REDACTED
	require.ElementsMatch(t,
		schedulerBucketsForTest([]int64{21, 22REDACTED, PlatformAnthropic, PlatformGemini, PlatformAntigravity),
		cache.capturedBuckets(),
	)
REDACTED

func TestSchedulerBulkAccountEventMissingAccountFallsBackToAllPlatforms(t *testing.T) {
	cache := newBulkEventSnapshotCache()
	repo := newBulkEventAccountRepo(&Account{ID: 3, Platform: PlatformOpenAI, GroupIDs: []int64{32REDACTEDREDACTED)
	svc := newBulkEventTestService(cache, repo)

	err := svc.handleBulkAccountEvent(context.Background(), bulkEventPayload([]int64{3, 4REDACTED, []int64{31REDACTED), make(map[batchSeenKey]struct{REDACTED))

REDACTED
	platforms := schedulerSnapshotPlatforms()
	require.ElementsMatch(t, schedulerBucketsForTest([]int64{31, 32REDACTED, platforms[:]...), cache.capturedBuckets())
	set, deleted := cache.accountWrites()
	require.Equal(t, []int64{3REDACTED, set)
	require.Equal(t, []int64{4REDACTED, deleted)
REDACTED

func TestSchedulerBulkAccountEventUnknownPlatformFallsBackToAllPlatforms(t *testing.T) {
	cache := newBulkEventSnapshotCache()
	repo := newBulkEventAccountRepo(&Account{ID: 5, GroupIDs: []int64{42REDACTEDREDACTED)
	svc := newBulkEventTestService(cache, repo)

	err := svc.handleBulkAccountEvent(context.Background(), bulkEventPayload([]int64{5REDACTED, []int64{41REDACTED), make(map[batchSeenKey]struct{REDACTED))

REDACTED
	platforms := schedulerSnapshotPlatforms()
	require.ElementsMatch(t, schedulerBucketsForTest([]int64{41, 42REDACTED, platforms[:]...), cache.capturedBuckets())
REDACTED
