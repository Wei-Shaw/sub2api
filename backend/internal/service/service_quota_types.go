package service

import (
	"context"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	SettingKeyServiceQuotaEnabled = "service_quota_enabled"

	ServiceQuotaScopeGlobal   = "global"
	ServiceQuotaScopePlatform = "platform"
	ServiceQuotaScopeGroup    = "group"
	ServiceQuotaScopeAccount  = "account"
	ServiceQuotaScopeModel    = "model"

	ServiceQuotaLimiterRPM         = "rpm"
	ServiceQuotaLimiterTPM         = "tpm"
	ServiceQuotaLimiterTPD         = "tpd"
	ServiceQuotaLimiterDailyUSD    = "daily_usd"
	ServiceQuotaLimiterConcurrency = "concurrency"

	ServiceQuotaTargetUser    = "user"
	ServiceQuotaTargetPerUser = "per_user"
	ServiceQuotaTargetShared  = "shared"
	ServiceQuotaTargetDefault = "default"

	ServiceQuotaWindowFixed   = "fixed"
	ServiceQuotaWindowRolling = "rolling"
)

var (
	ErrServiceQuotaExceeded = infraerrors.TooManyRequests(
		"SERVICE_QUOTA_EXCEEDED",
		"service quota exceeded",
	)
	ErrServiceQuotaRuleNotFound = infraerrors.NotFound(
		"SERVICE_QUOTA_RULE_NOT_FOUND",
		"service quota rule not found",
	)
)

type ServiceQuotaRule struct {
	ID           int64     `json:"id"`
	Enabled      bool      `json:"enabled"`
	ScopeLevel   string    `json:"scope_level"`
	Platform     *string   `json:"platform,omitempty"`
	GroupID      *int64    `json:"group_id,omitempty"`
	AccountID    *int64    `json:"account_id,omitempty"`
	ModelPattern *string   `json:"model_pattern,omitempty"`
	LimiterType  string    `json:"limiter_type"`
	TargetMode   string    `json:"target_mode"`
	TargetUserID *int64    `json:"target_user_id,omitempty"`
	WindowMode   string    `json:"window_mode"`
	LimitValue   float64   `json:"limit_value"`
	CurrentUsage *float64  `json:"current_usage,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type ServiceQuotaRuleInput struct {
	Enabled      *bool   `json:"enabled"`
	ScopeLevel   string  `json:"scope_level"`
	Platform     *string `json:"platform"`
	GroupID      *int64  `json:"group_id"`
	AccountID    *int64  `json:"account_id"`
	ModelPattern *string `json:"model_pattern"`
	LimiterType  string  `json:"limiter_type"`
	TargetMode   string  `json:"target_mode"`
	TargetUserID *int64  `json:"target_user_id"`
	WindowMode   string  `json:"window_mode"`
	LimitValue   float64 `json:"limit_value"`
}

type ServiceQuotaListFilter struct {
	Enabled     *bool
	ScopeLevel  string
	LimiterType string
}

type ServiceQuotaCheckRequest struct {
	UserID    int64
	Platform  string
	GroupID   int64
	AccountID int64
	Model     string
}

type ServiceQuotaRecordRequest struct {
	ServiceQuotaCheckRequest
	Tokens int64
	Cost   float64
}

type ServiceQuotaLease struct {
	Release func()
}

type ServiceQuotaRuleRepository interface {
	List(ctx context.Context, filter ServiceQuotaListFilter) ([]*ServiceQuotaRule, error)
	Create(ctx context.Context, input ServiceQuotaRuleInput) (*ServiceQuotaRule, error)
	Update(ctx context.Context, id int64, input ServiceQuotaRuleInput) (*ServiceQuotaRule, error)
	Delete(ctx context.Context, id int64) error
}

type ServiceQuotaLimiter interface {
	Current(ctx context.Context, key string, window time.Duration, mode string) (float64, error)
	Increment(ctx context.Context, key string, delta float64, window time.Duration, mode string) (float64, error)
	Acquire(ctx context.Context, key, member string, limit int64) (bool, error)
	Release(ctx context.Context, key, member string) error
}

type ServiceQuotaService interface {
	ListRules(ctx context.Context, filter ServiceQuotaListFilter) ([]*ServiceQuotaRule, error)
	CreateRule(ctx context.Context, input ServiceQuotaRuleInput) (*ServiceQuotaRule, error)
	UpdateRule(ctx context.Context, id int64, input ServiceQuotaRuleInput) (*ServiceQuotaRule, error)
	DeleteRule(ctx context.Context, id int64) error
	PreCheck(ctx context.Context, req ServiceQuotaCheckRequest) (*ServiceQuotaLease, error)
	Record(ctx context.Context, req ServiceQuotaRecordRequest)
}
