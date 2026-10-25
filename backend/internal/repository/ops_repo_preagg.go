package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
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

	if IsMySQL() {
		return r.upsertHourlyMetricsMySQL(ctx, start, end)
	}

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

func (r *opsRepository) UpsertDailyMetrics(ctx context.Context, startTime, endTime time.Time) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("nil ops repository")
	}
	if startTime.IsZero() || endTime.IsZero() || !endTime.After(startTime) {
		return nil
	}

	start := startTime.UTC()
	end := endTime.UTC()

	if IsMySQL() {
		return r.upsertDailyMetricsMySQL(ctx, start, end)
	}

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
	if IsMySQL() {
		return r.getLatestDailyBucketDateMySQL(ctx)
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

// ---------------------------------------------------------------------------
// MySQL 方言实现（见 DIALECT_MYSQL_GUIDE.md）。
//
// 时区约定：MySQL 侧 created_at / bucket_start 均为 DATETIME(3)，按 DSN 会话时区
// （固定偏移）墙钟存取。PG 版 bucket 是 `date_trunc('hour', created_at AT TIME ZONE
// 'UTC') AT TIME ZONE 'UTC'`（UTC 小时界的绝对时刻），MySQL 版用
// opsHourBucketStartMySQLExpr 先换算为 UTC 墙钟并截断到小时、再换算回会话墙钟存储，
// 使 time.Time 参数经驱动按 DSN Loc 格式化后的 [start, end) 比较与 PG 绝对时刻语义一致；
// GetLatestHourlyBucketStart 直接复用 PG 版语句（DATETIME 经驱动按 Loc 解析后 .UTC()
// 仍得到原时刻），无需单独分支。GET 报表读侧同理。
// ---------------------------------------------------------------------------

// opsHourBucketStartMySQLExpr 生成列 col（DATETIME(3)，会话时区墙钟）的 UTC 小时桶表达式，
// 语义对齐 PG `date_trunc('hour', col AT TIME ZONE 'UTC') AT TIME ZONE 'UTC'`：
// 先换算为 UTC 墙钟并截断到小时（DATE_FORMAT），再换算回会话时区墙钟。
// 会话时区为整点偏移（DSN 固定偏移，如 '+08:00'）时等价于 DATE_FORMAT(col, '%Y-%m-%d %H:00:00')。
func opsHourBucketStartMySQLExpr(col string) string {
	return "CONVERT_TZ(DATE_FORMAT(CONVERT_TZ(" + col + ", @@session.time_zone, '+00:00'), '%Y-%m-%d %H:00:00'), '+00:00', @@session.time_zone)"
}

// opsPercentileContMySQLExpr 生成 MySQL 版
// `percentile_cont(p) WITHIN GROUP (ORDER BY valueCol) FILTER (WHERE valueCol IS NOT NULL)`
// 的等价表达式（MySQL 8 无有序集聚合，用窗口列 + 线性插值模拟，结果与 PG 连续百分位一致：
// 排序后取第 idx = 1 + p*(n-1) 位，在 FLOOR(idx) 与 CEIL(idx) 两个值之间按小数部分插值）。
// 前置条件（派生表须提供窗口列）：
//   - rnCol：ROW_NUMBER() OVER (PARTITION BY <分组>, (valueCol IS NOT NULL) ORDER BY valueCol)
//     （加 (valueCol IS NOT NULL) 子分区可避免 NULL 行干扰非空值的序号）；
//   - cntCol：COUNT(valueCol) OVER (PARTITION BY <分组>)；
//   - notNullCond：valueCol IS NOT NULL（外层 CASE 守卫）。
func opsPercentileContMySQLExpr(valueCol, rnCol, cntCol, notNullCond, p string) string {
	// cntCol（组内计数，每组取值恒定）必须只出现在聚合函数实参里：
	// MySQL 不允许聚合嵌套（如 MAX(... MAX(cntCol) ...) 报 1111 Invalid use of
	// group function），而聚合实参内的裸列引用不受 ONLY_FULL_GROUP_BY 约束。
	// 因此 idx（含 cntCol）仅出现在 MAX(CASE WHEN ...) 实参与 MAX(idx - FLOOR(idx))
	//（组内恒定的插值小数部分）中；n=0（无非空值）时 vlo/vhi 为 NULL，表达式
	// 自然得到 NULL，与 PG percentile_cont 空集语义一致，无需额外 CASE 守卫。
	idx := "(1 + " + p + " * (" + cntCol + " - 1))"
	vlo := "MAX(CASE WHEN " + notNullCond + " AND " + rnCol + " = FLOOR" + idx + " THEN " + valueCol + " END)"
	vhi := "MAX(CASE WHEN " + notNullCond + " AND " + rnCol + " = CEIL" + idx + " THEN " + valueCol + " END)"
	frac := "MAX(" + idx + " - FLOOR" + idx + ")"
	return vlo + " + " + frac + " * (" + vhi + " - " + vlo + ")"
}

// opsHourlyUsageAggLevelMySQL 生成 usage_agg 单一分组粒度（对应 PG GROUPING SETS 的一组）的
// MySQL 子查询：FILTER 聚合改 SUM(CASE WHEN ...)/AVG/MAX（NULL 自动忽略），percentile_cont
// 用窗口列插值（见 opsPercentileContMySQLExpr）。被聚合掉的维度以 NULL 字面量对齐 PG
// `CASE WHEN GROUPING(x) = 1 THEN NULL ELSE x END`。
func opsHourlyUsageAggLevelMySQL(partitionCols, dimsSelect, groupByCols string) string {
	pct := func(col, rn, cnt, p string) string {
		return opsPercentileContMySQLExpr(col, rn, cnt, col+" IS NOT NULL", p)
	}
	return `
		SELECT
			` + dimsSelect + `,
			COUNT(*) AS success_count,
			SUM(CASE WHEN first_token_ms IS NOT NULL THEN 1 ELSE 0 END) AS ttft_sample_count,
			COALESCE(SUM(tokens), 0) AS token_consumed,
			` + pct("duration_ms", "d_rn", "d_n", "0.50") + ` AS duration_p50_ms,
			` + pct("duration_ms", "d_rn", "d_n", "0.90") + ` AS duration_p90_ms,
			` + pct("duration_ms", "d_rn", "d_n", "0.95") + ` AS duration_p95_ms,
			` + pct("duration_ms", "d_rn", "d_n", "0.99") + ` AS duration_p99_ms,
			AVG(duration_ms) AS duration_avg_ms,
			MAX(duration_ms) AS duration_max_ms,
			` + pct("first_token_ms", "t_rn", "t_n", "0.50") + ` AS ttft_p50_ms,
			` + pct("first_token_ms", "t_rn", "t_n", "0.90") + ` AS ttft_p90_ms,
			` + pct("first_token_ms", "t_rn", "t_n", "0.95") + ` AS ttft_p95_ms,
			` + pct("first_token_ms", "t_rn", "t_n", "0.99") + ` AS ttft_p99_ms,
			AVG(first_token_ms) AS ttft_avg_ms,
			MAX(first_token_ms) AS ttft_max_ms
		FROM (
			SELECT
				bucket_start,
				platform,
				group_id,
				duration_ms,
				first_token_ms,
				tokens,
				ROW_NUMBER() OVER (PARTITION BY ` + partitionCols + `, (duration_ms IS NOT NULL) ORDER BY duration_ms) AS d_rn,
				COUNT(duration_ms) OVER (PARTITION BY ` + partitionCols + `) AS d_n,
				ROW_NUMBER() OVER (PARTITION BY ` + partitionCols + `, (first_token_ms IS NOT NULL) ORDER BY first_token_ms) AS t_rn,
				COUNT(first_token_ms) OVER (PARTITION BY ` + partitionCols + `) AS t_n
			FROM usage_base
		) w
		GROUP BY ` + groupByCols
}

// opsHourlyErrorAggLevelMySQL 生成 error_agg 单一分组粒度的 MySQL 子查询。
// requireGroupID 对齐 PG `HAVING GROUPING(group_id) = 1 OR group_id IS NOT NULL`
// 中分组粒度含 group_id 时的过滤（丢弃 group_id 为 NULL 的组，避免与 platform 粒度行重复计数）。
func opsHourlyErrorAggLevelMySQL(partitionCols, dimsSelect, groupByCols string, requireGroupID bool) string {
	filter := ""
	if requireGroupID {
		filter = "\n\t\tWHERE group_id IS NOT NULL"
	}
	return `
		SELECT
			` + dimsSelect + `,
			SUM(CASE WHEN COALESCE(client_status_code, 0) >= 400 THEN 1 ELSE 0 END) AS error_count_total,
			SUM(CASE WHEN COALESCE(client_status_code, 0) >= 400 AND is_business_limited THEN 1 ELSE 0 END) AS business_limited_count,
			SUM(CASE WHEN COALESCE(client_status_code, 0) >= 400 AND NOT is_business_limited THEN 1 ELSE 0 END) AS error_count_sla,
			SUM(CASE WHEN error_owner = 'provider' AND NOT is_business_limited AND COALESCE(effective_status_code, 0) NOT IN (429, 529) THEN 1 ELSE 0 END) AS upstream_error_count_excl_429_529,
			SUM(CASE WHEN error_owner = 'provider' AND NOT is_business_limited AND COALESCE(effective_status_code, 0) = 429 THEN 1 ELSE 0 END) AS upstream_429_count,
			SUM(CASE WHEN error_owner = 'provider' AND NOT is_business_limited AND COALESCE(effective_status_code, 0) = 529 THEN 1 ELSE 0 END) AS upstream_529_count
		FROM error_base` + filter + `
		GROUP BY ` + groupByCols
}

// upsertHourlyMetricsMySQL 是 UpsertHourlyMetrics 的 MySQL 实现。与 PG 版的语义对应关系：
//   - GROUPING SETS 三个粒度 → UNION ALL 展开三段子查询（规范：GROUPING SETS → UNION ALL），
//     被聚合掉的维度以 NULL 字面量对齐 GROUPING() 语义；
//   - percentile_cont ... FILTER → 窗口列线性插值（opsPercentileContMySQLExpr）；
//   - FULL OUTER JOIN → LEFT JOIN + 反连接（LEFT JOIN ... WHERE u.bucket_start IS NULL）
//     UNION ALL，两个粒度行的连接键含 COALESCE 后不可能跨粒度匹配，与 PG 等价；
//   - 表达式唯一索引 ON CONFLICT (bucket_start, COALESCE(platform,”), COALESCE(group_id,0))
//     → MySQL 函数式唯一索引 idx_ops_metrics_hourly_unique_dim + ODKU（EXCLUDED.* → VALUES(...)）；
//   - ::int → CAST(... AS SIGNED)（四舍五入语义一致）；NOW() → NOW(3)。
func (r *opsRepository) upsertHourlyMetricsMySQL(ctx context.Context, start, end time.Time) error {
	usageLevels := []string{
		opsHourlyUsageAggLevelMySQL("bucket_start",
			"bucket_start,\n\t\t\tNULL AS platform,\n\t\t\tNULL AS group_id",
			"bucket_start"),
		opsHourlyUsageAggLevelMySQL("bucket_start, platform",
			"bucket_start,\n\t\t\tplatform,\n\t\t\tNULL AS group_id",
			"bucket_start, platform"),
		opsHourlyUsageAggLevelMySQL("bucket_start, platform, group_id",
			"bucket_start,\n\t\t\tplatform,\n\t\t\tgroup_id",
			"bucket_start, platform, group_id"),
	}
	errorLevels := []string{
		opsHourlyErrorAggLevelMySQL("bucket_start",
			"bucket_start,\n\t\t\tNULL AS platform,\n\t\t\tNULL AS group_id",
			"bucket_start", false),
		opsHourlyErrorAggLevelMySQL("bucket_start, platform",
			"bucket_start,\n\t\t\tplatform,\n\t\t\tNULL AS group_id",
			"bucket_start, platform", false),
		opsHourlyErrorAggLevelMySQL("bucket_start, platform, group_id",
			"bucket_start,\n\t\t\tplatform,\n\t\t\tgroup_id",
			"bucket_start, platform, group_id", true),
	}

	q := `
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
WITH usage_base AS (
  SELECT
    ` + opsHourBucketStartMySQLExpr("ul.created_at") + ` AS bucket_start,
    g.platform AS platform,
    ul.group_id AS group_id,
    ul.duration_ms AS duration_ms,
    ul.first_token_ms AS first_token_ms,
    (ul.input_tokens + ul.output_tokens + ul.cache_creation_tokens + ul.cache_read_tokens) AS tokens
  FROM usage_logs ul
  JOIN ` + "`groups`" + ` g ON g.id = ul.group_id
  WHERE ul.created_at >= ? AND ul.created_at < ?
),
error_base AS (
  SELECT
    ` + opsHourBucketStartMySQLExpr("created_at") + ` AS bucket_start,
    -- platform is NULL for some early-phase errors (e.g. before routing); map to a sentinel
    -- value so platform-level rows don't collide with the overall (platform=NULL) row.
    COALESCE(platform, 'unknown') AS platform,
    group_id AS group_id,
    is_business_limited AS is_business_limited,
    error_owner AS error_owner,
    status_code AS client_status_code,
    COALESCE(upstream_status_code, status_code, 0) AS effective_status_code
  FROM ops_error_logs
  -- Exclude count_tokens requests from error metrics as they are informational probes
  WHERE created_at >= ? AND created_at < ?
    AND is_count_tokens = FALSE
),
usage_agg AS (
` + strings.Join(usageLevels, `
		UNION ALL
`) + `
),
error_agg AS (
` + strings.Join(errorLevels, `
		UNION ALL
`) + `
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
  LEFT JOIN error_agg e
    ON u.bucket_start = e.bucket_start
   AND COALESCE(u.platform, '') = COALESCE(e.platform, '')
   AND COALESCE(u.group_id, 0) = COALESCE(e.group_id, 0)
  UNION ALL
  SELECT
    e.bucket_start,
    e.platform,
    e.group_id,
    0 AS success_count,
    0 AS ttft_sample_count,
    e.error_count_total,
    e.business_limited_count,
    e.error_count_sla,
    e.upstream_error_count_excl_429_529,
    e.upstream_429_count,
    e.upstream_529_count,
    0 AS token_consumed,
    NULL AS duration_p50_ms,
    NULL AS duration_p90_ms,
    NULL AS duration_p95_ms,
    NULL AS duration_p99_ms,
    NULL AS duration_avg_ms,
    NULL AS duration_max_ms,
    NULL AS ttft_p50_ms,
    NULL AS ttft_p90_ms,
    NULL AS ttft_p95_ms,
    NULL AS ttft_p99_ms,
    NULL AS ttft_avg_ms,
    NULL AS ttft_max_ms
  FROM error_agg e
  LEFT JOIN usage_agg u
    ON u.bucket_start = e.bucket_start
   AND COALESCE(u.platform, '') = COALESCE(e.platform, '')
   AND COALESCE(u.group_id, 0) = COALESCE(e.group_id, 0)
  WHERE u.bucket_start IS NULL
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
  CAST(duration_p50_ms AS SIGNED) AS duration_p50_ms,
  CAST(duration_p90_ms AS SIGNED) AS duration_p90_ms,
  CAST(duration_p95_ms AS SIGNED) AS duration_p95_ms,
  CAST(duration_p99_ms AS SIGNED) AS duration_p99_ms,
  duration_avg_ms,
  CAST(duration_max_ms AS SIGNED) AS duration_max_ms,
  CAST(ttft_p50_ms AS SIGNED) AS ttft_p50_ms,
  CAST(ttft_p90_ms AS SIGNED) AS ttft_p90_ms,
  CAST(ttft_p95_ms AS SIGNED) AS ttft_p95_ms,
  CAST(ttft_p99_ms AS SIGNED) AS ttft_p99_ms,
  ttft_avg_ms,
  CAST(ttft_max_ms AS SIGNED) AS ttft_max_ms,
  NOW(3) AS computed_at
FROM combined
WHERE bucket_start IS NOT NULL
  AND (platform IS NULL OR platform <> '')
ON DUPLICATE KEY UPDATE
  success_count = VALUES(success_count),
  ttft_sample_count = VALUES(ttft_sample_count),
  error_count_total = VALUES(error_count_total),
  business_limited_count = VALUES(business_limited_count),
  error_count_sla = VALUES(error_count_sla),
  upstream_error_count_excl_429_529 = VALUES(upstream_error_count_excl_429_529),
  upstream_429_count = VALUES(upstream_429_count),
  upstream_529_count = VALUES(upstream_529_count),
  token_consumed = VALUES(token_consumed),

  duration_p50_ms = VALUES(duration_p50_ms),
  duration_p90_ms = VALUES(duration_p90_ms),
  duration_p95_ms = VALUES(duration_p95_ms),
  duration_p99_ms = VALUES(duration_p99_ms),
  duration_avg_ms = VALUES(duration_avg_ms),
  duration_max_ms = VALUES(duration_max_ms),

  ttft_p50_ms = VALUES(ttft_p50_ms),
  ttft_p90_ms = VALUES(ttft_p90_ms),
  ttft_p95_ms = VALUES(ttft_p95_ms),
  ttft_p99_ms = VALUES(ttft_p99_ms),
  ttft_avg_ms = VALUES(ttft_avg_ms),
  ttft_max_ms = VALUES(ttft_max_ms),

  computed_at = VALUES(computed_at)
`

	_, err := r.db.ExecContext(ctx, q, start, end, start, end)
	return err
}

// upsertDailyMetricsMySQL 是 UpsertDailyMetrics 的 MySQL 实现：
// (bucket_start AT TIME ZONE 'UTC')::date → DATE(CONVERT_TZ(bucket_start, @@session.time_zone,
// '+00:00'))（bucket_start 存的是会话墙钟，换算回 UTC 墙钟取日期）；
// FILTER 加权平均 → SUM(CASE WHEN ... ELSE 0 END) / NULLIF(SUM(CASE ...), 0)，
// ROUND(...)::int → CAST(ROUND(...) AS SIGNED)；ON CONFLICT → ODKU；NOW() → NOW(3)。
func (r *opsRepository) upsertDailyMetricsMySQL(ctx context.Context, start, end time.Time) error {
	bucketDateExpr := "DATE(CONVERT_TZ(bucket_start, @@session.time_zone, '+00:00'))"
	weighted := func(col string, weight string, rounded bool) string {
		num := "SUM(CASE WHEN " + col + " IS NOT NULL THEN " + col + " * " + weight + " ELSE 0 END)"
		den := "NULLIF(SUM(CASE WHEN " + col + " IS NOT NULL THEN " + weight + " ELSE 0 END), 0)"
		expr := num + " / " + den
		if rounded {
			expr = "CAST(ROUND(" + expr + ") AS SIGNED)"
		}
		return expr
	}

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
  ` + bucketDateExpr + ` AS bucket_date,
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
  ` + weighted("duration_p50_ms", "success_count", true) + ` AS duration_p50_ms,
  ` + weighted("duration_p90_ms", "success_count", true) + ` AS duration_p90_ms,
  MAX(duration_p95_ms) AS duration_p95_ms,
  MAX(duration_p99_ms) AS duration_p99_ms,
  ` + weighted("duration_avg_ms", "success_count", false) + ` AS duration_avg_ms,
  MAX(duration_max_ms) AS duration_max_ms,

  -- TTFT is weighted by ttft_sample_count (streaming rows only), NOT success_count,
  -- because first_token_ms is recorded only for streaming requests.
  ` + weighted("ttft_p50_ms", "ttft_sample_count", true) + ` AS ttft_p50_ms,
  ` + weighted("ttft_p90_ms", "ttft_sample_count", true) + ` AS ttft_p90_ms,
  MAX(ttft_p95_ms) AS ttft_p95_ms,
  MAX(ttft_p99_ms) AS ttft_p99_ms,
  ` + weighted("ttft_avg_ms", "ttft_sample_count", false) + ` AS ttft_avg_ms,
  MAX(ttft_max_ms) AS ttft_max_ms,

  NOW(3)
FROM ops_metrics_hourly
WHERE bucket_start >= ? AND bucket_start < ?
GROUP BY bucket_date, platform, group_id
ON DUPLICATE KEY UPDATE
  success_count = VALUES(success_count),
  ttft_sample_count = VALUES(ttft_sample_count),
  error_count_total = VALUES(error_count_total),
  business_limited_count = VALUES(business_limited_count),
  error_count_sla = VALUES(error_count_sla),
  upstream_error_count_excl_429_529 = VALUES(upstream_error_count_excl_429_529),
  upstream_429_count = VALUES(upstream_429_count),
  upstream_529_count = VALUES(upstream_529_count),
  token_consumed = VALUES(token_consumed),

  duration_p50_ms = VALUES(duration_p50_ms),
  duration_p90_ms = VALUES(duration_p90_ms),
  duration_p95_ms = VALUES(duration_p95_ms),
  duration_p99_ms = VALUES(duration_p99_ms),
  duration_avg_ms = VALUES(duration_avg_ms),
  duration_max_ms = VALUES(duration_max_ms),

  ttft_p50_ms = VALUES(ttft_p50_ms),
  ttft_p90_ms = VALUES(ttft_p90_ms),
  ttft_p95_ms = VALUES(ttft_p95_ms),
  ttft_p99_ms = VALUES(ttft_p99_ms),
  ttft_avg_ms = VALUES(ttft_avg_ms),
  ttft_max_ms = VALUES(ttft_max_ms),

  computed_at = VALUES(computed_at)
`

	_, err := r.db.ExecContext(ctx, q, start, end)
	return err
}

// getLatestDailyBucketDateMySQL 是 GetLatestDailyBucketDate 的 MySQL 实现。
// DATE 列经驱动按 DSN Loc（应用时区）解析为当日零点，`.UTC()` 后日历日期会随偏移漂移
// （如 +08:00 会退回前一天），故按字符串取回并以 UTC 解析，对齐 PG 的「UTC 日期零点」语义。
func (r *opsRepository) getLatestDailyBucketDateMySQL(ctx context.Context) (time.Time, bool, error) {
	var value sql.NullString
	if err := r.db.QueryRowContext(ctx, `SELECT DATE_FORMAT(MAX(bucket_date), '%Y-%m-%d') FROM ops_metrics_daily`).Scan(&value); err != nil {
		return time.Time{}, false, err
	}
	if !value.Valid || value.String == "" {
		return time.Time{}, false, nil
	}
	t, err := time.ParseInLocation("2006-01-02", value.String, time.UTC)
	if err != nil {
		return time.Time{}, false, err
	}
	return t, true, nil
}
