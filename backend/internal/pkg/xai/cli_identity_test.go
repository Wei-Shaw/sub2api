package xai

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveCLIVersionDefaultsToPinnedClientVersion(t *testing.T) {
	t.Setenv(CLIVersionEnv, "")
	// Default advertise pin is CLIClientVersion; CLIStableVersion is only the floor.
	require.Equal(t, CLIClientVersion, ResolveCLIVersion())
	require.True(t, IsSupportedCLIVersion(CLIClientVersion))
	require.True(t, IsSupportedCLIVersion(CLIStableVersion))
REDACTED

func TestResolveCLIVersionAcceptsValidOverride(t *testing.T) {
	t.Setenv(CLIVersionEnv, "0.2.95-alpha.1")
	require.Equal(t, "0.2.95-alpha.1", ResolveCLIVersion())
REDACTED

func TestResolveCLIVersionRejectsUnsafeOrTooOld(t *testing.T) {
	for _, version := range []string{
		"0.2.92",
		"0.2.93-beta.1",
		"0.2.95\r\nX-Injected: true",
		"0.2.093",
		"0.3",
		"1",
REDACTED {
		t.Run(version, func(t *testing.T) {
			t.Setenv(CLIVersionEnv, version)
			require.Equal(t, CLIClientVersion, ResolveCLIVersion())
	REDACTED)
REDACTED
REDACTED

func TestApplyCLIProxyHeaders(t *testing.T) {
	t.Setenv(CLIVersionEnv, "")

	req, err := http.NewRequest(http.MethodPost, "https://cli-chat-proxy.grok.com/v1/responses", nil)
REDACTED
	req.Header.Set("User-Agent", "sub2api-grok/1.0")

	ApplyCLIProxyHeaders(req)

	require.Equal(t, CLIClientVersion, req.Header.Get("x-grok-client-version"))
	require.Equal(t, CLIClientIdentifier, req.Header.Get("x-grok-client-identifier"))
	require.Equal(t, CLITokenAuth, req.Header.Get("X-XAI-Token-Auth"))
	require.Equal(t, CLIUserAgent(CLIClientVersion), req.Header.Get("User-Agent"))
REDACTED

func TestApplyCLIProxyHeadersLeavesAPIHostUnchanged(t *testing.T) {
	t.Setenv(CLIVersionEnv, "0.2.95")

	req, err := http.NewRequest(http.MethodPost, "https://api.x.ai/v1/responses", nil)
REDACTED
	req.Header.Set("User-Agent", "sub2api-grok/1.0")

	ApplyCLIProxyHeaders(req)

	require.Empty(t, req.Header.Get("x-grok-client-version"))
	require.Empty(t, req.Header.Get("x-grok-client-identifier"))
	require.Empty(t, req.Header.Get("X-XAI-Token-Auth"))
	require.Equal(t, "sub2api-grok/1.0", req.Header.Get("User-Agent"))
REDACTED
