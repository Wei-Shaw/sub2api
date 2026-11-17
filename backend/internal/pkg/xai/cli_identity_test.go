package xai

import (
	"net/http"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/mod/semver"
)

func TestApplyCLIProxyHeadersMeetsUpstreamMinimumVersion(t *testing.T) {
	// 上游 426 明确要求至少 1.0.13；断言独立于生产版本常量，防止旧覆盖值绕过下限。
	for _, override := range []string{"", "0.2.93", "0.2.120", "1.0.12", "1.0.13-beta.1", "1.0.13", "1.0.14-alpha.1"} {
		t.Run("override="+override, func(t *testing.T) {
			t.Setenv(CLIVersionEnv, override)
			req, err := http.NewRequest(http.MethodPost, "https://cli-chat-proxy.grok.com/v1/responses", nil)
			require.NoError(t, err)

			ApplyCLIProxyHeaders(req)

			version := req.Header.Get("x-grok-client-version")
			require.True(t, semver.IsValid("v"+version))
			require.GreaterOrEqual(t, semver.Compare("v"+version, "v1.0.13"), 0,
				"Grok Responses 会以 426 拒绝版本 %s，最低要求为 1.0.13", version)
			require.Contains(t, req.Header.Get("User-Agent"), "grok-shell/"+version+" (")
		})
	}
}

func TestCLIUserAgentMatchesOfficialInteractiveCapture(t *testing.T) {
	// 来源：官方 CLI 1.0.46 交互式界面在 Linux x86_64 的主 Responses 请求抓包。
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("该抓包样本来自 Linux x86_64")
	}
	require.Equal(t, "grok-pager/1.0.46 grok-shell/1.0.46 (linux; x86_64)", CLIUserAgent("1.0.46"))
}

func TestResolveCLIVersionDefaultsToPinnedClientVersion(t *testing.T) {
	t.Setenv(CLIVersionEnv, "")
	// Default advertise pin is CLIClientVersion; CLIStableVersion is only the floor.
	require.Equal(t, CLIClientVersion, ResolveCLIVersion())
	require.True(t, IsSupportedCLIVersion(CLIClientVersion))
	require.True(t, IsSupportedCLIVersion(CLIStableVersion))
}

func TestResolveCLIVersionAcceptsValidOverride(t *testing.T) {
	t.Setenv(CLIVersionEnv, "1.0.14-alpha.1")
	require.Equal(t, "1.0.14-alpha.1", ResolveCLIVersion())
}

func TestResolveCLIVersionRejectsUnsafeOrTooOld(t *testing.T) {
	for _, version := range []string{
		"1.0.12",
		"1.0.13-beta.1",
		"1.0.14\r\nX-Injected: true",
		"1.0.014",
		"1.1",
		"2",
	} {
		t.Run(version, func(t *testing.T) {
			t.Setenv(CLIVersionEnv, version)
			require.Equal(t, CLIClientVersion, ResolveCLIVersion())
		})
	}
}

func TestApplyCLIProxyHeaders(t *testing.T) {
	t.Setenv(CLIVersionEnv, "")

	req, err := http.NewRequest(http.MethodPost, "https://cli-chat-proxy.grok.com/v1/responses", nil)
	require.NoError(t, err)
	req.Header.Set("User-Agent", "legacy-client/1.0")

	ApplyCLIProxyHeaders(req)

	require.Equal(t, CLIClientVersion, req.Header.Get("x-grok-client-version"))
	require.Equal(t, CLIClientIdentifier, req.Header.Get("x-grok-client-identifier"))
	require.Equal(t, CLITokenAuth, req.Header.Get("X-XAI-Token-Auth"))
	require.Equal(t, "interactive", req.Header.Get("x-grok-client-mode"))
	require.Equal(t, "authenticate-response", req.Header.Get("x-authenticateresponse"))
	require.Equal(t, CLIUserAgent(CLIClientVersion), req.Header.Get("User-Agent"))
}

func TestApplyCLIProxyHeadersLeavesAPIHostUnchanged(t *testing.T) {
	t.Setenv(CLIVersionEnv, "1.0.14")

	req, err := http.NewRequest(http.MethodPost, "https://api.x.ai/v1/responses", nil)
	require.NoError(t, err)
	req.Header.Set("User-Agent", "direct-api-client/1.0")

	ApplyCLIProxyHeaders(req)

	require.Empty(t, req.Header.Get("x-grok-client-version"))
	require.Empty(t, req.Header.Get("x-grok-client-identifier"))
	require.Empty(t, req.Header.Get("X-XAI-Token-Auth"))
	require.Empty(t, req.Header.Get("x-grok-client-mode"))
	require.Empty(t, req.Header.Get("x-authenticateresponse"))
	require.Equal(t, "direct-api-client/1.0", req.Header.Get("User-Agent"))
}
