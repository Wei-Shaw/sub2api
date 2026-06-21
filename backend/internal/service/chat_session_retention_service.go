package service

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

const (
	chatSessionRetentionWorkerName = "chat_session_retention_worker"
	chatSessionRetentionTZ         = "Asia/Shanghai"
	chatSessionRetentionHour       = 3
	chatSessionRetentionMinute     = 0
	chatSessionBytesPerGiB         = 1024 * 1024 * 1024
)

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

	s.startOnce.Do(func() {
		loc := chatSessionRetentionLocation()
		nextRun := nextChatSessionRetentionRun(time.Now(), loc)
		logger.LegacyPrintf(
			"service.chat_session_retention",
			"[ChatSessionRetention] started schedule=%02d:%02d timezone=%s next_run=%s retention_days=%d batch_size=%d task_timeout=%s",
			chatSessionRetentionHour,
			chatSessionRetentionMinute,
			loc.String(),
			nextRun.In(loc).Format(time.RFC3339),
			cfg.RetentionDays,
			cfg.BatchSize,
			time.Duration(cfg.TaskTimeoutSeconds)*time.Second,
		)
		go s.runLoop(loc)
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

func (s *ChatSessionRetentionService) runLoop(loc *time.Location) {
	if loc == nil {
		loc = chatSessionRetentionLocation()
	}
	for {
		nextRun := nextChatSessionRetentionRun(time.Now(), loc)
		wait := time.Until(nextRun)
		if wait < 0 {
			wait = 0
		}
		timer := time.NewTimer(wait)
		select {
		case <-timer.C:
			s.runOnce()
		case <-s.stopCh:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return
		}
	}
}

func chatSessionRetentionLocation() *time.Location {
	loc, err := time.LoadLocation(chatSessionRetentionTZ)
	if err == nil {
		return loc
	}
	return time.FixedZone(chatSessionRetentionTZ, 8*60*60)
}

func nextChatSessionRetentionRun(now time.Time, loc *time.Location) time.Time {
	if loc == nil {
		loc = chatSessionRetentionLocation()
	}
	localNow := now.In(loc)
	next := time.Date(
		localNow.Year(),
		localNow.Month(),
		localNow.Day(),
		chatSessionRetentionHour,
		chatSessionRetentionMinute,
		0,
		0,
		loc,
	)
	if localNow.After(next) {
		next = next.AddDate(0, 0, 1)
	}
	return next
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
			s.runLowDiskPayloadCleanup(ctx, cfg)
			return
		}
	}
}

func (s *ChatSessionRetentionService) runLowDiskPayloadCleanup(ctx context.Context, cfg config.ChatSessionRetentionConfig) {
	if s == nil || s.repo == nil || !cfg.LowDiskDeleteOldestPayloadDay || cfg.MinFreeDiskGB <= 0 {
		return
	}
	minFreeBytes := uint64(cfg.MinFreeDiskGB) * chatSessionBytesPerGiB
	minKeepDays := cfg.LowDiskMinKeepDays
	for {
		if err := ctx.Err(); err != nil {
			logger.LegacyPrintf("service.chat_session_retention", "[ChatSessionRetention] low_disk interrupted err=%v", err)
			return
		}
		result, err := s.repo.DeleteOldestPayloadDayIfLowDisk(ctx, minFreeBytes, minKeepDays)
		if err != nil {
			logger.LegacyPrintf("service.chat_session_retention", "[ChatSessionRetention] low_disk cleanup failed threshold_bytes=%d err=%v", minFreeBytes, err)
			return
		}
		if result == nil || !result.Triggered {
			return
		}
		if !result.Deleted {
			logger.LegacyPrintf(
				"service.chat_session_retention",
				"[ChatSessionRetention] low_disk triggered but no eligible payload day available_bytes=%d threshold_bytes=%d min_keep_days=%d",
				result.AvailableBytes,
				result.ThresholdBytes,
				minKeepDays,
			)
			return
		}
		logger.LegacyPrintf(
			"service.chat_session_retention",
			"[ChatSessionRetention] low_disk deleted oldest payload day available_bytes=%d threshold_bytes=%d deleted_date=%s deleted_path=%s freed_estimate_bytes=%d",
			result.AvailableBytes,
			result.ThresholdBytes,
			result.DeletedDate,
			result.DeletedPath,
			result.FreedEstimateBytes,
		)
	}
}

func (s *ChatSessionRetentionService) retentionConfig() config.ChatSessionRetentionConfig {
	if s != nil && s.cfg != nil {
		return s.cfg.ChatSessionRetention
	}
	return config.ChatSessionRetentionConfig{
		Enabled:                       true,
		RetentionDays:                 30,
		BatchSize:                     1000,
		IntervalSeconds:               int((24 * time.Hour).Seconds()),
		TaskTimeoutSeconds:            int((5 * time.Minute).Seconds()),
		MinFreeDiskGB:                 5,
		LowDiskDeleteOldestPayloadDay: true,
		LowDiskMinKeepDays:            1,
	}
}
