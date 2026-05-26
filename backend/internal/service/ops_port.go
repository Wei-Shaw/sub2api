package service

import (
	"context"
	"time"
)

type OpsRepository interface {
	InsertErrorLog(ctx context.Context, input *OpsInsertErrorLogInput) (int64, error)
	BatchInsertErrorLogs(ctx context.Context, inputs []*OpsInsertErrorLogInput) (int64, error)
	ListErrorLogs(ctx context.Context, filter *OpsErrorLogFilter) (*OpsErrorLogList, error)
	GetErrorLogByID(ctx context.Context, id int64) (*OpsErrorLogDetail, error)
	ListRequestDetails(ctx context.Context, filter *OpsRequestDetailFilter) ([]*OpsRequestDetail, int64, error)
	BatchInsertSystemLogs(ctx context.Context, inputs []*OpsInsertSystemLogInput) (int64, error)
	ListSystemLogs(ctx context.Context, filter *OpsSystemLogFilter) (*OpsSystemLogList, error)
	DeleteSystemLogs(ctx context.Context, filter *OpsSystemLogCleanupFilter) (int64, error)
	InsertSystemLogCleanupAudit(ctx context.Context, input *OpsSystemLogCleanupAudit) error

	InsertRetryAttempt(ctx context.Context, input *OpsInsertRetryAttemptInput) (int64, error)
	UpdateRetryAttempt(ctx context.Context, input *OpsUpdateRetryAttemptInput) error
	GetLatestRetryAttemptForError(ctx context.Context, sourceErrorID int64) (*OpsRetryAttempt, error)
	ListRetryAttemptsByErrorID(ctx context.Context, sourceErrorID int64, limit int) ([]*OpsRetryAttempt, error)
	UpdateErrorResolution(ctx context.Context, errorID int64, resolved bool, resolvedByUserID *int64, resolvedRetryID *int64, resolvedAt *time.Time) error

	// Lightweight window stats (for realtime WS / quick sampling).
	GetWindowStats(ctx context.Context, filter *OpsDashboardFilter) (*OpsWindowStats, error)
	// Lightweight realtime traffic summary (for the Ops dashboard header card).
	GetRealtimeTrafficSummary(ctx context.Context, filter *OpsDashboardFilter) (*OpsRealtimeTrafficSummary, error)

	GetDashboardOverview(ctx context.Context, filter *OpsDashboardFilter) (*OpsDashboardOverview, error)
	GetThroughputTrend(ctx context.Context, filter *OpsDashboardFilter, bucketSeconds int) (*OpsThroughputTrendResponse, error)
	GetLatencyHistogram(ctx context.Context, filter *OpsDashboardFilter) (*OpsLatencyHistogramResponse, error)
	GetErrorTrend(ctx context.Context, filter *OpsDashboardFilter, bucketSeconds int) (*OpsErrorTrendResponse, error)
	GetErrorDistribution(ctx context.Context, filter *OpsDashboardFilter) (*OpsErrorDistributionResponse, error)
	GetOpenAITokenStats(ctx context.Context, filter *OpsOpenAITokenStatsFilter) (*OpsOpenAITokenStatsResponse, error)

	InsertSystemMetrics(ctx context.Context, input *OpsInsertSystemMetricsInput) error
	GetLatestSystemMetrics(ctx context.Context, windowMinutes int) (*OpsSystemMetricsSnapshot, error)

	UpsertJobHeartbeat(ctx context.Context, input *OpsUpsertJobHeartbeatInput) error
	ListJobHeartbeats(ctx context.Context) ([]*OpsJobHeartbeat, error)

	// Alerts (rules + events)
	ListAlertRules(ctx context.Context) ([]*OpsAlertRule, error)
	CreateAlertRule(ctx context.Context, input *OpsAlertRule) (*OpsAlertRule, error)
	UpdateAlertRule(ctx context.Context, input *OpsAlertRule) (*OpsAlertRule, error)
	DeleteAlertRule(ctx context.Context, id int64) error

	ListAlertEvents(ctx context.Context, filter *OpsAlertEventFilter) ([]*OpsAlertEvent, error)
	GetAlertEventByID(ctx context.Context, eventID int64) (*OpsAlertEvent, error)
	GetActiveAlertEvent(ctx context.Context, ruleID int64) (*OpsAlertEvent, error)
	GetLatestAlertEvent(ctx context.Context, ruleID int64) (*OpsAlertEvent, error)
	CreateAlertEvent(ctx context.Context, event *OpsAlertEvent) (*OpsAlertEvent, error)
	UpdateAlertEventStatus(ctx context.Context, eventID int64, status string, resolvedAt *time.Time) error
	UpdateAlertEventEmailSent(ctx context.Context, eventID int64, emailSent bool) error

	// Alert silences
	CreateAlertSilence(ctx context.Context, input *OpsAlertSilence) (*OpsAlertSilence, error)
	IsAlertSilenced(ctx context.Context, ruleID int64, platform string, groupID *int64, region *string, now time.Time) (bool, error)

	// Pre-aggregation (hourly/daily) used for long-window dashboard performance.
	UpsertHourlyMetrics(ctx context.Context, startTime, endTime time.Time) error
	UpsertDailyMetrics(ctx context.Context, startTime, endTime time.Time) error
	GetLatestHourlyBucketStart(ctx context.Context) (time.Time, bool, error)
	GetLatestDailyBucketDate(ctx context.Context) (time.Time, bool, error)
}

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

	RequestBodyJSON      *string // sanitized json string (not raw bytes)
	RequestBodyTruncated bool
	RequestBodyBytes     *int
	RequestHeadersJSON   *string // optional json string

	IsRetryable bool
	RetryCount  int

	CreatedAt time.Time
}

type OpsInsertRetryAttemptInput struct {
	RequestedByUserID int64
	SourceErrorID     int64
	Mode              string
	PinnedAccountID   *int64

	// running|queued etc.
	Status    string
	StartedAt time.Time
}

type OpsUpdateRetryAttemptInput struct {
	ID int64

	// succeeded|failed
	Status     string
	FinishedAt time.Time
	DurationMs int64

	// Persisted execution results (best-effort)
	Success           *bool
	HTTPStatusCode    *int
	UpstreamRequestID *string
	UsedAccountID     *int64
	ResponsePreview   *string
	ResponseTruncated *bool

	// Optional correlation (legacy fields kept)
	ResultRequestID *string
	ResultErrorID   *int64

	ErrorMessage *string
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
	Level           string
	Component       string
	Message         string
	RequestID       string
	ClientRequestID string
	UserID          *int64
	AccountID       *int64
	Platform        string
	Model           string
	ExtraJSON       string
}

type OpsSystemLogFilter struct {
	StartTime *time.Time
	EndTime   *time.Time

	Level     string
	Component string

	RequestID       string
	ClientRequestID string
	UserID          *int64
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

	Level     string
	Component string

	RequestID       string
	ClientRequestID string
	UserID          *int64
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
