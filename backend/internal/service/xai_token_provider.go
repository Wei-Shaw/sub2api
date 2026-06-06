package service

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"time"
)

const (
	xaiTokenRefreshSkew = 3 * time.Minute
	xaiTokenCacheSkew   = 5 * time.Minute
)

// XAITokenCache token cache interface.
type XAITokenCache = GeminiTokenCache

// XAITokenProvider manages access_token for xAI OAuth accounts.
type XAITokenProvider struct {
	accountRepo     AccountRepository
	tokenCache      XAITokenCache
	xaiOAuthService *XAIOAuthService
	refreshAPI      *OAuthRefreshAPI
	executor        OAuthRefreshExecutor
	refreshPolicy   ProviderRefreshPolicy
}

func NewXAITokenProvider(
	accountRepo AccountRepository,
	tokenCache XAITokenCache,
	xaiOAuthService *XAIOAuthService,
) *XAITokenProvider {
	return &XAITokenProvider{
		accountRepo:     accountRepo,
		tokenCache:      tokenCache,
		xaiOAuthService: xaiOAuthService,
		refreshPolicy:   OpenAIProviderRefreshPolicy(),
	}
}

// SetRefreshAPI injects unified OAuth refresh API and executor.
func (p *XAITokenProvider) SetRefreshAPI(api *OAuthRefreshAPI, executor OAuthRefreshExecutor) {
	p.refreshAPI = api
	p.executor = executor
}

// SetRefreshPolicy injects caller-side refresh policy.
func (p *XAITokenProvider) SetRefreshPolicy(policy ProviderRefreshPolicy) {
	p.refreshPolicy = policy
}

// GetAccessToken returns a valid access_token.
func (p *XAITokenProvider) GetAccessToken(ctx context.Context, account *Account) (string, error) {
	if account == nil {
		return "", errors.New("account is nil")
	}
	if account.Platform != PlatformXAI || account.Type != AccountTypeOAuth {
		return "", errors.New("not an xai oauth account")
	}

	cacheKey := XAITokenCacheKey(account)

	// 1) Try cache first.
	if p.tokenCache != nil {
		if token, err := p.tokenCache.GetAccessToken(ctx, cacheKey); err == nil && strings.TrimSpace(token) != "" {
			slog.Debug("xai_token_cache_hit", "account_id", account.ID)
			return token, nil
		} else if err != nil {
			slog.Warn("xai_token_cache_get_failed", "account_id", account.ID, "error", err)
		}
	}

	// 2) Refresh if needed (pre-expiry skew).
	expiresAt := account.GetCredentialAsTime("expires_at")
	needsRefresh := expiresAt == nil || time.Until(*expiresAt) <= xaiTokenRefreshSkew
	refreshFailed := false

	if needsRefresh && p.refreshAPI != nil && p.executor != nil {
		result, err := p.refreshAPI.RefreshIfNeeded(ctx, account, p.executor, xaiTokenRefreshSkew)
		if err != nil {
			if p.refreshPolicy.OnRefreshError == ProviderRefreshErrorReturn {
				return "", err
			}
			slog.Warn("xai_token_refresh_failed", "account_id", account.ID, "error", err)
			refreshFailed = true
		} else if result.LockHeld {
			if p.refreshPolicy.OnLockHeld == ProviderLockHeldWaitForCache && p.tokenCache != nil {
				if token, cacheErr := p.tokenCache.GetAccessToken(ctx, cacheKey); cacheErr == nil && strings.TrimSpace(token) != "" {
					slog.Debug("xai_token_cache_hit_after_lock", "account_id", account.ID)
					return token, nil
				}
			}
			slog.Debug("xai_token_lock_held_use_old", "account_id", account.ID)
		} else {
			account = result.Account
			expiresAt = account.GetCredentialAsTime("expires_at")
		}
	} else if needsRefresh && p.tokenCache != nil {
		// Backward-compatible test path when refreshAPI is not injected.
		locked, lockErr := p.tokenCache.AcquireRefreshLock(ctx, cacheKey, 30*time.Second)
		if lockErr == nil && locked {
			defer func() { _ = p.tokenCache.ReleaseRefreshLock(ctx, cacheKey) }()
		} else if lockErr != nil {
			slog.Warn("xai_token_lock_failed", "account_id", account.ID, "error", lockErr)
		}
	}

	accessToken := account.GetXAIAccessToken()
	if strings.TrimSpace(accessToken) == "" {
		return "", errors.New("access_token not found in credentials")
	}

	// 3) Populate cache with TTL.
	if p.tokenCache != nil {
		latestAccount, isStale := CheckTokenVersion(ctx, account, p.accountRepo)
		if isStale && latestAccount != nil {
			slog.Debug("xai_token_version_stale_use_latest", "account_id", account.ID)
			accessToken = latestAccount.GetXAIAccessToken()
			if strings.TrimSpace(accessToken) == "" {
				return "", errors.New("access_token not found after version check")
			}
		} else {
			ttl := 30 * time.Minute
			if refreshFailed && p.refreshPolicy.FailureTTL > 0 {
				ttl = p.refreshPolicy.FailureTTL
			} else if expiresAt != nil {
				until := time.Until(*expiresAt)
				switch {
				case until > xaiTokenCacheSkew:
					ttl = until - xaiTokenCacheSkew
				case until > 0:
					ttl = until
				default:
					ttl = time.Minute
				}
			}
			if err := p.tokenCache.SetAccessToken(ctx, cacheKey, accessToken, ttl); err != nil {
				slog.Warn("xai_token_cache_set_failed", "account_id", account.ID, "error", err)
			}
		}
	}

	return accessToken, nil
}

func XAITokenCacheKey(account *Account) string {
	return "xai:account:" + strconv.FormatInt(account.ID, 10)
}
