package migrations

import (
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// expectedCompositeRouteTargetPlatforms 是 composite_model_routes.target_platform
// 允许的平台集合，需与 service.isConcreteRequestPlatform
// （internal/service/composite_platform.go）同步。
// 此处硬编码而非 import service：migrations 是叶子包，被 repository 依赖，
// 反向 import 会成环。
var expectedCompositeRouteTargetPlatforms = []string{
	"anthropic", "antigravity", "deepseek", "gemini", "grok", "kimi", "kiro", "openai", "zhipu",
}

const compositeRouteTargetPlatformConstraint = "composite_model_routes_target_platform_check"

// TestCompositeRoutesKiroMigration 校验 229 号迁移把 227 漏掉的 kiro 加进
// composite_model_routes.target_platform 的 CHECK 约束。
//
// 227 用 DROP + 重建的写法把国产 3 家加入该约束时，重建的列表里没有本 fork
// 独有的 kiro，导致管理端保存 target_platform='kiro' 的 composite 路由违约失败。
func TestCompositeRoutesKiroMigration(t *testing.T) {
	content, err := FS.ReadFile("258_composite_routes_add_kiro.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "DROP CONSTRAINT IF EXISTS "+compositeRouteTargetPlatformConstraint)
	require.Contains(t, sql,
		"CHECK (target_platform IN ('anthropic', 'openai', 'gemini', 'antigravity', 'kiro', 'grok', 'kimi', 'zhipu', 'deepseek'))")
}

// TestCompositeRouteTargetPlatformFinalStateCoversAllPlatforms 是防回归护栏：
// 按文件名顺序（= 迁移执行顺序）取最后一个重建该约束的迁移，断言其平台列表
// 覆盖全部允许平台。
//
// 227 的问题根因是 DROP + 重建时手抄平台列表漏了 kiro，而针对 227 自身的用例
// 只断言完整字符串匹配当时的 8 平台，无法表达「必须覆盖全集」。此用例校验终态，
// 任何后续重建该约束却漏平台的迁移都会在这里失败。
func TestCompositeRouteTargetPlatformFinalStateCoversAllPlatforms(t *testing.T) {
	entries, err := FS.ReadDir(".")
	require.NoError(t, err)

	var touching []string
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".sql") {
			continue
		}
		body, readErr := FS.ReadFile(name)
		require.NoError(t, readErr)
		if strings.Contains(string(body), compositeRouteTargetPlatformConstraint) {
			touching = append(touching, name)
		}
	}
	require.NotEmpty(t, touching, "no migration touches %s", compositeRouteTargetPlatformConstraint)
	sort.Strings(touching) // 与 ApplyMigrations 的执行顺序一致
	last := touching[len(touching)-1]

	body, err := FS.ReadFile(last)
	require.NoError(t, err)
	normalized := strings.Join(strings.Fields(string(body)), " ")

	// 取该文件里最后一处 CHECK (target_platform IN (...))，即约束终态。
	checkRe := regexp.MustCompile(`CHECK \(target_platform IN \(([^)]*)\)\)`)
	matches := checkRe.FindAllStringSubmatch(normalized, -1)
	require.NotEmpty(t, matches,
		"migration %s rebuilds the constraint but has no CHECK (target_platform IN (...))", last)

	var got []string
	for _, raw := range strings.Split(matches[len(matches)-1][1], ",") {
		if v := strings.Trim(strings.TrimSpace(raw), "'"); v != "" {
			got = append(got, v)
		}
	}
	sort.Strings(got)

	require.Equal(t, expectedCompositeRouteTargetPlatforms, got,
		"migration %s 定义的平台列表与 service.isConcreteRequestPlatform 不一致；"+
			"重建 CHECK 约束时必须列出全部允许平台，漏项会让对应平台的 composite 路由无法保存", last)
}
