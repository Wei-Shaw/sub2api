package service

import (
	"context"
	"log/slog"
)

// applyPluginSchedulingHints calls the plugin's GetSchedulingHints RPC
// for each platform that has a registered plugin, then applies the
// returned hints to the candidate accounts:
//   - temporarily_unavailable = true → account is removed from the slice
//   - priority_modifier != 0 → added to account.Priority for this round
//
// This only affects the in-memory accounts slice; database values are
// never modified. Errors (including codes.Unimplemented) are silently
// ignored so scheduling is never blocked by a plugin failure.
func (s *GatewayService) applyPluginSchedulingHints(
	ctx context.Context,
	accounts []Account,
	gatewayProtocol string,
	requestedModel string,
) []Account {
	provider := s.loadPluginSchedulingHintsProvider()
	if provider == nil {
		return accounts
	}
	if len(accounts) == 0 {
		return accounts
	}

	// Collect all hints across platforms.
	allHints := s.collectSchedulingHints(ctx, provider, accounts, gatewayProtocol, requestedModel)
	if len(allHints) == 0 {
		return accounts
	}

	// Apply priority modifiers and filter unavailable accounts.
	return s.applyCollectedHints(accounts, allHints)
}

// collectSchedulingHints groups accounts by platform and calls the
// plugin's GetSchedulingHints RPC for each group.
func (s *GatewayService) collectSchedulingHints(
	ctx context.Context,
	provider PluginSchedulingHintsProvider,
	accounts []Account,
	gatewayProtocol string,
	requestedModel string,
) map[int64]SchedulingHintResult {
	byPlatform := make(map[string][]*Account)
	for i := range accounts {
		byPlatform[accounts[i].Platform] = append(byPlatform[accounts[i].Platform], &accounts[i])
	}

	allHints := make(map[int64]SchedulingHintResult)
	for plat, platAccounts := range byPlatform {
		hints, err := provider.GetHints(ctx, plat, platAccounts, gatewayProtocol, requestedModel)
		if err != nil {
			slog.Debug("plugin_scheduling_hints_skipped",
				"platform", plat,
				"error", err)
			continue
		}
		for id, h := range hints {
			allHints[id] = h
		}
	}
	return allHints
}

// applyCollectedHints applies priority modifiers in-place and removes
// temporarily unavailable accounts from the slice.
func (s *GatewayService) applyCollectedHints(
	accounts []Account,
	hints map[int64]SchedulingHintResult,
) []Account {
	n := 0
	for i := range accounts {
		hint, hasHint := hints[accounts[i].ID]
		if hasHint && hint.TemporarilyUnavailable {
			slog.Debug("plugin_scheduling_hint_unavailable",
				"account_id", accounts[i].ID,
				"reason", hint.Reason)
			continue // remove from candidates
		}
		if hasHint && hint.PriorityModifier != 0 {
			accounts[i].Priority += int(hint.PriorityModifier)
			slog.Debug("plugin_scheduling_hint_priority",
				"account_id", accounts[i].ID,
				"modifier", hint.PriorityModifier,
				"new_priority", accounts[i].Priority,
				"reason", hint.Reason)
		}
		accounts[n] = accounts[i]
		n++
	}
	return accounts[:n]
}
