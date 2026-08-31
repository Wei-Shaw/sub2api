package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCodexProfileSurfaceIdentityMigrationExpandsProfileKeys(t *testing.T) {
	payload, err := FS.ReadFile("233_codex_profile_surface_identity.sql")
	require.NoError(t, err)
	sql := strings.Join(strings.Fields(string(payload)), " ")
	for _, required := range []string{
		"ADD COLUMN IF NOT EXISTS canonical_surface VARCHAR(20)",
		"SET canonical_surface = profiles.canonical_surface",
		"ALTER COLUMN canonical_surface SET NOT NULL",
		"UNIQUE(account_id, api_key_id, os_class, canonical_surface)",
		"account_codex_device_bindings_account_id_api_key_id_os_clas_key",
		"account_codex_profile_os_epoch_key",
		"UNIQUE(account_id, os_class, canonical_surface, epoch)",
		"ON account_codex_profiles(account_id, os_class, canonical_surface, epoch)",
		"ON account_codex_device_bindings(api_key_id, os_class, canonical_surface)",
		"binding_scope = 'api_key_os_surface'",
		"'{binding_scope}'",
		"'\"api_key_os_surface\"'::jsonb",
	} {
		require.Contains(t, sql, required)
	}
	releaseOldScope := strings.Index(sql, "DROP CONSTRAINT IF EXISTS account_codex_identity_binding_scope_check")
	backfillNewScope := strings.Index(sql, "SET binding_scope = 'api_key_os_surface'")
	require.NotEqual(t, -1, releaseOldScope)
	require.Greater(t, backfillNewScope, releaseOldScope, "the old CHECK must be removed before scope backfill")
	require.NotContains(t, strings.ToLower(sql), "usage_logs")
}
