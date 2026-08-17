//go:build unit

package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/require"
)

func testImageTiers() []ImagePricingTier {
	return []ImagePricingTier{
		{Label: "1K", Resolution: "1024x1536"},
		{Label: "2K", Resolution: "2048x3072"},
		{Label: "4K", Resolution: "4096x6144"},
	}
}

func TestMatchImagePricingTierRules(t *testing.T) {
	tier, err := MatchImagePricingTier(ImageDimensions{Width: 1200, Height: 1800}, testImageTiers())
	require.NoError(t, err)
	require.Equal(t, "2K", tier.Label)

	tier, err = MatchImagePricingTier(ImageDimensions{Width: 900, Height: 1800}, testImageTiers())
	require.NoError(t, err)
	require.Equal(t, "2K", tier.Label)

	_, err = MatchImagePricingTier(ImageDimensions{Width: 1000, Height: 3001}, testImageTiers())
	require.ErrorContains(t, err, "1:3")

	_, err = MatchImagePricingTier(ImageDimensions{Width: 4096, Height: 6145}, testImageTiers())
	require.ErrorContains(t, err, "total pixels")

	_, err = MatchImagePricingTier(ImageDimensions{Width: 800, Height: 800}, []ImagePricingTier{
		{Label: "1K", Resolution: "4096x4096"},
		{Label: "2K", Resolution: "2048x2048"},
		{Label: "4K", Resolution: "1024x1024"},
	})
	require.ErrorContains(t, err, "must not decrease")

	userTiers := []ImagePricingTier{
		{Label: "1K", Resolution: "1080x1080"},
		{Label: "2K", Resolution: "2160x2160"},
		{Label: "4K", Resolution: "3840x2160"},
	}
	tier, err = MatchImagePricingTier(ImageDimensions{Width: 2160, Height: 2160}, userTiers)
	require.NoError(t, err)
	require.Equal(t, "2K", tier.Label)

	tier, err = MatchImagePricingTier(ImageDimensions{Width: 3840, Height: 2160}, userTiers)
	require.NoError(t, err)
	require.Equal(t, "4K", tier.Label)
}

// ===== NormalizeImageQuality =====

func TestNormalizeImageQuality(t *testing.T) {
	cases := map[string]string{
		"low":     ImageQualityLow,
		"LOW":     ImageQualityLow,
		"medium":  ImageQualityMedium,
		"Medium":  ImageQualityMedium,
		"high":    ImageQualityHigh,
		"HIGH":    ImageQualityHigh,
		"auto":    ImageQualityHigh, // auto -> high
		"":        ImageQualityHigh, // empty -> high
		"unknown": ImageQualityHigh, // unknown -> high
		"  high ": ImageQualityHigh, // 前后空白可处理
	}
	for in, expect := range cases {
		require.Equalf(t, expect, NormalizeImageQuality(in), "input=%q", in)
	}
}

// ===== getImageUnitPrice / lookupImagePricingMatrix 三级回退 =====

func makeMatrix() domain.ImagePricingMatrix {
	return domain.ImagePricingMatrix{
		"1K": {
			ImageQualityLow:    0.006,
			ImageQualityMedium: 0.053,
			ImageQualityHigh:   0.211,
		},
		"4K": {
			ImageQualityHigh: 0.401, // 仅有 high，medium/low 缺失 -> 应回退
		},
	}
}

func TestCalculateImageCost_MatrixHit_LowMediumHigh(t *testing.T) {
	svc := &BillingService{}
	matrix := makeMatrix()

	// 1024x1024 / low
	cfg := &ImagePriceConfig{
		PricingMatrix: matrix,
		RawWidth:      1024,
		RawHeight:     1024,
		Quality:       "low",
	}
	cost := svc.CalculateImageCost("gpt-image-2", "1K", 1, cfg, 1.0)
	require.InDelta(t, 0.006, cost.TotalCost, 1e-9)

	// 1024x1024 / medium
	cfg.Quality = "medium"
	cost = svc.CalculateImageCost("gpt-image-2", "1K", 1, cfg, 1.0)
	require.InDelta(t, 0.053, cost.TotalCost, 1e-9)

	// 1024x1024 / high
	cfg.Quality = "high"
	cost = svc.CalculateImageCost("gpt-image-2", "1K", 1, cfg, 1.0)
	require.InDelta(t, 0.211, cost.TotalCost, 1e-9)
}

func TestCalculateImageCost_MatrixHit_AutoTreatedAsHigh(t *testing.T) {
	svc := &BillingService{}
	cfg := &ImagePriceConfig{
		PricingMatrix: makeMatrix(),
		RawWidth:      1024,
		RawHeight:     1024,
		Quality:       "auto",
	}
	cost := svc.CalculateImageCost("gpt-image-2", "1K", 2, cfg, 1.0)
	// high 单价 0.211 * 2 张 = 0.422
	require.InDelta(t, 0.422, cost.TotalCost, 1e-9)
}

func TestCalculateImageCost_MatrixRoundUpAcrossTiers(t *testing.T) {
	svc := &BillingService{}
	cfg := &ImagePriceConfig{
		PricingMatrix: makeMatrix(),
		// 短边 600 <= 1K 阈值 1024，应命中 1K。
		RawWidth:  800,
		RawHeight: 600,
		Quality:   "low",
	}
	cost := svc.CalculateImageCost("gpt-image-2", "1K", 1, cfg, 1.0)
	require.InDelta(t, 0.006, cost.TotalCost, 1e-9)
}

func TestCalculateImageCost_MatrixCellMissing_FallsBackToLegacyPrice(t *testing.T) {
	svc := &BillingService{}
	legacyPrice := 0.999
	cfg := &ImagePriceConfig{
		PricingMatrix: makeMatrix(),
		RawWidth:      3840,
		RawHeight:     2160,
		Quality:       "low", // 矩阵 4K 行只有 high，缺 low -> 回退
		Price4K:       &legacyPrice,
	}
	cost := svc.CalculateImageCost("gpt-image-2", "4K", 1, cfg, 1.0)
	require.InDelta(t, legacyPrice, cost.TotalCost, 1e-9)
}

func TestCalculateImageCost_MatrixEmpty_UsesLegacyAndDefault(t *testing.T) {
	svc := &BillingService{}

	// 空矩阵 + 旧字段 -> 走旧字段
	priceLegacy := 0.42
	cfg := &ImagePriceConfig{
		PricingMatrix: domain.ImagePricingMatrix{},
		RawWidth:      1024,
		RawHeight:     1024,
		Quality:       "high",
		Price1K:       &priceLegacy,
	}
	cost := svc.CalculateImageCost("gpt-image-2", "1K", 1, cfg, 1.0)
	require.InDelta(t, priceLegacy, cost.TotalCost, 1e-9)

	// nil 矩阵 + 无旧字段 -> 默认
	cfg2 := &ImagePriceConfig{
		RawWidth:  1024,
		RawHeight: 1024,
		Quality:   "high",
	}
	cost = svc.CalculateImageCost("gpt-image-2", "1K", 1, cfg2, 1.0)
	require.InDelta(t, 0.134, cost.TotalCost, 1e-9)
}

func TestCalculateImageCost_MatrixHitTakesPriorityOverLegacy(t *testing.T) {
	// 验证矩阵命中后旧字段不会再被使用
	svc := &BillingService{}
	legacy := 99.9
	cfg := &ImagePriceConfig{
		PricingMatrix: makeMatrix(),
		RawWidth:      1024,
		RawHeight:     1024,
		Quality:       "medium",
		Price1K:       &legacy, // 应该被忽略
	}
	cost := svc.CalculateImageCost("gpt-image-2", "1K", 1, cfg, 1.0)
	require.InDelta(t, 0.053, cost.TotalCost, 1e-9) // 矩阵 medium 单价
}

func TestCalculateImageCost_MatrixWithoutRawDimensions_FallsBack(t *testing.T) {
	// 矩阵存在但 RawWidth/RawHeight 缺失 -> 矩阵不命中，退到旧字段
	svc := &BillingService{}
	legacy := 0.5
	cfg := &ImagePriceConfig{
		PricingMatrix: makeMatrix(),
		// RawWidth/RawHeight 未设置（=0）
		Quality: "high",
		Price2K: &legacy,
	}
	cost := svc.CalculateImageCost("gpt-image-2", "2K", 1, cfg, 1.0)
	require.InDelta(t, legacy, cost.TotalCost, 1e-9)
}

func TestCalculateImageCost_MatrixApplyRateMultiplier(t *testing.T) {
	svc := &BillingService{}
	cfg := &ImagePriceConfig{
		PricingMatrix: makeMatrix(),
		RawWidth:      1024,
		RawHeight:     1024,
		Quality:       "high",
	}
	cost := svc.CalculateImageCost("gpt-image-2", "1K", 2, cfg, 1.5)
	// 单价 0.211 * 2 张 = 0.422 ; actual = 0.422 * 1.5 = 0.633
	require.InDelta(t, 0.422, cost.TotalCost, 1e-9)
	require.InDelta(t, 0.633, cost.ActualCost, 1e-9)
}
