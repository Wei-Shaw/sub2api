package service

import (
	"context"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// GetRealtimeTrafficSummary returns current/peak/average rates plus real traffic
// and user-charge buckets for the provided window.
func (s *OpsService) GetRealtimeTrafficSummary(ctx context.Context, filter *OpsDashboardFilter) (*OpsRealtimeTrafficSummary, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
	}
	if s.opsRepo == nil {
		return nil, infraerrors.ServiceUnavailable("OPS_REPO_UNAVAILABLE", "Ops repository not available")
	}
	if filter == nil {
		return nil, infraerrors.BadRequest("OPS_FILTER_REQUIRED", "filter is required")
	}
	if filter.StartTime.IsZero() || filter.EndTime.IsZero() {
		return nil, infraerrors.BadRequest("OPS_TIME_RANGE_REQUIRED", "start_time/end_time are required")
	}
	if filter.StartTime.After(filter.EndTime) {
		return nil, infraerrors.BadRequest("OPS_TIME_RANGE_INVALID", "start_time must be <= end_time")
	}
	if filter.EndTime.Sub(filter.StartTime) > time.Hour {
		return nil, infraerrors.BadRequest("OPS_TIME_RANGE_TOO_LARGE", "invalid time range: max window is 1 hour")
	}

	// Realtime traffic summary always uses raw logs (minute granularity peaks).
	filter.QueryMode = OpsQueryModeRaw

	return s.opsRepo.GetRealtimeTrafficSummary(ctx, filter)
}
