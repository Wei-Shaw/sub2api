package service

import "time"

// OpsUserUsageStatsFilter intentionally shares the OpenAI token stats list
// contract so both dashboard cards use identical time, TopN and pagination rules.
type OpsUserUsageStatsFilter = OpsOpenAITokenStatsFilter

type OpsUserUsageStatsItem struct {
	UserID        int64     `json:"user_id"`
	Username      string    `json:"username"`
	Email         string    `json:"email"`
	RequestCount  int64     `json:"request_count"`
	InputTokens   int64     `json:"input_tokens"`
	OutputTokens  int64     `json:"output_tokens"`
	CacheTokens   int64     `json:"cache_tokens"`
	TotalTokens   int64     `json:"total_tokens"`
	ActualCost    float64   `json:"actual_cost"`
	LastRequestAt time.Time `json:"last_request_at"`
}

type OpsUserUsageStatsResponse struct {
	TimeRange string    `json:"time_range"`
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`

	Platform string `json:"platform,omitempty"`
	GroupID  *int64 `json:"group_id,omitempty"`

	Items []*OpsUserUsageStatsItem `json:"items"`
	Total int64                    `json:"total"`

	Page     int  `json:"page,omitempty"`
	PageSize int  `json:"page_size,omitempty"`
	TopN     *int `json:"top_n,omitempty"`
}
