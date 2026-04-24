//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// matchRules is a helper that mirrors the filter step used by matchedRules but
// without touching repos / redis. It lets us exercise serviceQuotaRuleMatches
// and applyServiceQuotaFallbackOverride together.
func matchRules(rules []*ServiceQuotaRule, req ServiceQuotaCheckRequest) []*ServiceQuotaRule {
	out := make([]*ServiceQuotaRule, 0, len(rules))
	for _, rule := range rules {
		if serviceQuotaRuleMatches(rule, req) {
			out = append(out, rule)
		}
	}
	return applyServiceQuotaFallbackOverride(out)
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

func TestApplyServiceQuotaFallbackOverride_DropsFallbackWhenConcreteExists(t *testing.T) {
	t.Parallel()
	fallback := &ServiceQuotaRule{ID: 1, LimiterType: ServiceQuotaLimiterRPM, IsFallback: true, Enabled: true}
	concrete := &ServiceQuotaRule{ID: 2, LimiterType: ServiceQuotaLimiterRPM, IsFallback: false, Enabled: true}
	out := applyServiceQuotaFallbackOverride([]*ServiceQuotaRule{fallback, concrete})
	require.Len(t, out, 1)
	require.Equal(t, int64(2), out[0].ID)
}

func TestApplyServiceQuotaFallbackOverride_KeepsFallbackAcrossDifferentLimiterTypes(t *testing.T) {
	t.Parallel()
	rpmFallback := &ServiceQuotaRule{ID: 1, LimiterType: ServiceQuotaLimiterRPM, IsFallback: true, Enabled: true}
	tpmConcrete := &ServiceQuotaRule{ID: 2, LimiterType: ServiceQuotaLimiterTPM, IsFallback: false, Enabled: true}
	out := applyServiceQuotaFallbackOverride([]*ServiceQuotaRule{rpmFallback, tpmConcrete})
	require.Len(t, out, 2)
}

func TestApplyServiceQuotaFallbackOverride_KeepsLoneFallback(t *testing.T) {
	t.Parallel()
	fallback := &ServiceQuotaRule{ID: 1, LimiterType: ServiceQuotaLimiterRPM, IsFallback: true, Enabled: true}
	out := applyServiceQuotaFallbackOverride([]*ServiceQuotaRule{fallback})
	require.Len(t, out, 1)
	require.Equal(t, int64(1), out[0].ID)
}

func TestMatchRules_SharedFallback_WorksWhenNoConcreteRule(t *testing.T) {
	t.Parallel()
	rules := []*ServiceQuotaRule{{
		ID:          10,
		Enabled:     true,
		LimiterType: ServiceQuotaLimiterRPM,
		CounterMode: ServiceQuotaCounterModeShared,
		IsFallback:  true,
		LimitValue:  100,
	}}
	req := ServiceQuotaCheckRequest{UserID: 1, Platform: "anthropic"}
	matched := matchRules(rules, req)
	require.Len(t, matched, 1)
	require.Equal(t, ServiceQuotaCounterModeShared, matched[0].CounterMode)
	require.True(t, matched[0].IsFallback)
}

func TestMatchRules_SharedFallback_YieldsToPerUserConcrete(t *testing.T) {
	t.Parallel()
	sharedFallback := &ServiceQuotaRule{
		ID:          1,
		Enabled:     true,
		LimiterType: ServiceQuotaLimiterRPM,
		CounterMode: ServiceQuotaCounterModeShared,
		IsFallback:  true,
		LimitValue:  1000,
	}
	perUserConcrete := &ServiceQuotaRule{
		ID:          2,
		Enabled:     true,
		LimiterType: ServiceQuotaLimiterRPM,
		CounterMode: ServiceQuotaCounterModePerUser,
		LimitValue:  60,
	}
	matched := matchRules([]*ServiceQuotaRule{sharedFallback, perUserConcrete}, ServiceQuotaCheckRequest{UserID: 1})
	require.Len(t, matched, 1)
	require.Equal(t, int64(2), matched[0].ID)
}

func TestMatchRules_CounterModeUser_OnlyLetsInListedUsers(t *testing.T) {
	t.Parallel()
	rule := &ServiceQuotaRule{
		ID:            3,
		Enabled:       true,
		LimiterType:   ServiceQuotaLimiterRPM,
		CounterMode:   ServiceQuotaCounterModeUser,
		TargetUserIDs: []int64{10, 20},
		LimitValue:    120,
	}
	require.Len(t, matchRules([]*ServiceQuotaRule{rule}, ServiceQuotaCheckRequest{UserID: 10}), 1)
	require.Len(t, matchRules([]*ServiceQuotaRule{rule}, ServiceQuotaCheckRequest{UserID: 11}), 0)
	require.Len(t, matchRules([]*ServiceQuotaRule{rule}, ServiceQuotaCheckRequest{UserID: 20}), 1)
}

func TestNormalizeServiceQuotaRule_DefaultsAndValidation(t *testing.T) {
	t.Parallel()

	t.Run("applies defaults", func(t *testing.T) {
		input := &ServiceQuotaRuleInput{LimitValue: 60, LimiterType: ServiceQuotaLimiterRPM}
		require.NoError(t, normalizeServiceQuotaRule(input))
		require.Equal(t, ServiceQuotaCounterModePerUser, input.CounterMode)
		require.Equal(t, ServiceQuotaWindowFixed, input.WindowMode)
		require.NotNil(t, input.Enabled)
		require.True(t, *input.Enabled)
	})

	t.Run("rejects counter_mode=user without target ids", func(t *testing.T) {
		input := &ServiceQuotaRuleInput{LimitValue: 60, CounterMode: ServiceQuotaCounterModeUser, LimiterType: ServiceQuotaLimiterRPM}
		require.Error(t, normalizeServiceQuotaRule(input))
	})

	t.Run("accepts counter_mode=user with target ids", func(t *testing.T) {
		input := &ServiceQuotaRuleInput{
			LimitValue:    60,
			LimiterType:   ServiceQuotaLimiterRPM,
			CounterMode:   ServiceQuotaCounterModeUser,
			TargetUserIDs: []int64{5, 5, 0, 7},
		}
		require.NoError(t, normalizeServiceQuotaRule(input))
		require.Equal(t, []int64{5, 7}, input.TargetUserIDs)
	})

	t.Run("clears target ids when counter_mode is not user", func(t *testing.T) {
		input := &ServiceQuotaRuleInput{
			LimitValue:    60,
			LimiterType:   ServiceQuotaLimiterRPM,
			CounterMode:   ServiceQuotaCounterModeShared,
			TargetUserIDs: []int64{1},
		}
		require.NoError(t, normalizeServiceQuotaRule(input))
		require.Nil(t, input.TargetUserIDs)
	})

	t.Run("rejects unknown counter_mode", func(t *testing.T) {
		input := &ServiceQuotaRuleInput{LimitValue: 60, LimiterType: ServiceQuotaLimiterRPM, CounterMode: "bogus"}
		require.Error(t, normalizeServiceQuotaRule(input))
	})

	t.Run("rejects non-positive limit", func(t *testing.T) {
		require.Error(t, normalizeServiceQuotaRule(&ServiceQuotaRuleInput{LimitValue: 0, LimiterType: ServiceQuotaLimiterRPM}))
	})
}

func TestCounterKey_ShardsBySelectedMode(t *testing.T) {
	t.Parallel()
	svc := &serviceQuotaService{}

	shared := &ServiceQuotaRule{ID: 1, LimiterType: ServiceQuotaLimiterRPM, CounterMode: ServiceQuotaCounterModeShared}
	perUser := &ServiceQuotaRule{ID: 2, LimiterType: ServiceQuotaLimiterRPM, CounterMode: ServiceQuotaCounterModePerUser}
	userList := &ServiceQuotaRule{ID: 3, LimiterType: ServiceQuotaLimiterRPM, CounterMode: ServiceQuotaCounterModeUser}

	req := ServiceQuotaCheckRequest{UserID: 42}
	require.Equal(t, "svcquota:1:rpm:shared", svc.counterKey(req, shared))
	require.Equal(t, "svcquota:2:rpm:42", svc.counterKey(req, perUser))
	require.Equal(t, "svcquota:3:rpm:42", svc.counterKey(req, userList))
}
