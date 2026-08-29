package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (r *opsRepository) GetTokenStats(ctx context.Context, filter *service.OpsTokenStatsFilter) (*service.OpsTokenStatsResponse, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	if filter == nil {
		return nil, fmt.Errorf("nil filter")
	}
	if filter.StartTime.IsZero() || filter.EndTime.IsZero() {
		return nil, fmt.Errorf("start_time/end_time required")
	}
	// 允许 start_time == end_time（结果为空），与 service 层校验口径保持一致。
	if filter.StartTime.After(filter.EndTime) {
		return nil, fmt.Errorf("start_time must be <= end_time")
	}

	dashboardFilter := &service.OpsDashboardFilter{
		StartTime: filter.StartTime.UTC(),
		EndTime:   filter.EndTime.UTC(),
		GroupID:   filter.GroupID,
	}

	join, where, baseArgs, next := buildUsageWhere(dashboardFilter, dashboardFilter.StartTime, dashboardFilter.EndTime, 1)
	join += " LEFT JOIN groups g ON g.id = ul.group_id LEFT JOIN accounts a ON a.id = ul.account_id"
	platformExpr := "LOWER(TRIM(" + usageLogEffectivePlatformExpr + "))"
	modelExpr := resolveModelDimensionExpressionWithAlias(usagestats.ModelSourceRequested, "ul")
	platform := strings.TrimSpace(strings.ToLower(filter.Platform))
	if platform != "" {
		baseArgs = append(baseArgs, platform)
		where += fmt.Sprintf(" AND %s = $%d", platformExpr, next)
		next++
	}
	where += " AND NULLIF(TRIM(COALESCE(" + usageLogEffectivePlatformExpr + ", '')), '') IS NOT NULL"
	where += " AND (ul.input_tokens + ul.output_tokens + ul.cache_creation_tokens + ul.cache_read_tokens) > 0"

	baseCTE := `
	WITH stats AS (
	  SELECT
	    ` + platformExpr + ` AS platform,
	    ` + modelExpr + ` AS model,
    COUNT(*)::bigint AS request_count,
    ROUND(
      AVG(
        CASE
          WHEN ul.duration_ms > 0 AND ul.output_tokens > 0
          THEN ul.output_tokens * 1000.0 / ul.duration_ms
        END
      )::numeric,
      2
    )::float8 AS avg_tokens_per_sec,
    ROUND(AVG(ul.first_token_ms)::numeric, 2)::float8 AS avg_first_token_ms,
    COALESCE(SUM(ul.output_tokens), 0)::bigint AS total_output_tokens,
    COALESCE(ROUND(AVG(ul.duration_ms)::numeric, 0), 0)::bigint AS avg_duration_ms,
    COUNT(CASE WHEN ul.first_token_ms IS NOT NULL THEN 1 END)::bigint AS requests_with_first_token
  FROM usage_logs ul
  ` + join + `
  ` + where + `
	  GROUP BY ` + platformExpr + `, ` + modelExpr + `
)
`

	countSQL := baseCTE + `SELECT COUNT(*) FROM stats`
	var total int64
	if err := r.db.QueryRowContext(ctx, countSQL, baseArgs...).Scan(&total); err != nil {
		return nil, err
	}

	querySQL := baseCTE + `
SELECT
	  platform,
	  model,
  request_count,
  avg_tokens_per_sec,
  avg_first_token_ms,
  total_output_tokens,
  avg_duration_ms,
  requests_with_first_token
FROM stats
ORDER BY request_count DESC, platform ASC, model ASC`

	args := make([]any, 0, len(baseArgs)+2)
	args = append(args, baseArgs...)

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

	items := make([]*service.OpsTokenStatsItem, 0, 32)
	for rows.Next() {
		item := &service.OpsTokenStatsItem{}
		var avgTPS sql.NullFloat64
		var avgFirstToken sql.NullFloat64
		if err := rows.Scan(
			&item.Platform,
			&item.Model,
			&item.RequestCount,
			&avgTPS,
			&avgFirstToken,
			&item.TotalOutputTokens,
			&item.AvgDurationMs,
			&item.RequestsWithFirstToken,
		); err != nil {
			return nil, err
		}
		if avgTPS.Valid {
			v := avgTPS.Float64
			item.AvgTokensPerSec = &v
		}
		if avgFirstToken.Valid {
			v := avgFirstToken.Float64
			item.AvgFirstTokenMs = &v
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	resp := &service.OpsTokenStatsResponse{
		TimeRange: strings.TrimSpace(filter.TimeRange),
		StartTime: dashboardFilter.StartTime,
		EndTime:   dashboardFilter.EndTime,
		Platform:  platform,
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
