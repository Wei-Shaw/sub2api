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
	where += " AND LOWER(COALESCE(ul.routing_target_group, '')) IN ('active','exhausted','any')"

	querySQL := `
SELECT
  LOWER(COALESCE(NULLIF(ul.routing_target_group, ''), 'any')) AS routing_target_group,
  LOWER(COALESCE(NULLIF(ul.routing_selected_group, ''), ul.routing_target_group)) AS routing_selected_group,
  COUNT(*)::bigint AS request_count,
  COALESCE(SUM(COALESCE(ul.input_tokens, 0) + COALESCE(ul.output_tokens, 0)), 0)::bigint AS total_tokens,
  COALESCE(SUM(COALESCE(ul.input_tokens, 0)), 0)::bigint AS input_tokens,
  COALESCE(SUM(COALESCE(ul.output_tokens, 0)), 0)::bigint AS output_tokens,
  COUNT(*) FILTER (WHERE COALESCE(ul.routing_failover_count, 0) > 0)::bigint AS retried_request_count,
  COALESCE(SUM(COALESCE(ul.routing_failover_count, 0)), 0)::bigint AS retry_count
FROM usage_logs ul
` + join + `
` + where + `
GROUP BY LOWER(COALESCE(NULLIF(ul.routing_target_group, ''), 'any')), LOWER(COALESCE(NULLIF(ul.routing_selected_group, ''), ul.routing_target_group))
ORDER BY LOWER(COALESCE(NULLIF(ul.routing_target_group, ''), 'any')) ASC, LOWER(COALESCE(NULLIF(ul.routing_selected_group, ''), ul.routing_target_group)) ASC`

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
		var selectedGroup string
		var requestCount int64
		var totalTokens int64
		var inputTokens int64
		var outputTokens int64
		var retriedRequestCount int64
		var retryCount int64
		if err := rows.Scan(&targetGroup, &selectedGroup, &requestCount, &totalTokens, &inputTokens, &outputTokens, &retriedRequestCount, &retryCount); err != nil {
			return nil, err
		}
		targetGroup = strings.TrimSpace(strings.ToLower(targetGroup))
		selectedGroup = strings.TrimSpace(strings.ToLower(selectedGroup))
		resp.Breakdown = append(resp.Breakdown, service.OpsOpenAIRoutingStatsBreakdown{
			TargetGroup:         targetGroup,
			SelectedGroup:       selectedGroup,
			RequestCount:        requestCount,
			TotalTokens:         totalTokens,
			InputTokens:         inputTokens,
			OutputTokens:        outputTokens,
			RetriedRequestCount: retriedRequestCount,
			RetryCount:          retryCount,
		})
		if _, ok := resp.RequestCountByGroup[selectedGroup]; !ok {
			continue
		}
		resp.RequestCountByGroup[selectedGroup] += requestCount
		resp.TotalTokensByGroup[selectedGroup] += totalTokens
		resp.InputTokensByGroup[selectedGroup] += inputTokens
		resp.OutputTokensByGroup[selectedGroup] += outputTokens
		resp.RetriedRequestCountByGroup[selectedGroup] += retriedRequestCount
		resp.RetryCountByGroup[selectedGroup] += retryCount
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return resp, nil
}
