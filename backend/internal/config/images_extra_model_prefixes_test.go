package config

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestLoadOpenAIImagesExtraModelPrefixesFromEnvironment(t *testing.T) {
	resetViperWithJWTSecret(t)
	t.Setenv("GATEWAY_OPENAI_IMAGES_EXTRA_MODEL_PREFIXES", "gemini-nano-banana-, image2-pro")

	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, []string{"gemini-nano-banana-", "image2-pro"}, cfg.Gateway.OpenAIImagesExtraModelPrefixes)
}

func TestLoadOpenAIImagesExtraModelPrefixesDefaultEmpty(t *testing.T) {
	resetViperWithJWTSecret(t)

	cfg, err := Load()
	require.NoError(t, err)
	require.Empty(t, cfg.Gateway.OpenAIImagesExtraModelPrefixes)
}

func TestLoadOpenAIImagesExtraModelPrefixesFromYAML(t *testing.T) {
	resetViperWithJWTSecret(t)
	viper.Set("gateway.openai_images_extra_model_prefixes", []string{"gemini-nano-banana-"})

	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, []string{"gemini-nano-banana-"}, cfg.Gateway.OpenAIImagesExtraModelPrefixes)
}
