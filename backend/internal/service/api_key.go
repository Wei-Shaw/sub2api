package service

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
)

// API Key status constants
const (
	StatusAPIKeyActive         = "active"
	StatusAPIKeyDisabled       = "disabled"
	StatusAPIKeyQuotaExhausted = "quota_exhausted"
	StatusAPIKeyExpired        = "expired"
)

// Rate limit window durations
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

type APIKey struct {
	ID          int64
	UserID      int64
	Key         string
	Name        string
	GroupID     *int64
	Status      string
	IPWhitelist []string
	IPBlacklist []string
	// 预编译的 IP 规则，用于认证热路径避免重复 ParseIP/ParseCIDR。
	CompiledIPWhitelist *ip.CompiledIPRules `json:"-"`
	CompiledIPBlacklist *ip.CompiledIPRules `json:"-"`
	LastUsedAt          *time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
	User                *User
	Group               *Group

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

	// CacheStrategy 表达用户级缓存 TTL 偏好。仅在绑定账号 SupportsCachePolicy()
	// 时生效；否则该值被忽略（前端也会禁用对应控件）。
	// 合法取值：CacheStrategyAuto / CacheStrategyCostPriority / CacheStrategyLatencyPriority。
	CacheStrategy string

	// Per-model USD rate limits. Non-empty fully overrides the bound group's
	// DefaultModelRateLimits (replace, not merge). Empty/nil → inherit group.
	ModelRateLimits ModelRateLimits
}

// Cache strategy enum constants — see APIKey.CacheStrategy.
const (
	// CacheStrategyAuto 跟随账号/全局设置（现状行为）。
	CacheStrategyAuto = "auto"
	// CacheStrategyCostPriority 强制 5m TTL：把 cache_creation 归类到 5m 桶，
	// 并在请求改写时把注入断点 TTL 设为 5m。最省钱，命中率视客户端行为而定。
	CacheStrategyCostPriority = "cost_priority"
	// CacheStrategyLatencyPriority 强制 1h TTL：扩大缓存窗口，适合长会话；
	// 计费上 1h cache creation 比 5m 贵 ~2x，但跨小时仍能命中。
	CacheStrategyLatencyPriority = "latency_priority"
)

// NormalizeCacheStrategy 将外部输入归一化到合法枚举。空值/未知值 → "auto"。
func NormalizeCacheStrategy(s string) string {
	switch s {
	case CacheStrategyCostPriority, CacheStrategyLatencyPriority:
		return s
	default:
		return CacheStrategyAuto
	}
}

// EffectiveCacheStrategy 返回归一化后的偏好。空字符串（旧数据）当作 auto。
func (k *APIKey) EffectiveCacheStrategy() string {
	if k == nil {
		return CacheStrategyAuto
	}
	return NormalizeCacheStrategy(k.CacheStrategy)
}

// CacheStrategyTTLTarget 返回此偏好对应的强制 TTL 目标，若为 auto 则返回 ("", false)。
// 调用方应在 auto 时回落到账号/全局策略。
func (k *APIKey) CacheStrategyTTLTarget() (string, bool) {
	switch k.EffectiveCacheStrategy() {
	case CacheStrategyCostPriority:
		return "5m", true
	case CacheStrategyLatencyPriority:
		return "1h", true
	default:
		return "", false
	}
}

func (k *APIKey) IsActive() bool {
	return k.Status == StatusActive
}

// HasRateLimits returns true if any rate limit window is configured
func (k *APIKey) HasRateLimits() bool {
	return k.RateLimit5h > 0 || k.RateLimit1d > 0 || k.RateLimit7d > 0
}

// IsExpired checks if the API key has expired
func (k *APIKey) IsExpired() bool {
	if k.ExpiresAt == nil {
		return false
	}
	return time.Now().After(*k.ExpiresAt)
}

// IsQuotaExhausted checks if the API key quota is exhausted
func (k *APIKey) IsQuotaExhausted() bool {
	if k.Quota <= 0 {
		return false // unlimited
	}
	return k.QuotaUsed >= k.Quota
}

// GetQuotaRemaining returns remaining quota (-1 for unlimited)
func (k *APIKey) GetQuotaRemaining() float64 {
	if k.Quota <= 0 {
		return -1 // unlimited
	}
	remaining := k.Quota - k.QuotaUsed
	if remaining < 0 {
		return 0
	}
	return remaining
}

// GetDaysUntilExpiry returns days until expiry (-1 for never expires)
func (k *APIKey) GetDaysUntilExpiry() int {
	if k.ExpiresAt == nil {
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
	if IsWindowExpired(k.Window5hStart, RateLimitWindow5h) {
		return 0
	}
	return k.Usage5h
}

// EffectiveUsage1d returns the 1d window usage, or 0 if the window has expired.
func (k *APIKey) EffectiveUsage1d() float64 {
	if IsWindowExpired(k.Window1dStart, RateLimitWindow1d) {
		return 0
	}
	return k.Usage1d
}

// EffectiveUsage7d returns the 7d window usage, or 0 if the window has expired.
func (k *APIKey) EffectiveUsage7d() float64 {
	if IsWindowExpired(k.Window7dStart, RateLimitWindow7d) {
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
