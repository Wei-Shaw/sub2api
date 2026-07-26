package service

import (
	"context"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func perRequestPricingExtra(enabled bool, prices map[string]float64) map[string]any {
	modelPrices := make(map[string]any, len(prices))
	for model, price := range prices {
		modelPrices[model] = price
	}
	return map[string]any{
		AccountPerRequestPricingExtraKey: map[string]any{
			"enabled":      enabled,
			"model_prices": modelPrices,
		},
	}
}

func TestAccountPerRequestPricingAllowsConfiguredZeroPrice(t *testing.T) {
	account := &Account{Extra: perRequestPricingExtra(true, map[string]float64{" GPT-5.4 ": 0})}

	price, ok := account.AccountPerRequestPrice("gpt-5.4")
	require.True(t, ok)
	require.Equal(t, 0.0, price)
	require.True(t, (&GatewayService{}).isAccountPerRequestModelPriced(account, "GPT-5.4"))
	require.False(t, (&GatewayService{}).isAccountPerRequestModelPriced(account, "gpt-5.3"))
}

func TestValidateAccountPerRequestPricingExtra(t *testing.T) {
	tests := []struct {
		name  string
		extra map[string]any
		valid bool
	}{
		{name: "disabled may keep no rows", extra: perRequestPricingExtra(false, nil), valid: true},
		{name: "enabled needs rows", extra: perRequestPricingExtra(true, nil), valid: false},
		{name: "negative price", extra: perRequestPricingExtra(true, map[string]float64{"gpt-5.4": -1}), valid: false},
		{name: "non finite price", extra: perRequestPricingExtra(true, map[string]float64{"gpt-5.4": math.Inf(1)}), valid: false},
		{name: "zero price", extra: perRequestPricingExtra(true, map[string]float64{"gpt-5.4": 0}), valid: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAccountPerRequestPricingExtra(tt.extra)
			if tt.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestResolveAccountStatsCostUsesAccountPerRequestPriceBeforeChannelRules(t *testing.T) {
	account := &Account{ID: 42, Extra: perRequestPricingExtra(true, map[string]float64{"gpt-5.4": 0.07})}
	channel := &Channel{
		ID:     1,
		Status: StatusActive,
		AccountStatsPricingRules: []AccountStatsPricingRule{{
			AccountIDs: []int64{42},
			Pricing: []ChannelModelPricing{{
				Models:     []string{"gpt-5.4"},
				BillingMode: BillingModePerRequest,
				PerRequestPrice: func() *float64 {
					price := 99.0
					return &price
				}(),
			}},
		}},
	}
	cs := newTestChannelServiceForStats(t, channel, 10, "openai")

	cost := resolveAccountStatsCost(context.Background(), cs, nil, account.ID, 10, "gpt-5.4", UsageTokens{}, 1, 99, account)
	require.NotNil(t, cost)
	require.InDelta(t, 0.07, *cost, 1e-12)
}

