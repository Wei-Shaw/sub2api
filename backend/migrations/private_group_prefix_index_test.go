package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPrivateGroupPrefixIndexesMigration(t *testing.T) {
	content, err := FS.ReadFile("191_add_groups_private_prefix_indexes_notx.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_groups_private_name_active")
	require.Contains(t, sql, "ON groups (name)")
	require.Contains(t, sql, "name LIKE 'private-%'")
	require.Contains(t, sql, "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_groups_active_non_private_sort")
	require.Contains(t, sql, "ON groups (status, sort_order, id)")
	require.Contains(t, sql, "name NOT LIKE 'private-%'")
	require.Contains(t, sql, "deleted_at IS NULL")
}
