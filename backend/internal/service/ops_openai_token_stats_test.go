package service

import (
	"context"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

type tokenStatsRepoStub struct {
	OpsRepository
	resp     *OpsTokenStatsResponse
	err      error
	captured *OpsTokenStatsFilter
}

func (s *tokenStatsRepoStub) GetTokenStats(ctx context.Context, filter *OpsTokenStatsFilter) (*OpsTokenStatsResponse, error) {
	s.captured = filter
	if s.err != nil {
		return nil, s.err
	}
	if s.resp != nil {
		return s.resp, nil
	}
	return &OpsTokenStatsResponse{}, nil
}

func TestOpsServiceGetTokenStats_Validation(t *testing.T) {
	now := time.Now().UTC()

	tests := []struct {
		name       string
		filter     *OpsTokenStatsFilter
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
			filter: &OpsTokenStatsFilter{
				StartTime: time.Time{},
				EndTime:   now,
			},
			wantCode:   400,
			wantReason: "OPS_TIME_RANGE_REQUIRED",
		},
		{
			name: "start_time 不能晚于 end_time",
			filter: &OpsTokenStatsFilter{
				StartTime: now,
				EndTime:   now.Add(-1 * time.Minute),
			},
			wantCode:   400,
			wantReason: "OPS_TIME_RANGE_INVALID",
		},
		{
			name: "group_id 必须大于 0",
			filter: &OpsTokenStatsFilter{
				StartTime: now.Add(-time.Hour),
				EndTime:   now,
				GroupID:   int64Ptr(0),
			},
			wantCode:   400,
			wantReason: "OPS_GROUP_ID_INVALID",
		},
		{
			name: "platform 必须是具体平台",
			filter: &OpsTokenStatsFilter{
				StartTime: now.Add(-time.Hour),
				EndTime:   now,
				Platform:  PlatformComposite,
			},
			wantCode:   400,
			wantReason: "OPS_TOKEN_STATS_PLATFORM_INVALID",
		},
		{
			name: "top_n 与分页参数互斥",
			filter: &OpsTokenStatsFilter{
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
			filter: &OpsTokenStatsFilter{
				StartTime: now.Add(-time.Hour),
				EndTime:   now,
				TopN:      101,
			},
			wantCode:   400,
			wantReason: "OPS_TOPN_INVALID",
		},
		{
			name: "page_size 参数越界",
			filter: &OpsTokenStatsFilter{
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
				opsRepo: &tokenStatsRepoStub{},
			}

			_, err := svc.GetTokenStats(context.Background(), tt.filter)
			require.Error(t, err)
			require.Equal(t, tt.wantCode, infraerrors.Code(err))
			require.Equal(t, tt.wantReason, infraerrors.Reason(err))
		})
	}
}

func TestOpsServiceGetTokenStats_DefaultPagination(t *testing.T) {
	now := time.Now().UTC()
	repo := &tokenStatsRepoStub{
		resp: &OpsTokenStatsResponse{
			Items: []*OpsTokenStatsItem{
				{Model: "gpt-4o-mini", RequestCount: 10},
			},
			Total: 1,
		},
	}
	svc := &OpsService{opsRepo: repo}

	filter := &OpsTokenStatsFilter{
		TimeRange: "30d",
		StartTime: now.Add(-30 * 24 * time.Hour),
		EndTime:   now,
	}
	resp, err := svc.GetTokenStats(context.Background(), filter)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotNil(t, repo.captured)
	require.Equal(t, 1, repo.captured.Page)
	require.Equal(t, 20, repo.captured.PageSize)
	require.Equal(t, 0, repo.captured.TopN)
	require.Equal(t, "", repo.captured.Platform)
}

func TestOpsServiceGetTokenStats_NormalizesConcretePlatform(t *testing.T) {
	now := time.Now().UTC()
	repo := &tokenStatsRepoStub{}
	svc := &OpsService{opsRepo: repo}

	_, err := svc.GetTokenStats(context.Background(), &OpsTokenStatsFilter{
		StartTime: now.Add(-time.Hour),
		EndTime:   now,
		Platform:  " DeepSeek ",
		TopN:      10,
	})
	require.NoError(t, err)
	require.NotNil(t, repo.captured)
	require.Equal(t, PlatformDeepseek, repo.captured.Platform)
}

func TestOpsServiceGetTokenStats_RepoUnavailable(t *testing.T) {
	now := time.Now().UTC()
	svc := &OpsService{}

	_, err := svc.GetTokenStats(context.Background(), &OpsTokenStatsFilter{
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
