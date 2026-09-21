package migrations

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// normalizeSQL 折叠空白，得到便于逐字断言的单行文本。
func normalizeSQL(sql string) string {
	return strings.Join(strings.Fields(sql), " ")
}

// stripSQLComments 去掉 `--` 行注释，只保留可执行语句。
// 240 的注释里会提到被刻意跳过的表名，语句级断言必须先剥注释。
func stripSQLComments(sql string) string {
	lines := strings.Split(sql, "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		if idx := strings.Index(line, "--"); idx >= 0 {
			line = line[:idx]
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		kept = append(kept, line)
	}
	return normalizeSQL(strings.Join(kept, "\n"))
}

// platformCheckList 抽取 `CHECK (platform IN (...))` 中的平台字面量集合。
// 用于断言 240 的新约束是 238 的严格超集，而不是逐字比对（顺序无关）。
func platformCheckList(t *testing.T, sql string) []string {
	t.Helper()
	re := regexp.MustCompile(`CHECK \(platform IN \(([^)]*)\)\)`)
	match := re.FindStringSubmatch(normalizeSQL(stripSQLComments(sql)))
	require.NotNil(t, match, "未找到 CHECK (platform IN (...)) 约束")
	parts := strings.Split(match[1], ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if value := strings.Trim(strings.TrimSpace(part), "'"); value != "" {
			out = append(out, value)
		}
	}
	return out
}

func TestTypeSafePlatformMigration(t *testing.T) {
	content, err := FS.ReadFile("240_typesafe_platform.sql")
	require.NoError(t, err)

	file := normalizeSQL(string(content))
	statements := stripSQLComments(string(content))

	require.Contains(t, statements, "user_platform_quotas_platform_check")
	require.Contains(t, statements, "'typesafe'")
	require.Contains(t, statements,
		"CHECK (platform IN ('anthropic', 'openai', 'gemini', 'antigravity', 'grok', 'kimi', 'zhipu', 'deepseek', 'minimax', 'opencode_go', 'ollama_cloud', 'typesafe'))")
	require.Contains(t, statements,
		"ALTER TABLE user_platform_quotas DROP CONSTRAINT IF EXISTS user_platform_quotas_platform_check")

	// 自研 runner 把文件中全部 SQL 当作语句执行，任何 goose 标记或 Down SQL 都会被真的执行。
	require.NotContains(t, file, "+goose")

	// 本阶段只放宽 user_platform_quotas：composite 与渠道监控两张表刻意不动。
	require.NotContains(t, statements, "composite_model_routes")
	require.NotContains(t, statements, "channel_monitors")
	require.NotContains(t, statements, "channel_monitor_request_templates")
	// 零 DML 回填。
	require.NotContains(t, strings.ToUpper(statements), "INSERT INTO")
	require.NotContains(t, strings.ToUpper(statements), "UPDATE ")
	require.NotContains(t, strings.ToUpper(statements), "DELETE FROM")
}

// 240 的新约束必须是 238 的严格超集：integration 会先应用 239（ollama_cloud），
// 但本分支（基于 main）只看到 238，两者的平台都必须被保留。
func TestTypeSafePlatformMigrationIsStrictSupersetOf238(t *testing.T) {
	prevContent, err := FS.ReadFile("238_opencode_go_platform.sql")
	require.NoError(t, err)
	nextContent, err := FS.ReadFile("240_typesafe_platform.sql")
	require.NoError(t, err)

	prev := platformCheckList(t, string(prevContent))
	next := platformCheckList(t, string(nextContent))

	nextSet := make(map[string]struct{}, len(next))
	for _, platform := range next {
		nextSet[platform] = struct{}{}
	}
	for _, platform := range prev {
		_, ok := nextSet[platform]
		require.True(t, ok, "238 的平台 %q 必须保留在 240 的新约束中", platform)
	}

	// 严格超集：至少新增 typesafe 与 ollama_cloud（239 在 integration 上先应用）。
	require.Contains(t, nextSet, "typesafe")
	require.Contains(t, nextSet, "ollama_cloud")
	require.NotContains(t, prev, "typesafe")
	require.Greater(t, len(next), len(prev))
}
