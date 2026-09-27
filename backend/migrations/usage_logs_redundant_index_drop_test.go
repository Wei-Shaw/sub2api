package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDropRedundantUsageLogsIndexesMigration(t *testing.T) {
	content, err := FS.ReadFile("246_drop_redundant_usage_logs_indexes_notx.sql")
	require.NoError(t, err)

	// The runner rejects transaction control anywhere in a *_notx.sql file, comments included.
	upper := strings.ToUpper(string(content))
	for _, keyword := range []string{"BEGIN", "COMMIT", "ROLLBACK"} {
		require.NotContains(t, upper, keyword)
	}

	lines := strings.Split(string(content), "\n")
	for i, line := range lines {
		if idx := strings.Index(line, "--"); idx >= 0 {
			lines[i] = line[:idx]
		}
	}
	var statements []string
	for _, stmt := range strings.Split(strings.Join(lines, "\n"), ";") {
		if stmt = strings.Join(strings.Fields(stmt), " "); stmt != "" {
			statements = append(statements, stmt)
		}
	}

	// Only indexes with ~0 scans in production; user_id/api_key_id/account_id are still used.
	require.Equal(t, []string{
		"DROP INDEX CONCURRENTLY IF EXISTS idx_usage_logs_ip_address",
		"DROP INDEX CONCURRENTLY IF EXISTS idx_usage_logs_subscription_id",
		"DROP INDEX CONCURRENTLY IF EXISTS idx_usage_logs_model",
	}, statements)

	// The composites whose left prefix replaces the dropped single-column indexes must exist.
	for file, index := range map[string]string{
		"003_subscription.sql":                      "CREATE INDEX IF NOT EXISTS idx_usage_logs_sub_created ON usage_logs(subscription_id, created_at)",
		"010_add_usage_logs_aggregated_indexes.sql": "CREATE INDEX IF NOT EXISTS idx_usage_logs_model_created_at ON usage_logs(model, created_at)",
	} {
		sql, err := FS.ReadFile(file)
		require.NoError(t, err)
		require.Contains(t, string(sql), index)
	}
}
