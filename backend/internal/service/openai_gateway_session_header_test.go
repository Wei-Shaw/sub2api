package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAIAPIKeySessionHeadersForwarded(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &OpenAIGatewayService{cfg: &config.Config{
		Security: config.SecurityConfig{
			URLAllowlist: config.URLAllowlistConfig{Enabled: false},
		},
	}}
	account := &Account{
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"base_url": "https://upstream.example.com/v1"},
	}
	body := []byte(`{"model":"gpt-5","prompt_cache_key":"body-cache-key","input":"hello"}`)

	tests := []struct {
		name  string
		build func(*gin.Context) (*http.Request, error)
	}{
		{
			name: "normal responses",
			build: func(c *gin.Context) (*http.Request, error) {
				return svc.buildUpstreamRequest(context.Background(), c, account, body, "token", false, "body-cache-key", false)
			},
		},
		{
			name: "passthrough responses",
			build: func(c *gin.Context) (*http.Request, error) {
				return svc.buildUpstreamRequestOpenAIPassthrough(context.Background(), c, account, body, "token")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			c.Request.Header.Set("session-id", "client-hyphen-session")
			c.Request.Header.Set("session_id", "client-underscore-session")

			req, err := tt.build(c)
			require.NoError(t, err)
			t.Cleanup(func() { _ = req.Body.Close() })
			require.Equal(t, []string{"client-hyphen-session"}, req.Header.Values("session-id"))
			require.Equal(t, []string{"client-underscore-session"}, req.Header.Values("session_id"))
			forwardedBody, err := io.ReadAll(req.Body)
			require.NoError(t, err)
			require.Equal(t, body, forwardedBody)
		})
	}
}
