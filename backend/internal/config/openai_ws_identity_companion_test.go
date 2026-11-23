package config

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenAIWSIdentityCompanionRequiresExactPackageAndScope(t *testing.T) {
	resetViperWithJWTSecret(t)
	cfg, err := Load()
	require.NoError(t, err)
	require.False(t, cfg.Plugins.OpenAIWSIdentityCompanionConfigured())
	cfg.Plugins.OpenAIWSIdentityCompanionPackage = "/tmp/candidate-only/native.s2plugin"
	require.ErrorContains(t, cfg.Validate(), "require an absolute path and its lowercase SHA-256")
	cfg.Plugins.OpenAIWSIdentityCompanionSHA256 = strings.Repeat("a", 64)
	require.ErrorContains(t, cfg.Validate(), "requires an explicit account and API key canary")
	cfg.Plugins.OpenAIWSIdentityCanaryAccountID = 901
	cfg.Plugins.OpenAIWSIdentityCanaryAPIKeyID = 101
	require.NoError(t, cfg.Validate())
	require.False(t, cfg.Plugins.AllowsOpenAIWSIdentity(901, 101), "a configured archive does not enable native WS")
	cfg.Plugins.OpenAIWSIdentityCompanionPackage = "relative/native.s2plugin"
	require.ErrorContains(t, cfg.Validate(), "absolute path")
}
