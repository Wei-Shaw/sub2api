package service

import (
	"context"
	"encoding/json"
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
// 三态约定（见契约 §0.7）：
//   - 字段缺失：不改该列
//   - JSON 值为 null：清空回"跟随全局/不限"（写 NULL）
//   - JSON 值为 true/false/数值：写入该值
//
// 之所以自定义 UnmarshalJSON 而不继续用双指针（**bool/**float64）：
// encoding/json 对双指针的解码会把"字段缺失"和"值为 null"都产出外层 *nil，两者无法在
// 反序列化阶段区分。改成单指针 + `<field>Set bool` 私有标志后，我们在 UnmarshalJSON
// 里用 map[string]json.RawMessage 先看 key 是否出现，再决定是否把 Set=true。
// 下游（UpdateUserQuota）判断"是否更新"时读 UsageLimitEnabledSet / DailyUsageLimitUSDSet，
// 判断"是否清空回默认"时读字段本身是否为 nil，语义明确。
type UpdateUserQuotaRequest struct {
	UsageLimitEnabled  *bool    `json:"usage_limit_enabled"`
	DailyUsageLimitUSD *float64 `json:"daily_usage_limit_usd"`

	// usageLimitEnabledSet / dailyUsageLimitUSDSet 未导出：仅 UnmarshalJSON 置位，
	// 调用方通过 HasUsageLimitEnabled / HasDailyUsageLimitUSD 只读查询。不暴露写入，
	// 防止手工构造 request 对象时错设导致"没发字段却被更新"的 bug。
	usageLimitEnabledSet  bool
	dailyUsageLimitUSDSet bool
}

// HasUsageLimitEnabled 返回请求体是否提供了 usage_limit_enabled 字段（包含显式 null）。
func (r *UpdateUserQuotaRequest) HasUsageLimitEnabled() bool {
	return r.usageLimitEnabledSet
}

// HasDailyUsageLimitUSD 返回请求体是否提供了 daily_usage_limit_usd 字段（包含显式 null）。
func (r *UpdateUserQuotaRequest) HasDailyUsageLimitUSD() bool {
	return r.dailyUsageLimitUSDSet
}

// UnmarshalJSON 实现三态反序列化：通过 map[string]json.RawMessage 捕获 key 存在性，
// 然后按单字段解码到指针，区分字段缺失 / 显式 null / 有值三种情形。
func (r *UpdateUserQuotaRequest) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if v, ok := raw["usage_limit_enabled"]; ok {
		r.usageLimitEnabledSet = true
		if !isJSONNull(v) {
			var b bool
			if err := json.Unmarshal(v, &b); err != nil {
				return err
			}
			r.UsageLimitEnabled = &b
		}
	}
	if v, ok := raw["daily_usage_limit_usd"]; ok {
		r.dailyUsageLimitUSDSet = true
		if !isJSONNull(v) {
			var f float64
			if err := json.Unmarshal(v, &f); err != nil {
				return err
			}
			r.DailyUsageLimitUSD = &f
		}
	}
	return nil
}

// isJSONNull 判断 RawMessage 是否是字面量 null（允许前后空白）。
// 标准 JSON 反序列化对 null 字面量的表示就是四字符 "null"，但 json.RawMessage
// 保留原始字节，可能带前后空白（取决于上游是否紧凑化），这里做宽容比较。
func isJSONNull(v json.RawMessage) bool {
	// 去掉首尾空白后精确匹配 "null"
	start, end := 0, len(v)
	for start < end {
		c := v[start]
		if c != ' ' && c != '\t' && c != '\n' && c != '\r' {
			break
		}
		start++
	}
	for end > start {
		c := v[end-1]
		if c != ' ' && c != '\t' && c != '\n' && c != '\r' {
			break
		}
		end--
	}
	return string(v[start:end]) == "null"
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

// QuotaUserWriter 用户配额字段写入接口，只暴露"更新配额相关字段"这一能力。
//
// 设计动机：配额服务只需要写 users.usage_limit_enabled / daily_usage_limit_usd 两列，
// 而 UserRepository 聚集了 30+ 用户相关方法，引入会让 QuotaService 依赖过大。
// 用单方法小接口隔离，既便于单测 mock，也为未来拆出 quota 独立存储留口子。
//
// 注：当前 wire 装配中 QuotaUserWriter 和 UserRepository 被同一个 *userRepository
// 同时实现（双注入），这是 ISP 预留的第二实现占位，不是代码错误。
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
	ReplaceUserRules(ctx context.Context, userID int64, rules []CreateRuleRequest) ([]*QuotaRule, error)
	GetTodayUsage(ctx context.Context, userID int64, resolved *ResolvedQuota) (*QuotaUsageSnapshot, error)
}
