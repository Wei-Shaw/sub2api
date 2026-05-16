package service

import (
	"context"
	"fmt"
	"log/slog"
	"sort"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

// ─────────────────────────────────────────────────────────────────────────────
// 账号调度子方法：将 SelectAccountWithLoadAwareness 的内部逻辑分层组织
// ─────────────────────────────────────────────────────────────────────────────

// schedulingContext 封装账号调度所需的共享上下文。
// 在 prepareSchedulingContext 中一次性初始化，避免子方法间传递大量参数。
type schedulingContext struct {
	cfg             config.GatewaySchedulingConfig // 调度配置（超时、队列限制等）
	group           *Group                         // 用户所属分组
	groupID         *int64                         // 分组ID指针（可能被 Claude Code 限制修改）
	stickyAccountID int64                          // 粘性绑定的账号ID（0 表示无绑定）
	stickySource    string                         // 粘性来源: "prefetch" | "cache" | ""
	requestedModel  string                         // 请求的模型名称
	sessionHash     string                         // 会话哈希（用于粘性绑定）
	excludedIDs     map[int64]struct{}             // 已排除的账号集合（failover 后累加）
	metadataUserID  string                         // 客户端元数据用户ID
	sub2apiUserID   int64                          // 系统用户ID
}

// accountPool 缓存可调度账号列表和快速查找结构。
// 在 loadSchedulableAccountPool 中构建，供后续各层选择使用。
type accountPool struct {
	accounts    []Account          // 可调度账号完整列表
	byID        map[int64]*Account // 按ID快速查找
	useMixed    bool               // 是否混合调度模式（跨平台）
	platform    string             // 目标平台 (anthropic/openai/gemini)
	preferOAuth bool               // 是否优先选择 OAuth 账号
	routingIDs  []int64            // 模型路由指定的账号ID列表
	isExcluded  func(int64) bool   // 排除判断闭包
}

// prepareSchedulingContext 初始化调度上下文。
// 包括：加载配置、检查 Claude Code 限制、检查渠道定价限制、解析粘性绑定。
func (s *GatewayService) prepareSchedulingContext(
	ctx context.Context,
	groupID *int64,
	sessionHash string,
	requestedModel string,
	excludedIDs map[int64]struct{},
	metadataUserID string,
	sub2apiUserID int64,
) (context.Context, *schedulingContext, error) {
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

	// 检查 Claude Code 客户端限制（可能替换 groupID 为降级分组）
	group, groupID, err := s.checkClaudeCodeRestriction(ctx, groupID)
	if err != nil {
		return ctx, nil, err
	}
	ctx = s.withGroupContext(ctx, group)

	// 渠道定价限制预检查（必须使用解析后的分组）
	if s.checkChannelPricingRestriction(ctx, groupID, requestedModel) {
		slog.Warn("channel pricing restriction blocked request",
			"group_id", derefGroupID(groupID),
			"model", requestedModel)
		return ctx, nil, fmt.Errorf("%w supporting model: %s (channel pricing restriction)", ErrNoAvailableAccounts, requestedModel)
	}

	// 解析粘性绑定（优先从 context prefetch，其次从 Redis 缓存）
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

	sc := &schedulingContext{
		cfg:             cfg,
		group:           group,
		groupID:         groupID,
		stickyAccountID: stickyAccountID,
		stickySource:    stickySource,
		requestedModel:  requestedModel,
		sessionHash:     sessionHash,
		excludedIDs:     excludedIDs,
		metadataUserID:  metadataUserID,
		sub2apiUserID:   sub2apiUserID,
	}
	return ctx, sc, nil
}

// selectWithoutLoadAwareness 非负载感知的快速选择路径。
// 当 concurrencyService 为 nil 或 LoadBatchEnabled=false 时使用。
func (s *GatewayService) selectWithoutLoadAwareness(ctx context.Context, sc *schedulingContext) (*AccountSelectionResult, error) {
	localExcluded := make(map[int64]struct{})
	for k, v := range sc.excludedIDs {
		localExcluded[k] = v
	}

	for {
		account, err := s.SelectAccountForModelWithExclusions(ctx, sc.groupID, sc.sessionHash, sc.requestedModel, localExcluded)
		if err != nil {
			return nil, err
		}

		result, err := s.tryAcquireAccountSlot(ctx, account.ID, account.Concurrency)
		if err == nil && result.Acquired {
			if !s.checkAndRegisterSession(ctx, account, sc.sessionHash) {
				result.ReleaseFunc()
				localExcluded[account.ID] = struct{}{}
				continue
			}
			return s.newSelectionResult(ctx, account, true, result.ReleaseFunc, nil)
		}

		if !s.checkAndRegisterSession(ctx, account, sc.sessionHash) {
			localExcluded[account.ID] = struct{}{}
			continue
		}

		// 粘性账号优先使用粘性等待超时
		if sc.stickyAccountID > 0 && sc.stickyAccountID == account.ID && s.concurrencyService != nil {
			waitingCount, _ := s.concurrencyService.GetAccountWaitingCount(ctx, account.ID)
			if waitingCount < sc.cfg.StickySessionMaxWaiting {
				return s.newSelectionResult(ctx, account, false, nil, &AccountWaitPlan{
					AccountID:      account.ID,
					MaxConcurrency: account.Concurrency,
					Timeout:        sc.cfg.StickySessionWaitTimeout,
					MaxWaiting:     sc.cfg.StickySessionMaxWaiting,
				})
			}
		}
		return s.newSelectionResult(ctx, account, false, nil, &AccountWaitPlan{
			AccountID:      account.ID,
			MaxConcurrency: account.Concurrency,
			Timeout:        sc.cfg.FallbackWaitTimeout,
			MaxWaiting:     sc.cfg.FallbackMaxWaiting,
		})
	}
}

// loadSchedulableAccountPool 加载可调度账号列表并构建查找结构。
func (s *GatewayService) loadSchedulableAccountPool(ctx context.Context, sc *schedulingContext) (context.Context, *accountPool, error) {
	platform, hasForcePlatform, err := s.resolvePlatform(ctx, sc.groupID, sc.group)
	if err != nil {
		return ctx, nil, err
	}
	preferOAuth := platform == PlatformGemini

	if s.debugModelRoutingEnabled() && platform == PlatformAnthropic && sc.requestedModel != "" {
		logger.LegacyPrintf("service.gateway", "[ModelRoutingDebug] load-aware enabled: group_id=%v model=%s session=%s platform=%s",
			derefGroupID(sc.groupID), sc.requestedModel, shortSessionHash(sc.sessionHash), platform)
	}

	accounts, useMixed, err := s.listSchedulableAccounts(ctx, sc.groupID, platform, hasForcePlatform)
	if err != nil {
		return ctx, nil, err
	}
	if len(accounts) == 0 {
		return ctx, nil, ErrNoAvailableAccounts
	}

	// 预取窗口费用和 RPM 数据（批量 Redis 查询，避免 N+1）
	ctx = s.withWindowCostPrefetch(ctx, accounts)
	ctx = s.withRPMPrefetch(ctx, accounts)

	byID := make(map[int64]*Account, len(accounts))
	for i := range accounts {
		byID[accounts[i].ID] = &accounts[i]
	}

	isExcluded := func(accountID int64) bool {
		if sc.excludedIDs == nil {
			return false
		}
		_, excluded := sc.excludedIDs[accountID]
		return excluded
	}

	// 获取模型路由配置（仅 anthropic 平台有效）
	var routingIDs []int64
	if sc.group != nil && sc.requestedModel != "" && sc.group.Platform == PlatformAnthropic {
		routingIDs = sc.group.GetRoutingAccountIDs(sc.requestedModel)
		if s.debugModelRoutingEnabled() {
			logger.LegacyPrintf("service.gateway", "[ModelRoutingDebug] context group routing: group_id=%d model=%s enabled=%v rules=%d matched_ids=%v session=%s sticky_account=%d",
				sc.group.ID, sc.requestedModel, sc.group.ModelRoutingEnabled, len(sc.group.ModelRouting), routingIDs, shortSessionHash(sc.sessionHash), sc.stickyAccountID)
			if len(routingIDs) == 0 && sc.group.ModelRoutingEnabled && len(sc.group.ModelRouting) > 0 {
				keys := make([]string, 0, len(sc.group.ModelRouting))
				for k := range sc.group.ModelRouting {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				const maxKeys = 20
				if len(keys) > maxKeys {
					keys = keys[:maxKeys]
				}
				logger.LegacyPrintf("service.gateway", "[ModelRoutingDebug] context group routing miss: group_id=%d model=%s patterns(sample)=%v", sc.group.ID, sc.requestedModel, keys)
			}
		}
	}

	pool := &accountPool{
		accounts:    accounts,
		byID:        byID,
		useMixed:    useMixed,
		platform:    platform,
		preferOAuth: preferOAuth,
		routingIDs:  routingIDs,
		isExcluded:  isExcluded,
	}
	return ctx, pool, nil
}

// tryModelRoutingSelection 模型路由层选择（Layer 1）。
// 当分组配置了模型路由规则时，优先从指定账号列表中选择。
// 返回 nil 表示本层未命中，继续下一层。
func (s *GatewayService) tryModelRoutingSelection(ctx context.Context, sc *schedulingContext, pool *accountPool) (*AccountSelectionResult, error) {
	if len(pool.routingIDs) == 0 || s.concurrencyService == nil {
		return nil, nil
	}

	// 过滤出路由列表中可调度的账号（保留逐项计数器用于调试诊断）
	var routingCandidates []*Account
	var filteredExcluded, filteredMissing, filteredUnsched, filteredPlatform, filteredModelScope, filteredModelMapping, filteredWindowCost int
	var modelScopeSkippedIDs []int64

	for _, routingID := range pool.routingIDs {
		if pool.isExcluded(routingID) {
			filteredExcluded++
			continue
		}
		account, ok := pool.byID[routingID]
		if !ok {
			filteredMissing++
			continue
		}
		if !s.isAccountSchedulableForSelection(account) {
			filteredUnsched++
			continue
		}
		if !s.isAccountAllowedForPlatform(account, pool.platform, pool.useMixed) {
			filteredPlatform++
			continue
		}
		if sc.requestedModel != "" && !s.isModelSupportedByAccountWithContext(ctx, account, sc.requestedModel) {
			filteredModelMapping++
			continue
		}
		if !s.isAccountSchedulableForModelSelection(ctx, account, sc.requestedModel) {
			filteredModelScope++
			modelScopeSkippedIDs = append(modelScopeSkippedIDs, account.ID)
			continue
		}
		if !s.isAccountSchedulableForQuota(account) {
			continue
		}
		if !s.isAccountSchedulableForWindowCost(ctx, account, false) {
			filteredWindowCost++
			continue
		}
		if !s.isAccountSchedulableForRPM(ctx, account, false) {
			continue
		}
		routingCandidates = append(routingCandidates, account)
	}

	if s.debugModelRoutingEnabled() {
		logger.LegacyPrintf("service.gateway", "[ModelRoutingDebug] routed candidates: group_id=%v model=%s routed=%d candidates=%d filtered(excluded=%d missing=%d unsched=%d platform=%d model_scope=%d model_mapping=%d window_cost=%d)",
			derefGroupID(sc.groupID), sc.requestedModel, len(pool.routingIDs), len(routingCandidates),
			filteredExcluded, filteredMissing, filteredUnsched, filteredPlatform, filteredModelScope, filteredModelMapping, filteredWindowCost)
		if len(modelScopeSkippedIDs) > 0 {
			logger.LegacyPrintf("service.gateway", "[ModelRoutingDebug] model_rate_limited accounts skipped: group_id=%v model=%s account_ids=%v",
				derefGroupID(sc.groupID), sc.requestedModel, modelScopeSkippedIDs)
		}
	}

	if len(routingCandidates) == 0 {
		logger.LegacyPrintf("service.gateway", "[ModelRouting] All routed accounts unavailable for model=%s, falling back to normal selection", sc.requestedModel)
		return nil, nil
	}

	// Layer 1.5: 在路由范围内检查粘性会话
	if result, err := s.tryStickyWithinRouting(ctx, sc, pool, routingCandidates); result != nil || err != nil {
		return result, err
	}

	// 批量获取负载信息
	routingLoads := make([]AccountWithConcurrency, 0, len(routingCandidates))
	for _, acc := range routingCandidates {
		routingLoads = append(routingLoads, AccountWithConcurrency{
			ID:             acc.ID,
			MaxConcurrency: acc.EffectiveLoadFactor(),
		})
	}
	routingLoadMap, _ := s.concurrencyService.GetAccountsLoadBatch(ctx, routingLoads)

	// 筛选负载率 < 100% 的账号
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

	if len(routingAvailable) == 0 {
		logger.LegacyPrintf("service.gateway", "[ModelRouting] All routed accounts unavailable for model=%s, falling back to normal selection", sc.requestedModel)
		return nil, nil
	}

	sortAccountsWithLoadByPriority(routingAvailable)

	// 尝试获取槽位
	for _, item := range routingAvailable {
		result, err := s.tryAcquireAccountSlot(ctx, item.account.ID, item.account.Concurrency)
		if err == nil && result.Acquired {
			if !s.checkAndRegisterSession(ctx, item.account, sc.sessionHash) {
				result.ReleaseFunc()
				continue
			}
			if sc.sessionHash != "" && s.cache != nil {
				_ = s.cache.SetSessionAccountID(ctx, derefGroupID(sc.groupID), sc.sessionHash, item.account.ID, stickySessionTTL)
			}
			if s.debugModelRoutingEnabled() {
				logger.LegacyPrintf("service.gateway", "[ModelRoutingDebug] routed select: group_id=%v model=%s session=%s account=%d", derefGroupID(sc.groupID), sc.requestedModel, shortSessionHash(sc.sessionHash), item.account.ID)
			}
			return s.newSelectionResult(ctx, item.account, true, result.ReleaseFunc, nil)
		}
	}

	// 所有路由账号槽位满，返回等待计划（选择第一个通过会话限制的）
	for _, item := range routingAvailable {
		if !s.checkAndRegisterSession(ctx, item.account, sc.sessionHash) {
			continue
		}
		if s.debugModelRoutingEnabled() {
			logger.LegacyPrintf("service.gateway", "[ModelRoutingDebug] routed wait: group_id=%v model=%s session=%s account=%d", derefGroupID(sc.groupID), sc.requestedModel, shortSessionHash(sc.sessionHash), item.account.ID)
		}
		return s.newSelectionResult(ctx, item.account, false, nil, &AccountWaitPlan{
			AccountID:      item.account.ID,
			MaxConcurrency: item.account.Concurrency,
			Timeout:        sc.cfg.StickySessionWaitTimeout,
			MaxWaiting:     sc.cfg.StickySessionMaxWaiting,
		})
	}
	// 所有路由账号会话限制都已满，继续到 Layer 2
	return nil, nil
}

// tryStickyWithinRouting 在模型路由范围内尝试粘性会话复用。
// 当粘性账号恰好在路由列表中时，优先复用以保持 prompt cache 命中率。
func (s *GatewayService) tryStickyWithinRouting(ctx context.Context, sc *schedulingContext, pool *accountPool, routingCandidates []*Account) (*AccountSelectionResult, error) {
	if sc.sessionHash == "" || sc.stickyAccountID <= 0 {
		return nil, nil
	}

	slog.Debug("sticky.layer1_5_checking",
		"sticky_account_id", sc.stickyAccountID,
		"in_routing_list", containsInt64(pool.routingIDs, sc.stickyAccountID),
		"is_excluded", pool.isExcluded(sc.stickyAccountID),
		"in_account_map", func() bool { _, ok := pool.byID[sc.stickyAccountID]; return ok }(),
		"session", shortSessionHash(sc.sessionHash),
	)

	if !containsInt64(pool.routingIDs, sc.stickyAccountID) || pool.isExcluded(sc.stickyAccountID) {
		return nil, nil
	}

	stickyAccount, ok := pool.byID[sc.stickyAccountID]
	if !ok {
		_ = s.cache.DeleteSessionAccountID(ctx, derefGroupID(sc.groupID), sc.sessionHash)
		logger.LegacyPrintf("service.gateway", "[StickyCacheMiss] reason=account_cleared account_id=%d session=%s current_rpm=0 base_rpm=0",
			sc.stickyAccountID, shortSessionHash(sc.sessionHash))
		return nil, nil
	}

	var missReason string

	// 粘性路径使用完整资格检查（isSticky=true 放宽 WindowCost/RPM 阈值）
	gatePass := s.isAccountEligibleForScheduling(ctx, stickyAccount, eligibilityOpts{
		platform:       pool.platform,
		useMixed:       pool.useMixed,
		requestedModel: sc.requestedModel,
		isSticky:       true,
	})
	rpmPass := gatePass // RPM 已包含在 isAccountEligibleForScheduling 中

	if rpmPass {
		result, err := s.tryAcquireAccountSlot(ctx, sc.stickyAccountID, stickyAccount.Concurrency)
		if err == nil && result.Acquired {
			if !s.checkAndRegisterSession(ctx, stickyAccount, sc.sessionHash) {
				result.ReleaseFunc()
				missReason = "session_limit"
			} else {
				slog.Debug("sticky.layer1_5_hit",
					"account_id", sc.stickyAccountID,
					"session", shortSessionHash(sc.sessionHash),
					"result", "slot_acquired",
				)
				if s.debugModelRoutingEnabled() {
					logger.LegacyPrintf("service.gateway", "[ModelRoutingDebug] routed sticky hit: group_id=%v model=%s session=%s account=%d", derefGroupID(sc.groupID), sc.requestedModel, shortSessionHash(sc.sessionHash), sc.stickyAccountID)
				}
				return s.newSelectionResult(ctx, stickyAccount, true, result.ReleaseFunc, nil)
			}
		}

		if missReason == "" {
			waitingCount, _ := s.concurrencyService.GetAccountWaitingCount(ctx, sc.stickyAccountID)
			if waitingCount < sc.cfg.StickySessionMaxWaiting {
				if !s.checkAndRegisterSession(ctx, stickyAccount, sc.sessionHash) {
					missReason = "session_limit"
				} else {
					return &AccountSelectionResult{
						Account: stickyAccount,
						WaitPlan: &AccountWaitPlan{
							AccountID:      sc.stickyAccountID,
							MaxConcurrency: stickyAccount.Concurrency,
							Timeout:        sc.cfg.StickySessionWaitTimeout,
							MaxWaiting:     sc.cfg.StickySessionMaxWaiting,
						},
					}, nil
				}
			} else {
				missReason = "wait_queue_full"
			}
		}
	} else if !gatePass {
		missReason = "gate_check"
	} else {
		missReason = "rpm_red"
	}

	if missReason != "" {
		baseRPM := stickyAccount.GetBaseRPM()
		var currentRPM int
		if count, ok := rpmFromPrefetchContext(ctx, stickyAccount.ID); ok {
			currentRPM = count
		}
		logger.LegacyPrintf("service.gateway", "[StickyCacheMiss] reason=%s account_id=%d session=%s current_rpm=%d base_rpm=%d",
			missReason, sc.stickyAccountID, shortSessionHash(sc.sessionHash), currentRPM, baseRPM)
	}

	return nil, nil
}

// tryStickySessionSelection 粘性会话层选择（Layer 1.5）。
// 仅在无模型路由配置时生效，尝试复用上次绑定的账号。
// 返回 nil 表示本层未命中。
func (s *GatewayService) tryStickySessionSelection(ctx context.Context, sc *schedulingContext, pool *accountPool) (*AccountSelectionResult, error) {
	// 仅在无模型路由配置时执行
	if len(pool.routingIDs) > 0 {
		return nil, nil
	}

	if sc.sessionHash == "" || sc.stickyAccountID <= 0 || pool.isExcluded(sc.stickyAccountID) {
		if sc.sessionHash != "" {
			slog.Debug("sticky.layer1_5_no_routing_skip",
				"sticky_account_id", sc.stickyAccountID,
				"is_excluded", func() bool { return sc.stickyAccountID > 0 && pool.isExcluded(sc.stickyAccountID) }(),
				"session", shortSessionHash(sc.sessionHash),
				"reason", func() string {
					if sc.stickyAccountID == 0 {
						return "no_sticky_binding"
					}
					return "sticky_account_excluded"
				}(),
			)
		}
		return nil, nil
	}

	account, ok := pool.byID[sc.stickyAccountID]
	if !ok {
		slog.Debug("sticky.layer1_5_no_routing_miss",
			"account_id", sc.stickyAccountID,
			"reason", "account_not_in_map",
			"session", shortSessionHash(sc.sessionHash),
		)
		return nil, nil
	}

	// 检查账号是否需要清理粘性会话绑定
	clearSticky := shouldClearStickySession(account, sc.requestedModel)
	if clearSticky {
		slog.Debug("sticky.layer1_5_no_routing_clear",
			"account_id", sc.stickyAccountID,
			"reason", "should_clear_sticky_session",
			"session", shortSessionHash(sc.sessionHash),
		)
		_ = s.cache.DeleteSessionAccountID(ctx, derefGroupID(sc.groupID), sc.sessionHash)
	}

	// 完整门控检查
	platformOK := s.isAccountAllowedForPlatform(account, pool.platform, pool.useMixed)
	modelSupported := sc.requestedModel == "" || s.isModelSupportedByAccountWithContext(ctx, account, sc.requestedModel)
	modelSchedulable := s.isAccountSchedulableForModelSelection(ctx, account, sc.requestedModel)
	quotaOK := s.isAccountSchedulableForQuota(account)
	windowCostOK := s.isAccountSchedulableForWindowCost(ctx, account, true)
	rpmOK := s.isAccountSchedulableForRPM(ctx, account, true)
	schedulable := s.isAccountSchedulableForSelection(account)

	slog.Debug("sticky.layer1_5_no_routing_checks",
		"account_id", sc.stickyAccountID,
		"session", shortSessionHash(sc.sessionHash),
		"clear_sticky", clearSticky,
		"schedulable", schedulable,
		"platform_ok", platformOK,
		"model_supported", modelSupported,
		"model_schedulable", modelSchedulable,
		"quota_ok", quotaOK,
		"window_cost_ok", windowCostOK,
		"rpm_ok", rpmOK,
	)

	if clearSticky || !platformOK || !modelSupported || !modelSchedulable || !quotaOK || !windowCostOK || !rpmOK || !schedulable {
		if !clearSticky {
			slog.Debug("sticky.layer1_5_no_routing_miss",
				"account_id", sc.stickyAccountID,
				"reason", "gate_check_failed",
				"session", shortSessionHash(sc.sessionHash),
			)
		}
		return nil, nil
	}

	// 尝试获取槽位
	result, err := s.tryAcquireAccountSlot(ctx, sc.stickyAccountID, account.Concurrency)
	if err == nil && result.Acquired {
		if !s.checkAndRegisterSession(ctx, account, sc.sessionHash) {
			result.ReleaseFunc()
			slog.Debug("sticky.layer1_5_no_routing_miss",
				"account_id", sc.stickyAccountID,
				"reason", "session_limit",
				"session", shortSessionHash(sc.sessionHash),
			)
		} else {
			slog.Debug("sticky.layer1_5_no_routing_hit",
				"account_id", sc.stickyAccountID,
				"session", shortSessionHash(sc.sessionHash),
				"result", "slot_acquired",
			)
			if s.cache != nil {
				_ = s.cache.RefreshSessionTTL(ctx, derefGroupID(sc.groupID), sc.sessionHash, stickySessionTTL)
			}
			return s.newSelectionResult(ctx, account, true, result.ReleaseFunc, nil)
		}
	} else {
		slog.Debug("sticky.layer1_5_no_routing_slot_busy",
			"account_id", sc.stickyAccountID,
			"session", shortSessionHash(sc.sessionHash),
		)
	}

	// 槽位未获取，尝试等待计划
	waitingCount, _ := s.concurrencyService.GetAccountWaitingCount(ctx, sc.stickyAccountID)
	if waitingCount < sc.cfg.StickySessionMaxWaiting {
		if !s.checkAndRegisterSession(ctx, account, sc.sessionHash) {
			return nil, nil
		}
		slog.Debug("sticky.layer1_5_no_routing_hit",
			"account_id", sc.stickyAccountID,
			"session", shortSessionHash(sc.sessionHash),
			"result", "wait_plan",
		)
		return s.newSelectionResult(ctx, account, false, nil, &AccountWaitPlan{
			AccountID:      sc.stickyAccountID,
			MaxConcurrency: account.Concurrency,
			Timeout:        sc.cfg.StickySessionWaitTimeout,
			MaxWaiting:     sc.cfg.StickySessionMaxWaiting,
		})
	}

	return nil, nil
}

// selectByLoadBalance 负载均衡层选择（Layer 2）。
// 从所有可调度账号中按负载率选择最优账号。
// 返回 nil 表示所有账号满载，继续到 Layer 3 排队。
func (s *GatewayService) selectByLoadBalance(ctx context.Context, sc *schedulingContext, pool *accountPool) (*AccountSelectionResult, error) {
	slog.Debug("sticky.layer2_fallback",
		"session", shortSessionHash(sc.sessionHash),
		"sticky_account_id", sc.stickyAccountID,
		"reason", "sticky_not_used_falling_back_to_load_balance",
		"total_accounts", len(pool.accounts),
	)

	// 构建候选列表
	opts := eligibilityOpts{
		platform:       pool.platform,
		useMixed:       pool.useMixed,
		requestedModel: sc.requestedModel,
		isSticky:       false,
	}
	candidates := make([]*Account, 0, len(pool.accounts))
	for i := range pool.accounts {
		acc := &pool.accounts[i]
		if pool.isExcluded(acc.ID) {
			continue
		}
		if !s.isAccountEligibleForScheduling(ctx, acc, opts) {
			continue
		}
		candidates = append(candidates, acc)
	}

	if len(candidates) == 0 {
		return nil, nil
	}

	// 批量获取负载信息
	accountLoads := make([]AccountWithConcurrency, 0, len(candidates))
	for _, acc := range candidates {
		accountLoads = append(accountLoads, AccountWithConcurrency{
			ID:             acc.ID,
			MaxConcurrency: acc.EffectiveLoadFactor(),
		})
	}

	loadMap, err := s.concurrencyService.GetAccountsLoadBatch(ctx, accountLoads)
	if err != nil {
		// 负载查询失败时降级到传统排序
		if result, ok, legacyErr := s.tryAcquireByLegacyOrder(ctx, candidates, sc.groupID, sc.sessionHash, pool.preferOAuth); legacyErr != nil {
			return nil, legacyErr
		} else if ok {
			return result, nil
		}
		return nil, nil
	}

	var available []accountWithLoad
	for _, acc := range candidates {
		loadInfo := loadMap[acc.ID]
		if loadInfo == nil {
			loadInfo = &AccountLoadInfo{AccountID: acc.ID}
		}
		if loadInfo.LoadRate < 100 {
			available = append(available, accountWithLoad{account: acc, loadInfo: loadInfo})
		}
	}

	// 分层过滤选择：优先级 → 负载率 → LRU
	for len(available) > 0 {
		filtered := filterByMinPriority(available)
		filtered = filterByMinLoadRate(filtered)
		selected := selectByLRU(filtered, pool.preferOAuth)
		if selected == nil {
			break
		}

		result, err := s.tryAcquireAccountSlot(ctx, selected.account.ID, selected.account.Concurrency)
		if err == nil && result.Acquired {
			if !s.checkAndRegisterSession(ctx, selected.account, sc.sessionHash) {
				result.ReleaseFunc()
			} else {
				if sc.sessionHash != "" && s.cache != nil {
					_ = s.cache.SetSessionAccountID(ctx, derefGroupID(sc.groupID), sc.sessionHash, selected.account.ID, stickySessionTTL)
				}
				return s.newSelectionResult(ctx, selected.account, true, result.ReleaseFunc, nil)
			}
		}

		// 移除已尝试的账号
		selectedID := selected.account.ID
		newAvailable := make([]accountWithLoad, 0, len(available)-1)
		for _, acc := range available {
			if acc.account.ID != selectedID {
				newAvailable = append(newAvailable, acc)
			}
		}
		available = newAvailable
	}

	return nil, nil
}

// selectWithQueueFallback 兜底排队选择（Layer 3）。
// 当所有账号负载率 >= 100% 时，选择一个账号进入等待队列。
func (s *GatewayService) selectWithQueueFallback(ctx context.Context, sc *schedulingContext, pool *accountPool) (*AccountSelectionResult, error) {
	candidates := make([]*Account, 0, len(pool.accounts))
	for i := range pool.accounts {
		acc := &pool.accounts[i]
		if pool.isExcluded(acc.ID) {
			continue
		}
		if !s.isAccountSchedulableForSelection(acc) {
			continue
		}
		candidates = append(candidates, acc)
	}

	s.sortCandidatesForFallback(candidates, pool.preferOAuth, sc.cfg.FallbackSelectionMode)
	for _, acc := range candidates {
		if !s.checkAndRegisterSession(ctx, acc, sc.sessionHash) {
			continue
		}
		return s.newSelectionResult(ctx, acc, false, nil, &AccountWaitPlan{
			AccountID:      acc.ID,
			MaxConcurrency: acc.Concurrency,
			Timeout:        sc.cfg.FallbackWaitTimeout,
			MaxWaiting:     sc.cfg.FallbackMaxWaiting,
		})
	}
	return nil, ErrNoAvailableAccounts
}
