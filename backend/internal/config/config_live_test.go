package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadLiveAuthenticationModeRequiresExplicitOptIn(t *testing.T) {
	resetViperWithJWTSecret(t)
	cfg, err := Load()
	require.NoError(t, err)
	mode, err := cfg.Gateway.Live.EffectiveUpstreamAuthMode()
	require.NoError(t, err)
	require.Equal(t, LiveUpstreamAuthModeDeviceCheck, mode)

	t.Setenv("GATEWAY_LIVE_UPSTREAM_AUTH_MODE", "codex_oauth")
	cfg, err = Load()
	require.NoError(t, err)
	mode, err = cfg.Gateway.Live.EffectiveUpstreamAuthMode()
	require.NoError(t, err)
	require.Equal(t, LiveUpstreamAuthModeCodexOAuth, mode)
}

func TestLoadLiveAuthenticationModeRejectsUnknownValues(t *testing.T) {
	resetViperWithJWTSecret(t)
	t.Setenv("GATEWAY_LIVE_UPSTREAM_AUTH_MODE", "skip_checks")
	_, err := Load()
	require.ErrorContains(t, err, "gateway.live.upstream_auth_mode")
}

func TestZeroLiveConfigurationRetainsDeviceCheck(t *testing.T) {
	mode, err := (GatewayLiveConfig{}).EffectiveUpstreamAuthMode()
	require.NoError(t, err)
	require.Equal(t, LiveUpstreamAuthModeDeviceCheck, mode)
}
