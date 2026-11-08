package service

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestNormalizeOpenAICompactRequestBodyPreservesServiceTier(t *testing.T) {
	body := []byte(`{
		"model":"gpt-5.6-sol",
		"input":[{"type":"message","role":"user","content":"hello"}],
		"service_tier":"priority",
		"prompt_cache_key":"compact-cache-key",
		"store":false,
		"stream":true
	}`)

	normalized, changed, err := normalizeOpenAICompactRequestBody(body)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "gpt-5.6-sol", gjson.GetBytes(normalized, "model").String())
	require.Equal(t, "priority", gjson.GetBytes(normalized, "service_tier").String())
	require.False(t, gjson.GetBytes(normalized, "prompt_cache_key").Exists())
	require.False(t, gjson.GetBytes(normalized, "store").Exists())
	require.False(t, gjson.GetBytes(normalized, "stream").Exists())
}

func TestOpenAIOAuthCompactHTTPBuildersUsePreservedServiceTierInRoutingHint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{
		"model":"gpt-5.6-sol",
		"input":[{"type":"message","role":"user","content":"hello"}],
		"service_tier":"priority",
		"stream":true
	}`)
	normalized, changed, err := normalizeOpenAICompactRequestBody(body)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "priority", gjson.GetBytes(normalized, "service_tier").String())

	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"chatgpt_account_id": "test-account",
		},
	}
	svc := &OpenAIGatewayService{}

	tests := []struct {
		name  string
		build func(*gin.Context) (*http.Request, error)
	}{
		{
			name: "ordinary",
			build: func(c *gin.Context) (*http.Request, error) {
				return svc.buildUpstreamRequest(
					context.Background(), c, account, normalized, "test-token",
					false, "", true,
				)
			},
		},
		{
			name: "passthrough",
			build: func(c *gin.Context) (*http.Request, error) {
				return svc.buildUpstreamRequestOpenAIPassthrough(
					context.Background(), c, account, normalized, "test-token",
				)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(
				http.MethodPost,
				"/v1/responses/compact",
				bytes.NewReader(normalized),
			)

			req, buildErr := tt.build(c)
			require.NoError(t, buildErr)
			require.Equal(
				t,
				"model=gpt-5.6-sol;tier=priority",
				req.Header.Get(openAICodexRoutingHintHeader),
			)
			require.Equal(t, "priority", gjson.GetBytes(normalized, "service_tier").String())
		})
	}
}
