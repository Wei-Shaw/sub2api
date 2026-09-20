package service

import (
	"context"
	"net/http"
	"strings"
)

// OpenAIGatewayService implements service.PluginAccountDirectory for the OpenAI
// OAuth outbound transport capability. The directory is intentionally scoped to
// OpenAI OAuth-like, non-shadow accounts regardless of the requested filter, so a
// plugin can never enumerate or resolve credentials outside that set. The host
// additionally only wires this directory into plugins whose manifest declares the
// matching capability (see PluginManager.buildHostServices).

// ListPluginAccounts returns the ids of active OpenAI OAuth-like accounts.
func (s *OpenAIGatewayService) ListPluginAccounts(ctx context.Context, platform, accountType string) ([]int64, error) {
	if s == nil || s.accountRepo == nil {
		return nil, nil
	}
	if p := strings.TrimSpace(platform); p != "" && p != PlatformOpenAI {
		return nil, nil
	}
	if at := strings.TrimSpace(accountType); at != "" && at != AccountTypeOAuth {
		// Setup-token style accounts are OAuth-like but not AccountTypeOAuth; the
		// current binding only routes AccountTypeOAuth, so honour that filter.
		return nil, nil
	}
	accounts, err := s.accountRepo.ListByPlatform(ctx, PlatformOpenAI)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(accounts))
	for i := range accounts {
		account := accounts[i]
		if account.Status == StatusActive && account.IsOpenAIOAuthLike() && !account.IsShadow() && account.Type == AccountTypeOAuth {
			ids = append(ids, account.ID)
		}
	}
	return ids, nil
}

// ResolvePluginOutboundIdentity resolves the access token plus the outbound
// identity headers and proxy the host would attach to a live request for the
// account. It returns (nil, nil) for out-of-scope accounts or when no token can
// be resolved.
func (s *OpenAIGatewayService) ResolvePluginOutboundIdentity(ctx context.Context, accountID int64) (*PluginOutboundIdentity, error) {
	if s == nil || s.accountRepo == nil || accountID <= 0 {
		return nil, nil
	}
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if account == nil || account.Type != AccountTypeOAuth || !account.IsOpenAIOAuthLike() || account.IsShadow() {
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
