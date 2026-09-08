package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCodexSlotClientVersionsMigrationAddsValidatedColumns(t *testing.T) {
	payload, err := FS.ReadFile("238_codex_slot_client_versions.sql")
	require.NoError(t, err)
	sql := strings.Join(strings.Fields(string(payload)), " ")

	for _, table := range []string{"account_codex_device_slots", "codex_identity_template_slots"} {
		require.Contains(t, sql, "ALTER TABLE "+table)
	}
	for _, required := range []string{
		"ADD COLUMN IF NOT EXISTS client_version_mode VARCHAR(20) NOT NULL DEFAULT 'inherit'",
		"ADD COLUMN IF NOT EXISTS client_version VARCHAR(64) NOT NULL DEFAULT ''",
		"CHECK (client_version_mode IN ('inherit', 'pinned'))",
		"client_version_mode = 'inherit' AND client_version = ''",
		"client_version_mode = 'pinned'",
	} {
		require.Contains(t, sql, required)
	}
	require.NotContains(t, strings.ToLower(sql), "usage_logs")
}
