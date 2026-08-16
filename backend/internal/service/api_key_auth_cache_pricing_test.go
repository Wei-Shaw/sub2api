//go:build unit

package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func authPricingFloat64Ptr(v float64) *float64 { return &v }
func authPricingIntPtr(v int) *int             { return &v }

func TestAPIKeyAuthSnapshotPreservesGroupPricingPolicy(t *testing.T) {
	groupID := int64(100)
	apiKey := &APIKey{
		ID:      82,
		UserID:  40,
		GroupID: &groupID,
		Status:  StatusActive,
		User:    &User{ID: 40, Status: StatusActive},
		Group: &Group{
			ID:                        groupID,
			Name:                      "grok-heavy",
			Platform:                  PlatformGrok,
			Status:                    StatusActive,
			RateMultiplier:            1,
			LongContextPricingEnabled: true,
			ModelPricing: []ChannelModelPricing{{
				Platform:    PlatformGrok,
				Models:      []string{"grok-private"},
				BillingMode: BillingModeToken,
				InputPrice:  authPricingFloat64Ptr(3e-6),
			}},
		},
	}

	apiKeyService := &APIKeyService{}
	snapshot := apiKeyService.snapshotFromAPIKey(context.Background(), apiKey)
	require.NotNil(t, snapshot)

	payload, err := json.Marshal(&APIKeyAuthCacheEntry{Snapshot: snapshot})
	require.NoError(t, err)
	var cached APIKeyAuthCacheEntry
	require.NoError(t, json.Unmarshal(payload, &cached))

	materialized, used, err := apiKeyService.applyAuthCacheEntry("sk-test", &cached)
	require.NoError(t, err)
	require.True(t, used)
	require.NotNil(t, materialized.Group)
	require.True(t, materialized.Group.LongContextPricingEnabled)
	require.Equal(t, apiKey.Group.ModelPricing, materialized.Group.ModelPricing)

	channelPricing := []ChannelModelPricing{{
		Platform:       PlatformGrok,
		Models:         []string{"grok-4.6"},
		BillingMode:    BillingModeToken,
		InputPrice:     authPricingFloat64Ptr(2e-6),
		OutputPrice:    authPricingFloat64Ptr(6e-6),
		CacheReadPrice: authPricingFloat64Ptr(0.5e-6),
		Intervals: []PricingInterval{
			{
				MinTokens: 0, MaxTokens: authPricingIntPtr(199999),
				InputPrice: authPricingFloat64Ptr(2e-6), OutputPrice: authPricingFloat64Ptr(6e-6),
				CacheReadPrice: authPricingFloat64Ptr(0.5e-6),
			},
			{
				MinTokens:  199999,
				InputPrice: authPricingFloat64Ptr(4e-6), OutputPrice: authPricingFloat64Ptr(12e-6),
				CacheReadPrice: authPricingFloat64Ptr(1e-6),
			},
		},
	}}
	repo := &mockChannelRepository{
		listAllFn: func(context.Context) ([]Channel, error) {
			return []Channel{{
				ID: 6, Name: "Grok", Status: StatusActive,
				GroupIDs: []int64{groupID}, ModelPricing: channelPricing,
			}}, nil
		},
		getGroupPlatformsFn: func(context.Context, []int64) (map[int64]string, error) {
			return map[int64]string{groupID: PlatformGrok}, nil
		},
	}
	billingService := NewBillingService(&config.Config{}, nil)
	resolver := NewModelPricingResolver(NewChannelService(repo, nil, nil, nil), billingService)

	cost, err := billingService.CalculateCostUnified(CostInput{
		Ctx: context.Background(), Model: "grok-4.6", GroupID: &groupID, Group: materialized.Group,
		Tokens:         UsageTokens{InputTokens: 200191, CacheReadTokens: 128, OutputTokens: 1},
		RateMultiplier: 1, Resolver: resolver,
	})
	require.NoError(t, err)
	require.InDelta(t, 0.800764, cost.InputCost, 1e-12)
	require.InDelta(t, 0.000128, cost.CacheReadCost, 1e-12)
	require.InDelta(t, 0.000012, cost.OutputCost, 1e-12)
}
