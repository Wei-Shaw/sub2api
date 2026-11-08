// Package usagestats provides types for usage statistics and reporting.
package usagestats

import "time"

const (
	ModelSourceRequested = "requested"
	ModelSourceUpstream  = "upstream"
	ModelSourceMapping   = "mapping"
)

func IsValidModelSource(source string) bool {
	switch source {
	case ModelSourceRequested, ModelSourceUpstream, ModelSourceMapping:
		return true
	default:
		return false
	}
}

func NormalizeModelSource(source string) string {
	if IsValidModelSource(source) {
		return source
	}
	return ModelSourceRequested
}

// DashboardStats 仪表盘统计
type DashboardStats struct {
	// 用户统计
	TotalUsers    int64 `json:"total_users"`
	TodayNewUsers int64 `json:"today_new_users"` // 今日新增用户数
	ActiveUsers   int64 `json:"active_users"`    // 今日有请求的用户数
	// 小时活跃用户数（UTC 当前小时）
	HourlyActiveUsers int64 `json:"hourly_active_users"`

	// 预聚合新鲜度
	StatsUpdatedAt string `json:"stats_updated_at"`
	StatsStale     bool   `json:"stats_stale"`

	// API Key 统计
	TotalAPIKeys  int64 `json:"total_api_keys"`
	ActiveAPIKeys int64 `json:"active_api_keys"` // 状态为 active 的 API Key 数

	// 账户统计
	TotalAccounts     int64 `json:"total_accounts"`
	NormalAccounts    int64 `json:"normal_accounts"`    // 正常账户数 (schedulable=true, status=active)
	ErrorAccounts     int64 `json:"error_accounts"`     // 异常账户数 (status=error)
	RateLimitAccounts int64 `json:"ratelimit_accounts"` // 限流账户数
	OverloadAccounts  int64 `json:"overload_accounts"`  // 过载账户数

	// 累计 Token 使用统计
	TotalRequests            int64   `json:"total_requests"`
	TotalInputTokens         int64   `json:"total_input_tokens"`
	TotalOutputTokens        int64   `json:"total_output_tokens"`
	TotalCacheCreationTokens int64   `json:"total_cache_creation_tokens"`
	TotalCacheReadTokens     int64   `json:"total_cache_read_tokens"`
	TotalTokens              int64   `json:"total_tokens"`
	TotalCost                float64 `json:"total_cost"`         // 累计标准计费
	TotalActualCost          float64 `json:"total_actual_cost"`  // 累计实际扣除
	TotalAccountCost         float64 `json:"total_account_cost"` // 累计账号成本

	// 今日 Token 使用统计
	TodayRequests            int64   `json:"today_requests"`
	TodayInputTokens         int64   `json:"today_input_tokens"`
	TodayOutputTokens        int64   `json:"today_output_tokens"`
	TodayCacheCreationTokens int64   `json:"today_cache_creation_tokens"`
	TodayCacheReadTokens     int64   `json:"today_cache_read_tokens"`
	TodayTokens              int64   `json:"today_tokens"`
	TodayCost                float64 `json:"today_cost"`         // 今日标准计费
	TodayActualCost          float64 `json:"today_actual_cost"`  // 今日实际扣除
	TodayAccountCost         float64 `json:"today_account_cost"` // 今日账号成本

	// 系统运行统计
	AverageDurationMs float64 `json:"average_duration_ms"` // 平均响应时间

	// 性能指标
	Rpm int64 `json:"rpm"` // 近5分钟平均每分钟请求数
	Tpm int64 `json:"tpm"` // 近5分钟平均每分钟Token数
}

// TrendDataPoint represents a single point in trend data
type TrendDataPoint struct {
	Date                string  `json:"date"`
	Requests            int64   `json:"requests"`
	InputTokens         int64   `json:"input_tokens"`
	OutputTokens        int64   `json:"output_tokens"`
	CacheCreationTokens int64   `json:"cache_creation_tokens"`
	CacheReadTokens     int64   `json:"cache_read_tokens"`
	TotalTokens         int64   `json:"total_tokens"`
	Cost                float64 `json:"cost"`        // 标准计费
	ActualCost          float64 `json:"actual_cost"` // 实际扣除
	// UpstreamCost 是本时段内**已取到**的上游真实扣费合计（原币，A6 为美元）。
	//
	// 与 ActualCost 同币种，可直接相减得毛利；本站不做任何汇率换算。
	// 尚未反查回来的记录不计入（NULL 不参与 SUM），所以它是「已知成本」，
	// 与 ActualCost 相减得到的是**偏乐观**的毛利上界。
	UpstreamCost float64 `json:"upstream_cost"`
	// Profit 是毛利 = ActualCost - UpstreamCost，由后端算好。
	//
	// 不在前端相减：同一个数字要出现在顶部卡片、趋势图、明细行三处，
	// 各算各的迟早漂移成三个不一样的数，那是最难查的一类问题。
	Profit float64 `json:"profit"`
}

// ModelStat represents usage statistics for a single model
type ModelStat struct {
	Model               string  `json:"model"`
	Requests            int64   `json:"requests"`
	InputTokens         int64   `json:"input_tokens"`
	OutputTokens        int64   `json:"output_tokens"`
	CacheCreationTokens int64   `json:"cache_creation_tokens"`
	CacheReadTokens     int64   `json:"cache_read_tokens"`
	TotalTokens         int64   `json:"total_tokens"`
	Cost                float64 `json:"cost"`         // 标准计费
	ActualCost          float64 `json:"actual_cost"`  // 实际扣除
	AccountCost         float64 `json:"account_cost"` // 账号成本
}

// EndpointStat represents usage statistics for a single request endpoint.
type EndpointStat struct {
	Endpoint    string  `json:"endpoint"`
	Requests    int64   `json:"requests"`
	TotalTokens int64   `json:"total_tokens"`
	Cost        float64 `json:"cost"`        // 标准计费
	ActualCost  float64 `json:"actual_cost"` // 实际扣除
}

// GroupUsageSummary represents today's, yesterday's, and cumulative cost for a single group.
type GroupUsageSummary struct {
	GroupID       int64   `json:"group_id"`
	TodayCost     float64 `json:"today_cost"`
	YesterdayCost float64 `json:"yesterday_cost"`
	TotalCost     float64 `json:"total_cost"`
}

// GroupStat represents usage statistics for a single group
type GroupStat struct {
	GroupID     int64   `json:"group_id"`
	GroupName   string  `json:"group_name"`
	Requests    int64   `json:"requests"`
	TotalTokens int64   `json:"total_tokens"`
	Cost        float64 `json:"cost"`         // 标准计费
	ActualCost  float64 `json:"actual_cost"`  // 实际扣除
	AccountCost float64 `json:"account_cost"` // 账号成本
}

// UserUsageTrendPoint represents user usage trend data point
type UserUsageTrendPoint struct {
	Date       string  `json:"date"`
	UserID     int64   `json:"user_id"`
	Email      string  `json:"email"`
	Username   string  `json:"username"`
	Requests   int64   `json:"requests"`
	Tokens     int64   `json:"tokens"`
	Cost       float64 `json:"cost"`        // 标准计费
	ActualCost float64 `json:"actual_cost"` // 实际扣除
}

// UserSpendingRankingItem represents a user spending ranking row.
type UserSpendingRankingItem struct {
	UserID     int64   `json:"user_id"`
	Email      string  `json:"email"`
	Username   string  `json:"username"`
	ActualCost float64 `json:"actual_cost"` // 实际扣除
	Requests   int64   `json:"requests"`
	Tokens     int64   `json:"tokens"`
}

// UserSpendingRankingResponse represents ranking rows plus total spend for the time range.
type UserSpendingRankingResponse struct {
	Ranking         []UserSpendingRankingItem `json:"ranking"`
	TotalActualCost float64                   `json:"total_actual_cost"`
	TotalRequests   int64                     `json:"total_requests"`
	TotalTokens     int64                     `json:"total_tokens"`
}

// UserBreakdownItem represents per-user usage breakdown within a dimension (group, model, endpoint).
type UserBreakdownItem struct {
	UserID       int64   `json:"user_id"`
	Email        string  `json:"email"`
	Requests     int64   `json:"requests"`
	InputTokens  int64   `json:"input_tokens"`  // 输入 token 累计
	OutputTokens int64   `json:"output_tokens"` // 输出 token 累计
	CacheTokens  int64   `json:"cache_tokens"`  // 缓存创建 + 读取 token 累计
	TotalTokens  int64   `json:"total_tokens"`  // 输入+输出+缓存 token 累计
	Cost         float64 `json:"cost"`          // 标准计费
	ActualCost   float64 `json:"actual_cost"`   // 实际扣除
	AccountCost  float64 `json:"account_cost"`  // 账号成本
}

// UserBreakdownDimension specifies the dimension to filter for user breakdown.
type UserBreakdownDimension struct {
	GroupID      int64  // filter by group_id (>0 to enable)
	Model        string // filter by model name (non-empty to enable)
	ModelType    string // "requested", "upstream", or "mapping"
	Endpoint     string // filter by endpoint value (non-empty to enable)
	EndpointType string // "inbound", "upstream", or "path"
	// Additional filter conditions
	UserID             int64  // filter by user_id (>0 to enable)
	APIKeyID           int64  // filter by api_key_id (>0 to enable)
	AccountID          int64  // filter by account_id (>0 to enable)
	RequestType        *int16 // filter by request_type (non-nil to enable)
	Stream             *bool  // filter by stream flag (non-nil to enable)
	NativeCompactionV2 *bool  // filter by native compaction v2 flag (non-nil to enable)
	BillingType        *int8  // filter by billing_type (non-nil to enable)
	// SortBy 指定排序列(空 = 默认按 actual_cost)。合法值由 repo 层 allowlist 校验。
	SortBy string
}

// APIKeyUsageTrendPoint represents API key usage trend data point
type APIKeyUsageTrendPoint struct {
	Date     string `json:"date"`
	APIKeyID int64  `json:"api_key_id"`
	KeyName  string `json:"key_name"`
	Requests int64  `json:"requests"`
	Tokens   int64  `json:"tokens"`
}

// APIKeyDailyUsagePoint represents one day of usage for a single API key.
type APIKeyDailyUsagePoint struct {
	Date             string  `json:"date"`
	Requests         int64   `json:"requests"`
	InputTokens      int64   `json:"input_tokens"`
	OutputTokens     int64   `json:"output_tokens"`
	CacheReadTokens  int64   `json:"cache_read_tokens"`
	CacheWriteTokens int64   `json:"cache_write_tokens"`
	TotalTokens      int64   `json:"total_tokens"`
	Cost             float64 `json:"cost"`        // 标准计费
	ActualCost       float64 `json:"actual_cost"` // 实际扣除
}

// UserDashboardStats 用户仪表盘统计
type UserDashboardStats struct {
	// API Key 统计
	TotalAPIKeys  int64 `json:"total_api_keys"`
	ActiveAPIKeys int64 `json:"active_api_keys"`

	// 累计 Token 使用统计
	TotalRequests            int64   `json:"total_requests"`
	TotalInputTokens         int64   `json:"total_input_tokens"`
	TotalOutputTokens        int64   `json:"total_output_tokens"`
	TotalCacheCreationTokens int64   `json:"total_cache_creation_tokens"`
	TotalCacheReadTokens     int64   `json:"total_cache_read_tokens"`
	TotalTokens              int64   `json:"total_tokens"`
	TotalCost                float64 `json:"total_cost"`        // 累计标准计费
	TotalActualCost          float64 `json:"total_actual_cost"` // 累计实际扣除

	// 今日 Token 使用统计
	TodayRequests            int64   `json:"today_requests"`
	TodayInputTokens         int64   `json:"today_input_tokens"`
	TodayOutputTokens        int64   `json:"today_output_tokens"`
	TodayCacheCreationTokens int64   `json:"today_cache_creation_tokens"`
	TodayCacheReadTokens     int64   `json:"today_cache_read_tokens"`
	TodayTokens              int64   `json:"today_tokens"`
	TodayCost                float64 `json:"today_cost"`        // 今日标准计费
	TodayActualCost          float64 `json:"today_actual_cost"` // 今日实际扣除

	// 性能统计
	AverageDurationMs float64 `json:"average_duration_ms"`

	// 性能指标
	Rpm int64 `json:"rpm"` // 近5分钟平均每分钟请求数
	Tpm int64 `json:"tpm"` // 近5分钟平均每分钟Token数

	// 按"有效平台"维度拆分（与 ops 路径口径一致：group.platform 优先，否则 account.platform）
	ByPlatform []PlatformDashboardStats `json:"by_platform,omitempty"`
}

// PlatformDashboardStats 单个平台的用量明细。
type PlatformDashboardStats struct {
	Platform        string  `json:"platform"`
	TotalRequests   int64   `json:"total_requests"`
	TotalTokens     int64   `json:"total_tokens"`
	TotalActualCost float64 `json:"total_actual_cost"`
	TodayRequests   int64   `json:"today_requests"`
	TodayTokens     int64   `json:"today_tokens"`
	TodayActualCost float64 `json:"today_actual_cost"`
}

// UsageLogFilters represents filters for usage log queries
type UsageLogFilters struct {
	UserID    int64
	APIKeyID  int64
	AccountID int64
	GroupID   int64
	RequestID string
	Model     string
	// ModelFilterSource controls how Model is matched. Empty preserves raw usage_logs.model semantics.
	ModelFilterSource     string
	RequestType           *int16
	Stream                *bool
	NativeCompactionV2    *bool
	BillingType           *int8
	BillingMode           string
	UpstreamModelMismatch *bool
	// UpstreamReconciled 按「上游成本是否已反查回来」筛选。
	//
	// true  = 已对账（upstream_cost_fetched_at 非空）
	// false = 未对账（尚未反查到成本）
	//
	// ⚠️ false 额外要求 upstream_request_id 非空：没有上游请求 ID 的历史行
	// （对账能力上线之前的数据，可达数十万条）**本来就无法对账**，
	// 把它们算进「未对账」只会淹没真正待办的那几条。
	UpstreamReconciled *bool
	StartTime          *time.Time
	EndTime            *time.Time
	// ExactTotal requests exact COUNT(*) for pagination. Default false for fast large-table paging.
	ExactTotal bool
	// ProfitExcludedUserIDs 是「不计入盈亏」的用户名单（内部人员）。
	//
	// 语义刻意只作用于**收入**（actual_cost），不作用于**成本**（upstream_cost_original）：
	// 内部人员的余额由管理员手工调整、并没有真实付款，所以他们的「收入」是假的；
	// 但他们消耗掉的上游额度是真金白银花出去的，成本必须照实计入。因此这里做的是
	// 「收入打折、成本照算」，而不是把这些人整行排除——后者会把真实成本一起抹掉，
	// 让毛利虚高。毛利由 actual_cost - upstream_cost 后端算好，本字段只管收入那一侧。
	//
	// 放在结构体末尾是为了不破坏任何按位置初始化的既有字面量。
	ProfitExcludedUserIDs []int64
}

// UsageStats represents usage statistics
type UsageStats struct {
	TotalRequests            int64    `json:"total_requests"`
	TotalInputTokens         int64    `json:"total_input_tokens"`
	TotalOutputTokens        int64    `json:"total_output_tokens"`
	TotalCacheTokens         int64    `json:"total_cache_tokens"`
	TotalCacheCreationTokens int64    `json:"total_cache_creation_tokens"`
	TotalCacheReadTokens     int64    `json:"total_cache_read_tokens"`
	TotalTokens              int64    `json:"total_tokens"`
	TotalCost                float64  `json:"total_cost"`
	TotalActualCost          float64  `json:"total_actual_cost"`
	TotalAccountCost         *float64 `json:"total_account_cost,omitempty"`
	// TotalUpstreamCost 是所选范围内**已取到**的上游真实扣费合计（原币，A6 为美元）。
	//
	// 注意它是「已知成本」而不是「全部成本」：还没反查回来的记录不计入（NULL 不参与
	// SUM），因此它只会小于等于真实成本。与 TotalActualCost 相减得到的毛利是**偏乐观**
	// 的上界——真正的口径由同结构里的 UpstreamCostMissing 提示。
	//
	// json:"-" 是有意为之：这个结构体同时被**用户侧**的 /usage/stats 复用，
	// 而成本与毛利是经营数据，绝不能跟着普通用户的接口一起发出去。
	// 管理端由 handler 层的 adminUsageStatsResponse 显式带上这几个字段。
	TotalUpstreamCost float64 `json:"-"`
	// UpstreamCostMissing 是所选范围内「有上游请求 ID 但尚未取到成本」的记录数。
	UpstreamCostMissing int64 `json:"-"`
	// TotalProfit 是毛利 = 实收 - 已知成本。两个金额同为上游计价的原始币种，
	// 不做任何汇率换算，因此这里是可直接比较的差额。
	//
	// 由后端算好而不是让前端相减：同一个数字要在卡片、图表、明细行三处出现，
	// 各算各的迟早会出现「三处不一致」这种最难查的问题。
	TotalProfit       float64        `json:"-"`
	AverageDurationMs float64        `json:"average_duration_ms"`
	Endpoints         []EndpointStat `json:"endpoints,omitempty"`
	UpstreamEndpoints []EndpointStat `json:"upstream_endpoints,omitempty"`
	EndpointPaths     []EndpointStat `json:"endpoint_paths,omitempty"`
}

// PlatformUsage 表示某用户/某 API key 在单个"有效平台"维度的用量明细。
// Platform 取值与 ops 路径口径一致：优先 groups.platform，否则 accounts.platform。
type PlatformUsage struct {
	Platform        string  `json:"platform"`
	TodayActualCost float64 `json:"today_actual_cost"`
	TotalActualCost float64 `json:"total_actual_cost"`
}

// BatchUserUsageStats represents usage stats for a single user
type BatchUserUsageStats struct {
	UserID          int64           `json:"user_id"`
	TodayActualCost float64         `json:"today_actual_cost"`
	TotalActualCost float64         `json:"total_actual_cost"`
	ByPlatform      []PlatformUsage `json:"by_platform,omitempty"`
}

// BatchAPIKeyUsageStats represents usage stats for a single API key
type BatchAPIKeyUsageStats struct {
	APIKeyID        int64   `json:"api_key_id"`
	TodayActualCost float64 `json:"today_actual_cost"`
	TotalActualCost float64 `json:"total_actual_cost"`
}

// AccountUsageHistory represents daily usage history for an account
type AccountUsageHistory struct {
	Date       string  `json:"date"`
	Label      string  `json:"label"`
	Requests   int64   `json:"requests"`
	Tokens     int64   `json:"tokens"`
	Cost       float64 `json:"cost"`        // 标准计费（total_cost）
	ActualCost float64 `json:"actual_cost"` // 账号口径费用（total_cost * account_rate_multiplier）
	UserCost   float64 `json:"user_cost"`   // 用户口径费用（actual_cost，受分组倍率影响）
}

// AccountUsageSummary represents summary statistics for an account
type AccountUsageSummary struct {
	Days              int     `json:"days"`
	ActualDaysUsed    int     `json:"actual_days_used"`
	TotalCost         float64 `json:"total_cost"`      // 账号口径费用
	TotalUserCost     float64 `json:"total_user_cost"` // 用户口径费用
	TotalStandardCost float64 `json:"total_standard_cost"`
	TotalRequests     int64   `json:"total_requests"`
	TotalTokens       int64   `json:"total_tokens"`
	AvgDailyCost      float64 `json:"avg_daily_cost"` // 账号口径日均
	AvgDailyUserCost  float64 `json:"avg_daily_user_cost"`
	AvgDailyRequests  float64 `json:"avg_daily_requests"`
	AvgDailyTokens    float64 `json:"avg_daily_tokens"`
	AvgDurationMs     float64 `json:"avg_duration_ms"`
	Today             *struct {
		Date     string  `json:"date"`
		Cost     float64 `json:"cost"`
		UserCost float64 `json:"user_cost"`
		Requests int64   `json:"requests"`
		Tokens   int64   `json:"tokens"`
	} `json:"today"`
	HighestCostDay *struct {
		Date     string  `json:"date"`
		Label    string  `json:"label"`
		Cost     float64 `json:"cost"`
		UserCost float64 `json:"user_cost"`
		Requests int64   `json:"requests"`
	} `json:"highest_cost_day"`
	HighestRequestDay *struct {
		Date     string  `json:"date"`
		Label    string  `json:"label"`
		Requests int64   `json:"requests"`
		Cost     float64 `json:"cost"`
		UserCost float64 `json:"user_cost"`
	} `json:"highest_request_day"`
}

// AccountUsageStatsResponse represents the full usage statistics response for an account
type AccountUsageStatsResponse struct {
	History           []AccountUsageHistory `json:"history"`
	Summary           AccountUsageSummary   `json:"summary"`
	Models            []ModelStat           `json:"models"`
	Endpoints         []EndpointStat        `json:"endpoints"`
	UpstreamEndpoints []EndpointStat        `json:"upstream_endpoints"`
}
