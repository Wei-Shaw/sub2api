//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestOpenAIGatewayServiceRecordUsage_AggregatorClaudeUsesDefaultPricing(t *testing.T) {
	for _, platform := range []string{PlatformCommandCode, PlatformCline} {
		for _, model := range []string{"claude-sonnet-4-6", "anthropic/claude-sonnet-4-6"} {
			t.Run(platform+"/"+model, func(t *testing.T) {
				usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
				userRepo := &openAIRecordUsageUserRepoStub{}
				svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, &openAIRecordUsageSubRepoStub{}, nil)
				svc.resolver = NewModelPricingResolver(nil, svc.billingService)
				group := &Group{ID: 1, Platform: platform, RateMultiplier: 1.1}
				usage := OpenAIUsage{InputTokens: 1000, OutputTokens: 200}
				expected := expectedOpenAICost(t, svc, model, usage, group.RateMultiplier)
				require.Greater(t, expected.ActualCost, 0.0)

				err := svc.RecordUsage(context.Background(), providerClaudeRecordUsageInput(platform, model, group, usage))

				require.NoError(t, err)
				require.Equal(t, 1, usageRepo.calls)
				require.NotNil(t, usageRepo.lastLog)
				require.Equal(t, usage.InputTokens, usageRepo.lastLog.InputTokens)
				require.Equal(t, usage.OutputTokens, usageRepo.lastLog.OutputTokens)
				require.InDelta(t, expected.TotalCost, usageRepo.lastLog.TotalCost, 1e-12)
				require.InDelta(t, expected.ActualCost, usageRepo.lastLog.ActualCost, 1e-12)
				require.Equal(t, 1, userRepo.deductCalls)
				require.InDelta(t, expected.ActualCost, userRepo.lastAmount, 1e-12)
			})
		}
	}
}

func TestOpenAIGatewayServiceRecordUsage_CNAndOpenCodeClaudeRequireExplicitPricing(t *testing.T) {
	for _, platform := range []string{PlatformKimi, PlatformZhipu, PlatformDeepseek, PlatformMiniMax, PlatformOpenCodeGo} {
		t.Run(platform, func(t *testing.T) {
			usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
			userRepo := &openAIRecordUsageUserRepoStub{}
			svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, &openAIRecordUsageSubRepoStub{}, nil)
			svc.resolver = NewModelPricingResolver(nil, svc.billingService)
			group := &Group{ID: 1, Platform: platform, RateMultiplier: 1.1}
			usage := OpenAIUsage{InputTokens: 1000, OutputTokens: 200}

			err := svc.RecordUsage(context.Background(), providerClaudeRecordUsageInput(platform, "claude-sonnet-4-6", group, usage))

			require.NoError(t, err)
			require.Equal(t, 1, usageRepo.calls)
			require.NotNil(t, usageRepo.lastLog)
			require.Equal(t, usage.InputTokens, usageRepo.lastLog.InputTokens)
			require.Equal(t, usage.OutputTokens, usageRepo.lastLog.OutputTokens)
			require.Zero(t, usageRepo.lastLog.TotalCost)
			require.Zero(t, usageRepo.lastLog.ActualCost)
			require.Zero(t, userRepo.deductCalls)
		})
	}
}

func TestOpenAIGatewayServiceRecordUsage_CNAndOpenCodeClaudeHonorExplicitPricing(t *testing.T) {
	const model = "claude-sonnet-4-6"
	for _, platform := range []string{PlatformKimi, PlatformOpenCodeGo} {
		for _, source := range []string{PricingSourceGroup, PricingSourceChannel} {
			t.Run(platform+"/"+source, func(t *testing.T) {
				usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
				userRepo := &openAIRecordUsageUserRepoStub{}
				svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, &openAIRecordUsageSubRepoStub{}, nil)
				group := &Group{ID: 1, Platform: platform, RateMultiplier: 1.25}
				inputPrice, outputPrice := 1e-6, 2e-6
				pricing := ChannelModelPricing{
					Models: []string{model}, BillingMode: BillingModeToken,
					InputPrice: &inputPrice, OutputPrice: &outputPrice,
				}
				if source == PricingSourceGroup {
					group.ModelPricing = []ChannelModelPricing{pricing}
				} else {
					cache := newEmptyChannelCache()
					cache.pricingByGroupModel[channelModelKey{groupID: group.ID, platform: platform, model: model}] = &pricing
					cache.channelByGroupID[group.ID] = &Channel{ID: 1, Status: StatusActive}
					cache.groupPlatform[group.ID] = platform
					cache.loadedAt = time.Now()
					svc.channelService = &ChannelService{}
					svc.channelService.cache.Store(cache)
				}
				svc.resolver = NewModelPricingResolver(svc.channelService, svc.billingService)
				usage := OpenAIUsage{InputTokens: 1000, OutputTokens: 200}
				expectedTotal := float64(usage.InputTokens)*inputPrice + float64(usage.OutputTokens)*outputPrice
				expectedActual := expectedTotal * group.RateMultiplier

				err := svc.RecordUsage(context.Background(), providerClaudeRecordUsageInput(platform, model, group, usage))

				require.NoError(t, err)
				require.NotNil(t, usageRepo.lastLog)
				require.InDelta(t, expectedTotal, usageRepo.lastLog.TotalCost, 1e-12)
				require.InDelta(t, expectedActual, usageRepo.lastLog.ActualCost, 1e-12)
				require.Equal(t, 1, userRepo.deductCalls)
				require.InDelta(t, expectedActual, userRepo.lastAmount, 1e-12)
			})
		}
	}
}

func providerClaudeRecordUsageInput(platform, model string, group *Group, usage OpenAIUsage) *OpenAIRecordUsageInput {
	return &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "provider_claude_" + platform,
			Model:     model, BillingModel: model, UpstreamModel: model,
			Usage: usage, Duration: time.Second,
		},
		APIKey:  &APIKey{ID: 10, GroupID: &group.ID, Group: group},
		User:    &User{ID: 20},
		Account: &Account{ID: 30, Platform: platform, Type: AccountTypeAPIKey},
	}
}
