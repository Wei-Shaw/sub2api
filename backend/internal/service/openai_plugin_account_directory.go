package service

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

// OpenAIGatewayService implements service.PluginAccountDirectory. The directory
// itself is generic: it enumerates accounts and resolves outbound identities
// strictly within the scope the host derived from the plugin's declared
// capabilities (see PluginManager.buildHostServices). A plugin can never widen
// that scope, and metadata never carries credentials — those are only handed out
// by ResolvePluginOutboundIdentity.
//
// The implementation lives on OpenAIGatewayService because it owns both the
// account repository (for scope-neutral metadata) and the OpenAI token/header
// minting used by identity resolution. Metadata listing is platform-neutral;
// only the credential resolution is OpenAI-specific and degrades to (nil, nil)
// for account kinds it cannot mint tokens for.

// ListPluginAccounts returns the readable metadata for every account within the
// plugin's scope, optionally narrowed by the plugin's requested (platform,
// accountType) filter. Active accounts that are currently NOT schedulable because
// they are paused (rate-limited / temp-unschedulable / overloaded) are included
// with their status intact, so the plugin can skip them instead of probing them —
// a paused account keeps Status=="active", so the repository's active-only query
// still returns it. (Administratively disabled / expired accounts are already
// excluded by the repository query.) Shadow accounts are excluded here: they hold
// no credentials of their own and are not independently schedulable.
func (s *OpenAIGatewayService) ListPluginAccounts(ctx context.Context, scope PluginAccountScope, platform, accountType string) ([]PluginAccountInfo, error) {
	if s == nil || s.accountRepo == nil || scope.Empty() {
		return nil, nil
	}
	reqPlatform := strings.TrimSpace(platform)
	reqAccountType := strings.TrimSpace(accountType)

	infos := make([]PluginAccountInfo, 0)
	for _, scopePlatform := range scope.Platforms() {
		if reqPlatform != "" && reqPlatform != scopePlatform {
			continue
		}
		accounts, err := s.accountRepo.ListByPlatform(ctx, scopePlatform)
		if err != nil {
			return nil, err
		}
		for i := range accounts {
			account := accounts[i]
			if account.IsShadow() {
				continue
			}
			// Defense-in-depth: the contract exposes only active accounts (paused
			// ones stay active; disabled/expired are out). ListByPlatform already
			// filters to active at the DB, but do not silently depend on that — a
			// paused account keeps Status=="active" and still passes here.
			if !account.IsActive() {
				continue
			}
			if !scope.Contains(account.Platform, account.Type) {
				continue
			}
			if reqAccountType != "" && reqAccountType != account.Type {
				continue
			}
			infos = append(infos, accountToPluginInfo(&account))
		}
	}
	return infos, nil
}

// accountToPluginInfo maps a domain account onto the host's readable, non-secret
// view: a small stable typed core plus a full JSON snapshot. Schedulable is the
// host-authoritative decision so the plugin never has to re-derive the pause rules.
func accountToPluginInfo(account *Account) PluginAccountInfo {
	return PluginAccountInfo{
		ID:           account.ID,
		Platform:     account.Platform,
		AccountType:  account.Type,
		Name:         account.Name,
		Status:       account.Status,
		Schedulable:  account.IsSchedulable(),
		IsShadow:     account.IsShadow(),
		MetadataJSON: accountReadableSnapshotJSON(account),
	}
}

// accountReadableSnapshotJSON marshals the account's readable field set to JSON.
// It is a DENYLIST on purpose: any non-secret field added to the account model
// later flows through automatically with no host change. Only two categories are
// stripped:
//
//   - Credentials: holds the refresh_token (and possibly api_key), a long-lived
//     credential that ResolveOutboundIdentity does NOT hand out (that channel only
//     mints a short-lived access token). Keeping it out of this list keeps the
//     credential surface exactly what the outbound-identity channel already
//     exposes. (Extra and the proxy — including its password, which is already
//     handed out via the resolved ProxyURL — are intentionally NOT stripped.)
//   - Groups / AccountGroups: relational graphs with *Group/*Account
//     back-references. encoding/json does NOT detect reference cycles and would
//     recurse into a stack-overflow panic if the reverse relation is ever
//     populated. GroupIDs already conveys membership, so drop these for
//     correctness (not secrecy).
//
// Unexported fields (e.g. the hot-path caches) are never marshaled by encoding/json.
func accountReadableSnapshotJSON(account *Account) []byte {
	if account == nil {
		return nil
	}
	clone := *account
	clone.Credentials = nil
	clone.Groups = nil
	clone.AccountGroups = nil
	data, err := json.Marshal(&clone)
	if err != nil {
		return nil
	}
	return data
}

// ResolvePluginOutboundIdentity resolves the access token plus the outbound
// identity headers and proxy the host would attach to a live request for the
// account. It returns (nil, nil) for accounts outside the plugin's scope, for
// account kinds it cannot mint tokens for, or when no token can be resolved.
func (s *OpenAIGatewayService) ResolvePluginOutboundIdentity(ctx context.Context, scope PluginAccountScope, accountID int64) (*PluginOutboundIdentity, error) {
	if s == nil || s.accountRepo == nil || accountID <= 0 || scope.Empty() {
		return nil, nil
	}
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if account == nil || account.IsShadow() {
		return nil, nil
	}
	// Scope is the authoritative permission boundary: refuse any account the
	// plugin's declared capabilities do not cover, even if it exists.
	if !scope.Contains(account.Platform, account.Type) {
		return nil, nil
	}
	// Credential minting is OpenAI-specific; degrade to (nil, nil) for kinds this
	// resolver cannot issue tokens for.
	if account.Type != AccountTypeOAuth || !account.IsOpenAIOAuthLike() {
		return nil, nil
	}
	token, _, err := s.GetAccessToken(ctx, account)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(token) == "" {
		return nil, nil
	}
	headers := http.Header{}
	if err := resolveAndSetOpenAIChatGPTAccountHeaders(ctx, s.accountRepo, headers, account); err != nil {
		return nil, err
	}
	ensureCodexIdentityHeaders(headers)
	enforceCodexIdentityHeaders(headers)
	return &PluginOutboundIdentity{
		AccountID:   account.ID,
		Platform:    account.Platform,
		AccountType: account.Type,
		ProxyURL:    resolveAccountProxyURL(account),
		Token:       token,
		Headers:     headers,
	}, nil
}
