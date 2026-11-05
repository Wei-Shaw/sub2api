package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func newTrafficPersistenceTestPool(t *testing.T, policy string, queueSize int) *UsageRecordWorkerPool {
	t.Helper()
	pool := NewUsageRecordWorkerPoolWithOptions(UsageRecordWorkerPoolOptions{
		WorkerCount: 1, QueueSize: queueSize, TaskTimeout: time.Second,
		OverflowPolicy: policy, OverflowSamplePercent: 0, AutoScaleEnabled: false,
	})
	t.Cleanup(pool.Stop)
	return pool
}

func TestUsageRecordTraffic_MandatoryPoolNeverDrops(t *testing.T) {
	for _, policy := range []string{"drop", "sample", "sync"} {
		t.Run(policy, func(t *testing.T) {
			pool := newTrafficPersistenceTestPool(t, policy, 1)
			started, release := make(chan struct{}), make(chan struct{})
			defer close(release)
			require.Equal(t, UsageRecordSubmitModeEnqueued, pool.Submit(func(context.Context) { close(started); <-release }))
			<-started
			require.Equal(t, UsageRecordSubmitModeEnqueued, pool.Submit(func(context.Context) {}))
			called := 0
			require.Equal(t, UsageRecordSubmitModeSync, pool.SubmitMandatory(func(ctx context.Context) {
				called++
				require.NoError(t, ctx.Err())
				_, hasDeadline := ctx.Deadline()
				require.True(t, hasDeadline)
			}))
			require.Equal(t, 1, called)
			require.EqualValues(t, 1, pool.Stats().SyncFallbackTasks)
			require.Zero(t, pool.Stats().DroppedQueueFull, "mandatory persistence must not use the configured drop policy")
		})
	}
}

func TestUsageRecordTraffic_MandatoryNilStoppedAndPanic(t *testing.T) {
	stopped := newTrafficPersistenceTestPool(t, "drop", 1)
	stopped.Stop()
	for _, pool := range []*UsageRecordWorkerPool{nil, stopped} {
		called := false
		require.NotPanics(t, func() {
			require.Equal(t, UsageRecordSubmitModeSync, pool.SubmitMandatory(func(context.Context) { panic("writer panic") }))
		})
		require.Equal(t, UsageRecordSubmitModeSync, pool.SubmitMandatory(func(context.Context) { called = true }))
		require.True(t, called)
	}
}
