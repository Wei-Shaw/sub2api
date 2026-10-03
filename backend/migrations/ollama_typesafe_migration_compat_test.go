package migrations

import (
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// 912 兼容迁移与上游 typesafe 迁移（241）的文件名。
const (
	restoreOllamaCloudPlatformChecksFile = "912_restore_ollama_cloud_platform_checks.sql"
	typesafePlatformMigrationFile        = "241_add_typesafe_platform.sql"
)

// dmlStatementPattern 匹配 DML 关键字；912 只允许 DDL 约束变更。
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

// platformCheckValues 提取 "CHECK (<column> IN (...))" 中的平台字面量。
func platformCheckValues(t *testing.T, sql, column string) []string {
	t.Helper()

	re := regexp.MustCompile(`CHECK \(` + regexp.QuoteMeta(column) + ` IN \(([^)]*)\)\)`)
	match := re.FindStringSubmatch(sql)
	require.NotNil(t, match, "缺少 %s 的 CHECK 约束", column)

	quoted := regexp.MustCompile(`'([^']+)'`).FindAllStringSubmatch(match[1], -1)
	require.NotEmpty(t, quoted, "%s 的平台列表为空", column)

	values := make([]string, 0, len(quoted))
	for _, q := range quoted {
		values = append(values, q[1])
	}
	return values
}

func sortedValues(values []string) []string {
	out := append([]string(nil), values...)
	sort.Strings(out)
	return out
}

// TestRestoreOllamaCloudPlatformChecksMigration 校验 912：
// 只重建两个平台 CHECK、集合等于 241 的平台并集加 ollama_cloud、无 DML、
// 且约束为默认 validated（非 NOT VALID）。
func TestRestoreOllamaCloudPlatformChecksMigration(t *testing.T) {
	content, err := FS.ReadFile(restoreOllamaCloudPlatformChecksFile)
	require.NoError(t, err)

	sql := executableSQL(content)

	// 只处理两个平台 CHECK 约束，不触碰其它约束或表。
	require.Contains(t, sql, "DROP CONSTRAINT IF EXISTS user_platform_quotas_platform_check")
	require.Contains(t, sql, "DROP CONSTRAINT IF EXISTS composite_model_routes_target_platform_check")
	require.Contains(t, sql, "ADD CONSTRAINT user_platform_quotas_platform_check")
	require.Contains(t, sql, "ADD CONSTRAINT composite_model_routes_target_platform_check")
	require.Equal(t, 2, strings.Count(sql, "DROP CONSTRAINT IF EXISTS"), "912 只应移除两个约束")
	require.Equal(t, 2, strings.Count(sql, "ADD CONSTRAINT"), "912 只应新增两个约束")
	require.Equal(t, 2, strings.Count(sql, "CHECK ("), "912 只应包含两个 CHECK")
	require.NotContains(t, sql, "channel_monitors")
	require.NotContains(t, sql, "channel_monitor_request_templates")
	require.NotContains(t, sql, "NOT VALID", "平台 CHECK 必须是 validated 约束")

	// 无任何 DML 语句。
	require.NotRegexp(t, dmlStatementPattern, sql, "912 不得包含 DML 语句")

	typesafeContent, err := FS.ReadFile(typesafePlatformMigrationFile)
	require.NoError(t, err)
	typesafeSQL := executableSQL(typesafeContent)

	// 集合 = 241 的平台集合 ∪ {ollama_cloud}，不得漏类型或多类型。
	for _, column := range []string{"platform", "target_platform"} {
		base := platformCheckValues(t, typesafeSQL, column)

		expected := append(sortedValues(base), "ollama_cloud")
		sort.Strings(expected)
		require.Equal(t, len(base)+1, len(expected), "%s 的基准平台集合不应含 ollama_cloud", column)

		actual := sortedValues(platformCheckValues(t, sql, column))
		require.Equal(t, expected, actual,
			"912 的 %s 集合必须等于 241 的平台并集加 ollama_cloud", column)
		require.Contains(t, actual, "ollama_cloud")
		require.Contains(t, actual, "typesafe")
	}
}
