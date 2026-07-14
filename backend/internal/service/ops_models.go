package service

import (
	"strings"
	"time"
)

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
	// - phase: request|auth|routing|upstream|network|internal
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

	// 已删除 KEY 所有者（INVALID_API_KEY 且该 key 曾存在时的归因快照）。
	// 认证失败行 user_id 为空，列表用户列以此回退显示所有者。
	DeletedKeyOwnerUserID *int64 `json:"deleted_key_owner_user_id,omitempty"`
	DeletedKeyOwnerEmail  string `json:"deleted_key_owner_email,omitempty"`
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

	// Deleted key owner info (populated when INVALID_API_KEY and key was previously deleted).
	// OwnerUserID/OwnerEmail 已上移到 OpsErrorLog（列表用户列回退需要）。
	AttemptedKeyPrefix string `json:"attempted_key_prefix,omitempty"`
	DeletedKeyName     string `json:"deleted_key_name,omitempty"`

	// Bound (non-deleted) key prefix, snapshotted at error time; mutually exclusive with AttemptedKeyPrefix.
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
	Phase            string // Special: Phase=="upstream" bypasses status>=400 clause; do not set together with ErrorPhasesAny.
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

	// MatchDeletedKeyOwner: 用户侧专用。UserID 设置且为 true 时,归属从 user_id=UserID
	// 放宽为 (user_id=UserID OR deleted_key_owner_user_id=UserID),使原所有者能看到
	// 自己「已删除 key 认证失败」的记录。admin 路径不设此开关 → 行为不变。
	MatchDeletedKeyOwner bool

	// Model matches against requested_model first, then model.
	Model string
	// ModelFuzzy 为 true 时 Model 走 ILIKE 模糊匹配（仅用户端启用）；false（默认）保持精确 =，管理端语义不变。
	ModelFuzzy bool

	// ExcludeCountTokens drops count_tokens probe errors (is_count_tokens=true).
	ExcludeCountTokens bool

	// IncludeRecoveredUpstream 显式豁免 status>=400 守卫（仅在 Phase=="upstream" 时生效）：
	// ops 专用上游错误列表需要看到 status<400 的 recovered upstream 行。
	// 请求错误语义的端点不设此开关，phase=upstream 过滤照常生效且守卫保留。
	IncludeRecoveredUpstream bool

	// ErrorPhasesAny / ErrorTypesAny add plain ANY() filters WITHOUT touching the
	// special-cased single `Phase` field (only Phase=="upstream" with
	// IncludeRecoveredUpstream bypasses the status>=400 clause).
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
