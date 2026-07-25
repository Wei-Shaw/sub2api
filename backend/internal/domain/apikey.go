package domain

import (
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
)

// API-key-domain errors used by persistence and application layers.
var (
	ErrAPIKeyNotFound            = infraerrors.NotFound("API_KEY_NOT_FOUND", "api key not found")
	ErrAPIKeyExists              = infraerrors.Conflict("API_KEY_EXISTS", "api key already exists")
	ErrAPIKeyTooShort            = infraerrors.BadRequest("API_KEY_TOO_SHORT", "api key must be at least 16 characters")
	ErrAPIKeyInvalidChars        = infraerrors.BadRequest("API_KEY_INVALID_CHARS", "api key can only contain letters, numbers, underscores, and hyphens")
	ErrAPIKeyRateLimited         = infraerrors.TooManyRequests("API_KEY_RATE_LIMITED", "too many failed attempts, please try again later")
	ErrAPIKeyAuthOverloaded      = infraerrors.ServiceUnavailable("API_KEY_AUTH_OVERLOADED", "api key authentication is temporarily overloaded")
	ErrInvalidIPPattern          = infraerrors.BadRequest("INVALID_IP_PATTERN", "invalid IP or CIDR pattern")
	ErrAPIKeyExpired             = infraerrors.Forbidden("API_KEY_EXPIRED", "api key 已过期")
	ErrAPIKeyQuotaExhausted      = infraerrors.TooManyRequests("API_KEY_QUOTA_EXHAUSTED", "api key 额度已用完")
	ErrAPIKeyRateLimit5hExceeded = infraerrors.TooManyRequests("API_KEY_RATE_5H_EXCEEDED", "api key 5小时限额已用完")
	ErrAPIKeyRateLimit1dExceeded = infraerrors.TooManyRequests("API_KEY_RATE_1D_EXCEEDED", "api key 日限额已用完")
	ErrAPIKeyRateLimit7dExceeded = infraerrors.TooManyRequests("API_KEY_RATE_7D_EXCEEDED", "api key 7天限额已用完")
)

// API Key status constants (API-key-specific values).
const (
	StatusAPIKeyActive         = "active"
	StatusAPIKeyDisabled       = "disabled"
	StatusAPIKeyQuotaExhausted = "quota_exhausted"
	StatusAPIKeyExpired        = "expired"
)

// Rate limit window durations.
const (
	RateLimitWindow5h = 5 * time.Hour
	RateLimitWindow1d = 24 * time.Hour
	RateLimitWindow7d = 7 * 24 * time.Hour
)

// IsWindowExpired returns true if the window starting at windowStart has exceeded the given duration.
// A nil windowStart is treated as expired — no initialized window means any accumulated usage is stale.
func IsWindowExpired(windowStart *time.Time, duration time.Duration) bool {
	return windowStart == nil || time.Since(*windowStart) >= duration
}

// APIKey is the core API key aggregate.
type APIKey struct {
	ID          int64
	UserID      int64
	Key         string
	Name        string
	GroupID     *int64
	Status      string
	IPWhitelist []string
	IPBlacklist []string
	// Precompiled IP rules for the auth hot path (avoids repeated ParseIP/ParseCIDR).
	CompiledIPWhitelist *ip.CompiledIPRules `json:"-"`
	CompiledIPBlacklist *ip.CompiledIPRules `json:"-"`
	LastUsedAt          *time.Time
	LastUsedIP          *string
	CreatedAt           time.Time
	UpdatedAt           time.Time
	User                *User
	Group               *Group
	CurrentConcurrency  int

	// Quota fields
	Quota     float64    // Quota limit in USD (0 = unlimited)
	QuotaUsed float64    // Used quota amount
	ExpiresAt *time.Time // Expiration time (nil = never expires)

	// Rate limit fields
	RateLimit5h   float64    // Rate limit in USD per 5h (0 = unlimited)
	RateLimit1d   float64    // Rate limit in USD per 1d (0 = unlimited)
	RateLimit7d   float64    // Rate limit in USD per 7d (0 = unlimited)
	Usage5h       float64    // Used amount in current 5h window
	Usage1d       float64    // Used amount in current 1d window
	Usage7d       float64    // Used amount in current 7d window
	Window5hStart *time.Time // Start of current 5h window
	Window1dStart *time.Time // Start of current 1d window
	Window7dStart *time.Time // Start of current 7d window
}

func (k *APIKey) IsActive() bool {
	return k != nil && k.Status == StatusActive
}

// HasRateLimits returns true if any rate limit window is configured.
func (k *APIKey) HasRateLimits() bool {
	return k != nil && (k.RateLimit5h > 0 || k.RateLimit1d > 0 || k.RateLimit7d > 0)
}

// IsExpired checks if the API key has expired.
func (k *APIKey) IsExpired() bool {
	if k == nil || k.ExpiresAt == nil {
		return false
	}
	return time.Now().After(*k.ExpiresAt)
}

// IsQuotaExhausted checks if the API key quota is exhausted.
func (k *APIKey) IsQuotaExhausted() bool {
	if k == nil || k.Quota <= 0 {
		return false // unlimited
	}
	return k.QuotaUsed >= k.Quota
}

// GetQuotaRemaining returns remaining quota (-1 for unlimited).
func (k *APIKey) GetQuotaRemaining() float64 {
	if k == nil || k.Quota <= 0 {
		return -1 // unlimited
	}
	remaining := k.Quota - k.QuotaUsed
	if remaining < 0 {
		return 0
	}
	return remaining
}

// GetDaysUntilExpiry returns days until expiry (-1 for never expires).
func (k *APIKey) GetDaysUntilExpiry() int {
	if k == nil || k.ExpiresAt == nil {
		return -1 // never expires
	}
	duration := time.Until(*k.ExpiresAt)
	if duration < 0 {
		return 0
	}
	return int(duration.Hours() / 24)
}

// EffectiveUsage5h returns the 5h window usage, or 0 if the window has expired.
func (k *APIKey) EffectiveUsage5h() float64 {
	if k == nil || IsWindowExpired(k.Window5hStart, RateLimitWindow5h) {
		return 0
	}
	return k.Usage5h
}

// EffectiveUsage1d returns the 1d window usage, or 0 if the window has expired.
func (k *APIKey) EffectiveUsage1d() float64 {
	if k == nil || IsWindowExpired(k.Window1dStart, RateLimitWindow1d) {
		return 0
	}
	return k.Usage1d
}

// EffectiveUsage7d returns the 7d window usage, or 0 if the window has expired.
func (k *APIKey) EffectiveUsage7d() float64 {
	if k == nil || IsWindowExpired(k.Window7dStart, RateLimitWindow7d) {
		return 0
	}
	return k.Usage7d
}

// APIKeyListFilters holds optional filtering parameters for listing API keys.
type APIKeyListFilters struct {
	Search  string
	Status  string
	GroupID *int64 // nil=不筛选, 0=无分组, >0=指定分组
}

// APIKeyRateLimitData holds rate limit usage and window state for an API key.
type APIKeyRateLimitData struct {
	Usage5h       float64
	Usage1d       float64
	Usage7d       float64
	Window5hStart *time.Time
	Window1dStart *time.Time
	Window7dStart *time.Time
}

// EffectiveUsage5h returns the 5h window usage, or 0 if the window has expired.
func (d *APIKeyRateLimitData) EffectiveUsage5h() float64 {
	if d == nil || IsWindowExpired(d.Window5hStart, RateLimitWindow5h) {
		return 0
	}
	return d.Usage5h
}

// EffectiveUsage1d returns the 1d window usage, or 0 if the window has expired.
func (d *APIKeyRateLimitData) EffectiveUsage1d() float64 {
	if d == nil || IsWindowExpired(d.Window1dStart, RateLimitWindow1d) {
		return 0
	}
	return d.Usage1d
}

// EffectiveUsage7d returns the 7d window usage, or 0 if the window has expired.
func (d *APIKeyRateLimitData) EffectiveUsage7d() float64 {
	if d == nil || IsWindowExpired(d.Window7dStart, RateLimitWindow7d) {
		return 0
	}
	return d.Usage7d
}

// APIKeyQuotaUsageState captures the latest quota fields after an atomic quota update.
// It is intentionally small so repositories can return it from a single SQL statement.
type APIKeyQuotaUsageState struct {
	QuotaUsed float64
	Quota     float64
	Key       string
	Status    string
}
