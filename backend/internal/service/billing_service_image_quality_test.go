//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// imageQualityResolved 构造一个带 (size_tier × quality) 二维分层 + 单维兼容项的定价解析结果。
func imageQualityResolved() *ResolvedPricing {
	return &ResolvedPricing{
		Mode: BillingModeImage,
		RequestTiers: []PricingInterval{
			// 二维分层：1K 区分 low/high
			{TierLabel: "1K", Quality: "low", PerRequestPrice: testPtrFloat64(0.01)},
			{TierLabel: "1K", Quality: "high", PerRequestPrice: testPtrFloat64(0.03)},
			// 二维分层：2K 区分 high
			{TierLabel: "2K", Quality: "high", PerRequestPrice: testPtrFloat64(0.06)},
			// 存量单维：4K 不区分 quality
			{TierLabel: "4K", Quality: "", PerRequestPrice: testPtrFloat64(0.16)},
		},
		DefaultPerRequestPrice: 0.10,
	}
}

// TestCalculateCostUnified_ImageQuality_ExactMatch 命中 (size_tier × quality) 二维定价。
func TestCalculateCostUnified_ImageQuality_ExactMatch(t *testing.T) {
	bs := newTestBillingService()
	resolver := NewModelPricingResolver(nil, bs)
	resolved := imageQualityResolved()

	cost, err := bs.CalculateCostUnified(CostInput{
		Ctx:            context.Background(),
		Model:          "gpt-image-2",
		RequestCount:   2,
		SizeTier:       "1K",
		Quality:        "high",
		RateMultiplier: 1.0,
		Resolver:       resolver,
		Resolved:       resolved,
	})
	require.NoError(t, err)
	// 2 张 × 0.03 = 0.06
	require.InDelta(t, 0.06, cost.TotalCost, 1e-9)
	require.InDelta(t, 0.06, cost.ActualCost, 1e-9)
	require.Equal(t, string(BillingModeImage), cost.BillingMode)

	// 同尺寸不同质量价格不同
	costLow, err := bs.CalculateCostUnified(CostInput{
		Ctx: context.Background(), Model: "gpt-image-2", RequestCount: 1,
		SizeTier: "1K", Quality: "low", RateMultiplier: 1.0, Resolver: resolver, Resolved: resolved,
	})
	require.NoError(t, err)
	require.InDelta(t, 0.01, costLow.TotalCost, 1e-9)
}

// TestCalculateCostUnified_ImageQuality_FallbackToSingleDimension 命中存量单维定价（quality 维度为空）。
func TestCalculateCostUnified_ImageQuality_FallbackToSingleDimension(t *testing.T) {
	bs := newTestBillingService()
	resolver := NewModelPricingResolver(nil, bs)
	resolved := imageQualityResolved()

	// 4K 仅有单维定价（quality=""），即便请求带 quality 也应回退命中。
	cost, err := bs.CalculateCostUnified(CostInput{
		Ctx: context.Background(), Model: "gpt-image-2", RequestCount: 1,
		SizeTier: "4K", Quality: "high", RateMultiplier: 1.0, Resolver: resolver, Resolved: resolved,
	})
	require.NoError(t, err)
	require.InDelta(t, 0.16, cost.TotalCost, 1e-9)
}

// TestCalculateCostUnified_ImageQuality_MissTierFallsBackToDefault 二维与单维均未命中时回退默认按次价。
func TestCalculateCostUnified_ImageQuality_MissTierFallsBackToDefault(t *testing.T) {
	bs := newTestBillingService()
	resolver := NewModelPricingResolver(nil, bs)
	resolved := imageQualityResolved()

	// 2K + low 没有二维项，也没有 2K 单维项 → 回退 DefaultPerRequestPrice。
	cost, err := bs.CalculateCostUnified(CostInput{
		Ctx: context.Background(), Model: "gpt-image-2", RequestCount: 3,
		SizeTier: "2K", Quality: "low", RateMultiplier: 1.0, Resolver: resolver, Resolved: resolved,
	})
	require.NoError(t, err)
	// 3 × 0.10 = 0.30
	require.InDelta(t, 0.30, cost.TotalCost, 1e-9)
}

// TestCalculateCostUnified_ImageQuality_LegacyNoQuality 存量请求不带 quality，命中单维定价。
func TestCalculateCostUnified_ImageQuality_LegacyNoQuality(t *testing.T) {
	bs := newTestBillingService()
	resolver := NewModelPricingResolver(nil, bs)
	resolved := &ResolvedPricing{
		Mode: BillingModeImage,
		RequestTiers: []PricingInterval{
			{TierLabel: "2K", Quality: "", PerRequestPrice: testPtrFloat64(0.08)},
		},
		DefaultPerRequestPrice: 0.10,
	}

	cost, err := bs.CalculateCostUnified(CostInput{
		Ctx: context.Background(), Model: "gpt-image-2", RequestCount: 1,
		SizeTier: "2K", Quality: "", RateMultiplier: 1.0, Resolver: resolver, Resolved: resolved,
	})
	require.NoError(t, err)
	require.InDelta(t, 0.08, cost.TotalCost, 1e-9)
}

// TestGetRequestTierPriceWithQuality_Priority 直接验证二维查找的优先级与回退。
func TestGetRequestTierPriceWithQuality_Priority(t *testing.T) {
	resolver := &ModelPricingResolver{}
	resolved := imageQualityResolved()

	// 1) 二维完全匹配优先
	require.InDelta(t, 0.03, resolver.GetRequestTierPriceWithQuality(resolved, "1K", "high"), 1e-9)
	// 2) 二维未命中但存量单维存在 → 回退单维
	require.InDelta(t, 0.16, resolver.GetRequestTierPriceWithQuality(resolved, "4K", "high"), 1e-9)
	// 3) 二维与单维均未命中 → 0（由上层回退默认价）
	require.InDelta(t, 0.0, resolver.GetRequestTierPriceWithQuality(resolved, "2K", "low"), 1e-9)
	// 4) quality 为空时退化为单维查找
	require.InDelta(t, 0.16, resolver.GetRequestTierPriceWithQuality(resolved, "4K", ""), 1e-9)
}
