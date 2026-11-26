package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/stretchr/testify/require"
)

type upstreamErrorUsageLogRepoStub struct {
	usageBatchLogRepoStub
	windowStats  map[int64]*usagestats.AccountStats
	usageStats   *usagestats.AccountUsageStatsResponse
	errorCounts  map[int64]int64
	dailyCounts  map[string]int64
	errorsErr    error
	batchCalls   int
	dailyCalls   int
	batchStartAt time.Time
}

func (r *upstreamErrorUsageLogRepoStub) GetAccountWindowStatsBatch(_ context.Context, _ []int64, _ time.Time) (map[int64]*usagestats.AccountStats, error) {
	return r.windowStats, nil
}

func (r *upstreamErrorUsageLogRepoStub) GetAccountTodayStats(context.Context, int64) (*usagestats.AccountStats, error) {
	return &usagestats.AccountStats{Requests: 3}, nil
}

func (r *upstreamErrorUsageLogRepoStub) GetAccountUsageStats(context.Context, int64, time.Time, time.Time) (*usagestats.AccountUsageStatsResponse, error) {
	return r.usageStats, nil
}

func (r *upstreamErrorUsageLogRepoStub) GetAccountUpstreamErrorCountsBatch(_ context.Context, _ []int64, startTime time.Time) (map[int64]int64, error) {
	r.batchCalls++
	r.batchStartAt = startTime
	return r.errorCounts, r.errorsErr
}

func (r *upstreamErrorUsageLogRepoStub) GetAccountDailyUpstreamErrorCounts(context.Context, int64, time.Time, time.Time) (map[string]int64, error) {
	r.dailyCalls++
	return r.dailyCounts, r.errorsErr
}

type opsMonitoringStatusStub bool

func (s opsMonitoringStatusStub) IsMonitoringEnabled(context.Context) bool { return bool(s) }

func TestGetTodayStatsBatch_AttachesUpstreamErrorsWhenMonitoringEnabled(t *testing.T) {
	repo := &upstreamErrorUsageLogRepoStub{
		windowStats: map[int64]*usagestats.AccountStats{1: {Requests: 10}, 2: {Requests: 5}},
		errorCounts: map[int64]int64{1: 4},
	}
	svc := &AccountUsageService{usageLogRepo: repo, opsMonitoring: opsMonitoringStatusStub(true)}

	result, err := svc.GetTodayStatsBatch(context.Background(), []int64{1, 2})
	require.NoError(t, err)
	require.Equal(t, 1, repo.batchCalls)
	require.Equal(t, timezone.Today(), repo.batchStartAt)
	require.NotNil(t, result[1].UpstreamErrors)
	require.Equal(t, int64(4), *result[1].UpstreamErrors)
	require.NotNil(t, result[2].UpstreamErrors)
	require.Equal(t, int64(0), *result[2].UpstreamErrors)
	require.Equal(t, int64(10), result[1].Requests)
}

func TestGetTodayStatsBatch_OmitsUpstreamErrorsWhenMonitoringDisabled(t *testing.T) {
	for name, monitoring := range map[string]opsMonitoringStatusReader{
		"disabled": opsMonitoringStatusStub(false),
		"unwired":  nil,
	} {
		t.Run(name, func(t *testing.T) {
			repo := &upstreamErrorUsageLogRepoStub{
				windowStats: map[int64]*usagestats.AccountStats{1: {Requests: 10}},
				errorCounts: map[int64]int64{1: 4},
			}
			svc := &AccountUsageService{usageLogRepo: repo, opsMonitoring: monitoring}

			result, err := svc.GetTodayStatsBatch(context.Background(), []int64{1})
			require.NoError(t, err)
			require.Zero(t, repo.batchCalls)
			require.Nil(t, result[1].UpstreamErrors)
		})
	}
}

func TestGetTodayStats_UpstreamErrorQueryFailureKeepsUsageStats(t *testing.T) {
	repo := &upstreamErrorUsageLogRepoStub{errorsErr: errors.New("boom")}
	svc := &AccountUsageService{usageLogRepo: repo, opsMonitoring: opsMonitoringStatusStub(true)}

	result, err := svc.GetTodayStats(context.Background(), 1)
	require.NoError(t, err)
	require.Equal(t, int64(3), result.Requests)
	require.Nil(t, result.UpstreamErrors)
}

func TestGetAccountUsageStats_MergesDailyUpstreamErrors(t *testing.T) {
	today := timezone.Now().Format("2006-01-02")
	yesterday := timezone.Now().AddDate(0, 0, -1).Format("2006-01-02")
	twoDaysAgo := timezone.Now().AddDate(0, 0, -2).Format("2006-01-02")

	repo := &upstreamErrorUsageLogRepoStub{
		usageStats: &usagestats.AccountUsageStatsResponse{
			History: []usagestats.AccountUsageHistory{
				{Date: twoDaysAgo, Requests: 7},
				{Date: today, Requests: 2},
			},
			Summary: usagestats.AccountUsageSummary{ActualDaysUsed: 2},
		},
		// yesterday 只有错误、没有成功请求。
		dailyCounts: map[string]int64{yesterday: 5, today: 1},
	}
	svc := &AccountUsageService{usageLogRepo: repo, opsMonitoring: opsMonitoringStatusStub(true)}

	stats, err := svc.GetAccountUsageStats(context.Background(), 1, time.Now().AddDate(0, 0, -3), time.Now())
	require.NoError(t, err)
	require.Len(t, stats.History, 3)

	require.Equal(t, twoDaysAgo, stats.History[0].Date)
	require.Equal(t, int64(0), *stats.History[0].UpstreamErrors)
	require.Equal(t, yesterday, stats.History[1].Date)
	require.Equal(t, int64(0), stats.History[1].Requests)
	require.Equal(t, int64(5), *stats.History[1].UpstreamErrors)
	require.NotEmpty(t, stats.History[1].Label)
	require.Equal(t, today, stats.History[2].Date)
	require.Equal(t, int64(1), *stats.History[2].UpstreamErrors)

	require.Equal(t, int64(6), *stats.Summary.TotalUpstreamErrors)
	require.Equal(t, int64(1), *stats.Summary.TodayUpstreamErrors)
	// 补入的纯错误日期不计入活跃天数。
	require.Equal(t, 2, stats.Summary.ActualDaysUsed)
}

func TestGetAccountUsageStats_OmitsUpstreamErrorsWhenMonitoringDisabled(t *testing.T) {
	repo := &upstreamErrorUsageLogRepoStub{
		usageStats: &usagestats.AccountUsageStatsResponse{
			History: []usagestats.AccountUsageHistory{{Date: "2026-01-01", Requests: 1}},
		},
		dailyCounts: map[string]int64{"2026-01-02": 3},
	}
	svc := &AccountUsageService{usageLogRepo: repo, opsMonitoring: opsMonitoringStatusStub(false)}

	stats, err := svc.GetAccountUsageStats(context.Background(), 1, time.Now().AddDate(0, 0, -3), time.Now())
	require.NoError(t, err)
	require.Zero(t, repo.dailyCalls)
	require.Len(t, stats.History, 1)
	require.Nil(t, stats.History[0].UpstreamErrors)
	require.Nil(t, stats.Summary.TotalUpstreamErrors)
}
