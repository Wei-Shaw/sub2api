package service

import (
	"context"
	"os"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestClaude52Pricing(t *testing.T) {
	embedded, err := os.ReadFile("../../resources/model-pricing/model_prices_and_context_window.json")
	require.NoError(t, err)

	for _, family := range []string{"fable", "mythos"} {
		model := "claude-" + family + "-5-2"
		for _, source := range []string{"no_catalog", "older_catalog", "embedded_catalog", "fable_only_remote_catalog"} {
			t.Run(family+"/"+source, func(t *testing.T) {
				var catalog *PricingService
				switch source {
				case "older_catalog":
					catalog = newStubPricingServiceFromJSON(t, `{
						"claude-fable-5-1": {"input_cost_per_token": 1e-5, "output_cost_per_token": 5e-5},
						"claude-sonnet-4-5": {"input_cost_per_token": 3e-6, "output_cost_per_token": 1.5e-5}
					}`)
				case "fable_only_remote_catalog":
					// The default remote catalog already prices Fable 5.2 correctly.
					catalog = newStubPricingServiceFromJSON(t, `{
						"claude-fable-5-2": {"input_cost_per_token": 8e-6, "output_cost_per_token": 40e-6,
						"cache_creation_input_token_cost": 10e-6, "cache_creation_input_token_cost_above_1hr": 16e-6,
						"cache_read_input_token_cost": 0.2e-6}
					}`)
				case "embedded_catalog":
					catalog = newStubPricingServiceFromJSON(t, string(embedded))
				}
				svc := NewBillingService(&config.Config{}, catalog)
				for _, name := range []string{model, "models/" + model, "anthropic/" + model, "anthropic." + model, "global.anthropic." + model} {
					t.Run(name, func(t *testing.T) {
						pricing, err := svc.GetModelPricing(name)
						require.NoError(t, err)
						require.InDelta(t, 8e-6, pricing.InputPricePerToken, 1e-12)
						require.InDelta(t, 40e-6, pricing.OutputPricePerToken, 1e-12)
						require.InDelta(t, 10e-6, pricing.CacheCreationPricePerToken, 1e-12)
						require.InDelta(t, 10e-6, pricing.CacheCreation5mPrice, 1e-12)
						require.InDelta(t, 16e-6, pricing.CacheCreation1hPrice, 1e-12)
						require.InDelta(t, 0.2e-6, pricing.CacheReadPricePerToken, 1e-12)
						require.True(t, pricing.SupportsCacheBreakdown)
					})
				}

				// Cache writes have distinct TTL rates and are counted only once.
				tokens := UsageTokens{InputTokens: 250000, OutputTokens: 200, CacheCreationTokens: 1000,
					CacheCreation5mTokens: 600, CacheCreation1hTokens: 400, CacheReadTokens: 3000}
				for _, tier := range []string{"", "fast", "priority"} {
					t.Run("cost/"+tier, func(t *testing.T) {
						multiplier := 1.0
						if tier != "" {
							multiplier = 2
						}
						cost, err := svc.CalculateCostWithServiceTier(model, tokens, 1.5, tier)
						require.NoError(t, err)
						require.InDelta(t, 2*multiplier, cost.InputCost, 1e-12)
						require.InDelta(t, 0.008*multiplier, cost.OutputCost, 1e-12)
						require.InDelta(t, 0.0124*multiplier, cost.CacheCreationCost, 1e-12)
						require.InDelta(t, 0.0006*multiplier, cost.CacheReadCost, 1e-12)
						require.InDelta(t, 2.021*multiplier, cost.TotalCost, 1e-12)
						require.InDelta(t, 3.0315*multiplier, cost.ActualCost, 1e-12)
						require.False(t, cost.LongContextBillingApplied)

						unified, err := svc.CalculateCostUnified(CostInput{
							Ctx: context.Background(), Model: model, Tokens: tokens, RateMultiplier: 1.5,
							ServiceTier: tier, Resolver: NewModelPricingResolver(nil, svc),
						})
						require.NoError(t, err)
						cost.BillingMode = string(BillingModeToken)
						require.Equal(t, cost, unified)
					})
				}
			})
		}
	}
}

func TestClaude52CatalogOverridesAndVersionBoundaries(t *testing.T) {
	for _, family := range []string{"fable", "mythos"} {
		model := "claude-" + family + "-5-2"
		t.Run(family+"/catalog_override", func(t *testing.T) {
			catalog := newStubPricingServiceFromJSON(t, `{"`+model+`": {
				"input_cost_per_token": 0, "output_cost_per_token": 7e-6,
				"cache_creation_input_token_cost": 0, "cache_read_input_token_cost": 0
			}}`)
			svc := NewBillingService(&config.Config{}, catalog)
			pricing, err := svc.GetModelPricing(model)
			require.NoError(t, err)
			require.Zero(t, pricing.InputPricePerToken)
			require.InDelta(t, 7e-6, pricing.OutputPricePerToken, 1e-12)
			require.Zero(t, pricing.CacheCreationPricePerToken)
			require.Zero(t, pricing.CacheReadPricePerToken)
			for _, name := range []string{model, "anthropic/" + model, "global.anthropic." + model} {
				require.Same(t, catalog.pricingData[model], catalog.GetModelPricing(name))
			}
		})
	}

	svc := NewBillingService(&config.Config{}, nil)
	for _, model := range []string{"claude-fable-5", "claude-fable-5-1", "claude-fable-5-20", "claude-fable-5-2-preview", "claude-mythos-5-20"} {
		t.Run(model, func(t *testing.T) {
			pricing, err := svc.GetModelPricing(model)
			require.NoError(t, err)
			require.NotEqual(t, 8e-6, pricing.InputPricePerToken)
		})
	}
}
