//go:build unit

package service

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestBatchImageWorkerRuntime_QueueDisabledDoesNotStart(t *testing.T) {
	queue := &blockingBatchImageRuntimeQueue{REDACTED
	runtime := NewBatchImageWorkerRuntime(
		NewBatchImageWorker(queue, &fakeBatchImageProcessor{REDACTED, BatchImageWorkerOptions{REDACTED),
		&config.Config{BatchImage: config.BatchImageConfig{QueueEnabled: falseREDACTEDREDACTED,
	)

	runtime.Start()

	require.False(t, runtime.Running())
	require.Zero(t, queue.reserveCalls.Load())
	require.NotPanics(t, runtime.Stop)
REDACTED

func TestBatchImageWorkerRuntime_QueueEnabledStartsAndStops(t *testing.T) {
	queue := &blockingBatchImageRuntimeQueue{REDACTED
	processor := &fakeBatchImageProcessor{REDACTED
	runtime := NewBatchImageWorkerRuntime(
		NewBatchImageWorker(queue, processor, BatchImageWorkerOptions{
			DelayedPollInterval: time.Hour,
			RecoveryInterval:    time.Hour,
	REDACTED),
		&config.Config{BatchImage: config.BatchImageConfig{QueueEnabled: trueREDACTEDREDACTED,
	)

	runtime.Start()

	require.Eventually(t, func() bool {
		return runtime.Running() && queue.reserveCalls.Load() > 0
REDACTED, time.Second, 10*time.Millisecond)
	require.Empty(t, processor.processed)
	require.NotPanics(t, runtime.Stop)
	require.False(t, runtime.Running())
	require.NotPanics(t, runtime.Stop)
REDACTED

type blockingBatchImageRuntimeQueue struct {
	reserveCalls atomic.Int64
REDACTED

func (q *blockingBatchImageRuntimeQueue) Enqueue(context.Context, string) error {
	return nil
REDACTED

func (q *blockingBatchImageRuntimeQueue) Reserve(ctx context.Context, _ time.Duration) (ReservedBatchImageJob, error) {
	q.reserveCalls.Add(1)
	<-ctx.Done()
	return ReservedBatchImageJob{REDACTED, ctx.Err()
REDACTED

func (q *blockingBatchImageRuntimeQueue) RequeueAfter(context.Context, string, time.Duration) error {
	return nil
REDACTED

func (q *blockingBatchImageRuntimeQueue) Ack(context.Context, string) error {
	return nil
REDACTED

func (q *blockingBatchImageRuntimeQueue) Heartbeat(context.Context, string) error {
	return nil
REDACTED

func (q *blockingBatchImageRuntimeQueue) MoveDueDelayedToReady(context.Context, int) (int, error) {
	return 0, nil
REDACTED

func (q *blockingBatchImageRuntimeQueue) RecoverStaleActive(context.Context, time.Duration, int) (int, error) {
	return 0, nil
REDACTED

func (q *blockingBatchImageRuntimeQueue) TryAcquireJobLock(context.Context, string, time.Duration) (BatchImageJobLock, bool, error) {
	return nil, false, nil
REDACTED
