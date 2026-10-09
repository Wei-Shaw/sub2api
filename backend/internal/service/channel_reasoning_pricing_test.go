//go:build unit

package service

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidatePricingEntries_ReasoningEffortMultipliers(t *testing.T) {
	for _, tc := range []struct {
		name        string
		multipliers map[string]float64
		valid       bool
	}{
		{"unset", nil, true},
		{"cleared", map[string]float64{}, true},
		{"all levels", map[string]float64{"none": 0.5, "minimal": 1, "low": 1.25, "medium": 1.5, "high": 2, "xhigh": 2.5, "max": 3}, true},
		{"unknown level", map[string]float64{"ultra": 2}, false},
		{"blank level", map[string]float64{"": 2}, false},
		{"noncanonical level", map[string]float64{"MAX": 2}, false},
		{"zero", map[string]float64{"high": 0}, false},
		{"negative", map[string]float64{"high": -1}, false},
		{"nan", map[string]float64{"high": math.NaN()}, false},
		{"infinity", map[string]float64{"high": math.Inf(1)}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pricing := []ChannelModelPricing{{Models: []string{"custom-model"}, ReasoningEffortMultipliers: tc.multipliers}}
			err := validatePricingEntries(pricing)
			statsErr := validateAccountStatsPricingRules([]AccountStatsPricingRule{{Pricing: pricing}})
			if tc.valid {
				require.NoError(t, err)
				require.NoError(t, statsErr)
			} else {
				require.Error(t, err)
				require.Error(t, statsErr)
			}
		})
	}
}
