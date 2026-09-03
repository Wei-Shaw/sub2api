package service

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
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

// HashAPIKeyCredential 返回 API Key 的认证摘要：小写十六进制 SHA-256。
//
// 必须与 migrations/239 的回填表达式
// encode(sha256(convert_to(key, 'UTF8')), 'hex') 逐字节一致——Key 只含 ASCII，
// 所以 UTF8 编码后的字节即 []byte(key)。
//
// 为什么是裸 SHA-256 而不是 HMAC：本仓库的密钥库就是数据库自身
// （security_secrets 表），HMAC 的 pepper 只能躺在同一份备份里，对备份泄漏
// 这个威胁模型几乎不增加强度；而回填必须在纯 SQL 里跑完，PostgreSQL 内置
// sha256() 无需扩展，hmac() 则要 pgcrypto 且会把 pepper 写进迁移文件与
// schema_migrations 校验和。系统生成的 Key 是 32 字节 crypto/rand，
// 不存在彩虹表/暴力破解面。
//
// 空串返回空串：调用方据此区分"没有摘要"与"空串的摘要"，
// 后者会与滚动升级窗口里老版本写入的空 key_hash 撞在一起。
func HashAPIKeyCredential(key string) string {
	if key == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

// MaskAPIKeyCredential 把 Key 压成"前 12 位…后 4 位"的展示串。
//
// 管理端列表接口只下发这个结果：完整 Key 一旦进了 HTTP 响应，就同时留在了
// 浏览器网络面板、前端组件状态和任何中间日志里，前端用 substring 再遮一遍
// 并不能把它从响应体里拿掉。
//
// 太短以至于遮不住的 Key 直接整条打码，不泄露任何前缀。
func MaskAPIKeyCredential(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	const (
		prefixLen = 12
		suffixLen = 4
	)
	if len(key) <= prefixLen+suffixLen {
		return strings.Repeat("*", len(key))
	}
	return key[:prefixLen] + "..." + key[len(key)-suffixLen:]
}

type APIKey struct {
	ID     int64
	UserID int64
	Key    string
	// KeyHash 是 Key 的 SHA-256 摘要，认证查询走它。
	// 两阶段迁移的第 1 阶段仍同时保留明文 Key，第 2 阶段才删除明文列。
	KeyHash     string
	Name        string
	GroupID     *int64
	Status      string
	IPWhitelist []string
	IPBlacklist []string
	// 预编译的 IP 规则，用于认证热路径避免重复 ParseIP/ParseCIDR。
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
