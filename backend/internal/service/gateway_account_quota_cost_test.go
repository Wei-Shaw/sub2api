package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveAccountQuotaCostPrefersAccountStatsCost(t *testing.T) {
	statsCost := 3.25

	got := resolveAccountQuotaCost(&UsageLog{AccountStatsCost: &statsCost}, &postUsageBillingParams{
		Cost:                  &CostBreakdown{TotalCost: 10},
		AccountRateMultiplier: 2,
	})

	require.Equal(t, 3.25, got)
}

func TestResolveAccountQuotaCostFallsBackToTotalCostTimesAccountMultiplier(t *testing.T) {
	got := resolveAccountQuotaCost(&UsageLog{}, &postUsageBillingParams{
		Cost:                  &CostBreakdown{TotalCost: 10},
		AccountRateMultiplier: 0.5,
	})

	require.Equal(t, 5.0, got)
}

func TestResolveAccountQuotaCostIgnoresNonPositiveAccountStatsCost(t *testing.T) {
	statsCost := 0.0

	got := resolveAccountQuotaCost(&UsageLog{AccountStatsCost: &statsCost}, &postUsageBillingParams{
		Cost:                  &CostBreakdown{TotalCost: 10},
		AccountRateMultiplier: 0.25,
	})

	require.Equal(t, 2.5, got)
}
