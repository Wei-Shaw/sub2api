package repository

import (
	"context"
	"database/sql"
	"fmt"
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
			_, err = tx.ExecContext(ctx, fmt.Sprintf(`DELETE FROM %s WHERE bucket_start < $1`, rule.table), cutoff)
		} else {
			_, err = tx.ExecContext(ctx,
				fmt.Sprintf(`DELETE FROM %s WHERE bucket_seconds = $1 AND bucket_start < $2`, rule.table),
				rule.bucketSeconds, cutoff,
			)
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
		if _, err = tx.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s WHERE bucket_start >= $1 AND bucket_start < $2", table), start, end); err != nil {
			return err
		}
	}

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
	if err = r.recomputeFixedRollups(ctx, tx, start, end); err != nil {
		return err
	}
	// Drop rows past per-tier TTL (1m short, coarse rollups long). Safe after rollup
	// so a backfill chunk can build 1d rollups from temporary 1m rows then discard 1m.
	if err = r.pruneChannelMonitorV2Retention(ctx, tx, now); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, channelMonitorV2WatermarkSQL, start, end); err != nil {
		return err
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
      WHEN g.platform = 'composite' THEN COALESCE(NULLIF(TRIM(a.platform)), NULLIF(NULLIF(lower(TRIM(current_error.platform)), ''), 'composite'), 'unknown')
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

func sameFixedRollupBucket(start, end time.Time, seconds int) bool {
	if !end.After(start) {
		return true
	}
	interval := time.Duration(seconds) * time.Second
	return start.Truncate(interval).Equal(end.Add(-time.Nanosecond).Truncate(interval))
}

const channelMonitorV2FixedRollupBoundsSQL = `
WITH bounds AS (
  SELECT
    date_bin($1::interval, $3::timestamptz, TIMESTAMPTZ '1970-01-01') AS start_at,
    date_bin($1::interval, $4::timestamptz - INTERVAL '1 microsecond', TIMESTAMPTZ '1970-01-01') + $1::interval AS end_at
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
SELECT date_bin($1::interval, m.bucket_start, TIMESTAMPTZ '1970-01-01'), $2::integer,
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
SELECT date_bin($1::interval, m.bucket_start, TIMESTAMPTZ '1970-01-01'), $2::integer,
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
SELECT date_bin($1::interval, h.bucket_start, TIMESTAMPTZ '1970-01-01'), $2::integer,
       platform, group_id, model, user_id, metric, upper_bound_ms, SUM(sample_count)
FROM channel_monitor_v2_latency_histograms_1m h, bounds
WHERE h.bucket_start >= bounds.start_at AND h.bucket_start < bounds.end_at
GROUP BY 1, 2, 3, 4, 5, 6, 7, 8`

const channelMonitorV2ErrorRollupSQL = `
INSERT INTO channel_monitor_v2_error_metrics_rollup (
  bucket_start, bucket_seconds, platform, group_id, model, error_category, taxonomy_version, error_requests
)
` + channelMonitorV2FixedRollupBoundsSQL + `
SELECT date_bin($1::interval, e.bucket_start, TIMESTAMPTZ '1970-01-01'), $2::integer,
       platform, group_id, model, error_category, taxonomy_version, SUM(error_requests)
FROM channel_monitor_v2_error_metrics_1m e, bounds
WHERE e.bucket_start >= bounds.start_at AND e.bucket_start < bounds.end_at
GROUP BY 1, 2, 3, 4, 5, 6, 7`
