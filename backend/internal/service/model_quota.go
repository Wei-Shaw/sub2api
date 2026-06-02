package service

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// HashModelPattern produces the short, Redis-safe pattern identifier used in
// the per-model quota cache. Stable across processes — patterns are also fully
// known from the API key / group config, so the hash never needs reverse
// mapping.
func HashModelPattern(pattern string) string {
	sum := sha256.Sum256([]byte(pattern))
	return hex.EncodeToString(sum[:8]) // 16 hex chars
}

// Per-model USD quota types and resolution helpers.
//
// These are distinct from the account-level upstream rate-limit tracking in
// model_rate_limit.go: that file tracks 429 cooldowns reported by upstream
// providers, while this file implements admin-configured per-model USD quotas
// scoped to API keys / groups.

// Type aliases so callers in the service package don't need to import domain
// for these small value types.
type (
	ModelRateLimit  = domain.ModelRateLimit
	ModelRateLimits = domain.ModelRateLimits
)

// ModelQuotaUsage holds the per-(api_key, pattern) rolling usage state.
// Windows are stored as unix seconds; a zero value means "no window started
// yet" and the next write will initialize it.
type ModelQuotaUsage struct {
	Usage5h  float64
	Usage1d  float64
	Usage7d  float64
	Window5h int64
	Window1d int64
	Window7d int64
}

// EffectiveModelRateLimits returns the rules that actually apply to the given
// API key, honoring the "key overrides group" semantics: any non-empty
// per-key configuration fully replaces the group default (no merging).
func EffectiveModelRateLimits(apiKey *APIKey, group *Group) ModelRateLimits {
	if apiKey != nil && len(apiKey.ModelRateLimits) > 0 {
		return apiKey.ModelRateLimits
	}
	if group != nil && len(group.DefaultModelRateLimits) > 0 {
		return group.DefaultModelRateLimits
	}
	return nil
}

// MatchModelRateLimits returns the subset of rules whose pattern matches the
// requested model. A request can match multiple rules (e.g. exact rule plus a
// wildcard); each matched rule is an independent quota pool.
func MatchModelRateLimits(rules ModelRateLimits, requestedModel string) ModelRateLimits {
	if len(rules) == 0 || requestedModel == "" {
		return nil
	}
	var matched ModelRateLimits
	for _, rule := range rules {
		if rule.Pattern == "" {
			continue
		}
		if matchModelPattern(rule.Pattern, requestedModel) {
			matched = append(matched, rule)
		}
	}
	return matched
}

// SanitizeModelRateLimits normalises admin-supplied rules: trims empty patterns,
// clamps negative limits to 0, and drops rules where every limit is 0 (rules
// with no enforcement add Redis overhead for nothing). Returns nil for an
// empty/all-zero input so the DB stores NULL/empty consistently.
func SanitizeModelRateLimits(rules ModelRateLimits) ModelRateLimits {
	if len(rules) == 0 {
		return nil
	}
	out := make(ModelRateLimits, 0, len(rules))
	for _, r := range rules {
		r.Pattern = strings.TrimSpace(r.Pattern)
		if r.Pattern == "" {
			continue
		}
		if r.Limit5h < 0 {
			r.Limit5h = 0
		}
		if r.Limit1d < 0 {
			r.Limit1d = 0
		}
		if r.Limit7d < 0 {
			r.Limit7d = 0
		}
		if !r.HasAnyLimit() {
			continue
		}
		out = append(out, r)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
