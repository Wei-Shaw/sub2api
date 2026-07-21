package service

import (
	"context"
	"errors"
	"strings"
	"time"
)

// kimiTokenRefreshSkew 是 Kimi access_token 的提前刷新窗口。
// Kimi access_token 有效期约 15 分钟（900s），按 kimi-cli 策略
// max(300s, expires_in×0.5) 取 5 分钟作为固定提前量。
const kimiTokenRefreshSkew = 5 * time.Minute

type KimiTokenRefresher struct {
	kimiOAuthService KimiOAuthTokenService
}

func NewKimiTokenRefresher(kimiOAuthService KimiOAuthTokenService) *KimiTokenRefresher {
	return &KimiTokenRefresher{kimiOAuthService: kimiOAuthService}
}

func (r *KimiTokenRefresher) CacheKey(account *Account) string {
	return KimiTokenCacheKey(account)
}

func (r *KimiTokenRefresher) CanRefresh(account *Account) bool {
	return account != nil && account.Platform == PlatformKimi && account.Type == AccountTypeOAuth
}

func (r *KimiTokenRefresher) NeedsRefresh(account *Account, refreshWindow time.Duration) bool {
	if account == nil || strings.TrimSpace(account.GetKimiRefreshToken()) == "" {
		return false
	}
	expiresAt := account.GetCredentialAsTime("expires_at")
	if expiresAt == nil {
		return true
	}
	if refreshWindow < kimiTokenRefreshSkew {
		refreshWindow = kimiTokenRefreshSkew
	}
	return time.Until(*expiresAt) < refreshWindow
}

func (r *KimiTokenRefresher) Refresh(ctx context.Context, account *Account) (map[string]any, error) {
	if r == nil || r.kimiOAuthService == nil {
		return nil, errors.New("kimi oauth service is not configured")
	}
	tokenInfo, err := r.kimiOAuthService.RefreshAccountToken(ctx, account)
	if err != nil {
		return nil, err
	}
	newCredentials := r.kimiOAuthService.BuildAccountCredentials(tokenInfo)
	// refresh_token 轮换值已包含在 newCredentials 中，随合并原子写回；
	// device_id / base_url 等持久化字段由旧凭据保留兜底。
	newCredentials = MergeCredentials(account.Credentials, newCredentials)
	if baseURL := strings.TrimSpace(account.GetCredential("base_url")); baseURL != "" {
		newCredentials["base_url"] = baseURL
	}
	if deviceID := strings.TrimSpace(account.GetKimiDeviceID()); deviceID != "" {
		newCredentials["device_id"] = deviceID
	}
	return newCredentials, nil
}
