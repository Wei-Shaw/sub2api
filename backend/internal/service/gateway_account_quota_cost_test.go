package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveAccountQuotaCostMultipliesAccountStatsCostByAccountRate(t *testing.T) {
	statsCost := 3.25

	got := resolveAccountQuotaCost(&UsageLog{AccountStatsCost: &statsCost}, &postUsageBillingParams{
		Cost:                  &CostBreakdown{TotalCost: 10},
		AccountRateMultiplier: 2,
	})

	require.Equal(t, 6.5, got)
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

func TestBuildUsageBillingCommand_AccountQuotaUsesAccountCostWithoutChangingUserBilling(t *testing.T) {
	statsCost := 3.25
	groupID := int64(7)

	cmd := buildUsageBillingCommand("req-account-quota", &UsageLog{AccountStatsCost: &statsCost}, &postUsageBillingParams{
		Cost: &CostBreakdown{
			TotalCost:  10,
			ActualCost: 4.29,
		},
		User:                  &User{ID: 1},
		APIKey:                &APIKey{ID: 2, GroupID: &groupID},
		Account:               &Account{ID: 3, Type: AccountTypeAPIKey, Extra: map[string]any{"quota_limit": 100.0}},
		AccountRateMultiplier: 2,
	})

	require.NotNil(t, cmd)
	require.Equal(t, 6.5, cmd.AccountQuotaCost)
	require.Equal(t, 4.29, cmd.BalanceCost)
	require.Zero(t, cmd.SubscriptionCost)
	require.Zero(t, cmd.APIKeyQuotaCost)
	require.Zero(t, cmd.APIKeyRateLimitCost)
}
