//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCheckAPIKeyMaxRateMultiplierUsesUserAndPeakRates(t *testing.T) {
	groupID := int64(42)
	userRate := 2.0
	max := 5.9
	group := &Group{
		ID:                 groupID,
		RateMultiplier:     2.5,
		SubscriptionType:   SubscriptionTypeSubscription,
		PeakRateEnabled:    true,
		PeakStart:          "09:00",
		PeakEnd:            "18:00",
		PeakRateMultiplier: 3,
	}
	svc := NewAPIKeyService(nil, nil, nil, nil, &userGroupRateResolverRepoStub{rate: &userRate}, nil, nil)
	key := &APIKey{UserID: 7, GroupID: &groupID, Group: group, MaxRateMultiplier: &max}

	current, maximum, exceeded := svc.CheckAPIKeyMaxRateMultiplier(context.Background(), key, time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC))
	require.Equal(t, 6.0, current)
	require.Equal(t, max, maximum)
	require.True(t, exceeded)
}

func TestCheckAPIKeyMaxRateMultiplierAllowsUnsetAndAtLimit(t *testing.T) {
	groupID := int64(42)
	max := 2.5
	group := &Group{ID: groupID, RateMultiplier: 2.5}
	svc := NewAPIKeyService(nil, nil, nil, nil, nil, nil, nil)

	current, maximum, exceeded := svc.CheckAPIKeyMaxRateMultiplier(context.Background(), &APIKey{GroupID: &groupID, Group: group}, time.Now())
	require.Zero(t, current)
	require.Zero(t, maximum)
	require.False(t, exceeded)

	current, maximum, exceeded = svc.CheckAPIKeyMaxRateMultiplier(context.Background(), &APIKey{GroupID: &groupID, Group: group, MaxRateMultiplier: &max}, time.Now())
	require.Equal(t, 2.5, current)
	require.Equal(t, max, maximum)
	require.False(t, exceeded)
}
