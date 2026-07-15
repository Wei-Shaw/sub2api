package service

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"
)

const codeBuddyTokenRefreshSkew = time.Hour

// CodeBuddyTokenRefresher 实现 TokenRefresher / OAuthRefreshExecutor 接口。
type CodeBuddyTokenRefresher struct {
	codeBuddyOAuthService CodeBuddyOAuthTokenService
}

func NewCodeBuddyTokenRefresher(codeBuddyOAuthService CodeBuddyOAuthTokenService) *CodeBuddyTokenRefresher {
	return &CodeBuddyTokenRefresher{codeBuddyOAuthService: codeBuddyOAuthService}
}

func (r *CodeBuddyTokenRefresher) CacheKey(account *Account) string {
	return CodeBuddyTokenCacheKey(account)
}

func (r *CodeBuddyTokenRefresher) CanRefresh(account *Account) bool {
	return account != nil && account.Platform == PlatformCodeBuddy && account.Type == AccountTypeOAuth
}

func (r *CodeBuddyTokenRefresher) NeedsRefresh(account *Account, refreshWindow time.Duration) bool {
	if account == nil || strings.TrimSpace(account.GetCodeBuddyRefreshToken()) == "" {
		return false
	}
	expiresAt := account.GetCredentialAsTime("expires_at")
	if expiresAt == nil {
		return true
	}
	if refreshWindow < codeBuddyTokenRefreshSkew {
		refreshWindow = codeBuddyTokenRefreshSkew
	}
	return time.Until(*expiresAt) < refreshWindow
}

func (r *CodeBuddyTokenRefresher) Refresh(ctx context.Context, account *Account) (map[string]any, error) {
	if r == nil || r.codeBuddyOAuthService == nil {
		return nil, errors.New("codebuddy oauth service is not configured")
	}
	tokenInfo, err := r.codeBuddyOAuthService.RefreshAccountToken(ctx, account)
	if err != nil {
		return nil, err
	}
	newCredentials := r.codeBuddyOAuthService.BuildAccountCredentials(tokenInfo)
	newCredentials = MergeCredentials(account.Credentials, newCredentials)
	if baseURL := strings.TrimSpace(account.GetCredential("base_url")); baseURL != "" {
		newCredentials["base_url"] = baseURL
	}
	return newCredentials, nil
}

// CodeBuddyTokenCacheKey 返回 CodeBuddy 账号的 token 缓存键。
func CodeBuddyTokenCacheKey(account *Account) string {
	if account == nil {
		return "codebuddy:account:0"
	}
	return "codebuddy:account:" + strconv.FormatInt(account.ID, 10)
}
