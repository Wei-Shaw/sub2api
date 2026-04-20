//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetModelPricing(t *testing.T) {
	ch := &Channel{
		ModelPricing: []ChannelModelPricing{
			{ID: 1, Models: []string{"claude-sonnet-4"}, BillingMode: BillingModeToken, InputPrice: testPtrFloat64(3e-6)},
			{ID: 3, Models: []string{"gpt-5.1"}, BillingMode: BillingModePerRequest},
		},
	}

	tests := []struct {
		name    string
		model   string
		wantID  int64
		wantNil bool
	}{
		{"exact match", "claude-sonnet-4", 1, false},
		{"case insensitive", "Claude-Sonnet-4", 1, false},
		{"not found", "gemini-3.1-pro", 0, true},
		{"wildcard pattern not matched", "claude-opus-4-20250514", 0, true},
		{"per_request model", "gpt-5.1", 3, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ch.GetModelPricing(tt.model)
			if tt.wantNil {
				require.Nil(t, result)
				return
			}
			require.NotNil(t, result)
			require.Equal(t, tt.wantID, result.ID)
		})
	}
}

func TestGetModelPricing_ReturnsCopy(t *testing.T) {
	ch := &Channel{
		ModelPricing: []ChannelModelPricing{
			{ID: 1, Models: []string{"claude-sonnet-4"}, InputPrice: testPtrFloat64(3e-6)},
		},
	}

	result := ch.GetModelPricing("claude-sonnet-4")
	require.NotNil(t, result)

	// Modify the returned copy's slice — original should be unchanged
	result.Models = append(result.Models, "hacked")

	// Original should be unchanged
	require.Equal(t, 1, len(ch.ModelPricing[0].Models))
}

func TestGetModelPricing_EmptyPricing(t *testing.T) {
	ch := &Channel{ModelPricing: nil}
	require.Nil(t, ch.GetModelPricing("any-model"))

	ch2 := &Channel{ModelPricing: []ChannelModelPricing{}}
	require.Nil(t, ch2.GetModelPricing("any-model"))
}

func TestGetIntervalForContext(t *testing.T) {
	p := &ChannelModelPricing{
		Intervals: []PricingInterval{
			{MinTokens: 0, MaxTokens: testPtrInt(128000), InputPrice: testPtrFloat64(1e-6)},
			{MinTokens: 128000, MaxTokens: nil, InputPrice: testPtrFloat64(2e-6)},
		},
	}

	tests := []struct {
		name      string
		tokens    int
		wantPrice *float64
		wantNil   bool
	}{
		{"first interval", 50000, testPtrFloat64(1e-6), false},
		// (min, max] — 128000 在第一个区间的 max，包含，所以匹配第一个
		{"boundary: max of first (inclusive)", 128000, testPtrFloat64(1e-6), false},
		// 128001 > 128000，匹配第二个区间
		{"boundary: just above first max", 128001, testPtrFloat64(2e-6), false},
		{"unbounded interval", 500000, testPtrFloat64(2e-6), false},
		// (0, max] — 0 不匹配任何区间（左开）
		{"zero tokens: no match", 0, nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := p.GetIntervalForContext(tt.tokens)
			if tt.wantNil {
				require.Nil(t, result)
				return
			}
			require.NotNil(t, result)
			require.InDelta(t, *tt.wantPrice, *result.InputPrice, 1e-12)
		})
	}
}

func TestGetIntervalForContext_NoMatch(t *testing.T) {
	p := &ChannelModelPricing{
		Intervals: []PricingInterval{
			{MinTokens: 10000, MaxTokens: testPtrInt(50000)},
		},
	}
	require.Nil(t, p.GetIntervalForContext(5000))     // 5000 <= 10000, not > min
	require.Nil(t, p.GetIntervalForContext(10000))    // 10000 not > 10000 (left-open)
	require.NotNil(t, p.GetIntervalForContext(50000)) // 50000 <= 50000 (right-closed)
	require.Nil(t, p.GetIntervalForContext(50001))    // 50001 > 50000
}

func TestGetIntervalForContext_Empty(t *testing.T) {
	p := &ChannelModelPricing{Intervals: nil}
	require.Nil(t, p.GetIntervalForContext(1000))
}

func TestGetTierByLabel(t *testing.T) {
	p := &ChannelModelPricing{
		Intervals: []PricingInterval{
			{TierLabel: "1K", PerRequestPrice: testPtrFloat64(0.04)},
			{TierLabel: "2K", PerRequestPrice: testPtrFloat64(0.08)},
			{TierLabel: "HD", PerRequestPrice: testPtrFloat64(0.12)},
		},
	}

	tests := []struct {
		name    string
		label   string
		wantNil bool
		want    float64
	}{
		{"exact match", "1K", false, 0.04},
		{"case insensitive", "hd", false, 0.12},
		{"not found", "4K", true, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := p.GetTierByLabel(tt.label)
			if tt.wantNil {
				require.Nil(t, result)
				return
			}
			require.NotNil(t, result)
			require.InDelta(t, tt.want, *result.PerRequestPrice, 1e-12)
		})
	}
}

func TestGetTierByLabel_Empty(t *testing.T) {
	p := &ChannelModelPricing{Intervals: nil}
	require.Nil(t, p.GetTierByLabel("1K"))
}

func TestChannelClone(t *testing.T) {
	original := &Channel{
		ID:       1,
		Name:     "test",
		GroupIDs: []int64{10, 20},
		ModelPricing: []ChannelModelPricing{
			{
				ID:         100,
				Models:     []string{"model-a"},
				InputPrice: testPtrFloat64(5e-6),
			},
		},
	}

	cloned := original.Clone()
	require.NotNil(t, cloned)
	require.Equal(t, original.ID, cloned.ID)
	require.Equal(t, original.Name, cloned.Name)

	// Modify clone slices — original should not change
	cloned.GroupIDs[0] = 999
	require.Equal(t, int64(10), original.GroupIDs[0])

	cloned.ModelPricing[0].Models[0] = "hacked"
	require.Equal(t, "model-a", original.ModelPricing[0].Models[0])
}

func TestChannelClone_Nil(t *testing.T) {
	var ch *Channel
	require.Nil(t, ch.Clone())
}

func TestChannelModelPricingClone(t *testing.T) {
	original := ChannelModelPricing{
		Models: []string{"a", "b"},
		Intervals: []PricingInterval{
			{MinTokens: 0, TierLabel: "tier1"},
		},
	}

	cloned := original.Clone()

	// Modify clone slices — original unchanged
	cloned.Models[0] = "hacked"
	require.Equal(t, "a", original.Models[0])

	cloned.Intervals[0].TierLabel = "hacked"
	require.Equal(t, "tier1", original.Intervals[0].TierLabel)
}

// --- BillingMode.IsValid ---

func TestBillingModeIsValid(t *testing.T) {
	tests := []struct {
		name string
		mode BillingMode
		want bool
	}{
		{"token", BillingModeToken, true},
		{"per_request", BillingModePerRequest, true},
		{"image", BillingModeImage, true},
		{"empty", BillingMode(""), true},
		{"unknown", BillingMode("unknown"), false},
		{"random", BillingMode("xyz"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.mode.IsValid())
		})
	}
}

// --- Channel.IsActive ---

func TestChannelIsActive(t *testing.T) {
	tests := []struct {
		name   string
		status string
		want   bool
	}{
		{"active", StatusActive, true},
		{"disabled", "disabled", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ch := &Channel{Status: tt.status}
			require.Equal(t, tt.want, ch.IsActive())
		})
	}
}

// --- ChannelModelPricing.Clone edge cases ---

func TestChannelModelPricingClone_EdgeCases(t *testing.T) {
	t.Run("nil models", func(t *testing.T) {
		original := ChannelModelPricing{Models: nil}
		cloned := original.Clone()
		require.Nil(t, cloned.Models)
	})

	t.Run("nil intervals", func(t *testing.T) {
		original := ChannelModelPricing{Intervals: nil}
		cloned := original.Clone()
		require.Nil(t, cloned.Intervals)
	})

	t.Run("empty models", func(t *testing.T) {
		original := ChannelModelPricing{Models: []string{}}
		cloned := original.Clone()
		require.NotNil(t, cloned.Models)
		require.Empty(t, cloned.Models)
	})
}

// --- Channel.Clone edge cases ---

func TestChannelClone_EdgeCases(t *testing.T) {
	t.Run("nil model mapping", func(t *testing.T) {
		original := &Channel{ID: 1, ModelMapping: nil}
		cloned := original.Clone()
		require.Nil(t, cloned.ModelMapping)
	})

	t.Run("nil model pricing", func(t *testing.T) {
		original := &Channel{ID: 1, ModelPricing: nil}
		cloned := original.Clone()
		require.Nil(t, cloned.ModelPricing)
	})

	t.Run("deep copy model mapping", func(t *testing.T) {
		original := &Channel{
			ID: 1,
			ModelMapping: map[string]map[string]string{
				"openai": {"gpt-4": "gpt-4-turbo"},
			},
		}
		cloned := original.Clone()

		// Modify the cloned nested map
		cloned.ModelMapping["openai"]["gpt-4"] = "hacked"

		// Original must remain unchanged
		require.Equal(t, "gpt-4-turbo", original.ModelMapping["openai"]["gpt-4"])
	})
}

// --- ValidateIntervals ---

func TestValidateIntervals_Empty(t *testing.T) {
	require.NoError(t, ValidateIntervals(nil))
	require.NoError(t, ValidateIntervals([]PricingInterval{}))
}

func TestValidateIntervals_ValidIntervals(t *testing.T) {
	tests := []struct {
		name      string
		intervals []PricingInterval
	}{
		{
			name: "single bounded interval",
			intervals: []PricingInterval{
				{MinTokens: 0, MaxTokens: testPtrInt(128000), InputPrice: testPtrFloat64(1e-6)},
			},
		},
		{
			name: "two intervals with gap",
			intervals: []PricingInterval{
				{MinTokens: 0, MaxTokens: testPtrInt(100000), InputPrice: testPtrFloat64(1e-6)},
				{MinTokens: 128000, MaxTokens: nil, InputPrice: testPtrFloat64(2e-6)},
			},
		},
		{
			name: "two contiguous intervals",
			intervals: []PricingInterval{
				{MinTokens: 0, MaxTokens: testPtrInt(128000), InputPrice: testPtrFloat64(1e-6)},
				{MinTokens: 128000, MaxTokens: nil, InputPrice: testPtrFloat64(2e-6)},
			},
		},
		{
			name: "unsorted input (auto-sorted by validator)",
			intervals: []PricingInterval{
				{MinTokens: 128000, MaxTokens: nil, InputPrice: testPtrFloat64(2e-6)},
				{MinTokens: 0, MaxTokens: testPtrInt(128000), InputPrice: testPtrFloat64(1e-6)},
			},
		},
		{
			name: "single unbounded interval",
			intervals: []PricingInterval{
				{MinTokens: 0, MaxTokens: nil, InputPrice: testPtrFloat64(1e-6)},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NoError(t, ValidateIntervals(tt.intervals))
		})
	}
}

func TestValidateIntervals_NegativeMinTokens(t *testing.T) {
	intervals := []PricingInterval{
		{MinTokens: -1, MaxTokens: testPtrInt(100), InputPrice: testPtrFloat64(1e-6)},
	}
	err := ValidateIntervals(intervals)
	require.Error(t, err)
	require.Contains(t, err.Error(), "min_tokens")
	require.Contains(t, err.Error(), ">= 0")
}

func TestValidateIntervals_MaxTokensZero(t *testing.T) {
	intervals := []PricingInterval{
		{MinTokens: 0, MaxTokens: testPtrInt(0), InputPrice: testPtrFloat64(1e-6)},
	}
	err := ValidateIntervals(intervals)
	require.Error(t, err)
	require.Contains(t, err.Error(), "max_tokens")
	require.Contains(t, err.Error(), "> 0")
}

func TestValidateIntervals_MaxLessThanMin(t *testing.T) {
	intervals := []PricingInterval{
		{MinTokens: 100, MaxTokens: testPtrInt(50), InputPrice: testPtrFloat64(1e-6)},
	}
	err := ValidateIntervals(intervals)
	require.Error(t, err)
	require.Contains(t, err.Error(), "max_tokens")
	require.Contains(t, err.Error(), "> min_tokens")
}

func TestValidateIntervals_MaxEqualsMin(t *testing.T) {
	intervals := []PricingInterval{
		{MinTokens: 100, MaxTokens: testPtrInt(100), InputPrice: testPtrFloat64(1e-6)},
	}
	err := ValidateIntervals(intervals)
	require.Error(t, err)
	require.Contains(t, err.Error(), "max_tokens")
	require.Contains(t, err.Error(), "> min_tokens")
}

func TestValidateIntervals_NegativePrice(t *testing.T) {
	negPrice := -0.01
	intervals := []PricingInterval{
		{MinTokens: 0, MaxTokens: testPtrInt(100), InputPrice: &negPrice},
	}
	err := ValidateIntervals(intervals)
	require.Error(t, err)
	require.Contains(t, err.Error(), "input_price")
	require.Contains(t, err.Error(), ">= 0")
}

func TestValidateIntervals_OverlappingIntervals(t *testing.T) {
	intervals := []PricingInterval{
		{MinTokens: 0, MaxTokens: testPtrInt(200), InputPrice: testPtrFloat64(1e-6)},
		{MinTokens: 100, MaxTokens: testPtrInt(300), InputPrice: testPtrFloat64(2e-6)},
	}
	err := ValidateIntervals(intervals)
	require.Error(t, err)
	require.Contains(t, err.Error(), "overlap")
}

func TestValidateIntervals_UnboundedNotLast(t *testing.T) {
	intervals := []PricingInterval{
		{MinTokens: 0, MaxTokens: nil, InputPrice: testPtrFloat64(1e-6)},
		{MinTokens: 128000, MaxTokens: testPtrInt(256000), InputPrice: testPtrFloat64(2e-6)},
	}
	err := ValidateIntervals(intervals)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unbounded")
	require.Contains(t, err.Error(), "last")
}

func TestSupportedModels_ExactKeysAndPricing(t *testing.T) {
	ch := &Channel{
		ModelPricing: []ChannelModelPricing{
			{ID: 10, Platform: "anthropic", Models: []string{"claude-sonnet-4-6"}, InputPrice: testPtrFloat64(3e-6)},
			{ID: 11, Platform: "anthropic", Models: []string{"claude-opus-4-6"}, InputPrice: testPtrFloat64(1.5e-5)},
		},
		ModelMapping: map[string]map[string]string{
			"anthropic": {
				"claude-sonnet-4-6": "claude-sonnet-4-6",
				"claude-opus-4-6":   "claude-opus-4-6",
			},
		},
	}

	got := ch.SupportedModels()
	require.Len(t, got, 2)
	require.Equal(t, "anthropic", got[0].Platform)
	require.Equal(t, "claude-opus-4-6", got[0].Name)
	require.NotNil(t, got[0].Pricing)
	require.Equal(t, int64(11), got[0].Pricing.ID)
	require.Equal(t, "claude-sonnet-4-6", got[1].Name)
	require.Equal(t, int64(10), got[1].Pricing.ID)
}

func TestSupportedModels_WildcardExpandedFromPricing(t *testing.T) {
	ch := &Channel{
		ModelPricing: []ChannelModelPricing{
			{ID: 1, Platform: "anthropic", Models: []string{"claude-sonnet-4-6", "claude-sonnet-4-5"}},
			{ID: 2, Platform: "anthropic", Models: []string{"claude-opus-4-6"}},
		},
		ModelMapping: map[string]map[string]string{
			"anthropic": {
				"claude-sonnet-*": "claude-sonnet-4-6",
			},
		},
	}

	got := ch.SupportedModels()
	names := make([]string, 0, len(got))
	for _, m := range got {
		names = append(names, m.Name)
	}
	require.ElementsMatch(t, []string{"claude-sonnet-4-5", "claude-sonnet-4-6"}, names)
	for _, m := range got {
		require.NotContains(t, m.Name, "*")
	}
}

func TestSupportedModels_PlatformWithoutMappingSkipped(t *testing.T) {
	ch := &Channel{
		ModelPricing: []ChannelModelPricing{
			{ID: 1, Platform: "anthropic", Models: []string{"claude-sonnet-4-6"}},
			{ID: 2, Platform: "openai", Models: []string{"gpt-4o"}},
		},
		ModelMapping: map[string]map[string]string{
			"anthropic": {"claude-sonnet-4-6": "claude-sonnet-4-6"},
			// openai 没有 mapping 条目
		},
	}

	got := ch.SupportedModels()
	require.Len(t, got, 1)
	require.Equal(t, "anthropic", got[0].Platform)
	require.Equal(t, "claude-sonnet-4-6", got[0].Name)
}

func TestSupportedModels_MissingPricingKeepsNilPricing(t *testing.T) {
	ch := &Channel{
		ModelMapping: map[string]map[string]string{
			"anthropic": {"claude-sonnet-4-6": "claude-sonnet-4-6"},
		},
	}

	got := ch.SupportedModels()
	require.Len(t, got, 1)
	require.Equal(t, "claude-sonnet-4-6", got[0].Name)
	require.Nil(t, got[0].Pricing)
}

func TestSupportedModels_DedupAndSort(t *testing.T) {
	ch := &Channel{
		ModelPricing: []ChannelModelPricing{
			{ID: 1, Platform: "anthropic", Models: []string{"claude-sonnet-4-6", "claude-sonnet-4-5"}},
			{ID: 2, Platform: "openai", Models: []string{"gpt-4o"}},
		},
		ModelMapping: map[string]map[string]string{
			"anthropic": {
				"claude-sonnet-4-6": "upstream-a",
				"claude-sonnet-*":   "upstream-a",
			},
			"openai": {"gpt-4o": "gpt-4o"},
		},
	}

	got := ch.SupportedModels()
	require.Len(t, got, 3)
	require.Equal(t, "anthropic", got[0].Platform)
	require.Equal(t, "claude-sonnet-4-5", got[0].Name)
	require.Equal(t, "anthropic", got[1].Platform)
	require.Equal(t, "claude-sonnet-4-6", got[1].Name)
	require.Equal(t, "openai", got[2].Platform)
	require.Equal(t, "gpt-4o", got[2].Name)
}

func TestSupportedModels_NilChannelAndEmpty(t *testing.T) {
	var nilCh *Channel
	require.Nil(t, nilCh.SupportedModels())

	empty := &Channel{}
	require.Nil(t, empty.SupportedModels())
}

func TestGetModelPricingByPlatform(t *testing.T) {
	ch := &Channel{
		ModelPricing: []ChannelModelPricing{
			{ID: 1, Platform: "anthropic", Models: []string{"claude-sonnet-4-6"}, InputPrice: testPtrFloat64(3e-6)},
			{ID: 2, Platform: "openai", Models: []string{"claude-sonnet-4-6"}, InputPrice: testPtrFloat64(1e-6)},
		},
	}

	ant := ch.GetModelPricingByPlatform("anthropic", "claude-sonnet-4-6")
	require.NotNil(t, ant)
	require.Equal(t, int64(1), ant.ID)

	oa := ch.GetModelPricingByPlatform("openai", "claude-sonnet-4-6")
	require.NotNil(t, oa)
	require.Equal(t, int64(2), oa.ID)

	require.Nil(t, ch.GetModelPricingByPlatform("gemini", "claude-sonnet-4-6"))
}

func TestSupportedModels_WildcardOnlyPricingRowsSkipped(t *testing.T) {
	// 定价中含通配符条目（pattern），不应被当作具体模型名展开。
	ch := &Channel{
		ModelPricing: []ChannelModelPricing{
			{ID: 1, Platform: "anthropic", Models: []string{"claude-sonnet-*", "claude-sonnet-4-6"}},
		},
		ModelMapping: map[string]map[string]string{
			"anthropic": {"claude-sonnet-*": "claude-sonnet-4-6"},
		},
	}
	got := ch.SupportedModels()
	require.Len(t, got, 1)
	require.Equal(t, "claude-sonnet-4-6", got[0].Name)
	for _, m := range got {
		require.NotContains(t, m.Name, "*")
	}
}

func TestSupportedModels_WildcardPrefixMatchesNothing(t *testing.T) {
	// 通配符模式无任何对应定价模型时，该平台应产出 0 个模型。
	ch := &Channel{
		ModelPricing: []ChannelModelPricing{
			{ID: 1, Platform: "openai", Models: []string{"gpt-4o"}},
		},
		ModelMapping: map[string]map[string]string{
			"anthropic": {"gpt-foo-*": "gpt-foo-1"},
		},
	}
	require.Empty(t, ch.SupportedModels())
}

func TestSupportedModels_CrossPlatformPricingDoesNotBleed(t *testing.T) {
	// anthropic 的通配符不应拉入 openai 定价行，哪怕名字恰好前缀匹配。
	ch := &Channel{
		ModelPricing: []ChannelModelPricing{
			{ID: 1, Platform: "openai", Models: []string{"claude-sonnet-4-6"}},
		},
		ModelMapping: map[string]map[string]string{
			"anthropic": {"claude-sonnet-*": "x"},
		},
	}
	require.Empty(t, ch.SupportedModels())
}

func TestSupportedModels_CaseInsensitiveDedup(t *testing.T) {
	// 两行定价用不同大小写定义了同一模型，结果应去重为 1 条；首次出现的原始大小写保留。
	ch := &Channel{
		ModelPricing: []ChannelModelPricing{
			{ID: 1, Platform: "openai", Models: []string{"GPT-4o"}},
			{ID: 2, Platform: "openai", Models: []string{"gpt-4o"}},
		},
		ModelMapping: map[string]map[string]string{
			"openai": {"gpt-*": "x"},
		},
	}
	got := ch.SupportedModels()
	require.Len(t, got, 1)
	require.Equal(t, "GPT-4o", got[0].Name)
}

func TestSupportedModels_EmptyPlatformMapping(t *testing.T) {
	// ModelMapping 有一个 platform key 但 value 是空 map —— 该 platform 应被跳过。
	ch := &Channel{
		ModelPricing: []ChannelModelPricing{
			{ID: 1, Platform: "anthropic", Models: []string{"claude-sonnet-4-6"}},
		},
		ModelMapping: map[string]map[string]string{
			"anthropic": {},
		},
	}
	require.Empty(t, ch.SupportedModels())
}

func TestSupportedModels_ExactKeyUsesPricedCaseWhenAvailable(t *testing.T) {
	// mapping key uses uppercase, pricing uses lowercase — pricing's case should win.
	ch := &Channel{
		ModelPricing: []ChannelModelPricing{
			{ID: 1, Platform: "openai", Models: []string{"gpt-4o"}},
		},
		ModelMapping: map[string]map[string]string{
			"openai": {"GPT-4o": "gpt-4o"},
		},
	}
	got := ch.SupportedModels()
	require.Len(t, got, 1)
	require.Equal(t, "gpt-4o", got[0].Name) // pricing's case wins
}

func TestSupportedModels_AsteriskOnlyMappingExpandsAllPriced(t *testing.T) {
	// 映射 key 为单独的 "*"：前缀为空 → 命中该平台所有定价模型（透传场景）。
	ch := &Channel{
		ModelPricing: []ChannelModelPricing{
			{ID: 1, Platform: "openai", Models: []string{"gpt-4o", "gpt-4o-mini"}},
		},
		ModelMapping: map[string]map[string]string{
			"openai": {"*": "gpt-4o"},
		},
	}
	got := ch.SupportedModels()
	require.Len(t, got, 2)
	names := []string{got[0].Name, got[1].Name}
	require.ElementsMatch(t, []string{"gpt-4o", "gpt-4o-mini"}, names)
}
