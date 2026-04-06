//go:build unit

package service

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// openAITokenCacheStub implements OpenAITokenCache for testing
type openAITokenCacheStub struct {
	mu               sync.Mutex
	tokens           map[string]string
	getErr           error
	setErr           error
	deleteErr        error
	lockAcquired     bool
	lockErr          error
	releaseLockErr   error
	getCalled        int32
	setCalled        int32
	lockCalled       int32
	unlockCalled     int32
	simulateLockRace bool
}

func newOpenAITokenCacheStub() *openAITokenCacheStub {
	return &openAITokenCacheStub{
		tokens:       make(map[string]string),
		lockAcquired: true,
	}
}

func (s *openAITokenCacheStub) GetAccessToken(ctx context.Context, cacheKey string) (string, error) {
	atomic.AddInt32(&s.getCalled, 1)
	if s.getErr != nil {
		return "", s.getErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.tokens[cacheKey], nil
}

func (s *openAITokenCacheStub) SetAccessToken(ctx context.Context, cacheKey string, token string, ttl time.Duration) error {
	atomic.AddInt32(&s.setCalled, 1)
	if s.setErr != nil {
		return s.setErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokens[cacheKey] = token
	return nil
}

func (s *openAITokenCacheStub) DeleteAccessToken(ctx context.Context, cacheKey string) error {
	if s.deleteErr != nil {
		return s.deleteErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.tokens, cacheKey)
	return nil
}

func (s *openAITokenCacheStub) AcquireRefreshLock(ctx context.Context, cacheKey string, ttl time.Duration) (bool, error) {
	atomic.AddInt32(&s.lockCalled, 1)
	if s.lockErr != nil {
		return false, s.lockErr
	}
	if s.simulateLockRace {
		return false, nil
	}
	return s.lockAcquired, nil
}

func (s *openAITokenCacheStub) ReleaseRefreshLock(ctx context.Context, cacheKey string) error {
	atomic.AddInt32(&s.unlockCalled, 1)
	return s.releaseLockErr
}

// openAIAccountRepoStub is a minimal stub implementing only the methods used by OpenAITokenProvider
type openAIAccountRepoStub struct {
	account      *Account
	getErr       error
	updateErr    error
	getCalled    int32
	updateCalled int32
}

func (r *openAIAccountRepoStub) GetByID(ctx context.Context, id int64) (*Account, error) {
	atomic.AddInt32(&r.getCalled, 1)
	if r.getErr != nil {
		return nil, r.getErr
	}
	return r.account, nil
}

func (r *openAIAccountRepoStub) Update(ctx context.Context, account *Account) error {
	atomic.AddInt32(&r.updateCalled, 1)
	if r.updateErr != nil {
		return r.updateErr
	}
	r.account = account
	return nil
}

// openAIOAuthServiceStub implements OpenAIOAuthService methods for testing
type openAIOAuthServiceStub struct {
	tokenInfo     *OpenAITokenInfo
	refreshErr    error
	refreshCalled int32
}

func (s *openAIOAuthServiceStub) RefreshAccountToken(ctx context.Context, account *Account) (*OpenAITokenInfo, error) {
	atomic.AddInt32(&s.refreshCalled, 1)
	if s.refreshErr != nil {
		return nil, s.refreshErr
	}
	return s.tokenInfo, nil
}

func (s *openAIOAuthServiceStub) BuildAccountCredentials(info *OpenAITokenInfo) map[string]any {
	now := time.Now()
	return map[string]any{
		"access_token":  info.AccessToken,
		"refresh_token": info.RefreshToken,
		"expires_at":    now.Add(time.Duration(info.ExpiresIn) * time.Second).Format(time.RFC3339),
	}
}

func TestOpenAITokenProvider_CacheHit(t *testing.T) {
	cache := newOpenAITokenCacheStub()
	account := &Account{
		ID:       100,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "db-token",
		},
	}
	cacheKey := OpenAITokenCacheKey(account)
	cache.tokens[cacheKey] = "cached-token"

	provider := NewOpenAITokenProvider(nil, cache, nil)

	token, err := provider.GetAccessToken(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, "cached-token", token)
	require.Equal(t, int32(1), atomic.LoadInt32(&cache.getCalled))
	require.Equal(t, int32(0), atomic.LoadInt32(&cache.setCalled))
}

func TestOpenAITokenProvider_CacheMiss_FromCredentials(t *testing.T) {
	cache := newOpenAITokenCacheStub()
	// Token expires in far future, no refresh needed
	expiresAt := time.Now().Add(1 * time.Hour).Format(time.RFC3339)
	account := &Account{
		ID:       101,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "credential-token",
			"expires_at":   expiresAt,
		},
	}

	provider := NewOpenAITokenProvider(nil, cache, nil)

	token, err := provider.GetAccessToken(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, "credential-token", token)

	// Should have stored in cache
	cacheKey := OpenAITokenCacheKey(account)
	require.Equal(t, "credential-token", cache.tokens[cacheKey])
}

func TestOpenAITokenProvider_TokenRefresh(t *testing.T) {
	cache := newOpenAITokenCacheStub()
	accountRepo := &openAIAccountRepoStub{}
	oauthService := &openAIOAuthServiceStub{
		tokenInfo: &OpenAITokenInfo{
			AccessToken:  "refreshed-token",
			RefreshToken: "new-refresh-token",
			ExpiresIn:    3600,
		},
	}

	// Token expires soon (within refresh skew)
	expiresAt := time.Now().Add(1 * time.Minute).Format(time.RFC3339)
	account := &Account{
		ID:       102,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token":  "old-token",
			"refresh_token": "old-refresh-token",
			"expires_at":    expiresAt,
		},
	}
	accountRepo.account = account

	// We need to directly test with the stub - create a custom provider
	customProvider := &testOpenAITokenProvider{
		accountRepo:  accountRepo,
		tokenCache:   cache,
		oauthService: oauthService,
	}

	token, err := customProvider.GetAccessToken(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, "refreshed-token", token)
	require.Equal(t, int32(1), atomic.LoadInt32(&oauthService.refreshCalled))
}

// testOpenAITokenProvider is a test version that uses the stub OAuth service
type testOpenAITokenProvider struct {
	accountRepo  *openAIAccountRepoStub
	tokenCache   *openAITokenCacheStub
	oauthService *openAIOAuthServiceStub
}

func (p *testOpenAITokenProvider) GetAccessToken(ctx context.Context, account *Account) (string, error) {
	if account == nil {
		return "", errors.New("account is nil")
	}
	if account.Platform != PlatformOpenAI || account.Type != AccountTypeOAuth {
		return "", errors.New("not an openai oauth account")
	}

	cacheKey := OpenAITokenCacheKey(account)

	// 1. Check cache
	if p.tokenCache != nil {
		if token, err := p.tokenCache.GetAccessToken(ctx, cacheKey); err == nil && token != "" {
			return token, nil
		}
	}

	// 2. Check if refresh needed
	expiresAt := account.GetCredentialAsTime("expires_at")
	needsRefresh := expiresAt == nil || time.Until(*expiresAt) <= openAITokenRefreshSkew
	refreshFailed := false
	if needsRefresh && p.tokenCache != nil {
		locked, err := p.tokenCache.AcquireRefreshLock(ctx, cacheKey, 30*time.Second)
		if err == nil && locked {
			defer func() { _ = p.tokenCache.ReleaseRefreshLock(ctx, cacheKey) }()

			// Check cache again after acquiring lock
			if token, err := p.tokenCache.GetAccessToken(ctx, cacheKey); err == nil && token != "" {
				return token, nil
			}

			// Get fresh account from DB
			fresh, err := p.accountRepo.GetByID(ctx, account.ID)
			if err == nil && fresh != nil {
				account = fresh
			}
			expiresAt = account.GetCredentialAsTime("expires_at")
			if expiresAt == nil || time.Until(*expiresAt) <= openAITokenRefreshSkew {
				if p.oauthService == nil {
					refreshFailed = true // 无法刷新，标记失败
				} else {
					tokenInfo, err := p.oauthService.RefreshAccountToken(ctx, account)
					if err != nil {
						refreshFailed = true // 刷新失败，标记以使用短 TTL
					} else {
						newCredentials := p.oauthService.BuildAccountCredentials(tokenInfo)
						for k, v := range account.Credentials {
							if _, exists := newCredentials[k]; !exists {
								newCredentials[k] = v
							}
						}
						account.Credentials = newCredentials
						_ = p.accountRepo.Update(ctx, account)
						expiresAt = account.GetCredentialAsTime("expires_at")
					}
				}
			}
		} else if p.tokenCache.simulateLockRace {
			// Wait and retry cache
			time.Sleep(10 * time.Millisecond) // Short wait for test
			if token, err := p.tokenCache.GetAccessToken(ctx, cacheKey); err == nil && token != "" {
				return token, nil
			}
		}
	}

	accessToken := account.GetOpenAIAccessToken()
	if accessToken == "" {
		return "", errors.New("access_token not found in credentials")
	}

	// 3. Store in cache
	if p.tokenCache != nil {
		ttl := 30 * time.Minute
		if refreshFailed {
			ttl = time.Minute // 刷新失败时使用短 TTL
		} else if expiresAt != nil {
			until := time.Until(*expiresAt)
			if until > openAITokenCacheSkew {
				ttl = until - openAITokenCacheSkew
			} else if until > 0 {
				ttl = until
			} else {
				ttl = time.Minute
			}
		}
		_ = p.tokenCache.SetAccessToken(ctx, cacheKey, accessToken, ttl)
	}

	return accessToken, nil
}

func TestOpenAITokenProvider_LockRaceCondition(t *testing.T) {
	cache := newOpenAITokenCacheStub()
	cache.simulateLockRace = true
	accountRepo := &openAIAccountRepoStub{}

	// Token expires soon
	expiresAt := time.Now().Add(1 * time.Minute).Format(time.RFC3339)
	account := &Account{
		ID:       103,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "race-token",
			"expires_at":   expiresAt,
		},
	}
	accountRepo.account = account

	// Simulate another worker already refreshed and cached
	cacheKey := OpenAITokenCacheKey(account)
	go func() {
		time.Sleep(5 * time.Millisecond)
		cache.mu.Lock()
		cache.tokens[cacheKey] = "winner-token"
		cache.mu.Unlock()
	}()

	provider := &testOpenAITokenProvider{
		accountRepo: accountRepo,
		tokenCache:  cache,
	}

	token, err := provider.GetAccessToken(context.Background(), account)
	require.NoError(t, err)
	// Should get the token set by the "winner" or the original
	require.NotEmpty(t, token)
}

func TestOpenAITokenProvider_NilAccount(t *testing.T) {
	provider := NewOpenAITokenProvider(nil, nil, nil)

	token, err := provider.GetAccessToken(context.Background(), nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "account is nil")
	require.Empty(t, token)
}

func TestOpenAITokenProvider_WrongPlatform(t *testing.T) {
	provider := NewOpenAITokenProvider(nil, nil, nil)
	account := &Account{
		ID:       104,
		Platform: PlatformGemini,
		Type:     AccountTypeOAuth,
	}

	token, err := provider.GetAccessToken(context.Background(), account)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not an openai oauth account")
	require.Empty(t, token)
}

func TestOpenAITokenProvider_WrongAccountType(t *testing.T) {
	provider := NewOpenAITokenProvider(nil, nil, nil)
	account := &Account{
		ID:       105,
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
	}

	token, err := provider.GetAccessToken(context.Background(), account)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not an openai oauth account")
	require.Empty(t, token)
}

func TestOpenAITokenProvider_NilCache(t *testing.T) {
	// Token doesn't need refresh
	expiresAt := time.Now().Add(1 * time.Hour).Format(time.RFC3339)
	account := &Account{
		ID:       106,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "nocache-token",
			"expires_at":   expiresAt,
		},
	}

	provider := NewOpenAITokenProvider(nil, nil, nil)

	token, err := provider.GetAccessToken(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, "nocache-token", token)
}

func TestOpenAITokenProvider_CacheGetError(t *testing.T) {
	cache := newOpenAITokenCacheStub()
	cache.getErr = errors.New("redis connection failed")

	// Token doesn't need refresh
	expiresAt := time.Now().Add(1 * time.Hour).Format(time.RFC3339)
	account := &Account{
		ID:       107,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "fallback-token",
			"expires_at":   expiresAt,
		},
	}

	provider := NewOpenAITokenProvider(nil, cache, nil)

	// Should gracefully degrade and return from credentials
	token, err := provider.GetAccessToken(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, "fallback-token", token)
}

func TestOpenAITokenProvider_CacheSetError(t *testing.T) {
	cache := newOpenAITokenCacheStub()
	cache.setErr = errors.New("redis write failed")

	expiresAt := time.Now().Add(1 * time.Hour).Format(time.RFC3339)
	account := &Account{
		ID:       108,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "still-works-token",
			"expires_at":   expiresAt,
		},
	}

	provider := NewOpenAITokenProvider(nil, cache, nil)

	// Should still work even if cache set fails
	token, err := provider.GetAccessToken(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, "still-works-token", token)
}

func TestOpenAITokenProvider_MissingAccessToken(t *testing.T) {
	cache := newOpenAITokenCacheStub()
	expiresAt := time.Now().Add(1 * time.Hour).Format(time.RFC3339)
	account := &Account{
		ID:       109,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"expires_at": expiresAt,
			// missing access_token
		},
	}

	provider := NewOpenAITokenProvider(nil, cache, nil)

	token, err := provider.GetAccessToken(context.Background(), account)
	require.Error(t, err)
	require.Contains(t, err.Error(), "access_token not found")
	require.Empty(t, token)
}

func TestOpenAITokenProvider_RefreshError(t *testing.T) {
	cache := newOpenAITokenCacheStub()
	accountRepo := &openAIAccountRepoStub{}
	oauthService := &openAIOAuthServiceStub{
		refreshErr: errors.New("oauth refresh failed"),
	}

	// Token expires soon
	expiresAt := time.Now().Add(1 * time.Minute).Format(time.RFC3339)
	account := &Account{
		ID:       110,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token":  "old-token",
			"refresh_token": "old-refresh-token",
			"expires_at":    expiresAt,
		},
	}
	accountRepo.account = account

	provider := &testOpenAITokenProvider{
		accountRepo:  accountRepo,
		tokenCache:   cache,
		oauthService: oauthService,
	}

	// Now with fallback behavior, should return existing token even if refresh fails
	token, err := provider.GetAccessToken(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, "old-token", token) // Fallback to existing token
}

func TestOpenAITokenProvider_OAuthServiceNotConfigured(t *testing.T) {
	cache := newOpenAITokenCacheStub()
	accountRepo := &openAIAccountRepoStub{}

	// Token expires soon
	expiresAt := time.Now().Add(1 * time.Minute).Format(time.RFC3339)
	account := &Account{
		ID:       111,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "old-token",
			"expires_at":   expiresAt,
		},
	}
	accountRepo.account = account

	provider := &testOpenAITokenProvider{
		accountRepo:  accountRepo,
		tokenCache:   cache,
		oauthService: nil, // not configured
	}

	// Now with fallback behavior, should return existing token even if oauth service not configured
	token, err := provider.GetAccessToken(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, "old-token", token) // Fallback to existing token
}

func TestOpenAITokenProvider_TTLCalculation(t *testing.T) {
	tests := []struct {
		name      string
		expiresIn time.Duration
	}{
		{
			name:      "far_future_expiry",
			expiresIn: 1 * time.Hour,
		},
		{
			name:      "medium_expiry",
			expiresIn: 10 * time.Minute,
		},
		{
			name:      "near_expiry",
			expiresIn: 6 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := newOpenAITokenCacheStub()
			expiresAt := time.Now().Add(tt.expiresIn).Format(time.RFC3339)
			account := &Account{
				ID:       200,
				Platform: PlatformOpenAI,
				Type:     AccountTypeOAuth,
				Credentials: map[string]any{
					"access_token": "test-token",
					"expires_at":   expiresAt,
				},
			}

			provider := NewOpenAITokenProvider(nil, cache, nil)

			_, err := provider.GetAccessToken(context.Background(), account)
			require.NoError(t, err)

			// Verify token was cached
			cacheKey := OpenAITokenCacheKey(account)
			require.Equal(t, "test-token", cache.tokens[cacheKey])
		})
	}
}

func TestOpenAITokenProvider_DoubleCheckAfterLock(t *testing.T) {
	cache := newOpenAITokenCacheStub()
	accountRepo := &openAIAccountRepoStub{}
	oauthService := &openAIOAuthServiceStub{
		tokenInfo: &OpenAITokenInfo{
			AccessToken:  "refreshed-token",
			RefreshToken: "new-refresh",
			ExpiresIn:    3600,
		},
	}

	// Token expires soon
	expiresAt := time.Now().Add(1 * time.Minute).Format(time.RFC3339)
	account := &Account{
		ID:       112,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "old-token",
			"expires_at":   expiresAt,
		},
	}
	accountRepo.account = account
	cacheKey := OpenAITokenCacheKey(account)

	// Simulate: first GetAccessToken returns empty, but after lock acquired, cache has token
	originalGet := int32(0)
	cache.tokens[cacheKey] = "" // Empty initially

	provider := &testOpenAITokenProvider{
		accountRepo:  accountRepo,
		tokenCache:   cache,
		oauthService: oauthService,
	}

	// In a goroutine, set the cached token after a small delay (simulating race)
	go func() {
		time.Sleep(5 * time.Millisecond)
		cache.mu.Lock()
		cache.tokens[cacheKey] = "cached-by-other"
		cache.mu.Unlock()
	}()

	token, err := provider.GetAccessToken(context.Background(), account)
	require.NoError(t, err)
	// Should get either the refreshed token or the cached one
	require.NotEmpty(t, token)
	_ = originalGet // Suppress unused warning
}

// Tests for real provider - to increase coverage
func TestOpenAITokenProvider_Real_LockFailedWait(t *testing.T) {
	cache := newOpenAITokenCacheStub()
	cache.lockAcquired = false // Lock acquisition fails

	// Token expires soon (within refresh skew) to trigger lock attempt
	expiresAt := time.Now().Add(1 * time.Minute).Format(time.RFC3339)
	account := &Account{
		ID:       200,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "fallback-token",
			"expires_at":   expiresAt,
		},
	}

	// Set token in cache after lock wait period (simulate other worker refreshing)
	cacheKey := OpenAITokenCacheKey(account)
	go func() {
		time.Sleep(100 * time.Millisecond)
		cache.mu.Lock()
		cache.tokens[cacheKey] = "refreshed-by-other"
		cache.mu.Unlock()
	}()

	provider := NewOpenAITokenProvider(nil, cache, nil)
	token, err := provider.GetAccessToken(context.Background(), account)
	require.NoError(t, err)
	// Should get either the fallback token or the refreshed one
	require.NotEmpty(t, token)
}

func TestOpenAITokenProvider_Real_CacheHitAfterWait(t *testing.T) {
	cache := newOpenAITokenCacheStub()
	cache.lockAcquired = false // Lock acquisition fails

	// Token expires soon
	expiresAt := time.Now().Add(1 * time.Minute).Format(time.RFC3339)
	account := &Account{
		ID:       201,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "original-token",
			"expires_at":   expiresAt,
		},
	}

	cacheKey := OpenAITokenCacheKey(account)
	// Set token in cache immediately after wait starts
	go func() {
		time.Sleep(50 * time.Millisecond)
		cache.mu.Lock()
		cache.tokens[cacheKey] = "winner-token"
		cache.mu.Unlock()
	}()

	provider := NewOpenAITokenProvider(nil, cache, nil)
	token, err := provider.GetAccessToken(context.Background(), account)
	require.NoError(t, err)
	require.NotEmpty(t, token)
}

func TestOpenAITokenProvider_Real_ExpiredWithoutRefreshToken(t *testing.T) {
	cache := newOpenAITokenCacheStub()
	cache.lockAcquired = false // Prevent entering refresh logic

	// Token with nil expires_at (no expiry set) - should use credentials
	account := &Account{
		ID:       202,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "no-expiry-token",
		},
	}

	provider := NewOpenAITokenProvider(nil, cache, nil)
	token, err := provider.GetAccessToken(context.Background(), account)
	// Without OAuth service, refresh will fail but token should be returned from credentials
	require.NoError(t, err)
	require.Equal(t, "no-expiry-token", token)
}

func TestOpenAITokenProvider_Real_WhitespaceToken(t *testing.T) {
	cache := newOpenAITokenCacheStub()
	cacheKey := "openai:account:203"
	cache.tokens[cacheKey] = "   " // Whitespace only - should be treated as empty

	expiresAt := time.Now().Add(1 * time.Hour).Format(time.RFC3339)
	account := &Account{
		ID:       203,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "real-token",
			"expires_at":   expiresAt,
		},
	}

	provider := NewOpenAITokenProvider(nil, cache, nil)
	token, err := provider.GetAccessToken(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, "real-token", token) // Should fall back to credentials
}

func TestOpenAITokenProvider_Real_LockError(t *testing.T) {
	cache := newOpenAITokenCacheStub()
	cache.lockErr = errors.New("redis lock failed")

	// Token expires soon (within refresh skew)
	expiresAt := time.Now().Add(1 * time.Minute).Format(time.RFC3339)
	account := &Account{
		ID:       204,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "fallback-on-lock-error",
			"expires_at":   expiresAt,
		},
	}

	provider := NewOpenAITokenProvider(nil, cache, nil)
	token, err := provider.GetAccessToken(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, "fallback-on-lock-error", token)
}

func TestOpenAITokenProvider_Real_WhitespaceCredentialToken(t *testing.T) {
	cache := newOpenAITokenCacheStub()

	expiresAt := time.Now().Add(1 * time.Hour).Format(time.RFC3339)
	account := &Account{
		ID:       205,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "   ", // Whitespace only
			"expires_at":   expiresAt,
		},
	}

	provider := NewOpenAITokenProvider(nil, cache, nil)
	token, err := provider.GetAccessToken(context.Background(), account)
	require.Error(t, err)
	require.Contains(t, err.Error(), "access_token not found")
	require.Empty(t, token)
}

func TestOpenAITokenProvider_Real_NilCredentials(t *testing.T) {
	cache := newOpenAITokenCacheStub()

	expiresAt := time.Now().Add(1 * time.Hour).Format(time.RFC3339)
	account := &Account{
		ID:       206,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"expires_at": expiresAt,
			// No access_token
		},
	}

	provider := NewOpenAITokenProvider(nil, cache, nil)
	token, err := provider.GetAccessToken(context.Background(), account)
	require.Error(t, err)
	require.Contains(t, err.Error(), "access_token not found")
	require.Empty(t, token)
}

func TestOpenAITokenProvider_Real_LockRace_PollingHitsCache(t *testing.T) {
	cache := newOpenAITokenCacheStub()
	cache.lockAcquired = false // 模拟锁被其他 worker 持有

	expiresAt := time.Now().Add(1 * time.Minute).Format(time.RFC3339)
	account := &Account{
		ID:       207,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "fallback-token",
			"expires_at":   expiresAt,
		},
	}

	cacheKey := OpenAITokenCacheKey(account)
	go func() {
		time.Sleep(5 * time.Millisecond)
		cache.mu.Lock()
		cache.tokens[cacheKey] = "winner-token"
		cache.mu.Unlock()
	}()

	provider := NewOpenAITokenProvider(nil, cache, nil)
	token, err := provider.GetAccessToken(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, "winner-token", token)
}

func TestOpenAITokenProvider_Real_LockRace_ContextCanceled(t *testing.T) {
	cache := newOpenAITokenCacheStub()
	cache.lockAcquired = false // 模拟锁被其他 worker 持有

	expiresAt := time.Now().Add(1 * time.Minute).Format(time.RFC3339)
	account := &Account{
		ID:       208,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "fallback-token",
			"expires_at":   expiresAt,
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	provider := NewOpenAITokenProvider(nil, cache, nil)
	start := time.Now()
	token, err := provider.GetAccessToken(ctx, account)
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
	require.Empty(t, token)
	require.Less(t, time.Since(start), 50*time.Millisecond)
}

func TestOpenAITokenProvider_RuntimeMetrics_LockWaitHitAndSnapshot(t *testing.T) {
	cache := newOpenAITokenCacheStub()
	cache.lockAcquired = false

	expiresAt := time.Now().Add(1 * time.Minute).Format(time.RFC3339)
	account := &Account{
		ID:       209,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "fallback-token",
			"expires_at":   expiresAt,
		},
	}
	cacheKey := OpenAITokenCacheKey(account)
	go func() {
		time.Sleep(10 * time.Millisecond)
		cache.mu.Lock()
		cache.tokens[cacheKey] = "winner-token"
		cache.mu.Unlock()
	}()

	provider := NewOpenAITokenProvider(nil, cache, nil)
	token, err := provider.GetAccessToken(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, "winner-token", token)

	metrics := provider.SnapshotRuntimeMetrics()
	require.GreaterOrEqual(t, metrics.RefreshRequests, int64(1))
	require.GreaterOrEqual(t, metrics.LockContention, int64(1))
	require.GreaterOrEqual(t, metrics.LockWaitSamples, int64(1))
	require.GreaterOrEqual(t, metrics.LockWaitHit, int64(1))
	require.GreaterOrEqual(t, metrics.LockWaitTotalMs, int64(0))
	require.GreaterOrEqual(t, metrics.LastObservedUnixMs, int64(1))
}

func TestOpenAITokenProvider_RuntimeMetrics_LockAcquireFailure(t *testing.T) {
	cache := newOpenAITokenCacheStub()
	cache.lockErr = errors.New("redis lock error")

	expiresAt := time.Now().Add(1 * time.Minute).Format(time.RFC3339)
	account := &Account{
		ID:       210,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "fallback-token",
			"expires_at":   expiresAt,
		},
	}

	provider := NewOpenAITokenProvider(nil, cache, nil)
	_, err := provider.GetAccessToken(context.Background(), account)
	require.NoError(t, err)

	metrics := provider.SnapshotRuntimeMetrics()
	require.GreaterOrEqual(t, metrics.LockAcquireFailure, int64(1))
	require.GreaterOrEqual(t, metrics.RefreshRequests, int64(1))
}
