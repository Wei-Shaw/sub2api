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
    COUNT(*) FILTER (WHERE first_token_ms IS NOT NULL) AS ttft_sample_count,
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
    COALESCE(u.ttft_sample_count, 0) AS ttft_sample_count,
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
  ttft_sample_count,
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
  ttft_sample_count,
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
  ttft_sample_count = EXCLUDED.ttft_sample_count,
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

// opsMinuteDimensionHaving 保证三档 GROUPING SETS 产出的行落在互不相同的唯一键上。
//
// ops_metrics_minute 的唯一键是 (bucket_start, COALESCE(platform,”), COALESCE(group_id,0))，
// 而 GROUPING SETS 里"被卷起的 NULL"和"字段本身就是 NULL"最终折叠成同一个键：
//
//	(bucket_start)                      → (b, '', 0)   overall
//	(bucket_start, platform)            → (b, '', 0)   当 platform 真的是 NULL 时与 overall 撞车
//	(bucket_start, platform, group_id)  → (b, p,  0)   当 group_id 真的是 NULL 时与 platform 行撞车
//
// 撞车会让 ON CONFLICT DO UPDATE 在一条语句里两次命中同一行，PostgreSQL 直接报错
// "cannot affect row a second time"，整个分钟聚合失败。
//
// 丢掉这些行不会少算：真 NULL 的 platform/group 依然计入 overall 行，而 raw 侧的
// platform / group 过滤本来就排除它们（NULL 不等于任何值），口径一致。
//
// UpsertHourlyMetrics 不需要这层保护——它的 usage 侧是 INNER JOIN groups，
// platform 与 group_id 都不可能为 NULL。
const opsMinuteDimensionHaving = `HAVING (GROUPING(group_id) = 1 AND (GROUPING(platform) = 1 OR platform IS NOT NULL))
      OR (GROUPING(group_id) = 0 AND group_id IS NOT NULL)`

// UpsertMinuteMetrics 聚合 [startTime, endTime) 的分钟桶，供 ops trend 查询使用。
//
// 与 UpsertHourlyMetrics 的关键差异（刻意为之，不是疏漏）：
//   - usage 侧用 LEFT JOIN groups/accounts，与 buildUsageWhere 的口径一致；
//     UpsertHourlyMetrics 用 INNER JOIN groups，会丢掉 group_id IS NULL 的行。
//     trend 的 preagg 段要和 head/tail 的 raw 段拼接，口径必须一致，否则接缝处数字会跳。
//   - 额外聚合 switch_count：把原先散落在查询期的 jsonb_array_elements(upstream_errors)
//     展开挪到写入侧，每分钟只展开一次。
//   - 不存分位数：分钟粒度的 p50/p90 无意义，且 trend 不需要。
func (r *opsRepository) UpsertMinuteMetrics(ctx context.Context, startTime, endTime time.Time) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("nil ops repository")
	}
	if startTime.IsZero() || endTime.IsZero() || !endTime.After(startTime) {
		return nil
	}

	start := startTime.UTC()
	end := endTime.UTC()

	q := `
WITH usage_base AS (
  SELECT
    date_trunc('minute', ul.created_at AT TIME ZONE 'UTC') AT TIME ZONE 'UTC' AS bucket_start,
    -- 与 buildUsageWhere 相同：优先 group.platform，回退 account.platform。
    -- 外层再包一层 NULLIF：唯一键是 COALESCE(platform,'')，'' 与 NULL 同键，
    -- 不归一的话空串维度行会和 overall 行撞在同一个 ON CONFLICT 目标上。
    NULLIF(COALESCE(NULLIF(g.platform, ''), a.platform), '') AS platform,
    ul.group_id AS group_id,
    (ul.input_tokens + ul.output_tokens + ul.cache_creation_tokens + ul.cache_read_tokens) AS tokens
  FROM usage_logs ul
  LEFT JOIN groups g ON g.id = ul.group_id
  LEFT JOIN accounts a ON a.id = ul.account_id
  WHERE ul.created_at >= $1 AND ul.created_at < $2
),
usage_agg AS (
  SELECT
    bucket_start,
    CASE WHEN GROUPING(platform) = 1 THEN NULL ELSE platform END AS platform,
    CASE WHEN GROUPING(group_id) = 1 THEN NULL ELSE group_id END AS group_id,
    COUNT(*) AS success_count,
    COALESCE(SUM(tokens), 0) AS token_consumed
  FROM usage_base
  GROUP BY GROUPING SETS (
    (bucket_start),
    (bucket_start, platform),
    (bucket_start, platform, group_id)
  )
  ` + opsMinuteDimensionHaving + `
),
error_base AS (
  SELECT
    date_trunc('minute', created_at AT TIME ZONE 'UTC') AT TIME ZONE 'UTC' AS bucket_start,
    -- platform 在早期阶段错误里可能为 NULL。这里保持 NULL 并由 HAVING 丢掉该维度行，
    -- 而不是像 UpsertHourlyMetrics 那样映射成 'unknown'：raw 侧的 platform 过滤与
    -- platform 下钻都排除 NULL platform，造一个假平台会让 preagg 比 raw 多一行。
    NULLIF(platform, '') AS platform,
    group_id AS group_id,
    is_business_limited AS is_business_limited,
    error_owner AS error_owner,
    status_code AS client_status_code,
    COALESCE(upstream_status_code, status_code, 0) AS effective_status_code,
    upstream_errors AS upstream_errors
  FROM ops_error_logs
  -- 与 buildErrorWhere 一致：count_tokens 是探针请求，不计入错误指标。
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
  ` + opsMinuteDimensionHaving + `
),
switch_agg AS (
  SELECT
    bucket_start,
    CASE WHEN GROUPING(platform) = 1 THEN NULL ELSE platform END AS platform,
    CASE WHEN GROUPING(group_id) = 1 THEN NULL ELSE group_id END AS group_id,
    COALESCE(SUM(CASE
      WHEN split_part(ev->>'kind', ':', 1) IN ('failover', 'retry_exhausted_failover', 'failover_on_400') THEN 1
      ELSE 0
    END), 0) AS switch_count
  FROM error_base
  CROSS JOIN LATERAL jsonb_array_elements(
    COALESCE(NULLIF(upstream_errors, 'null'::jsonb), '[]'::jsonb)
  ) AS ev
  WHERE upstream_errors IS NOT NULL
  GROUP BY GROUPING SETS (
    (bucket_start),
    (bucket_start, platform),
    (bucket_start, platform, group_id)
  )
  ` + opsMinuteDimensionHaving + `
),
combined AS (
  SELECT
    COALESCE(u.bucket_start, e.bucket_start, s.bucket_start) AS bucket_start,
    COALESCE(u.platform, e.platform, s.platform) AS platform,
    COALESCE(u.group_id, e.group_id, s.group_id) AS group_id,

    COALESCE(u.success_count, 0) AS success_count,
    COALESCE(e.error_count_total, 0) AS error_count_total,
    COALESCE(e.business_limited_count, 0) AS business_limited_count,
    COALESCE(e.error_count_sla, 0) AS error_count_sla,
    COALESCE(e.upstream_error_count_excl_429_529, 0) AS upstream_error_count_excl_429_529,
    COALESCE(e.upstream_429_count, 0) AS upstream_429_count,
    COALESCE(e.upstream_529_count, 0) AS upstream_529_count,
    COALESCE(u.token_consumed, 0) AS token_consumed,
    COALESCE(s.switch_count, 0) AS switch_count
  FROM usage_agg u
  FULL OUTER JOIN error_agg e
    ON u.bucket_start = e.bucket_start
   AND COALESCE(u.platform, '') = COALESCE(e.platform, '')
   AND COALESCE(u.group_id, 0) = COALESCE(e.group_id, 0)
  FULL OUTER JOIN switch_agg s
    ON COALESCE(u.bucket_start, e.bucket_start) = s.bucket_start
   AND COALESCE(u.platform, e.platform, '') = COALESCE(s.platform, '')
   AND COALESCE(u.group_id, e.group_id, 0) = COALESCE(s.group_id, 0)
)
INSERT INTO ops_metrics_minute (
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
  switch_count,
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
  switch_count,
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
  switch_count = EXCLUDED.switch_count,
  computed_at = NOW()
`

	_, err := r.db.ExecContext(ctx, q, start, end)
	return err
}

// GetLatestMinuteBucketStart 返回分钟表已聚合到的最新桶，用于判断回填缺口。
func (r *opsRepository) GetLatestMinuteBucketStart(ctx context.Context) (time.Time, bool, error) {
	if r == nil || r.db == nil {
		return time.Time{}, false, fmt.Errorf("nil ops repository")
	}

	var value sql.NullTime
	if err := r.db.QueryRowContext(ctx, `SELECT MAX(bucket_start) FROM ops_metrics_minute`).Scan(&value); err != nil {
		return time.Time{}, false, err
	}
	if !value.Valid {
		return time.Time{}, false, nil
	}
	return value.Time.UTC(), true, nil
}

// GetEarliestMinuteBucketStart 返回分钟表最早的桶，用于判断历史回填是否已覆盖目标窗口。
func (r *opsRepository) GetEarliestMinuteBucketStart(ctx context.Context) (time.Time, bool, error) {
	if r == nil || r.db == nil {
		return time.Time{}, false, fmt.Errorf("nil ops repository")
	}

	var value sql.NullTime
	if err := r.db.QueryRowContext(ctx, `SELECT MIN(bucket_start) FROM ops_metrics_minute`).Scan(&value); err != nil {
		return time.Time{}, false, err
	}
	if !value.Valid {
		return time.Time{}, false, nil
	}
	return value.Time.UTC(), true, nil
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
  ttft_sample_count,
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
  COALESCE(SUM(ttft_sample_count), 0) AS ttft_sample_count,
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

  -- TTFT is weighted by ttft_sample_count (streaming rows only), NOT success_count,
  -- because first_token_ms is recorded only for streaming requests.
  ROUND(SUM(ttft_p50_ms::double precision * ttft_sample_count) FILTER (WHERE ttft_p50_ms IS NOT NULL)
    / NULLIF(SUM(ttft_sample_count) FILTER (WHERE ttft_p50_ms IS NOT NULL), 0))::int AS ttft_p50_ms,
  ROUND(SUM(ttft_p90_ms::double precision * ttft_sample_count) FILTER (WHERE ttft_p90_ms IS NOT NULL)
    / NULLIF(SUM(ttft_sample_count) FILTER (WHERE ttft_p90_ms IS NOT NULL), 0))::int AS ttft_p90_ms,
  MAX(ttft_p95_ms) AS ttft_p95_ms,
  MAX(ttft_p99_ms) AS ttft_p99_ms,
  SUM(ttft_avg_ms * ttft_sample_count) FILTER (WHERE ttft_avg_ms IS NOT NULL)
    / NULLIF(SUM(ttft_sample_count) FILTER (WHERE ttft_avg_ms IS NOT NULL), 0) AS ttft_avg_ms,
  MAX(ttft_max_ms) AS ttft_max_ms,

  NOW()
FROM ops_metrics_hourly
WHERE bucket_start >= $1 AND bucket_start < $2
GROUP BY 1, 2, 3
ON CONFLICT (bucket_date, COALESCE(platform, ''), COALESCE(group_id, 0)) DO UPDATE SET
  success_count = EXCLUDED.success_count,
  ttft_sample_count = EXCLUDED.ttft_sample_count,
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
