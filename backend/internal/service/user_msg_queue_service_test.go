//go:build unit

package service

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type cleanupWorkerUserMsgQueueCache struct {
	reconcileCalls atomic.Int64
	maxCount       atomic.Int64
REDACTED

var _ UserMsgQueueCache = (*cleanupWorkerUserMsgQueueCache)(nil)

func (c *cleanupWorkerUserMsgQueueCache) AcquireLock(context.Context, int64, string, int) (bool, error) {
	return true, nil
REDACTED

func (c *cleanupWorkerUserMsgQueueCache) ReleaseLock(context.Context, int64, string) (bool, error) {
	return true, nil
REDACTED

func (c *cleanupWorkerUserMsgQueueCache) GetLastCompletedMs(context.Context, int64) (int64, error) {
	return 0, nil
REDACTED

func (c *cleanupWorkerUserMsgQueueCache) GetCurrentTimeMs(context.Context) (int64, error) {
	return time.Now().UnixMilli(), nil
REDACTED

func (c *cleanupWorkerUserMsgQueueCache) ReconcileExpiredLockCandidates(_ context.Context, maxCount int) (int, error) {
	c.reconcileCalls.Add(1)
	c.maxCount.Store(int64(maxCount))
	return 1, nil
REDACTED

func TestStartCleanupWorker_ReconcilesExpiredLockCandidates(t *testing.T) {
	cache := &cleanupWorkerUserMsgQueueCache{REDACTED
	svc := NewUserMessageQueueService(cache, nil, nil)
	defer svc.Stop()

	svc.StartCleanupWorker(time.Millisecond)

	require.Eventually(t, func() bool {
		return cache.reconcileCalls.Load() > 0
REDACTED, time.Second, 10*time.Millisecond)
	require.EqualValues(t, 1000, cache.maxCount.Load())
REDACTED
