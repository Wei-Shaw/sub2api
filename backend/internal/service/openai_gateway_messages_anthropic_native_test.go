//go:build unit

package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// 国产供应商原生 Anthropic 直通路径（api_protocol=anthropic）的 reasoning_effort 记录。
// 回归背景：该路径此前从不提取 Claude 协议的 output_config.effort，也不做
// thinking-enabled 兜底，导致 kimi/zhipu/deepseek 平台分组的 /v1/messages 请求
// usage_log.reasoning_effort 恒为 NULL。

func nativeAnthropicTestAccount() *Account {
	return &Account{
		ID:          702,
		Name:        "kimi-native",
		Platform:    PlatformKimi,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":      "sk-test",
			"api_protocol": APIProtocolAnthropic,
			"api_base_urls": map[string]any{
				APIProtocolAnthropic: "http://anthropic.example",
			},
		},
	}
}

func nativeAnthropicGLMTestAccount() *Account {
	account := nativeAnthropicTestAccount()
	account.Name = "zhipu-native"
	account.Platform = PlatformZhipu
	return account
}

func TestNativeAnthropicBoundReasoningEffortPolicy(testContext *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, stream := range []bool{false, true} {
		for _, test := range []struct {
			name       string
			platform   string
			maxEffort  string
			overLimit  string
			to         string
			bound      bool
			omitEffort bool
			wantEffort string
			requested  string
			deny       bool
		}{
			{name: "deepseek low to max", platform: PlatformDeepseek, to: "max", bound: true, wantEffort: "max"},
			{name: "kimi low to max", platform: PlatformKimi, to: "max", bound: true, wantEffort: "max"},
			{name: "zhipu low to max", platform: PlatformZhipu, to: "max", bound: true, wantEffort: "max"},
			{name: "minimax low to max", platform: PlatformMiniMax, to: "max", bound: true, wantEffort: "max"},
			{name: "mapped ceiling", platform: PlatformDeepseek, to: "max", maxEffort: "high", bound: true, wantEffort: "high"},
			{name: "mapping deny", platform: PlatformDeepseek, to: ReasoningEffortMappingDeny, bound: true, deny: true},
			{name: "ceiling deny", platform: PlatformDeepseek, to: "max", maxEffort: "high", overLimit: ReasoningEffortOverLimitDeny, bound: true, deny: true},
			{name: "unbound", platform: PlatformDeepseek, wantEffort: "low"},
			{name: "empty policy", platform: PlatformDeepseek, bound: true, wantEffort: "low"},
			{name: "original context effort", platform: PlatformDeepseek, to: "max", bound: true, requested: "minimal", wantEffort: "max"},
			{name: "non CN", platform: PlatformOpenCodeGo, to: "max", bound: true, wantEffort: "low"},
			{name: "no synthesis", platform: PlatformDeepseek, to: "max", maxEffort: "low", bound: true, omitEffort: true},
		} {
			testContext.Run(fmt.Sprintf("%s/stream=%v", test.name, stream), func(testContext *testing.T) {
				effortJSON := `,"output_config":{"effort":"low"}`
				if test.omitEffort {
					effortJSON = ""
				}
				body := []byte(fmt.Sprintf(`{"model":"deepseek-flash","max_tokens":32,"stream":%v,"messages":[{"role":"user","content":"hi"}]%s}`, stream, effortJSON))
				ctx := context.Background()
				if test.requested != "" {
					ctx = WithRequestedReasoningEffort(ctx, test.requested)
				}
				if test.bound {
					var mappings []ReasoningEffortMapping
					if test.to != "" {
						mappings = []ReasoningEffortMapping{{From: "low", To: test.to, MatchType: "exact", Model: "deepseek-flash"}}
					}
					ctx = WithOpenAIReasoningEffortPolicy(ctx, test.maxEffort, mappings, test.overLimit)
				}
				response := nativeAnthropicBufferedResponse()
				if stream {
					response = nativeAnthropicStreamResponse()
				}
				upstream := &httpUpstreamRecorder{resp: response}
				svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
				account := nativeAnthropicTestAccount()
				account.Platform = test.platform
				recorder := httptest.NewRecorder()
				ginContext, _ := gin.CreateTestContext(recorder)
				ginContext.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(string(body))).WithContext(ctx)
				result, err := svc.ForwardAsAnthropic(ctx, ginContext, account, body, "", "upstream-rewritten-model")
				if test.deny {
					require.Error(testContext, err)
					require.True(testContext, IsReasoningEffortPolicyDenied(err))
					require.Nil(testContext, result)
					require.Empty(testContext, upstream.requests)
					require.Equal(testContext, http.StatusForbidden, ginContext.Writer.Status())
					require.Equal(testContext, "error", gjson.GetBytes(recorder.Body.Bytes(), "type").String())
					require.Equal(testContext, "forbidden_error", gjson.GetBytes(recorder.Body.Bytes(), "error.type").String())
					return
				}
				require.NoError(testContext, err)
				require.NotNil(testContext, result)
				require.Equal(testContext, stream, result.Stream)
				require.Equal(testContext, "upstream-rewritten-model", gjson.GetBytes(upstream.lastBody, "model").String())
				require.Equal(testContext, test.wantEffort, gjson.GetBytes(upstream.lastBody, "output_config.effort").String())
				if test.omitEffort {
					require.Nil(testContext, result.RequestedReasoningEffort)
					require.Nil(testContext, result.ReasoningEffort)
				} else {
					wantRequested := test.requested
					if wantRequested == "" {
						wantRequested = "low"
					}
					require.NotNil(testContext, result.RequestedReasoningEffort)
					require.NotNil(testContext, result.ReasoningEffort)
					require.Equal(testContext, wantRequested, *result.RequestedReasoningEffort)
					require.Equal(testContext, test.wantEffort, *result.ReasoningEffort)
				}
			})
		}
	}
}

func TestNativeAnthropicGLM53OutboundReasoningEffortPolicy(testContext *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, stream := range []bool{false, true} {
		for _, test := range []struct {
			name          string
			effort        string
			thinking      string
			maxEffort     string
			overLimit     string
			mappings      []ReasoningEffortMapping
			unbound       bool
			requested     string
			wantEffort    string
			denyRequested string
			denyMax       string
			errorContains string
		}{
			{name: "max capped to high", effort: "max", maxEffort: "high", wantEffort: "high"},
			{name: "max capped to low", effort: "max", maxEffort: "low", wantEffort: "low"},
			{name: "max cap", effort: "max", maxEffort: "max", wantEffort: "max"},
			{name: "max capped to unsupported medium", effort: "max", maxEffort: "medium", denyRequested: "high", denyMax: "medium", errorContains: `does not support capped reasoning effort "medium"`},
			{name: "normalized medium exceeds medium cap", effort: "medium", maxEffort: "medium", denyRequested: "high", denyMax: "medium", errorContains: `does not support capped reasoning effort "medium"`},
			{name: "xhigh cap incompatible with max", effort: "max", maxEffort: "xhigh", denyRequested: "max", denyMax: "xhigh", errorContains: `does not support capped reasoning effort "xhigh"`},
			{name: "minimal cap incompatible with low", effort: "low", maxEffort: "minimal", denyRequested: "low", denyMax: "minimal", errorContains: `does not support capped reasoning effort "minimal"`},
			{
				name: "original medium mapped to low", effort: "medium", maxEffort: "medium", wantEffort: "low",
				mappings: []ReasoningEffortMapping{{From: "medium", To: "low", MatchType: "exact", Model: "public-glm"}},
			},
			{
				name: "original medium mapped to low before incompatible cap", effort: "medium", maxEffort: "minimal", denyRequested: "low", denyMax: "minimal",
				mappings: []ReasoningEffortMapping{{From: "medium", To: "low", MatchType: "exact", Model: "public-glm"}},
			},
			{
				name: "original medium mapping deny", effort: "medium", denyRequested: "medium",
				mappings: []ReasoningEffortMapping{{From: "medium", To: ReasoningEffortMappingDeny, MatchType: "exact", Model: "public-glm"}},
			},
			{
				name: "mapping capped to high", effort: "low", maxEffort: "high", wantEffort: "high",
				mappings: []ReasoningEffortMapping{{From: "low", To: "max", MatchType: "exact", Model: "public-glm"}},
			},
			{
				name: "chained mappings single hop", effort: "low", wantEffort: "high",
				mappings: []ReasoningEffortMapping{
					{From: "low", To: "medium", MatchType: "exact", Model: "public-glm"},
					{From: "medium", To: "max", MatchType: "exact", Model: "public-glm"},
				},
			},
			{
				name: "mapped value not denied a second time", effort: "low", wantEffort: "high",
				mappings: []ReasoningEffortMapping{
					{From: "low", To: "high", MatchType: "exact", Model: "public-glm"},
					{From: "high", To: ReasoningEffortMappingDeny, MatchType: "exact", Model: "public-glm"},
				},
			},
			{
				name: "normalized value not mapped again", effort: "medium", maxEffort: "high", wantEffort: "high",
				mappings: []ReasoningEffortMapping{{From: "high", To: ReasoningEffortMappingDeny, MatchType: "exact", Model: "public-glm"}},
			},
			{
				name: "mapping deny", effort: "low", denyRequested: "low",
				mappings: []ReasoningEffortMapping{{From: "low", To: ReasoningEffortMappingDeny, MatchType: "exact", Model: "public-glm"}},
			},
			{
				name: "mapping ceiling deny", effort: "low", maxEffort: "high", overLimit: ReasoningEffortOverLimitDeny, denyRequested: "max", denyMax: "high",
				mappings: []ReasoningEffortMapping{{From: "low", To: "max", MatchType: "exact", Model: "public-glm"}},
			},
			{name: "ceiling deny", effort: "max", maxEffort: "medium", overLimit: ReasoningEffortOverLimitDeny, denyRequested: "max", denyMax: "medium"},
			{name: "normalized ceiling deny", effort: "medium", maxEffort: "medium", overLimit: ReasoningEffortOverLimitDeny, denyRequested: "high", denyMax: "medium"},
			{name: "normalized xhigh ceiling deny", effort: "xhigh", maxEffort: "xhigh", overLimit: ReasoningEffortOverLimitDeny, denyRequested: "max", denyMax: "xhigh"},
			{name: "low remains supported below medium cap", effort: "low", maxEffort: "medium", wantEffort: "low"},
			{name: "unbound retains normalization", effort: "medium", unbound: true, wantEffort: "high"},
			{name: "empty policy retains normalization", effort: "medium", wantEffort: "high"},
			{name: "requested context preserved", effort: "max", requested: "minimal", maxEffort: "high", wantEffort: "high"},
			{name: "unspecified effort not synthesized", maxEffort: "medium"},
			{name: "unspecified effort not denied", maxEffort: "low", overLimit: ReasoningEffortOverLimitDeny},
			{
				name: "disabled thinking not mapped", thinking: "disabled", maxEffort: "medium", wantEffort: "low",
				mappings: []ReasoningEffortMapping{{From: "low", To: ReasoningEffortMappingDeny, MatchType: "exact", Model: "public-glm"}},
			},
			{name: "disabled thinking unbound", thinking: "disabled", unbound: true, wantEffort: "low"},
			{name: "disabled thinking incompatible cap", thinking: "disabled", maxEffort: "minimal", denyRequested: "low", denyMax: "minimal"},
			{name: "explicit medium overrides disabled thinking", thinking: "disabled", effort: "medium", wantEffort: "high"},
		} {
			testContext.Run(fmt.Sprintf("%s/stream=%v", test.name, stream), func(testContext *testing.T) {
				effortJSON := ""
				if test.effort != "" {
					effortJSON = fmt.Sprintf(`,"output_config":{"effort":%q}`, test.effort)
				}
				if test.thinking != "" {
					effortJSON += fmt.Sprintf(`,"thinking":{"type":%q}`, test.thinking)
				}
				body := []byte(fmt.Sprintf(`{"model":"public-glm","max_tokens":32,"stream":%v,"messages":[{"role":"user","content":"hi"}]%s}`, stream, effortJSON))
				ctx := context.Background()
				if test.requested != "" {
					ctx = WithRequestedReasoningEffort(ctx, test.requested)
				}
				if !test.unbound {
					ctx = WithOpenAIReasoningEffortPolicy(ctx, test.maxEffort, test.mappings, test.overLimit)
				}
				response := nativeAnthropicBufferedResponse()
				if stream {
					response = nativeAnthropicStreamResponse()
				}
				upstream := &httpUpstreamRecorder{resp: response}
				svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
				recorder := httptest.NewRecorder()
				ginContext, _ := gin.CreateTestContext(recorder)
				ginContext.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(string(body))).WithContext(ctx)
				result, err := svc.ForwardAsAnthropic(ctx, ginContext, nativeAnthropicGLMTestAccount(), body, "", "glm-5.3")
				if test.denyRequested != "" {
					require.Error(testContext, err)
					require.True(testContext, IsReasoningEffortPolicyDenied(err))
					if test.denyMax != "" {
						var denied *ReasoningEffortOverLimitError
						require.ErrorAs(testContext, err, &denied)
						require.Equal(testContext, test.denyRequested, denied.Requested)
						require.Equal(testContext, test.denyMax, denied.Max)
					} else {
						var denied *ReasoningEffortMappingDeniedError
						require.ErrorAs(testContext, err, &denied)
						require.Equal(testContext, test.denyRequested, denied.Requested)
					}
					if test.errorContains != "" {
						require.ErrorContains(testContext, err, test.errorContains)
					}
					require.Nil(testContext, result)
					require.Empty(testContext, upstream.requests)
					require.Equal(testContext, http.StatusForbidden, recorder.Code)
					require.Equal(testContext, "forbidden_error", gjson.GetBytes(recorder.Body.Bytes(), "error.type").String())
					require.Equal(testContext, err.Error(), gjson.GetBytes(recorder.Body.Bytes(), "error.message").String())
					return
				}
				require.NoError(testContext, err)
				require.NotNil(testContext, result)
				require.Len(testContext, upstream.requests, 1)
				require.Equal(testContext, http.MethodPost, upstream.lastReq.Method)
				require.Equal(testContext, "/api/anthropic/v1/messages", upstream.lastReq.URL.Path)
				require.Equal(testContext, "glm-5.3", gjson.GetBytes(upstream.lastBody, "model").String())
				require.Equal(testContext, stream, gjson.GetBytes(upstream.lastBody, "stream").Bool())
				require.Equal(testContext, test.wantEffort, gjson.GetBytes(upstream.lastBody, "output_config.effort").String())
				require.Equal(testContext, stream, result.Stream)
				require.Equal(testContext, "public-glm", result.Model)
				require.Equal(testContext, "glm-5.3", result.UpstreamModel)
				if test.effort == "" && test.thinking == "" {
					require.False(testContext, gjson.GetBytes(upstream.lastBody, "output_config.effort").Exists())
					require.False(testContext, gjson.GetBytes(upstream.lastBody, "thinking").Exists())
					require.Nil(testContext, result.RequestedReasoningEffort)
					require.Nil(testContext, result.ReasoningEffort)
					return
				}
				require.Equal(testContext, "enabled", gjson.GetBytes(upstream.lastBody, "thinking.type").String())
				wantRequested := test.effort
				if test.requested != "" {
					wantRequested = test.requested
				}
				require.NotNil(testContext, result.ReasoningEffort)
				if wantRequested == "" {
					require.Nil(testContext, result.RequestedReasoningEffort)
				} else {
					require.NotNil(testContext, result.RequestedReasoningEffort)
					require.Equal(testContext, wantRequested, *result.RequestedReasoningEffort)
				}
				require.Equal(testContext, test.wantEffort, *result.ReasoningEffort)
			})
		}
	}
}

func nativeAnthropicBufferedResponse() *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(
			`{"id":"msg_1","type":"message","role":"assistant","model":"k3",` +
				`"content":[{"type":"text","text":"pong"}],"stop_reason":"end_turn",` +
				`"usage":{"input_tokens":93,"output_tokens":16}}`,
		)),
	}
}

func nativeAnthropicStreamResponse() *http.Response {
	sse := `event: message_start
data: {"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"k3","content":[],"stop_reason":null,"usage":{"input_tokens":93,"output_tokens":1}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"pong"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":16}}

event: message_stop
data: {"type":"message_stop"}

`
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(sse)),
	}
}

func TestNativeAnthropicPassthroughRecordsOutputConfigEffort(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"k3","max_tokens":32,"stream":false,` +
		`"output_config":{"effort":"low"},` +
		`"messages":[{"role":"user","content":"hi"}]}`)
	upstream := &httpUpstreamRecorder{resp: nativeAnthropicBufferedResponse()}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}

	result, err := svc.ForwardAsAnthropic(context.Background(),
		adaptiveProtocolTestContext("/v1/messages", body), nativeAnthropicTestAccount(), body, "", "")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.ReasoningEffort)
	require.Equal(t, "low", *result.ReasoningEffort)
}

func TestNativeAnthropicPassthroughThinkingEnabledFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// 未显式传 effort，但 thinking 已启用：k3 属于 passback-required 白名单，应兜底记为 high。
	body := []byte(`{"model":"k3","max_tokens":32,"stream":false,` +
		`"thinking":{"type":"enabled","budget_tokens":1024},` +
		`"messages":[{"role":"user","content":"hi"}]}`)
	upstream := &httpUpstreamRecorder{resp: nativeAnthropicBufferedResponse()}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}

	result, err := svc.ForwardAsAnthropic(context.Background(),
		adaptiveProtocolTestContext("/v1/messages", body), nativeAnthropicTestAccount(), body, "", "")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.ReasoningEffort)
	require.Equal(t, "high", *result.ReasoningEffort)
}

func TestNativeAnthropicPassthroughStreamRecordsEffort(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"k3","max_tokens":32,"stream":true,` +
		`"output_config":{"effort":"max"},` +
		`"messages":[{"role":"user","content":"hi"}]}`)
	upstream := &httpUpstreamRecorder{resp: nativeAnthropicStreamResponse()}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}

	result, err := svc.ForwardAsAnthropic(context.Background(),
		adaptiveProtocolTestContext("/v1/messages", body), nativeAnthropicTestAccount(), body, "", "")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.ReasoningEffort)
	require.Equal(t, "max", *result.ReasoningEffort)
}

func TestNativeAnthropicPassthroughNoEffortStaysNil(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// 既无 output_config.effort 也未启用 thinking：保持 nil，不做语义注入。
	body := []byte(`{"model":"k3","max_tokens":32,"stream":false,` +
		`"messages":[{"role":"user","content":"hi"}]}`)
	upstream := &httpUpstreamRecorder{resp: nativeAnthropicBufferedResponse()}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}

	result, err := svc.ForwardAsAnthropic(context.Background(),
		adaptiveProtocolTestContext("/v1/messages", body), nativeAnthropicTestAccount(), body, "", "")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Nil(t, result.ReasoningEffort)
}

func TestNativeAnthropicPassthroughNormalizesGLM53Thinking(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name       string
		stream     bool
		preference string
		wantEffort string
	}{
		{name: "disabled buffered", preference: `"thinking":{"type":"disabled"},`, wantEffort: "low"},
		{name: "off buffered", preference: `"thinking":{"type":"off"},`, wantEffort: "low"},
		{name: "none buffered", preference: `"thinking":{"type":"none"},`, wantEffort: "low"},
		{name: "enabled buffered", preference: `"thinking":{"type":"enabled"},`, wantEffort: "high"},
		{name: "enabled streaming", stream: true, preference: `"thinking":{"type":"enabled"},`, wantEffort: "high"},
		{name: "adaptive buffered", preference: `"thinking":{"type":"adaptive"},`, wantEffort: "high"},
		{name: "adaptive streaming", stream: true, preference: `"thinking":{"type":"adaptive"},`, wantEffort: "high"},
		{name: "minimal buffered", preference: `"output_config":{"effort":"minimal"},`, wantEffort: "low"},
		{name: "low buffered", preference: `"output_config":{"effort":"low"},`, wantEffort: "low"},
		{name: "medium streaming", stream: true, preference: `"output_config":{"effort":"medium"},`, wantEffort: "high"},
		{name: "high buffered", preference: `"output_config":{"effort":"high"},`, wantEffort: "high"},
		{name: "xhigh buffered", preference: `"output_config":{"effort":"xhigh"},`, wantEffort: "max"},
		{name: "max buffered", preference: `"output_config":{"effort":"max"},`, wantEffort: "max"},
		{name: "ultra buffered", preference: `"output_config":{"effort":"ultra"},`, wantEffort: "max"},
		{name: "output effort wins over thinking", preference: `"thinking":{"type":"adaptive"},"output_config":{"effort":"low"},`, wantEffort: "low"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := []byte(fmt.Sprintf(`{"model":"glm-5.3","max_tokens":32,"stream":%t,%s"messages":[{"role":"user","content":"hi"}]}`, tt.stream, tt.preference))
			response := nativeAnthropicBufferedResponse()
			if tt.stream {
				response = nativeAnthropicStreamResponse()
			}
			upstream := &httpUpstreamRecorder{resp: response}
			svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}

			_, err := svc.ForwardAsAnthropic(context.Background(),
				adaptiveProtocolTestContext("/v1/messages", body), nativeAnthropicGLMTestAccount(), body, "", "")
			require.NoError(t, err)
			require.Equal(t, "enabled", gjson.GetBytes(upstream.lastBody, "thinking.type").String())
			require.Equal(t, tt.wantEffort, gjson.GetBytes(upstream.lastBody, "output_config.effort").String())
		})
	}
}

func TestNativeAnthropicPassthroughLeavesOtherThinkingUntouched(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name string
		body string
	}{
		{name: "glm 5.3 unspecified", body: `{"model":"glm-5.3","max_tokens":32,"stream":false,"messages":[]}`},
		{name: "glm 5.2 disabled", body: `{"model":"glm-5.2","max_tokens":32,"stream":false,"thinking":{"type":"disabled"},"messages":[]}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := []byte(tt.body)
			upstream := &httpUpstreamRecorder{resp: nativeAnthropicBufferedResponse()}
			svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
			_, err := svc.ForwardAsAnthropic(context.Background(),
				adaptiveProtocolTestContext("/v1/messages", body), nativeAnthropicGLMTestAccount(), body, "", "")
			require.NoError(t, err)
			require.JSONEq(t, tt.body, string(upstream.lastBody))
		})
	}
}
