package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGroupCodexConfigDefaultModelMigration(t *testing.T) {
	content, err := FS.ReadFile("238_add_group_codex_config_default_model.sql")
	require.NoError(t, err)
	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "ALTER TABLE groups ADD COLUMN IF NOT EXISTS codex_config_default_model VARCHAR(200) NOT NULL DEFAULT ''")
	require.Contains(t, sql, "COMMENT ON COLUMN groups.codex_config_default_model")
}
