package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestZeroOutputFreeCostDisabledKeepsCost(t *testing.T) {
	cost := &CostBreakdown{InputCost: 1, OutputCost: 2, TotalCost: 3, ActualCost: 4, BillingMode: string(BillingModeToken)}
	got := zeroOutputFreeCost(&config.Config{}, cost, 0, 0, false)
	require.Same(t, cost, got)
	require.Equal(t, 3.0, got.TotalCost)
	require.Equal(t, 4.0, got.ActualCost)
}

func TestZeroOutputFreeCostClearsCustomerCharges(t *testing.T) {
	cfg := &config.Config{}
	cfg.Billing.ZeroOutputFree = true
	cost := &CostBreakdown{
		InputCost:                 1,
		ImageInputCost:            2,
		OutputCost:                3,
		ImageOutputCost:           4,
		CacheCreationCost:         5,
		CacheReadCost:             6,
		TotalCost:                 21,
		ActualCost:                22,
		BillingMode:               string(BillingModeToken),
		LongContextBillingApplied: true,
	}

	got := zeroOutputFreeCost(cfg, cost, 0, 0, false)
	require.Same(t, cost, got)
	require.Equal(t, CostBreakdown{
		BillingMode:               string(BillingModeToken),
		LongContextBillingApplied: true,
	}, *got)
}

func TestZeroOutputFreeCostKeepsNonZeroOutputAndMediaCharges(t *testing.T) {
	cfg := &config.Config{}
	cfg.Billing.ZeroOutputFree = true

	for _, tc := range []struct {
		name              string
		outputTokens      int
		imageOutputTokens int
		mediaUsage        bool
	}{
		{name: "text output", outputTokens: 1},
		{name: "image output", imageOutputTokens: 1},
		{name: "media request", mediaUsage: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cost := &CostBreakdown{TotalCost: 1, ActualCost: 2}
			got := zeroOutputFreeCost(cfg, cost, tc.outputTokens, tc.imageOutputTokens, tc.mediaUsage)
			require.Same(t, cost, got)
			require.Equal(t, 1.0, got.TotalCost)
			require.Equal(t, 2.0, got.ActualCost)
		})
	}
}
