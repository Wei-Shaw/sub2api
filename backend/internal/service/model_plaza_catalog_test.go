//go:build unit

package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParsePlazaCatalog(t *testing.T) {
	catalog, err := parsePlazaCatalog("")
	require.NoError(t, err)
	require.Nil(t, catalog)
	catalog, err = parsePlazaCatalog(`[]`)
	require.NoError(t, err)
	require.NotNil(t, catalog)
	for _, raw := range []string{`null`, `{}`, `oops`, `[{"group_id":0}]`, `[{"group_id":1},{"group_id":1}]`, `[{"group_id":1,"models":["*"]}]`, `[{"group_id":1,"models":["x"],"image_models":["x"]}]`} {
		_, err := parsePlazaCatalog(raw)
		require.Error(t, err, raw)
	}
}

func TestPlazaCatalogFiltersGroupsWithoutChangingPricing(t *testing.T) {
	channels := []Channel{plazaPricedChannel(1, "Claude", []int64{10, 20}, PlatformAnthropic, "claude-sonnet-4-6")}
	channels[0].BillingModelSource = "channel_mapped"
	groups := []Group{
		{ID: 10, Platform: PlatformAnthropic, RateMultiplier: 0.1},
		{ID: 20, Platform: PlatformAnthropic, RateMultiplier: 0.75},
	}
	before, err := json.Marshal(channels)
	require.NoError(t, err)
	svc := newPlazaServiceWithBilling(channels, groups, map[int64]string{10: PlatformAnthropic, 20: PlatformAnthropic}, nil)
	out, err := svc.ListCatalogGroups(context.Background(), []PlazaCatalogGroup{{GroupID: 10}})
	require.NoError(t, err)
	require.Len(t, out, 1)
	require.Equal(t, int64(10), out[0].ID)
	require.InDelta(t, 0.1, out[0].RateMultiplier, 1e-12)
	require.InDelta(t, 3e-6, *out[0].Models[0].Pricing.InputPrice, 1e-12)
	after, err := json.Marshal(channels)
	require.NoError(t, err)
	require.JSONEq(t, string(before), string(after))
	out, err = svc.ListCatalogGroups(context.Background(), []PlazaCatalogGroup{})
	require.NoError(t, err)
	require.Empty(t, out)
}

func TestPlazaCatalogSupplementUsesActualBilling(t *testing.T) {
	channels := []Channel{plazaPricedChannel(1, "custom", []int64{10}, PlatformOpenAI, "gpt-5.5")}
	groups := []Group{{ID: 10, Platform: PlatformOpenAI, RateMultiplier: 0.068}}
	svc := newPlazaServiceWithBilling(channels, groups, map[int64]string{10: PlatformOpenAI}, nil)
	out, err := svc.ListCatalogGroups(context.Background(), []PlazaCatalogGroup{{GroupID: 10, Models: []string{"gpt-5.5", "gpt-5.6-sol"}}})
	require.NoError(t, err)
	require.Len(t, out, 1)
	require.Len(t, out[0].Models, 2)
	for _, model := range out[0].Models {
		cost, err := svc.billingService.CalculateTokenCostForRequest(TokenCostRequest{
			Ctx: context.Background(), Model: model.Name, Group: &groups[0],
			Tokens: UsageTokens{InputTokens: 1000}, RateMultiplier: groups[0].RateMultiplier, Resolver: svc.resolver,
		})
		require.NoError(t, err)
		require.InDelta(t, cost.ActualCost, *model.Pricing.InputPrice*1000*out[0].RateMultiplier, 1e-12)
	}
}

func TestPlazaCatalogImagesWithoutChannelUseGatewayCost(t *testing.T) {
	groups := []Group{{ID: 116, Name: "images", Platform: PlatformOpenAI,
		AllowImageGeneration: true, RateMultiplier: 0.3,
		ImagePrice1K: testPtrFloat64(0.08), ImagePrice2K: testPtrFloat64(0.12), ImagePrice4K: testPtrFloat64(0.2),
	}}
	svc := newPlazaServiceWithBilling(nil, groups, map[int64]string{116: PlatformOpenAI}, nil)
	catalog := []PlazaCatalogGroup{{GroupID: 116, Models: []string{}, ImageModels: []string{"gpt-image-2"}}}
	out, err := svc.ListCatalogGroups(context.Background(), catalog)
	require.NoError(t, err)
	require.Len(t, out, 1)
	require.Len(t, out[0].Models, 1)
	p := out[0].Models[0].Pricing
	require.Equal(t, BillingModeImage, p.BillingMode)
	require.Nil(t, p.InputPrice)
	require.Len(t, p.Intervals, 3)
	for i, want := range []float64{0.08, 0.12, 0.2} {
		require.InDelta(t, want, *p.Intervals[i].PerRequestPrice, 1e-12)
	}
	// A group model override has higher precedence than the legacy size prices.
	groups[0].ModelPricing = []ChannelModelPricing{{Models: []string{"gpt-image-2"}, BillingMode: BillingModePerRequest, PerRequestPrice: testPtrFloat64(0.04)}}
	out, err = svc.ListCatalogGroups(context.Background(), catalog)
	require.NoError(t, err)
	require.InDelta(t, 0.04, *out[0].Models[0].Pricing.Intervals[0].PerRequestPrice, 1e-12)
	groups[0].AllowImageGeneration = false
	out, err = svc.ListCatalogGroups(context.Background(), catalog)
	require.NoError(t, err)
	require.Empty(t, out)
}
