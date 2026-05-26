package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (r *opsRepository) GetRealtimeTrafficSummary(ctx context.Context, filter *service.OpsDashboardFilter) (*service.OpsRealtimeTrafficSummary, error) {
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
	if start.After(end) {
		return nil, fmt.Errorf("start_time must be <= end_time")
	}

	window := end.Sub(start)
	if window <= 0 {
		return nil, fmt.Errorf("invalid time window")
	}
	if window > time.Hour {
		return nil, fmt.Errorf("window too large")
	}

	usageJoin, usageWhere, usageArgs, next := buildUsageWhere(filter, start, end, 1)
	errorWhere, errorArgs, _ := buildErrorWhere(filter, start, end, next)

	q := `
WITH usage_buckets AS (
  SELECT
    date_trunc('minute', ul.created_at) AS bucket,
    COALESCE(COUNT(*), 0) AS success_count,
    COALESCE(SUM(input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens), 0) AS token_sum
  FROM usage_logs ul
  ` + usageJoin + `
  ` + usageWhere + `
  GROUP BY 1
),
error_buckets AS (
  SELECT
    date_trunc('minute', created_at) AS bucket,
    COALESCE(COUNT(*), 0) AS error_count
  FROM ops_error_logs
  ` + errorWhere + `
    AND COALESCE(status_code, 0) >= 400
  GROUP BY 1
),
combined AS (
  SELECT
    COALESCE(u.bucket, e.bucket) AS bucket,
    COALESCE(u.success_count, 0) AS success_count,
    COALESCE(u.token_sum, 0) AS token_sum,
    COALESCE(e.error_count, 0) AS error_count,
    COALESCE(u.success_count, 0) + COALESCE(e.error_count, 0) AS request_total
  FROM usage_buckets u
  FULL OUTER JOIN error_buckets e ON u.bucket = e.bucket
)
SELECT
  COALESCE(SUM(success_count), 0) AS success_total,
  COALESCE(SUM(error_count), 0) AS error_total,
  COALESCE(SUM(token_sum), 0) AS token_total,
  COALESCE(MAX(request_total), 0) AS peak_requests_per_min,
  COALESCE(MAX(token_sum), 0) AS peak_tokens_per_min
FROM combined`

	args := append(usageArgs, errorArgs...)
	var successCount int64
	var errorTotal int64
	var tokenConsumed int64
	var peakRequestsPerMin int64
	var peakTokensPerMin int64
	if err := r.db.QueryRowContext(ctx, q, args...).Scan(
		&successCount,
		&errorTotal,
		&tokenConsumed,
		&peakRequestsPerMin,
		&peakTokensPerMin,
	); err != nil {
		return nil, err
	}

	windowSeconds := window.Seconds()
	if windowSeconds <= 0 {
		windowSeconds = 1
	}

	requestCountTotal := successCount + errorTotal
	qpsAvg := roundTo1DP(float64(requestCountTotal) / windowSeconds)
	tpsAvg := roundTo1DP(float64(tokenConsumed) / windowSeconds)

	// Keep "current" consistent with the dashboard overview semantics: last 1 minute.
	// This remains "within the selected window" since end=start+window.
	qpsCurrent, tpsCurrent, err := r.queryCurrentRates(ctx, filter, end)
	if err != nil {
		return nil, err
	}

	qpsPeak := roundTo1DP(float64(peakRequestsPerMin) / 60.0)
	tpsPeak := roundTo1DP(float64(peakTokensPerMin) / 60.0)

	return &service.OpsRealtimeTrafficSummary{
		StartTime: start,
		EndTime:   end,
		Platform:  strings.TrimSpace(filter.Platform),
		GroupID:   filter.GroupID,
		QPS: service.OpsRateSummary{
			Current: qpsCurrent,
			Peak:    qpsPeak,
			Avg:     qpsAvg,
		},
		TPS: service.OpsRateSummary{
			Current: tpsCurrent,
			Peak:    tpsPeak,
			Avg:     tpsAvg,
		},
	}, nil
}
