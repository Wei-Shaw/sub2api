package service

import (
	"context"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// ---- 错误变量（feature issue #1750） ----
var (
	// ErrQuotaExceeded 配额超限（HTTP 429）。
	// 通过 WithMetadata 携带 scope/limit_usd/used_usd/reset_at（可选 rule_id）。
	ErrQuotaExceeded = infraerrors.TooManyRequests(
		"USAGE_QUOTA_EXCEEDED",
		"daily usage quota exceeded",
	)
	// ErrRuleGroupsOverlap 规则 group_ids 与同用户其他规则重叠（HTTP 409）
	ErrRuleGroupsOverlap = infraerrors.Conflict(
		"QUOTA_RULE_GROUPS_OVERLAP",
		"quota rule group_ids overlap with existing rule",
	)
	// ErrRuleGroupSubscription 规则 group_ids 包含订阅分组（HTTP 400）
	ErrRuleGroupSubscription = infraerrors.BadRequest(
		"QUOTA_RULE_GROUP_SUBSCRIPTION",
		"subscription groups are not allowed in quota rules",
	)
	// ErrRuleGroupNotFound 规则 group_ids 元素不存在（HTTP 400）
	ErrRuleGroupNotFound = infraerrors.BadRequest(
		"QUOTA_RULE_GROUP_NOT_FOUND",
		"quota rule references non-existent group",
	)
	// ErrQuotaRuleNotFound 规则不存在（HTTP 404）
	ErrQuotaRuleNotFound = infraerrors.NotFound(
		"QUOTA_RULE_NOT_FOUND",
		"quota rule not found",
	)
)

// ---- DTO ----

// ResolvedQuota 用户当前生效的配额快照（resolve 后结果）
type ResolvedQuota struct {
	UserID     int64       `json:"user_id"`
	Enabled    bool        `json:"enabled"`
	DailyLimit *float64    `json:"daily_limit"` // nil = 不限
	Rules      []QuotaRule `json:"rules"`
	ResolvedAt time.Time   `json:"resolved_at"`
}

// QuotaRule 规则 DTO（与 DB 行一一对应）
type QuotaRule struct {
	ID            int64     `json:"id"`
	UserID        int64     `json:"user_id"`
	GroupIDs      []int64   `json:"group_ids"`
	DailyLimitUSD float64   `json:"daily_limit_usd"`
	Period        string    `json:"period"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// UserQuotaView Admin 侧 GET /quota 的完整视图
type UserQuotaView struct {
	UserOverride UserQuotaOverride   `json:"user_override"`
	Resolved     *ResolvedQuota      `json:"resolved"`
	TodayUsage   *QuotaUsageSnapshot `json:"today_usage"`
}

// UserQuotaOverride 用户自身写入的原始配置（未 resolve）
type UserQuotaOverride struct {
	UsageLimitEnabled  *bool    `json:"usage_limit_enabled"`
	DailyUsageLimitUSD *float64 `json:"daily_usage_limit_usd"`
}

// QuotaUsageSnapshot 今日已用数据
type QuotaUsageSnapshot struct {
	TotalUsedUSD float64           `json:"total_used_usd"`
	RulesUsed    map[int64]float64 `json:"rules_used"`
	ResetAt      time.Time         `json:"reset_at"`
}

// UpdateUserQuotaRequest Admin PUT /quota 的请求
//
// 双指针三态约定（见契约 §0.7）：
//   - 字段缺失（外层 nil，**bool == nil）→ 不改
//   - JSON 值为 null（外层非 nil，内层 nil）→ 清空回"跟随全局/不限"
//   - JSON 值为 true/false/数值（外层非 nil，内层非 nil）→ 写入该值
//
// 不加 omitempty：`encoding/json` 对 **bool/**float64 的 omitempty 判定为"外层指针为 nil
// 才省略"，与上面三态一致；但显式不加 tag 可避免误以为"值为 null 时也省略"，也方便前端看
// JSON 响应里明确知道该字段存在。
type UpdateUserQuotaRequest struct {
	UsageLimitEnabled  **bool    `json:"usage_limit_enabled"`
	DailyUsageLimitUSD **float64 `json:"daily_usage_limit_usd"`
}

// CreateRuleRequest 新增规则请求
type CreateRuleRequest struct {
	GroupIDs      []int64 `json:"group_ids" binding:"required,min=1"`
	DailyLimitUSD float64 `json:"daily_limit_usd" binding:"required,gt=0"`
	Period        string  `json:"period,omitempty"`
}

// UpdateRuleRequest 更新规则请求
type UpdateRuleRequest struct {
	GroupIDs      *[]int64 `json:"group_ids"`
	DailyLimitUSD *float64 `json:"daily_limit_usd"`
}

// ReplaceRuleInput 批量替换规则的单条输入（对应 CreateRuleRequest 子集）。
//
// 专用于 PUT /api/v1/admin/users/:id/quota/rules：整体 DELETE 后 INSERT。
type ReplaceRuleInput struct {
	GroupIDs      []int64 `json:"group_ids" binding:"required,min=1"`
	DailyLimitUSD float64 `json:"daily_limit_usd" binding:"required,gt=0"`
	Period        string  `json:"period,omitempty"`
}

// ---- Repository 接口（quota 专用） ----

// UserUsageLimitRuleRepository 规则仓储契约
type UserUsageLimitRuleRepository interface {
	ListByUser(ctx context.Context, userID int64) ([]*QuotaRule, error)
	GetByIDForUser(ctx context.Context, userID, ruleID int64) (*QuotaRule, error)
	Create(ctx context.Context, userID int64, req CreateRuleRequest) (*QuotaRule, error)
	Update(ctx context.Context, userID, ruleID int64, req UpdateRuleRequest) (*QuotaRule, error)
	Delete(ctx context.Context, userID, ruleID int64) error
	// ReplaceAll 在单事务内清空指定用户的所有规则并批量写入新规则。
	// 校验需在 service 层完成，repo 只负责事务边界。
	ReplaceAll(ctx context.Context, userID int64, rules []CreateRuleRequest) ([]*QuotaRule, error)
}

// QuotaUserWriter 用户配额字段写入接口（避免依赖完整 UserRepository）
type QuotaUserWriter interface {
	UpdateUsageLimit(ctx context.Context, userID int64, enabled *bool, dailyUsageLimitUSD *float64) error
}

// ---- QuotaService 接口 ----

// QuotaService 用户每日配额限制服务
type QuotaService interface {
	Resolve(ctx context.Context, userID int64) (*ResolvedQuota, error)
	MatchRule(resolved *ResolvedQuota, groupID int64) *QuotaRule
	GetUserQuota(ctx context.Context, userID int64) (*UserQuotaView, error)
	UpdateUserQuota(ctx context.Context, userID int64, req UpdateUserQuotaRequest) error
	ListRules(ctx context.Context, userID int64) ([]*QuotaRule, error)
	CreateRule(ctx context.Context, userID int64, req CreateRuleRequest) (*QuotaRule, error)
	UpdateRule(ctx context.Context, userID, ruleID int64, req UpdateRuleRequest) (*QuotaRule, error)
	DeleteRule(ctx context.Context, userID, ruleID int64) error
	// ReplaceUserRules 全量替换用户规则：单事务内 DELETE 旧规则 + 批量 INSERT 新规则。
	// 前置在应用层做校验（分组重叠、订阅分组、limit > 0）。校验或事务失败整体回滚。
	ReplaceUserRules(ctx context.Context, userID int64, rules []ReplaceRuleInput) ([]*QuotaRule, error)
	GetTodayUsage(ctx context.Context, userID int64, resolved *ResolvedQuota) (*QuotaUsageSnapshot, error)
}
