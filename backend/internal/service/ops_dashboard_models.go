package service

import "time"

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

// OpsHealthScoreBreakdown makes the backend-owned health score explainable.
// Component weights are expressed as a fraction of the final 100-point score.
type OpsHealthScoreBreakdown struct {
	Mode             string                     `json:"mode"`
	BusinessIncluded bool                       `json:"business_included"`
	Score            int                        `json:"score"`
	Components       []*OpsHealthScoreComponent `json:"components"`
}

type OpsHealthScoreComponent struct {
	Key             string                  `json:"key"`
	Score           float64                 `json:"score"`
	Weight          float64                 `json:"weight"`
	MaxPoints       float64                 `json:"max_points"`
	EarnedPoints    float64                 `json:"earned_points"`
	DeductionPoints float64                 `json:"deduction_points"`
	Reasons         []*OpsHealthScoreReason `json:"reasons"`
}

// OpsHealthScoreReason contains locale-neutral facts. The administrator UI
// owns presentation and translation while the backend remains the sole source
// of truth for why points were deducted.
type OpsHealthScoreReason struct {
	Code          string   `json:"code"`
	Value         *float64 `json:"value,omitempty"`
	Threshold     *float64 `json:"threshold,omitempty"`
	JobName       string   `json:"job_name,omitempty"`
	AgeSeconds    *float64 `json:"age_seconds,omitempty"`
	MaxAgeSeconds *float64 `json:"max_age_seconds,omitempty"`
}

type OpsDashboardOverview struct {
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
	Platform  string    `json:"platform"`
	GroupID   *int64    `json:"group_id"`

	// HealthScore is a backend-computed overall health score (0-100).
	// It is derived from the monitored metrics in this overview, plus best-effort system metrics/job heartbeats.
	HealthScore          int                      `json:"health_score"`
	HealthScoreBreakdown *OpsHealthScoreBreakdown `json:"health_score_breakdown"`

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
	// AvailabilityAvailable distinguishes a real 0% availability result from
	// an empty window where no availability sample exists.
	AvailabilityAvailable bool `json:"availability_available"`

	PlatformFailureCount int64 `json:"platform_failure_count"`
	ProviderFailureCount int64 `json:"provider_failure_count"`
	UnknownFailureCount  int64 `json:"unknown_failure_count"`
	ClientRejectedCount  int64 `json:"client_rejected_count"`
	CancelledCount       int64 `json:"cancelled_count"`
	SecurityBlockedCount int64 `json:"security_blocked_count"`
	RecoveredCount       int64 `json:"recovered_count"`

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
