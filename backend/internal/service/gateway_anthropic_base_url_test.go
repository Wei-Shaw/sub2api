//go:build unit

package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func gatewayAnthropicBaseURLAccount(apiBaseURLs map[string]any) *Account {
	return &Account{
		ID:       8074,
		Platform: PlatformDeepseek,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":       "test-key",
			"base_url":      "https://api.deepseek.com",
			"api_protocol":  APIProtocolAdaptive,
			"api_base_urls": apiBaseURLs,
		},
	}
}

func gatewayAnthropicBaseURLContext(t *testing.T) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	return c
}

func TestGatewayService_AdaptiveAnthropicBaseURLRouting(t *testing.T) {
	svc := &GatewayService{cfg: &config.Config{}}
	body := []byte(`{"model":"deepseek-chat","max_tokens":64,"messages":[{"role":"user","content":"hi"}]}`)

	t.Run("base present is used by messages and count_tokens", func(t *testing.T) {
		account := gatewayAnthropicBaseURLAccount(map[string]any{
			APIProtocolAnthropic:       "https://anthropic.example.com",
			APIProtocolChatCompletions: "https://chat.example.com",
		})
		c := gatewayAnthropicBaseURLContext(t)

		req, _, err := svc.buildUpstreamRequest(context.Background(), c, account, body, "test-key", "apikey", "deepseek-chat", false, false)
		require.NoError(t, err)
		require.Equal(t, "https://anthropic.example.com/v1/messages?beta=true", req.URL.String())

		countReq, _, err := svc.buildCountTokensRequest(context.Background(), c, account, body, "test-key", "apikey", "deepseek-chat", false)
		require.NoError(t, err)
		require.Equal(t, "https://anthropic.example.com/v1/messages/count_tokens?beta=true", countReq.URL.String())
	})

	t.Run("missing anthropic base uses provider fallback", func(t *testing.T) {
		account := gatewayAnthropicBaseURLAccount(map[string]any{
			APIProtocolChatCompletions: "https://chat.example.com",
		})
		req, _, err := svc.buildUpstreamRequest(context.Background(), gatewayAnthropicBaseURLContext(t), account, body, "test-key", "apikey", "deepseek-chat", false, false)
		require.NoError(t, err)
		require.Equal(t, "https://api.deepseek.com/anthropic/v1/messages?beta=true", req.URL.String())
	})

	t.Run("ordinary anthropic api key keeps base_url fallback", func(t *testing.T) {
		account := &Account{
			Platform: PlatformAnthropic,
			Type:     AccountTypeAPIKey,
			Credentials: map[string]any{
				"api_key":  "test-key",
				"base_url": "https://custom-anthropic.example.com",
			},
		}
		req, _, err := svc.buildUpstreamRequest(context.Background(), gatewayAnthropicBaseURLContext(t), account, body, "test-key", "apikey", "deepseek-chat", false, false)
		require.NoError(t, err)
		require.Equal(t, "https://custom-anthropic.example.com/v1/messages?beta=true", req.URL.String())
	})
}

func TestGatewayService_AnthropicPassthroughAndCountTokensUseSelectedBase(t *testing.T) {
	svc := &GatewayService{cfg: &config.Config{}}
	account := gatewayAnthropicBaseURLAccount(map[string]any{
		APIProtocolAnthropic: "https://ollama.com",
	})
	body := []byte(`{"model":"deepseek-v4-flash","max_tokens":256000,"messages":[{"role":"user","content":"hi"}]}`)
	c := gatewayAnthropicBaseURLContext(t)

	passthroughReq, wireBody, err := svc.buildUpstreamRequestAnthropicAPIKeyPassthrough(context.Background(), c, account, body, "test-key")
	require.NoError(t, err)
	require.Equal(t, "https://ollama.com/v1/messages?beta=true", passthroughReq.URL.String())
	require.Equal(t, "Bearer test-key", passthroughReq.Header.Get("Authorization"))
	require.Equal(t, int64(65535), gjson.GetBytes(wireBody, "max_tokens").Int())

	countPassthroughReq, err := svc.buildCountTokensRequestAnthropicAPIKeyPassthrough(context.Background(), c, account, body, "test-key")
	require.NoError(t, err)
	require.Equal(t, "https://ollama.com/v1/messages/count_tokens?beta=true", countPassthroughReq.URL.String())
	require.Equal(t, "Bearer test-key", countPassthroughReq.Header.Get("Authorization"))

	countReq, _, err := svc.buildCountTokensRequest(context.Background(), c, account, body, "test-key", "apikey", "deepseek-v4-flash", false)
	require.NoError(t, err)
	require.Equal(t, "https://ollama.com/v1/messages/count_tokens?beta=true", countReq.URL.String())
	require.Equal(t, "Bearer test-key", countReq.Header.Get("Authorization"))
}
