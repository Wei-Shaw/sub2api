package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeCacheStrategy_EmptyAndUnknownDefaultToAuto(t *testing.T) {
	require.Equal(t, CacheStrategyAuto, NormalizeCacheStrategy(""))
	require.Equal(t, CacheStrategyAuto, NormalizeCacheStrategy("auto"))
	require.Equal(t, CacheStrategyAuto, NormalizeCacheStrategy("not_a_real_value"))
	require.Equal(t, CacheStrategyAuto, NormalizeCacheStrategy("AUTO"),
		"case-sensitive on purpose: API contract is lowercase only; we don't want '5M' / 'Auto' silently accepted")
}

func TestNormalizeCacheStrategy_PreservesValidValues(t *testing.T) {
	require.Equal(t, CacheStrategyCostPriority, NormalizeCacheStrategy(CacheStrategyCostPriority))
	require.Equal(t, CacheStrategyLatencyPriority, NormalizeCacheStrategy(CacheStrategyLatencyPriority))
}

func TestAPIKey_EffectiveCacheStrategy_NilSafe(t *testing.T) {
	var k *APIKey
	require.Equal(t, CacheStrategyAuto, k.EffectiveCacheStrategy())
}

func TestAPIKey_EffectiveCacheStrategy_TreatsEmptyAsAuto(t *testing.T) {
	k := &APIKey{}
	require.Equal(t, CacheStrategyAuto, k.EffectiveCacheStrategy(),
		"legacy rows that pre-date the column will surface as empty string; must behave like 'auto'")
}

func TestAPIKey_CacheStrategyTTLTarget(t *testing.T) {
	cases := []struct {
		name        string
		strategy    string
		wantTarget  string
		wantApplied bool
	}{
		{"auto returns no forced target", CacheStrategyAuto, "", false},
		{"empty (legacy) returns no forced target", "", "", false},
		{"cost_priority forces 5m", CacheStrategyCostPriority, "5m", true},
		{"latency_priority forces 1h", CacheStrategyLatencyPriority, "1h", true},
		{"unknown value falls back to auto (no force)", "bogus", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			k := &APIKey{CacheStrategy: tc.strategy}
			target, ok := k.CacheStrategyTTLTarget()
			require.Equal(t, tc.wantApplied, ok)
			require.Equal(t, tc.wantTarget, target)
		})
	}
}
