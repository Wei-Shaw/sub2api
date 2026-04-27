package service

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/leaderlock"
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

// OpsCleanupService periodically deletes old ops data to prevent unbounded DB growth.
//
// - Scheduling: 5-field cron spec (minute hour dom month dow).
// - Multi-instance: best-effort Redis leader lock so only one node runs cleanup.
// - Safety: deletes in batches to avoid long transactions.
type OpsCleanupService struct {
	opsRepo     OpsRepository
	db          *sql.DB
	redisClient *redis.Client
	cfg         *config.Config

	instanceID string

	cron *cron.Cron

	startOnce sync.Once
	stopOnce  sync.Once
}

func NewOpsCleanupService(
	opsRepo OpsRepository,
	db *sql.DB,
	redisClient *redis.Client,
	cfg *config.Config,
) *OpsCleanupService {
	return &OpsCleanupService{
		opsRepo:     opsRepo,
		db:          db,
		redisClient: redisClient,
		cfg:         cfg,
		instanceID:  uuid.NewString(),
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

func (s *OpsCleanupService) runCleanupOnce(ctx context.Context) (opsCleanupDeletedCounts, error) {
	out := opsCleanupDeletedCounts{}
	if s == nil || s.db == nil || s.cfg == nil {
		return out, nil
	}

	batchSize := 5000

	now := time.Now().UTC()

	// Error-like tables: error logs / retry attempts / alert events.
	if days := s.cfg.Ops.Cleanup.ErrorLogRetentionDays; days > 0 {
		cutoff := now.AddDate(0, 0, -days)
		n, err := deleteOldRowsByID(ctx, s.db, "ops_error_logs", "created_at", cutoff, batchSize, false)
		if err != nil {
			return out, err
		}
		out.errorLogs = n

		n, err = deleteOldRowsByID(ctx, s.db, "ops_retry_attempts", "created_at", cutoff, batchSize, false)
		if err != nil {
			return out, err
		}
		out.retryAttempts = n

		n, err = deleteOldRowsByID(ctx, s.db, "ops_alert_events", "created_at", cutoff, batchSize, false)
		if err != nil {
			return out, err
		}
		out.alertEvents = n

		n, err = deleteOldRowsByID(ctx, s.db, "ops_system_logs", "created_at", cutoff, batchSize, false)
		if err != nil {
			return out, err
		}
		out.systemLogs = n

		n, err = deleteOldRowsByID(ctx, s.db, "ops_system_log_cleanup_audits", "created_at", cutoff, batchSize, false)
		if err != nil {
			return out, err
		}
		out.logAudits = n
	}

	// Minute-level metrics snapshots.
	if days := s.cfg.Ops.Cleanup.MinuteMetricsRetentionDays; days > 0 {
		cutoff := now.AddDate(0, 0, -days)
		n, err := deleteOldRowsByID(ctx, s.db, "ops_system_metrics", "created_at", cutoff, batchSize, false)
		if err != nil {
			return out, err
		}
		out.systemMetrics = n
	}

	// Pre-aggregation tables (hourly/daily).
	if days := s.cfg.Ops.Cleanup.HourlyMetricsRetentionDays; days > 0 {
		cutoff := now.AddDate(0, 0, -days)
		n, err := deleteOldRowsByID(ctx, s.db, "ops_metrics_hourly", "bucket_start", cutoff, batchSize, false)
		if err != nil {
			return out, err
		}
		out.hourlyPreagg = n

		n, err = deleteOldRowsByID(ctx, s.db, "ops_metrics_daily", "bucket_date", cutoff, batchSize, true)
		if err != nil {
			return out, err
		}
		out.dailyPreagg = n
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
			if strings.Contains(strings.ToLower(err.Error()), "does not exist") && strings.Contains(strings.ToLower(err.Error()), "relation") {
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

func (s *OpsCleanupService) tryAcquireLeaderLock(ctx context.Context) (func(), bool) {
	if s == nil {
		return nil, false
	}
	// Delegate to the shared leaderlock helper so the plugin JobScheduler
	// (V5 W2) can reuse the same Redis-first / DB-fallback semantics without
	// pulling in internal/service.
	return s.leaderLockProvider().TryAcquire(ctx, opsCleanupLeaderLockKeyDefault)
}

// leaderLockProvider lazily constructs the underlying provider. Kept as a
// method (rather than a struct field initialised in NewOpsCleanupService) to
// avoid disturbing existing callers and tests that construct the service
// directly without going through wire.
func (s *OpsCleanupService) leaderLockProvider() leaderlock.Provider {
	simple := s.cfg != nil && s.cfg.RunMode == config.RunModeSimple
	return leaderlock.New(s.redisClient, s.db, leaderlock.Config{
		InstanceID:     s.instanceID,
		TTL:            opsCleanupLeaderLockTTLDefault,
		SingleInstance: simple,
		Logger:         slog.Default().With("component", "ops_cleanup_leader_lock"),
	})
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
