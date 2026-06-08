// monitor_rollup_repo.go — channel-monitor 日聚合表（rollups）的写入、裁剪与
// watermark 维护。与 channel_monitor_repo.go 同 receiver `*channelMonitorRepository`、
// 同 package；调用方仅 OpsCleanupService 的每日维护任务。
package monitorrepository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	monitorservice "github.com/Wei-Shaw/sub2api/plugins/channel-management/monitor/service"
)

// UpsertDailyRollupsFor 把 targetDate 当天的明细按 (monitor_id, model, bucket_date)
// 聚合到 channel_monitor_daily_rollups。targetDate 会被截断到日期；
// 用 ON CONFLICT DO UPDATE 实现幂等回填，返回 upsert 影响的行数。
func (r *channelMonitorRepository) UpsertDailyRollupsFor(ctx context.Context, targetDate time.Time) (int64, error) {
	const q = `
		INSERT INTO channel_monitor_daily_rollups (
		    monitor_id, model, bucket_date,
		    total_checks, ok_count,
		    operational_count, degraded_count, failed_count, error_count,
		    sum_latency_ms, count_latency,
		    sum_ping_latency_ms, count_ping_latency,
		    computed_at
		)
		SELECT
		    monitor_id,
		    model,
		    $1::date AS bucket_date,
		    COUNT(*)                                                         AS total_checks,
		    COUNT(*) FILTER (WHERE status IN ('operational','degraded'))     AS ok_count,
		    COUNT(*) FILTER (WHERE status = 'operational')                   AS operational_count,
		    COUNT(*) FILTER (WHERE status = 'degraded')                      AS degraded_count,
		    COUNT(*) FILTER (WHERE status = 'failed')                        AS failed_count,
		    COUNT(*) FILTER (WHERE status = 'error')                         AS error_count,
		    COALESCE(SUM(latency_ms) FILTER (WHERE latency_ms IS NOT NULL), 0)             AS sum_latency_ms,
		    COUNT(latency_ms)                                                AS count_latency,
		    COALESCE(SUM(ping_latency_ms) FILTER (WHERE ping_latency_ms IS NOT NULL), 0)   AS sum_ping_latency_ms,
		    COUNT(ping_latency_ms)                                           AS count_ping_latency,
		    NOW()
		FROM channel_monitor_histories
		WHERE checked_at >= $1::date
		  AND checked_at <  ($1::date + INTERVAL '1 day')
		GROUP BY monitor_id, model
		ON CONFLICT (monitor_id, model, bucket_date) DO UPDATE SET
		    total_checks        = EXCLUDED.total_checks,
		    ok_count            = EXCLUDED.ok_count,
		    operational_count   = EXCLUDED.operational_count,
		    degraded_count      = EXCLUDED.degraded_count,
		    failed_count        = EXCLUDED.failed_count,
		    error_count         = EXCLUDED.error_count,
		    sum_latency_ms      = EXCLUDED.sum_latency_ms,
		    count_latency       = EXCLUDED.count_latency,
		    sum_ping_latency_ms = EXCLUDED.sum_ping_latency_ms,
		    count_ping_latency  = EXCLUDED.count_ping_latency,
		    computed_at         = NOW()
	`
	res, err := r.db.ExecContext(ctx, q, targetDate)
	if err != nil {
		return 0, fmt.Errorf("upsert daily rollups for %s: %w", targetDate.Format("2006-01-02"), err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("rows affected (upsert rollups): %w", err)
	}
	return n, nil
}

// DeleteRollupsBefore 软删 bucket_date < beforeDate 的聚合行，按 batch 进行。
func (r *channelMonitorRepository) DeleteRollupsBefore(ctx context.Context, beforeDate time.Time) (int64, error) {
	return deleteChannelMonitorBatched(ctx, r.db, channelMonitorPruneRollupSQL, beforeDate)
}

// LoadAggregationWatermark 读 watermark（id=1）。
// 返回 nil 表示从未聚合过；watermark 表本身预期已存在单行（migration 110 写入）。
func (r *channelMonitorRepository) LoadAggregationWatermark(ctx context.Context) (*time.Time, error) {
	const q = `SELECT last_aggregated_date FROM channel_monitor_aggregation_watermark WHERE id = 1`
	var t sql.NullTime
	if err := r.db.QueryRowContext(ctx, q).Scan(&t); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("load aggregation watermark: %w", err)
	}
	if !t.Valid {
		return nil, nil
	}
	return &t.Time, nil
}

// UpdateAggregationWatermark 写 watermark（UPSERT 到 id=1）。
func (r *channelMonitorRepository) UpdateAggregationWatermark(ctx context.Context, date time.Time) error {
	const q = `
		INSERT INTO channel_monitor_aggregation_watermark (id, last_aggregated_date, updated_at)
		VALUES (1, $1::date, NOW())
		ON CONFLICT (id) DO UPDATE SET
		    last_aggregated_date = EXCLUDED.last_aggregated_date,
		    updated_at           = NOW()
	`
	if _, err := r.db.ExecContext(ctx, q, date); err != nil {
		return fmt.Errorf("update aggregation watermark: %w", err)
	}
	return nil
}

// 静态校验：保证 ChannelMonitorRepository 接口完整覆盖。
// 拆分到多个文件后保留一个统一断言，方便 IDE / lint 检查实现完整性。
var _ monitorservice.ChannelMonitorRepository = (*channelMonitorRepository)(nil)

// channelMonitorPruneRollupSQL 是按 batch 软删 daily rollup 表的语句。
const channelMonitorPruneRollupSQL = `
WITH batch AS (
    SELECT id FROM channel_monitor_daily_rollups
    WHERE bucket_date < $1::date
    ORDER BY id
    LIMIT $2
)
DELETE FROM channel_monitor_daily_rollups
WHERE id IN (SELECT id FROM batch)
`
