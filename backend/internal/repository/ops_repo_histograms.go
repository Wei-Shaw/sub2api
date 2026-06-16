package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (r *opsRepository) GetLatencyHistogram(ctx context.Context, filter *service.OpsDashboardFilter) (*service.OpsLatencyHistogramResponse, error) {
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

	join, where, args, _ := buildUsageWhere(filter, start, end, 1)
	rangeExpr := latencyHistogramRangeCaseExpr("ul.duration_ms")
	orderExpr := latencyHistogramRangeOrderCaseExpr("ul.duration_ms")

	q := `
SELECT
  ` + rangeExpr + ` AS range,
  COALESCE(COUNT(*), 0) AS count,
  ` + orderExpr + ` AS ord
FROM usage_logs ul
` + join + `
` + where + `
AND ul.duration_ms IS NOT NULL
GROUP BY 1, 3
ORDER BY 3 ASC`

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	counts := make(map[string]int64, len(latencyHistogramOrderedRanges))
	var total int64
	for rows.Next() {
		var label string
		var count int64
		var _ord int
		if err := rows.Scan(&label, &count, &_ord); err != nil {
			return nil, err
		}
		counts[label] = count
		total += count
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	buckets := make([]*service.OpsLatencyHistogramBucket, 0, len(latencyHistogramOrderedRanges))
	for _, label := range latencyHistogramOrderedRanges {
		buckets = append(buckets, &service.OpsLatencyHistogramBucket{
			Range: label,
			Count: counts[label],
		})
	}

	return &service.OpsLatencyHistogramResponse{
		StartTime:     start,
		EndTime:       end,
		Platform:      strings.TrimSpace(filter.Platform),
		GroupID:       filter.GroupID,
		TotalRequests: total,
		Buckets:       buckets,
	}, nil
}
