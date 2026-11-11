//go:build unit

package service

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func newOpenAI401TestAccount() *Account {
	return &Account{ID: 5542, Name: "synthetic-oauth", Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Status: StatusActive, Concurrency: 1, Credentials: map[string]any{
			"access_token": "rejected-at", "refresh_token": "old-rt",
			"expires_at": time.Now().Add(2 * time.Hour).Format(time.RFC3339)}}
}

func TestOpenAIOAuth401HTTPRecoveryBoundaries(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		mode := "forward"
		if passthrough {
			mode = "passthrough"
		}
		for _, tc := range []struct {
			name, body, kind       string
			failRefresh, unchanged bool
			requests, refreshes    int
		}{
			{name: "revoked", body: `{"error":{"code":"token_revoked"}}`, requests: 1},
			{name: "invalidated", body: `{"error":{"code":"token_invalidated"}}`, requests: 1},
			{name: "detail_unauthorized", body: `{"detail":"Unauthorized"}`, requests: 1},
			{name: "scope", body: `{"error":{"message":"Missing scopes: api.responses.write."}}`, requests: 1},
			{name: "api_key", kind: "api_key", requests: 1},
			{name: "setup_token", kind: "setup_token", requests: 1},
			{name: "pat", kind: "pat", requests: 1},
			{name: "missing_refresh_token", kind: "no_rt", requests: 1},
			{name: "refresh_error", failRefresh: true, requests: 1, refreshes: 1},
			{name: "unchanged_access_token", unchanged: true, requests: 1, refreshes: 1},
			{name: "second_401", requests: 2, refreshes: 1},
		} {
			t.Run(mode+"/"+tc.name, func(t *testing.T) {
				account := newOpenAI401TestAccount()
				switch tc.kind {
				case "api_key":
					account.Type = AccountTypeAPIKey
					account.Credentials["api_key"] = "synthetic-api-key"
				case "setup_token":
					account.Type = AccountTypeSetupToken
				case "pat":
					account.Credentials["auth_mode"] = OpenAIAuthModePersonalAccessToken
				case "no_rt":
					delete(account.Credentials, "refresh_token")
				}
				expectedCredentials := shallowCopyMap(account.Credentials)
				repo := &refreshAPIAccountRepo{account: account}
				cache := newOpenAITokenCacheStub()
				executor := &refreshAPIExecutorStub{credentials: map[string]any{
					"access_token": "fresh-at", "refresh_token": "rotated-rt",
					"expires_at": time.Now().Add(2 * time.Hour).Format(time.RFC3339)}}
				if tc.failRefresh {
					executor.err = errors.New("synthetic refresh failure")
				}
				if tc.unchanged {
					executor.credentials["access_token"] = "rejected-at"
				}
				provider := NewOpenAITokenProvider(repo, cache, nil)
				provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), executor)
				rejection := tc.body
				if rejection == "" {
					rejection = `{"error":{"code":"invalid_api_key","message":"token rejected"}}`
				}
				upstream := &httpUpstreamRecorder{responses: []*http.Response{
					newOpenAIRejectedFieldTestResponse(401, rejection),
					newOpenAIRejectedFieldTestResponse(401, rejection),
				}}
				svc := newOpenAIRejectedFieldTestService(upstream)
				svc.openAITokenProvider = provider
				body := []byte(`{"model":"gpt-5.6-sol","instructions":"synthetic","input":[{"role":"user","content":"synthetic"}],"stream":false}`)
				c := newOpenAIRejectedFieldTestContext(body)
				SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)
				var err error
				if passthrough {
					_, err = svc.forwardOpenAIPassthrough(context.Background(), c, account, body, body, "gpt-5.6-sol", false, nil, false, time.Now())
				} else {
					_, err = svc.Forward(context.Background(), c, account, body)
				}
				require.Error(t, err)
				require.Len(t, upstream.requests, tc.requests)
				require.Equal(t, tc.refreshes, executor.refreshCalls)
				if tc.refreshes == 0 || tc.failRefresh {
					require.Equal(t, expectedCredentials, repo.account.Credentials)
				}
			})
		}
	}
}

// Snapshot reads keep the concurrent fixture independent of mutable request
// objects, as real repository reads do.
type openAI401ConcurrentRepo struct {
	AccountRepository
	mu      sync.Mutex
	account *Account
}

func (r *openAI401ConcurrentRepo) GetByID(context.Context, int64) (*Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return snapshotOAuthRefreshAccount(r.account), nil
}
func (r *openAI401ConcurrentRepo) UpdateCredentials(_ context.Context, _ int64, credentials map[string]any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.account.Credentials = shallowCopyMap(credentials)
	return nil
}
func TestOpenAIOAuth401ConcurrentRefreshReusesWinner(t *testing.T) {
	original := newOpenAI401TestAccount()
	repo := &openAI401ConcurrentRepo{account: snapshotOAuthRefreshAccount(original)}
	cache := newOpenAITokenCacheStub()
	entered := make(chan struct{})
	release := make(chan struct{})
	executor := &refreshAPIExecutorStub{credentials: map[string]any{
		"access_token": "fresh-at", "refresh_token": "rotated-rt",
		"expires_at": time.Now().Add(2 * time.Hour).Format(time.RFC3339)},
		onRefresh: func() { close(entered); <-release }}
	provider := NewOpenAITokenProvider(repo, cache, nil)
	provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), executor)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	type outcome struct {
		token string
		err   error
	}
	outcomes := make(chan outcome, 2)
	call := func() {
		token, err := provider.refreshRejectedAccessToken(ctx, original, "rejected-at")
		outcomes <- outcome{token, err}
	}
	go call()
	<-entered
	go call()
	close(release)
	for i := 0; i < 2; i++ {
		result := <-outcomes
		require.NoError(t, result.err)
		require.Equal(t, "fresh-at", result.token)
	}
	require.Equal(t, 1, executor.refreshCalls, "a late 401 must not rotate the winner's refresh token again")
	fresh, err := repo.GetByID(ctx, original.ID)
	require.NoError(t, err)
	require.Equal(t, "rotated-rt", fresh.GetOpenAIRefreshToken())
	require.Equal(t, "old-rt", original.GetOpenAIRefreshToken(), "request snapshots must remain untouched")
}

type openAI401WinnerRepo struct {
	AccountRepository
	old, fresh *Account
	reads      int
}

func (r *openAI401WinnerRepo) GetByID(context.Context, int64) (*Account, error) {
	r.reads++
	if r.reads == 1 {
		return snapshotOAuthRefreshAccount(r.old), nil
	}
	return snapshotOAuthRefreshAccount(r.fresh), nil
}
func TestOpenAIOAuth401DistributedLockWinner(t *testing.T) {
	old := newOpenAI401TestAccount()
	fresh := snapshotOAuthRefreshAccount(old)
	fresh.Credentials["access_token"] = "winner-at"
	fresh.Credentials["refresh_token"] = "winner-rt"
	repo := &openAI401WinnerRepo{old: old, fresh: fresh}
	cache := newOpenAITokenCacheStub()
	cache.lockAcquired = false
	cache.tokens[OpenAITokenCacheKey(old)] = "rejected-at"
	executor := &refreshAPIExecutorStub{}
	provider := NewOpenAITokenProvider(repo, cache, nil)
	provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), executor)
	token, err := provider.refreshRejectedAccessToken(context.Background(), old, "rejected-at")
	require.NoError(t, err)
	require.Equal(t, "winner-at", token)
	require.Zero(t, executor.refreshCalls)
	require.NotContains(t, cache.tokens, OpenAITokenCacheKey(old))
}

func TestOpenAIOAuth401RecoveryCancellationAndAccountChanges(t *testing.T) {
	t.Run("cancelled_refresh_does_not_persist", func(t *testing.T) {
		account := newOpenAI401TestAccount()
		repo := &refreshAPIAccountRepo{account: account}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		executor := &refreshAPIExecutorStub{credentials: map[string]any{"access_token": "late-at", "refresh_token": "late-rt"}, onRefresh: cancel}
		provider := NewOpenAITokenProvider(repo, nil, nil)
		provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, nil), executor)
		_, err := provider.refreshRejectedAccessToken(ctx, account, "rejected-at")
		require.ErrorIs(t, err, context.Canceled)
		require.Zero(t, repo.updateCredentialsCalls)
		require.Equal(t, "old-rt", account.GetOpenAIRefreshToken())
	})
	t.Run("changed_proxy", func(t *testing.T) {
		old := newOpenAI401TestAccount()
		fresh := snapshotOAuthRefreshAccount(old)
		proxyID := int64(44)
		fresh.ProxyID = &proxyID
		repo := &refreshAPIAccountRepo{account: fresh}
		executor := &refreshAPIExecutorStub{}
		provider := NewOpenAITokenProvider(repo, nil, nil)
		provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, nil), executor)
		_, err := provider.refreshRejectedAccessToken(context.Background(), old, "rejected-at")
		require.Error(t, err)
		require.Zero(t, executor.refreshCalls)
	})
	t.Run("lock_held_without_rotation", func(t *testing.T) {
		account := newOpenAI401TestAccount()
		repo := &refreshAPIAccountRepo{account: account}
		cache := newOpenAITokenCacheStub()
		cache.lockAcquired = false
		executor := &refreshAPIExecutorStub{}
		provider := NewOpenAITokenProvider(repo, cache, nil)
		provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), executor)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := provider.refreshRejectedAccessToken(ctx, account, "rejected-at")
		require.Error(t, err)
		require.Zero(t, executor.refreshCalls)
	})
}

// A durable winner can make RefreshIfNeeded return without invoking the
// executor. Cancellation at that boundary must still stop recovery.
type openAI401CancelledWinnerRepo struct {
	AccountRepository
	account *Account
	cancel  context.CancelFunc
}

func (r *openAI401CancelledWinnerRepo) GetByID(context.Context, int64) (*Account, error) {
	r.cancel()
	return snapshotOAuthRefreshAccount(r.account), nil
}

func TestOpenAIOAuth401CancelledWinnerIsNotRetried(t *testing.T) {
	old := newOpenAI401TestAccount()
	fresh := snapshotOAuthRefreshAccount(old)
	fresh.Credentials["access_token"] = "winner-at"
	fresh.Credentials["refresh_token"] = "winner-rt"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	repo := &openAI401CancelledWinnerRepo{account: fresh, cancel: cancel}
	executor := &refreshAPIExecutorStub{}
	provider := NewOpenAITokenProvider(repo, nil, nil)
	provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, nil), executor)
	token, err := provider.refreshRejectedAccessToken(ctx, old, "rejected-at")
	require.ErrorIs(t, err, context.Canceled)
	require.Empty(t, token)
	require.Zero(t, executor.refreshCalls)
	require.Equal(t, "old-rt", old.GetOpenAIRefreshToken())
}

func TestOpenAIOAuth401ShadowRefreshesCredentialOwner(t *testing.T) {
	owner := newOpenAI401TestAccount()
	shadow := &Account{ID: 5543, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Status: StatusActive, ParentAccountID: &owner.ID,
		Credentials: map[string]any{"model_mapping": map[string]any{"spark": "gpt-5.6-sol"}}}
	expectedShadow := shallowCopyMap(shadow.Credentials)
	repo := &refreshAPIAccountRepo{account: owner}
	cache := newOpenAITokenCacheStub()
	cache.tokens[OpenAITokenCacheKey(owner)] = "rejected-at"
	executor := &refreshAPIExecutorStub{credentials: map[string]any{
		"access_token": "fresh-at", "refresh_token": "rotated-rt",
		"expires_at": time.Now().Add(2 * time.Hour).Format(time.RFC3339)}}
	provider := NewOpenAITokenProvider(repo, cache, nil)
	provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), executor)
	svc := &OpenAIGatewayService{accountRepo: repo, openAITokenProvider: provider}
	token, recovered := svc.tryRefreshOpenAIHTTP401(context.Background(), shadow,
		http.StatusUnauthorized, []byte(`{"error":{"code":"invalid_api_key"}}`), "rejected-at")
	require.True(t, recovered)
	require.Equal(t, "fresh-at", token)
	require.Equal(t, owner.ID, repo.account.ID)
	require.Equal(t, "rotated-rt", repo.account.GetOpenAIRefreshToken())
	require.Equal(t, 1, executor.refreshCalls)
	require.Equal(t, expectedShadow, shadow.Credentials)
	require.NotContains(t, cache.tokens, OpenAITokenCacheKey(owner))
}

func TestOpenAIOAuth401ShadowChangedProxyNotRetried(t *testing.T) {
	owner := newOpenAI401TestAccount()
	proxyID := int64(44)
	owner.ProxyID = &proxyID
	// The selected shadow still represents the old direct transport.
	shadow := &Account{ID: 5543, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Status: StatusActive, ParentAccountID: &owner.ID}
	repo := &refreshAPIAccountRepo{account: owner}
	executor := &refreshAPIExecutorStub{credentials: map[string]any{
		"access_token": "fresh-at", "refresh_token": "rotated-rt"}}
	provider := NewOpenAITokenProvider(repo, nil, nil)
	provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, nil), executor)
	svc := &OpenAIGatewayService{accountRepo: repo, openAITokenProvider: provider}
	token, recovered := svc.tryRefreshOpenAIHTTP401(context.Background(), shadow,
		http.StatusUnauthorized, nil, "rejected-at")
	require.False(t, recovered)
	require.Empty(t, token)
	require.Zero(t, executor.refreshCalls)
	require.Equal(t, "old-rt", owner.GetOpenAIRefreshToken())
}

func TestOpenAIOAuth401AgentIdentityNotRefreshed(t *testing.T) {
	account := newOpenAI401TestAccount()
	account.Credentials["auth_mode"] = OpenAIAuthModeAgentIdentity
	repo := &refreshAPIAccountRepo{account: account}
	executor := &refreshAPIExecutorStub{}
	provider := NewOpenAITokenProvider(repo, nil, nil)
	provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, nil), executor)
	svc := &OpenAIGatewayService{accountRepo: repo, openAITokenProvider: provider}
	token, recovered := svc.tryRefreshOpenAIHTTP401(context.Background(), account,
		http.StatusUnauthorized, nil, "rejected-at")
	require.False(t, recovered)
	require.Empty(t, token)
	require.Zero(t, executor.refreshCalls)
	require.Zero(t, repo.updateCredentialsCalls)
}
