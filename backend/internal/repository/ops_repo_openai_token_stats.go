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
REDACTED
	if filter == nil {
		return nil, fmt.Errorf("nil filter")
REDACTED
	if filter.StartTime.IsZero() || filter.EndTime.IsZero() {
		return nil, fmt.Errorf("start_time/end_time required")
REDACTED
	// 允许 start_time == end_time（结果为空），与 service 层校验口径保持一致。
	if filter.StartTime.After(filter.EndTime) {
		return nil, fmt.Errorf("start_time must be <= end_time")
REDACTED

	dashboardFilter := &service.OpsDashboardFilter{
		StartTime: filter.StartTime.UTC(),
		EndTime:   filter.EndTime.UTC(),
		Platform:  strings.TrimSpace(strings.ToLower(filter.Platform)),
		GroupID:   filter.GroupID,
REDACTED

	join, where, baseArgs, next := buildUsageWhere(dashboardFilter, dashboardFilter.StartTime, dashboardFilter.EndTime, 1)
	where += " AND ul.model LIKE 'gpt%'"

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
REDACTED

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
REDACTED else {
		offset := (filter.Page - 1) * filter.PageSize
		querySQL += fmt.Sprintf("\nLIMIT $%d OFFSET $%d", next, next+1)
		args = append(args, filter.PageSize, offset)
REDACTED

	rows, err := r.db.QueryContext(ctx, querySQL, args...)
	if err != nil {
		return nil, err
REDACTED
	defer func() { _ = rows.Close() REDACTED()

	items := make([]*service.OpsOpenAITokenStatsItem, 0, 32)
	for rows.Next() {
		item := &service.OpsOpenAITokenStatsItem{REDACTED
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
	REDACTED
		if avgTPS.Valid {
			v := avgTPS.Float64
			item.AvgTokensPerSec = &v
	REDACTED
		if avgFirstToken.Valid {
			v := avgFirstToken.Float64
			item.AvgFirstTokenMs = &v
	REDACTED
		items = append(items, item)
REDACTED
	if err := rows.Err(); err != nil {
		return nil, err
REDACTED

	resp := &service.OpsOpenAITokenStatsResponse{
		TimeRange: strings.TrimSpace(filter.TimeRange),
		StartTime: dashboardFilter.StartTime,
		EndTime:   dashboardFilter.EndTime,
		Platform:  dashboardFilter.Platform,
		GroupID:   dashboardFilter.GroupID,
		Items:     items,
		Total:     total,
REDACTED
	if filter.IsTopNMode() {
		topN := filter.TopN
		resp.TopN = &topN
REDACTED else {
		resp.Page = filter.Page
		resp.PageSize = filter.PageSize
REDACTED
	return resp, nil
REDACTED
