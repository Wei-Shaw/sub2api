package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCompositeRouteSchemesMigration(t *testing.T) {
	content, err := FS.ReadFile("234_composite_route_schemes.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "CREATE TABLE IF NOT EXISTS composite_route_schemes")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS composite_route_scheme_id BIGINT NULL")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS scheme_id BIGINT NULL")
	require.Contains(t, sql, "DROP COLUMN IF EXISTS group_id")
	require.Contains(t, sql, "DROP CONSTRAINT IF EXISTS composite_model_routes_group_id_fkey")
	require.Contains(t, sql, "ON composite_model_routes (scheme_id, endpoint, match_type, public_model)")
}
