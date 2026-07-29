package service

import (
	"context"
	"time"
)

// Connection risk event statuses.
const (
	ConnectionRiskStatusOpen         = "open"
	ConnectionRiskStatusAcknowledged = "acknowledged"
	ConnectionRiskStatusResolved     = "resolved"
	ConnectionRiskStatusSuppressed   = "suppressed"

	ConnectionRiskSubjectAPIKey = "api_key"
	ConnectionRiskSubjectUser   = "user"

	ConnectionRiskActionNone     = "none"
	ConnectionRiskActionNotified = "notified"
)

// ConnectionRiskEvent is one scored risk finding.
type ConnectionRiskEvent struct {
	ID           int64                   `json:"id"`
	CreatedAt    time.Time               `json:"created_at"`
	UpdatedAt    time.Time               `json:"updated_at"`
	SubjectType  string                  `json:"subject_type"`
	UserID       *int64                  `json:"user_id,omitempty"`
	APIKeyID     *int64                  `json:"api_key_id,omitempty"`
	APIKeyPrefix string                  `json:"api_key_prefix"`
	RulesFired   []ConnectionRiskRuleHit `json:"rules_fired"`
	Severity     string                  `json:"severity"`
	Score        float64                 `json:"score"`
	Status       string                  `json:"status"`
	Title        string                  `json:"title"`
	Summary      string                  `json:"summary"`
	Evidence     map[string]any          `json:"evidence"`
	Metrics      map[string]any          `json:"metrics"`
	DedupeKey    string                  `json:"dedupe_key"`
	ActionTaken  string                  `json:"action_taken"`
	ResolverID   *int64                  `json:"resolver_id,omitempty"`
	ResolvedAt   *time.Time              `json:"resolved_at,omitempty"`
	FirstSeenAt  time.Time               `json:"first_seen_at"`
	LastSeenAt   time.Time               `json:"last_seen_at"`
	WindowStart  *time.Time              `json:"window_start,omitempty"`
	WindowEnd    *time.Time              `json:"window_end,omitempty"`
}

// ConnectionRiskEventFilter lists events for admin UI.
type ConnectionRiskEventFilter struct {
	Status   string
	Severity string
	UserID   *int64
	APIKeyID *int64
	Rule     string
	Page     int
	PageSize int
}

// ConnectionRiskEventList is a paginated result.
type ConnectionRiskEventList struct {
	Items    []*ConnectionRiskEvent `json:"items"`
	Total    int64                  `json:"total"`
	Page     int                    `json:"page"`
	PageSize int                    `json:"page_size"`
}

// ConnectionRiskEventRepository persists risk events (raw SQL, audit-style).
type ConnectionRiskEventRepository interface {
	UpsertOpen(ctx context.Context, event *ConnectionRiskEvent) (*ConnectionRiskEvent, error)
	GetByID(ctx context.Context, id int64) (*ConnectionRiskEvent, error)
	List(ctx context.Context, filter *ConnectionRiskEventFilter) (*ConnectionRiskEventList, error)
	UpdateStatus(ctx context.Context, id int64, status string, resolverID *int64) error
	Delete(ctx context.Context, id int64) error
	// DeleteOlderThan removes events with last_seen_at before cutoff; returns rows deleted.
	DeleteOlderThan(ctx context.Context, cutoff time.Time) (int64, error)
}
