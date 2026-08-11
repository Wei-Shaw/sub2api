package service

// 本文件由 gateway_service.go 纯移动拆分而来：账号选择与负载感知调度、窗口费用
// 与 RPM 预取、候选排序/过滤、混合平台调度与选择失败诊断。仅做代码搬迁，
// 无任何行为变更。

import (
	"context"
	"fmt"
	"log/slog"
	mathrand "math/rand"
	"sort"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
)

// SelectAccount 选择账号（粘性会话+优先级）
func (s *GatewayService) SelectAccount(ctx context.Context, groupID *int64, sessionHash string) (*Account, error) {
	return s.SelectAccountForModel(ctx, groupID, sessionHash, "")
}

// SelectAccountForModel 选择支持指定模型的账号（粘性会话+优先级+模型映射）
func (s *GatewayService) SelectAccountForModel(ctx context.Context, groupID *int64, sessionHash string, requestedModel string) (*Account, error) {
	return s.SelectAccountForModelWithExclusions(ctx, groupID, sessionHash, requestedModel, nil)
}

// SelectAccountForModelWithExclusions selects an account supporting the requested model while excluding specified accounts.
func (s *GatewayService) SelectAccountForModelWithExclusions(ctx context.Context, groupID *int64, sessionHash string, requestedModel string, excludedIDs map[int64]struct{}) (*Account, error) {
	// 优先检查 context 中的强制平台（/antigravity 路由）
	var platform string
	forcePlatform, hasForcePlatform := ctx.Value(ctxkey.ForcePlatform).(string)
	if hasForcePlatform && forcePlatform != "" {
		platform = forcePlatform
	} else if groupID != nil {
		group, resolvedGroupID, err := s.resolveGatewayGroup(ctx, groupID)
		if err != nil {
			return nil, err
		}
		if group == nil {
			return nil, ErrGroupNotFound
		}
		groupID = resolvedGroupID
		ctx = s.withGroupContext(ctx, group)
		platform = group.Platform
		if group.Platform == PlatformComposite {
			decision, ok, err := s.resolveCompositeRouteDecision(ctx, group, requestedModel, CompositeRouteEndpointAny)
			if err != nil {
				return nil, err
			}
			if !ok {
				return nil, fmt.Errorf("%w supporting model: %s (composite target platform unknown)", ErrNoAvailableAccounts, requestedModel)
			}
			platform = decision.TargetPlatform
			requestedModel = decision.UpstreamModel
			ctx = WithCompositeRouteDecision(ctx, decision)
		}
	} else {
		// 无分组时只使用原生 anthropic 平台
		platform = PlatformAnthropic
	}

	// Claude Code 限制可能已将 groupID 解析为 fallback group，
	// 渠道限制预检查必须使用解析后的分组。
	if s.checkChannelPricingRestriction(ctx, groupID, requestedModel) {
		slog.Warn("channel pricing restriction blocked request",
			"group_id", derefGroupID(groupID),
			"model", requestedModel)
		return nil, fmt.Errorf("%w supporting model: %s (channel pricing restriction)", ErrNoAvailableAccounts, requestedModel)
	}

	// anthropic/gemini 分组支持混合调度（包含启用了 mixed_scheduling 的 antigravity 账户）
	// 注意：强制平台模式不走混合调度
	if (platform == PlatformAnthropic || platform == PlatformGemini) && !hasForcePlatform {
		account, err := s.selectAccountWithMixedScheduling(ctx, groupID, sessionHash, requestedModel, excludedIDs, platform)
		if err != nil {
			return nil, err
		}
		return s.hydrateSelectedAccount(ctx, account)
	}

	// antigravity 分组、强制平台模式或无分组使用单平台选择
	// 注意：强制平台模式也必须遵守分组限制，不再回退到全平台查询
	account, err := s.selectAccountForModelWithPlatform(ctx, groupID, sessionHash, requestedModel, excludedIDs, platform)
	if err != nil {
		return nil, err
	}
	return s.hydrateSelectedAccount(ctx, account)
}

// SelectAccountWithLoadAwareness selects account with load-awareness and wait plan.
// metadataUserID: 用于客户端亲和调度，从中提取客户端 ID
// sub2apiUserID: 系统用户 ID，用于二维亲和调度
func (s *GatewayService) SelectAccountWithLoadAwareness(ctx context.Context, groupID *int64, sessionHash string, requestedModel string, excludedIDs map[int64]struct{}, metadataUserID string, sub2apiUserID int64) (*AccountSelectionResult, error) {
	// 调试日志：记录调度入口参数
	excludedIDsList := make([]int64, 0, len(excludedIDs))
	for id := range excludedIDs {
		excludedIDsList = append(excludedIDsList, id)
	}
	slog.Debug("account_scheduling_starting",
		"group_id", derefGroupID(groupID),
		"model", requestedModel,
		"session", shortSessionHash(sessionHash),
		"excluded_ids", excludedIDsList)

	cfg := s.schedulingConfig()

	// 检查 Claude Code 客户端限制（可能会替换 groupID 为降级分组）
	group, groupID, err := s.checkClaudeCodeRestriction(ctx, groupID)
	if err != nil {
		return nil, err
	}
	ctx = s.withGroupContext(ctx, group)

	// Claude Code 限制可能已将 groupID 解析为 fallback group，
	// 渠道限制预检查必须使用解析后的分组。
	if s.checkChannelPricingRestriction(ctx, groupID, requestedModel) {
		slog.Warn("channel pricing restriction blocked request",
			"group_id", derefGroupID(groupID),
			"model", requestedModel)
		return nil, fmt.Errorf("%w supporting model: %s (channel pricing restriction)", ErrNoAvailableAccounts, requestedModel)
	}

	var stickyAccountID int64
	var stickySource string
	if prefetch := prefetchedStickyAccountIDFromContext(ctx, groupID); prefetch > 0 {
		stickyAccountID = prefetch
		stickySource = "prefetch"
	} else if sessionHash != "" && s.cache != nil {
		if accountID, err := s.cache.GetSessionAccountID(ctx, derefGroupID(groupID), sessionHash); err == nil {
			stickyAccountID = accountID
			stickySource = "cache"
		}
	}

	// [DEBUG-STICKY] 调度器入口日志
	slog.Info("sticky.scheduler_entry",
		"group_id", derefGroupID(groupID),
		"session_hash", shortSessionHash(sessionHash),
		"sticky_account_id", stickyAccountID,
		"sticky_source", stickySource,
		"model", requestedModel,
		"load_batch", cfg.LoadBatchEnabled,
		"has_concurrency_svc", s.concurrencyService != nil,
		"excluded_count", len(excludedIDs),
	)

	if s.debugModelRoutingEnabled() && requestedModel != "" {
		groupPlatform := ""
		if group != nil {
			groupPlatform = group.Platform
		}
		logger.LegacyPrintf("service.gateway", "[ModelRoutingDebug] select entry: group_id=%v group_platform=%s model=%s session=%s sticky_account=%d load_batch=%v concurrency=%v",
			derefGroupID(groupID), groupPlatform, requestedModel, shortSessionHash(sessionHash), stickyAccountID, cfg.LoadBatchEnabled, s.concurrencyService != nil)
	}

	if s.concurrencyService == nil || !cfg.LoadBatchEnabled {
		// 复制排除列表，用于会话限制拒绝时的重试
		localExcluded := make(map[int64]struct{})
		for k, v := range excludedIDs {
			localExcluded[k] = v
		}

		for {
			account, err := s.SelectAccountForModelWithExclusions(ctx, groupID, sessionHash, requestedModel, localExcluded)
			if err != nil {
				return nil, err
			}

			result, err := s.tryAcquireAccountSlot(ctx, account.ID, account.Concurrency)
			if err == nil && result.Acquired {
				// 获取槽位后检查会话限制（使用 sessionHash 作为会话标识符）
				if !s.checkAndRegisterSession(ctx, account, sessionHash) {
					result.ReleaseFunc()                   // 释放槽位
					localExcluded[account.ID] = struct{}{} // 排除此账号
					continue                               // 重新选择
				}
				return s.newSelectionResult(ctx, account, true, result.ReleaseFunc, nil)
			}

			// 对于等待计划的情况，也需要先检查会话限制
			if !s.checkAndRegisterSession(ctx, account, sessionHash) {
				localExcluded[account.ID] = struct{}{}
				continue
			}

			if stickyAccountID > 0 && stickyAccountID == account.ID && s.concurrencyService != nil {
				waitingCount, _ := s.concurrencyService.GetAccountWaitingCount(ctx, account.ID)
				if waitingCount < cfg.StickySessionMaxWaiting {
					return s.newSelectionResult(ctx, account, false, nil, &AccountWaitPlan{
						AccountID:      account.ID,
						MaxConcurrency: account.Concurrency,
						Timeout:        cfg.StickySessionWaitTimeout,
						MaxWaiting:     cfg.StickySessionMaxWaiting,
					})
				}
			}
			return s.newSelectionResult(ctx, account, false, nil, &AccountWaitPlan{
				AccountID:      account.ID,
				MaxConcurrency: account.Concurrency,
				Timeout:        cfg.FallbackWaitTimeout,
				MaxWaiting:     cfg.FallbackMaxWaiting,
			})
		}
	}

	platform, hasForcePlatform, err := s.resolvePlatform(ctx, groupID, group, requestedModel)
	if err != nil {
		return nil, err
	}
	preferOAuth := platform == PlatformGemini
	if s.debugModelRoutingEnabled() && platform == PlatformAnthropic && requestedModel != "" {
		logger.LegacyPrintf("service.gateway", "[ModelRoutingDebug] load-aware enabled: group_id=%v model=%s session=%s platform=%s", derefGroupID(groupID), requestedModel, shortSessionHash(sessionHash), platform)
	}

	accounts, useMixed, err := s.listSchedulableAccounts(ctx, groupID, platform, hasForcePlatform)
	if err != nil {
		return nil, err
	}
	if len(accounts) == 0 {
		return nil, ErrNoAvailableAccounts
	}
	ctx = s.withWindowCostPrefetch(ctx, accounts)
	ctx = s.withRPMPrefetch(ctx, accounts)

	// 提前构建 accountByID（供 Layer 1 和 Layer 1.5 使用）
	accountByID := make(map[int64]*Account, len(accounts))
	for i := range accounts {
		accountByID[accounts[i].ID] = &accounts[i]
	}
	isExcluded := func(accountID int64) bool {
		if excludedIDs == nil {
			return false
		}
		_, excluded := excludedIDs[accountID]
		return excluded
	}

	// 获取模型路由配置（anthropic 目标平台；composite 分组按目标平台判断）
	var routingAccountIDs []int64
	if group != nil && requestedModel != "" && platform == PlatformAnthropic &&
		(group.Platform == PlatformAnthropic || group.Platform == PlatformComposite) {
		routingAccountIDs = group.GetRoutingAccountIDs(requestedModel)
		if s.debugModelRoutingEnabled() {
			logger.LegacyPrintf("service.gateway", "[ModelRoutingDebug] context group routing: group_id=%d model=%s enabled=%v rules=%d matched_ids=%v session=%s sticky_account=%d",
				group.ID, requestedModel, group.ModelRoutingEnabled, len(group.ModelRouting), routingAccountIDs, shortSessionHash(sessionHash), stickyAccountID)
			if len(routingAccountIDs) == 0 && group.ModelRoutingEnabled && len(group.ModelRouting) > 0 {
				keys := make([]string, 0, len(group.ModelRouting))
				for k := range group.ModelRouting {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				const maxKeys = 20
				if len(keys) > maxKeys {
					keys = keys[:maxKeys]
				}
				logger.LegacyPrintf("service.gateway", "[ModelRoutingDebug] context group routing miss: group_id=%d model=%s patterns(sample)=%v", group.ID, requestedModel, keys)
			}
		}
	}

	// ============ Layer 1: 模型路由优先选择（优先级高于粘性会话） ============
	if len(routingAccountIDs) > 0 && s.concurrencyService != nil {
		// 1. 过滤出路由列表中可调度的账号
		var routingCandidates []*Account
		var filteredExcluded, filteredMissing, filteredUnsched, filteredPlatform, filteredModelScope, filteredModelMapping, filteredWindowCost int
		var modelScopeSkippedIDs []int64 // 记录因模型限流被跳过的账号 ID
		for _, routingAccountID := range routingAccountIDs {
			if isExcluded(routingAccountID) {
				filteredExcluded++
				continue
			}
			account, ok := accountByID[routingAccountID]
			if !ok || !s.isAccountSchedulableForSelection(account) {
				if !ok {
					filteredMissing++
				} else {
					filteredUnsched++
				}
				continue
			}
			if !s.isAccountAllowedForPlatform(account, platform, useMixed) {
				filteredPlatform++
				continue
			}
			if requestedModel != "" && !s.isModelSupportedByAccountWithContext(ctx, account, requestedModel) {
				filteredModelMapping++
				continue
			}
			if !s.isAccountSchedulableForModelSelection(ctx, account, requestedModel) {
				filteredModelScope++
				modelScopeSkippedIDs = append(modelScopeSkippedIDs, account.ID)
				continue
			}
			// 配额检查
			if !s.isAccountSchedulableForQuota(account) {
				continue
			}
			// 窗口费用检查（非粘性会话路径）
			if !s.isAccountSchedulableForWindowCost(ctx, account, false) {
				filteredWindowCost++
				continue
			}
			// RPM 检查（非粘性会话路径）
			if !s.isAccountSchedulableForRPM(ctx, account, false) {
				continue
			}
			routingCandidates = append(routingCandidates, account)
		}

		if s.debugModelRoutingEnabled() {
			logger.LegacyPrintf("service.gateway", "[ModelRoutingDebug] routed candidates: group_id=%v model=%s routed=%d candidates=%d filtered(excluded=%d missing=%d unsched=%d platform=%d model_scope=%d model_mapping=%d window_cost=%d)",
				derefGroupID(groupID), requestedModel, len(routingAccountIDs), len(routingCandidates),
				filteredExcluded, filteredMissing, filteredUnsched, filteredPlatform, filteredModelScope, filteredModelMapping, filteredWindowCost)
			if len(modelScopeSkippedIDs) > 0 {
				logger.LegacyPrintf("service.gateway", "[ModelRoutingDebug] model_rate_limited accounts skipped: group_id=%v model=%s account_ids=%v",
					derefGroupID(groupID), requestedModel, modelScopeSkippedIDs)
			}
		}

		if len(routingCandidates) > 0 {
			// 1.5. 在路由账号范围内检查粘性会话
			if sessionHash != "" && stickyAccountID > 0 {
				slog.Debug("sticky.layer1_5_checking",
					"sticky_account_id", stickyAccountID,
					"in_routing_list", containsInt64(routingAccountIDs, stickyAccountID),
					"is_excluded", isExcluded(stickyAccountID),
					"in_account_map", func() bool { _, ok := accountByID[stickyAccountID]; return ok }(),
					"session", shortSessionHash(sessionHash),
				)
				if containsInt64(routingAccountIDs, stickyAccountID) && !isExcluded(stickyAccountID) {
					// 粘性账号在路由列表中，优先使用
					if stickyAccount, ok := accountByID[stickyAccountID]; ok {
						var stickyCacheMissReason string

						gatePass := s.isAccountSchedulableForSelection(stickyAccount) &&
							s.isAccountAllowedForPlatform(stickyAccount, platform, useMixed) &&
							(requestedModel == "" || s.isModelSupportedByAccountWithContext(ctx, stickyAccount, requestedModel)) &&
							s.isAccountSchedulableForModelSelection(ctx, stickyAccount, requestedModel) &&
							s.isAccountSchedulableForQuota(stickyAccount) &&
							s.isAccountSchedulableForWindowCost(ctx, stickyAccount, true)

						rpmPass := gatePass && s.isAccountSchedulableForRPM(ctx, stickyAccount, true)

						if rpmPass { // 粘性会话窗口费用+RPM 检查
							result, err := s.tryAcquireAccountSlot(ctx, stickyAccountID, stickyAccount.Concurrency)
							if err == nil && result.Acquired {
								// 会话数量限制检查
								if !s.checkAndRegisterSession(ctx, stickyAccount, sessionHash) {
									result.ReleaseFunc() // 释放槽位
									stickyCacheMissReason = "session_limit"
									// 继续到负载感知选择
								} else {
									slog.Debug("sticky.layer1_5_hit",
										"account_id", stickyAccountID,
										"session", shortSessionHash(sessionHash),
										"result", "slot_acquired",
									)
									if s.debugModelRoutingEnabled() {
										logger.LegacyPrintf("service.gateway", "[ModelRoutingDebug] routed sticky hit: group_id=%v model=%s session=%s account=%d", derefGroupID(groupID), requestedModel, shortSessionHash(sessionHash), stickyAccountID)
									}
									return s.newSelectionResult(ctx, stickyAccount, true, result.ReleaseFunc, nil)
								}
							}

							if stickyCacheMissReason == "" {
								waitingCount, _ := s.concurrencyService.GetAccountWaitingCount(ctx, stickyAccountID)
								if waitingCount < cfg.StickySessionMaxWaiting {
									// 会话数量限制检查（等待计划也需要占用会话配额）
									if !s.checkAndRegisterSession(ctx, stickyAccount, sessionHash) {
										stickyCacheMissReason = "session_limit"
										// 会话限制已满，继续到负载感知选择
									} else {
										// 必须走 newSelectionResult 以 hydrate 账号凭证：
										// 调度快照中的账号是精简版（OAuth token 等被剥离），
										// 直接返回会导致后续转发缺少凭证而鉴权失败。
										return s.newSelectionResult(ctx, stickyAccount, false, nil, &AccountWaitPlan{
											AccountID:      stickyAccountID,
											MaxConcurrency: stickyAccount.Concurrency,
											Timeout:        cfg.StickySessionWaitTimeout,
											MaxWaiting:     cfg.StickySessionMaxWaiting,
										})
									}
								} else {
									stickyCacheMissReason = "wait_queue_full"
								}
							}
							// 粘性账号槽位满且等待队列已满，继续使用负载感知选择
						} else if !gatePass {
							stickyCacheMissReason = "gate_check"
						} else {
							stickyCacheMissReason = "rpm_red"
						}

						// 记录粘性缓存未命中的结构化日志
						if stickyCacheMissReason != "" {
							baseRPM := stickyAccount.GetBaseRPM()
							var currentRPM int
							if count, ok := rpmFromPrefetchContext(ctx, stickyAccount.ID); ok {
								currentRPM = count
							}
							logger.LegacyPrintf("service.gateway", "[StickyCacheMiss] reason=%s account_id=%d session=%s current_rpm=%d base_rpm=%d",
								stickyCacheMissReason, stickyAccountID, shortSessionHash(sessionHash), currentRPM, baseRPM)
						}
					} else {
						_ = s.cache.DeleteSessionAccountID(ctx, derefGroupID(groupID), sessionHash)
						logger.LegacyPrintf("service.gateway", "[StickyCacheMiss] reason=account_cleared account_id=%d session=%s current_rpm=0 base_rpm=0",
							stickyAccountID, shortSessionHash(sessionHash))
					}
				}
			}

			// 2. 批量获取负载信息
			routingLoads := make([]AccountWithConcurrency, 0, len(routingCandidates))
			for _, acc := range routingCandidates {
				routingLoads = append(routingLoads, AccountWithConcurrency{
					ID:             acc.ID,
					MaxConcurrency: acc.EffectiveLoadFactor(),
				})
			}
			routingLoadMap, _ := s.concurrencyService.GetAccountsLoadBatch(ctx, routingLoads)

			// 3. 按负载感知排序
			var routingAvailable []accountWithLoad
			for _, acc := range routingCandidates {
				loadInfo := routingLoadMap[acc.ID]
				if loadInfo == nil {
					loadInfo = &AccountLoadInfo{AccountID: acc.ID}
				}
				if loadInfo.LoadRate < 100 {
					routingAvailable = append(routingAvailable, accountWithLoad{account: acc, loadInfo: loadInfo})
				}
			}

			if len(routingAvailable) > 0 {
				// 排序：优先级 > 负载率 > 最后使用时间
				sort.SliceStable(routingAvailable, func(i, j int) bool {
					a, b := routingAvailable[i], routingAvailable[j]
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
				shuffleWithinSortGroups(routingAvailable)

				// 4. 尝试获取槽位
				for _, item := range routingAvailable {
					result, err := s.tryAcquireAccountSlot(ctx, item.account.ID, item.account.Concurrency)
					if err == nil && result.Acquired {
						// 会话数量限制检查
						if !s.checkAndRegisterSession(ctx, item.account, sessionHash) {
							result.ReleaseFunc() // 释放槽位，继续尝试下一个账号
							continue
						}
						if sessionHash != "" && s.cache != nil {
							_ = s.cache.SetSessionAccountID(ctx, derefGroupID(groupID), sessionHash, item.account.ID, stickySessionTTL)
						}
						if s.debugModelRoutingEnabled() {
							logger.LegacyPrintf("service.gateway", "[ModelRoutingDebug] routed select: group_id=%v model=%s session=%s account=%d", derefGroupID(groupID), requestedModel, shortSessionHash(sessionHash), item.account.ID)
						}
						return s.newSelectionResult(ctx, item.account, true, result.ReleaseFunc, nil)
					}
				}

				// 5. 所有路由账号槽位满，尝试返回等待计划（选择负载最低的）
				// 遍历找到第一个满足会话限制的账号
				for _, item := range routingAvailable {
					if !s.checkAndRegisterSession(ctx, item.account, sessionHash) {
						continue // 会话限制已满，尝试下一个
					}
					if s.debugModelRoutingEnabled() {
						logger.LegacyPrintf("service.gateway", "[ModelRoutingDebug] routed wait: group_id=%v model=%s session=%s account=%d", derefGroupID(groupID), requestedModel, shortSessionHash(sessionHash), item.account.ID)
					}
					return s.newSelectionResult(ctx, item.account, false, nil, &AccountWaitPlan{
						AccountID:      item.account.ID,
						MaxConcurrency: item.account.Concurrency,
						Timeout:        cfg.StickySessionWaitTimeout,
						MaxWaiting:     cfg.StickySessionMaxWaiting,
					})
				}
				// 所有路由账号会话限制都已满，继续到 Layer 2 回退
			}
			// 路由列表中的账号都不可用（负载率 >= 100），继续到 Layer 2 回退
			logger.LegacyPrintf("service.gateway", "[ModelRouting] All routed accounts unavailable for model=%s, falling back to normal selection", requestedModel)
		}
	}

	// ============ Layer 1.5: 粘性会话（仅在无模型路由配置时生效） ============
	if len(routingAccountIDs) == 0 && sessionHash != "" && stickyAccountID > 0 && !isExcluded(stickyAccountID) {
		accountID := stickyAccountID
		if accountID > 0 && !isExcluded(accountID) {
			account, ok := accountByID[accountID]
			if ok {
				// 检查账户是否需要清理粘性会话绑定
				clearSticky := shouldClearStickySession(account, requestedModel)
				if clearSticky {
					slog.Debug("sticky.layer1_5_no_routing_clear",
						"account_id", accountID,
						"reason", "should_clear_sticky_session",
						"session", shortSessionHash(sessionHash),
					)
					_ = s.cache.DeleteSessionAccountID(ctx, derefGroupID(groupID), sessionHash)
				}

				// 注意：不再检查 isAccountInGroup，因为 accountByID 已经从按分组过滤的
				// accounts 列表构建，账号一定在分组内。而 scheduler snapshot 缓存
				// 反序列化后 AccountGroups 字段为空，导致 isAccountInGroup 永远返回 false。
				platformOK := s.isAccountAllowedForPlatform(account, platform, useMixed)
				modelSupported := requestedModel == "" || s.isModelSupportedByAccountWithContext(ctx, account, requestedModel)
				modelSchedulable := s.isAccountSchedulableForModelSelection(ctx, account, requestedModel)
				quotaOK := s.isAccountSchedulableForQuota(account)
				windowCostOK := s.isAccountSchedulableForWindowCost(ctx, account, true)
				rpmOK := s.isAccountSchedulableForRPM(ctx, account, true)
				schedulable := s.isAccountSchedulableForSelection(account)

				slog.Debug("sticky.layer1_5_no_routing_checks",
					"account_id", accountID,
					"session", shortSessionHash(sessionHash),
					"clear_sticky", clearSticky,
					"schedulable", schedulable,
					"platform_ok", platformOK,
					"model_supported", modelSupported,
					"model_schedulable", modelSchedulable,
					"quota_ok", quotaOK,
					"window_cost_ok", windowCostOK,
					"rpm_ok", rpmOK,
				)

				if !clearSticky && platformOK && modelSupported && modelSchedulable && quotaOK && windowCostOK && rpmOK && schedulable {
					result, err := s.tryAcquireAccountSlot(ctx, accountID, account.Concurrency)
					if err == nil && result.Acquired {
						// 会话数量限制检查
						if !s.checkAndRegisterSession(ctx, account, sessionHash) {
							result.ReleaseFunc() // 释放槽位，继续到 Layer 2
							slog.Debug("sticky.layer1_5_no_routing_miss",
								"account_id", accountID,
								"reason", "session_limit",
								"session", shortSessionHash(sessionHash),
							)
						} else {
							slog.Debug("sticky.layer1_5_no_routing_hit",
								"account_id", accountID,
								"session", shortSessionHash(sessionHash),
								"result", "slot_acquired",
							)
							if s.cache != nil {
								_ = s.cache.RefreshSessionTTL(ctx, derefGroupID(groupID), sessionHash, stickySessionTTL)
							}
							return s.newSelectionResult(ctx, account, true, result.ReleaseFunc, nil)
						}
					} else {
						slog.Debug("sticky.layer1_5_no_routing_slot_busy",
							"account_id", accountID,
							"session", shortSessionHash(sessionHash),
						)
					}

					waitingCount, _ := s.concurrencyService.GetAccountWaitingCount(ctx, accountID)
					if waitingCount < cfg.StickySessionMaxWaiting {
						// 会话数量限制检查（等待计划也需要占用会话配额）
						if !s.checkAndRegisterSession(ctx, account, sessionHash) {
							// 会话限制已满，继续到 Layer 2
						} else {
							slog.Debug("sticky.layer1_5_no_routing_hit",
								"account_id", accountID,
								"session", shortSessionHash(sessionHash),
								"result", "wait_plan",
							)
							return s.newSelectionResult(ctx, account, false, nil, &AccountWaitPlan{
								AccountID:      accountID,
								MaxConcurrency: account.Concurrency,
								Timeout:        cfg.StickySessionWaitTimeout,
								MaxWaiting:     cfg.StickySessionMaxWaiting,
							})
						}
					}
				} else if !clearSticky {
					slog.Debug("sticky.layer1_5_no_routing_miss",
						"account_id", accountID,
						"reason", "gate_check_failed",
						"session", shortSessionHash(sessionHash),
					)
				}
			} else {
				slog.Debug("sticky.layer1_5_no_routing_miss",
					"account_id", accountID,
					"reason", "account_not_in_map",
					"session", shortSessionHash(sessionHash),
				)
			}
		}
	} else if len(routingAccountIDs) == 0 && sessionHash != "" {
		slog.Debug("sticky.layer1_5_no_routing_skip",
			"sticky_account_id", stickyAccountID,
			"is_excluded", func() bool { return stickyAccountID > 0 && isExcluded(stickyAccountID) }(),
			"session", shortSessionHash(sessionHash),
			"reason", func() string {
				if stickyAccountID == 0 {
					return "no_sticky_binding"
				}
				return "sticky_account_excluded"
			}(),
		)
	}

	// ============ Layer 2: 负载感知选择 ============
	slog.Debug("sticky.layer2_fallback",
		"session", shortSessionHash(sessionHash),
		"sticky_account_id", stickyAccountID,
		"reason", "sticky_not_used_falling_back_to_load_balance",
		"total_accounts", len(accounts),
	)
	candidates := make([]*Account, 0, len(accounts))
	for i := range accounts {
		acc := &accounts[i]
		if isExcluded(acc.ID) {
			continue
		}
		// Scheduler snapshots can be temporarily stale (bucket rebuild is throttled);
		// re-check schedulability here so recently rate-limited/overloaded accounts
		// are not selected again before the bucket is rebuilt.
		if !s.isAccountSchedulableForSelection(acc) {
			continue
		}
		if !s.isAccountAllowedForPlatform(acc, platform, useMixed) {
			continue
		}
		if requestedModel != "" && !s.isModelSupportedByAccountWithContext(ctx, acc, requestedModel) {
			continue
		}
		if !s.isAccountSchedulableForModelSelection(ctx, acc, requestedModel) {
			continue
		}
		// 配额检查
		if !s.isAccountSchedulableForQuota(acc) {
			continue
		}
		// 窗口费用检查（非粘性会话路径）
		if !s.isAccountSchedulableForWindowCost(ctx, acc, false) {
			continue
		}
		// RPM 检查（非粘性会话路径）
		if !s.isAccountSchedulableForRPM(ctx, acc, false) {
			continue
		}
		candidates = append(candidates, acc)
	}

	if len(candidates) == 0 {
		return nil, ErrNoAvailableAccounts
	}

	accountLoads := make([]AccountWithConcurrency, 0, len(candidates))
	for _, acc := range candidates {
		accountLoads = append(accountLoads, AccountWithConcurrency{
			ID:             acc.ID,
			MaxConcurrency: acc.EffectiveLoadFactor(),
		})
	}

	loadMap, err := s.concurrencyService.GetAccountsLoadBatch(ctx, accountLoads)
	if err != nil {
		if result, ok, legacyErr := s.tryAcquireByLegacyOrder(ctx, candidates, groupID, sessionHash, preferOAuth); legacyErr != nil {
			return nil, legacyErr
		} else if ok {
			return result, nil
		}
	} else {
		var available []accountWithLoad
		for _, acc := range candidates {
			loadInfo := loadMap[acc.ID]
			if loadInfo == nil {
				loadInfo = &AccountLoadInfo{AccountID: acc.ID}
			}
			if loadInfo.LoadRate < 100 {
				available = append(available, accountWithLoad{
					account:  acc,
					loadInfo: loadInfo,
				})
			}
		}

		// 分层过滤选择：优先级 →（可选）最早重置 → 负载率 → LRU
		for len(available) > 0 {
			// 1. 取优先级最小的集合
			candidates := filterByMinPriority(available)
			// 2. （可选）use-it-or-lose-it：优先选用会话窗口最早重置的账号
			if cfg.PreferSoonestReset {
				candidates = filterBySoonestReset(candidates)
			}
			// 3. 取负载率最低的集合
			candidates = filterByMinLoadRate(candidates)
			// 4. LRU 选择最久未用的账号
			selected := selectByLRU(candidates, preferOAuth)
			if selected == nil {
				break
			}

			result, err := s.tryAcquireAccountSlot(ctx, selected.account.ID, selected.account.Concurrency)
			if err == nil && result.Acquired {
				// 会话数量限制检查
				if !s.checkAndRegisterSession(ctx, selected.account, sessionHash) {
					result.ReleaseFunc() // 释放槽位，继续尝试下一个账号
				} else {
					if sessionHash != "" && s.cache != nil {
						_ = s.cache.SetSessionAccountID(ctx, derefGroupID(groupID), sessionHash, selected.account.ID, stickySessionTTL)
					}
					return s.newSelectionResult(ctx, selected.account, true, result.ReleaseFunc, nil)
				}
			}

			// 移除已尝试的账号，重新进行分层过滤
			selectedID := selected.account.ID
			newAvailable := make([]accountWithLoad, 0, len(available)-1)
			for _, acc := range available {
				if acc.account.ID != selectedID {
					newAvailable = append(newAvailable, acc)
				}
			}
			available = newAvailable
		}
	}

	// ============ Layer 3: 兜底排队 ============
	s.sortCandidatesForFallback(candidates, preferOAuth, cfg.FallbackSelectionMode)
	for _, acc := range candidates {
		// 会话数量限制检查（等待计划也需要占用会话配额）
		if !s.checkAndRegisterSession(ctx, acc, sessionHash) {
			continue // 会话限制已满，尝试下一个账号
		}
		return s.newSelectionResult(ctx, acc, false, nil, &AccountWaitPlan{
			AccountID:      acc.ID,
			MaxConcurrency: acc.Concurrency,
			Timeout:        cfg.FallbackWaitTimeout,
			MaxWaiting:     cfg.FallbackMaxWaiting,
		})
	}
	return nil, ErrNoAvailableAccounts
}

func (s *GatewayService) tryAcquireByLegacyOrder(ctx context.Context, candidates []*Account, groupID *int64, sessionHash string, preferOAuth bool) (*AccountSelectionResult, bool, error) {
	ordered := append([]*Account(nil), candidates...)
	sortAccountsByPriorityAndLastUsed(ordered, preferOAuth)

	for _, acc := range ordered {
		result, err := s.tryAcquireAccountSlot(ctx, acc.ID, acc.Concurrency)
		if err == nil && result.Acquired {
			// 会话数量限制检查
			if !s.checkAndRegisterSession(ctx, acc, sessionHash) {
				result.ReleaseFunc() // 释放槽位，继续尝试下一个账号
				continue
			}
			if sessionHash != "" && s.cache != nil {
				_ = s.cache.SetSessionAccountID(ctx, derefGroupID(groupID), sessionHash, acc.ID, stickySessionTTL)
			}
			selection, err := s.newSelectionResult(ctx, acc, true, result.ReleaseFunc, nil)
			if err != nil {
				return nil, false, err
			}
			return selection, true, nil
		}
	}

	return nil, false, nil
}

func (s *GatewayService) schedulingConfig() config.GatewaySchedulingConfig {
	if s.cfg != nil {
		return s.cfg.Gateway.Scheduling
	}
	return config.GatewaySchedulingConfig{
		StickySessionMaxWaiting:  3,
		StickySessionWaitTimeout: 45 * time.Second,
		FallbackWaitTimeout:      30 * time.Second,
		FallbackMaxWaiting:       100,
		LoadBatchEnabled:         true,
		SlotCleanupInterval:      30 * time.Second,
	}
}

func (s *GatewayService) withGroupContext(ctx context.Context, group *Group) context.Context {
	if !IsGroupContextValid(group) {
		return ctx
	}
	if existing, ok := ctx.Value(ctxkey.Group).(*Group); ok && existing != nil && existing.ID == group.ID && IsGroupContextValid(existing) {
		return ctx
	}
	return context.WithValue(ctx, ctxkey.Group, group)
}

func (s *GatewayService) groupFromContext(ctx context.Context, groupID int64) *Group {
	if group, ok := ctx.Value(ctxkey.Group).(*Group); ok && IsGroupContextValid(group) && group.ID == groupID {
		return group
	}
	return nil
}

func (s *GatewayService) resolveGroupByID(ctx context.Context, groupID int64) (*Group, error) {
	if group := s.groupFromContext(ctx, groupID); group != nil {
		return group, nil
	}
	group, err := s.groupRepo.GetByIDLite(ctx, groupID)
	if err != nil {
		return nil, fmt.Errorf("get group failed: %w", err)
	}
	return group, nil
}

func (s *GatewayService) ResolveGroupByID(ctx context.Context, groupID int64) (*Group, error) {
	return s.resolveGroupByID(ctx, groupID)
}

func (s *GatewayService) routingAccountIDsForRequest(ctx context.Context, groupID *int64, requestedModel string, platform string) []int64 {
	if groupID == nil || requestedModel == "" || platform != PlatformAnthropic {
		return nil
	}
	group, err := s.resolveGroupByID(ctx, *groupID)
	if err != nil || group == nil {
		if s.debugModelRoutingEnabled() {
			logger.LegacyPrintf("service.gateway", "[ModelRoutingDebug] resolve group failed: group_id=%v model=%s platform=%s err=%v", derefGroupID(groupID), requestedModel, platform, err)
		}
		return nil
	}
	// Model routing applies only to requests resolved to Anthropic. Composite
	// groups may still use those rules once their model resolved to Anthropic.
	if group.Platform != PlatformAnthropic && group.Platform != PlatformComposite {
		if s.debugModelRoutingEnabled() {
			logger.LegacyPrintf("service.gateway", "[ModelRoutingDebug] skip: non-anthropic group platform: group_id=%d group_platform=%s model=%s", group.ID, group.Platform, requestedModel)
		}
		return nil
	}
	ids := group.GetRoutingAccountIDs(requestedModel)
	if s.debugModelRoutingEnabled() {
		logger.LegacyPrintf("service.gateway", "[ModelRoutingDebug] routing lookup: group_id=%d model=%s enabled=%v rules=%d matched_ids=%v",
			group.ID, requestedModel, group.ModelRoutingEnabled, len(group.ModelRouting), ids)
	}
	return ids
}

func (s *GatewayService) resolveGatewayGroup(ctx context.Context, groupID *int64) (*Group, *int64, error) {
	if groupID == nil {
		return nil, nil, nil
	}

	currentID := *groupID
	visited := map[int64]struct{}{}
	for {
		if _, seen := visited[currentID]; seen {
			return nil, nil, fmt.Errorf("fallback group cycle detected")
		}
		visited[currentID] = struct{}{}

		group, err := s.resolveGroupByID(ctx, currentID)
		if err != nil {
			return nil, nil, err
		}

		if !group.ClaudeCodeOnly || IsClaudeCodeClient(ctx) {
			return group, &currentID, nil
		}

		if group.FallbackGroupID == nil {
			return nil, nil, ErrClaudeCodeOnly
		}
		currentID = *group.FallbackGroupID
	}
}

// checkClaudeCodeRestriction 检查分组的 Claude Code 客户端限制
// 如果分组启用了 claude_code_only 且请求不是来自 Claude Code 客户端：
//   - 有降级分组：返回降级分组的 ID
//   - 无降级分组：返回 ErrClaudeCodeOnly 错误
func (s *GatewayService) checkClaudeCodeRestriction(ctx context.Context, groupID *int64) (*Group, *int64, error) {
	if groupID == nil {
		return nil, groupID, nil
	}

	// 强制平台模式不检查 Claude Code 限制
	if forcePlatform, hasForcePlatform := ctx.Value(ctxkey.ForcePlatform).(string); hasForcePlatform && forcePlatform != "" {
		return nil, groupID, nil
	}

	group, resolvedID, err := s.resolveGatewayGroup(ctx, groupID)
	if err != nil {
		return nil, nil, err
	}

	return group, resolvedID, nil
}

func (s *GatewayService) resolvePlatform(ctx context.Context, groupID *int64, group *Group, requestedModel string) (string, bool, error) {
	forcePlatform, hasForcePlatform := ctx.Value(ctxkey.ForcePlatform).(string)
	if hasForcePlatform && forcePlatform != "" {
		return forcePlatform, true, nil
	}
	if platform, ok := ResolvedTargetPlatformFromContext(ctx); ok {
		return platform, false, nil
	}
	if group != nil {
		if group.Platform == PlatformComposite {
			decision, ok, err := s.resolveCompositeRouteDecision(ctx, group, requestedModel, CompositeRouteEndpointAny)
			if err != nil {
				return "", false, err
			}
			if !ok {
				return "", false, fmt.Errorf("%w supporting model: %s (composite target platform unknown)", ErrNoAvailableAccounts, requestedModel)
			}
			return decision.TargetPlatform, false, nil
		}
		return group.Platform, false, nil
	}
	if groupID != nil {
		group, err := s.resolveGroupByID(ctx, *groupID)
		if err != nil {
			return "", false, err
		}
		if group.Platform == PlatformComposite {
			decision, ok, err := s.resolveCompositeRouteDecision(ctx, group, requestedModel, CompositeRouteEndpointAny)
			if err != nil {
				return "", false, err
			}
			if !ok {
				return "", false, fmt.Errorf("%w supporting model: %s (composite target platform unknown)", ErrNoAvailableAccounts, requestedModel)
			}
			return decision.TargetPlatform, false, nil
		}
		return group.Platform, false, nil
	}
	return PlatformAnthropic, false, nil
}

func (s *GatewayService) listSchedulableAccounts(ctx context.Context, groupID *int64, platform string, hasForcePlatform bool) ([]Account, bool, error) {
	if s.schedulerSnapshot != nil {
		accounts, useMixed, err := s.schedulerSnapshot.ListSchedulableAccounts(ctx, groupID, platform, hasForcePlatform)
		if err == nil {
			slog.Debug("account_scheduling_list_snapshot",
				"group_id", derefGroupID(groupID),
				"platform", platform,
				"use_mixed", useMixed,
				"count", len(accounts))
			if slog.Default().Enabled(ctx, slog.LevelDebug) {
				for _, acc := range accounts {
					slog.Debug("account_scheduling_account_detail",
						"account_id", acc.ID,
						"name", acc.Name,
						"platform", acc.Platform,
						"type", acc.Type,
						"status", acc.Status,
						"tls_fingerprint", acc.IsTLSFingerprintEnabled())
				}
			}
		}
		return accounts, useMixed, err
	}
	useMixed := (platform == PlatformAnthropic || platform == PlatformGemini) && !hasForcePlatform
	if useMixed {
		platforms := []string{platform, PlatformAntigravity}
		var accounts []Account
		var err error
		if groupID != nil {
			accounts, err = s.accountRepo.ListSchedulableByGroupIDAndPlatforms(ctx, *groupID, platforms)
		} else if s.cfg != nil && s.cfg.RunMode == config.RunModeSimple {
			accounts, err = s.accountRepo.ListSchedulableByPlatforms(ctx, platforms)
		} else {
			accounts, err = s.accountRepo.ListSchedulableUngroupedByPlatforms(ctx, platforms)
		}
		if err != nil {
			slog.Debug("account_scheduling_list_failed",
				"group_id", derefGroupID(groupID),
				"platform", platform,
				"error", err)
			return nil, useMixed, err
		}
		filtered := make([]Account, 0, len(accounts))
		for _, acc := range accounts {
			if acc.Platform == PlatformAntigravity && !acc.IsMixedSchedulingEnabled() {
				continue
			}
			filtered = append(filtered, acc)
		}
		slog.Debug("account_scheduling_list_mixed",
			"group_id", derefGroupID(groupID),
			"platform", platform,
			"raw_count", len(accounts),
			"filtered_count", len(filtered))
		if slog.Default().Enabled(ctx, slog.LevelDebug) {
			for _, acc := range filtered {
				slog.Debug("account_scheduling_account_detail",
					"account_id", acc.ID,
					"name", acc.Name,
					"platform", acc.Platform,
					"type", acc.Type,
					"status", acc.Status,
					"tls_fingerprint", acc.IsTLSFingerprintEnabled())
			}
		}
		return filtered, useMixed, nil
	}

	var accounts []Account
	var err error
	if s.cfg != nil && s.cfg.RunMode == config.RunModeSimple {
		accounts, err = s.accountRepo.ListSchedulableByPlatform(ctx, platform)
	} else if groupID != nil {
		accounts, err = s.accountRepo.ListSchedulableByGroupIDAndPlatform(ctx, *groupID, platform)
		// 分组内无账号则返回空列表，由上层处理错误，不再回退到全平台查询
	} else {
		accounts, err = s.accountRepo.ListSchedulableUngroupedByPlatform(ctx, platform)
	}
	if err != nil {
		slog.Debug("account_scheduling_list_failed",
			"group_id", derefGroupID(groupID),
			"platform", platform,
			"error", err)
		return nil, useMixed, err
	}
	slog.Debug("account_scheduling_list_single",
		"group_id", derefGroupID(groupID),
		"platform", platform,
		"count", len(accounts))
	if slog.Default().Enabled(ctx, slog.LevelDebug) {
		for _, acc := range accounts {
			slog.Debug("account_scheduling_account_detail",
				"account_id", acc.ID,
				"name", acc.Name,
				"platform", acc.Platform,
				"type", acc.Type,
				"status", acc.Status,
				"tls_fingerprint", acc.IsTLSFingerprintEnabled())
		}
	}
	return accounts, useMixed, nil
}

// IsSingleAntigravityAccountGroup 检查指定分组是否只有一个 antigravity 平台的可调度账号。
// 用于 Handler 层在首次请求时提前设置 SingleAccountRetry context，
// 避免单账号分组收到 503 时错误地设置模型限流标记导致后续请求连续快速失败。
func (s *GatewayService) IsSingleAntigravityAccountGroup(ctx context.Context, groupID *int64) bool {
	accounts, _, err := s.listSchedulableAccounts(ctx, groupID, PlatformAntigravity, true)
	if err != nil {
		return false
	}
	return len(accounts) == 1
}

func (s *GatewayService) isAccountAllowedForPlatform(account *Account, platform string, useMixed bool) bool {
	if account == nil {
		return false
	}
	if useMixed {
		if account.Platform == platform {
			return true
		}
		return account.Platform == PlatformAntigravity && account.IsMixedSchedulingEnabled()
	}
	return account.Platform == platform
}

func (s *GatewayService) isAccountSchedulableForSelection(account *Account) bool {
	if account == nil {
		return false
	}
	return account.IsSchedulable()
}

func (s *GatewayService) isAccountSchedulableForModelSelection(ctx context.Context, account *Account, requestedModel string) bool {
	if account == nil {
		return false
	}
	return account.IsSchedulableForModelWithContext(ctx, requestedModel)
}

// isAccountInGroup checks if the account belongs to the specified group.
// When groupID is nil, returns true only for ungrouped accounts (no group assignments).
func (s *GatewayService) isAccountInGroup(account *Account, groupID *int64) bool {
	if account == nil {
		return false
	}
	if groupID == nil {
		// 无分组的 API Key 只能使用未分组的账号
		return len(account.AccountGroups) == 0
	}
	for _, ag := range account.AccountGroups {
		if ag.GroupID == *groupID {
			return true
		}
	}
	return false
}

func (s *GatewayService) tryAcquireAccountSlot(ctx context.Context, accountID int64, maxConcurrency int) (*AcquireResult, error) {
	if s.concurrencyService == nil {
		return &AcquireResult{Acquired: true, ReleaseFunc: func() {}}, nil
	}
	return s.concurrencyService.AcquireAccountSlot(ctx, accountID, maxConcurrency)
}

type usageLogWindowStatsBatchProvider interface {
	GetAccountWindowStatsBatch(ctx context.Context, accountIDs []int64, startTime time.Time) (map[int64]*usagestats.AccountStats, error)
}

type windowCostPrefetchContextKeyType struct{}

var windowCostPrefetchContextKey = windowCostPrefetchContextKeyType{}

func windowCostFromPrefetchContext(ctx context.Context, accountID int64) (float64, bool) {
	if ctx == nil || accountID <= 0 {
		return 0, false
	}
	m, ok := ctx.Value(windowCostPrefetchContextKey).(map[int64]float64)
	if !ok || len(m) == 0 {
		return 0, false
	}
	v, exists := m[accountID]
	return v, exists
}

func (s *GatewayService) withWindowCostPrefetch(ctx context.Context, accounts []Account) context.Context {
	if ctx == nil || len(accounts) == 0 || s.sessionLimitCache == nil || s.usageLogRepo == nil {
		return ctx
	}

	accountByID := make(map[int64]*Account)
	accountIDs := make([]int64, 0, len(accounts))
	for i := range accounts {
		account := &accounts[i]
		if account == nil || !account.IsAnthropicOAuthOrSetupToken() {
			continue
		}
		if account.GetWindowCostLimit() <= 0 {
			continue
		}
		accountByID[account.ID] = account
		accountIDs = append(accountIDs, account.ID)
	}
	if len(accountIDs) == 0 {
		return ctx
	}

	costs := make(map[int64]float64, len(accountIDs))
	cacheValues, err := s.sessionLimitCache.GetWindowCostBatch(ctx, accountIDs)
	if err == nil {
		for accountID, cost := range cacheValues {
			costs[accountID] = cost
		}
		windowCostPrefetchCacheHitTotal.Add(int64(len(cacheValues)))
	} else {
		windowCostPrefetchErrorTotal.Add(1)
		logger.LegacyPrintf("service.gateway", "window_cost batch cache read failed: %v", err)
	}
	cacheMissCount := len(accountIDs) - len(costs)
	if cacheMissCount < 0 {
		cacheMissCount = 0
	}
	windowCostPrefetchCacheMissTotal.Add(int64(cacheMissCount))

	missingByStart := make(map[int64][]int64)
	startTimes := make(map[int64]time.Time)
	for _, accountID := range accountIDs {
		if _, ok := costs[accountID]; ok {
			continue
		}
		account := accountByID[accountID]
		if account == nil {
			continue
		}
		startTime := account.GetCurrentWindowStartTime()
		startKey := startTime.Unix()
		missingByStart[startKey] = append(missingByStart[startKey], accountID)
		startTimes[startKey] = startTime
	}
	if len(missingByStart) == 0 {
		return context.WithValue(ctx, windowCostPrefetchContextKey, costs)
	}

	batchReader, hasBatch := s.usageLogRepo.(usageLogWindowStatsBatchProvider)
	for startKey, ids := range missingByStart {
		startTime := startTimes[startKey]

		if hasBatch {
			windowCostPrefetchBatchSQLTotal.Add(1)
			queryStart := time.Now()
			statsByAccount, err := batchReader.GetAccountWindowStatsBatch(ctx, ids, startTime)
			if err == nil {
				slog.Debug("window_cost_batch_query_ok",
					"accounts", len(ids),
					"window_start", startTime.Format(time.RFC3339),
					"duration_ms", time.Since(queryStart).Milliseconds())
				for _, accountID := range ids {
					stats := statsByAccount[accountID]
					cost := 0.0
					if stats != nil {
						cost = stats.StandardCost
					}
					costs[accountID] = cost
					_ = s.sessionLimitCache.SetWindowCost(ctx, accountID, cost)
				}
				continue
			}
			windowCostPrefetchErrorTotal.Add(1)
			logger.LegacyPrintf("service.gateway", "window_cost batch db query failed: start=%s err=%v", startTime.Format(time.RFC3339), err)
		}

		// 回退路径：缺少批量仓储能力或批量查询失败时，按账号单查（失败开放）。
		windowCostPrefetchFallbackTotal.Add(int64(len(ids)))
		for _, accountID := range ids {
			stats, err := s.usageLogRepo.GetAccountWindowStats(ctx, accountID, startTime)
			if err != nil {
				windowCostPrefetchErrorTotal.Add(1)
				continue
			}
			cost := stats.StandardCost
			costs[accountID] = cost
			_ = s.sessionLimitCache.SetWindowCost(ctx, accountID, cost)
		}
	}

	return context.WithValue(ctx, windowCostPrefetchContextKey, costs)
}

// isAccountSchedulableForQuota 检查账号是否在配额限制内
// 适用于配置了 quota_limit 的 apikey 和 bedrock 类型账号
func (s *GatewayService) isAccountSchedulableForQuota(account *Account) bool {
	if !account.IsAPIKeyOrBedrock() {
		return true
	}
	return !account.IsQuotaExceeded()
}

// isAccountSchedulableForWindowCost 检查账号是否可根据窗口费用进行调度
// 仅适用于 Anthropic OAuth/SetupToken 账号
// 返回 true 表示可调度，false 表示不可调度
func (s *GatewayService) isAccountSchedulableForWindowCost(ctx context.Context, account *Account, isSticky bool) bool {
	// 只检查 Anthropic OAuth/SetupToken 账号
	if !account.IsAnthropicOAuthOrSetupToken() {
		return true
	}

	limit := account.GetWindowCostLimit()
	if limit <= 0 {
		return true // 未启用窗口费用限制
	}

	// 尝试从缓存获取窗口费用
	var currentCost float64
	if cost, ok := windowCostFromPrefetchContext(ctx, account.ID); ok {
		currentCost = cost
		goto checkSchedulability
	}
	if s.sessionLimitCache != nil {
		if cost, hit, err := s.sessionLimitCache.GetWindowCost(ctx, account.ID); err == nil && hit {
			currentCost = cost
			goto checkSchedulability
		}
	}

	// 缓存未命中，从数据库查询
	{
		// 使用统一的窗口开始时间计算逻辑（考虑窗口过期情况）
		startTime := account.GetCurrentWindowStartTime()

		stats, err := s.usageLogRepo.GetAccountWindowStats(ctx, account.ID, startTime)
		if err != nil {
			// 失败开放：查询失败时允许调度
			return true
		}

		// 使用标准费用（不含账号倍率）
		currentCost = stats.StandardCost

		// 设置缓存（忽略错误）
		if s.sessionLimitCache != nil {
			_ = s.sessionLimitCache.SetWindowCost(ctx, account.ID, currentCost)
		}
	}

checkSchedulability:
	schedulability := account.CheckWindowCostSchedulability(currentCost)

	switch schedulability {
	case WindowCostSchedulable:
		return true
	case WindowCostStickyOnly:
		return isSticky
	case WindowCostNotSchedulable:
		return false
	}
	return true
}

// rpmPrefetchContextKey is the context key for prefetched RPM counts.
type rpmPrefetchContextKeyType struct{}

var rpmPrefetchContextKey = rpmPrefetchContextKeyType{}

func rpmFromPrefetchContext(ctx context.Context, accountID int64) (int, bool) {
	if v, ok := ctx.Value(rpmPrefetchContextKey).(map[int64]int); ok {
		count, found := v[accountID]
		return count, found
	}
	return 0, false
}

// withRPMPrefetch 批量预取所有候选账号的 RPM 计数
func (s *GatewayService) withRPMPrefetch(ctx context.Context, accounts []Account) context.Context {
	if s.rpmCache == nil {
		return ctx
	}

	var ids []int64
	for i := range accounts {
		if accounts[i].IsAnthropicOAuthOrSetupToken() && accounts[i].GetBaseRPM() > 0 {
			ids = append(ids, accounts[i].ID)
		}
	}
	if len(ids) == 0 {
		return ctx
	}

	counts, err := s.rpmCache.GetRPMBatch(ctx, ids)
	if err != nil {
		return ctx // 失败开放
	}
	return context.WithValue(ctx, rpmPrefetchContextKey, counts)
}

// isAccountSchedulableForRPM 检查账号是否可根据 RPM 进行调度
// 仅适用于 Anthropic OAuth/SetupToken 账号
func (s *GatewayService) isAccountSchedulableForRPM(ctx context.Context, account *Account, isSticky bool) bool {
	if !account.IsAnthropicOAuthOrSetupToken() {
		return true
	}
	baseRPM := account.GetBaseRPM()
	if baseRPM <= 0 {
		return true
	}

	// 尝试从预取缓存获取
	var currentRPM int
	if count, ok := rpmFromPrefetchContext(ctx, account.ID); ok {
		currentRPM = count
	} else if s.rpmCache != nil {
		if count, err := s.rpmCache.GetRPM(ctx, account.ID); err == nil {
			currentRPM = count
		}
		// 失败开放：GetRPM 错误时允许调度
	}

	schedulability := account.CheckRPMSchedulability(currentRPM)
	switch schedulability {
	case WindowCostSchedulable:
		return true
	case WindowCostStickyOnly:
		return isSticky
	case WindowCostNotSchedulable:
		return false
	}
	return true
}

// IncrementAccountRPM increments the RPM counter for the given account.
// 已知 TOCTOU 竞态：调度时读取 RPM 计数与此处递增之间存在时间窗口，
// 高并发下可能短暂超出 RPM 限制。这是与 WindowCost 一致的 soft-limit
// 设计权衡——可接受的少量超额优于加锁带来的延迟和复杂度。
func (s *GatewayService) IncrementAccountRPM(ctx context.Context, accountID int64) error {
	if s.rpmCache == nil {
		return nil
	}
	_, err := s.rpmCache.IncrementRPM(ctx, accountID)
	return err
}

// checkAndRegisterSession 检查并注册会话，用于会话数量限制
// 仅适用于 Anthropic OAuth/SetupToken 账号
// sessionID: 会话标识符（使用粘性会话的 hash）
// 返回 true 表示允许（在限制内或会话已存在），false 表示拒绝（超出限制且是新会话）
func (s *GatewayService) checkAndRegisterSession(ctx context.Context, account *Account, sessionID string) bool {
	// 只检查 Anthropic OAuth/SetupToken 账号
	if !account.IsAnthropicOAuthOrSetupToken() {
		return true
	}

	maxSessions := account.GetMaxSessions()
	if maxSessions <= 0 || sessionID == "" {
		return true // 未启用会话限制或无会话ID
	}

	if s.sessionLimitCache == nil {
		return true // 缓存不可用时允许通过
	}

	idleTimeout := time.Duration(account.GetSessionIdleTimeoutMinutes()) * time.Minute

	allowed, err := s.sessionLimitCache.RegisterSession(ctx, account.ID, sessionID, maxSessions, idleTimeout)
	if err != nil {
		// 失败开放：缓存错误时允许通过
		return true
	}
	return allowed
}

func (s *GatewayService) getSchedulableAccount(ctx context.Context, accountID int64) (*Account, error) {
	if s.schedulerSnapshot != nil {
		return s.schedulerSnapshot.GetAccount(ctx, accountID)
	}
	return s.accountRepo.GetByID(ctx, accountID)
}

func (s *GatewayService) hydrateSelectedAccount(ctx context.Context, account *Account) (*Account, error) {
	if account == nil || s.schedulerSnapshot == nil {
		return account, nil
	}
	hydrated, err := s.schedulerSnapshot.GetAccount(ctx, account.ID)
	if err != nil {
		return nil, err
	}
	if hydrated == nil {
		return nil, fmt.Errorf("selected gateway account %d not found during hydration", account.ID)
	}
	return hydrated, nil
}

func (s *GatewayService) newSelectionResult(ctx context.Context, account *Account, acquired bool, release func(), waitPlan *AccountWaitPlan) (*AccountSelectionResult, error) {
	hydrated, err := s.hydrateSelectedAccount(ctx, account)
	if err != nil {
		return nil, err
	}
	return &AccountSelectionResult{
		Account:     hydrated,
		Acquired:    acquired,
		ReleaseFunc: release,
		WaitPlan:    waitPlan,
	}, nil
}

// filterByMinPriority 过滤出优先级最小的账号集合
func filterByMinPriority(accounts []accountWithLoad) []accountWithLoad {
	if len(accounts) == 0 {
		return accounts
	}
	minPriority := accounts[0].account.Priority
	for _, acc := range accounts[1:] {
		if acc.account.Priority < minPriority {
			minPriority = acc.account.Priority
		}
	}
	result := make([]accountWithLoad, 0, len(accounts))
	for _, acc := range accounts {
		if acc.account.Priority == minPriority {
			result = append(result, acc)
		}
	}
	return result
}

// filterByMinLoadRate 过滤出负载率最低的账号集合
func filterByMinLoadRate(accounts []accountWithLoad) []accountWithLoad {
	if len(accounts) == 0 {
		return accounts
	}
	minLoadRate := accounts[0].loadInfo.LoadRate
	for _, acc := range accounts[1:] {
		if acc.loadInfo.LoadRate < minLoadRate {
			minLoadRate = acc.loadInfo.LoadRate
		}
	}
	result := make([]accountWithLoad, 0, len(accounts))
	for _, acc := range accounts {
		if acc.loadInfo.LoadRate == minLoadRate {
			result = append(result, acc)
		}
	}
	return result
}

// filterBySoonestReset 过滤出「会话窗口最早重置」的账号集合（use-it-or-lose-it）。
// 仅保留拥有未来重置时间（SessionWindowEnd 在当前时间之后）且最早的账号；
// 窗口为空或已过期的账号视为无活跃窗口、优先级最低。
// 当所有账号都没有活跃窗口时，返回原集合（不改变后续 LRU 选择）。
func filterBySoonestReset(accounts []accountWithLoad) []accountWithLoad {
	if len(accounts) <= 1 {
		return accounts
	}
	now := time.Now()
	var minEnd *time.Time
	for _, acc := range accounts {
		end := acc.account.SessionWindowEnd
		if end == nil || !now.Before(*end) {
			continue
		}
		if minEnd == nil || end.Before(*minEnd) {
			minEnd = end
		}
	}
	if minEnd == nil {
		// 没有任何账号拥有活跃窗口，保持原集合
		return accounts
	}
	result := make([]accountWithLoad, 0, len(accounts))
	for _, acc := range accounts {
		end := acc.account.SessionWindowEnd
		if end != nil && now.Before(*end) && end.Equal(*minEnd) {
			result = append(result, acc)
		}
	}
	return result
}

// selectByLRU 从集合中选择最久未用的账号
// 如果有多个账号具有相同的最小 LastUsedAt，则随机选择一个
func selectByLRU(accounts []accountWithLoad, preferOAuth bool) *accountWithLoad {
	if len(accounts) == 0 {
		return nil
	}
	if len(accounts) == 1 {
		return &accounts[0]
	}

	// 1. 找到最小的 LastUsedAt（nil 被视为最小）
	var minTime *time.Time
	hasNil := false
	for _, acc := range accounts {
		if acc.account.LastUsedAt == nil {
			hasNil = true
			break
		}
		if minTime == nil || acc.account.LastUsedAt.Before(*minTime) {
			minTime = acc.account.LastUsedAt
		}
	}

	// 2. 收集所有具有最小 LastUsedAt 的账号索引
	var candidateIdxs []int
	for i, acc := range accounts {
		if hasNil {
			if acc.account.LastUsedAt == nil {
				candidateIdxs = append(candidateIdxs, i)
			}
		} else {
			if acc.account.LastUsedAt != nil && acc.account.LastUsedAt.Equal(*minTime) {
				candidateIdxs = append(candidateIdxs, i)
			}
		}
	}

	// 3. 如果只有一个候选，直接返回
	if len(candidateIdxs) == 1 {
		return &accounts[candidateIdxs[0]]
	}

	// 4. 如果有多个候选且 preferOAuth，优先选择 OAuth 类型
	if preferOAuth {
		var oauthIdxs []int
		for _, idx := range candidateIdxs {
			if accounts[idx].account.Type == AccountTypeOAuth {
				oauthIdxs = append(oauthIdxs, idx)
			}
		}
		if len(oauthIdxs) > 0 {
			candidateIdxs = oauthIdxs
		}
	}

	// 5. 随机选择一个
	selectedIdx := candidateIdxs[mathrand.Intn(len(candidateIdxs))]
	return &accounts[selectedIdx]
}

func sortAccountsByPriorityAndLastUsed(accounts []*Account, preferOAuth bool) {
	sort.SliceStable(accounts, func(i, j int) bool {
		a, b := accounts[i], accounts[j]
		if a.Priority != b.Priority {
			return a.Priority < b.Priority
		}
		switch {
		case a.LastUsedAt == nil && b.LastUsedAt != nil:
			return true
		case a.LastUsedAt != nil && b.LastUsedAt == nil:
			return false
		case a.LastUsedAt == nil && b.LastUsedAt == nil:
			if preferOAuth && a.Type != b.Type {
				return a.Type == AccountTypeOAuth
			}
			return false
		default:
			return a.LastUsedAt.Before(*b.LastUsedAt)
		}
	})
	shuffleWithinPriorityAndLastUsed(accounts, preferOAuth)
}

// shuffleWithinSortGroups 对排序后的 accountWithLoad 切片，按 (Priority, LoadRate, LastUsedAt) 分组后组内随机打乱。
// 防止并发请求读取同一快照时，确定性排序导致所有请求命中相同账号。
func shuffleWithinSortGroups(accounts []accountWithLoad) {
	if len(accounts) <= 1 {
		return
	}
	i := 0
	for i < len(accounts) {
		j := i + 1
		for j < len(accounts) && sameAccountWithLoadGroup(accounts[i], accounts[j]) {
			j++
		}
		if j-i > 1 {
			mathrand.Shuffle(j-i, func(a, b int) {
				accounts[i+a], accounts[i+b] = accounts[i+b], accounts[i+a]
			})
		}
		i = j
	}
}

// sameAccountWithLoadGroup 判断两个 accountWithLoad 是否属于同一排序组
func sameAccountWithLoadGroup(a, b accountWithLoad) bool {
	if a.account.Priority != b.account.Priority {
		return false
	}
	if a.loadInfo.LoadRate != b.loadInfo.LoadRate {
		return false
	}
	return sameLastUsedAt(a.account.LastUsedAt, b.account.LastUsedAt)
}

// shuffleWithinPriorityAndLastUsed 对排序后的 []*Account 切片，按 (Priority, LastUsedAt) 分组后组内随机打乱。
//
// 注意：当 preferOAuth=true 时，需要保证 OAuth 账号在同组内仍然优先，否则会把排序时的偏好打散掉。
// 因此这里采用"组内分区 + 分区内 shuffle"的方式：
// - 先把同组账号按 (OAuth / 非 OAuth) 拆成两段，保持 OAuth 段在前；
// - 再分别在各段内随机打散，避免热点。
func shuffleWithinPriorityAndLastUsed(accounts []*Account, preferOAuth bool) {
	if len(accounts) <= 1 {
		return
	}
	i := 0
	for i < len(accounts) {
		j := i + 1
		for j < len(accounts) && sameAccountGroup(accounts[i], accounts[j]) {
			j++
		}
		if j-i > 1 {
			if preferOAuth {
				oauth := make([]*Account, 0, j-i)
				others := make([]*Account, 0, j-i)
				for _, acc := range accounts[i:j] {
					if acc.Type == AccountTypeOAuth {
						oauth = append(oauth, acc)
					} else {
						others = append(others, acc)
					}
				}
				if len(oauth) > 1 {
					mathrand.Shuffle(len(oauth), func(a, b int) { oauth[a], oauth[b] = oauth[b], oauth[a] })
				}
				if len(others) > 1 {
					mathrand.Shuffle(len(others), func(a, b int) { others[a], others[b] = others[b], others[a] })
				}
				copy(accounts[i:], oauth)
				copy(accounts[i+len(oauth):], others)
			} else {
				mathrand.Shuffle(j-i, func(a, b int) {
					accounts[i+a], accounts[i+b] = accounts[i+b], accounts[i+a]
				})
			}
		}
		i = j
	}
}

// sameAccountGroup 判断两个 Account 是否属于同一排序组（Priority + LastUsedAt）
func sameAccountGroup(a, b *Account) bool {
	if a.Priority != b.Priority {
		return false
	}
	return sameLastUsedAt(a.LastUsedAt, b.LastUsedAt)
}

// sameLastUsedAt 判断两个 LastUsedAt 是否相同（精度到秒）
func sameLastUsedAt(a, b *time.Time) bool {
	switch {
	case a == nil && b == nil:
		return true
	case a == nil || b == nil:
		return false
	default:
		return a.Unix() == b.Unix()
	}
}

// sortCandidatesForFallback 根据配置选择排序策略
// mode: "last_used"(按最后使用时间) 或 "random"(随机)
func (s *GatewayService) sortCandidatesForFallback(accounts []*Account, preferOAuth bool, mode string) {
	if mode == "random" {
		// 先按优先级排序，然后在同优先级内随机打乱
		sortAccountsByPriorityOnly(accounts, preferOAuth)
		shuffleWithinPriority(accounts)
	} else {
		// 默认按最后使用时间排序
		sortAccountsByPriorityAndLastUsed(accounts, preferOAuth)
	}
}

// sortAccountsByPriorityOnly 仅按优先级排序
func sortAccountsByPriorityOnly(accounts []*Account, preferOAuth bool) {
	sort.SliceStable(accounts, func(i, j int) bool {
		a, b := accounts[i], accounts[j]
		if a.Priority != b.Priority {
			return a.Priority < b.Priority
		}
		if preferOAuth && a.Type != b.Type {
			return a.Type == AccountTypeOAuth
		}
		return false
	})
}

// shuffleWithinPriority 在同优先级内随机打乱顺序
func shuffleWithinPriority(accounts []*Account) {
	if len(accounts) <= 1 {
		return
	}
	r := mathrand.New(mathrand.NewSource(time.Now().UnixNano()))
	start := 0
	for start < len(accounts) {
		priority := accounts[start].Priority
		end := start + 1
		for end < len(accounts) && accounts[end].Priority == priority {
			end++
		}
		// 对 [start, end) 范围内的账户随机打乱
		if end-start > 1 {
			r.Shuffle(end-start, func(i, j int) {
				accounts[start+i], accounts[start+j] = accounts[start+j], accounts[start+i]
			})
		}
		start = end
	}
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
				continue
			}
			if acc.Priority < selected.Priority {
				selected = acc
			} else if acc.Priority == selected.Priority {
				switch {
				case acc.LastUsedAt == nil && selected.LastUsedAt != nil:
					selected = acc
				case acc.LastUsedAt != nil && selected.LastUsedAt == nil:
					// keep selected (never used is preferred)
				case acc.LastUsedAt == nil && selected.LastUsedAt == nil:
					if preferOAuth && acc.Type != selected.Type && acc.Type == AccountTypeOAuth {
						selected = acc
					}
				default:
					if acc.LastUsedAt.Before(*selected.LastUsedAt) {
						selected = acc
					}
				}
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
			continue
		}
		if acc.Priority < selected.Priority {
			selected = acc
		} else if acc.Priority == selected.Priority {
			switch {
			case acc.LastUsedAt == nil && selected.LastUsedAt != nil:
				selected = acc
			case acc.LastUsedAt != nil && selected.LastUsedAt == nil:
				// keep selected (never used is preferred)
			case acc.LastUsedAt == nil && selected.LastUsedAt == nil:
				if preferOAuth && acc.Type != selected.Type && acc.Type == AccountTypeOAuth {
					selected = acc
				}
			default:
				if acc.LastUsedAt.Before(*selected.LastUsedAt) {
					selected = acc
				}
			}
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
				continue
			}
			if acc.Priority < selected.Priority {
				selected = acc
			} else if acc.Priority == selected.Priority {
				switch {
				case acc.LastUsedAt == nil && selected.LastUsedAt != nil:
					selected = acc
				case acc.LastUsedAt != nil && selected.LastUsedAt == nil:
					// keep selected (never used is preferred)
				case acc.LastUsedAt == nil && selected.LastUsedAt == nil:
					if preferOAuth && acc.Platform == PlatformGemini && selected.Platform == PlatformGemini && acc.Type != selected.Type && acc.Type == AccountTypeOAuth {
						selected = acc
					}
				default:
					if acc.LastUsedAt.Before(*selected.LastUsedAt) {
						selected = acc
					}
				}
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
			continue
		}
		if acc.Priority < selected.Priority {
			selected = acc
		} else if acc.Priority == selected.Priority {
			switch {
			case acc.LastUsedAt == nil && selected.LastUsedAt != nil:
				selected = acc
			case acc.LastUsedAt != nil && selected.LastUsedAt == nil:
				// keep selected (never used is preferred)
			case acc.LastUsedAt == nil && selected.LastUsedAt == nil:
				if preferOAuth && acc.Platform == PlatformGemini && selected.Platform == PlatformGemini && acc.Type != selected.Type && acc.Type == AccountTypeOAuth {
					selected = acc
				}
			default:
				if acc.LastUsedAt.Before(*selected.LastUsedAt) {
					selected = acc
				}
			}
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

type selectionFailureStats struct {
	Total              int
	Eligible           int
	Excluded           int
	Unschedulable      int
	PlatformFiltered   int
	ModelUnsupported   int
	ModelRateLimited   int
	SamplePlatformIDs  []int64
	SampleMappingIDs   []int64
	SampleRateLimitIDs []string
}

type selectionFailureDiagnosis struct {
	Category string
	Detail   string
}

func (s *GatewayService) logDetailedSelectionFailure(
	ctx context.Context,
	groupID *int64,
	sessionHash string,
	requestedModel string,
	platform string,
	accounts []Account,
	excludedIDs map[int64]struct{},
	allowMixedScheduling bool,
) selectionFailureStats {
	stats := s.collectSelectionFailureStats(ctx, accounts, requestedModel, platform, excludedIDs, allowMixedScheduling)
	logger.LegacyPrintf(
		"service.gateway",
		"[SelectAccountDetailed] group_id=%v model=%s platform=%s session=%s total=%d eligible=%d excluded=%d unschedulable=%d platform_filtered=%d model_unsupported=%d model_rate_limited=%d sample_platform_filtered=%v sample_model_unsupported=%v sample_model_rate_limited=%v",
		derefGroupID(groupID),
		requestedModel,
		platform,
		shortSessionHash(sessionHash),
		stats.Total,
		stats.Eligible,
		stats.Excluded,
		stats.Unschedulable,
		stats.PlatformFiltered,
		stats.ModelUnsupported,
		stats.ModelRateLimited,
		stats.SamplePlatformIDs,
		stats.SampleMappingIDs,
		stats.SampleRateLimitIDs,
	)
	return stats
}

func (s *GatewayService) collectSelectionFailureStats(
	ctx context.Context,
	accounts []Account,
	requestedModel string,
	platform string,
	excludedIDs map[int64]struct{},
	allowMixedScheduling bool,
) selectionFailureStats {
	stats := selectionFailureStats{
		Total: len(accounts),
	}

	for i := range accounts {
		acc := &accounts[i]
		diagnosis := s.diagnoseSelectionFailure(ctx, acc, requestedModel, platform, excludedIDs, allowMixedScheduling)
		switch diagnosis.Category {
		case "excluded":
			stats.Excluded++
		case "unschedulable":
			stats.Unschedulable++
		case "platform_filtered":
			stats.PlatformFiltered++
			stats.SamplePlatformIDs = appendSelectionFailureSampleID(stats.SamplePlatformIDs, acc.ID)
		case "model_unsupported":
			stats.ModelUnsupported++
			stats.SampleMappingIDs = appendSelectionFailureSampleID(stats.SampleMappingIDs, acc.ID)
		case "model_rate_limited":
			stats.ModelRateLimited++
			remaining := acc.GetRateLimitRemainingTimeWithContext(ctx, requestedModel).Truncate(time.Second)
			stats.SampleRateLimitIDs = appendSelectionFailureRateSample(stats.SampleRateLimitIDs, acc.ID, remaining)
		default:
			stats.Eligible++
		}
	}

	return stats
}

func (s *GatewayService) diagnoseSelectionFailure(
	ctx context.Context,
	acc *Account,
	requestedModel string,
	platform string,
	excludedIDs map[int64]struct{},
	allowMixedScheduling bool,
) selectionFailureDiagnosis {
	if acc == nil {
		return selectionFailureDiagnosis{Category: "unschedulable", Detail: "account_nil"}
	}
	if _, excluded := excludedIDs[acc.ID]; excluded {
		return selectionFailureDiagnosis{Category: "excluded"}
	}
	if !s.isAccountSchedulableForSelection(acc) {
		return selectionFailureDiagnosis{Category: "unschedulable", Detail: "generic_unschedulable"}
	}
	if isPlatformFilteredForSelection(acc, platform, allowMixedScheduling) {
		return selectionFailureDiagnosis{
			Category: "platform_filtered",
			Detail:   fmt.Sprintf("account_platform=%s requested_platform=%s", acc.Platform, strings.TrimSpace(platform)),
		}
	}
	if requestedModel != "" && !s.isModelSupportedByAccountWithContext(ctx, acc, requestedModel) {
		return selectionFailureDiagnosis{
			Category: "model_unsupported",
			Detail:   fmt.Sprintf("model=%s", requestedModel),
		}
	}
	if !s.isAccountSchedulableForModelSelection(ctx, acc, requestedModel) {
		remaining := acc.GetRateLimitRemainingTimeWithContext(ctx, requestedModel).Truncate(time.Second)
		return selectionFailureDiagnosis{
			Category: "model_rate_limited",
			Detail:   fmt.Sprintf("remaining=%s", remaining),
		}
	}
	return selectionFailureDiagnosis{Category: "eligible"}
}

func isPlatformFilteredForSelection(acc *Account, platform string, allowMixedScheduling bool) bool {
	if acc == nil {
		return true
	}
	if allowMixedScheduling {
		if acc.Platform == PlatformAntigravity {
			return !acc.IsMixedSchedulingEnabled()
		}
		return acc.Platform != platform
	}
	if strings.TrimSpace(platform) == "" {
		return false
	}
	return acc.Platform != platform
}

func appendSelectionFailureSampleID(samples []int64, id int64) []int64 {
	const limit = 5
	if len(samples) >= limit {
		return samples
	}
	return append(samples, id)
}

func appendSelectionFailureRateSample(samples []string, accountID int64, remaining time.Duration) []string {
	const limit = 5
	if len(samples) >= limit {
		return samples
	}
	return append(samples, fmt.Sprintf("%d(%s)", accountID, remaining))
}

func summarizeSelectionFailureStats(stats selectionFailureStats) string {
	return fmt.Sprintf(
		"total=%d eligible=%d excluded=%d unschedulable=%d platform_filtered=%d model_unsupported=%d model_rate_limited=%d",
		stats.Total,
		stats.Eligible,
		stats.Excluded,
		stats.Unschedulable,
		stats.PlatformFiltered,
		stats.ModelUnsupported,
		stats.ModelRateLimited,
	)
}

// isModelSupportedByAccountWithContext 根据账户平台检查模型支持（带 context）
// 对于 Antigravity 平台，会先获取映射后的最终模型名（包括 thinking 后缀）再检查支持
func (s *GatewayService) isModelSupportedByAccountWithContext(ctx context.Context, account *Account, requestedModel string) bool {
	if account.Platform == PlatformAntigravity {
		if strings.TrimSpace(requestedModel) == "" {
			return true
		}
		// 使用与转发阶段一致的映射逻辑：自定义映射优先 → 默认映射兜底
		mapped := mapAntigravityModel(account, requestedModel)
		if mapped == "" {
			return false
		}
		// 应用 thinking 后缀后检查最终模型是否在账号映射中
		if enabled, ok := ThinkingEnabledFromContext(ctx); ok {
			finalModel := applyThinkingModelSuffix(mapped, enabled)
			if finalModel == mapped {
				return true // thinking 后缀未改变模型名，映射已通过
			}
			return account.IsModelSupported(finalModel)
		}
		return true
	}
	return s.isModelSupportedByAccount(account, requestedModel)
}

// isModelSupportedByAccount 根据账户平台检查模型支持（无 context，用于非 Antigravity 平台）
func (s *GatewayService) isModelSupportedByAccount(account *Account, requestedModel string) bool {
	if account.Platform == PlatformAntigravity {
		if strings.TrimSpace(requestedModel) == "" {
			return true
		}
		return mapAntigravityModel(account, requestedModel) != ""
	}
	if account.IsBedrock() {
		_, ok := ResolveBedrockModelID(account, requestedModel)
		return ok
	}
	// OpenAI 透传模式：仅替换认证，允许所有模型
	if account.Platform == PlatformOpenAI && account.IsOpenAIPassthroughEnabled() {
		return true
	}
	// OAuth/SetupToken 账号使用 Anthropic 标准映射（短ID → 长ID）
	if account.Platform == PlatformAnthropic && account.Type != AccountTypeAPIKey {
		if account.Type == AccountTypeServiceAccount {
			requestedModel = normalizeVertexAnthropicModelID(claude.NormalizeModelID(requestedModel))
		} else {
			requestedModel = claude.NormalizeModelID(requestedModel)
		}
	}
	// 其他平台使用账户的模型支持检查
	return account.IsModelSupported(requestedModel)
}
