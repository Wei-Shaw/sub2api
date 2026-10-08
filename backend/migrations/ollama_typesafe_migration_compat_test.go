package migrations

import (
	"regexp"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
)

// 上游 typesafe 迁移（241）的文件名。
const typesafePlatformMigrationFile = "241_add_typesafe_platform.sql"

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

// TestOllamaCloudPlatformChecksSupersetOfTypesafe 校验 242 的两张平台表 CHECK 集合
// 恰为 241 的平台集合 ∪ {ollama_cloud}：既防止退回 241 覆写 ollama_cloud 的旧状态，
// 也防止 242 抄写平台列表时漏项或凭空多出平台。
//
// 这里断言的是两个迁移之间的集合关系而非重复一份字面量，因此 241 若被上游改动
// （理论上不可变，但仍可能被重编号或重写），失败信息能直接指出是基准变了还是
// 242 漂移了。
func TestOllamaCloudPlatformChecksSupersetOfTypesafe(t *testing.T) {
	content, err := FS.ReadFile(ollamaCloudPlatformMigrationFile)
	require.NoError(t, err)
	sql := executableSQL(content)

	typesafeContent, err := FS.ReadFile(typesafePlatformMigrationFile)
	require.NoError(t, err)
	typesafeSQL := executableSQL(typesafeContent)

	for _, column := range []string{"platform", "target_platform"} {
		base := platformCheckValues(t, typesafeSQL, column)
		require.NotContains(t, base, "ollama_cloud", "241 的基准平台集合不应含 ollama_cloud")

		expected := append(sortedValues(base), "ollama_cloud")
		sort.Strings(expected)

		actual := sortedValues(platformCheckValues(t, sql, column))
		require.Equal(t, expected, actual,
			"242 的 %s 集合必须等于 241 的平台并集加 ollama_cloud", column)
		require.Contains(t, actual, "typesafe")
		require.Contains(t, actual, "ollama_cloud")
	}
}
