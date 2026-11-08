package service

import (
	"io"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/stretchr/testify/require"
)

func TestGrokSSEPrefiltersRespectConfiguredEventLimits(t *testing.T) {
	filters := []struct {
		name string
		wrap func(io.ReadCloser, *config.Config) io.ReadCloser
	}{
		{"billing_ping", func(source io.ReadCloser, cfg *config.Config) io.ReadCloser {
			return newGrokResponsesBillingPingFilterBody(source, &Account{Platform: PlatformGrok}, cfg.Gateway.MaxLineSize, cfg)
		}},
		{"client_tools", func(source io.ReadCloser, cfg *config.Config) io.ReadCloser {
			return newGrokResponsesClientToolStreamBody(source, apicompat.ResponsesClientToolMapping{}, cfg.Gateway.MaxLineSize, cfg)
		}},
	}
	for _, filter := range filters {
		t.Run(filter.name, func(t *testing.T) {
			for _, tc := range []struct {
				name    string
				gateway config.GatewayConfig
				wire    string
				limited bool
			}{
				{
					name:    "byte_limit",
					gateway: config.GatewayConfig{MaxLineSize: 1 << 20, SSEEventMaxBytes: 64},
					wire:    strings.Repeat(": "+strings.Repeat("x", 30)+"\n", 3) + "\n",
					limited: true,
				},
				{
					name:    "line_limit",
					gateway: config.GatewayConfig{MaxLineSize: 1 << 20, SSEEventMaxLines: 2},
					wire:    strings.Repeat(": keepalive\n", 3) + "\n",
					limited: true,
				},
				{
					name:    "event_larger_than_individual_line_limit",
					gateway: config.GatewayConfig{MaxLineSize: 128, SSEEventMaxBytes: 1024},
					wire:    strings.Repeat(": "+strings.Repeat("x", 30)+"\n", 8) + "\n",
				},
				{
					name:    "line_limit_above_default",
					gateway: config.GatewayConfig{MaxLineSize: 1 << 20, SSEEventMaxLines: defaultSSEEventMaxLines + 1},
					wire:    strings.Repeat(": keepalive\n", defaultSSEEventMaxLines+1) + "\n",
				},
			} {
				t.Run(tc.name, func(t *testing.T) {
					body := filter.wrap(io.NopCloser(strings.NewReader(tc.wire)), &config.Config{Gateway: tc.gateway})
					defer func() { require.NoError(t, body.Close()) }()
					output, err := io.ReadAll(body)
					if tc.limited {
						require.ErrorIs(t, err, errSSETransportLimit)
						return
					}
					require.NoError(t, err)
					require.Equal(t, tc.wire, string(output))
				})
			}
		})
	}
}
