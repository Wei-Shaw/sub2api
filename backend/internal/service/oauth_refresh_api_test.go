//go:build unit

package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// ---------- mock helpers ----------

// refreshAPIAccountRepo implements AccountRepository for OAuthRefreshAPI tests.
type refreshAPIAccountRepo struct {
	mockAccountRepoForGemini
	account                *Account // returned by GetByID
	getByIDErr             error
	updateErr              error
	updateCalls            int
	updateCredentialsCalls int
}

func (r *refreshAPIAccountRepo) GetByID(_ context.Context, _ int64) (*Account, error) {
	if r.getByIDErr != nil {
		return nil, r.getByIDErr
	}
	return r.account, nil
}

func (r *refreshAPIAccountRepo) Update(_ context.Context, _ *Account) error {
	r.updateCalls++
	return r.updateErr
}

func (r *refreshAPIAccountRepo) UpdateCredentials(_ context.Context, id int64, credentials map[string]any) error {
	r.updateCalls++
	r.updateCredentialsCalls++
	if r.updateErr != nil {
		return r.updateErr
	}
	if r.account == nil || r.account.ID != id {
		r.account = &Account{ID: id}
	}
	r.account.Credentials = cloneCredentials(credentials)
	return nil
}

// refreshAPIExecutorStub implements OAuthRefreshExecutor for tests.
type refreshAPIExecutorStub struct {
	needsRefresh bool
	credentials  map[string]any
	err          error
	refreshCalls int
}

func (e *refreshAPIExecutorStub) CanRefresh(_ *Account) bool { return true }

func (e *refreshAPIExecutorStub) NeedsRefresh(_ *Account, _ time.Duration) bool {
	return e.needsRefresh
}

func (e *refreshAPIExecutorStub) Refresh(_ context.Context, _ *Account) (map[string]any, error) {
	e.refreshCalls++
	if e.err != nil {
		return nil, e.err
	}
	return e.credentials, nil
}

func (e *refreshAPIExecutorStub) CacheKey(account *Account) string {
	return "test:api:" + account.Platform
}

// refreshAPICacheStub implements GeminiTokenCache for OAuthRefreshAPI tests.
type refreshAPICacheStub struct {
	lockResult   bool
	lockErr      error
	releaseCalls int
}

func (c *refreshAPICacheStub) GetAccessToken(context.Context, string) (string, error) {
	return "", nil
}

func (c *refreshAPICacheStub) SetAccessToken(context.Context, string, string, time.Duration) error {
	return nil
}

func (c *refreshAPICacheStub) DeleteAccessToken(context.Context, string) error { return nil }

func (c *refreshAPICacheStub) AcquireRefreshLock(context.Context, string, time.Duration) (bool, error) {
	return c.lockResult, c.lockErr
}

func (c *refreshAPICacheStub) ReleaseRefreshLock(context.Context, string) error {
	c.releaseCalls++
	return nil
}

// ========== RefreshIfNeeded tests ==========

func TestRefreshIfNeeded_Success(t *testing.T) {
	account := &Account{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeOAuth}
	repo := &refreshAPIAccountRepo{account: account}
	cache := &refreshAPICacheStub{lockResult: true}
	executor := &refreshAPIExecutorStub{
		needsRefresh: true,
		credentials:  map[string]any{"access_token": "new-token"},
	}

	api := NewOAuthRefreshAPI(repo, cache)
	result, err := api.RefreshIfNeeded(context.Background(), account, executor, 3*time.Minute)

	require.NoError(t, err)
	require.True(t, result.Refreshed)
	require.NotNil(t, result.NewCredentials)
	require.Equal(t, "new-token", result.NewCredentials["access_token"])
	require.NotNil(t, result.NewCredentials["_token_version"]) // version stamp set
	require.Equal(t, 1, repo.updateCalls)                      // DB updated
	require.Equal(t, 1, repo.updateCredentialsCalls)
	require.Equal(t, 1, cache.releaseCalls) // lock released
	require.Equal(t, 1, executor.refreshCalls)
}

func TestRefreshIfNeeded_UpdateCredentialsPreservesRateLimitState(t *testing.T) {
	resetAt := time.Now().Add(45 * time.Minute)
	account := &Account{
		ID:               11,
		Platform:         PlatformGemini,
		Type:             AccountTypeOAuth,
		RateLimitResetAt: &resetAt,
	}
	repo := &refreshAPIAccountRepo{account: account}
	cache := &refreshAPICacheStub{lockResult: true}
	executor := &refreshAPIExecutorStub{
		needsRefresh: true,
		credentials:  map[string]any{"access_token": "safe-token"},
	}

	api := NewOAuthRefreshAPI(repo, cache)
	result, err := api.RefreshIfNeeded(context.Background(), account, executor, 3*time.Minute)

	require.NoError(t, err)
	require.True(t, result.Refreshed)
	require.Equal(t, 1, repo.updateCredentialsCalls)
	require.NotNil(t, repo.account.RateLimitResetAt)
	require.WithinDuration(t, resetAt, *repo.account.RateLimitResetAt, time.Second)
}

func TestRefreshIfNeeded_LockHeld(t *testing.T) {
	account := &Account{ID: 2, Platform: PlatformAnthropic}
	repo := &refreshAPIAccountRepo{account: account}
	cache := &refreshAPICacheStub{lockResult: false} // lock not acquired
	executor := &refreshAPIExecutorStub{needsRefresh: true}

	api := NewOAuthRefreshAPI(repo, cache)
	result, err := api.RefreshIfNeeded(context.Background(), account, executor, 3*time.Minute)

	require.NoError(t, err)
	require.True(t, result.LockHeld)
	require.False(t, result.Refreshed)
	require.Equal(t, 0, repo.updateCalls)
	require.Equal(t, 0, executor.refreshCalls)
}

func TestRefreshIfNeeded_LockErrorDegrades(t *testing.T) {
	account := &Account{ID: 3, Platform: PlatformGemini, Type: AccountTypeOAuth}
	repo := &refreshAPIAccountRepo{account: account}
	cache := &refreshAPICacheStub{lockErr: errors.New("redis down")} // lock error
	executor := &refreshAPIExecutorStub{
		needsRefresh: true,
		credentials:  map[string]any{"access_token": "degraded-token"},
	}

	api := NewOAuthRefreshAPI(repo, cache)
	result, err := api.RefreshIfNeeded(context.Background(), account, executor, 3*time.Minute)

	require.NoError(t, err)
	require.True(t, result.Refreshed)       // still refreshed (degraded mode)
	require.Equal(t, 1, repo.updateCalls)   // DB updated
	require.Equal(t, 0, cache.releaseCalls) // no lock to release
	require.Equal(t, 1, executor.refreshCalls)
}

func TestRefreshIfNeeded_NoCacheNoLock(t *testing.T) {
	account := &Account{ID: 4, Platform: PlatformGemini, Type: AccountTypeOAuth}
	repo := &refreshAPIAccountRepo{account: account}
	executor := &refreshAPIExecutorStub{
		needsRefresh: true,
		credentials:  map[string]any{"access_token": "no-cache-token"},
	}

	api := NewOAuthRefreshAPI(repo, nil) // no cache = no lock
	result, err := api.RefreshIfNeeded(context.Background(), account, executor, 3*time.Minute)

	require.NoError(t, err)
	require.True(t, result.Refreshed)
	require.Equal(t, 1, repo.updateCalls)
}

func TestRefreshIfNeeded_AlreadyRefreshed(t *testing.T) {
	account := &Account{ID: 5, Platform: PlatformAnthropic}
	repo := &refreshAPIAccountRepo{account: account}
	cache := &refreshAPICacheStub{lockResult: true}
	executor := &refreshAPIExecutorStub{needsRefresh: false} // already refreshed

	api := NewOAuthRefreshAPI(repo, cache)
	result, err := api.RefreshIfNeeded(context.Background(), account, executor, 3*time.Minute)

	require.NoError(t, err)
	require.False(t, result.Refreshed)
	require.False(t, result.LockHeld)
	require.NotNil(t, result.Account) // returns fresh account
	require.Equal(t, 0, repo.updateCalls)
	require.Equal(t, 0, executor.refreshCalls)
}

func TestRefreshIfNeeded_RefreshError(t *testing.T) {
	account := &Account{ID: 6, Platform: PlatformAnthropic}
	repo := &refreshAPIAccountRepo{account: account}
	cache := &refreshAPICacheStub{lockResult: true}
	executor := &refreshAPIExecutorStub{
		needsRefresh: true,
		err:          errors.New("invalid_grant: token revoked"),
	}

	api := NewOAuthRefreshAPI(repo, cache)
	result, err := api.RefreshIfNeeded(context.Background(), account, executor, 3*time.Minute)

	require.Error(t, err)
	require.Nil(t, result)
	require.Contains(t, err.Error(), "invalid_grant")
	require.Equal(t, 0, repo.updateCalls)   // no DB update on refresh error
	require.Equal(t, 1, cache.releaseCalls) // lock still released via defer
}

func TestRefreshIfNeeded_DBUpdateError(t *testing.T) {
	account := &Account{ID: 7, Platform: PlatformGemini, Type: AccountTypeOAuth}
	repo := &refreshAPIAccountRepo{
		account:   account,
		updateErr: errors.New("db connection lost"),
	}
	cache := &refreshAPICacheStub{lockResult: true}
	executor := &refreshAPIExecutorStub{
		needsRefresh: true,
		credentials:  map[string]any{"access_token": "token"},
	}

	api := NewOAuthRefreshAPI(repo, cache)
	result, err := api.RefreshIfNeeded(context.Background(), account, executor, 3*time.Minute)

	require.Error(t, err)
	require.Nil(t, result)
	require.Contains(t, err.Error(), "DB update failed")
	require.Equal(t, 1, repo.updateCalls) // attempted
}

func TestRefreshIfNeeded_DBRereadFails(t *testing.T) {
	account := &Account{ID: 8, Platform: PlatformAnthropic, Type: AccountTypeOAuth}
	repo := &refreshAPIAccountRepo{
		account:    nil, // GetByID returns nil
		getByIDErr: errors.New("db timeout"),
	}
	cache := &refreshAPICacheStub{lockResult: true}
	executor := &refreshAPIExecutorStub{
		needsRefresh: true,
		credentials:  map[string]any{"access_token": "fallback-token"},
	}

	api := NewOAuthRefreshAPI(repo, cache)
	result, err := api.RefreshIfNeeded(context.Background(), account, executor, 3*time.Minute)

	require.NoError(t, err)
	require.True(t, result.Refreshed)
	require.Equal(t, 1, executor.refreshCalls) // still refreshes using passed-in account
}

func TestRefreshIfNeeded_NilCredentials(t *testing.T) {
	account := &Account{ID: 9, Platform: PlatformGemini, Type: AccountTypeOAuth}
	repo := &refreshAPIAccountRepo{account: account}
	cache := &refreshAPICacheStub{lockResult: true}
	executor := &refreshAPIExecutorStub{
		needsRefresh: true,
		credentials:  nil, // Refresh returns nil credentials
	}

	api := NewOAuthRefreshAPI(repo, cache)
	result, err := api.RefreshIfNeeded(context.Background(), account, executor, 3*time.Minute)

	require.NoError(t, err)
	require.True(t, result.Refreshed)
	require.Nil(t, result.NewCredentials)
	require.Equal(t, 0, repo.updateCalls) // no DB update when credentials are nil
}

// ========== MergeCredentials tests ==========

func TestMergeCredentials_Basic(t *testing.T) {
	old := map[string]any{"a": "1", "b": "2", "c": "3"}
	new := map[string]any{"a": "new", "d": "4"}

	result := MergeCredentials(old, new)

	require.Equal(t, "new", result["a"]) // new value preserved
	require.Equal(t, "2", result["b"])   // old value kept
	require.Equal(t, "3", result["c"])   // old value kept
	require.Equal(t, "4", result["d"])   // new value preserved
}

func TestMergeCredentials_NilNew(t *testing.T) {
	old := map[string]any{"a": "1"}

	result := MergeCredentials(old, nil)

	require.NotNil(t, result)
	require.Equal(t, "1", result["a"])
}

func TestMergeCredentials_NilOld(t *testing.T) {
	new := map[string]any{"a": "1"}

	result := MergeCredentials(nil, new)

	require.Equal(t, "1", result["a"])
}

func TestMergeCredentials_BothNil(t *testing.T) {
	result := MergeCredentials(nil, nil)
	require.NotNil(t, result)
	require.Empty(t, result)
}

func TestMergeCredentials_NewOverridesOld(t *testing.T) {
	old := map[string]any{"access_token": "old-token", "refresh_token": "old-refresh"}
	new := map[string]any{"access_token": "new-token"}

	result := MergeCredentials(old, new)

	require.Equal(t, "new-token", result["access_token"])    // overridden
	require.Equal(t, "old-refresh", result["refresh_token"]) // preserved
}

// ========== BuildClaudeAccountCredentials tests ==========

func TestBuildClaudeAccountCredentials_Full(t *testing.T) {
	tokenInfo := &TokenInfo{
		AccessToken:  "at-123",
		TokenType:    "Bearer",
		ExpiresIn:    3600,
		ExpiresAt:    1700000000,
		RefreshToken: "rt-456",
		Scope:        "openid",
	}

	creds := BuildClaudeAccountCredentials(tokenInfo)

	require.Equal(t, "at-123", creds["access_token"])
	require.Equal(t, "Bearer", creds["token_type"])
	require.Equal(t, "3600", creds["expires_in"])
	require.Equal(t, "1700000000", creds["expires_at"])
	require.Equal(t, "rt-456", creds["refresh_token"])
	require.Equal(t, "openid", creds["scope"])
}

func TestBuildClaudeAccountCredentials_Minimal(t *testing.T) {
	tokenInfo := &TokenInfo{
		AccessToken: "at-789",
		TokenType:   "Bearer",
		ExpiresIn:   7200,
		ExpiresAt:   1700003600,
	}

	creds := BuildClaudeAccountCredentials(tokenInfo)

	require.Equal(t, "at-789", creds["access_token"])
	require.Equal(t, "Bearer", creds["token_type"])
	require.Equal(t, "7200", creds["expires_in"])
	require.Equal(t, "1700003600", creds["expires_at"])
	_, hasRefresh := creds["refresh_token"]
	_, hasScope := creds["scope"]
	require.False(t, hasRefresh, "refresh_token should not be set when empty")
	require.False(t, hasScope, "scope should not be set when empty")
}

// refreshAPIAccountRepoWithRace supports returning a different account on subsequent GetByID calls
// to simulate race conditions where another worker has refreshed the token.
type refreshAPIAccountRepoWithRace struct {
	refreshAPIAccountRepo
	raceAccount  *Account // returned on 2nd+ GetByID call
	getByIDCalls int
}

func (r *refreshAPIAccountRepoWithRace) GetByID(_ context.Context, _ int64) (*Account, error) {
	r.getByIDCalls++
	if r.getByIDCalls > 1 && r.raceAccount != nil {
		return r.raceAccount, nil
	}
	if r.getByIDErr != nil {
		return nil, r.getByIDErr
	}
	return r.account, nil
}

// ========== Race recovery tests ==========

func TestRefreshIfNeeded_InvalidGrantRaceRecovered(t *testing.T) {
	// Account with old refresh token
	account := &Account{
		ID:          10,
		Platform:    PlatformAnthropic,
		Type:        AccountTypeOAuth,
		Credentials: map[string]any{"refresh_token": "old-rt", "access_token": "old-at"},
	}
	// After race, DB has new refresh token from another worker
	racedAccount := &Account{
		ID:          10,
		Platform:    PlatformAnthropic,
		Type:        AccountTypeOAuth,
		Credentials: map[string]any{"refresh_token": "new-rt", "access_token": "new-at"},
	}
	repo := &refreshAPIAccountRepoWithRace{
		refreshAPIAccountRepo: refreshAPIAccountRepo{account: account},
		raceAccount:           racedAccount,
	}
	cache := &refreshAPICacheStub{lockResult: true}
	executor := &refreshAPIExecutorStub{
		needsRefresh: true,
		err:          errors.New("invalid_grant: refresh token not found or invalid"),
	}

	api := NewOAuthRefreshAPI(repo, cache)
	result, err := api.RefreshIfNeeded(context.Background(), account, executor, 3*time.Minute)

	require.NoError(t, err, "race-recovered invalid_grant should not return error")
	require.False(t, result.Refreshed)
	require.False(t, result.LockHeld)
	require.NotNil(t, result.Account)
	require.Equal(t, "new-rt", result.Account.GetCredential("refresh_token"))
	require.Equal(t, 0, repo.updateCalls) // no DB update needed, another worker did it
}

func TestRefreshIfNeeded_InvalidGrantGenuine(t *testing.T) {
	// Account with revoked refresh token - DB still has the same token
	account := &Account{
		ID:          11,
		Platform:    PlatformAnthropic,
		Type:        AccountTypeOAuth,
		Credentials: map[string]any{"refresh_token": "revoked-rt", "access_token": "old-at"},
	}
	repo := &refreshAPIAccountRepoWithRace{
		refreshAPIAccountRepo: refreshAPIAccountRepo{account: account},
		raceAccount:           account, // same refresh_token on re-read
	}
	cache := &refreshAPICacheStub{lockResult: true}
	executor := &refreshAPIExecutorStub{
		needsRefresh: true,
		err:          errors.New("invalid_grant: refresh token revoked"),
	}

	api := NewOAuthRefreshAPI(repo, cache)
	result, err := api.RefreshIfNeeded(context.Background(), account, executor, 3*time.Minute)

	require.Error(t, err, "genuine invalid_grant should propagate error")
	require.Nil(t, result)
	require.Contains(t, err.Error(), "invalid_grant")
}

func TestRefreshIfNeeded_InvalidGrantDBRereadFailsOnRecovery(t *testing.T) {
	account := &Account{
		ID:          12,
		Platform:    PlatformAnthropic,
		Type:        AccountTypeOAuth,
		Credentials: map[string]any{"refresh_token": "old-rt"},
	}
	repo := &refreshAPIAccountRepoWithRace{
		refreshAPIAccountRepo: refreshAPIAccountRepo{account: account},
		raceAccount:           nil, // GetByID returns nil on recovery attempt
	}
	cache := &refreshAPICacheStub{lockResult: true}
	executor := &refreshAPIExecutorStub{
		needsRefresh: true,
		err:          errors.New("invalid_grant"),
	}

	api := NewOAuthRefreshAPI(repo, cache)
	result, err := api.RefreshIfNeeded(context.Background(), account, executor, 3*time.Minute)

	require.Error(t, err, "should propagate error when recovery DB re-read fails")
	require.Nil(t, result)
}

func TestRefreshIfNeeded_LocalMutexSerializesConcurrent(t *testing.T) {
	// Test that two goroutines for the same account are serialized by the local mutex.
	// The first goroutine refreshes successfully; the second sees NeedsRefresh=false.
	refreshed := &Account{
		ID:          20,
		Platform:    PlatformAnthropic,
		Type:        AccountTypeOAuth,
		Credentials: map[string]any{"refresh_token": "new-rt", "access_token": "new-at"},
	}
	callCount := 0
	repo := &refreshAPIAccountRepo{account: &Account{
		ID:          20,
		Platform:    PlatformAnthropic,
		Type:        AccountTypeOAuth,
		Credentials: map[string]any{"refresh_token": "old-rt"},
	}}

	// After first refresh, NeedsRefresh should return false
	// We simulate this by using an executor that decrements needsRefresh after first call
	var mu sync.Mutex
	dynamicExecutor := &dynamicRefreshExecutor{
		canRefresh: true,
		cacheKey:   "test:mutex:anthropic",
		refreshFunc: func(_ context.Context, _ *Account) (map[string]any, error) {
			mu.Lock()
			callCount++
			mu.Unlock()
			time.Sleep(50 * time.Millisecond) // slow refresh
			return map[string]any{"access_token": "new-at"}, nil
		},
		needsRefreshFunc: func() bool {
			mu.Lock()
			defer mu.Unlock()
			return callCount == 0 // only first call needs refresh
		},
	}

	_ = refreshed

	api := NewOAuthRefreshAPI(repo, nil) // no distributed lock, only local mutex

	var wg sync.WaitGroup
	results := make([]*OAuthRefreshResult, 2)
	errs := make([]error, 2)

	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx], errs[idx] = api.RefreshIfNeeded(context.Background(), repo.account, dynamicExecutor, 3*time.Minute)
		}(i)
	}
	wg.Wait()

	require.NoError(t, errs[0])
	require.NoError(t, errs[1])

	// Only one goroutine should have actually called Refresh
	mu.Lock()
	require.Equal(t, 1, callCount, "only one refresh call should have been made")
	mu.Unlock()
}

// dynamicRefreshExecutor is a test helper with function-based NeedsRefresh and Refresh.
type dynamicRefreshExecutor struct {
	canRefresh       bool
	cacheKey         string
	needsRefreshFunc func() bool
	refreshFunc      func(context.Context, *Account) (map[string]any, error)
}

func (e *dynamicRefreshExecutor) CanRefresh(_ *Account) bool { return e.canRefresh }

func (e *dynamicRefreshExecutor) NeedsRefresh(_ *Account, _ time.Duration) bool {
	return e.needsRefreshFunc()
}

func (e *dynamicRefreshExecutor) Refresh(ctx context.Context, account *Account) (map[string]any, error) {
	return e.refreshFunc(ctx, account)
}

func (e *dynamicRefreshExecutor) CacheKey(_ *Account) string {
	return e.cacheKey
}

// ========== NewOAuthRefreshAPI TTL tests ==========

func TestNewOAuthRefreshAPI_DefaultTTL(t *testing.T) {
	api := NewOAuthRefreshAPI(nil, nil)
	require.Equal(t, defaultRefreshLockTTL, api.lockTTL)
}

func TestNewOAuthRefreshAPI_CustomTTL(t *testing.T) {
	api := NewOAuthRefreshAPI(nil, nil, 90*time.Second)
	require.Equal(t, 90*time.Second, api.lockTTL)
}

func TestNewOAuthRefreshAPI_ZeroTTLUsesDefault(t *testing.T) {
	api := NewOAuthRefreshAPI(nil, nil, 0)
	require.Equal(t, defaultRefreshLockTTL, api.lockTTL)
}

// ========== isInvalidGrantError tests ==========

func TestIsInvalidGrantError(t *testing.T) {
	require.True(t, isInvalidGrantError(errors.New("invalid_grant: token revoked")))
	require.True(t, isInvalidGrantError(errors.New("INVALID_GRANT")))
	require.False(t, isInvalidGrantError(errors.New("invalid_client")))
	require.False(t, isInvalidGrantError(nil))
}

// ========== BackgroundRefreshPolicy tests ==========

func TestBackgroundRefreshPolicy_DefaultSkips(t *testing.T) {
	p := DefaultBackgroundRefreshPolicy()

	require.ErrorIs(t, p.handleLockHeld(), errRefreshSkipped)
	require.ErrorIs(t, p.handleAlreadyRefreshed(), errRefreshSkipped)
}

func TestBackgroundRefreshPolicy_SuccessOverride(t *testing.T) {
	p := BackgroundRefreshPolicy{
		OnLockHeld:       BackgroundSkipAsSuccess,
		OnAlreadyRefresh: BackgroundSkipAsSuccess,
	}

	require.NoError(t, p.handleLockHeld())
	require.NoError(t, p.handleAlreadyRefreshed())
}

// ========== ProviderRefreshPolicy tests ==========

func TestClaudeProviderRefreshPolicy(t *testing.T) {
	p := ClaudeProviderRefreshPolicy()
	require.Equal(t, ProviderRefreshErrorUseExistingToken, p.OnRefreshError)
	require.Equal(t, ProviderLockHeldWaitForCache, p.OnLockHeld)
	require.Equal(t, time.Minute, p.FailureTTL)
}

func TestOpenAIProviderRefreshPolicy(t *testing.T) {
	p := OpenAIProviderRefreshPolicy()
	require.Equal(t, ProviderRefreshErrorUseExistingToken, p.OnRefreshError)
	require.Equal(t, ProviderLockHeldWaitForCache, p.OnLockHeld)
	require.Equal(t, time.Minute, p.FailureTTL)
}

func TestGeminiProviderRefreshPolicy(t *testing.T) {
	p := GeminiProviderRefreshPolicy()
	require.Equal(t, ProviderRefreshErrorReturn, p.OnRefreshError)
	require.Equal(t, ProviderLockHeldUseExistingToken, p.OnLockHeld)
	require.Equal(t, time.Duration(0), p.FailureTTL)
}

func TestAntigravityProviderRefreshPolicy(t *testing.T) {
	p := AntigravityProviderRefreshPolicy()
	require.Equal(t, ProviderRefreshErrorReturn, p.OnRefreshError)
	require.Equal(t, ProviderLockHeldUseExistingToken, p.OnLockHeld)
	require.Equal(t, time.Duration(0), p.FailureTTL)
}
