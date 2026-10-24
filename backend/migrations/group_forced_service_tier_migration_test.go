package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGroupForcedServiceTierMigration(t *testing.T) {
	content, err := FS.ReadFile("246_group_forced_service_tier.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS forced_service_tier TEXT NOT NULL DEFAULT ''")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS free_openai_ultrafast BOOLEAN NOT NULL DEFAULT FALSE")
	require.Contains(t, sql, "SET forced_service_tier = 'priority'")
	require.Contains(t, sql, "WHERE force_openai_fast")
}
