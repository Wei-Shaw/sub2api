//go:build unit

package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type grokCredentialPersistingRepo struct {
	*tokenRefreshAccountRepo
}

func (r *grokCredentialPersistingRepo) SetError(ctx context.Context, id int64, message string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := r.tokenRefreshAccountRepo.SetError(ctx, id, message); err != nil {
		return err
	}
	if account := r.accountsByID[id]; account != nil {
		account.Status = StatusError
		account.Schedulable = false
		account.ErrorMessage = message
	}
	return nil
}

type grokCredentialProxyRepoStub struct {
	ProxyRepository
	proxy *Proxy
	err   error
}

func (r *grokCredentialProxyRepoStub) GetByID(context.Context, int64) (*Proxy, error) {
	return r.proxy, r.err
}

type grokCredentialBlockingRepo struct {
	*tokenRefreshAccountRepo
	setErrorStarted chan struct{}
	setTempStarted  chan struct{}
	onceError       sync.Once
	onceTemp        sync.Once
}

type grokCredentialCommitThenCancelRepo struct {
	*tokenRefreshAccountRepo
	returnErr error
}

type grokCredentialUncommittedDeadlineRepo struct {
	*tokenRefreshAccountRepo
}

func (r *grokCredentialUncommittedDeadlineRepo) SetGrokCredentialErrorIfMatch(
	context.Context,
	int64,
	GrokCredentialMutationSnapshot,
	string,
) (bool, error) {
	return false, context.DeadlineExceeded
}

func (r *grokCredentialUncommittedDeadlineRepo) SetGrokCredentialTempUnschedulableIfMatch(
	context.Context,
	int64,
	GrokCredentialMutationSnapshot,
	time.Time,
	string,
) (bool, error) {
	return false, context.DeadlineExceeded
}

func (r *grokCredentialCommitThenCancelRepo) SetGrokCredentialErrorIfMatch(
	ctx context.Context,
	id int64,
	_ GrokCredentialMutationSnapshot,
	reason string,
) (bool, error) {
	account := r.accountsByID[id]
	account.Status = StatusError
	account.Schedulable = false
	account.ErrorMessage = reason
	if r.returnErr != nil {
		return false, r.returnErr
	}
	<-ctx.Done()
	return false, ctx.Err()
}

func (r *grokCredentialCommitThenCancelRepo) SetGrokCredentialTempUnschedulableIfMatch(
	ctx context.Context,
	id int64,
	_ GrokCredentialMutationSnapshot,
	until time.Time,
	reason string,
) (bool, error) {
	account := r.accountsByID[id]
	account.TempUnschedulableUntil = &until
	account.TempUnschedulableReason = reason
	if r.returnErr != nil {
		return false, r.returnErr
	}
	<-ctx.Done()
	return false, ctx.Err()
}

func (r *grokCredentialBlockingRepo) SetError(ctx context.Context, _ int64, _ string) error {
	r.onceError.Do(func() { close(r.setErrorStarted) })
	<-ctx.Done()
	return ctx.Err()
}

func (r *grokCredentialBlockingRepo) SetTempUnschedulable(ctx context.Context, _ int64, _ time.Time, _ string) error {
	r.onceTemp.Do(func() { close(r.setTempStarted) })
	<-ctx.Done()
	return ctx.Err()
}

func (r *grokCredentialBlockingRepo) SetGrokCredentialErrorIfMatch(
	ctx context.Context,
	_ int64,
	_ GrokCredentialMutationSnapshot,
	_ string,
) (bool, error) {
	r.onceError.Do(func() { close(r.setErrorStarted) })
	<-ctx.Done()
	return false, ctx.Err()
}

func (r *grokCredentialBlockingRepo) SetGrokCredentialTempUnschedulableIfMatch(
	ctx context.Context,
	_ int64,
	_ GrokCredentialMutationSnapshot,
	_ time.Time,
	_ string,
) (bool, error) {
	r.onceTemp.Do(func() { close(r.setTempStarted) })
	<-ctx.Done()
	return false, ctx.Err()
}

type grokCredentialBlockingCache struct {
	GrokTokenCache
	deleteStarted chan struct{}
	releaseDelete chan struct{}
	once          sync.Once
	mu            sync.Mutex
	deleted       bool
}

type grokCredentialSequencedRepo struct {
	*tokenRefreshAccountRepo
	mu      sync.Mutex
	latest  *Account
	getCall int
}

type grokCredentialRereadFailureRepo struct {
	*tokenRefreshAccountRepo
	account *Account
	err     error
}

type grokCredentialCountingRefresher struct {
	refreshCalls int
}

func (r *grokCredentialCountingRefresher) CacheKey(account *Account) string {
	return GrokTokenCacheKey(account)
}

func (r *grokCredentialCountingRefresher) CanRefresh(*Account) bool { return true }

func (r *grokCredentialCountingRefresher) NeedsRefresh(*Account, time.Duration) bool { return true }

func (r *grokCredentialCountingRefresher) Refresh(context.Context, *Account) (map[string]any, error) {
	r.refreshCalls++
	return map[string]any{"access_token": "must-not-be-used"}, nil
}

func (r *grokCredentialRereadFailureRepo) GetByID(context.Context, int64) (*Account, error) {
	return r.account, r.err
}

func (r *grokCredentialSequencedRepo) GetByID(ctx context.Context, id int64) (*Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.getCall++
	if r.getCall > 1 && r.latest != nil {
		return r.latest, nil
	}
	return r.tokenRefreshAccountRepo.GetByID(ctx, id)
}

func (c *grokCredentialBlockingCache) DeleteAccessToken(ctx context.Context, _ string) error {
	c.once.Do(func() { close(c.deleteStarted) })
	select {
	case <-c.releaseDelete:
		c.mu.Lock()
		c.deleted = true
		c.mu.Unlock()
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *grokCredentialBlockingCache) wasDeleted() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.deleted
}

func TestUpstreamFailoverErrorNextAccountActionPreservesLegacyRetry(t *testing.T) {
	t.Parallel()

	require.True(t, (&UpstreamFailoverError{}).ShouldRetryNextAccount())
	require.True(t, (&UpstreamFailoverError{NextAccountAction: NextAccountRetry}).ShouldRetryNextAccount())
	require.False(t, (&UpstreamFailoverError{NextAccountAction: NextAccountStop}).ShouldRetryNextAccount())
}

func TestGetRequestCredentialMapsPermanentGrokOAuthFailureAndRedactsSecrets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	account := expiredGrokOAuthAccountForCredentialTest(701)
	repo := &tokenRefreshAccountRepo{}
	repo.accountsByID = map[int64]*Account{account.ID: account}
	cache := &grokTokenCacheForProviderTest{lockResult: true}
	provider := NewGrokTokenProvider(repo, cache)
	provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), &tokenRefresherStub{
		err: infraerrors.New(http.StatusBadGateway, "GROK_OAUTH_TOKEN_REFRESH_FAILED", "invalid_grant access_token=leaked-access refresh_token=leaked-refresh"),
	})
	svc := &OpenAIGatewayService{accountRepo: repo, grokTokenProvider: provider}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	token, kind, err := svc.getRequestCredential(context.Background(), c, account)
	require.Error(t, err)
	require.Empty(t, token)
	require.Empty(t, kind)

	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, GatewayFailureStageAccountAuth, failoverErr.Stage)
	require.Equal(t, GatewayFailureScopeAccount, failoverErr.Scope)
	require.Equal(t, GrokCredentialReasonRevoked, failoverErr.Reason)
	require.True(t, failoverErr.ShouldRetryNextAccount())
	require.Equal(t, 0, failoverErr.StatusCode)
	require.Equal(t, http.StatusServiceUnavailable, failoverErr.ClientStatusCode)
	require.NotContains(t, err.Error(), "leaked-access")
	require.NotContains(t, err.Error(), "leaked-refresh")

	require.Equal(t, 1, repo.setErrorCalls)
	require.Zero(t, repo.setTempUnschedCalls)
	require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))
	require.Equal(t, []string{GrokTokenCacheKey(account)}, cache.deletedKeys)
	require.NotContains(t, repo.lastErrorMessage, "leaked-access")
	require.NotContains(t, repo.lastErrorMessage, "leaked-refresh")

	rawEvents, ok := c.Get(OpsUpstreamErrorsKey)
	require.True(t, ok)
	events, ok := rawEvents.([]*OpsUpstreamErrorEvent)
	require.True(t, ok)
	require.Len(t, events, 1)
	require.Equal(t, string(GatewayFailureStageAccountAuth), events[0].Stage)
	require.Equal(t, string(GatewayFailureScopeAccount), events[0].Scope)
	require.Equal(t, string(GrokCredentialReasonRevoked), events[0].Reason)
	require.Zero(t, events[0].UpstreamStatusCode)
	require.NotContains(t, events[0].Message, "leaked-access")
	require.NotContains(t, events[0].Message, "leaked-refresh")
}

func TestGetRequestCredentialPermanentMappingsPersistAndInvalidate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name        string
		prepare     func(*Account)
		refreshErr  error
		wantReason  GatewayFailureReason
		cachedToken string
	}{
		{
			name: "missing refresh credential",
			prepare: func(account *Account) {
				delete(account.Credentials, "refresh_token")
			},
			wantReason: GrokCredentialReasonMissing,
		},
		{
			name: "missing access credential",
			prepare: func(account *Account) {
				delete(account.Credentials, "access_token")
				account.Credentials["expires_at"] = time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339)
			},
			wantReason:  GrokCredentialReasonMissing,
			cachedToken: "stale-cached-access",
		},
		{
			name:       "explicit entitlement action required",
			prepare:    func(*Account) {},
			refreshErr: infraerrors.New(http.StatusForbidden, "GROK_OAUTH_ENTITLEMENT_DENIED", "access_denied"),
			wantReason: GrokCredentialReasonEntitlement,
		},
	}

	for index, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := expiredGrokOAuthAccountForCredentialTest(int64(720 + index))
			account.Status = StatusActive
			account.Schedulable = true
			tt.prepare(account)
			baseRepo := &tokenRefreshAccountRepo{}
			baseRepo.accountsByID = map[int64]*Account{account.ID: account}
			repo := &grokCredentialPersistingRepo{tokenRefreshAccountRepo: baseRepo}
			cache := &grokTokenCacheForProviderTest{lockResult: true, token: tt.cachedToken}
			provider := NewGrokTokenProvider(repo, cache)
			provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), &tokenRefresherStub{err: tt.refreshErr})
			svc := &OpenAIGatewayService{accountRepo: repo, grokTokenProvider: provider}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())

			_, _, err := svc.getRequestCredential(context.Background(), c, account)
			var failoverErr *UpstreamFailoverError
			require.ErrorAs(t, err, &failoverErr)
			require.Equal(t, tt.wantReason, failoverErr.Reason)
			require.Equal(t, GatewayFailureScopeAccount, failoverErr.Scope)
			require.Equal(t, 1, baseRepo.setErrorCalls)
			require.Equal(t, StatusError, account.Status)
			require.False(t, account.Schedulable)
			require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))
			require.Equal(t, []string{GrokTokenCacheKey(account)}, cache.deletedKeys)
		})
	}
}

func TestGetRequestCredentialMissingAccessNeverRefreshesAndPermanentlyFailsOver(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name      string
		expiresAt *time.Time
	}{
		{name: "expiry missing"},
		{name: "expired", expiresAt: func() *time.Time { value := time.Now().Add(-time.Minute); return &value }()},
		{name: "near expiry", expiresAt: func() *time.Time { value := time.Now().Add(30 * time.Minute); return &value }()},
	}

	for index, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := expiredGrokOAuthAccountForCredentialTest(int64(760 + index))
			account.Schedulable = true
			delete(account.Credentials, "access_token")
			if tt.expiresAt == nil {
				delete(account.Credentials, "expires_at")
			} else {
				account.Credentials["expires_at"] = tt.expiresAt.UTC().Format(time.RFC3339)
			}
			baseRepo := &tokenRefreshAccountRepo{}
			baseRepo.accountsByID = map[int64]*Account{account.ID: account}
			repo := &grokCredentialPersistingRepo{tokenRefreshAccountRepo: baseRepo}
			cache := &grokTokenCacheForProviderTest{lockResult: true, token: "stale-cache-must-not-win"}
			refresher := &grokCredentialCountingRefresher{}
			provider := NewGrokTokenProvider(repo, cache)
			provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), refresher)
			svc := &OpenAIGatewayService{accountRepo: repo, grokTokenProvider: provider}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())

			token, kind, err := svc.getRequestCredential(context.Background(), c, account)
			var failoverErr *UpstreamFailoverError
			require.ErrorAs(t, err, &failoverErr)
			require.Empty(t, token)
			require.Empty(t, kind)
			require.Equal(t, GrokCredentialReasonMissing, failoverErr.Reason)
			require.Equal(t, NextAccountRetry, failoverErr.NextAccountAction)
			require.Zero(t, refresher.refreshCalls, "structurally missing access credentials must not reach the token endpoint")
			require.Equal(t, 1, baseRepo.setErrorCalls)
			require.Equal(t, StatusError, account.Status)
			require.False(t, account.Schedulable)
			require.Equal(t, []string{GrokTokenCacheKey(account)}, cache.deletedKeys)
			require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))
		})
	}
}

func TestGetRequestCredentialWarmCachedAccessWithMissingRefreshPermanentlyFailsOver(t *testing.T) {
	account := expiredGrokOAuthAccountForCredentialTest(764)
	account.Credentials["access_token"] = "valid-access"
	account.Credentials["expires_at"] = time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339)
	delete(account.Credentials, "refresh_token")
	baseRepo := &tokenRefreshAccountRepo{}
	baseRepo.accountsByID = map[int64]*Account{account.ID: account}
	repo := &grokCredentialPersistingRepo{tokenRefreshAccountRepo: baseRepo}
	cache := &grokTokenCacheForProviderTest{lockResult: true, token: "valid-access"}
	refresher := &grokCredentialCountingRefresher{}
	provider := NewGrokTokenProvider(repo, cache)
	provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), refresher)
	svc := &OpenAIGatewayService{accountRepo: repo, grokTokenProvider: provider}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	_, _, err := svc.getRequestCredential(context.Background(), c, account)

	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, GrokCredentialReasonMissing, failoverErr.Reason)
	require.Equal(t, NextAccountRetry, failoverErr.NextAccountAction)
	require.Zero(t, refresher.refreshCalls)
	require.Equal(t, 1, baseRepo.setErrorCalls)
	require.Equal(t, []string{GrokTokenCacheKey(account)}, cache.deletedKeys)
	require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))
}

func TestGetRequestCredentialMapsTransientAndProviderFailuresSeparately(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("account transient temporarily unschedules", func(t *testing.T) {
		account := expiredGrokOAuthAccountForCredentialTest(702)
		repo := &tokenRefreshAccountRepo{}
		repo.accountsByID = map[int64]*Account{account.ID: account}
		cache := &grokTokenCacheForProviderTest{lockResult: true}
		provider := NewGrokTokenProvider(repo, cache)
		provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), &tokenRefresherStub{err: errors.New("temporary refresh transport failure")})
		svc := &OpenAIGatewayService{accountRepo: repo, grokTokenProvider: provider}
		c, _ := gin.CreateTestContext(httptest.NewRecorder())

		_, _, err := svc.getRequestCredential(context.Background(), c, account)
		var failoverErr *UpstreamFailoverError
		require.ErrorAs(t, err, &failoverErr)
		require.Equal(t, GatewayFailureScopeAccount, failoverErr.Scope)
		require.Equal(t, GrokCredentialReasonRefreshTransient, failoverErr.Reason)
		require.True(t, failoverErr.ShouldRetryNextAccount())
		require.Zero(t, repo.setErrorCalls)
		require.Equal(t, 1, repo.setTempUnschedCalls)
		require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))
	})

	t.Run("shared provider configuration stops without mutation", func(t *testing.T) {
		account := expiredGrokOAuthAccountForCredentialTest(703)
		repo := &tokenRefreshAccountRepo{}
		repo.accountsByID = map[int64]*Account{account.ID: account}
		provider := NewGrokTokenProvider(repo, nil)
		svc := &OpenAIGatewayService{accountRepo: repo, grokTokenProvider: provider}
		c, _ := gin.CreateTestContext(httptest.NewRecorder())

		_, _, err := svc.getRequestCredential(context.Background(), c, account)
		var failoverErr *UpstreamFailoverError
		require.ErrorAs(t, err, &failoverErr)
		require.Equal(t, GatewayFailureScopeProvider, failoverErr.Scope)
		require.Equal(t, NextAccountStop, failoverErr.NextAccountAction)
		require.Zero(t, repo.setErrorCalls)
		require.Zero(t, repo.setTempUnschedCalls)
		require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
	})

	t.Run("account reread failures preserve shared versus missing-row scope", func(t *testing.T) {
		for _, tt := range []struct {
			name    string
			account *Account
			err     error
		}{
			{name: "repository error", err: errors.New("database temporarily unavailable")},
			{name: "missing row"},
		} {
			t.Run(tt.name, func(t *testing.T) {
				account := expiredGrokOAuthAccountForCredentialTest(712)
				baseRepo := &tokenRefreshAccountRepo{}
				baseRepo.accountsByID = map[int64]*Account{account.ID: account}
				repo := &grokCredentialRereadFailureRepo{tokenRefreshAccountRepo: baseRepo, account: tt.account, err: tt.err}
				cache := &grokTokenCacheForProviderTest{lockResult: true}
				provider := NewGrokTokenProvider(repo, cache)
				provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), NewGrokTokenRefresher(nil))
				svc := &OpenAIGatewayService{accountRepo: repo, grokTokenProvider: provider}
				c, _ := gin.CreateTestContext(httptest.NewRecorder())

				_, _, err := svc.getRequestCredential(context.Background(), c, account)
				var failoverErr *UpstreamFailoverError
				require.ErrorAs(t, err, &failoverErr)
				if tt.err != nil {
					require.Equal(t, GatewayFailureScopeProvider, failoverErr.Scope)
					require.Equal(t, GrokCredentialReasonProviderDown, failoverErr.Reason)
					require.Equal(t, NextAccountStop, failoverErr.NextAccountAction)
				} else {
					require.Equal(t, GatewayFailureScopeAccount, failoverErr.Scope)
					require.Equal(t, GrokCredentialReasonAccountChanged, failoverErr.Reason)
					require.Equal(t, NextAccountRetry, failoverErr.NextAccountAction)
				}
				require.Zero(t, baseRepo.setErrorCalls)
				require.Zero(t, baseRepo.setTempUnschedCalls)
				require.Empty(t, cache.deletedKeys)
				require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
			})
		}
	})

	t.Run("fresh account eligibility changes retry without mutating stale state", func(t *testing.T) {
		for _, tt := range []struct {
			name   string
			mutate func(*Account)
		}{
			{
				name: "account disabled",
				mutate: func(account *Account) {
					account.Status = StatusDisabled
				},
			},
			{
				name: "account converted",
				mutate: func(account *Account) {
					account.Type = AccountTypeUpstream
				},
			},
			{
				name: "account manually unschedulable",
				mutate: func(account *Account) {
					account.Schedulable = false
				},
			},
			{
				name: "account temporarily unschedulable",
				mutate: func(account *Account) {
					until := time.Now().Add(time.Minute)
					account.TempUnschedulableUntil = &until
				},
			},
		} {
			t.Run(tt.name, func(t *testing.T) {
				staleAccount := expiredGrokOAuthAccountForCredentialTest(713)
				freshAccount := *staleAccount
				freshAccount.Credentials = shallowCopyMap(staleAccount.Credentials)
				tt.mutate(&freshAccount)
				repo := &tokenRefreshAccountRepo{}
				repo.accountsByID = map[int64]*Account{staleAccount.ID: &freshAccount}
				cache := &grokTokenCacheForProviderTest{lockResult: true}
				provider := NewGrokTokenProvider(repo, cache)
				provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), NewGrokTokenRefresher(nil))
				svc := &OpenAIGatewayService{accountRepo: repo, grokTokenProvider: provider}
				c, _ := gin.CreateTestContext(httptest.NewRecorder())

				_, _, err := svc.getRequestCredential(context.Background(), c, staleAccount)
				var failoverErr *UpstreamFailoverError
				require.ErrorAs(t, err, &failoverErr)
				require.Equal(t, GatewayFailureScopeAccount, failoverErr.Scope)
				require.Equal(t, GrokCredentialReasonAccountChanged, failoverErr.Reason)
				require.Equal(t, NextAccountRetry, failoverErr.NextAccountAction)
				require.Zero(t, repo.setErrorCalls)
				require.Zero(t, repo.setTempUnschedCalls)
				require.Empty(t, cache.deletedKeys)
				require.False(t, svc.isOpenAIAccountRuntimeBlocked(staleAccount))
			})
		}
	})

	t.Run("fresh missing refresh credential permanently blocks the account", func(t *testing.T) {
		staleAccount := expiredGrokOAuthAccountForCredentialTest(714)
		staleAccount.Schedulable = true
		freshAccount := *staleAccount
		freshAccount.Credentials = shallowCopyMap(staleAccount.Credentials)
		delete(freshAccount.Credentials, "refresh_token")
		baseRepo := &tokenRefreshAccountRepo{}
		baseRepo.accountsByID = map[int64]*Account{staleAccount.ID: &freshAccount}
		repo := &grokCredentialPersistingRepo{tokenRefreshAccountRepo: baseRepo}
		cache := &grokTokenCacheForProviderTest{lockResult: true}
		provider := NewGrokTokenProvider(repo, cache)
		provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), NewGrokTokenRefresher(nil))
		svc := &OpenAIGatewayService{accountRepo: repo, grokTokenProvider: provider}
		c, _ := gin.CreateTestContext(httptest.NewRecorder())

		_, _, err := svc.getRequestCredential(context.Background(), c, staleAccount)
		var failoverErr *UpstreamFailoverError
		require.ErrorAs(t, err, &failoverErr)
		require.Equal(t, GatewayFailureScopeAccount, failoverErr.Scope)
		require.Equal(t, GrokCredentialReasonMissing, failoverErr.Reason)
		require.Equal(t, NextAccountRetry, failoverErr.NextAccountAction)
		require.Equal(t, 1, baseRepo.setErrorCalls)
		require.Zero(t, baseRepo.setTempUnschedCalls)
		require.Equal(t, StatusError, freshAccount.Status)
		require.False(t, freshAccount.Schedulable)
		require.Equal(t, []string{GrokTokenCacheKey(staleAccount)}, cache.deletedKeys)
		require.True(t, svc.isOpenAIAccountRuntimeBlocked(staleAccount))
	})

	t.Run("refresh added after locked structural failure wins conditional mutation", func(t *testing.T) {
		staleAccount := expiredGrokOAuthAccountForCredentialTest(717)
		freshAccount := *staleAccount
		freshAccount.Credentials = shallowCopyMap(staleAccount.Credentials)
		delete(freshAccount.Credentials, "refresh_token")
		repo := &tokenRefreshAccountRepo{}
		repo.accountsByID = map[int64]*Account{staleAccount.ID: &freshAccount}
		repo.beforeConditionalState = func() {
			repaired := freshAccount
			repaired.Credentials = shallowCopyMap(freshAccount.Credentials)
			repaired.Credentials["refresh_token"] = "repaired-refresh-token"
			repaired.Credentials["expires_at"] = time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
			repo.accountsByID[staleAccount.ID] = &repaired
		}
		cache := &grokTokenCacheForProviderTest{lockResult: true}
		provider := NewGrokTokenProvider(repo, cache)
		provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), NewGrokTokenRefresher(nil))
		svc := &OpenAIGatewayService{accountRepo: repo, grokTokenProvider: provider}
		c, _ := gin.CreateTestContext(httptest.NewRecorder())

		token, kind, err := svc.getRequestCredential(context.Background(), c, staleAccount)

		require.NoError(t, err)
		require.Equal(t, "expired-access-token", token)
		require.Equal(t, "oauth", kind)
		require.Zero(t, repo.setErrorCalls)
		require.Zero(t, repo.setTempUnschedCalls)
		require.Empty(t, cache.deletedKeys)
		require.False(t, svc.isOpenAIAccountRuntimeBlocked(staleAccount))
	})

	t.Run("expiry-only repair wins full credential fingerprint CAS", func(t *testing.T) {
		account := expiredGrokOAuthAccountForCredentialTest(718)
		repo := &tokenRefreshAccountRepo{}
		repo.accountsByID = map[int64]*Account{account.ID: account}
		repo.beforeConditionalState = func() {
			repaired := *account
			repaired.Credentials = shallowCopyMap(account.Credentials)
			repaired.Credentials["expires_at"] = time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
			repo.accountsByID[account.ID] = &repaired
		}
		cache := &grokTokenCacheForProviderTest{lockResult: true}
		provider := NewGrokTokenProvider(repo, cache)
		provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), &tokenRefresherStub{
			err: infraerrors.New(http.StatusBadGateway, "GROK_OAUTH_TOKEN_REFRESH_FAILED", "invalid_grant"),
		})
		svc := &OpenAIGatewayService{accountRepo: repo, grokTokenProvider: provider}
		c, _ := gin.CreateTestContext(httptest.NewRecorder())

		token, kind, err := svc.getRequestCredential(context.Background(), c, account)

		require.NoError(t, err)
		require.Equal(t, "expired-access-token", token)
		require.Equal(t, "oauth", kind)
		require.Zero(t, repo.setErrorCalls)
		require.Empty(t, cache.deletedKeys)
		require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
	})

	t.Run("generic token endpoint 403 stops as shared provider failure", func(t *testing.T) {
		account := expiredGrokOAuthAccountForCredentialTest(708)
		repo := &tokenRefreshAccountRepo{}
		repo.accountsByID = map[int64]*Account{account.ID: account}
		cache := &grokTokenCacheForProviderTest{lockResult: true}
		provider := NewGrokTokenProvider(repo, cache)
		provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), &tokenRefresherStub{
			err: infraerrors.New(http.StatusBadGateway, "GROK_OAUTH_TOKEN_REFRESH_FAILED", "token refresh failed: status 403, body: forbidden"),
		})
		svc := &OpenAIGatewayService{accountRepo: repo, grokTokenProvider: provider}
		c, _ := gin.CreateTestContext(httptest.NewRecorder())

		_, _, err := svc.getRequestCredential(context.Background(), c, account)
		var failoverErr *UpstreamFailoverError
		require.ErrorAs(t, err, &failoverErr)
		require.Equal(t, GatewayFailureScopeProvider, failoverErr.Scope)
		require.Equal(t, GrokCredentialReasonProviderDown, failoverErr.Reason)
		require.Equal(t, NextAccountStop, failoverErr.NextAccountAction)
		require.Zero(t, repo.setErrorCalls)
		require.Zero(t, repo.setTempUnschedCalls)
		require.Empty(t, cache.deletedKeys)
		require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
	})

	t.Run("account proxy generic 403 remains bounded account transient", func(t *testing.T) {
		account := expiredGrokOAuthAccountForCredentialTest(711)
		proxyID := int64(43)
		account.ProxyID = &proxyID
		account.Proxy = &Proxy{}
		repo := &tokenRefreshAccountRepo{}
		repo.accountsByID = map[int64]*Account{account.ID: account}
		cache := &grokTokenCacheForProviderTest{lockResult: true}
		provider := NewGrokTokenProvider(repo, cache)
		provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), &tokenRefresherStub{
			err: infraerrors.New(http.StatusBadGateway, "GROK_OAUTH_TOKEN_REFRESH_FAILED", "token refresh failed: status 403, body: forbidden"),
		})
		svc := &OpenAIGatewayService{accountRepo: repo, grokTokenProvider: provider}
		c, _ := gin.CreateTestContext(httptest.NewRecorder())

		_, _, err := svc.getRequestCredential(context.Background(), c, account)
		var failoverErr *UpstreamFailoverError
		require.ErrorAs(t, err, &failoverErr)
		require.Equal(t, GatewayFailureScopeAccount, failoverErr.Scope)
		require.Equal(t, GrokCredentialReasonRefreshTransient, failoverErr.Reason)
		require.Equal(t, NextAccountRetry, failoverErr.NextAccountAction)
		require.Zero(t, repo.setErrorCalls)
		require.Equal(t, 1, repo.setTempUnschedCalls)
		require.Empty(t, cache.deletedKeys)
		require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))
	})

	t.Run("proxy repository read failure stops without account mutation", func(t *testing.T) {
		account := expiredGrokOAuthAccountForCredentialTest(709)
		proxyID := int64(41)
		account.ProxyID = &proxyID
		account.Proxy = &Proxy{}
		repo := &tokenRefreshAccountRepo{}
		repo.accountsByID = map[int64]*Account{account.ID: account}
		cache := &grokTokenCacheForProviderTest{lockResult: true}
		oauthSvc := NewGrokOAuthService(&grokCredentialProxyRepoStub{err: errors.New("database temporarily unavailable")}, &grokOAuthClientStub{})
		defer oauthSvc.Stop()
		provider := NewGrokTokenProvider(repo, cache)
		provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), NewGrokTokenRefresher(oauthSvc))
		svc := &OpenAIGatewayService{accountRepo: repo, grokTokenProvider: provider}
		c, _ := gin.CreateTestContext(httptest.NewRecorder())

		_, _, err := svc.getRequestCredential(context.Background(), c, account)
		var failoverErr *UpstreamFailoverError
		require.ErrorAs(t, err, &failoverErr)
		require.Equal(t, GatewayFailureScopeProvider, failoverErr.Scope)
		require.Equal(t, GrokCredentialReasonProviderDown, failoverErr.Reason)
		require.Equal(t, NextAccountStop, failoverErr.NextAccountAction)
		require.Zero(t, repo.setErrorCalls)
		require.Zero(t, repo.setTempUnschedCalls)
		require.Empty(t, cache.deletedKeys)
		require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
	})

	t.Run("structurally missing configured proxy permanently blocks only that account", func(t *testing.T) {
		account := expiredGrokOAuthAccountForCredentialTest(710)
		account.Status = StatusActive
		account.Schedulable = true
		proxyID := int64(42)
		account.ProxyID = &proxyID
		baseRepo := &tokenRefreshAccountRepo{}
		baseRepo.accountsByID = map[int64]*Account{account.ID: account}
		repo := &grokCredentialPersistingRepo{tokenRefreshAccountRepo: baseRepo}
		cache := &grokTokenCacheForProviderTest{lockResult: true}
		oauthSvc := NewGrokOAuthService(&grokCredentialProxyRepoStub{err: ErrProxyNotFound}, &grokOAuthClientStub{})
		defer oauthSvc.Stop()
		provider := NewGrokTokenProvider(repo, cache)
		provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), NewGrokTokenRefresher(oauthSvc))
		svc := &OpenAIGatewayService{accountRepo: repo, grokTokenProvider: provider}
		c, _ := gin.CreateTestContext(httptest.NewRecorder())

		_, _, err := svc.getRequestCredential(context.Background(), c, account)
		var failoverErr *UpstreamFailoverError
		require.ErrorAs(t, err, &failoverErr)
		require.Equal(t, GatewayFailureScopeAccount, failoverErr.Scope)
		require.Equal(t, GrokCredentialReasonProxyInvalid, failoverErr.Reason)
		require.Equal(t, NextAccountRetry, failoverErr.NextAccountAction)
		require.Equal(t, 1, baseRepo.setErrorCalls)
		require.Equal(t, StatusError, account.Status)
		require.False(t, account.Schedulable)
		require.Equal(t, []string{GrokTokenCacheKey(account)}, cache.deletedKeys)
		require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))
	})
}

func TestGetRequestCredentialRuntimeBlockWinsBeforeWarmTokenCache(t *testing.T) {
	account := expiredGrokOAuthAccountForCredentialTest(716)
	account.Credentials["access_token"] = "valid-access"
	account.Credentials["expires_at"] = time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339)
	repo := &tokenRefreshAccountRepo{}
	repo.accountsByID = map[int64]*Account{account.ID: account}
	cache := &grokTokenCacheForProviderTest{token: "valid-access"}
	provider := NewGrokTokenProvider(repo, cache)
	svc := &OpenAIGatewayService{accountRepo: repo, grokTokenProvider: provider}
	svc.BlockAccountScheduling(account, time.Now().Add(time.Minute), "independent")
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	_, _, err := svc.getRequestCredential(context.Background(), c, account)

	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, GrokCredentialReasonAccountChanged, failoverErr.Reason)
	require.Equal(t, NextAccountRetry, failoverErr.NextAccountAction)
	require.Zero(t, cache.getCalls)
	require.Zero(t, repo.setErrorCalls)
	require.Zero(t, repo.setTempUnschedCalls)
}

func TestGetRequestCredentialWarmCachedAccessWithMissingConfiguredProxyPermanentlyFailsOver(t *testing.T) {
	account := expiredGrokOAuthAccountForCredentialTest(715)
	account.Credentials["access_token"] = "valid-access"
	account.Credentials["expires_at"] = time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339)
	proxyID := int64(44)
	account.ProxyID = &proxyID
	account.Proxy = nil
	baseRepo := &tokenRefreshAccountRepo{}
	baseRepo.accountsByID = map[int64]*Account{account.ID: account}
	repo := &grokCredentialPersistingRepo{tokenRefreshAccountRepo: baseRepo}
	cache := &grokTokenCacheForProviderTest{lockResult: true, token: "valid-access"}
	refresher := &grokCredentialCountingRefresher{}
	provider := NewGrokTokenProvider(repo, cache)
	provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), refresher)
	svc := &OpenAIGatewayService{accountRepo: repo, grokTokenProvider: provider}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	_, _, err := svc.getRequestCredential(context.Background(), c, account)

	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, GrokCredentialReasonProxyInvalid, failoverErr.Reason)
	require.Equal(t, NextAccountRetry, failoverErr.NextAccountAction)
	require.Zero(t, refresher.refreshCalls)
	require.Equal(t, 1, baseRepo.setErrorCalls)
	require.Equal(t, []string{GrokTokenCacheKey(account)}, cache.deletedKeys)
	require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))
}

func TestGetRequestCredentialCancellationAndBudgetDoNotMutateAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	account := expiredGrokOAuthAccountForCredentialTest(704)
	repo := &tokenRefreshAccountRepo{}
	repo.accountsByID = map[int64]*Account{account.ID: account}
	provider := NewGrokTokenProvider(repo, nil)
	svc := &OpenAIGatewayService{accountRepo: repo, grokTokenProvider: provider}

	t.Run("parent cancellation is returned directly", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		c, _ := gin.CreateTestContext(httptest.NewRecorder())

		_, _, err := svc.getRequestCredential(ctx, c, account)
		require.ErrorIs(t, err, context.Canceled)
		var failoverErr *UpstreamFailoverError
		require.False(t, errors.As(err, &failoverErr))
	})

	t.Run("request credential budget stops safely", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Set(grokCredentialFailoverDeadlineKey, time.Now().Add(-time.Second))

		_, _, err := svc.getRequestCredential(context.Background(), c, account)
		var failoverErr *UpstreamFailoverError
		require.ErrorAs(t, err, &failoverErr)
		require.Equal(t, GatewayFailureScopeRequest, failoverErr.Scope)
		require.Equal(t, GrokCredentialReasonFailoverTimeout, failoverErr.Reason)
		require.False(t, failoverErr.ShouldRetryNextAccount())
	})

	require.Zero(t, repo.setErrorCalls)
	require.Zero(t, repo.setTempUnschedCalls)
	require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
}

func TestGetRequestCredentialStateMutationFailureStopsAndKeepsRuntimeBlock(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name            string
		refreshErr      error
		configure       func(*tokenRefreshAccountRepo, *grokTokenCacheForProviderTest)
		wantSetError    int
		wantSetTemp     int
		wantCacheDelete int
	}{
		{
			name:       "permanent state persistence",
			refreshErr: infraerrors.New(http.StatusBadGateway, "GROK_OAUTH_TOKEN_REFRESH_FAILED", "invalid_grant"),
			configure: func(repo *tokenRefreshAccountRepo, _ *grokTokenCacheForProviderTest) {
				repo.setErrorErr = errors.New("database write failed")
			},
			wantSetError: 1,
		},
		{
			name:       "transient state persistence",
			refreshErr: errors.New("temporary refresh transport failure"),
			configure: func(repo *tokenRefreshAccountRepo, _ *grokTokenCacheForProviderTest) {
				repo.setTempUnschedErr = errors.New("database write failed")
			},
			wantSetTemp: 1,
		},
		{
			name:       "permanent token cache invalidation",
			refreshErr: infraerrors.New(http.StatusBadGateway, "GROK_OAUTH_TOKEN_REFRESH_FAILED", "invalid_grant"),
			configure: func(_ *tokenRefreshAccountRepo, cache *grokTokenCacheForProviderTest) {
				cache.deleteErr = errors.New("cache delete failed")
			},
			wantSetError:    1,
			wantCacheDelete: 1,
		},
	}

	for index, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := expiredGrokOAuthAccountForCredentialTest(int64(740 + index))
			repo := &tokenRefreshAccountRepo{}
			repo.accountsByID = map[int64]*Account{account.ID: account}
			cache := &grokTokenCacheForProviderTest{lockResult: true}
			tt.configure(repo, cache)
			provider := NewGrokTokenProvider(repo, cache)
			provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), &tokenRefresherStub{err: tt.refreshErr})
			svc := &OpenAIGatewayService{accountRepo: repo, grokTokenProvider: provider}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())

			_, _, err := svc.getRequestCredential(context.Background(), c, account)
			var failoverErr *UpstreamFailoverError
			require.ErrorAs(t, err, &failoverErr)
			require.Equal(t, GatewayFailureScopeProvider, failoverErr.Scope)
			require.Equal(t, GrokCredentialReasonStateUpdate, failoverErr.Reason)
			require.Equal(t, NextAccountStop, failoverErr.NextAccountAction)
			require.Equal(t, tt.wantSetError, repo.setErrorCalls)
			require.Equal(t, tt.wantSetTemp, repo.setTempUnschedCalls)
			require.Len(t, cache.deletedKeys, tt.wantCacheDelete)
			require.True(t, svc.isOpenAIAccountRuntimeBlocked(account), "failed mutation must retain the immediate local block")
		})
	}
}

func TestGrokCredentialMutationBoundariesHonorParentCancellation(t *testing.T) {
	t.Run("blocked SetError cancellation prevents cache and runtime mutation", func(t *testing.T) {
		account := expiredGrokOAuthAccountForCredentialTest(730)
		baseRepo := &tokenRefreshAccountRepo{}
		baseRepo.accountsByID = map[int64]*Account{account.ID: account}
		repo := &grokCredentialBlockingRepo{
			tokenRefreshAccountRepo: baseRepo,
			setErrorStarted:         make(chan struct{}),
			setTempStarted:          make(chan struct{}),
		}
		cache := &grokTokenCacheForProviderTest{}
		svc := &OpenAIGatewayService{accountRepo: repo, grokTokenProvider: NewGrokTokenProvider(repo, cache)}
		ctx, cancel := context.WithCancel(context.Background())
		result := make(chan error, 1)
		go func() {
			_, err := svc.applyGrokCredentialAccountFailure(ctx, account, grokCredentialFailureClass{
				reason: GrokCredentialReasonRevoked, permanent: true,
			})
			result <- err
		}()
		<-repo.setErrorStarted
		require.True(t, svc.isOpenAIAccountRuntimeBlocked(account), "runtime block must precede persistent SetError")
		cancel()

		require.ErrorIs(t, <-result, context.Canceled)
		require.Zero(t, baseRepo.setErrorCalls)
		require.Empty(t, cache.deletedKeys)
		require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
	})

	t.Run("blocked temporary unschedule cancellation prevents runtime mutation", func(t *testing.T) {
		account := expiredGrokOAuthAccountForCredentialTest(731)
		baseRepo := &tokenRefreshAccountRepo{}
		baseRepo.accountsByID = map[int64]*Account{account.ID: account}
		repo := &grokCredentialBlockingRepo{
			tokenRefreshAccountRepo: baseRepo,
			setErrorStarted:         make(chan struct{}),
			setTempStarted:          make(chan struct{}),
		}
		svc := &OpenAIGatewayService{accountRepo: repo}
		ctx, cancel := context.WithCancel(context.Background())
		result := make(chan error, 1)
		go func() {
			_, err := svc.applyGrokCredentialAccountFailure(ctx, account, grokCredentialFailureClass{
				reason: GrokCredentialReasonRefreshTransient, transient: true,
			})
			result <- err
		}()
		<-repo.setTempStarted
		require.True(t, svc.isOpenAIAccountRuntimeBlocked(account), "runtime block must precede temporary unscheduling")
		cancel()

		require.ErrorIs(t, <-result, context.Canceled)
		require.Zero(t, baseRepo.setTempUnschedCalls)
		require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
	})

	t.Run("post-commit cancellation finishes cache cleanup and retains quarantine", func(t *testing.T) {
		account := expiredGrokOAuthAccountForCredentialTest(732)
		repo := &tokenRefreshAccountRepo{}
		repo.accountsByID = map[int64]*Account{account.ID: account}
		cache := &grokCredentialBlockingCache{deleteStarted: make(chan struct{}), releaseDelete: make(chan struct{})}
		svc := &OpenAIGatewayService{accountRepo: repo, grokTokenProvider: NewGrokTokenProvider(repo, cache)}
		ctx, cancel := context.WithCancel(context.Background())
		result := make(chan error, 1)
		go func() {
			_, err := svc.applyGrokCredentialAccountFailure(ctx, account, grokCredentialFailureClass{
				reason: GrokCredentialReasonRevoked, permanent: true,
			})
			result <- err
		}()
		<-cache.deleteStarted
		require.True(t, svc.isOpenAIAccountRuntimeBlocked(account), "runtime block must precede cache invalidation")
		cancel()
		close(cache.releaseDelete)

		require.ErrorIs(t, <-result, context.Canceled)
		require.Equal(t, 1, repo.setErrorCalls)
		require.True(t, cache.wasDeleted())
		require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))
	})
}

func TestGrokCredentialMutationLockWaitHonorsCredentialBudget(t *testing.T) {
	account := expiredGrokOAuthAccountForCredentialTest(735)
	repo := &tokenRefreshAccountRepo{}
	repo.accountsByID = map[int64]*Account{account.ID: account}
	svc := &OpenAIGatewayService{accountRepo: repo, grokTokenProvider: NewGrokTokenProvider(repo, &grokTokenCacheForProviderTest{})}
	mutationLock := svc.grokCredentialMutationLock(account.ID)
	require.NoError(t, mutationLock.Lock(context.Background()))
	defer mutationLock.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	startedAt := time.Now()
	token, err := svc.applyGrokCredentialAccountFailure(ctx, account, grokCredentialFailureClass{
		reason: GrokCredentialReasonRevoked, permanent: true,
	})

	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Empty(t, token)
	require.Less(t, time.Since(startedAt), 500*time.Millisecond)
	require.Zero(t, repo.setErrorCalls)
	require.Zero(t, repo.setTempUnschedCalls)
	require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
}

func TestGetRequestCredentialBudgetBoundsBlockedConditionalMutation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	account := expiredGrokOAuthAccountForCredentialTest(736)
	baseRepo := &tokenRefreshAccountRepo{}
	baseRepo.accountsByID = map[int64]*Account{account.ID: account}
	repo := &grokCredentialBlockingRepo{
		tokenRefreshAccountRepo: baseRepo,
		setErrorStarted:         make(chan struct{}),
		setTempStarted:          make(chan struct{}),
	}
	cache := &grokTokenCacheForProviderTest{lockResult: true}
	provider := NewGrokTokenProvider(repo, cache)
	provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), &tokenRefresherStub{
		err: infraerrors.New(http.StatusBadGateway, "GROK_OAUTH_TOKEN_REFRESH_FAILED", "invalid_grant"),
	})
	svc := &OpenAIGatewayService{accountRepo: repo, grokTokenProvider: provider}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set(grokCredentialFailoverDeadlineKey, time.Now().Add(40*time.Millisecond))

	startedAt := time.Now()
	token, kind, err := svc.getRequestCredential(context.Background(), c, account)

	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, GatewayFailureScopeRequest, failoverErr.Scope)
	require.Equal(t, GrokCredentialReasonFailoverTimeout, failoverErr.Reason)
	require.Equal(t, NextAccountStop, failoverErr.NextAccountAction)
	require.Empty(t, token)
	require.Empty(t, kind)
	require.Less(t, time.Since(startedAt), 500*time.Millisecond)
	require.Zero(t, baseRepo.setErrorCalls)
	require.Zero(t, baseRepo.setTempUnschedCalls)
	require.Empty(t, cache.deletedKeys)
	require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
}

func TestGetRequestCredentialLockHeldTimeoutDoesNotQuarantineAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name       string
		buildRepo  func(*Account) AccountRepository
		wantScope  GatewayFailureScope
		wantReason GatewayFailureReason
		wantAction NextAccountAction
	}{
		{
			name: "authoritative row unchanged",
			buildRepo: func(account *Account) AccountRepository {
				repo := &tokenRefreshAccountRepo{}
				repo.accountsByID = map[int64]*Account{account.ID: account}
				return repo
			},
			wantScope:  GatewayFailureScopeAccount,
			wantReason: GrokCredentialReasonAccountChanged,
			wantAction: NextAccountRetry,
		},
		{
			name: "selected account was deleted",
			buildRepo: func(account *Account) AccountRepository {
				base := &tokenRefreshAccountRepo{}
				base.accountsByID = map[int64]*Account{account.ID: account}
				return &grokCredentialRereadFailureRepo{tokenRefreshAccountRepo: base}
			},
			wantScope:  GatewayFailureScopeAccount,
			wantReason: GrokCredentialReasonAccountChanged,
			wantAction: NextAccountRetry,
		},
		{
			name: "shared account store unavailable",
			buildRepo: func(account *Account) AccountRepository {
				base := &tokenRefreshAccountRepo{}
				base.accountsByID = map[int64]*Account{account.ID: account}
				return &grokCredentialRereadFailureRepo{tokenRefreshAccountRepo: base, err: errors.New("database unavailable")}
			},
			wantScope:  GatewayFailureScopeProvider,
			wantReason: GrokCredentialReasonProviderDown,
			wantAction: NextAccountStop,
		},
	}

	for index, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := expiredGrokOAuthAccountForCredentialTest(int64(7400 + index))
			repo := tt.buildRepo(account)
			cache := &grokTokenCacheForProviderTest{lockResult: false}
			provider := NewGrokTokenProvider(repo, cache)
			provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), &tokenRefresherStub{})
			svc := &OpenAIGatewayService{accountRepo: repo, grokTokenProvider: provider}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())

			startedAt := time.Now()
			_, _, err := svc.getRequestCredential(context.Background(), c, account)

			var failoverErr *UpstreamFailoverError
			require.ErrorAs(t, err, &failoverErr)
			require.Equal(t, tt.wantScope, failoverErr.Scope)
			require.Equal(t, tt.wantReason, failoverErr.Reason)
			require.Equal(t, tt.wantAction, failoverErr.NextAccountAction)
			require.Less(t, time.Since(startedAt), 3*time.Second)
			switch countingRepo := repo.(type) {
			case *tokenRefreshAccountRepo:
				require.Zero(t, countingRepo.setErrorCalls)
				require.Zero(t, countingRepo.setTempUnschedCalls)
			case *grokCredentialRereadFailureRepo:
				require.Zero(t, countingRepo.tokenRefreshAccountRepo.setErrorCalls)
				require.Zero(t, countingRepo.tokenRefreshAccountRepo.setTempUnschedCalls)
			}
			require.Empty(t, cache.deletedKeys)
			require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
		})
	}
}

func TestGrokCredentialMutationCancellationAmbiguityConfirmsDurableCommit(t *testing.T) {
	tests := []struct {
		name      string
		class     grokCredentialFailureClass
		committed func(*Account) bool
	}{
		{
			name:  "permanent quarantine",
			class: grokCredentialFailureClass{reason: GrokCredentialReasonRevoked, permanent: true},
			committed: func(account *Account) bool {
				return account.Status == StatusError && !account.Schedulable && account.ErrorMessage == string(GrokCredentialReasonRevoked)
			},
		},
		{
			name:  "temporary quarantine",
			class: grokCredentialFailureClass{reason: GrokCredentialReasonRefreshTransient, transient: true},
			committed: func(account *Account) bool {
				return account.TempUnschedulableUntil != nil && account.TempUnschedulableReason == string(GrokCredentialReasonRefreshTransient)
			},
		},
	}

	for index, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := expiredGrokOAuthAccountForCredentialTest(int64(737 + index))
			baseRepo := &tokenRefreshAccountRepo{}
			baseRepo.accountsByID = map[int64]*Account{account.ID: account}
			repo := &grokCredentialCommitThenCancelRepo{tokenRefreshAccountRepo: baseRepo}
			cache := &grokTokenCacheForProviderTest{}
			svc := &OpenAIGatewayService{accountRepo: repo, grokTokenProvider: NewGrokTokenProvider(repo, cache)}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
			defer cancel()

			token, err := svc.applyGrokCredentialAccountFailure(ctx, account, tt.class)

			require.ErrorIs(t, err, context.DeadlineExceeded)
			require.Empty(t, token)
			require.True(t, tt.committed(account), "the detached confirmation must recognize the durable mutation")
			require.True(t, svc.isOpenAIAccountRuntimeBlocked(account), "a confirmed durable quarantine must retain its runtime block")
			if tt.class.permanent {
				require.Equal(t, []string{GrokTokenCacheKey(account)}, cache.deletedKeys)
			} else {
				require.Empty(t, cache.deletedKeys)
			}
		})
	}
}

func TestGrokCredentialInnerStateDeadlineAmbiguityConfirmsDurableCommit(t *testing.T) {
	tests := []struct {
		name  string
		class grokCredentialFailureClass
	}{
		{name: "permanent quarantine", class: grokCredentialFailureClass{reason: GrokCredentialReasonRevoked, permanent: true}},
		{name: "temporary quarantine", class: grokCredentialFailureClass{reason: GrokCredentialReasonRefreshTransient, transient: true}},
	}

	for index, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := expiredGrokOAuthAccountForCredentialTest(int64(7500 + index))
			baseRepo := &tokenRefreshAccountRepo{}
			baseRepo.accountsByID = map[int64]*Account{account.ID: account}
			repo := &grokCredentialCommitThenCancelRepo{
				tokenRefreshAccountRepo: baseRepo,
				returnErr:               context.DeadlineExceeded,
			}
			cache := &grokTokenCacheForProviderTest{}
			svc := &OpenAIGatewayService{accountRepo: repo, grokTokenProvider: NewGrokTokenProvider(repo, cache)}

			token, err := svc.applyGrokCredentialAccountFailure(context.Background(), account, tt.class)

			require.NoError(t, err, "the detached readback must resolve the inner timeout's commit ambiguity")
			require.Empty(t, token)
			require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))
			if tt.class.permanent {
				require.Equal(t, StatusError, account.Status)
				require.False(t, account.Schedulable)
				require.Equal(t, []string{GrokTokenCacheKey(account)}, cache.deletedKeys)
			} else {
				require.NotNil(t, account.TempUnschedulableUntil)
				require.Equal(t, string(GrokCredentialReasonRefreshTransient), account.TempUnschedulableReason)
				require.Empty(t, cache.deletedKeys)
			}
		})
	}
}

func TestGrokCredentialUnconfirmedInnerStateDeadlineStopsAndRetainsSafetyBlock(t *testing.T) {
	tests := []struct {
		name  string
		class grokCredentialFailureClass
	}{
		{name: "permanent quarantine", class: grokCredentialFailureClass{reason: GrokCredentialReasonRevoked, permanent: true}},
		{name: "temporary quarantine", class: grokCredentialFailureClass{reason: GrokCredentialReasonRefreshTransient, transient: true}},
	}

	for index, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := expiredGrokOAuthAccountForCredentialTest(int64(7600 + index))
			baseRepo := &tokenRefreshAccountRepo{}
			baseRepo.accountsByID = map[int64]*Account{account.ID: account}
			repo := &grokCredentialUncommittedDeadlineRepo{tokenRefreshAccountRepo: baseRepo}
			cache := &grokTokenCacheForProviderTest{}
			svc := &OpenAIGatewayService{accountRepo: repo, grokTokenProvider: NewGrokTokenProvider(repo, cache)}

			token, err := svc.applyGrokCredentialAccountFailure(context.Background(), account, tt.class)

			require.ErrorIs(t, err, errGrokCredentialStateUpdateFailed)
			require.Empty(t, token)
			require.True(t, svc.isOpenAIAccountRuntimeBlocked(account), "an unknown commit outcome must retain the local safety block")
			require.Equal(t, StatusActive, account.Status)
			require.True(t, account.Schedulable)
			require.Nil(t, account.TempUnschedulableUntil)
			require.Empty(t, cache.deletedKeys)
		})
	}
}

func TestGrokCredentialRuntimeRollbackOwnership(t *testing.T) {
	account := expiredGrokOAuthAccountForCredentialTest(734)
	t.Run("later extending block survives", func(t *testing.T) {
		svc := &OpenAIGatewayService{}
		until := time.Now().Add(time.Minute)
		rollbackFirst := svc.blockGrokCredentialRuntime(account, until, "first")

		secondInstalled := make(chan struct{})
		go func() {
			svc.BlockAccountScheduling(account, until.Add(time.Minute), "independent")
			close(secondInstalled)
		}()
		<-secondInstalled
		rollbackFirst()

		require.True(t, svc.isOpenAIAccountRuntimeBlocked(account),
			"rollback owned by the first invocation must not remove a later extending block")
	})

	t.Run("independent shorter block steals rollback ownership", func(t *testing.T) {
		svc := &OpenAIGatewayService{}
		until := time.Now().Add(2 * time.Minute)
		rollbackFirst := svc.blockGrokCredentialRuntime(account, until, "first")
		svc.BlockAccountScheduling(account, until.Add(-time.Minute), "shorter-no-op")

		rollbackFirst()

		require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))
	})

	t.Run("serialized tentative rollbacks leave no block", func(t *testing.T) {
		svc := &OpenAIGatewayService{}
		for i := 0; i < 2; i++ {
			mu := svc.grokCredentialMutationLock(account.ID)
			require.NoError(t, mu.Lock(context.Background()))
			rollback := svc.blockGrokCredentialRuntime(account, time.Now().Add(time.Minute), "tentative")
			rollback()
			mu.Unlock()
		}
		require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
	})
}

func TestGetRequestCredentialAPIKeyBypassesOAuthFailureMapping(t *testing.T) {
	gin.SetMode(gin.TestMode)
	account := &Account{
		ID:       705,
		Platform: PlatformGrok,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "third-party-key",
			"base_url": "https://grok.example.test/v1",
		},
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	svc := &OpenAIGatewayService{}

	token, kind, err := svc.getRequestCredential(context.Background(), c, account)
	require.NoError(t, err)
	require.Equal(t, "third-party-key", token)
	require.Equal(t, "apikey", kind)
	_, hasEvents := c.Get(OpsUpstreamErrorsKey)
	require.False(t, hasEvents)
}

func TestPermanentCredentialFailureDoesNotDisableConcurrentlyRefreshedAccount(t *testing.T) {
	account := expiredGrokOAuthAccountForCredentialTest(707)
	latest := *account
	latest.Credentials = shallowCopyMap(account.Credentials)
	latest.Credentials["access_token"] = "fresh-access-token"
	latest.Credentials["refresh_token"] = "rotated-refresh-token"
	latest.Credentials["expires_at"] = time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	latest.Credentials["_token_version"] = time.Now().UnixMilli()
	repo := &tokenRefreshAccountRepo{}
	repo.accountsByID = map[int64]*Account{account.ID: &latest}
	cache := &grokTokenCacheForProviderTest{}
	svc := &OpenAIGatewayService{
		accountRepo:       repo,
		grokTokenProvider: NewGrokTokenProvider(repo, cache),
	}

	_, mutationErr := svc.applyGrokCredentialAccountFailure(context.Background(), account, grokCredentialFailureClass{
		scope:     GatewayFailureScopeAccount,
		reason:    GrokCredentialReasonRevoked,
		action:    NextAccountRetry,
		permanent: true,
	})

	require.NoError(t, mutationErr)
	require.Zero(t, repo.setErrorCalls)
	require.Empty(t, cache.deletedKeys)
	require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
}

func TestCredentialFailureConditionalMutationLosesToConcurrentRefresh(t *testing.T) {
	for index, tt := range []struct {
		name       string
		refreshErr error
	}{
		{name: "permanent", refreshErr: infraerrors.New(http.StatusBadGateway, "GROK_OAUTH_TOKEN_REFRESH_FAILED", "invalid_grant")},
		{name: "transient", refreshErr: errors.New("temporary refresh transport failure")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			account := expiredGrokOAuthAccountForCredentialTest(int64(770 + index))
			repo := &tokenRefreshAccountRepo{}
			repo.accountsByID = map[int64]*Account{account.ID: account}
			repo.beforeConditionalState = func() {
				fresh := *account
				fresh.Credentials = shallowCopyMap(account.Credentials)
				fresh.Credentials["access_token"] = "refresh-won-token"
				fresh.Credentials["refresh_token"] = "refresh-won-refresh"
				fresh.Credentials["expires_at"] = time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339)
				fresh.Credentials["_token_version"] = time.Now().UnixMilli()
				repo.accountsByID[account.ID] = &fresh
			}
			cache := &grokTokenCacheForProviderTest{lockResult: true}
			provider := NewGrokTokenProvider(repo, cache)
			provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), &tokenRefresherStub{err: tt.refreshErr})
			svc := &OpenAIGatewayService{accountRepo: repo, grokTokenProvider: provider}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())

			token, kind, err := svc.getRequestCredential(context.Background(), c, account)

			require.NoError(t, err)
			require.Equal(t, "refresh-won-token", token)
			require.Equal(t, "oauth", kind)
			require.Zero(t, repo.setErrorCalls)
			require.Zero(t, repo.setTempUnschedCalls)
			require.Empty(t, cache.deletedKeys)
			require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
		})
	}
}

func TestCredentialFailureConditionalMutationLosesToConcurrentProxyRepair(t *testing.T) {
	for index, tt := range []struct {
		name       string
		refreshErr error
	}{
		{name: "permanent", refreshErr: infraerrors.New(http.StatusBadGateway, "GROK_OAUTH_TOKEN_REFRESH_FAILED", "invalid_grant")},
		{name: "transient", refreshErr: errors.New("temporary refresh transport failure")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			account := expiredGrokOAuthAccountForCredentialTest(int64(780 + index))
			oldProxyID := int64(10)
			account.ProxyID = &oldProxyID
			account.Proxy = &Proxy{}
			repo := &tokenRefreshAccountRepo{}
			repo.accountsByID = map[int64]*Account{account.ID: account}
			repo.beforeConditionalState = func() {
				fresh := *account
				fresh.Credentials = shallowCopyMap(account.Credentials)
				repairedProxyID := int64(11)
				fresh.ProxyID = &repairedProxyID
				repo.accountsByID[account.ID] = &fresh
			}
			cache := &grokTokenCacheForProviderTest{lockResult: true}
			provider := NewGrokTokenProvider(repo, cache)
			provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), &tokenRefresherStub{err: tt.refreshErr})
			svc := &OpenAIGatewayService{accountRepo: repo, grokTokenProvider: provider}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())

			_, _, err := svc.getRequestCredential(context.Background(), c, account)

			var failoverErr *UpstreamFailoverError
			require.ErrorAs(t, err, &failoverErr)
			require.Equal(t, GrokCredentialReasonAccountChanged, failoverErr.Reason)
			require.Equal(t, NextAccountRetry, failoverErr.NextAccountAction)
			require.Zero(t, repo.setErrorCalls)
			require.Zero(t, repo.setTempUnschedCalls)
			require.Empty(t, cache.deletedKeys)
			require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
		})
	}
}

func TestCredentialFailureConditionalMutationLosesToSameIDProxyRestoration(t *testing.T) {
	account := expiredGrokOAuthAccountForCredentialTest(790)
	proxyID := int64(10)
	account.ProxyID = &proxyID
	account.Proxy = nil
	repo := &tokenRefreshAccountRepo{}
	repo.accountsByID = map[int64]*Account{account.ID: account}
	repo.beforeConditionalState = func() {
		account.Proxy = &Proxy{ID: proxyID}
	}
	cache := &grokTokenCacheForProviderTest{lockResult: true}
	provider := NewGrokTokenProvider(repo, cache)
	svc := &OpenAIGatewayService{accountRepo: repo, grokTokenProvider: provider}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	_, _, err := svc.getRequestCredential(context.Background(), c, account)

	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, GrokCredentialReasonAccountChanged, failoverErr.Reason)
	require.Equal(t, NextAccountRetry, failoverErr.NextAccountAction)
	require.Zero(t, repo.setErrorCalls)
	require.Zero(t, repo.setTempUnschedCalls)
	require.Empty(t, cache.deletedKeys)
	require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
}

func TestCredentialFailureConditionalMutationLosesToConcurrentUnschedulableState(t *testing.T) {
	future := time.Now().Add(time.Hour)
	states := []struct {
		name   string
		mutate func(*Account)
	}{
		{name: "admin schedulable false", mutate: func(account *Account) { account.Schedulable = false }},
		{name: "temporary cooldown", mutate: func(account *Account) { account.TempUnschedulableUntil = &future }},
		{name: "rate limit cooldown", mutate: func(account *Account) { account.RateLimitResetAt = &future }},
		{name: "overload cooldown", mutate: func(account *Account) { account.OverloadUntil = &future }},
	}
	classes := []struct {
		name  string
		class grokCredentialFailureClass
	}{
		{name: "permanent", class: grokCredentialFailureClass{reason: GrokCredentialReasonRevoked, permanent: true}},
		{name: "transient", class: grokCredentialFailureClass{reason: GrokCredentialReasonRefreshTransient, transient: true}},
	}

	for classIndex, classCase := range classes {
		for stateIndex, stateCase := range states {
			t.Run(classCase.name+"/"+stateCase.name, func(t *testing.T) {
				account := expiredGrokOAuthAccountForCredentialTest(int64(791 + classIndex*10 + stateIndex))
				repo := &tokenRefreshAccountRepo{}
				repo.accountsByID = map[int64]*Account{account.ID: account}
				repo.beforeConditionalState = func() { stateCase.mutate(account) }
				svc := &OpenAIGatewayService{accountRepo: repo, grokTokenProvider: NewGrokTokenProvider(repo, &grokTokenCacheForProviderTest{})}

				token, err := svc.applyGrokCredentialAccountFailure(context.Background(), account, classCase.class)

				require.ErrorIs(t, err, errOAuthRefreshAccountStateChanged)
				require.Empty(t, token)
				require.Zero(t, repo.setErrorCalls)
				require.Zero(t, repo.setTempUnschedCalls)
				require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
			})
		}
	}
}

func TestCredentialFailureCASMissDoesNotRecoverIneligibleLatestCredential(t *testing.T) {
	future := time.Now().Add(time.Hour)
	states := []struct {
		name               string
		mutate             func(*OpenAIGatewayService, *Account)
		wantRuntimeBlocked bool
	}{
		{name: "disabled", mutate: func(_ *OpenAIGatewayService, account *Account) { account.Status = StatusDisabled }},
		{name: "not schedulable", mutate: func(_ *OpenAIGatewayService, account *Account) { account.Schedulable = false }},
		{name: "temporarily unschedulable", mutate: func(_ *OpenAIGatewayService, account *Account) { account.TempUnschedulableUntil = &future }},
		{name: "rate limited", mutate: func(_ *OpenAIGatewayService, account *Account) { account.RateLimitResetAt = &future }},
		{name: "overloaded", mutate: func(_ *OpenAIGatewayService, account *Account) { account.OverloadUntil = &future }},
		{
			name: "independently runtime blocked",
			mutate: func(svc *OpenAIGatewayService, account *Account) {
				svc.BlockAccountScheduling(account, time.Now().Add(24*time.Hour), "independent")
			},
			wantRuntimeBlocked: true,
		},
	}
	classes := []struct {
		name  string
		class grokCredentialFailureClass
	}{
		{name: "permanent", class: grokCredentialFailureClass{reason: GrokCredentialReasonRevoked, permanent: true}},
		{name: "transient", class: grokCredentialFailureClass{reason: GrokCredentialReasonRefreshTransient, transient: true}},
	}

	for classIndex, classCase := range classes {
		for stateIndex, stateCase := range states {
			t.Run(classCase.name+"/"+stateCase.name, func(t *testing.T) {
				account := expiredGrokOAuthAccountForCredentialTest(int64(800 + classIndex*20 + stateIndex))
				repo := &tokenRefreshAccountRepo{}
				repo.accountsByID = map[int64]*Account{account.ID: account}
				svc := &OpenAIGatewayService{accountRepo: repo, grokTokenProvider: NewGrokTokenProvider(repo, &grokTokenCacheForProviderTest{})}
				repo.beforeConditionalState = func() {
					latest := *account
					latest.Credentials = shallowCopyMap(account.Credentials)
					latest.Credentials["access_token"] = "fresh-but-ineligible-token"
					latest.Credentials["refresh_token"] = "fresh-but-ineligible-refresh"
					latest.Credentials["expires_at"] = time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339)
					latest.Credentials["_token_version"] = time.Now().UnixMilli()
					stateCase.mutate(svc, &latest)
					repo.accountsByID[account.ID] = &latest
				}

				token, err := svc.applyGrokCredentialAccountFailure(context.Background(), account, classCase.class)

				require.ErrorIs(t, err, errOAuthRefreshAccountStateChanged)
				require.Empty(t, token)
				require.Zero(t, repo.setErrorCalls)
				require.Zero(t, repo.setTempUnschedCalls)
				require.Equal(t, stateCase.wantRuntimeBlocked, svc.isOpenAIAccountRuntimeBlocked(account))
			})
		}
	}
}

func TestGetRequestCredentialSharedCredentialPersistenceFailureStopsWithoutAccountMutation(t *testing.T) {
	account := expiredGrokOAuthAccountForCredentialTest(782)
	repo := &tokenRefreshAccountRepo{conditionalSuccessErr: errors.New("database unavailable")}
	repo.accountsByID = map[int64]*Account{account.ID: account}
	cache := &grokTokenCacheForProviderTest{lockResult: true}
	provider := NewGrokTokenProvider(repo, cache)
	provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), &tokenRefresherStub{credentials: map[string]any{
		"access_token":  "new-access-token",
		"refresh_token": "new-refresh-token",
		"expires_at":    time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
	}})
	svc := &OpenAIGatewayService{accountRepo: repo, grokTokenProvider: provider}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	_, _, err := svc.getRequestCredential(context.Background(), c, account)

	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, GatewayFailureScopeProvider, failoverErr.Scope)
	require.Equal(t, GrokCredentialReasonProviderDown, failoverErr.Reason)
	require.Equal(t, NextAccountStop, failoverErr.NextAccountAction)
	require.Equal(t, 1, repo.conditionalSuccessCalls)
	require.Zero(t, repo.updateCredentialsCalls)
	require.Zero(t, repo.setErrorCalls)
	require.Zero(t, repo.setTempUnschedCalls)
	require.Empty(t, cache.deletedKeys)
	require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
}

func TestGetRequestCredentialRecoversConcurrentRefreshWithoutFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	account := expiredGrokOAuthAccountForCredentialTest(733)
	latest := *account
	latest.Credentials = shallowCopyMap(account.Credentials)
	latest.Credentials["access_token"] = "fresh-concurrent-access"
	latest.Credentials["refresh_token"] = "fresh-concurrent-refresh"
	latest.Credentials["expires_at"] = time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339)
	latest.Credentials["_token_version"] = time.Now().UnixMilli()
	baseRepo := &tokenRefreshAccountRepo{}
	baseRepo.accountsByID = map[int64]*Account{account.ID: account}
	repo := &grokCredentialSequencedRepo{tokenRefreshAccountRepo: baseRepo, latest: &latest}
	cache := &grokTokenCacheForProviderTest{lockResult: true}
	provider := NewGrokTokenProvider(repo, cache)
	provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), &tokenRefresherStub{
		err: infraerrors.New(http.StatusForbidden, "GROK_OAUTH_ENTITLEMENT_DENIED", "access_denied"),
	})
	svc := &OpenAIGatewayService{accountRepo: repo, grokTokenProvider: provider}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	token, kind, err := svc.getRequestCredential(context.Background(), c, account)

	require.NoError(t, err)
	require.Equal(t, "fresh-concurrent-access", token)
	require.Equal(t, "oauth", kind)
	require.Zero(t, baseRepo.setErrorCalls)
	require.Zero(t, baseRepo.setTempUnschedCalls)
	require.Empty(t, cache.deletedKeys)
	require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
	_, hasEvents := c.Get(OpsUpstreamErrorsKey)
	require.False(t, hasEvents)
}

func expiredGrokOAuthAccountForCredentialTest(id int64) *Account {
	return &Account{
		ID:          id,
		Platform:    PlatformGrok,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Credentials: map[string]any{
			"access_token":  "expired-access-token",
			"refresh_token": "refresh-token",
			"expires_at":    time.Now().Add(-time.Minute).UTC().Format(time.RFC3339),
			"base_url":      xai.DefaultCLIBaseURL,
		},
	}
}
