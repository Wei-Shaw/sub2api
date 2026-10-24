package service

import (
	"context"
	"log/slog"
)

// ManagedWebSearchAccount resolves the account that should serve a pure
// web_search turn.
//
// Resolution order:
//  1. the globally configured managed_account_id;
//  2. otherwise, the first schedulable account in the caller's group that is
//     marked web_search_capable and speaks a native Anthropic protocol.
//
// Users keep using their normal sub2api API key and do not configure a search
// endpoint or a separate search key. The resolved account is forwarded
// zero-copy, preserving the web_search_20250305 server tool and the structured
// web_search_tool_result blocks.
//
// A nil result means the request is not a pure search or no account can serve
// native search. Callers fall back to normal routing. The global web-search
// emulation switch does not gate this path: an account marked
// web_search_capable is enough, matching the account-pool selection model.
func (s *OpenAIGatewayService) ManagedWebSearchAccount(ctx context.Context, groupID *int64, body []byte) *Account {
	if s == nil || s.settingService == nil || s.accountRepo == nil {
		return nil
	}
	if !IsOnlyWebSearchToolInBody(body) {
		return nil
	}
	cfg, cfgErr := s.settingService.GetWebSearchEmulationConfig(ctx)
	if cfgErr != nil {
		slog.Warn("managed web search: read global config failed, falling back to account capability",
			"error", cfgErr)
	}
	if cfg != nil {
		if accountID := cfg.ManagedAccount(); accountID > 0 {
			account, loadErr := s.accountRepo.GetByID(ctx, accountID)
			if loadErr != nil || account == nil {
				slog.Warn("managed web search: configured account unavailable",
					"account_id", accountID, "error", loadErr)
			} else if isNativeAnthropicSearchAccount(account) {
				return account
			} else {
				slog.Warn("managed web search: configured account is not on a native Anthropic protocol",
					"account_id", accountID, "platform", account.Platform)
			}
		}
	}
	if groupID == nil {
		return nil
	}
	accounts, err := s.accountRepo.ListSchedulableByGroupID(ctx, *groupID)
	if err != nil {
		slog.Warn("managed web search: list group accounts failed",
			"group_id", *groupID, "error", err)
		return nil
	}
	for i := range accounts {
		account := &accounts[i]
		if !account.SupportsWebSearch() {
			continue
		}
		if isNativeAnthropicSearchAccount(account) {
			return account
		}
	}
	return nil
}

func isNativeAnthropicSearchAccount(account *Account) bool {
	return account != nil && (account.IsAnthropicProtocol() || account.IsAdaptiveAPIProtocol())
}
