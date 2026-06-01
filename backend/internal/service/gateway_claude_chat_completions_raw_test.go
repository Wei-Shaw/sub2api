//go:build unit

package service

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// claudeRawCCTestAccount 构造一个 Claude 平台 + apikey-chat-completions 账号，
// 上游指向给定的 CC URL。
func claudeRawCCTestAccount(chatCompletionsURL string) *Account {
	return &Account{
		ID:          22101,
		Name:        "claude-raw-cc-svc-test",
		Platform:    PlatformAnthropic,
		Type:        AccountTypeAPIKeyChatCompletions,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":              "sk-claude-raw-cc",
			"chat_completions_url": chatCompletionsURL,
		},
	}
}

// TestForwardClaudeChatCompletionsRaw_FailoverOn429 验证可 failover 状态码（429）
// 不直接吐错给客户端，而是返回 *UpstreamFailoverError，让 handler 决定是否切账号。
func TestForwardClaudeChatCompletionsRaw_FailoverOn429(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"rate limited"}}`))
	}))
	defer upstream.Close()

	httpUp := &anthropicCCStubUpstream{client: upstream.Client()}
	svc := newAnthropicCCService(t, httpUp)
	account := claudeRawCCTestAccount(upstream.URL + "/v1/chat/completions")

	body := []byte(`{"model":"deepseek-chat","messages":[{"role":"user","content":"ping"}],"stream":false}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))

	parsed := &ParsedRequest{Body: NewRequestBodyRef(body), Model: "deepseek-chat", Stream: false}
	_, err := svc.ForwardClaudeChatCompletionsRaw(c.Request.Context(), c, account, body, parsed)
	require.Error(t, err)

	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusTooManyRequests, failoverErr.StatusCode)
	require.Contains(t, string(failoverErr.ResponseBody), "rate limited")

	// 客户端响应未被写出
	require.Equal(t, 0, rec.Body.Len())
}

// TestForwardClaudeChatCompletionsRaw_NoFailoverOn400 验证不可 failover 的 4xx
// 仍然写出 OpenAI CC 格式的 error envelope（不是 Anthropic 格式）。
func TestForwardClaudeChatCompletionsRaw_NoFailoverOn400(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"bad model"}}`))
	}))
	defer upstream.Close()

	httpUp := &anthropicCCStubUpstream{client: upstream.Client()}
	svc := newAnthropicCCService(t, httpUp)
	account := claudeRawCCTestAccount(upstream.URL + "/v1/chat/completions")

	body := []byte(`{"model":"deepseek-chat","messages":[{"role":"user","content":"ping"}],"stream":false}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))

	parsed := &ParsedRequest{Body: NewRequestBodyRef(body), Model: "deepseek-chat", Stream: false}
	_, err := svc.ForwardClaudeChatCompletionsRaw(c.Request.Context(), c, account, body, parsed)
	require.Error(t, err)

	var failoverErr *UpstreamFailoverError
	require.False(t, errorIsFailover(err, &failoverErr), "400 must not propagate as UpstreamFailoverError")

	// CC error envelope: {"error":{"type":"upstream_error","message":"..."}}
	clientBody := rec.Body.String()
	require.Contains(t, clientBody, `"error"`)
	require.Contains(t, clientBody, `upstream_error`)
}

// TestForwardClaudeChatCompletionsRaw_OnUpstreamAcceptedFires 验证 2xx 后立即触发回调。
func TestForwardClaudeChatCompletionsRaw_OnUpstreamAcceptedFires(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"x","object":"chat.completion","model":"deepseek-chat","choices":[{"index":0,"message":{"role":"assistant","content":"pong"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`))
	}))
	defer upstream.Close()

	httpUp := &anthropicCCStubUpstream{client: upstream.Client()}
	svc := newAnthropicCCService(t, httpUp)
	account := claudeRawCCTestAccount(upstream.URL + "/v1/chat/completions")

	body := []byte(`{"model":"deepseek-chat","messages":[{"role":"user","content":"ping"}],"stream":false}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))

	var acceptedCount int32
	parsed := &ParsedRequest{
		Body:   NewRequestBodyRef(body),
		Model:  "deepseek-chat",
		Stream: false,
		OnUpstreamAccepted: func() {
			atomic.AddInt32(&acceptedCount, 1)
		},
	}

	result, err := svc.ForwardClaudeChatCompletionsRaw(c.Request.Context(), c, account, body, parsed)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, int32(1), atomic.LoadInt32(&acceptedCount))
}

// TestForwardClaudeChatCompletionsRaw_NilParsedSafe 验证 parsed=nil 时不 panic
// （兼容当前 CC handler 未注入回调的情况）。
func TestForwardClaudeChatCompletionsRaw_NilParsedSafe(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"x","object":"chat.completion","model":"deepseek-chat","choices":[{"index":0,"message":{"role":"assistant","content":"pong"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`))
	}))
	defer upstream.Close()

	httpUp := &anthropicCCStubUpstream{client: upstream.Client()}
	svc := newAnthropicCCService(t, httpUp)
	account := claudeRawCCTestAccount(upstream.URL + "/v1/chat/completions")

	body := []byte(`{"model":"deepseek-chat","messages":[{"role":"user","content":"ping"}],"stream":false}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))

	require.NotPanics(t, func() {
		_, _ = svc.ForwardClaudeChatCompletionsRaw(c.Request.Context(), c, account, body, nil)
	})
}
