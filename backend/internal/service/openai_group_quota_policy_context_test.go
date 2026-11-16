//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenAIQuotaPolicyPricingRestrictionPreservesOriginalIdentity(t *testing.T) {
	groupID := int64(10)
	for _, tc := range []struct {
		name       string
		source     string
		priced     string
		restricted bool
	}{
		{"requested allowed although target unpriced", BillingModelSourceRequested, "public-model", false},
		{"requested blocked although target priced", BillingModelSourceRequested, "policy-target", true},
		{"channel mapped allowed although target unpriced", BillingModelSourceChannelMapped, "channel-model", false},
		{"channel mapped blocked although target priced", BillingModelSourceChannelMapped, "policy-target", true},
		{"response pricing uses frozen mapped precheck", BillingModelSourceResponse, "channel-model", false},
		{"upstream defers to actual account target", BillingModelSourceUpstream, "policy-target", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			channel := Channel{
				ID: 1, Status: StatusActive, GroupIDs: []int64{groupID},
				RestrictModels: true, BillingModelSource: tc.source,
				ModelPricing: []ChannelModelPricing{{Platform: PlatformOpenAI, Models: []string{tc.priced}}},
				ModelMapping: map[string]map[string]string{PlatformOpenAI: {
					"public-model": "channel-model", "policy-target": "must-not-map-again",
				}},
			}
			svc := &OpenAIGatewayService{channelService: newTestChannelService(makeStandardRepo(channel, map[int64]string{groupID: PlatformOpenAI}))}
			mapping := svc.channelService.ResolveChannelMapping(context.Background(), groupID, "public-model")
			ctx := WithOpenAIQuotaPolicyPricingContext(context.Background(), "public-model", mapping)
			mapping.MappedModel = "later-mutation"
			require.Equal(t, tc.restricted, svc.checkChannelPricingRestriction(ctx, &groupID, "policy-target"))
		})
	}
}

func TestOpenAIQuotaPolicyPricingRestrictionKeepsActualUpstreamCheck(t *testing.T) {
	groupID := int64(10)
	channel := Channel{
		ID: 1, Status: StatusActive, GroupIDs: []int64{groupID},
		RestrictModels: true, BillingModelSource: BillingModelSourceUpstream,
		ModelPricing: []ChannelModelPricing{{Platform: PlatformOpenAI, Models: []string{"actual-target"}}},
	}
	svc := &OpenAIGatewayService{channelService: newTestChannelService(makeStandardRepo(channel, map[int64]string{groupID: PlatformOpenAI}))}
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{
		"model_mapping": map[string]any{"policy-target": "actual-target", "public-model": "blocked-original-target"},
	}}
	mapping := ChannelMappingResult{MappedModel: "public-model", BillingModelSource: BillingModelSourceUpstream}
	ctx := WithOpenAIQuotaPolicyPricingContext(context.Background(), "public-model", mapping)
	require.False(t, svc.checkChannelPricingRestriction(ctx, &groupID, "policy-target"))
	require.False(t, svc.isUpstreamModelRestrictedByChannel(ctx, groupID, account, "policy-target", false))
	require.True(t, svc.isUpstreamModelRestrictedByChannel(ctx, groupID, account, "public-model", false))
	ctx = WithOpenAIForwardModel(ctx, "policy-target", false)
	require.False(t, svc.isUpstreamModelRestrictedByChannel(ctx, groupID, account, "public-model", false))
}

func TestOpenAIQuotaPolicyPricingContextAbsentRetainsExistingChecks(t *testing.T) {
	groupID := int64(10)
	channel := Channel{
		ID: 1, Status: StatusActive, GroupIDs: []int64{groupID},
		RestrictModels: true, BillingModelSource: BillingModelSourceRequested,
		ModelPricing: []ChannelModelPricing{{Platform: PlatformOpenAI, Models: []string{"public-model"}}},
	}
	svc := &OpenAIGatewayService{channelService: newTestChannelService(makeStandardRepo(channel, map[int64]string{groupID: PlatformOpenAI}))}
	require.False(t, svc.checkChannelPricingRestriction(context.Background(), &groupID, "public-model"))
	require.True(t, svc.checkChannelPricingRestriction(context.Background(), &groupID, "policy-target"))
	_, ok := openAIQuotaPolicyPricingFromContext(nil)
	require.False(t, ok)
	_, ok = openAIQuotaPolicyPricingFromContext(WithOpenAIQuotaPolicyPricingContext(nil, "public-model", ChannelMappingResult{}))
	require.True(t, ok)
}
