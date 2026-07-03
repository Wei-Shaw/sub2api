package service

import (
	"context"
	"errors"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

const (
	defaultBatchImageWorkerLockTTL             = 5 * time.Minute
	defaultBatchImageWorkerLockConflictDelay   = 5 * time.Second
	defaultBatchImageWorkerErrorRetryDelay     = time.Minute
	defaultBatchImageWorkerRequeueDelay        = 30 * time.Second
	defaultBatchImageWorkerDelayedPollInterval = 5 * time.Second
	defaultBatchImageWorkerRecoveryInterval    = 5 * time.Minute
	defaultBatchImageWorkerStaleActiveAfter    = 10 * time.Minute
	defaultBatchImageWorkerDelayedMoveLimit    = 100
	defaultBatchImageWorkerRecoverLimit        = 100
	defaultBatchImageWorkerErrorBackoff        = time.Second
	defaultBatchImageWorkerReserveBlockTimeout = 5 * time.Second
)

type BatchImageProcessor interface {
	Process(ctx context.Context, batchID string) (BatchImageProcessResult, error)
REDACTED

type BatchImageProcessResult struct {
	RequeueAfter time.Duration
	Terminal     bool
REDACTED

type BatchImageWorkerOptions struct {
	ReserveBlockTimeout time.Duration
	JobLockTTL          time.Duration
	LockConflictDelay   time.Duration
	DefaultRequeueDelay time.Duration
	ErrorRetryDelay     time.Duration
	ErrorBackoff        time.Duration
	DelayedPollInterval time.Duration
	RecoveryInterval    time.Duration
	StaleActiveAfter    time.Duration
	DelayedMoveLimit    int
	RecoverLimit        int
REDACTED

type BatchImageWorker struct {
	queue     BatchImageQueue
	processor BatchImageProcessor
	opts      BatchImageWorkerOptions
REDACTED

func NewBatchImageWorker(queue BatchImageQueue, processor BatchImageProcessor, opts BatchImageWorkerOptions) *BatchImageWorker {
	return &BatchImageWorker{
		queue:     queue,
		processor: processor,
		opts:      normalizeBatchImageWorkerOptions(opts),
REDACTED
REDACTED

func NewBatchImageWorkerOptionsFromConfig(cfg *config.Config) BatchImageWorkerOptions {
	if cfg == nil {
		return normalizeBatchImageWorkerOptions(BatchImageWorkerOptions{REDACTED)
REDACTED
	return normalizeBatchImageWorkerOptions(BatchImageWorkerOptions{
		JobLockTTL:          time.Duration(cfg.BatchImage.JobLockTTLSeconds) * time.Second,
		LockConflictDelay:   time.Duration(cfg.BatchImage.LockConflictDelaySeconds) * time.Second,
		DefaultRequeueDelay: time.Duration(cfg.BatchImage.DefaultRequeueDelaySeconds) * time.Second,
		ErrorRetryDelay:     time.Duration(cfg.BatchImage.ErrorRetryDelaySeconds) * time.Second,
		DelayedPollInterval: time.Duration(cfg.BatchImage.DelayedMoverIntervalSeconds) * time.Second,
		RecoveryInterval:    time.Duration(cfg.BatchImage.RecoveryIntervalSeconds) * time.Second,
		StaleActiveAfter:    time.Duration(cfg.BatchImage.StaleActiveAfterSeconds) * time.Second,
		DelayedMoveLimit:    cfg.BatchImage.DelayedMoveLimit,
		RecoverLimit:        cfg.BatchImage.RecoverLimit,
REDACTED)
REDACTED

func normalizeBatchImageWorkerOptions(opts BatchImageWorkerOptions) BatchImageWorkerOptions {
	if opts.ReserveBlockTimeout <= 0 {
		opts.ReserveBlockTimeout = defaultBatchImageWorkerReserveBlockTimeout
REDACTED
	if opts.JobLockTTL <= 0 {
		opts.JobLockTTL = defaultBatchImageWorkerLockTTL
REDACTED
	if opts.LockConflictDelay <= 0 {
		opts.LockConflictDelay = defaultBatchImageWorkerLockConflictDelay
REDACTED
	if opts.DefaultRequeueDelay <= 0 {
		opts.DefaultRequeueDelay = defaultBatchImageWorkerRequeueDelay
REDACTED
	if opts.ErrorRetryDelay <= 0 {
		opts.ErrorRetryDelay = defaultBatchImageWorkerErrorRetryDelay
REDACTED
	if opts.ErrorBackoff <= 0 {
		opts.ErrorBackoff = defaultBatchImageWorkerErrorBackoff
REDACTED
	if opts.DelayedPollInterval <= 0 {
		opts.DelayedPollInterval = defaultBatchImageWorkerDelayedPollInterval
REDACTED
	if opts.RecoveryInterval <= 0 {
		opts.RecoveryInterval = defaultBatchImageWorkerRecoveryInterval
REDACTED
	if opts.StaleActiveAfter <= 0 {
		opts.StaleActiveAfter = defaultBatchImageWorkerStaleActiveAfter
REDACTED
	if opts.DelayedMoveLimit <= 0 {
		opts.DelayedMoveLimit = defaultBatchImageWorkerDelayedMoveLimit
REDACTED
	if opts.RecoverLimit <= 0 {
		opts.RecoverLimit = defaultBatchImageWorkerRecoverLimit
REDACTED
	return opts
REDACTED

func (w *BatchImageWorker) Run(ctx context.Context) {
	if w == nil {
		return
REDACTED
	for {
		if err := ctx.Err(); err != nil {
			return
	REDACTED
		if err := w.RunOnce(ctx); err != nil && ctx.Err() == nil {
			sleepOrDone(ctx, w.opts.ErrorBackoff)
	REDACTED
REDACTED
REDACTED

func (w *BatchImageWorker) RunOnce(ctx context.Context) error {
	if w == nil || w.queue == nil || w.processor == nil {
		return nil
REDACTED

	reserved, err := w.queue.Reserve(ctx, w.opts.ReserveBlockTimeout)
	if errors.Is(err, ErrBatchImageQueueEmpty) {
		return nil
REDACTED
	if err != nil {
		return err
REDACTED

	lock, ok, err := w.queue.TryAcquireJobLock(ctx, reserved.BatchID, w.opts.JobLockTTL)
	if err != nil {
		if requeueErr := w.queue.RequeueAfter(ctx, reserved.BatchID, w.opts.LockConflictDelay); requeueErr != nil {
			return requeueErr
	REDACTED
		return err
REDACTED
	if !ok {
		return nil
REDACTED
	defer func() {
		_ = lock.Release(ctx)
REDACTED()

	result, err := w.processor.Process(ctx, reserved.BatchID)
	if err != nil {
		return w.queue.RequeueAfter(ctx, reserved.BatchID, w.opts.ErrorRetryDelay)
REDACTED
	if result.Terminal {
		return w.queue.Ack(ctx, reserved.BatchID)
REDACTED
	delay := result.RequeueAfter
	if delay <= 0 {
		delay = w.opts.DefaultRequeueDelay
REDACTED
	return w.queue.RequeueAfter(ctx, reserved.BatchID, delay)
REDACTED

func (w *BatchImageWorker) MoveDueDelayedOnce(ctx context.Context) (int, error) {
	if w == nil || w.queue == nil {
		return 0, nil
REDACTED
	return w.queue.MoveDueDelayedToReady(ctx, w.opts.DelayedMoveLimit)
REDACTED

func (w *BatchImageWorker) RunDelayedMover(ctx context.Context) {
	if w == nil {
		return
REDACTED
	for {
		if err := ctx.Err(); err != nil {
			return
	REDACTED
		moved, _ := w.MoveDueDelayedOnce(ctx)
		if moved > 0 {
			continue
	REDACTED
		sleepOrDone(ctx, w.opts.DelayedPollInterval)
REDACTED
REDACTED

func (w *BatchImageWorker) RecoverStaleActiveOnce(ctx context.Context) (int, error) {
	if w == nil || w.queue == nil {
		return 0, nil
REDACTED
	return w.queue.RecoverStaleActive(ctx, w.opts.StaleActiveAfter, w.opts.RecoverLimit)
REDACTED

func (w *BatchImageWorker) RunStaleActiveRecovery(ctx context.Context) {
	if w == nil {
		return
REDACTED
	for {
		if err := ctx.Err(); err != nil {
			return
	REDACTED
		_, _ = w.RecoverStaleActiveOnce(ctx)
		sleepOrDone(ctx, w.opts.RecoveryInterval)
REDACTED
REDACTED

func sleepOrDone(ctx context.Context, d time.Duration) {
	if d <= 0 {
		return
REDACTED
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
REDACTED
REDACTED
