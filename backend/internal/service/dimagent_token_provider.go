package service

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"
)

const dimAgentTokenRefreshSkew = time.Minute

// DimAgentTokenProvider obtains short-lived credentials issued by DimAgent's
// OAuth authorization-code flow. Refresh serialization and durable credential
// rotation are delegated to OAuthRefreshAPI, shared with other providers.
type DimAgentTokenProvider struct {
	accountRepo AccountRepository
	oauth       *DimAgentOAuthService
	refreshAPI  *OAuthRefreshAPI
	executor    *DimAgentTokenRefresher
}

func NewDimAgentTokenProvider(accountRepo AccountRepository, oauth *DimAgentOAuthService, refreshAPI *OAuthRefreshAPI) *DimAgentTokenProvider {
	return &DimAgentTokenProvider{
		accountRepo: accountRepo,
		oauth:       oauth,
		refreshAPI:  refreshAPI,
		executor:    NewDimAgentTokenRefresher(oauth),
	}
}

// RefreshAfterUnauthorized performs one forced refresh after an upstream 401.
// The caller must retry exactly once; subsequent 401s flow into the standard
// account failure/failover path rather than causing a refresh loop.
func (p *DimAgentTokenProvider) RefreshAfterUnauthorized(ctx context.Context, account *Account) (string, error) {
	if account == nil || !account.IsDimAgent() || account.Type != AccountTypeOAuth {
		return "", errors.New("not a DimAgent OAuth account")
	}
	if p == nil || p.oauth == nil {
		return "", errors.New("DimAgent OAuth refresh is not configured")
	}
	info, err := p.oauth.RefreshAccountToken(ctx, account)
	if err != nil {
		return "", err
	}
	credentials := MergeCredentials(account.Credentials, p.oauth.BuildAccountCredentials(info))
	if err := persistAccountCredentials(ctx, p.accountRepo, account, credentials); err != nil {
		return "", err
	}
	return strings.TrimSpace(info.AccessToken), nil
}

func (p *DimAgentTokenProvider) GetAccessToken(ctx context.Context, account *Account) (string, error) {
	if account == nil {
		return "", errors.New("DimAgent account is nil")
	}
	if !account.IsDimAgent() || account.Type != AccountTypeOAuth {
		return "", errors.New("not a DimAgent OAuth account")
	}
	if p == nil {
		return "", errors.New("DimAgent token provider is not configured")
	}
	if account.DimAgentNeedsRefresh(dimAgentTokenRefreshSkew) {
		if p.oauth == nil || p.refreshAPI == nil || p.executor == nil {
			return "", errors.New("DimAgent OAuth refresh is not configured")
		}
		result, err := p.refreshAPI.RefreshIfNeeded(withOAuthRefreshRequestPath(ctx), account, p.executor, dimAgentTokenRefreshSkew)
		if err != nil {
			return "", err
		}
		if result != nil && result.Account != nil {
			account = result.Account
		}
	}
	accessToken := strings.TrimSpace(account.DimAgentAccessToken())
	if accessToken == "" {
		return "", errors.New("DimAgent access_token not found in credentials")
	}
	return accessToken, nil
}

// DimAgentTokenRefresher plugs DimAgent OAuth into the shared refresh worker
// and distributed refresh lock. It retains all non-token account settings.
type DimAgentTokenRefresher struct {
	oauth *DimAgentOAuthService
}

func NewDimAgentTokenRefresher(oauth *DimAgentOAuthService) *DimAgentTokenRefresher {
	return &DimAgentTokenRefresher{oauth: oauth}
}

func (r *DimAgentTokenRefresher) CacheKey(account *Account) string {
	if account == nil {
		return "dimagent:account:0"
	}
	return "dimagent:account:" + strconv.FormatInt(account.ID, 10)
}

func (r *DimAgentTokenRefresher) CanRefresh(account *Account) bool {
	return r != nil && r.oauth != nil && account != nil && account.IsDimAgent() && account.Type == AccountTypeOAuth && account.DimAgentRefreshToken() != ""
}

func (r *DimAgentTokenRefresher) NeedsRefresh(account *Account, refreshWindow time.Duration) bool {
	if !r.CanRefresh(account) {
		return false
	}
	return account.DimAgentNeedsRefresh(refreshWindow)
}

func (r *DimAgentTokenRefresher) Refresh(ctx context.Context, account *Account) (map[string]any, error) {
	info, err := r.oauth.RefreshAccountToken(ctx, account)
	if err != nil {
		return nil, err
	}
	return MergeCredentials(account.Credentials, r.oauth.BuildAccountCredentials(info)), nil
}
