//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type channelMonitorRuntimeStub struct {
	rt ChannelMonitorRuntime
REDACTED

func (s channelMonitorRuntimeStub) GetChannelMonitorRuntime(context.Context) ChannelMonitorRuntime {
	return s.rt
REDACTED

func TestRunCheck_ModeV2NeverProbes(t *testing.T) {
	svc := NewChannelMonitorService(nil, nil)
	svc.SetRuntimeReader(channelMonitorRuntimeStub{rt: ChannelMonitorRuntime{
		Enabled: true,
		Mode:    ChannelMonitorModeV2,
REDACTEDREDACTED)

	results, err := svc.RunCheck(context.Background(), 1)
	require.ErrorIs(t, err, ErrChannelMonitorActiveProbesRetired)
	require.Nil(t, results)
REDACTED

func TestRunCheck_DisabledReturnsDisabled(t *testing.T) {
	svc := NewChannelMonitorService(nil, nil)
	svc.SetRuntimeReader(channelMonitorRuntimeStub{rt: ChannelMonitorRuntime{
		Enabled: false,
		Mode:    ChannelMonitorModeV1,
REDACTEDREDACTED)

	_, err := svc.RunCheck(context.Background(), 1)
	require.ErrorIs(t, err, ErrChannelMonitorDisabled)
REDACTED

func TestRunCheck_NilRuntimeReaderFailsClosedAsV2(t *testing.T) {
	svc := NewChannelMonitorService(nil, nil)
	// No SetRuntimeReader → probeRuntime defaults to mode=v2 (retired).
	_, err := svc.RunCheck(context.Background(), 1)
	require.ErrorIs(t, err, ErrChannelMonitorActiveProbesRetired)
REDACTED

func TestNormalizeChannelMonitorMode(t *testing.T) {
	require.Equal(t, ChannelMonitorModeV2, normalizeChannelMonitorMode(""))
	require.Equal(t, ChannelMonitorModeV1, normalizeChannelMonitorMode("v1"))
	require.Equal(t, ChannelMonitorModeV2, normalizeChannelMonitorMode("v2"))
	require.Equal(t, ChannelMonitorModeV2, normalizeChannelMonitorMode("invalid"))
	require.Equal(t, ChannelMonitorModeV1, normalizeChannelMonitorMode(" V1 "))
REDACTED

func TestChannelMonitorRuntimeActiveProbesAllowed(t *testing.T) {
	require.False(t, (ChannelMonitorRuntime{Enabled: false, Mode: ChannelMonitorModeV1REDACTED).ActiveProbesAllowed())
	require.True(t, (ChannelMonitorRuntime{Enabled: true, Mode: ChannelMonitorModeV1REDACTED).ActiveProbesAllowed())
	require.False(t, (ChannelMonitorRuntime{Enabled: true, Mode: ChannelMonitorModeV2REDACTED).ActiveProbesAllowed())
	require.True(t, (ChannelMonitorRuntime{Enabled: true, Mode: ChannelMonitorModeV2REDACTED).PassiveAggregationAllowed())
REDACTED
