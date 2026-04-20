package dto

// ChannelMonitorExtraModelStatus 渠道监控附加模型最近一次状态。
// 同时被 admin handler（List 响应）与 user handler（List 响应）复用，
// 字段必须保持一致以保证前端拿到统一结构。
type ChannelMonitorExtraModelStatus struct {
	Model     string `json:"model"`
	Status    string `json:"status"`
	LatencyMs *int   `json:"latency_ms"`
}
