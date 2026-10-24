package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCompositeRouteDestinationsMigrationFailsClosed(t *testing.T) {
	content, err := FS.ReadFile("247_composite_route_destinations.sql")
	require.NoError(t, err)
	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "target_group_id BIGINT NULL")
	require.Contains(t, sql, "target_account_id BIGINT NULL")
	require.Contains(t, sql, "CHECK (target_group_id IS NULL OR target_account_id IS NULL)")
	require.Contains(t, sql, "CHECK (target_group_id IS NULL OR target_group_id <> group_id)")
	require.Contains(t, sql, "CHECK (target_group_id IS NULL OR target_group_id > 0)")
	require.Contains(t, sql, "CHECK (target_account_id IS NULL OR target_account_id > 0)")
	require.Contains(t, sql, "REFERENCES groups(id) ON DELETE RESTRICT")
	require.Contains(t, sql, "REFERENCES accounts(id) ON DELETE RESTRICT")
	// Source-route deletion stays existing CASCADE behavior; destinations must
	// retain the restrictive grant, never silently become an unrestricted pool.
	statements := make([]string, 0)
	for _, line := range strings.Split(string(content), "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "--") {
			statements = append(statements, line)
		}
	}
	require.NotContains(t, strings.ToUpper(strings.Join(statements, " ")), "SET NULL")
}
