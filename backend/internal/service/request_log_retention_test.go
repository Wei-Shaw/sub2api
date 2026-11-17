package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/stretchr/testify/require"
)

func TestRequestLogRetention_RuntimePolicy(t *testing.T) {
	for _, tt := range []struct {
		name        string
		raw         string
		enabled     bool
		wantDays    int
		wantCleanup bool
	}{
		{"legacy uses deployment", `{"retention_days":30}`, true, 90, true},
		{"short window", `{"request_retention_days":7}`, true, 7, true},
		{"long window extends dedup", `{"request_retention_days":730}`, true, 730, true},
		{"forever skips request and dedup deletion", `{"request_retention_days":0}`, true, 0, false},
		{"works without aggregation", `{"request_retention_days":14}`, false, 14, true},
		{"legacy disabled stays disabled", `{"retention_days":30}`, false, 0, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			settings := newRuntimeSettingRepoStub()
			settings.values[SettingKeyOpsRuntimeLogConfig] = tt.raw
			repo := &dashboardAggregationRepoTestStub{}
			svc := NewDashboardAggregationService(repo, nil, &config.Config{DashboardAgg: config.DashboardAggregationConfig{
				Enabled:   tt.enabled,
				Retention: config.DashboardAggregationRetentionConfig{UsageLogsDays: 90, UsageBillingDedupDays: 365, HourlyDays: 180, DailyDays: 730},
			}})
			svc.settingRepo = settings
			now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
			svc.maybeCleanupRetention(context.Background(), now)
			if tt.wantCleanup {
				require.Equal(t, 1, repo.cleanupUsageCalls)
				wantUsageCutoff := GroupUsageTodayStart(now.AddDate(0, 0, -tt.wantDays))
				wantDedupCutoff := now.AddDate(0, 0, -max(365, tt.wantDays))
				if wantDedupCutoff.After(wantUsageCutoff) {
					wantDedupCutoff = wantUsageCutoff
				}
				require.Equal(t, wantUsageCutoff, repo.cleanupUsageCutoff)
				require.Equal(t, wantDedupCutoff, repo.cleanupDedupCutoff)
			} else {
				require.Zero(t, repo.cleanupUsageCalls)
				require.Zero(t, repo.cleanupDedupCalls)
			}
			if !tt.enabled {
				require.Zero(t, repo.cleanupAggregateCalls)
			}
			// Read the updated policy at the next rolling cleanup.
			settings.values[SettingKeyOpsRuntimeLogConfig] = `{"request_retention_days":180}`
			next := now.Add(dashboardAggregationRetentionInterval)
			svc.maybeCleanupRetention(context.Background(), next)
			require.Equal(t, GroupUsageTodayStart(next.AddDate(0, 0, -180)), repo.cleanupUsageCutoff)
		})
	}
}

func TestRequestLogRetention_ReadFailureSkipsDeletion(t *testing.T) {
	for _, raw := range []string{`{broken`, `{"request_retention_days":-1}`, `{"request_retention_days":3651}`} {
		settings := newRuntimeSettingRepoStub()
		settings.values[SettingKeyOpsRuntimeLogConfig] = raw
		repo := &dashboardAggregationRepoTestStub{}
		svc := &DashboardAggregationService{repo: repo, settingRepo: settings}
		svc.runScheduledRetention()
		require.Zero(t, repo.cleanupUsageCalls)
		require.Zero(t, repo.cleanupDedupCalls)
		require.Nil(t, svc.lastRetentionCleanup.Load())
	}
	settings := newRuntimeSettingRepoStub()
	settings.getValueFn = func(string) (string, error) { return "", errors.New("database unavailable") }
	svc := &DashboardAggregationService{repo: &dashboardAggregationRepoTestStub{}, settingRepo: settings}
	svc.runScheduledRetention()
	require.Nil(t, svc.lastRetentionCleanup.Load())
}

func TestRuntimeLogConfig_RequestRetentionCompatibility(t *testing.T) {
	settings := newRuntimeSettingRepoStub()
	settings.values[SettingKeyOpsRuntimeLogConfig] = `{"level":"info","retention_days":7,"request_retention_days":180}`
	svc := &OpsService{settingRepo: settings}
	require.NoError(t, logger.Init(logger.InitOptions{Level: "info", Format: "json", Output: logger.OutputOptions{ToStdout: true}}))
	t.Cleanup(logger.Sync)
	legacy := defaultOpsRuntimeLogConfig(nil)
	legacy.RequestRetentionDays = nil
	updated, err := svc.UpdateRuntimeLogConfig(context.Background(), legacy, 1)
	require.NoError(t, err)
	require.Equal(t, 180, *updated.RequestRetentionDays)
	for _, days := range []int{0, 1, 3650} {
		updated.RequestRetentionDays = &days
		saved, err := svc.UpdateRuntimeLogConfig(context.Background(), updated, 1)
		require.NoError(t, err)
		require.Equal(t, days, *saved.RequestRetentionDays)
		var persisted OpsRuntimeLogConfig
		require.NoError(t, json.Unmarshal([]byte(settings.values[SettingKeyOpsRuntimeLogConfig]), &persisted))
		require.Equal(t, days, *persisted.RequestRetentionDays)
	}
	for _, days := range []int{-1, 3651} {
		updated.RequestRetentionDays = &days
		before := settings.values[SettingKeyOpsRuntimeLogConfig]
		_, err := svc.UpdateRuntimeLogConfig(context.Background(), updated, 1)
		require.ErrorContains(t, err, "request_retention_days")
		require.Equal(t, before, settings.values[SettingKeyOpsRuntimeLogConfig])
	}
	reset, err := svc.ResetRuntimeLogConfig(context.Background(), 1)
	require.NoError(t, err)
	require.Equal(t, 90, *reset.RequestRetentionDays)
}

func TestRequestLogRetention_CutoffAlignsToConfiguredDayStart(t *testing.T) {
	previous := timezone.Name()
	require.NoError(t, timezone.Init("Asia/Shanghai"))
	t.Cleanup(func() { require.NoError(t, timezone.Init(previous)) })

	for _, tt := range []struct {
		name      string
		dedupDays int
		wantDedup time.Time
	}{
		// 2026-09-15 20:00 +08 往前 90 天是 2026-06-17 20:00 +08，对齐到 2026-06-17 00:00 +08。
		{"dedup longer than usage", 365, time.Date(2025, 9, 15, 12, 0, 0, 0, time.UTC)},
		{"dedup equal to usage keeps whole days", 90, time.Date(2026, 6, 16, 16, 0, 0, 0, time.UTC)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := &dashboardAggregationRepoTestStub{}
			svc := NewDashboardAggregationService(repo, nil, &config.Config{DashboardAgg: config.DashboardAggregationConfig{
				Enabled:   true,
				Retention: config.DashboardAggregationRetentionConfig{UsageLogsDays: 90, UsageBillingDedupDays: tt.dedupDays, HourlyDays: 180, DailyDays: 730},
			}})
			now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
			svc.maybeCleanupRetention(context.Background(), now)

			require.Equal(t, time.Date(2026, 6, 16, 16, 0, 0, 0, time.UTC), repo.cleanupUsageCutoff)
			require.Equal(t, tt.wantDedup, repo.cleanupDedupCutoff)
		})
	}
}

// 夏令时在零点跳变的日期没有 00:00：日界取该日期实际开始的时刻，而不是前一天的 23:00。
func TestGroupUsageDayBoundariesSkipMidnightDSTGap(t *testing.T) {
	previous := timezone.Name()
	require.NoError(t, timezone.Init("America/Santiago"))
	t.Cleanup(func() { require.NoError(t, timezone.Init(previous)) })
	loc := timezone.Location()

	noon := time.Date(2026, 9, 6, 12, 0, 0, 0, loc)
	start := GroupUsageTodayStart(noon)
	require.Equal(t, "2026-09-06", GroupUsageDate(start))
	require.Equal(t, "2026-09-05", GroupUsageDate(start.Add(-time.Nanosecond)))

	parsed, err := ParseGroupUsageDate("2026-09-06")
	require.NoError(t, err)
	require.True(t, parsed.Equal(start), "parsed=%s start=%s", parsed, start)

	nextNoon := time.Date(2026, 9, 7, 12, 0, 0, 0, loc)
	require.True(t, GroupUsageYesterdayStart(nextNoon).Equal(start))

	// 普通日期仍是本地零点。
	regular, err := ParseGroupUsageDate("2026-09-08")
	require.NoError(t, err)
	require.True(t, regular.Equal(time.Date(2026, 9, 8, 0, 0, 0, 0, loc)))
}

// 零点出现两次的日期（Asia/Gaza 2020-10-24 在 01:00 回拨到 00:00）：日界取第一次出现的 00:00。
func TestGroupUsageDayBoundariesUseFirstOfRepeatedMidnight(t *testing.T) {
	previous := timezone.Name()
	require.NoError(t, timezone.Init("Asia/Gaza"))
	t.Cleanup(func() { require.NoError(t, timezone.Init(previous)) })

	start, err := ParseGroupUsageDate("2020-10-24")
	require.NoError(t, err)
	require.Equal(t, "2020-10-24", GroupUsageDate(start))
	require.Equal(t, "2020-10-23", GroupUsageDate(start.Add(-time.Nanosecond)))
	require.True(t, GroupUsageTodayStart(start.Add(90*time.Minute)).Equal(start))
}
