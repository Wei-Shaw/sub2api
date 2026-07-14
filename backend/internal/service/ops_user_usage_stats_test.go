package service

import (
	"context"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

type userUsageStatsRepoStub struct {
	OpsRepository
	resp     *OpsUserUsageStatsResponse
	captured *OpsUserUsageStatsFilter
}

func (s *userUsageStatsRepoStub) GetUserUsageStats(_ context.Context, filter *OpsUserUsageStatsFilter) (*OpsUserUsageStatsResponse, error) {
	s.captured = filter
	return s.resp, nil
}

func TestOpsServiceGetUserUsageStats_DefaultPagination(t *testing.T) {
	now := time.Now().UTC()
	repo := &userUsageStatsRepoStub{resp: &OpsUserUsageStatsResponse{Total: 1}}
	svc := &OpsService{opsRepo: repo}
	filter := &OpsUserUsageStatsFilter{
		TimeRange: "24h",
		StartTime: now.Add(-24 * time.Hour),
		EndTime:   now,
	}

	resp, err := svc.GetUserUsageStats(context.Background(), filter)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, 1, repo.captured.Page)
	require.Equal(t, 20, repo.captured.PageSize)
}

func TestOpsServiceGetUserUsageStats_RepoUnavailable(t *testing.T) {
	now := time.Now().UTC()
	_, err := (&OpsService{}).GetUserUsageStats(context.Background(), &OpsUserUsageStatsFilter{
		TimeRange: "24h",
		StartTime: now.Add(-24 * time.Hour),
		EndTime:   now,
		TopN:      20,
	})
	require.Error(t, err)
	require.Equal(t, 503, infraerrors.Code(err))
	require.Equal(t, "OPS_REPO_UNAVAILABLE", infraerrors.Reason(err))
}
