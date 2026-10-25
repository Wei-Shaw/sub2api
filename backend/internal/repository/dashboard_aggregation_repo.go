package repository

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

type dashboardAggregationRepository struct {
	sql   sqlExecutor
	clock func() time.Time
}

const usageLogsCleanupBatchSize = 10000
const usageBillingDedupCleanupBatchSize = 10000

// NewDashboardAggregationRepository 创建仪表盘预聚合仓储。
func NewDashboardAggregationRepository(sqlDB *sql.DB) service.DashboardAggregationRepository {
	if sqlDB == nil {
		return nil
	}
	if !isPostgresDriver(sqlDB) {
		log.Printf("[DashboardAggregation] 检测到非 PostgreSQL 驱动，已自动禁用预聚合")
		return nil
	}
	return newDashboardAggregationRepositoryWithSQL(sqlDB)
}

func newDashboardAggregationRepositoryWithSQL(sqlq sqlExecutor) *dashboardAggregationRepository {
	return &dashboardAggregationRepository{sql: sqlq, clock: time.Now}
}

func (r *dashboardAggregationRepository) now() time.Time {
	if r.clock != nil {
		return r.clock()
	}
	return time.Now()
}

func isPostgresDriver(db *sql.DB) bool {
	if db == nil {
		return false
	}
	_, ok := db.Driver().(*pq.Driver)
	return ok
}

func (r *dashboardAggregationRepository) AggregateRange(ctx context.Context, start, end time.Time) error {
	if r == nil || r.sql == nil {
		return nil
	}
	loc := timezone.Location()
	startLocal := start.In(loc)
	endLocal := end.In(loc)
	if !endLocal.After(startLocal) {
		return nil
	}

	hourStart := startLocal.Truncate(time.Hour)
	hourEnd := endLocal.Truncate(time.Hour)
	if endLocal.After(hourEnd) {
		hourEnd = hourEnd.Add(time.Hour)
	}

	dayStart := truncateToDay(startLocal)
	dayEnd := truncateToDay(endLocal)
	if endLocal.After(dayEnd) {
		dayEnd = dayEnd.Add(24 * time.Hour)
	}

	if db, ok := r.sql.(*sql.DB); ok {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		txRepo := newDashboardAggregationRepositoryWithSQL(tx)
		if err := txRepo.aggregateRangeInTx(ctx, hourStart, hourEnd, dayStart, dayEnd); err != nil {
			_ = tx.Rollback()
			return err
		}
		return tx.Commit()
	}
	return r.aggregateRangeInTx(ctx, hourStart, hourEnd, dayStart, dayEnd)
}

func (r *dashboardAggregationRepository) aggregateRangeInTx(ctx context.Context, hourStart, hourEnd, dayStart, dayEnd time.Time) error {
	// 以桶边界聚合，允许覆盖 end 所在桶的剩余区间。
	if err := r.insertHourlyActiveUsers(ctx, hourStart, hourEnd); err != nil {
		return err
	}
	if err := r.insertDailyActiveUsers(ctx, hourStart, hourEnd); err != nil {
		return err
	}
	if err := r.upsertHourlyAggregates(ctx, hourStart, hourEnd); err != nil {
		return err
	}
	if err := r.upsertDailyAggregates(ctx, dayStart, dayEnd); err != nil {
		return err
	}
	return nil
}

func (r *dashboardAggregationRepository) RecomputeRange(ctx context.Context, start, end time.Time) error {
	if r == nil || r.sql == nil {
		return nil
	}
	loc := timezone.Location()
	startLocal := start.In(loc)
	endLocal := end.In(loc)
	if !endLocal.After(startLocal) {
		return nil
	}

	hourStart := startLocal.Truncate(time.Hour)
	hourEnd := endLocal.Truncate(time.Hour)
	if endLocal.After(hourEnd) {
		hourEnd = hourEnd.Add(time.Hour)
	}

	dayStart := truncateToDay(startLocal)
	dayEnd := truncateToDay(endLocal)
	if endLocal.After(dayEnd) {
		dayEnd = dayEnd.Add(24 * time.Hour)
	}

	// 尽量使用事务保证范围内的一致性（允许在非 *sql.DB 的情况下退化为非事务执行）。
	if db, ok := r.sql.(*sql.DB); ok {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if err := lockGroupUsageRollupState(ctx, tx); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := invalidateGroupUsageRollupsAt(ctx, tx, start); err != nil {
			_ = tx.Rollback()
			return err
		}
		txRepo := newDashboardAggregationRepositoryWithSQL(tx)
		if err := txRepo.recomputeRangeInTx(ctx, hourStart, hourEnd, dayStart, dayEnd); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := txRepo.syncGroupUsageRollupsInTx(ctx, service.GroupUsageTodayStart(r.now())); err != nil {
			_ = tx.Rollback()
			return err
		}
		return tx.Commit()
	}
	return r.recomputeRangeInTx(ctx, hourStart, hourEnd, dayStart, dayEnd)
}

func (r *dashboardAggregationRepository) recomputeRangeInTx(ctx context.Context, hourStart, hourEnd, dayStart, dayEnd time.Time) error {
	if IsMySQL() {
		return r.recomputeRangeInTxMySQL(ctx, hourStart, hourEnd, dayStart, dayEnd)
	}
	// 先清空范围内桶，再重建（避免仅增量插入导致活跃用户等指标无法回退）。
	if _, err := r.sql.ExecContext(ctx, "DELETE FROM usage_dashboard_hourly WHERE bucket_start >= $1 AND bucket_start < $2", hourStart, hourEnd); err != nil {
		return err
	}
	if _, err := r.sql.ExecContext(ctx, "DELETE FROM usage_dashboard_hourly_users WHERE bucket_start >= $1 AND bucket_start < $2", hourStart, hourEnd); err != nil {
		return err
	}
	if _, err := r.sql.ExecContext(ctx, "DELETE FROM usage_dashboard_daily WHERE bucket_date >= $1::date AND bucket_date < $2::date", dayStart, dayEnd); err != nil {
		return err
	}
	if _, err := r.sql.ExecContext(ctx, "DELETE FROM usage_dashboard_daily_users WHERE bucket_date >= $1::date AND bucket_date < $2::date", dayStart, dayEnd); err != nil {
		return err
	}

	if err := r.insertHourlyActiveUsers(ctx, hourStart, hourEnd); err != nil {
		return err
	}
	if err := r.insertDailyActiveUsers(ctx, hourStart, hourEnd); err != nil {
		return err
	}
	if err := r.upsertHourlyAggregates(ctx, hourStart, hourEnd); err != nil {
		return err
	}
	if err := r.upsertDailyAggregates(ctx, dayStart, dayEnd); err != nil {
		return err
	}
	return nil
}

func (r *dashboardAggregationRepository) GetAggregationWatermark(ctx context.Context) (time.Time, error) {
	var ts time.Time
	query := "SELECT last_aggregated_at FROM usage_dashboard_aggregation_watermark WHERE id = 1"
	if err := scanSingleRow(ctx, r.sql, query, nil, &ts); err != nil {
		if err == sql.ErrNoRows {
			return time.Unix(0, 0).UTC(), nil
		}
		return time.Time{}, err
	}
	return ts.UTC(), nil
}

func (r *dashboardAggregationRepository) UpdateAggregationWatermark(ctx context.Context, aggregatedAt time.Time) error {
	if IsMySQL() {
		_, err := r.sql.ExecContext(ctx, `
			INSERT INTO usage_dashboard_aggregation_watermark (id, last_aggregated_at, updated_at)
			VALUES (1, ?, NOW(3)) AS new_watermark
			ON DUPLICATE KEY UPDATE
				last_aggregated_at = new_watermark.last_aggregated_at,
				updated_at = new_watermark.updated_at
		`, aggregatedAt.UTC())
		return err
	}
	query := `
		INSERT INTO usage_dashboard_aggregation_watermark (id, last_aggregated_at, updated_at)
		VALUES (1, $1, NOW())
		ON CONFLICT (id)
		DO UPDATE SET last_aggregated_at = EXCLUDED.last_aggregated_at, updated_at = EXCLUDED.updated_at
	`
	_, err := r.sql.ExecContext(ctx, query, aggregatedAt.UTC())
	return err
}

func (r *dashboardAggregationRepository) CleanupAggregates(ctx context.Context, hourlyCutoff, dailyCutoff time.Time) error {
	hourlyCutoffUTC := hourlyCutoff.UTC()
	dailyCutoffUTC := dailyCutoff.UTC()
	if IsMySQL() {
		// 与 PG 版逐语句对应：$n→?；DATE 列与 DATETIME 字面量比较无需 ::date 显式转型。
		if _, err := r.sql.ExecContext(ctx, "DELETE FROM usage_dashboard_hourly WHERE bucket_start < ?", hourlyCutoffUTC); err != nil {
			return err
		}
		if _, err := r.sql.ExecContext(ctx, "DELETE FROM usage_dashboard_hourly_users WHERE bucket_start < ?", hourlyCutoffUTC); err != nil {
			return err
		}
		if _, err := r.sql.ExecContext(ctx, "DELETE FROM usage_dashboard_daily WHERE bucket_date < ?", dailyCutoffUTC); err != nil {
			return err
		}
		if _, err := r.sql.ExecContext(ctx, "DELETE FROM usage_dashboard_daily_users WHERE bucket_date < ?", dailyCutoffUTC); err != nil {
			return err
		}
		return nil
	}
	if _, err := r.sql.ExecContext(ctx, "DELETE FROM usage_dashboard_hourly WHERE bucket_start < $1", hourlyCutoffUTC); err != nil {
		return err
	}
	if _, err := r.sql.ExecContext(ctx, "DELETE FROM usage_dashboard_hourly_users WHERE bucket_start < $1", hourlyCutoffUTC); err != nil {
		return err
	}
	if _, err := r.sql.ExecContext(ctx, "DELETE FROM usage_dashboard_daily WHERE bucket_date < $1::date", dailyCutoffUTC); err != nil {
		return err
	}
	if _, err := r.sql.ExecContext(ctx, "DELETE FROM usage_dashboard_daily_users WHERE bucket_date < $1::date", dailyCutoffUTC); err != nil {
		return err
	}
	return nil
}

func (r *dashboardAggregationRepository) CleanupUsageLogs(ctx context.Context, cutoff time.Time) error {
	isPartitioned, err := r.isUsageLogsPartitioned(ctx)
	if err != nil {
		return err
	}
	if isPartitioned {
		if err := r.dropUsageLogsPartitions(ctx, cutoff); err != nil {
			return err
		}
	}
	// Remove the expired part of the boundary partition as well, so a retention
	// window measured in days does not silently round up to a whole month.
	if err := r.cleanupUsageLogsBatches(ctx, cutoff); err != nil {
		return err
	}
	return r.SyncGroupUsageRollups(ctx, service.GroupUsageTodayStart(r.now()))
}

func (r *dashboardAggregationRepository) cleanupUsageLogsBatches(ctx context.Context, cutoff time.Time) error {
	if IsMySQL() {
		return r.cleanupUsageLogsBatchesMySQL(ctx, cutoff)
	}
	db, transactional := r.sql.(*sql.DB)
	for {
		if transactional {
			affected, err := cleanupUsageLogsBatchWithRollupInvalidation(ctx, db, cutoff)
			if err != nil {
				return err
			}
			if affected < usageLogsCleanupBatchSize {
				return nil
			}
			continue
		}

		res, err := r.sql.ExecContext(ctx, `
			WITH victims AS (
				SELECT tableoid, ctid
				FROM usage_logs
				WHERE created_at < $1
				ORDER BY created_at ASC, id ASC
				LIMIT $2
			)
			DELETE FROM usage_logs
			WHERE (tableoid, ctid) IN (SELECT tableoid, ctid FROM victims)
		`, cutoff.UTC(), usageLogsCleanupBatchSize)
		if err != nil {
			return err
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if affected < usageLogsCleanupBatchSize {
			return nil
		}
	}
}

func cleanupUsageLogsBatchWithRollupInvalidation(ctx context.Context, db *sql.DB, cutoff time.Time) (int64, error) {
	if IsMySQL() {
		return cleanupUsageLogsBatchWithRollupInvalidationMySQL(ctx, db, cutoff)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	rollback := func(err error) (int64, error) {
		_ = tx.Rollback()
		return 0, err
	}

	if err := lockGroupUsageRollupState(ctx, tx); err != nil {
		return rollback(err)
	}
	rows, err := tx.QueryContext(ctx, `
		WITH victims AS (
			SELECT tableoid, ctid
			FROM usage_logs
			WHERE created_at < $1
			ORDER BY created_at ASC, id ASC
			LIMIT $2
		)
		DELETE FROM usage_logs
		WHERE (tableoid, ctid) IN (SELECT tableoid, ctid FROM victims)
		RETURNING created_at
	`, cutoff.UTC(), usageLogsCleanupBatchSize)
	if err != nil {
		return rollback(err)
	}

	var affected int64
	var earliestDeletedAt time.Time
	for rows.Next() {
		var deletedAt time.Time
		if err := rows.Scan(&deletedAt); err != nil {
			_ = rows.Close()
			return rollback(err)
		}
		affected++
		if earliestDeletedAt.IsZero() || deletedAt.Before(earliestDeletedAt) {
			earliestDeletedAt = deletedAt
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return rollback(err)
	}
	if err := rows.Close(); err != nil {
		return rollback(err)
	}
	if affected > 0 {
		if err := invalidateGroupUsageRollupsAt(ctx, tx, earliestDeletedAt); err != nil {
			return rollback(err)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return affected, nil
}

func (r *dashboardAggregationRepository) CleanupUsageBillingDedup(ctx context.Context, cutoff time.Time) error {
	if IsMySQL() {
		return r.cleanupUsageBillingDedupMySQL(ctx, cutoff)
	}
	for {
		res, err := r.sql.ExecContext(ctx, `
			WITH victims AS (
				SELECT ctid, request_id, api_key_id, request_fingerprint, created_at
				FROM usage_billing_dedup
				WHERE created_at < $1
				LIMIT $2
			), archived AS (
				INSERT INTO usage_billing_dedup_archive (request_id, api_key_id, request_fingerprint, created_at)
				SELECT request_id, api_key_id, request_fingerprint, created_at
				FROM victims
				ON CONFLICT (request_id, api_key_id) DO NOTHING
			)
			DELETE FROM usage_billing_dedup
			WHERE ctid IN (SELECT ctid FROM victims)
		`, cutoff.UTC(), usageBillingDedupCleanupBatchSize)
		if err != nil {
			return err
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if affected < usageBillingDedupCleanupBatchSize {
			return nil
		}
	}
}

func (r *dashboardAggregationRepository) EnsureUsageLogsPartitions(ctx context.Context, now time.Time) error {
	if IsMySQL() {
		// usage_logs 在 MySQL 是普通表（无分区），无需创建分区。
		return nil
	}
	isPartitioned, err := r.isUsageLogsPartitioned(ctx)
	if err != nil || !isPartitioned {
		return err
	}
	monthStart := truncateToMonthUTC(now)
	prevMonth := monthStart.AddDate(0, -1, 0)
	nextMonth := monthStart.AddDate(0, 1, 0)

	for _, m := range []time.Time{prevMonth, monthStart, nextMonth} {
		if err := r.createUsageLogsPartition(ctx, m); err != nil {
			return err
		}
	}
	return nil
}

func (r *dashboardAggregationRepository) insertHourlyActiveUsers(ctx context.Context, start, end time.Time) error {
	if IsMySQL() {
		return r.insertHourlyActiveUsersMySQL(ctx, start, end)
	}
	tzName := timezone.Name()
	query := `
		INSERT INTO usage_dashboard_hourly_users (bucket_start, user_id)
		SELECT DISTINCT
			date_trunc('hour', created_at AT TIME ZONE $3) AT TIME ZONE $3 AS bucket_start,
			user_id
		FROM usage_logs
		WHERE created_at >= $1 AND created_at < $2
		ON CONFLICT DO NOTHING
	`
	_, err := r.sql.ExecContext(ctx, query, start, end, tzName)
	return err
}

func (r *dashboardAggregationRepository) insertDailyActiveUsers(ctx context.Context, start, end time.Time) error {
	if IsMySQL() {
		return r.insertDailyActiveUsersMySQL(ctx, start, end)
	}
	tzName := timezone.Name()
	query := `
		INSERT INTO usage_dashboard_daily_users (bucket_date, user_id)
		SELECT DISTINCT
			(bucket_start AT TIME ZONE $3)::date AS bucket_date,
			user_id
		FROM usage_dashboard_hourly_users
		WHERE bucket_start >= $1 AND bucket_start < $2
		ON CONFLICT DO NOTHING
	`
	_, err := r.sql.ExecContext(ctx, query, start, end, tzName)
	return err
}

func (r *dashboardAggregationRepository) upsertHourlyAggregates(ctx context.Context, start, end time.Time) error {
	if IsMySQL() {
		return r.upsertHourlyAggregatesMySQL(ctx, start, end)
	}
	tzName := timezone.Name()
	query := `
		WITH hourly AS (
			SELECT
				date_trunc('hour', created_at AT TIME ZONE $3) AT TIME ZONE $3 AS bucket_start,
				COUNT(*) AS total_requests,
				COALESCE(SUM(input_tokens), 0) AS input_tokens,
				COALESCE(SUM(output_tokens), 0) AS output_tokens,
				COALESCE(SUM(cache_creation_tokens), 0) AS cache_creation_tokens,
				COALESCE(SUM(cache_read_tokens), 0) AS cache_read_tokens,
				COALESCE(SUM(total_cost), 0) AS total_cost,
				COALESCE(SUM(actual_cost), 0) AS actual_cost,
				COALESCE(SUM(COALESCE(account_stats_cost, total_cost) * COALESCE(account_rate_multiplier, 1)), 0) AS account_cost,
				COALESCE(SUM(COALESCE(duration_ms, 0)), 0) AS total_duration_ms
			FROM usage_logs
			WHERE created_at >= $1 AND created_at < $2
			GROUP BY 1
		),
		user_counts AS (
			SELECT bucket_start, COUNT(*) AS active_users
			FROM usage_dashboard_hourly_users
			WHERE bucket_start >= $1 AND bucket_start < $2
			GROUP BY bucket_start
		)
		INSERT INTO usage_dashboard_hourly (
			bucket_start,
			total_requests,
			input_tokens,
			output_tokens,
			cache_creation_tokens,
			cache_read_tokens,
			total_cost,
			actual_cost,
			account_cost,
			total_duration_ms,
			active_users,
			computed_at
		)
		SELECT
			hourly.bucket_start,
			hourly.total_requests,
			hourly.input_tokens,
			hourly.output_tokens,
			hourly.cache_creation_tokens,
			hourly.cache_read_tokens,
			hourly.total_cost,
			hourly.actual_cost,
			hourly.account_cost,
			hourly.total_duration_ms,
			COALESCE(user_counts.active_users, 0) AS active_users,
			NOW()
		FROM hourly
		LEFT JOIN user_counts ON user_counts.bucket_start = hourly.bucket_start
		ON CONFLICT (bucket_start)
		DO UPDATE SET
			total_requests = EXCLUDED.total_requests,
			input_tokens = EXCLUDED.input_tokens,
			output_tokens = EXCLUDED.output_tokens,
			cache_creation_tokens = EXCLUDED.cache_creation_tokens,
			cache_read_tokens = EXCLUDED.cache_read_tokens,
			total_cost = EXCLUDED.total_cost,
			actual_cost = EXCLUDED.actual_cost,
			account_cost = EXCLUDED.account_cost,
			total_duration_ms = EXCLUDED.total_duration_ms,
			active_users = EXCLUDED.active_users,
			computed_at = EXCLUDED.computed_at
	`
	_, err := r.sql.ExecContext(ctx, query, start, end, tzName)
	return err
}

func (r *dashboardAggregationRepository) upsertDailyAggregates(ctx context.Context, start, end time.Time) error {
	if IsMySQL() {
		return r.upsertDailyAggregatesMySQL(ctx, start, end)
	}
	tzName := timezone.Name()
	query := `
		WITH daily AS (
			SELECT
				(bucket_start AT TIME ZONE $5)::date AS bucket_date,
				COALESCE(SUM(total_requests), 0) AS total_requests,
				COALESCE(SUM(input_tokens), 0) AS input_tokens,
				COALESCE(SUM(output_tokens), 0) AS output_tokens,
				COALESCE(SUM(cache_creation_tokens), 0) AS cache_creation_tokens,
				COALESCE(SUM(cache_read_tokens), 0) AS cache_read_tokens,
				COALESCE(SUM(total_cost), 0) AS total_cost,
				COALESCE(SUM(actual_cost), 0) AS actual_cost,
				COALESCE(SUM(account_cost), 0) AS account_cost,
				COALESCE(SUM(total_duration_ms), 0) AS total_duration_ms
			FROM usage_dashboard_hourly
			WHERE bucket_start >= $1 AND bucket_start < $2
			GROUP BY (bucket_start AT TIME ZONE $5)::date
		),
		user_counts AS (
			SELECT bucket_date, COUNT(*) AS active_users
			FROM usage_dashboard_daily_users
			WHERE bucket_date >= $3::date AND bucket_date < $4::date
			GROUP BY bucket_date
		)
		INSERT INTO usage_dashboard_daily (
			bucket_date,
			total_requests,
			input_tokens,
			output_tokens,
			cache_creation_tokens,
			cache_read_tokens,
			total_cost,
			actual_cost,
			account_cost,
			total_duration_ms,
			active_users,
			computed_at
		)
		SELECT
			daily.bucket_date,
			daily.total_requests,
			daily.input_tokens,
			daily.output_tokens,
			daily.cache_creation_tokens,
			daily.cache_read_tokens,
			daily.total_cost,
			daily.actual_cost,
			daily.account_cost,
			daily.total_duration_ms,
			COALESCE(user_counts.active_users, 0) AS active_users,
			NOW()
		FROM daily
		LEFT JOIN user_counts ON user_counts.bucket_date = daily.bucket_date
		ON CONFLICT (bucket_date)
		DO UPDATE SET
			total_requests = EXCLUDED.total_requests,
			input_tokens = EXCLUDED.input_tokens,
			output_tokens = EXCLUDED.output_tokens,
			cache_creation_tokens = EXCLUDED.cache_creation_tokens,
			cache_read_tokens = EXCLUDED.cache_read_tokens,
			total_cost = EXCLUDED.total_cost,
			actual_cost = EXCLUDED.actual_cost,
			account_cost = EXCLUDED.account_cost,
			total_duration_ms = EXCLUDED.total_duration_ms,
			active_users = EXCLUDED.active_users,
			computed_at = EXCLUDED.computed_at
	`
	_, err := r.sql.ExecContext(ctx, query, start, end, start, end, tzName)
	return err
}

func (r *dashboardAggregationRepository) isUsageLogsPartitioned(ctx context.Context) (bool, error) {
	if IsMySQL() {
		// usage_logs 在 MySQL 是普通表（无分区），恒返回 false。
		return false, nil
	}
	query := `
		SELECT EXISTS(
			SELECT 1
			FROM pg_partitioned_table pt
			JOIN pg_class c ON c.oid = pt.partrelid
			WHERE c.relname = 'usage_logs'
		)
	`
	var partitioned bool
	if err := scanSingleRow(ctx, r.sql, query, nil, &partitioned); err != nil {
		return false, err
	}
	return partitioned, nil
}

func (r *dashboardAggregationRepository) dropUsageLogsPartitions(ctx context.Context, cutoff time.Time) error {
	if IsMySQL() {
		// usage_logs 在 MySQL 是普通表（无分区），无分区可删，直接 no-op。
		return nil
	}
	rows, err := r.sql.QueryContext(ctx, `
		SELECT c.relname
		FROM pg_inherits
		JOIN pg_class c ON c.oid = pg_inherits.inhrelid
		JOIN pg_class p ON p.oid = pg_inherits.inhparent
		WHERE p.relname = 'usage_logs'
	`)
	if err != nil {
		return err
	}
	cutoffMonth := truncateToMonthUTC(cutoff)
	type usageLogsPartition struct {
		name  string
		month time.Time
	}
	partitions := make([]usageLogsPartition, 0)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			_ = rows.Close()
			return err
		}
		if !strings.HasPrefix(name, "usage_logs_") {
			continue
		}
		suffix := strings.TrimPrefix(name, "usage_logs_")
		month, err := time.Parse("200601", suffix)
		if err != nil {
			continue
		}
		month = month.UTC()
		if month.Before(cutoffMonth) {
			partitions = append(partitions, usageLogsPartition{name: name, month: month})
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}

	sort.Slice(partitions, func(i, j int) bool {
		return partitions[i].month.Before(partitions[j].month)
	})
	if db, ok := r.sql.(*sql.DB); ok {
		for _, partition := range partitions {
			if err := dropUsageLogsPartitionWithRollupInvalidation(ctx, db, partition.name, partition.month); err != nil {
				return err
			}
		}
		return nil
	}
	for _, partition := range partitions {
		if _, err := r.sql.ExecContext(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s", pq.QuoteIdentifier(partition.name))); err != nil {
			return err
		}
	}
	return nil
}

func dropUsageLogsPartitionWithRollupInvalidation(ctx context.Context, db *sql.DB, name string, monthStart time.Time) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	rollback := func(err error) error {
		_ = tx.Rollback()
		return err
	}

	if err := lockGroupUsageRollupState(ctx, tx); err != nil {
		return rollback(err)
	}
	if err := invalidateGroupUsageRollupsAt(ctx, tx, monthStart); err != nil {
		return rollback(err)
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s", pq.QuoteIdentifier(name))); err != nil {
		return rollback(err)
	}
	return tx.Commit()
}

func (r *dashboardAggregationRepository) createUsageLogsPartition(ctx context.Context, month time.Time) error {
	if IsMySQL() {
		// usage_logs 在 MySQL 是普通表（无分区），直接 no-op。
		return nil
	}
	monthStart := truncateToMonthUTC(month)
	nextMonth := monthStart.AddDate(0, 1, 0)
	name := fmt.Sprintf("usage_logs_%s", monthStart.Format("200601"))
	query := fmt.Sprintf(
		"CREATE TABLE IF NOT EXISTS %s PARTITION OF usage_logs FOR VALUES FROM (%s) TO (%s)",
		pq.QuoteIdentifier(name),
		pq.QuoteLiteral(monthStart.Format("2006-01-02")),
		pq.QuoteLiteral(nextMonth.Format("2006-01-02")),
	)
	_, err := r.sql.ExecContext(ctx, query)
	return err
}

func truncateToDay(t time.Time) time.Time {
	return timezone.StartOfDay(t)
}

func truncateToMonthUTC(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}

// ---------------------------------------------------------------------------
// MySQL 方言实现（见 DIALECT_MYSQL_GUIDE.md）。
//
// 时区说明：MySQL 分支的桶边界计算直接用 created_at 的墙钟（DATE_FORMAT / DATE），
// 不做 AT TIME ZONE 换算——internal/config MySQLConfig 保证 DSN Loc == 会话 time_zone
// == 服务端时区，DATETIME 存取即服务端时区墙钟，与 PG「created_at AT TIME ZONE tz」
// 的桶语义一致。REVIEW: 若运行时 DSN Loc/会话 time_zone 与 timezone.Location() 不一致，
// 桶边界会整体平移。
// ---------------------------------------------------------------------------

// recomputeRangeInTxMySQL 是 recomputeRangeInTx 的 MySQL 实现：
// $n→?；DATE 列比较去掉 ::date 显式转型；四类聚合写入走各自 MySQL 版本。
func (r *dashboardAggregationRepository) recomputeRangeInTxMySQL(ctx context.Context, hourStart, hourEnd, dayStart, dayEnd time.Time) error {
	// 先清空范围内桶，再重建（与 PG 版语义一致）。
	if _, err := r.sql.ExecContext(ctx, "DELETE FROM usage_dashboard_hourly WHERE bucket_start >= ? AND bucket_start < ?", hourStart, hourEnd); err != nil {
		return err
	}
	if _, err := r.sql.ExecContext(ctx, "DELETE FROM usage_dashboard_hourly_users WHERE bucket_start >= ? AND bucket_start < ?", hourStart, hourEnd); err != nil {
		return err
	}
	if _, err := r.sql.ExecContext(ctx, "DELETE FROM usage_dashboard_daily WHERE bucket_date >= ? AND bucket_date < ?", dayStart, dayEnd); err != nil {
		return err
	}
	if _, err := r.sql.ExecContext(ctx, "DELETE FROM usage_dashboard_daily_users WHERE bucket_date >= ? AND bucket_date < ?", dayStart, dayEnd); err != nil {
		return err
	}

	if err := r.insertHourlyActiveUsersMySQL(ctx, hourStart, hourEnd); err != nil {
		return err
	}
	if err := r.insertDailyActiveUsersMySQL(ctx, hourStart, hourEnd); err != nil {
		return err
	}
	if err := r.upsertHourlyAggregatesMySQL(ctx, hourStart, hourEnd); err != nil {
		return err
	}
	return r.upsertDailyAggregatesMySQL(ctx, dayStart, dayEnd)
}

// cleanupUsageLogsBatchesMySQL 是 cleanupUsageLogsBatches 的 MySQL 实现。
// usage_logs 在 MySQL 是普通表（无分区）：isUsageLogsPartitioned 恒为 false，
// 批量清理直接按 created_at 分批 DELETE（ORDER BY/LIMIT），事务路径与 PG 对齐
// （锁定→删除→失效汇总水位）。
func (r *dashboardAggregationRepository) cleanupUsageLogsBatchesMySQL(ctx context.Context, cutoff time.Time) error {
	db, transactional := r.sql.(*sql.DB)
	for {
		if transactional {
			affected, err := cleanupUsageLogsBatchWithRollupInvalidationMySQL(ctx, db, cutoff)
			if err != nil {
				return err
			}
			if affected < usageLogsCleanupBatchSize {
				return nil
			}
			continue
		}

		res, err := r.sql.ExecContext(ctx, `
			DELETE FROM usage_logs
			WHERE created_at < ?
			ORDER BY created_at ASC, id ASC
			LIMIT ?
		`, cutoff.UTC(), usageLogsCleanupBatchSize)
		if err != nil {
			return err
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if affected < usageLogsCleanupBatchSize {
			return nil
		}
	}
}

// cleanupUsageLogsBatchWithRollupInvalidationMySQL 是 cleanupUsageLogsBatchWithRollupInvalidation
// 的 MySQL 实现。PG 版 DELETE (tableoid, ctid) RETURNING created_at；MySQL 拆为同事务两步：
// 先 SELECT id, created_at ... FOR UPDATE 取目标行（物理行标识 ctid 以主键 id 替代，
// 先取后删），再按主键删除，用删除行集合还原 RETURNING 的行数与最早 created_at。
func cleanupUsageLogsBatchWithRollupInvalidationMySQL(ctx context.Context, db *sql.DB, cutoff time.Time) (int64, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	rollback := func(err error) (int64, error) {
		_ = tx.Rollback()
		return 0, err
	}

	if err := lockGroupUsageRollupState(ctx, tx); err != nil {
		return rollback(err)
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT id, created_at
		FROM usage_logs
		WHERE created_at < ?
		ORDER BY created_at ASC, id ASC
		LIMIT ?
		FOR UPDATE
	`, cutoff.UTC(), usageLogsCleanupBatchSize)
	if err != nil {
		return rollback(err)
	}

	var affected int64
	var earliestDeletedAt time.Time
	idArgs := make([]any, 0, usageLogsCleanupBatchSize)
	for rows.Next() {
		var id int64
		var deletedAt time.Time
		if err := rows.Scan(&id, &deletedAt); err != nil {
			_ = rows.Close()
			return rollback(err)
		}
		idArgs = append(idArgs, id)
		affected++
		if earliestDeletedAt.IsZero() || deletedAt.Before(earliestDeletedAt) {
			earliestDeletedAt = deletedAt
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return rollback(err)
	}
	if err := rows.Close(); err != nil {
		return rollback(err)
	}
	if affected > 0 {
		if _, err := tx.ExecContext(ctx, "DELETE FROM usage_logs WHERE id IN ("+Placeholders(len(idArgs))+")", idArgs...); err != nil {
			return rollback(err)
		}
		if err := invalidateGroupUsageRollupsAt(ctx, tx, earliestDeletedAt); err != nil {
			return rollback(err)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return affected, nil
}

// cleanupUsageBillingDedupMySQL 是 CleanupUsageBillingDedup 的 MySQL 实现（分批循环）。
func (r *dashboardAggregationRepository) cleanupUsageBillingDedupMySQL(ctx context.Context, cutoff time.Time) error {
	for {
		affected, err := r.cleanupUsageBillingDedupBatchMySQL(ctx, cutoff)
		if err != nil {
			return err
		}
		if affected < usageBillingDedupCleanupBatchSize {
			return nil
		}
	}
}

// cleanupUsageBillingDedupBatchMySQL 执行一批「归档 + 删除」：*sql.DB 上按批开事务
// （与 PG 单条可写 CTE 的原子性对齐）；非 *sql.DB（测试 mock）退化为顺序执行。
func (r *dashboardAggregationRepository) cleanupUsageBillingDedupBatchMySQL(ctx context.Context, cutoff time.Time) (int64, error) {
	db, ok := r.sql.(*sql.DB)
	if !ok {
		return cleanupUsageBillingDedupBatchOnMySQL(ctx, r.sql, r.sql, cutoff)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	rollback := func(err error) (int64, error) {
		_ = tx.Rollback()
		return 0, err
	}
	affected, err := cleanupUsageBillingDedupBatchOnMySQL(ctx, tx, tx, cutoff)
	if err != nil {
		return rollback(err)
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return affected, nil
}

// cleanupUsageBillingDedupBatchOnMySQL 在同一连接/事务上执行一批归档+删除。
// PG 版为可写 CTE（victims + INSERT..ON CONFLICT (request_id, api_key_id) DO NOTHING
// 归档 + 按 ctid 删除）；MySQL 拆为三步：先 SELECT ... FOR UPDATE 取受害者行
// （ctid 以主键 id 替代），再多行 INSERT 归档（ODKU no-op 幂等，冲突保留既有归档行，
// 与 DO NOTHING 语义一致），最后按主键删除。返回删除行数（对应 PG DELETE 的 RowsAffected）。
// REVIEW: ODKU 引用既有行值用目标表限定名；INSERT..SELECT 形态不支持 8.0.19+ 行别名，
// 需要引用新行值时只能用 VALUES()（8.0.20+ 标记废弃但仍可用）。
func cleanupUsageBillingDedupBatchOnMySQL(ctx context.Context, q sqlQueryer, exec sqlExecutor, cutoff time.Time) (int64, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT id, request_id, api_key_id, request_fingerprint, created_at
		FROM usage_billing_dedup
		WHERE created_at < ?
		ORDER BY created_at ASC, id ASC
		LIMIT ?
		FOR UPDATE
	`, cutoff.UTC(), usageBillingDedupCleanupBatchSize)
	if err != nil {
		return 0, err
	}

	type usageBillingDedupRow struct {
		id                 int64
		requestID          string
		apiKeyID           int64
		requestFingerprint string
		createdAt          time.Time
	}
	victims := make([]usageBillingDedupRow, 0, usageBillingDedupCleanupBatchSize)
	for rows.Next() {
		var row usageBillingDedupRow
		if err := rows.Scan(&row.id, &row.requestID, &row.apiKeyID, &row.requestFingerprint, &row.createdAt); err != nil {
			_ = rows.Close()
			return 0, err
		}
		victims = append(victims, row)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return 0, err
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	if len(victims) == 0 {
		return 0, nil
	}

	var insert strings.Builder
	insert.WriteString(`
		INSERT INTO usage_billing_dedup_archive (request_id, api_key_id, request_fingerprint, created_at)
		VALUES
	`)
	args := make([]any, 0, len(victims)*4)
	for i, victim := range victims {
		if i > 0 {
			insert.WriteString(",")
		}
		insert.WriteString("(?, ?, ?, ?)")
		args = append(args, victim.requestID, victim.apiKeyID, victim.requestFingerprint, victim.createdAt)
	}
	insert.WriteString(`
		ON DUPLICATE KEY UPDATE
			request_id = usage_billing_dedup_archive.request_id,
			api_key_id = usage_billing_dedup_archive.api_key_id
	`)
	if _, err := exec.ExecContext(ctx, insert.String(), args...); err != nil {
		return 0, err
	}

	idArgs := make([]any, 0, len(victims))
	for _, victim := range victims {
		idArgs = append(idArgs, victim.id)
	}
	res, err := exec.ExecContext(ctx, "DELETE FROM usage_billing_dedup WHERE id IN ("+Placeholders(len(idArgs))+")", idArgs...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// insertHourlyActiveUsersMySQL 是 insertHourlyActiveUsers 的 MySQL 实现：
// date_trunc('hour', created_at AT TIME ZONE tz) AT TIME ZONE tz →
// CAST(DATE_FORMAT(created_at, '%Y-%m-%d %H:00:00') AS DATETIME)（墙钟即桶，见文件头时区说明）；
// ON CONFLICT DO NOTHING → ODKU no-op（目标表限定名避免与 SELECT 输出列歧义）。
func (r *dashboardAggregationRepository) insertHourlyActiveUsersMySQL(ctx context.Context, start, end time.Time) error {
	query := `
		INSERT INTO usage_dashboard_hourly_users (bucket_start, user_id)
		SELECT DISTINCT
			CAST(DATE_FORMAT(created_at, '%Y-%m-%d %H:00:00') AS DATETIME) AS bucket_start,
			user_id
		FROM usage_logs
		WHERE created_at >= ? AND created_at < ?
		ON DUPLICATE KEY UPDATE bucket_start = usage_dashboard_hourly_users.bucket_start
	`
	_, err := r.sql.ExecContext(ctx, query, start, end)
	return err
}

// insertDailyActiveUsersMySQL 是 insertDailyActiveUsers 的 MySQL 实现：
// (bucket_start AT TIME ZONE tz)::date → DATE(bucket_start)；ODKU no-op 同上。
func (r *dashboardAggregationRepository) insertDailyActiveUsersMySQL(ctx context.Context, start, end time.Time) error {
	query := `
		INSERT INTO usage_dashboard_daily_users (bucket_date, user_id)
		SELECT DISTINCT
			DATE(bucket_start) AS bucket_date,
			user_id
		FROM usage_dashboard_hourly_users
		WHERE bucket_start >= ? AND bucket_start < ?
		ON DUPLICATE KEY UPDATE bucket_date = usage_dashboard_daily_users.bucket_date
	`
	_, err := r.sql.ExecContext(ctx, query, start, end)
	return err
}

// upsertHourlyAggregatesMySQL 是 upsertHourlyAggregates 的 MySQL 实现：
// PG 的 hourly / user_counts 两个 CTE 改写为派生表 JOIN；ON CONFLICT (bucket_start)
// DO UPDATE（EXCLUDED.*）→ ODKU VALUES(...)（usage_dashboard_hourly 以 bucket_start 为主键）。
func (r *dashboardAggregationRepository) upsertHourlyAggregatesMySQL(ctx context.Context, start, end time.Time) error {
	query := `
		INSERT INTO usage_dashboard_hourly (
			bucket_start,
			total_requests,
			input_tokens,
			output_tokens,
			cache_creation_tokens,
			cache_read_tokens,
			total_cost,
			actual_cost,
			account_cost,
			total_duration_ms,
			active_users,
			computed_at
		)
		SELECT
			hourly.bucket_start,
			hourly.total_requests,
			hourly.input_tokens,
			hourly.output_tokens,
			hourly.cache_creation_tokens,
			hourly.cache_read_tokens,
			hourly.total_cost,
			hourly.actual_cost,
			hourly.account_cost,
			hourly.total_duration_ms,
			COALESCE(user_counts.active_users, 0) AS active_users,
			NOW(3)
		FROM (
			SELECT
				CAST(DATE_FORMAT(created_at, '%Y-%m-%d %H:00:00') AS DATETIME) AS bucket_start,
				COUNT(*) AS total_requests,
				COALESCE(SUM(input_tokens), 0) AS input_tokens,
				COALESCE(SUM(output_tokens), 0) AS output_tokens,
				COALESCE(SUM(cache_creation_tokens), 0) AS cache_creation_tokens,
				COALESCE(SUM(cache_read_tokens), 0) AS cache_read_tokens,
				COALESCE(SUM(total_cost), 0) AS total_cost,
				COALESCE(SUM(actual_cost), 0) AS actual_cost,
				COALESCE(SUM(COALESCE(account_stats_cost, total_cost) * COALESCE(account_rate_multiplier, 1)), 0) AS account_cost,
				COALESCE(SUM(COALESCE(duration_ms, 0)), 0) AS total_duration_ms
			FROM usage_logs
			WHERE created_at >= ? AND created_at < ?
			GROUP BY bucket_start
		) hourly
		LEFT JOIN (
			SELECT bucket_start, COUNT(*) AS active_users
			FROM usage_dashboard_hourly_users
			WHERE bucket_start >= ? AND bucket_start < ?
			GROUP BY bucket_start
		) user_counts ON user_counts.bucket_start = hourly.bucket_start
		ON DUPLICATE KEY UPDATE
			total_requests = VALUES(total_requests),
			input_tokens = VALUES(input_tokens),
			output_tokens = VALUES(output_tokens),
			cache_creation_tokens = VALUES(cache_creation_tokens),
			cache_read_tokens = VALUES(cache_read_tokens),
			total_cost = VALUES(total_cost),
			actual_cost = VALUES(actual_cost),
			account_cost = VALUES(account_cost),
			total_duration_ms = VALUES(total_duration_ms),
			active_users = VALUES(active_users),
			computed_at = VALUES(computed_at)
	`
	_, err := r.sql.ExecContext(ctx, query, start, end, start, end)
	return err
}

// upsertDailyAggregatesMySQL 是 upsertDailyAggregates 的 MySQL 实现：
// CTE 改写为派生表 JOIN，(bucket_start AT TIME ZONE tz)::date → DATE(bucket_start)；
// ON CONFLICT (bucket_date) DO UPDATE → ODKU VALUES(...)（usage_dashboard_daily 以 bucket_date 为主键）。
func (r *dashboardAggregationRepository) upsertDailyAggregatesMySQL(ctx context.Context, start, end time.Time) error {
	query := `
		INSERT INTO usage_dashboard_daily (
			bucket_date,
			total_requests,
			input_tokens,
			output_tokens,
			cache_creation_tokens,
			cache_read_tokens,
			total_cost,
			actual_cost,
			account_cost,
			total_duration_ms,
			active_users,
			computed_at
		)
		SELECT
			daily.bucket_date,
			daily.total_requests,
			daily.input_tokens,
			daily.output_tokens,
			daily.cache_creation_tokens,
			daily.cache_read_tokens,
			daily.total_cost,
			daily.actual_cost,
			daily.account_cost,
			daily.total_duration_ms,
			COALESCE(user_counts.active_users, 0) AS active_users,
			NOW(3)
		FROM (
			SELECT
				DATE(bucket_start) AS bucket_date,
				COALESCE(SUM(total_requests), 0) AS total_requests,
				COALESCE(SUM(input_tokens), 0) AS input_tokens,
				COALESCE(SUM(output_tokens), 0) AS output_tokens,
				COALESCE(SUM(cache_creation_tokens), 0) AS cache_creation_tokens,
				COALESCE(SUM(cache_read_tokens), 0) AS cache_read_tokens,
				COALESCE(SUM(total_cost), 0) AS total_cost,
				COALESCE(SUM(actual_cost), 0) AS actual_cost,
				COALESCE(SUM(account_cost), 0) AS account_cost,
				COALESCE(SUM(total_duration_ms), 0) AS total_duration_ms
			FROM usage_dashboard_hourly
			WHERE bucket_start >= ? AND bucket_start < ?
			GROUP BY DATE(bucket_start)
		) daily
		LEFT JOIN (
			SELECT bucket_date, COUNT(*) AS active_users
			FROM usage_dashboard_daily_users
			WHERE bucket_date >= ? AND bucket_date < ?
			GROUP BY bucket_date
		) user_counts ON user_counts.bucket_date = daily.bucket_date
		ON DUPLICATE KEY UPDATE
			total_requests = VALUES(total_requests),
			input_tokens = VALUES(input_tokens),
			output_tokens = VALUES(output_tokens),
			cache_creation_tokens = VALUES(cache_creation_tokens),
			cache_read_tokens = VALUES(cache_read_tokens),
			total_cost = VALUES(total_cost),
			actual_cost = VALUES(actual_cost),
			account_cost = VALUES(account_cost),
			total_duration_ms = VALUES(total_duration_ms),
			active_users = VALUES(active_users),
			computed_at = VALUES(computed_at)
	`
	_, err := r.sql.ExecContext(ctx, query, start, end, start, end)
	return err
}
