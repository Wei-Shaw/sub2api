package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOllamaCloudPlatformMigration(t *testing.T) {
	content, err := FS.ReadFile("239_ollama_cloud_platform.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "user_platform_quotas_platform_check")
	require.Contains(t, sql, "composite_model_routes_target_platform_check")
	require.Contains(t, sql, "channel_monitors_provider_check")
	require.Contains(t, sql, "channel_monitor_request_templates_provider_check")
	require.Contains(t, sql, "'ollama_cloud'")
	require.Contains(t, sql, "'minimax'")
	require.Contains(t, sql, "'opencode_go'")
	require.Contains(t, sql, "position('ollama_cloud' IN monitor_constraint_def) = 0")
	require.Contains(t, sql, "position('ollama_cloud' IN template_constraint_def) = 0")
	require.Contains(t, sql,
		"CHECK (platform IN ('anthropic', 'openai', 'gemini', 'antigravity', 'grok', 'kimi', 'zhipu', 'deepseek', 'minimax', 'opencode_go', 'ollama_cloud'))")
	require.Contains(t, sql,
		"CHECK (target_platform IN ('anthropic', 'openai', 'gemini', 'antigravity', 'grok', 'kimi', 'zhipu', 'deepseek', 'minimax', 'opencode_go', 'ollama_cloud'))")
	require.Contains(t, sql,
		"CHECK (provider IN ('openai', 'anthropic', 'gemini', 'grok', 'antigravity', 'kimi', 'zhipu', 'deepseek', 'minimax', 'opencode_go', 'ollama_cloud'))")
}
