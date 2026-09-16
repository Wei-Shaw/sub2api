package migrations

import (
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestForkPlatformConstraintsSupersetMigration(t *testing.T) {
	const name = "239_fork_platform_constraints_superset.sql"
	content, err := FS.ReadFile(name)
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql,
		"CHECK (platform IN ('anthropic', 'openai', 'gemini', 'antigravity', 'grok', 'kimi', 'zhipu', 'deepseek', 'kiro', 'minimax', 'adobe', 'opencode_go'))")
	require.Contains(t, sql,
		"CHECK (target_platform IN ('anthropic', 'openai', 'gemini', 'antigravity', 'grok', 'kimi', 'zhipu', 'deepseek', 'kiro', 'minimax', 'adobe', 'opencode_go'))")
	require.Contains(t, sql,
		"ADD CONSTRAINT channel_monitors_provider_check CHECK (provider IN ('openai', 'anthropic', 'gemini', 'grok', 'antigravity', 'kiro', 'kimi', 'zhipu', 'deepseek', 'minimax', 'opencode_go'))")
	require.Contains(t, sql,
		"ADD CONSTRAINT channel_monitor_request_templates_provider_check CHECK (provider IN ('openai', 'anthropic', 'gemini', 'grok', 'antigravity', 'kiro', 'kimi', 'zhipu', 'deepseek', 'minimax', 'opencode_go'))")

	// 必须是最后一个重建平台约束的迁移，否则乱序补跑的旧迁移会在它之后再次收窄约束。
	entries, err := FS.ReadDir(".")
	require.NoError(t, err)
	var touching []string
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		body, err := FS.ReadFile(e.Name())
		require.NoError(t, err)
		if strings.Contains(string(body), "ADD CONSTRAINT user_platform_quotas_platform_check") ||
			strings.Contains(string(body), "ADD CONSTRAINT composite_model_routes_target_platform_check") ||
			strings.Contains(string(body), "ADD CONSTRAINT channel_monitors_provider_check") {
			touching = append(touching, e.Name())
		}
	}
	sort.Strings(touching)
	require.Equal(t, name, touching[len(touching)-1])
}
