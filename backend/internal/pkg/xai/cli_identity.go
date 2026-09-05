package xai

import (
	"net/http"
	"os"
	"strings"

	"golang.org/x/mod/semver"
)

// Fixed Grok Build / CLI-chat-proxy client identity.
// These values are intentionally pinned in-binary (not scraped from live CLI).
// Operators may bump the version via XAI_GROK_CLI_VERSION without a release.
const (
	// CLIProxyHost is the hostname that requires the official CLI identity headers.
	CLIProxyHost = "cli-chat-proxy.grok.com"

	// CLIStableVersion is the known-good minimum client version accepted by cli-chat-proxy.
	CLIStableVersion = "0.2.93"

	// CLIVersionEnv is the optional operator override for CLIStableVersion.
	CLIVersionEnv = "XAI_GROK_CLI_VERSION"

	// CLITokenAuth is required by cli-chat-proxy for Grok Build OAuth tokens.
	CLITokenAuth = "xai-grok-cli"

	// CLIClientIdentifier is the x-grok-client-identifier value used by Grok shell/CLI.
	CLIClientIdentifier = "grok-shell"

	// CLIClientMode is used by billing / quota probes on the CLI surface.
	CLIClientMode = "cli"

	// CLIAuthenticateResponse is required by cli-chat-proxy auth middleware.
	CLIAuthenticateResponse = "authenticate-response"

	officialShellUserAgentPrefix = "grok-shell/"
)

// ResolveCLIVersion returns a supported CLI client version.
// Empty or invalid overrides fall back to CLIClientVersion (the pinned
// preferred client pin in billing.go). CLIStableVersion is only the minimum
// accepted by IsSupportedCLIVersion, not the default identity we advertise.
func ResolveCLIVersion() string {
	version := strings.TrimSpace(os.Getenv(CLIVersionEnv))
	if !IsSupportedCLIVersion(version) {
		return CLIClientVersion
	}
	return version
}

// IsSupportedCLIVersion reports whether version is a valid semver string at or
// above CLIStableVersion (prereleases below a higher release are rejected when
// they compare less than the stable pin).
func IsSupportedCLIVersion(version string) bool {
	canonical := "v" + version
	minimum := "v" + CLIStableVersion
	return semver.IsValid(canonical) &&
		semver.Canonical(canonical) == canonical &&
		semver.Compare(canonical, minimum) >= 0
}

// CLIUserAgent builds the workspace-style User-Agent for a CLI client version.
func CLIUserAgent(version string) string {
	if strings.TrimSpace(version) == "" {
		version = CLIClientVersion
	}
	return "xai-grok-workspace/" + version
}

// OfficialShellUserAgentVersion returns the grok-shell/ version from a User-Agent.
func OfficialShellUserAgentVersion(userAgent string) string {
	ua := strings.TrimSpace(userAgent)
	if !strings.HasPrefix(strings.ToLower(ua), officialShellUserAgentPrefix) {
		return ""
	}
	rest := ua[len(officialShellUserAgentPrefix):]
	if i := strings.IndexAny(rest, " \t"); i >= 0 {
		rest = rest[:i]
	}
	return strings.TrimSpace(rest)
}

// HasSupportedOfficialIdentity reports whether headers already identify a
// supported official grok-shell / grok CLI client.
func HasSupportedOfficialIdentity(h http.Header) bool {
	if h == nil {
		return false
	}
	version := strings.TrimSpace(firstHeaderValue(h, "x-grok-client-version"))
	if version == "" {
		version = OfficialShellUserAgentVersion(h.Get("User-Agent"))
	}
	if !IsSupportedCLIVersion(version) {
		return false
	}
	identifier := strings.TrimSpace(firstHeaderValue(h, "x-grok-client-identifier"))
	ua := strings.ToLower(strings.TrimSpace(h.Get("User-Agent")))
	return identifier == CLIClientIdentifier || strings.HasPrefix(ua, officialShellUserAgentPrefix)
}

// EnsureCLIProxyAuthHeaders fills cli-chat-proxy auth headers when missing.
func EnsureCLIProxyAuthHeaders(h http.Header) {
	if h == nil {
		return
	}
	if strings.TrimSpace(h.Get("X-XAI-Token-Auth")) == "" {
		h.Set("X-XAI-Token-Auth", CLITokenAuth)
	}
	if strings.TrimSpace(firstHeaderValue(h, "x-authenticateresponse")) == "" {
		h.Set("x-authenticateresponse", CLIAuthenticateResponse)
	}
}

func firstHeaderValue(h http.Header, key string) string {
	if h == nil {
		return ""
	}
	if v := strings.TrimSpace(h.Get(key)); v != "" {
		return v
	}
	return strings.TrimSpace(h.Get(http.CanonicalHeaderKey(key)))
}

// ApplyCLIProxyHeaders stamps the fixed Grok CLI identity when the request
// targets cli-chat-proxy. Direct api.x.ai traffic is left unchanged.
func ApplyCLIProxyHeaders(req *http.Request) {
	if req == nil || req.URL == nil || !strings.EqualFold(strings.TrimSpace(req.URL.Hostname()), CLIProxyHost) {
		return
	}
	if req.Header == nil {
		req.Header = make(http.Header)
	}
	version := ResolveCLIVersion()
	req.Header.Set("X-XAI-Token-Auth", CLITokenAuth)
	req.Header.Set("x-grok-client-version", version)
	req.Header.Set("x-grok-client-identifier", CLIClientIdentifier)
	req.Header.Set("User-Agent", CLIUserAgent(version))
}
