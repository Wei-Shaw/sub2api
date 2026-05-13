package service

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"
)

const (
	kiroTokenRefreshSkew = 3 * time.Minute
	kiroTokenCacheSkew   = 5 * time.Minute
)

type KiroTokenCache = GeminiTokenCache

type kiroAccountTokenRefresher interface {
	RefreshAccountToken(ctx context.Context, account *Account) (*KiroTokenInfo, error)
	BuildAccountCredentials(tokenInfo *KiroTokenInfo) map[string]any
}

type KiroTokenProvider struct {
	accountRepo      AccountRepository
	tokenCache       KiroTokenCache
	kiroOAuthService kiroAccountTokenRefresher
	refreshAPI       *OAuthRefreshAPI
	executor         OAuthRefreshExecutor
	refreshPolicy    ProviderRefreshPolicy
}

func NewKiroTokenProvider(
	accountRepo AccountRepository,
	tokenCache KiroTokenCache,
	kiroOAuthService *KiroOAuthService,
) *KiroTokenProvider {
	return &KiroTokenProvider{
		accountRepo:      accountRepo,
		tokenCache:       tokenCache,
		kiroOAuthService: kiroOAuthService,
		refreshPolicy:    GeminiProviderRefreshPolicy(),
	}
}

func (p *KiroTokenProvider) SetRefreshAPI(api *OAuthRefreshAPI, executor OAuthRefreshExecutor) {
	p.refreshAPI = api
	p.executor = executor
}

func (p *KiroTokenProvider) SetRefreshPolicy(policy ProviderRefreshPolicy) {
	p.refreshPolicy = policy
}

func (p *KiroTokenProvider) GetAccessToken(ctx context.Context, account *Account) (string, error) {
	if account == nil {
		return "", errors.New("account is nil")
	}
	if account.Platform != PlatformKiro || account.Type != AccountTypeOAuth {
		return "", errors.New("not a kiro oauth account")
	}

	cacheKey := KiroTokenCacheKey(account)
	if p.tokenCache != nil {
		if token, err := p.tokenCache.GetAccessToken(ctx, cacheKey); err == nil && strings.TrimSpace(token) != "" {
			return token, nil
		}
	}

	expiresAt := account.GetCredentialAsTime("expires_at")
	needsRefresh := expiresAt == nil || time.Until(*expiresAt) <= kiroTokenRefreshSkew

	if needsRefresh && p.refreshAPI != nil && p.executor != nil {
		result, err := p.refreshAPI.RefreshIfNeeded(ctx, account, p.executor, kiroTokenRefreshSkew)
		if err != nil {
			if p.refreshPolicy.OnRefreshError == ProviderRefreshErrorReturn {
				return "", err
			}
		} else if result.LockHeld {
			if p.refreshPolicy.OnLockHeld == ProviderLockHeldWaitForCache && p.tokenCache != nil {
				if token, cacheErr := p.tokenCache.GetAccessToken(ctx, cacheKey); cacheErr == nil && strings.TrimSpace(token) != "" {
					return token, nil
				}
			}
		} else {
			account = result.Account
			expiresAt = account.GetCredentialAsTime("expires_at")
		}
	} else if needsRefresh && p.tokenCache != nil {
		locked, lockErr := p.tokenCache.AcquireRefreshLock(ctx, cacheKey, 30*time.Second)
		if lockErr == nil && locked {
			defer func() { _ = p.tokenCache.ReleaseRefreshLock(ctx, cacheKey) }()
		}
	}

	accessToken := account.GetCredential("access_token")
	if strings.TrimSpace(accessToken) == "" {
		return "", errors.New("access_token not found in credentials")
	}

	if p.tokenCache != nil {
		latestAccount, isStale := CheckTokenVersion(ctx, account, p.accountRepo)
		if isStale && latestAccount != nil {
			accessToken = latestAccount.GetCredential("access_token")
			if strings.TrimSpace(accessToken) == "" {
				return "", errors.New("access_token not found after version check")
			}
		} else {
			ttl := 30 * time.Minute
			if expiresAt != nil {
				until := time.Until(*expiresAt)
				switch {
				case until > kiroTokenCacheSkew:
					ttl = until - kiroTokenCacheSkew
				case until > 0:
					ttl = until
				default:
					ttl = time.Minute
				}
			}
			_ = p.tokenCache.SetAccessToken(ctx, cacheKey, accessToken, ttl)
		}
	}

	return accessToken, nil
}

func KiroTokenCacheKey(account *Account) string {
	if account == nil {
		return "kiro:account:0"
	}
	if clientIDHash := strings.TrimSpace(account.GetCredential("client_id_hash")); clientIDHash != "" {
		return "kiro:" + clientIDHash
	}
	if clientID := strings.TrimSpace(account.GetCredential("client_id")); clientID != "" {
		return "kiro:client:" + clientID
	}
	return "kiro:account:" + strconv.FormatInt(account.ID, 10)
}

func (p *KiroTokenProvider) ForceRefreshAccessToken(ctx context.Context, account *Account) (string, error) {
	if account == nil {
		return "", errors.New("account is nil")
	}
	if account.Platform != PlatformKiro || account.Type != AccountTypeOAuth {
		return "", errors.New("not a kiro oauth account")
	}
	if p.kiroOAuthService == nil {
		return "", errors.New("kiro oauth service is nil")
	}

	cacheKey := KiroTokenCacheKey(account)
	lockHeld := false
	if p.tokenCache != nil {
		locked, lockErr := p.tokenCache.AcquireRefreshLock(ctx, cacheKey, 30*time.Second)
		if lockErr == nil && locked {
			lockHeld = true
			defer func() { _ = p.tokenCache.ReleaseRefreshLock(ctx, cacheKey) }()
		}
	}

	if p.accountRepo != nil {
		if latestAccount, err := p.accountRepo.GetByID(ctx, account.ID); err == nil && latestAccount != nil {
			account = latestAccount
		}
	}

	tokenInfo, err := p.kiroOAuthService.RefreshAccountToken(ctx, account)
	if err != nil {
		if !lockHeld {
			if latestAccount, stale := CheckTokenVersion(ctx, account, p.accountRepo); stale && latestAccount != nil {
				account = latestAccount
				if accessToken := strings.TrimSpace(account.GetCredential("access_token")); accessToken != "" {
					_ = p.cacheAccessToken(ctx, account, accessToken)
					return accessToken, nil
				}
			}
		}
		if isNonRetryableRefreshError(err) && p.accountRepo != nil {
			errorMsg := "Token refresh failed (non-retryable): " + err.Error()
			_ = p.accountRepo.SetError(ctx, account.ID, errorMsg)
		}
		return "", err
	}

	newCredentials := MergeCredentials(account.Credentials, p.kiroOAuthService.BuildAccountCredentials(tokenInfo))
	newCredentials["_token_version"] = time.Now().UnixMilli()
	if err := persistAccountCredentials(ctx, p.accountRepo, account, newCredentials); err != nil {
		return "", err
	}

	accessToken := strings.TrimSpace(account.GetCredential("access_token"))
	if accessToken == "" {
		accessToken = strings.TrimSpace(tokenInfo.AccessToken)
	}
	if accessToken == "" {
		return "", errors.New("access_token not found after kiro refresh")
	}
	if err := p.cacheAccessToken(ctx, account, accessToken); err != nil {
		return "", err
	}
	return accessToken, nil
}

func (p *KiroTokenProvider) cacheAccessToken(ctx context.Context, account *Account, accessToken string) error {
	if p.tokenCache == nil || account == nil || strings.TrimSpace(accessToken) == "" {
		return nil
	}
	ttl := 30 * time.Minute
	if expiresAt := account.GetCredentialAsTime("expires_at"); expiresAt != nil {
		until := time.Until(*expiresAt)
		switch {
		case until > kiroTokenCacheSkew:
			ttl = until - kiroTokenCacheSkew
		case until > 0:
			ttl = until
		default:
			ttl = time.Minute
		}
	}
	return p.tokenCache.SetAccessToken(ctx, KiroTokenCacheKey(account), accessToken, ttl)
}
