package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChannelMonitorCodingPlanMigration(t *testing.T) {
	content, err := FS.ReadFile("194_channel_monitor_coding_plan_providers.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "channel_monitors_provider_check")
	require.Contains(t, sql, "channel_monitor_request_templates_provider_check")
	require.Contains(t, sql, "CHECK (provider IN ('openai', 'anthropic', 'gemini', 'grok', 'deepseek', 'glm', 'kimi'))")
	require.Contains(t, sql, "position('deepseek' IN monitor_constraint_def) = 0")
	require.Contains(t, sql, "position('deepseek' IN template_constraint_def) = 0")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS quota JSONB")
}
