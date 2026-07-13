package handler

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// ──────────────────────────────────────────────────────────
// Canonical inbound / upstream endpoint paths.
// All normalization and derivation reference this single set
// of constants — add new paths HERE when a new API surface
// is introduced.
// ──────────────────────────────────────────────────────────

const (
	EndpointMessages          = "/v1/messages"
	EndpointChatCompletions   = "/v1/chat/completions"
	EndpointEmbeddings        = "/v1/embeddings"
	EndpointAlphaSearch       = "/v1/alpha/search"
	EndpointResponses         = "/v1/responses"
	EndpointResponsesCompact  = "/v1/responses/compact"
	EndpointImagesGenerations = "/v1/images/generations"
	EndpointImagesEdits       = "/v1/images/edits"
	EndpointVideosGenerations = "/v1/videos/generations"
	EndpointVideosEdits       = "/v1/videos/edits"
	EndpointVideosExtensions  = "/v1/videos/extensions"
	EndpointVideos            = "/v1/videos"
	EndpointGeminiModels      = "/v1beta/models"
)

// gin.Context keys used by the middleware and helpers below.
const (
	ctxKeyInboundEndpoint = "_gateway_inbound_endpoint"
)

// ──────────────────────────────────────────────────────────
// Normalization functions
// ──────────────────────────────────────────────────────────

// NormalizeInboundEndpoint maps a raw request path (which may carry
// prefixes like /antigravity, /openai) to its canonical form.
//
//	"/antigravity/v1/messages"   → "/v1/messages"
//	"/v1/chat/completions"       → "/v1/chat/completions"
//	"/openai/v1/responses/foo"   → "/v1/responses"
//	"/v1beta/models/gemini:gen"  → "/v1beta/models"
//
// The OpenAI Responses API is also exposed via a few bare/alias
// routes that do not carry a "/v1/" prefix (top-level bare route and
// the Codex direct route). "/responses/compact" (and "/backend-api/
// codex/responses/compact") is a distinct client endpoint — the
// "compact" client — and is normalized to its OWN canonical inbound
// endpoint, EndpointResponsesCompact, rather than being folded into
// the root Responses endpoint. Any other subpath under the bare/alias
// roots (i.e. not "compact" itself or nested under it) remains a
// subresource suffix of the root Responses endpoint:
//
//	"/v1/responses/compact"                         → EndpointResponsesCompact
//	"/v1/responses/compact/detail"                  → EndpointResponsesCompact
//	"/openai/v1/responses/compact"                  → EndpointResponsesCompact
//	"/openai/v1/responses/compact/detail"           → EndpointResponsesCompact
//	"/responses/compact"                            → EndpointResponsesCompact
//	"/responses/compact/detail"                     → EndpointResponsesCompact
//	"/backend-api/codex/responses/compact"          → EndpointResponsesCompact
//	"/backend-api/codex/responses/compact/detail"   → EndpointResponsesCompact
//	"/v1/responses"                                 → EndpointResponses
//	"/openai/v1/responses"                          → EndpointResponses
//	"/responses"                                    → EndpointResponses
//	"/backend-api/codex/responses"                  → EndpointResponses
//
// The compact check MUST be evaluated before the root Responses check,
// otherwise "/v1/responses" (a prefix of "/v1/responses/compact")
// would erroneously match first.
func NormalizeInboundEndpoint(path string) string {
	path = strings.TrimSpace(path)
	switch {
	case strings.Contains(path, EndpointEmbeddings):
		return EndpointEmbeddings
	case strings.Contains(path, EndpointAlphaSearch) || isBareOrSubpathOf(strings.TrimRight(path, "/"), "/alpha/search") || isBareOrSubpathOf(strings.TrimRight(path, "/"), "/backend-api/codex/alpha/search"):
		return EndpointAlphaSearch
	case strings.Contains(path, EndpointChatCompletions):
		return EndpointChatCompletions
	case strings.Contains(path, EndpointMessages):
		return EndpointMessages
	case strings.Contains(path, EndpointImagesGenerations) || strings.Contains(path, "/images/generations"):
		return EndpointImagesGenerations
	case strings.Contains(path, EndpointImagesEdits) || strings.Contains(path, "/images/edits"):
		return EndpointImagesEdits
	case strings.Contains(path, EndpointVideosGenerations) || strings.Contains(path, "/videos/generations"):
		return EndpointVideosGenerations
	case strings.Contains(path, EndpointVideosEdits) || strings.Contains(path, "/videos/edits"):
		return EndpointVideosEdits
	case strings.Contains(path, EndpointVideosExtensions) || strings.Contains(path, "/videos/extensions"):
		return EndpointVideosExtensions
	case strings.Contains(path, EndpointVideos) || strings.Contains(path, "/videos/"):
		return EndpointVideos
	case strings.Contains(path, EndpointResponsesCompact) || isResponsesCompactAliasPath(path):
		return EndpointResponsesCompact
	case strings.Contains(path, EndpointResponses) || isResponsesRootAliasPath(path):
		return EndpointResponses
	case strings.Contains(path, EndpointGeminiModels):
		return EndpointGeminiModels
	default:
		return path
	}
}

// isResponsesCompactAliasPath reports whether path is the bare/alias
// "compact" client endpoint — i.e. it is rooted at "/responses/compact"
// or "/backend-api/codex/responses/compact" (bare routes that serve
// the OpenAI Responses API "compact" client without a "/v1/" prefix),
// or any subpath nested under either of those roots:
//
//   - "/responses/compact"                   (bare route, compact client)
//   - "/responses/compact/*subpath"          (nested, e.g. "/responses/compact/detail")
//   - "/backend-api/codex/responses/compact" (Codex direct route, compact client)
//   - "/backend-api/codex/responses/compact/*subpath" (nested, e.g.
//     "/backend-api/codex/responses/compact/detail")
//
// This MUST be checked before isResponsesRootAliasPath, since
// "/responses" is a prefix of "/responses/compact".
func isResponsesCompactAliasPath(path string) bool {
	trimmed := strings.TrimRight(strings.TrimSpace(path), "/")
	if trimmed == "" {
		return false
	}
	return isBareOrSubpathOf(trimmed, "/responses/compact") || isBareOrSubpathOf(trimmed, "/backend-api/codex/responses/compact")
}

// isResponsesRootAliasPath reports whether path is one of the bare/alias
// routes that serve the root OpenAI Responses API without a "/v1/"
// prefix, or any non-"compact" subpath registered under them:
//
//   - "/responses"                    (top-level bare route)
//   - "/responses/*subpath"           (any subpath other than "compact",
//     since "compact" is its own distinct inbound endpoint)
//   - "/backend-api/codex/responses"  (Codex direct route)
//   - "/backend-api/codex/responses/*subpath" (any subpath other than
//     "compact")
//
// Only the top-level bare route and the Codex direct route (and their
// subpaths) are recognized here — this deliberately does NOT generalize
// to any path merely ending in "/responses" (e.g. an unrelated
// "/foo/responses" must not match).
func isResponsesRootAliasPath(path string) bool {
	trimmed := strings.TrimRight(strings.TrimSpace(path), "/")
	if trimmed == "" {
		return false
	}
	return isBareOrSubpathOf(trimmed, "/responses") || isBareOrSubpathOf(trimmed, "/backend-api/codex/responses")
}

// isBareOrSubpathOf reports whether path is exactly root, or a subpath
// rooted at root (i.e. root followed by "/"). This anchors the match
// at the start of path so it cannot match paths where root appears
// nested under some other unrelated prefix.
func isBareOrSubpathOf(path, root string) bool {
	return path == root || strings.HasPrefix(path, root+"/")
}

// DeriveUpstreamEndpoint determines the upstream endpoint from the
// account platform and the normalized inbound endpoint.
//
// Platform-specific rules:
//   - OpenAI and Grok text compatibility routes forward to /v1/responses
//     (with optional subpath such as /v1/responses/compact preserved from
//     the raw URL); native endpoints such as embeddings and alpha search
//     retain their paths. Grok raw Chat requests override this through the
//     forwarding result consumed by resolveOpenAIUpstreamEndpoint.
//   - Anthropic  → /v1/messages
//   - Gemini     → /v1beta/models
//   - Antigravity → /v1/messages (Claude) or gemini (Gemini)
//   - Antigravity routes may target either Claude or Gemini, so the
//     inbound endpoint is used to distinguish.
func DeriveUpstreamEndpoint(inbound, rawRequestPath, platform string) string {
	inbound = strings.TrimSpace(inbound)

	switch platform {
	case service.PlatformOpenAI, service.PlatformGrok:
		if inbound == EndpointEmbeddings || inbound == EndpointAlphaSearch || inbound == EndpointImagesGenerations || inbound == EndpointImagesEdits || inbound == EndpointVideosGenerations || inbound == EndpointVideosEdits || inbound == EndpointVideosExtensions || inbound == EndpointVideos {
			return inbound
		}
		// OpenAI forwards everything to the Responses API.
		// Preserve subresource suffix (e.g. /v1/responses/compact,
		// /v1/responses/compact/detail) as derived from the raw path.
		if suffix := responsesSubpathSuffix(rawRequestPath); suffix != "" {
			return EndpointResponses + suffix
		}
		// The raw path carried no derivable suffix (e.g. it was already
		// normalized upstream, or the caller only has the canonical
		// inbound endpoint available) — fall back to the canonical
		// compact endpoint when that's what the inbound request was
		// recognized as, so it isn't silently treated as the root
		// Responses endpoint.
		if inbound == EndpointResponsesCompact {
			return EndpointResponsesCompact
		}
		return EndpointResponses

	case service.PlatformAnthropic:
		return EndpointMessages

	case service.PlatformGemini:
		return EndpointGeminiModels

	case service.PlatformAntigravity:
		// Antigravity accounts serve both Claude and Gemini.
		if inbound == EndpointGeminiModels {
			return EndpointGeminiModels
		}
		return EndpointMessages
	}

	// Unknown platform — fall back to inbound.
	return inbound
}

// responsesSubpathSuffix extracts the part after "/responses" in a raw
// request path, e.g. "/openai/v1/responses/compact" → "/compact".
// Returns "" when there is no meaningful suffix.
func responsesSubpathSuffix(rawPath string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(rawPath), "/")
	idx := strings.LastIndex(trimmed, "/responses")
	if idx < 0 {
		return ""
	}
	suffix := trimmed[idx+len("/responses"):]
	if suffix == "" || suffix == "/" {
		return ""
	}
	if !strings.HasPrefix(suffix, "/") {
		return ""
	}
	return suffix
}

// ──────────────────────────────────────────────────────────
// Middleware
// ──────────────────────────────────────────────────────────

// InboundEndpointMiddleware normalizes the request path and stores the
// canonical inbound endpoint in gin.Context so that every handler in
// the chain can read it via GetInboundEndpoint.
//
// Apply this middleware to all gateway route groups.
func InboundEndpointMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		path := ""
		if c.Request != nil && c.Request.URL != nil {
			path = c.Request.URL.Path
		}
		if path == "" {
			path = c.FullPath()
		}
		c.Set(ctxKeyInboundEndpoint, NormalizeInboundEndpoint(path))
		c.Next()
	}
}

// ──────────────────────────────────────────────────────────
// Context helpers — used by handlers before building
// RecordUsageInput / RecordUsageLongContextInput.
// ──────────────────────────────────────────────────────────

// GetInboundEndpoint returns the canonical inbound endpoint stored by
// InboundEndpointMiddleware. If the middleware did not run (e.g. in
// tests), it falls back to normalizing c.Request.URL.Path on the fly
// (preferring the raw request path over c.FullPath(), which collapses
// wildcard route patterns such as "/v1/responses/*subpath" and would
// otherwise mis-normalize concrete requests like "/v1/responses/compact"
// to the root Responses endpoint).
func GetInboundEndpoint(c *gin.Context) string {
	if v, ok := c.Get(ctxKeyInboundEndpoint); ok {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	// Fallback: normalize on the fly.
	path := ""
	if c != nil {
		if c.Request != nil && c.Request.URL != nil {
			path = c.Request.URL.Path
		}
		if path == "" {
			path = c.FullPath()
		}
	}
	return NormalizeInboundEndpoint(path)
}

// GetUpstreamEndpoint derives the upstream endpoint from the context
// and the account platform. Handlers call this after scheduling an
// account, passing account.Platform.
func GetUpstreamEndpoint(c *gin.Context, platform string) string {
	inbound := GetInboundEndpoint(c)
	rawPath := ""
	if c != nil && c.Request != nil && c.Request.URL != nil {
		rawPath = c.Request.URL.Path
	}
	return DeriveUpstreamEndpoint(inbound, rawPath, platform)
}
