package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/stretchr/testify/require"
)

// profitExclusionProviderStub 是最小的名单提供者替身。
type profitExclusionProviderStub struct {
	ids []int64
	err error
}

func (s *profitExclusionProviderStub) ExcludedUserIDs(context.Context) ([]int64, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.ids, nil
}

// usageStatsFilterCapture 只实现 GetStatsWithFilters，其余方法由内嵌接口兜底
// （nil 时被调用会 panic，从而暴露「测试覆盖不到但生产会走到」的路径）。
type usageStatsFilterCapture struct {
	UsageLogRepository
	captured usagestats.UsageLogFilters
	calls    int
}

func (s *usageStatsFilterCapture) GetStatsWithFilters(_ context.Context, filters usagestats.UsageLogFilters) (*usagestats.UsageStats, error) {
	s.calls++
	s.captured = filters
	return &usagestats.UsageStats{}, nil
}

// usageTrendFilterCapture 只实现走 filters 的那条趋势路径。
type usageTrendFilterCapture struct {
	UsageLogRepository
	captured usagestats.UsageLogFilters
	calls    int
}

func (s *usageTrendFilterCapture) GetUsageTrendWithUsageFilters(_ context.Context, _ time.Time, _ time.Time, _ string, filters usagestats.UsageLogFilters) ([]usagestats.TrendDataPoint, error) {
	s.calls++
	s.captured = filters
	return []usagestats.TrendDataPoint{}, nil
}

// TestUsageServiceWithProfitExclusion_InjectsIntoStats 验证名单真的传到了查询层。
//
// 这是整条链路的关键接缝：名单在 service 层填、在 repository 层用 FILTER 生效。
// 接缝断了不会报错，只会让统计悄悄按「未排除」算——所以必须有测试钉住。
func TestUsageServiceWithProfitExclusion_InjectsIntoStats(t *testing.T) {
	repo := &usageStatsFilterCapture{}
	svc := NewUsageService(repo, nil, nil, nil, &profitExclusionProviderStub{ids: []int64{3, 9}})

	_, err := svc.GetStatsWithFilters(context.Background(), usagestats.UsageLogFilters{})
	require.NoError(t, err)
	require.Equal(t, 1, repo.calls)
	require.Equal(t, []int64{3, 9}, repo.captured.ProfitExcludedUserIDs)
}

func TestUsageServiceWithProfitExclusion_InjectsIntoTrend(t *testing.T) {
	repo := &usageTrendFilterCapture{}
	svc := NewUsageService(repo, nil, nil, nil, &profitExclusionProviderStub{ids: []int64{42}})

	now := time.Now()
	_, err := svc.GetUsageTrendWithFilters(context.Background(), now.Add(-time.Hour), now, "hour", usagestats.UsageLogFilters{})
	require.NoError(t, err)
	require.Equal(t, 1, repo.calls)
	require.Equal(t, []int64{42}, repo.captured.ProfitExcludedUserIDs)
}

// TestUsageServiceWithProfitExclusion_PreservesExistingFilters 验证原有筛选条件不被覆盖。
func TestUsageServiceWithProfitExclusion_PreservesExistingFilters(t *testing.T) {
	repo := &usageStatsFilterCapture{}
	svc := NewUsageService(repo, nil, nil, nil, &profitExclusionProviderStub{ids: []int64{1}})

	_, err := svc.GetStatsWithFilters(context.Background(), usagestats.UsageLogFilters{UserID: 7, Model: "gpt-5"})
	require.NoError(t, err)
	require.Equal(t, int64(7), repo.captured.UserID)
	require.Equal(t, "gpt-5", repo.captured.Model)
	require.Equal(t, []int64{1}, repo.captured.ProfitExcludedUserIDs)
}

// TestUsageServiceWithProfitExclusion_EmptyListLeavesFiltersUntouched
// 验证「没人被排除」时不产生任何多余字段，查询路径与改动前完全一致。
func TestUsageServiceWithProfitExclusion_EmptyListLeavesFiltersUntouched(t *testing.T) {
	repo := &usageStatsFilterCapture{}
	svc := NewUsageService(repo, nil, nil, nil, &profitExclusionProviderStub{ids: nil})

	_, err := svc.GetStatsWithFilters(context.Background(), usagestats.UsageLogFilters{})
	require.NoError(t, err)
	require.Nil(t, repo.captured.ProfitExcludedUserIDs)
}

// TestUsageServiceWithProfitExclusion_NilProviderIsNoop
// 验证 provider 缺席（既有测试与调用方直接构造 UsegeService 的情形）不报错。
func TestUsageServiceWithProfitExclusion_NilProviderIsNoop(t *testing.T) {
	repo := &usageStatsFilterCapture{}
	svc := NewUsageService(repo, nil, nil, nil, nil)

	_, err := svc.GetStatsWithFilters(context.Background(), usagestats.UsageLogFilters{})
	require.NoError(t, err)
	require.Nil(t, repo.captured.ProfitExcludedUserIDs)
}

// TestUsageServiceWithProfitExclusion_PropagatesReadFailure 是本文件最重要的一条。
//
// 读名单失败必须让整个统计请求失败，**不能**降级成「不排除」继续返回数字：
// 降级的结果是毛利偏高，而界面上完全看不出差别。
func TestUsageServiceWithProfitExclusion_PropagatesReadFailure(t *testing.T) {
	repo := &usageStatsFilterCapture{}
	svc := NewUsageService(repo, nil, nil, nil, &profitExclusionProviderStub{err: errors.New("store down")})

	_, err := svc.GetStatsWithFilters(context.Background(), usagestats.UsageLogFilters{})
	require.Error(t, err)
	require.Equal(t, 0, repo.calls, "名单没解析出来就不该去查统计，否则会拿到偏高的数字")
}
