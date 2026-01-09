package service

import "time"

type OpsErrorLog struct {
	ID        int64     `json:"id"`
	CreatedAt time.Time `json:"created_at"`

	Phase    string `json:"phase"`
	Type     string `json:"type"`
	Severity string `json:"severity"`

	StatusCode int    `json:"status_code"`
	Platform   string `json:"platform"`
	Model      string `json:"model"`

	LatencyMs *int `json:"latency_ms"`

	ClientRequestID string `json:"client_request_id"`
	RequestID       string `json:"request_id"`
	Message         string `json:"message"`

	UserID    *int64 `json:"user_id"`
	APIKeyID  *int64 `json:"api_key_id"`
	AccountID *int64 `json:"account_id"`
	GroupID   *int64 `json:"group_id"`

	ClientIP    *string `json:"client_ip"`
	RequestPath string  `json:"request_path"`
	Stream      bool    `json:"stream"`
REDACTED

type OpsErrorLogDetail struct {
	OpsErrorLog

	ErrorBody string `json:"error_body"`
	UserAgent string `json:"user_agent"`

	// Timings (optional)
	AuthLatencyMs       *int64 `json:"auth_latency_ms"`
	RoutingLatencyMs    *int64 `json:"routing_latency_ms"`
	UpstreamLatencyMs   *int64 `json:"upstream_latency_ms"`
	ResponseLatencyMs   *int64 `json:"response_latency_ms"`
	TimeToFirstTokenMs  *int64 `json:"time_to_first_token_ms"`

	// Retry context
	RequestBody          string `json:"request_body"`
	RequestBodyTruncated bool   `json:"request_body_truncated"`
	RequestBodyBytes     *int   `json:"request_body_bytes"`
	RequestHeaders       string `json:"request_headers,omitempty"`

	// vNext metric semantics
	IsBusinessLimited bool `json:"is_business_limited"`
REDACTED

type OpsErrorLogFilter struct {
	StartTime *time.Time
	EndTime   *time.Time

	Platform string
	GroupID  *int64
	AccountID *int64

	StatusCodes []int
	Phase       string
	Query       string

	Page     int
	PageSize int
REDACTED

type OpsErrorLogList struct {
	Errors   []*OpsErrorLog `json:"errors"`
	Total    int           `json:"total"`
	Page     int           `json:"page"`
	PageSize int           `json:"page_size"`
REDACTED

type OpsRetryAttempt struct {
	ID        int64     `json:"id"`
	CreatedAt time.Time `json:"created_at"`

	RequestedByUserID int64  `json:"requested_by_user_id"`
	SourceErrorID     int64  `json:"source_error_id"`
	Mode              string `json:"mode"`
	PinnedAccountID   *int64 `json:"pinned_account_id"`

	Status     string     `json:"status"`
	StartedAt  *time.Time `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`
	DurationMs *int64     `json:"duration_ms"`

	ResultRequestID *string `json:"result_request_id"`
	ResultErrorID   *int64  `json:"result_error_id"`

	ErrorMessage *string `json:"error_message"`
REDACTED

type OpsRetryResult struct {
	AttemptID int64 `json:"attempt_id"`
	Mode      string `json:"mode"`
	Status    string `json:"status"`

	PinnedAccountID *int64 `json:"pinned_account_id"`
	UsedAccountID   *int64 `json:"used_account_id"`

	HTTPStatusCode int    `json:"http_status_code"`
	UpstreamRequestID string `json:"upstream_request_id"`

	ResponsePreview     string `json:"response_preview"`
	ResponseTruncated   bool   `json:"response_truncated"`

	ErrorMessage string `json:"error_message"`

	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
	DurationMs int64     `json:"duration_ms"`
REDACTED
