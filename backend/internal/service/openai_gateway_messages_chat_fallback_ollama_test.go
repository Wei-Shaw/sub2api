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

// runOllamaMessagesChatFallback 直接走 /v1/messages → raw CC 降级桥
// forwardAnthropicViaRawChatCompletions（与 messages 桥既有测试同风格，直接调桥函数）。
func runOllamaMessagesChatFallback(t *testing.T, account *Account, body []byte, upstreamBody string, stream bool) (*httptest.ResponseRecorder, *httpUpstreamRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	contentType := "application/json"
	if stream {
		contentType = "text/event-stream"
	}
	upstreamResp := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{contentType},
			"x-request-id": []string{"rid_ollama_messages_fallback"},
		},
		Body: io.NopCloser(strings.NewReader(upstreamBody)),
	}
	upstream := &httpUpstreamRecorder{resp: upstreamResp}
	svc := &OpenAIGatewayService{
		cfg:          rawChatCompletionsTestConfig(),
		httpUpstream: upstream,
	}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	result, err := svc.forwardAnthropicViaRawChatCompletions(context.Background(), c, account, body, "")
	require.NoError(t, err)
	require.NotNil(t, result)
	return rec, upstream
}

func TestMessagesChatFallback_OllamaDeepSeek_ClampMaxTokens(t *testing.T) {
	account := ollamaUpstreamTestAccount(PlatformDeepseek, 701)

	t.Run("256000 clamps to 65535", func(t *testing.T) {
		body := []byte(`{"model":"deepseek-v4-flash","max_tokens":256000,"messages":[{"role":"user","content":"hello"}],"stream":false}`)
		upstreamJSON := `{"id":"chatcmpl_ollama","object":"chat.completion","model":"deepseek-v4-flash","choices":[{"index":0,"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":5,"total_tokens":8}}`
		_, upstream := runOllamaMessagesChatFallback(t, account, body, upstreamJSON, false)
		require.Equal(t, "deepseek-v4-flash", gjson.GetBytes(upstream.lastBody, "model").String())
		require.Equal(t, int64(65535), gjson.GetBytes(upstream.lastBody, "max_completion_tokens").Int())
	})

	t.Run("1000 stays 1000", func(t *testing.T) {
		body := []byte(`{"model":"deepseek-v4-flash","max_tokens":1000,"messages":[{"role":"user","content":"hello"}],"stream":false}`)
		upstreamJSON := `{"id":"chatcmpl_ollama","object":"chat.completion","model":"deepseek-v4-flash","choices":[{"index":0,"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":5,"total_tokens":8}}`
		_, upstream := runOllamaMessagesChatFallback(t, account, body, upstreamJSON, false)
		require.Equal(t, int64(1000), gjson.GetBytes(upstream.lastBody, "max_completion_tokens").Int())
	})
}

// TestMessagesChatFallback_OllamaDeepSeek_ReasoningAliasStreaming 验证流式响应侧
// 归一化：上游 delta.reasoning / delta.thinking 与官方形态 delta.reasoning_content
// 在归一化之后产出完全相同的 thinking 输出（同一转换路径跑多个输入对比）。
func TestMessagesChatFallback_OllamaDeepSeek_ReasoningAliasStreaming(t *testing.T) {
	account := ollamaUpstreamTestAccount(PlatformDeepseek, 702)
	body := []byte(`{"model":"deepseek-v4-flash","max_tokens":1000,"messages":[{"role":"user","content":"hello"}],"stream":true}`)

	run := func(reasoningField string) string {
		upstreamBody := strings.Join([]string{
			`data: {"id":"chatcmpl_ollama","object":"chat.completion.chunk","model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
			"",
			`data: {"id":"chatcmpl_ollama","object":"chat.completion.chunk","model":"deepseek-v4-flash","choices":[{"index":0,"delta":{` + reasoningField + `},"finish_reason":null}]}`,
			"",
			`data: {"id":"chatcmpl_ollama","object":"chat.completion.chunk","model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"content":"final answer"},"finish_reason":null}]}`,
			"",
			`data: {"id":"chatcmpl_ollama","object":"chat.completion.chunk","model":"deepseek-v4-flash","choices":[],"usage":{"prompt_tokens":3,"completion_tokens":5,"total_tokens":8}}`,
			"",
			"data: [DONE]",
			"",
		}, "\n")
		rec, _ := runOllamaMessagesChatFallback(t, account, body, upstreamBody, true)
		return rec.Body.String()
	}

	baseline := run(`"reasoning_content":"abc"`)
	for _, tc := range []struct {
		name  string
		field string
	}{
		{name: "delta.reasoning alias", field: `"reasoning":"abc"`},
		{name: "delta.thinking alias", field: `"thinking":"abc"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := run(tc.field)
			require.Equal(t, baseline, out, "归一化后输出必须与 reasoning_content 基线完全一致")
			require.Contains(t, out, "event: content_block_start")
			require.Contains(t, out, `"type":"thinking"`)
			require.Contains(t, out, `"thinking":"abc"`)
			require.Contains(t, out, `"type":"text"`)
			require.Contains(t, out, `"text":"final answer"`)
		})
	}
}

func TestMessagesChatFallback_OllamaDeepSeek_ThinkingAliasNonStreaming(t *testing.T) {
	account := ollamaUpstreamTestAccount(PlatformDeepseek, 703)
	body := []byte(`{"model":"deepseek-v4-flash","max_tokens":1000,"messages":[{"role":"user","content":"hello"}],"stream":false}`)
	upstreamJSON := `{"id":"chatcmpl_ollama","object":"chat.completion","model":"deepseek-v4-flash","choices":[{"index":0,"message":{"role":"assistant","thinking":"abc","content":"final answer"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":5,"total_tokens":8}}`
	rec, _ := runOllamaMessagesChatFallback(t, account, body, upstreamJSON, false)
	require.Equal(t, "thinking", gjson.Get(rec.Body.String(), "content.0.type").String())
	require.Equal(t, "abc", gjson.Get(rec.Body.String(), "content.0.thinking").String())
	require.Equal(t, "text", gjson.Get(rec.Body.String(), "content.1.type").String())
	require.Equal(t, "final answer", gjson.Get(rec.Body.String(), "content.1.text").String())
}

func TestMessagesChatFallback_OfficialDeepSeek_Untouched(t *testing.T) {
	official := officialDeepSeekTestAccount(704)
	body := []byte(`{"model":"deepseek-v4-flash","max_tokens":256000,"messages":[{"role":"user","content":"hello"}],"stream":false}`)
	// thinking 不归一化：转换器只认 reasoning_content/reasoning，thinking 被丢弃，
	// 输出不出现 thinking 块（与不挂钩子的原始路径行为一致）。
	upstreamJSON := `{"id":"chatcmpl_official","object":"chat.completion","model":"deepseek-v4-flash","choices":[{"index":0,"message":{"role":"assistant","thinking":"abc","content":"final answer"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":5,"total_tokens":8}}`
	rec, upstream := runOllamaMessagesChatFallback(t, official, body, upstreamJSON, false)
	require.Equal(t, int64(256000), gjson.GetBytes(upstream.lastBody, "max_completion_tokens").Int())
	require.NotContains(t, rec.Body.String(), `"type":"thinking"`)
	require.Equal(t, "final answer", gjson.Get(rec.Body.String(), "content.0.text").String())
}

func TestMessagesChatFallback_OllamaGLM_Untouched(t *testing.T) {
	account := ollamaUpstreamTestAccount(PlatformDeepseek, 705)
	body := []byte(`{"model":"glm-5.3-flash","max_tokens":256000,"messages":[{"role":"user","content":"hello"}],"stream":false}`)
	upstreamJSON := `{"id":"chatcmpl_ollama","object":"chat.completion","model":"glm-5.3-flash","choices":[{"index":0,"message":{"role":"assistant","thinking":"abc","content":"final answer"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":5,"total_tokens":8}}`
	rec, upstream := runOllamaMessagesChatFallback(t, account, body, upstreamJSON, false)
	require.Equal(t, int64(256000), gjson.GetBytes(upstream.lastBody, "max_completion_tokens").Int())
	require.NotContains(t, rec.Body.String(), `"type":"thinking"`)
	require.Equal(t, "final answer", gjson.Get(rec.Body.String(), "content.0.text").String())
}
