package service

import "time"

// OpsRealtimeTrafficPoint is one real database bucket rendered by the Ops
// dashboard. Rates are normalized so different window bucket sizes remain
// comparable, while ActualCost is the money deducted inside that bucket.
type OpsRealtimeTrafficPoint struct {
	Time            time.Time `json:"time"`
	RPM             float64   `json:"rpm"`
	TokensPerSecond float64   `json:"tokens_per_second"`
	ActualCost      float64   `json:"actual_cost"`
}

// OpsRealtimeTrafficSummary is a lightweight summary and real time series used
// by the Ops dashboard "Realtime Traffic" card.
type OpsRealtimeTrafficSummary struct {
	// Window is a normalized label (e.g. "1min", "5min", "30min", "1h").
	Window string `json:"window"`

	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`

	Platform string `json:"platform"`
	GroupID  *int64 `json:"group_id"`

	QPS OpsRateSummary `json:"qps"`
	TPS OpsRateSummary `json:"tps"`

	BucketSeconds   int                        `json:"bucket_seconds"`
	ActualCostTotal float64                    `json:"actual_cost_total"`
	Points          []*OpsRealtimeTrafficPoint `json:"points"`
}
