package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildXAIImageMediaCost_UsesOfficialImagePrices(t *testing.T) {
	base := buildXAIImageMediaCost("grok-imagine-image", 2, "", 1)
	require.NotNil(t, base)
	require.InDelta(t, 0.002, base.InputCost, 1e-12)
	require.InDelta(t, 0.04, base.ImageOutputCost, 1e-12)
	require.Equal(t, string(BillingModeImage), base.BillingMode)

	quality := buildXAIImageMediaCost("grok-imagine-image-quality", 1, "2k", 0)
	require.NotNil(t, quality)
	require.Zero(t, quality.InputCost)
	require.InDelta(t, 0.07, quality.ImageOutputCost, 1e-12)
}

func TestBuildXAIVideoMediaCost_UsesOfficialVideoPrices(t *testing.T) {
	base := buildXAIVideoMediaCost("grok-imagine-video", map[string]any{
		"duration":   2,
		"resolution": "720p",
		"image":      map[string]any{"url": "https://example.test/still.png"},
	})
	require.NotNil(t, base)
	require.InDelta(t, 0.002, base.InputCost, 1e-12)
	require.InDelta(t, 0.14, base.OutputCost, 1e-12)

	preview := buildXAIVideoMediaCost("grok-imagine-video-1.5-preview", map[string]any{
		"duration":   2,
		"resolution": "480p",
		"image":      map[string]any{"url": "https://example.test/still.png"},
	})
	require.NotNil(t, preview)
	require.InDelta(t, 0.01, preview.InputCost, 1e-12)
	require.InDelta(t, 0.16, preview.OutputCost, 1e-12)
	require.Equal(t, string(BillingModeImage), preview.BillingMode)
}

func TestApplyProviderMediaCostMultiplier(t *testing.T) {
	cost := applyProviderMediaCostMultiplier(&CostBreakdown{
		InputCost:   0.01,
		OutputCost:  0.16,
		BillingMode: string(BillingModeImage),
	}, 1.5)

	require.NotNil(t, cost)
	require.InDelta(t, 0.17, cost.TotalCost, 1e-12)
	require.InDelta(t, 0.255, cost.ActualCost, 1e-12)
	require.Equal(t, string(BillingModeImage), cost.BillingMode)
}

func TestBuildXAIMediaCostFromUsageOrEstimate_PrefersExactTicks(t *testing.T) {
	cost := buildXAIMediaCostFromUsageOrEstimate(OpenAIUsage{
		CostInUSDTicks: 300_000_000,
	}, &CostBreakdown{
		OutputCost:  0.02,
		BillingMode: string(BillingModeImage),
	})

	require.NotNil(t, cost)
	require.InDelta(t, 0.03, cost.OutputCost, 1e-12)
	require.Equal(t, string(BillingModeImage), cost.BillingMode)
}
