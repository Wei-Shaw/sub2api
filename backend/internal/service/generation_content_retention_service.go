package service

import (
	"context"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

// 保留期清理（NULL-OUT）的代码兜底默认（配置为 0 时使用），与 ContentRetention dark-by-default 配套：
// 配置无 SetDefault/Validate，全部走这里，避免强制配置 & 不破坏裸 &config.Config{} 测试。
const (
	defaultGenerationRetentionDays            = 90
	defaultGenerationRetentionBatchSize       = 500
	defaultGenerationRetentionIntervalSeconds = 3600
	// minGenerationRetentionDays 夹紧下限：护住看板的近 7 日窗口（CapturedWeek / 7 日序列），
	// 即使配置写了过小的保留天数也不会把近 7 日样本清空。
	minGenerationRetentionDays = 7
)

// GenerationContentRetentionService 按保留天数清理 ai_generation_content（NULL-OUT：清空内容、保留计数行），
// 与计费/usage 保留解耦。镜像 IdempotencyCleanupService：Start/Stop/runLoop/cleanupOnce。
// 额外导出 RunOnce(ctx, dryRun) 以满足「独立可调用 + 可 dry-run」。
type GenerationContentRetentionService struct {
	repo          GenerationContentRepository
	retentionDays int
	interval      time.Duration
	batch         int
	dryRun        bool

	startOnce sync.Once
	stopOnce  sync.Once
	stopCh    chan struct{}
}

func NewGenerationContentRetentionService(repo GenerationContentRepository, cfg *config.Config) *GenerationContentRetentionService {
	days := defaultGenerationRetentionDays
	interval := time.Duration(defaultGenerationRetentionIntervalSeconds) * time.Second
	batch := defaultGenerationRetentionBatchSize
	dryRun := false
	if cfg != nil {
		rc := cfg.Gateway.ContentRetention
		if rc.RetentionDays > 0 {
			days = rc.RetentionDays
		}
		if rc.IntervalSeconds > 0 {
			interval = time.Duration(rc.IntervalSeconds) * time.Second
		}
		if rc.BatchSize > 0 {
			batch = rc.BatchSize
		}
		dryRun = rc.DryRun
	}
	if days < minGenerationRetentionDays {
		days = minGenerationRetentionDays // 夹紧，护住看板 7 日窗口
	}
	return &GenerationContentRetentionService{
		repo:          repo,
		retentionDays: days,
		interval:      interval,
		batch:         batch,
		dryRun:        dryRun,
		stopCh:        make(chan struct{}),
	}
}

func (s *GenerationContentRetentionService) Start() {
	if s == nil || s.repo == nil {
		return
	}
	s.startOnce.Do(func() {
		logger.LegacyPrintf("service.generation_content_retention",
			"[GenContentRetention] started days=%d interval=%s batch=%d dryRun=%v",
			s.retentionDays, s.interval, s.batch, s.dryRun)
		go s.runLoop()
	})
}

func (s *GenerationContentRetentionService) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		close(s.stopCh)
		logger.LegacyPrintf("service.generation_content_retention", "[GenContentRetention] stopped")
	})
}

func (s *GenerationContentRetentionService) runLoop() {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	// 启动后先清理一轮，防止重启后积压。
	s.cleanupOnce()

	for {
		select {
		case <-ticker.C:
			s.cleanupOnce()
		case <-s.stopCh:
			return
		}
	}
}

func (s *GenerationContentRetentionService) cleanupOnce() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	affected, err := s.RunOnce(ctx, s.dryRun)
	if err != nil {
		logger.LegacyPrintf("service.generation_content_retention", "[GenContentRetention] cleanup failed err=%v", err)
		return
	}
	if affected > 0 {
		mode := "purged"
		if s.dryRun {
			mode = "would-purge(dry-run)"
		}
		logger.LegacyPrintf("service.generation_content_retention",
			"[GenContentRetention] %s count=%d retentionDays=%d", mode, affected, s.retentionDays)
	}
}

// RunOnce 独立可调用入口（daemon / 管理端 / 测试共用）：
// 计算 cutoff = now - retentionDays，把过期内容置空（NULL-OUT）。
// dryRun=true：只 COUNT 一批不改，返回该批将命中行数（证明「可 dry-run」且零副作用）。
// 否则按批清理直到某批 < batch（排空），返回累计受影响行数。
func (s *GenerationContentRetentionService) RunOnce(ctx context.Context, dryRun bool) (int64, error) {
	if s == nil || s.repo == nil {
		return 0, nil
	}
	cutoff := time.Now().Add(-time.Duration(s.retentionDays) * 24 * time.Hour)
	if dryRun {
		return s.repo.PurgeExpiredContent(ctx, cutoff, s.batch, true)
	}
	var total int64
	for {
		n, err := s.repo.PurgeExpiredContent(ctx, cutoff, s.batch, false)
		if err != nil {
			return total, err
		}
		total += n
		if n < int64(s.batch) {
			break // 末批，已排空
		}
		select {
		case <-ctx.Done():
			return total, ctx.Err()
		default:
		}
	}
	return total, nil
}
