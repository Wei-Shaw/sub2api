package service

import "context"

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
