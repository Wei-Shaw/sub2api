package service

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAIUpstreamRequestsScopeOpenCodeSession(t *testing.T) {
	gin.SetMode(gin.TestMode)

	builders := []struct {
		name           string
		forwardSession bool
		build          func(*OpenAIGatewayService, *gin.Context, *Account, []byte) (*http.Request, error)
	}{
		{
			name:           "normal responses",
			forwardSession: true,
			build: func(svc *OpenAIGatewayService, c *gin.Context, account *Account, body []byte) (*http.Request, error) {
				return svc.buildUpstreamRequest(c.Request.Context(), c, account, body, "upstream-token", false, "", false)
			},
		},
		{
			name:           "passthrough responses",
			forwardSession: true,
			build: func(svc *OpenAIGatewayService, c *gin.Context, account *Account, body []byte) (*http.Request, error) {
				return svc.buildUpstreamRequestOpenAIPassthrough(c.Request.Context(), c, account, body, "upstream-token")
			},
		},
		{
			name: "image generation",
			build: func(svc *OpenAIGatewayService, c *gin.Context, account *Account, body []byte) (*http.Request, error) {
				return svc.buildOpenAIImagesRequest(c.Request.Context(), c, account, body, "application/json", "upstream-token", openAIImagesGenerationsEndpoint)
			},
		},
		{
			name: "image edit",
			build: func(svc *OpenAIGatewayService, c *gin.Context, account *Account, body []byte) (*http.Request, error) {
				return svc.buildOpenAIImagesRequest(c.Request.Context(), c, account, body, "multipart/form-data", "upstream-token", openAIImagesEditsEndpoint)
			},
		},
	}

	for _, builder := range builders {
		t.Run(builder.name, func(t *testing.T) {
			for _, session := range []string{"opencode-session-123", ""} {
				name := "present"
				if session == "" {
					name = "absent"
				}
				t.Run(name, func(t *testing.T) {
					body := []byte(`{"model":"gpt-5","input":"hello"}`)
					rec := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(rec)
					c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
					c.Request.Header.Set("Authorization", "Bearer client-token")
					if session != "" {
						c.Request.Header.Set("X-OpenCode-Session", session)
					}

					svc := &OpenAIGatewayService{cfg: &config.Config{
						Security: config.SecurityConfig{
							URLAllowlist: config.URLAllowlistConfig{Enabled: false},
						},
					}}
					req, err := builder.build(svc, c, &Account{
						Platform: PlatformOpenAI,
						Type:     AccountTypeAPIKey,
					}, body)
					require.NoError(t, err)
					_, hasSession := req.Header[http.CanonicalHeaderKey("X-OpenCode-Session")]
					if session != "" && builder.forwardSession {
						require.True(t, hasSession)
						require.Equal(t, session, req.Header.Get("X-OpenCode-Session"))
					} else {
						require.False(t, hasSession)
					}
					require.Equal(t, "Bearer upstream-token", req.Header.Get("Authorization"))
					require.NotContains(t, req.Header.Get("Authorization"), "client-token")
				})
			}
		})
	}
}

func TestOpenAIResponsesOpenCodeSessionPreservesHeaderOverridePrecedence(t *testing.T) {
	gin.SetMode(gin.TestMode)

	builders := []struct {
		name  string
		build func(*OpenAIGatewayService, *gin.Context, *Account, []byte) (*http.Request, error)
	}{
		{
			name: "normal responses",
			build: func(svc *OpenAIGatewayService, c *gin.Context, account *Account, body []byte) (*http.Request, error) {
				return svc.buildUpstreamRequest(c.Request.Context(), c, account, body, "upstream-token", false, "", false)
			},
		},
		{
			name: "passthrough responses",
			build: func(svc *OpenAIGatewayService, c *gin.Context, account *Account, body []byte) (*http.Request, error) {
				return svc.buildUpstreamRequestOpenAIPassthrough(c.Request.Context(), c, account, body, "upstream-token")
			},
		},
	}

	for _, builder := range builders {
		t.Run(builder.name, func(t *testing.T) {
			body := []byte(`{"model":"gpt-5","input":"hello"}`)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
			c.Request.Header.Set("X-OpenCode-Session", "client-session")

			svc := &OpenAIGatewayService{cfg: &config.Config{
				Security: config.SecurityConfig{
					URLAllowlist: config.URLAllowlistConfig{Enabled: false},
				},
			}}
			req, err := builder.build(svc, c, &Account{
				Platform: PlatformOpenAI,
				Type:     AccountTypeAPIKey,
				Credentials: map[string]any{
					credKeyHeaderOverrideEnabled: true,
					credKeyHeaderOverrides: map[string]any{
						"x-opencode-session": "account-session",
					},
				},
			}, body)
			require.NoError(t, err)
			require.Equal(t, "account-session", getHeaderRaw(req.Header, "x-opencode-session"))
		})
	}
}
