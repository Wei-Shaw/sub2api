package service

import (
	"context"
	"time"
)

type openAITransientRequestedModelKey struct{}

func openAITransientRequestedModel(ctx context.Context) string {
	model, _ := ctx.Value(openAITransientRequestedModelKey{}).(string)
	return model
}

// This only delays a failed selection; the next pass repeats all eligibility
// and concurrency checks, without bypassing any cooldown or restriction.
func (s *OpenAIGatewayService) nextOpenAITransientRetryDelay(ctx context.Context, groupID *int64, model string, capability OpenAIEndpointCapability, requireCompact bool) time.Duration {
	accounts, err := s.listSchedulableAccounts(ctx, groupID, PlatformOpenAI)
	if err != nil {
		return 0
	}
	now := time.Now()
	var earliest time.Time
	eligible := 0
	for i := range accounts {
		account := &accounts[i]
		if !isOpenAICompatibleAccountEligibleForRequest(ctx, account, PlatformOpenAI, model, requireCompact, capability) {
			continue
		}
		eligible++
		if eligible > 5 {
			return 0
		}
		if !isOpenAIAccountTransientCircuitCandidate(account) {
			continue
		}
		accountState := s.getOpenAIAccountTransientState()
		modelState := s.getOpenAIAccountModelTransientState()
		canonical := canonicalOpenAIAccountSchedulingModel(account, model)
		at := accountState.retryAt(account.ID, now)
		modelAt := modelState.retryAt(account.ID, canonical, now)
		if accountState.isBlocked(account.ID, now) && at.IsZero() {
			continue
		}
		if modelState.isBlocked(account.ID, canonical, now) && modelAt.IsZero() {
			continue
		}
		if modelAt.After(at) {
			at = modelAt
		}
		if at.After(now) && (earliest.IsZero() || at.Before(earliest)) {
			earliest = at
		}
	}
	if earliest.IsZero() {
		return 0
	}
	delay := earliest.Sub(now) + 10*time.Millisecond
	if delay > 2*time.Second {
		return 0
	}
	if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) <= delay {
		return 0
	}
	return delay
}
