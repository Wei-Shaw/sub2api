package service

import (
	"context"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

type OpsOpenAIRoutingStatsFilter struct {
	TimeRange string
	StartTime time.Time
	EndTime   time.Time

	Platform string
	GroupID  *int64
}

type OpsOpenAIRoutingStatsResponse struct {
	TimeRange string    `json:"time_range"`
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`

	Platform string `json:"platform,omitempty"`
	GroupID  *int64 `json:"group_id,omitempty"`

	RequestCountByGroup        map[string]int64                 `json:"request_count_by_group"`
	TotalTokensByGroup         map[string]int64                 `json:"total_tokens_by_group"`
	InputTokensByGroup         map[string]int64                 `json:"input_tokens_by_group"`
	OutputTokensByGroup        map[string]int64                 `json:"output_tokens_by_group"`
	RetriedRequestCountByGroup map[string]int64                 `json:"retried_request_count_by_group"`
	RetryCountByGroup          map[string]int64                 `json:"retry_count_by_group"`
	Breakdown                  []OpsOpenAIRoutingStatsBreakdown `json:"breakdown"`
}

type OpsOpenAIRoutingStatsBreakdown struct {
	TargetGroup         string `json:"target_group"`
	SelectedGroup       string `json:"selected_group"`
	RequestCount        int64  `json:"request_count"`
	TotalTokens         int64  `json:"total_tokens"`
	InputTokens         int64  `json:"input_tokens"`
	OutputTokens        int64  `json:"output_tokens"`
	RetriedRequestCount int64  `json:"retried_request_count"`
	RetryCount          int64  `json:"retry_count"`
}

func GetOpsOpenAIRoutingStatsGroups() []string {
	return []string{string(TargetGroupActive), string(TargetGroupExhausted), "reserve"}
}

func NewOpsOpenAIRoutingStatsResponse() *OpsOpenAIRoutingStatsResponse {
	groups := GetOpsOpenAIRoutingStatsGroups()
	resp := &OpsOpenAIRoutingStatsResponse{
		RequestCountByGroup:        make(map[string]int64, len(groups)),
		TotalTokensByGroup:         make(map[string]int64, len(groups)),
		InputTokensByGroup:         make(map[string]int64, len(groups)),
		OutputTokensByGroup:        make(map[string]int64, len(groups)),
		RetriedRequestCountByGroup: make(map[string]int64, len(groups)),
		RetryCountByGroup:          make(map[string]int64, len(groups)),
		Breakdown:                  []OpsOpenAIRoutingStatsBreakdown{},
	}
	for _, group := range groups {
		resp.RequestCountByGroup[group] = 0
		resp.TotalTokensByGroup[group] = 0
		resp.InputTokensByGroup[group] = 0
		resp.OutputTokensByGroup[group] = 0
		resp.RetriedRequestCountByGroup[group] = 0
		resp.RetryCountByGroup[group] = 0
	}
	return resp
}

func (s *OpsService) GetOpenAIRoutingStats(ctx context.Context, filter *OpsOpenAIRoutingStatsFilter) (*OpsOpenAIRoutingStatsResponse, error) {
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

	return s.opsRepo.GetOpenAIRoutingStats(ctx, filter)
}
