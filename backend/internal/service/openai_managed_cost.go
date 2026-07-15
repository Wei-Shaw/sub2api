package service

import (
	"context"
	"time"
)

func (s *defaultOpenAIAccountScheduler) selectManagedOpenAICostTiers(
	ctx context.Context,
	req OpenAIAccountScheduleRequest,
	tiers []managedCostTier,
	loadMap map[int64]*AccountLoadInfo,
) (*AccountSelectionResult, int, int, float64, error) {
	for index, tier := range tiers {
		attempt := s.trySelectByLoadBalancePool(ctx, req, tier.accounts, loadMap)
		if attempt.result != nil {
			return attempt.result, attempt.candidateCount, attempt.topK, attempt.loadSkew, nil
		}
		if attempt.err != nil && !attempt.noCompactCandidates {
			return nil, attempt.candidateCount, attempt.topK, attempt.loadSkew, attempt.err
		}
		if index == len(tiers)-1 {
			if attempt.err != nil {
				return nil, attempt.candidateCount, attempt.topK, attempt.loadSkew, attempt.err
			}
			return s.finishLoadBalanceSelectionFallback(ctx, req, attempt)
		}
		if len(attempt.selectionOrder) == 0 {
			continue
		}
		selection, waitedAttempt, err := s.waitManagedOpenAITier(ctx, req, tier.accounts)
		if err != nil {
			return nil, waitedAttempt.candidateCount, waitedAttempt.topK, waitedAttempt.loadSkew, err
		}
		if selection != nil {
			return selection, waitedAttempt.candidateCount, waitedAttempt.topK, waitedAttempt.loadSkew, nil
		}
	}
	return nil, 0, 0, 0, noAvailableOpenAISelectionError(req.RequestedModel, false)
}

func (s *defaultOpenAIAccountScheduler) waitManagedOpenAITier(
	ctx context.Context,
	req OpenAIAccountScheduleRequest,
	accounts []*Account,
) (*AccountSelectionResult, openAIAccountLoadSelectionAttempt, error) {
	timer := time.NewTimer(managedCostTierWaitTimeout)
	defer timer.Stop()
	ticker := time.NewTicker(managedCostPollInterval)
	defer ticker.Stop()
	last := openAIAccountLoadSelectionAttempt{}
	for {
		select {
		case <-ctx.Done():
			return nil, last, ctx.Err()
		case <-timer.C:
			return nil, last, nil
		case <-ticker.C:
			loadMap := map[int64]*AccountLoadInfo{}
			if s.service.concurrencyService != nil {
				fresh, err := s.service.concurrencyService.GetAccountsLoadBatchFresh(ctx, buildOpenAIAccountLoadRequest(accounts))
				if err == nil {
					loadMap = fresh
				}
			}
			last = s.trySelectByLoadBalancePool(ctx, req, accounts, loadMap)
			if last.err != nil && !last.noCompactCandidates {
				return nil, last, last.err
			}
			if last.result != nil {
				return last.result, last, nil
			}
		}
	}
}
