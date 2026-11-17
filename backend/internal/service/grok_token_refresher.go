package service

import (
	"context"
	"errors"
	"hash/fnv"
	"strings"
	"time"
)

// Base warm window: refresh when access token lifetime remaining is below this.
// Grok access tokens are typically ~1h; refreshing up to 1h early keeps the pool
// warm for request path cache misses.
const grokTokenRefreshSkew = time.Hour

// Stampede spread: each account's effective warm window is reduced by a
// deterministic offset in [0, grokTokenRefreshJitterMax] so co-imported accounts
// do not all refresh in the same TokenRefreshService cycle (grok2api-style
// RefreshDueAt scatter).
const grokTokenRefreshJitterMax = 3 * time.Minute

// Floor so jitter cannot shrink the window below a useful threshold.
const grokTokenRefreshSkewMin = 30 * time.Minute

type GrokTokenRefresher struct {
	grokOAuthService GrokOAuthTokenService
}

func NewGrokTokenRefresher(grokOAuthService GrokOAuthTokenService) *GrokTokenRefresher {
	return &GrokTokenRefresher{grokOAuthService: grokOAuthService}
}

func (r *GrokTokenRefresher) CacheKey(account *Account) string {
	return GrokTokenCacheKey(account)
}

func (r *GrokTokenRefresher) CanRefresh(account *Account) bool {
	return account != nil && account.Platform == PlatformGrok && account.Type == AccountTypeOAuth &&
		strings.TrimSpace(account.GetGrokRefreshToken()) != ""
}

func (r *GrokTokenRefresher) NeedsRefresh(account *Account, refreshWindow time.Duration) bool {
	if account == nil || strings.TrimSpace(account.GetGrokRefreshToken()) == "" {
		return false
	}
	if strings.TrimSpace(account.GetGrokAccessToken()) == "" {
		return true
	}
	expiresAt := account.GetCredentialAsTime("expires_at")
	if expiresAt == nil {
		return true
	}
	if refreshWindow < grokTokenRefreshSkew {
		refreshWindow = grokTokenRefreshSkew
	}
	// Deterministic per-account jitter: spread warm refreshes without random
	// non-determinism in tests (hash of account id).
	refreshWindow = grokTokenRefreshWindowWithJitter(account.ID, refreshWindow)
	return time.Until(*expiresAt) < refreshWindow
}

// grokTokenRefreshWindowWithJitter returns refreshWindow minus a stable offset
// in [0, jitterMax] based on accountID. Result is never below grokTokenRefreshSkewMin
// when the base window is at least that large.
func grokTokenRefreshWindowWithJitter(accountID int64, refreshWindow time.Duration) time.Duration {
	if accountID <= 0 || refreshWindow <= grokTokenRefreshSkewMin {
		return refreshWindow
	}
	h := fnv.New32a()
	var b [8]byte
	id := uint64(accountID)
	for i := 0; i < 8; i++ {
		b[i] = byte(id >> (8 * i))
	}
	_, _ = h.Write(b[:])
	// Jitter in [0, grokTokenRefreshJitterMax).
	jitter := time.Duration(h.Sum32()%uint32(grokTokenRefreshJitterMax/time.Second)) * time.Second
	out := refreshWindow - jitter
	if out < grokTokenRefreshSkewMin {
		return grokTokenRefreshSkewMin
	}
	return out
}

func (r *GrokTokenRefresher) Refresh(ctx context.Context, account *Account) (map[string]any, error) {
	if r == nil || r.grokOAuthService == nil {
		return nil, errors.New("grok oauth service is not configured")
	}
	tokenInfo, err := r.grokOAuthService.RefreshAccountToken(ctx, account)
	if err != nil {
		return nil, err
	}
	newCredentials := r.grokOAuthService.BuildAccountCredentials(tokenInfo)
	newCredentials = MergeCredentials(account.Credentials, newCredentials)
	if baseURL := strings.TrimSpace(account.GetCredential("base_url")); baseURL != "" {
		newCredentials["base_url"] = baseURL
	}
	return newCredentials, nil
}
