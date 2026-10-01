package xai

import (
	"net/http"
	"net/textproto"
	"os"
	"strings"
	"sync/atomic"

	"golang.org/x/mod/semver"
)

// Fixed Grok Build / CLI-chat-proxy client identity.
// These values are intentionally pinned in-binary (not scraped from live CLI).
// Operators may bump the version via settings or XAI_GROK_CLI_VERSION without a release.
const (
	// CLIProxyHost is the hostname that requires the official CLI identity headers.
	CLIProxyHost = "cli-chat-proxy.grok.com"

	// CLIStableVersion is the known-good minimum client version accepted by cli-chat-proxy.
	CLIStableVersion = "1.0.13"

	// CLIVersionEnv is the optional operator override for the advertised CLI version.
	CLIVersionEnv = "XAI_GROK_CLI_VERSION"

	// CLITokenAuth is required by cli-chat-proxy for Grok Build OAuth tokens.
	CLITokenAuth = "xai-grok-cli"

	// CLIClientIdentifier is the x-grok-client-identifier value used by Grok shell/CLI.
	CLIClientIdentifier = "grok-shell"

	// CLIClientMode is used by billing / quota probes on the CLI surface.
	CLIClientMode = "cli"

	// CLIAuthenticateResponseHeader is required by cli-chat-proxy auth middleware.
	CLIAuthenticateResponseHeader = "x-authenticateresponse"
	CLIAuthenticateResponseValue  = "authenticate-response"
)

// CLIIdentityPolicy is the outbound Grok CLI identity the gateway should advertise.
type CLIIdentityPolicy struct {
	// Unify reports whether official CLI-proxy traffic should force Version/UA.
	// When false, transport leaves version/UA alone so inbound passthrough or
	// account header overrides can reach cli-chat-proxy.
	Unify bool
	// Version is the supported CLI client version to advertise when Unify is true,
	// and the fallback when Unify is false but the inbound request has no Grok identity.
	Version string
}

type cliIdentityResolverFunc func() CLIIdentityPolicy

var cliIdentityResolver atomic.Pointer[cliIdentityResolverFunc]

// SetCLIIdentityResolver injects the runtime identity policy (settings → env → pin).
// Passing nil restores the env/pin default used by unit tests.
func SetCLIIdentityResolver(resolver func() CLIIdentityPolicy) {
	if resolver == nil {
		cliIdentityResolver.Store(nil)
		return
	}
	r := cliIdentityResolverFunc(resolver)
	cliIdentityResolver.Store(&r)
}

func defaultCLIIdentity() CLIIdentityPolicy {
	return CLIIdentityPolicy{Unify: true, Version: EnvOrPinnedCLIVersion()}
}

// EnvOrPinnedCLIVersion returns XAI_GROK_CLI_VERSION when it is supported, otherwise the binary pin.
func EnvOrPinnedCLIVersion() string {
	version := strings.TrimSpace(os.Getenv(CLIVersionEnv))
	if !IsSupportedCLIVersion(version) {
		return CLIClientVersion
	}
	return version
}

// ResolveCLIIdentity returns the current outbound identity policy.
func ResolveCLIIdentity() CLIIdentityPolicy {
	if r := cliIdentityResolver.Load(); r != nil {
		policy := (*r)()
		if !IsSupportedCLIVersion(policy.Version) {
			policy.Version = defaultCLIIdentity().Version
		}
		return policy
	}
	return defaultCLIIdentity()
}

// ResolveCLIVersion returns a supported CLI client version.
// Empty or invalid overrides fall back to CLIClientVersion (the pinned
// preferred client pin in billing.go). CLIStableVersion is only the minimum
// accepted by IsSupportedCLIVersion, not the default identity we advertise.
func ResolveCLIVersion() string {
	return ResolveCLIIdentity().Version
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

// CLIUserAgent builds the official Grok shell User-Agent for a CLI client version.
func CLIUserAgent(version string) string {
	if strings.TrimSpace(version) == "" {
		version = CLIClientVersion
	}
	return "grok-shell/" + version + " (macos; aarch64)"
}

// IsGrokCLIUserAgent reports whether ua looks like an official Grok CLI product.
func IsGrokCLIUserAgent(ua string) bool {
	lower := strings.ToLower(strings.TrimSpace(ua))
	if lower == "" {
		return false
	}
	return strings.Contains(lower, "grok-shell/") ||
		strings.Contains(lower, "grok-pager/") ||
		strings.Contains(lower, "xai-grok-workspace/")
}

func deleteHeaderAllCasing(h http.Header, key string) {
	if h == nil || key == "" {
		return
	}
	canonical := textproto.CanonicalMIMEHeaderKey(key)
	h.Del(key)
	delete(h, key)
	delete(h, strings.ToLower(key))
	if canonical != key {
		delete(h, canonical)
	}
}

// StampCLIIdentityHeaders writes the unified CLI identity, clearing any mixed-case duplicates.
func StampCLIIdentityHeaders(h http.Header, version string) {
	if h == nil {
		return
	}
	if !IsSupportedCLIVersion(version) {
		version = ResolveCLIVersion()
	}
	deleteHeaderAllCasing(h, "User-Agent")
	deleteHeaderAllCasing(h, "x-grok-client-version")
	deleteHeaderAllCasing(h, "X-Grok-Client-Version")
	deleteHeaderAllCasing(h, "x-grok-client-identifier")
	deleteHeaderAllCasing(h, "X-XAI-Token-Auth")
	deleteHeaderAllCasing(h, CLIAuthenticateResponseHeader)
	h.Set("X-XAI-Token-Auth", CLITokenAuth)
	h.Set("x-grok-client-version", version)
	h.Set("x-grok-client-identifier", CLIClientIdentifier)
	h.Set(CLIAuthenticateResponseHeader, CLIAuthenticateResponseValue)
	h.Set("User-Agent", CLIUserAgent(version))
}

// EnsureCLIAuthHeaders fills cli-chat-proxy auth companion headers without touching version/UA.
func EnsureCLIAuthHeaders(h http.Header) {
	if h == nil {
		return
	}
	if strings.TrimSpace(h.Get("X-XAI-Token-Auth")) == "" {
		h.Set("X-XAI-Token-Auth", CLITokenAuth)
	}
	if strings.TrimSpace(h.Get(CLIAuthenticateResponseHeader)) == "" {
		h.Set(CLIAuthenticateResponseHeader, CLIAuthenticateResponseValue)
	}
	if strings.TrimSpace(h.Get("x-grok-client-identifier")) == "" {
		h.Set("x-grok-client-identifier", CLIClientIdentifier)
	}
}

// ApplyCLIProxyHeaders stamps or preserves Grok CLI identity when the request
// targets cli-chat-proxy. Direct api.x.ai traffic is left unchanged.
func ApplyCLIProxyHeaders(req *http.Request) {
	if req == nil || req.URL == nil || !strings.EqualFold(strings.TrimSpace(req.URL.Hostname()), CLIProxyHost) {
		return
	}
	if req.Header == nil {
		req.Header = make(http.Header)
	}
	policy := ResolveCLIIdentity()
	if policy.Unify {
		StampCLIIdentityHeaders(req.Header, policy.Version)
		return
	}
	EnsureCLIAuthHeaders(req.Header)
}
