package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRankAutoGroupCandidates_UsesBalanceGroupsAndUserRate(t *testing.T) {
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	candidates := []AutoGroupCandidate{
		{Group: &Group{ID: 1, SubscriptionType: SubscriptionTypeStandard, RateMultiplier: 0.8, SortOrder: 20}, Available: true},
		{Group: &Group{ID: 2, SubscriptionType: SubscriptionTypeSubscription, RateMultiplier: 0.01}, Available: true},
		{Group: &Group{ID: 3, SubscriptionType: SubscriptionTypeStandard, RateMultiplier: 0.7, SortOrder: 30}, Available: true},
	}

	ranked := rankAutoGroupCandidates(candidates, map[int64]float64{1: 0.5}, now, 0)

	require.Len(t, ranked, 2)
	require.Equal(t, int64(1), ranked[0].Group.ID)
	require.Equal(t, 0.5, ranked[0].EffectiveRate)
	require.Equal(t, int64(3), ranked[1].Group.ID)
}

func TestRankAutoGroupCandidates_AppliesPeakAndStableTieBreak(t *testing.T) {
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	candidates := []AutoGroupCandidate{
		{Group: &Group{ID: 9, SubscriptionType: SubscriptionTypeStandard, RateMultiplier: 0.5, SortOrder: 2}, Available: true},
		{Group: &Group{ID: 7, SubscriptionType: SubscriptionTypeStandard, RateMultiplier: 0.5, SortOrder: 1}, Available: true},
		{Group: &Group{ID: 6, SubscriptionType: SubscriptionTypeStandard, RateMultiplier: 0.5, SortOrder: 1}, Available: true},
		{Group: &Group{ID: 4, SubscriptionType: SubscriptionTypeStandard, RateMultiplier: 0.1, SortOrder: 0}, Available: false},
	}

	ranked := rankAutoGroupCandidates(candidates, nil, now, 0)

	require.Equal(t, []int64{6, 7, 9}, []int64{ranked[0].Group.ID, ranked[1].Group.ID, ranked[2].Group.ID})
}

func TestRankAutoGroupCandidates_PrefersEligibleStickyGroup(t *testing.T) {
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	candidates := []AutoGroupCandidate{
		{Group: &Group{ID: 1, SubscriptionType: SubscriptionTypeStandard, RateMultiplier: 0.2}, Available: true},
		{Group: &Group{ID: 2, SubscriptionType: SubscriptionTypeStandard, RateMultiplier: 0.4}, Available: true},
	}

	ranked := rankAutoGroupCandidates(candidates, nil, now, 2)
	require.Equal(t, int64(2), ranked[0].Group.ID)

	candidates[1].Available = false
	ranked = rankAutoGroupCandidates(candidates, nil, now, 2)
	require.Equal(t, int64(1), ranked[0].Group.ID)
}
