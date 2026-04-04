package service

import (
	"context"
	"errors"
	"log/slog"
	"math/rand/v2"
	"strings"
	"sync/atomic"
	"time"
)

const (
	openAITokenRefreshSkew    = 3 * time.Minute
	openAITokenCacheSkew      = 5 * time.Minute
	openAILockInitialWait     = 20 * time.Millisecond
	openAILockMaxWait         = 120 * time.Millisecond
	openAILockMaxAttempts     = 5
	openAILockJitterRatio     = 0.2
	openAILockWarnThresholdMs = 250
)

// OpenAITokenRuntimeMetrics is a snapshot of refresh and lock contention metrics.
type OpenAITokenRuntimeMetrics struct {
	RefreshRequests    int64
	RefreshSuccess     int64
	RefreshFailure     int64
	LockAcquireFailure int64
	LockContention     int64
	LockWaitSamples    int64
	LockWaitTotalMs    int64
	LockWaitHit        int64
	LockWaitMiss       int64
	LastObservedUnixMs int64
}

type openAITokenRuntimeMetricsStore struct {
	refreshRequests    atomic.Int64
	refreshSuccess     atomic.Int64
	refreshFailure     atomic.Int64
	lockAcquireFailure atomic.Int64
	lockContention     atomic.Int64
	lockWaitSamples    atomic.Int64
	lockWaitTotalMs    atomic.Int64
	lockWaitHit        atomic.Int64
	lockWaitMiss       atomic.Int64
	lastObservedUnixMs atomic.Int64
}

func (m *openAITokenRuntimeMetricsStore) snapshot() OpenAITokenRuntimeMetrics {
	if m == nil {
		return OpenAITokenRuntimeMetrics{}
	}
	return OpenAITokenRuntimeMetrics{
		RefreshRequests:    m.refreshRequests.Load(),
		RefreshSuccess:     m.refreshSuccess.Load(),
		RefreshFailure:     m.refreshFailure.Load(),
		LockAcquireFailure: m.lockAcquireFailure.Load(),
		LockContention:     m.lockContention.Load(),
		LockWaitSamples:    m.lockWaitSamples.Load(),
		LockWaitTotalMs:    m.lockWaitTotalMs.Load(),
		LockWaitHit:        m.lockWaitHit.Load(),
		LockWaitMiss:       m.lockWaitMiss.Load(),
		LastObservedUnixMs: m.lastObservedUnixMs.Load(),
	}
}

func (m *openAITokenRuntimeMetricsStore) touchNow() {
	if m == nil {
		return
	}
	m.lastObservedUnixMs.Store(time.Now().UnixMilli())
}

// OpenAITokenCache token cache interface.
type OpenAITokenCache = GeminiTokenCache

// OpenAITokenProvider manages access_token for OpenAI OAuth accounts.
type OpenAITokenProvider struct {
	accountRepo        AccountRepository
	tokenCache         OpenAITokenCache
	openAIOAuthService *OpenAIOAuthService
	metrics            *openAITokenRuntimeMetricsStore
	refreshAPI         *OAuthRefreshAPI
	executor           OAuthRefreshExecutor
	refreshPolicy      ProviderRefreshPolicy
}

func NewOpenAITokenProvider(
	accountRepo AccountRepository,
	tokenCache OpenAITokenCache,
	openAIOAuthService *OpenAIOAuthService,
) *OpenAITokenProvider {
	return &OpenAITokenProvider{
		accountRepo:        accountRepo,
		tokenCache:         tokenCache,
		openAIOAuthService: openAIOAuthService,
		metrics:            &openAITokenRuntimeMetricsStore{},
		refreshPolicy:      OpenAIProviderRefreshPolicy(),
	}
}

// SetRefreshAPI injects unified OAuth refresh API and executor.
func (p *OpenAITokenProvider) SetRefreshAPI(api *OAuthRefreshAPI, executor OAuthRefreshExecutor) {
	p.refreshAPI = api
	p.executor = executor
}

// SetRefreshPolicy injects caller-side refresh policy.
func (p *OpenAITokenProvider) SetRefreshPolicy(policy ProviderRefreshPolicy) {
	p.refreshPolicy = policy
}

func (p *OpenAITokenProvider) SnapshotRuntimeMetrics() OpenAITokenRuntimeMetrics {
	if p == nil {
		return OpenAITokenRuntimeMetrics{}
	}
	p.ensureMetrics()
	return p.metrics.snapshot()
}

func (p *OpenAITokenProvider) ensureMetrics() {
	if p != nil && p.metrics == nil {
		p.metrics = &openAITokenRuntimeMetricsStore{}
	}
}

// GetAccessToken returns a valid access_token.
func (p *OpenAITokenProvider) GetAccessToken(ctx context.Context, account *Account) (string, error) {
	p.ensureMetrics()
	if account == nil {
		return "", errors.New("account is nil")
	}
	if account.Platform != PlatformOpenAI || account.Type != AccountTypeOAuth {
		return "", errors.New("not an openai oauth account")
	}

	cacheKey := OpenAITokenCacheKey(account)

	// 1) Try cache first.
	if p.tokenCache != nil {
		if token, err := p.tokenCache.GetAccessToken(ctx, cacheKey); err == nil && strings.TrimSpace(token) != "" {
			slog.Debug("openai_token_cache_hit", "account_id", account.ID)
			return token, nil
		} else if err != nil {
			slog.Warn("openai_token_cache_get_failed", "account_id", account.ID, "error", err)
		}
	}

	slog.Debug("openai_token_cache_miss", "account_id", account.ID)

	// 2) Refresh if needed (pre-expiry skew).
	expiresAt := account.GetCredentialAsTime("expires_at")
	needsRefresh := expiresAt == nil || time.Until(*expiresAt) <= openAITokenRefreshSkew
	refreshFailed := false

	if needsRefresh && p.refreshAPI != nil && p.executor != nil {
		p.metrics.refreshRequests.Add(1)
		p.metrics.touchNow()

		result, err := p.refreshAPI.RefreshIfNeeded(ctx, account, p.executor, openAITokenRefreshSkew)
		if err != nil {
			if p.refreshPolicy.OnRefreshError == ProviderRefreshErrorReturn {
				return "", err
			}
			slog.Warn("openai_token_refresh_failed", "account_id", account.ID, "error", err)
			p.metrics.refreshFailure.Add(1)
			refreshFailed = true
		} else if result.LockHeld {
			if p.refreshPolicy.OnLockHeld == ProviderLockHeldWaitForCache {
				p.metrics.lockContention.Add(1)
				p.metrics.touchNow()
				token, waitErr := p.waitForTokenAfterLockRace(ctx, cacheKey)
				if waitErr != nil {
					return "", waitErr
				}
				if strings.TrimSpace(token) != "" {
					slog.Debug("openai_token_cache_hit_after_wait", "account_id", account.ID)
					return token, nil
				}
			}
		} else if result.Refreshed {
			p.metrics.refreshSuccess.Add(1)
			account = result.Account
			expiresAt = account.GetCredentialAsTime("expires_at")
		} else {
			account = result.Account
			expiresAt = account.GetCredentialAsTime("expires_at")
		}
	} else if needsRefresh && p.tokenCache != nil {
		// Backward-compatible test path when refreshAPI is not injected.
		p.metrics.refreshRequests.Add(1)
		p.metrics.touchNow()
		locked, lockErr := p.tokenCache.AcquireRefreshLock(ctx, cacheKey, 30*time.Second)
		if lockErr == nil && locked {
			defer func() { _ = p.tokenCache.ReleaseRefreshLock(ctx, cacheKey) }()
		} else if lockErr != nil {
			p.metrics.lockAcquireFailure.Add(1)
			p.metrics.touchNow()
			slog.Warn("openai_token_lock_failed", "account_id", account.ID, "error", lockErr)
		} else {
			p.metrics.lockContention.Add(1)
			p.metrics.touchNow()
			token, waitErr := p.waitForTokenAfterLockRace(ctx, cacheKey)
			if waitErr != nil {
				return "", waitErr
			}
			if strings.TrimSpace(token) != "" {
				slog.Debug("openai_token_cache_hit_after_wait", "account_id", account.ID)
				return token, nil
			}
		}
	}

	accessToken := account.GetCredential("access_token")
	if strings.TrimSpace(accessToken) == "" {
		return "", errors.New("access_token not found in credentials")
	}

	// 3) Populate cache with TTL.
	if p.tokenCache != nil {
		latestAccount, isStale := CheckTokenVersion(ctx, account, p.accountRepo)
		if isStale && latestAccount != nil {
			slog.Debug("openai_token_version_stale_use_latest", "account_id", account.ID)
			accessToken = latestAccount.GetOpenAIAccessToken()
			if strings.TrimSpace(accessToken) == "" {
				return "", errors.New("access_token not found after version check")
			}
		} else {
			ttl := 30 * time.Minute
			if refreshFailed {
				if p.refreshPolicy.FailureTTL > 0 {
					ttl = p.refreshPolicy.FailureTTL
				} else {
					ttl = time.Minute
				}
				slog.Debug("openai_token_cache_short_ttl", "account_id", account.ID, "reason", "refresh_failed")
			} else if expiresAt != nil {
				until := time.Until(*expiresAt)
				switch {
				case until > openAITokenCacheSkew:
					ttl = until - openAITokenCacheSkew
				case until > 0:
					ttl = until
				default:
					ttl = time.Minute
				}
			}
			if err := p.tokenCache.SetAccessToken(ctx, cacheKey, accessToken, ttl); err != nil {
				slog.Warn("openai_token_cache_set_failed", "account_id", account.ID, "error", err)
			}
		}
	}

	return accessToken, nil
}

func (p *OpenAITokenProvider) waitForTokenAfterLockRace(ctx context.Context, cacheKey string) (string, error) {
	wait := openAILockInitialWait
	totalWaitMs := int64(0)
	for i := 0; i < openAILockMaxAttempts; i++ {
		actualWait := jitterLockWait(wait)
		timer := time.NewTimer(actualWait)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return "", ctx.Err()
		case <-timer.C:
		}

		waitMs := actualWait.Milliseconds()
		if waitMs < 0 {
			waitMs = 0
		}
		totalWaitMs += waitMs
		p.metrics.lockWaitSamples.Add(1)
		p.metrics.lockWaitTotalMs.Add(waitMs)
		p.metrics.touchNow()

		token, err := p.tokenCache.GetAccessToken(ctx, cacheKey)
		if err == nil && strings.TrimSpace(token) != "" {
			p.metrics.lockWaitHit.Add(1)
			if totalWaitMs >= openAILockWarnThresholdMs {
				slog.Warn("openai_token_lock_wait_high", "wait_ms", totalWaitMs, "attempts", i+1)
			}
			return token, nil
		}

		if wait < openAILockMaxWait {
			wait *= 2
			if wait > openAILockMaxWait {
				wait = openAILockMaxWait
			}
		}
	}

	p.metrics.lockWaitMiss.Add(1)
	if totalWaitMs >= openAILockWarnThresholdMs {
		slog.Warn("openai_token_lock_wait_high", "wait_ms", totalWaitMs, "attempts", openAILockMaxAttempts)
	}
	return "", nil
}

func jitterLockWait(base time.Duration) time.Duration {
	if base <= 0 {
		return 0
	}
	minFactor := 1 - openAILockJitterRatio
	maxFactor := 1 + openAILockJitterRatio
	factor := minFactor + rand.Float64()*(maxFactor-minFactor)
	return time.Duration(float64(base) * factor)
}
