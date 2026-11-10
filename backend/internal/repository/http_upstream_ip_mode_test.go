package repository

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

func TestDefaultPoolSettingsResolvesUpstreamIPMode(t *testing.T) {
	cfg := &config.Config{Gateway: config.GatewayConfig{UpstreamIPMode: config.UpstreamIPModeIPv4}}

	require.Equal(t, config.UpstreamIPModeIPv4, defaultPoolSettings(cfg).ipMode)
	require.Equal(t, config.UpstreamIPModeAuto, defaultPoolSettings(nil).ipMode)
}

func TestNewUpstreamDialContextRestrictsAddressFamily(t *testing.T) {
	ctx := context.Background()

	_, err := newUpstreamDialContext(config.UpstreamIPModeIPv4)(ctx, "tcp", "[::1]:443")
	require.ErrorContains(t, err, "no suitable address")

	_, err = newUpstreamDialContext(config.UpstreamIPModeIPv6)(ctx, "tcp", "127.0.0.1:443")
	require.ErrorContains(t, err, "no suitable address")
}
