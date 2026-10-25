package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (r *opsRepository) GetOpenAITokenStats(ctx context.Context, filter *service.OpsOpenAITokenStatsFilter) (*service.OpsOpenAITokenStatsResponse, error) {
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
	if IsMySQL() {
		return r.getOpenAITokenStatsMySQL(ctx, filter)
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
    ul.model AS model,
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
  GROUP BY ul.model
)
`

	countSQL := baseCTE + `SELECT COUNT(*) FROM stats`
	var total int64
	if err := r.db.QueryRowContext(ctx, countSQL, baseArgs...).Scan(&total); err != nil {
		return nil, err
	}

	querySQL := baseCTE + `
SELECT
  model,
  request_count,
  avg_tokens_per_sec,
  avg_first_token_ms,
  total_output_tokens,
  avg_duration_ms,
  requests_with_first_token
FROM stats
ORDER BY request_count DESC, model ASC`

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

	items := make([]*service.OpsOpenAITokenStatsItem, 0, 32)
	for rows.Next() {
		item := &service.OpsOpenAITokenStatsItem{}
		var avgTPS sql.NullFloat64
		var avgFirstToken sql.NullFloat64
		if err := rows.Scan(
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

	resp := &service.OpsOpenAITokenStatsResponse{
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

// getOpenAITokenStatsMySQL 是 GetOpenAITokenStats 的 MySQL 实现：
// WHERE 构造复用 buildUsageWhereMySQL（$n→?、`groups` 反引号）；
// ::numeric/::float8 强转去除（AVG 对整型/DECIMAL 输入返回 DECIMAL，ROUND 精确
// 四舍五入，语义对齐 PG numeric；DECIMAL 扫描 float64 由 database/sql 自动转换）；
// ::bigint → CAST(... AS SIGNED)（SUM/ROUND(AVG) 在 MySQL 返回 DECIMAL）；
// LIMIT $n [OFFSET $n] → LIMIT ? [OFFSET ?]；CTE/分页/排序语义与 PG 版一致。
func (r *opsRepository) getOpenAITokenStatsMySQL(ctx context.Context, filter *service.OpsOpenAITokenStatsFilter) (*service.OpsOpenAITokenStatsResponse, error) {
	dashboardFilter := &service.OpsDashboardFilter{
		StartTime: filter.StartTime.UTC(),
		EndTime:   filter.EndTime.UTC(),
		Platform:  strings.TrimSpace(strings.ToLower(filter.Platform)),
		GroupID:   filter.GroupID,
	}

	join, where, baseArgs, _ := buildUsageWhereMySQL(dashboardFilter, dashboardFilter.StartTime, dashboardFilter.EndTime, 1)

	baseCTE := `
WITH stats AS (
  SELECT
    ul.model AS model,
    COUNT(*) AS request_count,
    ROUND(
      AVG(
        CASE
          WHEN ul.duration_ms > 0 AND ul.output_tokens > 0
          THEN ul.output_tokens * 1000.0 / ul.duration_ms
        END
      ),
      2
    ) AS avg_tokens_per_sec,
    ROUND(AVG(ul.first_token_ms), 2) AS avg_first_token_ms,
    CAST(COALESCE(SUM(ul.output_tokens), 0) AS SIGNED) AS total_output_tokens,
    CAST(COALESCE(ROUND(AVG(ul.duration_ms), 0), 0) AS SIGNED) AS avg_duration_ms,
    COUNT(CASE WHEN ul.first_token_ms IS NOT NULL THEN 1 END) AS requests_with_first_token
  FROM usage_logs ul
  ` + join + `
  ` + where + `
  GROUP BY ul.model
)
`

	countSQL := baseCTE + `SELECT COUNT(*) FROM stats`
	var total int64
	if err := r.db.QueryRowContext(ctx, countSQL, baseArgs...).Scan(&total); err != nil {
		return nil, err
	}

	querySQL := baseCTE + `
SELECT
  model,
  request_count,
  avg_tokens_per_sec,
  avg_first_token_ms,
  total_output_tokens,
  avg_duration_ms,
  requests_with_first_token
FROM stats
ORDER BY request_count DESC, model ASC`

	args := make([]any, 0, len(baseArgs)+2)
	args = append(args, baseArgs...)

	if filter.IsTopNMode() {
		querySQL += "\nLIMIT ?"
		args = append(args, filter.TopN)
	} else {
		offset := (filter.Page - 1) * filter.PageSize
		querySQL += "\nLIMIT ? OFFSET ?"
		args = append(args, filter.PageSize, offset)
	}

	rows, err := r.db.QueryContext(ctx, querySQL, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	items := make([]*service.OpsOpenAITokenStatsItem, 0, 32)
	for rows.Next() {
		item := &service.OpsOpenAITokenStatsItem{}
		var avgTPS sql.NullFloat64
		var avgFirstToken sql.NullFloat64
		if err := rows.Scan(
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

	resp := &service.OpsOpenAITokenStatsResponse{
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
