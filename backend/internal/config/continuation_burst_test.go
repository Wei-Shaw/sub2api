package config

import (
	"strconv"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestContinuationBurstLimitConfig(t *testing.T) {
	for _, value := range []int{-1, 0, 2, 5} {
		t.Run(strconv.Itoa(value), func(t *testing.T) {
			resetViperWithJWTSecret(t)
			viper.Set("gateway.scheduling.continuation_burst_limit", value)
			cfg, err := Load()
			if value < 0 {
				require.ErrorContains(t, err, "gateway.scheduling.continuation_burst_limit must be non-negative")
				return
			}
			require.NoError(t, err)
			require.Equal(t, value, cfg.Gateway.Scheduling.ContinuationBurstLimit)
		})
	}
	t.Run("default", func(t *testing.T) {
		resetViperWithJWTSecret(t)
		cfg, err := Load()
		require.NoError(t, err)
		require.Equal(t, 2, cfg.Gateway.Scheduling.ContinuationBurstLimit)
	})
}
