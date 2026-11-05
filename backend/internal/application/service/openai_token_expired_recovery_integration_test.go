//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/Wei-Shaw/sub2api/internal/shared/openai"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type tokenExpiredRecoveryOAuthClient struct {
	refreshCalls int32
}

func (c *tokenExpiredRecoveryOAuthClient) ExchangeCode(context.Context, string, string, string, string, string) (*openai.TokenResponse, error) {
	return nil, nil
}

func (c *tokenExpiredRecoveryOAuthClient) RefreshToken(context.Context, string, string) (*openai.TokenResponse, error) {
	return c.RefreshTokenWithClientID(context.Background(), "", "", "")
}

func (c *tokenExpiredRecoveryOAuthClient) RefreshTokenWithClientID(context.Context, string, string, string) (*openai.TokenResponse, error) {
	atomic.AddInt32(&c.refreshCalls, 1)
	return &openai.TokenResponse{
		AccessToken:  "refreshed-access-token",
		RefreshToken: "refreshed-refresh-token",
		ExpiresIn:    3600,
	}, nil
}

func runOpenAIGatewayServiceAuthRecovery(t *testing.T, errorBody string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		{
			StatusCode: http.StatusUnauthorized,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(errorBody)),
		},
		{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"id":"resp-after-refresh","model":"gpt-5.5","status":"completed","usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`)),
		},
	}}
	account := &Account{
		ID:          903,
		Name:        "codex-oauth-token-expired",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":       "expired-access-token",
			"refresh_token":      "refresh-token",
			"expires_at":         time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339),
			"chatgpt_account_id": "chatgpt-account-903",
			"base_url":           "https://example.invalid",
		},
	}
	repo := &refreshAPIAccountRepo{account: account}
	cache := newOpenAITokenCacheStub()
	oauthClient := &tokenExpiredRecoveryOAuthClient{}
	oauthService := NewOpenAIOAuthService(nil, oauthClient)
	defer oauthService.Stop()
	tokenProvider := NewOpenAITokenProvider(repo, cache, oauthService)
	tokenProvider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), NewOpenAITokenRefresher(oauthService, repo))

	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	svc := &OpenAIGatewayService{
		cfg:                 cfg,
		httpUpstream:        upstream,
		openAITokenProvider: tokenProvider,
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("Content-Type", "application/json")

	result, err := svc.Forward(context.Background(), c, account, []byte(`{"model":"gpt-5.5","stream":false,"input":"hello"}`))

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "resp-after-refresh", result.ResponseID)
	require.Len(t, upstream.requests, 2)
	require.Equal(t, int32(1), atomic.LoadInt32(&oauthClient.refreshCalls))
	require.Equal(t, "refreshed-access-token", account.GetOpenAIAccessToken())
	require.Equal(t, "refreshed-access-token", cache.tokens[OpenAITokenCacheKey(account)])
}

func TestOpenAIGatewayService_TokenExpiredRefreshesAndReplaysSameAccount(t *testing.T) {
	runOpenAIGatewayServiceAuthRecovery(t, `{
  "detail": {"code": "token_expired", "message": "Your authentication token has expired. Please try refreshing it."},
  "error": {"code": "token_expired", "message": "Your authentication token has expired.", "param": null, "type": "invalid_request_error"},
  "status": 401
}`)
}

func TestOpenAIGatewayService_TeamGenericUnauthorizedRefreshesAndReplaysSameAccount(t *testing.T) {
	runOpenAIGatewayServiceAuthRecovery(t, `{"detail":"Unauthorized","status":401}`)
}
