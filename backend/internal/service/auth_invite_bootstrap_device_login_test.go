//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInviteBootstrapHelpersTreatDeviceLoginAsBalanceBootstrap(t *testing.T) {
	deviceRedeem := &RedeemCode{Type: RedeemTypeDeviceLogin}
	subscriptionGroup := Group{
		ID:                  11,
		Platform:            "openai",
		Status:              StatusActive,
		SubscriptionType:    SubscriptionTypeSubscription,
		DefaultValidityDays: 30,
		ActiveAccountCount:  2,
	}
	balanceCheap := Group{
		ID:                 12,
		Platform:           "openai",
		Status:             StatusActive,
		SubscriptionType:   SubscriptionTypeStandard,
		RateMultiplier:     0.9,
		ActiveAccountCount: 2,
	}
	balanceExpensive := Group{
		ID:                 13,
		Platform:           "openai",
		Status:             StatusActive,
		SubscriptionType:   SubscriptionTypeStandard,
		RateMultiplier:     1.2,
		ActiveAccountCount: 2,
	}
	balanceOtherPlatform := Group{
		ID:                 14,
		Platform:           "anthropic",
		Status:             StatusActive,
		SubscriptionType:   SubscriptionTypeStandard,
		RateMultiplier:     1.1,
		ActiveAccountCount: 2,
	}

	require.True(t, isInviteLoginBootstrapRedeemType(RedeemTypeDeviceLogin))
	require.False(t, isGroupEligibleForInviteBootstrap(deviceRedeem, subscriptionGroup))
	require.True(t, isGroupEligibleForInviteBootstrap(deviceRedeem, balanceCheap))
	require.True(t, isInviteBootstrapGroupBetter(deviceRedeem, balanceCheap, balanceExpensive))

	selected := selectInviteBootstrapGroupsForRedeem(deviceRedeem, []Group{subscriptionGroup, balanceExpensive, balanceCheap, balanceOtherPlatform})
	require.Len(t, selected, 2)
	require.Equal(t, balanceCheap.ID, selected["openai"].ID)
	require.Equal(t, balanceOtherPlatform.ID, selected["anthropic"].ID)
}
