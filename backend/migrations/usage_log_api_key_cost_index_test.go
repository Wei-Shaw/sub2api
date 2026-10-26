package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUsageLogAPIKeyCostIndexMigration(t *testing.T) {
	content, err := FS.ReadFile("249_add_usage_logs_api_key_cost_index_notx.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_usage_logs_api_key_created_cost")
	require.Contains(t, sql, "ON usage_logs (api_key_id, created_at)")
	require.Contains(t, sql, "INCLUDE (actual_cost)")
}
