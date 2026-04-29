package service

import (
	"strings"
)

// plannedRow 是笛卡尔展开的中间态：单个 (rule, path, limiter, scope_user) 组合。
// 写入到结果前要经 applyFilter 过滤，再经 fetchSnapshots 填入 current 值。
//
// scopeUserID == nil 时该行对应 shared 计数器（counter_mode=shared 或 user 模式
// 在 admin 视角下尚未限定具体 user 时不会进入这条分支——见 scopeUsersForRule）。
type plannedRow struct {
	rule        *ServiceQuotaRule
	path        ServiceQuotaPathDef
	pathIndex   int
	limiter     ServiceQuotaLimiterDef
	scopeUserID *int64
}

// expandTargets 把规则集合展开成 plannedRow 列表。
//
// counter_mode 与 filter 共同决定 scope_user 的展开方式（详见 scopeUsersForRule
// 的注释）。返回 nil scope user 列表的规则在当前 filter 下完全跳过——例如
// admin 不带 user filter 时所有 per_user 规则都不会出现在结果中（不会出占位行）。
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
		if len(users) == 0 {
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
// 三个视角的展开规则：
//
//  1. user scope（filter.UserScope != nil）— 用户视角，仅此 user 自身：
//     shared   → [nil]            （shared 计数对全员可见）
//     user     → [&UserScope.UID] （仅当该 user 在 TargetUserIDs 中；matchesUserScope 再过滤）
//     per_user → [&UserScope.UID]
//
//  2. admin 带 user filter（filter.UserID != nil）— admin 已限定具体 user：
//     shared   → [nil]               （shared 行始终展示，与 user filter 无关）
//     user     → [&filter.UserID]    （仅当该 user 在 TargetUserIDs 中；否则空）
//     per_user → [&filter.UserID]
//
//  3. admin 无 user filter（filter.UserScope == nil 且 filter.UserID == nil）：
//     shared   → [nil]                       （展开 1 行，target=shared）
//     user     → 每个 TargetUserID 一行       （admin 看全部目标用户的当前用量）
//     per_user → nil                         （完全不展开；admin 必须选具体用户才能看 per_user）
//
// 返回 nil 表示该规则在当前 filter 下完全跳过，不出现在结果中。
func scopeUsersForRule(rule *ServiceQuotaRule, filter MonitorSnapshotFilter) []*int64 {
	if filter.UserScope != nil {
		return scopeUsersForUserView(rule, filter.UserScope.UserID)
	}
	if filter.UserID != nil && *filter.UserID > 0 {
		return scopeUsersForAdminWithUser(rule, *filter.UserID)
	}
	return scopeUsersForAdminNoUser(rule)
}

// scopeUsersForUserView 处理用户视角下的 scope user 展开。
// 真正的 user 可见性过滤在 matchesUserScope 中收尾，本函数只负责生成候选行。
func scopeUsersForUserView(rule *ServiceQuotaRule, userID int64) []*int64 {
	switch rule.CounterMode {
	case ServiceQuotaCounterModeShared:
		return []*int64{nil}
	case ServiceQuotaCounterModeUser, ServiceQuotaCounterModePerUser:
		uid := userID
		return []*int64{&uid}
	default:
		return nil
	}
}

// scopeUsersForAdminWithUser 处理 admin 带 user filter 的 scope user 展开。
// user 模式下要求 filter.UserID 必须在 rule.TargetUserIDs 中，否则返回空列表
// （让该规则在该 filter 下完全消失）。
func scopeUsersForAdminWithUser(rule *ServiceQuotaRule, filterUserID int64) []*int64 {
	switch rule.CounterMode {
	case ServiceQuotaCounterModeShared:
		return []*int64{nil}
	case ServiceQuotaCounterModeUser:
		if !ruleTargetsUser(rule, filterUserID) {
			return nil
		}
		uid := filterUserID
		return []*int64{&uid}
	case ServiceQuotaCounterModePerUser:
		uid := filterUserID
		return []*int64{&uid}
	default:
		return nil
	}
}

// scopeUsersForAdminNoUser 处理 admin 无 user filter 的 scope user 展开。
// per_user 完全不展开（admin 必须选具体用户才能看 per_user 计数）。
func scopeUsersForAdminNoUser(rule *ServiceQuotaRule) []*int64 {
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
		return nil
	default:
		return nil
	}
}

// ruleTargetsUser 判断 user 模式规则的 TargetUserIDs 是否包含指定用户。
func ruleTargetsUser(rule *ServiceQuotaRule, userID int64) bool {
	for _, uid := range rule.TargetUserIDs {
		if uid == userID {
			return true
		}
	}
	return false
}

// applyFilter 在 plannedRow 上做最终过滤。UserScope 优先级最高（用户视角），
// 否则按 admin path 维度逐个匹配（user filter 已在 scopeUsersForRule 阶段消化）。
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
//   - counter_mode=per_user：scope_user_id 必须等于该 user
func matchesUserScope(userID int64, row plannedRow) bool {
	switch row.rule.CounterMode {
	case ServiceQuotaCounterModeShared:
		return true
	case ServiceQuotaCounterModeUser:
		if row.scopeUserID == nil || *row.scopeUserID != userID {
			return false
		}
		return ruleTargetsUser(row.rule, userID)
	case ServiceQuotaCounterModePerUser:
		return row.scopeUserID != nil && *row.scopeUserID == userID
	default:
		return false
	}
}

// matchesAdminFilter 处理 admin 视角下的 path 维度过滤；nil filter 字段视为"不限制该维度"。
//
// path 维度（Platform/ChannelID/GroupID/AccountID）按"path 字段 nil 视为命中"语义：
// 例如 path.ChannelID=nil 时该 path 同时命中所有 ChannelID 过滤值。
//
// RuleID 与 UserID 维度已在 scopeUsersForRule 阶段消化（user filter 决定 scope user
// 展开），此处只补一个 RuleID 兜底过滤。
func matchesAdminFilter(filter MonitorSnapshotFilter, row plannedRow) bool {
	if filter.RuleID != nil && row.rule.ID != *filter.RuleID {
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
//
// ResetAtUnixMs 由底层 limiter snapshot 透传：
//   - fixed window：key 真实 PTTL（计数器到此时刻自然清零）
//   - rolling window：当前窗口右端 = now + window 长度
//   - concurrency 或 key 不存在：0（前端据此隐藏倒计时）
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
		ResetAtUnixMs:  snap.ResetAtUnixMs,
	}
	// counter_mode 是规则的 enum 类型描述（'shared' / 'user' / 'per_user'），不属于敏感字段：
	// admin 端公开，user 端也需要它来区分"全局共享限额"和"用户独立限额"（前端按此渲染"全"badge）。
	rt.CounterMode = row.rule.CounterMode
	if !userScope {
		rt.PathSummary = pathSummaryFrom(row.path)
		rt.ScopeUserID = row.scopeUserID
	} else {
		// user 视角下仍透出 platform 与 model_pattern，让用户能看到自己被
		// 限流的"业务路径"；channel/group/account 是内部资源拓扑，故意不暴露
		// （避免侧信道泄露平台架构信息）。
		rt.PathSummary = pathSummaryFromForUser(row.path)
		// 抹掉规则内部标识：rule_id 是 admin 才需要的内部关键字（用于 ResetCounter 等管理动作）；
		// is_fallback 是规则编排细节，不应暴露给最终用户。最小信息暴露原则。
		rt.RuleID = 0
		rt.IsFallback = false
	}
	return rt
}

// pathSummaryFrom 把 ServiceQuotaPathDef 浅拷贝成 PathSummary（admin 视角，5 维都暴露）。
func pathSummaryFrom(p ServiceQuotaPathDef) *PathSummary {
	return &PathSummary{
		Platform:     p.Platform,
		ChannelID:    p.ChannelID,
		GroupID:      p.GroupID,
		AccountID:    p.AccountID,
		ModelPattern: p.ModelPattern,
	}
}

// pathSummaryFromForUser 是用户视角的 PathSummary：仅暴露 platform 与 model_pattern。
//
// 当两者都为 nil 时（即 path 完全 wildcard），返回 nil 让前端 isPathAllStar
// 走"所有请求"分支；否则返回对象，让用户能看到平台 chip 和模型限制。
func pathSummaryFromForUser(p ServiceQuotaPathDef) *PathSummary {
	if p.Platform == nil && p.ModelPattern == nil {
		return nil
	}
	return &PathSummary{
		Platform:     p.Platform,
		ModelPattern: p.ModelPattern,
		// ChannelID / GroupID / AccountID 故意不设——内部资源拓扑不向用户暴露
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
