package service

import (
	"context"
	"net/http"
)

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
	if settingService == nil || account == nil {
		return ctx
	}
	if !settingService.IsUpstreamPolicyV1Enabled(ctx) {
		return ctx
	}
	defaults := settingService.GetUpstreamPassthroughDefaults(ctx)
	override := settingService.GetUpstreamPassthroughGlobalOverride(ctx)
	policy := ResolveUpstreamPassthroughPolicy(account, &defaults, override)
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
	p, ok := GetUpstreamPolicyFromContext(ctx)
	if !ok {
		return false
	}
	return p.ForwardUserNetworkInfo
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
