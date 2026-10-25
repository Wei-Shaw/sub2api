package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// Platform is derived from group/account (usage_logs has no provider column on upstream schema).
const channelMonitorV2PlatformSQL = `lower(` + usageLogEffectivePlatformExpr + `)`
const channelMonitorV2ModelSQL = `COALESCE(NULLIF(TRIM(ul.requested_model), ''), NULLIF(TRIM(ul.model), ''), 'unknown')`

// Tiered retention balances UI windows against storage:
//
//	1m facts  → short (late writes + rebuild rollups)
//	5m/1h/12h/1d rollups → longer, aligned to 90m / 24h / 7d / 30d(+audit)
//
// Backfill may still write short-lived 1m rows for old windows so rollups can be
// built; prune at end of each recompute drops them past their TTL while rollups remain.
const (
	channelMonitorV2RetentionUser1m      = 3 * 24 * time.Hour
	channelMonitorV2RetentionMetrics1m   = 7 * 24 * time.Hour
	channelMonitorV2RetentionError1m     = 7 * 24 * time.Hour
	channelMonitorV2RetentionHistogram1m = 7 * 24 * time.Hour
	channelMonitorV2RetentionRollup5m    = 7 * 24 * time.Hour  // bucket_seconds=300
	channelMonitorV2RetentionRollup1h    = 30 * 24 * time.Hour // 3600
	channelMonitorV2RetentionRollup12h   = 45 * 24 * time.Hour // 43200
	channelMonitorV2RetentionRollup1d    = 90 * 24 * time.Hour // 86400
	channelMonitorV2RetentionMax         = channelMonitorV2RetentionRollup1d
)

// channelMonitorV2MaxRetention is the longest stored window (1d rollup). Used to
// clamp recompute/backfill so we never scan older than product history needs.
func channelMonitorV2MaxRetention() time.Duration {
	return channelMonitorV2RetentionMax
}

func channelMonitorV2RetentionCutoff(now time.Time, retention time.Duration) time.Time {
	return now.UTC().Truncate(time.Minute).Add(-retention)
}

type channelMonitorV2RetentionRule struct {
	table         string
	retention     time.Duration
	bucketSeconds int // 0 = fact table (no bucket_seconds column)
}

// channelMonitorV2RetentionRules is ordered coarse→fine for predictable prune plans.
var channelMonitorV2RetentionRules = []channelMonitorV2RetentionRule{
	{table: "channel_monitor_v2_user_metrics_1m", retention: channelMonitorV2RetentionUser1m},
	{table: "channel_monitor_v2_metrics_1m", retention: channelMonitorV2RetentionMetrics1m},
	{table: "channel_monitor_v2_error_metrics_1m", retention: channelMonitorV2RetentionError1m},
	{table: "channel_monitor_v2_latency_histograms_1m", retention: channelMonitorV2RetentionHistogram1m},
	{table: "channel_monitor_v2_metrics_rollup", retention: channelMonitorV2RetentionRollup5m, bucketSeconds: 300},
	{table: "channel_monitor_v2_user_metrics_rollup", retention: channelMonitorV2RetentionRollup5m, bucketSeconds: 300},
	{table: "channel_monitor_v2_error_metrics_rollup", retention: channelMonitorV2RetentionRollup5m, bucketSeconds: 300},
	{table: "channel_monitor_v2_latency_histograms_rollup", retention: channelMonitorV2RetentionRollup5m, bucketSeconds: 300},
	{table: "channel_monitor_v2_metrics_rollup", retention: channelMonitorV2RetentionRollup1h, bucketSeconds: 3600},
	{table: "channel_monitor_v2_user_metrics_rollup", retention: channelMonitorV2RetentionRollup1h, bucketSeconds: 3600},
	{table: "channel_monitor_v2_error_metrics_rollup", retention: channelMonitorV2RetentionRollup1h, bucketSeconds: 3600},
	{table: "channel_monitor_v2_latency_histograms_rollup", retention: channelMonitorV2RetentionRollup1h, bucketSeconds: 3600},
	{table: "channel_monitor_v2_metrics_rollup", retention: channelMonitorV2RetentionRollup12h, bucketSeconds: 43200},
	{table: "channel_monitor_v2_user_metrics_rollup", retention: channelMonitorV2RetentionRollup12h, bucketSeconds: 43200},
	{table: "channel_monitor_v2_error_metrics_rollup", retention: channelMonitorV2RetentionRollup12h, bucketSeconds: 43200},
	{table: "channel_monitor_v2_latency_histograms_rollup", retention: channelMonitorV2RetentionRollup12h, bucketSeconds: 43200},
	{table: "channel_monitor_v2_metrics_rollup", retention: channelMonitorV2RetentionRollup1d, bucketSeconds: 86400},
	{table: "channel_monitor_v2_user_metrics_rollup", retention: channelMonitorV2RetentionRollup1d, bucketSeconds: 86400},
	{table: "channel_monitor_v2_error_metrics_rollup", retention: channelMonitorV2RetentionRollup1d, bucketSeconds: 86400},
	{table: "channel_monitor_v2_latency_histograms_rollup", retention: channelMonitorV2RetentionRollup1d, bucketSeconds: 86400},
}

func (r *channelMonitorV2Repository) pruneChannelMonitorV2Retention(ctx context.Context, tx *sql.Tx, now time.Time) error {
	// During historical bootstrap, retain all 1m facts until the cursor reaches
	// the oldest rollup boundary. Otherwise adjacent chunks would rebuild the
	// same daily bucket from source rows already pruned by the prior chunk.
	var backfillCursor time.Time
	if err := tx.QueryRowContext(ctx, `SELECT backfill_cursor FROM channel_monitor_v2_watermarks WHERE id = 1`).Scan(&backfillCursor); err == nil && backfillCursor.After(channelMonitorV2RetentionCutoff(now, channelMonitorV2RetentionMax)) {
		return nil
	}
	for _, rule := range channelMonitorV2RetentionRules {
		cutoff := channelMonitorV2RetentionCutoff(now, rule.retention)
		var err error
		if rule.bucketSeconds == 0 {
			if IsMySQL() {
				_, err = tx.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s WHERE bucket_start < ?", rule.table), cutoff)
			} else {
				_, err = tx.ExecContext(ctx, fmt.Sprintf(`DELETE FROM %s WHERE bucket_start < $1`, rule.table), cutoff)
			}
		} else {
			if IsMySQL() {
				_, err = tx.ExecContext(ctx,
					fmt.Sprintf("DELETE FROM %s WHERE bucket_seconds = ? AND bucket_start < ?", rule.table),
					rule.bucketSeconds, cutoff,
				)
			} else {
				_, err = tx.ExecContext(ctx,
					fmt.Sprintf(`DELETE FROM %s WHERE bucket_seconds = $1 AND bucket_start < $2`, rule.table),
					rule.bucketSeconds, cutoff,
				)
			}
		}
		if err != nil {
			return fmt.Errorf("prune %s (bucket_seconds=%d): %w", rule.table, rule.bucketSeconds, err)
		}
	}
	return nil
}

func (r *channelMonitorV2Repository) RecomputeRange(ctx context.Context, start, end time.Time) (err error) {
	start = start.UTC().Truncate(time.Minute)
	end = end.UTC().Truncate(time.Minute)
	now := time.Now().UTC().Truncate(time.Minute)
	// Clamp to longest rollup TTL so backfill does not scan beyond product history.
	maxCutoff := channelMonitorV2RetentionCutoff(now, channelMonitorV2MaxRetention())
	if start.Before(maxCutoff) {
		start = maxCutoff
	}
	if !start.Before(end) {
		return nil
	}
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	// Idempotent window rewrite: drop existing facts/rollups in [start,end) then re-insert.
	for _, table := range []string{
		"channel_monitor_v2_latency_histograms_rollup",
		"channel_monitor_v2_error_metrics_rollup",
		"channel_monitor_v2_user_metrics_rollup",
		"channel_monitor_v2_metrics_rollup",
		"channel_monitor_v2_latency_histograms_1m",
		"channel_monitor_v2_error_metrics_1m",
		"channel_monitor_v2_user_metrics_1m",
		"channel_monitor_v2_metrics_1m",
	} {
		if IsMySQL() {
			if _, err = tx.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s WHERE bucket_start >= ? AND bucket_start < ?", table), start, end); err != nil {
				return err
			}
		} else {
			if _, err = tx.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s WHERE bucket_start >= $1 AND bucket_start < $2", table), start, end); err != nil {
				return err
			}
		}
	}

	if IsMySQL() {
		if _, err = tx.ExecContext(ctx, fmt.Sprintf(channelMonitorV2UsageMetricsSQLMySQL, channelMonitorV2PlatformSQL, channelMonitorV2ModelSQL, channelMonitorV2PlatformSQL, channelMonitorV2ModelSQL), start, end); err != nil {
			return fmt.Errorf("aggregate channel monitor v2 usage: %w", err)
		}
		if _, err = tx.ExecContext(ctx, fmt.Sprintf(channelMonitorV2UserMetricsSQLMySQL, channelMonitorV2PlatformSQL, channelMonitorV2ModelSQL, channelMonitorV2PlatformSQL, channelMonitorV2ModelSQL), start, end); err != nil {
			return fmt.Errorf("aggregate channel monitor v2 users: %w", err)
		}
		if _, err = tx.ExecContext(ctx, fmt.Sprintf(channelMonitorV2HistogramSQLMySQL, channelMonitorV2PlatformSQL, channelMonitorV2ModelSQL, channelMonitorV2HistogramBoundSQL("latency.value_ms"), channelMonitorV2PlatformSQL, channelMonitorV2ModelSQL, channelMonitorV2HistogramBoundSQL("latency.value_ms")), start, end); err != nil {
			return fmt.Errorf("aggregate channel monitor v2 histograms: %w", err)
		}
		if err = r.execChannelMonitorV2ErrorAggregationMySQL(ctx, tx, start, end); err != nil {
			return fmt.Errorf("aggregate channel monitor v2 errors: %w", err)
		}
	} else {
		if _, err = tx.ExecContext(ctx, fmt.Sprintf(channelMonitorV2UsageMetricsSQL, channelMonitorV2PlatformSQL, channelMonitorV2ModelSQL), start, end); err != nil {
			return fmt.Errorf("aggregate channel monitor v2 usage: %w", err)
		}
		if _, err = tx.ExecContext(ctx, fmt.Sprintf(channelMonitorV2UserMetricsSQL, channelMonitorV2PlatformSQL, channelMonitorV2ModelSQL), start, end); err != nil {
			return fmt.Errorf("aggregate channel monitor v2 users: %w", err)
		}
		if _, err = tx.ExecContext(ctx, fmt.Sprintf(channelMonitorV2HistogramSQL, channelMonitorV2PlatformSQL, channelMonitorV2ModelSQL, channelMonitorV2HistogramBoundSQL("latency.value_ms")), start, end); err != nil {
			return fmt.Errorf("aggregate channel monitor v2 histograms: %w", err)
		}
		if _, err = tx.ExecContext(ctx, channelMonitorV2ErrorAggregationSQL, start, end); err != nil {
			return fmt.Errorf("aggregate channel monitor v2 errors: %w", err)
		}
	}
	if err = r.recomputeFixedRollups(ctx, tx, start, end); err != nil {
		return err
	}
	// Drop rows past per-tier TTL (1m short, coarse rollups long). Safe after rollup
	// so a backfill chunk can build 1d rollups from temporary 1m rows then discard 1m.
	if err = r.pruneChannelMonitorV2Retention(ctx, tx, now); err != nil {
		return err
	}
	if IsMySQL() {
		if _, err = tx.ExecContext(ctx, channelMonitorV2WatermarkSQLMySQL, start, start, end, start); err != nil {
			return err
		}
	} else {
		if _, err = tx.ExecContext(ctx, channelMonitorV2WatermarkSQL, start, end); err != nil {
			return err
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return nil
}

const channelMonitorV2UsageMetricsSQL = `
INSERT INTO channel_monitor_v2_metrics_1m (
  bucket_start, platform, group_id, model, success_requests,
  input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens,
  ttft_sum_ms, ttft_count, duration_sum_ms, duration_count, computed_at
)
SELECT date_trunc('minute', ul.created_at), %s, COALESCE(ul.group_id, 0), %s,
       COUNT(DISTINCT COALESCE(NULLIF(ul.request_id, ''), 'usage:' || ul.id::text))
         FILTER (WHERE COALESCE(ul.request_type, 0) NOT IN (4, 6) AND ` + usageLogSuccessFilterUL + `),
       COALESCE(SUM(ul.input_tokens) FILTER (WHERE ` + usageLogSuccessFilterUL + `), 0),
       COALESCE(SUM(ul.output_tokens) FILTER (WHERE ` + usageLogSuccessFilterUL + `), 0),
       COALESCE(SUM(ul.cache_creation_tokens) FILTER (WHERE ` + usageLogSuccessFilterUL + `), 0),
       COALESCE(SUM(ul.cache_read_tokens) FILTER (WHERE ` + usageLogSuccessFilterUL + `), 0),
       COALESCE(SUM(ul.first_token_ms) FILTER (WHERE ul.first_token_ms IS NOT NULL AND ` + usageLogSuccessFilterUL + `), 0),
       COUNT(ul.first_token_ms) FILTER (WHERE ` + usageLogSuccessFilterUL + `),
       COALESCE(SUM(ul.duration_ms) FILTER (WHERE ul.duration_ms IS NOT NULL AND ` + usageLogSuccessFilterUL + `), 0),
       COUNT(ul.duration_ms) FILTER (WHERE ` + usageLogSuccessFilterUL + `), NOW()
FROM usage_logs ul
LEFT JOIN groups g ON g.id = ul.group_id
LEFT JOIN accounts a ON a.id = ul.account_id
WHERE ul.created_at >= $1 AND ul.created_at < $2
GROUP BY 1, 2, 3, 4`

const channelMonitorV2UserMetricsSQL = `
INSERT INTO channel_monitor_v2_user_metrics_1m (
  bucket_start, platform, group_id, model, user_id, success_requests,
  input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens,
  ttft_sum_ms, ttft_count, duration_sum_ms, duration_count, computed_at
)
SELECT date_trunc('minute', ul.created_at), %s, COALESCE(ul.group_id, 0), %s, ul.user_id,
       COUNT(DISTINCT COALESCE(NULLIF(ul.request_id, ''), 'usage:' || ul.id::text))
         FILTER (WHERE COALESCE(ul.request_type, 0) NOT IN (4, 6) AND ` + usageLogSuccessFilterUL + `),
       COALESCE(SUM(ul.input_tokens) FILTER (WHERE ` + usageLogSuccessFilterUL + `), 0),
       COALESCE(SUM(ul.output_tokens) FILTER (WHERE ` + usageLogSuccessFilterUL + `), 0),
       COALESCE(SUM(ul.cache_creation_tokens) FILTER (WHERE ` + usageLogSuccessFilterUL + `), 0),
       COALESCE(SUM(ul.cache_read_tokens) FILTER (WHERE ` + usageLogSuccessFilterUL + `), 0),
       COALESCE(SUM(ul.first_token_ms) FILTER (WHERE ul.first_token_ms IS NOT NULL AND ` + usageLogSuccessFilterUL + `), 0),
       COUNT(ul.first_token_ms) FILTER (WHERE ` + usageLogSuccessFilterUL + `),
       COALESCE(SUM(ul.duration_ms) FILTER (WHERE ul.duration_ms IS NOT NULL AND ` + usageLogSuccessFilterUL + `), 0),
       COUNT(ul.duration_ms) FILTER (WHERE ` + usageLogSuccessFilterUL + `), NOW()
FROM usage_logs ul
LEFT JOIN groups g ON g.id = ul.group_id
LEFT JOIN accounts a ON a.id = ul.account_id
WHERE ul.created_at >= $1 AND ul.created_at < $2 AND ul.user_id IS NOT NULL
GROUP BY 1, 2, 3, 4, 5`

const channelMonitorV2HistogramSQL = `
INSERT INTO channel_monitor_v2_latency_histograms_1m (
  bucket_start, platform, group_id, model, user_id, metric, upper_bound_ms, sample_count
)
SELECT date_trunc('minute', ul.created_at), %s, COALESCE(ul.group_id, 0), %s,
       audience.user_id, latency.metric, %s, COUNT(*)
FROM usage_logs ul
LEFT JOIN groups g ON g.id = ul.group_id
LEFT JOIN accounts a ON a.id = ul.account_id
CROSS JOIN LATERAL (VALUES (0::bigint), (ul.user_id)) audience(user_id)
CROSS JOIN LATERAL (VALUES ('ttft'::text, ul.first_token_ms), ('duration'::text, ul.duration_ms)) latency(metric, value_ms)
WHERE ul.created_at >= $1 AND ul.created_at < $2
  AND audience.user_id IS NOT NULL AND latency.value_ms IS NOT NULL AND latency.value_ms >= 0
  AND ` + usageLogSuccessFilterUL + `
GROUP BY 1, 2, 3, 4, 5, 6, 7`

// channelMonitorV2UsageMetricsSQLMySQL 是 channelMonitorV2UsageMetricsSQL 的 MySQL 版，
// SELECT 列表与列语义逐一对齐：
//   - date_trunc('minute', ...) → DATE_FORMAT(..., '%Y-%m-%d %H:%i:00')（经 Sprintf 拼装故 %% 转义）；
//   - COUNT(DISTINCT ...) FILTER → COUNT(DISTINCT CASE WHEN cond THEN key END)（非命中行为 NULL，不计数）；
//   - SUM(col) FILTER → SUM(CASE WHEN cond THEN col END)（NULL 由 SUM 自动忽略）；
//   - COUNT(col) FILTER → COUNT(CASE WHEN cond THEN col END)；
//   - 'usage:' || ul.id::text → CONCAT('usage:', ul.id)；NOW() → NOW(3)；$n → ?；
//   - `groups` 为 MySQL 8 保留字加反引号；GROUP BY 重复 SELECT 表达式（避免别名歧义）。
const channelMonitorV2UsageMetricsSQLMySQL = `
INSERT INTO channel_monitor_v2_metrics_1m (
  bucket_start, platform, group_id, model, success_requests,
  input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens,
  ttft_sum_ms, ttft_count, duration_sum_ms, duration_count, computed_at
)
SELECT DATE_FORMAT(ul.created_at, '%%Y-%%m-%%d %%H:%%i:00'), %s, COALESCE(ul.group_id, 0), %s,
       COUNT(DISTINCT CASE WHEN COALESCE(ul.request_type, 0) NOT IN (4, 6) AND ` + usageLogSuccessFilterUL + `
         THEN COALESCE(NULLIF(ul.request_id, ''), CONCAT('usage:', ul.id)) END),
       COALESCE(SUM(CASE WHEN ` + usageLogSuccessFilterUL + ` THEN ul.input_tokens END), 0),
       COALESCE(SUM(CASE WHEN ` + usageLogSuccessFilterUL + ` THEN ul.output_tokens END), 0),
       COALESCE(SUM(CASE WHEN ` + usageLogSuccessFilterUL + ` THEN ul.cache_creation_tokens END), 0),
       COALESCE(SUM(CASE WHEN ` + usageLogSuccessFilterUL + ` THEN ul.cache_read_tokens END), 0),
       COALESCE(SUM(CASE WHEN ul.first_token_ms IS NOT NULL AND ` + usageLogSuccessFilterUL + ` THEN ul.first_token_ms END), 0),
       COUNT(CASE WHEN ` + usageLogSuccessFilterUL + ` THEN ul.first_token_ms END),
       COALESCE(SUM(CASE WHEN ul.duration_ms IS NOT NULL AND ` + usageLogSuccessFilterUL + ` THEN ul.duration_ms END), 0),
       COUNT(CASE WHEN ` + usageLogSuccessFilterUL + ` THEN ul.duration_ms END), NOW(3)
FROM usage_logs ul
LEFT JOIN ` + "`groups`" + ` g ON g.id = ul.group_id
LEFT JOIN accounts a ON a.id = ul.account_id
WHERE ul.created_at >= ? AND ul.created_at < ?
GROUP BY DATE_FORMAT(ul.created_at, '%%Y-%%m-%%d %%H:%%i:00'), %s, COALESCE(ul.group_id, 0), %s`

// channelMonitorV2UserMetricsSQLMySQL 是 channelMonitorV2UserMetricsSQL 的 MySQL 版，
// 改写规则同 channelMonitorV2UsageMetricsSQLMySQL。
const channelMonitorV2UserMetricsSQLMySQL = `
INSERT INTO channel_monitor_v2_user_metrics_1m (
  bucket_start, platform, group_id, model, user_id, success_requests,
  input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens,
  ttft_sum_ms, ttft_count, duration_sum_ms, duration_count, computed_at
)
SELECT DATE_FORMAT(ul.created_at, '%%Y-%%m-%%d %%H:%%i:00'), %s, COALESCE(ul.group_id, 0), %s, ul.user_id,
       COUNT(DISTINCT CASE WHEN COALESCE(ul.request_type, 0) NOT IN (4, 6) AND ` + usageLogSuccessFilterUL + `
         THEN COALESCE(NULLIF(ul.request_id, ''), CONCAT('usage:', ul.id)) END),
       COALESCE(SUM(CASE WHEN ` + usageLogSuccessFilterUL + ` THEN ul.input_tokens END), 0),
       COALESCE(SUM(CASE WHEN ` + usageLogSuccessFilterUL + ` THEN ul.output_tokens END), 0),
       COALESCE(SUM(CASE WHEN ` + usageLogSuccessFilterUL + ` THEN ul.cache_creation_tokens END), 0),
       COALESCE(SUM(CASE WHEN ` + usageLogSuccessFilterUL + ` THEN ul.cache_read_tokens END), 0),
       COALESCE(SUM(CASE WHEN ul.first_token_ms IS NOT NULL AND ` + usageLogSuccessFilterUL + ` THEN ul.first_token_ms END), 0),
       COUNT(CASE WHEN ` + usageLogSuccessFilterUL + ` THEN ul.first_token_ms END),
       COALESCE(SUM(CASE WHEN ul.duration_ms IS NOT NULL AND ` + usageLogSuccessFilterUL + ` THEN ul.duration_ms END), 0),
       COUNT(CASE WHEN ` + usageLogSuccessFilterUL + ` THEN ul.duration_ms END), NOW(3)
FROM usage_logs ul
LEFT JOIN ` + "`groups`" + ` g ON g.id = ul.group_id
LEFT JOIN accounts a ON a.id = ul.account_id
WHERE ul.created_at >= ? AND ul.created_at < ? AND ul.user_id IS NOT NULL
GROUP BY DATE_FORMAT(ul.created_at, '%%Y-%%m-%%d %%H:%%i:00'), %s, COALESCE(ul.group_id, 0), %s, ul.user_id`

// channelMonitorV2HistogramSQLMySQL 是 channelMonitorV2HistogramSQL 的 MySQL 版：
//   - CROSS JOIN LATERAL (VALUES (0::bigint), (ul.user_id)) → CROSS JOIN JSON_TABLE(
//     JSON_ARRAY(0, ul.user_id), '$[*]' ...)：user_id 为 NULL 时 JSON 数组产生 null 元素，
//     展开行得到 NULL 并被 `audience.user_id IS NOT NULL` 过滤，与 PG VALUES 行为一致；
//   - CROSS JOIN LATERAL (VALUES ('ttft', first_token_ms), ('duration', duration_ms)) →
//     CROSS JOIN JSON_TABLE(JSON_ARRAY(JSON_OBJECT(...), JSON_OBJECT(...)), '$[*]' ...)，
//     值为 NULL 的行被 `latency.value_ms IS NOT NULL` 过滤；
//   - 其余改写同 channelMonitorV2UsageMetricsSQLMySQL。
const channelMonitorV2HistogramSQLMySQL = `
INSERT INTO channel_monitor_v2_latency_histograms_1m (
  bucket_start, platform, group_id, model, user_id, metric, upper_bound_ms, sample_count
)
SELECT DATE_FORMAT(ul.created_at, '%%Y-%%m-%%d %%H:%%i:00'), %s, COALESCE(ul.group_id, 0), %s,
       audience.user_id, latency.metric, %s, COUNT(*)
FROM usage_logs ul
LEFT JOIN ` + "`groups`" + ` g ON g.id = ul.group_id
LEFT JOIN accounts a ON a.id = ul.account_id
CROSS JOIN JSON_TABLE(
  JSON_ARRAY(0, ul.user_id),
  '$[*]' COLUMNS (user_id BIGINT PATH '$')
) audience
CROSS JOIN JSON_TABLE(
  JSON_ARRAY(
    JSON_OBJECT('metric', 'ttft', 'value', ul.first_token_ms),
    JSON_OBJECT('metric', 'duration', 'value', ul.duration_ms)
  ),
  '$[*]' COLUMNS (metric VARCHAR(20) PATH '$.metric', value_ms BIGINT PATH '$.value')
) latency
WHERE ul.created_at >= ? AND ul.created_at < ?
  AND audience.user_id IS NOT NULL AND latency.value_ms IS NOT NULL AND latency.value_ms >= 0
  AND ` + usageLogSuccessFilterUL + `
GROUP BY DATE_FORMAT(ul.created_at, '%%Y-%%m-%%d %%H:%%i:00'), %s, COALESCE(ul.group_id, 0), %s,
         audience.user_id, latency.metric, %s`

func channelMonitorV2HistogramBoundSQL(column string) string {
	return `CASE
WHEN ` + column + ` <= 50 THEN 50 WHEN ` + column + ` <= 100 THEN 100
WHEN ` + column + ` <= 250 THEN 250 WHEN ` + column + ` <= 500 THEN 500
WHEN ` + column + ` <= 1000 THEN 1000 WHEN ` + column + ` <= 2000 THEN 2000
WHEN ` + column + ` <= 3000 THEN 3000 WHEN ` + column + ` <= 5000 THEN 5000
WHEN ` + column + ` <= 8000 THEN 8000 WHEN ` + column + ` <= 10000 THEN 10000
WHEN ` + column + ` <= 15000 THEN 15000 WHEN ` + column + ` <= 30000 THEN 30000
WHEN ` + column + ` <= 60000 THEN 60000 WHEN ` + column + ` <= 120000 THEN 120000
WHEN ` + column + ` <= 300000 THEN 300000 WHEN ` + column + ` <= 600000 THEN 600000
ELSE 2147483647 END`
}

// Error dedup lookback: request_id branch is bounded by chunk start minus 90
// minutes so candidate_ids never forces a full-history scan of ops_error_logs.
const channelMonitorV2ErrorAggregationSQL = `
WITH dedup AS (
  WITH candidate_ids AS MATERIALIZED (
    SELECT DISTINCT request_id
    FROM ops_error_logs
    WHERE created_at >= $1 AND created_at < $2 AND NULLIF(request_id, '') IS NOT NULL
  )
  SELECT DISTINCT ON (COALESCE(NULLIF(current_error.request_id, ''), 'error:' || current_error.id::text))
    date_trunc('minute', current_error.created_at) AS bucket_start,
    -- Composite groups are a routing layer: resolve the concrete account
    -- platform (mirrors usageLogEffectivePlatformExpr on the usage side) so
    -- error facts share the usage facts' platform key. Without this, composite
    -- group errors aggregate under platform 'composite', which is never an
    -- enabled config platform, and are filtered out of every monitor v2 query.
    lower(CASE
      WHEN g.platform = 'composite' THEN COALESCE(NULLIF(TRIM(a.platform), ''), NULLIF(NULLIF(lower(TRIM(current_error.platform)), ''), 'composite'), 'unknown')
      ELSE COALESCE(NULLIF(TRIM(current_error.platform), ''), 'unknown')
    END) AS platform,
    COALESCE(current_error.group_id, 0) AS group_id,
    COALESCE(NULLIF(TRIM(current_error.requested_model), ''), NULLIF(TRIM(current_error.model), ''), 'unknown') AS model,
    current_error.user_id, current_error.error_type, current_error.error_owner, COALESCE(current_error.status_code, 0) AS status_code,
    COALESCE(current_error.upstream_status_code, 0) AS upstream_status_code,
    lower(CONCAT_WS(' ', current_error.error_type, current_error.error_source, current_error.error_message, current_error.upstream_error_message, current_error.upstream_error_detail, current_error.error_body)) AS text,
    (CASE WHEN jsonb_typeof(current_error.upstream_errors) = 'array' THEN jsonb_array_length(current_error.upstream_errors) > 0 ELSE FALSE END
      OR current_error.error_owner = 'provider' OR current_error.upstream_status_code IS NOT NULL) AS upstream_affected,
    CASE WHEN jsonb_typeof(current_error.upstream_errors) = 'array' THEN jsonb_array_length(current_error.upstream_errors) ELSE 0 END AS upstream_attempts
  FROM ops_error_logs current_error
  LEFT JOIN groups g ON g.id = current_error.group_id
  LEFT JOIN accounts a ON a.id = current_error.account_id
  WHERE (
      (NULLIF(current_error.request_id, '') IS NULL AND current_error.created_at >= $1 AND current_error.created_at < $2)
      OR (
        current_error.request_id IN (SELECT request_id FROM candidate_ids)
        AND current_error.created_at >= $1 - INTERVAL '90 minutes'
        AND current_error.created_at < $2
      )
    )
    AND NOT current_error.is_count_tokens
    AND (COALESCE(current_error.status_code, 0) >= 400 OR current_error.error_type = 'cyber_policy')
  ORDER BY COALESCE(NULLIF(current_error.request_id, ''), 'error:' || current_error.id::text), current_error.created_at DESC, current_error.id DESC
), classified AS (
  SELECT *, CASE
    -- Keep in lockstep with service.ClassifyChannelMonitorV2Error needles.
    WHEN error_type = 'cyber_policy' OR text LIKE ANY(ARRAY['%content policy%','%content_policy%','%safety policy%','%moderation%','%blocked keyword%']) THEN 'content_policy'
    WHEN status_code = 401 OR upstream_status_code = 401 OR text LIKE ANY(ARRAY['%unauthorized%','%invalid api key%','%invalid_api_key%','%authentication%','%api_key_disabled%']) THEN 'authentication'
    WHEN text LIKE ANY(ARRAY['%context window%','%context length%','%maximum prompt length%','%too many tokens%','%max_tokens%']) THEN 'context_limit'
    WHEN text LIKE ANY(ARRAY['%failed to deserialize%','%missing required parameter%','%invalid request%','%invalid_request%','%tool_choice%']) THEN 'invalid_request'
    WHEN text LIKE ANY(ARRAY['%does not support the requested model%','%not supported by any configured account%','%model not supported%','%unsupported model%']) THEN 'model_unsupported'
    WHEN text LIKE ANY(ARRAY['%group not allowed%','%group_not_allowed%','%group access%']) THEN 'group_access'
    WHEN text LIKE ANY(ARRAY['%run out of credits%','%insufficient balance%','%insufficient quota%','%subscription%','%quota exceeded%','%billing hard limit%']) THEN 'quota_or_balance'
    WHEN text LIKE ANY(ARRAY['%no available accounts%','%no healthy account%','%no healthy upstream account%','%failover budget exhausted%','%account pool%']) THEN 'account_pool_unavailable'
    WHEN status_code = 429 OR upstream_status_code = 429 OR text LIKE ANY(ARRAY['%rate limit%','%rate_limit%','%high demand%','%overloaded%','%concurrency limit%','%capacity%']) THEN 'rate_or_capacity'
    WHEN status_code IN (408,504) OR text LIKE ANY(ARRAY['%timeout%','%deadline exceeded%','%error code: 524%','%gateway time-out%','%gateway timeout%']) THEN 'timeout'
    WHEN text LIKE ANY(ARRAY['%transport%','%stream_read_error%','%connection reset%','%connection refused%','%tls%','%http2%','%missing terminal event%','%unexpected eof%']) THEN 'transport_or_stream'
    WHEN status_code = 403 OR upstream_status_code = 403 THEN 'upstream_forbidden'
    WHEN status_code = 404 OR upstream_status_code = 404 THEN 'not_found'
    WHEN status_code = 499 OR text LIKE ANY(ARRAY['%client cancelled%','%client canceled%','%context canceled%']) THEN 'client_cancelled'
    WHEN upstream_status_code >= 500 OR (error_owner = 'provider' AND status_code >= 500) THEN 'upstream_5xx'
    WHEN status_code >= 500 OR error_type = 'internal' OR error_owner = 'system' THEN 'internal'
    ELSE 'other' END AS category
  FROM dedup
  WHERE bucket_start >= $1 AND bucket_start < $2
), metric_rows AS (
  INSERT INTO channel_monitor_v2_metrics_1m (bucket_start, platform, group_id, model, error_requests, upstream_affected_requests, upstream_attempt_count, computed_at)
  SELECT bucket_start, platform, group_id, model, COUNT(*), COUNT(*) FILTER (WHERE upstream_affected), SUM(upstream_attempts), NOW()
  FROM classified GROUP BY 1,2,3,4
  ON CONFLICT (bucket_start, platform, group_id, model) DO UPDATE SET
    error_requests = EXCLUDED.error_requests, upstream_affected_requests = EXCLUDED.upstream_affected_requests,
    upstream_attempt_count = EXCLUDED.upstream_attempt_count, computed_at = NOW()
), user_rows AS (
  INSERT INTO channel_monitor_v2_user_metrics_1m (bucket_start, platform, group_id, model, user_id, error_requests, computed_at)
  SELECT bucket_start, platform, group_id, model, user_id, COUNT(*), NOW()
  FROM classified WHERE user_id IS NOT NULL GROUP BY 1,2,3,4,5
  ON CONFLICT (bucket_start, platform, group_id, model, user_id) DO UPDATE SET error_requests = EXCLUDED.error_requests, computed_at = NOW()
)
INSERT INTO channel_monitor_v2_error_metrics_1m (bucket_start, platform, group_id, model, error_category, taxonomy_version, error_requests)
SELECT bucket_start, platform, group_id, model, category, 1, COUNT(*) FROM classified GROUP BY 1,2,3,4,5
ON CONFLICT (bucket_start, platform, group_id, model, error_category, taxonomy_version)
DO UPDATE SET error_requests = EXCLUDED.error_requests`

// channelMonitorV2TextLikeAnyMySQL 把 PG `col LIKE ANY(ARRAY[...])` 展开为
// OR 连接的 LIKE 链（utf8mb4_0900_ai_ci 比较天然大小写不敏感，text 列已 lower 归一）。
func channelMonitorV2TextLikeAnyMySQL(column string, patterns []string) string {
	parts := make([]string, 0, len(patterns))
	for _, pattern := range patterns {
		parts = append(parts, column+" LIKE '"+pattern+"'")
	}
	return "(" + strings.Join(parts, " OR ") + ")"
}

// execChannelMonitorV2ErrorAggregationMySQL 是 channelMonitorV2ErrorAggregationSQL 的 MySQL 实现，
// 逐条语句在同事务（tx）内执行。与 PG 版的语义对应关系：
//   - metric_rows / user_rows 两个可写 CTE 无 MySQL 对应，按规范拆为多条语句；
//     classified 被三条 INSERT 复用 → 物化为会话级临时表（事务内建、用后删除）；
//   - WITH candidate_ids AS MATERIALIZED → 内联为 DISTINCT 子查询（语义相同）；
//   - DISTINCT ON (key) ... ORDER BY key, created_at DESC, id DESC →
//     ROW_NUMBER() OVER (PARTITION BY key ORDER BY created_at DESC, id DESC) 取 rn=1；
//   - jsonb_typeof(x) = 'array' → JSON_TYPE(x) = 'ARRAY'（大写）；jsonb_array_length → JSON_LENGTH；
//   - text LIKE ANY(ARRAY[...]) → channelMonitorV2TextLikeAnyMySQL 展开；
//   - 'error:' || id::text → CONCAT('error:', id)；NOW() → NOW(3)；$n → ?；
//     $1 - INTERVAL '90 minutes' → ? - INTERVAL 90 MINUTE；
//   - ON CONFLICT ... DO UPDATE → ODKU（EXCLUDED.* → VALUES(...)）。
//
// 临时表放在 defer 中 DROP：失败路径下 tx 随后由调用方回滚，同连接下次执行前 IF EXISTS 兜底。
// REVIEW: bucket_start 在临时表中以 CAST(... AS DATETIME) 固化类型；若 ops_error_logs.created_at
// 精度未来调整为微秒，date_trunc('minute') 与 DATE_FORMAT 的对齐仍一致，无需改动。
func (r *channelMonitorV2Repository) execChannelMonitorV2ErrorAggregationMySQL(ctx context.Context, tx *sql.Tx, start, end time.Time) error {
	defer func() {
		_, _ = tx.ExecContext(ctx, "DROP TEMPORARY TABLE IF EXISTS channel_monitor_v2_err_classified")
		_, _ = tx.ExecContext(ctx, "DROP TEMPORARY TABLE IF EXISTS channel_monitor_v2_err_dedup")
	}()

	likeAny := func(patterns []string) string {
		return channelMonitorV2TextLikeAnyMySQL("`text`", patterns)
	}
	// 与 PG 版 classified CTE 的 CASE 分支保持逐条对应（含服务层
	// service.ClassifyChannelMonitorV2Error 的 needle 列表）。
	categoryCase := `CASE
    WHEN error_type = 'cyber_policy' OR ` + likeAny([]string{"%content policy%", "%content_policy%", "%safety policy%", "%moderation%", "%blocked keyword%"}) + ` THEN 'content_policy'
    WHEN status_code = 401 OR upstream_status_code = 401 OR ` + likeAny([]string{"%unauthorized%", "%invalid api key%", "%invalid_api_key%", "%authentication%", "%api_key_disabled%"}) + ` THEN 'authentication'
    WHEN ` + likeAny([]string{"%context window%", "%context length%", "%maximum prompt length%", "%too many tokens%", "%max_tokens%"}) + ` THEN 'context_limit'
    WHEN ` + likeAny([]string{"%failed to deserialize%", "%missing required parameter%", "%invalid request%", "%invalid_request%", "%tool_choice%"}) + ` THEN 'invalid_request'
    WHEN ` + likeAny([]string{"%does not support the requested model%", "%not supported by any configured account%", "%model not supported%", "%unsupported model%"}) + ` THEN 'model_unsupported'
    WHEN ` + likeAny([]string{"%group not allowed%", "%group_not_allowed%", "%group access%"}) + ` THEN 'group_access'
    WHEN ` + likeAny([]string{"%run out of credits%", "%insufficient balance%", "%insufficient quota%", "%subscription%", "%quota exceeded%", "%billing hard limit%"}) + ` THEN 'quota_or_balance'
    WHEN ` + likeAny([]string{"%no available accounts%", "%no healthy account%", "%no healthy upstream account%", "%failover budget exhausted%", "%account pool%"}) + ` THEN 'account_pool_unavailable'
    WHEN status_code = 429 OR upstream_status_code = 429 OR ` + likeAny([]string{"%rate limit%", "%rate_limit%", "%high demand%", "%overloaded%", "%concurrency limit%", "%capacity%"}) + ` THEN 'rate_or_capacity'
    WHEN status_code IN (408, 504) OR ` + likeAny([]string{"%timeout%", "%deadline exceeded%", "%error code: 524%", "%gateway time-out%", "%gateway timeout%"}) + ` THEN 'timeout'
    WHEN ` + likeAny([]string{"%transport%", "%stream_read_error%", "%connection reset%", "%connection refused%", "%tls%", "%http2%", "%missing terminal event%", "%unexpected eof%"}) + ` THEN 'transport_or_stream'
    WHEN status_code = 403 OR upstream_status_code = 403 THEN 'upstream_forbidden'
    WHEN status_code = 404 OR upstream_status_code = 404 THEN 'not_found'
    WHEN status_code = 499 OR ` + likeAny([]string{"%client cancelled%", "%client canceled%", "%context canceled%"}) + ` THEN 'client_cancelled'
    WHEN upstream_status_code >= 500 OR (error_owner = 'provider' AND status_code >= 500) THEN 'upstream_5xx'
    WHEN status_code >= 500 OR error_type = 'internal' OR error_owner = 'system' THEN 'internal'
    ELSE 'other' END`

	dedupSQL := `
CREATE TEMPORARY TABLE channel_monitor_v2_err_dedup AS
SELECT bucket_start, platform, group_id, model, user_id, error_type, error_owner,
       status_code, upstream_status_code, ` + "`text`" + `, upstream_affected, upstream_attempts
FROM (
  SELECT
    CAST(DATE_FORMAT(current_error.created_at, '%Y-%m-%d %H:%i:00') AS DATETIME) AS bucket_start,
    -- Composite groups are a routing layer: resolve the concrete account
    -- platform (mirrors usageLogEffectivePlatformExpr on the usage side) so
    -- error facts share the usage facts' platform key.
    lower(CASE
      WHEN g.platform = 'composite' THEN COALESCE(NULLIF(TRIM(a.platform), ''), NULLIF(NULLIF(lower(TRIM(current_error.platform)), ''), 'composite'), 'unknown')
      ELSE COALESCE(NULLIF(TRIM(current_error.platform), ''), 'unknown')
    END) AS platform,
    COALESCE(current_error.group_id, 0) AS group_id,
    COALESCE(NULLIF(TRIM(current_error.requested_model), ''), NULLIF(TRIM(current_error.model), ''), 'unknown') AS model,
    current_error.user_id, current_error.error_type, current_error.error_owner, COALESCE(current_error.status_code, 0) AS status_code,
    COALESCE(current_error.upstream_status_code, 0) AS upstream_status_code,
    lower(CONCAT_WS(' ', current_error.error_type, current_error.error_source, current_error.error_message, current_error.upstream_error_message, current_error.upstream_error_detail, current_error.error_body)) AS ` + "`text`" + `,
    (CASE WHEN JSON_TYPE(current_error.upstream_errors) = 'ARRAY' THEN JSON_LENGTH(current_error.upstream_errors) > 0 ELSE FALSE END
      OR current_error.error_owner = 'provider' OR current_error.upstream_status_code IS NOT NULL) AS upstream_affected,
    CASE WHEN JSON_TYPE(current_error.upstream_errors) = 'ARRAY' THEN JSON_LENGTH(current_error.upstream_errors) ELSE 0 END AS upstream_attempts,
    ROW_NUMBER() OVER (PARTITION BY COALESCE(NULLIF(current_error.request_id, ''), CONCAT('error:', current_error.id))
                       ORDER BY current_error.created_at DESC, current_error.id DESC) AS rn
  FROM ops_error_logs current_error
  LEFT JOIN ` + "`groups`" + ` g ON g.id = current_error.group_id
  LEFT JOIN accounts a ON a.id = current_error.account_id
  WHERE (
      (NULLIF(current_error.request_id, '') IS NULL AND current_error.created_at >= ? AND current_error.created_at < ?)
      OR (
        current_error.request_id IN (
          SELECT DISTINCT request_id FROM ops_error_logs
          WHERE created_at >= ? AND created_at < ? AND NULLIF(request_id, '') IS NOT NULL
        )
        AND current_error.created_at >= ? - INTERVAL 90 MINUTE
        AND current_error.created_at < ?
      )
    )
    AND NOT current_error.is_count_tokens
    AND (COALESCE(current_error.status_code, 0) >= 400 OR current_error.error_type = 'cyber_policy')
) ranked
WHERE rn = 1`
	if _, err := tx.ExecContext(ctx, dedupSQL, start, end, start, end, start, end); err != nil {
		return fmt.Errorf("dedup channel monitor v2 errors: %w", err)
	}

	classifiedSQL := `
CREATE TEMPORARY TABLE channel_monitor_v2_err_classified AS
SELECT d.*, ` + categoryCase + ` AS category
FROM channel_monitor_v2_err_dedup d
WHERE d.bucket_start >= ? AND d.bucket_start < ?`
	if _, err := tx.ExecContext(ctx, classifiedSQL, start, end); err != nil {
		return fmt.Errorf("classify channel monitor v2 errors: %w", err)
	}

	metricRowsSQL := `
INSERT INTO channel_monitor_v2_metrics_1m (bucket_start, platform, group_id, model, error_requests, upstream_affected_requests, upstream_attempt_count, computed_at)
SELECT bucket_start, platform, group_id, model, COUNT(*), SUM(CASE WHEN upstream_affected THEN 1 ELSE 0 END), SUM(upstream_attempts), NOW(3)
FROM channel_monitor_v2_err_classified GROUP BY bucket_start, platform, group_id, model
ON DUPLICATE KEY UPDATE
  error_requests = VALUES(error_requests), upstream_affected_requests = VALUES(upstream_affected_requests),
  upstream_attempt_count = VALUES(upstream_attempt_count), computed_at = NOW(3)`
	if _, err := tx.ExecContext(ctx, metricRowsSQL); err != nil {
		return fmt.Errorf("aggregate channel monitor v2 error metrics: %w", err)
	}

	userRowsSQL := `
INSERT INTO channel_monitor_v2_user_metrics_1m (bucket_start, platform, group_id, model, user_id, error_requests, computed_at)
SELECT bucket_start, platform, group_id, model, user_id, COUNT(*), NOW(3)
FROM channel_monitor_v2_err_classified WHERE user_id IS NOT NULL GROUP BY bucket_start, platform, group_id, model, user_id
ON DUPLICATE KEY UPDATE error_requests = VALUES(error_requests), computed_at = NOW(3)`
	if _, err := tx.ExecContext(ctx, userRowsSQL); err != nil {
		return fmt.Errorf("aggregate channel monitor v2 error users: %w", err)
	}

	errorRowsSQL := `
INSERT INTO channel_monitor_v2_error_metrics_1m (bucket_start, platform, group_id, model, error_category, taxonomy_version, error_requests)
SELECT bucket_start, platform, group_id, model, category, 1, COUNT(*) FROM channel_monitor_v2_err_classified
GROUP BY bucket_start, platform, group_id, model, category
ON DUPLICATE KEY UPDATE error_requests = VALUES(error_requests)`
	if _, err := tx.ExecContext(ctx, errorRowsSQL); err != nil {
		return fmt.Errorf("aggregate channel monitor v2 error categories: %w", err)
	}
	return nil
}

// Floor matches channelMonitorV2RetentionMax (90d). Keep the INTERVAL literal in
// sync when changing channelMonitorV2RetentionRollup1d.
//
// Coverage starts track how far back recompute has walked ($1 = chunk start), not
// "min(source_log.created_at)". Using global min(ops_error_logs) pins
// error_coverage_start to the first real error forever and collapses UI windows
// when errors only exist in a recent slice (common on first upgrade).
const channelMonitorV2WatermarkSQL = `
INSERT INTO channel_monitor_v2_watermarks (id, usage_coverage_start, error_coverage_start, data_through, last_successful_at, backfill_cursor, updated_at)
VALUES (
  1,
  $1,
  $1,
  $2, NOW(), $1, NOW()
)
ON CONFLICT (id) DO UPDATE SET
  usage_coverage_start = GREATEST(
    date_trunc('minute', NOW()) - INTERVAL '90 days',
    LEAST(COALESCE(channel_monitor_v2_watermarks.usage_coverage_start, EXCLUDED.usage_coverage_start), EXCLUDED.usage_coverage_start)
  ),
  error_coverage_start = GREATEST(
    date_trunc('minute', NOW()) - INTERVAL '90 days',
    LEAST(COALESCE(channel_monitor_v2_watermarks.error_coverage_start, EXCLUDED.error_coverage_start), EXCLUDED.error_coverage_start)
  ),
  data_through = GREATEST(COALESCE(channel_monitor_v2_watermarks.data_through, EXCLUDED.data_through), EXCLUDED.data_through),
  last_successful_at = NOW(),
  backfill_cursor = LEAST(COALESCE(channel_monitor_v2_watermarks.backfill_cursor, EXCLUDED.backfill_cursor), EXCLUDED.backfill_cursor),
  updated_at = NOW()`

// channelMonitorV2WatermarkSQLMySQL 是 channelMonitorV2WatermarkSQL 的 MySQL 版：
//   - ON CONFLICT (id) DO UPDATE → ODKU，EXCLUDED.* → VALUES(...)；
//   - date_trunc('minute', NOW()) → CAST(DATE_FORMAT(NOW(3), '%Y-%m-%d %H:%i:00') AS DATETIME)；
//   - INTERVAL '90 days' → INTERVAL 90 DAY；NOW() → NOW(3)；$n → ?。
//
// LEAST/GREATEST 的参数经 COALESCE 保证非 NULL（回退值为本次插入的非空参数），
// 与 PG「忽略 NULL」语义一致；VALUES(*) 列顺序：usage_coverage_start、error_coverage_start、
// data_through、backfill_cursor。
const channelMonitorV2WatermarkSQLMySQL = `
INSERT INTO channel_monitor_v2_watermarks (id, usage_coverage_start, error_coverage_start, data_through, last_successful_at, backfill_cursor, updated_at)
VALUES (
  1,
  ?,
  ?,
  ?, NOW(3), ?, NOW(3)
)
ON DUPLICATE KEY UPDATE
  usage_coverage_start = GREATEST(
    CAST(DATE_FORMAT(NOW(3), '%Y-%m-%d %H:%i:00') AS DATETIME) - INTERVAL 90 DAY,
    LEAST(COALESCE(usage_coverage_start, VALUES(usage_coverage_start)), VALUES(usage_coverage_start))
  ),
  error_coverage_start = GREATEST(
    CAST(DATE_FORMAT(NOW(3), '%Y-%m-%d %H:%i:00') AS DATETIME) - INTERVAL 90 DAY,
    LEAST(COALESCE(error_coverage_start, VALUES(error_coverage_start)), VALUES(error_coverage_start))
  ),
  data_through = GREATEST(COALESCE(data_through, VALUES(data_through)), VALUES(data_through)),
  last_successful_at = NOW(3),
  backfill_cursor = LEAST(COALESCE(backfill_cursor, VALUES(backfill_cursor)), VALUES(backfill_cursor)),
  updated_at = NOW(3)`

var channelMonitorV2FixedRollupSeconds = []int{300, 3600, 43200, 86400}

func (r *channelMonitorV2Repository) recomputeFixedRollups(ctx context.Context, tx *sql.Tx, start, end time.Time) error {
	for _, seconds := range channelMonitorV2FixedRollupSeconds {
		// Coarse buckets are immutable between boundaries during the normal
		// trailing refresh. Historical backfills and boundary-crossing windows
		// still rebuild them; this avoids repeatedly regrouping the full current
		// day/user table every few minutes.
		if seconds >= 43200 && sameFixedRollupBucket(start, end, seconds) {
			continue
		}
		if IsMySQL() {
			if err := r.recomputeFixedRollupMySQL(ctx, tx, start, end, seconds); err != nil {
				return err
			}
			continue
		}
		interval := fmt.Sprintf("%d seconds", seconds)
		for _, table := range []string{
			"channel_monitor_v2_latency_histograms_rollup",
			"channel_monitor_v2_error_metrics_rollup",
			"channel_monitor_v2_user_metrics_rollup",
			"channel_monitor_v2_metrics_rollup",
		} {
			if _, err := tx.ExecContext(ctx, fmt.Sprintf(channelMonitorV2FixedRollupDeleteSQL, table), interval, seconds, start, end); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, channelMonitorV2MetricsRollupSQL, interval, seconds, start, end); err != nil {
			return fmt.Errorf("roll up channel monitor v2 metrics %ds: %w", seconds, err)
		}
		if _, err := tx.ExecContext(ctx, channelMonitorV2UserMetricsRollupSQL, interval, seconds, start, end); err != nil {
			return fmt.Errorf("roll up channel monitor v2 user metrics %ds: %w", seconds, err)
		}
		if _, err := tx.ExecContext(ctx, channelMonitorV2HistogramRollupSQL, interval, seconds, start, end); err != nil {
			return fmt.Errorf("roll up channel monitor v2 histograms %ds: %w", seconds, err)
		}
		if _, err := tx.ExecContext(ctx, channelMonitorV2ErrorRollupSQL, interval, seconds, start, end); err != nil {
			return fmt.Errorf("roll up channel monitor v2 errors %ds: %w", seconds, err)
		}
	}
	return nil
}

// channelMonitorV2DateBinMySQLExpr 是 channelMonitorV2DateBinExpr 的 MySQL 版：
// date_bin($1::interval, col, TIMESTAMPTZ epoch) → FROM_UNIXTIME((FLOOR(UNIX_TIMESTAMP(col) / N)) * N)。
// UNIX_TIMESTAMP/FROM_UNIXTIME 在会话时区下互逆（DSN 会话 time_zone == 应用时区固定偏移），
// 桶界与 epoch 绝对时刻对齐（同 opsBucketExprForUsageMySQL）。
func channelMonitorV2DateBinMySQLExpr(column string, seconds int) string {
	return fmt.Sprintf("FROM_UNIXTIME((FLOOR(UNIX_TIMESTAMP(%s) / %d)) * %d)", column, seconds, seconds)
}

// recomputeFixedRollupMySQL 是 recomputeFixedRollups 单一秒数粒度的 MySQL 实现：
//   - bounds CTE 的 date_bin($1::interval, $3, epoch) 桶界改为应用侧 Truncate 计算
//     （time.Time.Truncate 自 UTC epoch 向下取整，与 date_bin 的 epoch 原点一致），
//     以参数形式传入，避免参数参与 INTERVAL 运算；
//   - DELETE ... USING bounds → 直接 WHERE 常量界；
//   - SELECT date_bin($1::interval, m.bucket_start, epoch) → channelMonitorV2DateBinMySQLExpr；
//   - NOW() → NOW(3)；$n → ?。PG 版 rollup INSERT 无 ON CONFLICT（先删后插），MySQL 同构。
func (r *channelMonitorV2Repository) recomputeFixedRollupMySQL(ctx context.Context, tx *sql.Tx, start, end time.Time, seconds int) error {
	interval := time.Duration(seconds) * time.Second
	startAt := start.Truncate(interval)
	endAt := end.Add(-time.Microsecond).Truncate(interval).Add(interval)
	bucketExpr := channelMonitorV2DateBinMySQLExpr("m.bucket_start", seconds)

	for _, table := range []string{
		"channel_monitor_v2_latency_histograms_rollup",
		"channel_monitor_v2_error_metrics_rollup",
		"channel_monitor_v2_user_metrics_rollup",
		"channel_monitor_v2_metrics_rollup",
	} {
		if _, err := tx.ExecContext(ctx,
			"DELETE FROM "+table+" WHERE bucket_seconds = ? AND bucket_start >= ? AND bucket_start < ?",
			seconds, startAt, endAt,
		); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf(channelMonitorV2MetricsRollupSQLMySQL, bucketExpr, bucketExpr), seconds, startAt, endAt); err != nil {
		return fmt.Errorf("roll up channel monitor v2 metrics %ds: %w", seconds, err)
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf(channelMonitorV2UserMetricsRollupSQLMySQL, bucketExpr, bucketExpr), seconds, startAt, endAt); err != nil {
		return fmt.Errorf("roll up channel monitor v2 user metrics %ds: %w", seconds, err)
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf(channelMonitorV2HistogramRollupSQLMySQL, bucketExpr, bucketExpr), seconds, startAt, endAt); err != nil {
		return fmt.Errorf("roll up channel monitor v2 histograms %ds: %w", seconds, err)
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf(channelMonitorV2ErrorRollupSQLMySQL, bucketExpr, bucketExpr), seconds, startAt, endAt); err != nil {
		return fmt.Errorf("roll up channel monitor v2 errors %ds: %w", seconds, err)
	}
	return nil
}

// 以下四个 MySQL rollup 模板的 %s 均为 bucket 桶表达式（channelMonitorV2DateBinMySQLExpr），
// 在 SELECT 与 GROUP BY 中重复同一文本，与 PG 的 GROUP BY 1 语义一致。
// 参数顺序：? = bucket_seconds、? = startAt、? = endAt。

const channelMonitorV2MetricsRollupSQLMySQL = `
INSERT INTO channel_monitor_v2_metrics_rollup (
  bucket_start, bucket_seconds, platform, group_id, model, success_requests, error_requests,
  upstream_affected_requests, upstream_attempt_count, input_tokens, output_tokens,
  cache_creation_tokens, cache_read_tokens, ttft_sum_ms, ttft_count, duration_sum_ms,
  duration_count, computed_at
)
SELECT %s, ?, platform, group_id, model, SUM(success_requests), SUM(error_requests),
       SUM(upstream_affected_requests), SUM(upstream_attempt_count), SUM(input_tokens),
       SUM(output_tokens), SUM(cache_creation_tokens), SUM(cache_read_tokens),
       SUM(ttft_sum_ms), SUM(ttft_count), SUM(duration_sum_ms), SUM(duration_count), NOW(3)
FROM channel_monitor_v2_metrics_1m m
WHERE m.bucket_start >= ? AND m.bucket_start < ?
GROUP BY %s, m.platform, m.group_id, m.model`

const channelMonitorV2UserMetricsRollupSQLMySQL = `
INSERT INTO channel_monitor_v2_user_metrics_rollup (
  bucket_start, bucket_seconds, platform, group_id, model, user_id, success_requests,
  error_requests, input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens,
  ttft_sum_ms, ttft_count, duration_sum_ms, duration_count, computed_at
)
SELECT %s, ?, platform, group_id, model, user_id, SUM(success_requests), SUM(error_requests),
       SUM(input_tokens), SUM(output_tokens), SUM(cache_creation_tokens), SUM(cache_read_tokens),
       SUM(ttft_sum_ms), SUM(ttft_count), SUM(duration_sum_ms), SUM(duration_count), NOW(3)
FROM channel_monitor_v2_user_metrics_1m m
WHERE m.bucket_start >= ? AND m.bucket_start < ?
GROUP BY %s, m.platform, m.group_id, m.model, m.user_id`

const channelMonitorV2HistogramRollupSQLMySQL = `
INSERT INTO channel_monitor_v2_latency_histograms_rollup (
  bucket_start, bucket_seconds, platform, group_id, model, user_id, metric, upper_bound_ms, sample_count
)
SELECT %s, ?, platform, group_id, model, user_id, metric, upper_bound_ms, SUM(sample_count)
FROM channel_monitor_v2_latency_histograms_1m h
WHERE h.bucket_start >= ? AND h.bucket_start < ?
GROUP BY %s, h.platform, h.group_id, h.model, h.user_id, h.metric, h.upper_bound_ms`

const channelMonitorV2ErrorRollupSQLMySQL = `
INSERT INTO channel_monitor_v2_error_metrics_rollup (
  bucket_start, bucket_seconds, platform, group_id, model, error_category, taxonomy_version, error_requests
)
SELECT %s, ?, platform, group_id, model, error_category, taxonomy_version, SUM(error_requests)
FROM channel_monitor_v2_error_metrics_1m e
WHERE e.bucket_start >= ? AND e.bucket_start < ?
GROUP BY %s, e.platform, e.group_id, e.model, e.error_category, e.taxonomy_version`

func sameFixedRollupBucket(start, end time.Time, seconds int) bool {
	if !end.After(start) {
		return true
	}
	interval := time.Duration(seconds) * time.Second
	return start.Truncate(interval).Equal(end.Add(-time.Nanosecond).Truncate(interval))
}

// PostgreSQL interprets a TIMESTAMPTZ literal without an explicit offset in
// the current session timezone. Keep date_bin's origin fixed in UTC so bucket
// boundaries do not shift when the database session runs in Asia/Shanghai (or
// any other non-UTC timezone).
const channelMonitorV2DateBinOrigin = "TIMESTAMPTZ '1970-01-01 00:00:00+00'"

func channelMonitorV2DateBinExpr(column string) string {
	return "date_bin($1::interval," + column + "," + channelMonitorV2DateBinOrigin + ")"
}

const channelMonitorV2FixedRollupBoundsSQL = `
WITH bounds AS (
  SELECT
    date_bin($1::interval, $3::timestamptz, ` + channelMonitorV2DateBinOrigin + `) AS start_at,
    date_bin($1::interval, $4::timestamptz - INTERVAL '1 microsecond', ` + channelMonitorV2DateBinOrigin + `) + $1::interval AS end_at
)`

const channelMonitorV2FixedRollupDeleteSQL = channelMonitorV2FixedRollupBoundsSQL + `
DELETE FROM %s
USING bounds
WHERE bucket_seconds = $2::integer
  AND bucket_start >= bounds.start_at
  AND bucket_start < bounds.end_at`

const channelMonitorV2MetricsRollupSQL = `
INSERT INTO channel_monitor_v2_metrics_rollup (
  bucket_start, bucket_seconds, platform, group_id, model, success_requests, error_requests,
  upstream_affected_requests, upstream_attempt_count, input_tokens, output_tokens,
  cache_creation_tokens, cache_read_tokens, ttft_sum_ms, ttft_count, duration_sum_ms,
  duration_count, computed_at
)
` + channelMonitorV2FixedRollupBoundsSQL + `
SELECT date_bin($1::interval, m.bucket_start, ` + channelMonitorV2DateBinOrigin + `), $2::integer,
       platform, group_id, model, SUM(success_requests), SUM(error_requests),
       SUM(upstream_affected_requests), SUM(upstream_attempt_count), SUM(input_tokens),
       SUM(output_tokens), SUM(cache_creation_tokens), SUM(cache_read_tokens),
       SUM(ttft_sum_ms), SUM(ttft_count), SUM(duration_sum_ms), SUM(duration_count), NOW()
FROM channel_monitor_v2_metrics_1m m, bounds
WHERE m.bucket_start >= bounds.start_at AND m.bucket_start < bounds.end_at
GROUP BY 1, 2, 3, 4, 5`

const channelMonitorV2UserMetricsRollupSQL = `
INSERT INTO channel_monitor_v2_user_metrics_rollup (
  bucket_start, bucket_seconds, platform, group_id, model, user_id, success_requests,
  error_requests, input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens,
  ttft_sum_ms, ttft_count, duration_sum_ms, duration_count, computed_at
)
` + channelMonitorV2FixedRollupBoundsSQL + `
SELECT date_bin($1::interval, m.bucket_start, ` + channelMonitorV2DateBinOrigin + `), $2::integer,
       platform, group_id, model, user_id, SUM(success_requests), SUM(error_requests),
       SUM(input_tokens), SUM(output_tokens), SUM(cache_creation_tokens), SUM(cache_read_tokens),
       SUM(ttft_sum_ms), SUM(ttft_count), SUM(duration_sum_ms), SUM(duration_count), NOW()
FROM channel_monitor_v2_user_metrics_1m m, bounds
WHERE m.bucket_start >= bounds.start_at AND m.bucket_start < bounds.end_at
GROUP BY 1, 2, 3, 4, 5, 6`

const channelMonitorV2HistogramRollupSQL = `
INSERT INTO channel_monitor_v2_latency_histograms_rollup (
  bucket_start, bucket_seconds, platform, group_id, model, user_id, metric, upper_bound_ms, sample_count
)
` + channelMonitorV2FixedRollupBoundsSQL + `
SELECT date_bin($1::interval, h.bucket_start, ` + channelMonitorV2DateBinOrigin + `), $2::integer,
       platform, group_id, model, user_id, metric, upper_bound_ms, SUM(sample_count)
FROM channel_monitor_v2_latency_histograms_1m h, bounds
WHERE h.bucket_start >= bounds.start_at AND h.bucket_start < bounds.end_at
GROUP BY 1, 2, 3, 4, 5, 6, 7, 8`

const channelMonitorV2ErrorRollupSQL = `
INSERT INTO channel_monitor_v2_error_metrics_rollup (
  bucket_start, bucket_seconds, platform, group_id, model, error_category, taxonomy_version, error_requests
)
` + channelMonitorV2FixedRollupBoundsSQL + `
SELECT date_bin($1::interval, e.bucket_start, ` + channelMonitorV2DateBinOrigin + `), $2::integer,
       platform, group_id, model, error_category, taxonomy_version, SUM(error_requests)
FROM channel_monitor_v2_error_metrics_1m e, bounds
WHERE e.bucket_start >= bounds.start_at AND e.bucket_start < bounds.end_at
GROUP BY 1, 2, 3, 4, 5, 6, 7`
