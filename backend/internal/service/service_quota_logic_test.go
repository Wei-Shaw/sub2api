//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// matchRules 模拟 matchedRules 的过滤步骤（不依赖 repo / redis），用于联合测试维度匹配 + fallback 让位逻辑。
func matchRules(rules []*ServiceQuotaRule, req ServiceQuotaCheckRequest) []*ServiceQuotaRule {
	out := make([]*ServiceQuotaRule, 0, len(rules))
	for _, rule := range rules {
		if !rule.Enabled || !serviceQuotaTargetMatches(rule, req.UserID) {
			continue
		}
		if !dimensionMatches(rule, req) {
			continue
		}
		out = append(out, rule)
	}
	return applyServiceQuotaFallbackOverride(out)
}

type ruleOpt func(*ServiceQuotaRule)

func withPlatforms(values ...string) ruleOpt {
	return func(r *ServiceQuotaRule) { r.Platforms = values }
}

func withAccountIDs(values ...int64) ruleOpt {
	return func(r *ServiceQuotaRule) { r.AccountIDs = values }
}

func withTargetUsers(values ...int64) ruleOpt {
	return func(r *ServiceQuotaRule) { r.TargetUserIDs = values }
}

func ruleWith(id int64, fallback bool, counterMode string, limiterTypes []string, opts ...ruleOpt) *ServiceQuotaRule {
	limiters := make([]ServiceQuotaLimiterDef, 0, len(limiterTypes))
	for _, lt := range limiterTypes {
		limiters = append(limiters, ServiceQuotaLimiterDef{
			RuleID:      id,
			LimiterType: lt,
			WindowMode:  ServiceQuotaWindowFixed,
			LimitValue:  60,
		})
	}
	rule := &ServiceQuotaRule{
		ID:          id,
		Enabled:     true,
		CounterMode: counterMode,
		IsFallback:  fallback,
		Limiters:    limiters,
	}
	for _, opt := range opts {
		opt(rule)
	}
	return rule
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
	fallback := ruleWith(1, true, ServiceQuotaCounterModeShared, []string{ServiceQuotaLimiterRPM, ServiceQuotaLimiterDailyUSD})
	concrete := ruleWith(2, false, ServiceQuotaCounterModePerUser, []string{ServiceQuotaLimiterRPM})
	out := applyServiceQuotaFallbackOverride([]*ServiceQuotaRule{fallback, concrete})
	require.Len(t, out, 2)

	concreteOut := findRule(out, 2)
	require.NotNil(t, concreteOut)
	require.Len(t, concreteOut.Limiters, 1)
	require.Equal(t, ServiceQuotaLimiterRPM, concreteOut.Limiters[0].LimiterType)

	fallbackOut := findRule(out, 1)
	require.NotNil(t, fallbackOut)
	require.Len(t, fallbackOut.Limiters, 1)
	require.Equal(t, ServiceQuotaLimiterDailyUSD, fallbackOut.Limiters[0].LimiterType)
}

func TestApplyFallbackOverride_KeepsLoneFallback(t *testing.T) {
	t.Parallel()
	fallback := ruleWith(1, true, ServiceQuotaCounterModeShared, []string{ServiceQuotaLimiterRPM})
	out := applyServiceQuotaFallbackOverride([]*ServiceQuotaRule{fallback})
	require.Len(t, out, 1)
	require.Equal(t, int64(1), out[0].ID)
}

func TestApplyFallbackOverride_DropsFallbackEntirelyWhenAllLimitersSuperseded(t *testing.T) {
	t.Parallel()
	fallback := ruleWith(1, true, ServiceQuotaCounterModeShared, []string{ServiceQuotaLimiterRPM})
	concrete := ruleWith(2, false, ServiceQuotaCounterModePerUser, []string{ServiceQuotaLimiterRPM})
	out := applyServiceQuotaFallbackOverride([]*ServiceQuotaRule{fallback, concrete})
	require.Len(t, out, 1)
	require.Equal(t, int64(2), out[0].ID)
}

func TestDimensionMatches_AllEmpty_MatchesEverything(t *testing.T) {
	t.Parallel()
	rule := &ServiceQuotaRule{}
	require.True(t, dimensionMatches(rule, ServiceQuotaCheckRequest{Platform: "anthropic", AccountID: 7}))
}

func TestDimensionMatches_PlatformSet_MustContainRequest(t *testing.T) {
	t.Parallel()
	rule := &ServiceQuotaRule{Platforms: []string{"anthropic", "openai"}}
	require.True(t, dimensionMatches(rule, ServiceQuotaCheckRequest{Platform: "anthropic"}))
	require.True(t, dimensionMatches(rule, ServiceQuotaCheckRequest{Platform: "OpenAI"})) // 大小写无关
	require.False(t, dimensionMatches(rule, ServiceQuotaCheckRequest{Platform: "gemini"}))
}

func TestDimensionMatches_AccountAndModelGlob(t *testing.T) {
	t.Parallel()
	rule := &ServiceQuotaRule{AccountIDs: []int64{7}, ModelPatterns: []string{"claude-opus-*"}}
	require.True(t, dimensionMatches(rule, ServiceQuotaCheckRequest{AccountID: 7, Model: "claude-opus-4-6"}))
	require.False(t, dimensionMatches(rule, ServiceQuotaCheckRequest{AccountID: 8, Model: "claude-opus-4-6"}))
	require.False(t, dimensionMatches(rule, ServiceQuotaCheckRequest{AccountID: 7, Model: "gpt-5"}))
}

func TestDimensionMatches_ModelPatterns_AnyMatchWins(t *testing.T) {
	t.Parallel()
	rule := &ServiceQuotaRule{ModelPatterns: []string{"claude-opus-*", "gpt-*"}}
	require.True(t, dimensionMatches(rule, ServiceQuotaCheckRequest{Model: "claude-opus-4-6"}))
	require.True(t, dimensionMatches(rule, ServiceQuotaCheckRequest{Model: "gpt-5"}))
	require.False(t, dimensionMatches(rule, ServiceQuotaCheckRequest{Model: "gemini-2-flash"}))
}

func TestMatchRules_SharedFallback_WorksWhenNoConcreteRule(t *testing.T) {
	t.Parallel()
	rule := ruleWith(10, true, ServiceQuotaCounterModeShared, []string{ServiceQuotaLimiterRPM})
	matched := matchRules([]*ServiceQuotaRule{rule}, ServiceQuotaCheckRequest{UserID: 1, Platform: "anthropic"})
	require.Len(t, matched, 1)
	require.True(t, matched[0].IsFallback)
}

func TestMatchRules_SharedFallback_RPMSupersededByPerUserConcrete(t *testing.T) {
	t.Parallel()
	fallback := ruleWith(1, true, ServiceQuotaCounterModeShared, []string{ServiceQuotaLimiterRPM})
	concrete := ruleWith(2, false, ServiceQuotaCounterModePerUser, []string{ServiceQuotaLimiterRPM})
	matched := matchRules([]*ServiceQuotaRule{fallback, concrete}, ServiceQuotaCheckRequest{UserID: 1})
	require.Len(t, matched, 1)
	require.Equal(t, int64(2), matched[0].ID)
}

func TestMatchRules_CounterModeUser_OnlyLetsInListedUsers(t *testing.T) {
	t.Parallel()
	rule := ruleWith(3, false, ServiceQuotaCounterModeUser, []string{ServiceQuotaLimiterRPM}, withTargetUsers(10, 20))
	require.Len(t, matchRules([]*ServiceQuotaRule{rule}, ServiceQuotaCheckRequest{UserID: 10}), 1)
	require.Len(t, matchRules([]*ServiceQuotaRule{rule}, ServiceQuotaCheckRequest{UserID: 11}), 0)
	require.Len(t, matchRules([]*ServiceQuotaRule{rule}, ServiceQuotaCheckRequest{UserID: 20}), 1)
}

func TestMatchRules_PlatformDimension_FiltersRules(t *testing.T) {
	t.Parallel()
	rule := ruleWith(1, false, ServiceQuotaCounterModePerUser, []string{ServiceQuotaLimiterRPM},
		withPlatforms("anthropic"))
	require.Len(t, matchRules([]*ServiceQuotaRule{rule}, ServiceQuotaCheckRequest{UserID: 1, Platform: "anthropic"}), 1)
	require.Empty(t, matchRules([]*ServiceQuotaRule{rule}, ServiceQuotaCheckRequest{UserID: 1, Platform: "gemini"}))
}

func TestMatchRules_AccountAndPlatformAreANDed(t *testing.T) {
	t.Parallel()
	rule := ruleWith(1, false, ServiceQuotaCounterModePerUser, []string{ServiceQuotaLimiterRPM},
		withPlatforms("anthropic"), withAccountIDs(7))
	// 同时满足 platform=anthropic 且 account=7 才命中
	require.Len(t, matchRules([]*ServiceQuotaRule{rule}, ServiceQuotaCheckRequest{UserID: 1, Platform: "anthropic", AccountID: 7}), 1)
	require.Empty(t, matchRules([]*ServiceQuotaRule{rule}, ServiceQuotaCheckRequest{UserID: 1, Platform: "anthropic", AccountID: 8}))
	require.Empty(t, matchRules([]*ServiceQuotaRule{rule}, ServiceQuotaCheckRequest{UserID: 1, Platform: "openai", AccountID: 7}))
}

func TestCounterKey_V3Format_NoPathID(t *testing.T) {
	t.Parallel()
	svc := &serviceQuotaService{}
	rule := &ServiceQuotaRule{ID: 1, CounterMode: ServiceQuotaCounterModePerUser}
	lim := ServiceQuotaLimiterDef{LimiterType: ServiceQuotaLimiterRPM}
	require.Equal(t, "svcquota:v3:1:rpm:42", svc.counterKey(ServiceQuotaCheckRequest{UserID: 42}, rule, lim))

	rule.CounterMode = ServiceQuotaCounterModeShared
	require.Equal(t, "svcquota:v3:1:rpm:shared", svc.counterKey(ServiceQuotaCheckRequest{UserID: 42}, rule, lim))
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

func TestNormalizeDimensions_DedupAndLowercase(t *testing.T) {
	t.Parallel()
	input := &ServiceQuotaRuleInput{
		Platforms:     []string{"Anthropic", "anthropic", "  OpenAI  ", ""},
		ModelPatterns: []string{"claude-opus-*", "claude-opus-*", "  gpt-*  "},
		ChannelIDs:    []int64{1, 1, 2, 0, -1},
		GroupIDs:      []int64{5, 5},
		AccountIDs:    []int64{7, 8, 7},
	}
	normalizeDimensions(input)
	require.Equal(t, []string{"anthropic", "openai"}, input.Platforms)
	require.Equal(t, []string{"claude-opus-*", "gpt-*"}, input.ModelPatterns)
	require.Equal(t, []int64{1, 2}, input.ChannelIDs)
	require.Equal(t, []int64{5}, input.GroupIDs)
	require.Equal(t, []int64{7, 8}, input.AccountIDs)
}

func findRule(rules []*ServiceQuotaRule, id int64) *ServiceQuotaRule {
	for _, r := range rules {
		if r.ID == id {
			return r
		}
	}
	return nil
}