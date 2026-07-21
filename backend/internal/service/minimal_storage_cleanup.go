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
	"github.com/lib/pq"
)

const (
	minimalStorageCleanupLockKey      int64 = 0x4d494e53544f5245 // "MINSTORE"
	minimalStorageDeleteBatch               = 5000
	minimalStorageMaxDeleteBatches          = 4
	minimalStorageLockTimeout               = 2 * time.Second
	minimalStorageSessionResetTimeout       = 5 * time.Second
	minimalStorageCleanupTimeout            = 30 * time.Minute
)

// minimalStorageOpsTables stay separate so cleanup can reuse the established
// OpsCleanup executor. Missing legacy tables are tolerated.
var minimalStorageOpsTables = []string{
	"ops_error_logs",
	"ops_retry_attempts",
	"ops_system_metrics",
	"ops_job_heartbeats",
	"ops_alert_events",
	"ops_metrics_hourly",
	"ops_metrics_daily",
	"ops_system_logs",
	"ops_system_log_cleanup_audits",
	"ops_ingress_reject_aggregates",
}

// minimalStorageTruncateTables contains non-ops history/observability tables
// that are not required to proxy, authenticate, schedule, rate-limit, or bill
// requests. They are truncated together without CASCADE so a future foreign
// key from a protected table fails closed instead of deleting core state.
var minimalStorageTruncateTables = []string{
	"usage_dashboard_hourly",
	"usage_dashboard_daily",
	"usage_dashboard_hourly_users",
	"usage_dashboard_daily_users",
	"usage_dashboard_aggregation_watermark",
	"usage_cleanup_tasks",
	"usage_billing_dedup_archive",
	"audit_logs",
	"content_moderation_logs",
	"deleted_api_key_audits",
	"scheduled_test_results",
	"channel_monitor_histories",
	"channel_monitor_daily_rollups",
	"channel_monitor_aggregation_watermark",
	"prompt_audit_events",
	"prompt_audit_jobs",
}

// minimalStorageProtectedTables is an executable safety contract. Keep this in
// sync with core proxy/auth/billing state if new tables are added in the future.
var minimalStorageProtectedTables = map[string]struct{}{
	"users":                      {},
	"api_keys":                   {},
	"accounts":                   {},
	"groups":                     {},
	"settings":                   {},
	"proxies":                    {},
	"subscription_plans":         {},
	"user_subscriptions":         {},
	"user_platform_quotas":       {},
	"payment_orders":             {},
	"payment_provider_instances": {},
	"payment_audit_logs":         {},
	"redeem_codes":               {},
}

type MinimalStorageCleanupResult struct {
	TruncatedTables        int
	DroppedUsagePartitions int
	DeletedUsageLogs       int64
	DeletedBillingKeys     int64
}

// MinimalStorageCleanupService performs bounded maintenance for minimal mode.
// TRUNCATE is deliberate: unlike mass DELETE it releases relation/index files
// without generating a WAL record for every historical row.
type MinimalStorageCleanupService struct {
	db        *sql.DB
	cfg       *config.Config
	cancel    context.CancelFunc
	done      chan struct{}
	startOnce sync.Once
	stopOnce  sync.Once
}

func NewMinimalStorageCleanupService(db *sql.DB, cfg *config.Config) *MinimalStorageCleanupService {
	return &MinimalStorageCleanupService{db: db, cfg: cfg, done: make(chan struct{})}
}

func ProvideMinimalStorageCleanupService(db *sql.DB, cfg *config.Config) *MinimalStorageCleanupService {
	svc := NewMinimalStorageCleanupService(db, cfg)
	svc.Start()
	return svc
}

func (s *MinimalStorageCleanupService) Start() {
	if s == nil || s.db == nil || s.cfg == nil || !s.cfg.MinimalStorageEnabled() {
		return
	}
	s.startOnce.Do(func() {
		ctx, cancel := context.WithCancel(context.Background())
		s.cancel = cancel
		go s.run(ctx)
	})
}

func (s *MinimalStorageCleanupService) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		if s.cancel == nil {
			return
		}
		s.cancel()
		<-s.done
	})
}

func (s *MinimalStorageCleanupService) run(ctx context.Context) {
	defer close(s.done)
	s.runAndLog(ctx)

	interval := time.Duration(s.cfg.Storage.CleanupIntervalMinutes) * time.Minute
	if interval <= 0 {
		interval = 6 * time.Hour
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runAndLog(ctx)
		}
	}
}

func (s *MinimalStorageCleanupService) runAndLog(parent context.Context) {
	ctx, cancel := context.WithTimeout(parent, minimalStorageCleanupTimeout)
	defer cancel()
	result, err := s.RunOnce(ctx)
	if err != nil {
		logger.LegacyPrintf("service.minimal_storage", "[MinimalStorage] cleanup failed: %v", err)
		return
	}
	logger.LegacyPrintf(
		"service.minimal_storage",
		"[MinimalStorage] cleanup complete: truncated_tables=%d dropped_usage_partitions=%d deleted_usage_logs=%d deleted_billing_keys=%d",
		result.TruncatedTables,
		result.DroppedUsagePartitions,
		result.DeletedUsageLogs,
		result.DeletedBillingKeys,
	)
}

func (s *MinimalStorageCleanupService) RunOnce(ctx context.Context) (MinimalStorageCleanupResult, error) {
	result := MinimalStorageCleanupResult{}
	if s == nil || s.db == nil || s.cfg == nil || !s.cfg.MinimalStorageEnabled() {
		return result, nil
	}

	conn, err := s.db.Conn(ctx)
	if err != nil {
		return result, err
	}
	defer func() { _ = conn.Close() }()

	var locked bool
	if err := conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", minimalStorageCleanupLockKey).Scan(&locked); err != nil {
		return result, err
	}
	if !locked {
		return result, nil
	}
	lockTimeoutSet := false
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), minimalStorageSessionResetTimeout)
		defer cancel()
		if lockTimeoutSet {
			_, _ = conn.ExecContext(cleanupCtx, "RESET lock_timeout")
		}
		_, _ = conn.ExecContext(cleanupCtx, "SELECT pg_advisory_unlock($1)", minimalStorageCleanupLockKey)
	}()

	lockTimeoutSQL := fmt.Sprintf("SET lock_timeout = '%dms'", minimalStorageLockTimeout.Milliseconds())
	if _, err := conn.ExecContext(ctx, lockTimeoutSQL); err != nil {
		return result, fmt.Errorf("set minimal storage lock timeout: %w", err)
	}
	lockTimeoutSet = true

	opsTruncated, err := truncateMinimalStorageOpsHistory(ctx, conn)
	if err != nil {
		return result, err
	}
	historyTruncated, err := truncateMinimalStorageHistory(ctx, conn)
	if err != nil {
		return result, err
	}
	result.TruncatedTables = opsTruncated + historyTruncated

	dropped, deleted, err := s.cleanupUsageLogs(ctx, conn)
	if err != nil {
		return result, err
	}
	result.DroppedUsagePartitions = dropped
	result.DeletedUsageLogs = deleted

	deleted, err = s.deleteExpiredBillingDedup(ctx, conn)
	if err != nil {
		return result, err
	}
	result.DeletedBillingKeys = deleted
	return result, nil
}

func existingMinimalStorageTables(ctx context.Context, conn *sql.Conn, tables []string) ([]string, error) {
	existing := make([]string, 0, len(tables))
	for _, table := range tables {
		var exists bool
		if err := conn.QueryRowContext(ctx, "SELECT to_regclass($1) IS NOT NULL", "public."+table).Scan(&exists); err != nil {
			return nil, err
		}
		if exists {
			existing = append(existing, table)
		}
	}
	return existing, nil
}

func truncateMinimalStorageOpsHistory(ctx context.Context, conn *sql.Conn) (int, error) {
	tables, err := existingMinimalStorageTables(ctx, conn, minimalStorageOpsTables)
	if err != nil {
		return 0, err
	}
	truncated := 0
	for _, table := range tables {
		rows, err := opsCleanupRunOne(ctx, conn, true, time.Time{}, table, "", false, 0)
		if err != nil {
			return truncated, fmt.Errorf("truncate minimal ops history %s: %w", table, err)
		}
		if rows > 0 {
			truncated++
		}
	}
	return truncated, nil
}

func truncateMinimalStorageHistory(ctx context.Context, conn *sql.Conn) (int, error) {
	tables, err := existingMinimalStorageTables(ctx, conn, minimalStorageTruncateTables)
	if err != nil || len(tables) == 0 {
		return 0, err
	}
	query, err := minimalStorageTruncateQuery(tables)
	if err != nil {
		return 0, err
	}
	if _, err := conn.ExecContext(ctx, query); err != nil {
		return 0, fmt.Errorf("truncate minimal storage history: %w", err)
	}
	return len(tables), nil
}

func minimalStorageTruncateQuery(tables []string) (string, error) {
	quoted := make([]string, 0, len(tables))
	for _, table := range tables {
		if _, protected := minimalStorageProtectedTables[table]; protected {
			return "", fmt.Errorf("refuse to truncate protected table %s", table)
		}
		quoted = append(quoted, pq.QuoteIdentifier("public")+"."+pq.QuoteIdentifier(table))
	}
	if len(quoted) == 0 {
		return "", nil
	}
	return "TRUNCATE TABLE " + strings.Join(quoted, ", "), nil
}

// cleanupUsageLogs keeps the smallest raw window that still covers all current
// scheduler and provider quota windows (including monthly Grok quotas). Fully
// expired monthly partitions are dropped to release disk immediately; the
// boundary partition/table is trimmed in small batches to avoid a WAL spike.
func (s *MinimalStorageCleanupService) cleanupUsageLogs(ctx context.Context, conn *sql.Conn) (int, int64, error) {
	var exists bool
	if err := conn.QueryRowContext(ctx, "SELECT to_regclass('public.usage_logs') IS NOT NULL").Scan(&exists); err != nil || !exists {
		return 0, 0, err
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -s.cfg.Storage.UsageRetentionDays)

	dropped, err := dropExpiredUsagePartitions(ctx, conn, cutoff)
	if err != nil {
		return 0, 0, err
	}

	var total int64
	for range minimalStorageMaxDeleteBatches {
		res, err := conn.ExecContext(ctx, `
WITH victims AS (
  SELECT tableoid AS relation_id, ctid AS row_id
  FROM usage_logs
  WHERE created_at < $1
  ORDER BY created_at, id
  LIMIT $2
)
DELETE FROM usage_logs AS usage
USING victims
WHERE usage.tableoid = victims.relation_id
  AND usage.ctid = victims.row_id
`, cutoff, minimalStorageDeleteBatch)
		if err != nil {
			return dropped, total, fmt.Errorf("delete expired usage logs: %w", err)
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return dropped, total, err
		}
		total += affected
		if affected < minimalStorageDeleteBatch {
			break
		}
	}
	return dropped, total, nil
}

func dropExpiredUsagePartitions(ctx context.Context, conn *sql.Conn, cutoff time.Time) (int, error) {
	rows, err := conn.QueryContext(ctx, `
SELECT child.relname
FROM pg_inherits
JOIN pg_class child ON child.oid = pg_inherits.inhrelid
JOIN pg_class parent ON parent.oid = pg_inherits.inhparent
JOIN pg_namespace namespace ON namespace.oid = parent.relnamespace
WHERE namespace.nspname = 'public'
  AND parent.relname = 'usage_logs'
`)
	if err != nil {
		return 0, fmt.Errorf("list usage log partitions: %w", err)
	}
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			_ = rows.Close()
			return 0, err
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return 0, err
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}

	dropped := 0
	for _, name := range names {
		const prefix = "usage_logs_"
		if len(name) != len(prefix)+6 || name[:len(prefix)] != prefix {
			continue
		}
		month, err := time.Parse("200601", name[len(prefix):])
		if err != nil || month.AddDate(0, 1, 0).After(cutoff) {
			continue
		}
		query := "DROP TABLE IF EXISTS " + pq.QuoteIdentifier("public") + "." + pq.QuoteIdentifier(name)
		if _, err := conn.ExecContext(ctx, query); err != nil {
			return dropped, fmt.Errorf("drop expired usage partition %s: %w", name, err)
		}
		dropped++
	}
	return dropped, nil
}

func (s *MinimalStorageCleanupService) deleteExpiredBillingDedup(ctx context.Context, conn *sql.Conn) (int64, error) {
	var exists bool
	if err := conn.QueryRowContext(ctx, "SELECT to_regclass('public.usage_billing_dedup') IS NOT NULL").Scan(&exists); err != nil || !exists {
		return 0, err
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -s.cfg.Storage.BillingDedupRetentionDays)
	var total int64
	for range minimalStorageMaxDeleteBatches {
		res, err := conn.ExecContext(ctx, `
WITH victims AS (
  SELECT id
  FROM usage_billing_dedup
  WHERE created_at < $1
  ORDER BY id
  LIMIT $2
)
DELETE FROM usage_billing_dedup
WHERE id IN (SELECT id FROM victims)
`, cutoff, minimalStorageDeleteBatch)
		if err != nil {
			return total, fmt.Errorf("delete expired usage billing dedup: %w", err)
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return total, err
		}
		total += affected
		if affected < minimalStorageDeleteBatch {
			return total, nil
		}
	}
	return total, nil
}
