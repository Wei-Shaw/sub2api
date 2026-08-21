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
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS fallback_group_id_on_prompt_audit_block BIGINT")
	require.Contains(t, sql, "CREATE OR REPLACE FUNCTION sub2api_group_prompt_audit_fallback_guard()")
	require.Contains(t, sql, "AFTER DELETE ON groups")

	index, err := FS.ReadFile("249_group_prompt_audit_fallback_index_notx.sql")
	require.NoError(t, err)
	require.Contains(t, string(index), "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_groups_fallback_group_id_on_prompt_audit_block")
}
