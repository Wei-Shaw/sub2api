package migrations

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGroupCodexConfigReviewModelMigration(t *testing.T) {
	content, err := FS.ReadFile("239_add_group_codex_config_review_model.sql")
	require.NoError(t, err)
	sql := string(content)
	require.Contains(t, sql, "ALTER TABLE groups")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS codex_config_review_model VARCHAR(200) NOT NULL DEFAULT ''")
	require.Contains(t, sql, "COMMENT ON COLUMN groups.codex_config_review_model")
}
