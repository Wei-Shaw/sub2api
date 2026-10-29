//go:build unit

package repository

import (
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestChannelMonitorDuplicateOperationMetadataStaysOutOfRuntimeHeaders(t *testing.T) {
	monitor := &service.ChannelMonitor{
		ExtraHeaders:         map[string]string{"User-Agent": "Codex"},
		DuplicateOperationID: "operation-digest",
	}

	persisted := channelMonitorHeadersForPersistence(monitor)
	require.Equal(t, "operation-digest", persisted[service.ChannelMonitorDuplicateOperationIDMetadataKey])
	require.Equal(t, "Codex", persisted["User-Agent"])
	require.NotContains(t, monitor.ExtraHeaders, service.ChannelMonitorDuplicateOperationIDMetadataKey)

	restored := entToServiceMonitor(&dbent.ChannelMonitor{ExtraHeaders: persisted})
	require.Equal(t, "operation-digest", restored.DuplicateOperationID)
	require.Equal(t, map[string]string{"User-Agent": "Codex"}, restored.ExtraHeaders)
	require.NotContains(t, restored.ExtraHeaders, service.ChannelMonitorDuplicateOperationIDMetadataKey)
	require.Equal(t, "operation-digest", persisted[service.ChannelMonitorDuplicateOperationIDMetadataKey], "decoding must not mutate the ent row")
}
