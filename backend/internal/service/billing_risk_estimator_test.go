//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func newBillingRiskEstimatorTest() *BillingRiskEstimator {
	billing := &BillingService{fallbackPrices: map[string]*ModelPricing{
		"claude-sonnet-4": {
			InputPricePerToken:  1e-6,
			OutputPricePerToken: 2e-6,
		},
		"claude-opus-4.5": {
			InputPricePerToken:  20e-6,
			OutputPricePerToken: 80e-6,
		},
	}}
	return NewBillingRiskEstimator(billing, NewModelPricingResolver(nil, billing))
}

func billingRiskTokenEstimate(model string, multiplier float64) BillingRiskEstimateInput {
	return BillingRiskEstimateInput{
		Model:          model,
		Tokens:         UsageTokens{InputTokens: 1_000, OutputTokens: 500},
		RateMultiplier: multiplier,
	}
}

func TestBillingRiskEstimatorAppliesUserAndPeakMultiplier(t *testing.T) {
	estimator := newBillingRiskEstimatorTest()
	ctx := context.Background()

	base, err := estimator.Estimate(ctx, billingRiskTokenEstimate("claude-sonnet-4", 1))
	require.NoError(t, err)
	highUserRate, err := estimator.Estimate(ctx, billingRiskTokenEstimate("claude-sonnet-4", 3))
	require.NoError(t, err)
	peakRate, err := estimator.Estimate(ctx, billingRiskTokenEstimate("claude-sonnet-4", 6))
	require.NoError(t, err)

	require.True(t, base.Certain)
	require.InDelta(t, base.Cost*3, highUserRate.Cost, 1e-12)
	require.InDelta(t, base.Cost*6, peakRate.Cost, 1e-12)
}

func TestBillingRiskEstimatorAppliesPriorityServiceTier(t *testing.T) {
	estimator := newBillingRiskEstimatorTest()
	ctx := context.Background()
	standardInput := billingRiskTokenEstimate("claude-sonnet-4", 1)
	priorityInput := standardInput
	priorityInput.ServiceTier = "priority"

	standard, err := estimator.Estimate(ctx, standardInput)
	require.NoError(t, err)
	priority, err := estimator.Estimate(ctx, priorityInput)
	require.NoError(t, err)

	require.InDelta(t, standard.Cost*2, priority.Cost, 1e-12)
}

func TestBillingRiskEstimatorUsesGroupExplicitHighPrice(t *testing.T) {
	estimator := newBillingRiskEstimatorTest()
	ctx := context.Background()
	inputPrice := 100e-6
	outputPrice := 300e-6
	group := &Group{
		Platform: PlatformOpenAI,
		ModelPricing: []ChannelModelPricing{{
			Platform:    PlatformOpenAI,
			Models:      []string{"claude-sonnet-4"},
			BillingMode: BillingModeToken,
			InputPrice:  &inputPrice,
			OutputPrice: &outputPrice,
		}},
	}
	input := billingRiskTokenEstimate("claude-sonnet-4", 1)
	input.Group = group

	result, err := estimator.Estimate(ctx, input)
	require.NoError(t, err)
	require.True(t, result.Certain)
	require.Equal(t, PricingSourceGroup, result.Source)
	require.InDelta(t, 1_000*inputPrice+500*outputPrice, result.Cost, 1e-12)
}

func TestBillingRiskEstimatorUsesFinalMappedExpensiveModel(t *testing.T) {
	estimator := newBillingRiskEstimatorTest()
	ctx := context.Background()

	requested, err := estimator.Estimate(ctx, billingRiskTokenEstimate("claude-sonnet-4", 1))
	require.NoError(t, err)
	mapped, err := estimator.Estimate(ctx, billingRiskTokenEstimate("claude-opus-4.5", 1))
	require.NoError(t, err)

	require.True(t, mapped.Certain)
	require.Greater(t, mapped.Cost, requested.Cost*10)
}

func TestBillingRiskEstimatorPricesPerRequestMediaWithMultiplier(t *testing.T) {
	estimator := newBillingRiskEstimatorTest()
	ctx := context.Background()
	price := 0.75
	group := &Group{
		Platform: PlatformOpenAI,
		ModelPricing: []ChannelModelPricing{{
			Platform:        PlatformOpenAI,
			Models:          []string{"custom-image"},
			BillingMode:     BillingModeImage,
			PerRequestPrice: &price,
		}},
	}

	result, err := estimator.Estimate(ctx, BillingRiskEstimateInput{
		Model:          "custom-image",
		Group:          group,
		RequestCount:   2,
		RateMultiplier: 4,
	})

	require.NoError(t, err)
	require.True(t, result.Certain)
	require.InDelta(t, 6, result.Cost, 1e-12)
}

func TestBillingRiskEstimatorUnknownPriceIsUncertainInsteadOfFree(t *testing.T) {
	estimator := newBillingRiskEstimatorTest()

	result, err := estimator.Estimate(context.Background(), billingRiskTokenEstimate("unknown-model", 1))

	require.NoError(t, err)
	require.False(t, result.Certain)
	require.Zero(t, result.Cost)
}
