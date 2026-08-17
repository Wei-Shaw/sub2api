package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResetOpenAICompactProbeProtocolMigration(t *testing.T) {
	content, err := FS.ReadFile("224_reset_openai_compact_probe_protocol.sql")
	require.NoError(t, err)
	sql := string(content)

	for _, key := range []string{
		"openai_compact_supported",
		"openai_compact_probe_version",
		"openai_compact_checked_at",
		"openai_compact_last_status",
		"openai_compact_last_error",
		"openai_compact_probe_observed_at_unix_nano",
	} {
		require.GreaterOrEqual(t, strings.Count(sql, key), 2, key)
	}
	require.Contains(t, sql, "WHERE platform = 'openai'")
	require.NotContains(t, sql, "deleted_at IS NULL")
	require.NotContains(t, sql, "openai_compact_mode")
	require.NotContains(t, sql, "openai_responses")
}
