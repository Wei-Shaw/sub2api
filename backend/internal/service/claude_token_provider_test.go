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

// claudeTokenCacheStub implements ClaudeTokenCache for testing
type claudeTokenCacheStub struct {
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

func newClaudeTokenCacheStub() *claudeTokenCacheStub {
	return &claudeTokenCacheStub{
		tokens:       make(map[string]string),
		lockAcquired: true,
	}
}

func (s *claudeTokenCacheStub) GetAccessToken(ctx context.Context, cacheKey string) (string, error) {
	atomic.AddInt32(&s.getCalled, 1)
	if s.getErr != nil {
		return "", s.getErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.tokens[cacheKey], nil
}

func (s *claudeTokenCacheStub) SetAccessToken(ctx context.Context, cacheKey string, token string, ttl time.Duration) error {
	atomic.AddInt32(&s.setCalled, 1)
	if s.setErr != nil {
		return s.setErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokens[cacheKey] = token
	return nil
}

func (s *claudeTokenCacheStub) DeleteAccessToken(ctx context.Context, cacheKey string) error {
	if s.deleteErr != nil {
		return s.deleteErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.tokens, cacheKey)
	return nil
}

func (s *claudeTokenCacheStub) AcquireRefreshLock(ctx context.Context, cacheKey string, ttl time.Duration) (bool, error) {
	atomic.AddInt32(&s.lockCalled, 1)
	if s.lockErr != nil {
		return false, s.lockErr
	}
	if s.simulateLockRace {
		return false, nil
	}
	return s.lockAcquired, nil
}

func (s *claudeTokenCacheStub) ReleaseRefreshLock(ctx context.Context, cacheKey string) error {
	atomic.AddInt32(&s.unlockCalled, 1)
	return s.releaseLockErr
}

// claudeAccountRepoStub is a minimal stub implementing only the methods used by ClaudeTokenProvider
type claudeAccountRepoStub struct {
	account      *Account
	getErr       error
	updateErr    error
	getCalled    int32
	updateCalled int32
}

func (r *claudeAccountRepoStub) GetByID(ctx context.Context, id int64) (*Account, error) {
	atomic.AddInt32(&r.getCalled, 1)
	if r.getErr != nil {
		return nil, r.getErr
	}
	return r.account, nil
}

func (r *claudeAccountRepoStub) Update(ctx context.Context, account *Account) error {
	atomic.AddInt32(&r.updateCalled, 1)
	if r.updateErr != nil {
		return r.updateErr
	}
	r.account = account
	return nil
}

// claudeOAuthServiceStub implements OAuthService methods for testing
type claudeOAuthServiceStub struct {
	tokenInfo     *TokenInfo
	refreshErr    error
	refreshCalled int32
}

func (s *claudeOAuthServiceStub) RefreshAccountToken(ctx context.Context, account *Account) (*TokenInfo, error) {
	atomic.AddInt32(&s.refreshCalled, 1)
	if s.refreshErr != nil {
		return nil, s.refreshErr
	}
	return s.tokenInfo, nil
}

// testClaudeTokenProvider is a test version that uses the stub OAuth service
type testClaudeTokenProvider struct {
	accountRepo  *claudeAccountRepoStub
	tokenCache   *claudeTokenCacheStub
	oauthService *claudeOAuthServiceStub
}

func (p *testClaudeTokenProvider) GetAccessToken(ctx context.Context, account *Account) (string, error) {
	if account == nil {
		return "", errors.New("account is nil")
	}
	if account.Platform != PlatformAnthropic || account.Type != AccountTypeOAuth {
		return "", errors.New("not an anthropic oauth or service account")
	}

	cacheKey := ClaudeTokenCacheKey(account)

	// 1. Check cache
	if p.tokenCache != nil {
		if token, err := p.tokenCache.GetAccessToken(ctx, cacheKey); err == nil && token != "" {
			return token, nil
		}
	}

	// 2. Check if refresh needed
	expiresAt := account.GetCredentialAsTime("expires_at")
	needsRefresh := expiresAt == nil || time.Until(*expiresAt) <= claudeTokenRefreshSkew
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
			if expiresAt == nil || time.Until(*expiresAt) <= claudeTokenRefreshSkew {
				if p.oauthService == nil {
					refreshFailed = true // 无法刷新，标记失败
				} else {
					tokenInfo, err := p.oauthService.RefreshAccountToken(ctx, account)
					if err != nil {
						refreshFailed = true // 刷新失败，标记以使用短 TTL
					} else {
						// Build new credentials
						newCredentials := make(map[string]any)
						for k, v := range account.Credentials {
							newCredentials[k] = v
						}
						newCredentials["access_token"] = tokenInfo.AccessToken
						newCredentials["token_type"] = tokenInfo.TokenType
						newCredentials["expires_at"] = time.Now().Add(time.Duration(tokenInfo.ExpiresIn) * time.Second).Format(time.RFC3339)
						if tokenInfo.RefreshToken != "" {
							newCredentials["refresh_token"] = tokenInfo.RefreshToken
						}
						account.Credentials = newCredentials
						_ = p.accountRepo.Update(ctx, account)
						expiresAt = account.GetCredentialAsTime("expires_at")
					}
				}
			}
		} else if p.tokenCache.simulateLockRace {
			// Wait and retry cache
			time.Sleep(10 * time.Millisecond)
			if token, err := p.tokenCache.GetAccessToken(ctx, cacheKey); err == nil && token != "" {
				return token, nil
			}
		}
	}

	accessToken := account.GetCredential("access_token")
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
			if until > claudeTokenCacheSkew {
				ttl = until - claudeTokenCacheSkew
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

func TestClaudeTokenProvider_CacheHit(t *testing.T) {
	cache := newClaudeTokenCacheStub()
	account := &Account{
		ID:       100,
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "db-token",
		},
	}
	cacheKey := ClaudeTokenCacheKey(account)
	cache.tokens[cacheKey] = "cached-token"

	provider := NewClaudeTokenProvider(nil, cache, nil)

	token, err := provider.GetAccessToken(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, "cached-token", token)
	require.Equal(t, int32(1), atomic.LoadInt32(&cache.getCalled))
	require.Equal(t, int32(0), atomic.LoadInt32(&cache.setCalled))
}

func TestClaudeTokenProvider_CacheMiss_FromCredentials(t *testing.T) {
	cache := newClaudeTokenCacheStub()
	// Token expires in far future, no refresh needed
	expiresAt := time.Now().Add(1 * time.Hour).Format(time.RFC3339)
	account := &Account{
		ID:       101,
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "credential-token",
			"expires_at":   expiresAt,
		},
	}

	provider := NewClaudeTokenProvider(nil, cache, nil)

	token, err := provider.GetAccessToken(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, "credential-token", token)

	// Should have stored in cache
	cacheKey := ClaudeTokenCacheKey(account)
	require.Equal(t, "credential-token", cache.tokens[cacheKey])
}

func TestClaudeTokenProvider_TokenRefresh(t *testing.T) {
	cache := newClaudeTokenCacheStub()
	accountRepo := &claudeAccountRepoStub{}
	oauthService := &claudeOAuthServiceStub{
		tokenInfo: &TokenInfo{
			AccessToken:  "refreshed-token",
			RefreshToken: "new-refresh-token",
			TokenType:    "Bearer",
			ExpiresIn:    3600,
			ExpiresAt:    time.Now().Add(time.Hour).Unix(),
		},
	}

	// Token expires soon (within refresh skew)
	expiresAt := time.Now().Add(1 * time.Minute).Format(time.RFC3339)
	account := &Account{
		ID:       102,
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token":  "old-token",
			"refresh_token": "old-refresh-token",
			"expires_at":    expiresAt,
		},
	}
	accountRepo.account = account

	provider := &testClaudeTokenProvider{
		accountRepo:  accountRepo,
		tokenCache:   cache,
		oauthService: oauthService,
	}

	token, err := provider.GetAccessToken(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, "refreshed-token", token)
	require.Equal(t, int32(1), atomic.LoadInt32(&oauthService.refreshCalled))
}

func TestClaudeTokenProvider_LockRaceCondition(t *testing.T) {
	cache := newClaudeTokenCacheStub()
	cache.simulateLockRace = true
	accountRepo := &claudeAccountRepoStub{}

	// Token expires soon
	expiresAt := time.Now().Add(1 * time.Minute).Format(time.RFC3339)
	account := &Account{
		ID:       103,
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "race-token",
			"expires_at":   expiresAt,
		},
	}
	accountRepo.account = account

	// Simulate another worker already refreshed and cached
	cacheKey := ClaudeTokenCacheKey(account)
	go func() {
		time.Sleep(5 * time.Millisecond)
		cache.mu.Lock()
		cache.tokens[cacheKey] = "winner-token"
		cache.mu.Unlock()
	}()

	provider := &testClaudeTokenProvider{
		accountRepo: accountRepo,
		tokenCache:  cache,
	}

	token, err := provider.GetAccessToken(context.Background(), account)
	require.NoError(t, err)
	require.NotEmpty(t, token)
}

func TestClaudeTokenProvider_NilAccount(t *testing.T) {
	provider := NewClaudeTokenProvider(nil, nil, nil)

	token, err := provider.GetAccessToken(context.Background(), nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "account is nil")
	require.Empty(t, token)
}

func TestClaudeTokenProvider_WrongPlatform(t *testing.T) {
	provider := NewClaudeTokenProvider(nil, nil, nil)
	account := &Account{
		ID:       104,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
	}

	token, err := provider.GetAccessToken(context.Background(), account)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not an anthropic oauth or service account")
	require.Empty(t, token)
}

func TestClaudeTokenProvider_WrongAccountType(t *testing.T) {
	provider := NewClaudeTokenProvider(nil, nil, nil)
	account := &Account{
		ID:       105,
		Platform: PlatformAnthropic,
		Type:     AccountTypeAPIKey,
	}

	token, err := provider.GetAccessToken(context.Background(), account)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not an anthropic oauth or service account")
	require.Empty(t, token)
}

func TestClaudeTokenProvider_SetupTokenType(t *testing.T) {
	provider := NewClaudeTokenProvider(nil, nil, nil)
	account := &Account{
		ID:       106,
		Platform: PlatformAnthropic,
		Type:     AccountTypeSetupToken,
	}

	token, err := provider.GetAccessToken(context.Background(), account)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not an anthropic oauth or service account")
	require.Empty(t, token)
}

func TestClaudeTokenProvider_NilCache(t *testing.T) {
	// Token doesn't need refresh
	expiresAt := time.Now().Add(1 * time.Hour).Format(time.RFC3339)
	account := &Account{
		ID:       107,
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "nocache-token",
			"expires_at":   expiresAt,
		},
	}

	provider := NewClaudeTokenProvider(nil, nil, nil)

	token, err := provider.GetAccessToken(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, "nocache-token", token)
}

func TestClaudeTokenProvider_CacheGetError(t *testing.T) {
	cache := newClaudeTokenCacheStub()
	cache.getErr = errors.New("redis connection failed")

	// Token doesn't need refresh
	expiresAt := time.Now().Add(1 * time.Hour).Format(time.RFC3339)
	account := &Account{
		ID:       108,
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "fallback-token",
			"expires_at":   expiresAt,
		},
	}

	provider := NewClaudeTokenProvider(nil, cache, nil)

	// Should gracefully degrade and return from credentials
	token, err := provider.GetAccessToken(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, "fallback-token", token)
}

func TestClaudeTokenProvider_CacheSetError(t *testing.T) {
	cache := newClaudeTokenCacheStub()
	cache.setErr = errors.New("redis write failed")

	expiresAt := time.Now().Add(1 * time.Hour).Format(time.RFC3339)
	account := &Account{
		ID:       109,
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "still-works-token",
			"expires_at":   expiresAt,
		},
	}

	provider := NewClaudeTokenProvider(nil, cache, nil)

	// Should still work even if cache set fails
	token, err := provider.GetAccessToken(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, "still-works-token", token)
}

func TestClaudeTokenProvider_MissingAccessToken(t *testing.T) {
	cache := newClaudeTokenCacheStub()
	expiresAt := time.Now().Add(1 * time.Hour).Format(time.RFC3339)
	account := &Account{
		ID:       110,
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"expires_at": expiresAt,
			// missing access_token
		},
	}

	provider := NewClaudeTokenProvider(nil, cache, nil)

	token, err := provider.GetAccessToken(context.Background(), account)
	require.Error(t, err)
	require.Contains(t, err.Error(), "access_token not found")
	require.Empty(t, token)
}

func TestClaudeTokenProvider_RefreshError(t *testing.T) {
	cache := newClaudeTokenCacheStub()
	accountRepo := &claudeAccountRepoStub{}
	oauthService := &claudeOAuthServiceStub{
		refreshErr: errors.New("oauth refresh failed"),
	}

	// Token expires soon
	expiresAt := time.Now().Add(1 * time.Minute).Format(time.RFC3339)
	account := &Account{
		ID:       111,
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token":  "old-token",
			"refresh_token": "old-refresh-token",
			"expires_at":    expiresAt,
		},
	}
	accountRepo.account = account

	provider := &testClaudeTokenProvider{
		accountRepo:  accountRepo,
		tokenCache:   cache,
		oauthService: oauthService,
	}

	// Now with fallback behavior, should return existing token even if refresh fails
	token, err := provider.GetAccessToken(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, "old-token", token) // Fallback to existing token
}

func TestClaudeTokenProvider_OAuthServiceNotConfigured(t *testing.T) {
	cache := newClaudeTokenCacheStub()
	accountRepo := &claudeAccountRepoStub{}

	// Token expires soon
	expiresAt := time.Now().Add(1 * time.Minute).Format(time.RFC3339)
	account := &Account{
		ID:       112,
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "old-token",
			"expires_at":   expiresAt,
		},
	}
	accountRepo.account = account

	provider := &testClaudeTokenProvider{
		accountRepo:  accountRepo,
		tokenCache:   cache,
		oauthService: nil, // not configured
	}

	// Now with fallback behavior, should return existing token even if oauth service not configured
	token, err := provider.GetAccessToken(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, "old-token", token) // Fallback to existing token
}

func TestClaudeTokenProvider_TTLCalculation(t *testing.T) {
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
			cache := newClaudeTokenCacheStub()
			expiresAt := time.Now().Add(tt.expiresIn).Format(time.RFC3339)
			account := &Account{
				ID:       200,
				Platform: PlatformAnthropic,
				Type:     AccountTypeOAuth,
				Credentials: map[string]any{
					"access_token": "test-token",
					"expires_at":   expiresAt,
				},
			}

			provider := NewClaudeTokenProvider(nil, cache, nil)

			_, err := provider.GetAccessToken(context.Background(), account)
			require.NoError(t, err)

			// Verify token was cached
			cacheKey := ClaudeTokenCacheKey(account)
			require.Equal(t, "test-token", cache.tokens[cacheKey])
		})
	}
}

func TestClaudeTokenProvider_AccountRepoGetError(t *testing.T) {
	cache := newClaudeTokenCacheStub()
	accountRepo := &claudeAccountRepoStub{
		getErr: errors.New("db connection failed"),
	}
	oauthService := &claudeOAuthServiceStub{
		tokenInfo: &TokenInfo{
			AccessToken:  "refreshed-token",
			RefreshToken: "new-refresh",
			TokenType:    "Bearer",
			ExpiresIn:    3600,
		},
	}

	// Token expires soon
	expiresAt := time.Now().Add(1 * time.Minute).Format(time.RFC3339)
	account := &Account{
		ID:       113,
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token":  "old-token",
			"refresh_token": "old-refresh",
			"expires_at":    expiresAt,
		},
	}

	provider := &testClaudeTokenProvider{
		accountRepo:  accountRepo,
		tokenCache:   cache,
		oauthService: oauthService,
	}

	// Should still work, just using the passed-in account
	token, err := provider.GetAccessToken(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, "refreshed-token", token)
}

func TestClaudeTokenProvider_AccountUpdateError(t *testing.T) {
	cache := newClaudeTokenCacheStub()
	accountRepo := &claudeAccountRepoStub{
		updateErr: errors.New("db write failed"),
	}
	oauthService := &claudeOAuthServiceStub{
		tokenInfo: &TokenInfo{
			AccessToken:  "refreshed-token",
			RefreshToken: "new-refresh",
			TokenType:    "Bearer",
			ExpiresIn:    3600,
		},
	}

	// Token expires soon
	expiresAt := time.Now().Add(1 * time.Minute).Format(time.RFC3339)
	account := &Account{
		ID:       114,
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token":  "old-token",
			"refresh_token": "old-refresh",
			"expires_at":    expiresAt,
		},
	}
	accountRepo.account = account

	provider := &testClaudeTokenProvider{
		accountRepo:  accountRepo,
		tokenCache:   cache,
		oauthService: oauthService,
	}

	// Should still return token even if update fails
	token, err := provider.GetAccessToken(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, "refreshed-token", token)
}

func TestClaudeTokenProvider_RefreshPreservesExistingCredentials(t *testing.T) {
	cache := newClaudeTokenCacheStub()
	accountRepo := &claudeAccountRepoStub{}
	oauthService := &claudeOAuthServiceStub{
		tokenInfo: &TokenInfo{
			AccessToken:  "new-access-token",
			RefreshToken: "new-refresh-token",
			TokenType:    "Bearer",
			ExpiresIn:    3600,
		},
	}

	// Token expires soon
	expiresAt := time.Now().Add(1 * time.Minute).Format(time.RFC3339)
	account := &Account{
		ID:       115,
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token":  "old-access-token",
			"refresh_token": "old-refresh-token",
			"expires_at":    expiresAt,
			"custom_field":  "should-be-preserved",
			"organization":  "test-org",
		},
	}
	accountRepo.account = account

	provider := &testClaudeTokenProvider{
		accountRepo:  accountRepo,
		tokenCache:   cache,
		oauthService: oauthService,
	}

	token, err := provider.GetAccessToken(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, "new-access-token", token)

	// Verify existing fields are preserved
	require.Equal(t, "should-be-preserved", accountRepo.account.Credentials["custom_field"])
	require.Equal(t, "test-org", accountRepo.account.Credentials["organization"])
	// Verify new fields are updated
	require.Equal(t, "new-access-token", accountRepo.account.Credentials["access_token"])
	require.Equal(t, "new-refresh-token", accountRepo.account.Credentials["refresh_token"])
}

func TestClaudeTokenProvider_DoubleCheckCacheAfterLock(t *testing.T) {
	cache := newClaudeTokenCacheStub()
	accountRepo := &claudeAccountRepoStub{}
	oauthService := &claudeOAuthServiceStub{
		tokenInfo: &TokenInfo{
			AccessToken:  "refreshed-token",
			RefreshToken: "new-refresh",
			TokenType:    "Bearer",
			ExpiresIn:    3600,
		},
	}

	// Token expires soon
	expiresAt := time.Now().Add(1 * time.Minute).Format(time.RFC3339)
	account := &Account{
		ID:       116,
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "old-token",
			"expires_at":   expiresAt,
		},
	}
	accountRepo.account = account
	cacheKey := ClaudeTokenCacheKey(account)

	// After lock is acquired, cache should have the token (simulating another worker)
	go func() {
		time.Sleep(5 * time.Millisecond)
		cache.mu.Lock()
		cache.tokens[cacheKey] = "cached-by-other-worker"
		cache.mu.Unlock()
	}()

	provider := &testClaudeTokenProvider{
		accountRepo:  accountRepo,
		tokenCache:   cache,
		oauthService: oauthService,
	}

	token, err := provider.GetAccessToken(context.Background(), account)
	require.NoError(t, err)
	require.NotEmpty(t, token)
}

// Tests for real provider - to increase coverage
func TestClaudeTokenProvider_Real_LockFailedWait(t *testing.T) {
	cache := newClaudeTokenCacheStub()
	cache.lockAcquired = false // Lock acquisition fails

	// Token expires soon (within refresh skew) to trigger lock attempt
	expiresAt := time.Now().Add(1 * time.Minute).Format(time.RFC3339)
	account := &Account{
		ID:       300,
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "fallback-token",
			"expires_at":   expiresAt,
		},
	}

	// Set token in cache after lock wait period (simulate other worker refreshing)
	cacheKey := ClaudeTokenCacheKey(account)
	go func() {
		time.Sleep(100 * time.Millisecond)
		cache.mu.Lock()
		cache.tokens[cacheKey] = "refreshed-by-other"
		cache.mu.Unlock()
	}()

	provider := NewClaudeTokenProvider(nil, cache, nil)
	token, err := provider.GetAccessToken(context.Background(), account)
	require.NoError(t, err)
	require.NotEmpty(t, token)
}

func TestClaudeTokenProvider_Real_CacheHitAfterWait(t *testing.T) {
	cache := newClaudeTokenCacheStub()
	cache.lockAcquired = false // Lock acquisition fails

	// Token expires soon
	expiresAt := time.Now().Add(1 * time.Minute).Format(time.RFC3339)
	account := &Account{
		ID:       301,
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "original-token",
			"expires_at":   expiresAt,
		},
	}

	cacheKey := ClaudeTokenCacheKey(account)
	// Set token in cache immediately after wait starts
	go func() {
		time.Sleep(50 * time.Millisecond)
		cache.mu.Lock()
		cache.tokens[cacheKey] = "winner-token"
		cache.mu.Unlock()
	}()

	provider := NewClaudeTokenProvider(nil, cache, nil)
	token, err := provider.GetAccessToken(context.Background(), account)
	require.NoError(t, err)
	require.NotEmpty(t, token)
}

func TestClaudeTokenProvider_Real_NoExpiresAt(t *testing.T) {
	cache := newClaudeTokenCacheStub()
	cache.lockAcquired = false // Prevent entering refresh logic

	// Token with nil expires_at (no expiry set)
	account := &Account{
		ID:       302,
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "no-expiry-token",
		},
	}

	// After lock wait, return token from credentials
	provider := NewClaudeTokenProvider(nil, cache, nil)
	token, err := provider.GetAccessToken(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, "no-expiry-token", token)
}

func TestClaudeTokenProvider_Real_WhitespaceToken(t *testing.T) {
	cache := newClaudeTokenCacheStub()
	cacheKey := "claude:account:303"
	cache.tokens[cacheKey] = "   " // Whitespace only - should be treated as empty

	expiresAt := time.Now().Add(1 * time.Hour).Format(time.RFC3339)
	account := &Account{
		ID:       303,
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "real-token",
			"expires_at":   expiresAt,
		},
	}

	provider := NewClaudeTokenProvider(nil, cache, nil)
	token, err := provider.GetAccessToken(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, "real-token", token)
}

func TestClaudeTokenProvider_Real_EmptyCredentialToken(t *testing.T) {
	cache := newClaudeTokenCacheStub()

	expiresAt := time.Now().Add(1 * time.Hour).Format(time.RFC3339)
	account := &Account{
		ID:       304,
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "   ", // Whitespace only
			"expires_at":   expiresAt,
		},
	}

	provider := NewClaudeTokenProvider(nil, cache, nil)
	token, err := provider.GetAccessToken(context.Background(), account)
	require.Error(t, err)
	require.Contains(t, err.Error(), "access_token not found")
	require.Empty(t, token)
}

func TestClaudeTokenProvider_Real_LockError(t *testing.T) {
	cache := newClaudeTokenCacheStub()
	cache.lockErr = errors.New("redis lock failed")

	// Token expires soon (within refresh skew)
	expiresAt := time.Now().Add(1 * time.Minute).Format(time.RFC3339)
	account := &Account{
		ID:       305,
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "fallback-on-lock-error",
			"expires_at":   expiresAt,
		},
	}

	provider := NewClaudeTokenProvider(nil, cache, nil)
	token, err := provider.GetAccessToken(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, "fallback-on-lock-error", token)
}

func TestClaudeTokenProvider_Real_NilCredentials(t *testing.T) {
	cache := newClaudeTokenCacheStub()

	expiresAt := time.Now().Add(1 * time.Hour).Format(time.RFC3339)
	account := &Account{
		ID:       306,
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"expires_at": expiresAt,
			// No access_token
		},
	}

	provider := NewClaudeTokenProvider(nil, cache, nil)
	token, err := provider.GetAccessToken(context.Background(), account)
	require.Error(t, err)
	require.Contains(t, err.Error(), "access_token not found")
	require.Empty(t, token)
}
