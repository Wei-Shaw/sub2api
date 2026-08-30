package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUsageLogEffectiveModelIndexesMigration(t *testing.T) {
	content, err := FS.ReadFile("226_add_usage_log_effective_model_indexes_notx.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_usage_logs_effective_requested_model_created")
	require.Contains(t, sql, "(COALESCE(NULLIF(BTRIM(requested_model), ''), model)), created_at DESC, id DESC")
	require.Contains(t, sql, "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_usage_logs_effective_upstream_model_created")
	require.Contains(t, sql, "(COALESCE(NULLIF(BTRIM(upstream_model), ''), model)), created_at DESC, id DESC")
}
