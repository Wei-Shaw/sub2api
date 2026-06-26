package service

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAccount_GetOpenAICustomHeadersSanitizesConfiguredHeaders(t *testing.T) {
	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Extra: map[string]any{
			openAICustomHeadersExtraKey: map[string]any{
				" X-Trace-Id ":        " trace-1 ",
				"CF-Access-Client-Id": "client-id",
				"OpenAI-Organization": "org_123",
				"Authorization":       "Bearer attacker",
				"content-type":        "text/plain",
				"session_id":          "shared-session",
				"Bad Header":          "bad",
				"X-Injected":          "line-1\nline-2",
				"X-Empty":             "",
				"X-Unsupported-Value": map[string]any{"nested": true},
			},
		},
	}

	headers := account.GetOpenAICustomHeaders()

	require.Equal(t, "trace-1", headers.Get("X-Trace-Id"))
	require.Equal(t, "client-id", headers.Get("CF-Access-Client-Id"))
	require.Equal(t, "org_123", headers.Get("OpenAI-Organization"))
	require.Equal(t, "Bearer attacker", headers.Get("Authorization"))
	require.Empty(t, headers.Get("Content-Type"))
	require.Empty(t, headers.Get("Session_Id"))
	require.Empty(t, headers.Get("Bad Header"))
	require.Empty(t, headers.Get("X-Injected"))
	require.Empty(t, headers.Get("X-Empty"))
	require.Empty(t, headers.Get("X-Unsupported-Value"))
}

func TestAccount_GetOpenAICustomHeadersAcceptsRowsFormat(t *testing.T) {
	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			openAICustomHeadersExtraKey: []any{
				map[string]any{"name": "X-Row-Name", "value": "row-value"},
				map[string]any{"key": "X-Row-Key", "value": "key-value"},
				map[string]any{"header": "authorization", "value": "Bearer attacker"},
			},
		},
	}

	headers := account.GetOpenAICustomHeaders()

	require.Equal(t, "row-value", headers.Get("X-Row-Name"))
	require.Equal(t, "key-value", headers.Get("X-Row-Key"))
	require.Equal(t, "Bearer attacker", headers.Get("Authorization"))
}

func TestOpenAIBuildUpstreamRequestAppliesCustomHeadersWhileProtectingCoreHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader([]byte(`{"model":"gpt-5"}`)))
	c.Request.Header.Set("Authorization", "Bearer client")
	c.Request.Header.Set("session_id", "client-session")

	svc := &OpenAIGatewayService{cfg: &config.Config{
		Security: config.SecurityConfig{
			URLAllowlist: config.URLAllowlistConfig{Enabled: false},
		},
	}}
	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			openAICustomHeadersExtraKey: map[string]any{
				"X-Account-Header": "account-value",
				"Authorization":    "Bearer attacker",
				"session_id":       "custom-session",
				"Content-Type":     "text/plain",
			},
		},
	}

	req, err := svc.buildUpstreamRequest(c.Request.Context(), c, account, []byte(`{"model":"gpt-5"}`), "token", false, "prompt-cache", true)

	require.NoError(t, err)
	require.Equal(t, "account-value", req.Header.Get("X-Account-Header"))
	require.Equal(t, "Bearer attacker", req.Header.Get("Authorization"))
	require.NotEqual(t, "custom-session", req.Header.Get("Session_Id"))
	require.Equal(t, "application/json", req.Header.Get("Content-Type"))
}

func TestOpenAIBuildUpstreamRequestPassthroughAppliesCustomHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader([]byte(`{"model":"gpt-5"}`)))

	svc := &OpenAIGatewayService{cfg: &config.Config{
		Security: config.SecurityConfig{
			URLAllowlist: config.URLAllowlistConfig{Enabled: false},
		},
	}}
	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Extra: map[string]any{
			openAICustomHeadersExtraKey: map[string]any{
				"X-Account-Header": "account-value",
				"Authorization":    "Bearer attacker",
			},
		},
	}

	req, err := svc.buildUpstreamRequestOpenAIPassthrough(c.Request.Context(), c, account, []byte(`{"model":"gpt-5"}`), "token")

	require.NoError(t, err)
	require.Equal(t, "account-value", req.Header.Get("X-Account-Header"))
	require.Equal(t, "Bearer attacker", req.Header.Get("Authorization"))
}

func TestOpenAIWSHeadersApplyCustomHeadersWhileProtectingCoreHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "codex_cli_rs/0.125.0")
	c.Request.Header.Set("session_id", "client-session")

	svc := &OpenAIGatewayService{}
	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			openAICustomHeadersExtraKey: map[string]any{
				"X-Account-Header": "account-value",
				"Authorization":    "Bearer attacker",
				"OpenAI-Beta":      "attacker",
				"User-Agent":       "attacker",
			},
		},
	}

	headers, _ := svc.buildOpenAIWSHeaders(
		c,
		account,
		"token",
		OpenAIWSProtocolDecision{Transport: OpenAIUpstreamTransportResponsesWebsocketV2},
		true,
		"",
		"",
		"prompt-cache",
	)

	require.Equal(t, "account-value", headers.Get("X-Account-Header"))
	require.Equal(t, "Bearer attacker", headers.Get("Authorization"))
	require.Equal(t, openAIWSBetaV2Value, headers.Get("OpenAI-Beta"))
	require.NotEqual(t, "attacker", headers.Get("User-Agent"))
}

func TestOpenAITokenProviderSkipsOAuthTokenFlowWithCustomAuthorization(t *testing.T) {
	cache := &openAICustomHeaderTokenCache{token: "cached-token"}
	provider := NewOpenAITokenProvider(nil, cache, nil)
	account := &Account{
		ID:       10,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			openAICustomHeadersExtraKey: map[string]any{
				"Authorization": "Bearer upstream-token",
			},
		},
	}

	token, err := provider.GetAccessToken(context.Background(), account)

	require.NoError(t, err)
	require.Equal(t, openAICustomAuthorizationTokenPlaceholder, token)
	require.Zero(t, atomic.LoadInt32(&cache.getCalled))
	require.Zero(t, atomic.LoadInt32(&cache.lockCalled))
	require.Zero(t, atomic.LoadInt32(&cache.setCalled))
}

func TestOpenAIGatewayGetAccessTokenSkipsMissingOAuthTokenWithCustomAuthorization(t *testing.T) {
	svc := &OpenAIGatewayService{}
	account := &Account{
		ID:       11,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			openAICustomHeadersExtraKey: map[string]any{
				"authorization": "Bearer upstream-token",
			},
		},
	}

	token, tokenType, err := svc.GetAccessToken(context.Background(), account)

	require.NoError(t, err)
	require.Equal(t, openAICustomAuthorizationTokenPlaceholder, token)
	require.Equal(t, "oauth", tokenType)
}

type openAICustomHeaderTokenCache struct {
	token      string
	getCalled  int32
	setCalled  int32
	lockCalled int32
}

func (c *openAICustomHeaderTokenCache) GetAccessToken(context.Context, string) (string, error) {
	atomic.AddInt32(&c.getCalled, 1)
	return c.token, nil
}

func (c *openAICustomHeaderTokenCache) SetAccessToken(context.Context, string, string, time.Duration) error {
	atomic.AddInt32(&c.setCalled, 1)
	return nil
}

func (c *openAICustomHeaderTokenCache) DeleteAccessToken(context.Context, string) error {
	return nil
}

func (c *openAICustomHeaderTokenCache) AcquireRefreshLock(context.Context, string, time.Duration) (bool, error) {
	atomic.AddInt32(&c.lockCalled, 1)
	return true, nil
}

func (c *openAICustomHeaderTokenCache) ReleaseRefreshLock(context.Context, string) error {
	return nil
}
