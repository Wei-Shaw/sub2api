//go:build unit

package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func zhipuForceChatFallbackAccount() *Account {
	account := forceChatResponsesFallbackAccount()
	account.Platform = PlatformZhipu
	return account
}

func chatFallbackCompleteStream() string {
	return strings.Join([]string{
		`data: {"id":"chatcmpl_zhipu_ok","object":"chat.completion.chunk","model":"glm-5.3-flash","choices":[{"index":0,"delta":{"content":"ok"},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl_zhipu_ok","object":"chat.completion.chunk","model":"glm-5.3-flash","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`,
		"",
		`data: [DONE]`,
		"",
	}, "\n")
}

func newZhipuChatFallbackService(defaultMaxTokens int) (*OpenAIGatewayService, *httpUpstreamRecorder) {
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid_zhipu_default"}},
		Body:       io.NopCloser(strings.NewReader(chatFallbackCompleteStream())),
	}}
	cfg := rawChatCompletionsTestConfig()
	cfg.Gateway.CNProviders.ZhipuDefaultMaxTokens = defaultMaxTokens
	return &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream}, upstream
}

func newChatFallbackTestContext(t *testing.T, body []byte) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return c, rec
}

// Codex 走 /v1/responses 且未携带 max_output_tokens 时，智谱上游默认按 8192
// 截断长思考。网关桥接到 /v1/chat/completions 时必须为智谱账号注入配置的
// max_tokens，让上游按显式上限截断而不是默认 8192。
func TestForwardResponses_ChatFallbackZhipuInjectsDefaultMaxTokens(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"glm-5.3-flash","input":"write a long answer","stream":true}`)
	c, rec := newChatFallbackTestContext(t, body)

	svc, upstream := newZhipuChatFallbackService(32768)
	result, err := svc.Forward(context.Background(), c, zhipuForceChatFallbackAccount(), body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, int64(32768), gjson.GetBytes(upstream.lastBody, "max_tokens").Int())
	require.False(t, gjson.GetBytes(upstream.lastBody, "max_completion_tokens").Exists())
	require.Contains(t, rec.Body.String(), "event: response.completed")
}

// 客户端显式给出 max_output_tokens 时保持原样透传为 max_completion_tokens，
// 不覆盖、不额外注入 max_tokens。
func TestForwardResponses_ChatFallbackZhipuKeepsClientMaxOutputTokens(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"glm-5.3-flash","input":"hi","stream":true,"max_output_tokens":1234}`)
	c, _ := newChatFallbackTestContext(t, body)

	svc, upstream := newZhipuChatFallbackService(32768)
	result, err := svc.Forward(context.Background(), c, zhipuForceChatFallbackAccount(), body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, int64(1234), gjson.GetBytes(upstream.lastBody, "max_completion_tokens").Int())
	require.False(t, gjson.GetBytes(upstream.lastBody, "max_tokens").Exists())
}

// 注入只作用于智谱账号，且默认值配置为 0 时关闭：其他平台与显式关闭都不应
// 修改上游请求的输出上限字段。
func TestForwardResponses_ChatFallbackMaxTokensInjectionScope(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := []struct {
		name    string
		account *Account
		def     int
	}{
		{"openai platform untouched", forceChatResponsesFallbackAccount(), 32768},
		{"zhipu disabled by zero", zhipuForceChatFallbackAccount(), 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(`{"model":"glm-5.3-flash","input":"hi","stream":true}`)
			c, _ := newChatFallbackTestContext(t, body)

			svc, upstream := newZhipuChatFallbackService(tc.def)
			result, err := svc.Forward(context.Background(), c, tc.account, body)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.False(t, gjson.GetBytes(upstream.lastBody, "max_tokens").Exists())
			require.False(t, gjson.GetBytes(upstream.lastBody, "max_completion_tokens").Exists())
		})
	}
}
