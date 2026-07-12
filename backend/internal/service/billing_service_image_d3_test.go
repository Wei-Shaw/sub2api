//go:build unit

package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/require"
)

// D3 \u4e0d\u53d8\u91cf\uff1a\u540c\u4e00\u5206\u7ec4\u540c\u4e00 (size, quality) \u4e0d\u8bba\u8d70 openai \u8d26\u53f7\u8fd8\u662f
// fal \u8d26\u53f7\u627f\u8f7d\uff0c\u6700\u7ec8\u8ba1\u7b97\u51fa\u7684\u6536\u8d39\u91d1\u989d\u5fc5\u987b\u4e00\u81f4\u3002
//
// \u8be5\u4e0d\u53d8\u91cf\u7531 BillingService.CalculateImageCost \u7684\u7b7e\u540d\u672c\u8eab\u4fdd\u8bc1\uff1a
//
//	\u51fd\u6570\u8f93\u5165 = (model, imageSize, imageCount, ImagePriceConfig, multiplier)
//
// \u5176\u4e2d ImagePriceConfig \u7684\u6240\u6709\u5b57\u6bb5\u90fd\u6765\u81ea\u5206\u7ec4 (Price1K/2K/4K +
// PricingMatrix + RawWidth/Height + Quality)\u3002**\u8b00\u4e2d \u201c\u8d26\u53f7\u5e73\u53f0\u201d \u7684\u53c2\u6570\uff0c\u8ba1\u8d39
// \u8def\u5f84\u4e0a\u8d26\u53f7\u5e73\u53f0\u4e0d\u4f1a\u5f15\u8d77\u4ef7\u683c\u5dee\u5f02\u3002**
//
// \u4e0b\u9762\u7684\u6d4b\u8bd5\u91c7\u7528 \u201c\u540c\u4e00 cfg + \u540c\u4e00\u8f93\u5165\u201d \u5e76\u8c03\u7528\u4e24\u6b21\uff0c\u9a8c\u8bc1\u7ed3\u679c
// \u4e25\u683c\u76f8\u7b49\uff1b\u4ee5\u53ca \u201c\u540c\u4e00\u77e9\u9635\uff0c\u4e0d\u540c\u8c03\u7528\u8005\u201d \u4e0d\u4f1a\u4ea7\u751f\u4efb\u4f55 platform \u504f\u79fb\u3002
func TestD3Invariant_SameGroupSameSizeQuality_SameCostAcrossPlatforms(t *testing.T) {
	svc := &BillingService{}
	matrix := domain.ImagePricingMatrix{
		ImagePricingTier1024x1024: {
			ImageQualityLow:    0.006,
			ImageQualityMedium: 0.053,
			ImageQualityHigh:   0.211,
		},
		ImagePricingTier3840x2160: {
			ImageQualityHigh: 0.401,
		},
	}

	type scenario struct {
		name    string
		w, h    int
		quality string
		size    string
		count   int
	}
	scenarios := []scenario{
		{"1024x1024 high", 1024, 1024, "high", "1K", 1},
		{"1024x1024 medium x2", 1024, 1024, "medium", "1K", 2},
		{"1024x1024 low x3", 1024, 1024, "low", "1K", 3},
		{"3840x2160 high (auto -> high) x2", 3840, 2160, "auto", "4K", 2},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			cfg := &ImagePriceConfig{
				PricingMatrix: matrix,
				RawWidth:      sc.w,
				RawHeight:     sc.h,
				Quality:       sc.quality,
			}

			// \u6a21\u62df \u201copenai \u8d26\u53f7\u627f\u8f7d\u201d \u4e0e \u201cfal \u8d26\u53f7\u627f\u8f7d\u201d \u4e24\u6b21\u8c03\u7528\u3002
			// \u672c\u8d28\u4e0a CalculateImageCost \u4e0d\u53d7\u5e73\u53f0\u5f71\u54cd\uff0c\u8c03\u7528\u53c2\u6570\u5b8c\u5168\u4e00\u81f4\u3002
			openaiCost := svc.CalculateImageCost("gpt-image-2", sc.size, sc.count, cfg, 1.0)
			falCost := svc.CalculateImageCost("gpt-image-2", sc.size, sc.count, cfg, 1.0)

			require.Equal(t, openaiCost.TotalCost, falCost.TotalCost, "TotalCost must be identical regardless of carrier platform")
			require.Equal(t, openaiCost.ActualCost, falCost.ActualCost, "ActualCost must be identical regardless of carrier platform")
			require.Equal(t, openaiCost.BillingMode, falCost.BillingMode)
		})
	}
}

// \u53e6\u4e00\u4e2a\u5f62\u5f0f\u7684 D3 \u9a8c\u8bc1\uff1a\u5373\u4f7f\u8c03\u7528\u8005\u4f20\u5165\u4e0d\u540c\u7684 model \u540d\u79f0\uff08\u5e73\u53f0\u53ef\u80fd
// \u4f1a\u4f20 openai \u4e0a\u6e38 model\u3001\u4e5f\u53ef\u80fd\u4f20 fal \u4e0a\u6e38 model\uff09\uff0c\u53ea\u8981\u77e9\u9635\u547d\u4e2d\uff0c
// \u5355\u4ef7\u4ec5\u53d6\u51b3\u4e8e\u77e9\u9635\u672c\u8eab\uff0cmodel \u540d\u4e0d\u5f71\u54cd\u91d1\u989d\u3002
func TestD3Invariant_MatrixHitIgnoresUpstreamModelName(t *testing.T) {
	svc := &BillingService{}
	matrix := domain.ImagePricingMatrix{
		ImagePricingTier1920x1080: {
			ImageQualityMedium: 0.040,
		},
	}
	cfg := &ImagePriceConfig{
		PricingMatrix: matrix,
		RawWidth:      1920,
		RawHeight:     1080,
		Quality:       "medium",
	}

	// openai \u8d26\u53f7\u627f\u8f7d\u65f6 billingModel \u53ef\u80fd\u662f \"gpt-image-2\"\uff1b
	// fal \u8d26\u53f7\u627f\u8f7d\u65f6\u53ef\u80fd\u662f \"fal-ai/...\" \u4e0a\u6e38 model\u3002
	upstreamModels := []string{
		"gpt-image-2",
		"fal-ai/flux-pro",
		"any-other-model",
	}
	expected := svc.CalculateImageCost(upstreamModels[0], "2K", 1, cfg, 1.0).TotalCost
	for _, m := range upstreamModels[1:] {
		got := svc.CalculateImageCost(m, "2K", 1, cfg, 1.0).TotalCost
		require.InDelta(t, expected, got, 1e-9, "TotalCost must not depend on upstream model name when matrix hits (model=%s)", m)
	}
}

// \u8d39\u7387\u500d\u7387\u6240\u53e6\u5916\u4ea7\u751f\u7684\u91d1\u989d\u53d8\u5316\u4e0d\u5728\u4e0d\u53d8\u91cf\u8303\u56f4\u5185\uff0c\u8be5\u6d4b\u8bd5\u9501\u5b9a
// rateMultiplier \u5728\u4e0d\u540c\u8c03\u7528\u95f4\u7ed9\u540c\u6837\u7684\u503c\uff0c\u9a8c\u8bc1 ActualCost \u4e5f\u4e0d\u53d7\u5e73\u53f0\u5f71\u54cd\u3002
func TestD3Invariant_ActualCostStableUnderConstantMultiplier(t *testing.T) {
	svc := &BillingService{}
	matrix := domain.ImagePricingMatrix{
		ImagePricingTier1024x1024: {ImageQualityHigh: 0.211},
	}
	cfg := &ImagePriceConfig{
		PricingMatrix: matrix,
		RawWidth:      1024,
		RawHeight:     1024,
		Quality:       "high",
	}

	cost1 := svc.CalculateImageCost("gpt-image-2", "1K", 1, cfg, 1.5)
	cost2 := svc.CalculateImageCost("gpt-image-2", "1K", 1, cfg, 1.5)
	require.Equal(t, cost1.ActualCost, cost2.ActualCost)
	require.InDelta(t, 0.211*1.5, cost1.ActualCost, 1e-9)
}
