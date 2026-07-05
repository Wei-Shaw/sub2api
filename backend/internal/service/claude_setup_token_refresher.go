package service

import (
	"context"
	"time"
)

// claudeSetupTokenRefreshAPI 刷新 setup-token 所需的最小依赖（*OAuthService 天然实现）
type claudeSetupTokenRefreshAPI interface {
	RefreshAccountToken(ctx context.Context, account *Account) (*TokenInfo, error)
}

// ClaudeSetupTokenRefresher 处理 Anthropic setup-token 账号的自动刷新。
// 独立于 ClaudeTokenRefresher(oauth)：历史上 setup-token 有效期约 1 年、无需刷新，
// 但 Claude 端拒收 expires_in 后改发短效（~1天）token，该假设失效。
// 此处让 setup-token 走与 oauth 相同的底层 refresh_token grant 逻辑，oauth 刷新器零改动。
type ClaudeSetupTokenRefresher struct {
	oauthService claudeSetupTokenRefreshAPI
}

// NewClaudeSetupTokenRefresher 创建 Claude setup-token 刷新器
func NewClaudeSetupTokenRefresher(oauthService *OAuthService) *ClaudeSetupTokenRefresher {
	return &ClaudeSetupTokenRefresher{
		oauthService: oauthService,
	}
}

// CacheKey 复用 Claude token 的分布式锁 key（按账号 ID 唯一；一个账号只可能是一种 type，不与 oauth 冲突）
func (r *ClaudeSetupTokenRefresher) CacheKey(account *Account) string {
	return ClaudeTokenCacheKey(account)
}

// CanRefresh 只处理 anthropic 平台的 setup-token 类型账号
func (r *ClaudeSetupTokenRefresher) CanRefresh(account *Account) bool {
	return account.Platform == PlatformAnthropic &&
		account.Type == AccountTypeSetupToken
}

// NeedsRefresh 基于 expires_at 字段判断是否在刷新窗口内
func (r *ClaudeSetupTokenRefresher) NeedsRefresh(account *Account, refreshWindow time.Duration) bool {
	expiresAt := account.GetCredentialAsTime("expires_at")
	if expiresAt == nil {
		return false
	}
	return time.Until(*expiresAt) < refreshWindow
}

// Refresh 用 refresh_token grant 换新 token，保留原有credentials中的非 token 字段
func (r *ClaudeSetupTokenRefresher) Refresh(ctx context.Context, account *Account) (map[string]any, error) {
	tokenInfo, err := r.oauthService.RefreshAccountToken(ctx, account)
	if err != nil {
		return nil, err
	}

	newCredentials := BuildClaudeAccountCredentials(tokenInfo)
	newCredentials = MergeCredentials(account.Credentials, newCredentials)

	return newCredentials, nil
}
