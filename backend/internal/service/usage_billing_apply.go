package service

import (
	"context"
	"errors"
)

// applyVideoUsageBilling has no legacy fallback: a successful video settlement
// requires both the charge and its usage row to have committed.
func applyVideoUsageBilling(ctx context.Context, requestID string, usageLog *UsageLog, p *postUsageBillingParams, deps *billingDeps, repo UsageBillingRepository) (bool, error) {
	return applyVideoUsageBillingCommand(ctx, buildUsageBillingCommand(requestID, usageLog, p), usageLog, p, deps, repo)
}

func applyVideoUsageBillingCommand(ctx context.Context, cmd *UsageBillingCommand, usageLog *UsageLog, p *postUsageBillingParams, deps *billingDeps, repo UsageBillingRepository) (bool, error) {
	if repo == nil {
		return false, errors.New("atomic video usage billing repository is required")
	}
	atomicRepo, ok := repo.(AtomicVideoUsageBillingRepository)
	if !ok {
		return false, errors.New("usage billing repository does not support atomic video usage")
	}
	if usageLog == nil || usageLog.VideoCount <= 0 || deps == nil || p == nil {
		return false, errors.New("atomic video usage billing requires a video usage log and dependencies")
	}
	if cmd == nil || cmd.RequestID == "" {
		return false, errors.New("invalid atomic video usage billing command")
	}
	billingCtx, cancel := detachedBillingContext(ctx)
	defer cancel()
	result, err := atomicRepo.ApplyVideo(billingCtx, cmd, usageLog)
	if err != nil {
		return false, err
	}
	if result == nil {
		return false, errors.New("atomic video usage billing returned no result")
	}
	if !result.Applied {
		deps.deferredService.ScheduleLastUsedUpdate(p.Account.ID)
		return false, nil
	}
	if result.APIKeyQuotaExhausted {
		if invalidator, ok := p.APIKeyService.(apiKeyAuthCacheInvalidator); ok && p.APIKey.Key != "" {
			invalidator.InvalidateAuthCacheByKey(billingCtx, p.APIKey.Key)
		}
	}
	finalizePostUsageBilling(billingCtx, p, deps, result)
	return true, nil
}
