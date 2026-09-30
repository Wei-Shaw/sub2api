package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAPIKeyPlatformLimitsMigration(t *testing.T) {
	content, err := FS.ReadFile("241_api_key_platform_limits.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")

	// 限额配置随 api_keys 行读出（热路径零额外查询），必须是可空的加法迁移。
	require.Contains(t, sql, "ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS platform_limits JSONB")

	// 用量表：唯一键 + 与 api_keys 同构的窗口列。
	require.Contains(t, sql, "CREATE TABLE IF NOT EXISTS api_key_platform_usages")
	require.Contains(t, sql, "REFERENCES api_keys(id) ON DELETE CASCADE")
	require.Contains(t, sql,
		"CHECK (platform IN ( 'anthropic', 'openai', 'gemini', 'antigravity', 'grok', 'kimi', 'zhipu', 'deepseek', 'minimax', 'opencode_go' ))")
	for _, col := range []string{"quota_used", "usage_5h", "usage_1d", "usage_7d", "window_5h_start", "window_1d_start", "window_7d_start"} {
		require.Contains(t, sql, col)
	}

	// 索引名必须与 ent 生成的一致，避免 auto-migrate 重复建索引。
	require.Contains(t, sql,
		"CREATE UNIQUE INDEX IF NOT EXISTS apikeyplatformusage_api_key_id_platform ON api_key_platform_usages (api_key_id, platform)")
	require.Contains(t, sql,
		"CREATE INDEX IF NOT EXISTS apikeyplatformusage_api_key_id ON api_key_platform_usages (api_key_id)")
}
