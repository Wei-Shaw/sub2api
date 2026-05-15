//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSelectInviteBootstrapGroupsForDevicePreferSubscription(t *testing.T) {
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
	balanceOtherPlatform := Group{
		ID:                 14,
		Platform:           "anthropic",
		Status:             StatusActive,
		SubscriptionType:   SubscriptionTypeStandard,
		RateMultiplier:     1.1,
		ActiveAccountCount: 2,
	}

	selected := selectInviteBootstrapGroupsForDevice([]Group{subscriptionGroup, balanceCheap, balanceOtherPlatform})
	require.Len(t, selected, 2)
	// Subscription preferred over standard for same platform
	require.Equal(t, subscriptionGroup.ID, selected["openai"].ID)
	require.Equal(t, balanceOtherPlatform.ID, selected["anthropic"].ID)
}

func TestIsDeviceBootstrapGroupBetterPrefsSubscription(t *testing.T) {
	sub := Group{SubscriptionType: SubscriptionTypeSubscription, DefaultValidityDays: 30}
	std := Group{SubscriptionType: SubscriptionTypeStandard, RateMultiplier: 0.5}
	require.True(t, isDeviceBootstrapGroupBetter(sub, std))
	require.False(t, isDeviceBootstrapGroupBetter(std, sub))
}
