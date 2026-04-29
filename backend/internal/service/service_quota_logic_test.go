//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// matchRules 模拟 matchedRules 的过滤步骤（不依赖 repo / redis），用于联合测试 path 匹配 + fallback 让位逻辑。
func matchRules(rules []*ServiceQuotaRule, req ServiceQuotaCheckRequest) []matchedQuotaRule {
	out := make([]matchedQuotaRule, 0, len(rules))
	for _, rule := range rules {
		if !rule.Enabled || !serviceQuotaTargetMatches(rule, req.UserID) {
			continue
		}
		paths := findMatchingPaths(rule, req)
		if len(paths) == 0 {
			continue
		}
		out = append(out, matchedQuotaRule{rule: rule, paths: paths})
	}
	return applyServiceQuotaFallbackOverride(out)
}

func ruleWith(id int64, fallback bool, counterMode string, limiterTypes []string, paths []ServiceQuotaPathDef, targetUsers ...int64) *ServiceQuotaRule {
	limiters := make([]ServiceQuotaLimiterDef, 0, len(limiterTypes))
	for _, lt := range limiterTypes {
		limiters = append(limiters, ServiceQuotaLimiterDef{
			RuleID:      id,
			LimiterType: lt,
			WindowMode:  ServiceQuotaWindowFixed,
			LimitValue:  60,
		})
	}
	if paths == nil {
		paths = []ServiceQuotaPathDef{{RuleID: id}}
	}
	return &ServiceQuotaRule{
		ID:            id,
		Enabled:       true,
		CounterMode:   counterMode,
		IsFallback:    fallback,
		Limiters:      limiters,
		Paths:         paths,
		TargetUserIDs: targetUsers,
	}
}

func TestServiceQuotaTargetMatches_CounterModeUser_MultipleUsers(t *testing.T) {
	t.Parallel()
	rule := &ServiceQuotaRule{
		Enabled:       true,
		CounterMode:   ServiceQuotaCounterModeUser,
		TargetUserIDs: []int64{1, 7, 42},
	}
	require.True(t, serviceQuotaTargetMatches(rule, 1))
	require.True(t, serviceQuotaTargetMatches(rule, 42))
	require.False(t, serviceQuotaTargetMatches(rule, 99))
}

func TestServiceQuotaTargetMatches_NonUserCounterMode_IgnoresTargetList(t *testing.T) {
	t.Parallel()
	rule := &ServiceQuotaRule{Enabled: true, CounterMode: ServiceQuotaCounterModePerUser}
	require.True(t, serviceQuotaTargetMatches(rule, 1))
	require.True(t, serviceQuotaTargetMatches(rule, 2))

	rule.CounterMode = ServiceQuotaCounterModeShared
	require.True(t, serviceQuotaTargetMatches(rule, 1))
}

func TestApplyFallbackOverride_DropsFallbackLimiterWhenConcreteHasSameType(t *testing.T) {
	t.Parallel()
	fallback := ruleWith(1, true, ServiceQuotaCounterModeShared, []string{ServiceQuotaLimiterRPM, ServiceQuotaLimiterDailyUSD}, nil)
	concrete := ruleWith(2, false, ServiceQuotaCounterModePerUser, []string{ServiceQuotaLimiterRPM}, nil)
	matched := []matchedQuotaRule{
		{rule: fallback, paths: fallback.Paths},
		{rule: concrete, paths: concrete.Paths},
	}
	out := applyServiceQuotaFallbackOverride(matched)
	require.Len(t, out, 2)

	// concrete unchanged
	concreteOut := findMatchedRule(out, 2)
	require.NotNil(t, concreteOut)
	require.Len(t, concreteOut.rule.Limiters, 1)
	require.Equal(t, ServiceQuotaLimiterRPM, concreteOut.rule.Limiters[0].LimiterType)

	// fallback keeps DailyUSD only (RPM dropped because concrete has it)
	fallbackOut := findMatchedRule(out, 1)
	require.NotNil(t, fallbackOut)
	require.Len(t, fallbackOut.rule.Limiters, 1)
	require.Equal(t, ServiceQuotaLimiterDailyUSD, fallbackOut.rule.Limiters[0].LimiterType)
}

func TestApplyFallbackOverride_KeepsLoneFallback(t *testing.T) {
	t.Parallel()
	fallback := ruleWith(1, true, ServiceQuotaCounterModeShared, []string{ServiceQuotaLimiterRPM}, nil)
	out := applyServiceQuotaFallbackOverride([]matchedQuotaRule{{rule: fallback, paths: fallback.Paths}})
	require.Len(t, out, 1)
	require.Equal(t, int64(1), out[0].rule.ID)
}

func TestApplyFallbackOverride_DropsFallbackEntirelyWhenAllLimitersSuperseded(t *testing.T) {
	t.Parallel()
	fallback := ruleWith(1, true, ServiceQuotaCounterModeShared, []string{ServiceQuotaLimiterRPM}, nil)
	concrete := ruleWith(2, false, ServiceQuotaCounterModePerUser, []string{ServiceQuotaLimiterRPM}, nil)
	out := applyServiceQuotaFallbackOverride([]matchedQuotaRule{
		{rule: fallback, paths: fallback.Paths},
		{rule: concrete, paths: concrete.Paths},
	})
	require.Len(t, out, 1)
	require.Equal(t, int64(2), out[0].rule.ID)
}

func TestPathMatches_NilFieldsMatchEverything(t *testing.T) {
	t.Parallel()
	emptyPath := ServiceQuotaPathDef{}
	req := ServiceQuotaCheckRequest{Platform: "anthropic", AccountID: 7}
	require.True(t, pathMatches(emptyPath, req))
}

func TestPathMatches_PlatformMismatch(t *testing.T) {
	t.Parallel()
	plat := "anthropic"
	path := ServiceQuotaPathDef{Platform: &plat}
	require.True(t, pathMatches(path, ServiceQuotaCheckRequest{Platform: "anthropic"}))
	require.False(t, pathMatches(path, ServiceQuotaCheckRequest{Platform: "openai"}))
}

func TestPathMatches_AccountAndModelGlob(t *testing.T) {
	t.Parallel()
	acc := int64(7)
	model := "claude-opus-*"
	path := ServiceQuotaPathDef{AccountID: &acc, ModelPattern: &model}
	require.True(t, pathMatches(path, ServiceQuotaCheckRequest{AccountID: 7, Model: "claude-opus-4-6"}))
	require.False(t, pathMatches(path, ServiceQuotaCheckRequest{AccountID: 8, Model: "claude-opus-4-6"}))
	require.False(t, pathMatches(path, ServiceQuotaCheckRequest{AccountID: 7, Model: "gpt-5"}))
}

func TestMatchRules_SharedFallback_WorksWhenNoConcreteRule(t *testing.T) {
	t.Parallel()
	rule := ruleWith(10, true, ServiceQuotaCounterModeShared, []string{ServiceQuotaLimiterRPM}, nil)
	matched := matchRules([]*ServiceQuotaRule{rule}, ServiceQuotaCheckRequest{UserID: 1, Platform: "anthropic"})
	require.Len(t, matched, 1)
	require.True(t, matched[0].rule.IsFallback)
}

func TestMatchRules_SharedFallback_RPMSupersededByPerUserConcrete(t *testing.T) {
	t.Parallel()
	fallback := ruleWith(1, true, ServiceQuotaCounterModeShared, []string{ServiceQuotaLimiterRPM}, nil)
	concrete := ruleWith(2, false, ServiceQuotaCounterModePerUser, []string{ServiceQuotaLimiterRPM}, nil)
	matched := matchRules([]*ServiceQuotaRule{fallback, concrete}, ServiceQuotaCheckRequest{UserID: 1})
	require.Len(t, matched, 1)
	require.Equal(t, int64(2), matched[0].rule.ID)
}

func TestMatchRules_CounterModeUser_OnlyLetsInListedUsers(t *testing.T) {
	t.Parallel()
	rule := ruleWith(3, false, ServiceQuotaCounterModeUser, []string{ServiceQuotaLimiterRPM}, nil, 10, 20)
	require.Len(t, matchRules([]*ServiceQuotaRule{rule}, ServiceQuotaCheckRequest{UserID: 10}), 1)
	require.Len(t, matchRules([]*ServiceQuotaRule{rule}, ServiceQuotaCheckRequest{UserID: 11}), 0)
	require.Len(t, matchRules([]*ServiceQuotaRule{rule}, ServiceQuotaCheckRequest{UserID: 20}), 1)
}

func TestFindMatchingPaths_MultiplePaths(t *testing.T) {
	t.Parallel()
	platA := "anthropic"
	platO := "openai"
	rule := &ServiceQuotaRule{
		ID: 1, Enabled: true, CounterMode: ServiceQuotaCounterModePerUser,
		Limiters: []ServiceQuotaLimiterDef{{LimiterType: ServiceQuotaLimiterRPM, LimitValue: 60, WindowMode: "fixed"}},
		Paths: []ServiceQuotaPathDef{
			{ID: 11, RuleID: 1, Platform: &platA},
			{ID: 12, RuleID: 1, Platform: &platO},
		},
	}
	matched := findMatchingPaths(rule, ServiceQuotaCheckRequest{Platform: "anthropic"})
	require.Len(t, matched, 1)
	require.Equal(t, int64(11), matched[0].ID)
	matched = findMatchingPaths(rule, ServiceQuotaCheckRequest{Platform: "gemini"})
	require.Empty(t, matched)
}

func TestCounterKey_V2Format_IncludesPathID(t *testing.T) {
	t.Parallel()
	svc := &serviceQuotaService{}
	rule := &ServiceQuotaRule{ID: 1, CounterMode: ServiceQuotaCounterModePerUser}
	path := ServiceQuotaPathDef{ID: 99, RuleID: 1}
	lim := ServiceQuotaLimiterDef{LimiterType: ServiceQuotaLimiterRPM}
	require.Equal(t, "svcquota:v2:1:99:rpm:42", svc.counterKey(ServiceQuotaCheckRequest{UserID: 42}, rule, path, lim))

	rule.CounterMode = ServiceQuotaCounterModeShared
	require.Equal(t, "svcquota:v2:1:99:rpm:shared", svc.counterKey(ServiceQuotaCheckRequest{UserID: 42}, rule, path, lim))
}

func TestNormalizeLimiters_RejectsEmptyAndDuplicate(t *testing.T) {
	t.Parallel()

	t.Run("empty", func(t *testing.T) {
		err := normalizeLimiters(&ServiceQuotaRuleInput{})
		require.Error(t, err)
	})

	t.Run("duplicate", func(t *testing.T) {
		err := normalizeLimiters(&ServiceQuotaRuleInput{
			Limiters: []ServiceQuotaLimiterInput{
				{LimiterType: ServiceQuotaLimiterRPM, LimitValue: 60},
				{LimiterType: ServiceQuotaLimiterRPM, LimitValue: 100},
			},
		})
		require.Error(t, err)
	})

	t.Run("non-positive limit", func(t *testing.T) {
		err := normalizeLimiters(&ServiceQuotaRuleInput{
			Limiters: []ServiceQuotaLimiterInput{
				{LimiterType: ServiceQuotaLimiterRPM, LimitValue: 0},
			},
		})
		require.Error(t, err)
	})

	t.Run("concurrency forces fixed window", func(t *testing.T) {
		input := &ServiceQuotaRuleInput{
			Limiters: []ServiceQuotaLimiterInput{
				{LimiterType: ServiceQuotaLimiterConcurrency, LimitValue: 5, WindowMode: "rolling"},
			},
		}
		require.NoError(t, normalizeLimiters(input))
		require.Equal(t, ServiceQuotaWindowFixed, input.Limiters[0].WindowMode)
	})
}

func findMatchedRule(matched []matchedQuotaRule, id int64) *matchedQuotaRule {
	for i := range matched {
		if matched[i].rule.ID == id {
			return &matched[i]
		}
	}
	return nil
}

// TestServiceQuotaRecordDelta_TokenComponents 覆盖 TPM/TPD 按 token_components 求和的各种组合：
//   - 默认（input+output+cache_creation）排除 cache_read
//   - 仅 input
//   - 仅 cache_read（极端：只想限缓存读取）
//   - 全选 4 项
//   - 空 components → 退化为默认
//   - 非 TPM/TPD limiter（如 daily_usd）：不看 TokenComponents，按 Cost 计
func TestServiceQuotaRecordDelta_TokenComponents(t *testing.T) {
	t.Parallel()

	req := ServiceQuotaRecordRequest{
		InputTokens:         100,
		OutputTokens:        50,
		CacheCreationTokens: 30,
		CacheReadTokens:     1000,
		Cost:                0.42,
	}

	cases := []struct {
		name      string
		lim       ServiceQuotaLimiterDef
		wantDelta float64
	}{
		{
			name:      "tpm 默认 = input+output+cache_creation",
			lim:       ServiceQuotaLimiterDef{LimiterType: ServiceQuotaLimiterTPM, TokenComponents: ServiceQuotaTokenComponentsDefault},
			wantDelta: 180,
		},
		{
			name:      "tpm 仅 input",
			lim:       ServiceQuotaLimiterDef{LimiterType: ServiceQuotaLimiterTPM, TokenComponents: []string{ServiceQuotaTokenComponentInput}},
			wantDelta: 100,
		},
		{
			name:      "tpm 仅 cache_read",
			lim:       ServiceQuotaLimiterDef{LimiterType: ServiceQuotaLimiterTPM, TokenComponents: []string{ServiceQuotaTokenComponentCacheRead}},
			wantDelta: 1000,
		},
		{
			name:      "tpd 全选 4 项",
			lim:       ServiceQuotaLimiterDef{LimiterType: ServiceQuotaLimiterTPD, TokenComponents: ServiceQuotaTokenComponentsAll},
			wantDelta: 1180,
		},
		{
			name:      "tpm 空 components 退化为默认",
			lim:       ServiceQuotaLimiterDef{LimiterType: ServiceQuotaLimiterTPM, TokenComponents: nil},
			wantDelta: 180,
		},
		{
			name:      "daily_usd 不看 TokenComponents，按 Cost 计",
			lim:       ServiceQuotaLimiterDef{LimiterType: ServiceQuotaLimiterDailyUSD, TokenComponents: []string{ServiceQuotaTokenComponentInput}},
			wantDelta: 0.42,
		},
		{
			// 旧语义：CountOnArrival=true（迁移 137 把存量 RPM 行迁到此值）→ arrival 阶段已 +1，
			// Record 不再重复计数，wantDelta=0。
			name:      "rpm CountOnArrival=true 不入 Record（应返回 0）",
			lim:       ServiceQuotaLimiterDef{LimiterType: ServiceQuotaLimiterRPM, CountOnArrival: true},
			wantDelta: 0,
		},
		{
			// 新默认：CountOnArrival=false → 仅成功才 +1，由 Record 阶段兜底，wantDelta=1。
			name:      "rpm CountOnArrival=false 进入 Record（成功才计 +1）",
			lim:       ServiceQuotaLimiterDef{LimiterType: ServiceQuotaLimiterRPM, CountOnArrival: false},
			wantDelta: 1,
		},
		{
			name:      "concurrency 不入 Record（应返回 0）",
			lim:       ServiceQuotaLimiterDef{LimiterType: ServiceQuotaLimiterConcurrency},
			wantDelta: 0,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := serviceQuotaRecordDelta(tc.lim, req)
			require.InDelta(t, tc.wantDelta, got, 1e-9)
		})
	}
}

// TestNormalizeLimiterTokenComponents 校验 normalize 行为：去重、非法值报错、非 TPM/TPD 强制清空。
func TestNormalizeLimiterTokenComponents(t *testing.T) {
	t.Parallel()

	t.Run("tpm 重复值去重", func(t *testing.T) {
		t.Parallel()
		l := &ServiceQuotaLimiterInput{
			LimiterType:     ServiceQuotaLimiterTPM,
			TokenComponents: []string{"input", "input", "output"},
		}
		require.NoError(t, normalizeLimiterTokenComponents(l))
		require.Equal(t, []string{"input", "output"}, l.TokenComponents)
	})

	t.Run("tpm 非法值报错", func(t *testing.T) {
		t.Parallel()
		l := &ServiceQuotaLimiterInput{
			LimiterType:     ServiceQuotaLimiterTPM,
			TokenComponents: []string{"input", "bogus"},
		}
		err := normalizeLimiterTokenComponents(l)
		require.Error(t, err)
	})

	t.Run("rpm 强制清空 TokenComponents", func(t *testing.T) {
		t.Parallel()
		l := &ServiceQuotaLimiterInput{
			LimiterType:     ServiceQuotaLimiterRPM,
			TokenComponents: []string{"input"}, // 用户误传也要被洗掉
		}
		require.NoError(t, normalizeLimiterTokenComponents(l))
		require.Nil(t, l.TokenComponents)
	})

	t.Run("tpm 空数组填充默认", func(t *testing.T) {
		t.Parallel()
		l := &ServiceQuotaLimiterInput{
			LimiterType:     ServiceQuotaLimiterTPM,
			TokenComponents: nil,
		}
		require.NoError(t, normalizeLimiterTokenComponents(l))
		require.Equal(t, ServiceQuotaTokenComponentsDefault, l.TokenComponents)
	})
}
