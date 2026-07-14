package service

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type schedulerFullRebuildTestCache struct {
	SchedulerCache

	mu        sync.Mutex
	listErr   error
	listCalls int
	captures  int
	lockCalls int
REDACTED

func (c *schedulerFullRebuildTestCache) ListBuckets(context.Context) ([]SchedulerBucket, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.listCalls++
	return nil, c.listErr
REDACTED

func (c *schedulerFullRebuildTestCache) TryLockBucket(context.Context, SchedulerBucket, time.Duration) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lockCalls++
	return false, nil
REDACTED

func (c *schedulerFullRebuildTestCache) CaptureBucketWriteToken(_ context.Context, bucket SchedulerBucket) (SchedulerBucketWriteToken, error) {
	c.mu.Lock()
	c.captures++
	c.mu.Unlock()
	return SchedulerBucketWriteToken{Bucket: bucket, Epoch: 1REDACTED, nil
REDACTED

func (c *schedulerFullRebuildTestCache) ReopenBucket(_ context.Context, bucket SchedulerBucket) (SchedulerBucketWriteToken, error) {
	return SchedulerBucketWriteToken{Bucket: bucket, Epoch: 1REDACTED, nil
REDACTED

func TestSchedulerSnapshotServiceFullRebuildCoalescesConcurrentRequestsIntoTrailingRun(t *testing.T) {
	svc := &SchedulerSnapshotService{REDACTED
	wantTrailingErr := errors.New("trailing rebuild failed")
	firstStarted := make(chan struct{REDACTED)
	releaseFirst := make(chan struct{REDACTED)
	var releaseOnce sync.Once
	release := func() {
		releaseOnce.Do(func() { close(releaseFirst) REDACTED)
REDACTED
	defer release()

	var calls atomic.Int32
	var active atomic.Int32
	var maxActive atomic.Int32
	run := func() error {
		call := calls.Add(1)
		currentActive := active.Add(1)
		defer active.Add(-1)
		for {
			previousMax := maxActive.Load()
			if currentActive <= previousMax || maxActive.CompareAndSwap(previousMax, currentActive) {
				break
		REDACTED
	REDACTED
		if call == 1 {
			close(firstStarted)
			<-releaseFirst
			return nil
	REDACTED
		return wantTrailingErr
REDACTED

	firstResult := make(chan error, 1)
	go func() {
		firstResult <- svc.coalesceFullRebuild(run)
REDACTED()

	select {
	case <-firstStarted:
	case <-time.After(time.Second):
		t.Fatal("first rebuild did not start")
REDACTED

	const followers = 20
	followerResults := make(chan error, followers)
	for range followers {
		go func() {
			followerResults <- svc.coalesceFullRebuild(run)
	REDACTED()
REDACTED

	require.Eventually(t, func() bool {
		requested, _ := schedulerFullRebuildState(svc)
		return requested == followers+1
REDACTED, time.Second, time.Millisecond)
	release()

	require.NoError(t, <-firstResult)
	for range followers {
		require.ErrorIs(t, <-followerResults, wantTrailingErr)
REDACTED
	require.EqualValues(t, 2, calls.Load())
	require.EqualValues(t, 1, maxActive.Load())
	requested, completed := schedulerFullRebuildState(svc)
	require.EqualValues(t, followers+1, requested)
	require.Equal(t, requested, completed)
REDACTED

func TestSchedulerSnapshotServiceFullRebuildRunsAgainForSequentialRequest(t *testing.T) {
	svc := &SchedulerSnapshotService{REDACTED
	wantSecondErr := errors.New("second rebuild failed")
	var calls atomic.Int32
	run := func() error {
		if calls.Add(1) == 2 {
			return wantSecondErr
	REDACTED
		return nil
REDACTED

	require.NoError(t, svc.coalesceFullRebuild(run))
	require.ErrorIs(t, svc.coalesceFullRebuild(run), wantSecondErr)
	require.EqualValues(t, 2, calls.Load())
	requested, completed := schedulerFullRebuildState(svc)
	require.EqualValues(t, 2, requested)
	require.Equal(t, requested, completed)
REDACTED

func TestSchedulerSnapshotServiceInitialFullRebuildFailsClosedWhenListBucketsFails(t *testing.T) {
	cache := &schedulerFullRebuildTestCache{listErr: errors.New("list buckets failed")REDACTED
	svc := NewSchedulerSnapshotService(cache, nil, nil, nil, nil)

	svc.runInitialRebuild()

	cache.mu.Lock()
	listCalls := cache.listCalls
	captures := cache.captures
	lockCalls := cache.lockCalls
	cache.mu.Unlock()
	require.Equal(t, 1, listCalls)
	require.Zero(t, captures)
	require.Zero(t, lockCalls)
	requested, completed := schedulerFullRebuildState(svc)
	require.EqualValues(t, 1, requested)
	require.Equal(t, requested, completed)
	svc.fullRebuildStateMu.Lock()
	require.ErrorIs(t, svc.fullRebuildLastErr, cache.listErr)
	svc.fullRebuildStateMu.Unlock()
REDACTED

func schedulerFullRebuildState(svc *SchedulerSnapshotService) (requested uint64, completed uint64) {
	svc.fullRebuildStateMu.Lock()
	defer svc.fullRebuildStateMu.Unlock()
	return svc.fullRebuildRequested, svc.fullRebuildCompleted
REDACTED
