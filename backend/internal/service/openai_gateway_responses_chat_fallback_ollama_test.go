//go:build unit

package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// runOllamaResponsesChatFallback 直接走 /v1/responses → raw CC 降级桥
// forwardResponsesViaRawChatCompletions（与 raw 路径测试同风格，直接调桥函数）。
func runOllamaResponsesChatFallback(t *testing.T, account *Account, body []byte, upstreamBody string, stream bool) (*httptest.ResponseRecorder, *httpUpstreamRecorder) {
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
			"x-request-id": []string{"rid_ollama_responses_fallback"},
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
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	result, err := svc.forwardResponsesViaRawChatCompletions(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	return rec, upstream
}

func TestResponsesChatFallback_OllamaDeepSeek_ClampMaxOutputTokens(t *testing.T) {
	account := ollamaUpstreamTestAccount(PlatformDeepseek, 601)

	t.Run("256000 clamps to 65535", func(t *testing.T) {
		body := []byte(`{"model":"deepseek-v4-flash","input":"hello","max_output_tokens":256000,"stream":false}`)
		upstreamJSON := `{"id":"chatcmpl_ollama","object":"chat.completion","model":"deepseek-v4-flash","choices":[{"index":0,"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":5,"total_tokens":8}}`
		_, upstream := runOllamaResponsesChatFallback(t, account, body, upstreamJSON, false)
		require.Equal(t, "deepseek-v4-flash", gjson.GetBytes(upstream.lastBody, "model").String())
		require.Equal(t, int64(65535), gjson.GetBytes(upstream.lastBody, "max_completion_tokens").Int())
	})

	t.Run("1000 stays 1000", func(t *testing.T) {
		body := []byte(`{"model":"deepseek-v4-flash","input":"hello","max_output_tokens":1000,"stream":false}`)
		upstreamJSON := `{"id":"chatcmpl_ollama","object":"chat.completion","model":"deepseek-v4-flash","choices":[{"index":0,"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":5,"total_tokens":8}}`
		_, upstream := runOllamaResponsesChatFallback(t, account, body, upstreamJSON, false)
		require.Equal(t, int64(1000), gjson.GetBytes(upstream.lastBody, "max_completion_tokens").Int())
	})
}

// TestResponsesChatFallback_OllamaDeepSeek_ReasoningAliasStreaming 验证流式响应侧
// 归一化：上游 delta.reasoning / delta.thinking 与官方形态 delta.reasoning_content
// 在归一化之后产出完全相同的 reasoning 输出（同一转换路径跑多个输入对比）。
// normalizeStreamIDs 抹平响应里每次运行随机生成的 item id 与 created_at 时间戳，
// 使两条输入路径的 SSE 输出可做语义级字节比较。
var normalizeStreamIDs = regexp.MustCompile(`("item_[0-9a-f]+"|"created_at":[0-9]+)`)

func TestResponsesChatFallback_OllamaDeepSeek_ReasoningAliasStreaming(t *testing.T) {
	account := ollamaUpstreamTestAccount(PlatformDeepseek, 602)
	body := []byte(`{"model":"deepseek-v4-flash","input":"hello","max_output_tokens":1000,"stream":true}`)

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
		rec, _ := runOllamaResponsesChatFallback(t, account, body, upstreamBody, true)
		return normalizeStreamIDs.ReplaceAllString(rec.Body.String(), `"item_x"`)
	}

	baseline := run(`"reasoning_content":"abc"`)
	require.Contains(t, baseline, "response.reasoning_summary_text.delta")
	require.Contains(t, baseline, `"delta":"abc"`)
	require.Contains(t, baseline, `"delta":"final answer"`)
	for _, tc := range []struct {
		name  string
		field string
	}{
		{name: "delta.reasoning alias", field: `"reasoning":"abc"`},
		{name: "delta.thinking alias", field: `"thinking":"abc"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, baseline, run(tc.field), "归一化后输出必须与 reasoning_content 基线完全一致")
		})
	}
}

func TestResponsesChatFallback_OllamaDeepSeek_ThinkingAliasNonStreaming(t *testing.T) {
	account := ollamaUpstreamTestAccount(PlatformDeepseek, 603)
	body := []byte(`{"model":"deepseek-v4-flash","input":"hello","max_output_tokens":1000,"stream":false}`)
	upstreamJSON := `{"id":"chatcmpl_ollama","object":"chat.completion","model":"deepseek-v4-flash","choices":[{"index":0,"message":{"role":"assistant","thinking":"abc","content":"final answer"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":5,"total_tokens":8}}`
	rec, _ := runOllamaResponsesChatFallback(t, account, body, upstreamJSON, false)
	require.Equal(t, "reasoning", gjson.Get(rec.Body.String(), "output.0.type").String())
	require.Equal(t, "abc", gjson.Get(rec.Body.String(), "output.0.summary.0.text").String())
	require.Equal(t, "final answer", gjson.Get(rec.Body.String(), "output.1.content.0.text").String())
}

func TestResponsesChatFallback_OfficialDeepSeek_Untouched(t *testing.T) {
	official := officialDeepSeekTestAccount(604)
	body := []byte(`{"model":"deepseek-v4-flash","input":"hello","max_output_tokens":256000,"stream":false}`)
	// thinking 不归一化：转换器只认 reasoning_content/reasoning，thinking 被丢弃，
	// 输出不出现 reasoning item（与不挂钩子的原始路径行为一致）。
	upstreamJSON := `{"id":"chatcmpl_official","object":"chat.completion","model":"deepseek-v4-flash","choices":[{"index":0,"message":{"role":"assistant","thinking":"abc","content":"final answer"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":5,"total_tokens":8}}`
	rec, upstream := runOllamaResponsesChatFallback(t, official, body, upstreamJSON, false)
	// 请求侧不被 clamp：max_completion_tokens 保持 256000。
	require.Equal(t, int64(256000), gjson.GetBytes(upstream.lastBody, "max_completion_tokens").Int())
	// 响应侧不归一化：输出只有 message item，没有 reasoning item。
	require.NotContains(t, rec.Body.String(), `"type":"reasoning"`)
	require.Equal(t, "final answer", gjson.Get(rec.Body.String(), "output.0.content.0.text").String())
}

// TestResponsesChatFallback_OllamaGLM_ClampsMaxTokens 验证 D1′ 后非 DeepSeek 模型
// （glm-5.3-flash）在 ollama host 的 Responses→CC 降级桥上同样被 clamp；reasoning
// 钩子仍按模型判定，非 DeepSeek 模型不做归一化。
func TestResponsesChatFallback_OllamaGLM_ClampsMaxTokens(t *testing.T) {
	account := ollamaUpstreamTestAccount(PlatformDeepseek, 605)
	body := []byte(`{"model":"glm-5.3-flash","input":"hello","max_output_tokens":256000,"stream":false}`)
	upstreamJSON := `{"id":"chatcmpl_ollama","object":"chat.completion","model":"glm-5.3-flash","choices":[{"index":0,"message":{"role":"assistant","thinking":"abc","content":"final answer"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":5,"total_tokens":8}}`
	rec, upstream := runOllamaResponsesChatFallback(t, account, body, upstreamJSON, false)
	require.Equal(t, int64(65535), gjson.GetBytes(upstream.lastBody, "max_completion_tokens").Int())
	require.NotContains(t, rec.Body.String(), `"type":"reasoning"`)
	require.Equal(t, "final answer", gjson.Get(rec.Body.String(), "output.0.content.0.text").String())
}
