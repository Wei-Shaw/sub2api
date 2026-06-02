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

// MergedModelRateLimits returns the union of group and key rules. When both
// sides define the same pattern, each window limit is the minimum of the two
// (group acts as ceiling). Patterns present in only one side pass through
// unchanged. This ensures admin-set group limits can never be exceeded by
// user-level key configuration.
func MergedModelRateLimits(apiKey *APIKey, group *Group) ModelRateLimits {
	var keyRules ModelRateLimits
	if apiKey != nil {
		keyRules = apiKey.ModelRateLimits
	}
	var groupRules ModelRateLimits
	if group != nil {
		groupRules = group.DefaultModelRateLimits
	}

	if len(groupRules) == 0 {
		return keyRules
	}
	if len(keyRules) == 0 {
		return groupRules
	}

	// Index key rules by pattern for O(n+m) merge.
	keyByPattern := make(map[string]ModelRateLimit, len(keyRules))
	for _, r := range keyRules {
		keyByPattern[r.Pattern] = r
	}

	merged := make(ModelRateLimits, 0, len(groupRules)+len(keyRules))
	seen := make(map[string]bool, len(groupRules)+len(keyRules))

	// Group rules first — they always appear; cap with key if present.
	for _, gr := range groupRules {
		seen[gr.Pattern] = true
		if kr, ok := keyByPattern[gr.Pattern]; ok {
			merged = append(merged, minLimits(gr, kr))
		} else {
			merged = append(merged, gr)
		}
	}
	// Key-only rules (no matching group pattern).
	for _, kr := range keyRules {
		if !seen[kr.Pattern] {
			merged = append(merged, kr)
		}
	}
	return merged
}

// EffectiveModelRateLimits is kept as an alias for MergedModelRateLimits so
// that any remaining callers continue to compile.
func EffectiveModelRateLimits(apiKey *APIKey, group *Group) ModelRateLimits {
	return MergedModelRateLimits(apiKey, group)
}

// minLimits returns a rule with the pattern from a and the per-window minimum
// of both sides. A zero limit means "no cap for this window" — so a non-zero
// limit from the other side wins outright (it's more restrictive than infinity).
func minLimits(a, b ModelRateLimit) ModelRateLimit {
	return ModelRateLimit{
		Pattern: a.Pattern,
		Limit5h: pickMin(a.Limit5h, b.Limit5h),
		Limit1d: pickMin(a.Limit1d, b.Limit1d),
		Limit7d: pickMin(a.Limit7d, b.Limit7d),
	}
}

// pickMin returns the effective minimum of two limits where 0 means "unlimited".
func pickMin(a, b float64) float64 {
	if a == 0 {
		return b
	}
	if b == 0 {
		return a
	}
	if a < b {
		return a
	}
	return b
}

// CapLimitsByGroup enforces group limits as a ceiling on user-supplied rules.
// For each user rule, if the group defines a matching pattern (exact match),
// each window is capped at the group's value. Rules with patterns not present
// in the group pass through unchanged.
func CapLimitsByGroup(userLimits, groupLimits ModelRateLimits) ModelRateLimits {
	if len(groupLimits) == 0 || len(userLimits) == 0 {
		return userLimits
	}
	groupByPattern := make(map[string]ModelRateLimit, len(groupLimits))
	for _, g := range groupLimits {
		groupByPattern[g.Pattern] = g
	}
	out := make(ModelRateLimits, 0, len(userLimits))
	for _, u := range userLimits {
		if g, ok := groupByPattern[u.Pattern]; ok {
			out = append(out, ModelRateLimit{
				Pattern: u.Pattern,
				Limit5h: capWindow(u.Limit5h, g.Limit5h),
				Limit1d: capWindow(u.Limit1d, g.Limit1d),
				Limit7d: capWindow(u.Limit7d, g.Limit7d),
			})
		} else {
			out = append(out, u)
		}
	}
	return out
}

// capWindow caps a user value at the group ceiling. A zero group value means
// "no ceiling for this window" and the user value passes through.
func capWindow(user, ceiling float64) float64 {
	if ceiling == 0 {
		return user
	}
	if user == 0 || user > ceiling {
		return ceiling
	}
	return user
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

// groupDefaultModelRateLimits is a nil-safe accessor for group's DefaultModelRateLimits.
func groupDefaultModelRateLimits(group *Group) ModelRateLimits {
	if group == nil {
		return nil
	}
	return group.DefaultModelRateLimits
}
