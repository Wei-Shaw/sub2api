package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCodexProxyRoutesMigrationBackfillsAndConstrainsExplicitRouting(t *testing.T) {
	payload, err := FS.ReadFile("230_codex_proxy_routes_and_cleanup.sql")
	require.NoError(t, err)
	sql := string(payload)
	for _, required := range []string{
		"ADD COLUMN IF NOT EXISTS proxy_mode",
		"WHEN proxy_id IS NOT NULL THEN 'proxy'",
		"ELSE 'inherit'",
		"proxy_mode IN ('inherit', 'proxy', 'direct')",
		"account_codex_profile_proxy_shape_check",
		"account_codex_slot_proxy_shape_check",
		"SET provisioning_state = 'pending'",
		"accounts_codex_identity_active_credentials_check",
		"WHEN LOWER(COALESCE(credentials->>'auth_mode', '')) = LOWER('agentIdentity')",
		"idx_account_codex_profiles_proxy",
		"idx_account_codex_slots_proxy",
		"idx_account_codex_bindings_slot",
	} {
		require.Truef(t, strings.Contains(sql, required), "migration must contain %q", required)
	}
}
