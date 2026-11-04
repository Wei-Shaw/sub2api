package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestContentModerationEngineMetaMigration(t *testing.T) {
	raw, err := FS.ReadFile("238b_content_moderation_engine_meta.sql")
	require.NoError(t, err)
	sql := strings.ToUpper(string(raw))
	require.Contains(t, sql, "ALTER TABLE CONTENT_MODERATION_LOGS ADD COLUMN IF NOT EXISTS ENGINE_META JSONB;")
	for _, disallowed := range []string{"NOT NULL", "DEFAULT", "CHECK (", "DROP COLUMN", "UPDATE CONTENT_MODERATION_LOGS"} {
		require.NotContains(t, sql, disallowed)
	}
}
