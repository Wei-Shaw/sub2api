package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type autoGroupAccountRepoStub struct {
	AccountRepository
	accountsByGroup map[int64][]Account
}

func (s *autoGroupAccountRepoStub) ListSchedulableByGroupIDAndPlatforms(_ context.Context, groupID int64, _ []string) ([]Account, error) {
	return append([]Account(nil), s.accountsByGroup[groupID]...), nil
}

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

func TestAutoGroupHasAvailableAccount_RejectsChannelRestrictedModel(t *testing.T) {
	const groupID int64 = 2
	channelSvc := newTestChannelService(makeStandardRepo(Channel{
		ID:                 3,
		Status:             StatusActive,
		GroupIDs:           []int64{groupID},
		RestrictModels:     true,
		BillingModelSource: BillingModelSourceRequested,
		ModelPricing: []ChannelModelPricing{
			{Platform: PlatformAnthropic, Models: []string{"claude-opus-4-8"}},
		},
	}, map[int64]string{groupID: PlatformAnthropic}))
	svc := &GatewayService{
		accountRepo: &autoGroupAccountRepoStub{accountsByGroup: map[int64][]Account{
			groupID: {{
				ID:          15,
				Platform:    PlatformAnthropic,
				Status:      StatusActive,
				Schedulable: true,
				Credentials: map[string]any{"model_mapping": map[string]any{
					"claude-fable-5": "claude-fable-5",
				}},
			}},
		}},
		channelService: channelSvc,
	}

	available, err := svc.autoGroupHasAvailableAccount(context.Background(), &Group{
		ID: groupID, Platform: PlatformAnthropic,
	}, "claude-fable-5")

	require.NoError(t, err)
	require.False(t, available)
}
