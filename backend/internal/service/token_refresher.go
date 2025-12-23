package service

import (
	"context"
	"strconv"
	"time"

	"sub2api/internal/model"
)

// TokenRefresher 定义平台特定的token刷新策略接口
// 通过此接口可以扩展支持不同平台（Anthropic/OpenAI/Gemini）
type TokenRefresher interface {
	// CanRefresh 检查此刷新器是否能处理指定账号
	CanRefresh(account *model.Account) bool

	// NeedsRefresh 检查账号的token是否需要刷新
	NeedsRefresh(account *model.Account, refreshWindow time.Duration) bool

	// Refresh 执行token刷新，返回更新后的credentials
	// 注意：返回的map应该保留原有credentials中的所有字段，只更新token相关字段
	Refresh(ctx context.Context, account *model.Account) (map[string]any, error)
}

// ClaudeTokenRefresher 处理Anthropic/Claude OAuth token刷新
type ClaudeTokenRefresher struct {
	oauthService *OAuthService
}

// NewClaudeTokenRefresher 创建Claude token刷新器
func NewClaudeTokenRefresher(oauthService *OAuthService) *ClaudeTokenRefresher {
	return &ClaudeTokenRefresher{
		oauthService: oauthService,
	}
}

// CanRefresh 检查是否能处理此账号
// 只处理 anthropic 平台的 oauth 类型账号
// setup-token 虽然也是OAuth，但有效期1年，不需要频繁刷新
func (r *ClaudeTokenRefresher) CanRefresh(account *model.Account) bool {
	return account.Platform == model.PlatformAnthropic &&
		account.Type == model.AccountTypeOAuth
}

// NeedsRefresh 检查token是否需要刷新
// 基于 expires_at 字段判断是否在刷新窗口内
func (r *ClaudeTokenRefresher) NeedsRefresh(account *model.Account, refreshWindow time.Duration) bool {
	expiresAtStr := account.GetCredential("expires_at")
	if expiresAtStr == "" {
		return false
	}

	expiresAt, err := strconv.ParseInt(expiresAtStr, 10, 64)
	if err != nil {
		return false
	}

	expiryTime := time.Unix(expiresAt, 0)
	return time.Until(expiryTime) < refreshWindow
}

// Refresh 执行token刷新
// 保留原有credentials中的所有字段，只更新token相关字段
func (r *ClaudeTokenRefresher) Refresh(ctx context.Context, account *model.Account) (map[string]any, error) {
	tokenInfo, err := r.oauthService.RefreshAccountToken(ctx, account)
	if err != nil {
		return nil, err
	}

	// 保留现有credentials中的所有字段
	newCredentials := make(map[string]any)
	for k, v := range account.Credentials {
		newCredentials[k] = v
	}

	// 只更新token相关字段
	// 注意：expires_at 和 expires_in 必须存为字符串，因为 GetCredential 只返回 string 类型
	newCredentials["access_token"] = tokenInfo.AccessToken
	newCredentials["token_type"] = tokenInfo.TokenType
	newCredentials["expires_in"] = strconv.FormatInt(tokenInfo.ExpiresIn, 10)
	newCredentials["expires_at"] = strconv.FormatInt(tokenInfo.ExpiresAt, 10)
	if tokenInfo.RefreshToken != "" {
		newCredentials["refresh_token"] = tokenInfo.RefreshToken
	}
	if tokenInfo.Scope != "" {
		newCredentials["scope"] = tokenInfo.Scope
	}

	return newCredentials, nil
}

// OpenAITokenRefresher 处理 OpenAI OAuth token刷新
type OpenAITokenRefresher struct {
	openaiOAuthService *OpenAIOAuthService
}

// NewOpenAITokenRefresher 创建 OpenAI token刷新器
func NewOpenAITokenRefresher(openaiOAuthService *OpenAIOAuthService) *OpenAITokenRefresher {
	return &OpenAITokenRefresher{
		openaiOAuthService: openaiOAuthService,
	}
}

// CanRefresh 检查是否能处理此账号
// 只处理 openai 平台的 oauth 类型账号
func (r *OpenAITokenRefresher) CanRefresh(account *model.Account) bool {
	return account.Platform == model.PlatformOpenAI &&
		account.Type == model.AccountTypeOAuth
}

// NeedsRefresh 检查token是否需要刷新
// 基于 expires_at 字段判断是否在刷新窗口内
func (r *OpenAITokenRefresher) NeedsRefresh(account *model.Account, refreshWindow time.Duration) bool {
	expiresAt := account.GetOpenAITokenExpiresAt()
	if expiresAt == nil {
		return false
	}

	return time.Until(*expiresAt) < refreshWindow
}

// Refresh 执行token刷新
// 保留原有credentials中的所有字段，只更新token相关字段
func (r *OpenAITokenRefresher) Refresh(ctx context.Context, account *model.Account) (map[string]any, error) {
	tokenInfo, err := r.openaiOAuthService.RefreshAccountToken(ctx, account)
	if err != nil {
		return nil, err
	}

	// 使用服务提供的方法构建新凭证，并保留原有字段
	newCredentials := r.openaiOAuthService.BuildAccountCredentials(tokenInfo)

	// 保留原有credentials中非token相关字段
	for k, v := range account.Credentials {
		if _, exists := newCredentials[k]; !exists {
			newCredentials[k] = v
		}
	}

	return newCredentials, nil
}
