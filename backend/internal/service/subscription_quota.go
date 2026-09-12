package service

import (
	"context"
	"errors"
	"time"
)

var (
	ErrSubscriptionQuotaRequired    = errors.New("subscription quota admission snapshot is required")
	ErrSubscriptionQuotaInvalid     = errors.New("subscription quota admission snapshot is invalid")
	ErrSubscriptionQuotaUnavailable = errors.New("subscription quota requires transactional billing")
)

// SubscriptionQuotaSnapshot is captured at final admission. It is copied for
// each logical request/WS turn and remains unchanged through retries and billing.
// Zero bucket IDs represent an admission before group accounting was enabled.
type SubscriptionQuotaSnapshot struct {
	SubscriptionID     int64      `json:"subscription_id"`
	UserID             int64      `json:"user_id"`
	GroupID            int64      `json:"group_id"`
	TermEpoch          int64      `json:"term_epoch"`
	GroupRevision      int64      `json:"group_revision"`
	StateVersion       int64      `json:"state_version"`
	DailyBucketID      int64      `json:"daily_bucket_id"`
	WeeklyBucketID     int64      `json:"weekly_bucket_id"`
	MonthlyBucketID    int64      `json:"monthly_bucket_id"`
	AdmittedAt         time.Time  `json:"admitted_at"`
	TermStartsAt       time.Time  `json:"term_starts_at"`
	DailyWindowStart   *time.Time `json:"daily_window_start"`
	WeeklyWindowStart  *time.Time `json:"weekly_window_start"`
	MonthlyWindowStart *time.Time `json:"monthly_window_start"`
}

func (s *SubscriptionQuotaSnapshot) IsLegacy() bool {
	return s != nil && s.DailyBucketID == 0 && s.WeeklyBucketID == 0 && s.MonthlyBucketID == 0
}

func (s *SubscriptionQuotaSnapshot) Clone() *SubscriptionQuotaSnapshot {
	if s == nil {
		return nil
	}
	copy := *s
	cloneTime := func(value *time.Time) *time.Time {
		if value == nil {
			return nil
		}
		result := *value
		return &result
	}
	copy.DailyWindowStart = cloneTime(s.DailyWindowStart)
	copy.WeeklyWindowStart = cloneTime(s.WeeklyWindowStart)
	copy.MonthlyWindowStart = cloneTime(s.MonthlyWindowStart)
	return &copy
}

type SubscriptionQuotaState struct {
	SubscriptionQuotaSnapshot
	Status          string     `json:"status"`
	StartsAt        time.Time  `json:"starts_at"`
	ExpiresAt       time.Time  `json:"expires_at"`
	DeletedAt       *time.Time `json:"deleted_at"`
	DailyUsageUSD   float64    `json:"daily_usage_usd"`
	WeeklyUsageUSD  float64    `json:"weekly_usage_usd"`
	MonthlyUsageUSD float64    `json:"monthly_usage_usd"`
}

type SubscriptionGroupQuotaState struct {
	GroupID   int64
	EnabledAt time.Time
	Revision  int64
}

type SubscriptionQuotaDimensions struct{ Daily, Weekly, Monthly bool }

func (d SubscriptionQuotaDimensions) Any() bool { return d.Daily || d.Weekly || d.Monthly }

type RotateSubscriptionQuotaInput struct {
	SubscriptionID     int64
	Dimensions         SubscriptionQuotaDimensions
	Reason             string
	At                 time.Time
	DailyWindowStart   *time.Time
	WeeklyWindowStart  *time.Time
	MonthlyWindowStart *time.Time
	// Expected is mandatory for stale-task-safe automatic maintenance. All
	// selected IDs must match; a mismatch returns the current state unchanged.
	Expected     *SubscriptionQuotaSnapshot
	NewTerm      bool
	TermStartsAt time.Time
	// First-use activation assigns anchors without forgiving existing usage.
	PreserveUsage bool
}

type RotateGroupQuotaInput struct {
	GroupID          int64
	Dimensions       SubscriptionQuotaDimensions
	Reason           string
	At               time.Time
	DailyWindowStart time.Time
}

type RotateGroupQuotaResult struct {
	GroupID               int64
	Revision              int64
	EffectiveAt           time.Time
	AffectedSubscriptions int64
}

type SubscriptionQuotaRepository interface {
	// QuotaNow uses the database clock after the caller has acquired its group
	// lock, keeping legacy admission tokens ordered with first enablement.
	QuotaNow(context.Context) (time.Time, error)
	// Group lock precedes subscription locks. Existing Ent transactions are
	// joined, allowing lifecycle mutations and observer audit/outbox to commit
	// with bucket switches. Do not upgrade a shared lock inside the callback.
	WithGroupQuotaTx(context.Context, int64, bool, func(context.Context) error) error
	GetGroupQuotaState(context.Context, int64) (*SubscriptionGroupQuotaState, error)
	EnableGroupQuota(context.Context, int64, time.Time) (*SubscriptionGroupQuotaState, error)
	GetSubscriptionQuotaState(context.Context, int64) (*SubscriptionQuotaState, error)
	EnsureSubscriptionQuotaState(context.Context, int64) (*SubscriptionQuotaState, error)
	RotateSubscriptionQuota(context.Context, RotateSubscriptionQuotaInput) (*SubscriptionQuotaState, error)
	RotateGroupQuota(context.Context, RotateGroupQuotaInput) (*RotateGroupQuotaResult, error)
}
