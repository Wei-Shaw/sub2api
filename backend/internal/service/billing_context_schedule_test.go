//go:build unit

package service

import (
	"context"
	"math"
	"math/rand"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// scheduleScenario 阶梯表测试场景：同一份场景既做断言也做与计费函数的对账。
type scheduleScenario struct {
	name          string
	model         string
	platform      string
	groupPlatform string
	group         *Group
	channel       []ChannelModelPricing
	catalog       *PricingService
	wantErr       bool
	wantNil       bool
	wantBasis     ContextPricingBasis
	check         func(t *testing.T, s *ContextPricingSchedule)
}

func enabledGroup(platform string) *Group {
	return &Group{ID: 100, Platform: platform, LongContextPricingEnabled: true}
}

func disabledGroup(platform string) *Group {
	return &Group{ID: 100, Platform: platform, LongContextPricingEnabled: false}
}

func sonnetChannel(iv ...PricingInterval) []ChannelModelPricing {
	return []ChannelModelPricing{{
		Platform: PlatformAnthropic, Models: []string{"claude-sonnet-4"}, BillingMode: BillingModeToken,
		InputPrice: testPtrFloat64(2e-6), Intervals: iv,
	}}
}

func requireTier(t *testing.T, tier ContextPricingTier, min int, max *int, label string, input, output, cacheWrite, cacheRead *float64) {
	t.Helper()
	require.Equal(t, min, tier.MinTokens, "min")
	if max == nil {
		require.Nil(t, tier.MaxTokens, "max should be unbounded")
	} else {
		require.NotNil(t, tier.MaxTokens, "max")
		require.Equal(t, *max, *tier.MaxTokens, "max")
	}
	require.Equal(t, label, tier.Label, "label")
	requirePrice(t, input, tier.Input, "input")
	requirePrice(t, output, tier.Output, "output")
	requirePrice(t, cacheWrite, tier.CacheWrite, "cache_write")
	requirePrice(t, cacheRead, tier.CacheRead, "cache_read")
}

func requirePrice(t *testing.T, want, got *float64, field string) {
	t.Helper()
	if want == nil {
		require.Nil(t, got, field)
		return
	}
	require.NotNil(t, got, field)
	require.InDelta(t, *want, *got, 1e-15, field)
}

func scheduleScenarios() []scheduleScenario {
	p := testPtrFloat64
	return []scheduleScenario{
		{
			name: "官方阶梯 gpt-5.4 整单两档", model: "gpt-5.4", platform: PlatformOpenAI, groupPlatform: PlatformOpenAI,
			group: enabledGroup(PlatformOpenAI), wantBasis: ContextPricingBasisWholeRequest,
			check: func(t *testing.T, s *ContextPricingSchedule) {
				require.Len(t, s.Tiers, 2)
				requireTier(t, s.Tiers[0], 0, intPtr(272000), "≤272K", p(2.5e-6), p(15e-6), p(2.5e-6), p(0.25e-6))
				requireTier(t, s.Tiers[1], 272000, nil, ">272K", p(5e-6), p(22.5e-6), p(5e-6), p(0.5e-6))
			},
		},
		{
			name: "Grok 达到阈值即进高档", model: "grok-4.5", platform: PlatformGrok, groupPlatform: PlatformGrok,
			group: enabledGroup(PlatformGrok), wantBasis: ContextPricingBasisWholeRequest,
			check: func(t *testing.T, s *ContextPricingSchedule) {
				require.Len(t, s.Tiers, 2)
				requireTier(t, s.Tiers[0], 0, intPtr(199999), "<200K", p(2e-6), p(6e-6), nil, p(0.3e-6))
				requireTier(t, s.Tiers[1], 199999, nil, "≥200K", p(4e-6), p(12e-6), nil, p(0.6e-6))
			},
		},
		{
			name: "分组关闭阶梯只剩基础档", model: "gpt-5.4", platform: PlatformOpenAI, groupPlatform: PlatformOpenAI,
			group: disabledGroup(PlatformOpenAI), wantBasis: ContextPricingBasisWholeRequest,
			check: func(t *testing.T, s *ContextPricingSchedule) {
				require.Len(t, s.Tiers, 1)
				requireTier(t, s.Tiers[0], 0, nil, "", p(2.5e-6), p(15e-6), p(2.5e-6), p(0.25e-6))
			},
		},
		{
			name: "官方参考价（无分组）带目录阶梯", model: "gpt-5.4", platform: "", groupPlatform: PlatformOpenAI,
			group: nil, wantBasis: ContextPricingBasisWholeRequest,
			check: func(t *testing.T, s *ContextPricingSchedule) {
				require.Len(t, s.Tiers, 2)
				requireTier(t, s.Tiers[1], 272000, nil, ">272K", p(5e-6), p(22.5e-6), p(5e-6), p(0.5e-6))
			},
		},
		{
			name: "渠道倍率区间按渠道平价覆盖后的 base 折算（区间自定义标签不用于档位）", model: "claude-sonnet-4", platform: PlatformAnthropic, groupPlatform: PlatformAnthropic,
			group: enabledGroup(PlatformAnthropic), wantBasis: ContextPricingBasisWholeRequest,
			channel: sonnetChannel(
				PricingInterval{MinTokens: 0, MaxTokens: intPtr(200000), InputMultiplier: p(1)},
				PricingInterval{MinTokens: 200000, TierLabel: "long", InputMultiplier: p(2), OutputMultiplier: p(1.5), CacheWriteMultiplier: p(2), CacheReadMultiplier: p(2)},
			),
			check: func(t *testing.T, s *ContextPricingSchedule) {
				require.Len(t, s.Tiers, 2)
				requireTier(t, s.Tiers[0], 0, intPtr(200000), "≤200K", p(2e-6), p(15e-6), p(3.75e-6), p(0.3e-6))
				requireTier(t, s.Tiers[1], 200000, nil, ">200K", p(4e-6), p(22.5e-6), p(7.5e-6), p(0.6e-6))
			},
		},
		{
			name: "区间显式价优先于倍率", model: "claude-sonnet-4", platform: PlatformAnthropic, groupPlatform: PlatformAnthropic,
			group: enabledGroup(PlatformAnthropic), wantBasis: ContextPricingBasisWholeRequest,
			channel: sonnetChannel(
				PricingInterval{MinTokens: 0, MaxTokens: intPtr(200000), InputMultiplier: p(1)},
				PricingInterval{MinTokens: 200000, InputPrice: p(9e-6), InputMultiplier: p(2)},
			),
			check: func(t *testing.T, s *ContextPricingSchedule) {
				require.Len(t, s.Tiers, 2)
				requirePrice(t, p(9e-6), s.Tiers[1].Input, "input")
			},
		},
		{
			name: "区间空洞按 base 补档且同价段合并", model: "claude-sonnet-4", platform: PlatformAnthropic, groupPlatform: PlatformAnthropic,
			group: enabledGroup(PlatformAnthropic), wantBasis: ContextPricingBasisWholeRequest,
			channel: sonnetChannel(
				PricingInterval{MinTokens: 0, MaxTokens: intPtr(100000), InputMultiplier: p(0.5)},
				PricingInterval{MinTokens: 200000, InputMultiplier: p(2)},
			),
			check: func(t *testing.T, s *ContextPricingSchedule) {
				require.Len(t, s.Tiers, 3)
				requireTier(t, s.Tiers[0], 0, intPtr(100000), "≤100K", p(1e-6), p(15e-6), p(3.75e-6), p(0.3e-6))
				requireTier(t, s.Tiers[1], 100000, intPtr(200000), "≤200K", p(2e-6), p(15e-6), p(3.75e-6), p(0.3e-6))
				requireTier(t, s.Tiers[2], 200000, nil, ">200K", p(4e-6), p(15e-6), p(3.75e-6), p(0.3e-6))
			},
		},
		{
			name: "首档同价空洞被合并", model: "claude-sonnet-4", platform: PlatformAnthropic, groupPlatform: PlatformAnthropic,
			group: enabledGroup(PlatformAnthropic), wantBasis: ContextPricingBasisWholeRequest,
			channel: sonnetChannel(
				PricingInterval{MinTokens: 0, MaxTokens: intPtr(100000), InputMultiplier: p(1)},
				PricingInterval{MinTokens: 200000, InputMultiplier: p(2)},
			),
			check: func(t *testing.T, s *ContextPricingSchedule) {
				require.Len(t, s.Tiers, 2)
				requireTier(t, s.Tiers[0], 0, intPtr(200000), "≤200K", p(2e-6), p(15e-6), p(3.75e-6), p(0.3e-6))
				requireTier(t, s.Tiers[1], 200000, nil, ">200K", p(4e-6), p(15e-6), p(3.75e-6), p(0.3e-6))
			},
		},
		{
			name: "末档有上限时尾段回落 base", model: "claude-sonnet-4", platform: PlatformAnthropic, groupPlatform: PlatformAnthropic,
			group: enabledGroup(PlatformAnthropic), wantBasis: ContextPricingBasisWholeRequest,
			channel: sonnetChannel(
				PricingInterval{MinTokens: 0, MaxTokens: intPtr(200000), InputMultiplier: p(1)},
				PricingInterval{MinTokens: 200000, MaxTokens: intPtr(1000000), InputMultiplier: p(2)},
			),
			check: func(t *testing.T, s *ContextPricingSchedule) {
				require.Len(t, s.Tiers, 3)
				requireTier(t, s.Tiers[1], 200000, intPtr(1000000), "≤1M", p(4e-6), p(15e-6), p(3.75e-6), p(0.3e-6))
				requireTier(t, s.Tiers[2], 1000000, nil, ">1M", p(2e-6), p(15e-6), p(3.75e-6), p(0.3e-6))
			},
		},
		{
			name: "分组关闭时渠道区间折叠到 1-token 档", model: "claude-sonnet-4", platform: PlatformAnthropic, groupPlatform: PlatformAnthropic,
			group: disabledGroup(PlatformAnthropic), wantBasis: ContextPricingBasisWholeRequest,
			channel: sonnetChannel(
				PricingInterval{MinTokens: 0, MaxTokens: intPtr(200000), InputMultiplier: p(0.5)},
				PricingInterval{MinTokens: 200000, InputMultiplier: p(2)},
			),
			check: func(t *testing.T, s *ContextPricingSchedule) {
				require.Len(t, s.Tiers, 1)
				requireTier(t, s.Tiers[0], 0, nil, "", p(1e-6), p(15e-6), p(3.75e-6), p(0.3e-6))
			},
		},
		{
			name: "分组关闭且首档不从 0 起时平价为 base", model: "claude-sonnet-4", platform: PlatformAnthropic, groupPlatform: PlatformAnthropic,
			group: disabledGroup(PlatformAnthropic), wantBasis: ContextPricingBasisWholeRequest,
			channel: sonnetChannel(PricingInterval{MinTokens: 200000, InputMultiplier: p(2)}),
			check: func(t *testing.T, s *ContextPricingSchedule) {
				require.Len(t, s.Tiers, 1)
				requirePrice(t, p(2e-6), s.Tiers[0].Input, "input")
			},
		},
		{
			name: "分组 token 价卡整张替换渠道定价并剥区间", model: "claude-sonnet-4", platform: PlatformAnthropic, groupPlatform: PlatformAnthropic,
			group: &Group{ID: 100, Platform: PlatformAnthropic, LongContextPricingEnabled: true, ModelPricing: []ChannelModelPricing{{
				Models: []string{"claude-sonnet-*"}, BillingMode: BillingModeToken, InputPrice: p(1e-6),
				Intervals: []PricingInterval{{MinTokens: 200000, InputMultiplier: p(5)}},
			}}},
			channel:   sonnetChannel(PricingInterval{MinTokens: 200000, InputMultiplier: p(3)}),
			wantBasis: ContextPricingBasisWholeRequest,
			check: func(t *testing.T, s *ContextPricingSchedule) {
				require.Len(t, s.Tiers, 1)
				requireTier(t, s.Tiers[0], 0, nil, "", p(1e-6), p(15e-6), p(3.75e-6), p(0.3e-6))
			},
		},
		{
			name: "分组价卡之上叠加官方阶梯", model: "gpt-5.4", platform: PlatformOpenAI, groupPlatform: PlatformOpenAI,
			group: &Group{ID: 100, Platform: PlatformOpenAI, LongContextPricingEnabled: true, ModelPricing: []ChannelModelPricing{{
				Models: []string{"gpt-5.4"}, BillingMode: BillingModeToken, InputPrice: p(1e-6),
			}}},
			wantBasis: ContextPricingBasisWholeRequest,
			check: func(t *testing.T, s *ContextPricingSchedule) {
				require.Len(t, s.Tiers, 2)
				requirePrice(t, p(1e-6), s.Tiers[0].Input, "input")
				requirePrice(t, p(2e-6), s.Tiers[1].Input, "input")
				requirePrice(t, p(22.5e-6), s.Tiers[1].Output, "output")
			},
		},
		{
			name: "Gemini 旧规则按超出部分计价", model: "gemini-2.5-pro", platform: PlatformGemini, groupPlatform: PlatformGemini,
			group: enabledGroup(PlatformGemini), catalog: geminiCatalogStub(), wantBasis: ContextPricingBasisMarginal,
			check: func(t *testing.T, s *ContextPricingSchedule) {
				require.Len(t, s.Tiers, 2)
				requireTier(t, s.Tiers[0], 0, intPtr(200000), "≤200K", p(1.25e-6), p(10e-6), nil, p(0.3125e-6))
				requireTier(t, s.Tiers[1], 200000, nil, ">200K", p(2.5e-6), p(10e-6), nil, p(0.625e-6))
			},
		},
		{
			name: "Gemini 分组关闭时不用旧规则", model: "gemini-2.5-pro", platform: PlatformGemini, groupPlatform: PlatformGemini,
			group: disabledGroup(PlatformGemini), catalog: geminiCatalogStub(), wantBasis: ContextPricingBasisWholeRequest,
			check: func(t *testing.T, s *ContextPricingSchedule) {
				require.Len(t, s.Tiers, 1)
			},
		},
		{
			name: "Gemini 有渠道定价时旧规则让位", model: "gemini-2.5-pro", platform: PlatformGemini, groupPlatform: PlatformGemini,
			group: enabledGroup(PlatformGemini), catalog: geminiCatalogStub(),
			channel: []ChannelModelPricing{{
				Platform: PlatformGemini, Models: []string{"gemini-2.5-pro"}, BillingMode: BillingModeToken, InputPrice: p(3e-6),
			}},
			wantBasis: ContextPricingBasisWholeRequest,
			check: func(t *testing.T, s *ContextPricingSchedule) {
				require.Len(t, s.Tiers, 1)
				requirePrice(t, p(3e-6), s.Tiers[0].Input, "input")
			},
		},
		{
			name: "Gemini 官方参考不套用站内旧规则", model: "gemini-2.5-pro", platform: "", groupPlatform: PlatformGemini,
			group: nil, catalog: geminiCatalogStub(), wantBasis: ContextPricingBasisWholeRequest,
			check: func(t *testing.T, s *ContextPricingSchedule) {
				require.Len(t, s.Tiers, 1)
			},
		},
		{
			name: "composite 分组按模型平台取渠道定价并叠加官方阶梯", model: "gpt-5.4", platform: PlatformOpenAI, groupPlatform: PlatformComposite,
			group: enabledGroup(PlatformComposite),
			channel: []ChannelModelPricing{{
				Platform: PlatformOpenAI, Models: []string{"gpt-5.4"}, BillingMode: BillingModeToken, InputPrice: p(1e-6),
			}},
			wantBasis: ContextPricingBasisWholeRequest,
			check: func(t *testing.T, s *ContextPricingSchedule) {
				require.Len(t, s.Tiers, 2)
				requirePrice(t, p(1e-6), s.Tiers[0].Input, "input")
				requirePrice(t, p(2e-6), s.Tiers[1].Input, "input")
			},
		},
		{
			name: "渠道显式 cache_write=0 保留为 0", model: "claude-sonnet-4", platform: PlatformAnthropic, groupPlatform: PlatformAnthropic,
			group: enabledGroup(PlatformAnthropic),
			channel: []ChannelModelPricing{{
				Platform: PlatformAnthropic, Models: []string{"claude-sonnet-4"}, BillingMode: BillingModeToken,
				InputPrice: p(2e-6), CacheWritePrice: p(0),
			}},
			wantBasis: ContextPricingBasisWholeRequest,
			check: func(t *testing.T, s *ContextPricingSchedule) {
				require.Len(t, s.Tiers, 1)
				require.NotNil(t, s.Tiers[0].CacheWrite)
				require.Zero(t, *s.Tiers[0].CacheWrite)
			},
		},
		{
			name: "gpt-5.6 缺 cache_write 时按策略补 1.25 倍并带阶梯", model: "gpt-5.6-sol", platform: PlatformOpenAI, groupPlatform: PlatformOpenAI,
			group: enabledGroup(PlatformOpenAI),
			catalog: newStubPricingServiceFromMap(map[string]*LiteLLMModelPricing{
				"gpt-5.6-sol": {Mode: "chat", InputCostPerToken: 5e-6, OutputCostPerToken: 30e-6, CacheReadInputTokenCost: 0.5e-6},
			}),
			wantBasis: ContextPricingBasisWholeRequest,
			check: func(t *testing.T, s *ContextPricingSchedule) {
				require.Len(t, s.Tiers, 2)
				requireTier(t, s.Tiers[0], 0, intPtr(272000), "≤272K", p(5e-6), p(30e-6), p(6.25e-6), p(0.5e-6))
				requireTier(t, s.Tiers[1], 272000, nil, ">272K", p(10e-6), p(45e-6), p(12.5e-6), p(1e-6))
			},
		},
		{
			name: "图片模式返回 nil", model: "gpt-image-2", platform: PlatformOpenAI, groupPlatform: PlatformOpenAI,
			group: enabledGroup(PlatformOpenAI),
			channel: []ChannelModelPricing{{
				Platform: PlatformOpenAI, Models: []string{"gpt-image-2"}, BillingMode: BillingModeImage, PerRequestPrice: p(0.04),
			}},
			wantNil: true,
		},
		{
			name: "无任何定价来源报错", model: "unknown-model-xyz", platform: PlatformOpenAI, groupPlatform: PlatformOpenAI,
			group: enabledGroup(PlatformOpenAI), wantErr: true,
		},
	}
}

func newScheduleTestEnv(t *testing.T, sc scheduleScenario) (*BillingService, *ModelPricingResolver) {
	t.Helper()
	return newTokenCostTestEnv(t, sc.groupPlatform, sc.channel, sc.catalog)
}

func TestResolveContextPricingSchedule_Scenarios(t *testing.T) {
	for _, sc := range scheduleScenarios() {
		t.Run(sc.name, func(t *testing.T) {
			bs, resolver := newScheduleTestEnv(t, sc)
			sched, err := bs.ResolveContextPricingSchedule(context.Background(), resolver, ContextPricingScheduleInput{
				Model: sc.model, Group: sc.group, Platform: sc.platform,
			})
			if sc.wantErr {
				require.Error(t, err)
				require.Nil(t, sched)
				return
			}
			require.NoError(t, err)
			if sc.wantNil {
				require.Nil(t, sched)
				return
			}
			require.NotNil(t, sched)
			require.Equal(t, sc.wantBasis, sched.Basis)
			require.NotEmpty(t, sched.Tiers)
			require.Zero(t, sched.Tiers[0].MinTokens, "首档从 0 起")
			for i := 1; i < len(sched.Tiers); i++ {
				require.NotNil(t, sched.Tiers[i-1].MaxTokens)
				require.Equal(t, *sched.Tiers[i-1].MaxTokens, sched.Tiers[i].MinTokens, "档位连续")
				require.Less(t, sched.Tiers[i-1].MinTokens, sched.Tiers[i].MinTokens, "档位按上下文升序")
			}
			if len(sched.Tiers) > 1 {
				for i, tier := range sched.Tiers {
					require.NotEmpty(t, tier.Label, "多档时每档都有标签 #%d", i)
				}
				require.Nil(t, sched.Tiers[len(sched.Tiers)-1].MaxTokens, "末档无上限")
			}
			if sc.check != nil {
				sc.check(t, sched)
			}
		})
	}
}

func TestResolveContextPricingSchedule_NilResolver(t *testing.T) {
	bs := NewBillingService(&config.Config{}, nil)
	sched, err := bs.ResolveContextPricingSchedule(context.Background(), nil, ContextPricingScheduleInput{Model: "gpt-5.4"})
	require.Error(t, err)
	require.Nil(t, sched)
}

// --- 对账：阶梯表推算的费用必须等于真实计费函数 ---

type tokenKind int

const (
	kindInput tokenKind = iota
	kindOutput
	kindCacheRead
	kindCacheWrite
)

func tierPrice(tier ContextPricingTier, kind tokenKind) float64 {
	var p *float64
	switch kind {
	case kindInput:
		p = tier.Input
	case kindOutput:
		p = tier.Output
	case kindCacheRead:
		p = tier.CacheRead
	case kindCacheWrite:
		p = tier.CacheWrite
	}
	if p == nil {
		return 0
	}
	return *p
}

func tierAt(tiers []ContextPricingTier, contextTokens int) ContextPricingTier {
	for _, tier := range tiers {
		if contextTokens > tier.MinTokens && (tier.MaxTokens == nil || contextTokens <= *tier.MaxTokens) {
			return tier
		}
	}
	return tiers[len(tiers)-1]
}

// expectedCostFromSchedule 按阶梯表推算 contextTokens 个某类 token 的费用：
// 整单基准取所在档单价 × 全量；边际基准逐段累加。
func expectedCostFromSchedule(s *ContextPricingSchedule, kind tokenKind, contextTokens int) float64 {
	if s.Basis == ContextPricingBasisMarginal {
		total := 0.0
		for _, tier := range s.Tiers {
			if contextTokens <= tier.MinTokens {
				break
			}
			upper := contextTokens
			if tier.MaxTokens != nil && *tier.MaxTokens < upper {
				upper = *tier.MaxTokens
			}
			total += float64(upper-tier.MinTokens) * tierPrice(tier, kind)
		}
		return total
	}
	return float64(contextTokens) * tierPrice(tierAt(s.Tiers, contextTokens), kind)
}

func TestResolveContextPricingSchedule_ParityWithBilling(t *testing.T) {
	const outputProbe = 1000
	rng := rand.New(rand.NewSource(20260823))
	for _, sc := range scheduleScenarios() {
		if sc.wantErr || sc.wantNil {
			continue
		}
		t.Run(sc.name, func(t *testing.T) {
			bs, resolver := newScheduleTestEnv(t, sc)
			ctx := context.Background()
			if sc.platform != "" {
				ctx = WithResolvedTargetPlatform(ctx, sc.platform)
			}
			sched, err := bs.ResolveContextPricingSchedule(ctx, resolver, ContextPricingScheduleInput{
				Model: sc.model, Group: sc.group, Platform: sc.platform,
			})
			require.NoError(t, err)
			require.NotNil(t, sched)

			// 消费方视角重建同一份计费请求（与网关一致）。
			pricingInput := PricingInput{Model: sc.model, Group: sc.group}
			if sc.group != nil {
				gid := sc.group.ID
				pricingInput.GroupID = &gid
			}
			resolved := resolver.Resolve(ctx, pricingInput)
			var legacy *LegacyLongContextRule
			if sc.group != nil {
				legacy = bs.LegacyLongContextRule(sc.platform)
			}
			if !legacyLongContextApplies(resolved, sc.group, legacy) {
				legacy = nil
			}
			cost := func(tokens UsageTokens) float64 {
				bd, err := bs.CalculateTokenCostForRequest(TokenCostRequest{
					Ctx: ctx, Model: sc.model, Group: sc.group, Tokens: tokens, RateMultiplier: 1,
					Resolver: resolver, Resolved: resolved, LegacyLongContext: legacy,
				})
				require.NoError(t, err)
				return bd.ActualCost
			}

			probes := []int{1, 2, 999, 1000, 1001, 10_000_000}
			for _, tier := range sched.Tiers {
				for _, b := range []*int{tier.MaxTokens} {
					if b == nil {
						continue
					}
					probes = append(probes, *b-1, *b, *b+1)
				}
			}
			for i := 0; i < 8; i++ {
				probes = append(probes, 1+rng.Intn(3_000_000))
			}

			for _, c := range probes {
				if c <= 0 {
					continue
				}
				assertCostClose(t, expectedCostFromSchedule(sched, kindInput, c), cost(UsageTokens{InputTokens: c}), "input@%d", c)
				assertCostClose(t, expectedCostFromSchedule(sched, kindCacheRead, c), cost(UsageTokens{CacheReadTokens: c}), "cache_read@%d", c)
				assertCostClose(t, expectedCostFromSchedule(sched, kindCacheWrite, c), cost(UsageTokens{CacheCreationTokens: c}), "cache_write@%d", c)
				wantOutput := float64(outputProbe) * tierPrice(tierAt(sched.Tiers, c), kindOutput)
				gotOutput := cost(UsageTokens{InputTokens: c, OutputTokens: outputProbe}) - cost(UsageTokens{InputTokens: c})
				assertCostClose(t, wantOutput, gotOutput, "output@%d", c)
			}
		})
	}
}

func assertCostClose(t *testing.T, want, got float64, format string, args ...any) {
	t.Helper()
	tolerance := 1e-12 + 1e-9*math.Abs(want)
	require.InDeltaf(t, want, got, tolerance, format, args...)
}

func sonnetChannelWithTimePricing(tp *ChannelTimePricing) []ChannelModelPricing {
	return []ChannelModelPricing{{
		Platform: PlatformAnthropic, Models: []string{"claude-sonnet-4"}, BillingMode: BillingModeToken,
		InputPrice: testPtrFloat64(2e-6), TimePricing: tp,
	}}
}

func TestResolveContextPricingSchedule_TimePricing(t *testing.T) {
	valid := &ChannelTimePricing{Timezone: "Asia/Shanghai", Periods: []ChannelTimePricingPeriod{
		{StartTime: "18:00", EndTime: "22:00:00", Multiplier: 1.2},
		{StartTime: "00:30", EndTime: "08:30", Multiplier: 0.5},
		{StartTime: "12:00", EndTime: "13:00", Multiplier: 1},
	}}

	t.Run("渠道分时按开始时间升序列出且跳过倍率 1 的时段", func(t *testing.T) {
		bs, resolver := newTokenCostTestEnv(t, PlatformAnthropic, sonnetChannelWithTimePricing(valid), nil)
		sched, err := bs.ResolveContextPricingSchedule(context.Background(), resolver, ContextPricingScheduleInput{
			Model: "claude-sonnet-4", Group: enabledGroup(PlatformAnthropic), Platform: PlatformAnthropic,
		})
		require.NoError(t, err)
		require.NotNil(t, sched.TimePricing)
		require.Equal(t, "Asia/Shanghai", sched.TimePricing.Timezone)
		require.False(t, sched.TimePricing.WeekdaysOnly)
		require.Equal(t, []TimePricingPeriod{
			{StartTime: "00:30", EndTime: "08:30", Multiplier: 0.5},
			{StartTime: "18:00", EndTime: "22:00:00", Multiplier: 1.2},
		}, sched.TimePricing.Periods)
		// 阶梯表单价是标准时段价，不含分时倍率
		requirePrice(t, testPtrFloat64(2e-6), sched.Tiers[0].Input, "input")

		// 对账：时段内的真实计费 = 标准单价 × token × 倍率
		group := enabledGroup(PlatformAnthropic)
		gid := group.ID
		resolved := resolver.Resolve(context.Background(), PricingInput{Model: "claude-sonnet-4", GroupID: &gid, Group: group})
		loc, err := time.LoadLocation("Asia/Shanghai")
		require.NoError(t, err)
		for _, tc := range []struct {
			at   time.Time
			want float64
		}{
			{time.Date(2026, 8, 23, 3, 0, 0, 0, loc), 0.5},
			{time.Date(2026, 8, 23, 12, 30, 0, 0, loc), 1},
			{time.Date(2026, 8, 23, 21, 59, 59, 0, loc), 1.2},
			{time.Date(2026, 8, 23, 22, 0, 0, 0, loc), 1},
		} {
			cost, err := bs.CalculateTokenCostForRequest(TokenCostRequest{
				Ctx: context.Background(), Model: "claude-sonnet-4", Group: group, Tokens: UsageTokens{InputTokens: 1000},
				RateMultiplier: 1, PricingAt: tc.at, Resolver: resolver, Resolved: resolved,
			})
			require.NoError(t, err)
			assertCostClose(t, 1000*2e-6*tc.want, cost.ActualCost, "at %s", tc.at)
		}
	})

	t.Run("仅工作日配置透传标注且时段仍列出", func(t *testing.T) {
		weekdays := &ChannelTimePricing{Timezone: "Asia/Shanghai", WeekdaysOnly: true, Periods: []ChannelTimePricingPeriod{
			{StartTime: "00:30", EndTime: "08:30", Multiplier: 0.5},
		}}
		bs, resolver := newTokenCostTestEnv(t, PlatformAnthropic, sonnetChannelWithTimePricing(weekdays), nil)
		sched, err := bs.ResolveContextPricingSchedule(context.Background(), resolver, ContextPricingScheduleInput{
			Model: "claude-sonnet-4", Group: enabledGroup(PlatformAnthropic), Platform: PlatformAnthropic,
		})
		require.NoError(t, err)
		require.NotNil(t, sched.TimePricing, "探针锚点必须落在工作日，否则仅工作日配置的时段会被整组剔除")
		require.True(t, sched.TimePricing.WeekdaysOnly)
		require.Equal(t, []TimePricingPeriod{
			{StartTime: "00:30", EndTime: "08:30", Multiplier: 0.5},
		}, sched.TimePricing.Periods)

		// 对账：工作日时段内乘倍率，周末同一时段按标准价
		group := enabledGroup(PlatformAnthropic)
		gid := group.ID
		resolved := resolver.Resolve(context.Background(), PricingInput{Model: "claude-sonnet-4", GroupID: &gid, Group: group})
		loc, err := time.LoadLocation("Asia/Shanghai")
		require.NoError(t, err)
		for _, tc := range []struct {
			at   time.Time
			want float64
		}{
			{time.Date(2026, 8, 24, 3, 0, 0, 0, loc), 0.5}, // 周一
			{time.Date(2026, 8, 23, 3, 0, 0, 0, loc), 1},   // 周日
		} {
			cost, err := bs.CalculateTokenCostForRequest(TokenCostRequest{
				Ctx: context.Background(), Model: "claude-sonnet-4", Group: group, Tokens: UsageTokens{InputTokens: 1000},
				RateMultiplier: 1, PricingAt: tc.at, Resolver: resolver, Resolved: resolved,
			})
			require.NoError(t, err)
			assertCostClose(t, 1000*2e-6*tc.want, cost.ActualCost, "at %s", tc.at)
		}
	})

	t.Run("配置非法时计费按 1 计，阶梯表不列分时", func(t *testing.T) {
		invalid := &ChannelTimePricing{Timezone: "Asia/Shanghai", Periods: []ChannelTimePricingPeriod{
			{StartTime: "00:30", EndTime: "08:30", Multiplier: 0.5},
			{StartTime: "08:00", EndTime: "09:00", Multiplier: 0.8}, // 与上一段重叠
		}}
		bs, resolver := newTokenCostTestEnv(t, PlatformAnthropic, sonnetChannelWithTimePricing(invalid), nil)
		sched, err := bs.ResolveContextPricingSchedule(context.Background(), resolver, ContextPricingScheduleInput{
			Model: "claude-sonnet-4", Group: enabledGroup(PlatformAnthropic), Platform: PlatformAnthropic,
		})
		require.NoError(t, err)
		require.Nil(t, sched.TimePricing)
	})

	t.Run("分组价卡覆盖后渠道分时不再生效", func(t *testing.T) {
		bs, resolver := newTokenCostTestEnv(t, PlatformAnthropic, sonnetChannelWithTimePricing(valid), nil)
		group := &Group{ID: 100, Platform: PlatformAnthropic, LongContextPricingEnabled: true, ModelPricing: []ChannelModelPricing{{
			Models: []string{"claude-sonnet-4"}, BillingMode: BillingModeToken, InputPrice: testPtrFloat64(1e-6),
		}}}
		sched, err := bs.ResolveContextPricingSchedule(context.Background(), resolver, ContextPricingScheduleInput{
			Model: "claude-sonnet-4", Group: group, Platform: PlatformAnthropic,
		})
		require.NoError(t, err)
		require.Nil(t, sched.TimePricing)
	})

	t.Run("无分时配置为 nil", func(t *testing.T) {
		bs, resolver := newTokenCostTestEnv(t, PlatformAnthropic, sonnetChannelWithTimePricing(nil), nil)
		sched, err := bs.ResolveContextPricingSchedule(context.Background(), resolver, ContextPricingScheduleInput{
			Model: "claude-sonnet-4", Group: enabledGroup(PlatformAnthropic), Platform: PlatformAnthropic,
		})
		require.NoError(t, err)
		require.Nil(t, sched.TimePricing)
	})
}
