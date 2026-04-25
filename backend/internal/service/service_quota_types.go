package service

import (
	"context"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	SettingKeyServiceQuotaEnabled = "service_quota_enabled"

	ServiceQuotaLimiterRPM         = "rpm"
	ServiceQuotaLimiterTPM         = "tpm"
	ServiceQuotaLimiterTPD         = "tpd"
	ServiceQuotaLimiterDailyUSD    = "daily_usd"
	ServiceQuotaLimiterConcurrency = "concurrency"

	// ServiceQuotaCounterMode* 决定规则 Redis 计数 key 的分片方式：
	//   - user     → 规则只对关联表中列出的用户生效，每个用户独立计数
	//   - per_user → 对 scope 内所有用户生效，按 user_id 分片
	//   - shared   → 对 scope 内所有用户生效，共享同一计数器
	ServiceQuotaCounterModeUser    = "user"
	ServiceQuotaCounterModePerUser = "per_user"
	ServiceQuotaCounterModeShared  = "shared"

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

// ServiceQuotaLimiterDef 单个限流器：一条规则可以挂多种类型（RPM、TPD 等）。
type ServiceQuotaLimiterDef struct {
	ID          int64   `json:"id"`
	RuleID      int64   `json:"rule_id"`
	LimiterType string  `json:"limiter_type"`
	WindowMode  string  `json:"window_mode"`
	LimitValue  float64 `json:"limit_value"`
}

type ServiceQuotaLimiterInput struct {
	LimiterType string  `json:"limiter_type"`
	WindowMode  string  `json:"window_mode"`
	LimitValue  float64 `json:"limit_value"`
}

// ServiceQuotaRuleUserRef 是绑定用户的轻量引用，用于前端 chip 展示（规则只在 counter_mode=user 时携带此字段）。
type ServiceQuotaRuleUserRef struct {
	ID    int64  `json:"id"`
	Email string `json:"email"`
}

// ServiceQuotaRule 一条规则。
//
// 规则 = N 个限流器 × 5 个维度集合（platform/channel/group/account/model_pattern）× 用户绑定。
// 维度匹配语义：每个非空维度必须包含本次请求的对应字段（AND-of-sets）。空维度视为不限制。
//
// CounterMode 与 IsFallback 正交：
//   - CounterMode 决定计数 key 的分片方式
//   - IsFallback=true 表示兜底规则：同一 limiter_type 有其他非 fallback 规则命中时该 limiter 自动让位
type ServiceQuotaRule struct {
	ID            int64                     `json:"id"`
	Enabled       bool                      `json:"enabled"`
	Name          *string                   `json:"name,omitempty"`
	CounterMode   string                    `json:"counter_mode"`
	IsFallback    bool                      `json:"is_fallback"`
	Limiters      []ServiceQuotaLimiterDef  `json:"limiters"`
	Platforms     []string                  `json:"platforms"`
	ChannelIDs    []int64                   `json:"channel_ids"`
	GroupIDs      []int64                   `json:"group_ids"`
	AccountIDs    []int64                   `json:"account_ids"`
	ModelPatterns []string                  `json:"model_patterns"`
	TargetUserIDs []int64                   `json:"target_user_ids,omitempty"`
	TargetUsers   []ServiceQuotaRuleUserRef `json:"target_users,omitempty"`
	CreatedAt     time.Time                 `json:"created_at"`
	UpdatedAt     time.Time                 `json:"updated_at"`
}

type ServiceQuotaRuleInput struct {
	Enabled       *bool                      `json:"enabled"`
	Name          *string                    `json:"name"`
	CounterMode   string                     `json:"counter_mode"`
	IsFallback    bool                       `json:"is_fallback"`
	Limiters      []ServiceQuotaLimiterInput `json:"limiters"`
	Platforms     []string                   `json:"platforms"`
	ChannelIDs    []int64                    `json:"channel_ids"`
	GroupIDs      []int64                    `json:"group_ids"`
	AccountIDs    []int64                    `json:"account_ids"`
	ModelPatterns []string                   `json:"model_patterns"`
	TargetUserIDs []int64                    `json:"target_user_ids"`
}

type ServiceQuotaListFilter struct {
	Enabled     *bool
	LimiterType string
}

type ServiceQuotaCheckRequest struct {
	UserID    int64
	Platform  string
	ChannelID int64
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

// AccountScopeInfo / GroupScopeInfo 用于维度链路一致性校验：账号必须属于指定的分组/平台。
type AccountScopeInfo struct {
	Platform string
	GroupIDs []int64
}

type GroupScopeInfo struct {
	Platform string
}

type ServiceQuotaRuleRepository interface {
	List(ctx context.Context, filter ServiceQuotaListFilter) ([]*ServiceQuotaRule, error)
	Create(ctx context.Context, input ServiceQuotaRuleInput) (*ServiceQuotaRule, error)
	Update(ctx context.Context, id int64, input ServiceQuotaRuleInput) (*ServiceQuotaRule, error)
	Delete(ctx context.Context, id int64) error
	FetchAccountScope(ctx context.Context, accountID int64) (*AccountScopeInfo, error)
	FetchGroupScope(ctx context.Context, groupID int64) (*GroupScopeInfo, error)
}

type ServiceQuotaLimiter interface {
	Current(ctx context.Context, key string, window time.Duration, mode string) (float64, error)
	Increment(ctx context.Context, key string, delta float64, window time.Duration, mode string) (float64, error)
	Acquire(ctx context.Context, key, member string, limit int64) (bool, error)
	Release(ctx context.Context, key, member string) error
}

type ServiceQuotaCache interface {
	GetRules(ctx context.Context) ([]*ServiceQuotaRule, bool, error)
	SetRules(ctx context.Context, rules []*ServiceQuotaRule) error
	GetEnabled(ctx context.Context) (*bool, error)
	SetEnabled(ctx context.Context, enabled bool) error
	Invalidate(ctx context.Context) error
}

type ServiceQuotaService interface {
	ListRules(ctx context.Context, filter ServiceQuotaListFilter) ([]*ServiceQuotaRule, error)
	CreateRule(ctx context.Context, input ServiceQuotaRuleInput) (*ServiceQuotaRule, error)
	UpdateRule(ctx context.Context, id int64, input ServiceQuotaRuleInput) (*ServiceQuotaRule, error)
	DeleteRule(ctx context.Context, id int64) error
	InvalidateEnabledCache(ctx context.Context)
	PreCheck(ctx context.Context, req ServiceQuotaCheckRequest) (*ServiceQuotaLease, error)
	Record(ctx context.Context, req ServiceQuotaRecordRequest)
}