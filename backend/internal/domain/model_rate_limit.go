package domain

// ModelRateLimit represents a per-model USD rate limit rule.
// Pattern supports the same wildcard syntax as the existing model routing
// rules (see service.matchModelPattern): exact match plus trailing "*".
//
// A limit value of 0 means "no limit for this window".
type ModelRateLimit struct {
	Pattern string  `json:"pattern"`
	Limit5h float64 `json:"limit_5h,omitempty"`
	Limit1d float64 `json:"limit_1d,omitempty"`
	Limit7d float64 `json:"limit_7d,omitempty"`
}

// ModelRateLimits is the JSON-encoded slice persisted on api_keys and groups.
//
// On an API key, a non-nil slice fully overrides the inherited group defaults
// (replace, not merge) so admins can reason about exactly which rules apply
// to a key without having to mentally union two sources.
type ModelRateLimits []ModelRateLimit

// HasAnyLimit returns true if at least one window has a non-zero limit.
func (m ModelRateLimit) HasAnyLimit() bool {
	return m.Limit5h > 0 || m.Limit1d > 0 || m.Limit7d > 0
}
