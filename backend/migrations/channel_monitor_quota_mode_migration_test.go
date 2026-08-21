package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChannelMonitorQuotaModeMigration(t *testing.T) {
	content, err := FS.ReadFile("244_channel_monitor_quota_mode.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")

	// provider CHECK 两张表扩到 8 平台。为避免在发布路径中使用 DO 块，
	// 约束逐条替换，并标记为经过兼容性审查。
	require.Contains(t, sql, "channel_monitors_provider_check")
	require.Contains(t, sql, "channel_monitor_request_templates_provider_check")
	require.Contains(t, sql, "CHECK (provider IN ('openai', 'anthropic', 'gemini', 'grok', 'antigravity', 'kimi', 'zhipu', 'deepseek'))")
	require.Contains(t, sql, "DROP CONSTRAINT IF EXISTS channel_monitors_provider_check")
	require.Contains(t, sql, "DROP CONSTRAINT IF EXISTS channel_monitor_request_templates_provider_check")

	// check_mode 三态。旧写入由触发器补为 probe，避免非兼容的 NOT NULL DEFAULT。
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS check_mode VARCHAR(32)")
	require.Contains(t, sql, "UPDATE channel_monitors SET check_mode = 'probe' WHERE check_mode IS NULL")
	require.Contains(t, sql, "CREATE OR REPLACE FUNCTION sub2api_channel_monitor_write_guard()")
	require.Contains(t, sql, "NEW.check_mode := 'probe'")
	require.Contains(t, sql, "CHECK (check_mode IN ('probe', 'quota', 'quota_probe'))")

	// account_id 关联账号，账号删除置空（监控保留，运行时报「账号未关联」）。
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS account_id BIGINT")
	require.Contains(t, sql, "CREATE OR REPLACE FUNCTION sub2api_clear_channel_monitor_account_on_delete()")
	require.Contains(t, sql, "AFTER DELETE ON accounts")

	index, err := FS.ReadFile("248_channel_monitor_account_id_index_notx.sql")
	require.NoError(t, err)
	require.Contains(t, string(index), "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_channel_monitors_account_id")

	// 历史表配额快照列。
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS quota JSONB")

	// 公开设置默认关闭。
	require.Contains(t, sql, "VALUES ('channel_monitor_show_quota', 'false')")
	require.Contains(t, sql, "ON CONFLICT (key) DO NOTHING")
}
