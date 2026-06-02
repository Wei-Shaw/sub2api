//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMatchBillingMultiplierRule_GroupOnly(t *testing.T) {
	rule := &ChannelBillingMultiplierRule{
		GroupIDs:       []int64{10},
		RateMultiplier: 1.5,
	}

	require.True(t, matchBillingMultiplierRule(rule, 10, 999))
	require.False(t, matchBillingMultiplierRule(rule, 20, 999))
}

func TestMatchBillingMultiplierRule_GroupAndAccountAreAnd(t *testing.T) {
	rule := &ChannelBillingMultiplierRule{
		GroupIDs:       []int64{10},
		AccountIDs:     []int64{2},
		RateMultiplier: 2,
	}

	require.True(t, matchBillingMultiplierRule(rule, 10, 2))
	require.False(t, matchBillingMultiplierRule(rule, 10, 3))
	require.False(t, matchBillingMultiplierRule(rule, 11, 2))
}

func TestMatchBillingMultiplierRule_RequiresGroupScope(t *testing.T) {
	rule := &ChannelBillingMultiplierRule{
		AccountIDs:     []int64{2},
		RateMultiplier: 2,
	}

	require.False(t, matchBillingMultiplierRule(rule, 10, 2))
}

func TestResolveBillingMultiplier_FirstMatchingRuleWins(t *testing.T) {
	groupID := int64(10)
	channel := &Channel{
		ID:       1,
		Status:   StatusActive,
		GroupIDs: []int64{groupID},
		BillingMultiplierRules: []ChannelBillingMultiplierRule{
			{
				GroupIDs:       []int64{groupID},
				AccountIDs:     []int64{7},
				RateMultiplier: 0.5,
				SortOrder:      0,
			},
			{
				GroupIDs:       []int64{groupID},
				RateMultiplier: 2,
				SortOrder:      1,
			},
		},
	}
	cs := newTestChannelServiceForStats(t, channel, groupID, PlatformAnthropic)

	require.InDelta(t, 0.5, cs.ResolveBillingMultiplier(context.Background(), groupID, 7), 1e-12)
	require.InDelta(t, 2.0, cs.ResolveBillingMultiplier(context.Background(), groupID, 8), 1e-12)
	require.InDelta(t, 1.0, cs.ResolveBillingMultiplier(context.Background(), 99, 7), 1e-12)
}

func TestApplyChannelBillingMultiplier_CombinesWithBaseMultiplier(t *testing.T) {
	groupID := int64(10)
	channel := &Channel{
		ID:       1,
		Status:   StatusActive,
		GroupIDs: []int64{groupID},
		BillingMultiplierRules: []ChannelBillingMultiplierRule{
			{
				GroupIDs:       []int64{groupID},
				AccountIDs:     []int64{7},
				RateMultiplier: 1.25,
			},
		},
	}
	cs := newTestChannelServiceForStats(t, channel, groupID, PlatformAnthropic)

	actual := applyChannelBillingMultiplier(context.Background(), cs, &groupID, 7, 2)

	require.InDelta(t, 2.5, actual, 1e-12)
}
