package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAPIKeyAuthSnapshotGroupPricingRoundtrip(t *testing.T) {
	groupID := int64(50)
	inputPrice := 1e-6
	outputPrice := 2e-6
	apiKey := &APIKey{
		ID: 82, UserID: 40, GroupID: &groupID, Key: "sk-pricing-roundtrip", Status: StatusActive,
		User: &User{ID: 40, Status: StatusActive},
		Group: &Group{
			ID: groupID, Name: "pricing-roundtrip", Platform: PlatformAnthropic, Status: StatusActive,
			LongContextPricingEnabled: true,
			ModelPricing: []ChannelModelPricing{{
				Models: []string{"claude-sonnet-*"}, BillingMode: BillingModeToken,
				InputPrice: &inputPrice, OutputPrice: &outputPrice,
			}},
		},
	}
	svc := &APIKeyService{}

	payload, err := json.Marshal(&APIKeyAuthCacheEntry{Snapshot: svc.snapshotFromAPIKey(context.Background(), apiKey)})
	require.NoError(t, err)
	var cached APIKeyAuthCacheEntry
	require.NoError(t, json.Unmarshal(payload, &cached))

	materialized, used, err := svc.applyAuthCacheEntry(apiKey.Key, &cached)
	require.NoError(t, err)
	require.True(t, used)
	require.NotNil(t, materialized.Group)
	require.True(t, materialized.Group.LongContextPricingEnabled)
	require.Equal(t, apiKey.Group.ModelPricing, materialized.Group.ModelPricing)

	billing := &BillingService{fallbackPrices: map[string]*ModelPricing{
		"claude-sonnet-4": {InputPricePerToken: 3e-6, OutputPricePerToken: 15e-6},
	}}
	resolver := NewModelPricingResolver(nil, billing)
	resolved := resolver.Resolve(context.Background(), PricingInput{Model: "claude-sonnet-4", Group: materialized.Group})
	require.Equal(t, PricingSourceGroup, resolved.Source)
	require.True(t, resolved.longContextPricingEnabled)
	require.InDelta(t, inputPrice, resolved.BasePricing.InputPricePerToken, 1e-12)
	require.InDelta(t, outputPrice, resolved.BasePricing.OutputPricePerToken, 1e-12)
}
