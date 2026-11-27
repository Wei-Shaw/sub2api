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

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func ollamaCloudRawChatCompletionsTestAccount() *Account {
	return &Account{
		ID:       143,
		Name:     "DeepSeek Ollama",
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "https://ollama.com",
		},
		Extra: map[string]any{
			openai_compat.ExtraKeyResponsesMode: string(openai_compat.ResponsesSupportModeForceChatCompletions),
		},
	}
}

// TestIsOllamaCloudRawChatCompletionsAccount 已随谓词删除：reasoning 修复把 CC 钩子
// 换成按 host + 模型判定的 isOllamaCloudDeepSeekUpstream（见下方同名测试），D1′ 又
// 移除了 clamp 侧最后一个引用，平台 + responses_mode 双门禁判定退出仓库。

func TestNormalizeOllamaCloudChatCompletionsResponseJSON(t *testing.T) {
	t.Parallel()

	t.Run("copies delta.reasoning to reasoning_content", func(t *testing.T) {
		t.Parallel()
		in := []byte(`{"choices":[{"delta":{"reasoning":"abc"}}]}`)
		out := normalizeOllamaCloudChatCompletionsResponseJSON(in)
		require.Equal(t, "abc", gjson.GetBytes(out, "choices.0.delta.reasoning").String())
		require.Equal(t, "abc", gjson.GetBytes(out, "choices.0.delta.reasoning_content").String())
	})

	t.Run("copies message.thinking to reasoning_content", func(t *testing.T) {
		t.Parallel()
		in := []byte(`{"choices":[{"message":{"thinking":"abc"}}]}`)
		out := normalizeOllamaCloudChatCompletionsResponseJSON(in)
		require.Equal(t, "abc", gjson.GetBytes(out, "choices.0.message.thinking").String())
		require.Equal(t, "abc", gjson.GetBytes(out, "choices.0.message.reasoning_content").String())
	})

	t.Run("does not overwrite existing reasoning_content", func(t *testing.T) {
		t.Parallel()
		in := []byte(`{"choices":[{"delta":{"reasoning":"new","reasoning_content":"old"}}]}`)
		out := normalizeOllamaCloudChatCompletionsResponseJSON(in)
		require.Equal(t, string(in), string(out))
		require.Equal(t, "old", gjson.GetBytes(out, "choices.0.delta.reasoning_content").String())
	})

	t.Run("empty reasoning does not open reasoning_content", func(t *testing.T) {
		t.Parallel()
		in := []byte(`{"choices":[{"delta":{"reasoning":""}}]}`)
		out := normalizeOllamaCloudChatCompletionsResponseJSON(in)
		require.Equal(t, string(in), string(out))
		require.False(t, gjson.GetBytes(out, "choices.0.delta.reasoning_content").Exists())
	})

	t.Run("empty thinking does not open reasoning_content", func(t *testing.T) {
		t.Parallel()
		in := []byte(`{"choices":[{"message":{"thinking":""}}]}`)
		out := normalizeOllamaCloudChatCompletionsResponseJSON(in)
		require.Equal(t, string(in), string(out))
		require.False(t, gjson.GetBytes(out, "choices.0.message.reasoning_content").Exists())
	})

	t.Run("tool call chunk is unchanged", func(t *testing.T) {
		t.Parallel()
		in := []byte(`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{}"}}]}}]}`)
		out := normalizeOllamaCloudChatCompletionsResponseJSON(in)
		require.Equal(t, string(in), string(out))
	})
}

func TestNormalizeOllamaCloudChatCompletionsRequest(t *testing.T) {
	t.Parallel()

	in := []byte(`{"messages":[{"role":"user","content":"weather"},{"role":"assistant","reasoning_content":"prev","content":"","tool_calls":[{"id":"call_1","type":"function","function":{"name":"get_weather","arguments":"{}"}}]}]}`)
	out := normalizeOllamaCloudChatCompletionsRequest(in)
	require.Equal(t, "prev", gjson.GetBytes(out, "messages.1.reasoning").String())
	require.Equal(t, "prev", gjson.GetBytes(out, "messages.1.reasoning_content").String())
	require.Equal(t, "", gjson.GetBytes(out, "messages.1.content").String())
	require.Equal(t, "get_weather", gjson.GetBytes(out, "messages.1.tool_calls.0.function.name").String())
	require.False(t, gjson.GetBytes(out, "messages.0.reasoning").Exists())
}

func TestApplyRawChatCompletionsReasoningCompatLeavesForeignAccountsUnchanged(t *testing.T) {
	t.Parallel()

	reqBody := []byte(`{"messages":[{"role":"assistant","reasoning_content":"prev","content":""}]}`)
	respBody := []byte(`{"choices":[{"delta":{"reasoning":"abc"}}]}`)
	sseLine := `data: {"choices":[{"delta":{"reasoning":"abc"}}]}`

	official := rawChatCompletionsTestAccount()
	official.Name = "DeepSeek"
	official.Credentials["base_url"] = "https://api.deepseek.com"
	official.Extra = map[string]any{
		openai_compat.ExtraKeyResponsesMode: string(openai_compat.ResponsesSupportModeForceChatCompletions),
	}

	opencode := rawChatCompletionsTestAccount()
	opencode.Credentials["base_url"] = "https://opencode.ai/zen/go/v1"
	opencode.Extra = map[string]any{
		openai_compat.ExtraKeyResponsesMode: string(openai_compat.ResponsesSupportModeForceChatCompletions),
		"opencode_go_usage_auto_refresh":    true,
	}

	for _, account := range []*Account{official, opencode} {
		// 出站 base_url 都不是 ollama.com，即使模型是 DeepSeek 系、即使带残留
		// usage extra，reasoning 归一化门控不命中，字节级不变。
		require.Equal(t, reqBody, applyRawChatCompletionsReasoningCompatRequest(account, "deepseek-v4-flash", reqBody))
		require.Equal(t, respBody, applyRawChatCompletionsReasoningCompatResponse(account, "deepseek-v4-flash", respBody))
		require.Equal(t, sseLine, applyRawChatCompletionsReasoningCompatSSELine(account, "deepseek-v4-flash", sseLine))
	}
}

func TestNormalizeOllamaCloudChatCompletionsSSELine(t *testing.T) {
	t.Parallel()

	before := `data: {"choices":[{"delta":{"reasoning":"abc"}}]}`
	after := normalizeOllamaCloudChatCompletionsSSELine(before)
	require.True(t, strings.HasPrefix(after, "data: "))
	payload := strings.TrimPrefix(after, "data: ")
	require.Equal(t, "abc", gjson.Get(payload, "choices.0.delta.reasoning").String())
	require.Equal(t, "abc", gjson.Get(payload, "choices.0.delta.reasoning_content").String())
	require.Equal(t, "data: [DONE]", normalizeOllamaCloudChatCompletionsSSELine("data: [DONE]"))
}

func selfHostedRawChatCompletionsTestAccount(extra map[string]any) *Account {
	account := rawChatCompletionsTestAccount()
	account.Name = "vLLM"
	account.Credentials["base_url"] = "http://vllm.internal:8000"
	account.Extra = map[string]any{
		openai_compat.ExtraKeyResponsesMode: string(openai_compat.ResponsesSupportModeForceChatCompletions),
	}
	for key, value := range extra {
		account.Extra[key] = value
	}
	return account
}

func TestRawChatCompletionsReasoningCompatOptIn(t *testing.T) {
	t.Parallel()

	reqBody := []byte(`{"messages":[{"role":"assistant","reasoning_content":"prev","content":""}]}`)
	respBody := []byte(`{"choices":[{"message":{"role":"assistant","reasoning":"abc","content":"done"}}]}`)
	sseLine := `data: {"choices":[{"delta":{"reasoning":"abc"}}]}`

	t.Run("opted in", func(t *testing.T) {
		t.Parallel()
		account := selfHostedRawChatCompletionsTestAccount(map[string]any{OpenAIChatReasoningContentExtraKey: true})
		require.True(t, rawChatCompletionsReasoningCompatEnabled(account, "deepseek-v4.1-flash"))
		require.Equal(t, "prev", gjson.GetBytes(applyRawChatCompletionsReasoningCompatRequest(account, "deepseek-v4.1-flash", reqBody), "messages.0.reasoning").String())
		resp := applyRawChatCompletionsReasoningCompatResponse(account, "deepseek-v4.1-flash", respBody)
		require.Equal(t, "abc", gjson.GetBytes(resp, "choices.0.message.reasoning").String())
		require.Equal(t, "abc", gjson.GetBytes(resp, "choices.0.message.reasoning_content").String())
		line := applyRawChatCompletionsReasoningCompatSSELine(account, "deepseek-v4.1-flash", sseLine)
		require.Equal(t, "abc", gjson.Get(strings.TrimPrefix(line, "data: "), "choices.0.delta.reasoning_content").String())
	})

	oauth := selfHostedRawChatCompletionsTestAccount(map[string]any{OpenAIChatReasoningContentExtraKey: true})
	oauth.Type = AccountTypeOAuth
	disabled := map[string]*Account{
		"not opted in":     selfHostedRawChatCompletionsTestAccount(nil),
		"explicitly false": selfHostedRawChatCompletionsTestAccount(map[string]any{OpenAIChatReasoningContentExtraKey: false}),
		"string value":     selfHostedRawChatCompletionsTestAccount(map[string]any{OpenAIChatReasoningContentExtraKey: "true"}),
		"OAuth account":    oauth,
	}
	for name, account := range disabled {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			require.False(t, rawChatCompletionsReasoningCompatEnabled(account, "deepseek-v4.1-flash"))
			require.Equal(t, reqBody, applyRawChatCompletionsReasoningCompatRequest(account, "deepseek-v4.1-flash", reqBody))
			require.Equal(t, respBody, applyRawChatCompletionsReasoningCompatResponse(account, "deepseek-v4.1-flash", respBody))
			require.Equal(t, sseLine, applyRawChatCompletionsReasoningCompatSSELine(account, "deepseek-v4.1-flash", sseLine))
		})
	}
}

func TestForwardAsRawChatCompletions_OllamaCloudReasoningAliasStreaming(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"deepseek-v4-pro","messages":[{"role":"user","content":"hello"}],"stream":true}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstreamBody := strings.Join([]string{
		`data: {"id":"chatcmpl_ollama","object":"chat.completion.chunk","model":"deepseek-v4-pro","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl_ollama","object":"chat.completion.chunk","model":"deepseek-v4-pro","choices":[{"index":0,"delta":{"reasoning":"abc"},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl_ollama","object":"chat.completion.chunk","model":"deepseek-v4-pro","choices":[{"index":0,"delta":{"content":"final answer"},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl_ollama","object":"chat.completion.chunk","model":"deepseek-v4-pro","choices":[],"usage":{"prompt_tokens":3,"completion_tokens":5,"total_tokens":8,"completion_tokens_details":{"reasoning_tokens":4}}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid_ollama_reasoning_stream"}},
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	}}

	svc := &OpenAIGatewayService{
		cfg:          rawChatCompletionsTestConfig(),
		httpUpstream: upstream,
	}

	result, err := svc.forwardAsRawChatCompletions(context.Background(), c, ollamaCloudRawChatCompletionsTestAccount(), body, "")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 3, result.Usage.InputTokens)
	require.Equal(t, 5, result.Usage.OutputTokens)
	require.Contains(t, rec.Body.String(), `"reasoning":"abc"`)
	require.Contains(t, rec.Body.String(), `"reasoning_content":"abc"`)
	require.Contains(t, rec.Body.String(), `"content":"final answer"`)
	require.Contains(t, rec.Body.String(), `"reasoning_tokens":4`)
	require.Contains(t, rec.Body.String(), "data: [DONE]")
}

func TestForwardAsRawChatCompletions_OllamaCloudThinkingAliasNonStreaming(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"deepseek-v4-pro","messages":[{"role":"user","content":"hello"},{"role":"assistant","reasoning_content":"prev","content":""}],"stream":false}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstreamJSON := `{"id":"chatcmpl_ollama","object":"chat.completion","model":"deepseek-v4-pro","choices":[{"index":0,"message":{"role":"assistant","thinking":"abc","content":"final answer"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":5,"total_tokens":8,"completion_tokens_details":{"reasoning_tokens":4}}}`
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}, "x-request-id": []string{"rid_ollama_thinking_json"}},
		Body:       io.NopCloser(strings.NewReader(upstreamJSON)),
	}}

	svc := &OpenAIGatewayService{
		cfg:          rawChatCompletionsTestConfig(),
		httpUpstream: upstream,
	}

	result, err := svc.forwardAsRawChatCompletions(context.Background(), c, ollamaCloudRawChatCompletionsTestAccount(), body, "")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "prev", gjson.GetBytes(upstream.lastBody, "messages.1.reasoning").String())
	require.Equal(t, "prev", gjson.GetBytes(upstream.lastBody, "messages.1.reasoning_content").String())
	require.Equal(t, "abc", gjson.Get(rec.Body.String(), "choices.0.message.thinking").String())
	require.Equal(t, "abc", gjson.Get(rec.Body.String(), "choices.0.message.reasoning_content").String())
	require.Equal(t, "final answer", gjson.Get(rec.Body.String(), "choices.0.message.content").String())
	require.Equal(t, int64(4), gjson.Get(rec.Body.String(), "usage.completion_tokens_details.reasoning_tokens").Int())
}

// TestIsOllamaCloudDeepSeekUpstream 验证唯一判据：出站 base_url 是 Ollama Cloud &&
// 出站模型是 DeepSeek 系，不看 platform / account.Type / responses-mode / usage extra。
func TestIsOllamaCloudDeepSeekUpstream(t *testing.T) {
	t.Parallel()

	t.Run("ollama.com + deepseek model regardless of platform", func(t *testing.T) {
		t.Parallel()
		for _, platform := range []string{PlatformDeepseek, PlatformZhipu, PlatformOpenAI} {
			require.True(t, isOllamaCloudDeepSeekUpstream(ollamaUpstreamTestAccount(platform, 410), "deepseek-v4-flash"),
				"platform %s should be recognized", platform)
		}
	})

	t.Run("ollama.com + non-deepseek model", func(t *testing.T) {
		t.Parallel()
		require.False(t, isOllamaCloudDeepSeekUpstream(ollamaUpstreamTestAccount(PlatformDeepseek, 411), "glm-5.3-flash"))
	})

	t.Run("official deepseek upstream is untouched", func(t *testing.T) {
		t.Parallel()
		require.False(t, isOllamaCloudDeepSeekUpstream(officialDeepSeekTestAccount(412), "deepseek-v4-flash"))
	})

	t.Run("nil account", func(t *testing.T) {
		t.Parallel()
		require.False(t, isOllamaCloudDeepSeekUpstream(nil, "deepseek-v4-flash"))
	})
}

// TestApplyOllamaCloudRawChatCompletionsByUpstream 验证三个 apply* 包装函数按唯一
// 判据（出站 base_url 是 Ollama Cloud && 出站模型是 DeepSeek 系）生效，不看
// platform / account.Type / responses-mode / usage extra；官方 DeepSeek 字节级不变。
func TestApplyOllamaCloudRawChatCompletionsByUpstream(t *testing.T) {
	t.Parallel()

	reqBody := []byte(`{"messages":[{"role":"assistant","reasoning_content":"prev","content":""}]}`)
	respBody := []byte(`{"choices":[{"delta":{"reasoning":"abc"}}]}`)
	sseLine := `data: {"choices":[{"delta":{"reasoning":"abc"}}]}`

	t.Run("deepseek platform ollama account + deepseek model is normalized", func(t *testing.T) {
		t.Parallel()
		account := ollamaUpstreamTestAccount(PlatformDeepseek, 401)
		normalizedReq := applyRawChatCompletionsReasoningCompatRequest(account, "deepseek-v4-flash", reqBody)
		require.Equal(t, "prev", gjson.GetBytes(normalizedReq, "messages.0.reasoning").String())
		require.Equal(t, "prev", gjson.GetBytes(normalizedReq, "messages.0.reasoning_content").String())

		normalizedResp := applyRawChatCompletionsReasoningCompatResponse(account, "deepseek-v4-flash", respBody)
		require.Equal(t, "abc", gjson.GetBytes(normalizedResp, "choices.0.delta.reasoning").String())
		require.Equal(t, "abc", gjson.GetBytes(normalizedResp, "choices.0.delta.reasoning_content").String())

		normalizedLine := applyRawChatCompletionsReasoningCompatSSELine(account, "deepseek-v4-flash", sseLine)
		require.True(t, strings.HasPrefix(normalizedLine, "data: "))
		payload := strings.TrimPrefix(normalizedLine, "data: ")
		require.Equal(t, "abc", gjson.Get(payload, "choices.0.delta.reasoning").String())
		require.Equal(t, "abc", gjson.Get(payload, "choices.0.delta.reasoning_content").String())
	})

	t.Run("official deepseek account is byte-identical", func(t *testing.T) {
		t.Parallel()
		official := officialDeepSeekTestAccount(402)
		require.Equal(t, reqBody, applyRawChatCompletionsReasoningCompatRequest(official, "deepseek-v4-flash", reqBody))
		require.Equal(t, respBody, applyRawChatCompletionsReasoningCompatResponse(official, "deepseek-v4-flash", respBody))
		require.Equal(t, sseLine, applyRawChatCompletionsReasoningCompatSSELine(official, "deepseek-v4-flash", sseLine))
	})

	t.Run("ollama account with non-deepseek model is byte-identical", func(t *testing.T) {
		t.Parallel()
		account := ollamaUpstreamTestAccount(PlatformDeepseek, 403)
		require.Equal(t, reqBody, applyRawChatCompletionsReasoningCompatRequest(account, "glm-5.3-flash", reqBody))
		require.Equal(t, respBody, applyRawChatCompletionsReasoningCompatResponse(account, "glm-5.3-flash", respBody))
		require.Equal(t, sseLine, applyRawChatCompletionsReasoningCompatSSELine(account, "glm-5.3-flash", sseLine))
	})
}

func TestForwardAsRawChatCompletions_OptInReasoningContentStreaming(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"deepseek-v4.1-flash","messages":[{"role":"user","content":"hello"}],"stream":true}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstreamBody := strings.Join([]string{
		`data: {"id":"chatcmpl_vllm","object":"chat.completion.chunk","model":"deepseek-v4.1-flash","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl_vllm","object":"chat.completion.chunk","model":"deepseek-v4.1-flash","choices":[{"index":0,"delta":{"reasoning":"abc"},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl_vllm","object":"chat.completion.chunk","model":"deepseek-v4.1-flash","choices":[{"index":0,"delta":{"content":"final answer"},"finish_reason":"stop"}]}`,
		"",
		`data: {"id":"chatcmpl_vllm","object":"chat.completion.chunk","model":"deepseek-v4.1-flash","choices":[],"usage":{"prompt_tokens":3,"completion_tokens":5,"total_tokens":8}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid_vllm_reasoning_stream"}},
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	}}

	svc := &OpenAIGatewayService{
		cfg:          rawChatCompletionsTestConfig(),
		httpUpstream: upstream,
	}

	account := selfHostedRawChatCompletionsTestAccount(map[string]any{OpenAIChatReasoningContentExtraKey: true})
	result, err := svc.forwardAsRawChatCompletions(context.Background(), c, account, body, "")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 3, result.Usage.InputTokens)
	require.Equal(t, 5, result.Usage.OutputTokens)
	require.Contains(t, rec.Body.String(), `"reasoning":"abc"`)
	require.Contains(t, rec.Body.String(), `"reasoning_content":"abc"`)
	require.Contains(t, rec.Body.String(), `"content":"final answer"`)
	require.Contains(t, rec.Body.String(), "data: [DONE]")
}
