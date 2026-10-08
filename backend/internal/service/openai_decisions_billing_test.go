//go:build unit

package service

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDecisionsBillingUsesDedicatedCardAndKeepsChannelPrices(t *testing.T) {
	for _, custom := range []bool{false, true} {
		billing := newTestBillingService()
		svc := &OpenAIGatewayService{billingService: billing}
		group := &Group{ID: 777, Platform: PlatformOpenAI}
		key := &APIKey{Group: group}
		if custom {
			cs := newChannelServiceWithPricings(group.ID, []ChannelModelPricing{tokenPricingForModels([]string{"gpt-6-luna"}, 1)})
			svc.resolver = NewModelPricingResolver(cs, billing)
		}
		result := &OpenAIForwardResult{Model: "gpt-6-luna", UpstreamModel: "openai/gpt-6-luna-decisions"}
		cost, err := svc.calculateOpenAIRecordUsageCost(context.Background(), result, key, []string{"gpt-6-luna"}, 2, 1, 1, 1, UsageTokens{InputTokens: 1_000_000, OutputTokens: 100, CacheReadTokens: 100}, "", nil, time.Now())
		require.NoError(t, err)
		if custom {
			require.InDelta(t, 2*(1+100*2.4e-6+100*0.04e-6), cost.ActualCost, 1e-10)
		} else {
			require.InDelta(t, 0.2, cost.ActualCost, 1e-10)
			require.Zero(t, cost.OutputCost)
			require.Zero(t, cost.CacheReadCost)
		}
	}
}
func TestDecisionsUsageRecordsRequestedAndUpstreamModels(t *testing.T) {
	repo := &openAIRecordUsageLogRepoStub{inserted: true}
	svc := newOpenAIRecordUsageServiceForTest(repo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil)
	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{Result: &OpenAIForwardResult{RequestID: "decision-usage", Model: "gpt-6-luna", UpstreamModel: "openai/gpt-6-luna-decisions", BillingModel: "openai/gpt-6-luna-decisions", Usage: OpenAIUsage{InputTokens: 1_000_000, CacheReadInputTokens: 100_000}}, APIKey: &APIKey{ID: 2}, User: &User{ID: 1}, Account: &Account{ID: 3, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, InboundEndpoint: "/v1/decisions", UpstreamEndpoint: "/api/alpha/decisions", ChannelUsageFields: ChannelUsageFields{OriginalModel: "gpt-6-luna", BillingModelSource: BillingModelSourceRequested}})
	require.NoError(t, err)
	require.NotNil(t, repo.lastLog)
	require.Equal(t, "openai/gpt-6-luna-decisions", *repo.lastLog.UpstreamModel)
	require.Equal(t, "/api/alpha/decisions", *repo.lastLog.UpstreamEndpoint)
	require.InDelta(t, 0.099, repo.lastLog.ActualCost, 1e-9)
}
func TestDecisionsSchedulingFiltersBothEngines(t *testing.T) {
	for _, protocol := range []string{"openai", "openrouter"} {
		account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Credentials: map[string]any{"openai_decisions_protocol": protocol}}
		req := OpenAIAccountScheduleRequest{RequestedModel: "gpt-6-luna", RequiredCapability: OpenAIEndpointCapabilityDecisionsMultiImage}
		scheduler := &defaultOpenAIAccountScheduler{}
		require.Equal(t, protocol == "openai", scheduler.isAccountRequestCompatible(context.Background(), account, req))
		require.Equal(t, protocol == "openai", isOpenAICompatibleAccountEligibleForRequest(context.Background(), account, PlatformOpenAI, "gpt-6-luna", false, req.RequiredCapability))
	}
}

func TestDecisionsPricingDoesNotInheritDynamicChatPrices(t *testing.T) {
	pricing := &PricingService{pricingData: map[string]*LiteLLMModelPricing{"gpt-6-luna": {InputCostPerToken: 99, OutputCostPerToken: 99}}}
	billing := NewBillingService(&config.Config{}, pricing)
	card, err := billing.GetModelPricing("openai/gpt-6-luna-decisions")
	require.NoError(t, err)
	require.InDelta(t, 0.1e-6, card.InputPricePerToken, 1e-12)
	require.Zero(t, card.OutputPricePerToken)
	require.Zero(t, card.CacheCreationPricePerToken)
}
