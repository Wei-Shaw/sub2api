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

func newOpenAIGPT56PromptCacheTestContext(t *testing.T, body []byte, apiKeyID int64) *gin.Context {
	t.Helper()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("api_key", &APIKey{ID: apiKeyID})
	return c
}

func openAIGPT56PromptCacheTestService(upstream *httpUpstreamRecorder) *OpenAIGatewayService {
	return &OpenAIGatewayService{
		httpUpstream: upstream,
		cfg: &config.Config{
			Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}},
		},
	}
}

func openAIGPT56PromptCacheAPIKeyAccount(promptCacheMode string) *Account {
	account := &Account{
		ID:          1,
		Name:        "openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "https://api.openai.com/v1",
		},
	}
	if promptCacheMode != "" {
		account.Extra = map[string]any{OpenAIPromptCacheModeExtraKey: promptCacheMode}
	}
	return account
}

func openAIGPT56PromptCacheResponsesRequest(
	t *testing.T,
	prefix string,
	dynamicSuffix string,
	tools []apicompat.ResponsesTool,
) *apicompat.ResponsesRequest {
	t.Helper()
	prefixContent, err := json.Marshal([]apicompat.ResponsesContentPart{{
		Type: "input_text",
		Text: prefix,
		PromptCacheBreakpoint: &apicompat.ResponsesPromptCacheBreakpoint{
			Mode: "explicit",
		},
	}})
	require.NoError(t, err)
	suffixContent, err := json.Marshal([]apicompat.ResponsesContentPart{{
		Type: "input_text",
		Text: dynamicSuffix,
	}})
	require.NoError(t, err)
	input, err := json.Marshal([]apicompat.ResponsesInputItem{
		{Type: "message", Role: "developer", Content: prefixContent},
		{Type: "message", Role: "user", Content: suffixContent},
	})
	require.NoError(t, err)
	parallelToolCalls := true
	return &apicompat.ResponsesRequest{
		Input:             input,
		Tools:             tools,
		ParallelToolCalls: &parallelToolCalls,
		Reasoning:         &apicompat.ResponsesReasoning{Effort: "xhigh", Summary: "auto"},
		Text:              &apicompat.ResponsesText{Verbosity: "medium"},
		PromptCacheOptions: &apicompat.ResponsesPromptCacheOptions{
			Mode: "explicit",
		},
	}
}

func TestDeriveOpenAIResponsesPromptCacheKey_StablePrefixAndTenantPartitioning(t *testing.T) {
	tools := []apicompat.ResponsesTool{{
		Type:       "function",
		Name:       "read_file",
		Parameters: json.RawMessage(`{"type":"object"}`),
	}}
	first := openAIGPT56PromptCacheResponsesRequest(t, "stable project prefix", "first task", tools)
	second := openAIGPT56PromptCacheResponsesRequest(t, "stable project prefix", "second task", tools)

	firstKey, found, err := deriveOpenAIResponsesPromptCacheKey(77, "gpt-5.6-luna", first)
	require.NoError(t, err)
	require.True(t, found)
	secondKey, found, err := deriveOpenAIResponsesPromptCacheKey(77, "gpt-5.6-luna", second)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, firstKey, secondKey)
	require.Regexp(t, `^sub2api-pc-v1-[0-9a-f]{32}$`, firstKey)
	require.NotContains(t, firstKey, "stable project prefix")
	require.NotContains(t, firstKey, "gpt-5.6-luna")

	differentAPIKey, found, err := deriveOpenAIResponsesPromptCacheKey(78, "gpt-5.6-luna", first)
	require.NoError(t, err)
	require.True(t, found)
	require.NotEqual(t, firstKey, differentAPIKey)

	differentModel, found, err := deriveOpenAIResponsesPromptCacheKey(77, "gpt-5.6-luna-fast", first)
	require.NoError(t, err)
	require.True(t, found)
	require.NotEqual(t, firstKey, differentModel)

	differentTools := openAIGPT56PromptCacheResponsesRequest(t, "stable project prefix", "first task", []apicompat.ResponsesTool{{
		Type:       "function",
		Name:       "write_file",
		Parameters: json.RawMessage(`{"type":"object"}`),
	}})
	differentToolsKey, found, err := deriveOpenAIResponsesPromptCacheKey(77, "gpt-5.6-luna", differentTools)
	require.NoError(t, err)
	require.True(t, found)
	require.NotEqual(t, firstKey, differentToolsKey)

	differentPrefix := openAIGPT56PromptCacheResponsesRequest(t, "different project prefix", "first task", tools)
	differentPrefixKey, found, err := deriveOpenAIResponsesPromptCacheKey(77, "gpt-5.6-luna", differentPrefix)
	require.NoError(t, err)
	require.True(t, found)
	require.NotEqual(t, firstKey, differentPrefixKey)
}

func TestForwardAsAnthropic_GPT56CacheModesSharePromptPrefixWithoutSharingConversation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const apiKeyID int64 = 77
	firstBody := []byte(`{
		"model":"claude-sonnet-4-5",
		"max_tokens":16,
		"metadata":{"user_id":"{\"device_id\":\"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\",\"account_uuid\":\"\",\"session_id\":\"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa\"}"},
		"system":[{"type":"text","text":"stable project instructions","cache_control":{"type":"ephemeral","ttl":"1h"}}],
		"messages":[{"role":"user","content":[{"type":"text","text":"first task","cache_control":{"type":"ephemeral"}}]}],
		"stream":false
	}`)
	secondBody := []byte(`{
		"model":"claude-sonnet-4-5",
		"max_tokens":16,
		"metadata":{"user_id":"{\"device_id\":\"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb\",\"account_uuid\":\"\",\"session_id\":\"bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb\"}"},
		"system":[{"type":"text","text":"stable project instructions","cache_control":{"type":"ephemeral","ttl":"1h"}}],
		"messages":[{"role":"user","content":[{"type":"text","text":"second task","cache_control":{"type":"ephemeral"}}]}],
		"stream":false
	}`)

	tests := []struct {
		name                string
		mode                string
		wantBreakpoints     bool
		wantExplicitOptions bool
	}{
		{
			name: "stable key only",
			mode: OpenAIPromptCacheModeStableKeyOnly,
		},
		{
			name:            "implicit breakpoints",
			mode:            OpenAIPromptCacheModeImplicitBreakpoints,
			wantBreakpoints: true,
		},
		{
			name:                "explicit only",
			mode:                OpenAIPromptCacheModeExplicitOnly,
			wantBreakpoints:     true,
			wantExplicitOptions: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstream := &httpUpstreamRecorder{responses: []*http.Response{
				openAICompatSSECompletedResponse("resp_session_a", "gpt-5.6-luna"),
				openAICompatSSECompletedResponse("resp_session_b", "gpt-5.6-luna"),
			}}
			svc := openAIGPT56PromptCacheTestService(upstream)
			account := openAIGPT56PromptCacheAPIKeyAccount(tt.mode)

			first, err := svc.ForwardAsAnthropic(
				context.Background(),
				newOpenAIGPT56PromptCacheTestContext(t, firstBody, apiKeyID),
				account,
				firstBody,
				"",
				"gpt-5.6-luna",
			)
			require.NoError(t, err)
			require.NotNil(t, first)

			second, err := svc.ForwardAsAnthropic(
				context.Background(),
				newOpenAIGPT56PromptCacheTestContext(t, secondBody, apiKeyID),
				account,
				secondBody,
				"",
				"gpt-5.6-luna",
			)
			require.NoError(t, err)
			require.NotNil(t, second)
			require.Len(t, upstream.bodies, 2)
			require.Len(t, upstream.requests, 2)

			firstKey := gjson.GetBytes(upstream.bodies[0], "prompt_cache_key").String()
			secondKey := gjson.GetBytes(upstream.bodies[1], "prompt_cache_key").String()
			require.NotEmpty(t, firstKey)
			require.Equal(t, firstKey, secondKey)
			if tt.wantExplicitOptions {
				require.Equal(t, "explicit", gjson.GetBytes(upstream.bodies[0], "prompt_cache_options.mode").String())
			} else {
				require.False(t, gjson.GetBytes(upstream.bodies[0], "prompt_cache_options").Exists())
			}
			if tt.wantBreakpoints {
				require.Equal(t, "explicit", gjson.GetBytes(upstream.bodies[0], "input.0.content.0.prompt_cache_breakpoint.mode").String())
				require.False(t, gjson.GetBytes(upstream.bodies[0], "input.0.content.0.prompt_cache_breakpoint.ttl").Exists())
			} else {
				require.NotContains(t, string(upstream.bodies[0]), "prompt_cache_breakpoint")
			}
			if tt.mode == OpenAIPromptCacheModeStableKeyOnly {
				require.False(t, gjson.GetBytes(upstream.bodies[1], "prompt_cache_options").Exists())
				require.NotContains(t, string(upstream.bodies[1]), "prompt_cache_breakpoint")
			}
			firstSessionID := upstream.requests[0].Header.Get("session_id")
			secondSessionID := upstream.requests[1].Header.Get("session_id")
			require.NotEmpty(t, firstSessionID)
			require.NotEmpty(t, secondSessionID)
			require.NotEqual(t, firstSessionID, secondSessionID)
			require.False(t, gjson.GetBytes(upstream.bodies[1], "previous_response_id").Exists())
		})
	}
}

func TestForwardAsAnthropic_GPT56OffKeepsLegacyPromptIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{
		"model":"claude-sonnet-4-5",
		"max_tokens":16,
		"system":[{"type":"text","text":"stable project instructions","cache_control":{"type":"ephemeral"}}],
		"messages":[{"role":"user","content":"task"}],
		"stream":false
	}`)

	upstream := &httpUpstreamRecorder{resp: openAICompatSSECompletedResponse("resp_off", "gpt-5.6-luna")}
	svc := openAIGPT56PromptCacheTestService(upstream)
	result, err := svc.ForwardAsAnthropic(
		context.Background(),
		newOpenAIGPT56PromptCacheTestContext(t, body, 77),
		openAIGPT56PromptCacheAPIKeyAccount(OpenAIPromptCacheModeOff),
		body,
		"",
		"gpt-5.6-luna",
	)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, gjson.GetBytes(upstream.lastBody, "prompt_cache_options").Exists())
	require.NotContains(t, string(upstream.lastBody), "prompt_cache_breakpoint")
	legacyKey := gjson.GetBytes(upstream.lastBody, "prompt_cache_key").String()
	require.True(t, strings.HasPrefix(legacyKey, "anthropic-cache-"))
	require.False(t, strings.HasPrefix(legacyKey, openAIResponsesPromptCacheKeyPrefix))
	require.NotEmpty(t, upstream.lastReq.Header.Get("session_id"))
}

func TestForwardAsAnthropic_OffModesKeepExplicitIdentityForAnyAPIKeyModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{
		"model":"claude-sonnet-4-5",
		"max_tokens":16,
		"messages":[{"role":"user","content":"task"}],
		"stream":false
	}`)
	const explicitIdentity = "downstream-session-identity"
	tests := []struct {
		name string
		mode string
	}{
		{name: "missing"},
		{name: "off", mode: OpenAIPromptCacheModeOff},
		{name: "invalid", mode: "automatic"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstream := &httpUpstreamRecorder{resp: openAICompatSSECompletedResponse("resp_legacy_identity", "gpt-4o")}
			svc := openAIGPT56PromptCacheTestService(upstream)
			result, err := svc.ForwardAsAnthropic(
				context.Background(),
				newOpenAIGPT56PromptCacheTestContext(t, body, 77),
				openAIGPT56PromptCacheAPIKeyAccount(tt.mode),
				body,
				explicitIdentity,
				"gpt-4o",
			)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, explicitIdentity, gjson.GetBytes(upstream.lastBody, "prompt_cache_key").String())
			require.False(t, gjson.GetBytes(upstream.lastBody, "prompt_cache_options").Exists())
			require.NotContains(t, string(upstream.lastBody), "prompt_cache_breakpoint")
		})
	}
}

func TestForwardAsAnthropic_ExplicitPromptCacheModeAppliesToConfiguredMappedModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{
		"model":"claude-sonnet-4-5",
		"max_tokens":16,
		"system":[{"type":"text","text":"stable project instructions","cache_control":{"type":"ephemeral"}}],
		"messages":[{"role":"user","content":"task"}],
		"stream":false
	}`)

	upstream := &httpUpstreamRecorder{resp: openAICompatSSECompletedResponse("resp_explicit_config", "gpt-4o")}
	svc := openAIGPT56PromptCacheTestService(upstream)
	result, err := svc.ForwardAsAnthropic(
		context.Background(),
		newOpenAIGPT56PromptCacheTestContext(t, body, 77),
		openAIGPT56PromptCacheAPIKeyAccount(OpenAIPromptCacheModeExplicitOnly),
		body,
		"downstream-session-identity",
		"gpt-4o",
	)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, strings.HasPrefix(
		gjson.GetBytes(upstream.lastBody, "prompt_cache_key").String(),
		openAIResponsesPromptCacheKeyPrefix,
	))
	require.Equal(t, "explicit", gjson.GetBytes(upstream.lastBody, "prompt_cache_options.mode").String())
	require.Equal(t, "explicit", gjson.GetBytes(upstream.lastBody, "input.0.content.0.prompt_cache_breakpoint.mode").String())
}

func TestForwardAsAnthropic_DoesNotSendGPT56CacheFieldsToUnsupportedTargets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const bodyJSON = `{
		"model":"claude-sonnet-4-5",
		"max_tokens":16,
		"system":[{"type":"text","text":"stable project instructions","cache_control":{"type":"ephemeral"}}],
		"messages":[{"role":"user","content":"task"}],
		"stream":false
	}`

	tests := []struct {
		name    string
		account *Account
		model   string
	}{
		{
			name:    "API key without capability declaration",
			account: openAIGPT56PromptCacheAPIKeyAccount(""),
			model:   "gpt-5.5",
		},
		{
			name: "GPT-5.6 OAuth upstream",
			account: &Account{
				ID:          2,
				Name:        "openai-oauth",
				Platform:    PlatformOpenAI,
				Type:        AccountTypeOAuth,
				Concurrency: 1,
				Credentials: map[string]any{
					"access_token":       "oauth-token",
					"chatgpt_account_id": "chatgpt-account",
				},
				Extra: map[string]any{
					OpenAIPromptCacheModeExtraKey: OpenAIPromptCacheModeExplicitOnly,
				},
			},
			model: "gpt-5.6-luna",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := []byte(bodyJSON)
			upstream := &httpUpstreamRecorder{resp: openAICompatSSECompletedResponse("resp_negative", tt.model)}
			svc := openAIGPT56PromptCacheTestService(upstream)
			result, err := svc.ForwardAsAnthropic(
				context.Background(),
				newOpenAIGPT56PromptCacheTestContext(t, body, 88),
				tt.account,
				body,
				"",
				tt.model,
			)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.False(t, gjson.GetBytes(upstream.lastBody, "prompt_cache_options").Exists())
			require.False(t, gjson.GetBytes(upstream.lastBody, "input.0.content.0.prompt_cache_breakpoint").Exists())
		})
	}
}

func TestForwardAsAnthropic_GPT56UnsupportedMarkersKeepLegacyPromptIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{
		"model":"claude-sonnet-4-5",
		"max_tokens":16,
		"tools":[{"name":"shell","input_schema":{"type":"object"},"cache_control":{"type":"ephemeral"}}],
		"messages":[
			{"role":"assistant","content":[
				{"type":"text","text":"history","cache_control":{"type":"ephemeral"}},
				{"type":"tool_use","id":"toolu_1","name":"shell","input":{},"cache_control":{"type":"ephemeral"}}
			]},
			{"role":"user","content":[
				{"type":"tool_result","tool_use_id":"toolu_1","content":"done","cache_control":{"type":"ephemeral"}}
			]}
		],
		"stream":false
	}`)

	for _, mode := range []string{
		OpenAIPromptCacheModeStableKeyOnly,
		OpenAIPromptCacheModeExplicitOnly,
	} {
		t.Run(mode, func(t *testing.T) {
			upstream := &httpUpstreamRecorder{resp: openAICompatSSECompletedResponse("resp_unsupported", "gpt-5.6-luna")}
			svc := openAIGPT56PromptCacheTestService(upstream)
			result, err := svc.ForwardAsAnthropic(
				context.Background(),
				newOpenAIGPT56PromptCacheTestContext(t, body, 99),
				openAIGPT56PromptCacheAPIKeyAccount(mode),
				body,
				"",
				"gpt-5.6-luna",
			)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.False(t, gjson.GetBytes(upstream.lastBody, "prompt_cache_options").Exists())
			legacyKey := gjson.GetBytes(upstream.lastBody, "prompt_cache_key").String()
			require.True(t, strings.HasPrefix(legacyKey, "anthropic-cache-"))
			require.False(t, strings.HasPrefix(legacyKey, openAIResponsesPromptCacheKeyPrefix))
			require.NotContains(t, string(upstream.lastBody), "prompt_cache_breakpoint")
		})
	}
}

func TestForwardAsAnthropic_GPT55WithoutMetadataKeepsLegacyCacheIdentityAndContinuation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const apiKeyID int64 = 88
	firstBody := []byte(`{
		"model":"claude-sonnet-4-5",
		"max_tokens":16,
		"system":"stable project instructions",
		"messages":[{"role":"user","content":"first task"}],
		"stream":false
	}`)
	secondBody := []byte(`{
		"model":"claude-sonnet-4-5",
		"max_tokens":16,
		"system":"stable project instructions",
		"messages":[
			{"role":"user","content":"first task"},
			{"role":"assistant","content":"first result"},
			{"role":"user","content":"second task"}
		],
		"stream":false
	}`)

	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		openAICompatSSECompletedResponse("resp_legacy_first", "gpt-5.5"),
		openAICompatSSECompletedResponse("resp_legacy_second", "gpt-5.5"),
	}}
	svc := openAIGPT56PromptCacheTestService(upstream)
	account := openAIGPT56PromptCacheAPIKeyAccount(OpenAIPromptCacheModeExplicitOnly)

	first, err := svc.ForwardAsAnthropic(
		context.Background(),
		newOpenAIGPT56PromptCacheTestContext(t, firstBody, apiKeyID),
		account,
		firstBody,
		"",
		"gpt-5.5",
	)
	require.NoError(t, err)
	require.NotNil(t, first)

	second, err := svc.ForwardAsAnthropic(
		context.Background(),
		newOpenAIGPT56PromptCacheTestContext(t, secondBody, apiKeyID),
		account,
		secondBody,
		"",
		"gpt-5.5",
	)
	require.NoError(t, err)
	require.NotNil(t, second)
	require.Len(t, upstream.bodies, 2)
	require.Len(t, upstream.requests, 2)

	firstKey := gjson.GetBytes(upstream.bodies[0], "prompt_cache_key").String()
	secondKey := gjson.GetBytes(upstream.bodies[1], "prompt_cache_key").String()
	require.NotEmpty(t, firstKey)
	require.Equal(t, firstKey, secondKey)
	require.Equal(t, "resp_legacy_first", gjson.GetBytes(upstream.bodies[1], "previous_response_id").String())
	firstSessionID := upstream.requests[0].Header.Get("session_id")
	secondSessionID := upstream.requests[1].Header.Get("session_id")
	require.NotEmpty(t, firstSessionID)
	require.Equal(t, firstSessionID, secondSessionID)
	require.False(t, gjson.GetBytes(upstream.bodies[0], "prompt_cache_options").Exists())
	require.NotContains(t, string(upstream.bodies[0]), "prompt_cache_breakpoint")
}

func TestForwardAsAnthropic_CacheModesContinuationOmitsMarkersAndRetryRestoresStableKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const apiKeyID int64 = 79
	firstBody := []byte(`{
		"model":"claude-sonnet-4-5",
		"max_tokens":16,
		"metadata":{"user_id":"{\"device_id\":\"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc\",\"account_uuid\":\"\",\"session_id\":\"cccccccc-cccc-4ccc-8ccc-cccccccccccc\"}"},
		"system":[{"type":"text","text":"stable project instructions","cache_control":{"type":"ephemeral"}}],
		"messages":[{"role":"user","content":"first task"}],
		"stream":false
	}`)
	secondBody := []byte(`{
		"model":"claude-sonnet-4-5",
		"max_tokens":16,
		"metadata":{"user_id":"{\"device_id\":\"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc\",\"account_uuid\":\"\",\"session_id\":\"cccccccc-cccc-4ccc-8ccc-cccccccccccc\"}"},
		"system":[{"type":"text","text":"stable project instructions","cache_control":{"type":"ephemeral"}}],
		"messages":[
			{"role":"user","content":"first task"},
			{"role":"assistant","content":"first result"},
			{"role":"user","content":[{"type":"text","text":"second task","cache_control":{"type":"ephemeral"}}]}
		],
		"stream":false
	}`)

	tests := []struct {
		name            string
		mode            string
		wantBreakpoints bool
	}{
		{
			name: "stable key only",
			mode: OpenAIPromptCacheModeStableKeyOnly,
		},
		{
			name:            "implicit breakpoints",
			mode:            OpenAIPromptCacheModeImplicitBreakpoints,
			wantBreakpoints: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstream := &httpUpstreamRecorder{responses: []*http.Response{
				openAICompatSSECompletedResponse("resp_cache_first", "gpt-5.6-luna"),
				{
					StatusCode: http.StatusBadRequest,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body: io.NopCloser(strings.NewReader(
						`{"error":{"code":"previous_response_not_found","message":"previous response not found"}}`,
					)),
				},
				openAICompatSSECompletedResponse("resp_cache_replayed", "gpt-5.6-luna"),
			}}
			svc := openAIGPT56PromptCacheTestService(upstream)
			account := openAIGPT56PromptCacheAPIKeyAccount(tt.mode)

			first, err := svc.ForwardAsAnthropic(
				context.Background(),
				newOpenAIGPT56PromptCacheTestContext(t, firstBody, apiKeyID),
				account,
				firstBody,
				"",
				"gpt-5.6-luna",
			)
			require.NoError(t, err)
			require.NotNil(t, first)

			second, err := svc.ForwardAsAnthropic(
				context.Background(),
				newOpenAIGPT56PromptCacheTestContext(t, secondBody, apiKeyID),
				account,
				secondBody,
				"",
				"gpt-5.6-luna",
			)
			require.NoError(t, err)
			require.NotNil(t, second)
			require.Len(t, upstream.bodies, 3)

			firstKey := gjson.GetBytes(upstream.bodies[0], "prompt_cache_key").String()
			require.True(t, strings.HasPrefix(firstKey, openAIResponsesPromptCacheKeyPrefix))
			require.False(t, gjson.GetBytes(upstream.bodies[0], "prompt_cache_options").Exists())
			if tt.wantBreakpoints {
				require.Equal(t, "explicit", gjson.GetBytes(upstream.bodies[0], "input.0.content.0.prompt_cache_breakpoint.mode").String())
			} else {
				require.NotContains(t, string(upstream.bodies[0]), "prompt_cache_breakpoint")
			}

			require.Equal(t, "resp_cache_first", gjson.GetBytes(upstream.bodies[1], "previous_response_id").String())
			require.False(t, gjson.GetBytes(upstream.bodies[1], "prompt_cache_key").Exists())
			require.False(t, gjson.GetBytes(upstream.bodies[1], "prompt_cache_options").Exists())
			require.NotContains(t, string(upstream.bodies[1]), "prompt_cache_breakpoint")

			require.False(t, gjson.GetBytes(upstream.bodies[2], "previous_response_id").Exists())
			require.Equal(t, firstKey, gjson.GetBytes(upstream.bodies[2], "prompt_cache_key").String())
			require.False(t, gjson.GetBytes(upstream.bodies[2], "prompt_cache_options").Exists())
			if tt.wantBreakpoints {
				require.Contains(t, string(upstream.bodies[2]), "prompt_cache_breakpoint")
			} else {
				require.NotContains(t, string(upstream.bodies[2]), "prompt_cache_breakpoint")
			}
		})
	}
}

func TestForwardAsAnthropic_GrokPrefersClaudeCodeSessionHeaderOverMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{
		"model":"grok",
		"max_tokens":16,
		"metadata":{"user_id":"{\"session_id\":\"metadata-session\"}"},
		"messages":[{"role":"user","content":"task"}],
		"stream":false
	}`)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set(claudeCodeSessionHeader, "header-session")
	var req apicompat.AnthropicRequest
	err := json.Unmarshal(body, &req)
	require.NoError(t, err)

	require.Equal(t, "header-session",
		resolveAnthropicForwardConversationIdentity(
			c,
			&Account{Platform: PlatformGrok},
			body,
			&req,
			"",
		),
	)
}
