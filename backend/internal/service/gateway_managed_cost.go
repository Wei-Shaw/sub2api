package service

import (
	"context"
	"time"
)

const managedCostPollInterval = 50 * time.Millisecond

func (s *GatewayService) selectManagedCostTiers(
	ctx context.Context,
	tiers []managedCostTier,
	groupID *int64,
	sessionHash string,
	preferOAuth bool,
	cfg gatewaySchedulingView,
) (*AccountSelectionResult, []*Account, error) {
	if len(tiers) == 0 {
		return nil, nil, nil
	}
	for index, tier := range tiers {
		selection, err := s.tryManagedGatewayTier(ctx, tier.accounts, groupID, sessionHash, preferOAuth, cfg.preferSoonestReset, false)
		if err != nil || selection != nil {
			return selection, tier.accounts, err
		}
		if index == len(tiers)-1 {
			return nil, tier.accounts, nil
		}
		selection, err = s.waitManagedGatewayTier(ctx, tier.accounts, groupID, sessionHash, preferOAuth, cfg.preferSoonestReset)
		if err != nil || selection != nil {
			return selection, tier.accounts, err
		}
	}
	return nil, tiers[len(tiers)-1].accounts, nil
}

type gatewaySchedulingView struct {
	preferSoonestReset bool
}

func (s *GatewayService) waitManagedGatewayTier(
	ctx context.Context,
	accounts []*Account,
	groupID *int64,
	sessionHash string,
	preferOAuth bool,
	preferSoonestReset bool,
) (*AccountSelectionResult, error) {
	timer := time.NewTimer(managedCostTierWaitTimeout)
	defer timer.Stop()
	ticker := time.NewTicker(managedCostPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timer.C:
			return nil, nil
		case <-ticker.C:
			selection, err := s.tryManagedGatewayTier(ctx, accounts, groupID, sessionHash, preferOAuth, preferSoonestReset, true)
			if err != nil || selection != nil {
				return selection, err
			}
		}
	}
}

func (s *GatewayService) tryManagedGatewayTier(
	ctx context.Context,
	accounts []*Account,
	groupID *int64,
	sessionHash string,
	preferOAuth bool,
	preferSoonestReset bool,
	freshLoad bool,
) (*AccountSelectionResult, error) {
	loads := make([]AccountWithConcurrency, 0, len(accounts))
	for _, account := range accounts {
		loads = append(loads, AccountWithConcurrency{ID: account.ID, MaxConcurrency: account.EffectiveLoadFactor()})
	}
	loadMap := map[int64]*AccountLoadInfo{}
	if s.concurrencyService != nil {
		var err error
		if freshLoad {
			loadMap, err = s.concurrencyService.GetAccountsLoadBatchFresh(ctx, loads)
		} else {
			loadMap, err = s.concurrencyService.GetAccountsLoadBatch(ctx, loads)
		}
		if err != nil {
			loadMap = map[int64]*AccountLoadInfo{}
		}
	}

	available := make([]accountWithLoad, 0, len(accounts))
	for _, account := range accounts {
		loadInfo := loadMap[account.ID]
		if loadInfo == nil {
			loadInfo = &AccountLoadInfo{AccountID: account.ID}
		}
		available = append(available, accountWithLoad{account: account, loadInfo: loadInfo})
	}
	for len(available) > 0 {
		pool := filterByMinPriority(available)
		if preferSoonestReset {
			pool = filterBySoonestReset(pool)
		}
		pool = filterByMinLoadRate(pool)
		selected := selectByLRU(pool, preferOAuth)
		if selected == nil {
			return nil, nil
		}
		result, err := s.tryAcquireAccountSlot(ctx, selected.account.ID, selected.account.Concurrency)
		if err != nil {
			return nil, err
		}
		if result != nil && result.Acquired {
			if !s.checkAndRegisterSession(ctx, selected.account, sessionHash) {
				result.ReleaseFunc()
			} else {
				if sessionHash != "" && s.cache != nil {
					_ = s.cache.SetSessionAccountID(ctx, derefGroupID(groupID), sessionHash, selected.account.ID, stickySessionTTL)
				}
				return s.newSelectionResult(ctx, selected.account, true, result.ReleaseFunc, nil)
			}
		}
		selectedID := selected.account.ID
		next := make([]accountWithLoad, 0, len(available)-1)
		for _, item := range available {
			if item.account.ID != selectedID {
				next = append(next, item)
			}
		}
		available = next
	}
	return nil, nil
}
