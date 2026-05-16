package service

import (
	"context"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

// accountWithLoad 账号与负载信息的组合，用于负载感知调度
type accountWithLoad struct {
	account  *Account
	loadInfo *AccountLoadInfo
}

func prefetchedStickyGroupIDFromContext(ctx context.Context) (int64, bool) {
	return PrefetchedStickyGroupIDFromContext(ctx)
}

func prefetchedStickyAccountIDFromContext(ctx context.Context, groupID *int64) int64 {
	prefetchedGroupID, ok := prefetchedStickyGroupIDFromContext(ctx)
	if !ok || prefetchedGroupID != derefGroupID(groupID) {
		return 0
	}
	if accountID, ok := PrefetchedStickyAccountIDFromContext(ctx); ok && accountID > 0 {
		return accountID
	}
	return 0
}

// shouldClearStickySession 检查账号是否处于不可调度状态，需要清理粘性会话绑定。
// 委托 IsSchedulable() 判断账号级可调度性（状态、配额、过载、限流等），
// 额外检查模型级限流。
//
// shouldClearStickySession checks if an account is in an unschedulable state
// and the sticky session binding should be cleared.
// Delegates to IsSchedulable() for account-level checks, plus model-level rate limiting.
func shouldClearStickySession(account *Account, requestedModel string) bool {
	if account == nil {
		return false
	}
	if !account.IsSchedulable() {
		return true
	}
	if remaining := account.GetRateLimitRemainingTimeWithContext(context.Background(), requestedModel); remaining > 0 {
		return true
	}
	return false
}

// SelectAccount 选择账号（粘性会话+优先级）
func (s *GatewayService) SelectAccount(ctx context.Context, groupID *int64, sessionHash string) (*Account, error) {
	return s.SelectAccountForModel(ctx, groupID, sessionHash, "")
}

// SelectAccountForModel 选择支持指定模型的账号（粘性会话+优先级+模型映射）
func (s *GatewayService) SelectAccountForModel(ctx context.Context, groupID *int64, sessionHash string, requestedModel string) (*Account, error) {
	return s.SelectAccountForModelWithExclusions(ctx, groupID, sessionHash, requestedModel, nil)
}

// SelectAccountWithLoadAwareness 基于负载感知选择可用账号。
// 按优先级依次尝试：模型路由 → 粘性会话 → 负载均衡 → 排队等待。
// 调度流程文档见 docs/ACCOUNT_SCHEDULING_FLOW.md 。
func (s *GatewayService) SelectAccountWithLoadAwareness(ctx context.Context, groupID *int64, sessionHash string, requestedModel string, excludedIDs map[int64]struct{}, metadataUserID string, sub2apiUserID int64) (*AccountSelectionResult, error) {
	// 阶段1: 初始化调度上下文（配置加载、Claude Code 限制、渠道定价、粘性绑定解析）
	ctx, sc, err := s.prepareSchedulingContext(ctx, groupID, sessionHash, requestedModel, excludedIDs, metadataUserID, sub2apiUserID)
	if err != nil {
		return nil, err
	}

	// 阶段2: 非负载感知快速路径（并发控制未启用时）
	if s.concurrencyService == nil || !sc.cfg.LoadBatchEnabled {
		return s.selectWithoutLoadAwareness(ctx, sc)
	}

	// 阶段3: 加载可调度账号列表并构建查找结构
	ctx, pool, err := s.loadSchedulableAccountPool(ctx, sc)
	if err != nil {
		return nil, err
	}

	// 阶段4: 模型路由层（Layer 1）— 按模型配置的专属账号选择
	if result, err := s.tryModelRoutingSelection(ctx, sc, pool); result != nil || err != nil {
		return result, err
	}

	// 阶段5: 粘性会话层（Layer 1.5）— 尝试复用上次绑定的账号
	if result, err := s.tryStickySessionSelection(ctx, sc, pool); result != nil || err != nil {
		return result, err
	}

	// 阶段6: 负载均衡层（Layer 2）— 从候选池中选最优
	if result, err := s.selectByLoadBalance(ctx, sc, pool); result != nil || err != nil {
		return result, err
	}

	// 阶段7: 排队等待层（Layer 3）— 所有账号满载时进入等待队列
	return s.selectWithQueueFallback(ctx, sc, pool)
}

// selectAccountForModelWithPlatform 选择单平台账户（完全隔离）
func (s *GatewayService) selectAccountForModelWithPlatform(ctx context.Context, groupID *int64, sessionHash string, requestedModel string, excludedIDs map[int64]struct{}, platform string) (*Account, error) {
	preferOAuth := platform == PlatformGemini
	routingAccountIDs := s.routingAccountIDsForRequest(ctx, groupID, requestedModel, platform)

	// require_privacy_set: 获取分组信息
	var schedGroup *Group
	if groupID != nil && s.groupRepo != nil {
		schedGroup, _ = s.groupRepo.GetByID(ctx, *groupID)
	}

	var accounts []Account
	accountsLoaded := false

	// ============ Model Routing (legacy path): apply before sticky session ============
	// When load-awareness is disabled (e.g. concurrency service not configured), we still honor model routing
	// so switching model can switch upstream account within the same sticky session.
	if len(routingAccountIDs) > 0 {
		if s.debugModelRoutingEnabled() {
			logger.LegacyPrintf("service.gateway", "[ModelRoutingDebug] legacy routed begin: group_id=%v model=%s platform=%s session=%s routed_ids=%v",
				derefGroupID(groupID), requestedModel, platform, shortSessionHash(sessionHash), routingAccountIDs)
		}
		// 1) Sticky session only applies if the bound account is within the routing set.
		if sessionHash != "" && s.cache != nil {
			accountID, err := s.cache.GetSessionAccountID(ctx, derefGroupID(groupID), sessionHash)
			if err == nil && accountID > 0 && containsInt64(routingAccountIDs, accountID) {
				if _, excluded := excludedIDs[accountID]; !excluded {
					account, err := s.getSchedulableAccount(ctx, accountID)
					// 检查账号分组归属和平台匹配（确保粘性会话不会跨分组或跨平台）
					if err == nil {
						clearSticky := shouldClearStickySession(account, requestedModel)
						if clearSticky {
							_ = s.cache.DeleteSessionAccountID(ctx, derefGroupID(groupID), sessionHash)
						}
						if !clearSticky && s.isAccountInGroup(account, groupID) && account.Platform == platform && (requestedModel == "" || s.isModelSupportedByAccountWithContext(ctx, account, requestedModel)) && s.isAccountSchedulableForModelSelection(ctx, account, requestedModel) && s.isAccountSchedulableForQuota(account) && s.isAccountSchedulableForWindowCost(ctx, account, true) && s.isAccountSchedulableForRPM(ctx, account, true) && !s.isStickyAccountUpstreamRestricted(ctx, groupID, account, requestedModel) {
							if s.debugModelRoutingEnabled() {
								logger.LegacyPrintf("service.gateway", "[ModelRoutingDebug] legacy routed sticky hit: group_id=%v model=%s session=%s account=%d", derefGroupID(groupID), requestedModel, shortSessionHash(sessionHash), accountID)
							}
							return account, nil
						}
					}
				}
			}
		}

		// 2) Select an account from the routed candidates.
		forcePlatform, hasForcePlatform := ctx.Value(ctxkey.ForcePlatform).(string)
		if hasForcePlatform && forcePlatform == "" {
			hasForcePlatform = false
		}
		var err error
		accounts, _, err = s.listSchedulableAccounts(ctx, groupID, platform, hasForcePlatform)
		if err != nil {
			return nil, fmt.Errorf("query accounts failed: %w", err)
		}
		accountsLoaded = true

		// 提前预取窗口费用+RPM 计数，确保 routing 段内的调度检查调用能命中缓存
		ctx = s.withWindowCostPrefetch(ctx, accounts)
		ctx = s.withRPMPrefetch(ctx, accounts)

		routingSet := make(map[int64]struct{}, len(routingAccountIDs))
		for _, id := range routingAccountIDs {
			if id > 0 {
				routingSet[id] = struct{}{}
			}
		}

		var selected *Account
		for i := range accounts {
			acc := &accounts[i]
			if _, ok := routingSet[acc.ID]; !ok {
				continue
			}
			if _, excluded := excludedIDs[acc.ID]; excluded {
				continue
			}
			// Scheduler snapshots can be temporarily stale; re-check schedulability here to
			// avoid selecting accounts that were recently rate-limited/overloaded.
			if !s.isAccountSchedulableForSelection(acc) {
				continue
			}
			// require_privacy_set: 跳过 privacy 未设置的账号并标记异常
			if schedGroup != nil && schedGroup.RequirePrivacySet && !acc.IsPrivacySet() {
				_ = s.accountRepo.SetError(ctx, acc.ID,
					fmt.Sprintf("Privacy not set, required by group [%s]", schedGroup.Name))
				continue
			}
			if requestedModel != "" && !s.isModelSupportedByAccountWithContext(ctx, acc, requestedModel) {
				continue
			}
			if !s.isAccountSchedulableForModelSelection(ctx, acc, requestedModel) {
				continue
			}
			if !s.isAccountSchedulableForQuota(acc) {
				continue
			}
			if !s.isAccountSchedulableForWindowCost(ctx, acc, false) {
				continue
			}
			if !s.isAccountSchedulableForRPM(ctx, acc, false) {
				continue
			}
			if selected == nil {
				selected = acc
			} else if isBetterAccountCandidate(acc, selected, preferOAuth, "") {
				selected = acc
			}
		}

		if selected != nil {
			if sessionHash != "" && s.cache != nil {
				if err := s.cache.SetSessionAccountID(ctx, derefGroupID(groupID), sessionHash, selected.ID, stickySessionTTL); err != nil {
					logger.LegacyPrintf("service.gateway", "set session account failed: session=%s account_id=%d err=%v", sessionHash, selected.ID, err)
				}
			}
			if s.debugModelRoutingEnabled() {
				logger.LegacyPrintf("service.gateway", "[ModelRoutingDebug] legacy routed select: group_id=%v model=%s session=%s account=%d", derefGroupID(groupID), requestedModel, shortSessionHash(sessionHash), selected.ID)
			}
			return selected, nil
		}
		logger.LegacyPrintf("service.gateway", "[ModelRouting] No routed accounts available for model=%s, falling back to normal selection", requestedModel)
	}

	// 1. 查询粘性会话
	if sessionHash != "" && s.cache != nil {
		accountID, err := s.cache.GetSessionAccountID(ctx, derefGroupID(groupID), sessionHash)
		if err == nil && accountID > 0 {
			if _, excluded := excludedIDs[accountID]; !excluded {
				account, err := s.getSchedulableAccount(ctx, accountID)
				// 检查账号分组归属和平台匹配（确保粘性会话不会跨分组或跨平台）
				if err == nil {
					clearSticky := shouldClearStickySession(account, requestedModel)
					if clearSticky {
						_ = s.cache.DeleteSessionAccountID(ctx, derefGroupID(groupID), sessionHash)
					}
					if !clearSticky && s.isAccountInGroup(account, groupID) && account.Platform == platform && (requestedModel == "" || s.isModelSupportedByAccountWithContext(ctx, account, requestedModel)) && s.isAccountSchedulableForModelSelection(ctx, account, requestedModel) && s.isAccountSchedulableForQuota(account) && s.isAccountSchedulableForWindowCost(ctx, account, true) && s.isAccountSchedulableForRPM(ctx, account, true) {
						return account, nil
					}
				}
			}
		}
	}

	// 2. 获取可调度账号列表（单平台）
	if !accountsLoaded {
		forcePlatform, hasForcePlatform := ctx.Value(ctxkey.ForcePlatform).(string)
		if hasForcePlatform && forcePlatform == "" {
			hasForcePlatform = false
		}
		var err error
		accounts, _, err = s.listSchedulableAccounts(ctx, groupID, platform, hasForcePlatform)
		if err != nil {
			return nil, fmt.Errorf("query accounts failed: %w", err)
		}
	}

	// 批量预取窗口费用+RPM 计数，避免逐个账号查询（N+1）
	ctx = s.withWindowCostPrefetch(ctx, accounts)
	ctx = s.withRPMPrefetch(ctx, accounts)

	// 3. 按优先级+最久未用选择（考虑模型支持）
	// needsUpstreamCheck 仅在主选择循环中使用；粘性会话命中时跳过此检查，
	// 因为粘性会话优先保持连接一致性，且 upstream 计费基准极少使用。
	needsUpstreamCheck := s.needsUpstreamChannelRestrictionCheck(ctx, groupID)
	var selected *Account
	for i := range accounts {
		acc := &accounts[i]
		if _, excluded := excludedIDs[acc.ID]; excluded {
			continue
		}
		// Scheduler snapshots can be temporarily stale; re-check schedulability here to
		// avoid selecting accounts that were recently rate-limited/overloaded.
		if !s.isAccountSchedulableForSelection(acc) {
			continue
		}
		// require_privacy_set: 跳过 privacy 未设置的账号并标记异常
		if schedGroup != nil && schedGroup.RequirePrivacySet && !acc.IsPrivacySet() {
			_ = s.accountRepo.SetError(ctx, acc.ID,
				fmt.Sprintf("Privacy not set, required by group [%s]", schedGroup.Name))
			continue
		}
		if requestedModel != "" && !s.isModelSupportedByAccountWithContext(ctx, acc, requestedModel) {
			continue
		}
		if needsUpstreamCheck && s.isUpstreamModelRestrictedByChannel(ctx, *groupID, acc, requestedModel) {
			continue
		}
		if !s.isAccountSchedulableForModelSelection(ctx, acc, requestedModel) {
			continue
		}
		if !s.isAccountSchedulableForQuota(acc) {
			continue
		}
		if !s.isAccountSchedulableForWindowCost(ctx, acc, false) {
			continue
		}
		if !s.isAccountSchedulableForRPM(ctx, acc, false) {
			continue
		}
		if selected == nil {
			selected = acc
		} else if isBetterAccountCandidate(acc, selected, preferOAuth, "") {
			selected = acc
		}
	}

	if selected == nil {
		stats := s.logDetailedSelectionFailure(ctx, groupID, sessionHash, requestedModel, platform, accounts, excludedIDs, false)
		if requestedModel != "" {
			return nil, fmt.Errorf("%w supporting model: %s (%s)", ErrNoAvailableAccounts, requestedModel, summarizeSelectionFailureStats(stats))
		}
		return nil, ErrNoAvailableAccounts
	}

	// 4. 建立粘性绑定
	if sessionHash != "" && s.cache != nil {
		if err := s.cache.SetSessionAccountID(ctx, derefGroupID(groupID), sessionHash, selected.ID, stickySessionTTL); err != nil {
			logger.LegacyPrintf("service.gateway", "set session account failed: session=%s account_id=%d err=%v", sessionHash, selected.ID, err)
		}
	}

	return selected, nil
}

// selectAccountWithMixedScheduling 选择账户（支持混合调度）
// 查询原生平台账户 + 启用 mixed_scheduling 的 antigravity 账户
func (s *GatewayService) selectAccountWithMixedScheduling(ctx context.Context, groupID *int64, sessionHash string, requestedModel string, excludedIDs map[int64]struct{}, nativePlatform string) (*Account, error) {
	preferOAuth := nativePlatform == PlatformGemini
	routingAccountIDs := s.routingAccountIDsForRequest(ctx, groupID, requestedModel, nativePlatform)

	// require_privacy_set: 获取分组信息
	var schedGroup *Group
	if groupID != nil && s.groupRepo != nil {
		schedGroup, _ = s.groupRepo.GetByID(ctx, *groupID)
	}

	var accounts []Account
	accountsLoaded := false

	// ============ Model Routing (legacy path): apply before sticky session ============
	if len(routingAccountIDs) > 0 {
		if s.debugModelRoutingEnabled() {
			logger.LegacyPrintf("service.gateway", "[ModelRoutingDebug] legacy mixed routed begin: group_id=%v model=%s platform=%s session=%s routed_ids=%v",
				derefGroupID(groupID), requestedModel, nativePlatform, shortSessionHash(sessionHash), routingAccountIDs)
		}
		// 1) Sticky session only applies if the bound account is within the routing set.
		if sessionHash != "" && s.cache != nil {
			accountID, err := s.cache.GetSessionAccountID(ctx, derefGroupID(groupID), sessionHash)
			if err == nil && accountID > 0 && containsInt64(routingAccountIDs, accountID) {
				if _, excluded := excludedIDs[accountID]; !excluded {
					account, err := s.getSchedulableAccount(ctx, accountID)
					// 检查账号分组归属和有效性：原生平台直接匹配，antigravity 需要启用混合调度
					if err == nil {
						clearSticky := shouldClearStickySession(account, requestedModel)
						if clearSticky {
							_ = s.cache.DeleteSessionAccountID(ctx, derefGroupID(groupID), sessionHash)
						}
						if !clearSticky && s.isAccountInGroup(account, groupID) && (requestedModel == "" || s.isModelSupportedByAccountWithContext(ctx, account, requestedModel)) && s.isAccountSchedulableForModelSelection(ctx, account, requestedModel) && s.isAccountSchedulableForQuota(account) && s.isAccountSchedulableForWindowCost(ctx, account, true) && s.isAccountSchedulableForRPM(ctx, account, true) {
							if account.Platform == nativePlatform || (account.Platform == PlatformAntigravity && account.IsMixedSchedulingEnabled()) {
								if s.debugModelRoutingEnabled() {
									logger.LegacyPrintf("service.gateway", "[ModelRoutingDebug] legacy mixed routed sticky hit: group_id=%v model=%s session=%s account=%d", derefGroupID(groupID), requestedModel, shortSessionHash(sessionHash), accountID)
								}
								return account, nil
							}
						}
					}
				}
			}
		}

		// 2) Select an account from the routed candidates.
		var err error
		accounts, _, err = s.listSchedulableAccounts(ctx, groupID, nativePlatform, false)
		if err != nil {
			return nil, fmt.Errorf("query accounts failed: %w", err)
		}
		accountsLoaded = true

		// 提前预取窗口费用+RPM 计数，确保 routing 段内的调度检查调用能命中缓存
		ctx = s.withWindowCostPrefetch(ctx, accounts)
		ctx = s.withRPMPrefetch(ctx, accounts)

		routingSet := make(map[int64]struct{}, len(routingAccountIDs))
		for _, id := range routingAccountIDs {
			if id > 0 {
				routingSet[id] = struct{}{}
			}
		}

		var selected *Account
		for i := range accounts {
			acc := &accounts[i]
			if _, ok := routingSet[acc.ID]; !ok {
				continue
			}
			if _, excluded := excludedIDs[acc.ID]; excluded {
				continue
			}
			// Scheduler snapshots can be temporarily stale; re-check schedulability here to
			// avoid selecting accounts that were recently rate-limited/overloaded.
			if !s.isAccountSchedulableForSelection(acc) {
				continue
			}
			// require_privacy_set: 跳过 privacy 未设置的账号并标记异常
			if schedGroup != nil && schedGroup.RequirePrivacySet && !acc.IsPrivacySet() {
				_ = s.accountRepo.SetError(ctx, acc.ID,
					fmt.Sprintf("Privacy not set, required by group [%s]", schedGroup.Name))
				continue
			}
			// 过滤：原生平台直接通过，antigravity 需要启用混合调度
			if acc.Platform == PlatformAntigravity && !acc.IsMixedSchedulingEnabled() {
				continue
			}
			if requestedModel != "" && !s.isModelSupportedByAccountWithContext(ctx, acc, requestedModel) {
				continue
			}
			if !s.isAccountSchedulableForModelSelection(ctx, acc, requestedModel) {
				continue
			}
			if !s.isAccountSchedulableForQuota(acc) {
				continue
			}
			if !s.isAccountSchedulableForWindowCost(ctx, acc, false) {
				continue
			}
			if !s.isAccountSchedulableForRPM(ctx, acc, false) {
				continue
			}
			if selected == nil {
				selected = acc
			} else if isBetterAccountCandidate(acc, selected, preferOAuth, PlatformGemini) {
				selected = acc
			}
		}

		if selected != nil {
			if sessionHash != "" && s.cache != nil {
				if err := s.cache.SetSessionAccountID(ctx, derefGroupID(groupID), sessionHash, selected.ID, stickySessionTTL); err != nil {
					logger.LegacyPrintf("service.gateway", "set session account failed: session=%s account_id=%d err=%v", sessionHash, selected.ID, err)
				}
			}
			if s.debugModelRoutingEnabled() {
				logger.LegacyPrintf("service.gateway", "[ModelRoutingDebug] legacy mixed routed select: group_id=%v model=%s session=%s account=%d", derefGroupID(groupID), requestedModel, shortSessionHash(sessionHash), selected.ID)
			}
			return selected, nil
		}
		logger.LegacyPrintf("service.gateway", "[ModelRouting] No routed accounts available for model=%s, falling back to normal selection", requestedModel)
	}

	// 1. 查询粘性会话
	if sessionHash != "" && s.cache != nil {
		accountID, err := s.cache.GetSessionAccountID(ctx, derefGroupID(groupID), sessionHash)
		if err == nil && accountID > 0 {
			if _, excluded := excludedIDs[accountID]; !excluded {
				account, err := s.getSchedulableAccount(ctx, accountID)
				// 检查账号分组归属和有效性：原生平台直接匹配，antigravity 需要启用混合调度
				if err == nil {
					clearSticky := shouldClearStickySession(account, requestedModel)
					if clearSticky {
						_ = s.cache.DeleteSessionAccountID(ctx, derefGroupID(groupID), sessionHash)
					}
					if !clearSticky && s.isAccountInGroup(account, groupID) && (requestedModel == "" || s.isModelSupportedByAccountWithContext(ctx, account, requestedModel)) && s.isAccountSchedulableForModelSelection(ctx, account, requestedModel) && s.isAccountSchedulableForQuota(account) && s.isAccountSchedulableForWindowCost(ctx, account, true) && s.isAccountSchedulableForRPM(ctx, account, true) && !s.isStickyAccountUpstreamRestricted(ctx, groupID, account, requestedModel) {
						if account.Platform == nativePlatform || (account.Platform == PlatformAntigravity && account.IsMixedSchedulingEnabled()) {
							return account, nil
						}
					}
				}
			}
		}
	}

	// 2. 获取可调度账号列表
	if !accountsLoaded {
		var err error
		accounts, _, err = s.listSchedulableAccounts(ctx, groupID, nativePlatform, false)
		if err != nil {
			return nil, fmt.Errorf("query accounts failed: %w", err)
		}
	}

	// 批量预取窗口费用+RPM 计数，避免逐个账号查询（N+1）
	ctx = s.withWindowCostPrefetch(ctx, accounts)
	ctx = s.withRPMPrefetch(ctx, accounts)

	// 3. 按优先级+最久未用选择（考虑模型支持和混合调度）
	// needsUpstreamCheck 仅在主选择循环中使用；粘性会话命中时跳过此检查。
	needsUpstreamCheck := s.needsUpstreamChannelRestrictionCheck(ctx, groupID)
	var selected *Account
	for i := range accounts {
		acc := &accounts[i]
		if _, excluded := excludedIDs[acc.ID]; excluded {
			continue
		}
		// Scheduler snapshots can be temporarily stale; re-check schedulability here to
		// avoid selecting accounts that were recently rate-limited/overloaded.
		if !s.isAccountSchedulableForSelection(acc) {
			continue
		}
		// require_privacy_set: 跳过 privacy 未设置的账号并标记异常
		if schedGroup != nil && schedGroup.RequirePrivacySet && !acc.IsPrivacySet() {
			_ = s.accountRepo.SetError(ctx, acc.ID,
				fmt.Sprintf("Privacy not set, required by group [%s]", schedGroup.Name))
			continue
		}
		// 过滤：原生平台直接通过，antigravity 需要启用混合调度
		if acc.Platform == PlatformAntigravity && !acc.IsMixedSchedulingEnabled() {
			continue
		}
		if requestedModel != "" && !s.isModelSupportedByAccountWithContext(ctx, acc, requestedModel) {
			continue
		}
		if needsUpstreamCheck && s.isUpstreamModelRestrictedByChannel(ctx, *groupID, acc, requestedModel) {
			continue
		}
		if !s.isAccountSchedulableForModelSelection(ctx, acc, requestedModel) {
			continue
		}
		if !s.isAccountSchedulableForQuota(acc) {
			continue
		}
		if !s.isAccountSchedulableForWindowCost(ctx, acc, false) {
			continue
		}
		if !s.isAccountSchedulableForRPM(ctx, acc, false) {
			continue
		}
		if selected == nil {
			selected = acc
		} else if isBetterAccountCandidate(acc, selected, preferOAuth, PlatformGemini) {
			selected = acc
		}
	}

	if selected == nil {
		stats := s.logDetailedSelectionFailure(ctx, groupID, sessionHash, requestedModel, nativePlatform, accounts, excludedIDs, true)
		if requestedModel != "" {
			return nil, fmt.Errorf("%w supporting model: %s (%s)", ErrNoAvailableAccounts, requestedModel, summarizeSelectionFailureStats(stats))
		}
		return nil, ErrNoAvailableAccounts
	}

	// 4. 建立粘性绑定
	if sessionHash != "" && s.cache != nil {
		if err := s.cache.SetSessionAccountID(ctx, derefGroupID(groupID), sessionHash, selected.ID, stickySessionTTL); err != nil {
			logger.LegacyPrintf("service.gateway", "set session account failed: session=%s account_id=%d err=%v", sessionHash, selected.ID, err)
		}
	}

	return selected, nil
}
