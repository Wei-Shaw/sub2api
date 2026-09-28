//go:build unit

package service

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestReasoningEffortBillingMultiplier(t *testing.T) {
	multipliers := map[string]float64{"none": 0.5, "minimal": 0.75, "low": 0.9, "medium": 1.25, "high": 1.5, "xhigh": 2, "max": 3}
	for _, tc := range []struct {
		effort string
		want   float64
	}{
		{"none", 0.5}, {"minimal", 0.75}, {"low", 0.9}, {"medium", 1.25},
		{"high", 1.5}, {"xhigh", 2}, {"max", 3}, {" Extra-High ", 2},
		{" MAX ", 3}, {"", 1}, {"unknown", 1},
	} {
		t.Run(tc.effort, func(t *testing.T) {
			require.Equal(t, tc.want, reasoningEffortBillingMultiplier(tc.effort, multipliers))
		})
	}
	require.Equal(t, 1.0, reasoningEffortBillingMultiplier("max", nil))
	for _, invalid := range []float64{0, -1, math.NaN(), math.Inf(1)} {
		require.Equal(t, 1.0, reasoningEffortBillingMultiplier("max", map[string]float64{"max": invalid}))
	}
}

func TestReasoningEffortBillingSupportsAllModelsAndLevels(t *testing.T) {
	for _, model := range []string{"claude-fable-5-1", "gpt-5.4", "gemini-2.5-pro", "custom-model"} {
		t.Run(model, func(t *testing.T) {
			bs, resolver := newTokenCostTestEnv(t, PlatformOpenAI, []ChannelModelPricing{{
				Platform: PlatformOpenAI, Models: []string{model}, BillingMode: BillingModeToken,
				InputPrice: testPtrFloat64(1e-6), OutputPrice: testPtrFloat64(2e-6),
				ReasoningEffortMultipliers: map[string]float64{"none": 0.5, "high": 1.5, "max": 3},
			}}, nil)
			group := &Group{ID: 100, Platform: PlatformOpenAI}
			for effort, multiplier := range map[string]float64{"none": 0.5, "high": 1.5, "max": 3, "low": 1, "": 1} {
				cost, err := bs.CalculateTokenCostForRequest(TokenCostRequest{
					Ctx: context.Background(), Model: model, Group: group, Resolver: resolver,
					Tokens: UsageTokens{InputTokens: 1000, OutputTokens: 200}, RateMultiplier: 0.8,
					ReasoningEffort: effort,
				})
				require.NoError(t, err)
				require.InDelta(t, 0.0014*multiplier, cost.TotalCost, 1e-12, effort)
				require.InDelta(t, cost.TotalCost*0.8, cost.ActualCost, 1e-12, effort)
			}
		})
	}
}

func TestReasoningEffortBillingStacksWithContextTierServiceTierAndTimePricing(t *testing.T) {
	for _, interval := range []bool{false, true} {
		name := "catalog long context"
		if interval {
			name = "channel interval"
		}
		t.Run(name, func(t *testing.T) {
			resolved := &ResolvedPricing{
				Mode: BillingModeToken, Source: PricingSourceChannel,
				BasePricing: &ModelPricing{
					InputPricePerToken: 1e-6, ImageInputPricePerToken: 2e-6,
					OutputPricePerToken: 3e-6, ImageOutputPricePerToken: 4e-6,
					CacheCreationPricePerToken: 5e-6, CacheReadPricePerToken: 6e-6,
					FastMultiplier:             testPtrFloat64(2.5),
					ReasoningEffortMultipliers: map[string]float64{"high": 1.7},
					LongContextInputThreshold:  100, LongContextInputMultiplier: 2, LongContextOutputMultiplier: 3,
				},
				longContextPricingEnabled: true,
				channelPricing: &ChannelModelPricing{TimePricing: &ChannelTimePricing{
					Timezone: "UTC", Periods: []ChannelTimePricingPeriod{{StartTime: "08:00", EndTime: "16:00", Multiplier: 1.2}},
				}},
			}
			if interval {
				resolved.Intervals = []PricingInterval{{MinTokens: 100, InputMultiplier: testPtrFloat64(2), OutputMultiplier: testPtrFloat64(3)}}
			}
			input := CostInput{
				Model: "custom-model", RateMultiplier: 0.8, ServiceTier: "fast",
				Tokens:   UsageTokens{InputTokens: 200, ImageInputTokens: 20, OutputTokens: 10, ImageOutputTokens: 2, CacheCreationTokens: 5, CacheReadTokens: 3},
				Resolver: &ModelPricingResolver{}, Resolved: resolved,
				PricingAt: time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC),
			}
			bs := &BillingService{}
			standard, err := bs.CalculateCostUnified(input)
			require.NoError(t, err)
			input.ReasoningEffort = "high"
			adjusted, err := bs.CalculateCostUnified(input)
			require.NoError(t, err)
			for _, costs := range [][2]float64{
				{standard.InputCost, adjusted.InputCost}, {standard.ImageInputCost, adjusted.ImageInputCost},
				{standard.OutputCost, adjusted.OutputCost}, {standard.ImageOutputCost, adjusted.ImageOutputCost},
				{standard.CacheCreationCost, adjusted.CacheCreationCost}, {standard.CacheReadCost, adjusted.CacheReadCost},
				{standard.TotalCost, adjusted.TotalCost}, {standard.ActualCost, adjusted.ActualCost},
			} {
				require.Positive(t, costs[0])
				require.InDelta(t, costs[0]*1.7, costs[1], 1e-12)
			}
			require.Equal(t, standard.LongContextBillingApplied, adjusted.LongContextBillingApplied)
		})
	}
}

func TestReasoningEffortBillingGroupOverrideAndUnitBilling(t *testing.T) {
	for _, mode := range []BillingMode{BillingModeToken, BillingModePerRequest, BillingModeImage, BillingModeVideo} {
		t.Run(string(mode), func(t *testing.T) {
			bs, resolver := newTokenCostTestEnv(t, PlatformOpenAI, []ChannelModelPricing{{
				Platform: PlatformOpenAI, Models: []string{"custom-model"}, BillingMode: mode,
				InputPrice: testPtrFloat64(1e-6), PerRequestPrice: testPtrFloat64(0.1),
				ReasoningEffortMultipliers: map[string]float64{"high": 2, "max": 3},
			}}, nil)
			group := &Group{ID: 100, Platform: PlatformOpenAI, ModelPricing: []ChannelModelPricing{{
				Models: []string{"custom-model"}, BillingMode: mode,
				InputPrice: testPtrFloat64(1e-6), PerRequestPrice: testPtrFloat64(0.1),
				ReasoningEffortMultipliers: map[string]float64{"high": 1.25},
			}}}
			for effort, multiplier := range map[string]float64{"high": 1.25, "max": 1} {
				input := CostInput{
					Ctx: context.Background(), Model: "custom-model", Group: group, GroupID: &group.ID,
					Tokens: UsageTokens{InputTokens: 1000}, RequestCount: 2, RateMultiplier: 0.8,
					ReasoningEffort: effort, Resolver: resolver,
				}
				cost, err := bs.CalculateCostUnified(input)
				require.NoError(t, err)
				base := 0.2
				if mode == BillingModeToken {
					base = 0.001
				}
				require.InDelta(t, base*multiplier, cost.TotalCost, 1e-12)
				require.InDelta(t, cost.TotalCost*0.8, cost.ActualCost, 1e-12)
			}
		})
	}
}

func TestAccountStatsCustomRulesUseOwnReasoningEffortMultipliers(t *testing.T) {
	for _, mode := range []BillingMode{BillingModeToken, BillingModePerRequest, BillingModeImage, BillingModeVideo} {
		t.Run(string(mode), func(t *testing.T) {
			channel := &Channel{AccountStatsPricingRules: []AccountStatsPricingRule{{
				AccountIDs: []int64{1}, Pricing: []ChannelModelPricing{{
					Models: []string{"claude-fable-5-1"}, BillingMode: mode,
					InputPrice: testPtrFloat64(0.001), PerRequestPrice: testPtrFloat64(0.1),
					ReasoningEffortMultipliers: map[string]float64{"high": 1.5},
				}},
			}}}
			for effort, multiplier := range map[string]float64{"high": 1.5, "max": 1} {
				cost := tryCustomRules(channel, 1, 100, PlatformAnthropic, "claude-fable-5-1", UsageTokens{InputTokens: 100}, 2, effort)
				require.NotNil(t, cost)
				base := 0.2
				if mode == BillingModeToken {
					base = 0.1
				}
				require.InDelta(t, base*multiplier, *cost, 1e-12)
			}
		})
	}
}

func TestDisplayPricingPreservesConfiguredReasoningEffortMultipliers(t *testing.T) {
	raw := &ChannelModelPricing{ReasoningEffortMultipliers: map[string]float64{"high": 1.5, "max": 3}}
	for _, mode := range []BillingMode{BillingModeToken, BillingModePerRequest, BillingModeImage} {
		raw.BillingMode = mode
		display := synthesizePricingFromLiteLLM(&LiteLLMModelPricing{InputCostPerToken: 1e-6, OutputCostPerImage: 0.1}, raw)
		require.Equal(t, raw.ReasoningEffortMultipliers, display.ReasoningEffortMultipliers)
		display.ReasoningEffortMultipliers["high"] = 9
		require.Equal(t, 1.5, raw.ReasoningEffortMultipliers["high"])
	}
	display := plazaPricingFromSchedule(raw, &ContextPricingSchedule{Tiers: []ContextPricingTier{{Input: testPtrFloat64(1e-6)}}})
	require.Equal(t, raw.ReasoningEffortMultipliers, display.ReasoningEffortMultipliers)
	display.ReasoningEffortMultipliers["max"] = 9
	require.Equal(t, 3.0, raw.ReasoningEffortMultipliers["max"])
}

func TestPlazaUsesGroupReasoningEffortMultipliers(t *testing.T) {
	m := &PlazaModel{Name: "custom-model", Platform: PlatformOpenAI, Pricing: &ChannelModelPricing{
		ReasoningEffortMultipliers: map[string]float64{"max": 3},
	}}
	g := &Group{ModelPricing: []ChannelModelPricing{{
		Models: []string{"custom-model"}, BillingMode: BillingModeToken,
		ReasoningEffortMultipliers: map[string]float64{"high": 1.5},
	}}}
	(&ModelPlazaService{}).fillDisplayPricing(context.Background(), m, g)
	require.Equal(t, map[string]float64{"high": 1.5}, m.Pricing.ReasoningEffortMultipliers)
	m.Pricing.ReasoningEffortMultipliers["high"] = 9
	require.Equal(t, 1.5, g.ModelPricing[0].ReasoningEffortMultipliers["high"])
}

func TestReasoningEffortBillingNoResolverPreservesCatalogPolicies(t *testing.T) {
	bs := NewBillingService(&config.Config{}, nil)
	tokens := UsageTokens{InputTokens: 300000, OutputTokens: 100, CacheCreationTokens: 20, CacheReadTokens: 10}
	for _, applyLongContext := range []bool{false, true} {
		for _, serviceTier := range []string{"", "priority", "flex"} {
			for _, rateMultiplier := range []float64{-1, 0, 0.8} {
				want, err := bs.calculateCostInternalWithPolicy("gpt-5.4", tokens, rateMultiplier, serviceTier, nil, applyLongContext)
				require.NoError(t, err)
				got, err := bs.CalculateCostUnified(CostInput{
					Model: "gpt-5.4", Tokens: tokens, RateMultiplier: rateMultiplier,
					ServiceTier: serviceTier, LongContextBillingEnabled: &applyLongContext, ReasoningEffort: "max",
				})
				require.NoError(t, err)
				require.Equal(t, want, got)
			}
		}
	}
}

func TestReasoningEffortBillingPreservesForwardedNoneAndMinimal(t *testing.T) {
	for _, effort := range []string{"none", "minimal"} {
		t.Run(effort, func(t *testing.T) {
			for _, body := range [][]byte{
				[]byte(`{"model":"gpt-5.4","reasoning":{"effort":"` + effort + `"}}`),
				[]byte(`{"model":"gpt-5.4","reasoning_effort":"` + effort + `"}`),
			} {
				got := extractOpenAIReasoningEffortFromBody(nil, body, "gpt-5.4")
				require.NotNil(t, got)
				require.Equal(t, effort, *got)
				require.Equal(t, 0.5, reasoningEffortBillingMultiplier(*got, map[string]float64{effort: 0.5}))
				require.Equal(t, got, extractCCReasoningEffortFromBody(body, "gpt-5.4"))
			}
			for _, body := range []map[string]any{
				{"reasoning": map[string]any{"effort": effort}},
				{"reasoning_effort": effort},
			} {
				got := extractOpenAIReasoningEffort(nil, body, "gpt-5.4")
				require.NotNil(t, got)
				require.Equal(t, effort, *got)
			}
			body := []byte(`{"model":"gpt-5.4","reasoning":{"effort":"` + effort + `"}}`)
			got := ExtractResponsesReasoningEffortFromBody(body, "gpt-5.4")
			require.NotNil(t, got)
			require.Equal(t, effort, *got)
		})
	}

	// A compatibility endpoint that strips none must not bill the removed request preference.
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": "https://compat.example/v1"}}
	filtered, err := filterOpenAIResponsesNoneReasoningEffortForAccount(account, []byte(`{"model":"custom-model","reasoning":{"effort":"none"}}`))
	require.NoError(t, err)
	require.Nil(t, extractOpenAIReasoningEffortFromBody(nil, filtered, "custom-model"))
}
