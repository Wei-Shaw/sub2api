package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMigration226NarrowsLegacyCodexFingerprintModes(t *testing.T) {
	content, err := FS.ReadFile("226_converge_codex_fingerprint_modes_to_device.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "platform = 'openai'")
	require.Contains(t, sql, "type = 'oauth'")
	require.Contains(t, sql, "BTRIM(extra->>'codex_fingerprint_mode') IN ('session', 'full')")
	require.Contains(t, sql, "jsonb_set(extra, '{codex_fingerprint_mode}', '\"device\"'::jsonb, true)")
}
