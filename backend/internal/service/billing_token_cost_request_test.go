//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// newTokenCostTestEnv 构造带渠道定价的计费环境：group 100 挂一个渠道，定价由 pricing 指定。
func newTokenCostTestEnv(t *testing.T, groupPlatform string, pricing []ChannelModelPricing, catalog *PricingService) (*BillingService, *ModelPricingResolver) {
	t.Helper()
	repo := &mockChannelRepository{
		listAllFn: func(_ context.Context) ([]Channel, error) {
			return []Channel{{
				ID: 1, Name: "ch", Status: StatusActive, GroupIDs: []int64{100}, ModelPricing: pricing,
			}}, nil
		},
		getGroupPlatformsFn: func(_ context.Context, _ []int64) (map[int64]string, error) {
			return map[int64]string{100: groupPlatform}, nil
		},
	}
	cs := NewChannelService(repo, nil, nil, nil)
	bs := NewBillingService(&config.Config{}, catalog)
	return bs, NewModelPricingResolver(cs, bs)
}

func geminiCatalogStub() *PricingService {
	return newStubPricingServiceFromMap(map[string]*LiteLLMModelPricing{
		"gemini-2.5-pro": {
			Mode:                    "chat",
			InputCostPerToken:       1.25e-6,
			OutputCostPerToken:      10e-6,
			CacheReadInputTokenCost: 0.3125e-6,
		},
	})
}

func TestLegacyLongContextRule_OnlyGemini(t *testing.T) {
	bs := NewBillingService(&config.Config{}, nil)
	rule := bs.LegacyLongContextRule(PlatformGemini)
	require.NotNil(t, rule)
	require.Equal(t, 200000, rule.Threshold)
	require.InDelta(t, 2.0, rule.Multiplier, 1e-12)
	for _, platform := range []string{PlatformOpenAI, PlatformAnthropic, PlatformAntigravity, PlatformComposite, ""} {
		require.Nil(t, bs.LegacyLongContextRule(platform), platform)
	}
}

func TestCalculateTokenCostForRequest_ChannelPricingWinsOverLegacyRule(t *testing.T) {
	bs, resolver := newTokenCostTestEnv(t, PlatformGemini, []ChannelModelPricing{{
		Platform: PlatformGemini, Models: []string{"gemini-2.5-pro"}, BillingMode: BillingModeToken,
		InputPrice: testPtrFloat64(10e-6), OutputPrice: testPtrFloat64(40e-6),
	}}, geminiCatalogStub())
	group := &Group{ID: 100, Platform: PlatformGemini, LongContextPricingEnabled: true}
	gid := group.ID
	resolved := resolver.Resolve(context.Background(), PricingInput{Model: "gemini-2.5-pro", GroupID: &gid, Group: group})
	require.Equal(t, PricingSourceChannel, resolved.Source)

	tokens := UsageTokens{InputTokens: 300000, OutputTokens: 1000}
	got, err := bs.CalculateTokenCostForRequest(TokenCostRequest{
		Ctx: context.Background(), Model: "gemini-2.5-pro", Group: group, Tokens: tokens, RateMultiplier: 1,
		Resolver: resolver, Resolved: resolved, LegacyLongContext: bs.LegacyLongContextRule(PlatformGemini),
	})
	require.NoError(t, err)
	want, err := bs.CalculateCostUnified(CostInput{
		Ctx: context.Background(), Model: "gemini-2.5-pro", GroupID: &gid, Group: group, Tokens: tokens,
		RequestCount: 1, RateMultiplier: 1, Resolver: resolver, Resolved: resolved,
	})
	require.NoError(t, err)
	require.Equal(t, want, got)
	// 渠道平价 10e-6 × 300K，旧规则未叠加
	require.InDelta(t, 3.0, got.InputCost, 1e-9)
}

func TestCalculateTokenCostForRequest_LegacyRuleFollowsGroupToggle(t *testing.T) {
	bs, resolver := newTokenCostTestEnv(t, PlatformGemini, nil, geminiCatalogStub())
	tokens := UsageTokens{InputTokens: 300000, OutputTokens: 1000}
	rule := bs.LegacyLongContextRule(PlatformGemini)

	for _, enabled := range []bool{true, false} {
		group := &Group{ID: 100, Platform: PlatformGemini, LongContextPricingEnabled: enabled}
		gid := group.ID
		resolved := resolver.Resolve(context.Background(), PricingInput{Model: "gemini-2.5-pro", GroupID: &gid, Group: group})
		require.Equal(t, PricingSourceLiteLLM, resolved.Source)

		got, err := bs.CalculateTokenCostForRequest(TokenCostRequest{
			Ctx: context.Background(), Model: "gemini-2.5-pro", Group: group, Tokens: tokens, RateMultiplier: 1,
			Resolver: resolver, Resolved: resolved, LegacyLongContext: rule,
		})
		require.NoError(t, err)
		if enabled {
			want, err := bs.CalculateCostWithLongContext("gemini-2.5-pro", tokens, 1, rule.Threshold, rule.Multiplier)
			require.NoError(t, err)
			require.Equal(t, want, got)
			// 输入 200K × 1.25e-6 + 超出 100K × 1.25e-6 × 2 = 0.5；输出 1000 × 10e-6 = 0.01。
			// 旧路径的加倍只体现在 ActualCost（分项 InputCost 不含倍率），探针也据此取值。
			require.InDelta(t, 0.51, got.ActualCost, 1e-9)
			require.True(t, got.LongContextBillingApplied)
		} else {
			want, err := bs.CalculateCostUnified(CostInput{
				Ctx: context.Background(), Model: "gemini-2.5-pro", GroupID: &gid, Group: group, Tokens: tokens,
				RequestCount: 1, RateMultiplier: 1, Resolver: resolver, Resolved: resolved,
			})
			require.NoError(t, err)
			require.Equal(t, want, got)
			require.InDelta(t, 0.385, got.ActualCost, 1e-9)
			require.False(t, got.LongContextBillingApplied)
		}
	}
}

func TestCalculateTokenCostForRequest_BuiltInPricingUsesUnifiedPath(t *testing.T) {
	bs, resolver := newTokenCostTestEnv(t, PlatformOpenAI, nil, nil)
	group := &Group{ID: 100, Platform: PlatformOpenAI, LongContextPricingEnabled: true}
	gid := group.ID
	tokens := UsageTokens{InputTokens: 300000, OutputTokens: 1000}
	resolved := resolver.Resolve(context.Background(), PricingInput{Model: "gpt-5.4", GroupID: &gid, Group: group})

	got, err := bs.CalculateTokenCostForRequest(TokenCostRequest{
		Ctx: context.Background(), Model: "gpt-5.4", Group: group, Tokens: tokens, RateMultiplier: 1,
		Resolver: resolver, Resolved: resolved,
	})
	require.NoError(t, err)
	want, err := bs.CalculateCostUnified(CostInput{
		Ctx: context.Background(), Model: "gpt-5.4", GroupID: &gid, Group: group, Tokens: tokens,
		RequestCount: 1, RateMultiplier: 1, Resolver: resolver,
	})
	require.NoError(t, err)
	require.Equal(t, want, got)
	// 目录阶梯：超 272K 整单输入 ×2
	require.InDelta(t, 300000*2.5e-6*2, got.InputCost, 1e-9)
	require.True(t, got.LongContextBillingApplied)
}

func TestCalculateTokenCostForRequest_NoResolverFallsBackToCatalog(t *testing.T) {
	bs := NewBillingService(&config.Config{}, nil)
	tokens := UsageTokens{InputTokens: 1000, OutputTokens: 10}
	got, err := bs.CalculateTokenCostForRequest(TokenCostRequest{Model: "gpt-5.4", Tokens: tokens, RateMultiplier: 1})
	require.NoError(t, err)
	want, err := bs.CalculateCost("gpt-5.4", tokens, 1)
	require.NoError(t, err)
	require.Equal(t, want, got)
}
