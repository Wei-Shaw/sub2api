package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestGeminiNeonFallbackBilling(t *testing.T) {
	svc := NewBillingService(&config.Config{}, nil)
	tokens := UsageTokens{InputTokens: 1_000_000, OutputTokens: 1_000_000, CacheReadTokens: 1_000_000}
	for _, tc := range []struct {
		model               string
		serviceTier         string
		input, output, read float64
	}{
		{"gemini-4-neon", "", 3, 15, 0.15},
		{"gemini-4-neon-xhigh", "", 3, 15, 0.15},
		{"gemini-4-neon-low", "", 3, 15, 0.15},
		{"models/gemini-4-neon", "", 3, 15, 0.15},
		{"projects/p/locations/global/publishers/google/models/gemini-4-neon", "", 3, 15, 0.15},
		{"gemini-4-neon", "priority", 5.4, 27, 0.27},
		{"gemini-4-hydrogen-xhigh", "", 1.5, 7.5, 0.075},
	} {
		t.Run(tc.model+"/"+tc.serviceTier, func(t *testing.T) {
			cost, err := svc.CalculateCostWithServiceTier(tc.model, tokens, 1, tc.serviceTier)
			require.NoError(t, err)
			require.InDelta(t, tc.input, cost.InputCost, 1e-12)
			require.InDelta(t, tc.output, cost.OutputCost, 1e-12)
			require.InDelta(t, tc.read, cost.CacheReadCost, 1e-12)
			require.InDelta(t, tc.input+tc.output+tc.read, cost.TotalCost, 1e-12)
			require.Zero(t, svc.CacheStoragePricePerTokenHour(tc.model))
		})
	}
}

func TestGeminiNeonAndHydrogenPricingAliases(t *testing.T) {
	neon := &LiteLLMModelPricing{InputCostPerToken: 3e-6}
	hydrogen := &LiteLLMModelPricing{InputCostPerToken: 1.5e-6}
	svc := &PricingService{pricingData: map[string]*LiteLLMModelPricing{"gemini-4-neon": neon, "gemini-4-hydrogen": hydrogen}}
	for _, level := range []string{"", "-xhigh", "-high", "-medium", "-low"} {
		require.Same(t, neon, svc.GetIdentifiedModelPricing("gemini-4-neon"+level), level)
		require.Same(t, hydrogen, svc.GetIdentifiedModelPricing("gemini-4-hydrogen"+level), level)
	}
	require.Equal(t, "gemini-4-neon-preview", normalizeGeminiThinkingTierAlias("gemini-4-neon-preview"))
}

func TestDefaultPricingIncludesGeminiNeon(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "resources", "model-pricing", "model_prices_and_context_window.json"))
	require.NoError(t, err)
	svc := &PricingService{}
	svc.pricingData, err = svc.parsePricingData(data)
	require.NoError(t, err)
	pricing := svc.GetIdentifiedModelPricing("gemini-4-neon-high")
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
}
