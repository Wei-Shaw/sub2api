package service

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/modelcatalog"
	"github.com/stretchr/testify/require"
)

func TestBillingService_CatalogGemini37FlashTiersAreBillable(t *testing.T) {
	svc := NewBillingService(&config.Config{}, nil)
	tokens := UsageTokens{InputTokens: 1_000_000, OutputTokens: 1_000_000, CacheReadTokens: 1_000_000}

	for _, model := range []string{
		"gemini-3.7-flash",
		"gemini-3.7-flash-high",
		"gemini-3.7-flash-low",
		"gemini-3.7-flash-medium",
		"gemini-3.7-flash-tiered",
	} {
		t.Run(model, func(t *testing.T) {
			cost, err := svc.CalculateCost(model, tokens, 1)
			require.NoError(t, err)
			require.InDelta(t, 0.75, cost.InputCost, 1e-12)
			require.InDelta(t, 3.75, cost.OutputCost, 1e-12)
			require.InDelta(t, 0.075, cost.CacheReadCost, 1e-12)
		})
	}
}

func TestMergeCatalogPricingData_LockedCardsOverlayWithoutWipingExtras(t *testing.T) {
	data := mergeCatalogPricingData(map[string]*LiteLLMModelPricing{
		"gemini-3.6-flash": {
			InputCostPerToken:               9e-6,
			OutputCostPerToken:              9e-6,
			CacheReadInputTokenCost:         9e-6,
			InputCostPerTokenPriority:       2.7e-6,
			OutputCostPerTokenPriority:      1.35e-5,
			CacheReadInputTokenCostPriority: 2.7e-7,
		},
	})

	got := data["gemini-3.6-flash"]
	require.NotNil(t, got)
	require.InDelta(t, 1.5e-6, got.InputCostPerToken, 1e-12)
	require.InDelta(t, 7.5e-6, got.OutputCostPerToken, 1e-12)
	require.InDelta(t, 0.15e-6, got.CacheReadInputTokenCost, 1e-12)
	require.InDelta(t, 2.7e-6, got.InputCostPerTokenPriority, 1e-12)
	require.InDelta(t, 1.35e-5, got.OutputCostPerTokenPriority, 1e-12)
	require.InDelta(t, 2.7e-7, got.CacheReadInputTokenCostPriority, 1e-12)
}

func TestMergeCatalogPricingData_DoesNotRemapSoldModelsOrWipeBundledRates(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "resources", "model-pricing", "model_prices_and_context_window.json"))
	require.NoError(t, err)

	svc := &PricingService{}
	before, err := svc.parsePricingData(body)
	require.NoError(t, err)
	after := mergeCatalogPricingData(cloneLiteLLMPricingData(before))

	sold := []string{
		"gpt-5.5",
		"gpt-5.5-pro",
		"codex-auto-review",
		"deepseek-chat",
		"deepseek-reasoner",
		"gpt-5.2",
		"gpt-5.4",
		"claude-sonnet-4-6",
		"claude-opus-4-5",
		"gemini-3.6-flash",
	}
	for _, model := range sold {
		want := before[model]
		got := after[model]
		require.NotNil(t, want, model)
		require.NotNil(t, got, model)
		require.InDelta(t, want.InputCostPerToken, got.InputCostPerToken, 1e-15, model)
		require.InDelta(t, want.OutputCostPerToken, got.OutputCostPerToken, 1e-15, model)
		require.InDelta(t, want.CacheReadInputTokenCost, got.CacheReadInputTokenCost, 1e-15, model)
		require.InDelta(t, want.CacheReadInputTokenCostPriority, got.CacheReadInputTokenCostPriority, 1e-15, model)
		require.InDelta(t, want.CacheCreationInputTokenCost, got.CacheCreationInputTokenCost, 1e-15, model)
		require.InDelta(t, want.CacheCreationInputTokenCostAbove1hr, got.CacheCreationInputTokenCostAbove1hr, 1e-15, model)
		require.InDelta(t, want.InputCostPerTokenPriority, got.InputCostPerTokenPriority, 1e-15, model)
		require.Equal(t, want.LongContextInputTokenThreshold, got.LongContextInputTokenThreshold, model)
	}

	require.InDelta(t, 0.2e-6, after["codex-auto-review"].InputCostPerToken, 1e-12)
	require.InDelta(t, 5e-6, after["gpt-5.5"].InputCostPerToken, 1e-12)
	require.InDelta(t, 3e-5, after["gpt-5.5-pro"].InputCostPerToken, 1e-12)
	require.InDelta(t, 3.5e-7, after["gpt-5.2"].CacheReadInputTokenCostPriority, 1e-15)

	require.Nil(t, before["gemini-3.7-flash"])
	require.NotNil(t, after["gemini-3.7-flash"])
	require.InDelta(t, 0.75e-6, after["gemini-3.7-flash"].InputCostPerToken, 1e-12)
	require.Nil(t, after["gemini-3.7-flash-high"], "thinking-tier aliases must not shadow the base card")
}

func TestCatalogDoesNotStealGrokInclusiveFallback(t *testing.T) {
	svc := NewBillingService(&config.Config{}, &PricingService{pricingData: mergeCatalogPricingData(map[string]*LiteLLMModelPricing{})})
	pricing, err := svc.GetModelPricing("grok-4.6")
	require.NoError(t, err)
	require.True(t, pricing.LongContextThresholdInclusive)
	require.Equal(t, 200000, pricing.LongContextInputThreshold)

	cost, err := svc.CalculateCost("grok-4.6", UsageTokens{InputTokens: 200000, OutputTokens: 0}, 1)
	require.NoError(t, err)
	require.InDelta(t, 0.8, cost.InputCost, 1e-12)
}

func TestUnlockedCatalogCardsDoNotOverwriteExistingFallback(t *testing.T) {
	svc := NewBillingService(&config.Config{}, nil)
	pricing, err := svc.GetModelPricing("gpt-5.2")
	require.NoError(t, err)
	require.InDelta(t, 0.35e-6, pricing.CacheReadPricePerTokenPriority, 1e-15)
	require.False(t, modelcatalog.Locked("gpt-5.2"))
	require.False(t, modelcatalog.Locked("gpt-5.5"))
	require.Empty(t, modelcatalog.SharedRateCardID("gpt-5.5"))
	require.Empty(t, modelcatalog.SharedRateCardID("codex-auto-review"))
}

func cloneLiteLLMPricingData(in map[string]*LiteLLMModelPricing) map[string]*LiteLLMModelPricing {
	out := make(map[string]*LiteLLMModelPricing, len(in))
	for key, pricing := range in {
		if pricing == nil {
			out[key] = nil
			continue
		}
		copied := *pricing
		out[key] = &copied
	}
	return out
}
