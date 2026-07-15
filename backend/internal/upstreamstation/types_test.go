package upstreamstation

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEffectiveRate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		groupRate          float64
		rechargeMultiplier float64
		want               float64
	}{
		{name: "divide by recharge multiplier", groupRate: 0.8, rechargeMultiplier: 2, want: 0.4},
		{name: "missing multiplier defaults to one", groupRate: 0.6, rechargeMultiplier: 0, want: 0.6},
		{name: "negative multiplier defaults to one", groupRate: 1.25, rechargeMultiplier: -3, want: 1.25},
		{name: "round to database precision", groupRate: 1, rechargeMultiplier: 3, want: 0.33333333},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, EffectiveRate(tt.groupRate, tt.rechargeMultiplier))
		})
	}
}

func TestManagedAPIKeyName(t *testing.T) {
	t.Parallel()

	require.Equal(t, "sub2api-auto-12-default", ManagedAPIKeyName(12, "Default / 低价组"))
	require.Equal(t, "sub2api-auto-9-group", ManagedAPIKeyName(9, "  "))
}
