package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (r *opsRepository) GetWindowStats(ctx context.Context, filter *service.OpsDashboardFilter) (*service.OpsWindowStats, error) {
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
	// Bound excessively large windows to prevent accidental heavy queries.
	if end.Sub(start) > 24*time.Hour {
		return nil, fmt.Errorf("window too large")
	}

	successCount, tokenConsumed, err := r.queryUsageCounts(ctx, filter, start, end)
	if err != nil {
		return nil, err
	}

	errorTotal, _, _, _, _, _, err := r.queryErrorCounts(ctx, filter, start, end)
	if err != nil {
		return nil, err
	}

	return &service.OpsWindowStats{
		StartTime: start,
		EndTime:   end,

		SuccessCount:    successCount,
		ErrorCountTotal: errorTotal,
		TokenConsumed:   tokenConsumed,
	}, nil
}
