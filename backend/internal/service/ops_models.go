package service

import "time"

type OpsSystemLog struct {
	ID              int64          `json:"id"`
	CreatedAt       time.Time      `json:"created_at"`
	Level           string         `json:"level"`
	Component       string         `json:"component"`
	Message         string         `json:"message"`
	RequestID       string         `json:"request_id"`
	ClientRequestID string         `json:"client_request_id"`
	UserID          *int64         `json:"user_id"`
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
}

type OpsErrorLogDetail struct {
	OpsErrorLog

	ErrorBody string `json:"error_body"`
	UserAgent string `json:"user_agent"`

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

	// Deleted key owner info (populated when INVALID_API_KEY and key was previously deleted)
	AttemptedKeyPrefix    string `json:"attempted_key_prefix,omitempty"`
	DeletedKeyOwnerUserID *int64 `json:"deleted_key_owner_user_id,omitempty"`
	DeletedKeyOwnerEmail  string `json:"deleted_key_owner_email,omitempty"`
	DeletedKeyName        string `json:"deleted_key_name,omitempty"`

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
	Phase            string
	Owner            string
	Source           string
	Resolved         *bool
	Query            string
	UserQuery        string // Search by user email

	// Optional correlation keys for exact matching.
	RequestID       string
	ClientRequestID string

	// View controls error categorization for list endpoints.
	// - errors: show actionable errors (exclude business-limited / 429 / 529)
	// - excluded: only show excluded errors
	// - all: show everything
	View string

	Page     int
	PageSize int
}

type OpsErrorLogList struct {
	Errors   []*OpsErrorLog `json:"errors"`
	Total    int            `json:"total"`
	Page     int            `json:"page"`
	PageSize int            `json:"page_size"`
}
