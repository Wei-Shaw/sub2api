//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestCalculateImageCost_DefaultPricing 测试无分组配置时使用默认价格
func TestCalculateImageCost_DefaultPricing(t *testing.T) {
	svc := &BillingService{} // pricingService 为 nil，使用硬编码默认值

	// 2K 尺寸，默认价格 $0.134 * 1.5 = $0.201
	cost := svc.CalculateImageCost("gemini-3-pro-image", "2K", 1, nil, 1.0)
	require.InDelta(t, 0.201, cost.TotalCost, 0.0001)
	require.InDelta(t, 0.201, cost.ActualCost, 0.0001)

	// 多张图片
	cost = svc.CalculateImageCost("gemini-3-pro-image", "2K", 3, nil, 1.0)
	require.InDelta(t, 0.603, cost.TotalCost, 0.0001)
}

// TestCalculateImageCost_GroupCustomPricing 测试分组自定义价格
func TestCalculateImageCost_GroupCustomPricing(t *testing.T) {
	svc := &BillingService{}

	price1K := 0.10
	price2K := 0.15
	price4K := 0.30
	groupConfig := &ImagePriceConfig{
		Price1K: &price1K,
		Price2K: &price2K,
		Price4K: &price4K,
	}

	// 1K 使用分组价格
	cost := svc.CalculateImageCost("gemini-3-pro-image", "1K", 2, groupConfig, 1.0)
	require.InDelta(t, 0.20, cost.TotalCost, 0.0001)

	// 2K 使用分组价格
	cost = svc.CalculateImageCost("gemini-3-pro-image", "2K", 1, groupConfig, 1.0)
	require.InDelta(t, 0.15, cost.TotalCost, 0.0001)

	// 4K 使用分组价格
	cost = svc.CalculateImageCost("gemini-3-pro-image", "4K", 1, groupConfig, 1.0)
	require.InDelta(t, 0.30, cost.TotalCost, 0.0001)
}

func TestCalculateImageCost_NormalizesInvalidSizeTo2K(t *testing.T) {
	svc := &BillingService{}

	price2K := 0.25
	groupConfig := &ImagePriceConfig{Price2K: &price2K}

	for _, imageSize := range []string{"", "auto", "not-a-size"} {
		t.Run(imageSize, func(t *testing.T) {
			cost := svc.CalculateImageCost("gemini-3-pro-image", imageSize, 2, groupConfig, 1.0)
			require.InDelta(t, 0.50, cost.TotalCost, 0.0001)
			require.InDelta(t, 0.50, cost.ActualCost, 0.0001)
		})
	}
}

// TestCalculateImageCost_4KDoublePrice 测试 4K 默认价格翻倍
func TestCalculateImageCost_4KDoublePrice(t *testing.T) {
	svc := &BillingService{}

	// 4K 尺寸，默认价格翻倍 $0.134 * 2 = $0.268
	cost := svc.CalculateImageCost("gemini-3-pro-image", "4K", 1, nil, 1.0)
	require.InDelta(t, 0.268, cost.TotalCost, 0.0001)
}

// TestCalculateImageCost_RateMultiplier 测试费率倍数
func TestCalculateImageCost_RateMultiplier(t *testing.T) {
	svc := &BillingService{}

	// 费率倍数 1.5x
	cost := svc.CalculateImageCost("gemini-3-pro-image", "2K", 1, nil, 1.5)
	require.InDelta(t, 0.201, cost.TotalCost, 0.0001)   // TotalCost = 0.134 * 1.5
	require.InDelta(t, 0.3015, cost.ActualCost, 0.0001) // ActualCost = 0.201 * 1.5

	// 费率倍数 2.0x
	cost = svc.CalculateImageCost("gemini-3-pro-image", "2K", 2, nil, 2.0)
	require.InDelta(t, 0.402, cost.TotalCost, 0.0001)
	require.InDelta(t, 0.804, cost.ActualCost, 0.0001)
}

// TestCalculateImageCost_ZeroCount 测试 imageCount=0
func TestCalculateImageCost_ZeroCount(t *testing.T) {
	svc := &BillingService{}

	cost := svc.CalculateImageCost("gemini-3-pro-image", "2K", 0, nil, 1.0)
	require.Equal(t, 0.0, cost.TotalCost)
	require.Equal(t, 0.0, cost.ActualCost)
}

// TestCalculateImageCost_NegativeCount 测试 imageCount=-1
func TestCalculateImageCost_NegativeCount(t *testing.T) {
	svc := &BillingService{}

	cost := svc.CalculateImageCost("gemini-3-pro-image", "2K", -1, nil, 1.0)
	require.Equal(t, 0.0, cost.TotalCost)
	require.Equal(t, 0.0, cost.ActualCost)
}

// TestCalculateImageCost_ZeroRateMultiplier 锁定新行为：倍率 0 直接按 0 计费
// （保存时已强制 > 0；若仍有 0 泄漏到计费层，零消耗比历史的 1.0 更安全）。
func TestCalculateImageCost_ZeroRateMultiplier(t *testing.T) {
	svc := &BillingService{}

	cost := svc.CalculateImageCost("gemini-3-pro-image", "2K", 1, nil, 0)
	require.InDelta(t, 0.201, cost.TotalCost, 0.0001)
	require.InDelta(t, 0.0, cost.ActualCost, 1e-10)
}

// TestGetImageUnitPrice_GroupPriorityOverDefault 测试分组价格优先于默认价格
func TestGetImageUnitPrice_GroupPriorityOverDefault(t *testing.T) {
	svc := &BillingService{}

	price2K := 0.20
	groupConfig := &ImagePriceConfig{
		Price2K: &price2K,
	}

	// 分组配置了 2K 价格，应该使用分组价格而不是默认的 $0.134
	cost := svc.CalculateImageCost("gemini-3-pro-image", "2K", 1, groupConfig, 1.0)
	require.InDelta(t, 0.20, cost.TotalCost, 0.0001)
}

// TestGetImageUnitPrice_PartialGroupConfig 测试分组部分配置时回退默认
func TestGetImageUnitPrice_PartialGroupConfig(t *testing.T) {
	svc := &BillingService{}

	// 只配置 1K 价格
	price1K := 0.10
	groupConfig := &ImagePriceConfig{
		Price1K: &price1K,
	}

	// 1K 使用分组价格
	cost := svc.CalculateImageCost("gemini-3-pro-image", "1K", 1, groupConfig, 1.0)
	require.InDelta(t, 0.10, cost.TotalCost, 0.0001)

	// 2K 回退默认价格 $0.201 (1.5倍)
	cost = svc.CalculateImageCost("gemini-3-pro-image", "2K", 1, groupConfig, 1.0)
	require.InDelta(t, 0.201, cost.TotalCost, 0.0001)

	// 4K 回退默认价格 $0.268 (翻倍)
	cost = svc.CalculateImageCost("gemini-3-pro-image", "4K", 1, groupConfig, 1.0)
	require.InDelta(t, 0.268, cost.TotalCost, 0.0001)
}

// TestGetDefaultImagePrice_FallbackHardcoded 测试 PricingService 无数据时使用硬编码默认值
func TestGetDefaultImagePrice_FallbackHardcoded(t *testing.T) {
	svc := &BillingService{} // pricingService 为 nil

	// 1K 默认价格 $0.134，2K 默认价格 $0.201 (1.5倍)
	cost := svc.CalculateImageCost("gemini-3-pro-image", "1K", 1, nil, 1.0)
	require.InDelta(t, 0.134, cost.TotalCost, 0.0001)

	cost = svc.CalculateImageCost("gemini-3-pro-image", "2K", 1, nil, 1.0)
	require.InDelta(t, 0.201, cost.TotalCost, 0.0001)
}

// TestGetDefaultAdobeImagePrice 单测 Adobe 兜底价的分档正确性。
// 只覆盖表本身；integration 通过 CalculateImageCost 的下面几个测试验证 wire。
func TestGetDefaultAdobeImagePrice(t *testing.T) {
	cases := []struct {
		model                  string
		want1K, want2K, want4K float64
	}{
		// nano 档 —— Google Gemini flash 系
		{"nano-banana", 0.02, 0.04, 0.08},
		{"nano-banana-pro", 0.02, 0.04, 0.08},
		{"nano-banana2", 0.02, 0.04, 0.08},
		// gpt-image 档
		{"gpt-image", 0.05, 0.08, 0.15},
		{"gpt-image-1", 0.05, 0.08, 0.15},
		{"gpt-image-1-mini", 0.05, 0.08, 0.15},
		{"gpt-image-1.5", 0.05, 0.08, 0.15},
		{"gpt-image-2", 0.05, 0.08, 0.15},
		// premium 档 —— gpt-image-2.5 旗舰
		{"gpt-image-2.5-flare", 0.10, 0.15, 0.25},
		{"gpt-image-2.5-prism", 0.10, 0.15, 0.25},
		{"gpt-image-2.5-sunburst", 0.10, 0.15, 0.25},
		// third 档
		{"flux-pro", 0.08, 0.12, 0.20},
		{"flux-ultra", 0.08, 0.12, 0.20},
		{"imagen-4", 0.08, 0.12, 0.20},
		{"imagen-4-fast", 0.08, 0.12, 0.20},
		{"gpt-4o-image", 0.08, 0.12, 0.20},
		{"runway-gen4-image", 0.08, 0.12, 0.20},
	}
	for _, c := range cases {
		t.Run(c.model, func(t *testing.T) {
			p, ok := getDefaultAdobeImagePrice(c.model, "1K")
			require.True(t, ok)
			require.InDelta(t, c.want1K, p, 1e-9)
			p, ok = getDefaultAdobeImagePrice(c.model, "2K")
			require.True(t, ok)
			require.InDelta(t, c.want2K, p, 1e-9)
			p, ok = getDefaultAdobeImagePrice(c.model, "4K")
			require.True(t, ok)
			require.InDelta(t, c.want4K, p, 1e-9)
		})
	}
}

// TestGetDefaultAdobeImagePriceMisses 保证表本身不误命中非 Adobe 对外名。
// gpt-image-1 仍在表内（Adobe 分组要用）；平台隔离由 getDefaultImagePrice 负责。
func TestGetDefaultAdobeImagePriceMisses(t *testing.T) {
	for _, model := range []string{
		"gemini-3-pro-image",     // gemini 官方
		"grok-imagine-image-2.0", // Grok
		"claude-opus-5",          // Anthropic
		"firefly-imagen-4",       // 内部族 id 也不该命中：Step 8 之后请求方永远拿到干净外部名
		"",                       // 空串
		"unknown-future-model",   // 未知
	} {
		_, ok := getDefaultAdobeImagePrice(model, "1K")
		require.False(t, ok, "%s 不应被 adobe 分档命中", model)
	}
}

func TestGetDefaultImagePrice_DoesNotApplyAdobeTiersWithoutAdobePlatform(t *testing.T) {
	svc := &BillingService{}
	for _, platform := range []string{"", PlatformOpenAI, PlatformGemini, PlatformGrok, "OpenAI"} {
		for _, model := range []string{"gpt-image-1", "gpt-image-2.5-flare"} {
			got := svc.getDefaultImagePrice(model, "1K", platform)
			adobePrice, _ := getDefaultAdobeImagePrice(model, "1K")
			require.NotEqual(t, adobePrice, got,
				"platform=%q model=%s 不得套 Adobe 档 $%v", platform, model, adobePrice)
			require.InDelta(t, defaultImageGenerationPrice, got, 1e-9,
				"无 output_cost_per_image 时应回落通用 $0.134")
		}
	}
}

// TestCalculateImageCost_AdobeTierWireup 是 wire 生效的整链验证：
// 仅 platform=adobe 时 nano-banana 走 $0.02，gpt-image-2.5-flare 4K 走 $0.25。
func TestCalculateImageCost_AdobeTierWireup(t *testing.T) {
	svc := &BillingService{}
	cost := svc.CalculateImageCostWithPlatform("nano-banana", "1K", 1, nil, 1.0, PlatformAdobe)
	require.InDelta(t, 0.02, cost.TotalCost, 1e-9,
		"nano-banana 1K 应走 adobe 分档 $0.02，而不是通用 $0.134")
	cost = svc.CalculateImageCostWithPlatform("gpt-image-2.5-flare", "4K", 1, nil, 1.0, PlatformAdobe)
	require.InDelta(t, 0.25, cost.TotalCost, 1e-9)
	cost = svc.CalculateImageCostWithPlatform("gpt-image-1", "1K", 1, nil, 1.0, "ADOBE")
	require.InDelta(t, 0.05, cost.TotalCost, 1e-9)
}

func TestCalculateImageCost_OpenAIGptImageDoesNotUseAdobeTiers(t *testing.T) {
	svc := &BillingService{}
	cost := svc.CalculateImageCost("gpt-image-1", "1K", 1, nil, 1.0)
	require.InDelta(t, defaultImageGenerationPrice, cost.TotalCost, 1e-9,
		"无 Adobe 平台时 gpt-image-1 不得按 Adobe $0.05 计")
	cost = svc.CalculateImageCostWithPlatform("gpt-image-1", "1K", 1, nil, 1.0, PlatformOpenAI)
	require.InDelta(t, defaultImageGenerationPrice, cost.TotalCost, 1e-9)
	cost = svc.CalculateImageCostWithPlatform("gpt-image-2.5-flare", "1K", 1, nil, 1.0, PlatformOpenAI)
	require.InDelta(t, defaultImageGenerationPrice, cost.TotalCost, 1e-9)
	cost = svc.CalculateImageCostWithPlatform("gpt-image-2.5-flare", "4K", 1, nil, 1.0, PlatformOpenAI)
	require.InDelta(t, defaultImageGenerationPrice*2, cost.TotalCost, 1e-9,
		"flare 4K 应是通用 $0.268，不是 Adobe premium $0.25")
}

func TestCalculateImageCost_NonAdobeLiteLLMImagePriceNotBlocked(t *testing.T) {
	svc := &BillingService{
		pricingService: &PricingService{
			pricingData: map[string]*LiteLLMModelPricing{
				"gpt-image-2.5-flare": {OutputCostPerImage: 0.04},
			},
		},
	}
	cost := svc.CalculateImageCostWithPlatform("gpt-image-2.5-flare", "1K", 1, nil, 1.0, PlatformOpenAI)
	require.InDelta(t, 0.04, cost.TotalCost, 1e-9,
		"非 Adobe 必须能落到 LiteLLM output_cost_per_image，不能被 Adobe 档截走")
	cost = svc.CalculateImageCostWithPlatform("gpt-image-2.5-flare", "1K", 1, nil, 1.0, PlatformAdobe)
	require.InDelta(t, 0.10, cost.TotalCost, 1e-9,
		"Adobe 平台仍走 premium 档，不被 LiteLLM 图片单价覆盖")
}

func TestCalculateOpenAIImageCost_IsolatesAdobeTiersByAccountPlatform(t *testing.T) {
	svc := &OpenAIGatewayService{billingService: &BillingService{}}
	result := &OpenAIForwardResult{ImageCount: 1, ImageSize: "1K"}
	apiKey := &APIKey{}

	openaiCost := svc.calculateOpenAIImageCost(t.Context(), "gpt-image-1", apiKey, result, 1.0, PlatformOpenAI)
	require.InDelta(t, defaultImageGenerationPrice, openaiCost.TotalCost, 1e-9)

	adobeCost := svc.calculateOpenAIImageCost(t.Context(), "gpt-image-1", apiKey, result, 1.0, PlatformAdobe)
	require.InDelta(t, 0.05, adobeCost.TotalCost, 1e-9)

	flareOpenAI := svc.calculateOpenAIImageCost(t.Context(), "gpt-image-2.5-flare", apiKey, result, 1.0, PlatformOpenAI)
	require.InDelta(t, defaultImageGenerationPrice, flareOpenAI.TotalCost, 1e-9)
	flareAdobe := svc.calculateOpenAIImageCost(t.Context(), "gpt-image-2.5-flare", apiKey, result, 1.0, PlatformAdobe)
	require.InDelta(t, 0.10, flareAdobe.TotalCost, 1e-9)
}
