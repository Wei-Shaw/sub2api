//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBatchImageWorker_ProcessesJobOnce(t *testing.T) {
	queue := newFakeBatchImageQueue("imgbatch_worker_once")
	processor := &fakeBatchImageProcessor{REDACTED
	worker := NewBatchImageWorker(queue, processor, BatchImageWorkerOptions{ReserveBlockTimeout: time.MillisecondREDACTED)

	require.NoError(t, worker.RunOnce(context.Background()))
	require.Equal(t, []string{"imgbatch_worker_once"REDACTED, processor.processed)
	require.Len(t, queue.requeued, 1)
	require.Equal(t, defaultBatchImageWorkerRequeueDelay, queue.requeued[0].delay)
	require.Equal(t, 1, queue.releaseCount)
REDACTED

func TestBatchImageWorker_RequeuesNonTerminalResultWithRequestedDelay(t *testing.T) {
	queue := newFakeBatchImageQueue("imgbatch_worker_requeue")
	processor := &fakeBatchImageProcessor{result: BatchImageProcessResult{RequeueAfter: 42 * time.SecondREDACTEDREDACTED
	worker := NewBatchImageWorker(queue, processor, BatchImageWorkerOptions{REDACTED)

	require.NoError(t, worker.RunOnce(context.Background()))
	require.Len(t, queue.requeued, 1)
	require.Equal(t, "imgbatch_worker_requeue", queue.requeued[0].batchID)
	require.Equal(t, 42*time.Second, queue.requeued[0].delay)
	require.Empty(t, queue.acked)
REDACTED

func TestBatchImageWorker_AcksTerminalResult(t *testing.T) {
	queue := newFakeBatchImageQueue("imgbatch_worker_terminal")
	processor := &fakeBatchImageProcessor{result: BatchImageProcessResult{Terminal: trueREDACTEDREDACTED
	worker := NewBatchImageWorker(queue, processor, BatchImageWorkerOptions{REDACTED)

	require.NoError(t, worker.RunOnce(context.Background()))
	require.Equal(t, []string{"imgbatch_worker_terminal"REDACTED, queue.acked)
	require.Empty(t, queue.requeued)
REDACTED

func TestBatchImageWorker_RequeuesOnProcessorError(t *testing.T) {
	queue := newFakeBatchImageQueue("imgbatch_worker_error")
	processor := &fakeBatchImageProcessor{err: errors.New("processor failed")REDACTED
	worker := NewBatchImageWorker(queue, processor, BatchImageWorkerOptions{ErrorRetryDelay: 7 * time.SecondREDACTED)

	require.NoError(t, worker.RunOnce(context.Background()))
	require.Len(t, queue.requeued, 1)
	require.Equal(t, 7*time.Second, queue.requeued[0].delay)
	require.Empty(t, queue.acked)
REDACTED

func TestBatchImageWorker_SkipsWhenJobLockNotAcquired(t *testing.T) {
	queue := newFakeBatchImageQueue("imgbatch_worker_locked")
	queue.lockAcquired = false
	processor := &fakeBatchImageProcessor{REDACTED
	worker := NewBatchImageWorker(queue, processor, BatchImageWorkerOptions{REDACTED)

	require.NoError(t, worker.RunOnce(context.Background()))
	require.Empty(t, processor.processed)
	require.Empty(t, queue.requeued)
	require.Empty(t, queue.acked)
REDACTED

func TestNewBatchImageWorkerOptionsFromConfig_UsesFiniteReserveTimeout(t *testing.T) {
	opts := NewBatchImageWorkerOptionsFromConfig(nil)
	require.Equal(t, defaultBatchImageWorkerReserveBlockTimeout, opts.ReserveBlockTimeout)
	require.Positive(t, opts.ReserveBlockTimeout)
REDACTED

type fakeBatchImageQueue struct {
	reserved     ReservedBatchImageJob
	lockAcquired bool
	acked        []string
	requeued     []fakeBatchImageRequeue
	releaseCount int
REDACTED

type fakeBatchImageRequeue struct {
	batchID string
	delay   time.Duration
REDACTED

func newFakeBatchImageQueue(batchID string) *fakeBatchImageQueue {
	return &fakeBatchImageQueue{
		reserved:     ReservedBatchImageJob{BatchID: batchIDREDACTED,
		lockAcquired: true,
REDACTED
REDACTED

func (q *fakeBatchImageQueue) Enqueue(context.Context, string) error {
	return nil
REDACTED

func (q *fakeBatchImageQueue) Reserve(context.Context, time.Duration) (ReservedBatchImageJob, error) {
	return q.reserved, nil
REDACTED

func (q *fakeBatchImageQueue) RequeueAfter(_ context.Context, batchID string, delay time.Duration) error {
	q.requeued = append(q.requeued, fakeBatchImageRequeue{batchID: batchID, delay: delayREDACTED)
	return nil
REDACTED

func (q *fakeBatchImageQueue) Ack(_ context.Context, batchID string) error {
	q.acked = append(q.acked, batchID)
	return nil
REDACTED

func (q *fakeBatchImageQueue) Heartbeat(context.Context, string) error {
	return nil
REDACTED

func (q *fakeBatchImageQueue) MoveDueDelayedToReady(context.Context, int) (int, error) {
	return 0, nil
REDACTED

func (q *fakeBatchImageQueue) RecoverStaleActive(context.Context, time.Duration, int) (int, error) {
	return 0, nil
REDACTED

func (q *fakeBatchImageQueue) TryAcquireJobLock(context.Context, string, time.Duration) (BatchImageJobLock, bool, error) {
	if !q.lockAcquired {
		return nil, false, nil
REDACTED
	return fakeBatchImageLock{release: func() { q.releaseCount++ REDACTEDREDACTED, true, nil
REDACTED

type fakeBatchImageLock struct {
	release func()
REDACTED

func (l fakeBatchImageLock) Release(context.Context) error {
	if l.release != nil {
		l.release()
REDACTED
	return nil
REDACTED

type fakeBatchImageProcessor struct {
	result    BatchImageProcessResult
	err       error
	processed []string
REDACTED

func (p *fakeBatchImageProcessor) Process(_ context.Context, batchID string) (BatchImageProcessResult, error) {
	p.processed = append(p.processed, batchID)
	return p.result, p.err
REDACTED
