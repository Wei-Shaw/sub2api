package service

import "strings"

// OpsMetricIsAggregatePercentile reports metrics whose threshold already
// represents an aggregate tail-latency condition rather than bad rows.
func OpsMetricIsAggregatePercentile(metricType string) bool {
	switch strings.TrimSpace(metricType) {
	case "ttft_p95_seconds", "ttft_p99_seconds", "ttft_max_seconds":
		return true
	default:
		return false
	}
}

// OpsMetricSupportsMinimumBadCount centralizes the cross-field capability used
// by both rule validation and evaluation. Existing non-percentile metrics keep
// their current behavior; aggregate TTFT metrics only use total sample count.
func OpsMetricSupportsMinimumBadCount(metricType string) bool {
	return !OpsMetricIsAggregatePercentile(metricType)
}
