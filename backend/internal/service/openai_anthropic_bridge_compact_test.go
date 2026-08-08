package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestAnthropicBridgeActiveSuffixItemCount(t *testing.T) {
	t.Run("plain user turn", func(t *testing.T) {
		var req apicompat.AnthropicRequest
		require.NoError(t, json.Unmarshal([]byte(`{
			"model":"gpt-5.6",
			"messages":[
				{"role":"user","content":"old"},
				{"role":"assistant","content":"done"},
				{"role":"user","content":"current"}
			]
		}`), &req))

		require.Equal(t, 1, anthropicBridgeActiveSuffixItemCount(&req))
	})

	t.Run("tool continuation keeps assistant call", func(t *testing.T) {
		var req apicompat.AnthropicRequest
		require.NoError(t, json.Unmarshal([]byte(`{
			"model":"gpt-5.6",
			"messages":[
				{"role":"user","content":"inspect"},
				{"role":"assistant","content":[{"type":"tool_use","id":"call_1","name":"Read","input":{"path":"a"}}]},
				{"role":"user","content":[{"type":"tool_result","tool_use_id":"call_1","content":"result"}]}
			]
		}`), &req))

		require.Equal(t, 2, anthropicBridgeActiveSuffixItemCount(&req))
	})
}

func TestMaybeAutoCompactAnthropicBridgePreservesOpaqueOutputAndSuffix(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	c.Request.Header.Set("User-Agent", "codex_cli_rs/0.144.1")

	compactResponse := `{
		"output":[
			{"type":"message","role":"assistant","content":[{"type":"output_text","text":"retained"}]},
			{"type":"compaction","encrypted_content":"opaque","unknown":{"nested":[1,2,3]}}
		],
		"usage":{"input_tokens":20,"output_tokens":3,"input_tokens_details":{"cached_tokens":7}}
	}`
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(compactResponse)),
	}}
	svc := &OpenAIGatewayService{
		httpUpstream: upstream,
		cfg: &config.Config{Gateway: config.GatewayConfig{
			OpenAICompactModel:                       "gpt-5.4",
			AnthropicBridgeAutoCompactEnabled:        true,
			AnthropicBridgeAutoCompactInputBytes:     1,
			AnthropicBridgeAutoCompactTimeoutSeconds: 60,
		}},
	}
	account := openAIAnthropicAutoCompactTestAccount()
	body := []byte(`{
		"model":"gpt-5.6",
		"instructions":"keep",
		"reasoning":{"effort":"max"},
		"prompt_cache_key":"drop-from-compact-body",
		"input":[
			{"type":"message","role":"developer","content":[{"type":"input_text","text":"system"}]},
			{"type":"message","role":"user","content":[{"type":"input_text","text":"` + strings.Repeat("history", 100) + `"}]},
			{"type":"message","role":"assistant","content":[{"type":"output_text","text":"done"}]},
			{"type":"message","role":"user","content":[{"type":"input_text","text":"current"}]}
		],
		"stream":true,
		"store":false
	}`)

	result := svc.maybeAutoCompactAnthropicBridge(
		context.Background(), c, account, body, "claude-gpt-5-6-sol", "token", "session-key", "turn-state", 1,
	)

	require.True(t, result.Applied)
	require.Equal(t, 20, result.Usage.InputTokens)
	require.Equal(t, 3, result.Usage.OutputTokens)
	require.Equal(t, 7, result.Usage.CacheReadInputTokens)
	require.Len(t, upstream.requests, 1)
	require.Equal(t, chatgptCodexURL+"/compact", upstream.requests[0].URL.String())
	require.Equal(t, "turn-state", upstream.requests[0].Header.Get("x-codex-turn-state"))
	require.Equal(t, "gpt-5.6", gjson.GetBytes(upstream.bodies[0], "model").String())
	require.Equal(t, "xhigh", gjson.GetBytes(upstream.bodies[0], "reasoning.effort").String())
	require.Equal(t, int64(3), gjson.GetBytes(upstream.bodies[0], "input.#").Int())
	require.False(t, gjson.GetBytes(upstream.bodies[0], "stream").Exists())
	require.False(t, gjson.GetBytes(upstream.bodies[0], "store").Exists())
	require.False(t, gjson.GetBytes(upstream.bodies[0], "prompt_cache_key").Exists())

	var updated struct {
		Input []json.RawMessage `json:"input"`
	}
	require.NoError(t, json.Unmarshal(result.Body, &updated))
	require.Len(t, updated.Input, 3)
	require.JSONEq(t, `{"type":"message","role":"assistant","content":[{"type":"output_text","text":"retained"}]}`, string(updated.Input[0]))
	require.JSONEq(t, `{"type":"compaction","encrypted_content":"opaque","unknown":{"nested":[1,2,3]}}`, string(updated.Input[1]))
	require.Contains(t, string(updated.Input[2]), "current")
}

func TestMaybeAutoCompactAnthropicBridgeFailsOpen(t *testing.T) {
	body := []byte(`{"model":"gpt-5.6","input":[{"role":"user","content":"history"},{"role":"user","content":"current"}]}`)
	tests := []struct {
		name       string
		statusCode int
		response   string
	}{
		{name: "upstream status", statusCode: http.StatusBadRequest, response: `{"error":{"message":"unsupported"}}`},
		{name: "malformed response", statusCode: http.StatusOK, response: `{`},
		{name: "empty output", statusCode: http.StatusOK, response: `{"output":[]}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: tt.statusCode,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(tt.response)),
			}}
			svc := newAnthropicAutoCompactTestService(upstream, true)

			result := svc.maybeAutoCompactAnthropicBridge(
				context.Background(), c, openAIAnthropicAutoCompactTestAccount(), body, "gpt-5.6", "token", "session-key", "", 1,
			)

			require.False(t, result.Applied)
			require.True(t, bytes.Equal(body, result.Body))
		})
	}
}

func TestMaybeAutoCompactAnthropicBridgeSkipsDisabledAndUnsplittableRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	body := []byte(`{"model":"gpt-5.6","input":[{"role":"user","content":"only-current-turn"}]}`)

	upstream := &httpUpstreamRecorder{}
	disabled := newAnthropicAutoCompactTestService(upstream, false)
	result := disabled.maybeAutoCompactAnthropicBridge(
		context.Background(), c, openAIAnthropicAutoCompactTestAccount(), body, "gpt-5.6", "token", "session-key", "", 1,
	)
	require.False(t, result.Applied)

	enabled := newAnthropicAutoCompactTestService(upstream, true)
	result = enabled.maybeAutoCompactAnthropicBridge(
		context.Background(), c, openAIAnthropicAutoCompactTestAccount(), body, "gpt-5.6", "token", "session-key", "", 1,
	)
	require.False(t, result.Applied)

	apiKeyAccount := openAIAnthropicAutoCompactTestAccount()
	apiKeyAccount.Type = AccountTypeAPIKey
	result = enabled.maybeAutoCompactAnthropicBridge(
		context.Background(), c, apiKeyAccount, body, "gpt-5.6", "token", "session-key", "", 1,
	)
	require.False(t, result.Applied)

	result = enabled.maybeAutoCompactAnthropicBridge(
		context.Background(), c, openAIAnthropicAutoCompactTestAccount(), body, "claude-fable-5", "token", "session-key", "", 1,
	)
	require.False(t, result.Applied)
	require.Empty(t, upstream.requests)
}

func TestIsExplicitGPTAnthropicBridgeModel(t *testing.T) {
	require.True(t, isExplicitGPTAnthropicBridgeModel("claude-gpt-5-6-sol"))
	require.True(t, isExplicitGPTAnthropicBridgeModel("gpt-5.6"))
	require.True(t, isExplicitGPTAnthropicBridgeModel("openai/gpt5.6"))
	require.False(t, isExplicitGPTAnthropicBridgeModel("claude-fable-5"))
	require.False(t, isExplicitGPTAnthropicBridgeModel("claude-opus-4-6"))
}

func TestForwardAsAnthropicAutoCompactsBeforeGeneration(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{
		"model":"gpt-5.6",
		"max_tokens":16,
		"messages":[
			{"role":"user","content":"` + strings.Repeat("old-history-", 100) + `"},
			{"role":"assistant","content":"done"},
			{"role":"user","content":"current-turn"}
		],
		"stream":false
	}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(`{
				"output":[{"type":"compaction","encrypted_content":"opaque-history"}],
				"usage":{"input_tokens":20,"output_tokens":3,"input_tokens_details":{"cached_tokens":7}}
			}`)),
		},
		openAICompatSSECompletedResponse("resp_auto_compact", "gpt-5.6"),
	}}
	svc := &OpenAIGatewayService{
		httpUpstream: upstream,
		cfg: &config.Config{
			Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}},
			Gateway: config.GatewayConfig{
				OpenAICompactModel:                       "gpt-5.4",
				AnthropicBridgeAutoCompactEnabled:        true,
				AnthropicBridgeAutoCompactInputBytes:     1,
				AnthropicBridgeAutoCompactTimeoutSeconds: 60,
			},
		},
	}

	result, err := svc.ForwardAsAnthropic(
		context.Background(), c, openAIAnthropicAutoCompactTestAccount(), body, "session-key", "gpt-5.6",
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.requests, 2)
	require.Equal(t, chatgptCodexURL+"/compact", upstream.requests[0].URL.String())
	require.Equal(t, chatgptCodexURL, upstream.requests[1].URL.String())
	require.Equal(t, upstream.requests[0].Header.Get("session_id"), upstream.requests[1].Header.Get("session_id"))
	require.Equal(t, gjson.GetBytes(upstream.bodies[1], "model").String(), gjson.GetBytes(upstream.bodies[0], "model").String())
	require.Equal(t, "compaction", gjson.GetBytes(upstream.bodies[1], "input.0.type").String())
	require.Equal(t, "current-turn", gjson.GetBytes(upstream.bodies[1], "input.1.content.0.text").String())
	require.Equal(t, 25, result.Usage.InputTokens)
	require.Equal(t, 5, result.Usage.OutputTokens)
	require.Equal(t, 7, result.Usage.CacheReadInputTokens)
}

func TestForwardAsAnthropicAutoCompactSkipsFableDispatch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{
		"model":"claude-fable-5",
		"max_tokens":16,
		"messages":[
			{"role":"user","content":"` + strings.Repeat("old-history-", 100) + `"},
			{"role":"assistant","content":"done"},
			{"role":"user","content":"current-turn"}
		],
		"stream":false
	}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: openAICompatSSECompletedResponse("resp_fable_control", "gpt-5.6")}
	svc := &OpenAIGatewayService{
		httpUpstream: upstream,
		cfg: &config.Config{
			Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}},
			Gateway: config.GatewayConfig{
				AnthropicBridgeAutoCompactEnabled:        true,
				AnthropicBridgeAutoCompactInputBytes:     1,
				AnthropicBridgeAutoCompactTimeoutSeconds: 60,
			},
		},
	}

	result, err := svc.ForwardAsAnthropic(
		context.Background(), c, openAIAnthropicAutoCompactTestAccount(), body, "session-key", "gpt-5.6",
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.requests, 1)
	require.Equal(t, chatgptCodexURL, upstream.requests[0].URL.String())
	require.NotContains(t, string(upstream.bodies[0]), `"type":"compaction"`)
}

func openAIAnthropicAutoCompactTestAccount() *Account {
	return &Account{
		ID:          77,
		Name:        "openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-acc",
		},
	}
}

func newAnthropicAutoCompactTestService(upstream HTTPUpstream, enabled bool) *OpenAIGatewayService {
	return &OpenAIGatewayService{
		httpUpstream: upstream,
		cfg: &config.Config{Gateway: config.GatewayConfig{
			AnthropicBridgeAutoCompactEnabled:        enabled,
			AnthropicBridgeAutoCompactInputBytes:     1,
			AnthropicBridgeAutoCompactTimeoutSeconds: 60,
		}},
	}
}
