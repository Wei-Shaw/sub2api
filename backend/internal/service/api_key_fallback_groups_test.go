//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type fallbackValidationUserSubRepo struct {
	userSubRepoNoop
	active map[int64]*UserSubscription
}

func (r *fallbackValidationUserSubRepo) GetActiveByUserIDAndGroupID(_ context.Context, userID, groupID int64) (*UserSubscription, error) {
	subscription := r.active[groupID]
	if subscription == nil || subscription.UserID != userID {
		return nil, ErrSubscriptionNotFound
	}
	clone := *subscription
	return &clone, nil
}

func TestValidateAPIKeyFallbackGroups(t *testing.T) {
	groups := map[int64]*Group{
		1: {ID: 1, Status: StatusActive, Platform: PlatformOpenAI, SubscriptionType: SubscriptionTypeStandard},
		2: {ID: 2, Status: StatusActive, Platform: PlatformOpenAI, SubscriptionType: SubscriptionTypeStandard},
		3: {ID: 3, Status: StatusActive, Platform: PlatformOpenAI, SubscriptionType: SubscriptionTypeStandard, IsExclusive: true},
		4: {ID: 4, Status: StatusDisabled, Platform: PlatformOpenAI, SubscriptionType: SubscriptionTypeStandard},
		5: {ID: 5, Status: StatusActive, Platform: PlatformOpenAI, SubscriptionType: SubscriptionTypeSubscription},
		6: {ID: 6, Status: StatusActive, Platform: PlatformAnthropic, SubscriptionType: SubscriptionTypeStandard},
	}
	svc := &APIKeyService{
		groupRepo: &groupRepoStubForAdmin{getByIDByID: groups},
		userSubRepo: &fallbackValidationUserSubRepo{active: map[int64]*UserSubscription{
			5: {ID: 50, UserID: 7, GroupID: 5, Status: StatusActive},
		}},
	}
	user := &User{ID: 7, AllowedGroups: []int64{3}}
	primary := int64(1)

	tests := []struct {
		name string
		ids  []int64
		want error
	}{
		{name: "ordered metered and subscription groups", ids: []int64{2, 5, 3}},
		{name: "more than five", ids: []int64{2, 3, 4, 5, 6, 7}, want: ErrInvalidFallbackGroups},
		{name: "duplicate", ids: []int64{2, 2}, want: ErrInvalidFallbackGroups},
		{name: "contains primary", ids: []int64{2, 1}, want: ErrInvalidFallbackGroups},
		{name: "disabled", ids: []int64{4}, want: ErrGroupNotAllowed},
		{name: "different platform", ids: []int64{6}, want: ErrInvalidFallbackGroups},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := svc.validateAPIKeyFallbackGroups(context.Background(), user, &primary, tt.ids)
			if tt.want == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, tt.want)
		})
	}

	unauthorized := *user
	unauthorized.AllowedGroups = nil
	require.ErrorIs(t, svc.validateAPIKeyFallbackGroups(context.Background(), &unauthorized, &primary, []int64{3}), ErrGroupNotAllowed)
}

func TestCreateAPIKeyRejectsFallbackGroupsForEnterpriseKey(t *testing.T) {
	organizationSubscriptionID := int64(90)
	svc := &APIKeyService{
		userRepo: &fallbackValidationUserRepo{user: &User{ID: 7, Status: StatusActive}},
	}

	_, err := svc.Create(context.Background(), 7, CreateAPIKeyRequest{
		OrganizationSubscriptionID: &organizationSubscriptionID,
		FallbackGroupIDs:           []int64{2},
	})
	require.ErrorIs(t, err, ErrInvalidFallbackGroups)
}

func TestResolveAPIKeyRoutingCandidatesRejectsCrossPlatformRuntimeData(t *testing.T) {
	primary := &Group{ID: 1, Status: StatusActive, Platform: PlatformOpenAI}
	crossPlatform := &Group{ID: 2, Status: StatusActive, Platform: PlatformAnthropic}
	samePlatform := &Group{ID: 3, Status: StatusActive, Platform: PlatformOpenAI}
	svc := &APIKeyService{groupRepo: &groupRepoStubForAdmin{getByIDByID: map[int64]*Group{
		primary.ID:       primary,
		crossPlatform.ID: crossPlatform,
		samePlatform.ID:  samePlatform,
	}}}
	apiKey := &APIKey{
		GroupID:          &primary.ID,
		Group:            primary,
		FallbackGroupIDs: []int64{crossPlatform.ID, samePlatform.ID},
	}

	candidates := svc.ResolveAPIKeyRoutingCandidates(context.Background(), apiKey)
	require.Len(t, candidates, 3)
	require.NoError(t, candidates[0].Unavailable)
	require.ErrorIs(t, candidates[1].Unavailable, ErrGroupNotAllowed)
	require.NoError(t, candidates[2].Unavailable)
	require.Equal(t, samePlatform.ID, candidates[2].Group.ID)
}

type fallbackValidationUserRepo struct {
	*userRepoStubForGroupUpdate
	user *User
}

func (r *fallbackValidationUserRepo) GetByID(context.Context, int64) (*User, error) {
	clone := *r.user
	return &clone, nil
}
