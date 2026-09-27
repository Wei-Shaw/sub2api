//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/stretchr/testify/require"
)

type dashboardStatsCountingRepo struct {
	UsageLogRepository

	userCalls map[int64]int
	keyCalls  map[int64]int
	keyErr    error
}

func (r *dashboardStatsCountingRepo) GetUserDashboardStats(_ context.Context, userID int64) (*usagestats.UserDashboardStats, error) {
	r.userCalls[userID]++
	return &usagestats.UserDashboardStats{TotalRequests: userID}, nil
}

func (r *dashboardStatsCountingRepo) GetAPIKeyDashboardStats(_ context.Context, apiKeyID int64) (*usagestats.UserDashboardStats, error) {
	r.keyCalls[apiKeyID]++
	if r.keyErr != nil {
		return nil, r.keyErr
	}
	return &usagestats.UserDashboardStats{TotalRequests: apiKeyID * 10}, nil
}

func TestUsageServiceDashboardStatsCachedWithinTTL(t *testing.T) {
	ctx := context.Background()
	repo := &dashboardStatsCountingRepo{userCalls: map[int64]int{}, keyCalls: map[int64]int{}}
	svc := NewUsageService(repo, nil, nil, nil)

	for range 2 {
		keyStats, err := svc.GetAPIKeyDashboardStats(ctx, 7)
		require.NoError(t, err)
		require.EqualValues(t, 70, keyStats.TotalRequests)

		userStats, err := svc.GetUserDashboardStats(ctx, 7)
		require.NoError(t, err)
		require.EqualValues(t, 7, userStats.TotalRequests)
	}
	t.Logf("repo aggregates for 2 calls within TTL: api_key=%d user=%d", repo.keyCalls[7], repo.userCalls[7])
	require.Equal(t, 1, repo.keyCalls[7])
	require.Equal(t, 1, repo.userCalls[7], "user and api key entries must not collide")

	_, err := svc.GetAPIKeyDashboardStats(ctx, 8)
	require.NoError(t, err)
	require.Equal(t, 1, repo.keyCalls[8])
}

func TestUsageServiceDashboardStatsErrorsAreNotCached(t *testing.T) {
	ctx := context.Background()
	repo := &dashboardStatsCountingRepo{userCalls: map[int64]int{}, keyCalls: map[int64]int{}, keyErr: errors.New("db down")}
	svc := NewUsageService(repo, nil, nil, nil)

	_, err := svc.GetAPIKeyDashboardStats(ctx, 7)
	require.Error(t, err)

	repo.keyErr = nil
	stats, err := svc.GetAPIKeyDashboardStats(ctx, 7)
	require.NoError(t, err)
	require.EqualValues(t, 70, stats.TotalRequests)
	require.Equal(t, 2, repo.keyCalls[7])
}
