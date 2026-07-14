package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (r *opsRepository) GetUserUsageStats(ctx context.Context, filter *service.OpsUserUsageStatsFilter) (*service.OpsUserUsageStatsResponse, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	if filter == nil {
		return nil, fmt.Errorf("nil filter")
	}
	if filter.StartTime.IsZero() || filter.EndTime.IsZero() {
		return nil, fmt.Errorf("start_time/end_time required")
	}
	if filter.StartTime.After(filter.EndTime) {
		return nil, fmt.Errorf("start_time must be <= end_time")
	}

	dashboardFilter := &service.OpsDashboardFilter{
		StartTime: filter.StartTime.UTC(),
		EndTime:   filter.EndTime.UTC(),
		Platform:  strings.TrimSpace(strings.ToLower(filter.Platform)),
		GroupID:   filter.GroupID,
	}

	join, where, baseArgs, next := buildUsageWhere(dashboardFilter, dashboardFilter.StartTime, dashboardFilter.EndTime, 1)
	baseCTE := `
WITH stats AS (
  SELECT
    ul.user_id,
    COALESCE(u.username, '') AS username,
    COALESCE(u.email, '') AS email,
    COUNT(*)::bigint AS request_count,
    COALESCE(SUM(ul.input_tokens), 0)::bigint AS input_tokens,
    COALESCE(SUM(ul.output_tokens), 0)::bigint AS output_tokens,
    COALESCE(SUM(ul.cache_creation_tokens + ul.cache_read_tokens), 0)::bigint AS cache_tokens,
    COALESCE(SUM(ul.input_tokens + ul.output_tokens + ul.cache_creation_tokens + ul.cache_read_tokens), 0)::bigint AS total_tokens,
    COALESCE(SUM(ul.actual_cost), 0)::float8 AS actual_cost,
    MAX(ul.created_at) AS last_request_at
  FROM usage_logs ul
  LEFT JOIN users u ON u.id = ul.user_id
  ` + join + `
  ` + where + `
  GROUP BY ul.user_id, u.username, u.email
)
`

	countSQL := baseCTE + `SELECT COUNT(*) FROM stats`
	var total int64
	if err := r.db.QueryRowContext(ctx, countSQL, baseArgs...).Scan(&total); err != nil {
		return nil, err
	}

	querySQL := baseCTE + `
SELECT
  user_id,
  username,
  email,
  request_count,
  input_tokens,
  output_tokens,
  cache_tokens,
  total_tokens,
  actual_cost,
  last_request_at
FROM stats
ORDER BY actual_cost DESC, total_tokens DESC, user_id ASC`

	args := append([]any{}, baseArgs...)
	if filter.IsTopNMode() {
		querySQL += fmt.Sprintf("\nLIMIT $%d", next)
		args = append(args, filter.TopN)
	} else {
		offset := (filter.Page - 1) * filter.PageSize
		querySQL += fmt.Sprintf("\nLIMIT $%d OFFSET $%d", next, next+1)
		args = append(args, filter.PageSize, offset)
	}

	rows, err := r.db.QueryContext(ctx, querySQL, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	items := make([]*service.OpsUserUsageStatsItem, 0, 32)
	for rows.Next() {
		item := &service.OpsUserUsageStatsItem{}
		if err := rows.Scan(
			&item.UserID,
			&item.Username,
			&item.Email,
			&item.RequestCount,
			&item.InputTokens,
			&item.OutputTokens,
			&item.CacheTokens,
			&item.TotalTokens,
			&item.ActualCost,
			&item.LastRequestAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	resp := &service.OpsUserUsageStatsResponse{
		TimeRange: strings.TrimSpace(filter.TimeRange),
		StartTime: dashboardFilter.StartTime,
		EndTime:   dashboardFilter.EndTime,
		Platform:  dashboardFilter.Platform,
		GroupID:   dashboardFilter.GroupID,
		Items:     items,
		Total:     total,
	}
	if filter.IsTopNMode() {
		topN := filter.TopN
		resp.TopN = &topN
	} else {
		resp.Page = filter.Page
		resp.PageSize = filter.PageSize
	}
	return resp, nil
}
