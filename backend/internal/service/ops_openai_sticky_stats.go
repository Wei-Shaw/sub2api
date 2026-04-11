package service

import (
	"context"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

type OpsOpenAIStickyStatsFilter struct {
	TimeRange string
	StartTime time.Time
	EndTime   time.Time

	Platform string
	GroupID  *int64
}

type OpsOpenAIStickyStatsResponse struct {
	TimeRange string    `json:"time_range"`
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`

	Platform string `json:"platform,omitempty"`
	GroupID  *int64 `json:"group_id,omitempty"`

	EvaluatedRequestCount  int64            `json:"evaluated_request_count"`
	StickyHitCount         int64            `json:"sticky_hit_count"`
	StickyHitRate          float64          `json:"sticky_hit_rate"`
	StickyAccountSwitchCount int64          `json:"sticky_account_switch_count"`
	StickyAccountSwitchRate  float64        `json:"sticky_account_switch_rate"`
	EvalResultCount        map[string]int64 `json:"eval_result_count"`
	SessionSourceCount     map[string]int64 `json:"session_source_count"`
}

func NewOpsOpenAIStickyStatsResponse() *OpsOpenAIStickyStatsResponse {
	return &OpsOpenAIStickyStatsResponse{
		EvalResultCount:    make(map[string]int64),
		SessionSourceCount: make(map[string]int64),
	}
}

func (s *OpsService) GetOpenAIStickyStats(ctx context.Context, filter *OpsOpenAIStickyStatsFilter) (*OpsOpenAIStickyStatsResponse, error) {
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
	if filter.GroupID != nil && *filter.GroupID <= 0 {
		return nil, infraerrors.BadRequest("OPS_GROUP_ID_INVALID", "group_id must be > 0")
	}

	return s.opsRepo.GetOpenAIStickyStats(ctx, filter)
}
