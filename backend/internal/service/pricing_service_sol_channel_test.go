//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestSolDefaultMigrationChannelBilling(t *testing.T) {
	svc := initializedSolPricing(t, solDefaultURL, obsoleteSolCatalog(t, nil), "", true)
	billing := NewBillingService(&config.Config{}, svc)
	channel := ChannelModelPricing{
		Platform: PlatformOpenAI, Models: []string{"gpt-5.6-sol"}, BillingMode: BillingModeToken,
		InputPrice: testPtrFloat64(0), OutputPrice: testPtrFloat64(3e-6),
		CacheWritePrice: testPtrFloat64(0), CacheReadPrice: testPtrFloat64(0), FastMultiplier: testPtrFloat64(3),
		Intervals: []PricingInterval{
			{MinTokens: 0, MaxTokens: testPtrInt(100000), OutputPrice: testPtrFloat64(3e-6)},
			{MinTokens: 100000, OutputPrice: testPtrFloat64(6e-6)},
		},
	}
	repo := &mockChannelRepository{
		listAllFn: func(context.Context) ([]Channel, error) {
			return []Channel{{ID: 1, Status: StatusActive, GroupIDs: []int64{100}, ModelPricing: []ChannelModelPricing{channel}}}, nil
		},
		getGroupPlatformsFn: func(context.Context, []int64) (map[int64]string, error) {
			return map[int64]string{100: PlatformOpenAI}, nil
		},
	}
	resolver := NewModelPricingResolver(NewChannelService(repo, nil, nil, nil, nil), billing)
	for _, enabled := range []bool{false, true} {
		group := &Group{ID: 100, LongContextPricingEnabled: enabled}
		resolved := resolver.Resolve(context.Background(), PricingInput{Model: "gpt-5.6-sol", GroupID: &group.ID, Group: group})
		require.Equal(t, PricingSourceChannel, resolved.Source)
		cost, err := billing.CalculateCostUnified(CostInput{
			Ctx: context.Background(), Model: "gpt-5.6-sol", GroupID: &group.ID, Group: group,
			Tokens: UsageTokens{InputTokens: 300000, OutputTokens: 100}, RateMultiplier: 1,
			ServiceTier: "priority", Resolver: resolver,
		})
		require.NoError(t, err)
		want := 100 * 3e-6 * 3
		if enabled {
			want = 100 * 6e-6 * 3
		}
		require.InDelta(t, want, cost.ActualCost, 1e-12)
		group.ModelPricing = []ChannelModelPricing{{Models: []string{"gpt-5.6-sol"}, BillingMode: BillingModeToken,
			InputPrice: testPtrFloat64(0), OutputPrice: testPtrFloat64(1e-6), FastMultiplier: testPtrFloat64(0)}}
		resolved = resolver.Resolve(context.Background(), PricingInput{Model: "gpt-5.6-sol", GroupID: &group.ID, Group: group})
		require.Equal(t, PricingSourceGroup, resolved.Source)
		cost, err = billing.CalculateCostUnified(CostInput{
			Ctx: context.Background(), Model: "gpt-5.6-sol", Tokens: UsageTokens{InputTokens: 300000, OutputTokens: 100},
			RateMultiplier: 1, ServiceTier: "priority", Resolver: resolver, Resolved: resolved,
		})
		require.NoError(t, err)
		require.Zero(t, cost.ActualCost)
	}
}
