package dto

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestGroupFromServiceUserHidesInternalRateMultiplier(t *testing.T) {
	displayRate := 0.9
	group := &service.Group{
		ID:                    9,
		Name:                  "订阅套餐",
		Platform:              service.PlatformOpenAI,
		RateMultiplier:        3.0,
		DisplayRateMultiplier: &displayRate,
		SubscriptionType:      service.SubscriptionTypeSubscription,
	}

	out := GroupFromServiceUser(group)

	require.NotNil(t, out)
	require.Equal(t, 1.0, out.RateMultiplier)
	require.NotNil(t, out.DisplayRateMultiplier)
	require.Equal(t, 1.0, *out.DisplayRateMultiplier)
}

func TestAPIKeyFromServiceHidesGroupInternalRateMultiplier(t *testing.T) {
	group := &service.Group{
		ID:             2,
		Name:           "codex-pro",
		Platform:       service.PlatformOpenAI,
		RateMultiplier: 3.0,
	}
	key := &service.APIKey{
		ID:      1,
		UserID:  1,
		Name:    "test",
		Key:     "sk-test",
		Status:  service.StatusActive,
		GroupID: &group.ID,
		Group:   group,
	}

	out := APIKeyFromService(key)

	require.NotNil(t, out)
	require.NotNil(t, out.Group)
	require.Equal(t, 1.0, out.Group.RateMultiplier)
	require.NotNil(t, out.Group.DisplayRateMultiplier)
	require.Equal(t, 1.0, *out.Group.DisplayRateMultiplier)
}

func TestUsageLogFromServiceUserHidesRateMultiplier(t *testing.T) {
	log := &service.UsageLog{
		ID:             1,
		UserID:         1,
		RequestID:      "req-test",
		Model:          "gpt-5.5",
		RateMultiplier: 3.0,
		Group: &service.Group{
			ID:             9,
			Name:           "订阅套餐",
			Platform:       service.PlatformOpenAI,
			RateMultiplier: 3.0,
		},
	}

	out := UsageLogFromService(log)

	require.NotNil(t, out)
	require.Equal(t, 1.0, out.RateMultiplier)
	require.NotNil(t, out.Group)
	require.Equal(t, 1.0, out.Group.RateMultiplier)
}
