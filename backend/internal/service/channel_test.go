//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func channelTestPtrFloat64(v float64) *float64 { return &v }
func channelTestPtrInt(v int) *int             { return &v }

func TestGetModelPricing(t *testing.T) {
	ch := &Channel{
		ModelPricing: []ChannelModelPricing{
			{ID: 1, Models: []string{"claude-sonnet-4"}, BillingMode: BillingModeToken, InputPrice: channelTestPtrFloat64(3e-6)},
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
			{ID: 1, Models: []string{"claude-sonnet-4"}, InputPrice: channelTestPtrFloat64(3e-6)},
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
			{MinTokens: 0, MaxTokens: channelTestPtrInt(128000), InputPrice: channelTestPtrFloat64(1e-6)},
			{MinTokens: 128000, MaxTokens: nil, InputPrice: channelTestPtrFloat64(2e-6)},
		},
	}

	tests := []struct {
		name       string
		tokens     int
		wantPrice  *float64
		wantNil    bool
	}{
		{"first interval", 50000, channelTestPtrFloat64(1e-6), false},
		// (min, max] — 128000 在第一个区间的 max，包含，所以匹配第一个
		{"boundary: max of first (inclusive)", 128000, channelTestPtrFloat64(1e-6), false},
		// 128001 > 128000，匹配第二个区间
		{"boundary: just above first max", 128001, channelTestPtrFloat64(2e-6), false},
		{"unbounded interval", 500000, channelTestPtrFloat64(2e-6), false},
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
			{MinTokens: 10000, MaxTokens: channelTestPtrInt(50000)},
		},
	}
	require.Nil(t, p.GetIntervalForContext(5000))  // 5000 <= 10000, not > min
	require.Nil(t, p.GetIntervalForContext(10000)) // 10000 not > 10000 (left-open)
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
			{TierLabel: "1K", PerRequestPrice: channelTestPtrFloat64(0.04)},
			{TierLabel: "2K", PerRequestPrice: channelTestPtrFloat64(0.08)},
			{TierLabel: "HD", PerRequestPrice: channelTestPtrFloat64(0.12)},
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
				InputPrice: channelTestPtrFloat64(5e-6),
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
