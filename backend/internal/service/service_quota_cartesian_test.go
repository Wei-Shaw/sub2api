//go:build unit

package service

import (
	"context"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestPreCheckAcquire_CartesianCounterKeys 验证 N path × M limiter 笛卡尔积下，
// 每个 (path, limiter) 组合在 PreCheckAcquire 阶段都生成独立的 counter key。
//
// 关键性：counter key 公式是 svcquota:v2:<rule>:<path>:<type>:<scope>，
// 同一规则下不同 path 的 RPM 计数器必须独立计数。如果实现误把 path_id 漏在 key 里
// （或同 limiter_type 在不同 path 复用同一 key），高 path 数规则会出现"互相污染"——
// path A 的请求把 path B 的窗口耗光。这条测试是这个不变量的回归防线。
//
// 用 RPM CountOnArrival=true 因为它在 PreCheckAcquire 阶段会调 Increment（写入 counter key），
// 让 fakeLimiter.incrementCalls 直接记录所有触达的 key。
func TestPreCheckAcquire_CartesianCounterKeys(t *testing.T) {
	t.Parallel()
	limiter := newFakeLimiter()

	// 1 条规则 × 2 path × 3 RPM limiter (实际 RPM 同 type 不能 dup，所以这里改成 3 种不同 type
	// 但都让它们在 arrival 阶段写计数：RPM-CountOnArrival + 另两个 RPM 不可行 → 用同 type 只能 1 个)。
	// 因此用 1 path 上 3 type（rpm/tpm/tpd）→ 不行 tpm/tpd 走 deferred 不写。
	// 改成 2 path × 1 RPM（CountOnArrival=true）展示笛卡尔展开：2 path → 2 个独立 key。
	// 再加 concurrency 验证不同 limiter type 也独立 key。
	platformA := "anthropic"
	platformB := "openai"
	rule := &ServiceQuotaRule{
		ID:          1,
		Enabled:     true,
		CounterMode: ServiceQuotaCounterModeShared,
		Limiters: []ServiceQuotaLimiterDef{
			{ID: 11, RuleID: 1, LimiterType: ServiceQuotaLimiterRPM, WindowMode: ServiceQuotaWindowFixed, LimitValue: 100, CountOnArrival: true},
			{ID: 12, RuleID: 1, LimiterType: ServiceQuotaLimiterConcurrency, WindowMode: ServiceQuotaWindowFixed, LimitValue: 10},
		},
		Paths: []ServiceQuotaPathDef{
			{ID: 100, RuleID: 1, Platform: &platformA},
			{ID: 101, RuleID: 1, Platform: &platformB},
		},
	}
	svc := newQuotaServiceForTest(t, []*ServiceQuotaRule{rule}, nil, limiter)

	// 命中 path A（platform=anthropic）：应只触发 path 100 上的 2 个 limiter，写入 1 个 RPM key + 1 个 concurrency key
	plan, err := svc.PreCheckSelect(context.Background(), ServiceQuotaCheckRequest{UserID: 42, Platform: platformA})
	require.NoError(t, err)
	lease, err := svc.PreCheckAcquire(context.Background(), plan, 0, 0)
	require.NoError(t, err)
	require.NotNil(t, lease)

	// 命中 path B（platform=openai）：应触发 path 101 的 RPM + concurrency
	plan2, err := svc.PreCheckSelect(context.Background(), ServiceQuotaCheckRequest{UserID: 42, Platform: platformB})
	require.NoError(t, err)
	lease2, err := svc.PreCheckAcquire(context.Background(), plan2, 0, 0)
	require.NoError(t, err)
	require.NotNil(t, lease2)

	// 总计 RPM Increment 应该 2 次（path 100 一次 + path 101 一次），key 不同。
	require.Len(t, limiter.incrementCalls, 2)
	rpmKeys := append([]string(nil), limiter.incrementCalls...)
	sort.Strings(rpmKeys)
	require.Equal(t, []string{
		"svcquota:v2:1:100:rpm:shared",
		"svcquota:v2:1:101:rpm:shared",
	}, rpmKeys, "不同 path 的 RPM 必须使用独立 counter key（path_id 必须出现在 key 里）")

	// concurrency Acquire 也应有 2 次（path 100 / 101 各一次），key 不同。
	require.Len(t, limiter.acquireCalls, 2)
	concKeys := append([]string(nil), limiter.acquireCalls...)
	sort.Strings(concKeys)
	require.Equal(t, []string{
		"svcquota:v2:1:100:concurrency:shared",
		"svcquota:v2:1:101:concurrency:shared",
	}, concKeys)

	lease.Release()
	lease2.Release()
}

// TestApplyFallbackOverride_TwoFallbacks_PartialOverlap 覆盖原 logic_test 漏的场景：
// 2 条 fallback 各自带不同 limiter 集合，concrete 命中 limiter X：
//   - fallback A 含 X + Y  → X 被 concrete 抢，保留 Y
//   - fallback B 含 Y + Z  → 不与 concrete 重叠，保留 Y + Z 全部
//
// 这证实 fallback 让位是 limiter_type 粒度（不是规则粒度），且每条 fallback 独立判定，
// 不会因为另一条 fallback 也含 Y 就把 Y 也丢掉。
func TestApplyFallbackOverride_TwoFallbacks_PartialOverlap(t *testing.T) {
	t.Parallel()
	fallbackA := ruleWith(10, true, ServiceQuotaCounterModeShared, []string{
		ServiceQuotaLimiterRPM, ServiceQuotaLimiterTPM,
	}, nil)
	fallbackB := ruleWith(20, true, ServiceQuotaCounterModeShared, []string{
		ServiceQuotaLimiterTPM, ServiceQuotaLimiterDailyUSD,
	}, nil)
	concrete := ruleWith(30, false, ServiceQuotaCounterModePerUser, []string{
		ServiceQuotaLimiterRPM,
	}, nil)
	out := applyServiceQuotaFallbackOverride([]matchedQuotaRule{
		{rule: fallbackA, paths: fallbackA.Paths},
		{rule: fallbackB, paths: fallbackB.Paths},
		{rule: concrete, paths: concrete.Paths},
	})

	// concrete 保留 RPM
	concreteOut := findMatchedRule(out, 30)
	require.NotNil(t, concreteOut)
	require.Equal(t, []string{ServiceQuotaLimiterRPM}, limiterTypes(concreteOut.rule.Limiters))

	// fallback A：RPM 被 concrete 抢，仅保留 TPM
	fallbackAOut := findMatchedRule(out, 10)
	require.NotNil(t, fallbackAOut)
	require.Equal(t, []string{ServiceQuotaLimiterTPM}, limiterTypes(fallbackAOut.rule.Limiters))

	// fallback B：与 concrete 无 limiter 重叠，TPM + DailyUSD 全部保留
	fallbackBOut := findMatchedRule(out, 20)
	require.NotNil(t, fallbackBOut)
	require.ElementsMatch(t, []string{ServiceQuotaLimiterTPM, ServiceQuotaLimiterDailyUSD},
		limiterTypes(fallbackBOut.rule.Limiters))
}

// TestApplyFallbackOverride_TwoFallbacks_NoConcrete 验证多 fallback 但无 concrete 命中时
// 全部 fallback 原样保留（不会因互相是 fallback 就误抢对方的 limiter）。
//
// 这个 case 关键：现有测试只覆盖 1 fallback + 1 concrete；多 fallback 且无 concrete 的语义
// 必须明确——fallback 只让位给 non-fallback，fallback 之间不互相覆盖。
func TestApplyFallbackOverride_TwoFallbacks_NoConcrete(t *testing.T) {
	t.Parallel()
	fallbackA := ruleWith(40, true, ServiceQuotaCounterModeShared, []string{ServiceQuotaLimiterRPM}, nil)
	fallbackB := ruleWith(50, true, ServiceQuotaCounterModeShared, []string{ServiceQuotaLimiterRPM, ServiceQuotaLimiterTPM}, nil)
	out := applyServiceQuotaFallbackOverride([]matchedQuotaRule{
		{rule: fallbackA, paths: fallbackA.Paths},
		{rule: fallbackB, paths: fallbackB.Paths},
	})

	require.Len(t, out, 2, "无 concrete 时所有 fallback 都应保留")
	a := findMatchedRule(out, 40)
	require.NotNil(t, a)
	require.Equal(t, []string{ServiceQuotaLimiterRPM}, limiterTypes(a.rule.Limiters))
	b := findMatchedRule(out, 50)
	require.NotNil(t, b)
	require.ElementsMatch(t, []string{ServiceQuotaLimiterRPM, ServiceQuotaLimiterTPM}, limiterTypes(b.rule.Limiters))
}

func limiterTypes(limiters []ServiceQuotaLimiterDef) []string {
	out := make([]string, len(limiters))
	for i, l := range limiters {
		out[i] = l.LimiterType
	}
	return out
}
