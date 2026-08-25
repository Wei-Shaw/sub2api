package migrations

import (
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// expectedChannelMonitorProviders 是渠道监控 provider 允许的集合，需与
// service.monitorProviders（internal/service/channel_monitor_validate.go）及
// ent/schema/channel_monitor.go、channel_monitor_request_template.go 的
// provider enum 同步。
// 此处硬编码而非 import service：migrations 是叶子包，被 repository 依赖，
// 反向 import 会成环。
var expectedChannelMonitorProviders = []string{
	"anthropic", "antigravity", "deepseek", "gemini", "grok", "kimi", "kiro", "openai", "zhipu",
}

// channelMonitorProviderConstraints 是承载 provider CHECK 的两个约束名。
var channelMonitorProviderConstraints = []string{
	"channel_monitors_provider_check",
	"channel_monitor_request_templates_provider_check",
}

// TestChannelMonitorKiroProviderMigration 校验 229 号迁移把 226 漏掉的 kiro
// 加进两张表的 provider CHECK 约束。
//
// 226 用 DROP + 重建的写法把 provider 扩到 8 平台时，重建的列表里没有本 fork
// 独有的 kiro，与 176 加 grok 时同型。约束缺 kiro 时管理端无法为 kiro 账号建
// 渠道监控。
//
// 幂等守卫的探测键必须是 'kiro' 而非 'kimi'：226 已把 kimi 写进约束定义，
// 沿用 kimi 作键会让本迁移在存量库上 no-op。
func TestChannelMonitorKiroProviderMigration(t *testing.T) {
	content, err := FS.ReadFile("257_channel_monitor_kiro_provider.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")

	for _, constraint := range channelMonitorProviderConstraints {
		require.Contains(t, sql, constraint)
	}
	require.Contains(t, sql,
		"CHECK (provider IN ('openai', 'anthropic', 'gemini', 'grok', 'antigravity', 'kiro', 'kimi', 'zhipu', 'deepseek'))")

	// 约束变更必须显式标记为经过兼容性审查，避免把 DROP/重建误当成普通可回滚 DDL。
	require.Equal(t, 4, strings.Count(sql, "sub2api-managed-update: reviewed-compatible"))
	require.Contains(t, sql, "ALTER TABLE ONLY channel_monitors DROP CONSTRAINT IF EXISTS channel_monitors_provider_check")
	require.Contains(t, sql, "ALTER TABLE ONLY channel_monitor_request_templates DROP CONSTRAINT IF EXISTS channel_monitor_request_templates_provider_check")
}

// TestChannelMonitorProviderCheckFinalStateCoversAllProviders 是防回归护栏：
// 对两个 provider 约束分别按文件名顺序（= 迁移执行顺序）取最后一个重建它的迁移，
// 断言其 provider 列表覆盖全部允许 provider。
//
// 226 的问题根因是 DROP + 重建时手抄列表漏了一项，而针对 226 自身的用例只断言
// 「新增的 antigravity/kimi/zhipu/deepseek 在列表里」，无法发现旧 provider 被删。
// 此用例校验终态，任何后续重建这两个约束却漏 provider 的迁移都会在这里失败。
func TestChannelMonitorProviderCheckFinalStateCoversAllProviders(t *testing.T) {
	entries, err := FS.ReadDir(".")
	require.NoError(t, err)

	var sqlFiles []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".sql") {
			sqlFiles = append(sqlFiles, e.Name())
		}
	}
	sort.Strings(sqlFiles) // 与 ApplyMigrations 的执行顺序一致

	checkRe := regexp.MustCompile(`CHECK \(provider IN \(([^)]*)\)\)`)

	for _, constraint := range channelMonitorProviderConstraints {
		var last string
		for _, name := range sqlFiles {
			body, readErr := FS.ReadFile(name)
			require.NoError(t, readErr)
			if strings.Contains(string(body), constraint) {
				last = name
			}
		}
		require.NotEmpty(t, last, "no migration touches %s", constraint)

		body, readErr := FS.ReadFile(last)
		require.NoError(t, readErr)
		normalized := strings.Join(strings.Fields(string(body)), " ")

		matches := checkRe.FindAllStringSubmatch(normalized, -1)
		require.NotEmpty(t, matches,
			"migration %s rebuilds %s but has no CHECK (provider IN (...))", last, constraint)

		// 取该文件里最后一处 CHECK (provider IN (...))，即约束终态。
		var got []string
		for _, raw := range strings.Split(matches[len(matches)-1][1], ",") {
			if v := strings.Trim(strings.TrimSpace(raw), "'"); v != "" {
				got = append(got, v)
			}
		}
		sort.Strings(got)

		require.Equal(t, expectedChannelMonitorProviders, got,
			"migration %s 定义的 provider 列表与 service.monitorProviders 不一致；"+
				"重建 %s 时必须列出全部允许 provider，漏项会让对应 provider 的监控无法创建",
			last, constraint)
	}
}
