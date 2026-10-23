package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGroupCapabilityAllowlistMigration(t *testing.T) {
	content, err := FS.ReadFile("246_group_capability_allowlist.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	// 幂等：重复执行不得报错，列缺省为 {}（白名单关闭）。
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS capability_allowlist JSONB NOT NULL DEFAULT '{}'::jsonb")
	require.Contains(t, sql, "COMMENT ON COLUMN groups.capability_allowlist")
}
