package service

import (
	"context"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

func (s *OpsService) GetOpenAITokenStats(ctx context.Context, filter *OpsOpenAITokenStatsFilter) (*OpsOpenAITokenStatsResponse, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
	}
	if s.opsRepo == nil {
		return nil, infraerrors.ServiceUnavailable("OPS_REPO_UNAVAILABLE", "Ops repository not available")
	}
	if err := validateAndNormalizeOpsRankedStatsFilter(filter); err != nil {
		return nil, err
	}

	return s.opsRepo.GetOpenAITokenStats(ctx, filter)
}

func validateAndNormalizeOpsRankedStatsFilter(filter *OpsOpenAITokenStatsFilter) error {
	if filter == nil {
		return infraerrors.BadRequest("OPS_FILTER_REQUIRED", "filter is required")
	}
	if filter.StartTime.IsZero() || filter.EndTime.IsZero() {
		return infraerrors.BadRequest("OPS_TIME_RANGE_REQUIRED", "start_time/end_time are required")
	}
	if filter.StartTime.After(filter.EndTime) {
		return infraerrors.BadRequest("OPS_TIME_RANGE_INVALID", "start_time must be <= end_time")
	}

	if filter.GroupID != nil && *filter.GroupID <= 0 {
		return infraerrors.BadRequest("OPS_GROUP_ID_INVALID", "group_id must be > 0")
	}

	// top_n cannot be mixed with page/page_size params.
	if filter.TopN > 0 && (filter.Page > 0 || filter.PageSize > 0) {
		return infraerrors.BadRequest("OPS_PAGINATION_CONFLICT", "top_n cannot be used with page/page_size")
	}

	if filter.TopN > 0 {
		if filter.TopN < 1 || filter.TopN > 100 {
			return infraerrors.BadRequest("OPS_TOPN_INVALID", "top_n must be between 1 and 100")
		}
	} else {
		if filter.Page <= 0 {
			filter.Page = 1
		}
		if filter.PageSize <= 0 {
			filter.PageSize = 20
		}
		if filter.Page < 1 {
			return infraerrors.BadRequest("OPS_PAGE_INVALID", "page must be >= 1")
		}
		if filter.PageSize < 1 || filter.PageSize > 100 {
			return infraerrors.BadRequest("OPS_PAGE_SIZE_INVALID", "page_size must be between 1 and 100")
		}
	}
	return nil
}
