//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func newOpenAIResolverWithChannel(t *testing.T, bs *BillingService, pricing []ChannelModelPricing) *ModelPricingResolver {
	t.Helper()
	const groupID = 100
	repo := &mockChannelRepository{
		listAllFn: func(_ context.Context) ([]Channel, error) {
			return []Channel{{
				ID:           1,
				Name:         "ultrafast-channel",
				Status:       StatusActive,
				GroupIDs:     []int64{groupID},
				ModelPricing: pricing,
			}}, nil
		},
		getGroupPlatformsFn: func(_ context.Context, _ []int64) (map[int64]string, error) {
			return map[int64]string{groupID: PlatformOpenAI}, nil
		},
	}
	return NewModelPricingResolver(NewChannelService(repo, nil, nil, nil, nil), bs)
}

func TestConfiguredServiceTierMultiplierUltrafastPrecedence(t *testing.T) {
	require.InDelta(t, 4, configuredServiceTierMultiplier("ultrafast", &ModelPricing{
		UltrafastMultiplier:         3,
		UltrafastMultiplierOverride: pricingMultiplier(4),
	}), 1e-12)
	require.InDelta(t, 4, configuredServiceTierMultiplier(" UltraFast ", &ModelPricing{
		UltrafastMultiplierOverride: pricingMultiplier(4),
	}), 1e-12, "override also applies to models without a model-owned Ultrafast multiplier")
	require.InDelta(t, 3, configuredServiceTierMultiplier("ultrafast", &ModelPricing{UltrafastMultiplier: 3}), 1e-12)
	// The Ultrafast override must never leak into other tiers.
	override := &ModelPricing{UltrafastMultiplierOverride: pricingMultiplier(4)}
	require.InDelta(t, 2, configuredServiceTierMultiplier("priority", override), 1e-12)
	require.InDelta(t, 0.5, configuredServiceTierMultiplier("flex", override), 1e-12)
	require.InDelta(t, 1, configuredServiceTierMultiplier("", override), 1e-12)
}

func TestChannelUltrafastMultiplierBilling(t *testing.T) {
	const model = "gpt-6.2-astra"
	const unit = 8e-6
	before := time.Date(2027, 3, 1, 7, 59, 59, 0, time.UTC)
	after := before.Add(time.Second)
	groupID := int64(100)
	tokens := UsageTokens{InputTokens: 100, OutputTokens: 50}
	standard := 100*unit + 50*unit*5

	cost := func(t *testing.T, resolver *ModelPricingResolver, tier string, at time.Time) float64 {
		t.Helper()
		bd, err := resolver.billingService.CalculateCostUnified(CostInput{
			Ctx: context.Background(), Model: model, GroupID: &groupID, Tokens: tokens,
			RateMultiplier: 1, ServiceTier: tier, PricingAt: at, Resolver: resolver,
		})
		require.NoError(t, err)
		return bd.TotalCost
	}

	t.Run("unset keeps the model-owned dated multiplier", func(t *testing.T) {
		resolver := newOpenAIResolverWithChannel(t, newTestBillingService(), []ChannelModelPricing{{
			Platform: PlatformOpenAI, Models: []string{model}, BillingMode: BillingModeToken,
			InputPrice: testPtrFloat64(unit), OutputPrice: testPtrFloat64(unit * 5),
			FastMultiplier: pricingMultiplier(2.5),
		}})
		require.InDelta(t, standard*3, cost(t, resolver, "ultrafast", before), 1e-12)
		require.InDelta(t, standard*6, cost(t, resolver, "ultrafast", after), 1e-12)
		require.InDelta(t, standard*2.5, cost(t, resolver, "priority", before), 1e-12)
	})

	t.Run("configured multiplier replaces the model-owned one at every instant", func(t *testing.T) {
		resolver := newOpenAIResolverWithChannel(t, newTestBillingService(), []ChannelModelPricing{{
			Platform: PlatformOpenAI, Models: []string{model}, BillingMode: BillingModeToken,
			InputPrice: testPtrFloat64(unit), OutputPrice: testPtrFloat64(unit * 5),
			FastMultiplier: pricingMultiplier(2.5), UltrafastMultiplier: pricingMultiplier(4),
		}})
		require.InDelta(t, standard*4, cost(t, resolver, "ultrafast", before), 1e-12)
		require.InDelta(t, standard*4, cost(t, resolver, "ultrafast", after), 1e-12)
		require.InDelta(t, standard*2.5, cost(t, resolver, "priority", before), 1e-12)
		require.InDelta(t, standard*0.5, cost(t, resolver, "flex", before), 1e-12)
		require.InDelta(t, standard, cost(t, resolver, "", before), 1e-12)
	})

	t.Run("interval pricing keeps the configured multiplier", func(t *testing.T) {
		resolver := newOpenAIResolverWithChannel(t, newTestBillingService(), []ChannelModelPricing{{
			Platform: PlatformOpenAI, Models: []string{model}, BillingMode: BillingModeToken,
			InputPrice: testPtrFloat64(unit), OutputPrice: testPtrFloat64(unit * 5),
			UltrafastMultiplier: pricingMultiplier(4),
			Intervals: []PricingInterval{{
				MinTokens: 0, InputMultiplier: pricingMultiplier(2), OutputMultiplier: pricingMultiplier(2),
			}},
		}})
		require.InDelta(t, standard*2*4, cost(t, resolver, "ultrafast", after), 1e-12)
	})
}

func TestGetModelPricingWithChannelCarriesUltrafastMultiplier(t *testing.T) {
	svc := newTestBillingService()
	pricing, err := svc.GetModelPricingWithChannel("gpt-6.2-sol", &ChannelModelPricing{UltrafastMultiplier: pricingMultiplier(5)})
	require.NoError(t, err)
	require.NotNil(t, pricing.UltrafastMultiplierOverride)
	require.InDelta(t, 5, *pricing.UltrafastMultiplierOverride, 1e-12)

	bd, err := svc.calculateCostInternal("gpt-6.2-sol", UsageTokens{InputTokens: 1000}, 1, "ultrafast", &ChannelModelPricing{UltrafastMultiplier: pricingMultiplier(5)})
	require.NoError(t, err)
	require.InDelta(t, 1000*2e-6*5, bd.InputCost, 1e-12)

	// Shared fallback pricing must not be mutated by a channel override.
	require.Nil(t, svc.fallbackPrices["gpt-6.2-sol"].UltrafastMultiplierOverride)
}

func TestChannelUltrafastMultiplierMustBePositive(t *testing.T) {
	zero := 0.0
	negative := -1.0
	require.Error(t, checkPricesNotNegative(ChannelModelPricing{UltrafastMultiplier: &zero}))
	require.Error(t, checkPricesNotNegative(ChannelModelPricing{UltrafastMultiplier: &negative}))
	require.NoError(t, checkPricesNotNegative(ChannelModelPricing{UltrafastMultiplier: pricingMultiplier(3)}))
}
