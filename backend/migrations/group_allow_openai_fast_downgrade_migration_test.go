package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGroupAllowOpenAIFastDowngradeMigration(t *testing.T) {
	content, err := FS.ReadFile("246_group_allow_openai_fast_downgrade.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql,
		"ADD COLUMN IF NOT EXISTS allow_openai_fast_downgrade BOOLEAN NOT NULL DEFAULT FALSE")
	require.Contains(t, sql, "COMMENT ON COLUMN groups.allow_openai_fast_downgrade")
}
