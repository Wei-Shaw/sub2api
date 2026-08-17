package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// supportedModelNames 从 []SupportedModel 中提取指定平台的模型名称列表
func supportedModelNames(models []SupportedModel, platform string) []string {
	var names []string
	for _, m := range models {
		if m.Platform == platform {
			names = append(names, m.Name)
		}
	}
	return names
}

// TestMultiSourceMapping 测试多源映射路由：同一 Target 的两个 Source 各自可以命中
func TestMultiSourceMapping(t *testing.T) {
	cache := newEmptyChannelCache()

	groupID := int64(1)
	platform := "openai"

	// 两个 Source 共同映射到同一 Target
	cache.mappingByGroupModel[channelModelKey{groupID: groupID, platform: platform, model: "gpt-4"}] = "gpt-4o"
	cache.mappingByGroupModel[channelModelKey{groupID: groupID, platform: platform, model: "gpt-4-turbo"}] = "gpt-4o"

	tests := []struct {
		name     string
		model    string
		expected string
	}{
		{"first source maps correctly", "gpt-4", "gpt-4o"},
		{"second source maps correctly", "gpt-4-turbo", "gpt-4o"},
		{"GPT-4 uppercase maps correctly (case-insensitive)", "GPT-4", "gpt-4o"},
		{"unmapped model returns empty", "gpt-3.5-turbo", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := lookupMappingAcrossPlatforms(cache, groupID, platform, normalizeChannelPricingModelName(tt.model))
			require.Equal(t, tt.expected, result)
		})
	}
}

// TestHiddenMappingRouting 验证隐藏条目仍参与路由，但不出现在 SupportedModels 列表
func TestHiddenMappingRouting(t *testing.T) {
	ch := &Channel{
		ID:     1,
		Status: StatusActive,
		ModelMapping: map[string][]ModelMappingEntry{
			"openai": {
				{
					Sources: []string{"gpt-4-deprecated"},
					Target:  "gpt-4o",
					Enabled: nil,  // nil = 默认启用
					Hidden:  true, // 隐藏
				},
			},
		},
	}

	// 模拟：隐藏条目已被 expandMappingToCache 写入映射缓存（Hidden 不过滤路由缓存）
	cache := newEmptyChannelCache()
	cache.groupPlatform[1] = "openai"
	cache.channelByGroupID[1] = ch
	cache.byID[1] = ch
	cache.mappingByGroupModel[channelModelKey{groupID: 1, platform: "openai", model: "gpt-4-deprecated"}] = "gpt-4o"

	// 路由：隐藏条目仍可映射
	mapped := lookupMappingAcrossPlatforms(cache, 1, "openai", "gpt-4-deprecated")
	require.Equal(t, "gpt-4o", mapped, "hidden mapping should still route")

	// 模型列表：隐藏条目不应出现
	names := supportedModelNames(ch.SupportedModels(), "openai")
	require.NotContains(t, names, "gpt-4-deprecated", "hidden mapping should not appear in model list")
}

// TestDisabledMappingNotInCache 验证停用映射条目不进入路由缓存
func TestDisabledMappingNotInCache(t *testing.T) {
	ch := &Channel{
		ID:     1,
		Status: StatusActive,
		ModelMapping: map[string][]ModelMappingEntry{
			"openai": {
				{
					Sources: []string{"gpt-4-old"},
					Target:  "gpt-4o",
					Enabled: boolPtr(false), // 停用
				},
				{
					Sources: []string{"gpt-4"},
					Target:  "gpt-4o",
					Enabled: nil, // 启用
				},
			},
		},
	}

	cache := newEmptyChannelCache()
	cache.groupPlatform[1] = "openai"
	cache.channelByGroupID[1] = ch

	expandMappingToCache(cache, ch, 1, "openai")

	// 停用条目不应进入缓存
	_, exists := cache.mappingByGroupModel[channelModelKey{groupID: 1, platform: "openai", model: "gpt-4-old"}]
	require.False(t, exists, "disabled mapping should not be in cache")

	// 启用条目应进入缓存
	_, existsEnabled := cache.mappingByGroupModel[channelModelKey{groupID: 1, platform: "openai", model: "gpt-4"}]
	require.True(t, existsEnabled, "enabled mapping should be in cache")
}

// TestDisabledPricingNotInCache 验证停用定价条目不进入计费缓存
func TestDisabledPricingNotInCache(t *testing.T) {
	ch := &Channel{
		ID:     1,
		Status: StatusActive,
		ModelPricing: []ChannelModelPricing{
			{
				Models:      []string{"gpt-4-deprecated"},
				Platform:    "openai",
				BillingMode: BillingModeToken,
				InputPrice:  float64Ptr(0.01),
				OutputPrice: float64Ptr(0.03),
				Enabled:     boolPtr(false), // 停用
			},
			{
				Models:      []string{"gpt-4o"},
				Platform:    "openai",
				BillingMode: BillingModeToken,
				InputPrice:  float64Ptr(0.005),
				OutputPrice: float64Ptr(0.015),
				Enabled:     boolPtr(true), // 启用
			},
		},
	}

	cache := newEmptyChannelCache()
	cache.groupPlatform[1] = "openai"
	cache.channelByGroupID[1] = ch

	expandPricingToCache(cache, ch, 1, "openai")

	// 停用定价不应进入缓存
	_, exists := cache.pricingByGroupModel[channelModelKey{groupID: 1, platform: "openai", model: "gpt-4-deprecated"}]
	require.False(t, exists, "disabled pricing should not be in cache")

	// 启用定价应进入缓存，且价格正确
	pricing, existsEnabled := cache.pricingByGroupModel[channelModelKey{groupID: 1, platform: "openai", model: "gpt-4o"}]
	require.True(t, existsEnabled, "enabled pricing should be in cache")
	require.NotNil(t, pricing.InputPrice)
	require.InDelta(t, 0.005, *pricing.InputPrice, 1e-9)
}

// TestHiddenPricingNotInModelList 验证隐藏定价进入计费缓存但不出现在模型列表
func TestHiddenPricingNotInModelList(t *testing.T) {
	ch := &Channel{
		ID:     1,
		Status: StatusActive,
		ModelPricing: []ChannelModelPricing{
			{
				Models:      []string{"gpt-4-internal"},
				Platform:    "openai",
				BillingMode: BillingModeToken,
				InputPrice:  float64Ptr(0.01),
				OutputPrice: float64Ptr(0.03),
				Enabled:     boolPtr(true), // 启用
				Hidden:      true,          // 隐藏
			},
			{
				Models:      []string{"gpt-4o"},
				Platform:    "openai",
				BillingMode: BillingModeToken,
				InputPrice:  float64Ptr(0.005),
				OutputPrice: float64Ptr(0.015),
				Enabled:     boolPtr(true),
				Hidden:      false,
			},
		},
	}

	cache := newEmptyChannelCache()
	cache.groupPlatform[1] = "openai"
	cache.channelByGroupID[1] = ch
	cache.byID[1] = ch

	expandPricingToCache(cache, ch, 1, "openai")

	// 隐藏定价应进入缓存（用于计费）
	pricing, exists := cache.pricingByGroupModel[channelModelKey{groupID: 1, platform: "openai", model: "gpt-4-internal"}]
	require.True(t, exists, "hidden pricing should be in billing cache")
	require.NotNil(t, pricing.InputPrice)
	require.InDelta(t, 0.01, *pricing.InputPrice, 1e-9)

	// 隐藏定价不应出现在模型列表
	names := supportedModelNames(ch.SupportedModels(), "openai")
	require.NotContains(t, names, "gpt-4-internal", "hidden pricing should not appear in model list")
	require.Contains(t, names, "gpt-4o", "visible pricing should appear in model list")
}

// TestWildcardMappingMultiSource 验证通配符映射的前缀匹配逻辑
func TestWildcardMappingMultiSource(t *testing.T) {
	cache := newEmptyChannelCache()
	gpKey := channelGroupPlatformKey{groupID: 1, platform: "openai"}
	cache.wildcardMappingByGP[gpKey] = []*wildcardMappingEntry{
		{prefix: "gpt-4-", target: "gpt-4o"},
		{prefix: "gpt-3.5-", target: "gpt-3.5-turbo"},
	}
	cache.groupPlatform[1] = "openai"

	tests := []struct {
		name     string
		model    string
		expected string
	}{
		{"gpt-4-turbo matches first wildcard", "gpt-4-turbo", "gpt-4o"},
		{"gpt-4-vision matches first wildcard", "gpt-4-vision", "gpt-4o"},
		{"gpt-3.5-turbo-16k matches second wildcard", "gpt-3.5-turbo-16k", "gpt-3.5-turbo"},
		{"gpt-5 does not match any wildcard", "gpt-5", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := lookupMappingAcrossPlatforms(cache, 1, "openai", normalizeChannelPricingModelName(tt.model))
			require.Equal(t, tt.expected, result)
		})
	}
}
