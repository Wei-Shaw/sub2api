package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadOpenAIRequestCompressionDefaults(t *testing.T) {
	resetViperWithJWTSecret(t)

	cfg, err := Load()
	require.NoError(t, err)
	require.False(t, cfg.Gateway.OpenAIRequestCompression.Enabled)
	require.True(t, cfg.Gateway.OpenAIRequestCompression.FallbackUncompressed)
}

func TestLoadOpenAIRequestCompressionFromEnvironment(t *testing.T) {
	resetViperWithJWTSecret(t)
	t.Setenv("GATEWAY_OPENAI_REQUEST_COMPRESSION_ENABLED", "true")
	t.Setenv("GATEWAY_OPENAI_REQUEST_COMPRESSION_FALLBACK_UNCOMPRESSED", "false")

	cfg, err := Load()
	require.NoError(t, err)
	require.True(t, cfg.Gateway.OpenAIRequestCompression.Enabled)
	require.False(t, cfg.Gateway.OpenAIRequestCompression.FallbackUncompressed)
}
