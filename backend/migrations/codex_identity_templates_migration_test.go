package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCodexIdentityTemplatesMigrationDefinesReusableControlPlane(t *testing.T) {
	payload, err := FS.ReadFile("237_codex_identity_templates.sql")
	require.NoError(t, err)
	sql := strings.Join(strings.Fields(string(payload)), " ")
	for _, required := range []string{
		"codex_identity_templates",
		"codex_identity_template_profiles",
		"codex_identity_template_slots",
		"UNIQUE(template_id, os_class, canonical_surface)",
		"UNIQUE(profile_id, slot_index)",
		"idx_codex_identity_templates_name_ci",
		"codex_identity_template_id",
		"codex_identity_template_applied_revision",
		"accounts_codex_identity_template_fk",
		"ON DELETE RESTRICT",
		"idx_accounts_codex_identity_template",
		"迁移账号",
		"jsonb_array_elements",
		"codex_identity_template_applied_revision=templates.revision",
	} {
		require.Contains(t, sql, required)
	}
	require.NotContains(t, strings.ToLower(sql), "usage_logs")
}
