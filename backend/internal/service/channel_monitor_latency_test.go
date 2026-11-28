//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestChannelMonitorConfiguredLatencyBoundary(t *testing.T) {
	for _, tc := range []struct {
		ms     int
		status string
	}{
		{999, MonitorStatusOperational},
		{1000, MonitorStatusDegraded},
		{1001, MonitorStatusDegraded},
		{3568, MonitorStatusDegraded},
	} {
		result := finalizeOperationalOrDegraded(&CheckResult{}, time.Duration(tc.ms)*time.Millisecond, tc.ms, &CheckOptions{DegradedThresholdMS: 1000})
		require.Equal(t, tc.status, result.Status, "latency=%dms", tc.ms)
	}
	for _, opts := range []*CheckOptions{nil, {}, {DegradedThresholdMS: -1}} {
		require.Equal(t, MonitorStatusOperational, finalizeOperationalOrDegraded(&CheckResult{}, 5*time.Second, 5000, opts).Status)
		require.Equal(t, MonitorStatusDegraded, finalizeOperationalOrDegraded(&CheckResult{}, 6*time.Second, 6000, opts).Status)
	}
}

func TestChannelMonitorRuntimeReadsLatencyThresholdChanges(t *testing.T) {
	repo := &settingPublicRepoStub{values: map[string]string{
		SettingKeyFirstTokenThresholdsMS: `{"warn":1000,"slow":30000,"critical":60000}`,
	}}
	svc := NewSettingService(repo, &config.Config{})
	require.Equal(t, 1000, svc.GetChannelMonitorRuntime(context.Background()).DegradedThresholdMS)
	repo.values[SettingKeyFirstTokenThresholdsMS] = `{"warn":2000,"slow":30000,"critical":60000}`
	require.Equal(t, 2000, svc.GetChannelMonitorRuntime(context.Background()).DegradedThresholdMS)
	delete(repo.values, SettingKeyFirstTokenThresholdsMS)
	require.Equal(t, 6000, svc.GetChannelMonitorRuntime(context.Background()).DegradedThresholdMS)
}
