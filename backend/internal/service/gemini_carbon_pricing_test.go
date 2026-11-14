package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestGeminiCarbonPricingAliases(t *testing.T) {
	base := &LiteLLMModelPricing{InputCostPerToken: 2e-6, OutputCostPerToken: 10e-6, CacheReadInputTokenCost: 0.1e-6}
	svc := &PricingService{pricingData: map[string]*LiteLLMModelPricing{"gemini-4-carbon": base}}
	for _, model := range []string{
		"gemini-4-carbon", "gemini-4-carbon-high", "gemini-4-carbon-medium", "gemini-4-carbon-low", "gemini-4-carbon-tiered",
		"models/gemini-4-carbon", "projects/p/locations/global/publishers/google/models/gemini-4-carbon-high",
	} {
		t.Run(model, func(t *testing.T) {
			require.Same(t, base, svc.GetIdentifiedModelPricing(model))
		})
	}
	// A separately published tier price must take precedence over base normalization.
	tier := &LiteLLMModelPricing{InputCostPerToken: 3e-6}
	svc.pricingData["gemini-4-carbon-high"] = tier
	require.Same(t, tier, svc.GetIdentifiedModelPricing("gemini-4-carbon-high"))
	require.Equal(t, "gemini-4-carbon-cyber", normalizeGeminiThinkingTierAlias("gemini-4-carbon-cyber"))
}

func TestGeminiCarbonFallbackBilling(t *testing.T) {
	svc := NewBillingService(&config.Config{}, nil)
	tokens := UsageTokens{InputTokens: 1_000_000, OutputTokens: 1_000_000, CacheReadTokens: 1_000_000}
	for _, model := range []string{"gemini-4-carbon", "gemini-4-carbon-low", "gemini-4-carbon-medium", "gemini-4-carbon-high", "models/gemini-4-carbon", "projects/p/locations/global/publishers/google/models/gemini-4-carbon"} {
		t.Run(model, func(t *testing.T) {
			cost, err := svc.CalculateCost(model, tokens, 1)
			require.NoError(t, err)
			require.InDelta(t, 2, cost.InputCost, 1e-12)
			require.InDelta(t, 10, cost.OutputCost, 1e-12)
			require.InDelta(t, 0.1, cost.CacheReadCost, 1e-12)
			require.InDelta(t, 12.1, cost.TotalCost, 1e-12)
			priority, err := svc.CalculateCostWithServiceTier(model, tokens, 1, "priority")
			require.NoError(t, err)
			require.InDelta(t, 12.1*1.8, priority.TotalCost, 1e-12)
			require.Zero(t, svc.CacheStoragePricePerTokenHour(model))
		})
	}
}

func TestDefaultPricingIncludesGeminiCarbon(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "resources", "model-pricing", "model_prices_and_context_window.json"))
	require.NoError(t, err)
	svc := &PricingService{}
	svc.pricingData, err = svc.parsePricingData(data)
	require.NoError(t, err)
	pricing := svc.GetIdentifiedModelPricing("gemini-4-carbon")
	require.NotNil(t, pricing)
	require.Equal(t, "gemini", pricing.LiteLLMProvider)
	require.InDelta(t, 2e-6, pricing.InputCostPerToken, 1e-15)
	require.InDelta(t, 10e-6, pricing.OutputCostPerToken, 1e-15)
	require.InDelta(t, 0.1e-6, pricing.CacheReadInputTokenCost, 1e-15)
	require.InDelta(t, 3.6e-6, pricing.InputCostPerTokenPriority, 1e-15)
	require.InDelta(t, 18e-6, pricing.OutputCostPerTokenPriority, 1e-15)
	require.InDelta(t, 0.18e-6, pricing.CacheReadInputTokenCostPriority, 1e-15)
	require.Zero(t, pricing.LongContextInputTokenThreshold)
	require.Zero(t, pricing.CacheStorageCostPerTokenPerHour)
	require.Contains(t, svc.ListModelNamesByProvider("gemini"), "gemini-4-carbon")

	var catalog map[string]map[string]any
	require.NoError(t, json.Unmarshal(data, &catalog))
	// The synchronous gateway must not advertise long-decode's separate 1M output limit.
	require.Equal(t, float64(1048576), catalog["gemini-4-carbon"]["max_input_tokens"])
	require.Equal(t, float64(65536), catalog["gemini-4-carbon"]["max_output_tokens"])
	require.Equal(t, float64(65536), catalog["gemini-4-carbon"]["max_tokens"])
}

func TestGeminiHydrogenPricingAndFallbackBilling(t *testing.T) {
	base := &LiteLLMModelPricing{InputCostPerToken: 1.5e-6, OutputCostPerToken: 7.5e-6, CacheReadInputTokenCost: 0.075e-6}
	svc := &PricingService{pricingData: map[string]*LiteLLMModelPricing{"gemini-4-hydrogen": base}}
	for _, model := range []string{
		"gemini-4-hydrogen", "gemini-4-hydrogen-xhigh", "gemini-4-hydrogen-high", "gemini-4-hydrogen-medium", "gemini-4-hydrogen-low",
		"models/gemini-4-hydrogen", "projects/p/locations/global/publishers/google/models/gemini-4-hydrogen-xhigh",
	} {
		t.Run("alias/"+model, func(t *testing.T) {
			require.Same(t, base, svc.GetIdentifiedModelPricing(model))
		})
	}

	billingSvc := NewBillingService(&config.Config{}, nil)
	tokens := UsageTokens{InputTokens: 1_000_000, OutputTokens: 1_000_000, CacheReadTokens: 1_000_000}
	for _, model := range []string{
		"gemini-4-hydrogen", "gemini-4-hydrogen-low", "gemini-4-hydrogen-medium", "gemini-4-hydrogen-high", "gemini-4-hydrogen-xhigh",
		"models/gemini-4-hydrogen", "projects/p/locations/global/publishers/google/models/gemini-4-hydrogen",
	} {
		t.Run("fallback/"+model, func(t *testing.T) {
			cost, err := billingSvc.CalculateCost(model, tokens, 1)
			require.NoError(t, err)
			require.InDelta(t, 1.5, cost.InputCost, 1e-12)
			require.InDelta(t, 7.5, cost.OutputCost, 1e-12)
			require.InDelta(t, 0.075, cost.CacheReadCost, 1e-12)
			require.InDelta(t, 9.075, cost.TotalCost, 1e-12)
			priority, err := billingSvc.CalculateCostWithServiceTier(model, tokens, 1, "priority")
			require.NoError(t, err)
			require.InDelta(t, 9.075*1.8, priority.TotalCost, 1e-12)
			require.Zero(t, billingSvc.CacheStoragePricePerTokenHour(model))
		})
	}
	for _, unknown := range []string{"gemini-4-hydrogen-lite", "gemini-4-hydrogen-cyber-permissive", "gemini-4-hydrogen-preview"} {
		require.Equal(t, unknown, normalizeGeminiThinkingTierAlias(unknown))
		_, err := billingSvc.CalculateCost(unknown, tokens, 1)
		require.Error(t, err, "distinct unpriced SKU %s must not silently match base Hydrogen fallback", unknown)
	}

	data, err := os.ReadFile(filepath.Join("..", "..", "resources", "model-pricing", "model_prices_and_context_window.json"))
	require.NoError(t, err)
	catalogSvc := &PricingService{}
	catalogSvc.pricingData, err = catalogSvc.parsePricingData(data)
	require.NoError(t, err)
	pricing := catalogSvc.GetIdentifiedModelPricing("gemini-4-hydrogen")
	require.NotNil(t, pricing)
	require.Equal(t, "gemini", pricing.LiteLLMProvider)
	require.InDelta(t, 1.5e-6, pricing.InputCostPerToken, 1e-15)
	require.InDelta(t, 7.5e-6, pricing.OutputCostPerToken, 1e-15)
	require.InDelta(t, 0.075e-6, pricing.CacheReadInputTokenCost, 1e-15)
	require.InDelta(t, 2.7e-6, pricing.InputCostPerTokenPriority, 1e-15)
	require.InDelta(t, 13.5e-6, pricing.OutputCostPerTokenPriority, 1e-15)
	require.InDelta(t, 0.135e-6, pricing.CacheReadInputTokenCostPriority, 1e-15)
	require.Zero(t, pricing.LongContextInputTokenThreshold)
	require.Zero(t, pricing.CacheStorageCostPerTokenPerHour)
	require.Contains(t, catalogSvc.ListModelNamesByProvider("gemini"), "gemini-4-hydrogen")

	var catalog map[string]map[string]any
	require.NoError(t, json.Unmarshal(data, &catalog))
	require.Equal(t, float64(1048576), catalog["gemini-4-hydrogen"]["max_input_tokens"])
	require.Equal(t, float64(65536), catalog["gemini-4-hydrogen"]["max_output_tokens"])
	require.Equal(t, float64(65536), catalog["gemini-4-hydrogen"]["max_tokens"])
}
