package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func openaiPlatformDeepSeekAccount() *Account {
	return &Account{
		ID:       18,
		Name:     "openai-deepseek-apikey",
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Extra:    map[string]any{"openai_responses_supported": true},
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "https://api.deepseek.com",
		},
		Status:      StatusActive,
		Schedulable: true,
	}
}

func deepSeekChatFallbackTestConfig() *config.Config {
	return &config.Config{
		Security: config.SecurityConfig{
			URLAllowlist: config.URLAllowlistConfig{
				Enabled:           false,
				AllowInsecureHTTP: true,
			},
		},
	}
}

func TestEnsureDeepSeekChatReasoningPlaceholders(t *testing.T) {
	deepSeekAccount := openaiPlatformDeepSeekAccount()
	nativeDeepSeek := &Account{Platform: PlatformDeepseek, Type: AccountTypeAPIKey}
	otherOpenAI := &Account{
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "sk-test", "base_url": "http://upstream.example"},
	}

	missing := []byte(`{"model":"deepseek-flash","messages":[{"role":"user","content":"hi"},{"role":"assistant","content":"","tool_calls":[{"id":"call_1","type":"function","function":{"name":"exec","arguments":"{}"}}]},{"role":"tool","tool_call_id":"call_1","content":"ok"}]}`)
	withPlain := []byte(`{"model":"deepseek-flash","messages":[{"role":"assistant","reasoning_content":"real thinking","content":"ok"}]}`)

	t.Run("openai_platform_deepseek_fills_empty_assistant", func(t *testing.T) {
		got := ensureDeepSeekChatReasoningPlaceholders(deepSeekAccount, missing)
		require.Equal(t, responsesReasoningPlaceholderText, gjson.GetBytes(got, "messages.1.reasoning_content").String())
		require.False(t, gjson.GetBytes(got, "messages.0.reasoning_content").Exists())
		require.False(t, gjson.GetBytes(got, "messages.2.reasoning_content").Exists())
	})

	t.Run("native_deepseek_platform_fills_empty_assistant", func(t *testing.T) {
		got := ensureDeepSeekChatReasoningPlaceholders(nativeDeepSeek, missing)
		require.Equal(t, responsesReasoningPlaceholderText, gjson.GetBytes(got, "messages.1.reasoning_content").String())
	})

	t.Run("does_not_overwrite_plaintext", func(t *testing.T) {
		got := ensureDeepSeekChatReasoningPlaceholders(deepSeekAccount, withPlain)
		require.Equal(t, "real thinking", gjson.GetBytes(got, "messages.0.reasoning_content").String())
		require.Equal(t, string(withPlain), string(got), "已有明文时必须原样返回")
	})

	t.Run("non_deepseek_upstream_unchanged", func(t *testing.T) {
		got := ensureDeepSeekChatReasoningPlaceholders(otherOpenAI, missing)
		require.Equal(t, string(missing), string(got))
		require.False(t, gjson.GetBytes(got, "messages.1.reasoning_content").Exists())
	})

	t.Run("nil_account_unchanged", func(t *testing.T) {
		got := ensureDeepSeekChatReasoningPlaceholders(nil, missing)
		require.Equal(t, string(missing), string(got))
	})
}

type reasoningHitCache struct {
	stubGatewayCache
	values map[string]string
}

func (c *reasoningHitCache) GetReasoningContent(_ context.Context, itemID string) (string, error) {
	if v, ok := c.values[itemID]; ok {
		return v, nil
	}
	return "", ErrReasoningContentNotFound
}

func deepSeekLiteHistoryWithEncryptedReasoning() []byte {
	return []byte(`{
		"model":"gpt-5.6-sol",
		"stream":false,
		"input":[
			{"type":"reasoning","id":"item_enc1","summary":[],"encrypted_content":"opaque"},
			{"type":"function_call","call_id":"call_1","name":"exec","arguments":"{}"},
			{"type":"function_call_output","call_id":"call_1","output":"ok"},
			{"type":"message","role":"user","content":[{"type":"input_text","text":"go on"}]},
			{"type":"additional_tools","role":"developer","tools":[
				{"type":"function","name":"exec","parameters":{"type":"object"}}
			]}
		]
	}`)
}

func newDeepSeekChatFallbackContext(t *testing.T, body []byte) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return c
}

func newOKChatCompletionsUpstream(requestID, body string) *httpUpstreamRecorder {
	return &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}, "x-request-id": []string{requestID}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}}
}

const deepSeekChatFallbackOKBody = `{"id":"chatcmpl_ph","object":"chat.completion","model":"deepseek-flash","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`

// 账号 18 形态：openai 平台 + DeepSeek base + Responses Lite。加密-only reasoning
// 在缓存未命中时桥接原本不写 reasoning_content，DeepSeek thinking mode 会 400。
func TestForwardResponses_DeepSeekChatFallbackInjectsReasoningPlaceholderOnCacheMiss(t *testing.T) {
	body := deepSeekLiteHistoryWithEncryptedReasoning()
	c := newDeepSeekChatFallbackContext(t, body)
	upstream := newOKChatCompletionsUpstream("rid_ds_rc_placeholder", deepSeekChatFallbackOKBody)
	svc := &OpenAIGatewayService{
		cfg:          deepSeekChatFallbackTestConfig(),
		httpUpstream: upstream,
	}

	result, err := svc.Forward(context.Background(), c, openaiPlatformDeepSeekAccount(), body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Contains(t, upstream.lastReq.URL.String(), "/chat/completions")
	require.Equal(t, responsesReasoningPlaceholderText, gjson.GetBytes(upstream.lastBody, "messages.0.reasoning_content").String())
	require.Equal(t, "call_1", gjson.GetBytes(upstream.lastBody, "messages.0.tool_calls.0.id").String())
}

func TestForwardResponses_DeepSeekChatFallbackKeepsCachedReasoningContent(t *testing.T) {
	body := deepSeekLiteHistoryWithEncryptedReasoning()
	c := newDeepSeekChatFallbackContext(t, body)
	upstream := newOKChatCompletionsUpstream("rid_ds_rc_cached", deepSeekChatFallbackOKBody)
	svc := &OpenAIGatewayService{
		cfg:          deepSeekChatFallbackTestConfig(),
		httpUpstream: upstream,
		cache:        &reasoningHitCache{values: map[string]string{"item_enc1": "cached thinking"}},
	}

	result, err := svc.Forward(context.Background(), c, openaiPlatformDeepSeekAccount(), body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "cached thinking", gjson.GetBytes(upstream.lastBody, "messages.0.reasoning_content").String())
}

func TestForwardResponses_NonDeepSeekChatFallbackDoesNotInjectReasoningPlaceholder(t *testing.T) {
	body := deepSeekLiteHistoryWithEncryptedReasoning()
	c := newDeepSeekChatFallbackContext(t, body)
	upstream := newOKChatCompletionsUpstream("rid_other_rc", deepSeekChatFallbackOKBody)
	account := &Account{
		ID:       99,
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Extra: map[string]any{
			openai_compat.ExtraKeyResponsesMode: string(openai_compat.ResponsesSupportModeForceChatCompletions),
		},
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "http://upstream.example",
		},
	}
	svc := &OpenAIGatewayService{
		cfg:          deepSeekChatFallbackTestConfig(),
		httpUpstream: upstream,
	}

	result, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, gjson.GetBytes(upstream.lastBody, "messages.0.reasoning_content").Exists())
}
