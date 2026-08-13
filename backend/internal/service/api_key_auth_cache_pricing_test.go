package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAPIKeyAuthSnapshotGroupPricingRoundtrip(t *testing.T) {
	groupID := int64(71)
	inputPrice := 1.25e-6
	apiKey := &APIKey{
		ID:      81,
		UserID:  41,
		GroupID: &groupID,
		Status:  StatusActive,
		User:    &User{ID: 41, Status: StatusActive},
		Group: &Group{
			ID:                        groupID,
			Name:                      "priced-group",
			Platform:                  PlatformOpenAI,
			Status:                    StatusActive,
			LongContextPricingEnabled: true,
			ModelPricing: []ChannelModelPricing{{
				Platform:    PlatformOpenAI,
				Models:      []string{"gpt-5.4"},
				BillingMode: BillingModeToken,
				InputPrice:  &inputPrice,
			}},
		},
	}

	svc := &APIKeyService{}
	snapshot := svc.snapshotFromAPIKey(context.Background(), apiKey)
	require.NotNil(t, snapshot)
	require.Equal(t, apiKeyAuthSnapshotVersion, snapshot.Version)

	payload, err := json.Marshal(&APIKeyAuthCacheEntry{Snapshot: snapshot})
	require.NoError(t, err)
	var restored APIKeyAuthCacheEntry
	require.NoError(t, json.Unmarshal(payload, &restored))

	materialized, used, err := svc.applyAuthCacheEntry("sk-pricing", &restored)
	require.NoError(t, err)
	require.True(t, used)
	require.NotNil(t, materialized.Group)
	require.True(t, materialized.Group.LongContextPricingEnabled)
	require.Len(t, materialized.Group.ModelPricing, 1)
	require.Equal(t, []string{"gpt-5.4"}, materialized.Group.ModelPricing[0].Models)
	require.NotNil(t, materialized.Group.ModelPricing[0].InputPrice)
	require.InDelta(t, inputPrice, *materialized.Group.ModelPricing[0].InputPrice, 1e-12)

	matched := matchGroupModelPricing(materialized.Group, "gpt-5.4")
	require.NotNil(t, matched, "materialized auth group must retain group-level pricing")
}
