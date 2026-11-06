package service

// OpsTrafficSummary covers finalized gateway HTTP requests in the independent
// traffic ledger, including failures and all their upstream retry attempts.
// It is independent of usage/error logging. HTTP/TCP/TLS headers and WebSocket
// frames are outside this metric. RequestCount is the ledger's own population,
// not the overview's usage/error-derived request count.
type OpsTrafficSummary struct {
	Source       string         `json:"source"`
	RequestCount int64          `json:"request_count"`
	Upstream     OpsTrafficLink `json:"upstream"`
	Downstream   OpsTrafficLink `json:"downstream"`
	// Local process lifetime health, not time/platform/group-filtered totals.
	Ingestion *OpsTrafficLedgerHealth `json:"ingestion"`
}

// OpsTrafficLink adds request and response body observations for one side.
// Every HTTP request contributes two potential samples, so each coverage
// denominator is 2 * RequestCount. Byte sums are nil when no sample is known;
// measured empty bodies are zero. Original and actual coverage may differ.
// CompressionPercent uses ONLY paired observations with both byte values known,
// never the independently covered OriginalBytes / ActualBytes sums. It is nil
// for a non-positive paired original sum, and can legitimately be negative.
type OpsTrafficLink struct {
	OriginalBytes       *int64   `json:"original_bytes"`
	ActualBytes         *int64   `json:"actual_bytes"`
	OriginalSampleCount int64    `json:"original_sample_count"`
	ActualSampleCount   int64    `json:"actual_sample_count"`
	PairedSampleCount   int64    `json:"paired_sample_count"`
	CompressionPercent  *float64 `json:"compression_percent"`
	// Legacy hourly aggregates may retain valid totals without a complete
	// directional breakdown. Never infer a split from those totals.
	Request  *OpsTrafficDirection `json:"request"`
	Response *OpsTrafficDirection `json:"response"`
}

// OpsTrafficDirection contains measured byte sums for one body direction.
// Each sample denominator is RequestCount, not the side's 2 * RequestCount.
// A nil byte count means no known observation; a measured empty body is zero.
// CompressionPercent uses ONLY this direction's paired observations, never the
// independently covered OriginalBytes / ActualBytes sums, and is nil when the
// ratio is unknown: no paired sample, a non-positive paired original total, or
// a window that includes buckets aggregated before directional pairing existed.
type OpsTrafficDirection struct {
	OriginalBytes       *int64   `json:"original_bytes"`
	ActualBytes         *int64   `json:"actual_bytes"`
	OriginalSampleCount int64    `json:"original_sample_count"`
	ActualSampleCount   int64    `json:"actual_sample_count"`
	CompressionPercent  *float64 `json:"compression_percent"`
}
