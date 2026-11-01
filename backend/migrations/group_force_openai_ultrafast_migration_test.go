package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGroupForceOpenAIUltrafastMigration(t *testing.T) {
	content, err := FS.ReadFile("250_group_force_openai_ultrafast.sql")
	require.NoError(t, err)
	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS force_openai_ultrafast BOOLEAN NOT NULL DEFAULT FALSE")
	require.Contains(t, sql, "COMMENT ON COLUMN groups.force_openai_ultrafast")
}
