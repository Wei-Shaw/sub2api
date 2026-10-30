//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestCalculateCostUnified_DeepseekProviderPrefixedPricing(t *testing.T) {
	// Reproduce a remote catalog containing peak rates. Pricing lookup already
	// accepts provider prefixes; the official rate policy must recognize them too.
	bs := NewBillingService(&config.Config{}, &PricingService{pricingData: map[string]*LiteLLMModelPricing{
		"deepseek-v4-pro": {InputCostPerToken: 1.32e-6, OutputCostPerToken: 3.96e-6, CacheReadInputTokenCost: 4.4e-8},
		"deepseek-flash":  {InputCostPerToken: 3e-7, OutputCostPerToken: 1.2e-6, CacheReadInputTokenCost: 6e-9},
	}})
	resolver := NewModelPricingResolver(nil, bs)
	tokens := UsageTokens{InputTokens: 1000, OutputTokens: 500, CacheReadTokens: 1000}

	for _, model := range []struct {
		name        string
		offPeakCost float64
	}{
		{"deepseek-v4-pro", 0.001672},
		{"cline-pass/deepseek-v4-pro", 0.001672},
		{"deepseek/deepseek-v4-pro-0813", 0.001672},
		{" CLINE-PASS/DEEPSEEK-V4-PRO ", 0.001672},
		{"deepseek-flash", 0.000453},
		{"cline-pass/deepseek-flash", 0.000453},
		{"cline-pass/deepseek-v4.1-flash-1017", 0.000453},
	} {
		for _, slot := range []struct {
			name       string
			at         time.Time
			multiplier float64
		}{
			{"peak", time.Date(2026, 10, 26, 2, 0, 0, 0, time.UTC), 2},
			{"off_peak", time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC), 1},
			{"weekend", time.Date(2026, 10, 31, 2, 0, 0, 0, time.UTC), 1},
		} {
			t.Run(model.name+"/"+slot.name, func(t *testing.T) {
				cost, err := bs.CalculateCostUnified(CostInput{
					Ctx: context.Background(), Model: model.name, Tokens: tokens,
					RateMultiplier: 0.5, Resolver: resolver, PricingAt: slot.at,
				})
				require.NoError(t, err)
				wantTotal := model.offPeakCost * slot.multiplier
				require.InDelta(t, wantTotal, cost.TotalCost, 1e-12)
				require.InDelta(t, wantTotal*0.5, cost.ActualCost, 1e-12)
			})
		}
	}
}

func TestCalculateCostUnified_DeepseekProviderPrefixedCustomPricing(t *testing.T) {
	const model = "cline-pass/deepseek-v4-pro"
	bs := newTestBillingService()
	inputPrice, outputPrice, cachePrice := 1e-6, 2e-6, 1e-7
	card := ChannelModelPricing{
		Models: []string{model}, Platform: PlatformCline, BillingMode: BillingModeToken,
		InputPrice: &inputPrice, OutputPrice: &outputPrice, CacheReadPrice: &cachePrice,
	}
	group := &Group{ID: 1, Platform: PlatformCline, ModelPricing: []ChannelModelPricing{card}}
	channelCard := card
	channelCard.TimePricing = &ChannelTimePricing{
		Timezone: "Asia/Shanghai",
		Periods:  []ChannelTimePricingPeriod{{StartTime: "09:00", EndTime: "12:00", Multiplier: 3}},
	}
	channelService := &ChannelService{}
	channelService.cache.Store(populateChannelCache([]Channel{{
		ID: 1, Status: StatusActive, GroupIDs: []int64{group.ID}, ModelPricing: []ChannelModelPricing{channelCard},
	}}, map[int64]string{group.ID: PlatformCline}))

	for _, source := range []string{"group", "channel"} {
		for _, hour := range []int{2, 12} {
			t.Run(source+"/"+time.Date(2026, 10, 26, hour, 0, 0, 0, time.UTC).Format("15:04"), func(t *testing.T) {
				input := CostInput{
					Ctx: context.Background(), Model: model,
					Tokens:         UsageTokens{InputTokens: 1000, OutputTokens: 500, CacheReadTokens: 1000},
					RateMultiplier: 0.5, PricingAt: time.Date(2026, 10, 26, hour, 0, 0, 0, time.UTC),
					Resolver: NewModelPricingResolver(channelService, bs),
				}
				if source == "group" {
					input.Group = group
				} else {
					input.GroupID = &group.ID
				}
				resolved := input.Resolver.Resolve(input.Ctx, PricingInput{
					Model: model, GroupID: input.GroupID, Group: input.Group,
				})
				require.Equal(t, source, resolved.Source)
				cost, err := bs.CalculateCostUnified(input)
				require.NoError(t, err)
				wantTotal := 0.0021
				if source == "channel" && hour == 2 {
					wantTotal *= 3
				}
				require.InDelta(t, wantTotal, cost.TotalCost, 1e-12)
				require.InDelta(t, wantTotal*0.5, cost.ActualCost, 1e-12)
			})
		}
	}
}

func TestOpenAIGatewayServiceRecordUsage_ClinePassDeepseekPricing(t *testing.T) {
	for _, hour := range []int{2, 12} {
		t.Run(time.Date(2026, 10, 26, hour, 0, 0, 0, time.UTC).Format("15:04"), func(t *testing.T) {
			usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
			userRepo := &openAIRecordUsageUserRepoStub{}
			svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, &openAIRecordUsageSubRepoStub{}, nil)
			svc.billingService = NewBillingService(&config.Config{}, &PricingService{pricingData: map[string]*LiteLLMModelPricing{
				"deepseek-v4-pro": {InputCostPerToken: 1.32e-6, OutputCostPerToken: 3.96e-6, CacheReadInputTokenCost: 4.4e-8},
			}})
			svc.resolver = NewModelPricingResolver(nil, svc.billingService)
			group := &Group{ID: 1, Platform: PlatformCline, RateMultiplier: 0.5}
			const upstreamModel = "cline-pass/deepseek-v4-pro"
			err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
				Result: &OpenAIForwardResult{
					RequestID: "cline_pass_deepseek_pricing", Model: "deepseek-v4-pro",
					BillingModel: upstreamModel, UpstreamModel: upstreamModel,
					Usage: OpenAIUsage{InputTokens: 2000, OutputTokens: 500, CacheReadInputTokens: 1000},
				},
				APIKey: &APIKey{ID: 1, GroupID: &group.ID, Group: group},
				User:   &User{ID: 1}, Account: &Account{ID: 1, Platform: PlatformCline},
				PricingAt:          time.Date(2026, 10, 26, hour, 0, 0, 0, time.UTC),
				ChannelUsageFields: ChannelUsageFields{OriginalModel: "deepseek-v4-pro"},
			})
			require.NoError(t, err)
			require.NotNil(t, usageRepo.lastLog)
			wantTotal := 0.001672
			if hour == 2 {
				wantTotal *= 2
			}
			require.InDelta(t, wantTotal, usageRepo.lastLog.TotalCost, 1e-12)
			require.InDelta(t, wantTotal*0.5, usageRepo.lastLog.ActualCost, 1e-12)
			require.InDelta(t, wantTotal*0.5, userRepo.lastAmount, 1e-12)
			require.Equal(t, 1, userRepo.deductCalls)
			require.NotNil(t, usageRepo.lastLog.UpstreamModel)
			require.Equal(t, upstreamModel, *usageRepo.lastLog.UpstreamModel)
		})
	}
}
