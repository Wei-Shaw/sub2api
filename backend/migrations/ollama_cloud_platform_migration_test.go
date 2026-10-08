package migrations

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// ollamaCloudPlatformMigrationFile 是 Ollama Cloud 平台迁移：一次性重建
// user_platform_quotas / composite_model_routes / channel_monitors /
// channel_monitor_request_templates 四个 CHECK 约束。
const ollamaCloudPlatformMigrationFile = "242_ollama_cloud_platform.sql"

// dmlStatementPattern 匹配 DML 关键字；242 只允许 DDL 约束变更。
var dmlStatementPattern = regexp.MustCompile(`(?i)\b(insert|update|delete|merge|truncate|copy|upsert)\b`)

// stripSQLLineComments 去掉 "--" 行注释，只对可执行语句做结构校验。
func stripSQLLineComments(content string) string {
	var b strings.Builder
	for _, line := range strings.Split(content, "\n") {
		if idx := strings.Index(line, "--"); idx >= 0 {
			line = line[:idx]
		}
		_, _ = b.WriteString(line)
		_, _ = b.WriteString("\n")
	}
	return b.String()
}

// executableSQL 返回去掉注释并归一化空白后的 SQL。
func executableSQL(content []byte) string {
	return strings.Join(strings.Fields(stripSQLLineComments(string(content))), " ")
}

// TestOllamaCloudPlatformMigration 校验 242 一次性重建四个约束：
// 两张平台表为 241 的 11 项 ∪ {ollama_cloud}（12 项），两张监控表为 238 的 10 项
// ∪ {ollama_cloud}（11 项，typesafe 不是对话模型不进 provider），且只有 DDL、零 DML。
func TestOllamaCloudPlatformMigration(t *testing.T) {
	content, err := FS.ReadFile(ollamaCloudPlatformMigrationFile)
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "DROP CONSTRAINT IF EXISTS user_platform_quotas_platform_check")
	require.Contains(t, sql, "DROP CONSTRAINT IF EXISTS composite_model_routes_target_platform_check")
	require.Contains(t, sql, "channel_monitors_provider_check")
	require.Contains(t, sql, "channel_monitor_request_templates_provider_check")

	// 四个约束各自恰好重建一次，不牵连其它约束。
	require.Equal(t, 4, strings.Count(sql, "ADD CONSTRAINT"), "242 只应新增四个约束")
	require.Equal(t, 4, strings.Count(sql, "CHECK ("), "242 只应包含四个 CHECK")

	// 两张平台表：241 的平台集合 ∪ {ollama_cloud}，必须同时保留 typesafe 与 ollama_cloud。
	require.Contains(t, sql,
		"CHECK (platform IN ('anthropic', 'openai', 'gemini', 'antigravity', 'grok', 'kimi', 'zhipu', 'deepseek', 'minimax', 'opencode_go', 'typesafe', 'ollama_cloud'))")
	require.Contains(t, sql,
		"CHECK (target_platform IN ('anthropic', 'openai', 'gemini', 'antigravity', 'grok', 'kimi', 'zhipu', 'deepseek', 'minimax', 'opencode_go', 'typesafe', 'ollama_cloud'))")

	// 两张监控表：238 的 provider 集合 ∪ {ollama_cloud}，不含 typesafe。
	require.Equal(t, 2, strings.Count(sql,
		"CHECK (provider IN ('openai', 'anthropic', 'gemini', 'grok', 'antigravity', 'kimi', 'zhipu', 'deepseek', 'minimax', 'opencode_go', 'ollama_cloud'))"),
		"两张监控表应重建为同一个 provider 集合（不含 typesafe）")

	// 监控表的两段保留幂等守卫：约束已含 ollama_cloud 时跳过重建。
	require.Contains(t, sql, "position('ollama_cloud' IN monitor_constraint_def) = 0")
	require.Contains(t, sql, "position('ollama_cloud' IN template_constraint_def) = 0")

	// 约束必须是默认 validated（非 NOT VALID），且整个文件只有 DDL。
	require.NotContains(t, sql, "NOT VALID", "平台 CHECK 必须是 validated 约束")
	require.NotRegexp(t, dmlStatementPattern, executableSQL(content), "242 不得包含 DML 语句")
}
