//go:build unit

package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/require"
)

// ===== ClassifyImagePricingTier6 =====

func TestClassifyImagePricingTier6_ExactMatchAllSixTiers(t *testing.T) {
	cases := []struct {
		name   string
		w, h   int
		expect string
	}{
		{"1024x768", 1024, 768, ImagePricingTier1024x768},
		{"1024x1024", 1024, 1024, ImagePricingTier1024x1024},
		{"1024x1536", 1024, 1536, ImagePricingTier1024x1536},
		{"1920x1080", 1920, 1080, ImagePricingTier1920x1080},
		{"2560x1440", 2560, 1440, ImagePricingTier2560x1440},
		{"3840x2160", 3840, 2160, ImagePricingTier3840x2160},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tier, ok := ClassifyImagePricingTier6(tc.w, tc.h)
			require.True(t, ok)
			require.Equal(t, tc.expect, tier)
		})
	}
}

func TestClassifyImagePricingTier6_RoundsUpByPixelCount(t *testing.T) {
	// 800x600 = 480000 像素，第一档 1024x768=786432 已能覆盖 -> 1024x768
	tier, ok := ClassifyImagePricingTier6(800, 600)
	require.True(t, ok)
	require.Equal(t, ImagePricingTier1024x768, tier)

	// 1024x769 = 787456 > 786432，需要进到下一档 1024x1024 = 1048576
	tier, ok = ClassifyImagePricingTier6(1024, 769)
	require.True(t, ok)
	require.Equal(t, ImagePricingTier1024x1024, tier)

	// 1500x1000 = 1500000 < 1572864（1024x1536），应落到 1024x1536
	tier, ok = ClassifyImagePricingTier6(1500, 1000)
	require.True(t, ok)
	require.Equal(t, ImagePricingTier1024x1536, tier)

	// 1920x1081 = 2075520 > 2073600（1920x1080），应落到 2560x1440=3686400
	tier, ok = ClassifyImagePricingTier6(1920, 1081)
	require.True(t, ok)
	require.Equal(t, ImagePricingTier2560x1440, tier)
}

func TestClassifyImagePricingTier6_CapsAt4K(t *testing.T) {
	// 5120x2880 = 14745600 > 8294400（3840x2160），应封顶到 3840x2160
	tier, ok := ClassifyImagePricingTier6(5120, 2880)
	require.True(t, ok)
	require.Equal(t, ImagePricingTier3840x2160, tier)

	// 7680x4320 (8K) 同样封顶
	tier, ok = ClassifyImagePricingTier6(7680, 4320)
	require.True(t, ok)
	require.Equal(t, ImagePricingTier3840x2160, tier)
}

func TestClassifyImagePricingTier6_OrientationAgnostic(t *testing.T) {
	// 像素总数相同，方向无关
	tier1, _ := ClassifyImagePricingTier6(1024, 1536)
	tier2, _ := ClassifyImagePricingTier6(1536, 1024)
	require.Equal(t, tier1, tier2)
}

func TestClassifyImagePricingTier6_InvalidInputs(t *testing.T) {
	for _, dim := range [][2]int{{0, 100}, {100, 0}, {-1, 100}, {100, -1}, {0, 0}} {
		_, ok := ClassifyImagePricingTier6(dim[0], dim[1])
		require.False(t, ok, "dim=%v should be invalid", dim)
	}
}

func TestParseImagePricingTier6_ParsesAndClassifies(t *testing.T) {
	tier, ok := ParseImagePricingTier6("1920x1080")
	require.True(t, ok)
	require.Equal(t, ImagePricingTier1920x1080, tier)

	// 大写 X 也接受
	tier, ok = ParseImagePricingTier6("1024X1024")
	require.True(t, ok)
	require.Equal(t, ImagePricingTier1024x1024, tier)

	// 非法格式
	_, ok = ParseImagePricingTier6("auto")
	require.False(t, ok)
	_, ok = ParseImagePricingTier6("")
	require.False(t, ok)
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
		ImagePricingTier1024x1024: {
			ImageQualityLow:    0.006,
			ImageQualityMedium: 0.053,
			ImageQualityHigh:   0.211,
		},
		ImagePricingTier3840x2160: {
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
		// 800x600 像素 < 1024x1024，应向上取到 1024x1024 档
		RawWidth:  800,
		RawHeight: 600,
		Quality:   "low",
	}
	// 800x600 像素总数 480000 ≤ 1024x768=786432，应取 1024x768。
	// makeMatrix 中没有 1024x768 行 -> 矩阵未命中 -> 回退到旧字段或默认
	// （这里没配旧字段也没配 pricingService，回退到硬编码默认）。
	cost := svc.CalculateImageCost("gpt-image-2", "1K", 1, cfg, 1.0)
	// 默认硬编码 0.134（基于 1K，无倍率）
	require.InDelta(t, 0.134, cost.TotalCost, 1e-9)
}

func TestCalculateImageCost_MatrixCellMissing_FallsBackToLegacyPrice(t *testing.T) {
	svc := &BillingService{}
	legacyPrice := 0.999
	cfg := &ImagePriceConfig{
		PricingMatrix: makeMatrix(),
		RawWidth:      3840,
		RawHeight:     2160,
		Quality:       "low", // 矩阵 3840x2160 行只有 high，缺 low -> 回退
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
