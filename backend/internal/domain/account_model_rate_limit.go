package domain

import (
	"strings"
	"time"
)

// Pure Extra-reader helpers for model-rate-limit / credits-exhausted checks.
// The ctx-cascade methods (modelRateLimitKeysForRequest etc.) stay in service
// as free functions (Phase 3 Account BC hybrid) so request-metadata / metrics
// helpers do not enter domain. These two helpers are pure Extra readers and
// are shared by domain.IsCreditsExhausted and the service free funcs.

const (
	// ModelRateLimitsKey is the accounts.extra key holding per-model rate-limit windows.
	ModelRateLimitsKey = "model_rate_limits"
	// CreditsExhaustedKey is the model_rate_limits scope that marks AI Credits exhaustion.
	CreditsExhaustedKey = "AICredits"
)

// ModelRateLimitResetAt returns the rate-limit reset time for the given scope
// from accounts.extra, or nil if unset/unparseable.
func (a *Account) ModelRateLimitResetAt(scope string) *time.Time {
	if a == nil || a.Extra == nil || scope == "" {
		return nil
	}
	rawLimits, ok := a.Extra[ModelRateLimitsKey].(map[string]any)
	if !ok {
		return nil
	}
	rawLimit, ok := rawLimits[scope].(map[string]any)
	if !ok {
		return nil
	}
	resetAtRaw, ok := rawLimit["rate_limit_reset_at"].(string)
	if !ok || strings.TrimSpace(resetAtRaw) == "" {
		return nil
	}
	resetAt, err := time.Parse(time.RFC3339, resetAtRaw)
	if err != nil {
		return nil
	}
	return &resetAt
}

// IsRateLimitActiveForKey reports whether the given model-rate-limit scope is
// currently active (reset_at in the future).
func (a *Account) IsRateLimitActiveForKey(key string) bool {
	resetAt := a.ModelRateLimitResetAt(key)
	return resetAt != nil && time.Now().Before(*resetAt)
}

// GetRateLimitRemainingForKey returns remaining duration for the given scope;
// 0 means not rate-limited or already expired.
func (a *Account) GetRateLimitRemainingForKey(key string) time.Duration {
	resetAt := a.ModelRateLimitResetAt(key)
	if resetAt == nil {
		return 0
	}
	remaining := time.Until(*resetAt)
	if remaining > 0 {
		return remaining
	}
	return 0
}

// IsCreditsExhausted reports whether the AICredits rate-limit key is active
// (credits exhausted). Lifted from service/antigravity_credits_overages.go.
func (a *Account) IsCreditsExhausted() bool {
	if a == nil {
		return false
	}
	return a.IsRateLimitActiveForKey(CreditsExhaustedKey)
}
