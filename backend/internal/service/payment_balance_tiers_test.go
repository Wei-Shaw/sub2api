package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveBalancePricingTierUsesFirstEnabledMatchingTier(t *testing.T) {
	tiers, err := parseBalancePricingTiers(`[{"min":100,"max":500,"multiplier":7.3,"label":"vip","enabled":true,"sortOrder":2},{"min":1,"max":99,"multiplier":1,"label":"base","enabled":true,"sortOrder":1}]`)
	require.NoError(t, err)

	resolved := resolveBalancePricingTier(320, tiers, 1)

	require.Equal(t, "vip", resolved.Label)
	require.Equal(t, 7.3, resolved.Multiplier)
	require.Equal(t, 43.84, resolved.CreditedAmount)
}

func TestResolveBalancePricingTierFallsBackToGlobalMultiplier(t *testing.T) {
	tiers, err := parseBalancePricingTiers(`[{"min":100,"max":500,"multiplier":7.3,"label":"vip","enabled":false,"sortOrder":1}]`)
	require.NoError(t, err)

	resolved := resolveBalancePricingTier(50, tiers, 2)

	require.Empty(t, resolved.Label)
	require.Equal(t, 2.0, resolved.Multiplier)
	require.Equal(t, 25.0, resolved.CreditedAmount)
}

func TestParseBalancePricingTiersRejectsOverlap(t *testing.T) {
	_, err := parseBalancePricingTiers(`[{"min":1,"max":100,"multiplier":1,"label":"a","enabled":true},{"min":100,"max":200,"multiplier":2,"label":"b","enabled":true}]`)
	require.Error(t, err)
}
