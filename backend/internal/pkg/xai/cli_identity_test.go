package xai

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveCLIVersionDefaultsToPinnedClientVersion(t *testing.T) {
	t.Setenv(CLIVersionEnv, "")
	SetCLIIdentityResolver(nil)
	require.Equal(t, CLIClientVersion, ResolveCLIVersion())
	require.True(t, IsSupportedCLIVersion(CLIClientVersion))
	require.True(t, IsSupportedCLIVersion(CLIStableVersion))
}

func TestResolveCLIVersionAcceptsValidOverride(t *testing.T) {
	t.Setenv(CLIVersionEnv, "1.0.20-alpha.1")
	SetCLIIdentityResolver(nil)
	require.Equal(t, "1.0.20-alpha.1", ResolveCLIVersion())
}

func TestResolveCLIVersionRejectsUnsafeOrTooOld(t *testing.T) {
	SetCLIIdentityResolver(nil)
	for _, version := range []string{
		"1.0.12",
		"1.0.13-beta.1",
		"0.2.120",
		"1.0.20\r\nX-Injected: true",
		"1.0.013",
		"1.0",
		"1",
	} {
		t.Run(version, func(t *testing.T) {
			t.Setenv(CLIVersionEnv, version)
			require.Equal(t, CLIClientVersion, ResolveCLIVersion())
		})
	}
}

func TestApplyCLIProxyHeaders(t *testing.T) {
	t.Setenv(CLIVersionEnv, "")
	SetCLIIdentityResolver(nil)

	req, err := http.NewRequest(http.MethodPost, "https://cli-chat-proxy.grok.com/v1/responses", nil)
	require.NoError(t, err)
	req.Header.Set("User-Agent", "legacy-client/1.0")

	ApplyCLIProxyHeaders(req)

	require.Equal(t, CLIClientVersion, req.Header.Get("x-grok-client-version"))
	require.Equal(t, CLIClientIdentifier, req.Header.Get("x-grok-client-identifier"))
	require.Equal(t, CLITokenAuth, req.Header.Get("X-XAI-Token-Auth"))
	require.Equal(t, CLIAuthenticateResponseValue, req.Header.Get(CLIAuthenticateResponseHeader))
	require.Equal(t, CLIUserAgent(CLIClientVersion), req.Header.Get("User-Agent"))
}

func TestApplyCLIProxyHeadersLeavesAPIHostUnchanged(t *testing.T) {
	t.Setenv(CLIVersionEnv, "1.0.20")
	SetCLIIdentityResolver(nil)

	req, err := http.NewRequest(http.MethodPost, "https://api.x.ai/v1/responses", nil)
	require.NoError(t, err)
	req.Header.Set("User-Agent", "direct-api-client/1.0")

	ApplyCLIProxyHeaders(req)

	require.Empty(t, req.Header.Get("x-grok-client-version"))
	require.Empty(t, req.Header.Get("x-grok-client-identifier"))
	require.Empty(t, req.Header.Get("X-XAI-Token-Auth"))
	require.Equal(t, "direct-api-client/1.0", req.Header.Get("User-Agent"))
}

func TestApplyCLIProxyHeadersUnifyOffPreservesVersionAndUA(t *testing.T) {
	t.Setenv(CLIVersionEnv, "")
	SetCLIIdentityResolver(func() CLIIdentityPolicy {
		return CLIIdentityPolicy{Unify: false, Version: CLIClientVersion}
	})
	t.Cleanup(func() { SetCLIIdentityResolver(nil) })

	req, err := http.NewRequest(http.MethodPost, "https://cli-chat-proxy.grok.com/v1/responses", nil)
	require.NoError(t, err)
	setRawHeader(req.Header, "x-grok-client-version", "1.0.20")
	req.Header.Set("User-Agent", "grok-shell/1.0.20 (linux; x86_64)")

	ApplyCLIProxyHeaders(req)

	require.Equal(t, []string{"1.0.20"}, rawHeader(req.Header, "x-grok-client-version"))
	require.Equal(t, "grok-shell/1.0.20 (linux; x86_64)", req.Header.Get("User-Agent"))
	require.Equal(t, CLITokenAuth, req.Header.Get("X-XAI-Token-Auth"))
	require.Equal(t, CLIAuthenticateResponseValue, req.Header.Get(CLIAuthenticateResponseHeader))
	require.Equal(t, CLIClientIdentifier, req.Header.Get("x-grok-client-identifier"))
}

func TestApplyCLIProxyHeadersUnifyOnOverwritesAccountOverride(t *testing.T) {
	t.Setenv(CLIVersionEnv, "")
	SetCLIIdentityResolver(func() CLIIdentityPolicy {
		return CLIIdentityPolicy{Unify: true, Version: CLIClientVersion}
	})
	t.Cleanup(func() { SetCLIIdentityResolver(nil) })

	req, err := http.NewRequest(http.MethodPost, "https://cli-chat-proxy.grok.com/v1/responses", nil)
	require.NoError(t, err)
	setRawHeader(req.Header, "x-grok-client-version", "1.0.20")
	req.Header.Set("X-Grok-Client-Version", "0.2.120")
	req.Header.Set("User-Agent", "xai-grok-workspace/1.0.44")

	ApplyCLIProxyHeaders(req)

	require.Empty(t, rawHeader(req.Header, "x-grok-client-version"))
	require.Equal(t, CLIClientVersion, req.Header.Get("x-grok-client-version"))
	require.Equal(t, CLIUserAgent(CLIClientVersion), req.Header.Get("User-Agent"))
	require.Equal(t, CLIAuthenticateResponseValue, req.Header.Get(CLIAuthenticateResponseHeader))
}

func TestStampCLIIdentityHeadersClearsMixedCaseDuplicates(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "https://cli-chat-proxy.grok.com/v1/responses", nil)
	require.NoError(t, err)
	setRawHeader(req.Header, "x-grok-client-version", "1.0.20")
	req.Header.Set("X-Grok-Client-Version", "0.2.120")
	req.Header.Set("User-Agent", "xai-grok-workspace/0.2.120")

	StampCLIIdentityHeaders(req.Header, CLIClientVersion)

	require.Empty(t, rawHeader(req.Header, "x-grok-client-version"))
	require.Equal(t, CLIClientVersion, req.Header.Get("x-grok-client-version"))
	require.Equal(t, CLIUserAgent(CLIClientVersion), req.Header.Get("User-Agent"))
}

func setRawHeader(h http.Header, key, value string) {
	h[key] = []string{value}
}

func rawHeader(h http.Header, key string) []string {
	return h[key]
}
