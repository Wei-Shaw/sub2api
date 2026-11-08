package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/stretchr/testify/require"
)

// dashboardTrendFiltersRepo 实现带 filters 的趋势方法（真实仓库的形状）。
type dashboardTrendFiltersRepo struct {
	UsageLogRepository
	captured usagestats.UsageLogFilters
	calls    int
}

func (s *dashboardTrendFiltersRepo) GetUsageTrendWithUsageFilters(_ context.Context, _ time.Time, _ time.Time, _ string, filters usagestats.UsageLogFilters) ([]usagestats.TrendDataPoint, error) {
	s.calls++
	s.captured = filters
	return []usagestats.TrendDataPoint{}, nil
}

// dashboardTrendLegacyRepo 只实现不带 filters 的旧签名，用来复现「承载不了排除名单」的场景。
type dashboardTrendLegacyRepo struct {
	UsageLogRepository
	calls int
}

func (s *dashboardTrendLegacyRepo) GetUsageTrendWithFilters(_ context.Context, _ time.Time, _ time.Time, _ string, _, _, _, _ int64, _ string, _ *int16, _ *bool, _ *int8) ([]usagestats.TrendDataPoint, error) {
	s.calls++
	return []usagestats.TrendDataPoint{}, nil
}

// TestDashboardServiceWithProfitExclusion_InjectsIntoTrend 是本文件最重要的一条。
//
// 使用记录页的趋势图走 `/admin/dashboard/snapshot-v2`，那条路从 DashboardService
// 直连 usage_logs，**不经过 UsageService**。只改 UsageService 的后果是：同一页上
// 汇总卡片排除了内部人员、趋势图的利润线没排除，两块数字互相矛盾且都不报错。
// 这条测试把「数量名单必须一路传到查询层」这个接缝钉死。
func TestDashboardServiceWithProfitExclusion_InjectsIntoTrend(t *testing.T) {
	repo := &dashboardTrendFiltersRepo{}
	svc := NewDashboardService(repo, nil, nil, &profitExclusionProviderStub{ids: []int64{4, 8}}, nil)

	now := time.Now()
	_, err := svc.GetUsageTrendWithUsageFilters(context.Background(), now.Add(-time.Hour), now, "hour", usagestats.UsageLogFilters{})
	require.NoError(t, err)
	require.Equal(t, 1, repo.calls)
	require.Equal(t, []int64{4, 8}, repo.captured.ProfitExcludedUserIDs)
}

// TestDashboardServiceWithProfitExclusion_EmptyListLeavesFiltersUntouched
// 验证没人被排除时，查询路径与改动前完全一致。
func TestDashboardServiceWithProfitExclusion_EmptyListLeavesFiltersUntouched(t *testing.T) {
	repo := &dashboardTrendFiltersRepo{}
	svc := NewDashboardService(repo, nil, nil, &profitExclusionProviderStub{}, nil)

	now := time.Now()
	_, err := svc.GetUsageTrendWithUsageFilters(context.Background(), now.Add(-time.Hour), now, "hour", usagestats.UsageLogFilters{Model: "gpt-5"})
	require.NoError(t, err)
	require.Nil(t, repo.captured.ProfitExcludedUserIDs)
	require.Equal(t, "gpt-5", repo.captured.Model, "既有筛选条件不能被覆盖")
}

// TestDashboardServiceWithProfitExclusion_NilProviderIsNoop 验证 provider 缺席不报错。
func TestDashboardServiceWithProfitExclusion_NilProviderIsNoop(t *testing.T) {
	repo := &dashboardTrendFiltersRepo{}
	svc := NewDashboardService(repo, nil, nil, nil, nil)

	now := time.Now()
	_, err := svc.GetUsageTrendWithUsageFilters(context.Background(), now.Add(-time.Hour), now, "hour", usagestats.UsageLogFilters{})
	require.NoError(t, err)
	require.Nil(t, repo.captured.ProfitExcludedUserIDs)
}

// TestDashboardServiceWithProfitExclusion_FailsLoudOnLegacyFallback
// 验证「回退到旧签名」时不会悄悄按未排除出数。
//
// 旧签名没有传递名单的位置，继续算下去就会给出偏高的利润且不报错。
// 这条测试锁死「宁可失败，也不发一个看不出错的错数字」。
func TestDashboardServiceWithProfitExclusion_FailsLoudOnLegacyFallback(t *testing.T) {
	repo := &dashboardTrendLegacyRepo{}
	svc := NewDashboardService(repo, nil, nil, &profitExclusionProviderStub{ids: []int64{7}}, nil)

	now := time.Now()
	_, err := svc.GetUsageTrendWithUsageFilters(context.Background(), now.Add(-time.Hour), now, "hour", usagestats.UsageLogFilters{})
	require.ErrorIs(t, err, ErrUsageProfitExclusionUnsupported)
	require.Equal(t, 0, repo.calls, "不能退到旧路径去算一个没排除任何人的数字")
}

// TestDashboardServiceWithProfitExclusion_LegacyFallbackStillWorksWhenNobodyExcluded
// 验证没人被排除时旧回退路径照常可用（不制造无谓的失败）。
func TestDashboardServiceWithProfitExclusion_LegacyFallbackStillWorksWhenNobodyExcluded(t *testing.T) {
	repo := &dashboardTrendLegacyRepo{}
	svc := NewDashboardService(repo, nil, nil, &profitExclusionProviderStub{}, nil)

	now := time.Now()
	_, err := svc.GetUsageTrendWithUsageFilters(context.Background(), now.Add(-time.Hour), now, "hour", usagestats.UsageLogFilters{})
	require.NoError(t, err)
	require.Equal(t, 1, repo.calls)
}

// TestDashboardServiceWithProfitExclusion_PropagatesReadFailure 验证读名单失败即整体失败。
func TestDashboardServiceWithProfitExclusion_PropagatesReadFailure(t *testing.T) {
	repo := &dashboardTrendFiltersRepo{}
	svc := NewDashboardService(repo, nil, nil, &profitExclusionProviderStub{err: context.DeadlineExceeded}, nil)

	now := time.Now()
	_, err := svc.GetUsageTrendWithUsageFilters(context.Background(), now.Add(-time.Hour), now, "hour", usagestats.UsageLogFilters{})
	require.Error(t, err)
	require.Equal(t, 0, repo.calls)
}
