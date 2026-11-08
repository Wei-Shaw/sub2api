//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type channelMonitorRuntimeStub struct {
	rt ChannelMonitorRuntime
}

func (s channelMonitorRuntimeStub) GetChannelMonitorRuntime(context.Context) ChannelMonitorRuntime {
	return s.rt
}

func TestRunCheck_ModeV2NeverProbes(t *testing.T) {
	svc := NewChannelMonitorService(nil, nil)
	svc.SetRuntimeReader(channelMonitorRuntimeStub{rt: ChannelMonitorRuntime{
		Enabled: true,
		Mode:    ChannelMonitorModeV2,
	}})

	results, err := svc.RunCheck(context.Background(), 1)
	require.ErrorIs(t, err, ErrChannelMonitorActiveProbesRetired)
	require.Nil(t, results)
}

func TestRunCheck_DisabledReturnsDisabled(t *testing.T) {
	svc := NewChannelMonitorService(nil, nil)
	svc.SetRuntimeReader(channelMonitorRuntimeStub{rt: ChannelMonitorRuntime{
		Enabled: false,
		Mode:    ChannelMonitorModeV1,
	}})

	_, err := svc.RunCheck(context.Background(), 1)
	require.ErrorIs(t, err, ErrChannelMonitorDisabled)
}

func TestRunCheck_NilRuntimeReaderFailsClosedAsV2(t *testing.T) {
	svc := NewChannelMonitorService(nil, nil)
	// No SetRuntimeReader → probeRuntime defaults to mode=v2 (retired).
	_, err := svc.RunCheck(context.Background(), 1)
	require.ErrorIs(t, err, ErrChannelMonitorActiveProbesRetired)
}

func TestNormalizeChannelMonitorMode(t *testing.T) {
	require.Equal(t, ChannelMonitorModeV1, normalizeChannelMonitorMode(""))
	require.Equal(t, ChannelMonitorModeV1, normalizeChannelMonitorMode("v1"))
	require.Equal(t, ChannelMonitorModeV2, normalizeChannelMonitorMode("v2"))
	require.Equal(t, ChannelMonitorModeV1, normalizeChannelMonitorMode("invalid"))
	require.Equal(t, ChannelMonitorModeV1, normalizeChannelMonitorMode(" V1 "))
}

func TestChannelMonitorRuntimeActiveProbesAllowed(t *testing.T) {
	require.False(t, (ChannelMonitorRuntime{Enabled: false, Mode: ChannelMonitorModeV1}).ActiveProbesAllowed())
	require.True(t, (ChannelMonitorRuntime{Enabled: true, Mode: ChannelMonitorModeV1}).ActiveProbesAllowed())
	require.False(t, (ChannelMonitorRuntime{Enabled: true, Mode: ChannelMonitorModeV2}).ActiveProbesAllowed())
	require.True(t, (ChannelMonitorRuntime{Enabled: true, Mode: ChannelMonitorModeV2}).PassiveAggregationAllowed())
}
