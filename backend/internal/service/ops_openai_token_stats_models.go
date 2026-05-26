package service

import "time"

type OpsOpenAITokenStatsFilter struct {
	TimeRange string
	StartTime time.Time
	EndTime   time.Time

	Platform string
	GroupID  *int64

	// Pagination mode (default): page/page_size
	Page     int
	PageSize int

	// TopN mode: top_n
	TopN int
}

func (f *OpsOpenAITokenStatsFilter) IsTopNMode() bool {
	return f != nil && f.TopN > 0
}

type OpsOpenAITokenStatsItem struct {
	Model                  string   `json:"model"`
	RequestCount           int64    `json:"request_count"`
	AvgTokensPerSec        *float64 `json:"avg_tokens_per_sec"`
	AvgFirstTokenMs        *float64 `json:"avg_first_token_ms"`
	TotalOutputTokens      int64    `json:"total_output_tokens"`
	AvgDurationMs          int64    `json:"avg_duration_ms"`
	RequestsWithFirstToken int64    `json:"requests_with_first_token"`
}

type OpsOpenAITokenStatsResponse struct {
	TimeRange string    `json:"time_range"`
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`

	Platform string `json:"platform,omitempty"`
	GroupID  *int64 `json:"group_id,omitempty"`

	Items []*OpsOpenAITokenStatsItem `json:"items"`

	// Total model rows before pagination/topN trimming.
	Total int64 `json:"total"`

	// Pagination mode metadata.
	Page     int `json:"page,omitempty"`
	PageSize int `json:"page_size,omitempty"`

	// TopN mode metadata.
	TopN *int `json:"top_n,omitempty"`
}
