package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestApplyProbeResult_IsolateAfterConsecutiveFails(t *testing.T) {
	meta := ProxyHealthMeta{}
	status := StatusActive
	var isolated bool
	for i := 0; i < 3; i++ {
		status, meta, isolated, _ = ApplyProbeResult(status, meta, false, 3, 2, true, int64(100+i))
	}
	require.True(t, isolated)
	require.Equal(t, StatusInactive, status)
	require.Equal(t, ProxyHealthIsolatedByHealth, meta.IsolatedBy)
	require.Equal(t, 3, meta.FailCount)
}

func TestApplyProbeResult_SuccessResetsFailCount(t *testing.T) {
	meta := ProxyHealthMeta{FailCount: 2}
	status, meta, isolated, _ := ApplyProbeResult(StatusActive, meta, true, 3, 2, true, 1)
	require.False(t, isolated)
	require.Equal(t, StatusActive, status)
	require.Equal(t, 0, meta.FailCount)
	require.Equal(t, 1, meta.SuccessCount)
}

func TestApplyProbeResult_RecoverOnlyHealthIsolated(t *testing.T) {
	// Health-isolated recovers after success_threshold.
	meta := ProxyHealthMeta{IsolatedBy: ProxyHealthIsolatedByHealth}
	status := StatusInactive
	var recovered bool
	status, meta, _, recovered = ApplyProbeResult(status, meta, true, 3, 2, true, 10)
	require.False(t, recovered)
	require.Equal(t, StatusInactive, status)
	status, meta, _, recovered = ApplyProbeResult(status, meta, true, 3, 2, true, 11)
	require.True(t, recovered)
	require.Equal(t, StatusActive, status)
	require.Empty(t, meta.IsolatedBy)

	// Manual inactive (no isolated_by) never auto-recovers.
	meta = ProxyHealthMeta{}
	status = StatusInactive
	for i := 0; i < 5; i++ {
		status, meta, _, recovered = ApplyProbeResult(status, meta, true, 3, 2, true, int64(20+i))
		require.False(t, recovered)
		require.Equal(t, StatusInactive, status)
	}
}

func TestApplyProbeResult_AutoRecoverDisabled(t *testing.T) {
	meta := ProxyHealthMeta{IsolatedBy: ProxyHealthIsolatedByHealth, SuccessCount: 0}
	status := StatusInactive
	var recovered bool
	for i := 0; i < 5; i++ {
		status, meta, _, recovered = ApplyProbeResult(status, meta, true, 3, 2, false, int64(30+i))
		require.False(t, recovered)
		require.Equal(t, StatusInactive, status)
	}
}

func TestProxyHealthService_ShouldSkipWarpPrefix(t *testing.T) {
	s := NewProxyHealthService(nil, nil, nil, nil, nil, nil, nil)
	// Default skip includes warp-
	require.True(t, s.shouldSkip(Proxy{Name: "warp-abc"}))
	require.False(t, s.shouldSkip(Proxy{Name: "pool-1"}))
}
