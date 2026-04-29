package service

import (
	"context"
	"database/sql"
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

	instanceID string

	cron *cron.Cron

	startOnce sync.Once
	stopOnce  sync.Once

	warnNoRedisOnce sync.Once
}

func NewOpsCleanupService(
	opsRepo OpsRepository,
	db *sql.DB,
	redisClient *redis.Client,
	cfg *config.Config,
	channelMonitorSvc *ChannelMonitorService,
) *OpsCleanupService {
	return &OpsCleanupService{
		opsRepo:           opsRepo,
		db:                db,
		redisClient:       redisClient,
		cfg:               cfg,
		channelMonitorSvc: channelMonitorSvc,
		instanceID:        uuid.NewString(),
	}
}

func (s *OpsCleanupService) Start() {
	if s == nil {
		return
	}
	if s.cfg != nil && !s.cfg.Ops.Enabled {
		return
	}
	if s.cfg != nil && !s.cfg.Ops.Cleanup.Enabled {
		logger.LegacyPrintf("service.ops_cleanup", "[OpsCleanup] not started (disabled)")
		return
	}
	if s.opsRepo == nil || s.db == nil {
		logger.LegacyPrintf("service.ops_cleanup", "[OpsCleanup] not started (missing deps)")
		return
	}

	s.startOnce.Do(func() {
		schedule := "0 2 * * *"
		if s.cfg != nil && strings.TrimSpace(s.cfg.Ops.Cleanup.Schedule) != "" {
			schedule = strings.TrimSpace(s.cfg.Ops.Cleanup.Schedule)
		}

		loc := time.Local
		if s.cfg != nil && strings.TrimSpace(s.cfg.Timezone) != "" {
			if parsed, err := time.LoadLocation(strings.TrimSpace(s.cfg.Timezone)); err == nil && parsed != nil {
				loc = parsed
			}
		}

		c := cron.New(cron.WithParser(opsCleanupCronParser), cron.WithLocation(loc))
		_, err := c.AddFunc(schedule, func() { s.runScheduled() })
		if err != nil {
			logger.LegacyPrintf("service.ops_cleanup", "[OpsCleanup] not started (invalid schedule=%q): %v", schedule, err)
			return
		}
		s.cron = c
		s.cron.Start()
		logger.LegacyPrintf("service.ops_cleanup", "[OpsCleanup] started (schedule=%q tz=%s)", schedule, loc.String())
	})
}

func (s *OpsCleanupService) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		if s.cron != nil {
			ctx := s.cron.Stop()
			select {
			case <-ctx.Done():
			case <-time.After(3 * time.Second):
				logger.LegacyPrintf("service.ops_cleanup", "[OpsCleanup] cron stop timed out")
			}
		}
	})
}

func (s *OpsCleanupService) runScheduled() {
	if s == nil || s.db == nil || s.opsRepo == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

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
	if cutoff, truncate, ok := opsCleanupPlan(now, s.cfg.Ops.Cleanup.ErrorLogRetentionDays); ok {
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
	if cutoff, truncate, ok := opsCleanupPlan(now, s.cfg.Ops.Cleanup.MinuteMetricsRetentionDays); ok {
		n, err := runOne(truncate, cutoff, "ops_system_metrics", "created_at", false)
		if err != nil {
			return out, err
		}
		out.systemMetrics = n
	}

	// Pre-aggregation tables (hourly/daily).
	if cutoff, truncate, ok := opsCleanupPlan(now, s.cfg.Ops.Cleanup.HourlyMetricsRetentionDays); ok {
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
