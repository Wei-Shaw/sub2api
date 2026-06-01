package handler

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func apikeyCCResponsesTestConfig() *config.Config {
	return &config.Config{
		Security: config.SecurityConfig{
			URLAllowlist: config.URLAllowlistConfig{
				Enabled:           false,
				AllowInsecureHTTP: true,
			},
		},
	}
}

// TestAPIKeyCCResponses_NonStream 验证 apikey-chat-completions 账号在
// /v1/responses 路径下的非流式协议转换：service 层会把 Responses 请求转成
// ChatCompletions 发给上游配置的 chat_completions_url，并把上游返回的
// ChatCompletions JSON 转回 Responses 形态返回给客户端。
func TestAPIKeyCCResponses_NonStream(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl_resp_apikey_cc",
			"object":"chat.completion",
			"created":1,
			"model":"gpt-4o",
			"choices":[{"index":0,"message":{"role":"assistant","content":"hello world"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":5,"completion_tokens":2,"total_tokens":7}
		}`))
	}))
	defer upstream.Close()

	httpUp := &apikeyCCHTTPUpstream{client: upstream.Client()}
	cfg := apikeyCCResponsesTestConfig()

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

	body := []byte(`{"model":"gpt-4o","input":"ping","stream":false}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	result, err := svc.ForwardResponsesAsChatCompletions(c.Request.Context(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.Stream)
	require.Equal(t, 5, result.Usage.InputTokens)
	require.Equal(t, 2, result.Usage.OutputTokens)

	// 上游 URL 必须是账号配置的 chat_completions_url
	require.Equal(t, upstream.URL+"/v1/chat/completions", httpUp.lastReqURL)
	require.Equal(t, "Bearer sk-handler-cc-test", httpUp.lastAuthz)
	require.Equal(t, "application/json", httpUp.lastAccept)

	// 发给上游的必须是 ChatCompletions（messages，不带 Responses 的 input）
	require.Contains(t, string(httpUp.lastBody), `"messages"`)
	require.NotContains(t, string(httpUp.lastBody), `"input":"ping"`)

	// 客户端响应应为 Responses API 形态 JSON
	clientBody := rec.Body.String()
	require.True(t, gjson.Valid(clientBody), "客户端响应必须是合法 JSON: %s", clientBody)
	require.Equal(t, "response", gjson.Get(clientBody, "object").String())
	require.Contains(t, clientBody, "hello world")
	require.Contains(t, clientBody, "output_text")
}

// TestAPIKeyCCResponses_Stream 验证 apikey-chat-completions 账号在
// /v1/responses 流式路径下的协议转换：上游 ChatCompletions SSE chunk 必须
// 被服务端转换为 Responses API SSE 事件序列（response.created /
// response.output_text.delta / response.completed / [DONE]）。
func TestAPIKeyCCResponses_Stream(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstreamSSE := strings.Join([]string{
		`data: {"id":"chatcmpl_resp_stream","object":"chat.completion.chunk","created":1,"model":"gpt-4o","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl_resp_stream","object":"chat.completion.chunk","created":1,"model":"gpt-4o","choices":[{"index":0,"delta":{"content":"Hi"},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl_resp_stream","object":"chat.completion.chunk","created":1,"model":"gpt-4o","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":4,"completion_tokens":1,"total_tokens":5}}`,
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
	cfg := apikeyCCResponsesTestConfig()

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

	body := []byte(`{"model":"gpt-4o","input":"hi","stream":true}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	result, err := svc.ForwardResponsesAsChatCompletions(c.Request.Context(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Stream)
	require.Equal(t, 4, result.Usage.InputTokens)
	require.Equal(t, 1, result.Usage.OutputTokens)

	// 上游 URL 与流式 Accept 头
	require.Equal(t, upstream.URL+"/v1/chat/completions", httpUp.lastReqURL)
	require.Equal(t, "text/event-stream", httpUp.lastAccept)
	require.Equal(t, "Bearer sk-handler-cc-test", httpUp.lastAuthz)

	// 客户端收到的必须是 Responses SSE 事件序列
	clientBody := rec.Body.String()
	require.Contains(t, clientBody, "event: response.created")
	require.Contains(t, clientBody, "event: response.output_item.added")
	require.Contains(t, clientBody, "event: response.content_part.added")
	require.Contains(t, clientBody, "event: response.output_text.delta")
	require.Contains(t, clientBody, "event: response.output_text.done")
	require.Contains(t, clientBody, "event: response.content_part.done")
	require.Contains(t, clientBody, "event: response.output_item.done")
	require.Contains(t, clientBody, "event: response.completed")
	require.Contains(t, clientBody, "data: [DONE]")
}
