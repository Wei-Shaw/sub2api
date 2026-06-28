package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

const defaultOpsAlertAccountBreakdownLimit = 5

func (r *opsRepository) ListAlertAccountBreakdown(ctx context.Context, filter *service.OpsAlertAccountBreakdownFilter) ([]*service.OpsAlertAccountBreakdown, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	if filter == nil {
		return nil, fmt.Errorf("nil filter")
	}
	start := filter.StartTime.UTC()
	end := filter.EndTime.UTC()
	if start.IsZero() || end.IsZero() || !start.Before(end) {
		return []*service.OpsAlertAccountBreakdown{}, nil
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = defaultOpsAlertAccountBreakdownLimit
	}
	if limit > 10 {
		limit = 10
	}

	dashboardFilter := &service.OpsDashboardFilter{
		StartTime: start,
		EndTime:   end,
		Platform:  filter.Platform,
		GroupID:   filter.GroupID,
	}
	usageJoin, usageWhere, usageArgs, next := buildUsageWhere(dashboardFilter, start, end, 1)
	errorWhere, errorArgs, next := buildErrorWhere(dashboardFilter, start, end, next)
	orderBy := opsAlertAccountBreakdownOrderBy(filter.MetricType)

	q := fmt.Sprintf(`
WITH usage_agg AS (
  SELECT
    ul.account_id,
    MIN(ul.group_id) FILTER (WHERE ul.group_id IS NOT NULL) AS group_id,
    COUNT(*)::bigint AS success_count,
    percentile_cont(0.95) WITHIN GROUP (ORDER BY ul.duration_ms) FILTER (WHERE ul.duration_ms IS NOT NULL) AS duration_p95_ms,
    percentile_cont(0.99) WITHIN GROUP (ORDER BY ul.duration_ms) FILTER (WHERE ul.duration_ms IS NOT NULL) AS duration_p99_ms,
    AVG(ul.duration_ms) FILTER (WHERE ul.duration_ms IS NOT NULL) AS duration_avg_ms,
    MAX(ul.duration_ms) AS duration_max_ms
  FROM usage_logs ul
  %s
  %s
  GROUP BY ul.account_id
),
error_agg AS (
  SELECT
    e.account_id,
    MIN(e.group_id) FILTER (WHERE e.group_id IS NOT NULL) AS group_id,
    MIN(COALESCE(NULLIF(e.platform, ''), '')) AS platform,
    COALESCE(COUNT(*) FILTER (WHERE COALESCE(e.status_code, 0) >= 400), 0)::bigint AS error_count_total,
    COALESCE(COUNT(*) FILTER (WHERE COALESCE(e.status_code, 0) >= 400 AND NOT e.is_business_limited), 0)::bigint AS error_count_sla,
    COALESCE(COUNT(*) FILTER (WHERE COALESCE(e.status_code, 0) >= 400 AND e.is_business_limited), 0)::bigint AS business_limited_count,
    COALESCE(COUNT(*) FILTER (WHERE e.error_owner = 'provider' AND NOT e.is_business_limited AND COALESCE(e.upstream_status_code, e.status_code, 0) NOT IN (429, 529)), 0)::bigint AS upstream_error_count,
    (array_agg(e.status_code ORDER BY e.created_at DESC) FILTER (WHERE e.status_code IS NOT NULL))[1] AS last_error_status_code,
    (array_agg(NULLIF(e.error_type, '') ORDER BY e.created_at DESC) FILTER (WHERE COALESCE(e.error_type, '') <> ''))[1] AS last_error_type,
    (array_agg(NULLIF(COALESCE(e.upstream_error_message, e.error_message, ''), '') ORDER BY e.created_at DESC) FILTER (WHERE COALESCE(e.upstream_error_message, e.error_message, '') <> ''))[1] AS last_error_message
  FROM ops_error_logs e
  %s
  GROUP BY e.account_id
),
combined_base AS (
  SELECT
    u.account_id,
    u.group_id,
    NULL::text AS platform,
    u.success_count,
    0::bigint AS error_count_total,
    0::bigint AS error_count_sla,
    0::bigint AS business_limited_count,
    0::bigint AS upstream_error_count,
    u.duration_p95_ms,
    u.duration_p99_ms,
    u.duration_avg_ms,
    u.duration_max_ms,
    NULL::integer AS last_error_status_code,
    NULL::text AS last_error_type,
    NULL::text AS last_error_message
  FROM usage_agg u
  UNION ALL
  SELECT
    e.account_id,
    e.group_id,
    e.platform,
    0::bigint AS success_count,
    e.error_count_total,
    e.error_count_sla,
    e.business_limited_count,
    e.upstream_error_count,
    NULL::double precision AS duration_p95_ms,
    NULL::double precision AS duration_p99_ms,
    NULL::double precision AS duration_avg_ms,
    NULL::integer AS duration_max_ms,
    e.last_error_status_code,
    e.last_error_type,
    e.last_error_message
  FROM error_agg e
),
combined AS (
  SELECT
    account_id,
    MIN(group_id) FILTER (WHERE group_id IS NOT NULL) AS group_id,
    COALESCE(MIN(NULLIF(platform, '')), '') AS platform,
    COALESCE(SUM(success_count), 0)::bigint AS success_count,
    COALESCE(SUM(error_count_total), 0)::bigint AS error_count_total,
    COALESCE(SUM(error_count_sla), 0)::bigint AS error_count_sla,
    COALESCE(SUM(business_limited_count), 0)::bigint AS business_limited_count,
    COALESCE(SUM(upstream_error_count), 0)::bigint AS upstream_error_count,
    MAX(duration_p95_ms) AS duration_p95_ms,
    MAX(duration_p99_ms) AS duration_p99_ms,
    MAX(duration_avg_ms) AS duration_avg_ms,
    MAX(duration_max_ms) AS duration_max_ms,
    MAX(last_error_status_code) AS last_error_status_code,
    MAX(last_error_type) AS last_error_type,
    MAX(last_error_message) AS last_error_message
  FROM combined_base
  GROUP BY account_id
)
SELECT
  c.account_id,
  COALESCE(NULLIF(a.name, ''), CASE WHEN c.account_id IS NULL THEN 'Không rõ account' ELSE 'account#' || c.account_id::text END) AS account_name,
  COALESCE(NULLIF(a.platform, ''), NULLIF(gr.platform, ''), NULLIF(c.platform, ''), '') AS platform,
  c.group_id,
  COALESCE(NULLIF(gr.name, ''), '') AS group_name,
  c.success_count,
  c.error_count_total,
  c.error_count_sla,
  c.business_limited_count,
  c.upstream_error_count,
  (c.success_count + c.error_count_sla)::bigint AS request_count_sla,
  CASE WHEN (c.success_count + c.error_count_sla) > 0 THEN (c.error_count_sla::double precision / (c.success_count + c.error_count_sla)::double precision) * 100 ELSE 0 END AS error_rate,
  c.duration_p95_ms,
  c.duration_p99_ms,
  c.duration_avg_ms,
  c.duration_max_ms,
  c.last_error_status_code,
  COALESCE(c.last_error_type, '') AS last_error_type,
  COALESCE(c.last_error_message, '') AS last_error_message
FROM combined c
LEFT JOIN accounts a ON a.id = c.account_id
LEFT JOIN groups gr ON gr.id = c.group_id
WHERE (c.success_count + c.error_count_total) > 0
ORDER BY %s
LIMIT $%d`, usageJoin, usageWhere, errorWhere, orderBy, next)

	args := append(usageArgs, errorArgs...)
	args = append(args, limit)

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	items := make([]*service.OpsAlertAccountBreakdown, 0, limit)
	for rows.Next() {
		var item service.OpsAlertAccountBreakdown
		var accountID sql.NullInt64
		var groupID sql.NullInt64
		var durationP95, durationP99, durationAvg sql.NullFloat64
		var durationMax sql.NullInt64
		var lastStatus sql.NullInt64
		if err := rows.Scan(
			&accountID,
			&item.AccountName,
			&item.Platform,
			&groupID,
			&item.GroupName,
			&item.SuccessCount,
			&item.ErrorCountTotal,
			&item.ErrorCountSLA,
			&item.BusinessLimitedCount,
			&item.UpstreamErrorCount,
			&item.RequestCountSLA,
			&item.ErrorRate,
			&durationP95,
			&durationP99,
			&durationAvg,
			&durationMax,
			&lastStatus,
			&item.LastErrorType,
			&item.LastErrorMessage,
		); err != nil {
			return nil, err
		}
		if accountID.Valid {
			v := accountID.Int64
			item.AccountID = &v
		}
		if groupID.Valid {
			v := groupID.Int64
			item.GroupID = &v
		}
		item.DurationP95Ms = floatToIntPtr(durationP95)
		item.DurationP99Ms = floatToIntPtr(durationP99)
		item.DurationAvgMs = floatToIntPtr(durationAvg)
		if durationMax.Valid {
			v := int(durationMax.Int64)
			item.DurationMaxMs = &v
		}
		if lastStatus.Valid {
			v := int(lastStatus.Int64)
			item.LastErrorStatusCode = &v
		}
		items = append(items, &item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func opsAlertAccountBreakdownOrderBy(metricType string) string {
	switch strings.TrimSpace(metricType) {
	case "p99_latency_ms":
		return "c.duration_p99_ms DESC NULLS LAST, c.duration_max_ms DESC NULLS LAST, request_count_sla DESC, account_name ASC"
	case "p95_latency_ms":
		return "c.duration_p95_ms DESC NULLS LAST, c.duration_max_ms DESC NULLS LAST, request_count_sla DESC, account_name ASC"
	case "upstream_error_rate":
		return "c.upstream_error_count DESC, error_rate DESC, c.error_count_sla DESC, request_count_sla DESC, account_name ASC"
	default:
		return "c.error_count_sla DESC, error_rate DESC, request_count_sla DESC, c.duration_p95_ms DESC NULLS LAST, account_name ASC"
	}
}
