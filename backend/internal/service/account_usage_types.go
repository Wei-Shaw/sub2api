package service

import (
	"context"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"golang.org/x/sync/singleflight"
)

type UsageLogRepository interface {
	// Create creates a usage log and returns whether it was actually inserted.
	// inserted is false when the insert was skipped due to conflict (idempotent retries).
	Create(ctx context.Context, log *UsageLog) (inserted bool, err error)
	GetByID(ctx context.Context, id int64) (*UsageLog, error)
	Delete(ctx context.Context, id int64) error

	ListByUser(ctx context.Context, userID int64, params pagination.PaginationParams) ([]UsageLog, *pagination.PaginationResult, error)
	ListByAPIKey(ctx context.Context, apiKeyID int64, params pagination.PaginationParams) ([]UsageLog, *pagination.PaginationResult, error)
	ListByAccount(ctx context.Context, accountID int64, params pagination.PaginationParams) ([]UsageLog, *pagination.PaginationResult, error)

	ListByUserAndTimeRange(ctx context.Context, userID int64, startTime, endTime time.Time) ([]UsageLog, *pagination.PaginationResult, error)
	ListByAPIKeyAndTimeRange(ctx context.Context, apiKeyID int64, startTime, endTime time.Time) ([]UsageLog, *pagination.PaginationResult, error)
	ListByAccountAndTimeRange(ctx context.Context, accountID int64, startTime, endTime time.Time) ([]UsageLog, *pagination.PaginationResult, error)
	ListByModelAndTimeRange(ctx context.Context, modelName string, startTime, endTime time.Time) ([]UsageLog, *pagination.PaginationResult, error)

	GetAccountWindowStats(ctx context.Context, accountID int64, startTime time.Time) (*usagestats.AccountStats, error)
	GetAccountTodayStats(ctx context.Context, accountID int64) (*usagestats.AccountStats, error)

	// Admin dashboard stats
	GetDashboardStats(ctx context.Context) (*usagestats.DashboardStats, error)
	GetUsageTrendWithFilters(ctx context.Context, startTime, endTime time.Time, granularity string, userID, apiKeyID, accountID, groupID int64, model string, requestType *int16, stream *bool, billingType *int8) ([]usagestats.TrendDataPoint, error)
	GetModelStatsWithFilters(ctx context.Context, startTime, endTime time.Time, userID, apiKeyID, accountID, groupID int64, requestType *int16, stream *bool, billingType *int8) ([]usagestats.ModelStat, error)
	GetEndpointStatsWithFilters(ctx context.Context, startTime, endTime time.Time, userID, apiKeyID, accountID, groupID int64, model string, requestType *int16, stream *bool, billingType *int8) ([]usagestats.EndpointStat, error)
	GetUpstreamEndpointStatsWithFilters(ctx context.Context, startTime, endTime time.Time, userID, apiKeyID, accountID, groupID int64, model string, requestType *int16, stream *bool, billingType *int8) ([]usagestats.EndpointStat, error)
	GetGroupStatsWithFilters(ctx context.Context, startTime, endTime time.Time, userID, apiKeyID, accountID, groupID int64, requestType *int16, stream *bool, billingType *int8) ([]usagestats.GroupStat, error)
	GetUserBreakdownStats(ctx context.Context, startTime, endTime time.Time, dim usagestats.UserBreakdownDimension, limit int) ([]usagestats.UserBreakdownItem, error)
	GetAllGroupUsageSummary(ctx context.Context, todayStart time.Time) ([]usagestats.GroupUsageSummary, error)
	GetAPIKeyUsageTrend(ctx context.Context, startTime, endTime time.Time, granularity string, limit int) ([]usagestats.APIKeyUsageTrendPoint, error)
	GetUserUsageTrend(ctx context.Context, startTime, endTime time.Time, granularity string, limit int, metric string) ([]usagestats.UserUsageTrendPoint, error)
	GetUserSpendingRanking(ctx context.Context, startTime, endTime time.Time, limit int) (*usagestats.UserSpendingRankingResponse, error)
	GetBatchUserUsageStats(ctx context.Context, userIDs []int64, startTime, endTime time.Time) (map[int64]*usagestats.BatchUserUsageStats, error)
	GetBatchAPIKeyUsageStats(ctx context.Context, apiKeyIDs []int64, startTime, endTime time.Time) (map[int64]*usagestats.BatchAPIKeyUsageStats, error)

	// User dashboard stats
	GetUserDashboardStats(ctx context.Context, userID int64) (*usagestats.UserDashboardStats, error)
	GetAPIKeyDashboardStats(ctx context.Context, apiKeyID int64) (*usagestats.UserDashboardStats, error)
	GetUserUsageTrendByUserID(ctx context.Context, userID int64, startTime, endTime time.Time, granularity string) ([]usagestats.TrendDataPoint, error)
	GetUserModelStats(ctx context.Context, userID int64, startTime, endTime time.Time) ([]usagestats.ModelStat, error)

	// Admin usage listing/stats
	ListWithFilters(ctx context.Context, params pagination.PaginationParams, filters usagestats.UsageLogFilters) ([]UsageLog, *pagination.PaginationResult, error)
	GetGlobalStats(ctx context.Context, startTime, endTime time.Time) (*usagestats.UsageStats, error)
	GetStatsWithFilters(ctx context.Context, filters usagestats.UsageLogFilters) (*usagestats.UsageStats, error)

	// Account stats
	GetAccountUsageStats(ctx context.Context, accountID int64, startTime, endTime time.Time) (*usagestats.AccountUsageStatsResponse, error)

	// Aggregated stats (optimized)
	GetUserStatsAggregated(ctx context.Context, userID int64, startTime, endTime time.Time) (*usagestats.UsageStats, error)
	GetAPIKeyStatsAggregated(ctx context.Context, apiKeyID int64, startTime, endTime time.Time) (*usagestats.UsageStats, error)
	GetAccountStatsAggregated(ctx context.Context, accountID int64, startTime, endTime time.Time) (*usagestats.UsageStats, error)
	GetModelStatsAggregated(ctx context.Context, modelName string, startTime, endTime time.Time) (*usagestats.UsageStats, error)
	GetDailyStatsAggregated(ctx context.Context, userID int64, startTime, endTime time.Time) ([]map[string]any, error)
}

type accountWindowStatsBatchReader interface {
	GetAccountWindowStatsBatch(ctx context.Context, accountIDs []int64, startTime time.Time) (map[int64]*usagestats.AccountStats, error)
}

// apiUsageCache 缓存从 Anthropic API 获取的使用率数据（utilization, resets_at）
// 同时支持缓存错误响应（负缓存），防止 429 等错误导致的重试风暴
type apiUsageCache struct {
	response  *ClaudeUsageResponse
	err       error // 非 nil 表示缓存的错误（负缓存）
	timestamp time.Time
}

// windowStatsCache 缓存从本地数据库查询的窗口统计（requests, tokens, cost）
type windowStatsCache struct {
	stats     *WindowStats
	timestamp time.Time
}

// antigravityUsageCache 缓存 Antigravity 额度数据
type antigravityUsageCache struct {
	usageInfo *UsageInfo
	timestamp time.Time
	identity  [32]byte
}

const (
	apiCacheTTL         = 3 * time.Minute
	apiErrorCacheTTL    = 1 * time.Minute        // 负缓存 TTL：429 等错误缓存 1 分钟
	antigravityErrorTTL = 1 * time.Minute        // Antigravity 错误缓存 TTL（可恢复错误）
	apiQueryMaxJitter   = 800 * time.Millisecond // 用量查询最大随机延迟
	windowStatsCacheTTL = 1 * time.Minute
	openAIProbeCacheTTL = 10 * time.Minute
	grokProbeRetryTTL   = 1 * time.Minute
	grokFreeQuotaWindow = 24 * time.Hour
)

// UsageCache 封装账户使用量相关的缓存
type UsageCache struct {
	apiCache          sync.Map           // accountID -> *apiUsageCache
	windowStatsCache  sync.Map           // accountID -> *windowStatsCache
	antigravityCache  sync.Map           // accountID -> *antigravityUsageCache
	apiFlight         singleflight.Group // 防止同一账号的并发请求击穿缓存（Anthropic）
	antigravityFlight singleflight.Group // 防止同一 Antigravity 账号的并发请求击穿缓存
	openAIProbeCache  sync.Map           // accountID -> time.Time
	grokProbeCache    sync.Map           // accountID -> last billing probe attempt
}

// NewUsageCache 创建 UsageCache 实例
func NewUsageCache() *UsageCache {
	return &UsageCache{}
}

// WindowStats 窗口期统计
//
// cost: 账号口径费用（total_cost * account_rate_multiplier）
// standard_cost: 标准费用（total_cost，不含倍率）
// user_cost: 用户/API Key 口径费用（actual_cost，受分组倍率影响）
type WindowStats struct {
	Requests     int64   `json:"requests"`
	Tokens       int64   `json:"tokens"`
	Cost         float64 `json:"cost"`
	StandardCost float64 `json:"standard_cost"`
	UserCost     float64 `json:"user_cost"`
}

// UsageProgress 使用量进度
type UsageProgress struct {
	Utilization      float64      `json:"utilization"`            // 使用率百分比 (0-100+，100表示100%)
	ResetsAt         *time.Time   `json:"resets_at"`              // 重置时间
	RemainingSeconds int          `json:"remaining_seconds"`      // 距重置剩余秒数
	WindowStats      *WindowStats `json:"window_stats,omitempty"` // 窗口期统计（从窗口开始到当前的使用量）
	UsedRequests     int64        `json:"used_requests,omitempty"`
	LimitRequests    int64        `json:"limit_requests,omitempty"`
}

// AntigravityModelQuota Antigravity 单个模型的配额信息
type AntigravityModelQuota struct {
	Utilization float64 `json:"utilization"` // 使用率 0-100，保留上游精度
	ResetTime   string  `json:"reset_time"`  // 重置时间 ISO8601
}

// AntigravityModelDetail Antigravity 单个模型的详细能力信息
type AntigravityModelDetail struct {
	DisplayName        string          `json:"display_name,omitempty"`
	SupportsImages     *bool           `json:"supports_images,omitempty"`
	SupportsThinking   *bool           `json:"supports_thinking,omitempty"`
	ThinkingBudget     *int            `json:"thinking_budget,omitempty"`
	Recommended        *bool           `json:"recommended,omitempty"`
	MaxTokens          *int            `json:"max_tokens,omitempty"`
	MaxOutputTokens    *int            `json:"max_output_tokens,omitempty"`
	SupportedMimeTypes map[string]bool `json:"supported_mime_types,omitempty"`
}

// AICredit 表示 Antigravity 账号的 AI Credits 余额信息。
type AICredit struct {
	CreditType     string  `json:"credit_type,omitempty"`
	Amount         float64 `json:"amount,omitempty"`
	MinimumBalance float64 `json:"minimum_balance,omitempty"`
}

// UsageInfo 账号使用量信息
type UsageInfo struct {
	Source             string         `json:"source,omitempty"`               // "passive" or "active"
	UpdatedAt          *time.Time     `json:"updated_at,omitempty"`           // 更新时间
	FiveHour           *UsageProgress `json:"five_hour"`                      // 5小时窗口
	SevenDay           *UsageProgress `json:"seven_day,omitempty"`            // 7天窗口
	SevenDaySonnet     *UsageProgress `json:"seven_day_sonnet,omitempty"`     // 7天Sonnet窗口
	SevenDayFable      *UsageProgress `json:"seven_day_fable,omitempty"`      // 7天Fable窗口（响应头 7d_oi）
	GeminiSharedDaily  *UsageProgress `json:"gemini_shared_daily,omitempty"`  // Gemini shared pool RPD (Google One / Code Assist)
	GeminiProDaily     *UsageProgress `json:"gemini_pro_daily,omitempty"`     // Gemini Pro 日配额
	GeminiFlashDaily   *UsageProgress `json:"gemini_flash_daily,omitempty"`   // Gemini Flash 日配额
	GeminiSharedMinute *UsageProgress `json:"gemini_shared_minute,omitempty"` // Gemini shared pool RPM (Google One / Code Assist)
	GeminiProMinute    *UsageProgress `json:"gemini_pro_minute,omitempty"`    // Gemini Pro RPM
	GeminiFlashMinute  *UsageProgress `json:"gemini_flash_minute,omitempty"`  // Gemini Flash RPM

	// Antigravity 多模型配额
	AntigravityQuota map[string]*AntigravityModelQuota `json:"antigravity_quota,omitempty"`
	// Upstream quota pools are independent of local rolling usage statistics.
	AntigravityQuotaGroups []AntigravityQuotaGroup `json:"antigravity_quota_groups,omitempty"`
	AntigravityQuotaSource string                  `json:"antigravity_quota_source,omitempty"`
	AntigravityQuotaError  string                  `json:"antigravity_quota_error,omitempty"`
	AntigravityQuotaStale  bool                    `json:"antigravity_quota_stale,omitempty"`

	// Grok / xAI 被动额度快照
	GrokRequestQuota       *xai.QuotaWindow `json:"grok_request_quota,omitempty"`
	GrokTokenQuota         *xai.QuotaWindow `json:"grok_token_quota,omitempty"`
	GrokRetryAfterSeconds  *int             `json:"grok_retry_after_seconds,omitempty"`
	GrokEntitlementStatus  string           `json:"grok_entitlement_status,omitempty"`
	GrokQuotaSnapshotState string           `json:"grok_quota_snapshot_state,omitempty"`
	GrokLastQuotaProbeAt   string           `json:"grok_last_quota_probe_at,omitempty"`
	GrokLastHeadersSeenAt  string           `json:"grok_last_headers_seen_at,omitempty"`
	GrokLastStatusCode     int              `json:"grok_last_status_code,omitempty"`
	GrokFreeTokenLimit     int64            `json:"grok_free_token_limit,omitempty"`
	GrokLocalUsage         *WindowStats     `json:"grok_local_usage,omitempty"`
	GrokLocalUsage24h      *WindowStats     `json:"grok_local_usage_24h,omitempty"`
	GrokLocalUsage7d       *WindowStats     `json:"grok_local_usage_7d,omitempty"`
	GrokLocalUsageMonthly  *WindowStats     `json:"grok_local_usage_monthly,omitempty"`
	// ThirtyDay is the official monthly billing window (used/monthlyLimit %).
	ThirtyDay   *UsageProgress      `json:"thirty_day,omitempty"`
	GrokBilling *xai.BillingSummary `json:"grok_billing,omitempty"`

	// Antigravity 账号级信息
	SubscriptionTier    string `json:"subscription_tier,omitempty"`     // 归一化订阅等级: FREE/PRO/ULTRA/UNKNOWN
	SubscriptionTierRaw string `json:"subscription_tier_raw,omitempty"` // 上游原始订阅等级名称

	// Antigravity 模型详细能力信息（与 antigravity_quota 同 key）
	AntigravityQuotaDetails map[string]*AntigravityModelDetail `json:"antigravity_quota_details,omitempty"`

	// Antigravity AI Credits 余额
	AICredits []AICredit `json:"ai_credits,omitempty"`

	// Antigravity 废弃模型转发规则 (old_model_id -> new_model_id)
	ModelForwardingRules map[string]string `json:"model_forwarding_rules,omitempty"`

	// Antigravity 账号是否被上游禁止 (HTTP 403)
	IsForbidden     bool   `json:"is_forbidden,omitempty"`
	ForbiddenReason string `json:"forbidden_reason,omitempty"`
	ForbiddenType   string `json:"forbidden_type,omitempty"` // "validation" / "violation" / "forbidden"
	ValidationURL   string `json:"validation_url,omitempty"` // 验证/申诉链接

	// 状态标记（从 ForbiddenType / HTTP 错误码推导）
	NeedsVerify bool `json:"needs_verify,omitempty"` // 需要人工验证（forbidden_type=validation）
	IsBanned    bool `json:"is_banned,omitempty"`    // 账号被封（forbidden_type=violation）
	NeedsReauth bool `json:"needs_reauth,omitempty"` // token 失效需重新授权（401）

	// 错误码（机器可读）：forbidden / unauthenticated / rate_limited / network_error
	ErrorCode string `json:"error_code,omitempty"`

	// 获取 usage 时的错误信息（降级返回，而非 500）
	Error string `json:"error,omitempty"`
}

// ClaudeUsageWindow Anthropic /api/oauth/usage 返回的单个用量窗口
type ClaudeUsageWindow struct {
	Utilization float64 `json:"utilization"`
	ResetsAt    string  `json:"resets_at"`
}

// ClaudeUsageResponse Anthropic API返回的usage结构
type ClaudeUsageResponse struct {
	FiveHour struct {
		Utilization float64 `json:"utilization"`
		ResetsAt    string  `json:"resets_at"`
	} `json:"five_hour"`
	SevenDay struct {
		Utilization float64 `json:"utilization"`
		ResetsAt    string  `json:"resets_at"`
	} `json:"seven_day"`
	SevenDaySonnet struct {
		Utilization float64 `json:"utilization"`
		ResetsAt    string  `json:"resets_at"`
	} `json:"seven_day_sonnet"`
	SevenDayOverageIncluded ClaudeUsageWindow `json:"seven_day_overage_included"`
}

// ClaudeUsageFetchOptions 包含获取 Claude 用量数据所需的所有选项
type ClaudeUsageFetchOptions struct {
	AccessToken string                  // OAuth access token
	ProxyURL    string                  // 代理 URL（可选）
	AccountID   int64                   // 账号 ID（用于连接池隔离）
	TLSProfile  *tlsfingerprint.Profile // TLS 指纹 Profile（nil 表示不启用）
	Fingerprint *Fingerprint            // 缓存的指纹信息（User-Agent 等）
}

// ClaudeUsageFetcher fetches usage data from Anthropic OAuth API
type ClaudeUsageFetcher interface {
	FetchUsage(ctx context.Context, accessToken, proxyURL string) (*ClaudeUsageResponse, error)
	FetchUsageWithOptions(ctx context.Context, opts *ClaudeUsageFetchOptions) (*ClaudeUsageResponse, error)
}

// AccountUsageService 账号使用量查询服务
type AccountUsageService struct {
	accountRepo             AccountRepository
	usageLogRepo            UsageLogRepository
	usageFetcher            ClaudeUsageFetcher
	geminiQuotaService      *GeminiQuotaService
	antigravityQuotaFetcher *AntigravityQuotaFetcher
	grokQuotaFetcher        *GrokQuotaFetcher
	grokQuotaService        *GrokQuotaService
	openAIQuotaService      *OpenAIQuotaService
	cache                   *UsageCache
	identityCache           IdentityCache
	tlsFPProfileService     *TLSFingerprintProfileService
	agentIdentityTaskMu     sync.Mutex
	agentIdentityWS         agentIdentityWSConnectionInvalidator
}
