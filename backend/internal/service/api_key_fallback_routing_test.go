//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func fallbackRoutingGroup(id int64, subscription bool) *Group {
	subscriptionType := SubscriptionTypeStandard
	if subscription {
		subscriptionType = SubscriptionTypeSubscription
	}
	return &Group{ID: id, Status: StatusActive, Platform: PlatformAnthropic, SubscriptionType: subscriptionType}
}

func TestAPIKeyRoutingStateCrossBillingCandidates(t *testing.T) {
	primary := fallbackRoutingGroup(1, true)
	metered := fallbackRoutingGroup(2, false)
	subscription := &UserSubscription{ID: 101, UserID: 7, GroupID: primary.ID}
	apiKey := &APIKey{GroupID: &primary.ID, Group: primary}
	state := NewAPIKeyRoutingState(apiKey, []APIKeyRoutingCandidate{
		{Group: primary, Subscription: subscription},
		{Group: metered},
	})
	stableSubscription := state.SubscriptionRef()
	state.SetEligibilityChecker(func(_ context.Context, _ *APIKey, group *Group, _ *UserSubscription) error {
		if group.ID == primary.ID {
			return ErrMonthlyLimitExceeded
		}
		return nil
	})

	index, err := state.EnsureEligibleFrom(context.Background(), 0)
	require.NoError(t, err)
	require.Equal(t, 1, index)
	require.Equal(t, metered.ID, *apiKey.GroupID)
	require.Same(t, metered, apiKey.Group)
	require.Same(t, stableSubscription, state.SubscriptionRef())
	require.Zero(t, stableSubscription.ID, "metered fallback must clear the request subscription")
}

func TestAPIKeyRoutingStateMeteredToSubscription(t *testing.T) {
	primary := fallbackRoutingGroup(1, false)
	fallback := fallbackRoutingGroup(2, true)
	subscription := &UserSubscription{ID: 202, UserID: 7, GroupID: fallback.ID}
	apiKey := &APIKey{GroupID: &primary.ID, Group: primary}
	state := NewAPIKeyRoutingState(apiKey, []APIKeyRoutingCandidate{
		{Group: primary},
		{Group: fallback, Subscription: subscription},
	})
	stableSubscription := state.SubscriptionRef()
	state.SetEligibilityChecker(func(_ context.Context, _ *APIKey, group *Group, _ *UserSubscription) error {
		if group.ID == primary.ID {
			return ErrInsufficientBalance
		}
		return nil
	})

	index, err := state.EnsureEligibleFrom(context.Background(), 0)
	require.NoError(t, err)
	require.Equal(t, 1, index)
	require.Equal(t, fallback.ID, *apiKey.GroupID)
	require.Same(t, stableSubscription, state.SubscriptionRef())
	require.Equal(t, subscription.ID, stableSubscription.ID)
}

func TestAPIKeyRoutingStateStopsOnRequestWideError(t *testing.T) {
	primary := fallbackRoutingGroup(1, false)
	fallback := fallbackRoutingGroup(2, false)
	apiKey := &APIKey{GroupID: &primary.ID, Group: primary}
	state := NewAPIKeyRoutingState(apiKey, []APIKeyRoutingCandidate{{Group: primary}, {Group: fallback}})
	calls := 0
	state.SetEligibilityChecker(func(context.Context, *APIKey, *Group, *UserSubscription) error {
		calls++
		return ErrUserRPMExceeded
	})

	_, err := state.EnsureEligibleFrom(context.Background(), 0)
	require.ErrorIs(t, err, ErrUserRPMExceeded)
	require.Equal(t, 1, calls)
	require.Equal(t, primary.ID, *apiKey.GroupID)
}

func TestAPIKeyRoutingStateCommitPreventsFurtherGroupChanges(t *testing.T) {
	primary := fallbackRoutingGroup(1, false)
	fallback := fallbackRoutingGroup(2, true)
	apiKey := &APIKey{GroupID: &primary.ID, Group: primary}
	state := NewAPIKeyRoutingState(apiKey, []APIKeyRoutingCandidate{{Group: primary}, {Group: fallback}})

	require.True(t, state.Commit(0))
	require.False(t, state.Activate(1))
	require.Empty(t, state.Candidates(apiKey.GroupID))
	require.Equal(t, primary.ID, *apiKey.GroupID)
}

func TestAPIKeyFallbackErrorClassification(t *testing.T) {
	require.True(t, IsAPIKeyFallbackSelectionError(errors.Join(errors.New("wrapped"), ErrNoAvailableAccounts)))
	require.False(t, IsAPIKeyFallbackSelectionError(ErrBillingServiceUnavailable))
	require.True(t, IsAPIKeyFallbackCandidateUnavailable(ErrInsufficientBalance))
	require.True(t, IsAPIKeyFallbackCandidateUnavailable(ErrSubscriptionInvalid))
	require.False(t, IsAPIKeyFallbackCandidateUnavailable(context.Canceled))
	require.False(t, IsAPIKeyFallbackCandidateUnavailable(ErrBillingServiceUnavailable))
	require.False(t, IsAPIKeyFallbackCandidateUnavailable(ErrAPIKeyRateLimit1dExceeded))
}
