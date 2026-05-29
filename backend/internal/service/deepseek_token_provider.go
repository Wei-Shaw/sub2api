package service

import (
	"context"
	"errors"
	"strings"
)

var ErrDeepSeekOAuthNotImplemented = errors.New("deepseek OAuth not implemented")

// DeepSeekTokenProvider 管理 DeepSeek 账号访问凭证。
// 当前仅支持 API Key 模式，OAuth 模式预留接口。
type DeepSeekTokenProvider struct{}

func NewDeepSeekTokenProvider() *DeepSeekTokenProvider {
	return &DeepSeekTokenProvider{}
}

// GetAccessToken 返回 DeepSeek 账号的访问令牌。
// API Key 模式：直接返回 credentials.api_key。
// OAuth 模式：预留，返回 ErrDeepSeekOAuthNotImplemented。
func (p *DeepSeekTokenProvider) GetAccessToken(ctx context.Context, account *Account) (string, error) {
	if account == nil {
		return "", errors.New("account is nil")
	}
	if account.Platform != PlatformDeepSeek {
		return "", errors.New("not a deepseek account")
	}

	if account.Type == AccountTypeAPIKey {
		token := account.GetCredential("api_key")
		if strings.TrimSpace(token) == "" {
			return "", errors.New("api_key not found in credentials")
		}
		return token, nil
	}

	return "", ErrDeepSeekOAuthNotImplemented
}
