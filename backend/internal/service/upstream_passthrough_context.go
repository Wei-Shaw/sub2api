package service

import (
	"context"
	"net/http"
)

// upstreamPolicyRuntimeEnabled is intentionally false. The upstream passthrough
// policy experiment diverged from upstream/main and can change request shape for
// Anthropic-compatible relays. Keep the types/settings for backward-compatible
// JSON parsing, but make all gateway hooks resolve to legacy behavior.
const upstreamPolicyRuntimeEnabled = false

// upstreamPolicyContextKey is the unexported type used as the context key.
// Using an unexported struct type as the key prevents collisions with other
// packages' context values (the standard idiom for context keys in Go).
type upstreamPolicyContextKey struct{}

// SetUpstreamPolicyInContext stores a resolved EffectiveUpstreamPolicy in ctx
// so downstream gateway code can retrieve it via GetUpstreamPolicyFromContext.
//
// If policy is nil, the original ctx is returned unchanged — this mirrors the
// "FeatureFlag OFF" path where we want zero behavior change.
func SetUpstreamPolicyInContext(ctx context.Context, policy *EffectiveUpstreamPolicy) context.Context {
	if policy == nil {
		return ctx
	}
	return context.WithValue(ctx, upstreamPolicyContextKey{}, policy)
}

// GetUpstreamPolicyFromContext retrieves a policy that was previously stored
// via SetUpstreamPolicyInContext. Returns (nil, false) when no policy is set
// (e.g., FeatureFlag was OFF or this code path bypassed Resolve).
//
// Callers that need a non-nil policy for downstream branching should use
// RequireUpstreamPolicyFromContext instead.
func GetUpstreamPolicyFromContext(ctx context.Context) (*EffectiveUpstreamPolicy, bool) {
	if !upstreamPolicyRuntimeEnabled {
		return nil, false
	}
	v := ctx.Value(upstreamPolicyContextKey{})
	if v == nil {
		return nil, false
	}
	policy, ok := v.(*EffectiveUpstreamPolicy)
	if !ok || policy == nil {
		return nil, false
	}
	return policy, true
}

// RequireUpstreamPolicyFromContext returns the policy from ctx, or a safe
// Protected fallback if absent. Used by call sites that always need a usable
// policy without the get-and-branch dance.
//
// The fallback is constructed at call time (cheap — ~70 bytes of struct copy)
// rather than holding a package-level singleton to avoid accidental mutation.
func RequireUpstreamPolicyFromContext(ctx context.Context) EffectiveUpstreamPolicy {
	if p, ok := GetUpstreamPolicyFromContext(ctx); ok {
		return *p
	}
	toggles := ProfileToggleValues(ProfileProtected)
	return EffectiveUpstreamPolicy{
		ForwardClientHeaders:   toggles.ForwardClientHeaders,
		ForwardUserNetworkInfo: toggles.ForwardUserNetworkInfo,
		SkipBodyScrub:          toggles.SkipBodyScrub,
		SkipSystemPromptInject: toggles.SkipSystemPromptInject,
		ForwardClientUA:        toggles.ForwardClientUA,
		ForwardBetaFlags:       toggles.ForwardBetaFlags,
		SkipModelRewrite:       toggles.SkipModelRewrite,
		Category:               CategoryOfficial,
		ProfileApplied:         ProfileProtected,
	}
}

// ResolveAndStorePolicy is the single entry point used by gateway services to
// resolve the per-request upstream policy and attach it to ctx. Returns ctx
// unchanged in these defensive cases:
//   - settingService is nil
//   - account is nil
//   - FeatureFlag (SettingKeyUpstreamPolicyV1Enabled) is false (Phase B-1 default)
//
// In all other cases, calls ResolveUpstreamPassthroughPolicy with the live
// system defaults + kill switch, stores the result in ctx, and returns the
// enriched ctx. Downstream code retrieves via RequireUpstreamPolicyFromContext.
//
// This helper is the central seam: Phase B-2 will read from ctx (no settingService
// dependency at call sites), and the FeatureFlag flip happens by setting the
// SettingKey to "true" (no code change needed at the call sites).
func ResolveAndStorePolicy(ctx context.Context, account *Account, settingService *SettingService) context.Context {
	if !upstreamPolicyRuntimeEnabled {
		return ctx
	}
	if settingService == nil || account == nil {
		return ctx
	}
	if !settingService.IsUpstreamPolicyV1Enabled(ctx) {
		return ctx
	}
	defaults := settingService.GetUpstreamPassthroughDefaults(ctx)
	override := settingService.GetUpstreamPassthroughGlobalOverride(ctx)
	// 三级覆盖中间层：分组级默认 profile（account > group > system）。
	// group 取认证中间件写入 ctx 的 API Key 绑定分组；账号级覆盖仍优先于 group。
	policy := ResolveUpstreamPassthroughPolicyWithGroup(account, GroupFromContext(ctx), &defaults, override)
	return SetUpstreamPolicyInContext(ctx, &policy)
}

// ShouldScrubBody reports whether downstream code should run ScrubThirdPartyBody
// and similar body-cleanup steps.
//
// Returns true (= "do the legacy scrub") when:
//   - No policy is in ctx (FeatureFlag OFF — preserves today's behavior)
//   - A policy is in ctx with SkipBodyScrub == false (Protected/Strict profiles)
//
// Returns false only when a policy is present with SkipBodyScrub == true
// (Transparent profile — relay accounts where the upstream relay will do its own
// hygiene).
//
// This is the call-site read pattern for Phase B-2: legacy-by-default,
// policy-overrides-when-present.
func ShouldScrubBody(ctx context.Context) bool {
	if !upstreamPolicyRuntimeEnabled {
		return true
	}
	p, ok := GetUpstreamPolicyFromContext(ctx)
	if !ok {
		return true
	}
	return !p.SkipBodyScrub
}

// ShouldInjectSystemPrompt reports whether downstream code should run the
// Claude Code system block injector (or similar client-impersonation
// system-prompt injection).
//
// Returns true (= "run the legacy injector, which itself may be a no-op
// depending on cfg.Gateway.InjectCCSystemBlocks") when:
//   - No policy is in ctx (FeatureFlag OFF — preserves today's behavior)
//   - A policy is in ctx with SkipSystemPromptInject == false (Strict profile —
//     reverse-client accounts that MUST inject to look like the impersonated client)
//
// Returns false when policy is present with SkipSystemPromptInject == true
// (Transparent or Protected profiles — relay accounts or official direct accounts
// where the client/user owns their system prompt).
func ShouldInjectSystemPrompt(ctx context.Context) bool {
	if !upstreamPolicyRuntimeEnabled {
		return true
	}
	p, ok := GetUpstreamPolicyFromContext(ctx)
	if !ok {
		return true
	}
	return !p.SkipSystemPromptInject
}

// userNetworkInfoHeaderKeys lists the request headers that carry the end-user's
// network identity. These are NOT in the default outbound whitelist and are
// silently dropped on the upstream request — protecting official API accounts
// from leaking client IPs that might trigger anomaly detection.
//
// When policy.ForwardUserNetworkInfo == true (Transparent profile, relay accounts),
// these headers are copied from the client request to the upstream request.
//
// Keys are written in canonical http.Header form (the form Go produces after
// parsing inbound requests), so http.Header.Get() lookups succeed against
// inbound request headers.
var userNetworkInfoHeaderKeys = []string{
	"X-Forwarded-For",
	"X-Real-Ip", // Go canonical: X-Real-Ip (not X-Real-IP)
	"Forwarded",
	"Cf-Connecting-Ip", // Go canonical: Cf-Connecting-Ip
	"True-Client-Ip",   // Go canonical: True-Client-Ip
}

// ShouldForwardUserNetworkInfo reports whether downstream code should copy
// the user's network identity headers (X-Forwarded-For, X-Real-IP, Forwarded,
// CF-Connecting-IP, True-Client-IP) from the client request to the upstream
// request.
//
// Returns false (= don't forward) when:
//   - No policy is in ctx (FeatureFlag OFF — preserves today's behavior, which
//     never forwards these headers)
//   - A policy is in ctx with ForwardUserNetworkInfo == false (Protected/Strict)
//
// Returns true only when a policy is present with ForwardUserNetworkInfo == true
// (Transparent profile — relay accounts where the relay may need the real IP
// for audit/billing).
//
// This is the inverse fallback shape of ShouldScrubBody/ShouldInjectSystemPrompt:
// those are "Skip" toggles (legacy = do the thing); this is a "Forward" toggle
// (legacy = don't forward).
func ShouldForwardUserNetworkInfo(ctx context.Context) bool {
	if !upstreamPolicyRuntimeEnabled {
		return false
	}
	p, ok := GetUpstreamPolicyFromContext(ctx)
	if !ok {
		return false
	}
	return p.ForwardUserNetworkInfo
}

// clientHeaderForwardBlacklist names the headers that MUST always be stripped
// from upstream requests, even when the resolved policy says
// ForwardClientHeaders=true. These leak inbound auth/session secrets or are
// otherwise nonsensical on a relay-mode forward.
//
// Keys are lowercase (matching the strings.ToLower comparison done at each
// call site).
var clientHeaderForwardBlacklist = map[string]struct{}{
	"authorization":       {},
	"proxy-authorization": {},
	"cookie":              {},
	"set-cookie":          {},
	"x-api-key":           {},
	"x-goog-api-key":      {},
}

// IsClientHeaderBlacklistedForForward reports whether a lowercase header key
// must be stripped from upstream requests regardless of any allow rule. Used
// by the ForwardClientHeaders blacklist-mode forwarding.
//
// The caller must pass a lowercased key. The blacklist intentionally does not
// strip platform-specific identity headers (e.g., x-app, anthropic-beta) —
// those are dropped via the legacy whitelist when ForwardClientHeaders=false
// and kept when =true (because in true mode the caller has explicitly opted
// out of strict mimicry).
func IsClientHeaderBlacklistedForForward(lowerKey string) bool {
	_, ok := clientHeaderForwardBlacklist[lowerKey]
	return ok
}

// ShouldForwardClientHeaders reports whether downstream code should switch
// the upstream-header copy loop from strict whitelist mode (current default)
// to permissive blacklist mode (Transparent profile).
//
// Returns false (= keep legacy strict whitelist) when:
//   - No policy is in ctx (FeatureFlag OFF — preserves today's behavior)
//   - A policy is in ctx with ForwardClientHeaders == false (Protected/Strict)
//
// Returns true only when a policy is present with ForwardClientHeaders == true
// (Transparent profile — relay accounts where every client header should reach
// the upstream relay, except the secret blacklist).
func ShouldForwardClientHeaders(ctx context.Context) bool {
	if !upstreamPolicyRuntimeEnabled {
		return false
	}
	p, ok := GetUpstreamPolicyFromContext(ctx)
	if !ok {
		return false
	}
	return p.ForwardClientHeaders
}

// ShouldCopyClientHeader applies the ForwardClientHeaders policy to a single
// inbound header key. It is the single seam each call site uses to decide
// whether to copy a client header to the upstream request.
//
//   - When ShouldForwardClientHeaders(ctx) is true: copies everything except
//     IsClientHeaderBlacklistedForForward(lowerKey).
//   - Otherwise: delegates to the call-site-specific whitelist predicate
//     (e.g., allowedHeaders[lowerKey] for the Anthropic paths,
//     isOpenAIPassthroughAllowedRequestHeader for the OpenAI passthrough path).
//
// The caller must pass a lowercased key.
func ShouldCopyClientHeader(ctx context.Context, lowerKey string, legacyWhitelist func(string) bool) bool {
	if ShouldForwardClientHeaders(ctx) {
		return !IsClientHeaderBlacklistedForForward(lowerKey)
	}
	if legacyWhitelist == nil {
		return false
	}
	return legacyWhitelist(lowerKey)
}

// ShouldSkipModelRewrite reports whether downstream code should bypass the
// account's per-model rewrite map (e.g., Account.GetModelMapping /
// GetMappedModel) and send the client's requested model string verbatim to
// the upstream.
//
// Returns false (= apply legacy model rewrite) when:
//   - No policy is in ctx (FeatureFlag OFF — preserves today's behavior)
//   - A policy is in ctx with SkipModelRewrite == false (Protected/Strict)
//
// Returns true only when a policy is present with SkipModelRewrite == true
// (Transparent profile — relay accounts whose upstream relay does its own
// model resolution).
//
// Per spec §4.2, this toggle only affects the upstream-facing wire model
// (request body / URL). Rate-limit buckets, billing keys, and admin views
// continue to use the mapped name so internal bookkeeping stays consistent
// regardless of what string was sent on the wire.
func ShouldSkipModelRewrite(ctx context.Context) bool {
	if !upstreamPolicyRuntimeEnabled {
		return false
	}
	p, ok := GetUpstreamPolicyFromContext(ctx)
	if !ok {
		return false
	}
	return p.SkipModelRewrite
}

// ShouldForwardBetaFlags reports whether downstream code should pass the
// client's anthropic-beta / OpenAI-Beta header through to the upstream
// without applying platform-decided beta merging, stripping, or injection
// (e.g., the OAuth-required claude.BetaOAuth set or force_1m_context).
//
// Returns false (= keep legacy platform-decided beta logic) when:
//   - No policy is in ctx (FeatureFlag OFF — preserves today's behavior)
//   - A policy is in ctx with ForwardBetaFlags == false (Protected/Strict)
//
// Returns true only when a policy is present with ForwardBetaFlags == true
// (Transparent profile — relay accounts whose upstream relay decides beta
// semantics itself).
//
// Per spec §4.2, "true" means the platform's beta-decision block — including
// the force_1m_context injection of context-1m-2025-08-07 — is skipped
// entirely. Relay accounts whose upstream auto-enables 1M context are still
// safe because they typically don't require sub2api to inject the beta flag
// (the upstream handles its own gating).
func ShouldForwardBetaFlags(ctx context.Context) bool {
	if !upstreamPolicyRuntimeEnabled {
		return false
	}
	p, ok := GetUpstreamPolicyFromContext(ctx)
	if !ok {
		return false
	}
	return p.ForwardBetaFlags
}

// ShouldForwardClientUA reports whether downstream code should preserve the
// client's original User-Agent header on the upstream request instead of
// substituting a platform-specific mimic value (e.g., Claude CLI UA on the
// OAuth+mimicClaudeCode path).
//
// Returns false (= keep legacy mimic) when:
//   - No policy is in ctx (FeatureFlag OFF — preserves today's behavior)
//   - A policy is in ctx with ForwardClientUA == false (Protected/Strict)
//
// Returns true only when a policy is present with ForwardClientUA == true
// (Transparent profile — relay accounts where the upstream should see the
// real client UA).
//
// Note: per spec §4.2, sticky-session hash is computed from a normalized UA
// value (see GenerateSessionHash + SessionContext.UserAgent) and is NOT
// affected by this toggle. The toggle only controls the User-Agent that goes
// on the upstream wire.
func ShouldForwardClientUA(ctx context.Context) bool {
	if !upstreamPolicyRuntimeEnabled {
		return false
	}
	p, ok := GetUpstreamPolicyFromContext(ctx)
	if !ok {
		return false
	}
	return p.ForwardClientUA
}

// ForwardUserNetworkInfoHeaders copies the user-network headers from
// clientHeaders to upstreamHeaders IFF ShouldForwardUserNetworkInfo(ctx) is
// true. No-op when policy is absent or set to false.
//
// nil-safe on both header maps. Headers absent from the client request are
// silently skipped (no empty value set on upstream).
//
// Storage uses Go canonical key form (e.g., "X-Real-Ip") so http.Header.Get
// lookups succeed regardless of caller casing. Go's HTTP client writes these
// canonical keys onto the wire; intermediate proxies and upstream parsers
// treat HTTP header names case-insensitively per RFC 7230 §3.2, so any
// upstream that cares about XFF will still see it.
func ForwardUserNetworkInfoHeaders(ctx context.Context, upstreamHeaders, clientHeaders http.Header) {
	if !ShouldForwardUserNetworkInfo(ctx) {
		return
	}
	if upstreamHeaders == nil || clientHeaders == nil {
		return
	}
	for _, canonKey := range userNetworkInfoHeaderKeys {
		values := clientHeaders.Values(canonKey)
		if len(values) == 0 {
			continue
		}
		for _, v := range values {
			upstreamHeaders.Add(canonKey, v)
		}
	}
}
