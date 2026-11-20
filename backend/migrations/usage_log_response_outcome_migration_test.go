package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUsageLogResponseOutcomeMigrationIsAdditiveAndNullable(t *testing.T) {
	content, err := FS.ReadFile("242_add_usage_log_response_outcome.sql")
	require.NoError(t, err)
	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS response_outcome VARCHAR(32);")
	require.NotContains(t, sql, "NOT NULL", "legacy rows and old binaries keep NULL")
	require.NotContains(t, sql, "DEFAULT", "NULL must not be rewritten to a delivered outcome")
	require.Contains(t, sql, "response_outcome IN ('response_written', 'client_cancelled', 'write_failed', 'upstream_failed')")
	require.Contains(t, sql, "NOT VALID", "no full-table validation scan of usage_logs")
	for _, line := range strings.Split(string(content), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "--") {
			continue
		}
		require.NotContains(t, strings.ToUpper(trimmed), "DROP COLUMN", "up-only: no destructive down migration")
	}
}
