package repository

import (
	"database/sql"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUsageProxySnapshotPersistence(t *testing.T) {
	snapshot := &service.UsageProxySnapshot{ID: 42, Name: "original"}
	encoded := usageProxySnapshotJSON(snapshot).(string)
	require.JSONEq(t, `{"id":42,"name":"original"}`, encoded)
	loaded := decodeUsageProxySnapshot(sql.NullString{Valid: true, String: encoded})
	require.Equal(t, snapshot, loaded)
	require.Nil(t, decodeUsageProxySnapshot(sql.NullString{}))
	require.Nil(t, decodeUsageProxySnapshot(sql.NullString{Valid: true, String: "invalid"}))
	require.Nil(t, usageProxySnapshotJSON(nil))
	require.Equal(t, "{}", usageProxySnapshotJSON(&service.UsageProxySnapshot{}))
}

func TestUsageProxySnapshotIgnoresHistoricalExitIP(t *testing.T) {
	loaded := decodeUsageProxySnapshot(sql.NullString{Valid: true, String: `{"id":42,"name":"original","exit_ip":"203.0.113.7","exit_ip_checked_at":"2026-10-24T00:00:00Z"}`})
	require.Equal(t, &service.UsageProxySnapshot{ID: 42, Name: "original"}, loaded)
	require.JSONEq(t, `{"id":42,"name":"original"}`, usageProxySnapshotJSON(loaded).(string))
}
