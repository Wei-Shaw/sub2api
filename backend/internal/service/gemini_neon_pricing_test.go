//go:build unit

package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestGeminiNeonAndHydrogenPricingAliases(t *testing.T) {
	neon := &LiteLLMModelPricing{InputCostPerToken: 3e-6, OutputCostPerToken: 15e-6, CacheReadInputTokenCost: 0.15e-6}
	hydrogen := &LiteLLMModelPricing{InputCostPerToken: 1.5e-6, OutputCostPerToken: 7.5e-6, CacheReadInputTokenCost: 0.075e-6}
	svc := &PricingService{pricingData: map[string]*LiteLLMModelPricing{
		"gemini-4-neon":     neon,
		"gemini-4-hydrogen": hydrogen,
	}}
	for _, model := range []string{
		"gemini-4-neon", "gemini-4-neon-low", "gemini-4-neon-medium", "gemini-4-neon-high", "gemini-4-neon-xhigh",
		"gemini-4-neon-tiered", "gemini-4-neon-tiered-high", "gemini-4-neon-tiered-xhigh", "gemini-4-neon-sonar",
		"models/gemini-4-neon", "projects/p/locations/global/publishers/google/models/gemini-4-neon-xhigh",
	} {
		t.Run(model, func(t *testing.T) {
			require.Same(t, neon, svc.GetIdentifiedModelPricing(model))
		})
	}
	for _, model := range []string{
		"gemini-4-hydrogen", "gemini-4-hydrogen-low", "gemini-4-hydrogen-medium", "gemini-4-hydrogen-high",
		"gemini-4-hydrogen-xhigh", "gemini-4-hydrogen-tiered", "gemini-4-hydrogen-tiered-high",
		"gemini-4-hydrogen-tiered-xhigh", "gemini-4-hydrogen-sonar",
	} {
		t.Run(model, func(t *testing.T) {
			require.Same(t, hydrogen, svc.GetIdentifiedModelPricing(model))
		})
	}

	// A separately published tier price still takes precedence over base normalization.
	tier := &LiteLLMModelPricing{InputCostPerToken: 4e-6}
	svc.pricingData["gemini-4-neon-xhigh"] = tier
	require.Same(t, tier, svc.GetIdentifiedModelPricing("gemini-4-neon-xhigh"))
	require.Equal(t, "gemini-4-neon-cyber", normalizeGeminiThinkingTierAlias("gemini-4-neon-cyber"))
}

func TestGeminiNeonFallbackBilling(t *testing.T) {
	svc := NewBillingService(&config.Config{}, nil)
	tokens := UsageTokens{InputTokens: 1_000_000, OutputTokens: 1_000_000, CacheReadTokens: 1_000_000}
	for _, model := range []string{
		"gemini-4-neon",
		"gemini-4-neon-low",
		"gemini-4-neon-medium",
		"gemini-4-neon-high",
		"gemini-4-neon-xhigh",
		"models/gemini-4-neon",
		"projects/p/locations/global/publishers/google/models/gemini-4-neon",
	} {
		t.Run(model, func(t *testing.T) {
			cost, err := svc.CalculateCost(model, tokens, 1)
			require.NoError(t, err)
			require.InDelta(t, 3, cost.InputCost, 1e-12)
			require.InDelta(t, 15, cost.OutputCost, 1e-12)
			require.InDelta(t, 0.15, cost.CacheReadCost, 1e-12)
			require.InDelta(t, 18.15, cost.TotalCost, 1e-12)
			priority, err := svc.CalculateCostWithServiceTier(model, tokens, 1, "priority")
			require.NoError(t, err)
			require.InDelta(t, 18.15*1.8, priority.TotalCost, 1e-12)
			require.Zero(t, svc.CacheStoragePricePerTokenHour(model))
		})
	}
}

func TestDefaultPricingIncludesGeminiNeon(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "resources", "model-pricing", "model_prices_and_context_window.json"))
	require.NoError(t, err)
	svc := &PricingService{}
	svc.pricingData, err = svc.parsePricingData(data)
	require.NoError(t, err)
	pricing := svc.GetIdentifiedModelPricing("gemini-4-neon")
	require.NotNil(t, pricing)
	require.Equal(t, "gemini", pricing.LiteLLMProvider)
	require.InDelta(t, 3e-6, pricing.InputCostPerToken, 1e-15)
	require.InDelta(t, 15e-6, pricing.OutputCostPerToken, 1e-15)
	require.InDelta(t, 0.15e-6, pricing.CacheReadInputTokenCost, 1e-15)
	require.InDelta(t, 5.4e-6, pricing.InputCostPerTokenPriority, 1e-15)
	require.InDelta(t, 27e-6, pricing.OutputCostPerTokenPriority, 1e-15)
	require.InDelta(t, 0.27e-6, pricing.CacheReadInputTokenCostPriority, 1e-15)
	require.Zero(t, pricing.LongContextInputTokenThreshold)
	require.Zero(t, pricing.CacheStorageCostPerTokenPerHour)
	require.Contains(t, svc.ListModelNamesByProvider("gemini"), "gemini-4-neon")

	var catalog map[string]map[string]any
	require.NoError(t, json.Unmarshal(data, &catalog))
	require.Equal(t, float64(1048576), catalog["gemini-4-neon"]["max_input_tokens"])
	require.Equal(t, float64(65536), catalog["gemini-4-neon"]["max_output_tokens"])
	require.Equal(t, float64(65536), catalog["gemini-4-neon"]["max_tokens"])
}
