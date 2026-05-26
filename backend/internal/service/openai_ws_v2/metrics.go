package openai_ws_v2

import (
	"sync/atomic"
)

// MetricsSnapshot 是 OpenAI WS v2 passthrough 路径的轻量运行时指标快照。
type MetricsSnapshot struct {
	SemanticMutationTotal  int64 `json:"semantic_mutation_total"`
	UsageParseFailureTotal int64 `json:"usage_parse_failure_total"`
}

var (
	// passthrough 路径默认不会做语义改写，该计数通常应保持为 0（保留用于未来防御性校验）。
	passthroughSemanticMutationTotal  atomic.Int64
	passthroughUsageParseFailureTotal atomic.Int64
)

func recordUsageParseFailure() {
	passthroughUsageParseFailureTotal.Add(1)
}

// SnapshotMetrics 返回当前 passthrough 指标快照。
func SnapshotMetrics() MetricsSnapshot {
	return MetricsSnapshot{
		SemanticMutationTotal:  passthroughSemanticMutationTotal.Load(),
		UsageParseFailureTotal: passthroughUsageParseFailureTotal.Load(),
	}
}
