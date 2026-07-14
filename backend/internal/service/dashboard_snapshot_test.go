package service

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/stretchr/testify/require"
)

type dashboardSnapshotRepoStub struct {
	UsageLogRepository
	active    atomic.Int32
	maxActive atomic.Int32
}

func (r *dashboardSnapshotRepoStub) run() {
	active := r.active.Add(1)
	defer r.active.Add(-1)
	for {
		current := r.maxActive.Load()
		if active <= current || r.maxActive.CompareAndSwap(current, active) {
			break
		}
	}
	time.Sleep(15 * time.Millisecond)
}

func (r *dashboardSnapshotRepoStub) GetDashboardStats(context.Context) (*usagestats.DashboardStats, error) {
	r.run()
	return &usagestats.DashboardStats{TotalUsers: 1}, nil
}

func (r *dashboardSnapshotRepoStub) GetUsageTrendWithFilters(context.Context, time.Time, time.Time, string, int64, int64, int64, int64, string, *int16, *bool, *int8) ([]usagestats.TrendDataPoint, error) {
	r.run()
	return []usagestats.TrendDataPoint{{Requests: 2}}, nil
}

func (r *dashboardSnapshotRepoStub) GetModelStatsWithFilters(context.Context, time.Time, time.Time, int64, int64, int64, int64, *int16, *bool, *int8) ([]usagestats.ModelStat, error) {
	r.run()
	return []usagestats.ModelStat{{Model: "m"}}, nil
}

func (r *dashboardSnapshotRepoStub) GetGroupStatsWithFilters(context.Context, time.Time, time.Time, int64, int64, int64, int64, *int16, *bool, *int8) ([]usagestats.GroupStat, error) {
	r.run()
	return []usagestats.GroupStat{{GroupID: 3}}, nil
}

func (r *dashboardSnapshotRepoStub) GetDashboardUserInsights(context.Context, time.Time, time.Time, string, int, int) (*usagestats.DashboardUserInsights, error) {
	r.run()
	return &usagestats.DashboardUserInsights{
		Trend: []usagestats.UserUsageTrendPoint{{UserID: 4}},
		Ranking: usagestats.UserSpendingRankingResponse{
			Ranking: []usagestats.UserSpendingRankingItem{{UserID: 4}},
		},
	}, nil
}

func TestDashboardServiceGetDashboardSnapshotRunsIndependentReadsConcurrently(t *testing.T) {
	repo := &dashboardSnapshotRepoStub{}
	svc := NewDashboardService(repo, nil, nil, nil)
	result, err := svc.GetDashboardSnapshot(context.Background(), DashboardSnapshotQuery{
		StartTime:         time.Now().Add(-time.Hour),
		EndTime:           time.Now(),
		Granularity:       "hour",
		IncludeStats:      true,
		IncludeTrend:      true,
		IncludeModels:     true,
		IncludeGroups:     true,
		IncludeUsersTrend: true,
		IncludeRanking:    true,
		UsersTrendLimit:   12,
		RankingLimit:      12,
	})

	require.NoError(t, err)
	require.Equal(t, int64(1), result.Stats.TotalUsers)
	require.Len(t, result.Trend, 1)
	require.Len(t, result.Models, 1)
	require.Len(t, result.Groups, 1)
	require.NotNil(t, result.UserInsights)
	require.GreaterOrEqual(t, repo.maxActive.Load(), int32(4))
}
