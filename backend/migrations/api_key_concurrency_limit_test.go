package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAPIKeyConcurrencyLimitMigrationKeepsExistingKeysUnlimited(t *testing.T) {
	content, err := FS.ReadFile("182_add_api_key_concurrency_limit.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS concurrency_limit integer NOT NULL DEFAULT 0")
}
