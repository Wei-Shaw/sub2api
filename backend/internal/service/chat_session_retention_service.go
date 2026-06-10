package service

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

const chatSessionRetentionWorkerName = "chat_session_retention_worker"

// ChatSessionRetentionService periodically deletes old captured chat sessions.
// chat_messages and chat_message_events are removed by ON DELETE CASCADE.
type ChatSessionRetentionService struct {
	repo ChatSessionRepository
	cfg  *config.Config

	running   int32
	startOnce sync.Once
	stopOnce  sync.Once
	stopCh    chan struct{}
}

func NewChatSessionRetentionService(repo ChatSessionRepository, cfg *config.Config) *ChatSessionRetentionService {
	return &ChatSessionRetentionService{
		repo:   repo,
		cfg:    cfg,
		stopCh: make(chan struct{}),
	}
}

func (s *ChatSessionRetentionService) Start() {
	if s == nil {
		return
	}
	cfg := s.retentionConfig()
	if !cfg.Enabled {
		logger.LegacyPrintf("service.chat_session_retention", "[ChatSessionRetention] not started (disabled)")
		return
	}
	if s.repo == nil {
		logger.LegacyPrintf("service.chat_session_retention", "[ChatSessionRetention] not started (missing deps)")
		return
	}

	interval := time.Duration(cfg.IntervalSeconds) * time.Second
	if interval <= 0 {
		interval = 24 * time.Hour
	}
	s.startOnce.Do(func() {
		logger.LegacyPrintf("service.chat_session_retention", "[ChatSessionRetention] started interval=%s retention_days=%d batch_size=%d task_timeout=%s", interval, cfg.RetentionDays, cfg.BatchSize, time.Duration(cfg.TaskTimeoutSeconds)*time.Second)
		go s.runLoop(interval)
	})
}

func (s *ChatSessionRetentionService) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		if s.stopCh != nil {
			close(s.stopCh)
		}
		logger.LegacyPrintf("service.chat_session_retention", "[ChatSessionRetention] stopped")
	})
}

func (s *ChatSessionRetentionService) runLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	s.runOnce()
	for {
		select {
		case <-ticker.C:
			s.runOnce()
		case <-s.stopCh:
			return
		}
	}
}

func (s *ChatSessionRetentionService) runOnce() {
	if s == nil || s.repo == nil {
		return
	}
	if !atomic.CompareAndSwapInt32(&s.running, 0, 1) {
		logger.LegacyPrintf("service.chat_session_retention", "[ChatSessionRetention] skipped: already_running=true")
		return
	}
	defer atomic.StoreInt32(&s.running, 0)

	cfg := s.retentionConfig()
	if !cfg.Enabled || cfg.RetentionDays <= 0 {
		return
	}
	batchSize := cfg.BatchSize
	if batchSize <= 0 {
		batchSize = 1000
	}
	timeout := time.Duration(cfg.TaskTimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cutoff := time.Now().AddDate(0, 0, -cfg.RetentionDays)
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			logger.LegacyPrintf("service.chat_session_retention", "[ChatSessionRetention] interrupted deleted=%d err=%v", total, err)
			return
		}
		deleted, err := s.repo.DeleteSessionsBefore(ctx, cutoff, batchSize)
		if err != nil {
			logger.LegacyPrintf("service.chat_session_retention", "[ChatSessionRetention] cleanup failed deleted=%d err=%v", total, err)
			return
		}
		total += deleted
		if deleted == 0 || deleted < int64(batchSize) {
			if total > 0 {
				logger.LegacyPrintf("service.chat_session_retention", "[ChatSessionRetention] cleanup completed deleted=%d cutoff=%s", total, cutoff.UTC().Format(time.RFC3339))
			}
			return
		}
	}
}

func (s *ChatSessionRetentionService) retentionConfig() config.ChatSessionRetentionConfig {
	if s != nil && s.cfg != nil {
		return s.cfg.ChatSessionRetention
	}
	return config.ChatSessionRetentionConfig{
		Enabled:            true,
		RetentionDays:      90,
		BatchSize:          1000,
		IntervalSeconds:    int((24 * time.Hour).Seconds()),
		TaskTimeoutSeconds: int((5 * time.Minute).Seconds()),
	}
}
