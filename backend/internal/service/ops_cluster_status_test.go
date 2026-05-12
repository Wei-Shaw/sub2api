package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestBuildClusterStatusSummary(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 12, 12, 0, 0, 0, time.UTC)

	t.Run("master and slave online is healthy", func(t *testing.T) {
		t.Parallel()

		summary := buildClusterStatusSummary(now, []*OpsInstanceHeartbeat{
			{
				InstanceID:                  "master-1",
				Role:                        config.DeploymentRoleMaster,
				Hostname:                    "master-host",
				AutonomousBackgroundEnabled: true,
				StartedAt:                   now.Add(-10 * time.Minute),
				LastSeenAt:                  now.Add(-10 * time.Second),
			},
			{
				InstanceID:                  "slave-1",
				Role:                        config.DeploymentRoleSlave,
				Hostname:                    "slave-host",
				AutonomousBackgroundEnabled: false,
				StartedAt:                   now.Add(-8 * time.Minute),
				LastSeenAt:                  now.Add(-12 * time.Second),
			},
		}, nil)

		require.Equal(t, opsClusterStatusHealthy, summary.Status)
		require.Equal(t, 2, summary.OnlineInstances)
		require.Equal(t, 1, summary.OnlineMasters)
		require.Equal(t, 1, summary.OnlineSlaves)
		require.Len(t, summary.Instances, 2)
		require.Equal(t, "master-1", summary.Instances[0].InstanceID)
		require.Equal(t, opsClusterInstanceOnline, summary.Instances[0].Status)
	})

	t.Run("missing slave is warning", func(t *testing.T) {
		t.Parallel()

		summary := buildClusterStatusSummary(now, []*OpsInstanceHeartbeat{
			{
				InstanceID:                  "master-1",
				Role:                        config.DeploymentRoleMaster,
				AutonomousBackgroundEnabled: true,
				StartedAt:                   now.Add(-10 * time.Minute),
				LastSeenAt:                  now.Add(-10 * time.Second),
			},
		}, nil)

		require.Equal(t, opsClusterStatusWarning, summary.Status)
		require.Equal(t, 1, summary.OnlineMasters)
		require.Equal(t, 0, summary.OnlineSlaves)
	})

	t.Run("multiple masters is critical", func(t *testing.T) {
		t.Parallel()

		summary := buildClusterStatusSummary(now, []*OpsInstanceHeartbeat{
			{InstanceID: "master-1", Role: config.DeploymentRoleMaster, StartedAt: now, LastSeenAt: now.Add(-5 * time.Second)},
			{InstanceID: "master-2", Role: config.DeploymentRoleMaster, StartedAt: now, LastSeenAt: now.Add(-8 * time.Second)},
			{InstanceID: "slave-1", Role: config.DeploymentRoleSlave, StartedAt: now, LastSeenAt: now.Add(-6 * time.Second)},
		}, nil)

		require.Equal(t, opsClusterStatusCritical, summary.Status)
		require.Equal(t, 2, summary.OnlineMasters)
	})

	t.Run("job warnings degrade healthy cluster", func(t *testing.T) {
		t.Parallel()

		successAt := now.Add(-30 * time.Second)
		errorAt := now.Add(-10 * time.Second)
		summary := buildClusterStatusSummary(now, []*OpsInstanceHeartbeat{
			{InstanceID: "master-1", Role: config.DeploymentRoleMaster, StartedAt: now, LastSeenAt: now.Add(-5 * time.Second)},
			{InstanceID: "slave-1", Role: config.DeploymentRoleSlave, StartedAt: now, LastSeenAt: now.Add(-5 * time.Second)},
		}, []*OpsJobHeartbeat{
			{JobName: "ops_cleanup", LastSuccessAt: &successAt, LastErrorAt: &errorAt},
		})

		require.Equal(t, opsClusterStatusWarning, summary.Status)
		require.Equal(t, 1, summary.JobWarningCount)
	})

	t.Run("standalone instance stays healthy", func(t *testing.T) {
		t.Parallel()

		summary := buildClusterStatusSummary(now, []*OpsInstanceHeartbeat{
			{InstanceID: "standalone-1", Role: config.DeploymentRoleStandalone, StartedAt: now, LastSeenAt: now.Add(-5 * time.Second)},
		}, nil)

		require.Equal(t, opsClusterStatusHealthy, summary.Status)
		require.Equal(t, 1, summary.OnlineInstances)
		require.Equal(t, 0, summary.OnlineMasters)
		require.Equal(t, 0, summary.OnlineSlaves)
	})

	t.Run("stale heartbeat is offline and unknown", func(t *testing.T) {
		t.Parallel()

		summary := buildClusterStatusSummary(now, []*OpsInstanceHeartbeat{
			{InstanceID: "master-1", Role: config.DeploymentRoleMaster, StartedAt: now, LastSeenAt: now.Add(-2 * time.Minute)},
		}, nil)

		require.Equal(t, opsClusterStatusUnknown, summary.Status)
		require.Equal(t, 0, summary.OnlineInstances)
		require.Equal(t, opsClusterInstanceOffline, summary.Instances[0].Status)
	})
}

func TestOpsServiceGetDashboardOverviewIncludesClusterStatus(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	svc := &OpsService{
		opsRepo: &opsRepoMock{
			GetDashboardOverviewFn: func(ctx context.Context, filter *OpsDashboardFilter) (*OpsDashboardOverview, error) {
				return &OpsDashboardOverview{}, nil
			},
			GetLatestSystemMetricsFn: func(ctx context.Context, windowMinutes int) (*OpsSystemMetricsSnapshot, error) {
				return &OpsSystemMetricsSnapshot{}, nil
			},
			ListJobHeartbeatsFn: func(ctx context.Context) ([]*OpsJobHeartbeat, error) {
				return []*OpsJobHeartbeat{}, nil
			},
			ListInstanceHeartbeatsFn: func(ctx context.Context) ([]*OpsInstanceHeartbeat, error) {
				return []*OpsInstanceHeartbeat{
					{InstanceID: "master-1", Role: config.DeploymentRoleMaster, StartedAt: now, LastSeenAt: now},
					{InstanceID: "slave-1", Role: config.DeploymentRoleSlave, StartedAt: now, LastSeenAt: now},
				}, nil
			},
		},
		cfg: &config.Config{Ops: config.OpsConfig{Enabled: true}},
	}

	filter := &OpsDashboardFilter{
		StartTime: now.Add(-time.Hour),
		EndTime:   now,
	}

	result, err := svc.GetDashboardOverview(context.Background(), filter)
	require.NoError(t, err)
	require.NotNil(t, result.ClusterStatus)
	require.Equal(t, opsClusterStatusHealthy, result.ClusterStatus.Status)
}
