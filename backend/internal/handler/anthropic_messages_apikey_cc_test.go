package handler

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func anthropicAPIKeyCCTestConfig() *config.Config {
	return &config.Config{
		Security: config.SecurityConfig{
			URLAllowlist: config.URLAllowlistConfig{
				Enabled:           false,
				AllowInsecureHTTP: true,
			},
		},
	}
}

func anthropicAPIKeyCCTestAccount(chatCompletionsURL string) *service.Account {
	return &service.Account{
		ID:          22002,
		Name:        "anthropic-cc-handler-test",
		Platform:    service.PlatformAnthropic,
		Type:        service.AccountTypeAPIKeyChatCompletions,
		Status:      service.StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":              "sk-anth-cc-handler",
			"chat_completions_url": chatCompletionsURL,
		},
	}
}

func newAnthropicCCGatewayService(t *testing.T, httpUp service.HTTPUpstream) *service.GatewayService {
	t.Helper()
	cfg := anthropicAPIKeyCCTestConfig()
	return service.NewGatewayService(
		nil, nil, nil, nil, nil, nil, nil,
		nil, // cache
		cfg,
		nil, // schedulerSnapshot
		nil, // concurrencyService
		nil, // billingService
		nil, // rateLimitService
		nil, // billingCacheService
		nil, // identityService
		httpUp,
		nil, // deferredService
		nil, // claudeTokenProvider
		nil, // sessionLimitCache
		nil, // rpmCache
		nil, // digestStore
		nil, // settingService
		nil, // tlsFPProfileService
		nil, // channelService
		nil, // resolver
		nil, // balanceNotifyService
		nil, // userPlatformQuotaRepo
	)
}

// TestAPIKeyCCMessages_NonStream verifies that an Anthropic-platform
// apikey-chat-completions account correctly translates a /v1/messages request
// into a Chat Completions upstream call and converts the upstream SSE response
// back to an Anthropic JSON envelope for the client.
func TestAPIKeyCCMessages_NonStream(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstreamSSE := strings.Join([]string{
		`data: {"id":"chk","object":"chat.completion.chunk","model":"deepseek-chat","choices":[{"index":0,"delta":{"role":"assistant"}}]}`,
		``,
		`data: {"id":"chk","object":"chat.completion.chunk","model":"deepseek-chat","choices":[{"index":0,"delta":{"content":"hello"}}]}`,
		``,
		`data: {"id":"chk","object":"chat.completion.chunk","model":"deepseek-chat","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":2,"total_tokens":7}}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(upstreamSSE))
	}))
	defer upstream.Close()

	httpUp := &apikeyCCHTTPUpstream{client: upstream.Client()}
	svc := newAnthropicCCGatewayService(t, httpUp)
	account := anthropicAPIKeyCCTestAccount(upstream.URL + "/v1/chat/completions")

	body := []byte(`{"model":"claude-sonnet-4","max_tokens":64,"stream":false,"messages":[{"role":"user","content":"ping"}]}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("anthropic-beta", "tools-2024-05-16")

	parsed := &service.ParsedRequest{Body: service.NewRequestBodyRef(body), Model: "claude-sonnet-4", Stream: false}
	result, err := svc.ForwardAnthropicAsChatCompletions(c.Request.Context(), c, account, parsed)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.Stream)

	require.Equal(t, int32(1), atomic.LoadInt32(&httpUp.requestCount))
	require.Equal(t, upstream.URL+"/v1/chat/completions", httpUp.lastReqURL)
	require.Equal(t, "Bearer sk-anth-cc-handler", httpUp.lastAuthz)

	// anthropic-* headers must NOT leak to the OpenAI-compatible upstream.
	require.NotContains(t, strings.ToLower(string(httpUp.lastBody)), "anthropic-beta")

	clientBody := rec.Body.String()
	require.True(t, gjson.Valid(clientBody))
	require.Equal(t, "message", gjson.Get(clientBody, "type").String())
	require.Contains(t, clientBody, "hello")
}

func TestAPIKeyCCMessages_Stream(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstreamSSE := strings.Join([]string{
		`data: {"id":"chk","object":"chat.completion.chunk","model":"m","choices":[{"index":0,"delta":{"role":"assistant"}}]}`,
		``,
		`data: {"id":"chk","object":"chat.completion.chunk","model":"m","choices":[{"index":0,"delta":{"content":"yo"}}]}`,
		``,
		`data: {"id":"chk","object":"chat.completion.chunk","model":"m","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(upstreamSSE))
	}))
	defer upstream.Close()

	httpUp := &apikeyCCHTTPUpstream{client: upstream.Client()}
	svc := newAnthropicCCGatewayService(t, httpUp)
	account := anthropicAPIKeyCCTestAccount(upstream.URL + "/v1/chat/completions")

	body := []byte(`{"model":"claude-sonnet-4","max_tokens":64,"stream":true,"messages":[{"role":"user","content":"hi"}]}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	parsed := &service.ParsedRequest{Body: service.NewRequestBodyRef(body), Model: "claude-sonnet-4", Stream: true}
	result, err := svc.ForwardAnthropicAsChatCompletions(c.Request.Context(), c, account, parsed)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Stream)

	clientBody := rec.Body.String()
	require.Contains(t, clientBody, "event: message_start")
	require.Contains(t, clientBody, "event: content_block_delta")
	require.Contains(t, clientBody, "event: message_delta")
	require.Contains(t, clientBody, "event: message_stop")
}

// TestAPIKeyCCMessages_UpstreamError 验证可 failover 状态码（500/429/etc.）
// 不直接写客户端，而是返回 *UpstreamFailoverError 让 handler 端的 failover 循环处理
// （决定是否切换到下一个账号，或最终调用 handleFailoverExhausted 写出错误响应）。
func TestAPIKeyCCMessages_UpstreamError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"message":"boom"}}`))
	}))
	defer upstream.Close()

	httpUp := &apikeyCCHTTPUpstream{client: upstream.Client()}
	svc := newAnthropicCCGatewayService(t, httpUp)
	account := anthropicAPIKeyCCTestAccount(upstream.URL + "/v1/chat/completions")

	body := []byte(`{"model":"claude-sonnet-4","max_tokens":64,"stream":false,"messages":[{"role":"user","content":"ping"}]}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))

	parsed := &service.ParsedRequest{Body: service.NewRequestBodyRef(body), Model: "claude-sonnet-4", Stream: false}
	_, err := svc.ForwardAnthropicAsChatCompletions(c.Request.Context(), c, account, parsed)
	require.Error(t, err)

	var failoverErr *service.UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusInternalServerError, failoverErr.StatusCode)
	require.Contains(t, string(failoverErr.ResponseBody), "boom")

	// 客户端响应未被写出，由外层 failover loop 决定后续动作
	require.Equal(t, 0, rec.Body.Len())
}

// TestAPIKeyCCChatCompletions_ClaudePlatform verifies that on an Anthropic
// platform group, a client POST /v1/chat/completions against an
// apikey-chat-completions account performs a raw passthrough to the upstream
// CC URL (no protocol conversion).
func TestAPIKeyCCChatCompletions_ClaudePlatform(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl_anth_raw",
			"object":"chat.completion",
			"created":1,
			"model":"deepseek-chat",
			"choices":[{"index":0,"message":{"role":"assistant","content":"pong"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":3,"completion_tokens":1,"total_tokens":4}
		}`))
	}))
	defer upstream.Close()

	httpUp := &apikeyCCHTTPUpstream{client: upstream.Client()}
	svc := newAnthropicCCGatewayService(t, httpUp)
	account := anthropicAPIKeyCCTestAccount(upstream.URL + "/v1/chat/completions")

	body := []byte(`{"model":"deepseek-chat","messages":[{"role":"user","content":"ping"}],"stream":false}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	result, err := svc.ForwardClaudeChatCompletionsRaw(c.Request.Context(), c, account, body, &service.ParsedRequest{Body: service.NewRequestBodyRef(body)})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.Stream)

	require.Equal(t, upstream.URL+"/v1/chat/completions", httpUp.lastReqURL)
	require.Equal(t, "Bearer sk-anth-cc-handler", httpUp.lastAuthz)

	clientBody := rec.Body.String()
	require.True(t, gjson.Valid(clientBody))
	require.Equal(t, "chat.completion", gjson.Get(clientBody, "object").String())
	require.Contains(t, clientBody, "pong")
}
