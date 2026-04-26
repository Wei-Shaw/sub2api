package service

import (
	"strings"
)

// plannedRow 是笛卡尔展开的中间态：单个 (rule, path, limiter, scope_user) 组合。
// 写入到结果前要经 applyFilter 过滤，再经 fetchSnapshots 填入 current 值。
type plannedRow struct {
	rule        *ServiceQuotaRule
	path        ServiceQuotaPathDef
	pathIndex   int
	limiter     ServiceQuotaLimiterDef
	scopeUserID *int64
}

// expandTargets 把规则集合展开成 plannedRow 列表。
//
// counter_mode 决定 scope_user 的展开方式：
//   - shared   → 单行，scope_user_id=nil
//   - user     → 每个 target_user_id 一行
//   - per_user → 仅当 filter 提供 user_id 或 user_scope 时按该 user 展开（v1 限制：admin
//     端不指定用户时跳过，避免笛卡尔爆炸；用户端通过 UserScope 已经隐式提供 user_id）
//
// path_index 是规则内 path 的 1-based 位置（按 rule.Paths 顺序）。
func (s *serviceQuotaMonitorService) expandTargets(rules []*ServiceQuotaRule, filter MonitorSnapshotFilter) []plannedRow {
	if len(rules) == 0 {
		return nil
	}
	out := make([]plannedRow, 0)
	for _, rule := range rules {
		if rule == nil || !rule.Enabled {
			continue
		}
		users := scopeUsersForRule(rule, filter)
		if users == nil {
			continue
		}
		expandRuleRows(rule, users, &out)
	}
	return out
}

// expandRuleRows 是 expandTargets 的子流程：单条 rule × 该 rule 的 scope user 集合，
// 笛卡尔展开成 plannedRow。拆出来是为了让 expandTargets 保持 ≤30 行。
func expandRuleRows(rule *ServiceQuotaRule, users []*int64, out *[]plannedRow) {
	for pathIdx, path := range rule.Paths {
		for _, lim := range rule.Limiters {
			for _, scopeUser := range users {
				*out = append(*out, plannedRow{
					rule:        rule,
					path:        path,
					pathIndex:   pathIdx + 1,
					limiter:     lim,
					scopeUserID: scopeUser,
				})
			}
		}
	}
}

// scopeUsersForRule 根据 rule.CounterMode 与 filter 决定要展开成哪些 scope user。
//
// 返回 nil 表示该规则在当前 filter 下完全不展开（per_user 没有提供 user 维度的情况）；
// 返回 [{nil}] 表示 shared 计数器（单行，target=shared）。
func scopeUsersForRule(rule *ServiceQuotaRule, filter MonitorSnapshotFilter) []*int64 {
	switch rule.CounterMode {
	case ServiceQuotaCounterModeShared:
		return []*int64{nil}
	case ServiceQuotaCounterModeUser:
		if len(rule.TargetUserIDs) == 0 {
			return nil
		}
		users := make([]*int64, 0, len(rule.TargetUserIDs))
		for _, uid := range rule.TargetUserIDs {
			id := uid
			users = append(users, &id)
		}
		return users
	case ServiceQuotaCounterModePerUser:
		if filter.UserScope != nil {
			id := filter.UserScope.UserID
			return []*int64{&id}
		}
		if filter.UserID != nil && *filter.UserID > 0 {
			id := *filter.UserID
			return []*int64{&id}
		}
		return nil
	default:
		return nil
	}
}

// applyFilter 在 plannedRow 上做最终过滤。UserScope 优先级最高（用户视角），
// 否则按 admin 字段逐个匹配。
func applyFilter(filter MonitorSnapshotFilter, rows []plannedRow) []plannedRow {
	if len(rows) == 0 {
		return rows
	}
	out := make([]plannedRow, 0, len(rows))
	for _, row := range rows {
		if filter.UserScope != nil {
			if matchesUserScope(filter.UserScope.UserID, row) {
				out = append(out, row)
			}
			continue
		}
		if matchesAdminFilter(filter, row) {
			out = append(out, row)
		}
	}
	return out
}

// matchesUserScope 判断给定 row 是否对该 user 可见。
//
//   - counter_mode=shared：对全体可见
//   - counter_mode=user：rule 必须 target 该 user，且本行 scope_user_id 必须等于该 user
//     （expandTargets 把 user 模式展开成 N 行，每个 target_user 一行；用户视角下只保留自己那行）
//   - counter_mode=per_user：scope_user_id 必须等于该 user
func matchesUserScope(userID int64, row plannedRow) bool {
	switch row.rule.CounterMode {
	case ServiceQuotaCounterModeShared:
		return true
	case ServiceQuotaCounterModeUser:
		if row.scopeUserID == nil || *row.scopeUserID != userID {
			return false
		}
		for _, uid := range row.rule.TargetUserIDs {
			if uid == userID {
				return true
			}
		}
		return false
	case ServiceQuotaCounterModePerUser:
		return row.scopeUserID != nil && *row.scopeUserID == userID
	default:
		return false
	}
}

// matchesAdminFilter 处理 admin 视角下的复合过滤；nil filter 字段视为"不限制该维度"。
//
// path 维度（Platform/ChannelID/GroupID/AccountID）按"path 字段 nil 视为命中"语义：
// 例如 path.ChannelID=nil 时该 path 同时命中所有 ChannelID 过滤值。
//
// UserID 同时作用于 scope_user_id（per_user/user 模式按 user 分片的当前行）和
// rule.TargetUserIDs（user 模式下规则绑定的用户列表），两者命中其一即通过。
func matchesAdminFilter(filter MonitorSnapshotFilter, row plannedRow) bool {
	if filter.RuleID != nil && row.rule.ID != *filter.RuleID {
		return false
	}
	if filter.UserID != nil && !matchesUserIDFilter(*filter.UserID, row) {
		return false
	}
	if filter.Platform != nil && row.path.Platform != nil && !strings.EqualFold(*filter.Platform, *row.path.Platform) {
		return false
	}
	if filter.ChannelID != nil && row.path.ChannelID != nil && *filter.ChannelID != *row.path.ChannelID {
		return false
	}
	if filter.GroupID != nil && row.path.GroupID != nil && *filter.GroupID != *row.path.GroupID {
		return false
	}
	if filter.AccountID != nil && row.path.AccountID != nil && *filter.AccountID != *row.path.AccountID {
		return false
	}
	return true
}

// matchesUserIDFilter 检查 admin filter.UserID 是否与某 row 关联。
//
// 对按 user 分片的行（per_user 与 user 模式，row.scopeUserID 非 nil）只保留
// scope_user_id 与 filter.UserID 完全一致的那一行；对共享行（shared 模式或 fallback
// 通配，row.scopeUserID == nil）回退到 rule.TargetUserIDs 包含目标 user 的检查，
// 让 admin 在按 user 过滤时看到与该 user 相关的所有规则（包括 shared 规则）。
func matchesUserIDFilter(target int64, row plannedRow) bool {
	if row.scopeUserID != nil {
		return *row.scopeUserID == target
	}
	if len(row.rule.TargetUserIDs) == 0 {
		return true // shared 行没有 target，过滤维度上视作通配
	}
	for _, uid := range row.rule.TargetUserIDs {
		if uid == target {
			return true
		}
	}
	return false
}

// buildSnapshotKeys 把 plannedRow 列表转换成 limiter.SnapshotMany 的入参。
//
// concurrency 走 ZSET 路径（IsConcurrency=true、Mode 与 Window 由底层忽略），
// fixed/rolling 走 GET / ZRangeByScoreWithScores 路径，使用 serviceQuotaWindow + WindowMode。
//
// Key 由 BuildServiceQuotaCounterKey 生成——绝对不能在这里复制粘贴 key 公式，
// 否则与 PreCheck 主链路的写路径会不一致，监控页读出的会是空数据。
func buildSnapshotKeys(rows []plannedRow) []SnapshotKey {
	if len(rows) == 0 {
		return nil
	}
	out := make([]SnapshotKey, 0, len(rows))
	for _, row := range rows {
		key := BuildServiceQuotaCounterKey(row.rule.ID, row.path.ID, row.limiter.LimiterType, row.scopeUserID)
		if row.limiter.LimiterType == ServiceQuotaLimiterConcurrency {
			out = append(out, SnapshotKey{Key: key, IsConcurrency: true})
			continue
		}
		out = append(out, SnapshotKey{
			Key:    key,
			Window: serviceQuotaWindow(row.limiter),
			Mode:   row.limiter.WindowMode,
		})
	}
	return out
}

// assembleRuntime 把 plannedRow + 对应位置的 LimiterSnapshot 拼装成 LimiterRuntime。
//
// userScope=true 时抹掉 PathSummary / ScopeUserID / CounterMode 三个 admin 专属字段。
// snapshots 长度可能小于 rows（极端 fail-soft 情况），缺失位以 Exists=false 兜底。
func assembleRuntime(rows []plannedRow, snapshots []LimiterSnapshot, userScope bool) []LimiterRuntime {
	if len(rows) == 0 {
		return []LimiterRuntime{}
	}
	out := make([]LimiterRuntime, 0, len(rows))
	for i, row := range rows {
		var snap LimiterSnapshot
		if i < len(snapshots) {
			snap = snapshots[i]
		}
		out = append(out, buildLimiterRuntime(row, snap, userScope))
	}
	return out
}

// buildLimiterRuntime 单条转换。拆出来便于测试。
func buildLimiterRuntime(row plannedRow, snap LimiterSnapshot, userScope bool) LimiterRuntime {
	rt := LimiterRuntime{
		RuleID:         row.rule.ID,
		RuleName:       ruleDisplayName(row.rule),
		PathID:         row.path.ID,
		PathIndex:      row.pathIndex,
		LimiterType:    row.limiter.LimiterType,
		WindowMode:     monitorWindowLabel(row.limiter),
		LimitValue:     row.limiter.LimitValue,
		Current:        snap.Current,
		UtilizationPct: utilizationPct(snap.Current, row.limiter.LimitValue),
		IsFallback:     row.rule.IsFallback,
		Exists:         snap.Exists,
	}
	if !userScope {
		rt.PathSummary = pathSummaryFrom(row.path)
		rt.CounterMode = row.rule.CounterMode
		rt.ScopeUserID = row.scopeUserID
	}
	return rt
}

// pathSummaryFrom 把 ServiceQuotaPathDef 浅拷贝成 PathSummary（前端对外契约）。
func pathSummaryFrom(p ServiceQuotaPathDef) *PathSummary {
	return &PathSummary{
		Platform:     p.Platform,
		ChannelID:    p.ChannelID,
		GroupID:      p.GroupID,
		AccountID:    p.AccountID,
		ModelPattern: p.ModelPattern,
	}
}

// monitorWindowLabel 与 limiterWindowLabel 同语义（concurrency=none，否则 fixed/rolling）。
// 复用现有函数避免双份实现，只是把返回值映射到本文件常量名以便维护。
func monitorWindowLabel(lim ServiceQuotaLimiterDef) string {
	switch limiterWindowLabel(lim) {
	case ServiceQuotaWindowFixed:
		return monitorWindowFixed
	case ServiceQuotaWindowRolling:
		return monitorWindowRolling
	default:
		return monitorWindowNone
	}
}

// utilizationPct 计算"已用量占比"，limit<=0 时返回 0；上限 100，避免 UI 出现 120%。
func utilizationPct(current, limit float64) float64 {
	if limit <= 0 {
		return 0
	}
	pct := current / limit * 100
	if pct < 0 {
		return 0
	}
	if pct > 100 {
		return 100
	}
	return pct
}
