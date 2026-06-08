package service

import (
	"context"
	"time"
)

func (s *ModerationService) enqueueAsync(input ContentModerationCheckInput, cfg *ContentModerationConfig, content ContentModerationInput, hashText string) {
	if s == nil || s.asyncQueue == nil {
		return
	}
	queueSize := defaultContentModerationQueueSize
	if cfg != nil && cfg.QueueSize > 0 {
		queueSize = cfg.QueueSize
	}
	if len(s.asyncQueue) >= queueSize {
		s.logger.Warn("content_moderation.async_queue_full", "user_id", input.UserID, "endpoint", input.Endpoint, "queue_size", queueSize)
		s.asyncDropped.Add(1)
		return
	}
	task := contentModerationTask{
		input:      input,
		content:    content,
		inputHash:  hashText,
		enqueuedAt: time.Now(),
	}
	select {
	case s.asyncQueue <- task:
		s.asyncEnqueued.Add(1)
	default:
		s.logger.Warn("content_moderation.async_queue_full", "user_id", input.UserID, "endpoint", input.Endpoint)
		s.asyncDropped.Add(1)
	}
}

func (s *ModerationService) enqueueRecord(input ContentModerationCheckInput, cfg *ContentModerationConfig, log *ContentModerationLog, inputHash string, recordHash bool, applySideEffects bool) {
	if s == nil || s.asyncQueue == nil || log == nil {
		return
	}
	queueSize := defaultContentModerationQueueSize
	if cfg != nil && cfg.QueueSize > 0 {
		queueSize = cfg.QueueSize
	}
	if len(s.asyncQueue) >= queueSize {
		s.logger.Warn("content_moderation.record_queue_full", "user_id", input.UserID, "endpoint", input.Endpoint, "action", log.Action, "queue_size", queueSize)
		s.asyncDropped.Add(1)
		return
	}
	task := contentModerationTask{
		input:            input,
		inputHash:        inputHash,
		log:              log,
		config:           cloneContentModerationConfig(cfg),
		recordHash:       recordHash,
		applySideEffects: applySideEffects,
		enqueuedAt:       time.Now(),
	}
	select {
	case s.asyncQueue <- task:
		s.asyncEnqueued.Add(1)
	default:
		s.logger.Warn("content_moderation.record_queue_full", "user_id", input.UserID, "endpoint", input.Endpoint, "action", log.Action)
		s.asyncDropped.Add(1)
	}
}

func (s *ModerationService) worker(id int) {
	defer s.wg.Done()
	for {
		select {
		case <-s.ctx.Done():
			return
		default:
		}
		ctx, cancel := context.WithTimeout(s.ctx, maxContentModerationTimeoutMS*time.Millisecond+10*time.Second)
		cfg, err := s.loadConfig(ctx)
		if err != nil || id >= cfg.WorkerCount {
			cancel()
			select {
			case <-s.ctx.Done():
				return
			case <-time.After(time.Second):
			}
			continue
		}
		task, ok := s.dequeueAsyncTask(ctx, time.Second)
		if !ok {
			cancel()
			continue
		}
		s.runWorkerTask(ctx, id, cfg, task)
		cancel()
	}
}

func (s *ModerationService) runWorkerTask(ctx context.Context, id int, cfg *ContentModerationConfig, task contentModerationTask) {
	defer func() {
		if r := recover(); r != nil {
			s.logger.Error("content_moderation.worker_panic", "worker_id", id, "recover", r)
		}
	}()
	if task.log != nil {
		s.asyncActive.Add(1)
		defer s.asyncActive.Add(-1)
		queueDelay := int(time.Since(task.enqueuedAt).Milliseconds())
		task.log.QueueDelayMS = &queueDelay
		taskCfg := task.config
		if taskCfg == nil {
			taskCfg = cfg
		}
		s.persistContentModerationLog(ctx, taskCfg, task.log, task.inputHash, task.recordHash, task.applySideEffects)
		s.asyncProcessed.Add(1)
		return
	}
	if !cfg.Enabled || cfg.Mode == ContentModerationModeOff || len(cfg.apiKeys()) == 0 {
		return
	}
	if !cfg.includesGroup(task.input.GroupID) {
		return
	}
	if !cfg.includesModel(task.input.Model) {
		return
	}
	s.asyncActive.Add(1)
	defer s.asyncActive.Add(-1)
	queueDelay := int(time.Since(task.enqueuedAt).Milliseconds())
	_ = s.checkSync(ctx, task.input, cfg, task.content, task.inputHash, &queueDelay, false)
	s.asyncProcessed.Add(1)
}

func (s *ModerationService) dequeueAsyncTask(ctx context.Context, idleWait time.Duration) (contentModerationTask, bool) {
	var zero contentModerationTask
	if s == nil || s.asyncQueue == nil {
		return zero, false
	}
	if idleWait <= 0 {
		idleWait = time.Second
	}
	timer := time.NewTimer(idleWait)
	defer timer.Stop()
	select {
	case task, ok := <-s.asyncQueue:
		return task, ok
	case <-ctx.Done():
		return zero, false
	case <-timer.C:
		return zero, false
	}
}

func (s *ModerationService) cleanupWorker() {
	defer s.wg.Done()
	timer := time.NewTimer(contentModerationCleanupDelay)
	defer timer.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-timer.C:
			s.runCleanupOnce()
			timer.Reset(contentModerationCleanupInterval)
		}
	}
}

func (s *ModerationService) runCleanupOnce() {
	if s == nil || s.repo == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), contentModerationCleanupTimeout)
	defer cancel()
	cfg, err := s.loadConfig(ctx)
	if err != nil {
		s.logger.Warn("content_moderation.cleanup_load_config_failed", "error", err)
		return
	}
	now := time.Now()
	hitBefore := now.AddDate(0, 0, -cfg.HitRetentionDays)
	nonHitBefore := now.AddDate(0, 0, -cfg.NonHitRetentionDays)
	result, err := s.repo.CleanupExpiredLogs(ctx, hitBefore, nonHitBefore)
	if err != nil {
		s.logger.Warn("content_moderation.cleanup_failed", "error", err)
		return
	}
	if result == nil {
		return
	}
	s.lastCleanupUnix.Store(result.FinishedAt.Unix())
	s.lastCleanupDeletedHit.Store(result.DeletedHit)
	s.lastCleanupDeletedNonHit.Store(result.DeletedNonHit)
}
