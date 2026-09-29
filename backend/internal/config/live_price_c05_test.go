//go:build unit

package config

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadDefaultLivePricePerMinuteIsFree_C05(t *testing.T) {
	resetViperWithJWTSecret(t)

	cfg, err := Load()
	require.NoError(t, err)
	require.Zero(t, cfg.Gateway.Live.PricePerMinuteUSD)
}

func TestLoadLivePricePerMinuteFromEnv_C05(t *testing.T) {
	resetViperWithJWTSecret(t)
	t.Setenv("GATEWAY_LIVE_PRICE_PER_MINUTE_USD", "0.25")

	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, 0.25, cfg.Gateway.Live.PricePerMinuteUSD)
}

func TestValidateLivePricePerMinuteRejectsNegativeAndNonFinite_C05(t *testing.T) {
	resetViperWithJWTSecret(t)
	cfg, err := Load()
	require.NoError(t, err)

	for _, price := range []float64{-0.01, math.NaN(), math.Inf(1)} {
		cfg.Gateway.Live.PricePerMinuteUSD = price
		err := cfg.Validate()
		require.Error(t, err, "price=%v", price)
		require.Contains(t, err.Error(), "gateway.live.price_per_minute_usd")
	}

	cfg.Gateway.Live.PricePerMinuteUSD = 0.5
	require.NoError(t, cfg.Validate())
}
