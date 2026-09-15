//go:build unit

package config

import (
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestStrictConfigIntRejectsFractionalAndOverflow(t *testing.T) {
	for _, tc := range []struct {
		name    string
		value   any
		want    int
		wantErr bool
	}{
		{"int", 20, 20, false},
		{"int64", int64(30), 30, false},
		{"float whole", float64(20), 20, false},
		{"string", "42", 42, false},
		{"string spaces", " 7 ", 7, false},
		{"fraction string", "20.5", 0, true},
		{"fraction float", 1.5, 0, true},
		{"empty string", "", 0, true},
		{"alpha string", "twenty", 0, true},
		{"negative string", "-1", -1, false},
		{"overflow string", "9223372036854775808", 0, true},
		{"unsupported", []int{1}, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := strictConfigInt(tc.value)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestLoadAPIKeyQueueConfigDefaultsAndValidation(t *testing.T) {
	load := func(t *testing.T, maxWaiting, seconds string) (APIKeyQueueConfig, error) {
		t.Helper()
		viper.Reset()
		t.Cleanup(viper.Reset)
		setDefaults()
		if maxWaiting != "" {
			viper.Set("gateway.api_key_queue.max_waiting", maxWaiting)
		}
		if seconds != "" {
			viper.Set("gateway.api_key_queue.timeout_seconds", seconds)
		}
		return loadAPIKeyQueueConfig()
	}

	t.Run("defaults", func(t *testing.T) {
		cfg, err := load(t, "", "")
		require.NoError(t, err)
		require.Equal(t, 20, cfg.MaxWaiting)
		require.Equal(t, 30, cfg.TimeoutSeconds)
	})

	t.Run("explicit zero waiting is preserved", func(t *testing.T) {
		cfg, err := load(t, "0", "30")
		require.NoError(t, err)
		require.Zero(t, cfg.MaxWaiting)
	})

	t.Run("negative waiting rejected", func(t *testing.T) {
		_, err := load(t, "-1", "30")
		require.ErrorContains(t, err, "non-negative")
	})

	t.Run("zero timeout rejected", func(t *testing.T) {
		_, err := load(t, "20", "0")
		require.ErrorContains(t, err, "positive")
	})

	t.Run("fraction rejected", func(t *testing.T) {
		_, err := load(t, "20.5", "30")
		require.ErrorContains(t, err, "integer")
	})

	t.Run("invalid string rejected", func(t *testing.T) {
		_, err := load(t, "20", "thirty")
		require.ErrorContains(t, err, "integer")
	})

	t.Run("overflow seconds rejected", func(t *testing.T) {
		_, err := load(t, "20", "9223372036854775807")
		require.ErrorContains(t, err, "duration range")
	})

	t.Run("env variable maps to key", func(t *testing.T) {
		viper.Reset()
		t.Cleanup(viper.Reset)
		t.Setenv("GATEWAY_API_KEY_QUEUE_MAX_WAITING", "5")
		t.Setenv("GATEWAY_API_KEY_QUEUE_TIMEOUT_SECONDS", "10")
		viper.AutomaticEnv()
		viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
		setDefaults()
		cfg, err := loadAPIKeyQueueConfig()
		require.NoError(t, err)
		require.Equal(t, 5, cfg.MaxWaiting)
		require.Equal(t, 10, cfg.TimeoutSeconds)
		require.Equal(t, 10, int(cfg.Timeout()/time.Second))
	})
}
