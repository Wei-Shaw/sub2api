package service

import "time"

type OpsErrorClassificationStats struct {
	SuccessCount           int64
	FinalErrorCount        int64
	SLAFailureCount        int64
	PlatformFailureCount   int64
	ProviderFailureCount   int64
	UnknownFailureCount    int64
	PlatformCapacityCount  int64
	CompatibilityCount     int64
	ClientRejectedCount    int64
	BusinessLimitedCount   int64
	CancelledCount         int64
	SecurityBlockedCount   int64
	RecoveredProviderCount int64
	DataAsOf               time.Time
}

// Ops alert rule/event models.
//
// NOTE: These are admin-facing DTOs and intentionally keep JSON naming aligned
// with the existing ops dashboard frontend (backup style).

const (
	OpsAlertStatusFiring         = "firing"
	OpsAlertStatusResolved       = "resolved"
	OpsAlertStatusManualResolved = "manual_resolved"

	OpsAlertEvaluationStatusOK          = "ok"
	OpsAlertEvaluationStatusBreached    = "breached"
	OpsAlertEvaluationStatusNoData      = "no_data"
	OpsAlertEvaluationStatusStale       = "stale"
	OpsAlertEvaluationStatusError       = "error"
	OpsAlertEvaluationStatusUnsupported = "unsupported"
	OpsAlertEvaluationStatusDisabled    = "disabled"
	OpsAlertEvaluationStatusShadow      = "shadow"
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
	MinimumSamples   int `json:"minimum_samples"`
	MinimumBadCount  int `json:"minimum_bad_count"`

	RecoveryOperator         string   `json:"recovery_operator,omitempty"`
	RecoveryThreshold        *float64 `json:"recovery_threshold,omitempty"`
	RecoverySustainedMinutes int      `json:"recovery_sustained_minutes"`
	IncidentFamily           string   `json:"incident_family"`
	ShadowMode               bool     `json:"shadow_mode"`

	NotifyEmail bool `json:"notify_email"`

	Filters map[string]any `json:"filters,omitempty"`

	LastTriggeredAt *time.Time `json:"last_triggered_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type OpsAlertRuleEvaluation struct {
	ID               int64      `json:"id"`
	RuleID           int64      `json:"rule_id"`
	EvaluatedAt      time.Time  `json:"evaluated_at"`
	WindowStart      time.Time  `json:"window_start"`
	WindowEnd        time.Time  `json:"window_end"`
	Status           string     `json:"status"`
	Breached         bool       `json:"breached"`
	MetricValue      *float64   `json:"metric_value,omitempty"`
	ThresholdValue   *float64   `json:"threshold_value,omitempty"`
	SampleCount      int64      `json:"sample_count"`
	BadCount         int64      `json:"bad_count"`
	DataAsOf         *time.Time `json:"data_as_of,omitempty"`
	ErrorCode        string     `json:"error_code,omitempty"`
	ErrorMessage     string     `json:"error_message,omitempty"`
	EvaluatorVersion string     `json:"evaluator_version"`
	CreatedAt        time.Time  `json:"created_at"`
}

type OpsAlertRuleState struct {
	RuleID                int64      `json:"rule_id"`
	LastEvaluatedAt       *time.Time `json:"last_evaluated_at,omitempty"`
	ConsecutiveBreaches   int        `json:"consecutive_breaches"`
	ConsecutiveRecoveries int        `json:"consecutive_recoveries"`
	UpdatedAt             time.Time  `json:"updated_at"`
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

	EmailSent   bool `json:"email_sent"`
	EmailQueued bool `json:"email_queued"`
	// EmailDeliveryStatus is derived from the durable notification delivery
	// rows. It is the observable truth; EmailSent/EmailQueued remain for
	// backwards-compatible clients.
	EmailDeliveryStatus string    `json:"email_delivery_status,omitempty"`
	EmailDeliveryDetail string    `json:"email_delivery_detail,omitempty"`
	CreatedAt           time.Time `json:"created_at"`
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
