package service

// APIKeyAuthSnapshot API Key 认证缓存快照（仅包含认证所需字段）
type APIKeyAuthSnapshot struct {
	APIKeyID    int64                    `json:"api_key_id"`
	UserID      int64                    `json:"user_id"`
	GroupID     *int64                   `json:"group_id,omitempty"`
	Status      string                   `json:"status"`
	IPWhitelist []string                 `json:"ip_whitelist,omitempty"`
	IPBlacklist []string                 `json:"ip_blacklist,omitempty"`
	User        APIKeyAuthUserSnapshot   `json:"user"`
	Group       *APIKeyAuthGroupSnapshot `json:"group,omitempty"`
REDACTED

// APIKeyAuthUserSnapshot 用户快照
type APIKeyAuthUserSnapshot struct {
	ID          int64   `json:"id"`
	Status      string  `json:"status"`
	Role        string  `json:"role"`
	Balance     float64 `json:"balance"`
	Concurrency int     `json:"concurrency"`
REDACTED

// APIKeyAuthGroupSnapshot 分组快照
type APIKeyAuthGroupSnapshot struct {
	ID               int64    `json:"id"`
	Name             string   `json:"name"`
	Platform         string   `json:"platform"`
	Status           string   `json:"status"`
	SubscriptionType string   `json:"subscription_type"`
	RateMultiplier   float64  `json:"rate_multiplier"`
	DailyLimitUSD    *float64 `json:"daily_limit_usd,omitempty"`
	WeeklyLimitUSD   *float64 `json:"weekly_limit_usd,omitempty"`
	MonthlyLimitUSD  *float64 `json:"monthly_limit_usd,omitempty"`
	ImagePrice1K     *float64 `json:"image_price_1k,omitempty"`
	ImagePrice2K     *float64 `json:"image_price_2k,omitempty"`
	ImagePrice4K     *float64 `json:"image_price_4k,omitempty"`
	ClaudeCodeOnly   bool     `json:"claude_code_only"`
	FallbackGroupID  *int64   `json:"fallback_group_id,omitempty"`
REDACTED

// APIKeyAuthCacheEntry 缓存条目，支持负缓存
type APIKeyAuthCacheEntry struct {
	NotFound bool                `json:"not_found"`
	Snapshot *APIKeyAuthSnapshot `json:"snapshot,omitempty"`
REDACTED
