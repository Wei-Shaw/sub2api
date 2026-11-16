//go:build unit

package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestBuiltinGeminiCacheStoragePrice(t *testing.T) {
	for _, tc := range []struct {
		model string
		want  float64
	}{
		// 已发布型号
		{"gemini-3.8-flash", 0.5e-6},
		{"gemini-3.7-flash", 0.5e-6},
		{"gemini-3.6-flash", 0.5e-6},
		{"gemini-3.8-flash-cyber", 1e-6},
		{"gemini-3.5-flash", 1e-6},
		{"gemini-3.5-flash-lite", 1e-6},
		{"gemini-3.1-flash-lite", 1e-6},
		{"gemini-3.1-pro-preview", 4.5e-6},
		{"gemini-3.1-pro-preview-customtools", 4.5e-6},
		{"gemini-3-flash-preview", 1e-6},
		{"gemini-3-pro-preview", 4.5e-6},
		{"gemini-2.5-pro", 4.5e-6},
		{"gemini-2.5-flash", 1e-6},
		{"gemini-2.5-flash-lite", 1e-6},
		{"gemini-2.0-flash-001", 1e-6},
		{"gemini-2.0-flash-lite-001", 1e-6},
		// 资源名与大小写
		{"models/gemini-3.8-flash", 0.5e-6},
		{"projects/p/locations/global/publishers/google/models/gemini-2.5-pro", 4.5e-6},
		{" Gemini-3.8-Flash ", 0.5e-6},
		// 系列兜底：未发布型号与别名按标准价
		{"gemini-3.9-flash", 1e-6},
		{"gemini-3.9-flash-lite", 1e-6},
		{"gemini-3.8-flash-lite", 1e-6},
		{"gemini-3.9-pro-preview", 4.5e-6},
		{"gemini-flash-latest", 1e-6},
		{"gemini-flash-lite-latest", 1e-6},
		{"gemini-pro-latest", 4.5e-6},
		// 无存储价（Gemini 4 Argon 仅支持隐式缓存）
		{"gemini-4-argon", 0},
		{"gemini-4-argon-high", 0},
		{"gemini-4-argon-cyber-permissive", 0},
		{"gemini-embedding-001", 0},
		{"gemini-unknown-model", 0},
		{"claude-sonnet-5-5", 0},
		{"", 0},
	} {
		require.InDelta(t, tc.want, builtinGeminiCacheStoragePrice(tc.model), 1e-15, tc.model)
	}
}

func TestCacheStoragePricePerTokenHourPriority(t *testing.T) {
	pricing := &PricingService{}
	data, err := pricing.parsePricingData([]byte(`{
		"gemini-3.8-flash":{"litellm_provider":"gemini","input_cost_per_token":0.00000075,"output_cost_per_token":0.00000375,
			"cache_storage_cost_per_token_per_hour":0.000002},
		"gemini-3.9-flash":{"litellm_provider":"gemini","input_cost_per_token":0.0000015,"output_cost_per_token":0.0000075,
			"cache_storage_cost_per_token_per_hour":0.0000007},
		"gemini-2.5-pro":{"litellm_provider":"gemini","input_cost_per_token":0.00000125,"output_cost_per_token":0.00001}}`))
	require.NoError(t, err)
	pricing.pricingData = data
	billing := NewBillingService(&config.Config{}, pricing)

	require.InDelta(t, 2e-6, billing.CacheStoragePricePerTokenHour("gemini-3.8-flash"), 1e-15, "价格表配置值优先于内置型号价")
	require.InDelta(t, 0.7e-6, billing.CacheStoragePricePerTokenHour("gemini-3.9-flash"), 1e-15, "价格表配置值优先于内置系列兜底价")
	require.InDelta(t, 4.5e-6, billing.CacheStoragePricePerTokenHour("gemini-2.5-pro"), 1e-15, "价格表条目缺存储价时取内置型号价")
	require.InDelta(t, 0.5e-6, billing.CacheStoragePricePerTokenHour("gemini-3.7-flash"), 1e-15, "价格表无条目时取内置型号价")
	require.InDelta(t, 4.5e-6, billing.CacheStoragePricePerTokenHour("gemini-3.9-pro"), 1e-15, "内置型号未命中时取系列兜底价")
	require.Zero(t, billing.CacheStoragePricePerTokenHour("customer-alias"))

	noCatalog := NewBillingService(&config.Config{}, nil)
	require.InDelta(t, 0.5e-6, noCatalog.CacheStoragePricePerTokenHour("gemini-3.8-flash"), 1e-15, "无价格服务时取内置价")
}
