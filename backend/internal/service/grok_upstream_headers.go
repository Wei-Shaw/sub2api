package service

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
)

// grokUpstreamUserAgent is kept for compatibility with older Grok request
// tests. Current requests use the pinned default UA from this package.
const grokUpstreamUserAgent = "sub2api-grok/1.0"

// Fixed CLI identity aliases — single source of truth is internal/pkg/xai.
const (
	grokClientVersionHeader    = xai.CLIStableVersion
	grokClientIdentifierHeader = xai.CLIClientIdentifier
	grokClientModeHeader       = xai.CLIClientMode
)

// defaultGrokUpstreamUserAgent is the pinned Grok CLI / workspace UA.
// Grok upstream must not forward Claude Code / Codex / browser client UAs.
func defaultGrokUpstreamUserAgent() string {
	return xai.CLIUserAgent(xai.ResolveCLIVersion())
}

func applyDefaultGrokUpstreamHeaders(req *http.Request) {
	if req == nil {
		return
	}
	// Always stamp CLI identity. Do not preserve inbound client UA (Claude Code,
	// Codex, curl, etc.) — xAI chat/CLI surfaces fingerprint the client string.
	req.Header.Set("User-Agent", defaultGrokUpstreamUserAgent())
	req.Header.Set("x-grok-client-version", xai.ResolveCLIVersion())
	req.Header.Set("x-grok-client-identifier", grokClientIdentifierHeader)
}

func applyGrokTLSProfileHeaders(req *http.Request, profile *tlsfingerprint.Profile) {
	// HEAD Profile is TLS-only (no HTTP UserAgent/Originator fields). Always stamp CLI identity.
	applyDefaultGrokUpstreamHeaders(req)
	_ = profile
}

// openAITLSFingerprintRuntime is the resolved TLS fingerprint routing result
// used by OpenAI/Grok outbound header application. Defined here so Grok header
// helpers compile even when the full OpenAI TLS router is not present on HEAD.
type openAITLSFingerprintRuntime struct {
	Profile            *tlsfingerprint.Profile
	UpstreamUserAgent  string
	UpstreamOriginator string
	Matched            bool
}

func applyGrokRuntimeHeaders(req *http.Request, runtime openAITLSFingerprintRuntime) {
	applyDefaultGrokUpstreamHeaders(req)
	if req == nil {
		return
	}
	// Apply Originator only; force CLI UA after so router overrides cannot
	// leak Codex/Claude Code identity onto Grok upstream.
	if originator := strings.TrimSpace(runtime.UpstreamOriginator); originator != "" {
		req.Header.Set("Originator", originator)
	}
	req.Header.Set("User-Agent", defaultGrokUpstreamUserAgent())
}

// resolveGrokUpstreamUserAgent always returns the pinned Grok CLI User-Agent.
// Inbound client UAs (Claude Code, Codex, browsers, libraries) are never forwarded.
func resolveGrokUpstreamUserAgent(_ *gin.Context) string {
	return defaultGrokUpstreamUserAgent()
}
