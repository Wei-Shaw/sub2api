package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

func (r *opsRepository) UpsertHourlyMetrics(ctx context.Context, startTime, endTime time.Time) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("nil ops repository")
	}
	if startTime.IsZero() || endTime.IsZero() || !endTime.After(startTime) {
		return nil
	}

	start := startTime.UTC()
	end := endTime.UTC()

	// NOTE:
	// - We aggregate usage_logs + ops_error_logs into ops_metrics_hourly.
	// - We emit three dimension granularities via GROUPING SETS:
	//   1) overall: (bucket_start)
	//   2) platform: (bucket_start, platform)
	//   3) group: (bucket_start, platform, group_id)
	//
	// IMPORTANT: Postgres UNIQUE treats NULLs as distinct, so the table uses a COALESCE-based
	// unique index; our ON CONFLICT target must match that expression set.
	q := `
WITH usage_base AS (
  SELECT
    date_trunc('hour', ul.created_at AT TIME ZONE 'UTC') AT TIME ZONE 'UTC' AS bucket_start,
    g.platform AS platform,
    ul.group_id AS group_id,
    ul.duration_ms AS duration_ms,
    ul.first_token_ms AS first_token_ms,
    (ul.input_tokens + ul.output_tokens + ul.cache_creation_tokens + ul.cache_read_tokens) AS tokens
  FROM usage_logs ul
  JOIN groups g ON g.id = ul.group_id
  WHERE ul.created_at >= $1 AND ul.created_at < $2
),
usage_agg AS (
  SELECT
    bucket_start,
    CASE WHEN GROUPING(platform) = 1 THEN NULL ELSE platform END AS platform,
    CASE WHEN GROUPING(group_id) = 1 THEN NULL ELSE group_id END AS group_id,
    COUNT(*) AS success_count,
    COALESCE(SUM(tokens), 0) AS token_consumed,

    percentile_cont(0.50) WITHIN GROUP (ORDER BY duration_ms) FILTER (WHERE duration_ms IS NOT NULL) AS duration_p50_ms,
    percentile_cont(0.90) WITHIN GROUP (ORDER BY duration_ms) FILTER (WHERE duration_ms IS NOT NULL) AS duration_p90_ms,
    percentile_cont(0.95) WITHIN GROUP (ORDER BY duration_ms) FILTER (WHERE duration_ms IS NOT NULL) AS duration_p95_ms,
    percentile_cont(0.99) WITHIN GROUP (ORDER BY duration_ms) FILTER (WHERE duration_ms IS NOT NULL) AS duration_p99_ms,
    AVG(duration_ms) FILTER (WHERE duration_ms IS NOT NULL) AS duration_avg_ms,
    MAX(duration_ms) AS duration_max_ms,

    percentile_cont(0.50) WITHIN GROUP (ORDER BY first_token_ms) FILTER (WHERE first_token_ms IS NOT NULL) AS ttft_p50_ms,
    percentile_cont(0.90) WITHIN GROUP (ORDER BY first_token_ms) FILTER (WHERE first_token_ms IS NOT NULL) AS ttft_p90_ms,
    percentile_cont(0.95) WITHIN GROUP (ORDER BY first_token_ms) FILTER (WHERE first_token_ms IS NOT NULL) AS ttft_p95_ms,
    percentile_cont(0.99) WITHIN GROUP (ORDER BY first_token_ms) FILTER (WHERE first_token_ms IS NOT NULL) AS ttft_p99_ms,
    AVG(first_token_ms) FILTER (WHERE first_token_ms IS NOT NULL) AS ttft_avg_ms,
    MAX(first_token_ms) AS ttft_max_ms
  FROM usage_base
  GROUP BY GROUPING SETS (
    (bucket_start),
    (bucket_start, platform),
    (bucket_start, platform, group_id)
  )
),
error_base AS (
  SELECT
    date_trunc('hour', created_at AT TIME ZONE 'UTC') AT TIME ZONE 'UTC' AS bucket_start,
    -- platform is NULL for some early-phase errors (e.g. before routing); map to a sentinel
    -- value so platform-level GROUPING SETS don't collide with the overall (platform=NULL) row.
    COALESCE(platform, 'unknown') AS platform,
    group_id AS group_id,
    is_business_limited AS is_business_limited,
    error_owner AS error_owner,
    status_code AS client_status_code,
    COALESCE(upstream_status_code, status_code, 0) AS effective_status_code
  FROM ops_error_logs
  -- Exclude count_tokens requests from error metrics as they are informational probes
  WHERE created_at >= $1 AND created_at < $2
    AND is_count_tokens = FALSE
),
error_agg AS (
  SELECT
    bucket_start,
    CASE WHEN GROUPING(platform) = 1 THEN NULL ELSE platform END AS platform,
    CASE WHEN GROUPING(group_id) = 1 THEN NULL ELSE group_id END AS group_id,
    COUNT(*) FILTER (WHERE COALESCE(client_status_code, 0) >= 400) AS error_count_total,
    COUNT(*) FILTER (WHERE COALESCE(client_status_code, 0) >= 400 AND is_business_limited) AS business_limited_count,
    COUNT(*) FILTER (WHERE COALESCE(client_status_code, 0) >= 400 AND NOT is_business_limited) AS error_count_sla,
    COUNT(*) FILTER (WHERE error_owner = 'provider' AND NOT is_business_limited AND COALESCE(effective_status_code, 0) NOT IN (429, 529)) AS upstream_error_count_excl_429_529,
    COUNT(*) FILTER (WHERE error_owner = 'provider' AND NOT is_business_limited AND COALESCE(effective_status_code, 0) = 429) AS upstream_429_count,
    COUNT(*) FILTER (WHERE error_owner = 'provider' AND NOT is_business_limited AND COALESCE(effective_status_code, 0) = 529) AS upstream_529_count
  FROM error_base
  GROUP BY GROUPING SETS (
    (bucket_start),
    (bucket_start, platform),
    (bucket_start, platform, group_id)
  )
  HAVING GROUPING(group_id) = 1 OR group_id IS NOT NULL
),
combined AS (
  SELECT
    COALESCE(u.bucket_start, e.bucket_start) AS bucket_start,
    COALESCE(u.platform, e.platform) AS platform,
    COALESCE(u.group_id, e.group_id) AS group_id,

    COALESCE(u.success_count, 0) AS success_count,
    COALESCE(e.error_count_total, 0) AS error_count_total,
    COALESCE(e.business_limited_count, 0) AS business_limited_count,
    COALESCE(e.error_count_sla, 0) AS error_count_sla,
    COALESCE(e.upstream_error_count_excl_429_529, 0) AS upstream_error_count_excl_429_529,
    COALESCE(e.upstream_429_count, 0) AS upstream_429_count,
    COALESCE(e.upstream_529_count, 0) AS upstream_529_count,

    COALESCE(u.token_consumed, 0) AS token_consumed,

    u.duration_p50_ms,
    u.duration_p90_ms,
    u.duration_p95_ms,
    u.duration_p99_ms,
    u.duration_avg_ms,
    u.duration_max_ms,

    u.ttft_p50_ms,
    u.ttft_p90_ms,
    u.ttft_p95_ms,
    u.ttft_p99_ms,
    u.ttft_avg_ms,
    u.ttft_max_ms
  FROM usage_agg u
  FULL OUTER JOIN error_agg e
    ON u.bucket_start = e.bucket_start
   AND COALESCE(u.platform, '') = COALESCE(e.platform, '')
   AND COALESCE(u.group_id, 0) = COALESCE(e.group_id, 0)
)
INSERT INTO ops_metrics_hourly (
  bucket_start,
  platform,
  group_id,
  success_count,
  error_count_total,
  business_limited_count,
  error_count_sla,
  upstream_error_count_excl_429_529,
  upstream_429_count,
  upstream_529_count,
  token_consumed,
  duration_p50_ms,
  duration_p90_ms,
  duration_p95_ms,
  duration_p99_ms,
  duration_avg_ms,
  duration_max_ms,
  ttft_p50_ms,
  ttft_p90_ms,
  ttft_p95_ms,
  ttft_p99_ms,
  ttft_avg_ms,
  ttft_max_ms,
  computed_at
)
SELECT
  bucket_start,
  NULLIF(platform, '') AS platform,
  group_id,
  success_count,
  error_count_total,
  business_limited_count,
  error_count_sla,
  upstream_error_count_excl_429_529,
  upstream_429_count,
  upstream_529_count,
  token_consumed,
  duration_p50_ms::int,
  duration_p90_ms::int,
  duration_p95_ms::int,
  duration_p99_ms::int,
  duration_avg_ms,
  duration_max_ms::int,
  ttft_p50_ms::int,
  ttft_p90_ms::int,
  ttft_p95_ms::int,
  ttft_p99_ms::int,
  ttft_avg_ms,
  ttft_max_ms::int,
  NOW()
FROM combined
WHERE bucket_start IS NOT NULL
  AND (platform IS NULL OR platform <> '')
ON CONFLICT (bucket_start, COALESCE(platform, ''), COALESCE(group_id, 0)) DO UPDATE SET
  success_count = EXCLUDED.success_count,
  error_count_total = EXCLUDED.error_count_total,
  business_limited_count = EXCLUDED.business_limited_count,
  error_count_sla = EXCLUDED.error_count_sla,
  upstream_error_count_excl_429_529 = EXCLUDED.upstream_error_count_excl_429_529,
  upstream_429_count = EXCLUDED.upstream_429_count,
  upstream_529_count = EXCLUDED.upstream_529_count,
  token_consumed = EXCLUDED.token_consumed,

  duration_p50_ms = EXCLUDED.duration_p50_ms,
  duration_p90_ms = EXCLUDED.duration_p90_ms,
  duration_p95_ms = EXCLUDED.duration_p95_ms,
  duration_p99_ms = EXCLUDED.duration_p99_ms,
  duration_avg_ms = EXCLUDED.duration_avg_ms,
  duration_max_ms = EXCLUDED.duration_max_ms,

  ttft_p50_ms = EXCLUDED.ttft_p50_ms,
  ttft_p90_ms = EXCLUDED.ttft_p90_ms,
  ttft_p95_ms = EXCLUDED.ttft_p95_ms,
  ttft_p99_ms = EXCLUDED.ttft_p99_ms,
  ttft_avg_ms = EXCLUDED.ttft_avg_ms,
  ttft_max_ms = EXCLUDED.ttft_max_ms,

  computed_at = NOW()
`

	_, err := r.db.ExecContext(ctx, q, start, end)
	return err
}

func (r *opsRepository) UpsertDailyMetrics(ctx context.Context, startTime, endTime time.Time) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("nil ops repository")
	}
	if startTime.IsZero() || endTime.IsZero() || !endTime.After(startTime) {
		return nil
	}

	start := startTime.UTC()
	end := endTime.UTC()

	q := `
INSERT INTO ops_metrics_daily (
  bucket_date,
  platform,
  group_id,
  success_count,
  error_count_total,
  business_limited_count,
  error_count_sla,
  upstream_error_count_excl_429_529,
  upstream_429_count,
  upstream_529_count,
  token_consumed,
  duration_p50_ms,
  duration_p90_ms,
  duration_p95_ms,
  duration_p99_ms,
  duration_avg_ms,
  duration_max_ms,
  ttft_p50_ms,
  ttft_p90_ms,
  ttft_p95_ms,
  ttft_p99_ms,
  ttft_avg_ms,
  ttft_max_ms,
  computed_at
)
SELECT
  (bucket_start AT TIME ZONE 'UTC')::date AS bucket_date,
  platform,
  group_id,

  COALESCE(SUM(success_count), 0) AS success_count,
  COALESCE(SUM(error_count_total), 0) AS error_count_total,
  COALESCE(SUM(business_limited_count), 0) AS business_limited_count,
  COALESCE(SUM(error_count_sla), 0) AS error_count_sla,
  COALESCE(SUM(upstream_error_count_excl_429_529), 0) AS upstream_error_count_excl_429_529,
  COALESCE(SUM(upstream_429_count), 0) AS upstream_429_count,
  COALESCE(SUM(upstream_529_count), 0) AS upstream_529_count,
  COALESCE(SUM(token_consumed), 0) AS token_consumed,

  -- Approximation: weighted average for p50/p90, max for p95/p99 (conservative tail).
  ROUND(SUM(duration_p50_ms::double precision * success_count) FILTER (WHERE duration_p50_ms IS NOT NULL)
    / NULLIF(SUM(success_count) FILTER (WHERE duration_p50_ms IS NOT NULL), 0))::int AS duration_p50_ms,
  ROUND(SUM(duration_p90_ms::double precision * success_count) FILTER (WHERE duration_p90_ms IS NOT NULL)
    / NULLIF(SUM(success_count) FILTER (WHERE duration_p90_ms IS NOT NULL), 0))::int AS duration_p90_ms,
  MAX(duration_p95_ms) AS duration_p95_ms,
  MAX(duration_p99_ms) AS duration_p99_ms,
  SUM(duration_avg_ms * success_count) FILTER (WHERE duration_avg_ms IS NOT NULL)
    / NULLIF(SUM(success_count) FILTER (WHERE duration_avg_ms IS NOT NULL), 0) AS duration_avg_ms,
  MAX(duration_max_ms) AS duration_max_ms,

  ROUND(SUM(ttft_p50_ms::double precision * success_count) FILTER (WHERE ttft_p50_ms IS NOT NULL)
    / NULLIF(SUM(success_count) FILTER (WHERE ttft_p50_ms IS NOT NULL), 0))::int AS ttft_p50_ms,
  ROUND(SUM(ttft_p90_ms::double precision * success_count) FILTER (WHERE ttft_p90_ms IS NOT NULL)
    / NULLIF(SUM(success_count) FILTER (WHERE ttft_p90_ms IS NOT NULL), 0))::int AS ttft_p90_ms,
  MAX(ttft_p95_ms) AS ttft_p95_ms,
  MAX(ttft_p99_ms) AS ttft_p99_ms,
  SUM(ttft_avg_ms * success_count) FILTER (WHERE ttft_avg_ms IS NOT NULL)
    / NULLIF(SUM(success_count) FILTER (WHERE ttft_avg_ms IS NOT NULL), 0) AS ttft_avg_ms,
  MAX(ttft_max_ms) AS ttft_max_ms,

  NOW()
FROM ops_metrics_hourly
WHERE bucket_start >= $1 AND bucket_start < $2
GROUP BY 1, 2, 3
ON CONFLICT (bucket_date, COALESCE(platform, ''), COALESCE(group_id, 0)) DO UPDATE SET
  success_count = EXCLUDED.success_count,
  error_count_total = EXCLUDED.error_count_total,
  business_limited_count = EXCLUDED.business_limited_count,
  error_count_sla = EXCLUDED.error_count_sla,
  upstream_error_count_excl_429_529 = EXCLUDED.upstream_error_count_excl_429_529,
  upstream_429_count = EXCLUDED.upstream_429_count,
  upstream_529_count = EXCLUDED.upstream_529_count,
  token_consumed = EXCLUDED.token_consumed,

  duration_p50_ms = EXCLUDED.duration_p50_ms,
  duration_p90_ms = EXCLUDED.duration_p90_ms,
  duration_p95_ms = EXCLUDED.duration_p95_ms,
  duration_p99_ms = EXCLUDED.duration_p99_ms,
  duration_avg_ms = EXCLUDED.duration_avg_ms,
  duration_max_ms = EXCLUDED.duration_max_ms,

  ttft_p50_ms = EXCLUDED.ttft_p50_ms,
  ttft_p90_ms = EXCLUDED.ttft_p90_ms,
  ttft_p95_ms = EXCLUDED.ttft_p95_ms,
  ttft_p99_ms = EXCLUDED.ttft_p99_ms,
  ttft_avg_ms = EXCLUDED.ttft_avg_ms,
  ttft_max_ms = EXCLUDED.ttft_max_ms,

  computed_at = NOW()
`

	_, err := r.db.ExecContext(ctx, q, start, end)
	return err
}

func (r *opsRepository) GetLatestHourlyBucketStart(ctx context.Context) (time.Time, bool, error) {
	if r == nil || r.db == nil {
		return time.Time{}, false, fmt.Errorf("nil ops repository")
	}

	var value sql.NullTime
	if err := r.db.QueryRowContext(ctx, `SELECT MAX(bucket_start) FROM ops_metrics_hourly`).Scan(&value); err != nil {
		return time.Time{}, false, err
	}
	if !value.Valid {
		return time.Time{}, false, nil
	}
	return value.Time.UTC(), true, nil
}

func (r *opsRepository) GetLatestDailyBucketDate(ctx context.Context) (time.Time, bool, error) {
	if r == nil || r.db == nil {
		return time.Time{}, false, fmt.Errorf("nil ops repository")
	}

	var value sql.NullTime
	if err := r.db.QueryRowContext(ctx, `SELECT MAX(bucket_date) FROM ops_metrics_daily`).Scan(&value); err != nil {
		return time.Time{}, false, err
	}
	if !value.Valid {
		return time.Time{}, false, nil
	}
	t := value.Time.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC), true, nil
}
