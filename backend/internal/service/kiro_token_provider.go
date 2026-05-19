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
	kiroTokenRefreshSkew      = 3 * time.Minute
	kiroTokenCacheSkew        = 5 * time.Minute
	kiroRequestRefreshTimeout = 8 * time.Second
)

// KiroTokenCache reuses the same shape as the other providers
// (GeminiTokenCache is the canonical alias).
type KiroTokenCache = GeminiTokenCache

// KiroTokenProvider manages access_token for kiro accounts on the gateway
// hot path. Mirrors AntigravityTokenProvider's shape so the surrounding
// scheduler / failover machinery doesn't need provider-specific knowledge.
type KiroTokenProvider struct {
	accountRepo      AccountRepository
	tokenCache       KiroTokenCache
	kiroOAuthService *KiroOAuthService
	refreshAPI       *OAuthRefreshAPI
	executor         OAuthRefreshExecutor
	refreshPolicy    ProviderRefreshPolicy
	tempUnschedCache TempUnschedCache
	// markedTempUnsched dedupes temp-unsched writes per account.
	markedTempUnsched sync.Map
}

// NewKiroTokenProvider constructs the provider with default refresh policy.
func NewKiroTokenProvider(
	accountRepo AccountRepository,
	tokenCache KiroTokenCache,
	kiroOAuthService *KiroOAuthService,
) *KiroTokenProvider {
	return &KiroTokenProvider{
		accountRepo:      accountRepo,
		tokenCache:       tokenCache,
		kiroOAuthService: kiroOAuthService,
		refreshPolicy:    KiroProviderRefreshPolicy(),
	}
}

// SetRefreshAPI wires in the shared OAuthRefreshAPI used for distributed
// lock + dedupe.
func (p *KiroTokenProvider) SetRefreshAPI(api *OAuthRefreshAPI, executor OAuthRefreshExecutor) {
	p.refreshAPI = api
	p.executor = executor
}

// SetRefreshPolicy overrides the default policy (used by tests).
func (p *KiroTokenProvider) SetRefreshPolicy(policy ProviderRefreshPolicy) {
	p.refreshPolicy = policy
}

// SetTempUnschedCache injects the Redis temp-unsched cache so a request-path
// refresh failure marks the account unschedulable immediately.
func (p *KiroTokenProvider) SetTempUnschedCache(cache TempUnschedCache) {
	p.tempUnschedCache = cache
}

// GetAccessToken returns a non-expired access token for the account.
// Refreshes if within kiroTokenRefreshSkew of expiry.
func (p *KiroTokenProvider) GetAccessToken(ctx context.Context, account *Account) (string, error) {
	if account == nil {
		return "", errors.New("account is nil")
	}
	if account.Platform != PlatformKiro {
		return "", errors.New("not a kiro account")
	}
	if account.Type != AccountTypeOAuth {
		return "", errors.New("not a kiro oauth account")
	}

	cacheKey := KiroTokenCacheKey(account)

	// 1) Cache fast-path.
	if p.tokenCache != nil {
		if tok, err := p.tokenCache.GetAccessToken(ctx, cacheKey); err == nil && strings.TrimSpace(tok) != "" {
			return tok, nil
		}
	}

	// 2) Refresh if needed.
	expiresAt := account.GetCredentialAsTime("expires_at")
	needsRefresh := expiresAt == nil || time.Until(*expiresAt) <= kiroTokenRefreshSkew
	if needsRefresh && p.refreshAPI != nil && p.executor != nil {
		refreshCtx, cancel := context.WithTimeout(ctx, kiroRequestRefreshTimeout)
		defer cancel()
		result, err := p.refreshAPI.RefreshIfNeeded(refreshCtx, account, p.executor, kiroTokenRefreshSkew)
		if err != nil {
			p.markTempUnschedulable(account, err)
			if p.refreshPolicy.OnRefreshError == ProviderRefreshErrorReturn {
				return "", err
			}
		} else if result.LockHeld {
			if p.refreshPolicy.OnLockHeld == ProviderLockHeldWaitForCache && p.tokenCache != nil {
				if tok, cerr := p.tokenCache.GetAccessToken(ctx, cacheKey); cerr == nil && strings.TrimSpace(tok) != "" {
					return tok, nil
				}
			}
		} else {
			account = result.Account
			expiresAt = account.GetCredentialAsTime("expires_at")
		}
	} else if needsRefresh && p.tokenCache != nil {
		// Test path: refreshAPI not wired. Honor the cache lock so concurrent
		// callers don't all hit upstream.
		locked, err := p.tokenCache.AcquireRefreshLock(ctx, cacheKey, 30*time.Second)
		if err == nil && locked {
			defer func() { _ = p.tokenCache.ReleaseRefreshLock(ctx, cacheKey) }()
		}
	}

	accessToken := account.GetCredential("access_token")
	if strings.TrimSpace(accessToken) == "" {
		return "", errors.New("access_token not found in credentials")
	}

	// 3) Repopulate cache with TTL based on real expiry.
	if p.tokenCache != nil {
		latestAccount, isStale := CheckTokenVersion(ctx, account, p.accountRepo)
		if isStale && latestAccount != nil {
			slog.Debug("kiro_token_version_stale_use_latest", "account_id", account.ID)
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

// markTempUnschedulable mirrors antigravity's behaviour: on request-path
// refresh failure, write a temp-unsched record (DB + Redis) so the scheduler
// stops picking this account while the background refresher keeps retrying.
func (p *KiroTokenProvider) markTempUnschedulable(account *Account, refreshErr error) {
	if p.accountRepo == nil || account == nil {
		return
	}
	if _, loaded := p.markedTempUnsched.LoadOrStore(account.ID, struct{}{}); loaded {
		// Already marked in this process recently; let the scheduler handle the rest.
		return
	}
	defer func() {
		// Re-enable marking after one refresh cycle.
		go func(id int64) {
			time.Sleep(time.Minute)
			p.markedTempUnsched.Delete(id)
		}(account.ID)
	}()

	now := time.Now()
	until := now.Add(tokenRefreshTempUnschedDuration)
	reason := "token refresh failed on request path: " + refreshErr.Error()
	bgCtx := context.Background()
	if err := p.accountRepo.SetTempUnschedulable(bgCtx, account.ID, until, reason); err != nil {
		slog.Warn("kiro_token_provider.set_temp_unschedulable_failed",
			"account_id", account.ID,
			"error", err,
		)
		return
	}
	slog.Warn("kiro_token_provider.temp_unschedulable_set",
		"account_id", account.ID,
		"until", until.Format(time.RFC3339),
		"reason", reason,
	)
	if p.tempUnschedCache != nil {
		state := &TempUnschedState{
			UntilUnix:       until.Unix(),
			TriggeredAtUnix: now.Unix(),
			ErrorMessage:    reason,
		}
		if err := p.tempUnschedCache.SetTempUnsched(bgCtx, account.ID, state); err != nil {
			slog.Warn("kiro_token_provider.temp_unsched_cache_set_failed",
				"account_id", account.ID,
				"error", err,
			)
		}
	}
}

// KiroTokenCacheKey is the redis/local cache key for a Kiro access token.
func KiroTokenCacheKey(account *Account) string {
	return "kiro:account:" + strconv.FormatInt(account.ID, 10)
}

// KiroProviderRefreshPolicy is the default refresh policy for kiro.
// Same shape as Antigravity: return error on refresh failure, wait for
// cache when another worker holds the lock.
func KiroProviderRefreshPolicy() ProviderRefreshPolicy {
	return ProviderRefreshPolicy{
		OnRefreshError: ProviderRefreshErrorReturn,
		OnLockHeld:     ProviderLockHeldWaitForCache,
	}
}
