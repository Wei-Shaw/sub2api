package service

import (
	"context"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

type openAITokenStatsRepoStub struct {
	OpsRepository
	resp     *OpsOpenAITokenStatsResponse
	err      error
	captured *OpsOpenAITokenStatsFilter
}

func (s *openAITokenStatsRepoStub) GetOpenAITokenStats(ctx context.Context, filter *OpsOpenAITokenStatsFilter) (*OpsOpenAITokenStatsResponse, error) {
	s.captured = filter
	if s.err != nil {
		return nil, s.err
	}
	if s.resp != nil {
		return s.resp, nil
	}
	return &OpsOpenAITokenStatsResponse{}, nil
}

func TestOpsServiceGetOpenAITokenStats_Validation(t *testing.T) {
	now := time.Now().UTC()

	tests := []struct {
		name       string
		filter     *OpsOpenAITokenStatsFilter
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
			filter: &OpsOpenAITokenStatsFilter{
				StartTime: time.Time{},
				EndTime:   now,
			},
			wantCode:   400,
			wantReason: "OPS_TIME_RANGE_REQUIRED",
		},
		{
			name: "start_time 不能晚于 end_time",
			filter: &OpsOpenAITokenStatsFilter{
				StartTime: now,
				EndTime:   now.Add(-1 * time.Minute),
			},
			wantCode:   400,
			wantReason: "OPS_TIME_RANGE_INVALID",
		},
		{
			name: "group_id 必须大于 0",
			filter: &OpsOpenAITokenStatsFilter{
				StartTime: now.Add(-time.Hour),
				EndTime:   now,
				GroupID:   int64Ptr(0),
			},
			wantCode:   400,
			wantReason: "OPS_GROUP_ID_INVALID",
		},
		{
			name: "top_n 与分页参数互斥",
			filter: &OpsOpenAITokenStatsFilter{
				StartTime: now.Add(-time.Hour),
				EndTime:   now,
				TopN:      10,
				Page:      1,
			},
			wantCode:   400,
			wantReason: "OPS_PAGINATION_CONFLICT",
		},
		{
			name: "top_n 参数越界",
			filter: &OpsOpenAITokenStatsFilter{
				StartTime: now.Add(-time.Hour),
				EndTime:   now,
				TopN:      101,
			},
			wantCode:   400,
			wantReason: "OPS_TOPN_INVALID",
		},
		{
			name: "page_size 参数越界",
			filter: &OpsOpenAITokenStatsFilter{
				StartTime: now.Add(-time.Hour),
				EndTime:   now,
				Page:      1,
				PageSize:  101,
			},
			wantCode:   400,
			wantReason: "OPS_PAGE_SIZE_INVALID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &OpsService{
				opsRepo: &openAITokenStatsRepoStub{},
			}

			_, err := svc.GetOpenAITokenStats(context.Background(), tt.filter)
			require.Error(t, err)
			require.Equal(t, tt.wantCode, infraerrors.Code(err))
			require.Equal(t, tt.wantReason, infraerrors.Reason(err))
		})
	}
}

func TestOpsServiceGetOpenAITokenStats_DefaultPagination(t *testing.T) {
	now := time.Now().UTC()
	repo := &openAITokenStatsRepoStub{
		resp: &OpsOpenAITokenStatsResponse{
			Items: []*OpsOpenAITokenStatsItem{
				{Model: "gpt-4o-mini", RequestCount: 10},
			},
			Total: 1,
		},
	}
	svc := &OpsService{opsRepo: repo}

	filter := &OpsOpenAITokenStatsFilter{
		TimeRange: "30d",
		StartTime: now.Add(-30 * 24 * time.Hour),
		EndTime:   now,
	}
	resp, err := svc.GetOpenAITokenStats(context.Background(), filter)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotNil(t, repo.captured)
	require.Equal(t, 1, repo.captured.Page)
	require.Equal(t, 20, repo.captured.PageSize)
	require.Equal(t, 0, repo.captured.TopN)
}

func TestOpsServiceGetOpenAITokenStats_RepoUnavailable(t *testing.T) {
	now := time.Now().UTC()
	svc := &OpsService{}

	_, err := svc.GetOpenAITokenStats(context.Background(), &OpsOpenAITokenStatsFilter{
		TimeRange: "1h",
		StartTime: now.Add(-time.Hour),
		EndTime:   now,
		TopN:      10,
	})
	require.Error(t, err)
	require.Equal(t, 503, infraerrors.Code(err))
	require.Equal(t, "OPS_REPO_UNAVAILABLE", infraerrors.Reason(err))
}

func int64Ptr(v int64) *int64 { return &v }
