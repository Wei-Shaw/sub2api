package migrations

import (
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// expectedUserPlatformQuotaPlatforms 是 user_platform_quotas.platform 允许的平台集合，
// 需与 service.AllowedQuotaPlatforms 及 ent/schema/user_platform_quota.go 的 Validate 同步。
// 此处硬编码而非 import service：migrations 是叶子包，被 repository 依赖，反向 import 会成环。
var expectedUserPlatformQuotaPlatforms = []string{
	"anthropic", "antigravity", "deepseek", "gemini", "grok", "kimi", "kiro", "openai", "zhipu",
}

// TestUserPlatformQuotasRestoreKiroMigration 校验 227 号迁移把 224 漏掉的 kiro
// 重新加回 user_platform_quotas.platform 的 CHECK 约束。
//
// kiro 自 145 号迁移起就在约束内，224 号用 DROP + 重建的写法加国产 3 家时漏掉了它，
// 等于回退了 145。约束缺 kiro 时，注册预填充 9 平台默认配额会因该行违约中止整条
// INSERT → 快照 fail-open 仅 warn → 新用户零配额行（缺失配额行 = 无限额）。
func TestUserPlatformQuotasRestoreKiroMigration(t *testing.T) {
	content, err := FS.ReadFile("256_user_platform_quotas_restore_kiro.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "DROP CONSTRAINT IF EXISTS user_platform_quotas_platform_check")
	require.Contains(t, sql,
		"CHECK (platform IN ('anthropic', 'openai', 'gemini', 'antigravity', 'kiro', 'grok', 'kimi', 'zhipu', 'deepseek'))")
}

// TestUserPlatformQuotaPlatformCheckFinalStateCoversAllPlatforms 是防回归护栏：
// 按文件名顺序（= 迁移执行顺序）取最后一个重建 user_platform_quotas_platform_check
// 的迁移，断言其平台列表覆盖全部允许平台。
//
// 224 的事故根因是 DROP + 重建时手抄平台列表漏了一项，而针对 224 自身的用例只断言
// 「新增的 kimi/zhipu/deepseek 在列表里」，无法发现旧平台被删。此用例校验终态，
// 任何后续重建该约束却漏平台的迁移都会在这里失败。
func TestUserPlatformQuotaPlatformCheckFinalStateCoversAllPlatforms(t *testing.T) {
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
		if strings.Contains(string(body), "user_platform_quotas_platform_check") {
			touching = append(touching, name)
		}
	}
	require.NotEmpty(t, touching, "no migration touches user_platform_quotas_platform_check")
	sort.Strings(touching) // 与 ApplyMigrations 的执行顺序一致
	last := touching[len(touching)-1]

	body, err := FS.ReadFile(last)
	require.NoError(t, err)
	normalized := strings.Join(strings.Fields(string(body)), " ")

	// 取该文件里最后一处 CHECK (platform IN (...))，即约束终态。
	checkRe := regexp.MustCompile(`CHECK \(platform IN \(([^)]*)\)\)`)
	matches := checkRe.FindAllStringSubmatch(normalized, -1)
	require.NotEmpty(t, matches, "migration %s rebuilds the constraint but has no CHECK (platform IN (...))", last)

	var got []string
	for _, raw := range strings.Split(matches[len(matches)-1][1], ",") {
		if v := strings.Trim(strings.TrimSpace(raw), "'"); v != "" {
			got = append(got, v)
		}
	}
	sort.Strings(got)

	require.Equal(t, expectedUserPlatformQuotaPlatforms, got,
		"migration %s 定义的平台列表与 service.AllowedQuotaPlatforms 不一致；"+
			"重建 CHECK 约束时必须列出全部允许平台，漏项会让对应平台的配额行插入失败", last)
}
