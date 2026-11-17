//go:build integration

package repository

import (
	"context"
	"math"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func newReasoningPricingIntegrationChannel(t *testing.T, channel *service.Channel) *channelRepository {
	t.Helper()
	repo := &channelRepository{db: integrationDB}
	channel.Name = t.Name()
	channel.Status = service.StatusActive
	channel.BillingModelSource = service.BillingModelSourceRequested
	require.NoError(t, repo.Create(context.Background(), channel))
	// Channel writes manage their own transactions, so clean up committed fixtures.
	t.Cleanup(func() {
		require.NoError(t, repo.Delete(context.Background(), channel.ID))
	})
	return repo
}

func requireReasoningPricingIntegrationMaps(t *testing.T, pricing []service.ChannelModelPricing, expected map[string]map[string]float64) {
	t.Helper()
	require.Len(t, pricing, len(expected))
	for _, entry := range pricing {
		require.Len(t, entry.Models, 1)
		multipliers, ok := expected[entry.Models[0]]
		require.True(t, ok, "unexpected model %s", entry.Models[0])
		if len(multipliers) == 0 {
			require.Empty(t, entry.ReasoningEffortMultipliers, entry.Models[0])
		} else {
			require.Equal(t, multipliers, entry.ReasoningEffortMultipliers, entry.Models[0])
		}
	}
}

func TestChannelReasoningPricingCreateUpdateReplace(t *testing.T) {
	ctx := context.Background()
	channel := &service.Channel{}
	repo := newReasoningPricingIntegrationChannel(t, channel)
	allLevels := map[string]float64{"none": 0.5, "minimal": 0.75, "low": 0.9, "medium": 1.25, "high": 1.5, "xhigh": 2, "max": 3}
	pricing := service.ChannelModelPricing{
		ChannelID: channel.ID, Platform: service.PlatformOpenAI, Models: []string{"custom-reasoning-model"},
		ReasoningEffortMultipliers: allLevels,
	}
	require.NoError(t, repo.CreateModelPricing(ctx, &pricing))
	require.NotZero(t, pricing.ID)
	assertStored := func(expected map[string]map[string]float64) {
		t.Helper()
		listed, err := repo.ListModelPricing(ctx, channel.ID)
		require.NoError(t, err)
		requireReasoningPricingIntegrationMaps(t, listed, expected)
		// Cache refresh uses this batch loading path instead of ListModelPricing.
		batch, err := repo.batchLoadModelPricing(ctx, []int64{channel.ID})
		require.NoError(t, err)
		requireReasoningPricingIntegrationMaps(t, batch[channel.ID], expected)
	}
	assertStored(map[string]map[string]float64{"custom-reasoning-model": allLevels})

	pricing.ReasoningEffortMultipliers = map[string]float64{"high": 2.25}
	require.NoError(t, repo.UpdateModelPricing(ctx, &pricing))
	assertStored(map[string]map[string]float64{"custom-reasoning-model": {"high": 2.25}})

	pricing.ReasoningEffortMultipliers = nil
	require.NoError(t, repo.UpdateModelPricing(ctx, &pricing))
	assertStored(map[string]map[string]float64{"custom-reasoning-model": nil})
	var stored string
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		`SELECT reasoning_effort_multipliers::text FROM channel_model_pricing WHERE id = $1`, pricing.ID).Scan(&stored))
	require.JSONEq(t, `{}`, stored)

	replacement := []service.ChannelModelPricing{
		{Platform: service.PlatformOpenAI, Models: []string{"custom-reasoning-model"}, ReasoningEffortMultipliers: map[string]float64{"low": 0.8, "max": 4}},
		{Platform: service.PlatformAnthropic, Models: []string{"claude-fable-5-1"}, ReasoningEffortMultipliers: map[string]float64{}},
	}
	require.NoError(t, repo.ReplaceModelPricing(ctx, channel.ID, replacement))
	expected := map[string]map[string]float64{"custom-reasoning-model": {"low": 0.8, "max": 4}, "claude-fable-5-1": nil}
	assertStored(expected)

	// A serialization failure after deleting the old rows must roll back the replacement.
	require.Error(t, repo.ReplaceModelPricing(ctx, channel.ID, []service.ChannelModelPricing{{
		Models: []string{"invalid"}, ReasoningEffortMultipliers: map[string]float64{"max": math.Inf(1)},
	}}))
	assertStored(expected)
}

func TestAccountStatsReasoningPricingRoundTrip(t *testing.T) {
	ctx := context.Background()
	channel := &service.Channel{
		ModelPricing: []service.ChannelModelPricing{{
			Platform: service.PlatformOpenAI, Models: []string{"custom-reasoning-model"},
			ReasoningEffortMultipliers: map[string]float64{"max": 4},
		}},
		AccountStatsPricingRules: []service.AccountStatsPricingRule{{
			Name: "independent account statistics", GroupIDs: []int64{}, AccountIDs: []int64{},
			Pricing: []service.ChannelModelPricing{
				{Platform: service.PlatformOpenAI, Models: []string{"custom-reasoning-model"}, ReasoningEffortMultipliers: map[string]float64{"none": 0.5, "high": 1.25, "max": 2.5}},
				{Platform: service.PlatformAnthropic, Models: []string{"claude-fable-5-1"}},
			},
		}},
	}
	repo := newReasoningPricingIntegrationChannel(t, channel)
	assertStored := func(multipliers map[string]float64) *service.Channel {
		t.Helper()
		loaded, err := repo.GetByID(ctx, channel.ID)
		require.NoError(t, err)
		require.Len(t, loaded.AccountStatsPricingRules, 1)
		expected := map[string]map[string]float64{"custom-reasoning-model": multipliers, "claude-fable-5-1": nil}
		requireReasoningPricingIntegrationMaps(t, loaded.AccountStatsPricingRules[0].Pricing, expected)
		batch, err := repo.batchLoadAccountStatsPricingRules(ctx, []int64{channel.ID})
		require.NoError(t, err)
		require.Len(t, batch[channel.ID], 1)
		requireReasoningPricingIntegrationMaps(t, batch[channel.ID][0].Pricing, expected)
		// Custom statistics prices must not alter the user-facing channel prices.
		requireReasoningPricingIntegrationMaps(t, loaded.ModelPricing, map[string]map[string]float64{"custom-reasoning-model": {"max": 4}})
		return loaded
	}
	loaded := assertStored(map[string]float64{"none": 0.5, "high": 1.25, "max": 2.5})
	for _, multipliers := range []map[string]float64{{"low": 0.75, "max": 1.5}, nil, {"high": 2}, {}} {
		loaded.AccountStatsPricingRules[0].Pricing[0].ReasoningEffortMultipliers = multipliers
		require.NoError(t, repo.Update(ctx, loaded))
		loaded = assertStored(multipliers)
	}
	var stored string
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		`SELECT reasoning_effort_multipliers::text FROM channel_account_stats_model_pricing WHERE id = $1`, loaded.AccountStatsPricingRules[0].Pricing[0].ID).Scan(&stored))
	require.JSONEq(t, `{}`, stored)
}

func TestGroupReasoningPricingRoundTripAndBilling(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	repo := newGroupRepositoryWithSQL(tx.Client(), tx)
	inputPrice, outputPrice := 1e-6, 2e-6
	allLevels := map[string]float64{"none": 0.5, "minimal": 0.75, "low": 0.9, "medium": 1.25, "high": 1.5, "xhigh": 2, "max": 3}
	group := &service.Group{
		Name: t.Name(), Platform: service.PlatformAnthropic, RateMultiplier: 0.8,
		Status: service.StatusActive, SubscriptionType: service.SubscriptionTypeStandard,
		ModelPricing: []service.ChannelModelPricing{{
			Platform: service.PlatformAnthropic, Models: []string{"claude-fable-5-1"}, BillingMode: service.BillingModeToken,
			InputPrice: &inputPrice, OutputPrice: &outputPrice, ReasoningEffortMultipliers: allLevels,
		}},
	}
	require.NoError(t, repo.Create(ctx, group))
	billing := service.NewBillingService(&config.Config{}, nil)
	resolver := service.NewModelPricingResolver(nil, billing)
	assertStoredAndBilled := func(expected map[string]float64) *service.Group {
		t.Helper()
		loaded, err := repo.GetByID(ctx, group.ID)
		require.NoError(t, err)
		requireReasoningPricingIntegrationMaps(t, loaded.ModelPricing, map[string]map[string]float64{"claude-fable-5-1": expected})
		for _, effort := range []string{"none", "minimal", "low", "medium", "high", "xhigh", "max", ""} {
			multiplier := 1.0
			if configured, ok := expected[effort]; ok {
				multiplier = configured
			}
			cost, err := billing.CalculateTokenCostForRequest(service.TokenCostRequest{
				Ctx: ctx, Model: "claude-fable-5-1", Group: loaded, Resolver: resolver,
				Tokens:         service.UsageTokens{InputTokens: 1000, OutputTokens: 200},
				RateMultiplier: loaded.RateMultiplier, ReasoningEffort: effort,
			})
			require.NoError(t, err)
			require.InDelta(t, 0.0014*multiplier, cost.TotalCost, 1e-12, "effort=%q", effort)
			require.InDelta(t, 0.0014*multiplier*0.8, cost.ActualCost, 1e-12, "effort=%q", effort)
		}
		return loaded
	}
	loaded := assertStoredAndBilled(allLevels)
	for _, multipliers := range []map[string]float64{{"high": 2.25, "max": 4}, nil, {"high": 1.75}, {}} {
		loaded.ModelPricing[0].ReasoningEffortMultipliers = multipliers
		require.NoError(t, repo.Update(ctx, loaded))
		loaded = assertStoredAndBilled(multipliers)
	}
}
