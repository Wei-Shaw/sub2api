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

// ServiceQuotaRule 表示一条服务配额规则。
//
// CounterMode 与 IsFallback 正交：
//   - CounterMode 决定计数 key 的分片方式
//   - IsFallback=true 表示兜底规则：同一 scope+limiter 有其他非 fallback 规则时该条自动让位
//
// TargetUserIDs 只在 CounterMode=user 时有效，允许一条规则绑定多个用户。
// ServiceQuotaRule 的 scope 由 Platform / GroupID / AccountID / ModelPattern 的非 nil 组合决定：
//   - 全部为 nil        → 对所有请求生效（全局）
//   - 只设置 Platform   → 该平台所有请求
//   - GroupID+AccountID → 该分组下使用该账号的请求（链路级）
//   - 其它组合以此类推，多个字段之间是 AND 关系
//
// ServiceQuotaRuleUserRef 是绑定用户的轻量引用，用于前端 chip 展示（规则只在 counter_mode=user 时携带此字段）。
type ServiceQuotaRuleUserRef struct {
	ID    int64  `json:"id"`
	Email string `json:"email"`
}

type ServiceQuotaRule struct {
	ID            int64                     `json:"id"`
	Enabled       bool                      `json:"enabled"`
	Platform      *string                   `json:"platform,omitempty"`
	GroupID       *int64                    `json:"group_id,omitempty"`
	AccountID     *int64                    `json:"account_id,omitempty"`
	ModelPattern  *string                   `json:"model_pattern,omitempty"`
	LimiterType   string                    `json:"limiter_type"`
	CounterMode   string                    `json:"counter_mode"`
	IsFallback    bool                      `json:"is_fallback"`
	TargetUserIDs []int64                   `json:"target_user_ids,omitempty"`
	TargetUsers   []ServiceQuotaRuleUserRef `json:"target_users,omitempty"`
	WindowMode    string                    `json:"window_mode"`
	LimitValue    float64                   `json:"limit_value"`
	CurrentUsage  *float64                  `json:"current_usage,omitempty"`
	CreatedAt     time.Time                 `json:"created_at"`
	UpdatedAt     time.Time                 `json:"updated_at"`
}

type ServiceQuotaRuleInput struct {
	Enabled       *bool   `json:"enabled"`
	Platform      *string `json:"platform"`
	GroupID       *int64  `json:"group_id"`
	AccountID     *int64  `json:"account_id"`
	ModelPattern  *string `json:"model_pattern"`
	LimiterType   string  `json:"limiter_type"`
	CounterMode   string  `json:"counter_mode"`
	IsFallback    bool    `json:"is_fallback"`
	TargetUserIDs []int64 `json:"target_user_ids"`
	WindowMode    string  `json:"window_mode"`
	LimitValue    float64 `json:"limit_value"`
}

type ServiceQuotaListFilter struct {
	Enabled     *bool
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

// AccountScopeInfo / GroupScopeInfo 用于 scope 链路一致性校验：下级必须是上级的子孙。
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

type ServiceQuotaService interface {
	ListRules(ctx context.Context, filter ServiceQuotaListFilter) ([]*ServiceQuotaRule, error)
	CreateRule(ctx context.Context, input ServiceQuotaRuleInput) (*ServiceQuotaRule, error)
	UpdateRule(ctx context.Context, id int64, input ServiceQuotaRuleInput) (*ServiceQuotaRule, error)
	DeleteRule(ctx context.Context, id int64) error
	PreCheck(ctx context.Context, req ServiceQuotaCheckRequest) (*ServiceQuotaLease, error)
	Record(ctx context.Context, req ServiceQuotaRecordRequest)
}
