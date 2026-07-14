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

type retirementRaceCache struct {
	SchedulerCache

	mu          sync.Mutex
	epochs      map[string]int64
	retired     map[string]bool
	listBuckets []SchedulerBucket
	captures    []SchedulerBucket
	reopens     []SchedulerBucket
	setAttempts map[string]int
	published   map[string]int
	versions    map[string]int
	beforeSet   func()
REDACTED

func newRetirementRaceCache(buckets ...SchedulerBucket) *retirementRaceCache {
	return &retirementRaceCache{
		epochs:      make(map[string]int64),
		retired:     make(map[string]bool),
		listBuckets: buckets,
		setAttempts: make(map[string]int),
		published:   make(map[string]int),
		versions:    make(map[string]int),
REDACTED
REDACTED

func (c *retirementRaceCache) GetSnapshot(context.Context, SchedulerBucket) ([]*Account, bool, error) {
	return nil, false, nil
REDACTED

func (c *retirementRaceCache) CaptureBucketWriteToken(_ context.Context, bucket SchedulerBucket) (SchedulerBucketWriteToken, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := bucket.String()
	c.captures = append(c.captures, bucket)
	if c.retired[key] {
		return SchedulerBucketWriteToken{REDACTED, ErrSchedulerBucketRetired
REDACTED
	if c.epochs[key] == 0 {
		c.epochs[key] = 1
REDACTED
	return SchedulerBucketWriteToken{Bucket: bucket, Epoch: c.epochs[key]REDACTED, nil
REDACTED

func (c *retirementRaceCache) SetSnapshot(_ context.Context, bucket SchedulerBucket, token SchedulerBucketWriteToken, _ []Account) error {
	if c.beforeSet != nil {
		c.beforeSet()
REDACTED
	c.mu.Lock()
	defer c.mu.Unlock()
	key := bucket.String()
	c.setAttempts[key]++
	if !token.ValidFor(bucket) {
		return ErrSchedulerBucketWriteFenced
REDACTED
	if c.retired[key] {
		return ErrSchedulerBucketRetired
REDACTED
	if c.epochs[key] != token.Epoch {
		return ErrSchedulerBucketWriteFenced
REDACTED
	c.versions[key]++
	c.published[key]++
	return nil
REDACTED

func (c *retirementRaceCache) RetireBucket(_ context.Context, bucket SchedulerBucket) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := bucket.String()
	if !c.retired[key] {
		c.epochs[key]++
		if c.epochs[key] < 1 {
			c.epochs[key] = 1
	REDACTED
		c.retired[key] = true
REDACTED
	return nil
REDACTED

func (c *retirementRaceCache) ReopenBucket(_ context.Context, bucket SchedulerBucket) (SchedulerBucketWriteToken, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := bucket.String()
	if c.epochs[key] == 0 {
		c.epochs[key] = 1
REDACTED
	delete(c.retired, key)
	c.reopens = append(c.reopens, bucket)
	return SchedulerBucketWriteToken{Bucket: bucket, Epoch: c.epochs[key]REDACTED, nil
REDACTED

func (c *retirementRaceCache) TryLockBucket(context.Context, SchedulerBucket, time.Duration) (bool, error) {
	return true, nil
REDACTED

func (c *retirementRaceCache) UnlockBucket(context.Context, SchedulerBucket) error {
	return nil
REDACTED

func (c *retirementRaceCache) ListBuckets(context.Context) ([]SchedulerBucket, error) {
	return append([]SchedulerBucket(nil), c.listBuckets...), nil
REDACTED

func (c *retirementRaceCache) counts(bucket SchedulerBucket) (setAttempts, published int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.setAttempts[bucket.String()], c.published[bucket.String()]
REDACTED

func (c *retirementRaceCache) captureAndReopenCounts() (captures, reopens int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.captures), len(c.reopens)
REDACTED

func (c *retirementRaceCache) version(bucket SchedulerBucket) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.versions[bucket.String()]
REDACTED

type retirementGroupRepo struct {
	GroupRepository
	groups []Group
	err    error
REDACTED

func (r *retirementGroupRepo) ListActive(context.Context) ([]Group, error) {
	return r.groups, r.err
REDACTED

func TestSchedulerFullRebuildCapturesAllRegistryTokensBeforeDBLoad(t *testing.T) {
	first := SchedulerBucket{GroupID: 61, Platform: PlatformOpenAI, Mode: SchedulerModeSingleREDACTED
	queued := SchedulerBucket{GroupID: 61, Platform: PlatformOpenAI, Mode: SchedulerModeForcedREDACTED
	cache := newRetirementRaceCache(first, queued)
	dbStarted := make(chan struct{REDACTED)
	releaseDB := make(chan struct{REDACTED)
	var firstDB sync.Once
	repo := &mockAccountRepoForPlatform{
		listPlatformFunc: func(context.Context, string) ([]Account, error) {
			firstDB.Do(func() {
				close(dbStarted)
				<-releaseDB
		REDACTED)
			return []Account{{ID: 6101, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: trueREDACTEDREDACTED, nil
	REDACTED,
REDACTED
	svc := NewSchedulerSnapshotService(cache, nil, repo, nil, &config.Config{
		RunMode: config.RunModeStandard,
		Gateway: config.GatewayConfig{Scheduling: config.GatewaySchedulingConfig{
			DbFallbackEnabled: true,
REDACTED
REDACTED)

	result := make(chan error, 1)
	go func() { result <- svc.triggerFullRebuild("retirement_race_a") REDACTED()
	select {
	case <-dbStarted:
	case <-time.After(time.Second):
		t.Fatal("first DB load did not start")
REDACTED

	captures, reopens := cache.captureAndReopenCounts()
	require.Equal(t, 2, captures, "all registry tokens must be captured before the first DB load")
	require.Zero(t, reopens)
	require.NoError(t, cache.RetireBucket(context.Background(), queued))
	_, err := cache.ReopenBucket(context.Background(), queued)
REDACTED
	close(releaseDB)
	require.NoError(t, <-result)

	_, firstPublished := cache.counts(first)
	queuedAttempts, queuedPublished := cache.counts(queued)
	require.Equal(t, 1, firstPublished)
	require.Equal(t, 1, queuedAttempts)
	require.Zero(t, queuedPublished, "queued registry task must not adopt the reopened epoch")
REDACTED

func TestSchedulerRebuildRetireAfterDBLoadFencesPublish(t *testing.T) {
	bucket := SchedulerBucket{GroupID: 62, Platform: PlatformOpenAI, Mode: SchedulerModeSingleREDACTED
	cache := newRetirementRaceCache()
	dbReturned := make(chan struct{REDACTED)
	setEntered := make(chan struct{REDACTED)
	releaseSet := make(chan struct{REDACTED)
	cache.beforeSet = func() {
		close(setEntered)
		<-releaseSet
REDACTED
	repo := &mockAccountRepoForPlatform{
		listPlatformFunc: func(context.Context, string) ([]Account, error) {
			close(dbReturned)
			return []Account{{ID: 6201, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: trueREDACTEDREDACTED, nil
	REDACTED,
REDACTED
	svc := NewSchedulerSnapshotService(cache, nil, repo, nil, &config.Config{
		RunMode: config.RunModeStandard,
		Gateway: config.GatewayConfig{Scheduling: config.GatewaySchedulingConfig{
			DbFallbackEnabled: true,
REDACTED
REDACTED)

	result := make(chan error, 1)
	go func() {
		result <- svc.rebuildBuckets(context.Background(), []SchedulerBucket{bucketREDACTED, "retirement_race_b")
REDACTED()
	select {
	case <-dbReturned:
	case <-time.After(time.Second):
		t.Fatal("DB load did not return")
REDACTED
	select {
	case <-setEntered:
	case <-time.After(time.Second):
		t.Fatal("snapshot writer did not reach allocation boundary")
REDACTED
	require.NoError(t, cache.RetireBucket(context.Background(), bucket))
	close(releaseSet)
	require.NoError(t, <-result)

	setAttempts, published := cache.counts(bucket)
	require.Equal(t, 1, setAttempts)
	require.Zero(t, published)
	require.Zero(t, cache.version(bucket), "retirement before allocation must not advance the snapshot version")
REDACTED

func TestSchedulerFallbackReturnsDBAccountsWhenBucketRetired(t *testing.T) {
	bucket := SchedulerBucket{GroupID: 63, Platform: PlatformOpenAI, Mode: SchedulerModeSingleREDACTED
	cache := newRetirementRaceCache()
	require.NoError(t, cache.RetireBucket(context.Background(), bucket))
	repo := &mockAccountRepoForPlatform{
		accounts: []Account{{ID: 6301, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: trueREDACTEDREDACTED,
REDACTED
	svc := NewSchedulerSnapshotService(cache, nil, repo, nil, &config.Config{
		RunMode: config.RunModeStandard,
		Gateway: config.GatewayConfig{Scheduling: config.GatewaySchedulingConfig{
			DbFallbackEnabled: true,
REDACTED
REDACTED)
	groupID := bucket.GroupID

	accounts, useMixed, err := svc.ListSchedulableAccounts(context.Background(), &groupID, bucket.Platform, false)
REDACTED
	require.False(t, useMixed)
	require.Len(t, accounts, 1)
	setAttempts, published := cache.counts(bucket)
	require.Zero(t, setAttempts)
	require.Zero(t, published)
REDACTED

func TestSchedulerDefaultBucketsUseCaptureAndListActiveFailureKeepsGroupZero(t *testing.T) {
	cache := newRetirementRaceCache()
	svc := NewSchedulerSnapshotService(
		cache,
		nil,
		nil,
		&retirementGroupRepo{err: errors.New("list active failed")REDACTED,
		testConfig(),
	)

	buckets, err := svc.defaultBuckets(context.Background())
REDACTED
	require.NotEmpty(t, buckets)
	for _, bucket := range buckets {
		require.Zero(t, bucket.GroupID)
REDACTED

	tasks, err := svc.prepareBucketWriteTasks(context.Background(), buckets)
REDACTED
	require.Len(t, tasks, len(buckets))
	captures, reopens := cache.captureAndReopenCounts()
	require.Equal(t, len(buckets), captures)
	require.Zero(t, reopens)
REDACTED
