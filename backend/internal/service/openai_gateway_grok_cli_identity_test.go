//go:build unit

package service

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
)

func TestApplyGrokCLIIdentityUnifyOnIgnoresInboundVersion(t *testing.T) {
	t.Setenv(xai.CLIVersionEnv, "")
	xai.SetCLIIdentityResolver(func() xai.CLIIdentityPolicy {
		return xai.CLIIdentityPolicy{Unify: true, Version: xai.CLIClientVersion}
	})
	t.Cleanup(func() { xai.SetCLIIdentityResolver(nil) })

	headers := make(http.Header)
	inbound := make(http.Header)
	inbound.Set("x-grok-client-version", "1.0.20")
	inbound.Set("User-Agent", "grok-shell/1.0.20 (linux; x86_64)")

	applyGrokCLIIdentity(headers, inbound)

	require.Equal(t, xai.CLIClientVersion, headers.Get("x-grok-client-version"))
	require.Equal(t, xai.CLIUserAgent(xai.CLIClientVersion), headers.Get("User-Agent"))
	require.Equal(t, "interactive", headers.Get("X-Grok-Client-Mode"))
}

func TestApplyGrokCLIIdentityUnifyOffPassesThroughGrokClient(t *testing.T) {
	t.Setenv(xai.CLIVersionEnv, "")
	xai.SetCLIIdentityResolver(func() xai.CLIIdentityPolicy {
		return xai.CLIIdentityPolicy{Unify: false, Version: xai.CLIClientVersion}
	})
	t.Cleanup(func() { xai.SetCLIIdentityResolver(nil) })

	headers := make(http.Header)
	inbound := make(http.Header)
	inbound.Set("x-grok-client-version", "1.0.20")
	inbound.Set("User-Agent", "grok-shell/1.0.20 (linux; x86_64)")

	applyGrokCLIIdentity(headers, inbound)

	require.Equal(t, "1.0.20", headers.Get("x-grok-client-version"))
	require.Equal(t, "grok-shell/1.0.20 (linux; x86_64)", headers.Get("User-Agent"))
	require.Equal(t, "interactive", headers.Get("X-Grok-Client-Mode"))
}

func TestApplyGrokCLIIdentityUnifyOffFallsBackWhenInboundIsNotGrok(t *testing.T) {
	t.Setenv(xai.CLIVersionEnv, "")
	xai.SetCLIIdentityResolver(func() xai.CLIIdentityPolicy {
		return xai.CLIIdentityPolicy{Unify: false, Version: xai.CLIClientVersion}
	})
	t.Cleanup(func() { xai.SetCLIIdentityResolver(nil) })

	headers := make(http.Header)
	inbound := make(http.Header)
	inbound.Set("User-Agent", "claude-cli/2.1.280 (Mac OS; arm64)")
	inbound.Set("x-grok-client-version", "0.2.120")

	applyGrokCLIIdentity(headers, inbound)

	require.Equal(t, xai.CLIClientVersion, headers.Get("x-grok-client-version"))
	require.Equal(t, xai.CLIUserAgent(xai.CLIClientVersion), headers.Get("User-Agent"))
}
