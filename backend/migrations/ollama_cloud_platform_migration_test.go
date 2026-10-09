package migrations

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// ollamaCloudMonitorProviderMigrationFile 把 ollama_cloud 加入渠道监控 provider 白名单。
// 平台白名单的两个 CHECK 已由 242 删除，改为应用层按平台清单校验。
const ollamaCloudMonitorProviderMigrationFile = "243_ollama_cloud_channel_monitor_provider.sql"

// dmlStatementPattern 匹配 DML 关键字；243 只允许 DDL 约束变更。
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

// TestOllamaCloudMonitorProviderMigration 校验 243 只重建两张监控表的 provider CHECK
// （238 的 10 项 ∪ {ollama_cloud}，不含 typesafe），不触碰 242 已删除的平台 CHECK，且零 DML。
func TestOllamaCloudMonitorProviderMigration(t *testing.T) {
	content, err := FS.ReadFile(ollamaCloudMonitorProviderMigrationFile)
	require.NoError(t, err)
	sql := executableSQL(content)

	require.Contains(t, sql, "channel_monitors_provider_check")
	require.Contains(t, sql, "channel_monitor_request_templates_provider_check")
	require.NotContains(t, sql, "user_platform_quotas")
	require.NotContains(t, sql, "composite_model_routes")

	require.Equal(t, 2, strings.Count(sql, "ADD CONSTRAINT"), "243 只应重建两个监控表约束")
	require.Equal(t, 2, strings.Count(sql,
		"CHECK (provider IN ('openai', 'anthropic', 'gemini', 'grok', 'antigravity', 'kimi', 'zhipu', 'deepseek', 'minimax', 'opencode_go', 'ollama_cloud'))"),
		"两张监控表应重建为同一个 provider 集合（不含 typesafe）")

	// 幂等守卫：约束已含 ollama_cloud 时跳过重建。
	require.Contains(t, sql, "position('ollama_cloud' IN monitor_constraint_def) = 0")
	require.Contains(t, sql, "position('ollama_cloud' IN template_constraint_def) = 0")

	require.NotContains(t, sql, "NOT VALID", "provider CHECK 必须是 validated 约束")
	require.NotRegexp(t, dmlStatementPattern, sql, "243 不得包含 DML 语句")
}
