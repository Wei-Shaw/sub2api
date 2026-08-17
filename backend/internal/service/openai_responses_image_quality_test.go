package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestResolveOpenAIResponsesImageQualityPrecedence(t *testing.T) {
	require.Equal(t, ImageQualityHigh, resolveOpenAIResponsesImageQuality("high", "low", "medium"))
	require.Equal(t, ImageQualityLow, resolveOpenAIResponsesImageQuality("", "low", "high"))
	require.Equal(t, ImageQualityHigh, resolveOpenAIResponsesImageQuality("", "", "high"))
	require.Equal(t, ImageQualityMedium, resolveOpenAIResponsesImageQuality("", "", ""))
}

func TestChannelResponsesDefaultImageQuality(t *testing.T) {
	require.Equal(t, ImageQualityMedium, (*Channel)(nil).ResponsesDefaultImageQuality(PlatformOpenAI))
	require.Equal(t, ImageQualityMedium, (&Channel{}).ResponsesDefaultImageQuality(PlatformOpenAI))
	require.Equal(t, ImageQualityLow, (&Channel{FeaturesConfig: map[string]any{
		featureKeyResponsesDefaultImageQuality: map[string]any{PlatformOpenAI: "LOW"},
	}}).ResponsesDefaultImageQuality(PlatformOpenAI))
	require.Equal(t, ImageQualityMedium, (&Channel{FeaturesConfig: map[string]any{
		featureKeyResponsesDefaultImageQuality: map[string]any{PlatformOpenAI: "invalid"},
	}}).ResponsesDefaultImageQuality(PlatformOpenAI))
}

func TestCalculateOpenAIImageCostUsesResolvedQualityTier(t *testing.T) {
	groupID := int64(42)
	lowPrice := 1.25
	mediumPrice := 2.5
	highPrice := 5.0
	cache := newEmptyChannelCache()
	cache.pricingByGroupModel[channelModelKey{groupID: groupID, model: "gpt-image-2"}] = &ChannelModelPricing{
		BillingMode: BillingModeImage,
		Intervals: []PricingInterval{
			{TierLabel: "1K", Quality: ImageQualityLow, PerRequestPrice: &lowPrice},
			{TierLabel: "1K", Quality: ImageQualityMedium, PerRequestPrice: &mediumPrice},
			{TierLabel: "1K", Quality: ImageQualityHigh, PerRequestPrice: &highPrice},
		},
	}
	cache.channelByGroupID[groupID] = &Channel{ID: 9, Status: StatusActive}
	cache.loadedAt = time.Now()
	channelService := &ChannelService{}
	channelService.cache.Store(cache)
	billingService := NewBillingService(&config.Config{}, nil)
	gateway := &OpenAIGatewayService{
		billingService: billingService,
		resolver:       NewModelPricingResolver(channelService, billingService),
	}
	apiKey := &APIKey{GroupID: &groupID, Group: &Group{ID: groupID}}

	cost := gateway.calculateOpenAIImageCost(context.Background(), "gpt-image-2", apiKey, &OpenAIForwardResult{
		ImageCount:   1,
		ImageSize:    "1K",
		ImageQuality: ImageQualityMedium,
	}, 1)

	require.NotNil(t, cost)
	require.InDelta(t, mediumPrice, cost.TotalCost, 1e-12)
}
