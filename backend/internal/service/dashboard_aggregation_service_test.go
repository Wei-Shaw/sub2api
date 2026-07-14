package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type dashboardAggregationRepoTestStub struct {
	aggregateCalls       int
	recomputeCalls       int
	cleanupUsageCalls    int
	cleanupDedupCalls    int
	ensurePartitionCalls int
	lastStart            time.Time
	lastEnd              time.Time
	lastUsageCutoff      time.Time
	watermark            time.Time
	aggregateErr         error
	cleanupAggregatesErr error
	cleanupUsageErr      error
	cleanupDedupErr      error
	ensurePartitionErr   error
}

func (s *dashboardAggregationRepoTestStub) AggregateRange(ctx context.Context, start, end time.Time) error {
	s.aggregateCalls++
	s.lastStart = start
	s.lastEnd = end
	return s.aggregateErr
}

func (s *dashboardAggregationRepoTestStub) RecomputeRange(ctx context.Context, start, end time.Time) error {
	s.recomputeCalls++
	return s.AggregateRange(ctx, start, end)
}

func (s *dashboardAggregationRepoTestStub) GetAggregationWatermark(ctx context.Context) (time.Time, error) {
	return s.watermark, nil
}

func (s *dashboardAggregationRepoTestStub) UpdateAggregationWatermark(ctx context.Context, aggregatedAt time.Time) error {
	return nil
}

func (s *dashboardAggregationRepoTestStub) CleanupAggregates(ctx context.Context, hourlyCutoff, dailyCutoff time.Time) error {
	return s.cleanupAggregatesErr
}

func (s *dashboardAggregationRepoTestStub) CleanupUsageLogs(ctx context.Context, cutoff time.Time) error {
	s.cleanupUsageCalls++
	s.lastUsageCutoff = cutoff
	return s.cleanupUsageErr
}

func (s *dashboardAggregationRepoTestStub) CleanupUsageBillingDedup(ctx context.Context, cutoff time.Time) error {
	s.cleanupDedupCalls++
	return s.cleanupDedupErr
}

func (s *dashboardAggregationRepoTestStub) EnsureUsageLogsPartitions(ctx context.Context, now time.Time) error {
	s.ensurePartitionCalls++
	return s.ensurePartitionErr
}

func TestDashboardAggregationService_RunScheduledAggregation_EpochUsesRetentionStart(t *testing.T) {
	repo := &dashboardAggregationRepoTestStub{watermark: time.Unix(0, 0).UTC()}
	svc := &DashboardAggregationService{
		repo: repo,
		cfg: config.DashboardAggregationConfig{
			Enabled:         true,
			IntervalSeconds: 60,
			LookbackSeconds: 120,
			Retention: config.DashboardAggregationRetentionConfig{
				UsageLogsDays: 1,
				HourlyDays:    1,
				DailyDays:     1,
			},
		},
	}

	svc.runScheduledAggregation()

	require.Equal(t, 1, repo.aggregateCalls)
	require.False(t, repo.lastEnd.IsZero())
	require.Equal(t, truncateToDayUTC(repo.lastEnd.AddDate(0, 0, -1)), repo.lastStart)
}

func TestDashboardAggregationService_CleanupRetentionFailure_DoesNotRecord(t *testing.T) {
	repo := &dashboardAggregationRepoTestStub{cleanupAggregatesErr: errors.New("清理失败")}
	svc := &DashboardAggregationService{
		repo: repo,
		cfg: config.DashboardAggregationConfig{
			Retention: config.DashboardAggregationRetentionConfig{
				UsageLogsDays: 1,
				HourlyDays:    1,
				DailyDays:     1,
			},
		},
	}

	svc.maybeCleanupRetention(context.Background(), time.Now().UTC())

	require.Nil(t, svc.lastRetentionCleanup.Load())
	require.Equal(t, 1, repo.cleanupUsageCalls)
	require.Equal(t, 1, repo.cleanupDedupCalls)
}

func TestDashboardAggregationService_CleanupDedupFailure_DoesNotRecord(t *testing.T) {
	repo := &dashboardAggregationRepoTestStub{cleanupDedupErr: errors.New("dedup cleanup failed")}
	svc := &DashboardAggregationService{
		repo: repo,
		cfg: config.DashboardAggregationConfig{
			Retention: config.DashboardAggregationRetentionConfig{
				UsageLogsDays: 1,
				HourlyDays:    1,
				DailyDays:     1,
			},
		},
	}

	svc.maybeCleanupRetention(context.Background(), time.Now().UTC())

	require.Nil(t, svc.lastRetentionCleanup.Load())
	require.Equal(t, 1, repo.cleanupDedupCalls)
}

func TestDashboardAggregationService_UserRequestLogRetentionUsesRuntimeSetting(t *testing.T) {
	repo := &dashboardAggregationRepoTestStub{}
	settings := newRuntimeSettingRepoStub()
	settings.values[SettingKeyOpsAdvancedSettings] = `{"data_retention":{"user_request_log_retention_days":14}}`
	svc := &DashboardAggregationService{
		repo:        repo,
		settingRepo: settings,
		cfg: config.DashboardAggregationConfig{
			Retention: config.DashboardAggregationRetentionConfig{
				UsageLogsDays:         90,
				UsageBillingDedupDays: 365,
				HourlyDays:            180,
				DailyDays:             730,
			},
		},
	}
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)

	svc.maybeCleanupRetention(context.Background(), now)

	require.Equal(t, now.AddDate(0, 0, -14), repo.lastUsageCutoff)
}

func TestDashboardAggregationService_UserRequestLogRetentionLegacySettingFallsBackToConfig(t *testing.T) {
	settings := newRuntimeSettingRepoStub()
	settings.values[SettingKeyOpsAdvancedSettings] = `{"data_retention":{"error_log_retention_days":7}}`
	svc := &DashboardAggregationService{
		settingRepo: settings,
		cfg: config.DashboardAggregationConfig{
			Retention: config.DashboardAggregationRetentionConfig{UsageLogsDays: 45},
		},
	}

	require.Equal(t, 45, svc.userRequestLogRetentionDays(context.Background()))
}

func TestDashboardAggregationService_PartitionFailure_DoesNotAggregate(t *testing.T) {
	repo := &dashboardAggregationRepoTestStub{ensurePartitionErr: errors.New("partition failed")}
	svc := &DashboardAggregationService{
		repo: repo,
		cfg: config.DashboardAggregationConfig{
			Enabled:         true,
			IntervalSeconds: 60,
			LookbackSeconds: 120,
			Retention: config.DashboardAggregationRetentionConfig{
				UsageLogsDays:         1,
				UsageBillingDedupDays: 2,
				HourlyDays:            1,
				DailyDays:             1,
			},
		},
	}

	svc.runScheduledAggregation()

	require.Equal(t, 1, repo.ensurePartitionCalls)
	require.Equal(t, 1, repo.aggregateCalls)
}

func TestDashboardAggregationService_TriggerBackfill_TooLarge(t *testing.T) {
	repo := &dashboardAggregationRepoTestStub{}
	svc := &DashboardAggregationService{
		repo: repo,
		cfg: config.DashboardAggregationConfig{
			BackfillEnabled: true,
			BackfillMaxDays: 1,
		},
	}

	start := time.Now().AddDate(0, 0, -3)
	end := time.Now()
	err := svc.TriggerBackfill(start, end)
	require.ErrorIs(t, err, ErrDashboardBackfillTooLarge)
	require.Equal(t, 0, repo.aggregateCalls)
}
