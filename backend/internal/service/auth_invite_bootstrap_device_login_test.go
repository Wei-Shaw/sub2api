//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInviteBootstrapHelpersTreatDeviceLoginAsSubscriptionBootstrap(t *testing.T) {
	deviceRedeem := &RedeemCode{Type: RedeemTypeDeviceLogin}
	subscriptionGroup := Group{
		ID:                  11,
		Platform:            "openai",
		Status:              StatusActive,
		SubscriptionType:    SubscriptionTypeSubscription,
		DefaultValidityDays: 30,
		ActiveAccountCount:  2,
	}
	nonSubscriptionGroup := Group{
		ID:                 12,
		Platform:           "anthropic",
		Status:             StatusActive,
		SubscriptionType:   SubscriptionTypeStandard,
		RateMultiplier:     0.9,
		ActiveAccountCount: 2,
	}

	require.True(t, isInviteLoginBootstrapRedeemType(RedeemTypeDeviceLogin))
	require.True(t, isGroupEligibleForInviteBootstrap(deviceRedeem, subscriptionGroup))
	require.False(t, isGroupEligibleForInviteBootstrap(deviceRedeem, nonSubscriptionGroup))

	candidates := loadInviteBootstrapSubscriptionCandidates([]Group{subscriptionGroup, nonSubscriptionGroup}, deviceRedeem)
	require.Len(t, candidates, 1)
	require.Equal(t, subscriptionGroup.ID, candidates[0].ID)

	selected := selectInviteBootstrapGroupsForRedeem(deviceRedeem, []Group{subscriptionGroup, nonSubscriptionGroup})
	require.Len(t, selected, 1)
	require.Equal(t, subscriptionGroup.ID, selected["openai"].ID)
}
