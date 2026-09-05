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
	watermark            time.Time
	aggregateErr         error
	cleanupAggregatesErr error
	cleanupUsageErr      error
	cleanupDedupErr      error
	ensurePartitionErr   error
	aggregateCtx         context.Context
	events               *[]string
}

type dashboardAggregationRollupRepoTestStub struct {
	*dashboardAggregationRepoTestStub
	groupRollupCalls int
	groupRollupAt    time.Time
	groupRollupErr   error
	groupRollupCtx   context.Context
}

func (s *dashboardAggregationRollupRepoTestStub) SyncGroupUsageRollups(ctx context.Context, todayStart time.Time) error {
	s.groupRollupCalls++
	s.groupRollupAt = todayStart
	s.groupRollupCtx = ctx
	if s.events != nil {
		*s.events = append(*s.events, "group_rollup")
	}
	return s.groupRollupErr
}

func (s *dashboardAggregationRepoTestStub) AggregateRange(ctx context.Context, start, end time.Time) error {
	s.aggregateCalls++
	s.aggregateCtx = ctx
	s.lastStart = start
	s.lastEnd = end
	if s.events != nil {
		*s.events = append(*s.events, "dashboard_aggregation")
	}
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

func TestDashboardAggregationService_RunScheduledAggregationSyncsGroupUsageRollups(t *testing.T) {
	baseRepo := &dashboardAggregationRepoTestStub{watermark: time.Now().UTC()}
	repo := &dashboardAggregationRollupRepoTestStub{dashboardAggregationRepoTestStub: baseRepo}
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

	before := GroupUsageTodayStart(time.Now())
	svc.runScheduledAggregation()
	after := GroupUsageTodayStart(time.Now())

	require.Equal(t, 1, repo.groupRollupCalls)
	require.Contains(t, []time.Time{before, after}, repo.groupRollupAt)
}

func TestDashboardAggregationService_RunScheduledAggregationSyncsGroupAfterDashboardEarlyReturn(t *testing.T) {
	events := make([]string, 0, 2)
	baseRepo := &dashboardAggregationRepoTestStub{
		watermark:    time.Now().UTC(),
		aggregateErr: errors.New("dashboard aggregation failed"),
		events:       &events,
	}
	repo := &dashboardAggregationRollupRepoTestStub{
		dashboardAggregationRepoTestStub: baseRepo,
		groupRollupErr:                   errors.New("group rollup failed"),
	}
	svc := &DashboardAggregationService{
		repo: repo,
		cfg: config.DashboardAggregationConfig{
			LookbackSeconds: 120,
			Retention: config.DashboardAggregationRetentionConfig{
				UsageLogsDays: 1,
			},
		},
	}

	svc.runScheduledAggregation()

	require.Equal(t, []string{"dashboard_aggregation", "group_rollup"}, events)
	require.NotNil(t, repo.aggregateCtx)
	require.NotNil(t, repo.groupRollupCtx)
	if repo.aggregateCtx == repo.groupRollupCtx {
		t.Fatal("分组日汇总必须使用独立于 dashboard 聚合的 context")
	}
	groupDeadline, ok := repo.groupRollupCtx.Deadline()
	require.True(t, ok, "group rollup context must be bounded")
	require.LessOrEqual(t, time.Until(groupDeadline), defaultDashboardAggregationTimeout)
}

type dashboardAggregationLeaderLockRecordingCache struct {
	delegate    *fakeLeaderLockCache
	acquireKeys []string
	acquireTTLs []time.Duration
}

func (c *dashboardAggregationLeaderLockRecordingCache) TryAcquireLeaderLock(ctx context.Context, key, owner string, ttl time.Duration) (bool, error) {
	c.acquireKeys = append(c.acquireKeys, key)
	c.acquireTTLs = append(c.acquireTTLs, ttl)
	return c.delegate.TryAcquireLeaderLock(ctx, key, owner, ttl)
}

func (c *dashboardAggregationLeaderLockRecordingCache) ReleaseLeaderLock(ctx context.Context, key, owner string) error {
	return c.delegate.ReleaseLeaderLock(ctx, key, owner)
}

func TestDashboardAggregationService_StartupGroupSyncUsesIndependentLongLivedLeaderLock(t *testing.T) {
	delegate := &fakeLeaderLockCache{}
	_, err := delegate.TryAcquireLeaderLock(context.Background(), dashboardAggregationLeaderLockKey, "periodic-peer", time.Hour)
	require.NoError(t, err)
	cache := &dashboardAggregationLeaderLockRecordingCache{delegate: delegate}
	repo := &dashboardAggregationRollupRepoTestStub{dashboardAggregationRepoTestStub: &dashboardAggregationRepoTestStub{}}
	svc := &DashboardAggregationService{
		repo:       repo,
		lockCache:  cache,
		instanceID: "startup-instance",
	}

	svc.runStartupGroupUsageSync()

	require.Len(t, cache.acquireKeys, 1)
	require.NotEqual(t, dashboardAggregationLeaderLockKey, cache.acquireKeys[0])
	require.Len(t, cache.acquireTTLs, 1)
	require.Greater(t, cache.acquireTTLs[0], defaultDashboardAggregationBackfillTimeout)
	require.Equal(t, 1, repo.groupRollupCalls)
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
