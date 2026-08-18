package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChannelMonitorQuotaModeMigration(t *testing.T) {
	content, err := FS.ReadFile("226_channel_monitor_quota_mode.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")

	// provider CHECK 两张表扩到 8 平台，且带幂等守卫（仿 176 grok 迁移）。
	require.Contains(t, sql, "channel_monitors_provider_check")
	require.Contains(t, sql, "channel_monitor_request_templates_provider_check")
	require.Contains(t, sql, "CHECK (provider IN ('openai', 'anthropic', 'gemini', 'grok', 'antigravity', 'kimi', 'zhipu', 'deepseek'))")
	require.Contains(t, sql, "position('kimi' IN monitor_constraint_def) = 0")
	require.Contains(t, sql, "position('kimi' IN template_constraint_def) = 0")

	// check_mode 三态，默认 probe。
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS check_mode VARCHAR(32) NOT NULL DEFAULT 'probe'")
	require.Contains(t, sql, "CHECK (check_mode IN ('probe', 'quota', 'quota_probe'))")

	// account_id 关联账号，账号删除置空（监控保留，运行时报「账号未关联」）。
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS account_id BIGINT REFERENCES accounts(id) ON DELETE SET NULL")
	require.Contains(t, sql, "CREATE INDEX IF NOT EXISTS idx_channel_monitors_account_id ON channel_monitors(account_id)")

	// 历史表配额快照列。
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS quota JSONB")

	// 公开设置默认关闭。
	require.Contains(t, sql, "VALUES ('channel_monitor_show_quota', 'false')")
	require.Contains(t, sql, "ON CONFLICT (key) DO NOTHING")
}
