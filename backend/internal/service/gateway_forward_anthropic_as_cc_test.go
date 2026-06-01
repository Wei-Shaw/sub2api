//go:build unit

package service

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// anthropicCCStubUpstream forwards Do/DoWithTLS to an httptest.Server's client,
// remembering the last request's URL / Authorization / body for assertion.
type anthropicCCStubUpstream struct {
	client       *http.Client
	requestCount int32
	lastURL      string
	lastAuthz    string
	lastAccept   string
	lastBody     []byte
	lastHeaders  http.Header
}

func (u *anthropicCCStubUpstream) Do(req *http.Request, proxyURL string, accountID int64, accountConcurrency int) (*http.Response, error) {
	atomic.AddInt32(&u.requestCount, 1)
	u.lastURL = req.URL.String()
	u.lastAuthz = req.Header.Get("Authorization")
	u.lastAccept = req.Header.Get("Accept")
	u.lastHeaders = req.Header.Clone()
	if req.Body != nil {
		buf := new(bytes.Buffer)
		_, _ = buf.ReadFrom(req.Body)
		u.lastBody = buf.Bytes()
		_ = req.Body.Close()
		req.Body = anthropicCCReusableBody(u.lastBody)
		req.ContentLength = int64(len(u.lastBody))
	}
	return u.client.Do(req)
}

func (u *anthropicCCStubUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, profile *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, accountConcurrency)
}

type anthropicCCBody struct {
	*bytes.Reader
}

func (r *anthropicCCBody) Close() error { return nil }

func anthropicCCReusableBody(b []byte) *anthropicCCBody {
	return &anthropicCCBody{Reader: bytes.NewReader(b)}
}

func anthropicCCTestConfig() *config.Config {
	return &config.Config{
		Security: config.SecurityConfig{
			URLAllowlist: config.URLAllowlistConfig{
				Enabled:           false,
				AllowInsecureHTTP: true,
			},
		},
	}
}

func anthropicCCTestAccount(chatCompletionsURL string) *Account {
	return &Account{
		ID:          22001,
		Name:        "anthropic-cc-svc-test",
		Platform:    PlatformAnthropic,
		Type:        AccountTypeAPIKeyChatCompletions,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":              "sk-anthropic-cc-test",
			"chat_completions_url": chatCompletionsURL,
		},
	}
}

func newAnthropicCCService(t *testing.T, httpUp HTTPUpstream) *GatewayService {
	t.Helper()
	cfg := anthropicCCTestConfig()
	svc := &GatewayService{
		cfg:          cfg,
		httpUpstream: httpUp,
	}
	return svc
}

func TestForwardAnthropicAsChatCompletions_NonStream(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstreamSSE := strings.Join([]string{
		`data: {"id":"chk1","object":"chat.completion.chunk","model":"deepseek-chat","choices":[{"index":0,"delta":{"role":"assistant"}}]}`,
		``,
		`data: {"id":"chk1","object":"chat.completion.chunk","model":"deepseek-chat","choices":[{"index":0,"delta":{"content":"hello"}}]}`,
		``,
		`data: {"id":"chk1","object":"chat.completion.chunk","model":"deepseek-chat","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":4,"completion_tokens":1,"total_tokens":5}}`,
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

	httpUp := &anthropicCCStubUpstream{client: upstream.Client()}
	svc := newAnthropicCCService(t, httpUp)
	account := anthropicCCTestAccount(upstream.URL + "/v1/chat/completions")

	body := []byte(`{"model":"claude-sonnet-4","max_tokens":64,"stream":false,"messages":[{"role":"user","content":"ping"}]}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("anthropic-beta", "tools-2024-05-16")
	c.Request.Header.Set("anthropic-version", "2023-06-01")

	parsed := &ParsedRequest{Body: NewRequestBodyRef(body), Model: "claude-sonnet-4", Stream: false}
	result, err := svc.ForwardAnthropicAsChatCompletions(c.Request.Context(), c, account, parsed)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.Stream)

	require.Equal(t, upstream.URL+"/v1/chat/completions", httpUp.lastURL)
	require.Equal(t, "Bearer sk-anthropic-cc-test", httpUp.lastAuthz)
	require.Equal(t, "text/event-stream", httpUp.lastAccept)

	// Header whitelist: anthropic-* MUST NOT leak.
	require.Empty(t, httpUp.lastHeaders.Get("anthropic-beta"))
	require.Empty(t, httpUp.lastHeaders.Get("anthropic-version"))
	require.Empty(t, httpUp.lastHeaders.Get("x-api-key"))

	// Upstream body must be ChatCompletions (messages, not Anthropic 'system' style).
	require.Contains(t, string(httpUp.lastBody), `"messages"`)
	require.True(t, gjson.GetBytes(httpUp.lastBody, "stream").Bool())
	require.True(t, gjson.GetBytes(httpUp.lastBody, "stream_options.include_usage").Bool())

	// Client got an Anthropic JSON response.
	clientBody := rec.Body.String()
	require.True(t, gjson.Valid(clientBody))
	require.Equal(t, "message", gjson.Get(clientBody, "type").String())
	require.Contains(t, clientBody, "hello")
}

func TestForwardAnthropicAsChatCompletions_Stream(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstreamSSE := strings.Join([]string{
		`data: {"id":"chk2","object":"chat.completion.chunk","model":"deepseek-chat","choices":[{"index":0,"delta":{"role":"assistant"}}]}`,
		``,
		`data: {"id":"chk2","object":"chat.completion.chunk","model":"deepseek-chat","choices":[{"index":0,"delta":{"content":"Hi"}}]}`,
		``,
		`data: {"id":"chk2","object":"chat.completion.chunk","model":"deepseek-chat","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":1,"total_tokens":4}}`,
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

	httpUp := &anthropicCCStubUpstream{client: upstream.Client()}
	svc := newAnthropicCCService(t, httpUp)
	account := anthropicCCTestAccount(upstream.URL + "/v1/chat/completions")

	body := []byte(`{"model":"claude-sonnet-4","max_tokens":64,"stream":true,"messages":[{"role":"user","content":"hi"}]}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	parsed := &ParsedRequest{Body: NewRequestBodyRef(body), Model: "claude-sonnet-4", Stream: true}
	result, err := svc.ForwardAnthropicAsChatCompletions(c.Request.Context(), c, account, parsed)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Stream)

	clientBody := rec.Body.String()
	require.Contains(t, clientBody, "event: message_start")
	require.Contains(t, clientBody, "event: content_block_start")
	require.Contains(t, clientBody, "event: content_block_delta")
	require.Contains(t, clientBody, "event: message_delta")
	require.Contains(t, clientBody, "event: message_stop")
}

func TestForwardAnthropicAsChatCompletions_ToolUse(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstreamSSE := strings.Join([]string{
		`data: {"id":"chk3","object":"chat.completion.chunk","model":"m","choices":[{"index":0,"delta":{"role":"assistant"}}]}`,
		``,
		`data: {"id":"chk3","object":"chat.completion.chunk","model":"m","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_x","function":{"name":"lookup"}}]}}]}`,
		``,
		`data: {"id":"chk3","object":"chat.completion.chunk","model":"m","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"q\":\"x\"}"}}]}}]}`,
		``,
		`data: {"id":"chk3","object":"chat.completion.chunk","model":"m","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
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

	httpUp := &anthropicCCStubUpstream{client: upstream.Client()}
	svc := newAnthropicCCService(t, httpUp)
	account := anthropicCCTestAccount(upstream.URL + "/v1/chat/completions")

	body := []byte(`{"model":"claude-sonnet-4","max_tokens":64,"stream":true,"tools":[{"name":"lookup","input_schema":{"type":"object","properties":{"q":{"type":"string"}}}}],"messages":[{"role":"user","content":"please lookup"}]}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	parsed := &ParsedRequest{Body: NewRequestBodyRef(body), Model: "claude-sonnet-4", Stream: true}
	result, err := svc.ForwardAnthropicAsChatCompletions(c.Request.Context(), c, account, parsed)
	require.NoError(t, err)
	require.NotNil(t, result)

	clientBody := rec.Body.String()
	require.Contains(t, clientBody, "tool_use")
	require.Contains(t, clientBody, "lookup")
	require.Contains(t, clientBody, "input_json_delta")
}

// TestForwardAnthropicAsChatCompletions_FailoverOn429 验证可 failover 状态码（429）
// 不直接吐错给客户端，而是返回 *UpstreamFailoverError，让外层 handler 决定是否切账号。
func TestForwardAnthropicAsChatCompletions_FailoverOn429(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Cf-Ray", "abc123")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"rate limited"}}`))
	}))
	defer upstream.Close()

	httpUp := &anthropicCCStubUpstream{client: upstream.Client()}
	svc := newAnthropicCCService(t, httpUp)
	account := anthropicCCTestAccount(upstream.URL + "/v1/chat/completions")

	body := []byte(`{"model":"claude-sonnet-4","max_tokens":64,"stream":false,"messages":[{"role":"user","content":"ping"}]}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))

	parsed := &ParsedRequest{Body: NewRequestBodyRef(body), Model: "claude-sonnet-4", Stream: false}
	_, err := svc.ForwardAnthropicAsChatCompletions(c.Request.Context(), c, account, parsed)
	require.Error(t, err)

	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusTooManyRequests, failoverErr.StatusCode)
	require.Contains(t, string(failoverErr.ResponseBody), "rate limited")
	require.Equal(t, "abc123", failoverErr.ResponseHeaders.Get("X-Cf-Ray"))

	// 客户端响应未被写出，让外层 handler 决定后续动作
	require.Equal(t, 0, rec.Body.Len())
}

// TestForwardAnthropicAsChatCompletions_FailoverOn500 同样验证 5xx 走 failover。
func TestForwardAnthropicAsChatCompletions_FailoverOn500(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"message":"upstream busted"}}`))
	}))
	defer upstream.Close()

	httpUp := &anthropicCCStubUpstream{client: upstream.Client()}
	svc := newAnthropicCCService(t, httpUp)
	account := anthropicCCTestAccount(upstream.URL + "/v1/chat/completions")

	body := []byte(`{"model":"claude-sonnet-4","max_tokens":64,"stream":false,"messages":[{"role":"user","content":"ping"}]}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))

	parsed := &ParsedRequest{Body: NewRequestBodyRef(body), Model: "claude-sonnet-4", Stream: false}
	_, err := svc.ForwardAnthropicAsChatCompletions(c.Request.Context(), c, account, parsed)

	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusInternalServerError, failoverErr.StatusCode)
	require.Equal(t, 0, rec.Body.Len())
}

// TestForwardAnthropicAsChatCompletions_NoFailoverOn400 验证不可 failover 的 4xx
// （如 400/404/422）保持原有行为：写 Anthropic error envelope + 返回 plain error。
func TestForwardAnthropicAsChatCompletions_NoFailoverOn400(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"bad model name"}}`))
	}))
	defer upstream.Close()

	httpUp := &anthropicCCStubUpstream{client: upstream.Client()}
	svc := newAnthropicCCService(t, httpUp)
	account := anthropicCCTestAccount(upstream.URL + "/v1/chat/completions")

	body := []byte(`{"model":"claude-sonnet-4","max_tokens":64,"stream":false,"messages":[{"role":"user","content":"ping"}]}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))

	parsed := &ParsedRequest{Body: NewRequestBodyRef(body), Model: "claude-sonnet-4", Stream: false}
	_, err := svc.ForwardAnthropicAsChatCompletions(c.Request.Context(), c, account, parsed)
	require.Error(t, err)

	var failoverErr *UpstreamFailoverError
	require.False(t, errorIsFailover(err, &failoverErr), "400 must not propagate as UpstreamFailoverError")

	clientBody := rec.Body.String()
	require.True(t, gjson.Valid(clientBody))
	require.Equal(t, "error", gjson.Get(clientBody, "type").String())
	require.Equal(t, "invalid_request_error", gjson.Get(clientBody, "error.type").String())
}

// TestForwardAnthropicAsChatCompletions_OnUpstreamAcceptedFires 验证上游返回 2xx
// 之后立即触发 OnUpstreamAccepted 回调，便于 handler 提前释放用户串行锁。
func TestForwardAnthropicAsChatCompletions_OnUpstreamAcceptedFires(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstreamSSE := strings.Join([]string{
		`data: {"id":"chk1","object":"chat.completion.chunk","model":"deepseek-chat","choices":[{"index":0,"delta":{"role":"assistant"}}]}`,
		``,
		`data: {"id":"chk1","object":"chat.completion.chunk","model":"deepseek-chat","choices":[{"index":0,"delta":{"content":"ok"}}]}`,
		``,
		`data: {"id":"chk1","object":"chat.completion.chunk","model":"deepseek-chat","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":1,"total_tokens":4}}`,
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

	httpUp := &anthropicCCStubUpstream{client: upstream.Client()}
	svc := newAnthropicCCService(t, httpUp)
	account := anthropicCCTestAccount(upstream.URL + "/v1/chat/completions")

	body := []byte(`{"model":"claude-sonnet-4","max_tokens":64,"stream":false,"messages":[{"role":"user","content":"ping"}]}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))

	var acceptedCount int32
	parsed := &ParsedRequest{
		Body:   NewRequestBodyRef(body),
		Model:  "claude-sonnet-4",
		Stream: false,
		OnUpstreamAccepted: func() {
			atomic.AddInt32(&acceptedCount, 1)
		},
	}

	result, err := svc.ForwardAnthropicAsChatCompletions(c.Request.Context(), c, account, parsed)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, int32(1), atomic.LoadInt32(&acceptedCount))
}

// errorIsFailover 是 errors.As 的薄包装，避免重复使用 Go 1.20 ErrorAs 的笔误。
func errorIsFailover(err error, target **UpstreamFailoverError) bool {
	if err == nil {
		return false
	}
	for cur := err; cur != nil; {
		if cast, ok := cur.(*UpstreamFailoverError); ok {
			*target = cast
			return true
		}
		type unwrapper interface{ Unwrap() error }
		if u, ok := cur.(unwrapper); ok {
			cur = u.Unwrap()
			continue
		}
		break
	}
	return false
}

func TestForwardAnthropicAsChatCompletions_MissingURL(t *testing.T) {
	gin.SetMode(gin.TestMode)

	httpUp := &anthropicCCStubUpstream{client: http.DefaultClient}
	svc := newAnthropicCCService(t, httpUp)
	account := anthropicCCTestAccount("")

	body := []byte(`{"model":"claude-sonnet-4","max_tokens":64,"stream":false,"messages":[{"role":"user","content":"ping"}]}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))

	parsed := &ParsedRequest{Body: NewRequestBodyRef(body), Model: "claude-sonnet-4", Stream: false}
	_, err := svc.ForwardAnthropicAsChatCompletions(c.Request.Context(), c, account, parsed)
	require.Error(t, err)
	require.Contains(t, err.Error(), "chat_completions_url")
	require.Equal(t, int32(0), atomic.LoadInt32(&httpUp.requestCount))
}
