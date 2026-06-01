package handler

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// apikeyCCHTTPUpstream 是一个轻量 HTTPUpstream 实现，把请求直接发给一个
// 由测试启动的 httptest.Server。专用于 apikey-chat-completions 类型账号的
// handler 集成 smoke test：验证 service 层在该账号类型下确实把请求送到
// account.GetOpenAIChatCompletionsURL() 配置的 URL，且 body/header 正确。
type apikeyCCHTTPUpstream struct {
	client       *http.Client
	requestCount int32
	lastReqURL   string
	lastAuthz    string
	lastAccept   string
	lastBody     []byte
}

func (u *apikeyCCHTTPUpstream) Do(req *http.Request, proxyURL string, accountID int64, accountConcurrency int) (*http.Response, error) {
	atomic.AddInt32(&u.requestCount, 1)
	u.lastReqURL = req.URL.String()
	u.lastAuthz = req.Header.Get("Authorization")
	u.lastAccept = req.Header.Get("Accept")
	if req.Body != nil {
		buf := new(bytes.Buffer)
		_, _ = buf.ReadFrom(req.Body)
		u.lastBody = buf.Bytes()
		_ = req.Body.Close()
		req.Body = http.NoBody
		req.Body = newReusableBody(u.lastBody)
		req.ContentLength = int64(len(u.lastBody))
	}
	return u.client.Do(req)
}

func (u *apikeyCCHTTPUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, profile *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, accountConcurrency)
}

func newReusableBody(b []byte) *reusableBody {
	return &reusableBody{Reader: bytes.NewReader(b)}
}

type reusableBody struct {
	*bytes.Reader
}

func (r *reusableBody) Close() error { return nil }

func apikeyCCTestAccount(chatCompletionsURL string) *service.Account {
	return &service.Account{
		ID:          11001,
		Name:        "apikey-cc-handler-test",
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeAPIKeyChatCompletions,
		Status:      service.StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":              "sk-handler-cc-test",
			"chat_completions_url": chatCompletionsURL,
		},
	}
}

func apikeyCCTestConfig() *config.Config {
	return &config.Config{
		Security: config.SecurityConfig{
			URLAllowlist: config.URLAllowlistConfig{
				Enabled:           false,
				AllowInsecureHTTP: true,
			},
		},
	}
}

// TestAPIKeyCCChatCompletions_NonStream 验证 apikey-chat-completions 账号在
// /v1/chat/completions 路径下的非流式转发：上游 URL 必须等于
// account.GetOpenAIChatCompletionsURL()，Authorization 必须使用账号 api_key，
// 客户端能拿到合法的 ChatCompletions JSON。
func TestAPIKeyCCChatCompletions_NonStream(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl_apikey_cc_1",
			"object":"chat.completion",
			"created":1,
			"model":"gpt-4o",
			"choices":[{"index":0,"message":{"role":"assistant","content":"pong"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":3,"completion_tokens":1,"total_tokens":4}
		}`))
	}))
	defer upstream.Close()

	httpUp := &apikeyCCHTTPUpstream{client: upstream.Client()}
	cfg := apikeyCCTestConfig()

	// 通过 NewOpenAIGatewayService 构造完整的 service，但仅注入测试需要的字段。
	svc := service.NewOpenAIGatewayService(
		nil, nil, nil, nil, nil, nil, nil,
		cfg,
		nil, nil,
		service.NewBillingService(cfg, nil),
		nil, nil,
		httpUp,
		&service.DeferredService{},
		nil, nil, nil, nil, nil,
		nil,
	)

	account := apikeyCCTestAccount(upstream.URL + "/v1/chat/completions")

	body := []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"ping"}],"stream":false}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	result, err := svc.ForwardAsChatCompletions(c.Request.Context(), c, account, body, "", "")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.Stream)

	// 上游 URL 与认证头
	require.Equal(t, int32(1), atomic.LoadInt32(&httpUp.requestCount), "应只调用一次上游")
	require.Equal(t, upstream.URL+"/v1/chat/completions", httpUp.lastReqURL)
	require.Equal(t, "Bearer sk-handler-cc-test", httpUp.lastAuthz)
	require.Equal(t, "application/json", httpUp.lastAccept)

	// 上游收到的 body 必须仍是 ChatCompletions 格式（含 messages，不含 input）
	require.Contains(t, string(httpUp.lastBody), `"messages"`)
	require.NotContains(t, string(httpUp.lastBody), `"input":[`)

	// 客户端响应是合法 ChatCompletions JSON
	clientBody := rec.Body.String()
	require.True(t, gjson.Valid(clientBody), "客户端响应必须是合法 JSON: %s", clientBody)
	require.Equal(t, "chat.completion", gjson.Get(clientBody, "object").String())
	require.Contains(t, clientBody, "pong")
}

// TestAPIKeyCCChatCompletions_Stream 验证 apikey-chat-completions 账号在
// /v1/chat/completions 路径下的流式转发：上游 stream_options.include_usage
// 必被网关强制打开，客户端能收到 SSE 事件并以 [DONE] 结束。
func TestAPIKeyCCChatCompletions_Stream(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstreamSSE := strings.Join([]string{
		`data: {"id":"chatcmpl_stream","object":"chat.completion.chunk","created":1,"model":"gpt-4o","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl_stream","object":"chat.completion.chunk","created":1,"model":"gpt-4o","choices":[{"index":0,"delta":{"content":"hi"},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl_stream","object":"chat.completion.chunk","created":1,"model":"gpt-4o","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(upstreamSSE))
	}))
	defer upstream.Close()

	httpUp := &apikeyCCHTTPUpstream{client: upstream.Client()}
	cfg := apikeyCCTestConfig()

	svc := service.NewOpenAIGatewayService(
		nil, nil, nil, nil, nil, nil, nil,
		cfg,
		nil, nil,
		service.NewBillingService(cfg, nil),
		nil, nil,
		httpUp,
		&service.DeferredService{},
		nil, nil, nil, nil, nil,
		nil,
	)

	account := apikeyCCTestAccount(upstream.URL + "/v1/chat/completions")

	body := []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}],"stream":true}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	result, err := svc.ForwardAsChatCompletions(c.Request.Context(), c, account, body, "", "")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Stream)

	// 上游 Accept: text/event-stream
	require.Equal(t, "text/event-stream", httpUp.lastAccept)
	require.Equal(t, upstream.URL+"/v1/chat/completions", httpUp.lastReqURL)
	require.Equal(t, "Bearer sk-handler-cc-test", httpUp.lastAuthz)

	// 上游收到的 stream=true 且 stream_options.include_usage 被网关强制打开
	require.True(t, gjson.GetBytes(httpUp.lastBody, "stream").Bool())
	require.True(t, gjson.GetBytes(httpUp.lastBody, "stream_options.include_usage").Bool())

	// 客户端收到 SSE 事件，含 chunk 与终止符
	clientBody := rec.Body.String()
	require.Contains(t, clientBody, `"object":"chat.completion.chunk"`)
	require.Contains(t, clientBody, "data: [DONE]")
}
