package service

import (
	"context"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// GetWindowStats returns lightweight request/token counts for the provided window.
// It is intended for realtime sampling (e.g. WebSocket QPS push) without computing percentiles/peaks.
func (s *OpsService) GetWindowStats(ctx context.Context, startTime, endTime time.Time) (*OpsWindowStats, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
	}
	if s.opsRepo == nil {
		return nil, infraerrors.ServiceUnavailable("OPS_REPO_UNAVAILABLE", "Ops repository not available")
	}
	filter := &OpsDashboardFilter{
		StartTime: startTime,
		EndTime:   endTime,
	}
	return s.opsRepo.GetWindowStats(ctx, filter)
}
