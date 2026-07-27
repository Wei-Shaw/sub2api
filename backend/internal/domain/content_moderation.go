package domain

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

// ContentModerationActionCyberPolicy is the audit-log action value recorded
// for hard cyber-policy blocks. It is excluded from auto-ban violation counts.
const ContentModerationActionCyberPolicy = "cyber_policy"

// ContentModerationLog is the persisted audit row for a moderation decision.
type ContentModerationLog struct {
	ID                int64              `json:"id"`
	RequestID         string             `json:"request_id"`
	UserID            *int64             `json:"user_id,omitempty"`
	UserEmail         string             `json:"user_email"`
	APIKeyID          *int64             `json:"api_key_id,omitempty"`
	APIKeyName        string             `json:"api_key_name"`
	GroupID           *int64             `json:"group_id,omitempty"`
	GroupName         string             `json:"group_name"`
	Endpoint          string             `json:"endpoint"`
	Provider          string             `json:"provider"`
	Model             string             `json:"model"`
	Mode              string             `json:"mode"`
	Action            string             `json:"action"`
	Flagged           bool               `json:"flagged"`
	HighestCategory   string             `json:"highest_category"`
	HighestScore      float64            `json:"highest_score"`
	MatchedKeyword    string             `json:"matched_keyword"`
	CategoryScores    map[string]float64 `json:"category_scores"`
	ThresholdSnapshot map[string]float64 `json:"threshold_snapshot"`
	InputExcerpt      string             `json:"input_excerpt"`
	UpstreamLatencyMS *int               `json:"upstream_latency_ms,omitempty"`
	Error             string             `json:"error"`
	ViolationCount    int                `json:"violation_count"`
	AutoBanned        bool               `json:"auto_banned"`
	EmailSent         bool               `json:"email_sent"`
	UserStatus        string             `json:"user_status"`
	QueueDelayMS      *int               `json:"queue_delay_ms,omitempty"`
	CreatedAt         time.Time          `json:"created_at"`
}

// ContentModerationLogFilter is the query filter for listing moderation logs.
type ContentModerationLogFilter struct {
	Pagination pagination.PaginationParams
	Result     string
	GroupID    *int64
	Endpoint   string
	Search     string
	From       *time.Time
	To         *time.Time
}

// ContentModerationCleanupResult reports rows purged by a cleanup pass.
type ContentModerationCleanupResult struct {
	DeletedHit    int64     `json:"deleted_hit"`
	DeletedNonHit int64     `json:"deleted_non_hit"`
	FinishedAt    time.Time `json:"finished_at"`
}
