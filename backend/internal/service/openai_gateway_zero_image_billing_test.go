//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCalculateOpenAIRecordUsageCost_ZeroImageBilling(t *testing.T) {
	for _, source := range []string{PricingSourceChannel, PricingSourceGroup} {
		for _, testCase := range []struct {
			name       string
			mode       BillingMode
			model      string
			imageCount int
			searches   int
			tokens     UsageTokens
			wantTotal  float64
			wantActual float64
		}{
			{name: "text_only", mode: BillingModeImage, tokens: UsageTokens{InputTokens: 1000, OutputTokens: 500}},
			{name: "tool_only", mode: BillingModeImage, tokens: UsageTokens{InputTokens: 200, OutputTokens: 100}},
			{name: "no_token_usage", mode: BillingModeImage},
			{name: "search_only", mode: BillingModeImage, searches: 100, wantTotal: 1, wantActual: 2},
			{name: "text_with_search", mode: BillingModeImage, searches: 100,
				tokens: UsageTokens{InputTokens: 1000, OutputTokens: 500}, wantTotal: 1, wantActual: 2},
			{name: "one_image", mode: BillingModeImage, imageCount: 1, wantTotal: 3, wantActual: 3},
			{name: "multiple_images", mode: BillingModeImage, imageCount: 2, wantTotal: 6, wantActual: 6},
			{name: "non_image_per_request", mode: BillingModePerRequest, model: "text-per-request",
				tokens: UsageTokens{InputTokens: 1000, OutputTokens: 500}, wantTotal: 3, wantActual: 3.9},
			{name: "per_request_with_search", mode: BillingModePerRequest, model: "text-per-request", searches: 100,
				wantTotal: 4, wantActual: 5.9},
			{name: "normal_tokens", mode: BillingModeToken, model: "text-token",
				tokens: UsageTokens{InputTokens: 1000, OutputTokens: 500}, wantTotal: 0.0105, wantActual: 0.01365},
			{name: "image_alias_tokens", mode: BillingModeToken,
				tokens: UsageTokens{InputTokens: 1000, OutputTokens: 500}, wantTotal: 0.0105, wantActual: 0.01365},
			{name: "token_priced_image_output", mode: BillingModeToken, imageCount: 1,
				tokens: UsageTokens{InputTokens: 1000, OutputTokens: 500}, wantTotal: 0.0105, wantActual: 0.01365},
			{name: "tokens_with_search", mode: BillingModeToken, model: "text-token", searches: 100,
				tokens: UsageTokens{InputTokens: 1000, OutputTokens: 500}, wantTotal: 1.0105, wantActual: 2.01365},
		} {
			t.Run(source+"/"+testCase.name, func(t *testing.T) {
				groupID := int64(8949)
				model := testCase.model
				if model == "" {
					model = "gpt-image-2.5-sunburst"
				}
				resolver := newOpenAIImageChannelPricingResolverForTest(t, groupID, model, 3)
				pricing := ChannelModelPricing{
					Models: []string{model}, BillingMode: testCase.mode,
					PerRequestPrice: f64p(3), InputPrice: f64p(3e-6), OutputPrice: f64p(15e-6),
				}
				cache := resolver.channelService.cache.Load().(*channelCache)
				cache.pricingByGroupModel[channelModelKey{groupID: groupID, model: model}] = &pricing
				group := &Group{
					ID: groupID, Platform: PlatformOpenAI, Status: StatusActive, Hydrated: true,
					RateMultiplier: 1.3, ImageRateIndependent: true, ImageRateMultiplier: 1,
					SearchPricePer1k: f64p(10),
				}
				if source == PricingSourceGroup {
					group.ModelPricing = []ChannelModelPricing{pricing}
				}
				apiKey := &APIKey{ID: 1, GroupID: &groupID, Group: group}
				svc := &OpenAIGatewayService{billingService: resolver.billingService, resolver: resolver}
				result := &OpenAIForwardResult{
					Model: model, UpstreamModel: "gpt-5.6-luna", ImageCount: testCase.imageCount,
					SearchCount: testCase.searches,
				}
				if testCase.imageCount > 0 {
					result.ImageSize = "4K"
				}
				cost, err := svc.calculateOpenAIRecordUsageCost(context.Background(), result, apiKey,
					[]string{model}, 1.3, 1, 1, 2, testCase.tokens, "", boolPtr(false), false, time.Time{})
				require.NoError(t, err)
				require.NotNil(t, cost)
				require.Equal(t, string(testCase.mode), cost.BillingMode)
				require.InDelta(t, testCase.wantTotal, cost.TotalCost, 1e-12)
				require.InDelta(t, testCase.wantActual, cost.ActualCost, 1e-12)
			})
		}
	}
}

func TestCalculateCostUnified_UnknownRequestCountStillBillsOne(t *testing.T) {
	for _, mode := range []BillingMode{BillingModeImage, BillingModePerRequest} {
		t.Run(string(mode), func(t *testing.T) {
			resolver := NewModelPricingResolver(nil, newTestBillingService())
			cost, err := resolver.billingService.CalculateCostUnified(CostInput{
				Ctx: context.Background(), Resolver: resolver, RateMultiplier: 1.3,
				Resolved: &ResolvedPricing{Mode: mode, DefaultPerRequestPrice: 3},
			})
			require.NoError(t, err)
			require.InDelta(t, 3, cost.TotalCost, 1e-12)
			require.InDelta(t, 3.9, cost.ActualCost, 1e-12)
		})
	}
}

func TestOpenAIGatewayServiceRecordUsage_ZeroImageSubscriptionBilling(t *testing.T) {
	for _, testCase := range []struct {
		name       string
		imageCount int
		wantCost   float64
	}{
		{name: "zero_images"},
		{name: "one_image", imageCount: 1, wantCost: 3},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			groupID := int64(8949)
			model := "gpt-image-2.5-sunburst"
			usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
			billingRepo := &openAIRecordUsageBillingRepoStub{}
			svc := newOpenAIRecordUsageServiceForTest(usageRepo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil)
			svc.resolver = newOpenAIImageChannelPricingResolverForTest(t, groupID, model, 3)
			svc.usageBillingRepo = billingRepo
			result := &OpenAIForwardResult{
				RequestID: testCase.name, Model: model, UpstreamModel: "gpt-5.6-luna",
				ImageCount: testCase.imageCount, Stream: true, Duration: time.Second,
				Usage: OpenAIUsage{InputTokens: 1000, OutputTokens: 500},
			}
			if testCase.imageCount > 0 {
				result.ImageSize = "4K"
			}
			err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
				Result: result, OriginalModel: model, BillingModelSource: BillingModelSourceRequested,
				InboundEndpoint: "/v1/responses", UpstreamEndpoint: "/v1/responses",
				APIKey: &APIKey{ID: 100, GroupID: &groupID, Group: &Group{
					ID: groupID, Hydrated: true, SubscriptionType: SubscriptionTypeSubscription,
					RateMultiplier: 1.3, ImageRateIndependent: true, ImageRateMultiplier: 1,
					ImagePrice1K: f64p(1), ImagePrice2K: f64p(2), ImagePrice4K: f64p(3),
				}},
				User: &User{ID: 200}, Account: &Account{ID: 300, Platform: PlatformOpenAI, Type: AccountTypeOAuth},
				Subscription: &UserSubscription{ID: 99},
			})
			require.NoError(t, err)
			require.NotNil(t, usageRepo.lastLog)
			require.Equal(t, testCase.imageCount, usageRepo.lastLog.ImageCount)
			require.Equal(t, string(BillingModeImage), *usageRepo.lastLog.BillingMode)
			require.Equal(t, BillingTypeSubscription, usageRepo.lastLog.BillingType)
			require.InDelta(t, testCase.wantCost, usageRepo.lastLog.TotalCost, 1e-12)
			require.InDelta(t, testCase.wantCost, usageRepo.lastLog.ActualCost, 1e-12)
			require.NotNil(t, billingRepo.lastCmd)
			require.InDelta(t, testCase.wantCost, billingRepo.lastCmd.SubscriptionCost, 1e-12)
			require.Zero(t, billingRepo.lastCmd.BalanceCost)
		})
	}
}
