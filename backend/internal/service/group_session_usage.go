package service

import (
	"context"
	"time"
)

// groupSessionUsage 是分组活跃会话软上限的快照结果。
type groupSessionUsage struct {
	// Used 是本分组当前持有的活跃会话数（去重后的 sessionHash 数量）。
	Used int
	// ContainsSession 表示被查询的 sessionHash 已计入 Used，
	// 因此它不会占用新的分组会话名额。
	ContainsSession bool
	// Computed 为 false 表示缓存不可用或查询失败，调用方必须失败开放。
	Computed bool
}

// sessionLimitedAccounts 提取启用了账号级 max_sessions 的账号及其 idle timeout。
// 未启用的账号不会写入会话集合，查询它们没有意义。
func sessionLimitedAccounts(accounts []Account) ([]int64, map[int64]time.Duration) {
	accountIDs := make([]int64, 0, len(accounts))
	idleTimeouts := make(map[int64]time.Duration, len(accounts))
	for i := range accounts {
		account := &accounts[i]
		if account.GetMaxSessions() <= 0 {
			continue
		}
		accountIDs = append(accountIDs, account.ID)
		idleTimeouts[account.ID] = time.Duration(account.GetSessionIdleTimeoutMinutes()) * time.Minute
	}
	return accountIDs, idleTimeouts
}

// computeGroupSessionUsage 统计某个分组当前持有的活跃会话数。
//
// 分组维度不额外维护 Redis 结构，也不引入第二个 idle timeout 来源，使用量完全由
// 现有数据推导：
//  1. 按各账号自己的 idle timeout 读取其活跃 sessionHash 集合；
//  2. 对 sessionHash 去重后批量读取 sticky_session:{groupID}:{sessionHash};
//  3. 仅当 sticky 绑定的账号仍在传入的分组账号集合内、且该 sessionHash 在绑定账号
//     的活跃集合中时计数。
//
// 第 3 步保证共享账号上由其他分组建立的会话不会被计入本分组，也过滤掉 failover
// 之后指向已失效账号的残留绑定。
//
// 该统计是快照，天然是软限制：调用方在快照之后才做账号级原子注册，两者之间存在
// TOCTOU 窗口，高并发下允许极少量瞬时超额。
func computeGroupSessionUsage(
	ctx context.Context,
	sessionCache SessionLimitCache,
	stickyCache GatewayCache,
	groupID int64,
	accountIDs []int64,
	idleTimeouts map[int64]time.Duration,
	sessionID string,
) groupSessionUsage {
	usage := groupSessionUsage{}
	if sessionCache == nil || stickyCache == nil || groupID <= 0 {
		return usage
	}
	if len(accountIDs) == 0 {
		usage.Computed = true
		return usage
	}

	activeByAccount, err := sessionCache.GetActiveSessionsBatch(ctx, accountIDs, idleTimeouts)
	if err != nil {
		return usage
	}

	candidates := make([]string, 0)
	seen := make(map[string]struct{})
	for _, sessions := range activeByAccount {
		for sessionHash := range sessions {
			if _, ok := seen[sessionHash]; ok {
				continue
			}
			seen[sessionHash] = struct{}{}
			candidates = append(candidates, sessionHash)
		}
	}
	if len(candidates) == 0 {
		usage.Computed = true
		return usage
	}

	bindings, err := stickyCache.GetSessionAccountIDBatch(ctx, groupID, candidates)
	if err != nil {
		return usage
	}

	groupAccountIDs := make(map[int64]struct{}, len(accountIDs))
	for _, accountID := range accountIDs {
		groupAccountIDs[accountID] = struct{}{}
	}

	for sessionHash, accountID := range bindings {
		if _, ok := groupAccountIDs[accountID]; !ok {
			continue
		}
		if _, ok := activeByAccount[accountID][sessionHash]; !ok {
			continue
		}
		usage.Used++
		if sessionID != "" && sessionHash == sessionID {
			usage.ContainsSession = true
		}
	}
	usage.Computed = true
	return usage
}
