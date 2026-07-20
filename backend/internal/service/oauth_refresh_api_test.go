//go:build unit

package service

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// ---------- mock helpers ----------

// refreshAPIAccountRepo implements AccountRepository for OAuthRefreshAPI tests.
type refreshAPIAccountRepo struct {
	mockAccountRepoForGemini
	account                 *Account // returned by GetByID
	getByIDErr              error
	getByIDCalls            int
	getByIDErrAfterCall     int
	getByIDErrAfterCallErr  error
	updateErr               error
	updateCalls             int
	updateCredentialsCalls  int
	successCASCalls         int
	beforeSuccessCAS        func(*refreshAPIAccountRepo)
	lastExpectedCredentials map[string]any
	lastExpectedProxyID     *int64
}

func (r *refreshAPIAccountRepo) GetByID(_ context.Context, _ int64) (*Account, error) {
	r.getByIDCalls++
	if r.getByIDErrAfterCall > 0 && r.getByIDCalls >= r.getByIDErrAfterCall {
		return nil, r.getByIDErrAfterCallErr
	}
	if r.getByIDErr != nil {
		return nil, r.getByIDErr
	}
	return activeRefreshAPITestAccount(r.account), nil
}

func activeRefreshAPITestAccount(account *Account) *Account {
	if account == nil || account.Status != "" {
		return account
	}
	copy := *account
	copy.Status = StatusActive
	return &copy
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
	r.account.Credentials = shallowCopyMap(credentials)
	return nil
}

func (r *refreshAPIAccountRepo) UpdateGrokOAuthCredentialsIfUnchanged(
	_ context.Context,
	id int64,
	expectedCredentials map[string]any,
	expectedProxyID *int64,
	credentials map[string]any,
) (bool, error) {
	r.successCASCalls++
	r.lastExpectedCredentials = shallowCopyMap(expectedCredentials)
	if expectedProxyID != nil {
		proxyID := *expectedProxyID
		r.lastExpectedProxyID = &proxyID
	} else {
		r.lastExpectedProxyID = nil
	}
	if r.beforeSuccessCAS != nil {
		r.beforeSuccessCAS(r)
	}
	if r.updateErr != nil {
		return false, r.updateErr
	}
	if r.account == nil || r.account.ID != id || r.account.Platform != PlatformGrok ||
		r.account.Type != AccountTypeOAuth ||
		!reflect.DeepEqual(r.account.Credentials, expectedCredentials) ||
		!reflect.DeepEqual(r.account.ProxyID, expectedProxyID) {
		return false, nil
	}
	r.updateCalls++
	r.updateCredentialsCalls++
	r.account.Credentials = shallowCopyMap(credentials)
	return true, nil
}

// refreshAPIExecutorStub implements OAuthRefreshExecutor for tests.
type refreshAPIExecutorStub struct {
	needsRefresh  bool
	cannotRefresh bool
	credentials   map[string]any
	err           error
	refreshCalls  int
	canRefresh    func(*Account) bool
	onRefresh     func()
	delay         time.Duration
}

func (e *refreshAPIExecutorStub) CanRefresh(account *Account) bool {
	if e.cannotRefresh {
		return false
	}
	if e.canRefresh != nil {
		return e.canRefresh(account)
	}
	return true
}

func (e *refreshAPIExecutorStub) NeedsRefresh(_ *Account, _ time.Duration) bool {
	return e.needsRefresh
}

func (e *refreshAPIExecutorStub) Refresh(_ context.Context, _ *Account) (map[string]any, error) {
	e.refreshCalls++
	if e.delay > 0 {
		time.Sleep(e.delay)
	}
	if e.onRefresh != nil {
		e.onRefresh()
	}
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
	lockResult    bool
	lockErr       error
	releaseCalls  int
	releaseCtxErr error
	deleteCalls   int
	deleteKey     string
	deleteCtxErr  error
}

func (c *refreshAPICacheStub) GetAccessToken(context.Context, string) (string, error) {
	return "", nil
}

func (c *refreshAPICacheStub) SetAccessToken(context.Context, string, string, time.Duration) error {
	return nil
}

func (c *refreshAPICacheStub) DeleteAccessToken(ctx context.Context, key string) error {
	c.deleteCalls++
	c.deleteKey = key
	c.deleteCtxErr = ctx.Err()
	return nil
}

func (c *refreshAPICacheStub) AcquireRefreshLock(context.Context, string, time.Duration) (bool, error) {
	return c.lockResult, c.lockErr
}

func (c *refreshAPICacheStub) ReleaseRefreshLock(ctx context.Context, _ string) error {
	c.releaseCalls++
	c.releaseCtxErr = ctx.Err()
	return nil
}

// ========== RefreshIfNeeded tests ==========

func TestRefreshIfNeeded_Success(t *testing.T) {
	account := &Account{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeOAuth, Status: StatusActive}
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
		Status:           StatusActive,
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
	account := &Account{ID: 2, Platform: PlatformAnthropic, Status: StatusActive}
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
	account := &Account{ID: 3, Platform: PlatformGemini, Type: AccountTypeOAuth, Status: StatusActive}
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
	account := &Account{ID: 4, Platform: PlatformGemini, Type: AccountTypeOAuth, Status: StatusActive}
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
	account := &Account{ID: 5, Platform: PlatformAnthropic, Status: StatusActive}
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
	account := &Account{ID: 6, Platform: PlatformAnthropic, Status: StatusActive}
	repo := &refreshAPIAccountRepo{account: account}
	cache := &refreshAPICacheStub{lockResult: true}
	executor := &refreshAPIExecutorStub{
		needsRefresh: true,
		err:          errors.New("invalid_grant: token revoked"),
	}

	api := NewOAuthRefreshAPI(repo, cache)
	result, err := api.RefreshIfNeeded(context.Background(), account, executor, 3*time.Minute)

	require.Error(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.Account)
	require.Equal(t, account.ID, result.Account.ID)
	require.Contains(t, err.Error(), "invalid_grant")
	require.Equal(t, 0, repo.updateCalls)   // no DB update on refresh error
	require.Equal(t, 1, cache.releaseCalls) // lock still released via defer
}

func TestRefreshIfNeeded_DBUpdateError(t *testing.T) {
	account := &Account{ID: 7, Platform: PlatformGemini, Type: AccountTypeOAuth, Status: StatusActive}
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
	require.ErrorIs(t, err, errOAuthRefreshCredentialPersist)
	require.Equal(t, 1, repo.updateCalls) // attempted
}

func TestRefreshIfNeeded_GrokSuccessCASLetsConcurrentReauthorizationWin(t *testing.T) {
	proxyID := int64(17)
	account := &Account{
		ID:       70,
		Platform: PlatformGrok,
		Type:     AccountTypeOAuth,
		Status:   StatusActive,
		ProxyID:  &proxyID,
		Credentials: map[string]any{
			"access_token":   "attempted-access",
			"refresh_token":  "attempted-refresh",
			"_token_version": int64(1),
		},
	}
	repo := &refreshAPIAccountRepo{account: account}
	repo.beforeSuccessCAS = func(r *refreshAPIAccountRepo) {
		repairedProxyID := int64(23)
		r.account.ProxyID = &repairedProxyID
		r.account.Credentials = map[string]any{
			"access_token":   "reauthorized-access",
			"refresh_token":  "reauthorized-refresh",
			"_token_version": int64(2),
		}
	}
	executor := &refreshAPIExecutorStub{
		needsRefresh: true,
		credentials: map[string]any{
			"access_token":  "provider-access",
			"refresh_token": "provider-refresh",
		},
	}

	result, err := NewOAuthRefreshAPI(repo, nil).RefreshIfNeeded(context.Background(), account, executor, time.Hour)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.Refreshed, "a lost success CAS is an already-refreshed skip")
	require.Nil(t, result.NewCredentials)
	require.Equal(t, "reauthorized-refresh", result.Account.GetGrokRefreshToken())
	require.NotNil(t, result.Account.ProxyID)
	require.Equal(t, int64(23), *result.Account.ProxyID)
	require.Equal(t, 1, repo.successCASCalls)
	require.Equal(t, "attempted-refresh", repo.lastExpectedCredentials["refresh_token"])
	require.NotNil(t, repo.lastExpectedProxyID)
	require.Equal(t, proxyID, *repo.lastExpectedProxyID)
	require.Zero(t, repo.updateCredentialsCalls, "the provider result must not overwrite a concurrent repair")
}

func TestRefreshIfNeeded_GrokSuccessPersistenceFailureIsProviderContainment(t *testing.T) {
	account := &Account{
		ID:       71,
		Platform: PlatformGrok,
		Type:     AccountTypeOAuth,
		Status:   StatusActive,
		Credentials: map[string]any{
			"access_token":  "attempted-access",
			"refresh_token": "attempted-refresh",
		},
	}
	repo := &refreshAPIAccountRepo{account: account, updateErr: errors.New("database unavailable")}
	executor := &refreshAPIExecutorStub{
		needsRefresh: true,
		credentials: map[string]any{
			"access_token":  "provider-access",
			"refresh_token": "provider-refresh",
		},
	}

	result, err := NewOAuthRefreshAPI(repo, nil).RefreshIfNeeded(context.Background(), account, executor, time.Hour)

	require.Error(t, err)
	require.Nil(t, result)
	var containmentErr *providerCycleContainmentRefreshError
	require.ErrorAs(t, err, &containmentErr)
	require.Equal(t, "attempted-refresh", account.GetGrokRefreshToken(),
		"an ambiguous persistence result must not mutate the in-memory account")
	require.Equal(t, 1, repo.successCASCalls)
	require.Zero(t, repo.updateCredentialsCalls)
}

func TestRefreshIfNeeded_GrokSuccessDurableRereadFailureIsProviderContainment(t *testing.T) {
	account := &Account{
		ID:       72,
		Platform: PlatformGrok,
		Type:     AccountTypeOAuth,
		Status:   StatusActive,
		Credentials: map[string]any{
			"access_token":  "attempted-access",
			"refresh_token": "attempted-refresh",
		},
	}
	repo := &refreshAPIAccountRepo{
		account:                account,
		getByIDErrAfterCall:    2,
		getByIDErrAfterCallErr: errors.New("durable state unavailable"),
	}
	cache := &refreshAPICacheStub{lockResult: true}
	executor := &refreshAPIExecutorStub{
		needsRefresh: true,
		credentials: map[string]any{
			"access_token":  "provider-access",
			"refresh_token": "provider-refresh",
		},
	}

	result, err := NewOAuthRefreshAPI(repo, cache).RefreshIfNeeded(context.Background(), account, executor, time.Hour)

	require.Error(t, err)
	require.Nil(t, result)
	var containmentErr *providerCycleContainmentRefreshError
	require.ErrorAs(t, err, &containmentErr)
	require.Equal(t, 2, repo.getByIDCalls)
	require.Equal(t, 1, repo.successCASCalls)
	require.Equal(t, 1, cache.deleteCalls, "a committed credential rotation must invalidate the pre-rotation access-token cache")
	require.NoError(t, cache.deleteCtxErr)
}

func TestRefreshIfNeeded_DBRereadFails(t *testing.T) {
	account := &Account{ID: 8, Platform: PlatformAnthropic, Type: AccountTypeOAuth, Status: StatusActive}
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

	require.Error(t, err)
	var stateUnavailable *oauthRefreshStateUnavailableError
	require.ErrorAs(t, err, &stateUnavailable)
	require.Nil(t, result)
	require.Zero(t, executor.refreshCalls, "a failed DB reread must not refresh stale credentials")
	require.Zero(t, repo.updateCalls)
	require.Equal(t, 1, cache.releaseCalls)
}

func TestRefreshIfNeeded_RequestPathDBRereadNilFailsClosed(t *testing.T) {
	account := &Account{ID: 81, Platform: PlatformGrok, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true}
	repo := &refreshAPIAccountRepo{}
	cache := &refreshAPICacheStub{lockResult: true}
	executor := &refreshAPIExecutorStub{needsRefresh: true}

	api := NewOAuthRefreshAPI(repo, cache)
	result, err := api.RefreshIfNeeded(withOAuthRefreshRequestPath(context.Background()), account, executor, 3*time.Minute)

	require.ErrorIs(t, err, errOAuthRefreshAccountStateChanged)
	require.Nil(t, result)
	require.Zero(t, executor.refreshCalls)
	require.Zero(t, repo.updateCalls)
	require.Equal(t, 1, cache.releaseCalls)
}

func TestRefreshIfNeeded_RequestPathDBRereadInactiveFailsClosed(t *testing.T) {
	account := &Account{ID: 82, Platform: PlatformGrok, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true}
	freshAccount := &Account{ID: account.ID, Platform: PlatformGrok, Type: AccountTypeOAuth, Status: StatusDisabled}
	repo := &refreshAPIAccountRepo{account: freshAccount}
	executor := &refreshAPIExecutorStub{needsRefresh: true}

	api := NewOAuthRefreshAPI(repo, nil)
	result, err := api.RefreshIfNeeded(withOAuthRefreshRequestPath(context.Background()), account, executor, 3*time.Minute)

	require.ErrorContains(t, err, "account is not active")
	require.Nil(t, result)
	require.Zero(t, executor.refreshCalls)
	require.Zero(t, repo.updateCalls)
}

func TestRefreshIfNeeded_RequestPathDBRereadRevalidatesExecutorContract(t *testing.T) {
	tests := []struct {
		name          string
		freshPlatform string
		freshType     string
	}{
		{name: "platform changed", freshPlatform: PlatformAnthropic, freshType: AccountTypeOAuth},
		{name: "type changed", freshPlatform: PlatformGrok, freshType: AccountTypeUpstream},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := &Account{ID: 83, Platform: PlatformGrok, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true}
			freshAccount := &Account{ID: account.ID, Platform: tt.freshPlatform, Type: tt.freshType, Status: StatusActive, Schedulable: true}
			repo := &refreshAPIAccountRepo{account: freshAccount}
			executor := NewGrokTokenRefresher(nil)

			api := NewOAuthRefreshAPI(repo, nil)
			result, err := api.RefreshIfNeeded(withOAuthRefreshRequestPath(context.Background()), account, executor, 3*time.Minute)

			require.ErrorIs(t, err, errOAuthRefreshAccountStateChanged)
			require.Nil(t, result)
			require.Zero(t, repo.updateCalls)
		})
	}
}

func TestRefreshIfNeeded_LocalLockWaitHonorsContext(t *testing.T) {
	account := &Account{ID: 80, Platform: PlatformGrok, Type: AccountTypeOAuth, Status: StatusActive}
	repo := &refreshAPIAccountRepo{account: account}
	executor := &refreshAPIExecutorStub{needsRefresh: true}
	api := NewOAuthRefreshAPI(repo, nil)
	lock := api.getLocalLock(executor.CacheKey(account))
	require.NoError(t, lock.Lock(context.Background()))
	defer lock.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	result, err := api.RefreshIfNeeded(ctx, account, executor, time.Hour)

	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Nil(t, result)
	require.Zero(t, executor.refreshCalls)
}

func TestRefreshIfNeeded_ReleasesDistributedLockAfterParentCancellation(t *testing.T) {
	account := &Account{ID: 81, Platform: PlatformGrok, Type: AccountTypeOAuth, Status: StatusActive}
	repo := &refreshAPIAccountRepo{account: account}
	cache := &refreshAPICacheStub{lockResult: true}
	ctx, cancel := context.WithCancel(context.Background())
	executor := &refreshAPIExecutorStub{
		needsRefresh: true,
		err:          errors.New("temporary provider error"),
		onRefresh:    cancel,
	}
	api := NewOAuthRefreshAPI(repo, cache)

	_, err := api.RefreshIfNeeded(ctx, account, executor, time.Hour)

	require.Error(t, err)
	require.Equal(t, 1, cache.releaseCalls)
	require.NoError(t, cache.releaseCtxErr, "lock cleanup must not reuse the canceled attempt context")
}

func TestRefreshIfNeeded_RevalidatesFreshAccountBeforeRefresh(t *testing.T) {
	selected := &Account{ID: 82, Platform: PlatformGrok, Type: AccountTypeOAuth, Status: StatusActive}
	tests := []struct {
		name  string
		fresh *Account
	}{
		{name: "converted to API key", fresh: &Account{ID: 82, Platform: PlatformGrok, Type: AccountTypeAPIKey, Status: StatusActive}},
		{name: "disabled", fresh: &Account{ID: 82, Platform: PlatformGrok, Type: AccountTypeOAuth, Status: StatusDisabled}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &refreshAPIAccountRepo{account: tt.fresh}
			executor := &refreshAPIExecutorStub{
				needsRefresh: true,
				canRefresh: func(account *Account) bool {
					return account.Platform == PlatformGrok && account.Type == AccountTypeOAuth
				},
			}
			api := NewOAuthRefreshAPI(repo, nil)

			result, err := api.RefreshIfNeeded(context.Background(), selected, executor, time.Hour)

			require.NoError(t, err)
			require.False(t, result.Refreshed)
			require.Zero(t, executor.refreshCalls)
			require.Zero(t, repo.updateCalls)
		})
	}
}

func TestRefreshIfNeeded_RequestPathDBRereadMissingGrokRefreshCredentialReturnsPermanentSignal(t *testing.T) {
	account := &Account{
		ID:          84,
		Platform:    PlatformGrok,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Credentials: map[string]any{
			"refresh_token": "caller-snapshot-refresh-token",
		},
	}
	freshAccount := &Account{ID: account.ID, Platform: PlatformGrok, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true}
	repo := &refreshAPIAccountRepo{account: freshAccount}
	executor := NewGrokTokenRefresher(nil)

	api := NewOAuthRefreshAPI(repo, nil)
	result, err := api.RefreshIfNeeded(withOAuthRefreshRequestPath(context.Background()), account, executor, 3*time.Minute)

	require.ErrorIs(t, err, errGrokOAuthRefreshTokenMissing)
	require.Nil(t, result)
	require.Zero(t, repo.updateCalls)
}

func TestRefreshIfNeeded_LateSuccessAfterDeadlineDoesNotPersist(t *testing.T) {
	account := &Account{ID: 85, Platform: PlatformGrok, Type: AccountTypeOAuth, Status: StatusActive}
	repo := &refreshAPIAccountRepo{account: account}
	executor := &refreshAPIExecutorStub{
		needsRefresh: true,
		credentials:  map[string]any{"access_token": "late-token"},
		delay:        30 * time.Millisecond,
	}
	api := NewOAuthRefreshAPI(repo, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	result, err := api.RefreshIfNeeded(ctx, account, executor, time.Hour)

	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Nil(t, result)
	require.Zero(t, repo.updateCredentialsCalls, "late credentials must not cross the unified API persistence boundary")
}

func TestRefreshIfNeeded_NilCredentials(t *testing.T) {
	account := &Account{ID: 9, Platform: PlatformGemini, Type: AccountTypeOAuth, Status: StatusActive}
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
		return activeRefreshAPITestAccount(r.raceAccount), nil
	}
	if r.getByIDErr != nil {
		return nil, r.getByIDErr
	}
	return activeRefreshAPITestAccount(r.account), nil
}

// ========== Race recovery tests ==========

func TestRefreshIfNeeded_InvalidGrantRaceRecovered(t *testing.T) {
	// Account with old refresh token
	account := &Account{
		ID:          10,
		Platform:    PlatformAnthropic,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Credentials: map[string]any{"refresh_token": "old-rt", "access_token": "old-at"},
	}
	// After race, DB has new refresh token from another worker
	racedAccount := &Account{
		ID:          10,
		Platform:    PlatformAnthropic,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
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
		Status:      StatusActive,
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
	require.NotNil(t, result)
	require.NotNil(t, result.Account)
	require.Equal(t, "revoked-rt", result.Account.GetCredential("refresh_token"))
	require.Contains(t, err.Error(), "invalid_grant")
}

func TestRefreshIfNeeded_InvalidGrantDBRereadFailsOnRecovery(t *testing.T) {
	account := &Account{
		ID:          12,
		Platform:    PlatformAnthropic,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
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
	require.NotNil(t, result)
	require.NotNil(t, result.Account)
	require.Equal(t, "old-rt", result.Account.GetCredential("refresh_token"))
}

func TestRefreshIfNeeded_LocalMutexSerializesConcurrent(t *testing.T) {
	// Test that two goroutines for the same account are serialized by the local mutex.
	// The first goroutine refreshes successfully; the second sees NeedsRefresh=false.
	refreshed := &Account{
		ID:          20,
		Platform:    PlatformAnthropic,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Credentials: map[string]any{"refresh_token": "new-rt", "access_token": "new-at"},
	}
	callCount := 0
	repo := &refreshAPIAccountRepo{account: &Account{
		ID:          20,
		Platform:    PlatformAnthropic,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
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

func TestRefreshIfNeeded_LocalLockWaitHonorsContextCancellation(t *testing.T) {
	account := &Account{ID: 21, Platform: PlatformGrok, Type: AccountTypeOAuth, Status: StatusActive}
	repo := &refreshAPIAccountRepo{account: account}
	refreshStarted := make(chan struct{})
	releaseRefresh := make(chan struct{})
	var once sync.Once
	executor := &dynamicRefreshExecutor{
		canRefresh:       true,
		cacheKey:         "test:context-lock:grok",
		needsRefreshFunc: func() bool { return true },
		refreshFunc: func(context.Context, *Account) (map[string]any, error) {
			once.Do(func() { close(refreshStarted) })
			<-releaseRefresh
			return map[string]any{"access_token": "new-at"}, nil
		},
	}
	api := NewOAuthRefreshAPI(repo, nil)
	firstDone := make(chan error, 1)
	go func() {
		_, err := api.RefreshIfNeeded(context.Background(), account, executor, 3*time.Minute)
		firstDone <- err
	}()
	<-refreshStarted

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	startedAt := time.Now()
	result, err := api.RefreshIfNeeded(ctx, account, executor, 3*time.Minute)

	require.Nil(t, result)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Less(t, time.Since(startedAt), 500*time.Millisecond)
	close(releaseRefresh)
	require.NoError(t, <-firstDone)
}

func TestRefreshIfNeeded_ReleasesDistributedLockWithCleanupContext(t *testing.T) {
	account := &Account{
		ID:       22,
		Platform: PlatformGrok,
		Type:     AccountTypeOAuth,
		Status:   StatusActive,
		Credentials: map[string]any{
			"access_token":  "old-access",
			"refresh_token": "old-refresh",
		},
	}
	repo := &refreshAPIAccountRepo{account: account}
	cache := &refreshAPICacheStub{lockResult: true}
	ctx, cancel := context.WithCancel(context.Background())
	executor := &dynamicRefreshExecutor{
		canRefresh:       true,
		cacheKey:         "test:cleanup:grok",
		needsRefreshFunc: func() bool { return true },
		refreshFunc: func(context.Context, *Account) (map[string]any, error) {
			cancel()
			return map[string]any{"access_token": "new-at"}, nil
		},
	}
	api := NewOAuthRefreshAPI(repo, cache)

	result, err := api.RefreshIfNeeded(ctx, account, executor, 3*time.Minute)

	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, result)
	require.Zero(t, repo.updateCalls)
	require.Equal(t, "old-access", account.GetGrokAccessToken())
	require.Zero(t, account.GetCredentialAsInt64("_token_version"))
	require.Equal(t, 1, cache.releaseCalls)
	require.NoError(t, cache.releaseCtxErr)
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
