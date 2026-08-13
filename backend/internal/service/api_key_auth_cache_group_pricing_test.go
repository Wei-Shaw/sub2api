package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAPIKeyAuthSnapshotGroupPricingRoundTrip(t *testing.T) {
	groupID := int64(71)
	maxTokens := 200000
	inputPrice := 1.25e-6
	outputPrice := 5e-6
	longContextInputPrice := 2.5e-6
	apiKey := &APIKey{
		ID:      82,
		UserID:  40,
		GroupID: &groupID,
		Status:  StatusActive,
		User: &User{
			ID:     40,
			Status: StatusActive,
			Role:   RoleUser,
		},
		Group: &Group{
			ID:                        groupID,
			Name:                      "group-pricing-roundtrip",
			Platform:                  PlatformOpenAI,
			Status:                    StatusActive,
			SubscriptionType:          SubscriptionTypeStandard,
			RateMultiplier:            1,
			LongContextPricingEnabled: true,
			ModelPricing: []ChannelModelPricing{{
				Platform:    PlatformOpenAI,
				Models:      []string{"gpt-cache-test-*"},
				BillingMode: BillingModeToken,
				InputPrice:  &inputPrice,
				OutputPrice: &outputPrice,
				Intervals: []PricingInterval{{
					MinTokens:  0,
					MaxTokens:  &maxTokens,
					TierLabel:  "base",
					InputPrice: &longContextInputPrice,
				}},
			}},
		},
	}
	svc := &APIKeyService{}

	snapshot := svc.snapshotFromAPIKey(context.Background(), apiKey)
	require.NotNil(t, snapshot)
	require.NotNil(t, snapshot.Group)
	require.Equal(t, apiKeyAuthSnapshotVersion, snapshot.Version)

	// Redis L2 stores this entry as JSON, so exercise the real serialization boundary.
	payload, err := json.Marshal(&APIKeyAuthCacheEntry{Snapshot: snapshot})
	require.NoError(t, err)
	var cached APIKeyAuthCacheEntry
	require.NoError(t, json.Unmarshal(payload, &cached))

	materialized, used, err := svc.applyAuthCacheEntry("sk-group-pricing", &cached)
	require.NoError(t, err)
	require.True(t, used)
	require.NotNil(t, materialized)
	require.NotNil(t, materialized.Group)
	require.True(t, materialized.Group.LongContextPricingEnabled)
	require.Equal(t, apiKey.Group.ModelPricing, materialized.Group.ModelPricing)
	require.Len(t, materialized.Group.ModelPricing, 1)
	require.Len(t, materialized.Group.ModelPricing[0].Intervals, 1)

	// The request-side resolver must still select group pricing after a cache hit.
	resolver := NewModelPricingResolver(nil, &BillingService{fallbackPrices: map[string]*ModelPricing{
		"gpt-cache-test-2026": {
			InputPricePerToken:  9e-6,
			OutputPricePerToken: 9e-6,
		},
	}})
	resolved := resolver.Resolve(context.Background(), PricingInput{
		Model: "gpt-cache-test-2026",
		Group: materialized.Group,
	})
	require.Equal(t, PricingSourceGroup, resolved.Source)
	require.True(t, resolved.longContextPricingEnabled)
	require.NotNil(t, resolved.BasePricing)
	require.InDelta(t, inputPrice, resolved.BasePricing.InputPricePerToken, 1e-12)
	require.InDelta(t, outputPrice, resolved.BasePricing.OutputPricePerToken, 1e-12)
}
