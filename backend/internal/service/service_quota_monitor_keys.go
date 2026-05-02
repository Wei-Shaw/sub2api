package service

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
