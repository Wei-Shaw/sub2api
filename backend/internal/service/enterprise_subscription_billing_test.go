//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEnterpriseAPIKeyUsesSubscriptionBillingWithoutPersonalSubscription(t *testing.T) {
	organizationSubscriptionID := int64(501)
	key := &APIKey{
		OrganizationSubscriptionID: &organizationSubscriptionID,
		Group:                      &Group{SubscriptionType: SubscriptionTypeSubscription},
	}

	require.True(t, isSubscriptionBillingForAPIKey(key, nil))
	key.Group = nil
	require.True(t, isSubscriptionBillingForAPIKey(key, nil))
}

func TestGPT56EnterpriseAPIKeyBuildsOrganizationSubscriptionCharge(t *testing.T) {
	organizationSubscriptionID := int64(501)
	key := &APIKey{
		ID:                         11,
		OrganizationSubscriptionID: &organizationSubscriptionID,
		Group:                      &Group{SubscriptionType: SubscriptionTypeSubscription},
	}
	cmd := buildUsageBillingCommand("resp_gpt_5_6", &UsageLog{
		Model:       "gpt-5.6",
		BillingType: BillingTypeSubscription,
	}, &postUsageBillingParams{
		Cost:               &CostBreakdown{TotalCost: 1.25, ActualCost: 1.25},
		User:               &User{ID: 22},
		APIKey:             key,
		Account:            &Account{ID: 33},
		IsSubscriptionBill: isSubscriptionBillingForAPIKey(key, nil),
	})

	require.NotNil(t, cmd)
	require.Zero(t, cmd.BalanceCost)
	require.Equal(t, 1.25, cmd.SubscriptionCost)
	require.Equal(t, organizationSubscriptionID, *cmd.OrganizationSubscriptionID)
	require.Nil(t, cmd.SubscriptionID)
}

func TestEnterpriseSubscriptionSnapshotsUsageBalanceSource(t *testing.T) {
	organizationSubscriptionID := int64(501)
	usageLog := &UsageLog{}
	resolved := &BillingContext{BalanceSource: "allocated"}
	snapshotEnterpriseSubscriptionSource(usageLog, &postUsageBillingParams{
		IsSubscriptionBill: true,
		APIKey:             &APIKey{OrganizationSubscriptionID: &organizationSubscriptionID},
	}, resolved)

	require.Equal(t, "subscription", resolved.BalanceSource)
	require.NotNil(t, usageLog.BalanceSource)
	require.Equal(t, "subscription", *usageLog.BalanceSource)
}
