// Package upstream defines the single seam every supported upstream must
// implement.
//
// Today each upstream lives in its own god file:
//
//	service/gateway_service.go            9471 lines  (Claude)
//	service/openai_gateway_service.go     7062 lines  (OpenAI / Codex)
//	service/antigravity_gateway_service.go 3945 lines (Antigravity / Gemini)
//
// Auth header assembly, base-URL resolution, upstream error classification and
// session-affinity key derivation are re-implemented in all three. This package
// hoists exactly those four concerns behind one interface so that a new
// platform is a ~200 line adapter instead of another 4000-9000 line gateway.
//
// The package deliberately depends on the standard library only: it can be
// adopted one call-site at a time without dragging the service package (and its
// ent/redis/aws dependency tree) into a test binary.
package upstream

import (
	"fmt"
	"net/http"
	"time"
)

// Platform identifies a supported upstream provider.
type Platform string

const (
	// PlatformClaude is the Anthropic Messages API (claude.ai / Console keys).
	PlatformClaude Platform = "claude"
	// PlatformOpenAI is the OpenAI API plus the ChatGPT/Codex backend.
	PlatformOpenAI Platform = "openai"
	// PlatformAntigravity is the Google Cloud Code / Gemini backend.
	PlatformAntigravity Platform = "antigravity"
)

// AuthKind is how an account authenticates to its upstream. The two kinds use
// different headers and different beta flags, and the difference is currently
// re-checked at ~30 call sites across the three gateway files.
type AuthKind string

const (
	// AuthAPIKey is a static provider-issued key.
	AuthAPIKey AuthKind = "api_key"
	// AuthOAuth is a refresh-token-derived bearer token.
	AuthOAuth AuthKind = "oauth"
)

// Credentials is the minimal account-side input an adapter needs. It mirrors
// the subset of service.Account that actually affects the wire format, so the
// service layer can build one without this package importing it.
type Credentials struct {
	Kind AuthKind

	// APIKey is set when Kind == AuthAPIKey.
	APIKey string
	// AccessToken is set when Kind == AuthOAuth.
	AccessToken string

	// BaseURL is an optional per-account override (Account.Credentials["base_url"]).
	BaseURL string
	// AccountID is the upstream-side account identifier, when the platform uses one.
	AccountID string
	// ExtraBeta carries account-level beta/feature flags that must be merged into
	// the platform's mandatory beta header.
	ExtraBeta string
	// ImpersonateClient selects client-identification headers ("codex" for the
	// Codex CLI fingerprint, "claude-code" for the Claude Code one). Empty means
	// identify as a plain API client. This is the only place the fingerprint
	// decision lives, instead of being re-derived in three gateway files.
	ImpersonateClient string
	// APIKeyScheme selects how an API key is delivered upstream. Empty means the
	// platform default. Anthropic-compatible relays differ: some want x-api-key,
	// others want Authorization: Bearer with the same key
	// (service/anthropic_apikey_auth.go today).
	APIKeyScheme APIKeyScheme
}

// APIKeyScheme is how a static API key is presented on the wire.
type APIKeyScheme string

const (
	// APIKeySchemeDefault is the provider's native header (x-api-key for
	// Anthropic, Authorization: Bearer for OpenAI/Google).
	APIKeySchemeDefault APIKeyScheme = ""
	// APIKeySchemeXAPIKey forces x-api-key (Anthropic-compatible relays).
	APIKeySchemeXAPIKey APIKeyScheme = "x_api_key"
	// APIKeySchemeAuthorizationBearer forces Authorization: Bearer <key>.
	APIKeySchemeAuthorizationBearer APIKeyScheme = "authorization_bearer"
)

// BaseURLFallbacks is an optional interface for platforms that expose several
// equivalent endpoints (e.g. a daily versus a production host). The gateway
// walks the list in order and only advances on FailureTransient/FailureOverload.
// Antigravity currently has this logic inline in resolveAntigravityForwardBaseURL.
type BaseURLFallbacks interface {
	BaseURLFallbacks(creds Credentials) []string
}

// RequestMeta is the request-side input needed for affinity decisions.
type RequestMeta struct {
	// SessionKey is the client-supplied session_id / conversation_id /
	// prompt_cache_key, already resolved by the caller.
	SessionKey string
	// Model is the requested model id.
	Model string
}

// FailureKind classifies an upstream outcome so the scheduler can act on it
// without knowing which platform produced it. This is the enum that currently
// hides in ~90 scattered status-code comparisons.
type FailureKind int

const (
	// FailureNone means the request was accepted.
	FailureNone FailureKind = iota
	// FailureAuth means the credential is dead (401/403): disable the account
	// and fail over.
	FailureAuth
	// FailureRateLimit means the upstream quota window is exhausted (429):
	// keep the account but make it unschedulable until RetryAt.
	FailureRateLimit
	// FailureOverload means the upstream is temporarily unhealthy (5xx/529):
	// retry another account, do not penalise the credential.
	FailureOverload
	// FailureTransient means a network/timeout error: retry, no state change.
	FailureTransient
	// FailureClient means the caller's request is at fault (400/404/413/422):
	// do not retry, surface the upstream error verbatim.
	FailureClient
)

// String renders a FailureKind for logs and metrics.
func (k FailureKind) String() string {
	switch k {
	case FailureNone:
		return "none"
	case FailureAuth:
		return "auth"
	case FailureRateLimit:
		return "rate_limit"
	case FailureOverload:
		return "overload"
	case FailureTransient:
		return "transient"
	case FailureClient:
		return "client"
	default:
		return "unknown"
	}
}

// Failure is a classified upstream outcome.
type Failure struct {
	Kind FailureKind
	// RetryAt is when the account becomes schedulable again. Only meaningful for
	// FailureRateLimit (and for FailureOverload when upstream advertises a window).
	RetryAt time.Time
	// Reason is a short, already-redacted explanation for logs and ops dashboards.
	Reason string
}

// Schedulable reports whether the account may be used again at now.
func (f Failure) Schedulable(now time.Time) bool {
	switch f.Kind {
	case FailureNone, FailureTransient, FailureClient:
		return true
	case FailureRateLimit, FailureOverload:
		return !now.Before(f.RetryAt)
	default:
		return false
	}
}

// Adapter is the entire per-platform surface. Everything else (retry, failover,
// billing, streaming plumbing) belongs to the shared gateway and must not be
// re-implemented per platform.
type Adapter interface {
	// Platform reports which provider this adapter speaks for.
	Platform() Platform

	// DefaultBaseURL is used when the account has no base_url override.
	DefaultBaseURL() string

	// AuthHeaders returns the headers that authenticate this account. It must
	// include every header that is mandatory for the platform (version pins,
	// beta flags, client-identification headers), so callers never special-case
	// a platform again.
	AuthHeaders(creds Credentials) (http.Header, error)

	// BaseURL resolves the effective base URL, including per-platform
	// rewriting of a custom override (e.g. Antigravity appends /antigravity).
	BaseURL(creds Credentials) string

	// Classify maps an upstream response into a scheduler action.
	//
	// now is injected rather than read from the clock inside the adapter: the
	// result carries a deadline (RetryAt), so a hidden time.Now() here would make
	// cooldown behaviour untestable and non-replayable.
	Classify(now time.Time, status int, header http.Header, body []byte) Failure

	// StickyKey derives the affinity key that pins consecutive turns of one
	// conversation to one account. Returning "" disables affinity.
	StickyKey(meta RequestMeta) string
}

// Registry resolves a Platform to its adapter.
type Registry struct {
	byPlatform map[Platform]Adapter
}

// NewRegistry builds a registry from adapters, rejecting duplicates and nils so
// a wiring mistake fails at startup instead of at request time.
func NewRegistry(adapters ...Adapter) (*Registry, error) {
	r := &Registry{byPlatform: make(map[Platform]Adapter, len(adapters))}
	for _, a := range adapters {
		if a == nil {
			return nil, fmt.Errorf("upstream: nil adapter")
		}
		p := a.Platform()
		if p == "" {
			return nil, fmt.Errorf("upstream: adapter %T reports empty platform", a)
		}
		if _, dup := r.byPlatform[p]; dup {
			return nil, fmt.Errorf("upstream: duplicate adapter for platform %q", p)
		}
		r.byPlatform[p] = a
	}
	return r, nil
}

// Get returns the adapter for p.
func (r *Registry) Get(p Platform) (Adapter, bool) {
	if r == nil {
		return nil, false
	}
	a, ok := r.byPlatform[p]
	return a, ok
}

// MustGet is Get for wiring code where a missing adapter is a programming error.
func (r *Registry) MustGet(p Platform) Adapter {
	a, ok := r.Get(p)
	if !ok {
		panic(fmt.Sprintf("upstream: no adapter registered for platform %q", p))
	}
	return a
}

// Platforms returns the registered platforms, for startup self-checks.
func (r *Registry) Platforms() []Platform {
	out := make([]Platform, 0, len(r.byPlatform))
	for p := range r.byPlatform {
		out = append(out, p)
	}
	return out
}
