package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (r *opsRepository) GetOpenAIStickyStats(ctx context.Context, filter *service.OpsOpenAIStickyStatsFilter) (*service.OpsOpenAIStickyStatsResponse, error) {
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

	conditions := make([]string, 0, 4)
	args := make([]any, 0, 4)
	args = append(args, filter.StartTime.UTC(), filter.EndTime.UTC())

	if platform := strings.TrimSpace(strings.ToLower(filter.Platform)); platform != "" {
		conditions = append(conditions, fmt.Sprintf("platform = $%d", len(args)+1))
		args = append(args, platform)
	}
	if filter.GroupID != nil && *filter.GroupID > 0 {
		conditions = append(conditions, fmt.Sprintf("group_id = $%d", len(args)+1))
		args = append(args, *filter.GroupID)
	}

	where := ""
	if len(conditions) > 0 {
		where = "WHERE " + strings.Join(conditions, " AND ")
	}

	querySQL := `
WITH combined AS (
  SELECT
    COALESCE(NULLIF(g.platform, ''), NULLIF(a.platform, ''), '') AS platform,
    ul.group_id AS group_id,
    COALESCE(NULLIF(ul.sticky_session_source, ''), '') AS sticky_session_source,
    COALESCE(NULLIF(ul.sticky_eval_result, ''), '') AS sticky_eval_result,
    COALESCE(ul.sticky_selected_account_changed, false) AS sticky_selected_account_changed
  FROM usage_logs ul
  LEFT JOIN groups g ON g.id = ul.group_id
  LEFT JOIN accounts a ON a.id = ul.account_id
  WHERE ul.created_at >= $1 AND ul.created_at < $2

  UNION ALL

  SELECT
    COALESCE(NULLIF(o.platform, ''), NULLIF(g.platform, ''), NULLIF(a.platform, ''), '') AS platform,
    o.group_id AS group_id,
    COALESCE(NULLIF(o.sticky_session_source, ''), '') AS sticky_session_source,
    COALESCE(NULLIF(o.sticky_eval_result, ''), '') AS sticky_eval_result,
    COALESCE(o.sticky_selected_account_changed, false) AS sticky_selected_account_changed
  FROM ops_error_logs o
  LEFT JOIN groups g ON g.id = o.group_id
  LEFT JOIN accounts a ON a.id = o.account_id
  WHERE o.created_at >= $1 AND o.created_at < $2
    AND COALESCE(o.status_code, 0) >= 400
)
SELECT
  sticky_session_source,
  sticky_eval_result,
  sticky_selected_account_changed,
  COUNT(*)::bigint AS request_count
FROM combined
` + where + `
GROUP BY sticky_session_source, sticky_eval_result, sticky_selected_account_changed
ORDER BY sticky_session_source, sticky_eval_result`

	rows, err := r.db.QueryContext(ctx, querySQL, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	resp := service.NewOpsOpenAIStickyStatsResponse()
	resp.TimeRange = strings.TrimSpace(filter.TimeRange)
	resp.StartTime = filter.StartTime.UTC()
	resp.EndTime = filter.EndTime.UTC()
	resp.Platform = strings.TrimSpace(strings.ToLower(filter.Platform))
	resp.GroupID = filter.GroupID

	for rows.Next() {
		var sessionSource string
		var evalResult string
		var selectedChanged bool
		var requestCount int64
		if err := rows.Scan(&sessionSource, &evalResult, &selectedChanged, &requestCount); err != nil {
			return nil, err
		}
		sessionSource = strings.TrimSpace(sessionSource)
		evalResult = strings.TrimSpace(evalResult)
		if sessionSource != "" {
			resp.SessionSourceCount[sessionSource] += requestCount
		}
		if evalResult != "" {
			resp.EvalResultCount[evalResult] += requestCount
		}
		if isOpenAIStickyEvaluatedResult(evalResult) {
			resp.EvaluatedRequestCount += requestCount
			if evalResult == "hit" {
				resp.StickyHitCount += requestCount
			}
			if selectedChanged {
				resp.StickyAccountSwitchCount += requestCount
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if resp.EvaluatedRequestCount > 0 {
		resp.StickyHitRate = float64(resp.StickyHitCount) / float64(resp.EvaluatedRequestCount)
		resp.StickyAccountSwitchRate = float64(resp.StickyAccountSwitchCount) / float64(resp.EvaluatedRequestCount)
	}

	return resp, nil
}

func isOpenAIStickyEvaluatedResult(evalResult string) bool {
	switch strings.TrimSpace(evalResult) {
	case "", "bypassed_previous_response_id", "no_session_signal":
		return false
	default:
		return true
	}
}
