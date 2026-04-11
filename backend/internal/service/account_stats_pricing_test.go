//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// matchAccountStatsRule
// ---------------------------------------------------------------------------

func TestMatchAccountStatsRule_BothEmpty_NoMatch(t *testing.T) {
	rule := &AccountStatsPricingRule{}
	require.False(t, matchAccountStatsRule(rule, 1, 10))
}

func TestMatchAccountStatsRule_AccountIDMatch(t *testing.T) {
	rule := &AccountStatsPricingRule{AccountIDs: []int64{1, 2, 3}}
	require.True(t, matchAccountStatsRule(rule, 2, 999))
}

func TestMatchAccountStatsRule_GroupIDMatch(t *testing.T) {
	rule := &AccountStatsPricingRule{GroupIDs: []int64{10, 20}}
	require.True(t, matchAccountStatsRule(rule, 999, 20))
}

func TestMatchAccountStatsRule_BothConfigured_AccountMatch(t *testing.T) {
	rule := &AccountStatsPricingRule{
		AccountIDs: []int64{1, 2},
		GroupIDs:   []int64{10, 20},
	}
	require.True(t, matchAccountStatsRule(rule, 2, 999))
}

func TestMatchAccountStatsRule_BothConfigured_GroupMatch(t *testing.T) {
	rule := &AccountStatsPricingRule{
		AccountIDs: []int64{1, 2},
		GroupIDs:   []int64{10, 20},
	}
	require.True(t, matchAccountStatsRule(rule, 999, 10))
}

func TestMatchAccountStatsRule_BothConfigured_NeitherMatch(t *testing.T) {
	rule := &AccountStatsPricingRule{
		AccountIDs: []int64{1, 2},
		GroupIDs:   []int64{10, 20},
	}
	require.False(t, matchAccountStatsRule(rule, 999, 999))
}

// ---------------------------------------------------------------------------
// findPricingForModel
// ---------------------------------------------------------------------------

func TestFindPricingForModel(t *testing.T) {
	exactPricing := ChannelModelPricing{
		ID:     1,
		Models: []string{"claude-opus-4"},
	}
	wildcardPricing := ChannelModelPricing{
		ID:     2,
		Models: []string{"claude-*"},
	}
	platformPricing := ChannelModelPricing{
		ID:       3,
		Platform: "openai",
		Models:   []string{"gpt-4o"},
	}
	emptyPlatformPricing := ChannelModelPricing{
		ID:     4,
		Models: []string{"gemini-2.5-pro"},
	}

	tests := []struct {
		name     string
		list     []ChannelModelPricing
		platform string
		model    string
		wantID   int64
		wantNil  bool
	}{
		{
			name:     "exact match",
			list:     []ChannelModelPricing{exactPricing},
			platform: "anthropic",
			model:    "claude-opus-4",
			wantID:   1,
		},
		{
			name:     "exact match case insensitive",
			list:     []ChannelModelPricing{{ID: 5, Models: []string{"Claude-Opus-4"}}},
			platform: "",
			model:    "claude-opus-4",
			wantID:   5,
		},
		{
			name:     "wildcard match",
			list:     []ChannelModelPricing{wildcardPricing},
			platform: "anthropic",
			model:    "claude-opus-4",
			wantID:   2,
		},
		{
			name:     "exact match takes priority over wildcard",
			list:     []ChannelModelPricing{wildcardPricing, exactPricing},
			platform: "anthropic",
			model:    "claude-opus-4",
			wantID:   1,
		},
		{
			name:     "platform mismatch skipped",
			list:     []ChannelModelPricing{platformPricing},
			platform: "anthropic",
			model:    "gpt-4o",
			wantNil:  true,
		},
		{
			name:     "empty platform in pricing matches any",
			list:     []ChannelModelPricing{emptyPlatformPricing},
			platform: "gemini",
			model:    "gemini-2.5-pro",
			wantID:   4,
		},
		{
			name:     "empty platform in query matches any pricing platform",
			list:     []ChannelModelPricing{platformPricing},
			platform: "",
			model:    "gpt-4o",
			wantID:   3,
		},
		{
			name:     "no match at all",
			list:     []ChannelModelPricing{exactPricing, wildcardPricing},
			platform: "anthropic",
			model:    "gpt-4o",
			wantNil:  true,
		},
		{
			name:    "empty list returns nil",
			list:    nil,
			model:   "claude-opus-4",
			wantNil: true,
		},
		{
			name: "longer wildcard prefix wins over shorter",
			list: []ChannelModelPricing{
				{ID: 10, Models: []string{"claude-*"}},
				{ID: 11, Models: []string{"claude-opus-*"}},
			},
			platform: "",
			model:    "claude-opus-4",
			wantID:   11, // "claude-opus-" (12 chars) > "claude-" (7 chars)
		},
		{
			name: "shorter wildcard used when longer does not match",
			list: []ChannelModelPricing{
				{ID: 10, Models: []string{"claude-*"}},
				{ID: 11, Models: []string{"claude-opus-*"}},
			},
			platform: "",
			model:    "claude-sonnet-4",
			wantID:   10, // only "claude-*" matches
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := findPricingForModel(tt.list, tt.platform, tt.model)
			if tt.wantNil {
				require.Nil(t, result)
				return
			}
			require.NotNil(t, result)
			require.Equal(t, tt.wantID, result.ID)
		})
	}
}

// ---------------------------------------------------------------------------
// calculateStatsCost
// ---------------------------------------------------------------------------

func TestCalculateStatsCost_NilPricing(t *testing.T) {
	result := calculateStatsCost(nil, UsageTokens{}, 1)
	require.Nil(t, result)
}

func TestCalculateStatsCost_TokenBilling(t *testing.T) {
	pricing := &ChannelModelPricing{
		BillingMode: BillingModeToken,
		InputPrice:  testPtrFloat64(0.001),
		OutputPrice: testPtrFloat64(0.002),
	}
	tokens := UsageTokens{
		InputTokens:  100,
		OutputTokens: 50,
	}
	result := calculateStatsCost(pricing, tokens, 1)
	require.NotNil(t, result)
	// 100*0.001 + 50*0.002 = 0.1 + 0.1 = 0.2
	require.InDelta(t, 0.2, *result, 1e-12)
}

func TestCalculateStatsCost_TokenBilling_WithCache(t *testing.T) {
	pricing := &ChannelModelPricing{
		BillingMode:     BillingModeToken,
		InputPrice:      testPtrFloat64(0.001),
		OutputPrice:     testPtrFloat64(0.002),
		CacheWritePrice: testPtrFloat64(0.003),
		CacheReadPrice:  testPtrFloat64(0.0005),
	}
	tokens := UsageTokens{
		InputTokens:         100,
		OutputTokens:        50,
		CacheCreationTokens: 200,
		CacheReadTokens:     300,
	}
	result := calculateStatsCost(pricing, tokens, 1)
	require.NotNil(t, result)
	// 100*0.001 + 50*0.002 + 200*0.003 + 300*0.0005
	// = 0.1 + 0.1 + 0.6 + 0.15 = 0.95
	require.InDelta(t, 0.95, *result, 1e-12)
}

func TestCalculateStatsCost_TokenBilling_WithImageOutput(t *testing.T) {
	pricing := &ChannelModelPricing{
		BillingMode:      BillingModeToken,
		InputPrice:       testPtrFloat64(0.001),
		OutputPrice:      testPtrFloat64(0.002),
		ImageOutputPrice: testPtrFloat64(0.01),
	}
	tokens := UsageTokens{
		InputTokens:       100,
		OutputTokens:      50,
		ImageOutputTokens: 10,
	}
	result := calculateStatsCost(pricing, tokens, 1)
	require.NotNil(t, result)
	// 100*0.001 + 50*0.002 + 10*0.01 = 0.1 + 0.1 + 0.1 = 0.3
	require.InDelta(t, 0.3, *result, 1e-12)
}

func TestCalculateStatsCost_TokenBilling_PartialPricesNil(t *testing.T) {
	pricing := &ChannelModelPricing{
		BillingMode: BillingModeToken,
		InputPrice:  testPtrFloat64(0.001),
		// OutputPrice, CacheWritePrice, etc. are all nil → treated as 0
	}
	tokens := UsageTokens{
		InputTokens:         100,
		OutputTokens:        50,
		CacheCreationTokens: 200,
	}
	result := calculateStatsCost(pricing, tokens, 1)
	require.NotNil(t, result)
	// Only input contributes: 100*0.001 = 0.1
	require.InDelta(t, 0.1, *result, 1e-12)
}

func TestCalculateStatsCost_TokenBilling_AllTokensZero(t *testing.T) {
	pricing := &ChannelModelPricing{
		BillingMode: BillingModeToken,
		InputPrice:  testPtrFloat64(0.001),
		OutputPrice: testPtrFloat64(0.002),
	}
	tokens := UsageTokens{} // all zeros
	result := calculateStatsCost(pricing, tokens, 1)
	// totalCost == 0 → returns nil (does not override, falls back to default formula)
	require.Nil(t, result)
}

func TestCalculateStatsCost_PerRequestBilling(t *testing.T) {
	pricing := &ChannelModelPricing{
		BillingMode:     BillingModePerRequest,
		PerRequestPrice: testPtrFloat64(0.05),
	}
	tokens := UsageTokens{InputTokens: 999, OutputTokens: 999}
	result := calculateStatsCost(pricing, tokens, 3)
	require.NotNil(t, result)
	// 0.05 * 3 = 0.15
	require.InDelta(t, 0.15, *result, 1e-12)
}

func TestCalculateStatsCost_PerRequestBilling_PriceNil(t *testing.T) {
	pricing := &ChannelModelPricing{
		BillingMode: BillingModePerRequest,
		// PerRequestPrice is nil
	}
	result := calculateStatsCost(pricing, UsageTokens{}, 1)
	require.Nil(t, result)
}

func TestCalculateStatsCost_PerRequestBilling_PriceZero(t *testing.T) {
	pricing := &ChannelModelPricing{
		BillingMode:     BillingModePerRequest,
		PerRequestPrice: testPtrFloat64(0),
	}
	result := calculateStatsCost(pricing, UsageTokens{}, 1)
	// price == 0 → condition *pricing.PerRequestPrice > 0 is false → returns nil
	require.Nil(t, result)
}

func TestCalculateStatsCost_ImageBilling(t *testing.T) {
	pricing := &ChannelModelPricing{
		BillingMode:     BillingModeImage,
		PerRequestPrice: testPtrFloat64(0.10),
	}
	result := calculateStatsCost(pricing, UsageTokens{}, 2)
	require.NotNil(t, result)
	// 0.10 * 2 = 0.20
	require.InDelta(t, 0.20, *result, 1e-12)
}

func TestCalculateStatsCost_ImageBilling_PriceNil(t *testing.T) {
	pricing := &ChannelModelPricing{
		BillingMode: BillingModeImage,
		// PerRequestPrice is nil
	}
	result := calculateStatsCost(pricing, UsageTokens{}, 1)
	require.Nil(t, result)
}

func TestCalculateStatsCost_DefaultBillingMode_FallsToToken(t *testing.T) {
	// BillingMode is empty string (default) → falls into token billing
	pricing := &ChannelModelPricing{
		InputPrice:  testPtrFloat64(0.001),
		OutputPrice: testPtrFloat64(0.002),
	}
	tokens := UsageTokens{
		InputTokens:  100,
		OutputTokens: 50,
	}
	result := calculateStatsCost(pricing, tokens, 1)
	require.NotNil(t, result)
	require.InDelta(t, 0.2, *result, 1e-12)
}

// ---------------------------------------------------------------------------
// tryCustomRules — 多规则顺序测试
// ---------------------------------------------------------------------------

func TestTryCustomRules_FirstMatchWins(t *testing.T) {
	channel := &Channel{
		AccountStatsPricingRules: []AccountStatsPricingRule{
			{
				GroupIDs: []int64{1},
				Pricing: []ChannelModelPricing{
					{ID: 100, Models: []string{"claude-opus-4"}, InputPrice: testPtrFloat64(0.01), OutputPrice: testPtrFloat64(0.02)},
				},
			},
			{
				GroupIDs: []int64{1},
				Pricing: []ChannelModelPricing{
					{ID: 200, Models: []string{"claude-opus-4"}, InputPrice: testPtrFloat64(0.99), OutputPrice: testPtrFloat64(0.99)},
				},
			},
		},
	}
	tokens := UsageTokens{InputTokens: 100, OutputTokens: 50}
	result := tryCustomRules(channel, 999, 1, "", "claude-opus-4", tokens, 1)
	require.NotNil(t, result)
	// 应使用第一条规则的价格：100*0.01 + 50*0.02 = 2.0
	require.InDelta(t, 2.0, *result, 1e-12)
}

func TestTryCustomRules_SkipsNonMatchingRules(t *testing.T) {
	channel := &Channel{
		AccountStatsPricingRules: []AccountStatsPricingRule{
			{
				AccountIDs: []int64{888}, // 不匹配
				Pricing: []ChannelModelPricing{
					{ID: 100, Models: []string{"claude-opus-4"}, InputPrice: testPtrFloat64(0.99)},
				},
			},
			{
				GroupIDs: []int64{1}, // 匹配
				Pricing: []ChannelModelPricing{
					{ID: 200, Models: []string{"claude-opus-4"}, InputPrice: testPtrFloat64(0.05)},
				},
			},
		},
	}
	tokens := UsageTokens{InputTokens: 100}
	result := tryCustomRules(channel, 999, 1, "", "claude-opus-4", tokens, 1)
	require.NotNil(t, result)
	// 跳过规则1（账号不匹配），使用规则2：100*0.05 = 5.0
	require.InDelta(t, 5.0, *result, 1e-12)
}

func TestTryCustomRules_NoMatch_ReturnsNil(t *testing.T) {
	channel := &Channel{
		AccountStatsPricingRules: []AccountStatsPricingRule{
			{
				AccountIDs: []int64{888},
				Pricing: []ChannelModelPricing{
					{ID: 100, Models: []string{"claude-opus-4"}, InputPrice: testPtrFloat64(0.01)},
				},
			},
		},
	}
	tokens := UsageTokens{InputTokens: 100}
	result := tryCustomRules(channel, 999, 2, "", "claude-opus-4", tokens, 1)
	require.Nil(t, result) // 账号和分组都不匹配
}

func TestTryCustomRules_RuleMatchesButModelNot_ContinuesToNext(t *testing.T) {
	channel := &Channel{
		AccountStatsPricingRules: []AccountStatsPricingRule{
			{
				GroupIDs: []int64{1},
				Pricing: []ChannelModelPricing{
					{ID: 100, Models: []string{"gpt-4o"}, InputPrice: testPtrFloat64(0.01)}, // 模型不匹配
				},
			},
			{
				GroupIDs: []int64{1},
				Pricing: []ChannelModelPricing{
					{ID: 200, Models: []string{"claude-opus-4"}, InputPrice: testPtrFloat64(0.05)}, // 模型匹配
				},
			},
		},
	}
	tokens := UsageTokens{InputTokens: 100}
	result := tryCustomRules(channel, 999, 1, "", "claude-opus-4", tokens, 1)
	require.NotNil(t, result)
	require.InDelta(t, 5.0, *result, 1e-12) // 使用规则2
}
