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

func TestIsOllamaCloudRawChatCompletionsAccount(t *testing.T) {
	t.Parallel()

	t.Run("ollama.com + force_chat_completions", func(t *testing.T) {
		t.Parallel()
		require.True(t, isOllamaCloudRawChatCompletionsAccount(ollamaCloudRawChatCompletionsTestAccount()))
	})

	t.Run("extra usage signal without ollama host", func(t *testing.T) {
		t.Parallel()
		account := rawChatCompletionsTestAccount()
		account.Credentials["base_url"] = "https://example.invalid/v1"
		account.Extra = map[string]any{
			openai_compat.ExtraKeyResponsesMode: string(openai_compat.ResponsesSupportModeForceChatCompletions),
			OllamaCloudUsageSnapshotExtraKey:    map[string]any{"status": "ok"},
		}
		require.True(t, isOllamaCloudRawChatCompletionsAccount(account))
	})

	t.Run("official DeepSeek", func(t *testing.T) {
		t.Parallel()
		account := rawChatCompletionsTestAccount()
		account.Name = "DeepSeek"
		account.Credentials["base_url"] = "https://api.deepseek.com"
		account.Extra = map[string]any{
			openai_compat.ExtraKeyResponsesMode: string(openai_compat.ResponsesSupportModeForceChatCompletions),
		}
		require.False(t, isOllamaCloudRawChatCompletionsAccount(account))
	})

	t.Run("OpenCode Go extra", func(t *testing.T) {
		t.Parallel()
		account := rawChatCompletionsTestAccount()
		account.Credentials["base_url"] = "https://opencode.ai/zen/go/v1"
		account.Extra = map[string]any{
			openai_compat.ExtraKeyResponsesMode: string(openai_compat.ResponsesSupportModeForceChatCompletions),
			"opencode_go_usage_auto_refresh":    true,
		}
		require.False(t, isOllamaCloudRawChatCompletionsAccount(account))
	})

	t.Run("ollama.com without force_chat_completions", func(t *testing.T) {
		t.Parallel()
		account := ollamaCloudRawChatCompletionsTestAccount()
		account.Extra = nil
		require.False(t, isOllamaCloudRawChatCompletionsAccount(account))
	})

	t.Run("anthropic ollama.com", func(t *testing.T) {
		t.Parallel()
		account := ollamaCloudRawChatCompletionsTestAccount()
		account.Platform = PlatformAnthropic
		require.False(t, isOllamaCloudRawChatCompletionsAccount(account))
	})
}

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

func TestApplyOllamaCloudRawChatCompletionsLeavesForeignAccountsUnchanged(t *testing.T) {
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
		require.Equal(t, reqBody, applyOllamaCloudRawChatCompletionsRequest(account, reqBody))
		require.Equal(t, respBody, applyOllamaCloudRawChatCompletionsResponse(account, respBody))
		require.Equal(t, sseLine, applyOllamaCloudRawChatCompletionsSSELine(account, sseLine))
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
