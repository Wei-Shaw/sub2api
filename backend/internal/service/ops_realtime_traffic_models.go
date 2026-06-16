package service

import "time"

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
