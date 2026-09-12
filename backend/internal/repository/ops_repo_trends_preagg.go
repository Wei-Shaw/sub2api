package repository

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// 分钟预聚合的安全边界：聚合作业按 opsAggMinuteSafeDelay(90s) 落后于当前时间推进，
// 这里留出更宽的余量，尾部不足一个桶的部分交给 raw 段补齐。
const opsMinutePreaggSafeLag = 3 * time.Minute

// opsMinutePreaggSafeEnd 返回可以信任分钟预聚合的时间上界。
func opsMinutePreaggSafeEnd(endTime time.Time) time.Time {
	cutoff := time.Now().UTC().Add(-opsMinutePreaggSafeLag)
	if endTime.After(cutoff) {
		return cutoff
	}
	return endTime
}

func utcFloorToBucket(t time.Time, bucketSeconds int) time.Time {
	if bucketSeconds <= 0 {
		bucketSeconds = 60
	}
	step := time.Duration(bucketSeconds) * time.Second
	return t.UTC().Truncate(step)
}

func utcCeilToBucket(t time.Time, bucketSeconds int) time.Time {
	u := t.UTC()
	f := utcFloorToBucket(u, bucketSeconds)
	if f.Equal(u) {
		return f
	}
	return f.Add(time.Duration(bucketSeconds) * time.Second)
}

// buildMinutePreaggWhere 把过滤条件映射到 ops_metrics_minute 的三种维度行。
// 写入侧用 GROUPING SETS 产出 (overall) / (platform) / (platform, group) 三档，
// 因此这里的匹配规则与 listHourlyMetricsRows 完全一致。
func buildMinutePreaggWhere(filter *service.OpsDashboardFilter, start, end time.Time) (string, []any) {
	where := "bucket_start >= $1 AND bucket_start < $2"
	args := []any{start.UTC(), end.UTC()}
	idx := 3

	platform := ""
	groupID := (*int64)(nil)
	if filter != nil {
		platform = strings.TrimSpace(strings.ToLower(filter.Platform))
		groupID = filter.GroupID
	}

	switch {
	case groupID != nil && *groupID > 0:
		where += fmt.Sprintf(" AND group_id = $%d", idx)
		args = append(args, *groupID)
		idx++
		if platform != "" {
			where += fmt.Sprintf(" AND platform = $%d", idx)
			args = append(args, platform)
		}
	case platform != "":
		where += fmt.Sprintf(" AND platform = $%d AND group_id IS NULL", idx)
		args = append(args, platform)
	default:
		where += " AND platform IS NULL AND group_id IS NULL"
	}

	return where, args
}

// minutePreaggRowsExist 判断给定窗口内分钟表是否已有数据，用于区分
// 「这段时间本来就没流量」和「预聚合还没跑到」。
func (r *opsRepository) minutePreaggRowsExist(ctx context.Context, start, end time.Time) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx, `
SELECT EXISTS (
  SELECT 1 FROM ops_metrics_minute
  WHERE bucket_start >= $1 AND bucket_start < $2
)`, start.UTC(), end.UTC()).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}

type opsThroughputAgg struct {
	requestCount  int64
	tokenConsumed int64
	switchCount   int64
}

// queryThroughputFromMinutePreagg 按目标桶粒度把分钟行折叠成 trend 点。
func (r *opsRepository) queryThroughputFromMinutePreagg(
	ctx context.Context,
	filter *service.OpsDashboardFilter,
	start, end time.Time,
	bucketSeconds int,
) (map[int64]*opsThroughputAgg, error) {
	where, args := buildMinutePreaggWhere(filter, start, end)
	args = append(args, bucketSeconds)
	bucketArg := fmt.Sprintf("$%d", len(args))

	q := `
SELECT
  to_timestamp(floor(extract(epoch FROM bucket_start) / ` + bucketArg + `) * ` + bucketArg + `) AS bucket,
  COALESCE(SUM(success_count + error_count_total), 0) AS request_count,
  COALESCE(SUM(token_consumed), 0) AS token_consumed,
  COALESCE(SUM(switch_count), 0) AS switch_count
FROM ops_metrics_minute
WHERE ` + where + `
GROUP BY 1`

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make(map[int64]*opsThroughputAgg, 256)
	for rows.Next() {
		var bucket time.Time
		var requests, tokens, switches int64
		if err := rows.Scan(&bucket, &requests, &tokens, &switches); err != nil {
			return nil, err
		}
		out[bucket.UTC().Unix()] = &opsThroughputAgg{
			requestCount:  requests,
			tokenConsumed: tokens,
			switchCount:   switches,
		}
	}
	return out, rows.Err()
}

// queryThroughputFromRaw 直接从明细统计一段（仅用于 preagg 覆盖不到的首尾碎片）。
func (r *opsRepository) queryThroughputFromRaw(
	ctx context.Context,
	filter *service.OpsDashboardFilter,
	start, end time.Time,
	bucketSeconds int,
) (map[int64]*opsThroughputAgg, error) {
	if !start.Before(end) {
		return map[int64]*opsThroughputAgg{}, nil
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
SELECT bucket, (success_count + error_count) AS request_count, token_consumed, switch_count
FROM combined`

	args := append(usageArgs, errorArgs...)
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make(map[int64]*opsThroughputAgg, 128)
	for rows.Next() {
		var bucket time.Time
		var requests int64
		var tokens, switches sql.NullInt64
		if err := rows.Scan(&bucket, &requests, &tokens, &switches); err != nil {
			return nil, err
		}
		out[bucket.UTC().Unix()] = &opsThroughputAgg{
			requestCount:  requests,
			tokenConsumed: tokens.Int64,
			switchCount:   switches.Int64,
		}
	}
	return out, rows.Err()
}

// getThroughputTrendPreaggregated 用分钟预聚合服务 trend，首尾不足一个稳定桶的部分回落到明细。
//
// 三种桶粒度（60/300/3600）统一由 ops_metrics_minute 折叠得出，不混用 ops_metrics_hourly：
// 后者的 usage 侧是 INNER JOIN groups，会丢掉 group_id IS NULL 的行，与本函数的
// raw 首尾段口径不一致，拼接处会出现数字跳变。
func (r *opsRepository) getThroughputTrendPreaggregated(
	ctx context.Context,
	filter *service.OpsDashboardFilter,
	bucketSeconds int,
) (*service.OpsThroughputTrendResponse, error) {
	start := filter.StartTime.UTC()
	end := filter.EndTime.UTC()

	aggSafeEnd := opsMinutePreaggSafeEnd(end)
	aggFullStart := utcCeilToBucket(start, bucketSeconds)
	aggFullEnd := utcFloorToBucket(aggSafeEnd, bucketSeconds)

	// 窗口太短，凑不出一个完整的稳定桶：直接走明细。
	if !aggFullStart.Before(aggFullEnd) {
		return r.getThroughputTrendRaw(ctx, filter, bucketSeconds)
	}

	preagg, err := r.queryThroughputFromMinutePreagg(ctx, filter, aggFullStart, aggFullEnd, bucketSeconds)
	if err != nil {
		return nil, err
	}
	if len(preagg) == 0 {
		// 区分「没有流量」与「预聚合尚未覆盖」。
		exists, err := r.minutePreaggRowsExist(ctx, aggFullStart, aggFullEnd)
		if err == nil && !exists {
			if rawExists, err := r.rawOpsDataExists(ctx, filter, aggFullStart, aggFullEnd); err == nil && rawExists {
				return nil, service.ErrOpsPreaggregatedNotPopulated
			}
		}
	}

	merged := preagg

	// 首尾碎片走明细：各自最多一个桶的跨度，代价可控。
	if start.Before(aggFullStart) {
		head, err := r.queryThroughputFromRaw(ctx, filter, start, minTime(end, aggFullStart), bucketSeconds)
		if err != nil {
			return nil, err
		}
		mergeThroughputAgg(merged, head)
	}
	if aggFullEnd.Before(end) {
		tail, err := r.queryThroughputFromRaw(ctx, filter, maxTime(start, aggFullEnd), end, bucketSeconds)
		if err != nil {
			return nil, err
		}
		mergeThroughputAgg(merged, tail)
	}

	points := make([]*service.OpsThroughputTrendPoint, 0, len(merged))
	denom := float64(bucketSeconds)
	if denom <= 0 {
		denom = 60
	}
	for ts, agg := range merged {
		points = append(points, &service.OpsThroughputTrendPoint{
			BucketStart:   time.Unix(ts, 0).UTC(),
			RequestCount:  agg.requestCount,
			TokenConsumed: agg.tokenConsumed,
			SwitchCount:   agg.switchCount,
			QPS:           roundTo1DP(float64(agg.requestCount) / denom),
			TPS:           roundTo1DP(float64(agg.tokenConsumed) / denom),
		})
	}
	sortThroughputPoints(points)
	points = fillOpsThroughputBuckets(start, end, bucketSeconds, points)

	byPlatform, topGroups, err := r.throughputBreakdownsPreagg(ctx, filter, aggFullStart, aggFullEnd)
	if err != nil {
		return nil, err
	}

	return &service.OpsThroughputTrendResponse{
		Bucket:     opsBucketLabel(bucketSeconds),
		Points:     points,
		ByPlatform: byPlatform,
		TopGroups:  topGroups,
	}, nil
}

type opsErrorAgg struct {
	errorCountTotal      int64
	businessLimitedCount int64
	errorCountSLA        int64
	upstreamExcl429529   int64
	upstream429Count     int64
	upstream529Count     int64
}

// queryErrorFromMinutePreagg 按目标桶粒度折叠分钟表的错误分项。
func (r *opsRepository) queryErrorFromMinutePreagg(
	ctx context.Context,
	filter *service.OpsDashboardFilter,
	start, end time.Time,
	bucketSeconds int,
) (map[int64]*opsErrorAgg, error) {
	where, args := buildMinutePreaggWhere(filter, start, end)
	args = append(args, bucketSeconds)
	bucketArg := fmt.Sprintf("$%d", len(args))

	q := `
SELECT
  to_timestamp(floor(extract(epoch FROM bucket_start) / ` + bucketArg + `) * ` + bucketArg + `) AS bucket,
  COALESCE(SUM(error_count_total), 0),
  COALESCE(SUM(business_limited_count), 0),
  COALESCE(SUM(error_count_sla), 0),
  COALESCE(SUM(upstream_error_count_excl_429_529), 0),
  COALESCE(SUM(upstream_429_count), 0),
  COALESCE(SUM(upstream_529_count), 0)
FROM ops_metrics_minute
WHERE ` + where + `
GROUP BY 1`

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make(map[int64]*opsErrorAgg, 256)
	for rows.Next() {
		var bucket time.Time
		agg := &opsErrorAgg{}
		if err := rows.Scan(
			&bucket,
			&agg.errorCountTotal,
			&agg.businessLimitedCount,
			&agg.errorCountSLA,
			&agg.upstreamExcl429529,
			&agg.upstream429Count,
			&agg.upstream529Count,
		); err != nil {
			return nil, err
		}
		out[bucket.UTC().Unix()] = agg
	}
	return out, rows.Err()
}

// queryErrorFromRaw 统计一段明细，仅用于 preagg 覆盖不到的首尾碎片。
func (r *opsRepository) queryErrorFromRaw(
	ctx context.Context,
	filter *service.OpsDashboardFilter,
	start, end time.Time,
	bucketSeconds int,
) (map[int64]*opsErrorAgg, error) {
	if !start.Before(end) {
		return map[int64]*opsErrorAgg{}, nil
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
GROUP BY 1`

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make(map[int64]*opsErrorAgg, 128)
	for rows.Next() {
		var bucket time.Time
		agg := &opsErrorAgg{}
		if err := rows.Scan(
			&bucket,
			&agg.errorCountTotal,
			&agg.businessLimitedCount,
			&agg.errorCountSLA,
			&agg.upstreamExcl429529,
			&agg.upstream429Count,
			&agg.upstream529Count,
		); err != nil {
			return nil, err
		}
		out[bucket.UTC().Unix()] = agg
	}
	return out, rows.Err()
}

// getErrorTrendPreaggregated 与 throughput 版同构：稳定段走分钟预聚合，首尾碎片回落明细。
func (r *opsRepository) getErrorTrendPreaggregated(
	ctx context.Context,
	filter *service.OpsDashboardFilter,
	bucketSeconds int,
) (*service.OpsErrorTrendResponse, error) {
	start := filter.StartTime.UTC()
	end := filter.EndTime.UTC()

	aggSafeEnd := opsMinutePreaggSafeEnd(end)
	aggFullStart := utcCeilToBucket(start, bucketSeconds)
	aggFullEnd := utcFloorToBucket(aggSafeEnd, bucketSeconds)

	if !aggFullStart.Before(aggFullEnd) {
		return r.getErrorTrendRaw(ctx, filter, bucketSeconds)
	}

	merged, err := r.queryErrorFromMinutePreagg(ctx, filter, aggFullStart, aggFullEnd, bucketSeconds)
	if err != nil {
		return nil, err
	}
	if len(merged) == 0 {
		exists, err := r.minutePreaggRowsExist(ctx, aggFullStart, aggFullEnd)
		if err == nil && !exists {
			if rawExists, err := r.rawOpsDataExists(ctx, filter, aggFullStart, aggFullEnd); err == nil && rawExists {
				return nil, service.ErrOpsPreaggregatedNotPopulated
			}
		}
	}

	if start.Before(aggFullStart) {
		head, err := r.queryErrorFromRaw(ctx, filter, start, minTime(end, aggFullStart), bucketSeconds)
		if err != nil {
			return nil, err
		}
		mergeErrorAgg(merged, head)
	}
	if aggFullEnd.Before(end) {
		tail, err := r.queryErrorFromRaw(ctx, filter, maxTime(start, aggFullEnd), end, bucketSeconds)
		if err != nil {
			return nil, err
		}
		mergeErrorAgg(merged, tail)
	}

	points := make([]*service.OpsErrorTrendPoint, 0, len(merged))
	for ts, agg := range merged {
		points = append(points, &service.OpsErrorTrendPoint{
			BucketStart:                  time.Unix(ts, 0).UTC(),
			ErrorCountTotal:              agg.errorCountTotal,
			BusinessLimitedCount:         agg.businessLimitedCount,
			ErrorCountSLA:                agg.errorCountSLA,
			UpstreamErrorCountExcl429529: agg.upstreamExcl429529,
			Upstream429Count:             agg.upstream429Count,
			Upstream529Count:             agg.upstream529Count,
		})
	}
	sort.Slice(points, func(i, j int) bool {
		return points[i].BucketStart.Before(points[j].BucketStart)
	})
	points = fillOpsErrorTrendBuckets(start, end, bucketSeconds, points)

	return &service.OpsErrorTrendResponse{
		Bucket: opsBucketLabel(bucketSeconds),
		Points: points,
	}, nil
}

func mergeErrorAgg(dst, src map[int64]*opsErrorAgg) {
	for ts, agg := range src {
		cur, ok := dst[ts]
		if !ok {
			cur = &opsErrorAgg{}
			dst[ts] = cur
		}
		cur.errorCountTotal += agg.errorCountTotal
		cur.businessLimitedCount += agg.businessLimitedCount
		cur.errorCountSLA += agg.errorCountSLA
		cur.upstreamExcl429529 += agg.upstreamExcl429529
		cur.upstream429Count += agg.upstream429Count
		cur.upstream529Count += agg.upstream529Count
	}
}

func sortThroughputPoints(points []*service.OpsThroughputTrendPoint) {
	sort.Slice(points, func(i, j int) bool {
		return points[i].BucketStart.Before(points[j].BucketStart)
	})
}

func mergeThroughputAgg(dst, src map[int64]*opsThroughputAgg) {
	for ts, agg := range src {
		if cur, ok := dst[ts]; ok {
			cur.requestCount += agg.requestCount
			cur.tokenConsumed += agg.tokenConsumed
			cur.switchCount += agg.switchCount
			continue
		}
		dst[ts] = &opsThroughputAgg{
			requestCount:  agg.requestCount,
			tokenConsumed: agg.tokenConsumed,
			switchCount:   agg.switchCount,
		}
	}
}

// throughputBreakdownsPreagg 复用分钟表的 platform / group 维度行产出下钻数据。
func (r *opsRepository) throughputBreakdownsPreagg(
	ctx context.Context,
	filter *service.OpsDashboardFilter,
	start, end time.Time,
) ([]*service.OpsThroughputPlatformBreakdownItem, []*service.OpsThroughputGroupBreakdownItem, error) {
	platform := ""
	groupID := (*int64)(nil)
	if filter != nil {
		platform = strings.TrimSpace(strings.ToLower(filter.Platform))
		groupID = filter.GroupID
	}

	switch {
	case platform == "" && (groupID == nil || *groupID <= 0):
		items, err := r.getThroughputBreakdownByPlatformPreagg(ctx, start, end)
		return items, nil, err
	case platform != "" && (groupID == nil || *groupID <= 0):
		items, err := r.getThroughputTopGroupsByPlatformPreagg(ctx, start, end, platform, 10)
		return nil, items, err
	default:
		return nil, nil, nil
	}
}

func (r *opsRepository) getThroughputBreakdownByPlatformPreagg(
	ctx context.Context,
	start, end time.Time,
) ([]*service.OpsThroughputPlatformBreakdownItem, error) {
	q := `
SELECT platform,
       COALESCE(SUM(success_count + error_count_total), 0) AS request_count,
       COALESCE(SUM(token_consumed), 0) AS token_consumed
FROM ops_metrics_minute
WHERE bucket_start >= $1 AND bucket_start < $2
  AND platform IS NOT NULL AND platform <> ''
  AND group_id IS NULL
GROUP BY 1
ORDER BY request_count DESC`

	rows, err := r.db.QueryContext(ctx, q, start.UTC(), end.UTC())
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	items := make([]*service.OpsThroughputPlatformBreakdownItem, 0, 8)
	for rows.Next() {
		var platform string
		var requestCount, tokenConsumed int64
		if err := rows.Scan(&platform, &requestCount, &tokenConsumed); err != nil {
			return nil, err
		}
		items = append(items, &service.OpsThroughputPlatformBreakdownItem{
			Platform:      platform,
			RequestCount:  requestCount,
			TokenConsumed: tokenConsumed,
		})
	}
	return items, rows.Err()
}

func (r *opsRepository) getThroughputTopGroupsByPlatformPreagg(
	ctx context.Context,
	start, end time.Time,
	platform string,
	limit int,
) ([]*service.OpsThroughputGroupBreakdownItem, error) {
	if strings.TrimSpace(platform) == "" {
		return nil, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 10
	}

	q := `
SELECT m.group_id,
       COALESCE(g.name, '') AS group_name,
       COALESCE(SUM(m.success_count + m.error_count_total), 0) AS request_count,
       COALESCE(SUM(m.token_consumed), 0) AS token_consumed
FROM ops_metrics_minute m
LEFT JOIN groups g ON g.id = m.group_id
WHERE m.bucket_start >= $1 AND m.bucket_start < $2
  AND m.platform = $3
  AND m.group_id IS NOT NULL
GROUP BY 1, 2
ORDER BY request_count DESC
LIMIT $4`

	rows, err := r.db.QueryContext(ctx, q, start.UTC(), end.UTC(), platform, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	items := make([]*service.OpsThroughputGroupBreakdownItem, 0, limit)
	for rows.Next() {
		var groupID int64
		var groupName string
		var requestCount, tokenConsumed int64
		if err := rows.Scan(&groupID, &groupName, &requestCount, &tokenConsumed); err != nil {
			return nil, err
		}
		items = append(items, &service.OpsThroughputGroupBreakdownItem{
			GroupID:       groupID,
			GroupName:     groupName,
			RequestCount:  requestCount,
			TokenConsumed: tokenConsumed,
		})
	}
	return items, rows.Err()
}
