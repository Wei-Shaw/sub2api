package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPromptAuditFallbackMigration(t *testing.T) {
	content, err := FS.ReadFile("247_add_group_prompt_audit_fallback.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS fallback_group_id_on_prompt_audit_block BIGINT REFERENCES groups(id) ON DELETE SET NULL")
	require.Contains(t, sql, "CREATE INDEX IF NOT EXISTS idx_groups_fallback_group_id_on_prompt_audit_block")
}
