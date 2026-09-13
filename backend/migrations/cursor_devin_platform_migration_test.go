package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCursorDevinPlatformMigration(t *testing.T) {
	content, err := FS.ReadFile("239_add_cursor_devin_platforms.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "user_platform_quotas_platform_check")
	require.Contains(t, sql, "composite_model_routes_target_platform_check")
	require.Contains(t, sql, "channel_monitors_provider_check")
	require.Contains(t, sql, "channel_monitor_request_templates_provider_check")
	require.Contains(t, sql, "'cursor'")
	require.Contains(t, sql, "'devin'")
	require.Contains(t, sql, "'opencode_go'")
	require.Contains(t, sql, "position('cursor' IN monitor_constraint_def) = 0")
	require.Contains(t, sql, "position('cursor' IN template_constraint_def) = 0")
	require.Contains(t, sql,
		"CHECK (platform IN ('anthropic', 'openai', 'gemini', 'antigravity', 'grok', 'kimi', 'zhipu', 'deepseek', 'minimax', 'opencode_go', 'cursor', 'devin'))")
	require.Contains(t, sql,
		"CHECK (target_platform IN ('anthropic', 'openai', 'gemini', 'antigravity', 'grok', 'kimi', 'zhipu', 'deepseek', 'minimax', 'opencode_go', 'cursor', 'devin'))")
	require.Contains(t, sql,
		"CHECK (provider IN ('openai', 'anthropic', 'gemini', 'grok', 'antigravity', 'kimi', 'zhipu', 'deepseek', 'minimax', 'opencode_go', 'cursor', 'devin'))")
}
