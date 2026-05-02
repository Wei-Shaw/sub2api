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
