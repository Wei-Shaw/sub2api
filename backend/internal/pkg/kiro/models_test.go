package kiro

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMapModelKnownAliases(t *testing.T) {
	t.Parallel()

	mapped, ok := MapModel("claude-sonnet-4-6")
	require.True(t, ok)
	require.Equal(t, "claude-sonnet-4.6", mapped)

	mapped, ok = MapModel("claude-sonnet-4-5")
	require.True(t, ok)
	require.Equal(t, "claude-sonnet-4.5", mapped)

	mapped, ok = MapModel("claude-haiku-4-5")
	require.True(t, ok)
	require.Equal(t, "claude-haiku-4.5", mapped)
}

func TestMapModelStripsDateSuffix(t *testing.T) {
	t.Parallel()

	mapped, ok := MapModel("claude-sonnet-4-5-20250929")
	require.True(t, ok)
	require.Equal(t, "claude-sonnet-4.5", mapped)

	mapped, ok = MapModel("claude-haiku-4-5-20251001")
	require.True(t, ok)
	require.Equal(t, "claude-haiku-4.5", mapped)
}

func TestMapModelPassesThroughKiroNativeNames(t *testing.T) {
	t.Parallel()

	// 已经是 Kiro 形态（点号版本号）的直接透传，ok=true——
	// 这样上游新增型号无需改代码，是否真的存在交给上游判定。
	mapped, ok := MapModel("claude-sonnet-4.6")
	require.True(t, ok)
	require.Equal(t, "claude-sonnet-4.6", mapped)

	mapped, ok = MapModel("claude-sonnet-9.9")
	require.True(t, ok)
	require.Equal(t, "claude-sonnet-9.9", mapped)

	mapped, ok = MapModel("auto")
	require.True(t, ok)
	require.Equal(t, "auto", mapped)
}

// TestMapModelUnknownIsRejected 是真实账号测试发现的回归：不属于 Kiro 的
// 模型名（既不是 Kiro 原生点号形态，也不在别名表里）必须返回 ok=false，
// 调用方据此拒绝请求——不能像之前那样静默换成 defaultKiroModel 再假装
// 请求成功了。这正是"管理端测试连接不管选什么模型都显示完成"这个用户
// 报告的症状的根因：MapModel 从不报告"这个模型我不认识"。设计参照
// AntigravityGatewayService 的既有约定（一份准确的白名单，命中就映射、
// 未命中就干净拒绝），不是转发给上游让它兜底。
func TestMapModelUnknownIsRejected(t *testing.T) {
	t.Parallel()

	_, ok := MapModel("gpt-4o")
	require.False(t, ok)

	_, ok = MapModel("")
	require.False(t, ok)

	// claude-fable-5/claude-fable-5-1：真实账号测试证实 Kiro 对
	// "claude-fable-5" 直接返回 400 INVALID_MODEL_ID
	// （"Invalid model. Please select a different model to continue."），
	// 2026-09-06 用账号自己的权威接口 ListAvailableModels 复核过，这两个
	// 型号确实不在账号的真实模型清单里，维持拒绝——见
	// TestMapModelOpusFamilyRequiresRealVerification 的完整历史说明。
	_, ok = MapModel("claude-fable-5")
	require.False(t, ok)
	_, ok = MapModel("claude-fable-5-1")
	require.False(t, ok)
}

func TestMapModelIsCaseInsensitive(t *testing.T) {
	t.Parallel()

	mapped, ok := MapModel("Claude-Sonnet-4-6")
	require.True(t, ok)
	require.Equal(t, "claude-sonnet-4.6", mapped)
}

// TestMapModelOpusFamilyRequiresRealVerification 记录 claude-opus 家族
// 别名表内容的完整历史，防止未来重蹈同一个坑：
//
//  1. I5（整分支最终评审）判定"Kiro 没有 Opus 型号"并删除了错误的
//     opus-4-5/opus-4-6→sonnet-4.6 别名。方向是对的（不能悄悄把 Opus
//     请求换成 Sonnet 输出），但结论错了。
//  2. 用户真实账号测试 claude-opus-5 时被误拒——核实一个第三方 Kiro
//     代理实现的 map_model 表，发现 Kiro 其实支持 opus-5，遂据此一次性
//     加入 opus-4.5/4.6/4.7/4.8/5 整个家族 + sonnet-5 + fable-5/fable-5-1。
//  3. 真实账号测试证实这批加入的别名并不都对：Kiro 对
//     "claude-fable-5" 直接返回 400 INVALID_MODEL_ID
//     （"Invalid model. Please select a different model to continue."）。
//     既然同一个参考来源在 fable 上出过错，其余未单独验证的条目
//     （opus-4.5/4.6/4.7/4.8、sonnet-5）同样不可信，即使 claude-opus-5
//     恰好是对的——"这个来源猜对过一次"不构成"这个来源整体可靠"的证据。
//     全部移除，只保留 claude-opus-5 这一条（有独立的真实账号测试证据，
//     不依赖那个参考实现）。
//  4. 2026-09-06：拿到了真实账号的 profileArn 后，直接调用 Kiro 自己的权威
//     接口 ListAvailableModels（不是第三方参考实现，也不是"测试连接猜一
//     个模型名"），拉到了账号完整的真实模型清单，逐条核实后发现
//     opus-4.5/4.6/4.7/4.8、sonnet-5 这次真的在清单里——跟第 3 步移除的
//     不是同一份证据，不能因为上次那份第三方参考实现整体不可靠就继续
//     怀疑这批新证据。fable-5/fable-5-1 复核后依然不在清单里，与第 3 步
//     的真实测试结论一致，继续保持拒绝。
//
// 结论：白名单机制本身没问题，问题是内容来源的可靠性。新增条目前必须用
// 账号自己的权威接口（ListAvailableModels）或真实账号测试连接单独验证
// 每一条（点号原生形态可以不经别名表直接透传，见 MapModel 规则 2，天然
// 适合拿来试探候选模型名），不能整批照抄第三方参考实现。
func TestMapModelOpusFamilyRequiresRealVerification(t *testing.T) {
	t.Parallel()

	// 2026-09-06 更新：以下条目均已用真实账号的权威接口 ListAvailableModels
	// 核实在案，不再是"未经验证需要拒绝"的状态。
	for requested, want := range map[string]string{
		"claude-opus-5":   "claude-opus-5",
		"claude-sonnet-5": "claude-sonnet-5",
		"claude-opus-4-5": "claude-opus-4.5",
		"claude-opus-4-6": "claude-opus-4.6",
		"claude-opus-4-7": "claude-opus-4.7",
		"claude-opus-4-8": "claude-opus-4.8",
	} {
		mapped, ok := MapModel(requested)
		require.True(t, ok, "%s 已经过 ListAvailableModels 核实，应该被接受", requested)
		require.Equal(t, want, mapped)
	}
}

func TestDefaultModelsNonEmptyAndContainsDefault(t *testing.T) {
	t.Parallel()

	models := DefaultModels()
	require.NotEmpty(t, models)
	require.Contains(t, models, defaultKiroModel)
}

// TestDefaultModelsStaysInSyncWithAliasTargets 是防漂移守卫：每次给
// kiroModelAliases 加新别名，DefaultModels() 必须同步收录其映射目标，
// 否则 /v1/models 与管理端候选列表会缺新模型（I4 的场景）。
func TestDefaultModelsStaysInSyncWithAliasTargets(t *testing.T) {
	t.Parallel()

	defaults := DefaultModels()
	defaultSet := make(map[string]bool, len(defaults))
	for _, m := range defaults {
		defaultSet[m] = true
	}

	seen := make(map[string]bool)
	for _, target := range kiroModelAliases {
		if seen[target] {
			continue
		}
		seen[target] = true
		require.True(t, defaultSet[target],
			"kiroModelAliases 的映射目标 %q 必须出现在 DefaultModels() 里", target)
	}
}
