//go:build unit

package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRecordUsage_ImageReasoningPricing(t *testing.T) {
	for _, platform := range []string{PlatformAnthropic, PlatformOpenAI} {
		for _, source := range []string{PricingSourceChannel, PricingSourceGroup} {
			for _, mode := range []BillingMode{BillingModeImage, BillingModePerRequest} {
				for _, independent := range []bool{false, true} {
					for _, effort := range []string{"high", "low", ""} {
						name := fmt.Sprintf("%s/%s/%s/independent=%t/effort=%s", platform, source, mode, independent, effort)
						t.Run(name, func(t *testing.T) {
							usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
							userRepo := &openAIRecordUsageUserRepoStub{}
							subRepo := &openAIRecordUsageSubRepoStub{}
							groupID := int64(77)
							model := "gemini-3-pro-image-preview"
							price := 0.25
							pricing := ChannelModelPricing{
								Models: []string{model}, BillingMode: mode, PerRequestPrice: &price,
								ReasoningEffortMultipliers: map[string]float64{"high": 2},
							}
							resolver := newOpenAIImageChannelPricingResolverForTest(t, groupID, model, price)
							cache := resolver.channelService.cache.Load().(*channelCache)
							cache.pricingByGroupModel[channelModelKey{groupID: groupID, model: model}] = &pricing
							group := &Group{
								ID: groupID, Platform: platform, Status: StatusActive, Hydrated: true,
								RateMultiplier: 0.5, ImageRateIndependent: independent, ImageRateMultiplier: 0.25,
							}
							if source == PricingSourceGroup {
								group.ModelPricing = []ChannelModelPricing{pricing}
								// A matching group card owns the multiplier; the channel must not stack on top.
								pricing.ReasoningEffortMultipliers = map[string]float64{"high": 3}
							}
							apiKey := &APIKey{ID: 10, GroupID: &groupID, Group: group}
							account := &Account{ID: 30, Platform: platform, Type: AccountTypeAPIKey}
							user := &User{ID: 20}
							var forwardedEffort *string
							if effort != "" {
								forwardedEffort = &effort
							}
							if platform == PlatformAnthropic {
								svc := newGatewayRecordUsageServiceForTest(usageRepo, userRepo, subRepo)
								svc.resolver = resolver
								require.NoError(t, svc.RecordUsage(context.Background(), &RecordUsageInput{
									Result: &ForwardResult{
										RequestID: "image_reasoning_pricing", Model: model, Duration: time.Second,
										ImageCount: 2, ImageSize: "1K", ReasoningEffort: forwardedEffort,
									},
									APIKey: apiKey, User: user, Account: account,
								}))
							} else {
								svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, subRepo, nil)
								svc.resolver = resolver
								require.NoError(t, svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
									Result: &OpenAIForwardResult{
										RequestID: "image_reasoning_pricing", Model: model, Duration: time.Second,
										ImageCount: 2, ImageSize: "1K", ReasoningEffort: forwardedEffort,
									},
									APIKey: apiKey, User: user, Account: account,
								}))
							}
							wantTotal := 0.5
							if effort == "high" {
								wantTotal *= 2
							}
							wantRate := 0.5
							if independent {
								wantRate = 0.25
							}
							require.NotNil(t, usageRepo.lastLog)
							require.InDelta(t, wantTotal, usageRepo.lastLog.TotalCost, 1e-12)
							require.InDelta(t, wantTotal*wantRate, usageRepo.lastLog.ActualCost, 1e-12)
							require.InDelta(t, wantTotal*wantRate, userRepo.lastAmount, 1e-12)
							require.Equal(t, forwardedEffort, usageRepo.lastLog.ReasoningEffort)
							require.Equal(t, 2, usageRepo.lastLog.ImageCount)
							require.Equal(t, string(mode), *usageRepo.lastLog.BillingMode)
						})
					}
				}
			}
		}
	}
}

func TestCalculateRecordUsageCost_MediaReasoningPricing(t *testing.T) {
	for _, source := range []string{PricingSourceChannel, PricingSourceGroup} {
		for _, media := range []string{"gateway_audio", "openai_audio", "openai_video"} {
			t.Run(source+"/"+media, func(t *testing.T) {
				groupID := int64(77)
				model, effort := "media-model", "high"
				price := 0.25
				resolver := newOpenAIImageChannelPricingResolverForTest(t, groupID, model, price)
				cache := resolver.channelService.cache.Load().(*channelCache)
				pricing := cache.pricingByGroupModel[channelModelKey{groupID: groupID, model: model}]
				pricing.BillingMode = BillingModePerRequest
				pricing.ReasoningEffortMultipliers = map[string]float64{"high": 2}
				wantTotal := 1.0 // Two units at $0.25, with high=2.
				if media == "openai_video" {
					pricing.BillingMode = BillingModeVideo
					wantTotal = 5 // Two five-second videos at $0.25/second, with high=2.
				}
				apiKey := &APIKey{GroupID: &groupID, Group: &Group{ID: groupID, Hydrated: true}}
				if source == PricingSourceGroup {
					pricing.Models = []string{model}
					apiKey.Group.ModelPricing = []ChannelModelPricing{*pricing}
					pricing.ReasoningEffortMultipliers = map[string]float64{"high": 3}
				}
				var cost *CostBreakdown
				if media == "gateway_audio" {
					svc := &GatewayService{billingService: resolver.billingService, resolver: resolver}
					cost = svc.calculateRecordUsageCost(context.Background(), &ForwardResult{
						ReasoningEffort: &effort, AudioUsage: &AudioUsage{Mode: "tts", DurationOrUnits: 2},
					}, apiKey, model, 0.5, 0.5, time.Time{})
				} else {
					svc := &OpenAIGatewayService{billingService: resolver.billingService, resolver: resolver}
					result := &OpenAIForwardResult{ReasoningEffort: &effort}
					if media == "openai_audio" {
						result.AudioUsage = &AudioUsage{Mode: "tts", DurationOrUnits: 2}
					} else {
						result.VideoCount, result.VideoDurationSeconds = 2, 5
					}
					var err error
					cost, err = svc.calculateOpenAIRecordUsageCost(context.Background(), result, apiKey,
						[]string{model}, 0.5, 0.5, 0.5, 0.5, UsageTokens{}, "", nil, time.Time{}, "")
					require.NoError(t, err)
				}
				require.NotNil(t, cost)
				require.InDelta(t, wantTotal, cost.TotalCost, 1e-12)
				require.InDelta(t, wantTotal*0.5, cost.ActualCost, 1e-12)
			})
		}
	}
}
