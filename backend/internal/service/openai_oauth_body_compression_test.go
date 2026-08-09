package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/require"
)

func TestOpenAIHTTPBuildersCompressOnlyOAuthRequestBodies(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"gpt-5.6-codex","input":[{"role":"user","content":"compress this final request body"}]}`)
	svc := &OpenAIGatewayService{cfg: &config.Config{
		Security: config.SecurityConfig{
			URLAllowlist: config.URLAllowlistConfig{Enabled: false},
		},
	}}
	oauthAccount := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"chatgpt_account_id": "test-account",
		},
	}
	apiKeyAccount := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key": "test-api-key",
		},
	}

	tests := []struct {
		name       string
		compressed bool
		build      func(*gin.Context) (*http.Request, error)
	}{
		{
			name:       "ordinary OAuth",
			compressed: true,
			build: func(c *gin.Context) (*http.Request, error) {
				return svc.buildUpstreamRequest(context.Background(), c, oauthAccount, body, "oauth-token", false, "", true)
			},
		},
		{
			name:       "OAuth passthrough",
			compressed: true,
			build: func(c *gin.Context) (*http.Request, error) {
				return svc.buildUpstreamRequestOpenAIPassthrough(context.Background(), c, oauthAccount, body, "oauth-token")
			},
		},
		{
			name: "ordinary API key",
			build: func(c *gin.Context) (*http.Request, error) {
				return svc.buildUpstreamRequest(context.Background(), c, apiKeyAccount, body, "api-key-token", false, "", true)
			},
		},
		{
			name: "API key passthrough",
			build: func(c *gin.Context) (*http.Request, error) {
				return svc.buildUpstreamRequestOpenAIPassthrough(context.Background(), c, apiKeyAccount, body, "api-key-token")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))

			req, err := tt.build(c)
			require.NoError(t, err)
			defer req.Body.Close()

			encodedBody, err := io.ReadAll(req.Body)
			require.NoError(t, err)
			require.Equal(t, int64(len(encodedBody)), req.ContentLength)

			if !tt.compressed {
				require.Empty(t, req.Header.Get("content-encoding"))
				require.Equal(t, body, encodedBody)
				return
			}

			require.Equal(t, "zstd", req.Header.Get("content-encoding"))
			decoder, err := zstd.NewReader(nil)
			require.NoError(t, err)
			decompressedBody, err := decoder.DecodeAll(encodedBody, nil)
			decoder.Close()
			require.NoError(t, err)
			require.Equal(t, body, decompressedBody)
		})
	}
}
