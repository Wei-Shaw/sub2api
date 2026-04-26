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

// ServiceQuotaPathDef 路径定义：单向递进链 平台→渠道→分组→账号→模型，
// 每层非 nil 才参与匹配，nil 视作不限制该维度。
type ServiceQuotaPathDef struct {
	ID           int64   `json:"id"`
	RuleID       int64   `json:"rule_id"`
	Platform     *string `json:"platform,omitempty"`
	ChannelID    *int64  `json:"channel_id,omitempty"`
	GroupID      *int64  `json:"group_id,omitempty"`
	AccountID    *int64  `json:"account_id,omitempty"`
	ModelPattern *string `json:"model_pattern,omitempty"`
}

type ServiceQuotaPathInput struct {
	Platform     *string `json:"platform"`
	ChannelID    *int64  `json:"channel_id"`
	GroupID      *int64  `json:"group_id"`
	AccountID    *int64  `json:"account_id"`
	ModelPattern *string `json:"model_pattern"`
}

// ServiceQuotaRuleUserRef 是绑定用户的轻量引用，用于前端 chip 展示（规则只在 counter_mode=user 时携带此字段）。
type ServiceQuotaRuleUserRef struct {
	ID    int64  `json:"id"`
	Email string `json:"email"`
}

// ServiceQuotaRule 一条层级规则。
//
// 一条规则 = N 个限流器 × M 条路径 × 用户绑定。每个 path×limiter 组合在 Redis 中
// 维护独立的计数器。CounterMode 与 IsFallback 正交：
//   - CounterMode 决定计数 key 的分片方式
//   - IsFallback=true 表示兜底规则：同一 limiter_type 有其他非 fallback 规则命中时该 limiter 自动让位
type ServiceQuotaRule struct {
	ID            int64                     `json:"id"`
	Enabled       bool                      `json:"enabled"`
	Name          *string                   `json:"name,omitempty"`
	CounterMode   string                    `json:"counter_mode"`
	IsFallback    bool                      `json:"is_fallback"`
	Limiters      []ServiceQuotaLimiterDef  `json:"limiters"`
	Paths         []ServiceQuotaPathDef     `json:"paths"`
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
	Paths         []ServiceQuotaPathInput    `json:"paths"`
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

// AccountScopeInfo / GroupScopeInfo 用于 path 链路一致性校验：下级必须是上级的子孙。
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

// ServiceQuotaUserChecker 是 service quota 在校验 target_user_ids 时使用的最小化用户存在性检查接口。
//
// 单独抽出小接口（接口隔离原则）避免侵入庞大的 UserRepository，同时让 *userRepository
// 自动满足该接口（duck typing）。返回值与 group_repo.ExistsByIDs 对齐：map[id]exists，
// 缺失 ID 由调用方按需收集。
type ServiceQuotaUserChecker interface {
	ExistsByIDs(ctx context.Context, ids []int64) (map[int64]bool, error)
}

type ServiceQuotaLimiter interface {
	Current(ctx context.Context, key string, window time.Duration, mode string) (float64, error)
	Increment(ctx context.Context, key string, delta float64, window time.Duration, mode string) (float64, error)
	Acquire(ctx context.Context, key, member string, limit int64) (bool, error)
	Release(ctx context.Context, key, member string) error
}

// ServiceQuotaCache 抽象服务限额规则与开关的缓存层。
//
// 读路径（GetRules / GetEnabled）未命中时由 service 层自行通过 singleflight
// 加载并调用 SetRules / SetEnabled 回填，写路径只调 Invalidate* 把 key 删掉，
// 让其他实例下次读时重新拉取，避免多实例间陈旧数据的不一致窗口。
type ServiceQuotaCache interface {
	GetRules(ctx context.Context) ([]*ServiceQuotaRule, bool, error)
	SetRules(ctx context.Context, rules []*ServiceQuotaRule) error
	InvalidateRules(ctx context.Context) error
	GetEnabled(ctx context.Context) (*bool, error)
	SetEnabled(ctx context.Context, enabled bool) error
	InvalidateEnabled(ctx context.Context) error
	Invalidate(ctx context.Context) error
}

type ServiceQuotaService interface {
	ListRules(ctx context.Context, filter ServiceQuotaListFilter) ([]*ServiceQuotaRule, error)
	CreateRule(ctx context.Context, input ServiceQuotaRuleInput) (*ServiceQuotaRule, error)
	UpdateRule(ctx context.Context, id int64, input ServiceQuotaRuleInput) (*ServiceQuotaRule, error)
	DeleteRule(ctx context.Context, id int64) error
	InvalidateEnabledCache(ctx context.Context)
	// ReloadCache 失效规则缓存，让所有实例下次读时重新拉取数据库。
	// 用于 handler 层在删除 channel/group/account/user 等关联实体后
	// 主动让服务限额规则缓存失效。
	ReloadCache(ctx context.Context) error
	PreCheck(ctx context.Context, req ServiceQuotaCheckRequest) (*ServiceQuotaLease, error)
	Record(ctx context.Context, req ServiceQuotaRecordRequest)
}
