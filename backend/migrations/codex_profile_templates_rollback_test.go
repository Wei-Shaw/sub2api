package migrations

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCodexProfileTemplatesRollbackRestoresOldBinaryContract(t *testing.T) {
	payload, err := os.ReadFile(filepath.Join("..", "..", "deploy", "rollback-codex-profile-templates-v1.sql"))
	require.NoError(t, err)
	sql := strings.Join(strings.Fields(string(payload)), " ")
	for _, required := range []string{
		"DELETE FROM account_codex_device_bindings",
		"DELETE FROM account_codex_device_slots",
		"DELETE FROM account_codex_profiles",
		"binding_scope='api_key_os'",
		"UNIQUE(account_id, api_key_id, os_class)",
		"UNIQUE(account_id, os_class, epoch)",
		"DROP COLUMN IF EXISTS canonical_surface",
		"information_schema.columns",
		"SET codex_identity_template_id=NULL, codex_identity_template_applied_revision=NULL",
		"DROP CONSTRAINT IF EXISTS account_codex_device_bindings_account_id_api_key_id_os_class_key",
		"DROP CONSTRAINT IF EXISTS account_codex_profiles_account_id_os_class_epoch_key",
		"DROP TABLE IF EXISTS codex_identity_templates",
		"DROP COLUMN IF EXISTS client_version_mode",
		"DROP COLUMN IF EXISTS client_version",
		"DELETE FROM schema_migrations",
		"236_codex_profile_surface_identity.sql",
		"237_codex_identity_templates.sql",
		"238_codex_slot_client_versions.sql",
	} {
		require.Contains(t, sql, required)
	}
	dropScopeCheck := strings.Index(sql, "DROP CONSTRAINT IF EXISTS account_codex_identity_binding_scope_check")
	writeOldScope := strings.Index(sql, "binding_scope='api_key_os'")
	require.NotEqual(t, -1, dropScopeCheck)
	require.Greater(t, writeOldScope, dropScopeCheck, "the new CHECK must be removed before restoring the old scope")
	require.NotContains(t, strings.ToLower(sql), "usage_logs")
}
