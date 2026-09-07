package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func pricingMultiplier(value float64) *float64 { return &value }

func TestConfiguredServiceTierMultiplier(t *testing.T) {
	tests := []struct {
		name        string
		serviceTier string
		pricing     *ModelPricing
		want        float64
	}{
		{name: "gpt-5.5 fast", serviceTier: "fast", pricing: &ModelPricing{FastMultiplier: pricingMultiplier(2.5)}, want: 2.5},
		{name: "priority alias", serviceTier: "priority", pricing: &ModelPricing{FastMultiplier: pricingMultiplier(2)}, want: 2},
		{name: "flex configured", serviceTier: "flex", pricing: &ModelPricing{FlexMultiplier: pricingMultiplier(0.4)}, want: 0.4},
		{name: "legacy fast default", serviceTier: "fast", pricing: &ModelPricing{}, want: 2},
		{name: "ultrafast default", serviceTier: "ultrafast", pricing: &ModelPricing{}, want: 2},
		{name: "ultrafast ignores fast multiplier", serviceTier: "ultrafast", pricing: &ModelPricing{FastMultiplier: pricingMultiplier(2.5)}, want: 2},
		{name: "legacy flex default", serviceTier: "flex", pricing: &ModelPricing{}, want: 0.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.InDelta(t, tt.want, configuredServiceTierMultiplier(tt.serviceTier, tt.pricing), 1e-12)
		})
	}
}

func TestConfiguredServiceTierMultiplierAppliesToEveryTokenComponent(t *testing.T) {
	pricing := &ModelPricing{
		InputPricePerToken:         5e-6,
		OutputPricePerToken:        30e-6,
		CacheCreationPricePerToken: 6.25e-6,
		CacheReadPricePerToken:     0.5e-6,
		FastMultiplier:             pricingMultiplier(2.5),
		FlexMultiplier:             pricingMultiplier(0.5),
	}
	tokens := UsageTokens{
		InputTokens:         1_000_000,
		OutputTokens:        1_000_000,
		CacheCreationTokens: 1_000_000,
		CacheReadTokens:     1_000_000,
	}
	service := &BillingService{}

	fast := service.computeTokenBreakdown(pricing, tokens, 1, "fast", false)
	require.InDelta(t, 12.5, fast.InputCost, 1e-12)
	require.InDelta(t, 75, fast.OutputCost, 1e-12)
	require.InDelta(t, 15.625, fast.CacheCreationCost, 1e-12)
	require.InDelta(t, 1.25, fast.CacheReadCost, 1e-12)

	flex := service.computeTokenBreakdown(pricing, tokens, 1, "flex", false)
	require.InDelta(t, 2.5, flex.InputCost, 1e-12)
	require.InDelta(t, 15, flex.OutputCost, 1e-12)
	require.InDelta(t, 3.125, flex.CacheCreationCost, 1e-12)
	require.InDelta(t, 0.25, flex.CacheReadCost, 1e-12)
}

func TestChannelOverridePreservesCatalogFastRatioByDefault(t *testing.T) {
	pricing := &ModelPricing{
		InputPricePerToken:          2,
		InputPricePerTokenPriority:  4,
		OutputPricePerToken:         6,
		OutputPricePerTokenPriority: 12,
	}
	applyChannelTokenPriceOverrides(pricing, &ChannelModelPricing{
		InputPrice:  pricingMultiplier(3),
		OutputPrice: pricingMultiplier(9),
	})

	require.InDelta(t, 3, pricing.InputPricePerToken, 1e-12)
	require.InDelta(t, 6, pricing.InputPricePerTokenPriority, 1e-12)
	require.InDelta(t, 9, pricing.OutputPricePerToken, 1e-12)
	require.InDelta(t, 18, pricing.OutputPricePerTokenPriority, 1e-12)
}

func TestAnthropicFastUsesDefaultMultiplierWithoutCatalogTier(t *testing.T) {
	pricing := &ModelPricing{InputPricePerToken: 5e-6, OutputPricePerToken: 25e-6}
	cost := (&BillingService{}).computeTokenBreakdown(pricing, UsageTokens{
		InputTokens: 1_000_000, OutputTokens: 1_000_000,
	}, 1, "fast", false)

	require.InDelta(t, 10, cost.InputCost, 1e-12)
	require.InDelta(t, 50, cost.OutputCost, 1e-12)
}

func TestBuiltInModelFastDefaults(t *testing.T) {
	service := &BillingService{fallbackPrices: make(map[string]*ModelPricing)}
	service.initFallbackPricing()

	for _, tt := range []struct {
		model string
		want  float64
	}{
		{model: "gpt-5.5", want: 2.5},
		{model: "claude-opus-4.8", want: 2},
		{model: "claude-opus-5", want: 2},
	} {
		pricing := service.fallbackPrices[tt.model]
		require.NotNil(t, pricing)
		require.InDelta(t, tt.want, pricing.InputPricePerTokenPriority/pricing.InputPricePerToken, 1e-12)
		require.InDelta(t, tt.want, pricing.OutputPricePerTokenPriority/pricing.OutputPricePerToken, 1e-12)
	}
}

func TestIntervalMultipliersApplyToChannelBase(t *testing.T) {
	base := &ModelPricing{
		InputPricePerToken:         5,
		OutputPricePerToken:        30,
		CacheCreationPricePerToken: 6.25,
		CacheCreation5mPrice:       6.25,
		CacheCreation1hPrice:       6.25,
		CacheReadPricePerToken:     0.5,
		FastMultiplier:             pricingMultiplier(2),
		FlexMultiplier:             pricingMultiplier(0.5),
	}
	resolved := &ResolvedPricing{
		BasePricing: base,
		Intervals: []PricingInterval{{
			MinTokens:            272000,
			InputMultiplier:      pricingMultiplier(2),
			OutputMultiplier:     pricingMultiplier(1.5),
			CacheWriteMultiplier: pricingMultiplier(2),
			CacheReadMultiplier:  pricingMultiplier(2),
		}},
	}

	pricing := (&ModelPricingResolver{}).GetIntervalPricing(resolved, 272001)
	require.InDelta(t, 10, pricing.InputPricePerToken, 1e-12)
	require.InDelta(t, 45, pricing.OutputPricePerToken, 1e-12)
	require.InDelta(t, 12.5, pricing.CacheCreationPricePerToken, 1e-12)
	require.InDelta(t, 1, pricing.CacheReadPricePerToken, 1e-12)
	require.Same(t, base, (&ModelPricingResolver{}).GetIntervalPricing(resolved, 272000))
}

func TestIntervalExplicitPriceTakesPrecedenceOverMultiplier(t *testing.T) {
	pricing := intervalToModelPricing(&PricingInterval{
		InputPrice:      pricingMultiplier(7),
		InputMultiplier: pricingMultiplier(2),
	}, &ModelPricing{InputPricePerToken: 5}, nil)

	require.InDelta(t, 7, pricing.InputPricePerToken, 1e-12)
}

func TestIntervalPricePreservesDefaultFastRatio(t *testing.T) {
	pricing := intervalToModelPricing(&PricingInterval{
		InputPrice: pricingMultiplier(7),
	}, &ModelPricing{
		InputPricePerToken:         5,
		InputPricePerTokenPriority: 10,
	}, nil)

	require.InDelta(t, 7, pricing.InputPricePerToken, 1e-12)
	require.InDelta(t, 14, pricing.InputPricePerTokenPriority, 1e-12)
}

func TestAnthropicSpeedServiceTier(t *testing.T) {
	account := &Account{Platform: PlatformAnthropic}

	for _, model := range []string{"claude-opus-5", "claude-opus-4-8", "claude-opus-4.8"} {
		tier := anthropicSpeedServiceTier(account, "fast", model)
		require.NotNil(t, tier, "model %s should bill as fast", model)
		require.Equal(t, "fast", *tier)
	}

	require.Nil(t, anthropicSpeedServiceTier(&Account{Platform: PlatformOpenAI}, "fast", "claude-opus-5"))
	require.Nil(t, anthropicSpeedServiceTier(account, "standard", "claude-opus-5"))
}

// fast mode 不存在于这些模型/承载上，即便客户端传了 speed=fast 也不能计 2x。
func TestAnthropicSpeedServiceTierRejectsUnsupportedTargets(t *testing.T) {
	account := &Account{Platform: PlatformAnthropic}

	for _, model := range []string{
		"claude-opus-4-7",  // fast mode 已被移除
		"claude-opus-4-6",  //
		"claude-opus-4-5",  // 不能被 "opus-5" 规则误判
		"claude-sonnet-5",  // 非 Opus
		"claude-haiku-4-5", //
		"",                 //
	} {
		require.Nil(t, anthropicSpeedServiceTier(account, "fast", model),
			"model %q must not bill as fast", model)
	}

	bedrock := &Account{Platform: PlatformAnthropic, Type: AccountTypeBedrock}
	require.Nil(t, anthropicSpeedServiceTier(bedrock, "fast", "claude-opus-5"))
}

func TestAnthropicSpeedModelPrefersMappedUpstreamModel(t *testing.T) {
	parsed := &ParsedRequest{Model: "claude-opus-5"}
	require.Equal(t, "claude-opus-4-7", anthropicSpeedModel(parsed, &ForwardResult{
		UpstreamModel: "claude-opus-4-7",
	}))
	require.Equal(t, "claude-opus-5", anthropicSpeedModel(parsed, &ForwardResult{}))
}

func TestMultiplierOnlyIntervalIsValid(t *testing.T) {
	require.NoError(t, ValidateIntervals([]PricingInterval{{
		MinTokens:       199999,
		InputMultiplier: pricingMultiplier(2),
	}}, BillingModeToken))
	require.NoError(t, checkIntervalsHavePrices(ChannelModelPricing{
		Models: []string{"grok-4.6"},
		Intervals: []PricingInterval{{
			MinTokens:       199999,
			InputMultiplier: pricingMultiplier(2),
		}},
	}))
}

func TestChannelMultipliersMustBePositive(t *testing.T) {
	zero := 0.0
	require.Error(t, checkPricesNotNegative(ChannelModelPricing{FastMultiplier: &zero}))
	require.Error(t, checkPricesNotNegative(ChannelModelPricing{FlexMultiplier: &zero}))
	require.Error(t, ValidateIntervals([]PricingInterval{{
		MinTokens:       100,
		InputMultiplier: &zero,
	}}, BillingModeToken))
}

func TestCalculateTokenCostContextTierEnablement(t *testing.T) {
	base := &ModelPricing{InputPricePerToken: 1e-6}
	resolved := &ResolvedPricing{
		BasePricing: base,
		Intervals: []PricingInterval{{
			MinTokens:       100,
			InputMultiplier: pricingMultiplier(2),
		}},
	}
	resolver := &ModelPricingResolver{}
	service := &BillingService{}
	tokens := UsageTokens{InputTokens: 200}

	t.Run("group disabled uses base tier", func(t *testing.T) {
		resolved.longContextPricingEnabled = false
		cost, err := service.calculateTokenCost(resolved, CostInput{
			Model: "custom", Tokens: tokens, RateMultiplier: 1, Resolver: resolver,
		})
		require.NoError(t, err)
		require.InDelta(t, 200e-6, cost.TotalCost, 1e-12)
	})

	t.Run("group enabled uses interval", func(t *testing.T) {
		resolved.longContextPricingEnabled = true
		accountDisabled := false
		cost, err := service.calculateTokenCost(resolved, CostInput{
			Model: "custom", Tokens: tokens, RateMultiplier: 1, Resolver: resolver,
			LongContextBillingEnabled: &accountDisabled,
		})
		require.NoError(t, err)
		require.InDelta(t, 400e-6, cost.TotalCost, 1e-12)
	})

	t.Run("account enabled overrides disabled group", func(t *testing.T) {
		resolved.longContextPricingEnabled = false
		accountEnabled := true
		cost, err := service.calculateTokenCost(resolved, CostInput{
			Model: "custom", Tokens: tokens, RateMultiplier: 1, Resolver: resolver,
			LongContextBillingEnabled: &accountEnabled,
		})
		require.NoError(t, err)
		require.InDelta(t, 400e-6, cost.TotalCost, 1e-12)
	})
}

func TestCalculateTokenCostCombinesIntervalAndFastMultiplier(t *testing.T) {
	resolved := &ResolvedPricing{
		BasePricing: &ModelPricing{
			InputPricePerToken: 1e-6,
			FastMultiplier:     pricingMultiplier(2.5),
		},
		Intervals: []PricingInterval{{
			MinTokens:       100,
			InputMultiplier: pricingMultiplier(2),
		}},
		longContextPricingEnabled: true,
	}
	cost, err := (&BillingService{}).calculateTokenCost(resolved, CostInput{
		Model: "custom", Tokens: UsageTokens{InputTokens: 200}, RateMultiplier: 1,
		ServiceTier: "fast", Resolver: &ModelPricingResolver{},
	})
	require.NoError(t, err)
	require.InDelta(t, 1e-3, cost.TotalCost, 1e-12)
}
