package service

import (
	"context"
	"log/slog"
)

// Account-level RPM limiting on the OpenAI-compatible scheduling paths.
//
// The per-account RPM limit (extra.base_rpm, see Account.GetBaseRPM) used to be
// enforced only for Anthropic OAuth/SetupToken accounts. API-key accounts of
// OpenAI-compatible platforms (openai, kimi, zhipu, deepseek, minimax, ...) are
// commonly bound by a provider-side requests-per-minute plan as well, and an
// upstream 429 usually takes the whole account out of rotation for a cooldown.
// These helpers apply the same soft limit (CheckRPMSchedulability: tiered or
// sticky_exempt) and the same per-minute counter (RPMCache) to those accounts.

// SetRPMCache wires the per-account RPM counter shared with GatewayService.
func (s *OpenAIGatewayService) SetRPMCache(cache RPMCache) {
	if s == nil {
		return
	}
	s.rpmCache = cache
}

// openAIRPMCounts prefetches the current-minute request counts of every
// candidate that has an RPM limit. Returns nil when nothing needs checking or the
// cache is unavailable (callers then fall back to per-account lookups / fail open).
func (s *OpenAIGatewayService) openAIRPMCounts(ctx context.Context, accounts []Account) map[int64]int {
	if s == nil || s.rpmCache == nil {
		return nil
	}
	var ids []int64
	for i := range accounts {
		if accounts[i].GetBaseRPM() > 0 {
			ids = append(ids, accounts[i].ID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	counts, err := s.rpmCache.GetRPMBatch(ctx, ids)
	if err != nil {
		return nil // fail open, same as GatewayService.withRPMPrefetch
	}
	return counts
}

// isOpenAIAccountSchedulableForRPM reports whether the account may take another
// request this minute. isSticky selects the sticky-session semantics of the
// account's RPM strategy (a sticky session may use the tiered buffer).
func (s *OpenAIGatewayService) isOpenAIAccountSchedulableForRPM(ctx context.Context, account *Account, isSticky bool, counts map[int64]int) bool {
	if s == nil || account == nil || account.GetBaseRPM() <= 0 {
		return true
	}
	current, found := 0, false
	if counts != nil {
		current, found = counts[account.ID]
	}
	if !found && s.rpmCache != nil {
		if count, err := s.rpmCache.GetRPM(ctx, account.ID); err == nil {
			current = count
		}
	}
	switch account.CheckRPMSchedulability(current) {
	case WindowCostSchedulable:
		return true
	case WindowCostStickyOnly:
		return isSticky
	default:
		return false
	}
}

// recordOpenAIAccountRPM counts a selected account against its RPM window.
// Counting at selection time (every upstream attempt, including failover
// attempts) matches how providers count requests.
func (s *OpenAIGatewayService) recordOpenAIAccountRPM(ctx context.Context, selection *AccountSelectionResult) {
	if s == nil || s.rpmCache == nil || selection == nil || selection.Account == nil || selection.Account.GetBaseRPM() <= 0 {
		return
	}
	if _, err := s.rpmCache.IncrementRPM(ctx, selection.Account.ID); err != nil {
		slog.Warn("openai.rpm_increment_failed", "account_id", selection.Account.ID, "error", err)
	}
}
