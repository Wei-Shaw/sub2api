//go:build unit

package service

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type openAIRecoveryRepo struct {
	mockAccountRepoForGemini
	mu        sync.Mutex
	account   *Account
	writes    int
	beforeCAS func()
}

func (r *openAIRecoveryRepo) GetByID(context.Context, int64) (*Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return snapshotOAuthRefreshAccount(r.account), nil
}

func (r *openAIRecoveryRepo) UpdateExtra(context.Context, int64, map[string]any) error {
	return nil
}

func (r *openAIRecoveryRepo) UpdateOpenAIOAuthCredentialsIfUnchanged(_ context.Context, _ int64, expected map[string]any, proxy *int64, credentials map[string]any) (bool, error) {
	if r.beforeCAS != nil {
		r.beforeCAS()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !reflect.DeepEqual(r.account.Credentials, expected) || !reflect.DeepEqual(r.account.ProxyID, proxy) {
		return false, nil
	}
	r.account.Credentials = shallowCopyMap(credentials)
	r.writes++
	return true, nil
}

func (r *openAIRecoveryRepo) SetOpenAIOAuthRecoveryStateIfUnchanged(_ context.Context, expected *Account, message string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !reflect.DeepEqual(r.account.Credentials, expected.Credentials) ||
		!reflect.DeepEqual(r.account.ProxyID, expected.ProxyID) || r.account.Status != expected.Status ||
		r.account.ErrorMessage != expected.ErrorMessage || r.account.Schedulable != expected.Schedulable {
		return false, nil
	}
	r.account.Status = StatusError
	r.account.ErrorMessage = message
	r.account.Schedulable = false
	if message == "" {
		r.account.Status = StatusActive
		r.account.Schedulable = true
	}
	return true, nil
}

func newOpenAIRecoveryFixture() (*Account, *openAIRecoveryRepo, *OpenAITokenProvider, *refreshAPIExecutorStub) {
	account := &Account{
		ID: 42, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true,
		Credentials: map[string]any{
			"access_token": "rejected", "refresh_token": "refresh-original",
			"expires_at": time.Now().Add(24 * time.Hour).Unix(), "_token_version": int64(1),
		},
	}
	repo := &openAIRecoveryRepo{account: snapshotOAuthRefreshAccount(account)}
	executor := &refreshAPIExecutorStub{credentials: map[string]any{
		"access_token": "replacement", "refresh_token": "refresh-rotated",
		"expires_at": time.Now().Add(24 * time.Hour).Unix(),
	}}
	provider := NewOpenAITokenProvider(repo, nil, nil)
	provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, nil), executor)
	return account, repo, provider, executor
}

func recoveryTestService(repo *openAIRecoveryRepo, provider *OpenAITokenProvider, upstream *queuedHTTPUpstream) *AccountTestService {
	return &AccountTestService{accountRepo: repo, httpUpstream: upstream,
		openaiGatewayService: &OpenAIGatewayService{accountRepo: repo, openAITokenProvider: provider}}
}

func TestOpenAIOAuthRecovery_ScheduledProbe(t *testing.T) {
	for _, legacyError := range []bool{false, true} {
		name := "active"
		if legacyError {
			name = "legacy_test_error"
		}
		t.Run(name, func(t *testing.T) {
			account, repo, provider, executor := newOpenAIRecoveryFixture()
			if legacyError {
				repo.account.Status = StatusError
				repo.account.Schedulable = false
				repo.account.ErrorMessage = `Authentication failed (401): {"error":{"code":"token_expired"}}`
			}
			upstream := &queuedHTTPUpstream{responses: []*http.Response{
				newJSONResponse(401, `{"error":{"code":"token_expired"}}`),
				newJSONResponse(200, "data: {\"type\":\"response.completed\"}\n\n"),
			}}
			svc := recoveryTestService(repo, provider, upstream)
			result, err := svc.RunTestBackground(context.Background(), account.ID, "gpt-5.4")
			require.NoError(t, err)
			require.Equal(t, "success", result.Status)
			require.Equal(t, 1, executor.refreshCalls)
			require.Len(t, upstream.requests, 2)
			require.Equal(t, "Bearer replacement", upstream.requests[1].Header.Get("Authorization"))
			if legacyError {
				// Refresh/probe success cannot restore scheduling by itself.
				require.Equal(t, StatusError, repo.account.Status)
				require.False(t, repo.account.Schedulable)
				runner := &ScheduledTestRunnerService{rateLimitSvc: &RateLimitService{accountRepo: repo}}
				runner.tryRecoverAccount(context.Background(), account.ID, 1)
				require.Equal(t, StatusActive, repo.account.Status)
				require.True(t, repo.account.Schedulable)
			}
			require.Equal(t, "refresh-rotated", repo.account.GetOpenAIRefreshToken())
		})
	}
}

func TestOpenAIOAuthRecovery_StopsAndPreservesState(t *testing.T) {
	for _, tc := range []struct {
		name       string
		body       string
		refreshErr error
		second401  bool
		wantCalls  int
		wantError  bool
	}{
		{"revoked", `{"error":{"code":"token_revoked"}}`, nil, false, 0, true},
		{"scope", `{"error":{"code":"insufficient_scope"}}`, nil, false, 0, true},
		{"invalid_refresh", `{"error":{"code":"token_expired"}}`, errors.New("invalid_grant"), false, 1, true},
		{"transient_refresh", `{"error":{"code":"token_expired"}}`, errors.New("upstream unavailable"), false, 1, false},
		{"second_401", `{"error":{"code":"token_expired"}}`, nil, true, 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			account, repo, provider, executor := newOpenAIRecoveryFixture()
			executor.err = tc.refreshErr
			responses := []*http.Response{newJSONResponse(401, tc.body)}
			if tc.second401 {
				responses = append(responses, newJSONResponse(401, tc.body))
			}
			upstream := &queuedHTTPUpstream{responses: responses}
			svc := recoveryTestService(repo, provider, upstream)
			result, err := svc.RunTestBackground(context.Background(), account.ID, "gpt-5.4")
			require.NoError(t, err)
			require.Equal(t, "failed", result.Status)
			require.Equal(t, tc.wantCalls, executor.refreshCalls)
			if tc.wantError {
				require.Equal(t, StatusError, repo.account.Status)
				require.Contains(t, repo.account.ErrorMessage, openAIReauthorizationRequired)
				require.False(t, repo.account.Schedulable)
				upstream.responses = []*http.Response{newJSONResponse(401, tc.body)}
				_, err = svc.RunTestBackground(context.Background(), account.ID, "gpt-5.4")
				require.NoError(t, err)
				require.Equal(t, tc.wantCalls, executor.refreshCalls, "terminal errors must not refresh again")
			} else {
				require.Equal(t, StatusActive, repo.account.Status)
				require.True(t, repo.account.Schedulable)
			}
			require.LessOrEqual(t, len(upstream.requests), 2)
		})
	}
}

func TestOpenAIOAuthRecovery_ConcurrentRejectionsRotateOnce(t *testing.T) {
	account, repo, provider, executor := newOpenAIRecoveryFixture()
	var wg sync.WaitGroup
	for range 12 {
		wg.Go(func() {
			fresh, err := provider.refreshRejectedAccessToken(context.Background(), account, "rejected", false)
			require.NoError(t, err)
			if fresh != nil {
				require.Equal(t, "replacement", fresh.GetOpenAIAccessToken())
			}
		})
	}
	wg.Wait()
	require.Equal(t, 1, executor.refreshCalls)
	require.Equal(t, 1, repo.writes)
	require.Equal(t, "refresh-rotated", repo.account.GetOpenAIRefreshToken())
}

func TestOpenAIOAuthRecovery_ConcurrentCredentialWinner(t *testing.T) {
	for _, duringRefresh := range []bool{false, true} {
		t.Run(map[bool]string{false: "before_lock", true: "before_persist"}[duringRefresh], func(t *testing.T) {
			account, repo, provider, executor := newOpenAIRecoveryFixture()
			rotate := func() {
				repo.mu.Lock()
				defer repo.mu.Unlock()
				repo.account.Credentials = map[string]any{
					"access_token": "winner", "refresh_token": "winner-refresh", "_token_version": int64(3),
				}
			}
			if duringRefresh {
				repo.beforeCAS = rotate
			} else {
				rotate()
			}
			fresh, err := provider.refreshRejectedAccessToken(context.Background(), account, "rejected", false)
			require.NoError(t, err)
			require.Equal(t, "winner", fresh.GetOpenAIAccessToken())
			require.Equal(t, "winner-refresh", repo.account.GetOpenAIRefreshToken())
			require.Zero(t, repo.writes)
			require.Equal(t, map[bool]int{false: 0, true: 1}[duringRefresh], executor.refreshCalls)
		})
	}
}

func TestOpenAIOAuthRecovery_LockAndStateBoundaries(t *testing.T) {
	for _, tc := range []string{"redis_error", "lock_held", "cancelled", "admin_disabled", "version_changed", "unchanged_token"} {
		t.Run(tc, func(t *testing.T) {
			account, repo, provider, executor := newOpenAIRecoveryFixture()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			cache := &refreshAPICacheStub{lockResult: true}
			provider.tokenCache = cache
			provider.refreshAPI = NewOAuthRefreshAPI(repo, cache)
			switch tc {
			case "redis_error":
				cache.lockErr = errors.New("redis unavailable")
			case "lock_held":
				cache.lockResult = false
			case "cancelled":
				cancel()
			case "admin_disabled":
				repo.account.Status = StatusDisabled
			case "version_changed":
				repo.account.Credentials["_token_version"] = int64(2)
			case "unchanged_token":
				executor.credentials["access_token"] = "rejected"
			}
			_, err := provider.refreshRejectedAccessToken(ctx, account, "rejected", false)
			require.Error(t, err)
			if tc != "unchanged_token" {
				require.Zero(t, executor.refreshCalls)
			}
		})
	}
}

func TestOpenAIOAuthRecovery_HTTPForwardAndPassthrough(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		t.Run(map[bool]string{false: "forward", true: "passthrough"}[passthrough], func(t *testing.T) {
			account, repo, provider, executor := newOpenAIRecoveryFixture()
			account.Extra = map[string]any{"openai_passthrough": passthrough}
			upstream := &httpUpstreamRecorder{responses: []*http.Response{
				newJSONResponse(401, `{"error":{"code":"token_expired"}}`),
				newJSONResponse(200, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_test\",\"output\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n"),
			}}
			svc := &OpenAIGatewayService{accountRepo: repo, openAITokenProvider: provider,
				httpUpstream: upstream, cfg: &config.Config{}}
			body := []byte(`{"model":"gpt-5.4","input":"hi","stream":false}`)
			c := adaptiveProtocolTestContext("/v1/responses", body)
			_, err := svc.Forward(context.Background(), c, account, body)
			require.NoError(t, err)
			require.Len(t, upstream.requests, 2)
			require.Equal(t, "Bearer rejected", upstream.requests[0].Header.Get("Authorization"))
			require.Equal(t, "Bearer replacement", upstream.requests[1].Header.Get("Authorization"))
			require.Equal(t, upstream.bodies[0], upstream.bodies[1])
			require.Equal(t, 1, executor.refreshCalls)
		})
	}
}

func TestOpenAIOAuthRecovery_RevokedAfterRotation(t *testing.T) {
	account, repo, provider, executor := newOpenAIRecoveryFixture()
	upstream := &queuedHTTPUpstream{responses: []*http.Response{
		newJSONResponse(401, `{"error":{"code":"token_expired"}}`),
		newJSONResponse(401, `{"error":{"code":"token_revoked"}}`),
	}}
	result, err := recoveryTestService(repo, provider, upstream).RunTestBackground(context.Background(), account.ID, "gpt-5.4")
	require.NoError(t, err)
	require.Equal(t, "failed", result.Status)
	require.Equal(t, 1, executor.refreshCalls)
	require.Equal(t, "refresh-rotated", repo.account.GetOpenAIRefreshToken())
	require.Equal(t, StatusError, repo.account.Status)
	require.Contains(t, repo.account.ErrorMessage, openAIReauthorizationRequired)
}

func TestOpenAIOAuthRecovery_InvalidatesCachedToken(t *testing.T) {
	account, repo, provider, _ := newOpenAIRecoveryFixture()
	cache := &refreshAPICacheStub{lockResult: true}
	provider.tokenCache = cache
	provider.refreshAPI = NewOAuthRefreshAPI(repo, cache)
	fresh, err := provider.refreshRejectedAccessToken(context.Background(), account, "rejected", false)
	require.NoError(t, err)
	require.Equal(t, "replacement", fresh.GetOpenAIAccessToken())
	require.Positive(t, cache.deleteCalls)
	require.Equal(t, OpenAITokenCacheKey(account), cache.deleteKey)
	require.NoError(t, cache.deleteCtxErr)
}
