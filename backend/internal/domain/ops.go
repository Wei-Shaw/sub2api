package domain

import (
	"errors"
	"strings"
	"time"
)

// OpsIngressRejectAggregate 是入口拒绝聚合读模型的一行（按 bucket + 维度去重计数）。
// 属 Ops/dashboard 读模型 BC；字段均为标量 FK（UserID/APIKeyID），不嵌入其他 BC 实体。
type OpsIngressRejectAggregate struct {
	ID           int64     `json:"id"`
	BucketStart  time.Time `json:"bucket_start"`
	RejectReason string    `json:"reject_reason"`
	RouteFamily  string    `json:"route_family"`
	Protocol     string    `json:"protocol"`
	ClientIP     string    `json:"client_ip"`
	UserID       *int64    `json:"user_id,omitempty"`
	APIKeyID     *int64    `json:"api_key_id,omitempty"`
	RequestCount int64     `json:"request_count"`
	FirstSeen    time.Time `json:"first_seen"`
	LastSeen     time.Time `json:"last_seen"`
}

// OpsIngressRejectFilter 过滤入口拒绝聚合读模型。
type OpsIngressRejectFilter struct {
	StartTime    *time.Time
	EndTime      *time.Time
	RejectReason string
	RouteFamily  string
	Protocol     string
	ClientIP     string
	UserID       *int64
	APIKeyID     *int64
	Page         int
	PageSize     int
}

// OpsIngressRejectList 是入口拒绝聚合的分页结果。
type OpsIngressRejectList struct {
	Items    []*OpsIngressRejectAggregate `json:"items"`
	Total    int                          `json:"total"`
	Page     int                          `json:"page"`
	PageSize int                          `json:"page_size"`
}

// --- Ops query mode (from service/ops_query_mode.go) ---

type OpsQueryMode string

const (
	OpsQueryModeAuto   OpsQueryMode = "auto"
	OpsQueryModeRaw    OpsQueryMode = "raw"
	OpsQueryModePreagg OpsQueryMode = "preagg"
)

// ErrOpsPreaggregatedNotPopulated indicates that raw logs exist for a window, but the
// pre-aggregation tables are not populated yet. This is primarily used to implement
// the forced `preagg` mode UX.
var ErrOpsPreaggregatedNotPopulated = errors.New("ops pre-aggregated tables not populated")

func ParseOpsQueryMode(raw string) OpsQueryMode {
	v := strings.ToLower(strings.TrimSpace(raw))
	switch v {
	case string(OpsQueryModeRaw):
		return OpsQueryModeRaw
	case string(OpsQueryModePreagg):
		return OpsQueryModePreagg
	default:
		return OpsQueryModeAuto
	}
}

func (m OpsQueryMode) IsValid() bool {
	switch m {
	case OpsQueryModeAuto, OpsQueryModeRaw, OpsQueryModePreagg:
		return true
	default:
		return false
	}
}

// --- Ops error/system log read models (from service/ops_models.go) ---

type OpsSystemLog struct {
	ID              int64          `json:"id"`
	CreatedAt       time.Time      `json:"created_at"`
	Host            string         `json:"host"`
	Level           string         `json:"level"`
	Component       string         `json:"component"`
	Message         string         `json:"message"`
	RequestID       string         `json:"request_id"`
	ClientRequestID string         `json:"client_request_id"`
	UserID          *int64         `json:"user_id"`
	APIKeyID        *int64         `json:"api_key_id"`
	AccountID       *int64         `json:"account_id"`
	Platform        string         `json:"platform"`
	Model           string         `json:"model"`
	Extra           map[string]any `json:"extra,omitempty"`
}

type OpsErrorLog struct {
	ID        int64     `json:"id"`
	CreatedAt time.Time `json:"created_at"`

	// Standardized classification
	// - phase: request|auth|account_auth|routing|upstream|network|internal
	// - owner: client|provider|platform
	// - source: client_request|upstream_http|gateway
	Phase string `json:"phase"`
	Type  string `json:"type"`

	Owner  string `json:"error_owner"`
	Source string `json:"error_source"`

	Severity string `json:"severity"`

	StatusCode int    `json:"status_code"`
	Platform   string `json:"platform"`
	Model      string `json:"model"`

	Resolved           bool       `json:"resolved"`
	ResolvedAt         *time.Time `json:"resolved_at"`
	ResolvedByUserID   *int64     `json:"resolved_by_user_id"`
	ResolvedByUserName string     `json:"resolved_by_user_name"`
	ResolvedStatusRaw  string     `json:"-"`

	ClientRequestID string `json:"client_request_id"`
	RequestID       string `json:"request_id"`
	Message         string `json:"message"`

	UserID      *int64 `json:"user_id"`
	UserEmail   string `json:"user_email"`
	APIKeyID    *int64 `json:"api_key_id"`
	AccountID   *int64 `json:"account_id"`
	AccountName string `json:"account_name"`
	GroupID     *int64 `json:"group_id"`
	GroupName   string `json:"group_name"`

	ClientIP    *string `json:"client_ip"`
	RequestPath string  `json:"request_path"`
	Stream      bool    `json:"stream"`

	InboundEndpoint  string `json:"inbound_endpoint"`
	UpstreamEndpoint string `json:"upstream_endpoint"`
	RequestedModel   string `json:"requested_model"`
	UpstreamModel    string `json:"upstream_model"`
	RequestType      *int16 `json:"request_type"`
	UserAgent        string `json:"user_agent"`

	// 关联 api_key 名称（LEFT JOIN api_keys 取得；软删只覆盖 key 列，name 保留，故已删 key 仍有原名）。
	APIKeyName    string `json:"api_key_name,omitempty"`
	APIKeyDeleted bool   `json:"api_key_deleted,omitempty"`
}

type OpsErrorLogDetail struct {
	OpsErrorLog

	ErrorBody string `json:"error_body"`

	// Upstream context (optional)
	UpstreamStatusCode   *int   `json:"upstream_status_code,omitempty"`
	UpstreamErrorMessage string `json:"upstream_error_message,omitempty"`
	UpstreamErrorDetail  string `json:"upstream_error_detail,omitempty"`
	UpstreamErrors       string `json:"upstream_errors,omitempty"` // JSON array (string) for display/parsing

	// Timings (optional)
	AuthLatencyMs      *int64 `json:"auth_latency_ms"`
	RoutingLatencyMs   *int64 `json:"routing_latency_ms"`
	UpstreamLatencyMs  *int64 `json:"upstream_latency_ms"`
	ResponseLatencyMs  *int64 `json:"response_latency_ms"`
	TimeToFirstTokenMs *int64 `json:"time_to_first_token_ms"`

	// vNext metric semantics
	IsBusinessLimited bool `json:"is_business_limited"`

	// Bound (non-deleted) key prefix, snapshotted at error time.
	APIKeyPrefix string `json:"api_key_prefix,omitempty"`
}

type OpsErrorLogFilter struct {
	StartTime *time.Time
	EndTime   *time.Time

	Platform  string
	GroupID   *int64
	AccountID *int64

	StatusCodes      []int
	StatusCodesOther bool
	Phase            string // Recovered provider rows bypass status>=400 only with the explicit opt-in below.
	Owner            string
	Source           string
	Resolved         *bool
	Query            string
	UserQuery        string // Search by user email

	// Optional correlation keys for exact matching.
	RequestID       string
	ClientRequestID string

	// User-scoped filters (used by the user-facing error requests endpoint and
	// by admin drill-down from the usage page).
	UserID   *int64
	APIKeyID *int64

	// Model matches against requested_model first, then model.
	Model string
	// ModelFuzzy 为 true 时 Model 走 ILIKE 模糊匹配（仅用户端启用）；false（默认）保持精确 =，管理端语义不变。
	ModelFuzzy bool

	// ExcludeCountTokens drops count_tokens probe errors (is_count_tokens=true).
	ExcludeCountTokens bool

	// IncludeRecoveredUpstream explicitly exempts provider-health phases
	// (upstream and account_auth) from the status>=400 guard. Ops provider
	// health lists need status<400 recovered rows; request-error endpoints do
	// not set this flag and retain client-error semantics.
	IncludeRecoveredUpstream bool

	// ErrorPhasesAny / ErrorTypesAny add plain ANY() filters WITHOUT touching the
	// special-cased single `Phase` field. With IncludeRecoveredUpstream, an ANY
	// list containing only upstream/account_auth also bypasses status>=400.
	// NOTE: these ANY filters do NOT bypass status>=400; records with error_phase='upstream'
	// but status_code<400 (recovered upstream errors) remain excluded.
	// Used to map user-facing coarse categories to backend conditions.
	ErrorPhasesAny []string
	ErrorTypesAny  []string

	// View controls error categorization for list endpoints.
	// - errors: show actionable errors (exclude business-limited / 429 / 529)
	// - excluded: only show excluded errors
	// - all: show everything
	View string

	Page     int
	PageSize int

	// SortBy/SortOrder: server-side sorting aligned with the usage-log list.
	// Repo whitelists columns (created_at/model/status_code); anything else
	// falls back to created_at. SortOrder is "asc"/"desc" (default desc).
	SortBy    string
	SortOrder string
}

// SetSort normalizes raw sort_by/sort_order query values into the filter.
// Shared by the admin and user-facing error list handlers.
func (f *OpsErrorLogFilter) SetSort(sortBy, sortOrder string) {
	f.SortBy = strings.TrimSpace(sortBy)
	f.SortOrder = strings.TrimSpace(sortOrder)
}

type OpsErrorLogList struct {
	Errors   []*OpsErrorLog `json:"errors"`
	Total    int            `json:"total"`
	Page     int            `json:"page"`
	PageSize int            `json:"page_size"`
}

// --- Ops write-model inputs + system metrics snapshots (from service/ops_port.go) ---

type OpsInsertErrorLogInput struct {
	RequestID       string
	ClientRequestID string

	UserID    *int64
	APIKeyID  *int64
	AccountID *int64
	GroupID   *int64
	ClientIP  *string

	Platform    string
	Model       string
	RequestPath string
	Stream      bool
	// InboundEndpoint is the normalized client-facing API endpoint path, e.g. /v1/chat/completions.
	InboundEndpoint string
	// UpstreamEndpoint is the normalized upstream endpoint path, e.g. /v1/responses.
	UpstreamEndpoint string
	// RequestedModel is the client-requested model name before mapping.
	RequestedModel string
	// UpstreamModel is the actual model sent to upstream after mapping. Empty means no mapping.
	UpstreamModel string
	// RequestType is the granular request type: 0=unknown, 1=sync, 2=stream, 3=ws_v2.
	// Matches service.RequestType enum semantics from usage_log.go.
	RequestType *int16
	UserAgent   string

	ErrorPhase        string
	ErrorType         string
	Severity          string
	StatusCode        int
	IsBusinessLimited bool
	IsCountTokens     bool // 是否为 count_tokens 请求

	ErrorMessage string
	ErrorBody    string

	ErrorSource string
	ErrorOwner  string

	UpstreamStatusCode   *int
	UpstreamErrorMessage *string
	UpstreamErrorDetail  *string
	// UpstreamErrors captures all upstream error attempts observed during handling this request.
	// It is populated during request processing (gin context) and sanitized+serialized by OpsService.
	UpstreamErrors []*OpsUpstreamErrorEvent
	// UpstreamErrorsJSON is the sanitized JSON string stored into ops_error_logs.upstream_errors.
	// It is set by OpsService.RecordError before persisting.
	UpstreamErrorsJSON *string

	AuthLatencyMs      *int64
	RoutingLatencyMs   *int64
	UpstreamLatencyMs  *int64
	ResponseLatencyMs  *int64
	TimeToFirstTokenMs *int64

	CreatedAt time.Time

	// 有效(未删除)key 报错时快照的 key 脱敏前缀(前 8 位)。
	// 落库快照而非读时 JOIN:key 之后被删(key 列被 tombstone 覆盖)仍保留当时前缀。
	APIKeyPrefix string
}

type OpsInsertSystemMetricsInput struct {
	CreatedAt     time.Time
	WindowMinutes int

	Platform *string
	GroupID  *int64

	SuccessCount         int64
	ErrorCountTotal      int64
	BusinessLimitedCount int64
	ErrorCountSLA        int64

	UpstreamErrorCountExcl429529 int64
	Upstream429Count             int64
	Upstream529Count             int64

	TokenConsumed      int64
	AccountSwitchCount int64

	QPS *float64
	TPS *float64

	DurationP50Ms *int
	DurationP90Ms *int
	DurationP95Ms *int
	DurationP99Ms *int
	DurationAvgMs *float64
	DurationMaxMs *int

	TTFTP50Ms *int
	TTFTP90Ms *int
	TTFTP95Ms *int
	TTFTP99Ms *int
	TTFTAvgMs *float64
	TTFTMaxMs *int

	CPUUsagePercent    *float64
	MemoryUsedMB       *int64
	MemoryTotalMB      *int64
	MemoryUsagePercent *float64

	DBOK    *bool
	RedisOK *bool

	RedisConnTotal *int
	RedisConnIdle  *int

	DBConnActive  *int
	DBConnIdle    *int
	DBConnWaiting *int

	GoroutineCount        *int
	ConcurrencyQueueDepth *int
}

type OpsInsertSystemLogInput struct {
	CreatedAt       time.Time
	Host            string
	Level           string
	Component       string
	Message         string
	RequestID       string
	ClientRequestID string
	UserID          *int64
	APIKeyID        *int64
	AccountID       *int64
	Platform        string
	Model           string
	ExtraJSON       string
}

type OpsSystemLogFilter struct {
	StartTime *time.Time
	EndTime   *time.Time
	Host      string

	Level     string
	Component string

	RequestID       string
	ClientRequestID string
	UserID          *int64
	APIKeyID        *int64
	AccountID       *int64
	Platform        string
	Model           string
	Query           string

	Page     int
	PageSize int
}

type OpsSystemLogCleanupFilter struct {
	StartTime *time.Time
	EndTime   *time.Time
	Host      string

	Level     string
	Component string

	RequestID       string
	ClientRequestID string
	UserID          *int64
	APIKeyID        *int64
	AccountID       *int64
	Platform        string
	Model           string
	Query           string
}

type OpsSystemLogList struct {
	Logs     []*OpsSystemLog `json:"logs"`
	Total    int             `json:"total"`
	Page     int             `json:"page"`
	PageSize int             `json:"page_size"`
}

type OpsSystemLogCleanupAudit struct {
	CreatedAt   time.Time
	OperatorID  int64
	Conditions  string
	DeletedRows int64
}

type OpsSystemMetricsSnapshot struct {
	ID            int64     `json:"id"`
	CreatedAt     time.Time `json:"created_at"`
	WindowMinutes int       `json:"window_minutes"`

	CPUUsagePercent    *float64 `json:"cpu_usage_percent"`
	MemoryUsedMB       *int64   `json:"memory_used_mb"`
	MemoryTotalMB      *int64   `json:"memory_total_mb"`
	MemoryUsagePercent *float64 `json:"memory_usage_percent"`

	DBOK    *bool `json:"db_ok"`
	RedisOK *bool `json:"redis_ok"`

	// Config-derived limits (best-effort). These are not historical metrics; they help UI render "current vs max".
	DBMaxOpenConns *int `json:"db_max_open_conns"`
	RedisPoolSize  *int `json:"redis_pool_size"`

	RedisConnTotal *int `json:"redis_conn_total"`
	RedisConnIdle  *int `json:"redis_conn_idle"`

	DBConnActive  *int `json:"db_conn_active"`
	DBConnIdle    *int `json:"db_conn_idle"`
	DBConnWaiting *int `json:"db_conn_waiting"`

	GoroutineCount        *int   `json:"goroutine_count"`
	ConcurrencyQueueDepth *int   `json:"concurrency_queue_depth"`
	AccountSwitchCount    *int64 `json:"account_switch_count"`
}

type OpsUpsertJobHeartbeatInput struct {
	JobName string

	LastRunAt      *time.Time
	LastSuccessAt  *time.Time
	LastErrorAt    *time.Time
	LastError      *string
	LastDurationMs *int64

	// LastResult is an optional human-readable summary of the last successful run.
	LastResult *string
}

type OpsJobHeartbeat struct {
	JobName string `json:"job_name"`

	LastRunAt      *time.Time `json:"last_run_at"`
	LastSuccessAt  *time.Time `json:"last_success_at"`
	LastErrorAt    *time.Time `json:"last_error_at"`
	LastError      *string    `json:"last_error"`
	LastDurationMs *int64     `json:"last_duration_ms"`
	LastResult     *string    `json:"last_result"`

	UpdatedAt time.Time `json:"updated_at"`
}

type OpsWindowStats struct {
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`

	SuccessCount    int64 `json:"success_count"`
	ErrorCountTotal int64 `json:"error_count_total"`
	TokenConsumed   int64 `json:"token_consumed"`
}

// --- Ops upstream error event (pure-scalar read model from service/ops_upstream_context.go) ---
// Impure gin-context helpers from the same source file STAY in the service package.

// OpsUpstreamErrorEvent describes one upstream error attempt during a single gateway request.
// It is stored in ops_error_logs.upstream_errors as a JSON array.
type OpsUpstreamErrorEvent struct {
	AtUnixMs int64 `json:"at_unix_ms,omitempty"`

	// Passthrough 表示本次请求是否命中“原样透传（仅替换认证）”分支。
	// 该字段用于排障与灰度评估；存入 JSON，不涉及 DB schema 变更。
	Passthrough bool `json:"passthrough,omitempty"`

	// Context
	Platform    string `json:"platform,omitempty"`
	AccountID   int64  `json:"account_id,omitempty"`
	AccountName string `json:"account_name,omitempty"`

	// Outcome
	UpstreamStatusCode int    `json:"upstream_status_code,omitempty"`
	UpstreamRequestID  string `json:"upstream_request_id,omitempty"`

	// UpstreamURL is the actual upstream URL that was called (host + path, query/fragment stripped).
	// Helps debug 404/routing errors by showing which endpoint was targeted.
	UpstreamURL string `json:"upstream_url,omitempty"`

	// Best-effort upstream response capture (sanitized+trimmed).
	UpstreamResponseBody string `json:"upstream_response_body,omitempty"`

	// Kind: http_error | request_error | retry_exhausted | failover
	Kind string `json:"kind,omitempty"`
	// Stage/Scope/Reason distinguish credential acquisition from inference
	// without overloading upstream_status_code with a synthetic HTTP status.
	Stage  string `json:"stage,omitempty"`
	Scope  string `json:"scope,omitempty"`
	Reason string `json:"reason,omitempty"`

	Message string `json:"message,omitempty"`
	Detail  string `json:"detail,omitempty"`
}

// --- Ops dashboard models (from service/ops_dashboard_models.go) ---

type OpsDashboardFilter struct {
	StartTime time.Time
	EndTime   time.Time

	Platform string
	GroupID  *int64

	// QueryMode controls whether dashboard queries should use raw logs or pre-aggregated tables.
	// Expected values: auto/raw/preagg (see OpsQueryMode).
	QueryMode OpsQueryMode
}

type OpsRateSummary struct {
	Current float64 `json:"current"`
	Peak    float64 `json:"peak"`
	Avg     float64 `json:"avg"`
}

type OpsPercentiles struct {
	P50 *int `json:"p50_ms"`
	P90 *int `json:"p90_ms"`
	P95 *int `json:"p95_ms"`
	P99 *int `json:"p99_ms"`
	Avg *int `json:"avg_ms"`
	Max *int `json:"max_ms"`
}

type OpsDashboardOverview struct {
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
	Platform  string    `json:"platform"`
	GroupID   *int64    `json:"group_id"`

	// HealthScore is a backend-computed overall health score (0-100).
	// It is derived from the monitored metrics in this overview, plus best-effort system metrics/job heartbeats.
	HealthScore int `json:"health_score"`

	// Latest system-level snapshot (window=1m, global).
	SystemMetrics *OpsSystemMetricsSnapshot `json:"system_metrics"`

	// Background jobs health (heartbeats).
	JobHeartbeats []*OpsJobHeartbeat `json:"job_heartbeats"`

	SuccessCount         int64 `json:"success_count"`
	ErrorCountTotal      int64 `json:"error_count_total"`
	BusinessLimitedCount int64 `json:"business_limited_count"`

	ErrorCountSLA     int64 `json:"error_count_sla"`
	RequestCountTotal int64 `json:"request_count_total"`
	RequestCountSLA   int64 `json:"request_count_sla"`

	TokenConsumed int64 `json:"token_consumed"`

	SLA                          float64 `json:"sla"`
	ErrorRate                    float64 `json:"error_rate"`
	UpstreamErrorRate            float64 `json:"upstream_error_rate"`
	UpstreamErrorCountExcl429529 int64   `json:"upstream_error_count_excl_429_529"`
	Upstream429Count             int64   `json:"upstream_429_count"`
	Upstream529Count             int64   `json:"upstream_529_count"`

	QPS OpsRateSummary `json:"qps"`
	TPS OpsRateSummary `json:"tps"`

	Duration OpsPercentiles `json:"duration"`
	TTFT     OpsPercentiles `json:"ttft"`
}

type OpsLatencyHistogramBucket struct {
	Range string `json:"range"`
	Count int64  `json:"count"`
}

// OpsLatencyHistogramResponse is a coarse latency distribution histogram (success requests only).
// It is used by the Ops dashboard to quickly identify tail latency regressions.
type OpsLatencyHistogramResponse struct {
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
	Platform  string    `json:"platform"`
	GroupID   *int64    `json:"group_id"`

	TotalRequests int64                        `json:"total_requests"`
	Buckets       []*OpsLatencyHistogramBucket `json:"buckets"`
}

// --- Ops trend models (from service/ops_trend_models.go) ---

type OpsThroughputTrendPoint struct {
	BucketStart   time.Time `json:"bucket_start"`
	RequestCount  int64     `json:"request_count"`
	TokenConsumed int64     `json:"token_consumed"`
	SwitchCount   int64     `json:"switch_count"`
	QPS           float64   `json:"qps"`
	TPS           float64   `json:"tps"`
}

type OpsThroughputPlatformBreakdownItem struct {
	Platform      string `json:"platform"`
	RequestCount  int64  `json:"request_count"`
	TokenConsumed int64  `json:"token_consumed"`
}

type OpsThroughputGroupBreakdownItem struct {
	GroupID       int64  `json:"group_id"`
	GroupName     string `json:"group_name"`
	RequestCount  int64  `json:"request_count"`
	TokenConsumed int64  `json:"token_consumed"`
}

type OpsThroughputTrendResponse struct {
	Bucket string `json:"bucket"`

	Points []*OpsThroughputTrendPoint `json:"points"`

	// Optional drilldown helpers:
	// - When no platform/group is selected: returns totals by platform.
	// - When platform is selected but group is not: returns top groups in that platform.
	ByPlatform []*OpsThroughputPlatformBreakdownItem `json:"by_platform,omitempty"`
	TopGroups  []*OpsThroughputGroupBreakdownItem    `json:"top_groups,omitempty"`
}

type OpsErrorTrendPoint struct {
	BucketStart time.Time `json:"bucket_start"`

	ErrorCountTotal      int64 `json:"error_count_total"`
	BusinessLimitedCount int64 `json:"business_limited_count"`
	ErrorCountSLA        int64 `json:"error_count_sla"`

	UpstreamErrorCountExcl429529 int64 `json:"upstream_error_count_excl_429_529"`
	Upstream429Count             int64 `json:"upstream_429_count"`
	Upstream529Count             int64 `json:"upstream_529_count"`
}

type OpsErrorTrendResponse struct {
	Bucket string                `json:"bucket"`
	Points []*OpsErrorTrendPoint `json:"points"`
}

type OpsErrorDistributionItem struct {
	StatusCode      int   `json:"status_code"`
	Total           int64 `json:"total"`
	SLA             int64 `json:"sla"`
	BusinessLimited int64 `json:"business_limited"`
}

type OpsErrorDistributionResponse struct {
	Total int64                       `json:"total"`
	Items []*OpsErrorDistributionItem `json:"items"`
}

// --- Ops alert models (from service/ops_alert_models.go) ---

const (
	OpsAlertStatusFiring         = "firing"
	OpsAlertStatusResolved       = "resolved"
	OpsAlertStatusManualResolved = "manual_resolved"
)

type OpsAlertRule struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`

	Enabled  bool   `json:"enabled"`
	Severity string `json:"severity"`

	MetricType string  `json:"metric_type"`
	Operator   string  `json:"operator"`
	Threshold  float64 `json:"threshold"`

	WindowMinutes    int `json:"window_minutes"`
	SustainedMinutes int `json:"sustained_minutes"`
	CooldownMinutes  int `json:"cooldown_minutes"`

	NotifyEmail bool `json:"notify_email"`

	Filters map[string]any `json:"filters,omitempty"`

	LastTriggeredAt *time.Time `json:"last_triggered_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type OpsAlertEvent struct {
	ID       int64  `json:"id"`
	RuleID   int64  `json:"rule_id"`
	Severity string `json:"severity"`
	Status   string `json:"status"`

	Title       string `json:"title"`
	Description string `json:"description"`

	MetricValue    *float64 `json:"metric_value,omitempty"`
	ThresholdValue *float64 `json:"threshold_value,omitempty"`

	Dimensions map[string]any `json:"dimensions,omitempty"`

	FiredAt    time.Time  `json:"fired_at"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`

	EmailSent bool      `json:"email_sent"`
	CreatedAt time.Time `json:"created_at"`
}

type OpsAlertSilence struct {
	ID int64 `json:"id"`

	RuleID   int64   `json:"rule_id"`
	Platform string  `json:"platform"`
	GroupID  *int64  `json:"group_id,omitempty"`
	Region   *string `json:"region,omitempty"`

	Until  time.Time `json:"until"`
	Reason string    `json:"reason"`

	CreatedBy *int64    `json:"created_by,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type OpsAlertEventFilter struct {
	Limit int

	// Cursor pagination (descending by fired_at, then id).
	BeforeFiredAt *time.Time
	BeforeID      *int64

	// Optional filters.
	Status    string
	Severity  string
	EmailSent *bool

	StartTime *time.Time
	EndTime   *time.Time

	// Dimensions filters (best-effort).
	Platform string
	GroupID  *int64
}

// --- Ops request details (from service/ops_request_details.go) ---

type OpsRequestKind string

const (
	OpsRequestKindSuccess OpsRequestKind = "success"
	OpsRequestKindError   OpsRequestKind = "error"
)

// OpsRequestDetail is a request-level view across success (usage_logs) and error (ops_error_logs).
// It powers "request drilldown" UIs without exposing full request bodies for successful requests.
type OpsRequestDetail struct {
	Kind      OpsRequestKind `json:"kind"`
	CreatedAt time.Time      `json:"created_at"`
	RequestID string         `json:"request_id"`

	Platform string `json:"platform,omitempty"`
	Model    string `json:"model,omitempty"`

	DurationMs *int `json:"duration_ms,omitempty"`
	StatusCode *int `json:"status_code,omitempty"`

	// When Kind == "error", ErrorID links to /admin/ops/errors/:id.
	ErrorID *int64 `json:"error_id,omitempty"`

	Phase    string `json:"phase,omitempty"`
	Severity string `json:"severity,omitempty"`
	Message  string `json:"message,omitempty"`

	UserID    *int64 `json:"user_id,omitempty"`
	APIKeyID  *int64 `json:"api_key_id,omitempty"`
	AccountID *int64 `json:"account_id,omitempty"`
	GroupID   *int64 `json:"group_id,omitempty"`

	Stream bool `json:"stream"`
}

type OpsRequestDetailFilter struct {
	StartTime *time.Time
	EndTime   *time.Time

	// kind: success|error|all
	Kind string

	Platform string
	GroupID  *int64

	UserID    *int64
	APIKeyID  *int64
	AccountID *int64

	Model     string
	RequestID string
	Query     string

	MinDurationMs *int
	MaxDurationMs *int

	// Sort: created_at_desc (default) or duration_desc.
	Sort string

	Page     int
	PageSize int
}

func (f *OpsRequestDetailFilter) Normalize() (page, pageSize int, startTime, endTime time.Time) {
	page = 1
	pageSize = 50
	endTime = time.Now()
	startTime = endTime.Add(-1 * time.Hour)

	if f == nil {
		return page, pageSize, startTime, endTime
	}

	if f.Page > 0 {
		page = f.Page
	}
	if f.PageSize > 0 {
		pageSize = f.PageSize
	}
	if pageSize > 100 {
		pageSize = 100
	}

	if f.EndTime != nil {
		endTime = *f.EndTime
	}
	if f.StartTime != nil {
		startTime = *f.StartTime
	} else if f.EndTime != nil {
		startTime = endTime.Add(-1 * time.Hour)
	}

	if startTime.After(endTime) {
		startTime, endTime = endTime, startTime
	}

	return page, pageSize, startTime, endTime
}

type OpsRequestDetailList struct {
	Items    []*OpsRequestDetail `json:"items"`
	Total    int64               `json:"total"`
	Page     int                 `json:"page"`
	PageSize int                 `json:"page_size"`
}

// --- Ops OpenAI token stats (from service/ops_openai_token_stats_models.go) ---

type OpsOpenAITokenStatsFilter struct {
	TimeRange string
	StartTime time.Time
	EndTime   time.Time

	Platform string
	GroupID  *int64

	// Pagination mode (default): page/page_size
	Page     int
	PageSize int

	// TopN mode: top_n
	TopN int
}

func (f *OpsOpenAITokenStatsFilter) IsTopNMode() bool {
	return f != nil && f.TopN > 0
}

type OpsOpenAITokenStatsItem struct {
	Model                  string   `json:"model"`
	RequestCount           int64    `json:"request_count"`
	AvgTokensPerSec        *float64 `json:"avg_tokens_per_sec"`
	AvgFirstTokenMs        *float64 `json:"avg_first_token_ms"`
	TotalOutputTokens      int64    `json:"total_output_tokens"`
	AvgDurationMs          int64    `json:"avg_duration_ms"`
	RequestsWithFirstToken int64    `json:"requests_with_first_token"`
}

type OpsOpenAITokenStatsResponse struct {
	TimeRange string    `json:"time_range"`
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`

	Platform string `json:"platform,omitempty"`
	GroupID  *int64 `json:"group_id,omitempty"`

	Items []*OpsOpenAITokenStatsItem `json:"items"`

	// Total model rows before pagination/topN trimming.
	Total int64 `json:"total"`

	// Pagination mode metadata.
	Page     int `json:"page,omitempty"`
	PageSize int `json:"page_size,omitempty"`

	// TopN mode metadata.
	TopN *int `json:"top_n,omitempty"`
}

// --- Ops realtime traffic (from service/ops_realtime_traffic_models.go) ---

// OpsRealtimeTrafficSummary is a lightweight summary used by the Ops dashboard "Realtime Traffic" card.
// It reports QPS/TPS current/peak/avg for the requested time window.
type OpsRealtimeTrafficSummary struct {
	// Window is a normalized label (e.g. "1min", "5min", "30min", "1h").
	Window string `json:"window"`

	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`

	Platform string `json:"platform"`
	GroupID  *int64 `json:"group_id"`

	QPS OpsRateSummary `json:"qps"`
	TPS OpsRateSummary `json:"tps"`
}
