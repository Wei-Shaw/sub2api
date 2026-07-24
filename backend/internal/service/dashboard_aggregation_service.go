package service

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/google/uuid"
)

const (
	defaultDashboardAggregationTimeout         = 2 * time.Minute
	defaultDashboardAggregationBackfillTimeout = 30 * time.Minute
	dashboardAggregationRetentionInterval      = 6 * time.Hour

	// dashboardAggregationLeaderLockKey gates the periodic scheduled aggregation so
	// that only one instance runs it per cycle in a multi-replica deployment.
	dashboardAggregationLeaderLockKey = "dashboard:aggregation:leader"
	// dashboardAggregationLeaderLockTTL must exceed the job's worst-case runtime
	// (defaultDashboardAggregationTimeout) so the lock never expires mid-run.
	dashboardAggregationLeaderLockTTL = 5 * time.Minute
)

var (
	// ErrDashboardBackfillDisabled 当配置禁用回填时返回。
	ErrDashboardBackfillDisabled = errors.New("仪表盘聚合回填已禁用")
	// ErrDashboardBackfillTooLarge 当回填跨度超过限制时返回。
	ErrDashboardBackfillTooLarge   = errors.New("回填时间跨度过大")
	errDashboardAggregationRunning = errors.New("聚合作业正在运行")
	errDashboardAggregationStopped = errors.New("聚合服务正在停止")
)

// DashboardAggregationRepository 定义仪表盘预聚合仓储接口。
type DashboardAggregationRepository interface {
	AggregateRange(ctx context.Context, start, end time.Time) error
	// RecomputeRange 重新计算指定时间范围内的聚合数据（包含活跃用户等派生表）。
	// 设计目的：当 usage_logs 被批量删除/回滚后，确保聚合表可恢复一致性。
	RecomputeRange(ctx context.Context, start, end time.Time) error
	GetAggregationWatermark(ctx context.Context) (time.Time, error)
	UpdateAggregationWatermark(ctx context.Context, aggregatedAt time.Time) error
	CleanupAggregates(ctx context.Context, hourlyCutoff, dailyCutoff time.Time) error
	CleanupUsageLogs(ctx context.Context, cutoff time.Time) error
	CleanupUsageBillingDedup(ctx context.Context, cutoff time.Time) error
	EnsureUsageLogsPartitions(ctx context.Context, now time.Time) error
}

// DashboardAggregationService 负责定时聚合与回填。
type DashboardAggregationService struct {
	repo                 DashboardAggregationRepository
	timingWheel          *TimingWheelService
	cfg                  config.DashboardAggregationConfig
	running              int32
	lastRetentionCleanup atomic.Value // time.Time
	lifecycleMu          sync.Mutex
	started              bool
	stopped              bool
	runCtx               context.Context
	runCancel            context.CancelFunc
	wg                   sync.WaitGroup

	lockCache  LeaderLockCache
	db         *sql.DB
	instanceID string
}

// NewDashboardAggregationService 创建聚合服务。
func NewDashboardAggregationService(repo DashboardAggregationRepository, timingWheel *TimingWheelService, cfg *config.Config) *DashboardAggregationService {
	var aggCfg config.DashboardAggregationConfig
	if cfg != nil {
		aggCfg = cfg.DashboardAgg
	}
	runCtx, runCancel := context.WithCancel(context.Background())
	return &DashboardAggregationService{
		repo:        repo,
		timingWheel: timingWheel,
		cfg:         aggCfg,
		instanceID:  uuid.NewString(),
		runCtx:      runCtx,
		runCancel:   runCancel,
	}
}

// SetLeaderLock injects the leader-lock cache and DB used to elect a single
// instance for the periodic scheduled aggregation. When both are nil the job runs
// ungated (single-instance / test behavior).
func (s *DashboardAggregationService) SetLeaderLock(lockCache LeaderLockCache, db *sql.DB) {
	if s == nil {
		return
	}
	s.lockCache = lockCache
	s.db = db
}

// Start 启动定时聚合作业（重启生效配置）。
func (s *DashboardAggregationService) Start() {
	if s == nil || s.repo == nil || s.timingWheel == nil {
		return
	}
	if !s.cfg.Enabled {
		logger.LegacyPrintf("service.dashboard_aggregation", "[DashboardAggregation] 聚合作业已禁用")
		return
	}
	s.lifecycleMu.Lock()
	if s.started || s.stopped {
		s.lifecycleMu.Unlock()
		return
	}
	s.started = true

	interval := time.Duration(s.cfg.IntervalSeconds) * time.Second
	if interval <= 0 {
		interval = time.Minute
	}

	if s.cfg.RecomputeDays > 0 {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.recomputeRecentDays()
		}()
	}

	s.timingWheel.ScheduleRecurring("dashboard:aggregation", interval, func() {
		s.runManagedScheduledAggregation()
	})
	s.lifecycleMu.Unlock()
	logger.LegacyPrintf("service.dashboard_aggregation", "[DashboardAggregation] 聚合作业启动 (interval=%v, lookback=%ds)", interval, s.cfg.LookbackSeconds)
	if !s.cfg.BackfillEnabled {
		logger.LegacyPrintf("service.dashboard_aggregation", "[DashboardAggregation] 回填已禁用，如需补齐保留窗口以外历史数据请手动回填")
	}
}

func (s *DashboardAggregationService) runManagedScheduledAggregation() {
	if err := s.beginManagedRun(); err != nil {
		return
	}
	defer s.wg.Done()
	s.runScheduledAggregation()
}

func (s *DashboardAggregationService) Stop() {
	ctx, cancel := context.WithTimeout(context.Background(), defaultBackgroundStopTimeout)
	defer cancel()
	if err := s.StopContext(ctx); err != nil {
		logger.LegacyPrintf("service.dashboard_aggregation", "[DashboardAggregation] shutdown incomplete: %v", err)
	}
}

func (s *DashboardAggregationService) StopContext(ctx context.Context) error {
	if s == nil {
		return nil
	}
	s.lifecycleMu.Lock()
	if !s.stopped {
		s.stopped = true
		if s.runCancel != nil {
			s.runCancel()
		}
		if s.started && s.timingWheel != nil {
			s.timingWheel.Cancel("dashboard:aggregation")
		}
	}
	s.lifecycleMu.Unlock()
	return waitForBackgroundWorkers(ctx, &s.wg)
}

func (s *DashboardAggregationService) beginManagedRun() error {
	if err := requireActiveDeploymentRuntime(); err != nil {
		return err
	}
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	if err := requireActiveDeploymentRuntime(); err != nil {
		return err
	}
	if s.stopped {
		return errDashboardAggregationStopped
	}
	s.wg.Add(1)
	return nil
}

func (s *DashboardAggregationService) launchManaged(run func(context.Context)) error {
	if err := s.beginManagedRun(); err != nil {
		return err
	}
	parent := s.runtimeContext()
	go func() {
		defer s.wg.Done()
		run(parent)
	}()
	return nil
}

// TriggerBackfill 触发回填（异步）。
func (s *DashboardAggregationService) TriggerBackfill(start, end time.Time) error {
	if s == nil || s.repo == nil {
		return errors.New("聚合服务未初始化")
	}
	if err := requireActiveDeploymentRuntime(); err != nil {
		return err
	}
	if !s.cfg.BackfillEnabled {
		logger.LegacyPrintf("service.dashboard_aggregation", "[DashboardAggregation] 回填被拒绝: backfill_enabled=false")
		return ErrDashboardBackfillDisabled
	}
	if !end.After(start) {
		return errors.New("回填时间范围无效")
	}
	if s.cfg.BackfillMaxDays > 0 {
		maxRange := time.Duration(s.cfg.BackfillMaxDays) * 24 * time.Hour
		if end.Sub(start) > maxRange {
			return ErrDashboardBackfillTooLarge
		}
	}

	return s.launchManaged(func(parent context.Context) {
		ctx, cancel := context.WithTimeout(parent, defaultDashboardAggregationBackfillTimeout)
		defer cancel()
		if err := s.backfillRange(ctx, start, end); err != nil {
			logger.LegacyPrintf("service.dashboard_aggregation", "[DashboardAggregation] 回填失败: %v", err)
		}
	})
}

// TriggerRecomputeRange 触发指定范围的重新计算（异步）。
// 与 TriggerBackfill 不同：
// - 不依赖 backfill_enabled（这是内部一致性修复）
// - 不更新 watermark（避免影响正常增量聚合游标）
func (s *DashboardAggregationService) TriggerRecomputeRange(start, end time.Time) error {
	if s == nil || s.repo == nil {
		return errors.New("聚合服务未初始化")
	}
	if err := requireActiveDeploymentRuntime(); err != nil {
		return err
	}
	if !s.cfg.Enabled {
		return errors.New("聚合服务已禁用")
	}
	if !end.After(start) {
		return errors.New("重新计算时间范围无效")
	}

	return s.launchManaged(func(parent context.Context) {
		const maxRetries = 3
		for i := 0; i < maxRetries; i++ {
			ctx, cancel := context.WithTimeout(parent, defaultDashboardAggregationBackfillTimeout)
			err := s.recomputeRange(ctx, start, end)
			cancel()
			if err == nil {
				return
			}
			if !errors.Is(err, errDashboardAggregationRunning) {
				logger.LegacyPrintf("service.dashboard_aggregation", "[DashboardAggregation] 重新计算失败: %v", err)
				return
			}
			timer := time.NewTimer(5 * time.Second)
			select {
			case <-timer.C:
			case <-parent.Done():
				timer.Stop()
				return
			}
		}
		logger.LegacyPrintf("service.dashboard_aggregation", "[DashboardAggregation] 重新计算放弃: 聚合作业持续占用")
	})
}

func (s *DashboardAggregationService) recomputeRecentDays() {
	days := s.cfg.RecomputeDays
	if days <= 0 {
		return
	}
	now := time.Now().UTC()
	start := now.AddDate(0, 0, -days)

	ctx, cancel := context.WithTimeout(s.runtimeContext(), defaultDashboardAggregationBackfillTimeout)
	defer cancel()
	if err := s.backfillRange(ctx, start, now); err != nil {
		logger.LegacyPrintf("service.dashboard_aggregation", "[DashboardAggregation] 启动重算失败: %v", err)
		return
	}
}

func (s *DashboardAggregationService) runtimeContext() context.Context {
	if s != nil && s.runCtx != nil {
		return s.runCtx
	}
	return context.Background()
}

func (s *DashboardAggregationService) recomputeRange(ctx context.Context, start, end time.Time) error {
	if !atomic.CompareAndSwapInt32(&s.running, 0, 1) {
		return errDashboardAggregationRunning
	}
	defer atomic.StoreInt32(&s.running, 0)

	jobStart := time.Now().UTC()
	if err := s.repo.RecomputeRange(ctx, start, end); err != nil {
		return err
	}
	logger.LegacyPrintf("service.dashboard_aggregation", "[DashboardAggregation] 重新计算完成 (start=%s end=%s duration=%s)",
		start.UTC().Format(time.RFC3339),
		end.UTC().Format(time.RFC3339),
		time.Since(jobStart).String(),
	)
	return nil
}

func (s *DashboardAggregationService) runScheduledAggregation() {
	if !atomic.CompareAndSwapInt32(&s.running, 0, 1) {
		return
	}
	defer atomic.StoreInt32(&s.running, 0)

	jobStart := time.Now().UTC()
	ctx, cancel := context.WithTimeout(s.runtimeContext(), defaultDashboardAggregationTimeout)
	defer cancel()

	// Multi-instance guard: only the leader runs the periodic aggregation; peers
	// skip this cycle to avoid N× redundant GROUP BY queries and watermark races.
	release, ok := tryAcquireSingletonLeaderLock(ctx, s.lockCache, s.db, dashboardAggregationLeaderLockKey, s.instanceID, dashboardAggregationLeaderLockTTL)
	if !ok {
		return
	}
	defer release()

	now := time.Now().UTC()
	last, err := s.repo.GetAggregationWatermark(ctx)
	if err != nil {
		logger.LegacyPrintf("service.dashboard_aggregation", "[DashboardAggregation] 读取水位失败: %v", err)
		last = time.Unix(0, 0).UTC()
	}

	lookback := time.Duration(s.cfg.LookbackSeconds) * time.Second
	epoch := time.Unix(0, 0).UTC()
	start := last.Add(-lookback)
	if !last.After(epoch) {
		retentionDays := s.cfg.Retention.UsageLogsDays
		if retentionDays <= 0 {
			retentionDays = 1
		}
		start = truncateToDayUTC(now.AddDate(0, 0, -retentionDays))
	} else if start.After(now) {
		start = now.Add(-lookback)
	}

	if err := s.aggregateRange(ctx, start, now); err != nil {
		logger.LegacyPrintf("service.dashboard_aggregation", "[DashboardAggregation] 聚合失败: %v", err)
		return
	}

	updateErr := s.repo.UpdateAggregationWatermark(ctx, now)
	if updateErr != nil {
		logger.LegacyPrintf("service.dashboard_aggregation", "[DashboardAggregation] 更新水位失败: %v", updateErr)
	}
	slog.Debug("[DashboardAggregation] 聚合完成",
		"start", start.Format(time.RFC3339),
		"end", now.Format(time.RFC3339),
		"duration", time.Since(jobStart).String(),
		"watermark_updated", updateErr == nil,
	)

	s.maybeCleanupRetention(ctx, now)
}

func (s *DashboardAggregationService) backfillRange(ctx context.Context, start, end time.Time) error {
	if !atomic.CompareAndSwapInt32(&s.running, 0, 1) {
		return errDashboardAggregationRunning
	}
	defer atomic.StoreInt32(&s.running, 0)

	jobStart := time.Now().UTC()
	startUTC := start.UTC()
	endUTC := end.UTC()
	if !endUTC.After(startUTC) {
		return errors.New("回填时间范围无效")
	}

	cursor := truncateToDayUTC(startUTC)
	for cursor.Before(endUTC) {
		windowEnd := cursor.Add(24 * time.Hour)
		if windowEnd.After(endUTC) {
			windowEnd = endUTC
		}
		if err := s.aggregateRange(ctx, cursor, windowEnd); err != nil {
			return err
		}
		cursor = windowEnd
	}

	updateErr := s.repo.UpdateAggregationWatermark(ctx, endUTC)
	if updateErr != nil {
		logger.LegacyPrintf("service.dashboard_aggregation", "[DashboardAggregation] 更新水位失败: %v", updateErr)
	}
	logger.LegacyPrintf("service.dashboard_aggregation", "[DashboardAggregation] 回填聚合完成 (start=%s end=%s duration=%s watermark_updated=%t)",
		startUTC.Format(time.RFC3339),
		endUTC.Format(time.RFC3339),
		time.Since(jobStart).String(),
		updateErr == nil,
	)

	s.maybeCleanupRetention(ctx, endUTC)
	return nil
}

func (s *DashboardAggregationService) aggregateRange(ctx context.Context, start, end time.Time) error {
	if !end.After(start) {
		return nil
	}
	if err := s.repo.EnsureUsageLogsPartitions(ctx, end); err != nil {
		logger.LegacyPrintf("service.dashboard_aggregation", "[DashboardAggregation] 分区检查失败: %v", err)
	}
	return s.repo.AggregateRange(ctx, start, end)
}

func (s *DashboardAggregationService) maybeCleanupRetention(ctx context.Context, now time.Time) {
	lastAny := s.lastRetentionCleanup.Load()
	if lastAny != nil {
		if last, ok := lastAny.(time.Time); ok && now.Sub(last) < dashboardAggregationRetentionInterval {
			return
		}
	}

	hourlyCutoff := now.AddDate(0, 0, -s.cfg.Retention.HourlyDays)
	dailyCutoff := now.AddDate(0, 0, -s.cfg.Retention.DailyDays)
	usageCutoff := now.AddDate(0, 0, -s.cfg.Retention.UsageLogsDays)
	dedupCutoff := now.AddDate(0, 0, -s.cfg.Retention.UsageBillingDedupDays)

	aggErr := s.repo.CleanupAggregates(ctx, hourlyCutoff, dailyCutoff)
	if aggErr != nil {
		logger.LegacyPrintf("service.dashboard_aggregation", "[DashboardAggregation] 聚合保留清理失败: %v", aggErr)
	}
	usageErr := s.repo.CleanupUsageLogs(ctx, usageCutoff)
	if usageErr != nil {
		logger.LegacyPrintf("service.dashboard_aggregation", "[DashboardAggregation] usage_logs 保留清理失败: %v", usageErr)
	}
	dedupErr := s.repo.CleanupUsageBillingDedup(ctx, dedupCutoff)
	if dedupErr != nil {
		logger.LegacyPrintf("service.dashboard_aggregation", "[DashboardAggregation] usage_billing_dedup 保留清理失败: %v", dedupErr)
	}
	if aggErr == nil && usageErr == nil && dedupErr == nil {
		s.lastRetentionCleanup.Store(now)
	}
}

func truncateToDayUTC(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}
