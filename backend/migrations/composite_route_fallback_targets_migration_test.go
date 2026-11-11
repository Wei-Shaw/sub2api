package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCompositeRouteFallbackTargetsMigration(t *testing.T) {
	content, err := FS.ReadFile("246_composite_route_fallback_targets.sql")
	require.NoError(t, err)
	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "ALTER TABLE composite_model_routes ADD COLUMN IF NOT EXISTS fallback_targets jsonb NOT NULL DEFAULT '[]'::jsonb")
	require.NotContains(t, strings.ToUpper(sql), "UPDATE ")
	require.NotContains(t, strings.ToUpper(sql), "DELETE ")
}
