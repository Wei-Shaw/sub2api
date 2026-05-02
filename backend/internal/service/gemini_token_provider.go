package service

import (
	"context"
	"errors"
	"log"
	"log/slog"
	"strconv"
	"strings"
	"time"
)

const (
	geminiTokenRefreshSkew = 3 * time.Minute
	geminiTokenCacheSkew   = 5 * time.Minute
)

// GeminiTokenProvider manages access_token for Gemini OAuth and Vertex service account accounts.
type GeminiTokenProvider struct {
	accountRepo        AccountRepository
	tokenCache         GeminiTokenCache
	geminiOAuthService *GeminiOAuthService
	refreshAPI         *OAuthRefreshAPI
	executor           OAuthRefreshExecutor
	refreshPolicy      ProviderRefreshPolicy
}

func NewGeminiTokenProvider(
	accountRepo AccountRepository,
	tokenCache GeminiTokenCache,
	geminiOAuthService *GeminiOAuthService,
) *GeminiTokenProvider {
	return &GeminiTokenProvider{
		accountRepo:        accountRepo,
		tokenCache:         tokenCache,
		geminiOAuthService: geminiOAuthService,
		refreshPolicy:      GeminiProviderRefreshPolicy(),
	}
}

// SetRefreshAPI injects unified OAuth refresh API and executor.
func (p *GeminiTokenProvider) SetRefreshAPI(api *OAuthRefreshAPI, executor OAuthRefreshExecutor) {
	p.refreshAPI = api
	p.executor = executor
}

// SetRefreshPolicy injects caller-side refresh policy.
func (p *GeminiTokenProvider) SetRefreshPolicy(policy ProviderRefreshPolicy) {
	p.refreshPolicy = policy
}

func (p *GeminiTokenProvider) GetAccessToken(ctx context.Context, account *Account) (string, error) {
	if account == nil {
		return "", errors.New("account is nil")
	}
	if account.Platform != PlatformGemini || (account.Type != AccountTypeOAuth && account.Type != AccountTypeServiceAccount) {
		return "", errors.New("not a gemini oauth or service account")
	}
	if account.Type == AccountTypeServiceAccount {
		return p.getServiceAccountAccessToken(ctx, account)
	}

	cacheKey := GeminiTokenCacheKey(account)

	// 1) Try cache first.
	if p.tokenCache != nil {
		if token, err := p.tokenCache.GetAccessToken(ctx, cacheKey); err == nil && strings.TrimSpace(token) != "" {
			return token, nil
		}
	}

	// 2) Refresh if needed (pre-expiry skew).
	expiresAt := account.GetCredentialAsTime("expires_at")
	needsRefresh := expiresAt == nil || time.Until(*expiresAt) <= geminiTokenRefreshSkew

	if needsRefresh && p.refreshAPI != nil && p.executor != nil {
		result, err := p.refreshAPI.RefreshIfNeeded(ctx, account, p.executor, geminiTokenRefreshSkew)
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
			slog.Debug("gemini_token_lock_held_use_old", "account_id", account.ID)
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
			slog.Warn("gemini_token_lock_failed", "account_id", account.ID, "error", lockErr)
		}
	}

	accessToken := account.GetCredential("access_token")
	if strings.TrimSpace(accessToken) == "" {
		return "", errors.New("access_token not found in credentials")
	}

	// project_id is optional now:
	// - If present: use Code Assist API (requires project_id)
	// - If absent: use AI Studio API with OAuth token.
	projectID := strings.TrimSpace(account.GetCredential("project_id"))
	autoDetectProjectID := account.GetCredential("auto_detect_project_id") == "true"

	if projectID == "" && autoDetectProjectID {
		if p.geminiOAuthService == nil {
			return accessToken, nil
		}

		var proxyURL string
		if account.ProxyID != nil && p.geminiOAuthService.proxyRepo != nil {
			if proxy, err := p.geminiOAuthService.proxyRepo.GetByID(ctx, *account.ProxyID); err == nil && proxy != nil {
				proxyURL = proxy.URL()
			}
		}

		detected, tierID, err := p.geminiOAuthService.fetchProjectID(ctx, accessToken, proxyURL)
		if err != nil {
			log.Printf("[GeminiTokenProvider] Auto-detect project_id failed: %v, fallback to AI Studio API mode", err)
			return accessToken, nil
		}
		detected = strings.TrimSpace(detected)
		tierID = strings.TrimSpace(tierID)
		if detected != "" {
			if account.Credentials == nil {
				account.Credentials = make(map[string]any)
			}
			account.Credentials["project_id"] = detected
			if tierID != "" {
				account.Credentials["tier_id"] = tierID
			}
			_ = persistAccountCredentials(ctx, p.accountRepo, account, account.Credentials)
		}
	}

	// 3) Populate cache with TTL.
	if p.tokenCache != nil {
		latestAccount, isStale := CheckTokenVersion(ctx, account, p.accountRepo)
		if isStale && latestAccount != nil {
			slog.Debug("gemini_token_version_stale_use_latest", "account_id", account.ID)
			accessToken = latestAccount.GetCredential("access_token")
			if strings.TrimSpace(accessToken) == "" {
				return "", errors.New("access_token not found after version check")
			}
		} else {
			ttl := 30 * time.Minute
			if expiresAt != nil {
				until := time.Until(*expiresAt)
				switch {
				case until > geminiTokenCacheSkew:
					ttl = until - geminiTokenCacheSkew
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

func (p *GeminiTokenProvider) getServiceAccountAccessToken(ctx context.Context, account *Account) (string, error) {
	return getVertexServiceAccountAccessToken(ctx, p.tokenCache, account)
}

func GeminiTokenCacheKey(account *Account) string {
	if account != nil && account.Type == AccountTypeServiceAccount {
		if key, err := parseVertexServiceAccountKey(account); err == nil {
			return vertexServiceAccountCacheKey(account, key)
		}
	}
	projectID := strings.TrimSpace(account.GetCredential("project_id"))
	if projectID != "" {
		return "gemini:" + projectID
	}
	return "gemini:account:" + strconv.FormatInt(account.ID, 10)
}
