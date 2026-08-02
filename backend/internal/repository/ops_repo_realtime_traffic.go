package repository

import (
	"context"
	"fmt"
	"math"
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

	bucketSeconds := realtimeBucketSeconds(window)
	usageJoin, usageWhere, usageArgs, next := buildUsageWhere(filter, start, end, 1)
	errorStartIndex := next
	errorWhere, errorArgs, _ := buildErrorWhere(filter, start, end, next)

	q := fmt.Sprintf(`
WITH buckets AS (
  SELECT generate_series(
    $1::timestamptz,
    $2::timestamptz - interval '1 microsecond',
    interval '%d seconds'
  ) AS bucket
),
usage_buckets AS (
  SELECT
    date_bin(interval '%d seconds', ul.created_at, $1::timestamptz) AS bucket,
    COALESCE(COUNT(*), 0) AS success_count,
    COALESCE(SUM(input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens), 0) AS token_sum,
    COALESCE(SUM(actual_cost), 0) AS actual_cost
  FROM usage_logs ul
  `+usageJoin+`
  `+usageWhere+`
  GROUP BY 1
),
error_buckets AS (
  SELECT
    date_bin(interval '%d seconds', created_at, $%d::timestamptz) AS bucket,
    COALESCE(COUNT(*), 0) AS error_count
  FROM ops_error_logs
  `+errorWhere+`
    AND COALESCE(status_code, 0) >= 400
  GROUP BY 1
)
SELECT
  b.bucket,
  COALESCE(u.success_count, 0) + COALESCE(e.error_count, 0) AS request_total,
  COALESCE(u.token_sum, 0) AS token_total,
  COALESCE(u.actual_cost, 0) AS actual_cost
FROM buckets b
LEFT JOIN usage_buckets u ON u.bucket = b.bucket
LEFT JOIN error_buckets e ON e.bucket = b.bucket
ORDER BY b.bucket`, bucketSeconds, bucketSeconds, bucketSeconds, errorStartIndex)

	args := append(usageArgs, errorArgs...)
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	points := make([]*service.OpsRealtimeTrafficPoint, 0, int(math.Ceil(window.Seconds()/float64(bucketSeconds))))
	var requestCountTotal int64
	var tokenConsumed int64
	var actualCostTotal float64
	var qpsPeak float64
	var tpsPeak float64
	var currentRequests int64
	var currentTokens int64
	currentStart := end.Add(-time.Minute)
	if currentStart.Before(start) {
		currentStart = start
	}
	for rows.Next() {
		var bucket time.Time
		var requestCount int64
		var tokenCount int64
		var actualCost float64
		if err := rows.Scan(&bucket, &requestCount, &tokenCount, &actualCost); err != nil {
			return nil, err
		}
		rpm := float64(requestCount) * 60 / float64(bucketSeconds)
		tps := float64(tokenCount) / float64(bucketSeconds)
		points = append(points, &service.OpsRealtimeTrafficPoint{
			Time:            bucket.UTC(),
			RPM:             roundTo1DP(rpm),
			TokensPerSecond: roundTo1DP(tps),
			ActualCost:      actualCost,
		})
		requestCountTotal += requestCount
		tokenConsumed += tokenCount
		actualCostTotal += actualCost
		qpsPeak = math.Max(qpsPeak, rpm/60)
		tpsPeak = math.Max(tpsPeak, tps)
		if !bucket.Before(currentStart) {
			currentRequests += requestCount
			currentTokens += tokenCount
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	windowSeconds := window.Seconds()
	if windowSeconds <= 0 {
		windowSeconds = 1
	}

	qpsAvg := roundTo1DP(float64(requestCountTotal) / windowSeconds)
	tpsAvg := roundTo1DP(float64(tokenConsumed) / windowSeconds)
	currentSeconds := end.Sub(currentStart).Seconds()
	if currentSeconds <= 0 {
		currentSeconds = 1
	}
	qpsCurrent := roundTo1DP(float64(currentRequests) / currentSeconds)
	tpsCurrent := roundTo1DP(float64(currentTokens) / currentSeconds)

	return &service.OpsRealtimeTrafficSummary{
		StartTime: start,
		EndTime:   end,
		Platform:  strings.TrimSpace(filter.Platform),
		GroupID:   filter.GroupID,
		QPS: service.OpsRateSummary{
			Current: qpsCurrent,
			Peak:    roundTo1DP(qpsPeak),
			Avg:     qpsAvg,
		},
		TPS: service.OpsRateSummary{
			Current: tpsCurrent,
			Peak:    roundTo1DP(tpsPeak),
			Avg:     tpsAvg,
		},
		BucketSeconds:   bucketSeconds,
		ActualCostTotal: actualCostTotal,
		Points:          points,
	}, nil
}

func realtimeBucketSeconds(window time.Duration) int {
	switch {
	case window <= time.Minute:
		return 5
	case window <= 5*time.Minute:
		return 15
	default:
		return 60
	}
}
