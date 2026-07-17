package securityaudit

import (
	"context"
	"time"
)

const (
	SettingKeyPromptAuditConfig = "prompt_audit_config"
	SettingKeyRiskControl       = "risk_control_enabled"

	ConfigInvalidationChannel = "sub2api:prompt_guard:config:invalidate"
	PayloadKeyPrefix          = "sub2api:prompt_audit:payload:"

	ErrorCodeBlocked         = "prompt_guard_blocked"
	ErrorCodeUnavailable     = "prompt_guard_unavailable"
	ErrorCodeInvalidResponse = "prompt_guard_invalid_response"
	ErrorCodeConfigConflict  = "prompt_audit_config_conflict"
	ErrorCodeRequiresEnabled = "prompt_guard_requires_audit_enabled"

	DefaultGuardModel = "sileader/qwen3guard:0.6b"
)

type Mode string

const (
	ModeOff      Mode = "off"
	ModeAsync    Mode = "async_audit"
	ModeBlocking Mode = "blocking"
)

type DecisionKind string

const (
	DecisionAllow       DecisionKind = "allow"
	DecisionFlag        DecisionKind = "flag"
	DecisionBlock       DecisionKind = "block"
	DecisionUnavailable DecisionKind = "unavailable"
	DecisionInvalid     DecisionKind = "invalid"
)

type EventDecision string

const (
	EventPass     EventDecision = "pass"
	EventFlag     EventDecision = "flag"
	EventCritical EventDecision = "critical"
)

type RiskLevel string

const (
	RiskLow      RiskLevel = "low"
	RiskMedium   RiskLevel = "medium"
	RiskHigh     RiskLevel = "high"
	RiskCritical RiskLevel = "critical"
)

type Action string

const (
	ActionAllow Action = "Allow"
	ActionWarn  Action = "Warn"
	ActionBlock Action = "Block"
)

type Request struct {
	RequestID  string
	UserID     int64
	Username   string
	UserEmail  string
	APIKeyID   int64
	APIKeyName string
	GroupID    *int64
	GroupName  string
	Provider   string
	Endpoint   string
	Protocol   string
	Model      string
	Body       []byte
	Stage      string
}

func (r Request) Clone() Request {
	r.Body = append([]byte(nil), r.Body...)
	if r.GroupID != nil {
		id := *r.GroupID
		r.GroupID = &id
	}
	return r
}

type PromptSnapshot struct {
	RequestID          string `json:"request_id"`
	UserID             int64  `json:"user_id"`
	UsernameSnapshot   string `json:"username"`
	UserEmailSnapshot  string `json:"user_email"`
	APIKeyID           int64  `json:"api_key_id"`
	APIKeyNameSnapshot string `json:"api_key_name"`
	GroupID            *int64 `json:"group_id,omitempty"`
	GroupName          string `json:"group_name"`
	Provider           string `json:"provider"`
	Endpoint           string `json:"endpoint"`
	Protocol           string `json:"protocol"`
	Model              string `json:"model"`
	PromptHash         string `json:"prompt_hash"`
	RedactedPreview    string `json:"redacted_preview"`
	FullPrompt         string `json:"full_prompt"`
	PromptLength       int    `json:"prompt_length"`
	MessageCount       int    `json:"message_count"`
	Stage              string `json:"stage"`

	ScanText string `json:"-"`
}

func (s PromptSnapshot) Redacted() PromptSnapshot {
	s.ScanText = ""
	return s
}

type NormalizedResult struct {
	Decision          EventDecision      `json:"decision"`
	RiskLevel         RiskLevel          `json:"risk_level"`
	Action            Action             `json:"action"`
	Safety            string             `json:"safety"`
	Categories        []string           `json:"categories"`
	MatchedScanners   []string           `json:"matched_scanners"`
	ScannerScores     map[string]float64 `json:"scanner_scores"`
	ScannerEvidence   map[string]string  `json:"scanner_evidence"`
	ScannerBackend    string             `json:"scanner_backend"`
	ScannerVersion    string             `json:"scanner_version"`
	GuardEndpointID   string             `json:"guard_endpoint_id"`
	PolicyID          string             `json:"policy_id"`
	PolicyVersion     int                `json:"policy_version"`
	ChunkTotal        int                `json:"chunk_total"`
	LatencyMS         int                `json:"latency_ms"`
	UnknownCategories []string           `json:"unknown_categories,omitempty"`
}

type PromptDecision struct {
	Kind           DecisionKind      `json:"kind"`
	ErrorCode      string            `json:"error_code,omitempty"`
	Result         *NormalizedResult `json:"result,omitempty"`
	AllowNextStage bool              `json:"allow_next_stage"`
}

type LegacyDecision struct {
	Allowed    bool   `json:"allowed"`
	Blocked    bool   `json:"blocked"`
	Flagged    bool   `json:"flagged"`
	Message    string `json:"message"`
	StatusCode int    `json:"status_code"`
	ErrorCode  string `json:"error_code"`
	Action     string `json:"action"`
}

type Decision struct {
	Kind           DecisionKind    `json:"kind"`
	HTTPStatus     int             `json:"http_status"`
	ErrorCode      string          `json:"error_code,omitempty"`
	ClientMessage  string          `json:"client_message,omitempty"`
	Legacy         *LegacyDecision `json:"legacy,omitempty"`
	Prompt         *PromptDecision `json:"prompt,omitempty"`
	AllowNextStage bool            `json:"allow_next_stage"`
}

type IssueSummary struct {
	Category      string  `json:"category"`
	ScannerID     string  `json:"scanner_id"`
	Title         string  `json:"title"`
	Description   string  `json:"description"`
	Severity      string  `json:"severity"`
	SeverityLabel string  `json:"severity_label"`
	Action        string  `json:"action"`
	ActionLabel   string  `json:"action_label"`
	Code          string  `json:"code"`
	Score         float64 `json:"score"`
	Evidence      string  `json:"evidence"`
	EvidenceHash  string  `json:"evidence_hash"`
	StartRune     *int    `json:"start_rune,omitempty"`
	EndRune       *int    `json:"end_rune,omitempty"`
}

type ProbeResult struct {
	OK           bool      `json:"ok"`
	Status       string    `json:"status"`
	ErrorCode    string    `json:"error_code,omitempty"`
	Message      string    `json:"message"`
	LatencyMS    int       `json:"latency_ms"`
	HTTPStatus   int       `json:"http_status"`
	Retryable    bool      `json:"retryable"`
	CheckedAt    time.Time `json:"checked_at"`
	TokenApplied bool      `json:"token_applied"`
}

type GuardMetricsSnapshot struct {
	Total        int64 `json:"total"`
	Allowed      int64 `json:"allowed"`
	Flagged      int64 `json:"flagged"`
	Blocked      int64 `json:"blocked"`
	Unavailable  int64 `json:"unavailable"`
	Invalid      int64 `json:"invalid"`
	Timeouts     int64 `json:"timeouts"`
	Failovers    int64 `json:"failovers"`
	BulkheadFull int64 `json:"bulkhead_full"`
	RecordFailed int64 `json:"record_failed"`
	LatencyCount int64 `json:"latency_count"`
	LatencyAvgMS int64 `json:"latency_avg_ms"`
	LatencyP50MS int64 `json:"latency_p50_ms"`
	LatencyP95MS int64 `json:"latency_p95_ms"`
	LatencyP99MS int64 `json:"latency_p99_ms"`
	LatencyMaxMS int64 `json:"latency_max_ms"`
}

type AuditMetricsSnapshot struct {
	Enqueued int64 `json:"enqueued"`
	Dropped  int64 `json:"dropped"`
}

type QueueStats struct {
	Staging    int64 `json:"staging"`
	Queued     int64 `json:"queued"`
	Processing int64 `json:"processing"`
	Retry      int64 `json:"retry"`
	Done       int64 `json:"done"`
	Failed     int64 `json:"failed"`
	Active     int64 `json:"active"`
}

type RuntimeSnapshot struct {
	ProcessStatus         string                 `json:"process_status"`
	EffectiveMode         Mode                   `json:"effective_mode"`
	ExpectedConfigVersion int64                  `json:"expected_config_version"`
	ActiveConfigVersion   int64                  `json:"active_config_version"`
	ConfigLoadedAt        *time.Time             `json:"config_loaded_at,omitempty"`
	ConfigLoadError       string                 `json:"config_load_error,omitempty"`
	WorkerTotal           int                    `json:"worker_total"`
	WorkerActive          int64                  `json:"worker_active"`
	WorkerHeartbeatAt     *time.Time             `json:"worker_heartbeat_at,omitempty"`
	QueueCapacity         int                    `json:"queue_capacity"`
	Queue                 QueueStats             `json:"queue"`
	ProcessedTotal        int64                  `json:"processed_total"`
	FailedTotal           int64                  `json:"failed_total"`
	EnqueuedTotal         int64                  `json:"enqueued_total"`
	DroppedTotal          int64                  `json:"dropped_total"`
	LastProcessedAt       *time.Time             `json:"last_processed_at,omitempty"`
	LastErrorCode         string                 `json:"last_error_code,omitempty"`
	LastErrorMessage      string                 `json:"last_error_message,omitempty"`
	DatabaseStatus        string                 `json:"database_status"`
	RedisStatus           string                 `json:"redis_status"`
	Endpoints             map[string]ProbeResult `json:"endpoints"`
	GuardMetrics          GuardMetricsSnapshot   `json:"guard_metrics"`
}

type Clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now().UTC() }

type Metrics interface {
	Snapshot() GuardMetricsSnapshot
	AuditSnapshot() AuditMetricsSnapshot
	Observe(kind DecisionKind, latency time.Duration)
	IncEnqueued()
	IncDropped()
	IncTimeout()
	IncFailover()
	IncBulkheadFull()
	IncRecordFailed()
}

type PromptScanner interface {
	Scan(ctx context.Context, endpoint ActiveEndpoint, chunk string, enabledScanners []string) (*NormalizedResult, error)
}
