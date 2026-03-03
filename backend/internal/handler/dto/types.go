package dto

import "time"

type User struct {
	ID            int64     `json:"id"`
	Email         string    `json:"email"`
	Username      string    `json:"username"`
	Role          string    `json:"role"`
	Balance       float64   `json:"balance"`
	Concurrency   int       `json:"concurrency"`
	Status        string    `json:"status"`
	AllowedGroups []int64   `json:"allowed_groups"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`

	APIKeys       []APIKey           `json:"api_keys,omitempty"`
	Subscriptions []UserSubscription `json:"subscriptions,omitempty"`
REDACTED

// AdminUser 是管理员接口使用的 user DTO（包含敏感/内部字段）。
// 注意：普通用户接口不得返回 notes 等管理员备注信息。
type AdminUser struct {
	User

	Notes string `json:"notes"`
	// GroupRates 用户专属分组倍率配置
	// map[groupID]rateMultiplier
	GroupRates            map[int64]float64 `json:"group_rates,omitempty"`
	SoraStorageQuotaBytes int64             `json:"sora_storage_quota_bytes"`
	SoraStorageUsedBytes  int64             `json:"sora_storage_used_bytes"`
REDACTED

type APIKey struct {
	ID          int64      `json:"id"`
	UserID      int64      `json:"user_id"`
	Key         string     `json:"key"`
	Name        string     `json:"name"`
	GroupID     *int64     `json:"group_id"`
	Status      string     `json:"status"`
	IPWhitelist []string   `json:"ip_whitelist"`
	IPBlacklist []string   `json:"ip_blacklist"`
	LastUsedAt  *time.Time `json:"last_used_at"`
	Quota       float64    `json:"quota"`      // Quota limit in USD (0 = unlimited)
	QuotaUsed   float64    `json:"quota_used"` // Used quota amount in USD
	ExpiresAt   *time.Time `json:"expires_at"` // Expiration time (nil = never expires)
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`

	// Rate limit fields
	RateLimit5h   float64    `json:"rate_limit_5h"`
	RateLimit1d   float64    `json:"rate_limit_1d"`
	RateLimit7d   float64    `json:"rate_limit_7d"`
	Usage5h       float64    `json:"usage_5h"`
	Usage1d       float64    `json:"usage_1d"`
	Usage7d       float64    `json:"usage_7d"`
	Window5hStart *time.Time `json:"window_5h_start"`
	Window1dStart *time.Time `json:"window_1d_start"`
	Window7dStart *time.Time `json:"window_7d_start"`

	User  *User  `json:"user,omitempty"`
	Group *Group `json:"group,omitempty"`
REDACTED

type Group struct {
	ID             int64   `json:"id"`
	Name           string  `json:"name"`
	Description    string  `json:"description"`
	Platform       string  `json:"platform"`
	RateMultiplier float64 `json:"rate_multiplier"`
	IsExclusive    bool    `json:"is_exclusive"`
	Status         string  `json:"status"`

	SubscriptionType string   `json:"subscription_type"`
	DailyLimitUSD    *float64 `json:"daily_limit_usd"`
	WeeklyLimitUSD   *float64 `json:"weekly_limit_usd"`
	MonthlyLimitUSD  *float64 `json:"monthly_limit_usd"`

	// 图片生成计费配置（仅 antigravity 平台使用）
	ImagePrice1K *float64 `json:"image_price_1k"`
	ImagePrice2K *float64 `json:"image_price_2k"`
	ImagePrice4K *float64 `json:"image_price_4k"`

	// Sora 按次计费配置
	SoraImagePrice360          *float64 `json:"sora_image_price_360"`
	SoraImagePrice540          *float64 `json:"sora_image_price_540"`
	SoraVideoPricePerRequest   *float64 `json:"sora_video_price_per_request"`
	SoraVideoPricePerRequestHD *float64 `json:"sora_video_price_per_request_hd"`

	// Claude Code 客户端限制
	ClaudeCodeOnly  bool   `json:"claude_code_only"`
	FallbackGroupID *int64 `json:"fallback_group_id"`
	// 无效请求兜底分组
	FallbackGroupIDOnInvalidRequest *int64 `json:"fallback_group_id_on_invalid_request"`

	// Sora 存储配额
	SoraStorageQuotaBytes int64 `json:"sora_storage_quota_bytes"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
REDACTED

// AdminGroup 是管理员接口使用的 group DTO（包含敏感/内部字段）。
// 注意：普通用户接口不得返回 model_routing/account_count/account_groups 等内部信息。
type AdminGroup struct {
	Group

	// 模型路由配置（仅 anthropic 平台使用）
	ModelRouting        map[string][]int64 `json:"model_routing"`
	ModelRoutingEnabled bool               `json:"model_routing_enabled"`

	// MCP XML 协议注入（仅 antigravity 平台使用）
	MCPXMLInject bool `json:"mcp_xml_inject"`

	// 支持的模型系列（仅 antigravity 平台使用）
	SupportedModelScopes []string       `json:"supported_model_scopes"`
	AccountGroups        []AccountGroup `json:"account_groups,omitempty"`
	AccountCount         int64          `json:"account_count,omitempty"`

	// 分组排序
	SortOrder int `json:"sort_order"`
REDACTED

type Account struct {
	ID                 int64          `json:"id"`
	Name               string         `json:"name"`
	Notes              *string        `json:"notes"`
	Platform           string         `json:"platform"`
	Type               string         `json:"type"`
	Credentials        map[string]any `json:"credentials"`
	Extra              map[string]any `json:"extra"`
	ProxyID            *int64         `json:"proxy_id"`
	Concurrency        int            `json:"concurrency"`
	Priority           int            `json:"priority"`
	RateMultiplier     float64        `json:"rate_multiplier"`
	Status             string         `json:"status"`
	ErrorMessage       string         `json:"error_message"`
	LastUsedAt         *time.Time     `json:"last_used_at"`
	ExpiresAt          *int64         `json:"expires_at"`
	AutoPauseOnExpired bool           `json:"auto_pause_on_expired"`
	CreatedAt          time.Time      `json:"created_at"`
	UpdatedAt          time.Time      `json:"updated_at"`

	Schedulable bool `json:"schedulable"`

	RateLimitedAt    *time.Time `json:"rate_limited_at"`
	RateLimitResetAt *time.Time `json:"rate_limit_reset_at"`
	OverloadUntil    *time.Time `json:"overload_until"`

	TempUnschedulableUntil  *time.Time `json:"temp_unschedulable_until"`
	TempUnschedulableReason string     `json:"temp_unschedulable_reason"`

	SessionWindowStart  *time.Time `json:"session_window_start"`
	SessionWindowEnd    *time.Time `json:"session_window_end"`
	SessionWindowStatus string     `json:"session_window_status"`

	// 5h窗口费用控制（仅 Anthropic OAuth/SetupToken 账号有效）
	// 从 extra 字段提取，方便前端显示和编辑
	WindowCostLimit         *float64 `json:"window_cost_limit,omitempty"`
	WindowCostStickyReserve *float64 `json:"window_cost_sticky_reserve,omitempty"`

	// 会话数量控制（仅 Anthropic OAuth/SetupToken 账号有效）
	// 从 extra 字段提取，方便前端显示和编辑
	MaxSessions           *int `json:"max_sessions,omitempty"`
	SessionIdleTimeoutMin *int `json:"session_idle_timeout_minutes,omitempty"`

	// RPM 限制（仅 Anthropic OAuth/SetupToken 账号有效）
	// 从 extra 字段提取，方便前端显示和编辑
	BaseRPM          *int    `json:"base_rpm,omitempty"`
	RPMStrategy      *string `json:"rpm_strategy,omitempty"`
	RPMStickyBuffer  *int    `json:"rpm_sticky_buffer,omitempty"`
	UserMsgQueueMode *string `json:"user_msg_queue_mode,omitempty"`

	// TLS指纹伪装（仅 Anthropic OAuth/SetupToken 账号有效）
	// 从 extra 字段提取，方便前端显示和编辑
	EnableTLSFingerprint *bool `json:"enable_tls_fingerprint,omitempty"`

	// 会话ID伪装（仅 Anthropic OAuth/SetupToken 账号有效）
	// 启用后将在15分钟内固定 metadata.user_id 中的 session ID
	// 从 extra 字段提取，方便前端显示和编辑
	EnableSessionIDMasking *bool `json:"session_id_masking_enabled,omitempty"`

	// 缓存 TTL 强制替换（仅 Anthropic OAuth/SetupToken 账号有效）
	// 启用后将所有 cache creation tokens 归入指定的 TTL 类型计费
	CacheTTLOverrideEnabled *bool   `json:"cache_ttl_override_enabled,omitempty"`
	CacheTTLOverrideTarget  *string `json:"cache_ttl_override_target,omitempty"`

	Proxy         *Proxy         `json:"proxy,omitempty"`
	AccountGroups []AccountGroup `json:"account_groups,omitempty"`

	GroupIDs []int64  `json:"group_ids,omitempty"`
	Groups   []*Group `json:"groups,omitempty"`
REDACTED

type AccountGroup struct {
	AccountID int64     `json:"account_id"`
	GroupID   int64     `json:"group_id"`
	Priority  int       `json:"priority"`
	CreatedAt time.Time `json:"created_at"`

	Account *Account `json:"account,omitempty"`
	Group   *Group   `json:"group,omitempty"`
REDACTED

type Proxy struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Protocol  string    `json:"protocol"`
	Host      string    `json:"host"`
	Port      int       `json:"port"`
	Username  string    `json:"username"`
	Password  string    `json:"-"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
REDACTED

type ProxyWithAccountCount struct {
	Proxy
	AccountCount   int64  `json:"account_count"`
	LatencyMs      *int64 `json:"latency_ms,omitempty"`
	LatencyStatus  string `json:"latency_status,omitempty"`
	LatencyMessage string `json:"latency_message,omitempty"`
	IPAddress      string `json:"ip_address,omitempty"`
	Country        string `json:"country,omitempty"`
	CountryCode    string `json:"country_code,omitempty"`
	Region         string `json:"region,omitempty"`
	City           string `json:"city,omitempty"`
	QualityStatus  string `json:"quality_status,omitempty"`
	QualityScore   *int   `json:"quality_score,omitempty"`
	QualityGrade   string `json:"quality_grade,omitempty"`
	QualitySummary string `json:"quality_summary,omitempty"`
	QualityChecked *int64 `json:"quality_checked,omitempty"`
REDACTED

// AdminProxy 是管理员接口使用的 proxy DTO（包含密码等敏感字段）。
// 注意：普通接口不得使用此 DTO。
type AdminProxy struct {
	Proxy
	Password string `json:"password,omitempty"`
REDACTED

// AdminProxyWithAccountCount 是管理员接口使用的带账号统计的 proxy DTO。
type AdminProxyWithAccountCount struct {
	AdminProxy
	AccountCount   int64  `json:"account_count"`
	LatencyMs      *int64 `json:"latency_ms,omitempty"`
	LatencyStatus  string `json:"latency_status,omitempty"`
	LatencyMessage string `json:"latency_message,omitempty"`
	IPAddress      string `json:"ip_address,omitempty"`
	Country        string `json:"country,omitempty"`
	CountryCode    string `json:"country_code,omitempty"`
	Region         string `json:"region,omitempty"`
	City           string `json:"city,omitempty"`
	QualityStatus  string `json:"quality_status,omitempty"`
	QualityScore   *int   `json:"quality_score,omitempty"`
	QualityGrade   string `json:"quality_grade,omitempty"`
	QualitySummary string `json:"quality_summary,omitempty"`
	QualityChecked *int64 `json:"quality_checked,omitempty"`
REDACTED

type ProxyAccountSummary struct {
	ID       int64   `json:"id"`
	Name     string  `json:"name"`
	Platform string  `json:"platform"`
	Type     string  `json:"type"`
	Notes    *string `json:"notes,omitempty"`
REDACTED

type RedeemCode struct {
	ID        int64      `json:"id"`
	Code      string     `json:"code"`
	Type      string     `json:"type"`
	Value     float64    `json:"value"`
	Status    string     `json:"status"`
	UsedBy    *int64     `json:"used_by"`
	UsedAt    *time.Time `json:"used_at"`
	CreatedAt time.Time  `json:"created_at"`

	GroupID      *int64 `json:"group_id"`
	ValidityDays int    `json:"validity_days"`

	// Notes is only populated for admin_balance/admin_concurrency types
	// so users can see why they were charged or credited
	Notes *string `json:"notes,omitempty"`

	User  *User  `json:"user,omitempty"`
	Group *Group `json:"group,omitempty"`
REDACTED

// AdminRedeemCode 是管理员接口使用的 redeem code DTO（包含 notes 等字段）。
// 注意：普通用户接口不得返回 notes 等内部信息。
type AdminRedeemCode struct {
	RedeemCode

	Notes string `json:"notes"`
REDACTED

// UsageLog 是普通用户接口使用的 usage log DTO（不包含管理员字段）。
type UsageLog struct {
	ID        int64  `json:"id"`
	UserID    int64  `json:"user_id"`
	APIKeyID  int64  `json:"api_key_id"`
	AccountID int64  `json:"account_id"`
	RequestID string `json:"request_id"`
	Model     string `json:"model"`
	// ReasoningEffort is the request's reasoning effort level (OpenAI Responses API).
	// nil means not provided / not applicable.
	ReasoningEffort *string `json:"reasoning_effort,omitempty"`

	GroupID        *int64 `json:"group_id"`
	SubscriptionID *int64 `json:"subscription_id"`

	InputTokens         int `json:"input_tokens"`
	OutputTokens        int `json:"output_tokens"`
	CacheCreationTokens int `json:"cache_creation_tokens"`
	CacheReadTokens     int `json:"cache_read_tokens"`

	CacheCreation5mTokens int `json:"cache_creation_5m_tokens"`
	CacheCreation1hTokens int `json:"cache_creation_1h_tokens"`

	InputCost         float64 `json:"input_cost"`
	OutputCost        float64 `json:"output_cost"`
	CacheCreationCost float64 `json:"cache_creation_cost"`
	CacheReadCost     float64 `json:"cache_read_cost"`
	TotalCost         float64 `json:"total_cost"`
	ActualCost        float64 `json:"actual_cost"`
	RateMultiplier    float64 `json:"rate_multiplier"`

	BillingType  int8   `json:"billing_type"`
	RequestType  string `json:"request_type"`
	Stream       bool   `json:"stream"`
	OpenAIWSMode bool   `json:"openai_ws_mode"`
	DurationMs   *int   `json:"duration_ms"`
	FirstTokenMs *int   `json:"first_token_ms"`

	// 图片生成字段
	ImageCount int     `json:"image_count"`
	ImageSize  *string `json:"image_size"`
	MediaType  *string `json:"media_type"`

	// User-Agent
	UserAgent *string `json:"user_agent"`

	// Cache TTL Override 标记
	CacheTTLOverridden bool `json:"cache_ttl_overridden"`

	CreatedAt time.Time `json:"created_at"`

	User         *User             `json:"user,omitempty"`
	APIKey       *APIKey           `json:"api_key,omitempty"`
	Group        *Group            `json:"group,omitempty"`
	Subscription *UserSubscription `json:"subscription,omitempty"`
REDACTED

// AdminUsageLog 是管理员接口使用的 usage log DTO（包含管理员字段）。
type AdminUsageLog struct {
	UsageLog

	// AccountRateMultiplier 账号计费倍率快照（nil 表示按 1.0 处理）
	AccountRateMultiplier *float64 `json:"account_rate_multiplier"`

	// IPAddress 用户请求 IP（仅管理员可见）
	IPAddress *string `json:"ip_address,omitempty"`

	// Account 最小账号信息（避免泄露敏感字段）
	Account *AccountSummary `json:"account,omitempty"`
REDACTED

type UsageCleanupFilters struct {
	StartTime   time.Time `json:"start_time"`
	EndTime     time.Time `json:"end_time"`
	UserID      *int64    `json:"user_id,omitempty"`
	APIKeyID    *int64    `json:"api_key_id,omitempty"`
	AccountID   *int64    `json:"account_id,omitempty"`
	GroupID     *int64    `json:"group_id,omitempty"`
	Model       *string   `json:"model,omitempty"`
	RequestType *string   `json:"request_type,omitempty"`
	Stream      *bool     `json:"stream,omitempty"`
	BillingType *int8     `json:"billing_type,omitempty"`
REDACTED

type UsageCleanupTask struct {
	ID           int64               `json:"id"`
	Status       string              `json:"status"`
	Filters      UsageCleanupFilters `json:"filters"`
	CreatedBy    int64               `json:"created_by"`
	DeletedRows  int64               `json:"deleted_rows"`
	ErrorMessage *string             `json:"error_message,omitempty"`
	CanceledBy   *int64              `json:"canceled_by,omitempty"`
	CanceledAt   *time.Time          `json:"canceled_at,omitempty"`
	StartedAt    *time.Time          `json:"started_at,omitempty"`
	FinishedAt   *time.Time          `json:"finished_at,omitempty"`
	CreatedAt    time.Time           `json:"created_at"`
	UpdatedAt    time.Time           `json:"updated_at"`
REDACTED

// AccountSummary is a minimal account info for usage log display.
// It intentionally excludes sensitive fields like Credentials, Proxy, etc.
type AccountSummary struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
REDACTED

type Setting struct {
	ID        int64     `json:"id"`
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	UpdatedAt time.Time `json:"updated_at"`
REDACTED

type UserSubscription struct {
	ID      int64 `json:"id"`
	UserID  int64 `json:"user_id"`
	GroupID int64 `json:"group_id"`

	StartsAt  time.Time `json:"starts_at"`
	ExpiresAt time.Time `json:"expires_at"`
	Status    string    `json:"status"`

	DailyWindowStart   *time.Time `json:"daily_window_start"`
	WeeklyWindowStart  *time.Time `json:"weekly_window_start"`
	MonthlyWindowStart *time.Time `json:"monthly_window_start"`

	DailyUsageUSD   float64 `json:"daily_usage_usd"`
	WeeklyUsageUSD  float64 `json:"weekly_usage_usd"`
	MonthlyUsageUSD float64 `json:"monthly_usage_usd"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	User  *User  `json:"user,omitempty"`
	Group *Group `json:"group,omitempty"`
REDACTED

// AdminUserSubscription 是管理员接口使用的订阅 DTO（包含分配信息/备注等字段）。
// 注意：普通用户接口不得返回 assigned_by/assigned_at/notes/assigned_by_user 等管理员字段。
type AdminUserSubscription struct {
	UserSubscription

	AssignedBy *int64    `json:"assigned_by"`
	AssignedAt time.Time `json:"assigned_at"`
	Notes      string    `json:"notes"`

	AssignedByUser *User `json:"assigned_by_user,omitempty"`
REDACTED

type BulkAssignResult struct {
	SuccessCount  int                     `json:"success_count"`
	CreatedCount  int                     `json:"created_count"`
	ReusedCount   int                     `json:"reused_count"`
	FailedCount   int                     `json:"failed_count"`
	Subscriptions []AdminUserSubscription `json:"subscriptions"`
	Errors        []string                `json:"errors"`
	Statuses      map[string]string       `json:"statuses,omitempty"`
REDACTED

// PromoCode 注册优惠码
type PromoCode struct {
	ID          int64      `json:"id"`
	Code        string     `json:"code"`
	BonusAmount float64    `json:"bonus_amount"`
	MaxUses     int        `json:"max_uses"`
	UsedCount   int        `json:"used_count"`
	Status      string     `json:"status"`
	ExpiresAt   *time.Time `json:"expires_at"`
	Notes       string     `json:"notes"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
REDACTED

// PromoCodeUsage 优惠码使用记录
type PromoCodeUsage struct {
	ID          int64     `json:"id"`
	PromoCodeID int64     `json:"promo_code_id"`
	UserID      int64     `json:"user_id"`
	BonusAmount float64   `json:"bonus_amount"`
	UsedAt      time.Time `json:"used_at"`

	User *User `json:"user,omitempty"`
REDACTED
