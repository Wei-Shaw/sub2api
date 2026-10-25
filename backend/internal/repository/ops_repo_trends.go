package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (r *opsRepository) GetThroughputTrend(ctx context.Context, filter *service.OpsDashboardFilter, bucketSeconds int) (*service.OpsThroughputTrendResponse, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	if filter == nil {
		return nil, fmt.Errorf("nil filter")
	}
	if filter.StartTime.IsZero() || filter.EndTime.IsZero() {
		return nil, fmt.Errorf("start_time/end_time required")
	}

	if bucketSeconds <= 0 {
		bucketSeconds = 60
	}
	if bucketSeconds != 60 && bucketSeconds != 300 && bucketSeconds != 3600 {
		// Keep a small, predictable set of supported buckets for now.
		bucketSeconds = 60
	}

	start := filter.StartTime.UTC()
	end := filter.EndTime.UTC()

	if IsMySQL() {
		return r.getThroughputTrendMySQL(ctx, filter, bucketSeconds, start, end)
	}

	usageJoin, usageWhere, usageArgs, next := buildUsageWhere(filter, start, end, 1)
	errorWhere, errorArgs, _ := buildErrorWhere(filter, start, end, next)

	usageBucketExpr := opsBucketExprForUsage(bucketSeconds)
	errorBucketExpr := opsBucketExprForError(bucketSeconds)

	q := `
WITH usage_buckets AS (
  SELECT ` + usageBucketExpr + ` AS bucket,
         COUNT(*) AS success_count,
         COALESCE(SUM(input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens), 0) AS token_consumed
  FROM usage_logs ul
  ` + usageJoin + `
  ` + usageWhere + `
  GROUP BY 1
),
error_buckets AS (
  SELECT ` + errorBucketExpr + ` AS bucket,
         COUNT(*) AS error_count
  FROM ops_error_logs
  ` + errorWhere + `
    AND COALESCE(status_code, 0) >= 400
  GROUP BY 1
),
switch_buckets AS (
  SELECT ` + errorBucketExpr + ` AS bucket,
         COALESCE(SUM(CASE
           WHEN split_part(ev->>'kind', ':', 1) IN ('failover', 'retry_exhausted_failover', 'failover_on_400') THEN 1
           ELSE 0
         END), 0) AS switch_count
  FROM ops_error_logs
  CROSS JOIN LATERAL jsonb_array_elements(
    COALESCE(NULLIF(upstream_errors, 'null'::jsonb), '[]'::jsonb)
  ) AS ev
  ` + errorWhere + `
    AND upstream_errors IS NOT NULL
  GROUP BY 1
),
combined AS (
  SELECT
    bucket,
    SUM(success_count) AS success_count,
    SUM(error_count) AS error_count,
    SUM(token_consumed) AS token_consumed,
    SUM(switch_count) AS switch_count
  FROM (
    SELECT bucket, success_count, 0 AS error_count, token_consumed, 0 AS switch_count
    FROM usage_buckets
    UNION ALL
    SELECT bucket, 0, error_count, 0, 0
    FROM error_buckets
    UNION ALL
    SELECT bucket, 0, 0, 0, switch_count
    FROM switch_buckets
  ) t
  GROUP BY bucket
)
SELECT
  bucket,
  (success_count + error_count) AS request_count,
  token_consumed,
  switch_count
FROM combined
ORDER BY bucket ASC`

	args := append(usageArgs, errorArgs...)

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	points := make([]*service.OpsThroughputTrendPoint, 0, 256)
	for rows.Next() {
		var bucket time.Time
		var requests int64
		var tokens sql.NullInt64
		var switches sql.NullInt64
		if err := rows.Scan(&bucket, &requests, &tokens, &switches); err != nil {
			return nil, err
		}
		tokenConsumed := int64(0)
		if tokens.Valid {
			tokenConsumed = tokens.Int64
		}
		switchCount := int64(0)
		if switches.Valid {
			switchCount = switches.Int64
		}

		denom := float64(bucketSeconds)
		if denom <= 0 {
			denom = 60
		}
		qps := roundTo1DP(float64(requests) / denom)
		tps := roundTo1DP(float64(tokenConsumed) / denom)

		points = append(points, &service.OpsThroughputTrendPoint{
			BucketStart:   bucket.UTC(),
			RequestCount:  requests,
			TokenConsumed: tokenConsumed,
			SwitchCount:   switchCount,
			QPS:           qps,
			TPS:           tps,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Fill missing buckets with zeros so charts render continuous timelines.
	points = fillOpsThroughputBuckets(start, end, bucketSeconds, points)

	var byPlatform []*service.OpsThroughputPlatformBreakdownItem
	var topGroups []*service.OpsThroughputGroupBreakdownItem

	platform := ""
	if filter != nil {
		platform = strings.TrimSpace(strings.ToLower(filter.Platform))
	}
	groupID := (*int64)(nil)
	if filter != nil {
		groupID = filter.GroupID
	}

	// Drilldown helpers:
	// - No platform/group: totals by platform
	// - Platform selected but no group: top groups in that platform
	if platform == "" && (groupID == nil || *groupID <= 0) {
		items, err := r.getThroughputBreakdownByPlatform(ctx, start, end)
		if err != nil {
			return nil, err
		}
		byPlatform = items
	} else if platform != "" && (groupID == nil || *groupID <= 0) {
		items, err := r.getThroughputTopGroupsByPlatform(ctx, start, end, platform, 10)
		if err != nil {
			return nil, err
		}
		topGroups = items
	}

	return &service.OpsThroughputTrendResponse{
		Bucket: opsBucketLabel(bucketSeconds),
		Points: points,

		ByPlatform: byPlatform,
		TopGroups:  topGroups,
	}, nil
}

func (r *opsRepository) getThroughputBreakdownByPlatform(ctx context.Context, start, end time.Time) ([]*service.OpsThroughputPlatformBreakdownItem, error) {
	if IsMySQL() {
		return r.getThroughputBreakdownByPlatformMySQL(ctx, start, end)
	}
	q := `
WITH usage_totals AS (
  SELECT COALESCE(NULLIF(g.platform,''), a.platform) AS platform,
         COUNT(*) AS success_count,
         COALESCE(SUM(input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens), 0) AS token_consumed
  FROM usage_logs ul
  LEFT JOIN groups g ON g.id = ul.group_id
  LEFT JOIN accounts a ON a.id = ul.account_id
  WHERE ul.created_at >= $1 AND ul.created_at < $2
  GROUP BY 1
),
error_totals AS (
  SELECT platform,
         COUNT(*) AS error_count
  FROM ops_error_logs
  WHERE created_at >= $1 AND created_at < $2
    AND COALESCE(status_code, 0) >= 400
    AND is_count_tokens = FALSE  -- 排除 count_tokens 请求的错误
  GROUP BY 1
),
combined AS (
  SELECT COALESCE(u.platform, e.platform) AS platform,
         COALESCE(u.success_count, 0) AS success_count,
         COALESCE(e.error_count, 0) AS error_count,
         COALESCE(u.token_consumed, 0) AS token_consumed
  FROM usage_totals u
  FULL OUTER JOIN error_totals e ON u.platform = e.platform
)
SELECT platform, (success_count + error_count) AS request_count, token_consumed
FROM combined
WHERE platform IS NOT NULL AND platform <> ''
ORDER BY request_count DESC`

	rows, err := r.db.QueryContext(ctx, q, start, end)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	items := make([]*service.OpsThroughputPlatformBreakdownItem, 0, 8)
	for rows.Next() {
		var platform string
		var requests int64
		var tokens sql.NullInt64
		if err := rows.Scan(&platform, &requests, &tokens); err != nil {
			return nil, err
		}
		tokenConsumed := int64(0)
		if tokens.Valid {
			tokenConsumed = tokens.Int64
		}
		items = append(items, &service.OpsThroughputPlatformBreakdownItem{
			Platform:      platform,
			RequestCount:  requests,
			TokenConsumed: tokenConsumed,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

// getThroughputBreakdownByPlatformMySQL 是 getThroughputBreakdownByPlatform 的 MySQL 实现：
// FULL OUTER JOIN → LEFT JOIN + 反连接 UNION ALL（u-only 桶 error=0、e-only 桶 success/tokens=0）；
// `groups` 为 MySQL 8 保留字加反引号；$n→?；GROUP BY 1 → GROUP BY 别名。
func (r *opsRepository) getThroughputBreakdownByPlatformMySQL(ctx context.Context, start, end time.Time) ([]*service.OpsThroughputPlatformBreakdownItem, error) {
	q := `
WITH usage_totals AS (
  SELECT COALESCE(NULLIF(g.platform,''), a.platform) AS platform,
         COUNT(*) AS success_count,
         COALESCE(SUM(input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens), 0) AS token_consumed
  FROM usage_logs ul
  LEFT JOIN ` + "`groups`" + ` g ON g.id = ul.group_id
  LEFT JOIN accounts a ON a.id = ul.account_id
  WHERE ul.created_at >= ? AND ul.created_at < ?
  GROUP BY platform
),
error_totals AS (
  SELECT platform,
         COUNT(*) AS error_count
  FROM ops_error_logs
  WHERE created_at >= ? AND created_at < ?
    AND COALESCE(status_code, 0) >= 400
    AND is_count_tokens = FALSE  -- 排除 count_tokens 请求的错误
  GROUP BY platform
)
SELECT platform, (success_count + error_count) AS request_count, token_consumed
FROM (
  SELECT COALESCE(u.platform, e.platform) AS platform,
         COALESCE(u.success_count, 0) AS success_count,
         COALESCE(e.error_count, 0) AS error_count,
         COALESCE(u.token_consumed, 0) AS token_consumed
  FROM usage_totals u
  LEFT JOIN error_totals e ON u.platform = e.platform
  UNION ALL
  SELECT e.platform, 0 AS success_count, COALESCE(e.error_count, 0) AS error_count, 0 AS token_consumed
  FROM error_totals e
  LEFT JOIN usage_totals u ON u.platform = e.platform
  WHERE u.platform IS NULL
) combined
WHERE platform IS NOT NULL AND platform <> ''
ORDER BY request_count DESC`

	rows, err := r.db.QueryContext(ctx, q, start, end, start, end)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	items := make([]*service.OpsThroughputPlatformBreakdownItem, 0, 8)
	for rows.Next() {
		var platform string
		var requests int64
		var tokens sql.NullInt64
		if err := rows.Scan(&platform, &requests, &tokens); err != nil {
			return nil, err
		}
		tokenConsumed := int64(0)
		if tokens.Valid {
			tokenConsumed = tokens.Int64
		}
		items = append(items, &service.OpsThroughputPlatformBreakdownItem{
			Platform:      platform,
			RequestCount:  requests,
			TokenConsumed: tokenConsumed,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}
func (r *opsRepository) getThroughputTopGroupsByPlatform(ctx context.Context, start, end time.Time, platform string, limit int) ([]*service.OpsThroughputGroupBreakdownItem, error) {
	if strings.TrimSpace(platform) == "" {
		return nil, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 10
	}
	if IsMySQL() {
		return r.getThroughputTopGroupsByPlatformMySQL(ctx, start, end, platform, limit)
	}

	q := `
WITH usage_totals AS (
  SELECT ul.group_id AS group_id,
         g.name AS group_name,
         COUNT(*) AS success_count,
         COALESCE(SUM(input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens), 0) AS token_consumed
  FROM usage_logs ul
  JOIN groups g ON g.id = ul.group_id
  WHERE ul.created_at >= $1 AND ul.created_at < $2
    AND g.platform = $3
  GROUP BY 1, 2
),
error_totals AS (
  SELECT group_id,
         COUNT(*) AS error_count
  FROM ops_error_logs
  WHERE created_at >= $1 AND created_at < $2
    AND platform = $3
    AND group_id IS NOT NULL
    AND COALESCE(status_code, 0) >= 400
    AND is_count_tokens = FALSE  -- 排除 count_tokens 请求的错误
  GROUP BY 1
),
combined AS (
  SELECT COALESCE(u.group_id, e.group_id) AS group_id,
         COALESCE(u.group_name, g2.name, '') AS group_name,
         COALESCE(u.success_count, 0) AS success_count,
         COALESCE(e.error_count, 0) AS error_count,
         COALESCE(u.token_consumed, 0) AS token_consumed
  FROM usage_totals u
  FULL OUTER JOIN error_totals e ON u.group_id = e.group_id
  LEFT JOIN groups g2 ON g2.id = COALESCE(u.group_id, e.group_id)
)
SELECT group_id, group_name, (success_count + error_count) AS request_count, token_consumed
FROM combined
WHERE group_id IS NOT NULL
ORDER BY request_count DESC
LIMIT $4`

	rows, err := r.db.QueryContext(ctx, q, start, end, platform, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	items := make([]*service.OpsThroughputGroupBreakdownItem, 0, limit)
	for rows.Next() {
		var groupID int64
		var groupName sql.NullString
		var requests int64
		var tokens sql.NullInt64
		if err := rows.Scan(&groupID, &groupName, &requests, &tokens); err != nil {
			return nil, err
		}
		tokenConsumed := int64(0)
		if tokens.Valid {
			tokenConsumed = tokens.Int64
		}
		name := ""
		if groupName.Valid {
			name = groupName.String
		}
		items = append(items, &service.OpsThroughputGroupBreakdownItem{
			GroupID:       groupID,
			GroupName:     name,
			RequestCount:  requests,
			TokenConsumed: tokenConsumed,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

// getThroughputTopGroupsByPlatformMySQL 是 getThroughputTopGroupsByPlatform 的 MySQL 实现：
// FULL OUTER JOIN → LEFT JOIN + 反连接 UNION ALL；g2 关联键 COALESCE(u.group_id, e.group_id)
// 在两个分支里分别为 u.group_id / e.group_id（u 分支 group_id 必非空），故下沉为对 union
// 结果统一 LEFT JOIN `groups`（`groups` 为 MySQL 8 保留字加反引号）；GROUP BY 1,2 → 显式列；
// LIMIT $4 → LIMIT ?（MySQL 预处理语句支持 LIMIT 占位符）。
func (r *opsRepository) getThroughputTopGroupsByPlatformMySQL(ctx context.Context, start, end time.Time, platform string, limit int) ([]*service.OpsThroughputGroupBreakdownItem, error) {
	q := `
WITH usage_totals AS (
  SELECT ul.group_id AS group_id,
         g.name AS group_name,
         COUNT(*) AS success_count,
         COALESCE(SUM(input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens), 0) AS token_consumed
  FROM usage_logs ul
  JOIN ` + "`groups`" + ` g ON g.id = ul.group_id
  WHERE ul.created_at >= ? AND ul.created_at < ?
    AND g.platform = ?
  GROUP BY ul.group_id, g.name
),
error_totals AS (
  SELECT group_id,
         COUNT(*) AS error_count
  FROM ops_error_logs
  WHERE created_at >= ? AND created_at < ?
    AND platform = ?
    AND group_id IS NOT NULL
    AND COALESCE(status_code, 0) >= 400
    AND is_count_tokens = FALSE  -- 排除 count_tokens 请求的错误
  GROUP BY group_id
),
combined AS (
  SELECT u.group_id,
         u.group_name,
         u.success_count,
         COALESCE(e.error_count, 0) AS error_count,
         u.token_consumed
  FROM usage_totals u
  LEFT JOIN error_totals e ON u.group_id = e.group_id
  UNION ALL
  SELECT e.group_id,
         NULL AS group_name,
         0 AS success_count,
         COALESCE(e.error_count, 0) AS error_count,
         0 AS token_consumed
  FROM error_totals e
  LEFT JOIN usage_totals u ON u.group_id = e.group_id
  WHERE u.group_id IS NULL
)
SELECT
  c.group_id,
  COALESCE(c.group_name, g2.name, '') AS group_name,
  (c.success_count + c.error_count) AS request_count,
  c.token_consumed
FROM combined c
LEFT JOIN ` + "`groups`" + ` g2 ON g2.id = c.group_id
WHERE c.group_id IS NOT NULL
ORDER BY request_count DESC
LIMIT ?`

	rows, err := r.db.QueryContext(ctx, q, start, end, platform, start, end, platform, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	items := make([]*service.OpsThroughputGroupBreakdownItem, 0, limit)
	for rows.Next() {
		var groupID int64
		var groupName sql.NullString
		var requests int64
		var tokens sql.NullInt64
		if err := rows.Scan(&groupID, &groupName, &requests, &tokens); err != nil {
			return nil, err
		}
		tokenConsumed := int64(0)
		if tokens.Valid {
			tokenConsumed = tokens.Int64
		}
		name := ""
		if groupName.Valid {
			name = groupName.String
		}
		items = append(items, &service.OpsThroughputGroupBreakdownItem{
			GroupID:       groupID,
			GroupName:     name,
			RequestCount:  requests,
			TokenConsumed: tokenConsumed,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func opsBucketExprForUsage(bucketSeconds int) string {
	switch bucketSeconds {
	case 3600:
		return "date_trunc('hour', ul.created_at)"
	case 300:
		// 5-minute buckets in UTC.
		return "to_timestamp(floor(extract(epoch from ul.created_at) / 300) * 300)"
	default:
		return "date_trunc('minute', ul.created_at)"
	}
}

func opsBucketExprForError(bucketSeconds int) string {
	switch bucketSeconds {
	case 3600:
		return "date_trunc('hour', created_at)"
	case 300:
		return "to_timestamp(floor(extract(epoch from created_at) / 300) * 300)"
	default:
		return "date_trunc('minute', created_at)"
	}
}

// opsBucketExprForUsageMySQL / opsBucketExprForErrorMySQL 是对应 PG 桶表达式的 MySQL 版，
// 结果均为 DATETIME（可被驱动按 ParseTime+Loc 扫描为 time.Time，与 PG 返回 timestamptz 对齐）：
//   - date_trunc('hour'/'minute', ts) → CAST(DATE_FORMAT(ts, ...) AS DATETIME)
//     （PG 对 timestamptz 按会话时区截断，MySQL 按会话墙钟截断，语义一致）；
//   - to_timestamp(floor(epoch/300)*300)（按绝对 epoch 对齐 5 分钟桶）
//     → FROM_UNIXTIME((FLOOR(UNIX_TIMESTAMP(ts) / 300)) * 300)
//     （UNIX_TIMESTAMP/FROM_UNIXTIME 在会话时区下互逆，桶界与绝对 epoch 对齐）。
func opsBucketExprForUsageMySQL(bucketSeconds int) string {
	switch bucketSeconds {
	case 3600:
		return "CAST(DATE_FORMAT(ul.created_at, '%Y-%m-%d %H:00:00') AS DATETIME)"
	case 300:
		return "FROM_UNIXTIME((FLOOR(UNIX_TIMESTAMP(ul.created_at) / 300)) * 300)"
	default:
		return "CAST(DATE_FORMAT(ul.created_at, '%Y-%m-%d %H:%i:00') AS DATETIME)"
	}
}

func opsBucketExprForErrorMySQL(bucketSeconds int) string {
	switch bucketSeconds {
	case 3600:
		return "CAST(DATE_FORMAT(created_at, '%Y-%m-%d %H:00:00') AS DATETIME)"
	case 300:
		return "FROM_UNIXTIME((FLOOR(UNIX_TIMESTAMP(created_at) / 300)) * 300)"
	default:
		return "CAST(DATE_FORMAT(created_at, '%Y-%m-%d %H:%i:00') AS DATETIME)"
	}
}

func opsBucketLabel(bucketSeconds int) string {
	if bucketSeconds <= 0 {
		return "1m"
	}
	if bucketSeconds%3600 == 0 {
		h := bucketSeconds / 3600
		if h <= 0 {
			h = 1
		}
		return fmt.Sprintf("%dh", h)
	}
	m := bucketSeconds / 60
	if m <= 0 {
		m = 1
	}
	return fmt.Sprintf("%dm", m)
}

func opsFloorToBucketStart(t time.Time, bucketSeconds int) time.Time {
	t = t.UTC()
	if bucketSeconds <= 0 {
		bucketSeconds = 60
	}
	secs := t.Unix()
	floored := secs - (secs % int64(bucketSeconds))
	return time.Unix(floored, 0).UTC()
}

func fillOpsThroughputBuckets(start, end time.Time, bucketSeconds int, points []*service.OpsThroughputTrendPoint) []*service.OpsThroughputTrendPoint {
	if bucketSeconds <= 0 {
		bucketSeconds = 60
	}
	if !start.Before(end) {
		return points
	}

	endMinus := end.Add(-time.Nanosecond)
	if endMinus.Before(start) {
		return points
	}

	first := opsFloorToBucketStart(start, bucketSeconds)
	last := opsFloorToBucketStart(endMinus, bucketSeconds)
	step := time.Duration(bucketSeconds) * time.Second

	existing := make(map[int64]*service.OpsThroughputTrendPoint, len(points))
	for _, p := range points {
		if p == nil {
			continue
		}
		existing[p.BucketStart.UTC().Unix()] = p
	}

	out := make([]*service.OpsThroughputTrendPoint, 0, int(last.Sub(first)/step)+1)
	for cursor := first; !cursor.After(last); cursor = cursor.Add(step) {
		if p, ok := existing[cursor.Unix()]; ok && p != nil {
			out = append(out, p)
			continue
		}
		out = append(out, &service.OpsThroughputTrendPoint{
			BucketStart:   cursor,
			RequestCount:  0,
			TokenConsumed: 0,
			SwitchCount:   0,
			QPS:           0,
			TPS:           0,
		})
	}
	return out
}

func (r *opsRepository) GetErrorTrend(ctx context.Context, filter *service.OpsDashboardFilter, bucketSeconds int) (*service.OpsErrorTrendResponse, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	if filter == nil {
		return nil, fmt.Errorf("nil filter")
	}
	if filter.StartTime.IsZero() || filter.EndTime.IsZero() {
		return nil, fmt.Errorf("start_time/end_time required")
	}

	if bucketSeconds <= 0 {
		bucketSeconds = 60
	}
	if bucketSeconds != 60 && bucketSeconds != 300 && bucketSeconds != 3600 {
		bucketSeconds = 60
	}

	start := filter.StartTime.UTC()
	end := filter.EndTime.UTC()
	if IsMySQL() {
		return r.getErrorTrendMySQL(ctx, filter, bucketSeconds, start, end)
	}
	where, args, _ := buildErrorWhere(filter, start, end, 1)
	bucketExpr := opsBucketExprForError(bucketSeconds)

	q := `
SELECT
  ` + bucketExpr + ` AS bucket,
  COUNT(*) FILTER (WHERE COALESCE(status_code, 0) >= 400) AS error_total,
  COUNT(*) FILTER (WHERE COALESCE(status_code, 0) >= 400 AND is_business_limited) AS business_limited,
  COUNT(*) FILTER (WHERE COALESCE(status_code, 0) >= 400 AND NOT is_business_limited) AS error_sla,
  COUNT(*) FILTER (WHERE error_owner = 'provider' AND NOT is_business_limited AND COALESCE(upstream_status_code, status_code, 0) NOT IN (429, 529)) AS upstream_excl,
  COUNT(*) FILTER (WHERE error_owner = 'provider' AND NOT is_business_limited AND COALESCE(upstream_status_code, status_code, 0) = 429) AS upstream_429,
  COUNT(*) FILTER (WHERE error_owner = 'provider' AND NOT is_business_limited AND COALESCE(upstream_status_code, status_code, 0) = 529) AS upstream_529
FROM ops_error_logs
` + where + `
GROUP BY 1
ORDER BY 1 ASC`

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	points := make([]*service.OpsErrorTrendPoint, 0, 256)
	for rows.Next() {
		var bucket time.Time
		var total, businessLimited, sla, upstreamExcl, upstream429, upstream529 int64
		if err := rows.Scan(&bucket, &total, &businessLimited, &sla, &upstreamExcl, &upstream429, &upstream529); err != nil {
			return nil, err
		}
		points = append(points, &service.OpsErrorTrendPoint{
			BucketStart: bucket.UTC(),

			ErrorCountTotal:      total,
			BusinessLimitedCount: businessLimited,
			ErrorCountSLA:        sla,

			UpstreamErrorCountExcl429529: upstreamExcl,
			Upstream429Count:             upstream429,
			Upstream529Count:             upstream529,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	points = fillOpsErrorTrendBuckets(start, end, bucketSeconds, points)

	return &service.OpsErrorTrendResponse{
		Bucket: opsBucketLabel(bucketSeconds),
		Points: points,
	}, nil
}

func fillOpsErrorTrendBuckets(start, end time.Time, bucketSeconds int, points []*service.OpsErrorTrendPoint) []*service.OpsErrorTrendPoint {
	if bucketSeconds <= 0 {
		bucketSeconds = 60
	}
	if !start.Before(end) {
		return points
	}

	endMinus := end.Add(-time.Nanosecond)
	if endMinus.Before(start) {
		return points
	}

	first := opsFloorToBucketStart(start, bucketSeconds)
	last := opsFloorToBucketStart(endMinus, bucketSeconds)
	step := time.Duration(bucketSeconds) * time.Second

	existing := make(map[int64]*service.OpsErrorTrendPoint, len(points))
	for _, p := range points {
		if p == nil {
			continue
		}
		existing[p.BucketStart.UTC().Unix()] = p
	}

	out := make([]*service.OpsErrorTrendPoint, 0, int(last.Sub(first)/step)+1)
	for cursor := first; !cursor.After(last); cursor = cursor.Add(step) {
		if p, ok := existing[cursor.Unix()]; ok && p != nil {
			out = append(out, p)
			continue
		}
		out = append(out, &service.OpsErrorTrendPoint{
			BucketStart: cursor,

			ErrorCountTotal:      0,
			BusinessLimitedCount: 0,
			ErrorCountSLA:        0,

			UpstreamErrorCountExcl429529: 0,
			Upstream429Count:             0,
			Upstream529Count:             0,
		})
	}
	return out
}

func (r *opsRepository) GetErrorDistribution(ctx context.Context, filter *service.OpsDashboardFilter) (*service.OpsErrorDistributionResponse, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	if filter == nil {
		return nil, fmt.Errorf("nil filter")
	}
	if filter.StartTime.IsZero() || filter.EndTime.IsZero() {
		return nil, fmt.Errorf("start_time/end_time required")
	}

	start := filter.StartTime.UTC()
	end := filter.EndTime.UTC()
	if IsMySQL() {
		return r.getErrorDistributionMySQL(ctx, filter, start, end)
	}
	where, args, _ := buildErrorWhere(filter, start, end, 1)

	q := `
SELECT
  COALESCE(upstream_status_code, status_code, 0) AS status_code,
  COUNT(*) AS total,
  COUNT(*) FILTER (WHERE NOT is_business_limited) AS sla,
  COUNT(*) FILTER (WHERE is_business_limited) AS business_limited
FROM ops_error_logs
` + where + `
  AND COALESCE(status_code, 0) >= 400
GROUP BY 1
ORDER BY total DESC
LIMIT 20`

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	items := make([]*service.OpsErrorDistributionItem, 0, 16)
	var total int64
	for rows.Next() {
		var statusCode int
		var cntTotal, cntSLA, cntBiz int64
		if err := rows.Scan(&statusCode, &cntTotal, &cntSLA, &cntBiz); err != nil {
			return nil, err
		}
		total += cntTotal
		items = append(items, &service.OpsErrorDistributionItem{
			StatusCode:      statusCode,
			Total:           cntTotal,
			SLA:             cntSLA,
			BusinessLimited: cntBiz,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return &service.OpsErrorDistributionResponse{
		Total: total,
		Items: items,
	}, nil
}

// getThroughputTrendMySQL 是 GetThroughputTrend 的 MySQL 实现，语义对应关系：
//   - CROSS JOIN LATERAL jsonb_array_elements(...) → CROSS JOIN JSON_TABLE（规范：
//     jsonb_array_elements 行展开 → JSON_TABLE）。JSON_TABLE 的 VARCHAR 列抽取
//     自动去除 JSON 字符串引号，对应 PG `ev->>'kind'`；数组元素缺 kind / 非对象元素
//     与 PG `->>` 一样得到 NULL，`IN` 判定同落 ELSE 0。
//   - split_part(x, ':', 1) → SUBSTRING_INDEX(x, ':', 1)（n=1 时等价）。
//   - 'null'::jsonb / '[]'::jsonb → CAST('null' AS JSON) / CAST('[]' AS JSON)。
//   - 桶表达式见 opsBucketExprForUsageMySQL / opsBucketExprForErrorMySQL；$n→?；
//     GROUP BY 1 → GROUP BY 别名。查询执行后的补桶/下钻逻辑与 PG 版逐行一致
//     （下钻函数各自内部分方言）。
//
// REVIEW: upstream_errors 若存有非数组 JSON（如对象），PG jsonb_array_elements 会直接报错，
// MySQL JSON_TABLE '$[*]' 则返回空集（该行不产生 switch 计数）；正常写入路径只写数组，影响有限。
func (r *opsRepository) getThroughputTrendMySQL(ctx context.Context, filter *service.OpsDashboardFilter, bucketSeconds int, start, end time.Time) (*service.OpsThroughputTrendResponse, error) {
	usageJoin, usageWhere, usageArgs, next := buildUsageWhereMySQL(filter, start, end, 1)
	errorWhere, errorArgs, _ := buildErrorWhereMySQL(filter, start, end, next)

	usageBucketExpr := opsBucketExprForUsageMySQL(bucketSeconds)
	errorBucketExpr := opsBucketExprForErrorMySQL(bucketSeconds)

	q := `
WITH usage_buckets AS (
  SELECT ` + usageBucketExpr + ` AS bucket,
         COUNT(*) AS success_count,
         COALESCE(SUM(input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens), 0) AS token_consumed
  FROM usage_logs ul
  ` + usageJoin + `
  ` + usageWhere + `
  GROUP BY bucket
),
error_buckets AS (
  SELECT ` + errorBucketExpr + ` AS bucket,
         COUNT(*) AS error_count
  FROM ops_error_logs
  ` + errorWhere + `
    AND COALESCE(status_code, 0) >= 400
  GROUP BY bucket
),
switch_buckets AS (
  SELECT ` + errorBucketExpr + ` AS bucket,
         COALESCE(SUM(CASE
           WHEN SUBSTRING_INDEX(ev.kind, ':', 1) IN ('failover', 'retry_exhausted_failover', 'failover_on_400') THEN 1
           ELSE 0
         END), 0) AS switch_count
  FROM ops_error_logs
  CROSS JOIN JSON_TABLE(
    COALESCE(NULLIF(upstream_errors, CAST('null' AS JSON)), CAST('[]' AS JSON)),
    '$[*]' COLUMNS (kind VARCHAR(255) PATH '$.kind')
  ) ev
  ` + errorWhere + `
    AND upstream_errors IS NOT NULL
  GROUP BY bucket
),
combined AS (
  SELECT
    bucket,
    SUM(success_count) AS success_count,
    SUM(error_count) AS error_count,
    SUM(token_consumed) AS token_consumed,
    SUM(switch_count) AS switch_count
  FROM (
    SELECT bucket, success_count, 0 AS error_count, token_consumed, 0 AS switch_count
    FROM usage_buckets
    UNION ALL
    SELECT bucket, 0, error_count, 0, 0
    FROM error_buckets
    UNION ALL
    SELECT bucket, 0, 0, 0, switch_count
    FROM switch_buckets
  ) t
  GROUP BY bucket
)
SELECT
  bucket,
  (success_count + error_count) AS request_count,
  token_consumed,
  switch_count
FROM combined
ORDER BY bucket ASC`

	args := append(usageArgs, errorArgs...)

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	points := make([]*service.OpsThroughputTrendPoint, 0, 256)
	for rows.Next() {
		var bucket time.Time
		var requests int64
		var tokens sql.NullInt64
		var switches sql.NullInt64
		if err := rows.Scan(&bucket, &requests, &tokens, &switches); err != nil {
			return nil, err
		}
		tokenConsumed := int64(0)
		if tokens.Valid {
			tokenConsumed = tokens.Int64
		}
		switchCount := int64(0)
		if switches.Valid {
			switchCount = switches.Int64
		}

		denom := float64(bucketSeconds)
		if denom <= 0 {
			denom = 60
		}
		qps := roundTo1DP(float64(requests) / denom)
		tps := roundTo1DP(float64(tokenConsumed) / denom)

		points = append(points, &service.OpsThroughputTrendPoint{
			BucketStart:   bucket.UTC(),
			RequestCount:  requests,
			TokenConsumed: tokenConsumed,
			SwitchCount:   switchCount,
			QPS:           qps,
			TPS:           tps,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Fill missing buckets with zeros so charts render continuous timelines.
	points = fillOpsThroughputBuckets(start, end, bucketSeconds, points)

	var byPlatform []*service.OpsThroughputPlatformBreakdownItem
	var topGroups []*service.OpsThroughputGroupBreakdownItem

	platform := ""
	if filter != nil {
		platform = strings.TrimSpace(strings.ToLower(filter.Platform))
	}
	groupID := (*int64)(nil)
	if filter != nil {
		groupID = filter.GroupID
	}

	// Drilldown helpers:
	// - No platform/group: totals by platform
	// - Platform selected but no group: top groups in that platform
	if platform == "" && (groupID == nil || *groupID <= 0) {
		items, err := r.getThroughputBreakdownByPlatform(ctx, start, end)
		if err != nil {
			return nil, err
		}
		byPlatform = items
	} else if platform != "" && (groupID == nil || *groupID <= 0) {
		items, err := r.getThroughputTopGroupsByPlatform(ctx, start, end, platform, 10)
		if err != nil {
			return nil, err
		}
		topGroups = items
	}

	return &service.OpsThroughputTrendResponse{
		Bucket: opsBucketLabel(bucketSeconds),
		Points: points,

		ByPlatform: byPlatform,
		TopGroups:  topGroups,
	}, nil
}

// getErrorTrendMySQL 是 GetErrorTrend 的 MySQL 实现：
// COUNT(*) FILTER (WHERE cond) → SUM(CASE WHEN cond THEN 1 ELSE 0 END)；
// 桶表达式见 opsBucketExprForErrorMySQL；$n→?；GROUP BY 1/ORDER BY 1 → GROUP BY/ORDER BY 别名。
// 补桶逻辑与 PG 版一致。
func (r *opsRepository) getErrorTrendMySQL(ctx context.Context, filter *service.OpsDashboardFilter, bucketSeconds int, start, end time.Time) (*service.OpsErrorTrendResponse, error) {
	where, args, _ := buildErrorWhereMySQL(filter, start, end, 1)
	bucketExpr := opsBucketExprForErrorMySQL(bucketSeconds)

	q := `
SELECT
  ` + bucketExpr + ` AS bucket,
  SUM(CASE WHEN COALESCE(status_code, 0) >= 400 THEN 1 ELSE 0 END) AS error_total,
  SUM(CASE WHEN COALESCE(status_code, 0) >= 400 AND is_business_limited THEN 1 ELSE 0 END) AS business_limited,
  SUM(CASE WHEN COALESCE(status_code, 0) >= 400 AND NOT is_business_limited THEN 1 ELSE 0 END) AS error_sla,
  SUM(CASE WHEN error_owner = 'provider' AND NOT is_business_limited AND COALESCE(upstream_status_code, status_code, 0) NOT IN (429, 529) THEN 1 ELSE 0 END) AS upstream_excl,
  SUM(CASE WHEN error_owner = 'provider' AND NOT is_business_limited AND COALESCE(upstream_status_code, status_code, 0) = 429 THEN 1 ELSE 0 END) AS upstream_429,
  SUM(CASE WHEN error_owner = 'provider' AND NOT is_business_limited AND COALESCE(upstream_status_code, status_code, 0) = 529 THEN 1 ELSE 0 END) AS upstream_529
FROM ops_error_logs
` + where + `
GROUP BY bucket
ORDER BY bucket ASC`

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	points := make([]*service.OpsErrorTrendPoint, 0, 256)
	for rows.Next() {
		var bucket time.Time
		var total, businessLimited, sla, upstreamExcl, upstream429, upstream529 int64
		if err := rows.Scan(&bucket, &total, &businessLimited, &sla, &upstreamExcl, &upstream429, &upstream529); err != nil {
			return nil, err
		}
		points = append(points, &service.OpsErrorTrendPoint{
			BucketStart: bucket.UTC(),

			ErrorCountTotal:      total,
			BusinessLimitedCount: businessLimited,
			ErrorCountSLA:        sla,

			UpstreamErrorCountExcl429529: upstreamExcl,
			Upstream429Count:             upstream429,
			Upstream529Count:             upstream529,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	points = fillOpsErrorTrendBuckets(start, end, bucketSeconds, points)

	return &service.OpsErrorTrendResponse{
		Bucket: opsBucketLabel(bucketSeconds),
		Points: points,
	}, nil
}

// getErrorDistributionMySQL 是 GetErrorDistribution 的 MySQL 实现：
// COUNT(*) FILTER (WHERE cond) → SUM(CASE WHEN cond THEN 1 ELSE 0 END)；$n→?；
// GROUP BY 1 → GROUP BY 别名（表达式与 PG 相同）。
func (r *opsRepository) getErrorDistributionMySQL(ctx context.Context, filter *service.OpsDashboardFilter, start, end time.Time) (*service.OpsErrorDistributionResponse, error) {
	where, args, _ := buildErrorWhereMySQL(filter, start, end, 1)

	q := `
SELECT
  COALESCE(upstream_status_code, status_code, 0) AS status_code,
  COUNT(*) AS total,
  SUM(CASE WHEN NOT is_business_limited THEN 1 ELSE 0 END) AS sla,
  SUM(CASE WHEN is_business_limited THEN 1 ELSE 0 END) AS business_limited
FROM ops_error_logs
` + where + `
  AND COALESCE(status_code, 0) >= 400
GROUP BY status_code
ORDER BY total DESC
LIMIT 20`

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	items := make([]*service.OpsErrorDistributionItem, 0, 16)
	var total int64
	for rows.Next() {
		var statusCode int
		var cntTotal, cntSLA, cntBiz int64
		if err := rows.Scan(&statusCode, &cntTotal, &cntSLA, &cntBiz); err != nil {
			return nil, err
		}
		total += cntTotal
		items = append(items, &service.OpsErrorDistributionItem{
			StatusCode:      statusCode,
			Total:           cntTotal,
			SLA:             cntSLA,
			BusinessLimited: cntBiz,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return &service.OpsErrorDistributionResponse{
		Total: total,
		Items: items,
	}, nil
}
