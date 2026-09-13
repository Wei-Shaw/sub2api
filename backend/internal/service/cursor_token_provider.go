package service

import (
	"context"
	"errors"
	"strings"
	"time"
)

type CursorTokenCache = GeminiTokenCache

type CursorTokenProvider struct {
	accountRepo   AccountRepository
	tokenCache    CursorTokenCache
	refreshAPI    *OAuthRefreshAPI
	executor      OAuthRefreshExecutor
	refreshPolicy ProviderRefreshPolicy
}

func NewCursorTokenProvider(accountRepo AccountRepository, tokenCache CursorTokenCache) *CursorTokenProvider {
	return &CursorTokenProvider{
		accountRepo:   accountRepo,
		tokenCache:    tokenCache,
		refreshPolicy: AntigravityProviderRefreshPolicy(),
	}
}

func (p *CursorTokenProvider) SetRefreshAPI(api *OAuthRefreshAPI, executor OAuthRefreshExecutor) {
	p.refreshAPI = api
	p.executor = executor
}

func (p *CursorTokenProvider) GetAccessToken(ctx context.Context, account *Account) (string, error) {
	if account == nil {
		return "", errors.New("account is nil")
	}
	if !account.IsCursorOAuth() {
		return "", errors.New("not a cursor oauth account")
	}
	accessToken := strings.TrimSpace(account.GetCursorAccessToken())
	if accessToken == "" {
		return "", errors.New("cursor access token is missing")
	}
	expiresAt := account.GetCredentialAsTime("expires_at")
	if expiresAt != nil && time.Until(*expiresAt) > cursorTokenRefreshSkew {
		if p.tokenCache != nil {
			_ = p.tokenCache.SetAccessToken(ctx, CursorTokenCacheKey(account), accessToken, time.Until(*expiresAt))
		}
		return accessToken, nil
	}
	if p.refreshAPI == nil || p.executor == nil {
		if expiresAt != nil && time.Now().Before(*expiresAt) {
			return accessToken, nil
		}
		return "", errors.New("cursor oauth refresh is not configured")
	}
	result, err := p.refreshAPI.RefreshIfNeeded(ctx, account, p.executor, cursorTokenRefreshSkew)
	if err != nil {
		if expiresAt != nil && time.Now().Before(*expiresAt) {
			return accessToken, nil
		}
		return "", err
	}
	if result != nil && result.Account != nil {
		account = result.Account
	}
	token := strings.TrimSpace(account.GetCursorAccessToken())
	if token == "" {
		return "", errors.New("cursor access token is missing after refresh")
	}
	return token, nil
}
