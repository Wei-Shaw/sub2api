package service

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	antigravityTokenRefreshSkew = 3 * time.Minute
	antigravityTokenCacheSkew   = 5 * time.Minute
	antigravityBackfillCooldown = 5 * time.Minute
)

// AntigravityTokenCache token cache interface.
type AntigravityTokenCache = GeminiTokenCache

// AntigravityTokenProvider manages access_token for antigravity accounts.
type AntigravityTokenProvider struct {
	accountRepo             AccountRepository
	tokenCache              AntigravityTokenCache
	antigravityOAuthService *AntigravityOAuthService
	backfillCooldown        sync.Map // key: accountID -> last attempt time
	refreshAPI              *OAuthRefreshAPI
	executor                OAuthRefreshExecutor
	refreshPolicy           ProviderRefreshPolicy
}

func NewAntigravityTokenProvider(
	accountRepo AccountRepository,
	tokenCache AntigravityTokenCache,
	antigravityOAuthService *AntigravityOAuthService,
) *AntigravityTokenProvider {
	return &AntigravityTokenProvider{
		accountRepo:             accountRepo,
		tokenCache:              tokenCache,
		antigravityOAuthService: antigravityOAuthService,
		refreshPolicy:           AntigravityProviderRefreshPolicy(),
	}
}

// SetRefreshAPI injects unified OAuth refresh API and executor.
func (p *AntigravityTokenProvider) SetRefreshAPI(api *OAuthRefreshAPI, executor OAuthRefreshExecutor) {
	p.refreshAPI = api
	p.executor = executor
}

// SetRefreshPolicy injects caller-side refresh policy.
func (p *AntigravityTokenProvider) SetRefreshPolicy(policy ProviderRefreshPolicy) {
	p.refreshPolicy = policy
}

// GetAccessToken returns a valid access_token.
func (p *AntigravityTokenProvider) GetAccessToken(ctx context.Context, account *Account) (string, error) {
	if account == nil {
		return "", errors.New("account is nil")
	}
	if account.Platform != PlatformAntigravity {
		return "", errors.New("not an antigravity account")
	}

	// upstream accounts use static api_key and never refresh oauth token.
	if account.Type == AccountTypeUpstream {
		apiKey := account.GetCredential("api_key")
		if apiKey == "" {
			return "", errors.New("upstream account missing api_key in credentials")
		}
		return apiKey, nil
	}
	if account.Type != AccountTypeOAuth {
		return "", errors.New("not an antigravity oauth account")
	}

	cacheKey := AntigravityTokenCacheKey(account)

	// 1) Try cache first.
	if p.tokenCache != nil {
		if token, err := p.tokenCache.GetAccessToken(ctx, cacheKey); err == nil && strings.TrimSpace(token) != "" {
			return token, nil
		}
	}

	// 2) Refresh if needed (pre-expiry skew).
	expiresAt := account.GetCredentialAsTime("expires_at")
	needsRefresh := expiresAt == nil || time.Until(*expiresAt) <= antigravityTokenRefreshSkew
	if needsRefresh && p.refreshAPI != nil && p.executor != nil {
		result, err := p.refreshAPI.RefreshIfNeeded(ctx, account, p.executor, antigravityTokenRefreshSkew)
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
			// default policy: continue with existing token.
		} else {
			account = result.Account
			expiresAt = account.GetCredentialAsTime("expires_at")
		}
	} else if needsRefresh && p.tokenCache != nil {
		// Backward-compatible test path when refreshAPI is not injected.
		locked, err := p.tokenCache.AcquireRefreshLock(ctx, cacheKey, 30*time.Second)
		if err == nil && locked {
			defer func() { _ = p.tokenCache.ReleaseRefreshLock(ctx, cacheKey) }()
		}
	}

	accessToken := account.GetCredential("access_token")
	if strings.TrimSpace(accessToken) == "" {
		return "", errors.New("access_token not found in credentials")
	}

	// Backfill project_id online when missing, with cooldown to avoid hammering.
	if strings.TrimSpace(account.GetCredential("project_id")) == "" && p.antigravityOAuthService != nil {
		if p.shouldAttemptBackfill(account.ID) {
			p.markBackfillAttempted(account.ID)
			if projectID, err := p.antigravityOAuthService.FillProjectID(ctx, account, accessToken); err == nil && projectID != "" {
				account.Credentials["project_id"] = projectID
				if updateErr := p.accountRepo.Update(ctx, account); updateErr != nil {
					slog.Warn("antigravity_project_id_backfill_persist_failed",
						"account_id", account.ID,
						"error", updateErr,
					)
				}
			}
		}
	}

	// 3) Populate cache with TTL.
	if p.tokenCache != nil {
		latestAccount, isStale := CheckTokenVersion(ctx, account, p.accountRepo)
		if isStale && latestAccount != nil {
			slog.Debug("antigravity_token_version_stale_use_latest", "account_id", account.ID)
			accessToken = latestAccount.GetCredential("access_token")
			if strings.TrimSpace(accessToken) == "" {
				return "", errors.New("access_token not found after version check")
			}
		} else {
			ttl := 30 * time.Minute
			if expiresAt != nil {
				until := time.Until(*expiresAt)
				switch {
				case until > antigravityTokenCacheSkew:
					ttl = until - antigravityTokenCacheSkew
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

// shouldAttemptBackfill checks backfill cooldown.
func (p *AntigravityTokenProvider) shouldAttemptBackfill(accountID int64) bool {
	if v, ok := p.backfillCooldown.Load(accountID); ok {
		if lastAttempt, ok := v.(time.Time); ok {
			return time.Since(lastAttempt) > antigravityBackfillCooldown
		}
	}
	return true
}

func (p *AntigravityTokenProvider) markBackfillAttempted(accountID int64) {
	p.backfillCooldown.Store(accountID, time.Now())
}

func AntigravityTokenCacheKey(account *Account) string {
	projectID := strings.TrimSpace(account.GetCredential("project_id"))
	if projectID != "" {
		return "ag:" + projectID
	}
	return "ag:account:" + strconv.FormatInt(account.ID, 10)
}
