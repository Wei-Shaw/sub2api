package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIGatewayService_APIKeyPassthrough_InvalidEncryptedContentPreservesStructured400(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const upstreamBody = `{
		"error": {
			"message": "The encrypted content gAAAAA_fake could not be verified. Reason: Encrypted content could not be decrypted or parsed.",
			"type": "invalid_request_error",
			"param": "input",
			"code": "invalid_encrypted_content",
			"internal_token": "sk-upstream-secret"
		},
		"debug": {"upstream": "secret-upstream.example"}
	}`

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(nil))

	svc := &OpenAIGatewayService{
		cfg: &config.Config{Gateway: config.GatewayConfig{ForceCodexCLI: false}},
		httpUpstream: &httpUpstreamRecorder{resp: &http.Response{
			StatusCode: http.StatusBadRequest,
			Header: http.Header{
				"Content-Type":  []string{"application/json"},
				"Server":        []string{"secret-upstream-proxy"},
				"Set-Cookie":    []string{"admin_token=secret"},
				"X-Admin-Debug": []string{"internal-route"},
			},
			Body: io.NopCloser(strings.NewReader(upstreamBody)),
		}},
	}
	account := &Account{
		ID:          201,
		Name:        "azure-passthrough",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "https://secret-upstream.example/openai/v1",
		},
		Extra:       map[string]any{"openai_passthrough": true},
		Status:      StatusActive,
		Schedulable: true,
	}

	_, err := svc.Forward(context.Background(), c, account, []byte(`{"model":"gpt-5.4","store":false,"input":[{"type":"reasoning","encrypted_content":"x"}]}`))

	require.Error(t, err)
	require.Equal(t, http.StatusBadRequest, rec.Code)

	body := rec.Body.String()
	require.Equal(t, "invalid_request_error", gjson.Get(body, "error.type").String())
	require.Equal(t, "invalid_encrypted_content", gjson.Get(body, "error.code").String())
	require.Equal(t, "input", gjson.Get(body, "error.param").String())
	require.Contains(t, gjson.Get(body, "error.message").String(), "Encrypted content could not be decrypted or parsed.")
	require.NotContains(t, body, "sk-upstream-secret")
	require.NotContains(t, body, "secret-upstream.example")
	require.False(t, gjson.Get(body, "debug").Exists())
	require.False(t, gjson.Get(body, "error.internal_token").Exists())
	require.Empty(t, rec.Header().Values("Server"))
	require.Empty(t, rec.Header().Values("Set-Cookie"))
	require.Empty(t, rec.Header().Values("X-Admin-Debug"))
}

func TestOpenAIGatewayService_APIKeyPassthrough_InvalidEncryptedContentRedactsUpstreamHostAndGuardsNon400(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("redacts configured base_url host inside allowlisted 400", func(t *testing.T) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(nil))

		svc := &OpenAIGatewayService{
			cfg: &config.Config{Gateway: config.GatewayConfig{ForceCodexCLI: false}},
			httpUpstream: &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusBadRequest,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body: io.NopCloser(strings.NewReader(
					`{"error":{"message":"https://secret-upstream.example/v1?key=secret123 rejected encrypted content for sk-test-secret-key","type":"invalid_request_error","code":"invalid_encrypted_content","param":"input"}}`,
				)),
			}},
		}
		account := &Account{
			ID: 202, Name: "azure-redact", Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1,
			Credentials: map[string]any{"api_key": "sk-test-secret-key", "base_url": "https://secret-upstream.example/openai/v1"},
			Extra:       map[string]any{"openai_passthrough": true}, Status: StatusActive, Schedulable: true,
		}

		_, err := svc.Forward(context.Background(), c, account, []byte(`{"model":"gpt-5.4","input":"hi"}`))
		require.Error(t, err)
		require.Equal(t, http.StatusBadRequest, rec.Code)
		body := rec.Body.String()
		require.Equal(t, "invalid_encrypted_content", gjson.Get(body, "error.code").String())
		require.NotContains(t, body, "secret-upstream.example")
		require.NotContains(t, body, "secret123")
		require.NotContains(t, body, "sk-test-secret-key")
		require.Contains(t, gjson.Get(body, "error.message").String(), "[upstream]")
		require.Contains(t, gjson.Get(body, "error.message").String(), "[redacted]")
	})

	t.Run("non-400 status with invalid_encrypted_content stays sanitized", func(t *testing.T) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(nil))

		svc := &OpenAIGatewayService{
			cfg: &config.Config{Gateway: config.GatewayConfig{ForceCodexCLI: false}},
			httpUpstream: &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusForbidden,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body: io.NopCloser(strings.NewReader(
					`{"error":{"message":"secret-upstream.example denied","type":"invalid_request_error","code":"invalid_encrypted_content"}}`,
				)),
			}},
		}
		account := &Account{
			ID: 203, Name: "azure-403", Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1,
			Credentials: map[string]any{"api_key": "sk-test", "base_url": "https://secret-upstream.example"},
			Extra:       map[string]any{"openai_passthrough": true}, Status: StatusActive, Schedulable: true,
		}

		_, err := svc.Forward(context.Background(), c, account, []byte(`{"model":"gpt-5.4","input":"hi"}`))
		require.Error(t, err)
		require.Equal(t, http.StatusBadGateway, rec.Code)
		body := rec.Body.String()
		require.Equal(t, "upstream_error", gjson.Get(body, "error.type").String())
		require.Equal(t, "Upstream access denied", gjson.Get(body, "error.message").String())
		require.False(t, gjson.Get(body, "error.code").Exists())
		require.NotContains(t, body, "secret-upstream.example")
	})
}

func TestOpenAIGatewayService_APIKeyPassthrough_AppliesErrorPassthroughRules(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("passthrough_body=true preserves structured type/code/param and honors skip_monitoring", func(t *testing.T) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(nil))

		ruleSvc := &ErrorPassthroughService{}
		ruleSvc.setLocalCache([]*model.ErrorPassthroughRule{
			{
				ID:              1,
				Name:            "openai-400-passthrough",
				Enabled:         true,
				Priority:        1,
				ErrorCodes:      []int{http.StatusBadRequest},
				Keywords:        []string{"previous_response_not_found"},
				MatchMode:       model.MatchModeAny,
				Platforms:       []string{PlatformOpenAI},
				PassthroughCode: true,
				PassthroughBody: true,
				SkipMonitoring:  true,
			},
		})
		BindErrorPassthroughService(c, ruleSvc)

		svc := &OpenAIGatewayService{
			cfg: &config.Config{Gateway: config.GatewayConfig{ForceCodexCLI: false}},
			httpUpstream: &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusBadRequest,
				Header: http.Header{
					"Content-Type": []string{"application/json"},
					"Server":       []string{"internal-upstream"},
				},
				Body: io.NopCloser(strings.NewReader(
					`{"error":{"message":"Previous response not found.","type":"invalid_request_error","code":"previous_response_not_found","param":"previous_response_id","internal_secret":"drop-me"},"debug":{"x":1}}`,
				)),
			}},
		}
		account := &Account{
			ID: 204, Name: "openai-pt-rule", Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1,
			Credentials: map[string]any{"api_key": "sk-test", "base_url": "https://api.example.test"},
			Extra:       map[string]any{"openai_passthrough": true}, Status: StatusActive, Schedulable: true,
		}

		_, err := svc.Forward(context.Background(), c, account, []byte(`{"model":"gpt-5.4","input":"hi"}`))
		require.Error(t, err)
		require.Equal(t, http.StatusBadRequest, rec.Code)

		body := rec.Body.String()
		require.Equal(t, "invalid_request_error", gjson.Get(body, "error.type").String())
		require.Equal(t, "previous_response_not_found", gjson.Get(body, "error.code").String())
		require.Equal(t, "previous_response_id", gjson.Get(body, "error.param").String())
		require.Equal(t, "Previous response not found.", gjson.Get(body, "error.message").String())
		require.False(t, gjson.Get(body, "error.internal_secret").Exists())
		require.False(t, gjson.Get(body, "debug").Exists())
		require.Empty(t, rec.Header().Values("Server"))
		skipVal, ok := c.Get(OpsSkipPassthroughKey)
		require.True(t, ok)
		require.Equal(t, true, skipVal)
	})

	t.Run("custom message rule overrides invalid_encrypted_content default", func(t *testing.T) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(nil))

		customStatus := http.StatusUnprocessableEntity
		customMsg := "Custom encrypted content error"
		ruleSvc := &ErrorPassthroughService{}
		ruleSvc.setLocalCache([]*model.ErrorPassthroughRule{
			{
				ID:              2,
				Name:            "override-encrypted",
				Enabled:         true,
				Priority:        1,
				ErrorCodes:      []int{http.StatusBadRequest},
				Keywords:        []string{"invalid_encrypted_content"},
				MatchMode:       model.MatchModeAny,
				Platforms:       []string{PlatformOpenAI},
				PassthroughCode: false,
				ResponseCode:    &customStatus,
				PassthroughBody: false,
				CustomMessage:   &customMsg,
			},
		})
		BindErrorPassthroughService(c, ruleSvc)

		svc := &OpenAIGatewayService{
			cfg: &config.Config{Gateway: config.GatewayConfig{ForceCodexCLI: false}},
			httpUpstream: &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusBadRequest,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body: io.NopCloser(strings.NewReader(
					`{"error":{"message":"Encrypted content could not be decrypted","type":"invalid_request_error","code":"invalid_encrypted_content"}}`,
				)),
			}},
		}
		account := &Account{
			ID: 205, Name: "openai-pt-custom", Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1,
			Credentials: map[string]any{"api_key": "sk-test"},
			Extra:       map[string]any{"openai_passthrough": true}, Status: StatusActive, Schedulable: true,
		}

		_, err := svc.Forward(context.Background(), c, account, []byte(`{"model":"gpt-5.4","input":"hi"}`))
		require.Error(t, err)
		require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
		body := rec.Body.String()
		require.Equal(t, "upstream_error", gjson.Get(body, "error.type").String())
		require.Equal(t, customMsg, gjson.Get(body, "error.message").String())
		require.False(t, gjson.Get(body, "error.code").Exists())
	})
}

func TestOpenAIGatewayService_APIKeyPassthrough_ContextWindowPreservesCodeAndCompactSSEBridge(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("context_length_exceeded preserves error.code", func(t *testing.T) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(nil))

		svc := &OpenAIGatewayService{
			cfg: &config.Config{Gateway: config.GatewayConfig{ForceCodexCLI: false}},
			httpUpstream: &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusBadRequest,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body: io.NopCloser(strings.NewReader(
					`{"error":{"message":"Your input exceeds the context window of this model.","type":"invalid_request_error","code":"context_length_exceeded","param":"input"}}`,
				)),
			}},
		}
		account := &Account{
			ID: 206, Name: "openai-pt-ctx", Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1,
			Credentials: map[string]any{"api_key": "sk-test"},
			Extra:       map[string]any{"openai_passthrough": true}, Status: StatusActive, Schedulable: true,
		}

		_, err := svc.Forward(context.Background(), c, account, []byte(`{"model":"gpt-5.4","input":"hi"}`))
		require.Error(t, err)
		require.Equal(t, http.StatusBadRequest, rec.Code)
		body := rec.Body.String()
		require.Equal(t, "invalid_request_error", gjson.Get(body, "error.type").String())
		require.Equal(t, "context_length_exceeded", gjson.Get(body, "error.code").String())
		require.Equal(t, "input", gjson.Get(body, "error.param").String())
		require.Contains(t, gjson.Get(body, "error.message").String(), "exceeds the context window")
	})

	t.Run("invalid_encrypted_content after compact keepalive emits response.failed SSE", func(t *testing.T) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses/compact", bytes.NewReader(nil))
		MarkOpenAICompactClientStream(c)
		stop := StartOpenAICompactSSEKeepalive(c, keepaliveTestInterval)
		defer stop()
		waitForKeepaliveBeats()

		svc := &OpenAIGatewayService{
			cfg: &config.Config{Gateway: config.GatewayConfig{ForceCodexCLI: false}},
			httpUpstream: &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusBadRequest,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body: io.NopCloser(strings.NewReader(
					`{"error":{"message":"The encrypted content could not be verified.","type":"invalid_request_error","code":"invalid_encrypted_content"}}`,
				)),
			}},
		}
		account := &Account{
			ID: 207, Name: "openai-pt-compact", Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1,
			Credentials: map[string]any{"api_key": "sk-test"},
			Extra:       map[string]any{"openai_passthrough": true}, Status: StatusActive, Schedulable: true,
		}

		_, err := svc.Forward(context.Background(), c, account, []byte(`{"model":"gpt-5.4","input":"hi"}`))
		require.Error(t, err)
		require.Equal(t, http.StatusOK, rec.Code)
		require.Contains(t, rec.Result().Header.Get("Content-Type"), "text/event-stream")
		events := parseCompactBridgeSSE(t, stripKeepaliveComments(rec.Body.String()))
		require.Len(t, events, 1)
		require.Equal(t, "response.failed", events[0][0])
		require.Contains(t, gjson.Get(events[0][1], "response.error.message").String(), "could not be verified")
	})
}

var _ = time.Second
