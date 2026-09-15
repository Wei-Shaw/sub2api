package service

import (
	"context"
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestAPIReferenceCostUsesUpstreamModelAndPreservesPrice(t *testing.T) {
	svc := &OpenAIGatewayService{billingService: NewBillingService(&config.Config{}, nil)}
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	at := time.Date(2026, 9, 12, 3, 0, 0, 0, time.UTC)
	tokens := UsageTokens{InputTokens: 1000, OutputTokens: 500, CacheReadTokens: 200}
	log := &UsageLog{TotalCost: 99, ActualCost: 199}
	result := &OpenAIForwardResult{Model: "customer-alias", UpstreamModel: "gpt-5.6-terra"}
	svc.applyOpenAIAPIReferenceCost(context.Background(), log, account, result, tokens, "priority", at)
	require.NotNil(t, log.APIReferenceCost)
	// Terra priority: input $4/M, output $24/M, cache reads $0.4/M.
	require.InDelta(t, 0.01608, *log.APIReferenceCost, 1e-10)
	require.Equal(t, "gpt-5.6-terra", log.APIReferencePricing.Model)
	require.Equal(t, "model_catalog", log.APIReferencePricing.Source)
	require.Equal(t, at, log.APIReferencePricing.PricingAt)
	require.Equal(t, 99.0, log.TotalCost)
	require.Equal(t, 199.0, log.ActualCost)
	before, err := json.Marshal(log.APIReferencePricing)
	require.NoError(t, err)
	// Catalog replacement/mutation must not change already-recorded evidence.
	svc.billingService.fallbackPrices["gpt-5.6-terra"].InputPricePerToken = 123
	after, err := json.Marshal(log.APIReferencePricing)
	require.NoError(t, err)
	require.Equal(t, string(before), string(after))
}

func TestAPIReferenceCostUnknownIsDifferentFromZero(t *testing.T) {
	svc := &OpenAIGatewayService{billingService: NewBillingService(&config.Config{}, nil)}
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	for _, tc := range []struct {
		name    string
		model   string
		tokens  UsageTokens
		missing bool
	}{
		{"known zero usage", "gpt-5.6-terra", UsageTokens{}, false},
		{"unknown model with usage", "unknown-model", UsageTokens{InputTokens: 10}, true},
		{"unknown model without usage", "unknown-model", UsageTokens{}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			log := &UsageLog{}
			svc.applyOpenAIAPIReferenceCost(context.Background(), log, account, &OpenAIForwardResult{Model: tc.model}, tc.tokens, "", time.Now())
			if tc.missing {
				require.Nil(t, log.APIReferenceCost)
				require.Nil(t, log.APIReferencePricing)
			} else {
				require.NotNil(t, log.APIReferenceCost)
				require.Zero(t, *log.APIReferenceCost)
			}
		})
	}
	// A known free catalog card also records zero, rather than missing pricing.
	svc.billingService.fallbackPrices["gpt-5.6-terra"] = &ModelPricing{}
	free := &UsageLog{}
	svc.applyOpenAIAPIReferenceCost(context.Background(), free, account, &OpenAIForwardResult{Model: "gpt-5.6-terra"}, UsageTokens{InputTokens: 10}, "", time.Now())
	require.NotNil(t, free.APIReferenceCost)
	require.Zero(t, *free.APIReferenceCost)
}

func TestAPIReferenceCostRejectsUnsupportedScopeAndInvalidPrices(t *testing.T) {
	svc := &OpenAIGatewayService{billingService: NewBillingService(&config.Config{}, nil)}
	parent := int64(1)
	for _, account := range []*Account{
		nil,
		{Platform: PlatformOpenAI, Type: AccountTypeAPIKey},
		{Platform: PlatformAnthropic, Type: AccountTypeOAuth},
		{Platform: PlatformOpenAI, Type: AccountTypeOAuth, ParentAccountID: &parent},
		{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"auth_mode": OpenAIAuthModePersonalAccessToken}},
	} {
		log := &UsageLog{}
		svc.applyOpenAIAPIReferenceCost(context.Background(), log, account, &OpenAIForwardResult{Model: "gpt-5.6-terra"}, UsageTokens{InputTokens: 10}, "", time.Now())
		require.Nil(t, log.APIReferenceCost)
	}
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	tool := &UsageLog{}
	svc.applyOpenAIAPIReferenceCost(context.Background(), tool, account, &OpenAIForwardResult{Model: "gpt-5.6-terra", WebSearchCalls: 1}, UsageTokens{InputTokens: 10}, "", time.Now())
	require.Nil(t, tool.APIReferenceCost, "token-only subtotal must not claim a complete tool request")
	svc.billingService.fallbackPrices["gpt-5.6-terra"] = &ModelPricing{InputPricePerToken: math.NaN()}
	invalid := &UsageLog{}
	svc.applyOpenAIAPIReferenceCost(context.Background(), invalid, account, &OpenAIForwardResult{Model: "gpt-5.6-terra"}, UsageTokens{InputTokens: 10}, "", time.Now())
	require.Nil(t, invalid.APIReferenceCost)
}

func TestRecordUsageAPIReferenceCostIgnoresCustomerPricesAndMultipliers(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil)
	svc.resolver = NewModelPricingResolver(nil, svc.billingService)
	groupID := int64(2)
	customPrice, accountMultiplier := 100.0, 9.0
	requestAt := time.Date(2026, 9, 12, 1, 2, 3, 0, time.UTC)
	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{RequestID: "reference-custom-prices", Model: "customer-alias", UpstreamModel: "gpt-5.6-terra", Usage: OpenAIUsage{InputTokens: 1200, CacheReadInputTokens: 200, OutputTokens: 500}},
		APIKey: &APIKey{ID: 1, GroupID: &groupID, Group: &Group{ID: groupID, RateMultiplier: 7, ModelPricing: []ChannelModelPricing{{Models: []string{"gpt-5.6-terra"}, InputPrice: &customPrice, OutputPrice: &customPrice}}}},
		User:   &User{ID: 2}, Account: &Account{ID: 3, Platform: PlatformOpenAI, Type: AccountTypeOAuth, RateMultiplier: &accountMultiplier}, PricingAt: requestAt,
	})
	require.NoError(t, err)
	require.NotNil(t, usageRepo.lastLog)
	require.NotNil(t, usageRepo.lastLog.APIReferenceCost)
	require.InDelta(t, 0.00804, *usageRepo.lastLog.APIReferenceCost, 1e-10)
	require.Greater(t, usageRepo.lastLog.TotalCost, *usageRepo.lastLog.APIReferenceCost)
	require.Equal(t, requestAt, usageRepo.lastLog.APIReferencePricing.PricingAt)
	require.Equal(t, 1000, usageRepo.lastLog.APIReferencePricing.Tokens.InputTokens, "cache input is not double charged")
}

func TestRecordUsageAPIReferenceCostPreservesCodexFastTier(t *testing.T) {
	for _, tc := range []struct {
		requested string
		observed  string
		priced    string
		want      float64
	}{
		{"priority", "default", "priority", 0.01608},
		{"priority", "priority", "priority", 0.01608},
		{"default", "default", "default", 0.00804},
	} {
		t.Run(tc.requested+"/"+tc.observed, func(t *testing.T) {
			usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
			svc := newOpenAIRecordUsageServiceForTest(usageRepo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil)
			tier := tc.requested
			err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
				Result: &OpenAIForwardResult{
					RequestID: "reference-tier-" + tc.observed, Model: "gpt-5.6-terra",
					ServiceTier: &tier, UpstreamResponseServiceTier: tc.observed,
					Usage: OpenAIUsage{InputTokens: 1200, CacheReadInputTokens: 200, OutputTokens: 500},
				},
				APIKey: &APIKey{ID: 1, Group: &Group{ID: 4, Platform: PlatformOpenAI, FreeOpenAIFast: true, RateMultiplier: 0.5}},
				User:   &User{ID: 2}, Account: &Account{ID: 3, Platform: PlatformOpenAI, Type: AccountTypeOAuth},
			})
			require.NoError(t, err)
			require.NotNil(t, usageRepo.lastLog.APIReferenceCost)
			require.InDelta(t, tc.want, *usageRepo.lastLog.APIReferenceCost, 1e-10)
			require.Equal(t, tc.priced, usageRepo.lastLog.APIReferencePricing.ServiceTier)
			require.Equal(t, tc.observed, usageRepo.lastLog.APIReferencePricing.ObservedServiceTier)
		})
	}
}
