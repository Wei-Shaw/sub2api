//go:build unit

package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/stretchr/testify/require"
)

type grokTokenCacheForProviderTest struct {
	token        string
	setKey       string
	setToken     string
	setTTL       time.Duration
	lockResult   bool
	releaseCalls int
	deletedKeys  []string
	deleteErr    error
	getCalls     int
	mu           sync.Mutex
}

type grokCredentialRaceRepo struct {
	*tokenRefreshAccountRepo
	mu sync.RWMutex
}

func (r *grokCredentialRaceRepo) GetByID(ctx context.Context, id int64) (*Account, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.tokenRefreshAccountRepo.GetByID(ctx, id)
}

func (r *grokCredentialRaceRepo) setAccount(account *Account) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.accountsByID[account.ID] = account
}

func (c *grokTokenCacheForProviderTest) GetAccessToken(context.Context, string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.getCalls++
	if c.token == "" {
		return "", errors.New("not cached")
	}
	return c.token, nil
}

func (c *grokTokenCacheForProviderTest) SetAccessToken(_ context.Context, key string, token string, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.setKey = key
	c.setToken = token
	c.setTTL = ttl
	return nil
}

func (c *grokTokenCacheForProviderTest) DeleteAccessToken(_ context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.deletedKeys = append(c.deletedKeys, key)
	return c.deleteErr
}

func (c *grokTokenCacheForProviderTest) AcquireRefreshLock(context.Context, string, time.Duration) (bool, error) {
	return c.lockResult, nil
}

func (c *grokTokenCacheForProviderTest) ReleaseRefreshLock(context.Context, string) error {
	c.releaseCalls++
	return nil
}

func TestGrokTokenProviderRefreshesExpiredTokenOnRequestPath(t *testing.T) {
	t.Setenv(xai.EnvBaseURL, xai.DefaultCLIBaseURL)

	expiredAt := time.Now().Add(-time.Minute).UTC().Format(time.RFC3339)
	account := &Account{
		ID:          54,
		Platform:    PlatformGrok,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Credentials: map[string]any{
			"access_token":  "expired-access-token",
			"refresh_token": "refresh-token",
			"expires_at":    expiredAt,
			"base_url":      xai.DefaultCLIBaseURL,
			"client_id":     "client-id",
		},
	}
	repo := &tokenRefreshAccountRepo{}
	repo.accountsByID = map[int64]*Account{54: account}
	cache := &grokTokenCacheForProviderTest{lockResult: true}
	oauthSvc := NewGrokOAuthService(nil, &grokOAuthClientStub{
		refreshResponse: &xai.TokenResponse{
			AccessToken: "new-access-token",
			TokenType:   "Bearer",
			ExpiresIn:   3600,
		},
	})
	defer oauthSvc.Stop()

	provider := NewGrokTokenProvider(repo, cache)
	provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), NewGrokTokenRefresher(oauthSvc))

	token, err := provider.GetAccessToken(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, "new-access-token", token)
	require.Equal(t, 1, repo.updateCredentialsCalls)
	require.Equal(t, "new-access-token", repo.accountsByID[54].GetGrokAccessToken())
	require.Equal(t, "refresh-token", repo.accountsByID[54].GetGrokRefreshToken())
	require.Equal(t, xai.DefaultCLIBaseURL, repo.accountsByID[54].GetGrokBaseURL())
	require.Equal(t, "grok:account:54", cache.setKey)
	require.Equal(t, "new-access-token", cache.setToken)
	require.Greater(t, cache.setTTL, time.Duration(0))
	require.Equal(t, 1, cache.releaseCalls)
}

func TestGrokTokenProviderRefreshFailureUnschedulesWithRedactedReason(t *testing.T) {
	expiredAt := time.Now().Add(-time.Minute).UTC().Format(time.RFC3339)
	account := &Account{
		ID:          55,
		Platform:    PlatformGrok,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Credentials: map[string]any{
			"access_token":  "expired-access-token",
			"refresh_token": "refresh-token",
			"expires_at":    expiredAt,
			"base_url":      xai.DefaultCLIBaseURL,
		},
	}
	repo := &tokenRefreshAccountRepo{}
	repo.accountsByID = map[int64]*Account{55: account}
	cache := &grokTokenCacheForProviderTest{lockResult: true}
	provider := NewGrokTokenProvider(repo, cache)
	provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), &tokenRefresherStub{
		err: errors.New("temporary refresh failure access_token=leaked-access refresh_token=leaked-refresh"),
	})

	token, err := provider.GetAccessToken(context.Background(), account)
	require.Error(t, err)
	require.Empty(t, token)
	require.Equal(t, 0, repo.setTempUnschedCalls)
	require.Equal(t, 0, repo.setErrorCalls)
}

func TestGrokTokenProviderLockHeldWaitsForRefreshedCacheAndNeverUsesExpiredToken(t *testing.T) {
	account := expiredGrokOAuthAccountForCredentialTest(56)
	baseRepo := &tokenRefreshAccountRepo{}
	baseRepo.accountsByID = map[int64]*Account{account.ID: account}
	repo := &grokCredentialRaceRepo{tokenRefreshAccountRepo: baseRepo}
	cache := &grokTokenCacheForProviderTest{lockResult: false, token: "expired-access-token"}
	provider := NewGrokTokenProvider(repo, cache)
	provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), &tokenRefresherStub{})

	go func() {
		time.Sleep(40 * time.Millisecond)
		refreshed := *account
		refreshed.Credentials = shallowCopyMap(account.Credentials)
		refreshed.Credentials["access_token"] = "refreshed-after-lock"
		refreshed.Credentials["expires_at"] = time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
		refreshed.Credentials["_token_version"] = time.Now().UnixMilli()
		repo.setAccount(&refreshed)
		cache.mu.Lock()
		cache.token = "refreshed-after-lock"
		cache.mu.Unlock()
	}()

	startedAt := time.Now()
	token, err := provider.GetAccessToken(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, "refreshed-after-lock", token)
	require.NotEqual(t, "expired-access-token", token)
	require.GreaterOrEqual(t, time.Since(startedAt), 25*time.Millisecond,
		"expired account metadata must prevent returning the old cached token")
}

func TestGrokTokenProviderLockHeldTimeoutDoesNotReturnExpiredToken(t *testing.T) {
	account := expiredGrokOAuthAccountForCredentialTest(57)
	repo := &tokenRefreshAccountRepo{}
	repo.accountsByID = map[int64]*Account{account.ID: account}
	cache := &grokTokenCacheForProviderTest{lockResult: false}
	provider := NewGrokTokenProvider(repo, cache)
	provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), &tokenRefresherStub{})
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()

	token, err := provider.GetAccessToken(ctx, account)
	require.Error(t, err)
	require.Empty(t, token)
}

func TestGrokTokenProviderLockHeldRejectsChangedTokenWithoutExpiry(t *testing.T) {
	account := expiredGrokOAuthAccountForCredentialTest(58)
	baseRepo := &tokenRefreshAccountRepo{}
	baseRepo.accountsByID = map[int64]*Account{account.ID: account}
	repo := &grokCredentialRaceRepo{tokenRefreshAccountRepo: baseRepo}
	cache := &grokTokenCacheForProviderTest{lockResult: false, token: "expired-access-token"}
	provider := NewGrokTokenProvider(repo, cache)
	provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), &tokenRefresherStub{})

	go func() {
		time.Sleep(30 * time.Millisecond)
		refreshed := *account
		refreshed.Credentials = shallowCopyMap(account.Credentials)
		refreshed.Credentials["access_token"] = "changed-without-expiry"
		delete(refreshed.Credentials, "expires_at")
		refreshed.Credentials["_token_version"] = time.Now().UnixMilli()
		repo.setAccount(&refreshed)
		cache.mu.Lock()
		cache.token = "changed-without-expiry"
		cache.mu.Unlock()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	token, err := provider.GetAccessToken(ctx, account)

	require.Error(t, err)
	require.Empty(t, token, "an unbounded credential must not win the lock-held race")
}

func TestGrokTokenProviderLockHeldUsesVersionedDBTokenAndRepairsStaleCache(t *testing.T) {
	account := expiredGrokOAuthAccountForCredentialTest(60)
	baseRepo := &tokenRefreshAccountRepo{}
	baseRepo.accountsByID = map[int64]*Account{account.ID: account}
	repo := &grokCredentialRaceRepo{tokenRefreshAccountRepo: baseRepo}
	cache := &grokTokenCacheForProviderTest{lockResult: false, token: "expired-access-token"}
	provider := NewGrokTokenProvider(repo, cache)
	provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), &tokenRefresherStub{})

	go func() {
		time.Sleep(30 * time.Millisecond)
		refreshed := *account
		refreshed.Credentials = shallowCopyMap(account.Credentials)
		refreshed.Credentials["access_token"] = "db-authoritative-token"
		refreshed.Credentials["expires_at"] = time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339)
		refreshed.Credentials["_token_version"] = time.Now().UnixMilli()
		repo.setAccount(&refreshed)
	}()

	token, err := provider.GetAccessToken(context.Background(), account)

	require.NoError(t, err)
	require.Equal(t, "db-authoritative-token", token)
	require.Equal(t, "db-authoritative-token", cache.setToken)
	require.Greater(t, cache.setTTL, time.Duration(0))
}

func TestGrokTokenProviderRejectsStaleDBTokenWithoutExpiry(t *testing.T) {
	expiresAt := time.Now().Add(2 * grokTokenRefreshSkew).UTC().Format(time.RFC3339)
	account := &Account{
		ID:          59,
		Platform:    PlatformGrok,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Credentials: map[string]any{
			"access_token":  "old-access-token",
			"refresh_token": "refresh-token",
			"expires_at":    expiresAt,
		},
	}
	latest := *account
	latest.Credentials = shallowCopyMap(account.Credentials)
	latest.Credentials["access_token"] = "new-access-token-without-expiry"
	latest.Credentials["_token_version"] = time.Now().UnixMilli()
	delete(latest.Credentials, "expires_at")
	repo := &tokenRefreshAccountRepo{}
	repo.accountsByID = map[int64]*Account{account.ID: &latest}
	cache := &grokTokenCacheForProviderTest{}
	provider := NewGrokTokenProvider(repo, cache)

	token, err := provider.GetAccessToken(context.Background(), account)

	require.ErrorIs(t, err, errGrokOAuthAccessTokenExpired)
	require.Empty(t, token)
}

func TestGrokTokenProviderRejectsIneligibleSelectedAccountBeforeWarmCache(t *testing.T) {
	future := time.Now().Add(time.Hour)
	tests := []struct {
		name   string
		mutate func(*Account)
	}{
		{name: "disabled", mutate: func(account *Account) { account.Status = StatusDisabled }},
		{name: "not schedulable", mutate: func(account *Account) { account.Schedulable = false }},
		{name: "temporarily unschedulable", mutate: func(account *Account) { account.TempUnschedulableUntil = &future }},
		{name: "rate limited", mutate: func(account *Account) { account.RateLimitResetAt = &future }},
		{name: "overloaded", mutate: func(account *Account) { account.OverloadUntil = &future }},
	}

	for index, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := expiredGrokOAuthAccountForCredentialTest(int64(90 + index))
			account.Credentials["access_token"] = "warm-cache-token"
			account.Credentials["expires_at"] = time.Now().Add(2 * grokTokenRefreshSkew).UTC().Format(time.RFC3339)
			tt.mutate(account)
			cache := &grokTokenCacheForProviderTest{token: "warm-cache-token"}
			provider := NewGrokTokenProvider(&tokenRefreshAccountRepo{}, cache)

			token, err := provider.GetAccessToken(context.Background(), account)

			require.ErrorIs(t, err, errOAuthRefreshAccountStateChanged)
			require.Empty(t, token)
			require.Zero(t, cache.getCalls, "an ineligible selected account must be rejected before cache lookup")
		})
	}
}
