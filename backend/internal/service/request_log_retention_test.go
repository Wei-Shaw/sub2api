package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
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
				require.Equal(t, now.AddDate(0, 0, -tt.wantDays), repo.cleanupUsageCutoff)
				require.Equal(t, now.AddDate(0, 0, -max(365, tt.wantDays)), repo.cleanupDedupCutoff)
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
			require.Equal(t, next.AddDate(0, 0, -180), repo.cleanupUsageCutoff)
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
