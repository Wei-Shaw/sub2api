//go:build unit

package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestGrokChatResponsesBridgeEligibility(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		body   string
		want   bool
		reason string
	}{
		{
			name: "plain text chat",
			body: `{"model":"grok","messages":[{"role":"system","content":"concise"},{"role":"user","content":"hi"}],"stream":false}`,
			want: true,
		},
		{
			name: "safe generation options",
			body: `{"model":"grok","messages":[{"role":"user","content":"hi"}],"stream":true,"stream_options":{"include_usage":true},"max_completion_tokens":256,"temperature":0.2,"top_p":0.9,"prompt_cache_key":"session","tools":[],"functions":null,"tool_choice":"none"}`,
			want: true,
		},
		{
			name: "Trae compatible fields",
			body: `{"model":"grok","messages":[{"role":"user","content":"return json"}],"instructions":"Be concise","response_format":{"type":"json_object"},"service_tier":"fast","stop":null,"reasoning_effort":null}`,
			want: true,
		},
		{
			name: "Trae nullable SDK fields",
			body: `{"model":"grok","messages":[{"role":"user","content":"hi"}],"instructions":null,"response_format":null,"service_tier":null,"stop":null,"reasoning_effort":null}`,
			want: true,
		},
		{
			name:   "invalid instructions fall back",
			body:   `{"model":"grok","messages":[{"role":"user","content":"hi"}],"instructions":{"text":"invalid"}}`,
			reason: "invalid_instructions",
		},
		{
			name:   "invalid response format falls back",
			body:   `{"model":"grok","messages":[{"role":"user","content":"hi"}],"response_format":"json_object"}`,
			reason: "invalid_response_format",
		},
		{
			name:   "invalid service tier falls back",
			body:   `{"model":"grok","messages":[{"role":"user","content":"hi"}],"service_tier":1}`,
			reason: "invalid_service_tier",
		},
		{
			name:   "stop falls back",
			body:   `{"model":"grok","messages":[{"role":"user","content":"hi"}],"stop":"done"}`,
			reason: "unsupported_stop",
		},
		{
			name:   "developer role falls back",
			body:   `{"model":"grok","messages":[{"role":"developer","content":"rules"},{"role":"user","content":"hi"}]}`,
			reason: "unsupported_message_role_developer",
		},
		{
			name: "image content is bridgeable",
			body: `{"model":"grok","messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"data:image/png;base64,QQ=="}}]}]}`,
			want: true,
		},
		{
			name: "text and image parts are bridgeable",
			body: `{"model":"grok","messages":[{"role":"user","content":[{"type":"text","text":"what is this"},{"type":"image_url","image_url":{"url":"data:image/png;base64,QQ=="}}]}]}`,
			want: true,
		},
		{
			name: "text only parts are bridgeable",
			body: `{"model":"grok","messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}]}`,
			want: true,
		},
		{
			name:   "unknown content part falls back",
			body:   `{"model":"grok","messages":[{"role":"user","content":[{"type":"input_audio","input_audio":{"data":"AA=="}}]}]}`,
			reason: "unsupported_content_part_input_audio",
		},
		{
			name:   "empty content array falls back",
			body:   `{"model":"grok","messages":[{"role":"user","content":[]}]}`,
			reason: "empty_message_content",
		},
		{
			name: "function tools bridge",
			body: `{"model":"grok","messages":[{"role":"user","content":"hi"}],"tools":[{"type":"function","function":{"name":"lookup","parameters":{"type":"object"},"strict":false}}]}`,
			want: true,
		},
		{
			name:   "legacy functions fall back",
			body:   `{"model":"grok","messages":[{"role":"user","content":"hi"}],"functions":[{"name":"lookup","parameters":{"type":"object"}}]}`,
			reason: "unsupported_functions",
		},
		{
			name: "automatic tool choice bridges",
			body: `{"model":"grok","messages":[{"role":"user","content":"hi"}],"tools":[],"tool_choice":"auto"}`,
			want: true,
		},
		{
			name:   "required tool choice without tools falls back",
			body:   `{"model":"grok","messages":[{"role":"user","content":"hi"}],"tools":[],"tool_choice":"required"}`,
			reason: "required_tool_choice_without_tools",
		},
		{
			name: "tool history bridges",
			body: `{"model":"grok","messages":[{"role":"assistant","content":null,"tool_calls":[{"id":"call_lookup","type":"function","function":{"name":"lookup","arguments":"{\"key\":\"alpha\"}"}}]},{"role":"tool","tool_call_id":"call_lookup","content":"{\"value\":\"ok\"}"},{"role":"user","content":"summarize"}],"tools":[{"type":"function","function":{"name":"lookup","parameters":{"type":"object"}}}],"tool_choice":"auto","parallel_tool_calls":true}`,
			want: true,
		},
		{
			name: "Trae reasoning and indexed tool history bridges",
			body: `{"model":"grok","messages":[{"role":"assistant","content":null,"reasoning_content":"I should call lookup","tool_calls":[{"index":0,"id":"call_lookup","type":"function","function":{"name":"lookup","arguments":"{\"key\":\"alpha\"}"}}]},{"role":"tool","tool_call_id":"call_lookup","content":"{\"value\":\"ok\"}"},{"role":"user","content":"summarize"}],"tools":[{"type":"function","function":{"name":"lookup","parameters":{"type":"object"}}}]}`,
			want: true,
		},
		{
			name: "reasoning only assistant history bridges",
			body: `{"model":"grok","messages":[{"role":"assistant","reasoning_content":"Prior reasoning"},{"role":"user","content":"continue"}]}`,
			want: true,
		},
		{
			name:   "invalid reasoning content falls back",
			body:   `{"model":"grok","messages":[{"role":"assistant","reasoning_content":{"text":"invalid"}},{"role":"user","content":"continue"}]}`,
			reason: "invalid_reasoning_content",
		},
		{
			name:   "negative tool call index falls back",
			body:   `{"model":"grok","messages":[{"role":"assistant","content":null,"tool_calls":[{"index":-1,"id":"call_lookup","type":"function","function":{"name":"lookup","arguments":"{}"}}]}]}`,
			reason: "invalid_tool_call_index",
		},
		{
			name:   "unknown tool type falls back",
			body:   `{"model":"grok","messages":[{"role":"user","content":"hi"}],"tools":[{"type":"web_search","function":{"name":"lookup","parameters":{"type":"object"}}}]}`,
			reason: "unsupported_tool_type",
		},
		{
			name:   "missing tool schema falls back",
			body:   `{"model":"grok","messages":[{"role":"user","content":"hi"}],"tools":[{"type":"function","function":{"name":"lookup"}}]}`,
			reason: "invalid_tool_function_parameters",
		},
		{
			name:   "named tool choice falls back",
			body:   `{"model":"grok","messages":[{"role":"user","content":"hi"}],"tools":[{"type":"function","function":{"name":"lookup","parameters":{"type":"object"}}}],"tool_choice":{"type":"function","function":{"name":"lookup"}}}`,
			reason: "unsupported_tool_choice",
		},
		{
			name:   "invalid tool call arguments fall back",
			body:   `{"model":"grok","messages":[{"role":"assistant","content":null,"tool_calls":[{"id":"call_lookup","type":"function","function":{"name":"lookup","arguments":"{"}}]}]}`,
			reason: "invalid_tool_call_arguments",
		},
		{
			name:   "tool result without call id falls back",
			body:   `{"model":"grok","messages":[{"role":"tool","content":"ok"}]}`,
			reason: "invalid_tool_call_id",
		},
		{
			name:   "non boolean parallel tool calls falls back",
			body:   `{"model":"grok","messages":[{"role":"user","content":"hi"}],"parallel_tool_calls":"true"}`,
			reason: "invalid_parallel_tool_calls",
		},
		{
			name:   "reasoning effort falls back because conversion adds summary",
			body:   `{"model":"grok","messages":[{"role":"user","content":"hi"}],"reasoning_effort":"high"}`,
			reason: "unsupported_reasoning_effort",
		},
		{
			name:   "both token limits fall back",
			body:   `{"model":"grok","messages":[{"role":"user","content":"hi"}],"max_tokens":256,"max_completion_tokens":256}`,
			reason: "conflicting_max_tokens",
		},
		{
			name:   "empty message falls back",
			body:   `{"model":"grok","messages":[{"role":"assistant","content":""},{"role":"user","content":"hi"}]}`,
			reason: "empty_message_content",
		},
		{
			name:   "empty tool history falls back",
			body:   `{"model":"grok","messages":[{"role":"assistant","content":"","tool_calls":[]}]}`,
			reason: "empty_message_content",
		},
		{
			name:   "unknown field falls back",
			body:   `{"model":"grok","messages":[{"role":"user","content":"hi"}],"seed":7}`,
			reason: "unknown_field_seed",
		},
		{
			name:   "small max tokens falls back because conversion clamps it",
			body:   `{"model":"grok","messages":[{"role":"user","content":"hi"}],"max_tokens":32}`,
			reason: "unsafe_max_tokens",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, reason := grokChatResponsesBridgeEligibility([]byte(tt.body))
			require.Equal(t, tt.want, got)
			require.Equal(t, tt.reason, reason)
		})
	}
}

func TestGrokChatResponsesRuntimeEligibility(t *testing.T) {
	t.Parallel()
	require.True(t, grokChatResponsesRuntimeEligible("grok-4.5", "isolated-id"))
	require.False(t, grokChatResponsesRuntimeEligible("grok-4.3", "isolated-id"))
	require.False(t, grokChatResponsesRuntimeEligible("grok-4.5-build-free", "isolated-id"))
	require.False(t, grokChatResponsesRuntimeEligible("grok-4.5", ""))
}

func TestForwardGrokChatViaResponsesNonStreamingCachesAndReturnsChat(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"grok","messages":[{"role":"system","content":"be concise"},{"role":"user","content":"hi"}],"stream":false,"prompt_cache_key":"stable-session","tools":[],"functions":null,"tool_choice":"none"}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, grokChatRawEndpoint, bytes.NewReader(body))
	c.Set("api_key", &APIKey{ID: 7101})

	account := grokChatBridgeTestAccount(71)
	repo := &grokQuotaAccountRepo{mockAccountRepoForPlatform: &mockAccountRepoForPlatform{
		accountsByID: map[int64]*Account{account.ID: account},
	}}
	upstream := &httpUpstreamRecorder{resp: grokChatBridgeCompletedResponse("resp_grok_chat_cache", 9856)}
	svc := &OpenAIGatewayService{
		httpUpstream:      upstream,
		grokTokenProvider: NewGrokTokenProvider(repo, nil),
		accountRepo:       repo,
	}

	result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, xai.DefaultCLIBaseURL+"/responses", upstream.lastReq.URL.String())
	require.Equal(t, grokChatResponsesEndpoint, result.UpstreamEndpoint)
	require.Equal(t, "grok-4.5", result.UpstreamModel)
	require.Equal(t, 9908, result.Usage.InputTokens)
	require.Equal(t, 12, result.Usage.OutputTokens)
	require.Equal(t, 9856, result.Usage.CacheReadInputTokens)

	identity := gjson.GetBytes(upstream.lastBody, "prompt_cache_key").String()
	require.NotEmpty(t, identity)
	require.NotEqual(t, "stable-session", identity)
	require.Equal(t, identity, upstream.lastReq.Header.Get(grokConversationIDHeader))
	require.Equal(t, "web_search", gjson.GetBytes(upstream.lastBody, "tools.0.type").String())
	require.Equal(t, "x_search", gjson.GetBytes(upstream.lastBody, "tools.1.type").String())
	require.Equal(t, grokFreeCacheDisabledToolChoice, gjson.GetBytes(upstream.lastBody, "tool_choice").String())
	require.True(t, gjson.GetBytes(upstream.lastBody, "stream").Bool())
	require.Equal(t, "system", gjson.GetBytes(upstream.lastBody, "input.0.role").String())
	require.Equal(t, "user", gjson.GetBytes(upstream.lastBody, "input.1.role").String())
	require.False(t, gjson.GetBytes(upstream.lastBody, "instructions").Exists())
	require.False(t, gjson.GetBytes(upstream.lastBody, "include").Exists())
	require.False(t, gjson.GetBytes(upstream.lastBody, "store").Exists())

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "cached ok", gjson.Get(recorder.Body.String(), "choices.0.message.content").String())
	require.Equal(t, int64(9856), gjson.Get(recorder.Body.String(), "usage.prompt_tokens_details.cached_tokens").Int())
	require.NotNil(t, repo.updates[account.ID][grokQuotaSnapshotExtraKey])
}

func TestForwardGrokChatViaResponsesTraeToolHistoryKeepsCacheRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)

	firstTurnBody := []byte(`{"model":"grok","messages":[{"role":"system","content":"Be concise"},{"role":"user","content":"Find alpha"}],"stream":false,"prompt_cache_key":"trae-session","tools":[{"type":"function","function":{"name":"lookup","description":"Lookup a value","parameters":{"type":"object","properties":{"key":{"type":"string"}},"required":["key"]},"strict":false}}],"tool_choice":"auto","parallel_tool_calls":true}`)
	body := []byte(`{"model":"grok","messages":[{"role":"system","content":"Be concise"},{"role":"user","content":"Find alpha"},{"role":"assistant","content":null,"tool_calls":[{"id":"call_lookup","type":"function","function":{"name":"lookup","arguments":"{\"key\":\"alpha\"}"}}]},{"role":"tool","tool_call_id":"call_lookup","content":"{\"value\":\"ok\"}"},{"role":"user","content":"Summarize"}],"stream":false,"prompt_cache_key":"trae-session","tools":[{"type":"function","function":{"name":"lookup","description":"Lookup a value","parameters":{"type":"object","properties":{"key":{"type":"string"}},"required":["key"]},"strict":false}}],"tool_choice":"auto","parallel_tool_calls":true}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, grokChatRawEndpoint, bytes.NewReader(body))
	c.Request.Header.Set(grokClientToolCacheOptInHeader, "prefer-cache")
	c.Set("api_key", &APIKey{ID: 7151})

	account := grokChatBridgeTestAccount(715)
	account.Credentials["subscription_tier"] = "free"
	repo := &grokQuotaAccountRepo{mockAccountRepoForPlatform: &mockAccountRepoForPlatform{
		accountsByID: map[int64]*Account{account.ID: account},
	}}
	upstream := &httpUpstreamRecorder{resp: grokChatBridgeCompletedResponse("resp_grok_chat_trae", 8192)}
	svc := &OpenAIGatewayService{
		httpUpstream:      upstream,
		grokTokenProvider: NewGrokTokenProvider(repo, nil),
		accountRepo:       repo,
	}

	firstTurnIdentity := resolveGrokCacheIdentity(c, firstTurnBody, "", "grok-4.5")
	extendedTurnIdentity := resolveGrokCacheIdentity(c, body, "", "grok-4.5")
	require.NotEmpty(t, firstTurnIdentity)
	require.Equal(t, firstTurnIdentity, extendedTurnIdentity)

	result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, xai.DefaultCLIBaseURL+"/responses", upstream.lastReq.URL.String())
	require.Equal(t, grokChatResponsesEndpoint, result.UpstreamEndpoint)
	require.Equal(t, extendedTurnIdentity, gjson.GetBytes(upstream.lastBody, "prompt_cache_key").String())
	require.Equal(t, extendedTurnIdentity, upstream.lastReq.Header.Get(grokConversationIDHeader))

	tools := gjson.GetBytes(upstream.lastBody, "tools").Array()
	require.Len(t, tools, 3)
	require.Equal(t, "function", tools[0].Get("type").String())
	require.Equal(t, "lookup", tools[0].Get("name").String())
	require.Equal(t, "Lookup a value", tools[0].Get("description").String())
	require.Equal(t, "string", tools[0].Get("parameters.properties.key.type").String())
	require.True(t, tools[0].Get("strict").Exists())
	require.False(t, tools[0].Get("strict").Bool())
	require.Equal(t, "web_search", tools[1].Get("type").String())
	require.Equal(t, "x_search", tools[2].Get("type").String())
	require.Equal(t, "auto", gjson.GetBytes(upstream.lastBody, "tool_choice").String())
	require.True(t, gjson.GetBytes(upstream.lastBody, "parallel_tool_calls").Bool())

	require.Equal(t, "function_call", gjson.GetBytes(upstream.lastBody, "input.2.type").String())
	require.Equal(t, "call_lookup", gjson.GetBytes(upstream.lastBody, "input.2.call_id").String())
	require.Equal(t, "lookup", gjson.GetBytes(upstream.lastBody, "input.2.name").String())
	require.Equal(t, `{"key":"alpha"}`, gjson.GetBytes(upstream.lastBody, "input.2.arguments").String())
	require.Equal(t, "function_call_output", gjson.GetBytes(upstream.lastBody, "input.3.type").String())
	require.Equal(t, "call_lookup", gjson.GetBytes(upstream.lastBody, "input.3.call_id").String())
	require.Equal(t, `{"value":"ok"}`, gjson.GetBytes(upstream.lastBody, "input.3.output").String())
}

func TestForwardGrokChatViaResponsesTraeCompatibilityFieldsKeepCacheRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)

	firstTurnBody := []byte(`{"model":"grok","messages":[{"role":"user","content":"Find alpha"}],"instructions":"Return concise JSON","stream":false,"response_format":{"type":"json_object"},"service_tier":"fast","stop":null,"reasoning_effort":null,"tools":[{"type":"function","function":{"name":"lookup","description":"Lookup a value","parameters":{"type":"object","properties":{"key":{"type":"string"}},"required":["key"]}}}],"tool_choice":"auto","parallel_tool_calls":true}`)
	body := []byte(`{"model":"grok","messages":[{"role":"user","content":"Find alpha"},{"role":"assistant","content":null,"reasoning_content":"I should use lookup","tool_calls":[{"index":0,"id":"call_lookup","type":"function","function":{"name":"lookup","arguments":"{\"key\":\"alpha\"}"}}]},{"role":"tool","tool_call_id":"call_lookup","content":"{\"value\":\"ok\"}"},{"role":"user","content":"Summarize"}],"instructions":"Return concise JSON","stream":false,"response_format":{"type":"json_object"},"service_tier":"fast","stop":null,"reasoning_effort":null,"tools":[{"type":"function","function":{"name":"lookup","description":"Lookup a value","parameters":{"type":"object","properties":{"key":{"type":"string"}},"required":["key"]}}}],"tool_choice":"auto","parallel_tool_calls":true}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, grokChatRawEndpoint, bytes.NewReader(body))
	c.Set("api_key", &APIKey{ID: 7161})

	account := grokChatBridgeTestAccount(716)
	account.Credentials["subscription_tier"] = "free"
	repo := &grokQuotaAccountRepo{mockAccountRepoForPlatform: &mockAccountRepoForPlatform{
		accountsByID: map[int64]*Account{account.ID: account},
	}}
	upstream := &httpUpstreamRecorder{resp: grokChatBridgeCompletedResponse("resp_grok_chat_trae_compat", 12288)}
	svc := &OpenAIGatewayService{
		httpUpstream:      upstream,
		grokTokenProvider: NewGrokTokenProvider(repo, nil),
		accountRepo:       repo,
	}

	firstTurnIdentity := resolveGrokCacheIdentity(c, firstTurnBody, "", "grok-4.5")
	extendedTurnIdentity := resolveGrokCacheIdentity(c, body, "", "grok-4.5")
	require.NotEmpty(t, firstTurnIdentity)
	require.Equal(t, firstTurnIdentity, extendedTurnIdentity)

	result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, xai.DefaultCLIBaseURL+"/responses", upstream.lastReq.URL.String())
	require.Equal(t, grokChatResponsesEndpoint, result.UpstreamEndpoint)
	require.Equal(t, 12288, result.Usage.CacheReadInputTokens)
	require.Equal(t, extendedTurnIdentity, gjson.GetBytes(upstream.lastBody, "prompt_cache_key").String())
	require.Equal(t, extendedTurnIdentity, upstream.lastReq.Header.Get(grokConversationIDHeader))
	require.Equal(t, "Return concise JSON", gjson.GetBytes(upstream.lastBody, "instructions").String())
	require.Equal(t, "json_object", gjson.GetBytes(upstream.lastBody, "text.format.type").String())
	require.Equal(t, "priority", gjson.GetBytes(upstream.lastBody, "service_tier").String())
	require.False(t, gjson.GetBytes(upstream.lastBody, "stop").Exists())
	require.False(t, gjson.GetBytes(upstream.lastBody, "reasoning").Exists())
	require.Contains(t, gjson.GetBytes(upstream.lastBody, "input.1.content.0.text").String(), "<thinking>I should use lookup</thinking>")
	require.Equal(t, "function_call", gjson.GetBytes(upstream.lastBody, "input.2.type").String())
	require.Equal(t, "function_call_output", gjson.GetBytes(upstream.lastBody, "input.3.type").String())

	tools := gjson.GetBytes(upstream.lastBody, "tools").Array()
	require.Len(t, tools, 3)
	require.Equal(t, "lookup", tools[0].Get("name").String())
	require.Equal(t, "web_search", tools[1].Get("type").String())
	require.Equal(t, "x_search", tools[2].Get("type").String())
	require.Equal(t, int64(12288), gjson.Get(recorder.Body.String(), "usage.prompt_tokens_details.cached_tokens").Int())
}

func TestForwardGrokChatViaResponsesStreamingPropagatesCachedUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"grok","messages":[{"role":"user","content":"hi"}],"stream":true}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, grokChatRawEndpoint, bytes.NewReader(body))
	c.Set("api_key", &APIKey{ID: 7201})

	account := grokChatBridgeTestAccount(72)
	repo := &grokQuotaAccountRepo{mockAccountRepoForPlatform: &mockAccountRepoForPlatform{
		accountsByID: map[int64]*Account{account.ID: account},
	}}
	upstream := &httpUpstreamRecorder{resp: grokChatBridgeCompletedResponse("resp_grok_chat_stream", 4096)}
	svc := &OpenAIGatewayService{
		httpUpstream:      upstream,
		grokTokenProvider: NewGrokTokenProvider(repo, nil),
		accountRepo:       repo,
	}

	result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Stream)
	require.Equal(t, grokChatResponsesEndpoint, result.UpstreamEndpoint)
	require.Equal(t, 4096, result.Usage.CacheReadInputTokens)
	require.Contains(t, recorder.Header().Get("Content-Type"), "text/event-stream")
	require.Contains(t, recorder.Body.String(), `"content":"cached ok"`)
	require.Contains(t, recorder.Body.String(), `"cached_tokens":4096`)
	require.Contains(t, recorder.Body.String(), "data: [DONE]")
}

func TestForwardGrokChatRuntimeGateFallsBackToRaw(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name         string
		setAPIKey    bool
		mappedModel  string
		wantUpstream string
	}{
		{name: "missing cache identity", wantUpstream: "grok-4.5"},
		{name: "non cache capable mapped model", setAPIKey: true, mappedModel: "grok-4.3", wantUpstream: "grok-4.3"},
	}

	for index, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := []byte(`{"model":"grok","messages":[{"role":"user","content":"hi"}],"stream":false}`)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, grokChatRawEndpoint, bytes.NewReader(body))
			if tt.setAPIKey {
				c.Set("api_key", &APIKey{ID: int64(7301 + index)})
			}

			account := grokChatBridgeTestAccount(int64(73 + index))
			if tt.mappedModel != "" {
				account.Credentials["model_mapping"] = map[string]any{"grok": tt.mappedModel}
			}
			repo := &grokQuotaAccountRepo{mockAccountRepoForPlatform: &mockAccountRepoForPlatform{
				accountsByID: map[int64]*Account{account.ID: account},
			}}
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body: io.NopCloser(strings.NewReader(
					`{"id":"chat_raw","object":"chat.completion","model":"` + tt.wantUpstream + `","choices":[{"index":0,"message":{"role":"assistant","content":"raw ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`,
				)),
			}}
			svc := &OpenAIGatewayService{
				httpUpstream:      upstream,
				grokTokenProvider: NewGrokTokenProvider(repo, nil),
				accountRepo:       repo,
			}

			result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, xai.DefaultCLIBaseURL+"/chat/completions", upstream.lastReq.URL.String())
			require.Equal(t, grokChatRawEndpoint, result.UpstreamEndpoint)
			require.Equal(t, tt.wantUpstream, result.UpstreamModel)
			require.False(t, gjson.GetBytes(upstream.lastBody, "tools").Exists())
			require.Equal(t, "raw ok", gjson.Get(recorder.Body.String(), "choices.0.message.content").String())
		})
	}
}

func TestForwardGrokChatViaResponses429UsesGrokRateLimitPolicy(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"grok","messages":[{"role":"user","content":"hi"}],"stream":false}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, grokChatRawEndpoint, bytes.NewReader(body))
	c.Set("api_key", &APIKey{ID: 7501})

	account := grokChatBridgeTestAccount(75)
	repo := &grokQuotaAccountRepo{mockAccountRepoForPlatform: &mockAccountRepoForPlatform{
		accountsByID: map[int64]*Account{account.ID: account},
	}}
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header: http.Header{
			"Content-Type": []string{"application/json"},
			"Retry-After":  []string{"45"},
		},
		Body: io.NopCloser(strings.NewReader(`{"error":{"message":"rate limited"}}`)),
	}}
	svc := &OpenAIGatewayService{
		httpUpstream:      upstream,
		grokTokenProvider: NewGrokTokenProvider(repo, nil),
		accountRepo:       repo,
	}
	before := time.Now()

	result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
	require.Error(t, err)
	require.Nil(t, result)
	var failoverErr *UpstreamFailoverError
	require.True(t, errors.As(err, &failoverErr))
	require.Equal(t, http.StatusTooManyRequests, failoverErr.StatusCode)
	require.Equal(t, "45", failoverErr.ResponseHeaders.Get("Retry-After"))
	require.Equal(t, xai.DefaultCLIBaseURL+"/responses", upstream.lastReq.URL.String())
	require.Equal(t, grokChatResponsesEndpoint, GetActualOpenAIUpstreamEndpoint(c))
	require.Equal(t, 1, repo.rateLimitedCalls)
	require.Zero(t, repo.tempUnschedCalls)
	require.WithinDuration(t, before.Add(45*time.Second), repo.lastRateLimitResetAt, time.Second)
	require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))
}

func TestForwardGrokRawChat429PreservesRetryAfter(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"grok","messages":[{"role":"user","content":"hi"}],"stream":false,"stop":"done"}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, grokChatRawEndpoint, bytes.NewReader(body))
	c.Set("api_key", &APIKey{ID: 7551})

	account := grokChatBridgeTestAccount(755)
	account.Credentials["expires_at"] = time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339)
	repo := &grokQuotaAccountRepo{mockAccountRepoForPlatform: &mockAccountRepoForPlatform{
		accountsByID: map[int64]*Account{account.ID: account},
	}}
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header: http.Header{
			"Content-Type": []string{"application/json"},
			"Retry-After":  []string{"45"},
		},
		Body: io.NopCloser(strings.NewReader(`{"error":{"message":"rate limited"}}`)),
	}}
	svc := &OpenAIGatewayService{
		httpUpstream:      upstream,
		grokTokenProvider: NewGrokTokenProvider(repo, nil),
		accountRepo:       repo,
	}

	result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")

	require.Error(t, err)
	require.Nil(t, result)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusTooManyRequests, failoverErr.StatusCode)
	require.Equal(t, "45", failoverErr.ResponseHeaders.Get("Retry-After"))
	require.Equal(t, xai.DefaultCLIBaseURL+"/chat/completions", upstream.lastReq.URL.String())
}

func TestForwardGrokRawChatErrorRecordsActualEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"grok","messages":[{"role":"user","content":"hi"}],"stream":false,"stop":"done"}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, grokChatRawEndpoint, bytes.NewReader(body))
	c.Set("api_key", &APIKey{ID: 7601})

	account := grokChatBridgeTestAccount(76)
	repo := &grokQuotaAccountRepo{mockAccountRepoForPlatform: &mockAccountRepoForPlatform{
		accountsByID: map[int64]*Account{account.ID: account},
	}}
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"bad request"}}`)),
	}}
	svc := &OpenAIGatewayService{
		httpUpstream:      upstream,
		grokTokenProvider: NewGrokTokenProvider(repo, nil),
		accountRepo:       repo,
	}

	result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
	require.Error(t, err)
	require.Nil(t, result)
	require.Equal(t, xai.DefaultCLIBaseURL+"/chat/completions", upstream.lastReq.URL.String())
	require.Equal(t, grokChatRawEndpoint, GetActualOpenAIUpstreamEndpoint(c))
}

func grokChatBridgeTestAccount(id int64) *Account {
	return &Account{
		ID:          id,
		Name:        "grok-cache-bridge",
		Platform:    PlatformGrok,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":  "access-token",
			"refresh_token": "refresh-token",
			"expires_at":    time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339),
			"base_url":      xai.DefaultCLIBaseURL,
		},
	}
}

func grokChatBridgeCompletedResponse(responseID string, cachedTokens int) *http.Response {
	body := strings.Join([]string{
		`data: {"type":"response.output_text.delta","sequence_number":0,"delta":"cached ok"}`,
		"",
		`data: {"type":"response.completed","sequence_number":1,"response":{"id":"` + responseID + `","object":"response","model":"grok-4.5","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"cached ok"}]}],"usage":{"input_tokens":9908,"output_tokens":12,"total_tokens":9920,"input_tokens_details":{"cached_tokens":` + strconv.Itoa(cachedTokens) + `}}}}`,
		"",
	}, "\n")
	return &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type":                   []string{"text/event-stream"},
			"Xai-Request-Id":                 []string{responseID + "-request"},
			"X-Ratelimit-Limit-Requests":     []string{"10"},
			"X-Ratelimit-Remaining-Requests": []string{"9"},
		},
		Body: io.NopCloser(strings.NewReader(body)),
	}
}
