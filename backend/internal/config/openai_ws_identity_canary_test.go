package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenAIWSIdentityCanaryLoadsExactPairFromEnv(t *testing.T) {
	resetViperWithJWTSecret(t)
	t.Setenv("PLUGINS_OPENAI_WS_IDENTITY_ENABLED", "true")
	t.Setenv("PLUGINS_OPENAI_WS_IDENTITY_CANARY_ACCOUNT_ID", "901")
	t.Setenv("PLUGINS_OPENAI_WS_IDENTITY_CANARY_API_KEY_ID", "101")
	cfg, err := Load()
	require.NoError(t, err)
	require.True(t, cfg.Plugins.AllowsOpenAIWSIdentity(901, 101))
	require.False(t, cfg.Plugins.AllowsOpenAIWSIdentity(901, 102))
	require.False(t, cfg.Plugins.AllowsOpenAIWSIdentity(902, 101))
	require.False(t, cfg.Plugins.AllowsOpenAIWSIdentity(901, 0))
	cfg.Plugins.OpenAIWSIdentityEnabled = false
	require.False(t, cfg.Plugins.AllowsOpenAIWSIdentity(901, 101))
}

func TestOpenAIWSIdentityCanaryRejectsIncompleteScope(t *testing.T) {
	for _, tc := range []struct{ account, key int64 }{{901, 0}, {0, 101}, {-1, 101}, {901, -1}} {
		resetViperWithJWTSecret(t)
		cfg, err := Load()
		require.NoError(t, err)
		cfg.Plugins.OpenAIWSIdentityEnabled = true
		cfg.Plugins.OpenAIWSIdentityCanaryAccountID = tc.account
		cfg.Plugins.OpenAIWSIdentityCanaryAPIKeyID = tc.key
		require.ErrorContains(t, cfg.Validate(), "must both be positive")
		require.False(t, cfg.Plugins.AllowsOpenAIWSIdentity(901, 101))
	}
}

func TestOpenAIWSIdentityCanaryUnsetPreservesOptIn(t *testing.T) {
	resetViperWithJWTSecret(t)
	cfg, err := Load()
	require.NoError(t, err)
	require.Zero(t, cfg.Plugins.OpenAIWSIdentityCanaryAccountID)
	require.Zero(t, cfg.Plugins.OpenAIWSIdentityCanaryAPIKeyID)
	require.False(t, cfg.Plugins.AllowsOpenAIWSIdentity(901, 101))
	cfg.Plugins.OpenAIWSIdentityEnabled = true
	require.True(t, cfg.Plugins.AllowsOpenAIWSIdentity(901, 101))
}
