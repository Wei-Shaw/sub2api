package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMatchTimePricingPeriod_UsesTimezoneAndHalfOpenRange(t *testing.T) {
	input := 0.01
	config := &TimePricingConfig{
		Enabled:  true,
		Timezone: "Asia/Shanghai",
		Periods: []TimePricingPeriod{{
			Name:       "business",
			StartTime:  "09:00",
			EndTime:    "18:00",
			Weekdays:   []int{1, 2, 3, 4, 5},
			InputPrice: &input,
		}},
	}

	loc, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	mondayAtNine := time.Date(2026, time.July, 13, 9, 0, 0, 0, loc)
	mondayAtSix := time.Date(2026, time.July, 13, 18, 0, 0, 0, loc)
	sundayAtTen := time.Date(2026, time.July, 12, 10, 0, 0, 0, loc)

	require.NotNil(t, MatchTimePricingPeriod(config, mondayAtNine))
	require.Nil(t, MatchTimePricingPeriod(config, mondayAtSix))
	require.Nil(t, MatchTimePricingPeriod(config, sundayAtTen))
}

func TestMatchTimePricingPeriod_OvernightUsesStartDay(t *testing.T) {
	config := &TimePricingConfig{
		Enabled:  true,
		Timezone: "UTC",
		Periods: []TimePricingPeriod{{
			Name:      "night",
			StartTime: "22:00",
			EndTime:   "02:00",
			Weekdays:  []int{1},
		}},
	}

	mondayNight := time.Date(2026, time.July, 13, 23, 0, 0, 0, time.UTC)
	tuesdayEarly := time.Date(2026, time.July, 14, 1, 59, 0, 0, time.UTC)
	tuesdayLate := time.Date(2026, time.July, 14, 2, 0, 0, 0, time.UTC)
	sundayEarly := time.Date(2026, time.July, 12, 1, 0, 0, 0, time.UTC)

	require.NotNil(t, MatchTimePricingPeriod(config, mondayNight))
	require.NotNil(t, MatchTimePricingPeriod(config, tuesdayEarly))
	require.Nil(t, MatchTimePricingPeriod(config, tuesdayLate))
	require.Nil(t, MatchTimePricingPeriod(config, sundayEarly))
}

func TestValidateTimePricing_RejectsOverlapAndInvalidTimezone(t *testing.T) {
	price := 0.01
	overlap := ChannelModelPricing{
		BillingMode: BillingModeToken,
		TimePricing: &TimePricingConfig{
			Enabled:  true,
			Timezone: "UTC",
			Periods: []TimePricingPeriod{
				{StartTime: "09:00", EndTime: "12:00", InputPrice: &price},
				{StartTime: "11:00", EndTime: "13:00", OutputPrice: &price},
			},
		},
	}
	require.Error(t, validateTimePricing(overlap))

	invalidZone := overlap
	invalidZone.TimePricing = &TimePricingConfig{
		Enabled:  true,
		Timezone: "Not/AZone",
		Periods:  []TimePricingPeriod{{StartTime: "09:00", EndTime: "10:00", InputPrice: &price}},
	}
	require.Error(t, validateTimePricing(invalidZone))
}

func TestApplyTimePricingToModelPricing_InheritsUnspecifiedFields(t *testing.T) {
	input := 0.01
	output := 0.02
	periodInput := 0.03
	pricing := ChannelModelPricing{
		InputPrice:  &input,
		OutputPrice: &output,
	}
	effective := applyTimePricingToChannelPricing(&pricing, &TimePricingPeriod{InputPrice: &periodInput})
	require.Equal(t, 0.03, *effective.InputPrice)
	require.Equal(t, 0.02, *effective.OutputPrice)
}

func TestCalculateTokenStatsCostAt_AppliesPeriodAfterIntervalSelection(t *testing.T) {
	baseInput := 0.01
	intervalInput := 0.02
	periodOutput := 0.04
	maxTokens := 1000
	pricing := &ChannelModelPricing{
		BillingMode: BillingModeToken,
		InputPrice:  &baseInput,
		Intervals: []PricingInterval{{
			MinTokens: 0,
			MaxTokens: &maxTokens,
			InputPrice: &intervalInput,
		}},
	}

	cost := calculateTokenStatsCostAt(
		pricing,
		UsageTokens{InputTokens: 100, OutputTokens: 100},
		&TimePricingPeriod{OutputPrice: &periodOutput},
	)

	require.NotNil(t, cost)
	require.Equal(t, 6.0, *cost)
}
