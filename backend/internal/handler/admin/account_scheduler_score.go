package admin

import (
	"context"
	"log/slog"
	"sort"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (h *AccountHandler) openAIAccountSchedulerScoresForList(
	ctx context.Context,
	accounts []service.Account,
	platform string,
	accountType string,
	status string,
	search string,
	groupID int64,
	privacyMode string,
) (map[int64]*AccountSchedulerScore, map[int64][]AccountSchedulerGroupScore) {
	hasOpenAIAccount := false
	for i := range accounts {
		if accounts[i].Platform == service.PlatformOpenAI {
			hasOpenAIAccount = true
			break
		}
	}
	if !hasOpenAIAccount {
		return nil, nil
	}
	filterPool := h.listAccountSchedulerScoreFilterPool(ctx, platform, accountType, status, search, groupID, privacyMode)
	return h.buildOpenAIAccountSchedulerScores(ctx, accounts, filterPool)
}

func (h *AccountHandler) scoreOpenAIAccountSchedulerPool(
	ctx context.Context,
	accounts []service.Account,
	loadMap map[int64]*service.AccountLoadInfo,
) map[int64]AccountSchedulerScore {
	if len(accounts) == 0 {
		return nil
	}

	openAIAccounts := make([]*service.Account, 0, len(accounts))
	for i := range accounts {
		account := &accounts[i]
		if account.Platform == service.PlatformOpenAI {
			openAIAccounts = append(openAIAccounts, account)
		}
	}
	if len(openAIAccounts) == 0 {
		return nil
	}

	if loadMap == nil {
		loadMap = h.fetchOpenAIAccountLoadMap(ctx, openAIAccounts)
	}
	var scores map[int64]service.OpenAIAccountSchedulerScoreSnapshot
	if h.rateLimitService != nil {
		scores = h.rateLimitService.BuildOpenAIAccountSchedulerScoreSnapshot(ctx, openAIAccounts, loadMap)
	} else {
		scores = service.BuildOpenAIAccountSchedulerScoreSnapshot(openAIAccounts, loadMap)
	}
	result := make(map[int64]AccountSchedulerScore, len(scores))
	for accountID, score := range scores {
		result[accountID] = AccountSchedulerScore{
			BaseScore:             score.BaseScore,
			StickyScore:           score.StickyScore,
			StickyScoreInfinity:   score.StickyScoreInfinity,
			StickyWeightedEnabled: score.StickyWeightedEnabled,
		}
	}
	return result
}

func (h *AccountHandler) fetchOpenAIAccountLoadMap(
	ctx context.Context,
	openAIAccounts []*service.Account,
) map[int64]*service.AccountLoadInfo {
	loadMap := map[int64]*service.AccountLoadInfo{}
	if h.concurrencyService == nil || len(openAIAccounts) == 0 {
		return loadMap
	}
	seen := make(map[int64]struct{}, len(openAIAccounts))
	loadReq := make([]service.AccountWithConcurrency, 0, len(openAIAccounts))
	for _, account := range openAIAccounts {
		if account == nil {
			continue
		}
		if _, ok := seen[account.ID]; ok {
			continue
		}
		seen[account.ID] = struct{}{}
		loadReq = append(loadReq, service.AccountWithConcurrency{
			ID:             account.ID,
			MaxConcurrency: account.EffectiveLoadFactor(),
		})
	}
	batchLoad, err := h.concurrencyService.GetAccountsLoadBatch(ctx, loadReq)
	if err != nil {
		slog.Warn("openai_scheduler_score_load_batch_failed", "error", err)
		return loadMap
	}
	if batchLoad != nil {
		return batchLoad
	}
	return loadMap
}

func (h *AccountHandler) buildOpenAIAccountSchedulerScores(
	ctx context.Context,
	accounts []service.Account,
	filterPool []service.Account,
) (map[int64]*AccountSchedulerScore, map[int64][]AccountSchedulerGroupScore) {
	if len(accounts) == 0 {
		return nil, nil
	}
	if len(filterPool) == 0 {
		filterPool = accounts
	}

	pageAccountIDs, groupIDs := collectOpenAIPageAccountGroups(accounts)
	if len(pageAccountIDs) == 0 {
		return nil, nil
	}

	groupPools := h.openAIAccountSchedulerGroupPools(ctx, groupIDs)
	loadMap := h.fetchOpenAIAccountLoadMap(ctx, collectOpenAIAccountLoadUnion(filterPool, groupPools))
	baseScores := h.openAIAccountBaseSchedulerScores(ctx, filterPool, loadMap)
	groupScores := h.openAIAccountGroupSchedulerScores(ctx, pageAccountIDs, groupPools, loadMap)
	return baseScores, groupScores
}

func collectOpenAIPageAccountGroups(accounts []service.Account) (map[int64]struct{}, map[int64]struct{}) {
	pageAccountIDs := make(map[int64]struct{})
	groupIDs := make(map[int64]struct{})
	for i := range accounts {
		account := &accounts[i]
		if account.Platform != service.PlatformOpenAI {
			continue
		}
		pageAccountIDs[account.ID] = struct{}{}
		for _, accountGroup := range account.AccountGroups {
			if accountGroup.GroupID > 0 {
				groupIDs[accountGroup.GroupID] = struct{}{}
			}
		}
		for _, groupID := range account.GroupIDs {
			if groupID > 0 {
				groupIDs[groupID] = struct{}{}
			}
		}
	}
	return pageAccountIDs, groupIDs
}

func (h *AccountHandler) openAIAccountSchedulerGroupPools(
	ctx context.Context,
	groupIDs map[int64]struct{},
) map[int64][]service.Account {
	groupIDList := make([]int64, 0, len(groupIDs))
	for groupID := range groupIDs {
		groupIDList = append(groupIDList, groupID)
	}
	sort.Slice(groupIDList, func(i, j int) bool { return groupIDList[i] < groupIDList[j] })
	groupPools := make(map[int64][]service.Account, len(groupIDList))
	if h.adminService == nil {
		return groupPools
	}
	for _, groupID := range groupIDList {
		gid := groupID
		pool, err := h.adminService.ListOpenAISchedulableAccountsForSchedulerScore(ctx, &gid)
		if err != nil {
			slog.Warn("openai_scheduler_group_score_pool_failed", "group_id", gid, "error", err)
			continue
		}
		groupPools[gid] = pool
	}
	return groupPools
}

func collectOpenAIAccountLoadUnion(
	filterPool []service.Account,
	groupPools map[int64][]service.Account,
) []*service.Account {
	loadUnion := make([]*service.Account, 0, len(filterPool))
	collectOpenAIAccounts := func(pool []service.Account) {
		for i := range pool {
			if pool[i].Platform == service.PlatformOpenAI {
				loadUnion = append(loadUnion, &pool[i])
			}
		}
	}
	collectOpenAIAccounts(filterPool)
	for _, pool := range groupPools {
		collectOpenAIAccounts(pool)
	}
	return loadUnion
}

func (h *AccountHandler) openAIAccountBaseSchedulerScores(
	ctx context.Context,
	filterPool []service.Account,
	loadMap map[int64]*service.AccountLoadInfo,
) map[int64]*AccountSchedulerScore {
	baseScores := make(map[int64]*AccountSchedulerScore)
	for accountID, score := range h.scoreOpenAIAccountSchedulerPool(ctx, filterPool, loadMap) {
		copiedScore := score
		baseScores[accountID] = &copiedScore
	}
	return baseScores
}

func (h *AccountHandler) openAIAccountGroupSchedulerScores(
	ctx context.Context,
	pageAccountIDs map[int64]struct{},
	groupPools map[int64][]service.Account,
	loadMap map[int64]*service.AccountLoadInfo,
) map[int64][]AccountSchedulerGroupScore {
	groupScoresByAccount := make(map[int64][]AccountSchedulerGroupScore)
	groupIDList := sortedSchedulerGroupIDs(groupPools)
	for _, groupID := range groupIDList {
		gid := groupID
		pool := groupPools[gid]
		groupNameByID, priorityByAccount := schedulerGroupMetadata(pool, gid)
		scores := h.scoreOpenAIAccountSchedulerPool(ctx, pool, loadMap)
		for accountID, schedulerScore := range scores {
			if _, ok := pageAccountIDs[accountID]; !ok {
				continue
			}
			groupScore := AccountSchedulerGroupScore{
				GroupID:               &gid,
				GroupName:             groupNameByID[gid],
				AccountSchedulerScore: schedulerScore,
			}
			if priority, ok := priorityByAccount[accountID]; ok {
				groupScore.GroupPriority = &priority
			}
			groupScoresByAccount[accountID] = append(groupScoresByAccount[accountID], groupScore)
		}
	}
	return groupScoresByAccount
}

func sortedSchedulerGroupIDs(groupPools map[int64][]service.Account) []int64 {
	groupIDs := make([]int64, 0, len(groupPools))
	for groupID := range groupPools {
		groupIDs = append(groupIDs, groupID)
	}
	sort.Slice(groupIDs, func(i, j int) bool { return groupIDs[i] < groupIDs[j] })
	return groupIDs
}

func schedulerGroupMetadata(pool []service.Account, groupID int64) (map[int64]string, map[int64]int) {
	groupNameByID := make(map[int64]string)
	priorityByAccount := make(map[int64]int)
	for i := range pool {
		account := &pool[i]
		for _, accountGroup := range account.AccountGroups {
			if accountGroup.GroupID != groupID {
				continue
			}
			priorityByAccount[account.ID] = accountGroup.Priority
			if accountGroup.Group != nil {
				groupNameByID[groupID] = accountGroup.Group.Name
			}
		}
	}
	return groupNameByID, priorityByAccount
}

func (h *AccountHandler) listAccountSchedulerScoreFilterPool(
	ctx context.Context,
	platform string,
	accountType string,
	status string,
	search string,
	groupID int64,
	privacyMode string,
) []service.Account {
	if h.adminService == nil || (platform != "" && platform != service.PlatformOpenAI) {
		return nil
	}

	accounts, err := h.adminService.ListAccountsForSchedulerScoreFilter(
		ctx,
		service.PlatformOpenAI,
		accountType,
		status,
		search,
		groupID,
		privacyMode,
	)
	if err != nil {
		slog.Warn("openai_scheduler_filter_score_pool_failed", "error", err)
		return nil
	}
	return accounts
}
