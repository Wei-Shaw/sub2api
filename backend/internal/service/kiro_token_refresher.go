package service

import (
	"context"
	"time"
)

// kiroRefreshWindow Kiro token 提前刷新窗口：15 分钟。
// Kiro 上游 social 与 IdC token 都为 1 小时有效；提前 15 分钟刷新与
// Antigravity 行为一致。
const kiroRefreshWindow = 15 * time.Minute

// KiroTokenRefresher implements TokenRefresher + OAuthRefreshExecutor for
// kiro accounts. Used by TokenRefreshService for periodic background refresh.
type KiroTokenRefresher struct {
	kiroOAuthService *KiroOAuthService
}

// NewKiroTokenRefresher constructs the refresher.
func NewKiroTokenRefresher(kiroOAuthService *KiroOAuthService) *KiroTokenRefresher {
	return &KiroTokenRefresher{kiroOAuthService: kiroOAuthService}
}

// CacheKey satisfies OAuthRefreshExecutor (distributed-lock key).
func (r *KiroTokenRefresher) CacheKey(account *Account) string {
	return KiroTokenCacheKey(account)
}

// CanRefresh accepts only kiro OAuth accounts.
func (r *KiroTokenRefresher) CanRefresh(account *Account) bool {
	return account.Platform == PlatformKiro && account.Type == AccountTypeOAuth
}

// NeedsRefresh checks expires_at against kiroRefreshWindow. Ignores the
// global refreshWindow argument, matching Antigravity's fixed-window pattern.
func (r *KiroTokenRefresher) NeedsRefresh(account *Account, _ time.Duration) bool {
	if !r.CanRefresh(account) {
		return false
	}
	expiresAt := account.GetCredentialAsTime("expires_at")
	if expiresAt == nil {
		return false
	}
	return time.Until(*expiresAt) < kiroRefreshWindow
}

// Refresh executes a token refresh and returns the new credentials map.
// Preserves any fields the refresh doesn't touch (e.g., client_id /
// client_secret for IdC accounts) via MergeCredentials.
func (r *KiroTokenRefresher) Refresh(ctx context.Context, account *Account) (map[string]any, error) {
	tokenInfo, err := r.kiroOAuthService.RefreshAccountToken(ctx, account)
	if err != nil {
		return nil, err
	}
	newCreds := r.kiroOAuthService.BuildAccountCredentials(tokenInfo)
	return MergeCredentials(account.Credentials, newCreds), nil
}
