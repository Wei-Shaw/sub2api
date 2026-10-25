package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChannelMonitorStepFunProviderMigration(t *testing.T) {
	content, err := FS.ReadFile("246_channel_monitor_stepfun_provider.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "channel_monitors_provider_check")
	require.Contains(t, sql, "channel_monitor_request_templates_provider_check")
	require.Contains(t, sql, "position('stepfun' IN monitor_constraint_def) = 0")
	require.Contains(t, sql, "position('stepfun' IN template_constraint_def) = 0")
	require.Contains(t, sql,
		"CHECK (provider IN ('openai', 'anthropic', 'gemini', 'grok', 'antigravity', 'kimi', 'zhipu', 'deepseek', 'minimax', 'opencode_go', 'stepfun'))")
}
