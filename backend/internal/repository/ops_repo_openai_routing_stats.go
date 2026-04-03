package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (r *opsRepository) GetOpenAIRoutingStats(ctx context.Context, filter *service.OpsOpenAIRoutingStatsFilter) (*service.OpsOpenAIRoutingStatsResponse, error) {
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

	join, where, args, _ := buildUsageWhere(dashboardFilter, dashboardFilter.StartTime, dashboardFilter.EndTime, 1)
	where += " AND LOWER(COALESCE(ul.routing_target_group, '')) IN ('active','exhausted')"

	querySQL := `
SELECT
  LOWER(ul.routing_target_group) AS routing_target_group,
  COUNT(*)::bigint AS request_count,
  COALESCE(SUM(COALESCE(ul.input_tokens, 0) + COALESCE(ul.output_tokens, 0)), 0)::bigint AS total_tokens,
  COALESCE(SUM(COALESCE(ul.input_tokens, 0)), 0)::bigint AS input_tokens,
  COALESCE(SUM(COALESCE(ul.output_tokens, 0)), 0)::bigint AS output_tokens
FROM usage_logs ul
` + join + `
` + where + `
GROUP BY LOWER(ul.routing_target_group)
ORDER BY LOWER(ul.routing_target_group) ASC`

	rows, err := r.db.QueryContext(ctx, querySQL, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	resp := service.NewOpsOpenAIRoutingStatsResponse()
	resp.TimeRange = strings.TrimSpace(filter.TimeRange)
	resp.StartTime = dashboardFilter.StartTime
	resp.EndTime = dashboardFilter.EndTime
	resp.Platform = dashboardFilter.Platform
	resp.GroupID = dashboardFilter.GroupID

	for rows.Next() {
		var targetGroup string
		var requestCount int64
		var totalTokens int64
		var inputTokens int64
		var outputTokens int64
		if err := rows.Scan(&targetGroup, &requestCount, &totalTokens, &inputTokens, &outputTokens); err != nil {
			return nil, err
		}
		targetGroup = strings.TrimSpace(strings.ToLower(targetGroup))
		if _, ok := resp.RequestCountByGroup[targetGroup]; !ok {
			continue
		}
		resp.RequestCountByGroup[targetGroup] = requestCount
		resp.TotalTokensByGroup[targetGroup] = totalTokens
		resp.InputTokensByGroup[targetGroup] = inputTokens
		resp.OutputTokensByGroup[targetGroup] = outputTokens
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return resp, nil
}
