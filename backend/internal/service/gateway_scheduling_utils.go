package service

import (
	"context"
	"sort"
)

// ─────────────────────────────────────────────────────────────────────────────
// 调度共享工具：消除账号选择中的重复模式
// ─────────────────────────────────────────────────────────────────────────────

// eligibilityOpts 账号调度资格检查的选项。
type eligibilityOpts struct {
	platform       string // 目标平台
	useMixed       bool   // 是否混合调度模式
	requestedModel string // 请求的模型名
	isSticky       bool   // 是否粘性路径（影响 WindowCost/RPM 阈值宽松度）
}

// isAccountEligibleForScheduling 执行完整的 7 项调度资格检查。
// 统一了 tryModelRoutingSelection / selectByLoadBalance / tryStickySessionSelection 中
// 重复出现的门控检查逻辑。isSticky=true 时 WindowCost/RPM 使用宽松阈值。
func (s *GatewayService) isAccountEligibleForScheduling(ctx context.Context, acc *Account, opts eligibilityOpts) bool {
	if !s.isAccountSchedulableForSelection(acc) {
		return false
	}
	if !s.isAccountAllowedForPlatform(acc, opts.platform, opts.useMixed) {
		return false
	}
	if opts.requestedModel != "" && !s.isModelSupportedByAccountWithContext(ctx, acc, opts.requestedModel) {
		return false
	}
	if !s.isAccountSchedulableForModelSelection(ctx, acc, opts.requestedModel) {
		return false
	}
	if !s.isAccountSchedulableForQuota(acc) {
		return false
	}
	if !s.isAccountSchedulableForWindowCost(ctx, acc, opts.isSticky) {
		return false
	}
	if !s.isAccountSchedulableForRPM(ctx, acc, opts.isSticky) {
		return false
	}
	return true
}

// isBetterAccountCandidate 判断 candidate 是否优于 current（优先级→LRU→OAuth偏好）。
// oauthPlatformFilter 非空时，仅在两者都属于该平台时才应用 OAuth 偏好。
func isBetterAccountCandidate(candidate, current *Account, preferOAuth bool, oauthPlatformFilter string) bool {
	if candidate.Priority < current.Priority {
		return true
	}
	if candidate.Priority > current.Priority {
		return false
	}
	// 同优先级：比较 LastUsedAt（nil 视为最优）
	switch {
	case candidate.LastUsedAt == nil && current.LastUsedAt != nil:
		return true
	case candidate.LastUsedAt != nil && current.LastUsedAt == nil:
		return false
	case candidate.LastUsedAt == nil && current.LastUsedAt == nil:
		if preferOAuth && candidate.Type != current.Type && candidate.Type == AccountTypeOAuth {
			if oauthPlatformFilter == "" {
				return true
			}
			return candidate.Platform == oauthPlatformFilter && current.Platform == oauthPlatformFilter
		}
		return false
	default:
		return candidate.LastUsedAt.Before(*current.LastUsedAt)
	}
}

// sortAccountsWithLoadByPriority 对带负载信息的账号列表按优先级→负载率→LRU排序，
// 并在同组内随机打乱以防止热点。
func sortAccountsWithLoadByPriority(accounts []accountWithLoad) {
	sort.SliceStable(accounts, func(i, j int) bool {
		a, b := accounts[i], accounts[j]
		if a.account.Priority != b.account.Priority {
			return a.account.Priority < b.account.Priority
		}
		if a.loadInfo.LoadRate != b.loadInfo.LoadRate {
			return a.loadInfo.LoadRate < b.loadInfo.LoadRate
		}
		switch {
		case a.account.LastUsedAt == nil && b.account.LastUsedAt != nil:
			return true
		case a.account.LastUsedAt != nil && b.account.LastUsedAt == nil:
			return false
		case a.account.LastUsedAt == nil && b.account.LastUsedAt == nil:
			return false
		default:
			return a.account.LastUsedAt.Before(*b.account.LastUsedAt)
		}
	})
	shuffleWithinSortGroups(accounts)
}
