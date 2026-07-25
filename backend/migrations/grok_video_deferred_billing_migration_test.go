package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGrokVideoDeferredBillingMigration(t *testing.T) {
	content, err := FS.ReadFile("190_grok_video_deferred_billing.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "CREATE TABLE IF NOT EXISTS grok_video_settlements")
	require.Contains(t, sql, "UNIQUE INDEX IF NOT EXISTS grok_video_settlements_request_api_key_uq")
	require.Contains(t, sql, "ON grok_video_settlements (request_id, api_key_id)")
	require.Contains(t, sql, "CHECK (status IN ('pending', 'settled', 'failed', 'expired', 'cancelled'))")
	require.Contains(t, sql, "WHERE status = 'pending'")
	require.Contains(t, sql, "request_fingerprint VARCHAR(64) NOT NULL")
	require.Contains(t, sql, "subscription_id BIGINT")
	require.Contains(t, sql, "session_id VARCHAR(255) NOT NULL DEFAULT ''")
	require.Contains(t, sql, "pricing_snapshot_version INTEGER NOT NULL")
	require.Contains(t, sql, "pricing_basis IN ('video_second', 'fixed_request', 'token')")
	require.Contains(t, sql, "billing_type IN (0, 1)")
	require.Contains(t, sql, "actual_cost NUMERIC(20, 10) NOT NULL")
	require.Contains(t, sql, "account_rate_multiplier NUMERIC(20, 10) NOT NULL")
}

func TestGrokVideoDeferredBillingMigrationUpgradesPreSessionTable(t *testing.T) {
	content, err := FS.ReadFile("190_grok_video_deferred_billing.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "ALTER TABLE grok_video_settlements ADD COLUMN IF NOT EXISTS session_id VARCHAR(255) NOT NULL DEFAULT ''")
}
