package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/robfig/cron/v3"
)

const (
	opsCleanupJobName = "ops_cleanup"

	opsCleanupLeaderLockKeyDefault = "ops:cleanup:leader"
	opsCleanupLeaderLockTTLDefault = 30 * time.Minute
)

var opsCleanupCronParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

var opsCleanupReleaseScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("DEL", KEYS[1])
end
return 0
`)

// OpsCleanupService periodically deletes old ops data to prevent unbounded DB growth.
//
// - Scheduling: 5-field cron spec (minute hour dom month dow).
// - Multi-instance: best-effort Redis leader lock so only one node runs cleanup.
// - Safety: deletes in batches to avoid long transactions.
//
// 附带：在 runCleanupOnce 末尾调用 ChannelMonitorService.RunDailyMaintenance，
// 统一共享 cron schedule + leader lock + heartbeat，避免再引一套调度。
type OpsCleanupService struct {
	opsRepo           OpsRepository
	db                *sql.DB
	redisClient       *redis.Client
	cfg               *config.Config
	channelMonitorSvc *ChannelMonitorService
	settingRepo       SettingRepository

	instanceID string

	// mu 守护 cron 实例切换 + effective 配置切换。
	// 这里不再用 startOnce/stopOnce，是因为 Reload 需要"停旧 cron 重启新 cron"，
	// 而 Once 一旦触发就无法再次执行；改为 started/stopped 布尔配合 mu。
	mu        sync.Mutex
	cron      *cron.Cron
	started   bool
	stopped   bool
	effective config.OpsCleanupConfig

	warnNoRedisOnce sync.Once
}

func NewOpsCleanupService(
	opsRepo OpsRepository,
	db *sql.DB,
	redisClient *redis.Client,
	cfg *config.Config,
	channelMonitorSvc *ChannelMonitorService,
	settingRepo SettingRepository,
) *OpsCleanupService {
	return &OpsCleanupService{
		opsRepo:           opsRepo,
		db:                db,
		redisClient:       redisClient,
		cfg:               cfg,
		channelMonitorSvc: channelMonitorSvc,
		settingRepo:       settingRepo,
		instanceID:        uuid.NewString(),
	}
}

// Start 首次启动 cron 调度。Enabled / Schedule 由 effective 配置决定（settings 优先 cfg）。
// 重复调用幂等。
func (s *OpsCleanupService) Start() {
	if s == nil {
		return
	}
	if s.cfg != nil && !s.cfg.Ops.Enabled {
		return
	}
	if s.opsRepo == nil || s.db == nil {
		logger.LegacyPrintf("service.ops_cleanup", "[OpsCleanup] not started (missing deps)")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started || s.stopped {
		return
	}
	s.started = true
	if err := s.applyScheduleLocked(context.Background()); err != nil {
		logger.LegacyPrintf("service.ops_cleanup", "[OpsCleanup] not started: %v", err)
	}
}

// Stop 关闭 cron。幂等。
func (s *OpsCleanupService) Stop() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped {
		return
	}
	s.stopped = true
	s.stopCronLocked()
}

// stopCronLocked 停掉当前 cron 实例（带 3s 超时）。调用方持锁。
func (s *OpsCleanupService) stopCronLocked() {
	if s.cron == nil {
		return
	}
	ctx := s.cron.Stop()
	select {
	case <-ctx.Done():
	case <-time.After(3 * time.Second):
		logger.LegacyPrintf("service.ops_cleanup", "[OpsCleanup] cron stop timed out")
	}
	s.cron = nil
}

// applyScheduleLocked 重新计算 effective 配置并按其 schedule 重建 cron。调用方持锁。
// 若 effective.Enabled=false（用户在 UI 关闭清理），停旧 cron 后直接返回，不创建新 cron。
func (s *OpsCleanupService) applyScheduleLocked(ctx context.Context) error {
	s.computeEffectiveLocked(ctx)
	s.stopCronLocked()

	if !s.effective.Enabled {
		logger.LegacyPrintf("service.ops_cleanup", "[OpsCleanup] cron disabled by settings")
		return nil
	}

	schedule := strings.TrimSpace(s.effective.Schedule)
	if schedule == "" {
		schedule = "0 2 * * *"
	}

	loc := time.Local
	if s.cfg != nil && strings.TrimSpace(s.cfg.Timezone) != "" {
		if parsed, err := time.LoadLocation(strings.TrimSpace(s.cfg.Timezone)); err == nil && parsed != nil {
			loc = parsed
		}
	}

	c := cron.New(cron.WithParser(opsCleanupCronParser), cron.WithLocation(loc))
	if _, err := c.AddFunc(schedule, func() { s.runScheduled() }); err != nil {
		return fmt.Errorf("invalid schedule %q: %w", schedule, err)
	}
	c.Start()
	s.cron = c
	logger.LegacyPrintf("service.ops_cleanup",
		"[OpsCleanup] scheduled (schedule=%q tz=%s retention_days=err:%d/min:%d/hour:%d)",
		schedule, loc.String(),
		s.effective.ErrorLogRetentionDays,
		s.effective.MinuteMetricsRetentionDays,
		s.effective.HourlyMetricsRetentionDays,
	)
	return nil
}

// Reload 重新读取 ops_advanced_settings.data_retention 并按新配置重建 cron。
// 适用于 admin 在 UI 修改清理设置后立即生效（schedule / enabled 改动需要 Reload；
// retention 改动 runScheduled 顶部也会刷新，下一次触发即生效）。
// 若 service 还未 Start 或已 Stop，Reload 不做任何事。
func (s *OpsCleanupService) Reload(ctx context.Context) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.started || s.stopped {
		return nil
	}
	return s.applyScheduleLocked(ctx)
}

// computeEffectiveLocked 计算"生效配置"并写入 s.effective。调用方持锁。
//
// 优先级：UI 写入的 settings.ops_advanced_settings.data_retention（权威）覆盖 cfg.Ops.Cleanup 的副本。
//   - Enabled：settings 直接覆盖
//   - Schedule：settings 非空时覆盖，否则保留 cfg
//   - *RetentionDays：settings >=0 时覆盖（包括 0=TRUNCATE），<0 沿用 cfg
//
// 若 settings 表无该 key（ErrSettingNotFound）或解析失败，整体 fallback 到 cfg.Ops.Cleanup。
func (s *OpsCleanupService) computeEffectiveLocked(ctx context.Context) {
	base := config.OpsCleanupConfig{}
	if s.cfg != nil {
		base = s.cfg.Ops.Cleanup
	}
	defer func() { s.effective = base }()

	if s.settingRepo == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	raw, err := s.settingRepo.GetValue(ctx, SettingKeyOpsAdvancedSettings)
	if err != nil {
		if !errors.Is(err, ErrSettingNotFound) {
			logger.LegacyPrintf("service.ops_cleanup",
				"[OpsCleanup] read advanced settings failed, using cfg: %v", err)
		}
		return
	}
	var adv OpsAdvancedSettings
	if err := json.Unmarshal([]byte(raw), &adv); err != nil {
		logger.LegacyPrintf("service.ops_cleanup",
			"[OpsCleanup] parse advanced settings failed, using cfg: %v", err)
		return
	}
	dr := adv.DataRetention
	base.Enabled = dr.CleanupEnabled
	if sched := strings.TrimSpace(dr.CleanupSchedule); sched != "" {
		base.Schedule = sched
	}
	if dr.ErrorLogRetentionDays >= 0 {
		base.ErrorLogRetentionDays = dr.ErrorLogRetentionDays
	}
	if dr.MinuteMetricsRetentionDays >= 0 {
		base.MinuteMetricsRetentionDays = dr.MinuteMetricsRetentionDays
	}
	if dr.HourlyMetricsRetentionDays >= 0 {
		base.HourlyMetricsRetentionDays = dr.HourlyMetricsRetentionDays
	}
}

// snapshotEffective 取一份 effective 副本（runCleanupOnce 等读路径使用）。
func (s *OpsCleanupService) snapshotEffective() config.OpsCleanupConfig {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.effective
}

// refreshEffectiveBeforeRun 在 cron 触发时刷新 effective，让 retention 改动当次即生效。
// schedule 改动不影响当次（cron 调度由库管理，需要 Reload 才换 schedule）。
func (s *OpsCleanupService) refreshEffectiveBeforeRun(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.computeEffectiveLocked(ctx)
}

func (s *OpsCleanupService) runScheduled() {
	if s == nil || s.db == nil || s.opsRepo == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	// 让 retention 改动当次生效（schedule/enabled 改动需要 Reload）。
	s.refreshEffectiveBeforeRun(ctx)

	release, ok := s.tryAcquireLeaderLock(ctx)
	if !ok {
		return
	}
	if release != nil {
		defer release()
	}

	startedAt := time.Now().UTC()
	runAt := startedAt

	counts, err := s.runCleanupOnce(ctx)
	if err != nil {
		s.recordHeartbeatError(runAt, time.Since(startedAt), err)
		logger.LegacyPrintf("service.ops_cleanup", "[OpsCleanup] cleanup failed: %v", err)
		return
	}
	s.recordHeartbeatSuccess(runAt, time.Since(startedAt), counts)
	logger.LegacyPrintf("service.ops_cleanup", "[OpsCleanup] cleanup complete: %s", counts)
}

type opsCleanupDeletedCounts struct {
	errorLogs     int64
	retryAttempts int64
	alertEvents   int64
	systemLogs    int64
	logAudits     int64
	systemMetrics int64
	hourlyPreagg  int64
	dailyPreagg   int64
}

func (c opsCleanupDeletedCounts) String() string {
	return fmt.Sprintf(
		"error_logs=%d retry_attempts=%d alert_events=%d system_logs=%d log_audits=%d system_metrics=%d hourly_preagg=%d daily_preagg=%d",
		c.errorLogs,
		c.retryAttempts,
		c.alertEvents,
		c.systemLogs,
		c.logAudits,
		c.systemMetrics,
		c.hourlyPreagg,
		c.dailyPreagg,
	)
}

// opsCleanupPlan 把"保留天数"翻译成具体的清理动作。
//   - days < 0  → 跳过该项清理（ok=false），保留兼容老数据
//   - days == 0 → TRUNCATE TABLE（O(1) 全清），truncate=true
//   - days > 0  → 批量 DELETE 早于 now-N天 的行，cutoff = now - N 天
//
// 之所以 days==0 走 TRUNCATE 而非"now+24h cutoff + DELETE"：
//   - 速度从 O(N) 降到 O(1)，对百万行级表毫秒完成
//   - 无 WAL 写入、无后续 VACUUM 压力
//   - 这些 ops 表只有 cleanup 任务自己写，TRUNCATE 的 ACCESS EXCLUSIVE 锁影响可忽略
func opsCleanupPlan(now time.Time, days int) (cutoff time.Time, truncate, ok bool) {
	if days < 0 {
		return time.Time{}, false, false
	}
	if days == 0 {
		return time.Time{}, true, true
	}
	return now.AddDate(0, 0, -days), false, true
}

func (s *OpsCleanupService) runCleanupOnce(ctx context.Context) (opsCleanupDeletedCounts, error) {
	out := opsCleanupDeletedCounts{}
	if s == nil || s.db == nil || s.cfg == nil {
		return out, nil
	}

	effective := s.snapshotEffective()

	batchSize := 5000

	now := time.Now().UTC()

	// runOne 把"truncate? cutoff? batched delete?"封装到一处，
	// 让三组清理（错误日志类 / 分钟指标 / 小时+日预聚合）调用方只关心表名和列名。
	runOne := func(truncate bool, cutoff time.Time, table, timeCol string, castDate bool) (int64, error) {
		if truncate {
			return truncateOpsTable(ctx, s.db, table)
		}
		return deleteOldRowsByID(ctx, s.db, table, timeCol, cutoff, batchSize, castDate)
	}

	// Error-like tables: error logs / retry attempts / alert events / system logs / cleanup audits.
	if cutoff, truncate, ok := opsCleanupPlan(now, effective.ErrorLogRetentionDays); ok {
		n, err := runOne(truncate, cutoff, "ops_error_logs", "created_at", false)
		if err != nil {
			return out, err
		}
		out.errorLogs = n

		n, err = runOne(truncate, cutoff, "ops_retry_attempts", "created_at", false)
		if err != nil {
			return out, err
		}
		out.retryAttempts = n

		n, err = runOne(truncate, cutoff, "ops_alert_events", "created_at", false)
		if err != nil {
			return out, err
		}
		out.alertEvents = n

		n, err = runOne(truncate, cutoff, "ops_system_logs", "created_at", false)
		if err != nil {
			return out, err
		}
		out.systemLogs = n

		n, err = runOne(truncate, cutoff, "ops_system_log_cleanup_audits", "created_at", false)
		if err != nil {
			return out, err
		}
		out.logAudits = n
	}

	// Minute-level metrics snapshots.
	if cutoff, truncate, ok := opsCleanupPlan(now, effective.MinuteMetricsRetentionDays); ok {
		n, err := runOne(truncate, cutoff, "ops_system_metrics", "created_at", false)
		if err != nil {
			return out, err
		}
		out.systemMetrics = n
	}

	// Pre-aggregation tables (hourly/daily).
	if cutoff, truncate, ok := opsCleanupPlan(now, effective.HourlyMetricsRetentionDays); ok {
		n, err := runOne(truncate, cutoff, "ops_metrics_hourly", "bucket_start", false)
		if err != nil {
			return out, err
		}
		out.hourlyPreagg = n

		n, err = runOne(truncate, cutoff, "ops_metrics_daily", "bucket_date", true)
		if err != nil {
			return out, err
		}
		out.dailyPreagg = n
	}

	// Channel monitor 每日维护（聚合昨日明细 + 软删过期明细/聚合）。
	// 失败只记日志，不影响 ops 清理的成功状态（与 ops 各步骤风格一致）；
	// 维护本身已经把每步错误打到 slog，heartbeat result 不再分项记录。
	if s.channelMonitorSvc != nil {
		if err := s.channelMonitorSvc.RunDailyMaintenance(ctx); err != nil {
			logger.LegacyPrintf("service.ops_cleanup", "[OpsCleanup] channel monitor maintenance failed: %v", err)
		}
	}

	return out, nil
}

func deleteOldRowsByID(
	ctx context.Context,
	db *sql.DB,
	table string,
	timeColumn string,
	cutoff time.Time,
	batchSize int,
	castCutoffToDate bool,
) (int64, error) {
	if db == nil {
		return 0, nil
	}
	if batchSize <= 0 {
		batchSize = 5000
	}

	where := fmt.Sprintf("%s < $1", timeColumn)
	if castCutoffToDate {
		where = fmt.Sprintf("%s < $1::date", timeColumn)
	}

	q := fmt.Sprintf(`
WITH batch AS (
  SELECT id FROM %s
  WHERE %s
  ORDER BY id
  LIMIT $2
)
DELETE FROM %s
WHERE id IN (SELECT id FROM batch)
`, table, where, table)

	var total int64
	for {
		res, err := db.ExecContext(ctx, q, cutoff, batchSize)
		if err != nil {
			// If ops tables aren't present yet (partial deployments), treat as no-op.
			if isMissingRelationError(err) {
				return total, nil
			}
			return total, err
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return total, err
		}
		total += affected
		if affected == 0 {
			break
		}
	}
	return total, nil
}

// truncateOpsTable 用 TRUNCATE TABLE 清空指定表，先 SELECT COUNT(*) 取得清空前行数用于 heartbeat。
//
// 与 deleteOldRowsByID 的差异：
//   - 不可指定 WHERE 条件，仅用于 days==0 的"清空全部"语义
//   - O(1) 释放表的物理存储页，毫秒级完成，无 WAL 写入、无 VACUUM 压力
//   - 需要 ACCESS EXCLUSIVE 锁，但 ops 表只有清理任务自己写入，瞬间锁影响可忽略
//
// 表不存在（部分部署）静默返回 0，与 deleteOldRowsByID 保持一致。
func truncateOpsTable(ctx context.Context, db *sql.DB, table string) (int64, error) {
	if db == nil {
		return 0, nil
	}
	var count int64
	if err := db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", table)).Scan(&count); err != nil {
		if isMissingRelationError(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("count %s: %w", table, err)
	}
	if count == 0 {
		return 0, nil
	}
	if _, err := db.ExecContext(ctx, fmt.Sprintf("TRUNCATE TABLE %s", table)); err != nil {
		if isMissingRelationError(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("truncate %s: %w", table, err)
	}
	return count, nil
}

// isMissingRelationError 判断 PG 报错是否为"表不存在"，用于让清理任务在部分部署场景静默跳过。
func isMissingRelationError(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "does not exist") && strings.Contains(s, "relation")
}

func (s *OpsCleanupService) tryAcquireLeaderLock(ctx context.Context) (func(), bool) {
	if s == nil {
		return nil, false
	}
	// In simple run mode, assume single instance.
	if s.cfg != nil && s.cfg.RunMode == config.RunModeSimple {
		return nil, true
	}

	key := opsCleanupLeaderLockKeyDefault
	ttl := opsCleanupLeaderLockTTLDefault

	// Prefer Redis leader lock when available, but avoid stampeding the DB when Redis is flaky by
	// falling back to a DB advisory lock.
	if s.redisClient != nil {
		ok, err := s.redisClient.SetNX(ctx, key, s.instanceID, ttl).Result()
		if err == nil {
			if !ok {
				return nil, false
			}
			return func() {
				_, _ = opsCleanupReleaseScript.Run(ctx, s.redisClient, []string{key}, s.instanceID).Result()
			}, true
		}
		// Redis error: fall back to DB advisory lock.
		s.warnNoRedisOnce.Do(func() {
			logger.LegacyPrintf("service.ops_cleanup", "[OpsCleanup] leader lock SetNX failed; falling back to DB advisory lock: %v", err)
		})
	} else {
		s.warnNoRedisOnce.Do(func() {
			logger.LegacyPrintf("service.ops_cleanup", "[OpsCleanup] redis not configured; using DB advisory lock")
		})
	}

	release, ok := tryAcquireDBAdvisoryLock(ctx, s.db, hashAdvisoryLockID(key))
	if !ok {
		return nil, false
	}
	return release, true
}

func (s *OpsCleanupService) recordHeartbeatSuccess(runAt time.Time, duration time.Duration, counts opsCleanupDeletedCounts) {
	if s == nil || s.opsRepo == nil {
		return
	}
	now := time.Now().UTC()
	durMs := duration.Milliseconds()
	result := truncateString(counts.String(), 2048)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = s.opsRepo.UpsertJobHeartbeat(ctx, &OpsUpsertJobHeartbeatInput{
		JobName:        opsCleanupJobName,
		LastRunAt:      &runAt,
		LastSuccessAt:  &now,
		LastDurationMs: &durMs,
		LastResult:     &result,
	})
}

func (s *OpsCleanupService) recordHeartbeatError(runAt time.Time, duration time.Duration, err error) {
	if s == nil || s.opsRepo == nil || err == nil {
		return
	}
	now := time.Now().UTC()
	durMs := duration.Milliseconds()
	msg := truncateString(err.Error(), 2048)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = s.opsRepo.UpsertJobHeartbeat(ctx, &OpsUpsertJobHeartbeatInput{
		JobName:        opsCleanupJobName,
		LastRunAt:      &runAt,
		LastErrorAt:    &now,
		LastError:      &msg,
		LastDurationMs: &durMs,
	})
}
