//go:build unit

package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWarpProxyOwnershipMigrationUsesExplicitLegacyEvidence(t *testing.T) {
	content, err := FS.ReadFile("194_warp_proxy_ownership.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS managed_by")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS external_id")
	require.Contains(t, sql, "p.group_id = pg.id")
	require.Contains(t, sql, "p.name LIKE 'warp-%'")
	require.Contains(t, sql, "pg.description = 'Cloudflare WARP proxy pool (auto-managed by warp-gateway sync)'")
	require.Contains(t, sql, "WHERE deleted_at IS NULL AND managed_by IS NOT NULL AND external_id IS NOT NULL")
}
