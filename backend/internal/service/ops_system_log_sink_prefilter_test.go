//go:build unit

package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/stretchr/testify/require"
)

func TestOpsSystemLogSink_AcceptsLogEntryMatchesIndexPolicy(t *testing.T) {
	var _ logger.SinkEntryFilter = (*OpsSystemLogSink)(nil)
	sink := NewOpsSystemLogSink(nil)

	require.False(t, sink.AcceptsLogEntry(logger.LevelInfo, "handler.gateway.messages"))
	require.False(t, sink.AcceptsLogEntry(logger.LevelDebug, "slog"))
	require.True(t, sink.AcceptsLogEntry(logger.LevelWarn, "handler.gateway.messages"))
	require.True(t, sink.AcceptsLogEntry(logger.LevelError, ""))
	require.True(t, sink.AcceptsLogEntry(logger.LevelInfo, "audit.log"))

	require.False(t, sink.AcceptsLogEntry(logger.LevelInfo, "http.access"))
	sink.SetPersistAccessLogs(true)
	require.True(t, sink.AcceptsLogEntry(logger.LevelInfo, "http.access"))
}
