package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenAIWSNativeReplayConfig(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		resetViperWithJWTSecret(t)
		cfg, err := Load()
		require.NoError(t, err)
		require.EqualValues(t, 64<<20, cfg.Gateway.OpenAIWS.NativeReplayMaxSessionBytes)
		require.Equal(t, 32768, cfg.Gateway.OpenAIWS.NativeReplayMaxItems)
	})
	for _, key := range []string{"SESSION_BYTES", "ITEMS"} {
		t.Run(key, func(t *testing.T) {
			resetViperWithJWTSecret(t)
			t.Setenv("GATEWAY_OPENAI_WS_NATIVE_REPLAY_MAX_"+key, "0")
			_, err := Load()
			require.NoError(t, err, "zero disables caching")
			t.Setenv("GATEWAY_OPENAI_WS_NATIVE_REPLAY_MAX_"+key, "-1")
			_, err = Load()
			require.ErrorContains(t, err, "native replay budgets")
		})
	}
}
