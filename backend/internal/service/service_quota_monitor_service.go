package service

import (
	"context"
	"log/slog"
	"time"
)

// monitorMaxRows 限制单次 Snapshot 返回的最大 limiter 行数。
// 笛卡尔展开（rule × path × limiter × scope_user）在大规则集下可能爆炸，
// 截断到上限并返回 truncated=true 让前端提示用户加过滤条件。
const monitorMaxRows = 5000

// 监控视角下的 window 标签，与 limiterWindowLabel 输出一致；本文件用作 LimiterRuntime.WindowMode 的值。
const (
	monitorWindowFixed   = ServiceQuotaWindowFixed
	monitorWindowRolling = ServiceQuotaWindowRolling
	monitorWindowNone    = "none"
)

// MonitorSnapshotFilter 是 Snapshot 入参。
//
// admin 视角通过 RuleID/UserID/ChannelID/GroupID/AccountID/Platform 任意组合过滤；
// user 视角只填 UserScope，强制只返回与该用户相关的 limiter 行。两类视角互斥：
// 调用方任选其一，UserScope 优先级高于其他字段。
type MonitorSnapshotFilter struct {
	UserScope *MonitorUserScope
	RuleID    *int64
	UserID    *int64
	ChannelID *int64
	GroupID   *int64
	AccountID *int64
	Platform  *string
}

// MonitorUserScope 标识"以某个 user 视角看自己的限额"。
// 区别于 admin 端 MonitorSnapshotFilter.UserID（按 ScopeUserID/TargetUserIDs 过滤）—
// UserScope 只会保留与该 user 相关的行（target_user_ids 命中 / scope_user_id 等于自身 / shared 全员可见）。
type MonitorUserScope struct{ UserID int64 }

// MonitorSnapshot 是 Snapshot 的返回值。
//
// Enabled=false 表示 service quota 总开关关闭，此时 Items 必为空，前端展示降级提示。
// Truncated=true 表示笛卡尔展开行数超过 monitorMaxRows，已截断。
type MonitorSnapshot struct {
	Enabled    bool             `json:"enabled"`
	AsOfUnixMs int64            `json:"as_of_unix_ms"`
	Items      []LimiterRuntime `json:"items"`
	Truncated  bool             `json:"truncated"`
}

// LimiterRuntime 单条 limiter 的运行时快照。
//
// 字段命名与前端 i18n 对齐；用户视角下 PathSummary/ScopeUserID/CounterMode 抹空。
// PathIndex 是规则内 path 的 1-based 位置，方便前端按规则分组展示而无需依赖 PathID。
//
// ResetAtUnixMs 表示当前计数器/窗口下次"自然清零"的 Unix 毫秒时间戳：
//   - fixed window：key 真实 PTTL 推算的过期时刻
//   - rolling window：now + 窗口长度（窗口右端，与 PreCheck 算 cutoff 的方式对齐）
//   - concurrency 或 key 不存在：0（前端据此隐藏倒计时）
//
// 前端用 max(0, ceil((reset_at - now)/1000)) 显示"X 秒后重置"，刷新一次即更新。
type LimiterRuntime struct {
	RuleID         int64        `json:"rule_id"`
	RuleName       string       `json:"rule_name"`
	PathID         int64        `json:"path_id"`
	PathIndex      int          `json:"path_index"`
	PathSummary    *PathSummary `json:"path_summary,omitempty"`
	LimiterType    string       `json:"limiter_type"`
	WindowMode     string       `json:"window_mode"`
	LimitValue     float64      `json:"limit_value"`
	Current        float64      `json:"current"`
	UtilizationPct float64      `json:"utilization_pct"`
	CounterMode    string       `json:"counter_mode,omitempty"`
	ScopeUserID    *int64       `json:"scope_user_id,omitempty"`
	IsFallback     bool         `json:"is_fallback"`
	Exists         bool         `json:"exists"`
	ResetAtUnixMs  int64        `json:"reset_at_unix_ms,omitempty"`
}

// PathSummary 是 admin 视角下 path 的轻量副本。nil 字段含义"不限制该维度"，与
// ServiceQuotaPathDef 一致。用户视角下整个对象抹成 nil（避免泄露底层资源拓扑）。
type PathSummary struct {
	Platform     *string `json:"platform,omitempty"`
	ChannelID    *int64  `json:"channel_id,omitempty"`
	GroupID      *int64  `json:"group_id,omitempty"`
	AccountID    *int64  `json:"account_id,omitempty"`
	ModelPattern *string `json:"model_pattern,omitempty"`
}

// ServiceQuotaMonitorService 暴露服务限额规则的运行时只读快照。
//
// 与 ServiceQuotaService 解耦：本接口只读 limiter（Snapshot/SnapshotMany），不调用
// Acquire/Increment/Release，因此不会污染计数器。规则数据复用 ServiceQuotaCache，
// 与 PreCheck 主链路共享同一缓存条目，避免双份缓存与不一致。
type ServiceQuotaMonitorService interface {
	Snapshot(ctx context.Context, filter MonitorSnapshotFilter) (*MonitorSnapshot, error)
}

type serviceQuotaMonitorService struct {
	rules    ServiceQuotaRuleRepository
	limiter  ServiceQuotaLimiter
	cache    ServiceQuotaCache
	settings *SettingService
}

// NewServiceQuotaMonitorService 构造监控服务。任一依赖为 nil 时 Snapshot 会立即返回空结果，
// 让上层 handler/wire 在尚未接线时仍能返回安全降级响应。
func NewServiceQuotaMonitorService(
	rules ServiceQuotaRuleRepository,
	limiter ServiceQuotaLimiter,
	cache ServiceQuotaCache,
	settings *SettingService,
) ServiceQuotaMonitorService {
	return &serviceQuotaMonitorService{
		rules:    rules,
		limiter:  limiter,
		cache:    cache,
		settings: settings,
	}
}

// Snapshot 是唯一入口：返回当前所有规则 × 路径 × 限流器 的实时计数。
//
// 算法：
//  1. 读 setting service_quota_enabled，关则返回空（fail-open，与 PreCheck 行为对齐）
//  2. 缓存命中则用缓存的规则集，未命中则 List 后回填缓存
//  3. expandTargets 做笛卡尔展开（rule × path × limiter × scope_user）
//  4. applyFilter 应用 admin/user 过滤
//  5. 截断到 monitorMaxRows
//  6. 调 limiter.SnapshotMany 一次性拿到所有当前值（错误时整批降级为 Exists=false）
//  7. 拼装 LimiterRuntime，user 视角下抹掉 PathSummary/ScopeUserID/CounterMode
func (s *serviceQuotaMonitorService) Snapshot(ctx context.Context, filter MonitorSnapshotFilter) (*MonitorSnapshot, error) {
	now := time.Now().UnixMilli()
	empty := &MonitorSnapshot{Enabled: true, AsOfUnixMs: now, Items: []LimiterRuntime{}}
	if s == nil || s.rules == nil || s.limiter == nil || s.settings == nil {
		empty.Enabled = false
		return empty, nil
	}
	if !s.snapshotEnabled(ctx) {
		empty.Enabled = false
		return empty, nil
	}
	rules, err := s.loadRules(ctx)
	if err != nil {
		return nil, err
	}
	if len(rules) == 0 {
		return empty, nil
	}
	rows, truncated := s.planRows(rules, filter)
	keys := buildSnapshotKeys(rows)
	snapshots := s.fetchSnapshots(ctx, keys)
	items := assembleRuntime(rows, snapshots, filter.UserScope != nil)
	return &MonitorSnapshot{
		Enabled:    true,
		AsOfUnixMs: now,
		Items:      items,
		Truncated:  truncated,
	}, nil
}

// planRows 把规则集合按 filter 展开成 plannedRow，并按 monitorMaxRows 截断。
// 拆出来让 Snapshot 主流程保持 ≤30 行：planning（展开 + 过滤 + 截断）属于
// 纯数据流转，与 Redis 批量读 / 结果拼装是各自独立的阶段。
func (s *serviceQuotaMonitorService) planRows(rules []*ServiceQuotaRule, filter MonitorSnapshotFilter) ([]plannedRow, bool) {
	rows := s.expandTargets(rules, filter)
	rows = applyFilter(filter, rows)
	truncated := false
	if len(rows) > monitorMaxRows {
		rows = rows[:monitorMaxRows]
		truncated = true
	}
	return rows, truncated
}

// snapshotEnabled 委托给 serviceQuotaIsEnabled 包级函数，与 PreCheck/Record 共享同一份
// "读 cache 优先 + fail-open 走 settings + 回填 cache" 实现。任何一边新增逻辑都必须改 helper
// 而不是 fork 一份新副本。
func (s *serviceQuotaMonitorService) snapshotEnabled(ctx context.Context) bool {
	return serviceQuotaIsEnabled(ctx, s.cache, s.settings)
}

// loadRules 走 ServiceQuotaCache（与 PreCheck 共享），未命中时 List 并回填。
// 失败上抛给调用方决定（不像 isEnabled 那样吞错；监控页有错误就让前端看到原因）。
func (s *serviceQuotaMonitorService) loadRules(ctx context.Context) ([]*ServiceQuotaRule, error) {
	if s.cache != nil {
		rules, hit, err := s.cache.GetRules(ctx)
		if err == nil && hit {
			return rules, nil
		}
	}
	enabled := true
	rules, err := s.rules.List(ctx, ServiceQuotaListFilter{Enabled: &enabled})
	if err != nil {
		return nil, err
	}
	if s.cache != nil {
		_ = s.cache.SetRules(ctx, rules)
	}
	return rules, nil
}

// fetchSnapshots 调底层 limiter.SnapshotMany；失败时降级为全空（Exists=false），打 warn。
func (s *serviceQuotaMonitorService) fetchSnapshots(ctx context.Context, keys []SnapshotKey) []LimiterSnapshot {
	if len(keys) == 0 {
		return nil
	}
	res, err := s.limiter.SnapshotMany(ctx, keys)
	if err != nil {
		slog.WarnContext(ctx, "service quota monitor snapshot batch failed",
			"key_count", len(keys), "error", err)
		return make([]LimiterSnapshot, len(keys))
	}
	if len(res) != len(keys) {
		slog.WarnContext(ctx, "service quota monitor snapshot length mismatch",
			"expected", len(keys), "got", len(res))
		return make([]LimiterSnapshot, len(keys))
	}
	return res
}
