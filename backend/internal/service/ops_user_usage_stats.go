package service

import (
	"context"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

func (s *OpsService) GetUserUsageStats(ctx context.Context, filter *OpsUserUsageStatsFilter) (*OpsUserUsageStatsResponse, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
	}
	if s.opsRepo == nil {
		return nil, infraerrors.ServiceUnavailable("OPS_REPO_UNAVAILABLE", "Ops repository not available")
	}
	if err := validateAndNormalizeOpsRankedStatsFilter(filter); err != nil {
		return nil, err
	}
	return s.opsRepo.GetUserUsageStats(ctx, filter)
}
