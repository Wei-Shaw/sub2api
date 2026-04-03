package service

import (
	"context"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

type openAIRoutingStatsRepoStub struct {
	OpsRepository
	resp     *OpsOpenAIRoutingStatsResponse
	err      error
	captured *OpsOpenAIRoutingStatsFilter
}

func (s *openAIRoutingStatsRepoStub) GetOpenAIRoutingStats(ctx context.Context, filter *OpsOpenAIRoutingStatsFilter) (*OpsOpenAIRoutingStatsResponse, error) {
	s.captured = filter
	if s.err != nil {
		return nil, s.err
	}
	if s.resp != nil {
		return s.resp, nil
	}
	return &OpsOpenAIRoutingStatsResponse{}, nil
}

func TestOpsServiceGetOpenAIRoutingStats_GroupsByTargetGroup(t *testing.T) {
	now := time.Now().UTC()
	repo := &openAIRoutingStatsRepoStub{
		resp: &OpsOpenAIRoutingStatsResponse{
			RequestCountByGroup: map[string]int64{"active": 10, "exhausted": 4},
			TotalTokensByGroup:  map[string]int64{"active": 1200, "exhausted": 320},
			InputTokensByGroup:  map[string]int64{"active": 700, "exhausted": 200},
			OutputTokensByGroup: map[string]int64{"active": 500, "exhausted": 120},
		},
	}
	svc := &OpsService{opsRepo: repo}

	filter := &OpsOpenAIRoutingStatsFilter{
		TimeRange: "1d",
		StartTime: now.Add(-24 * time.Hour),
		EndTime:   now,
	}

	resp, err := svc.GetOpenAIRoutingStats(context.Background(), filter)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotNil(t, repo.captured)
	require.Equal(t, int64(10), resp.RequestCountByGroup["active"])
	require.Equal(t, int64(4), resp.RequestCountByGroup["exhausted"])
	require.Equal(t, int64(1200), resp.TotalTokensByGroup["active"])
	require.Equal(t, int64(320), resp.TotalTokensByGroup["exhausted"])
	require.Equal(t, int64(700), resp.InputTokensByGroup["active"])
	require.Equal(t, int64(200), resp.InputTokensByGroup["exhausted"])
	require.Equal(t, int64(500), resp.OutputTokensByGroup["active"])
	require.Equal(t, int64(120), resp.OutputTokensByGroup["exhausted"])
}

func TestOpsServiceGetOpenAIRoutingStats_Validation(t *testing.T) {
	now := time.Now().UTC()

	tests := []struct {
		name       string
		filter     *OpsOpenAIRoutingStatsFilter
		wantCode   int
		wantReason string
	}{
		{
			name:       "filter 不能为空",
			filter:     nil,
			wantCode:   400,
			wantReason: "OPS_FILTER_REQUIRED",
		},
		{
			name: "start_time/end_time 必填",
			filter: &OpsOpenAIRoutingStatsFilter{
				EndTime: now,
			},
			wantCode:   400,
			wantReason: "OPS_TIME_RANGE_REQUIRED",
		},
		{
			name: "start_time 不能晚于 end_time",
			filter: &OpsOpenAIRoutingStatsFilter{
				StartTime: now,
				EndTime:   now.Add(-time.Minute),
			},
			wantCode:   400,
			wantReason: "OPS_TIME_RANGE_INVALID",
		},
		{
			name: "group_id 必须大于 0",
			filter: &OpsOpenAIRoutingStatsFilter{
				StartTime: now.Add(-time.Hour),
				EndTime:   now,
				GroupID:   int64Ptr(0),
			},
			wantCode:   400,
			wantReason: "OPS_GROUP_ID_INVALID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &OpsService{opsRepo: &openAIRoutingStatsRepoStub{}}

			_, err := svc.GetOpenAIRoutingStats(context.Background(), tt.filter)
			require.Error(t, err)
			require.Equal(t, tt.wantCode, infraerrors.Code(err))
			require.Equal(t, tt.wantReason, infraerrors.Reason(err))
		})
	}
}

func TestOpsServiceGetOpenAIRoutingStats_RepoUnavailable(t *testing.T) {
	now := time.Now().UTC()
	svc := &OpsService{}

	_, err := svc.GetOpenAIRoutingStats(context.Background(), &OpsOpenAIRoutingStatsFilter{
		TimeRange: "1h",
		StartTime: now.Add(-time.Hour),
		EndTime:   now,
	})
	require.Error(t, err)
	require.Equal(t, 503, infraerrors.Code(err))
	require.Equal(t, "OPS_REPO_UNAVAILABLE", infraerrors.Reason(err))
}
