package dto

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestRedeemCodeFromService_MapsUsagePolicyFields(t *testing.T) {
	maxTotalUses := 50
	maxUsesPerUser := 1
	rc := &service.RedeemCode{
		ID:             7,
		Code:           "CAMPAIGN-A",
		Type:           service.RedeemTypeBalance,
		Value:          25,
		Status:         service.StatusUnused,
		UsagePolicy:    service.RedeemUsagePolicyOncePerUser,
		UsageScope:     "campaign-2026",
		MaxTotalUses:   &maxTotalUses,
		MaxUsesPerUser: &maxUsesPerUser,
		UsedCount:      3,
	}

	out := RedeemCodeFromService(rc)
	require.NotNil(t, out)
	require.Equal(t, service.RedeemUsagePolicyOncePerUser, out.UsagePolicy)
	require.Equal(t, "campaign-2026", out.UsageScope)
	require.Equal(t, &maxTotalUses, out.MaxTotalUses)
	require.Equal(t, &maxUsesPerUser, out.MaxUsesPerUser)
	require.Equal(t, 3, out.UsedCount)
}
