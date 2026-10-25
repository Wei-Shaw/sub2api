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

func TestForwardAsAnthropic_ConvertedCNReasoningPolicy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, platform := range []string{PlatformKimi, PlatformZhipu, PlatformDeepseek, PlatformMiniMax} {
		for _, protocol := range []string{APIProtocolChatCompletions, APIProtocolResponses} {
			forwardedProtocol := protocol
			if platform == PlatformZhipu && protocol == APIProtocolResponses {
				forwardedProtocol = APIProtocolChatCompletions
			}
			for _, stream := range []bool{false, true} {
				for _, test := range []struct {
					name       string
					effort     string
					maxEffort  string
					overLimit  string
					mappings   []ReasoningEffortMapping
					unbound    bool
					wantEffort string
					wantStatus int
				}{
					{name: "mapping", mappings: []ReasoningEffortMapping{{From: "low", To: "max"}}, wantEffort: "max"},
					{name: "mapping once", mappings: []ReasoningEffortMapping{{From: "low", To: "high"}, {From: "high", To: "max"}}, wantEffort: "high"},
					{name: "mapped value not denied twice", mappings: []ReasoningEffortMapping{{From: "low", To: "high"}, {From: "high", To: ReasoningEffortMappingDeny}}, wantEffort: "high"},
					{name: "original max mapping", effort: "max", mappings: []ReasoningEffortMapping{{From: "max", To: "low", MatchType: "exact", Model: "public-alias"}}, wantEffort: "low"},
					{name: "mapping cap", maxEffort: "medium", mappings: []ReasoningEffortMapping{{From: "low", To: "max"}}, wantEffort: "medium"},
					{name: "ceiling filter", effort: "high", maxEffort: "medium", wantEffort: "medium"},
					{name: "ceiling only deny", effort: "high", maxEffort: "medium", overLimit: ReasoningEffortOverLimitDeny, wantStatus: http.StatusForbidden},
					{name: "mapping deny", mappings: []ReasoningEffortMapping{{From: "low", To: ReasoningEffortMappingDeny}}, wantStatus: http.StatusForbidden},
					{name: "ceiling deny", maxEffort: "high", overLimit: ReasoningEffortOverLimitDeny, mappings: []ReasoningEffortMapping{{From: "low", To: "max"}}, wantStatus: http.StatusForbidden},
					{name: "public model match", mappings: []ReasoningEffortMapping{{From: "low", To: "max", MatchType: "exact", Model: "public-alias"}}, wantEffort: "max"},
					{name: "public model deny", mappings: []ReasoningEffortMapping{{From: "low", To: ReasoningEffortMappingDeny, MatchType: "exact", Model: "public-alias"}}, wantStatus: http.StatusForbidden},
					{name: "final model does not remap", mappings: []ReasoningEffortMapping{{From: "low", To: ReasoningEffortMappingDeny, MatchType: "exact", Model: "deepseek-flash"}}, wantEffort: "low"},
					{name: "unbound", unbound: true, wantEffort: "low"},
					{name: "empty policy", wantEffort: "low"},
				} {
					t.Run(fmt.Sprintf("%s/%s/stream=%v/%s", platform, protocol, stream, test.name), func(t *testing.T) {
						ctx := context.Background()
						if !test.unbound {
							ctx = WithOpenAIReasoningEffortPolicy(ctx, test.maxEffort, test.mappings, test.overLimit)
						}
						effort := test.effort
						if effort == "" {
							effort = "low"
						}
						result, upstream, recorder := forwardConvertedMessagesPolicy(t, ctx, platform, protocol, "deepseek-flash", effort, stream)
						if test.wantStatus != 0 {
							require.Nil(t, result)
							require.Empty(t, upstream.requests)
							require.Equal(t, test.wantStatus, recorder.Code)
							require.Equal(t, "forbidden_error", gjson.GetBytes(recorder.Body.Bytes(), "error.type").String())
							return
						}
						assertConvertedMessagesPolicyForward(t, result, upstream, forwardedProtocol, "deepseek-flash", test.wantEffort)
					})
				}
			}
		}
	}
}

func TestForwardAsAnthropic_ConvertedPolicyFinalModelCapabilities(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, protocol := range []string{APIProtocolChatCompletions, APIProtocolResponses} {
		for _, test := range []struct {
			name           string
			platform       string
			model          string
			effort         string
			to             string
			maxEffort      string
			unbound        bool
			wantEffort     string
			wantChatEffort string
			wantStatus     int
		}{
			{name: "mapped GPT61 none rejected", platform: PlatformKimi, model: "gpt-6.1-sol", effort: "high", to: "none", wantStatus: http.StatusBadRequest},
			{name: "mapped GPT61 minimal cap rejected", platform: PlatformKimi, model: "gpt-6.1-sol", effort: "high", maxEffort: "minimal", wantStatus: http.StatusBadRequest},
			{name: "OpenAI GPT61 final validation", platform: PlatformOpenAI, model: "gpt-6.1-sol", effort: "high", to: "none", wantStatus: http.StatusBadRequest},
			{name: "OpenAI GPT61 minimal ceiling rejected", platform: PlatformOpenAI, model: "gpt-6.1-sol", effort: "high", maxEffort: "minimal", wantStatus: http.StatusBadRequest},
			{name: "mapped GPT61 valid", platform: PlatformKimi, model: "gpt-6.1-sol", effort: "high", to: "low", wantEffort: "low"},
			{name: "other final model allows none", platform: PlatformKimi, model: "gpt-5.5", effort: "high", to: "none", wantEffort: "none"},
			{name: "GLM53 low cap", platform: PlatformZhipu, model: "glm-5.3", effort: "high", maxEffort: "low", wantEffort: "low"},
			{name: "GLM53 medium cap rejected", platform: PlatformZhipu, model: "glm-5.3", effort: "high", maxEffort: "medium", wantStatus: http.StatusForbidden},
			{name: "GLM older low cap rejected", platform: PlatformZhipu, model: "glm-4.7", effort: "high", maxEffort: "low", wantStatus: http.StatusForbidden},
			{name: "GLM deny before native normalization", platform: PlatformZhipu, model: "glm-4.7", effort: "low", to: ReasoningEffortMappingDeny, wantStatus: http.StatusForbidden},
			{name: "GLM native max", platform: PlatformZhipu, model: "glm-5.3", effort: "high", to: "xhigh", wantEffort: "max"},
			{name: "GLM native mapping capped", platform: PlatformZhipu, model: "glm-5.3", effort: "high", to: "xhigh", maxEffort: "high", wantEffort: "high"},
			{name: "GLM mapped once after normalization", platform: PlatformZhipu, model: "glm-5.3", effort: "high", to: "medium", wantEffort: "high"},
			{name: "non CN policy unchanged", platform: PlatformOpenCodeGo, model: "cn-final", effort: "high", to: ReasoningEffortMappingDeny, wantEffort: "high"},
			{name: "OpenAI existing cap", platform: PlatformOpenAI, model: "gpt-5.5", effort: "high", maxEffort: "medium", wantEffort: "medium"},
			{name: "GLM unbound unchanged", platform: PlatformZhipu, model: "glm-5.3", effort: "high", unbound: true, wantEffort: "high"},
			{name: "GLM unbound normalization unchanged", platform: PlatformZhipu, model: "glm-5.3", effort: "medium", unbound: true, wantEffort: "medium", wantChatEffort: "high"},
			{name: "GLM empty policy normalization unchanged", platform: PlatformZhipu, model: "glm-5.3", effort: "medium", wantEffort: "medium", wantChatEffort: "high"},
			{name: "omitted effort default unchanged", platform: PlatformDeepseek, model: "deepseek-flash", unbound: true, wantEffort: "medium"},
		} {
			t.Run(protocol+"/"+test.name, func(t *testing.T) {
				platform := test.platform
				if platform == PlatformZhipu && protocol == APIProtocolResponses {
					platform = PlatformKimi
				}
				ctx := context.Background()
				if !test.unbound {
					var mappings []ReasoningEffortMapping
					if test.to != "" {
						mappings = []ReasoningEffortMapping{{From: test.effort, To: test.to}}
					}
					ctx = WithOpenAIReasoningEffortPolicy(ctx, test.maxEffort, mappings, "")
				}
				result, upstream, recorder := forwardConvertedMessagesPolicy(t, ctx, platform, protocol, test.model, test.effort, false)
				if test.wantStatus != 0 {
					require.Nil(t, result)
					require.Empty(t, upstream.requests)
					require.Equal(t, test.wantStatus, recorder.Code)
					if test.wantStatus == http.StatusBadRequest {
						require.Equal(t, "invalid_request_error", gjson.GetBytes(recorder.Body.Bytes(), "error.type").String())
						require.Contains(t, gjson.GetBytes(recorder.Body.Bytes(), "error.message").String(), "gpt-6.1-sol does not support reasoning effort")
					} else {
						require.Equal(t, "forbidden_error", gjson.GetBytes(recorder.Body.Bytes(), "error.type").String())
					}
					return
				}
				wantEffort := test.wantEffort
				if protocol == APIProtocolChatCompletions && test.wantChatEffort != "" {
					wantEffort = test.wantChatEffort
				}
				assertConvertedMessagesPolicyForward(t, result, upstream, protocol, test.model, wantEffort)
			})
		}
	}
}

func forwardConvertedMessagesPolicy(t *testing.T, ctx context.Context, platform, protocol, model, effort string, stream bool) (*OpenAIForwardResult, *httpUpstreamRecorder, *httptest.ResponseRecorder) {
	t.Helper()
	effortJSON := ""
	if effort != "" {
		effortJSON = fmt.Sprintf(`,"output_config":{"effort":%q}`, effort)
	}
	body := []byte(fmt.Sprintf(`{"model":"public-alias","max_tokens":32,"messages":[{"role":"user","content":"hi"}],"stream":%v%s}`, stream, effortJSON))
	account := rawChatCompletionsTestAccount()
	if platform == PlatformOpenAI && protocol == APIProtocolChatCompletions {
		account = forceChatMessagesFallbackAccount()
	}
	account.Platform = platform
	account.Credentials["api_protocol"] = protocol
	account.Credentials["api_base_urls"] = map[string]any{protocol: "http://upstream.example"}
	account.Credentials["model_mapping"] = map[string]any{"public-alias": model}
	if platform == PlatformZhipu && protocol == APIProtocolResponses {
		protocol = APIProtocolChatCompletions
	}
	responseBody := `{"id":"chatcmpl_policy","model":"` + model + `","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2}}`
	contentType := "application/json"
	if protocol == APIProtocolResponses {
		contentType = "text/event-stream"
		responseBody = "data: " + `{"type":"response.completed","response":{"id":"resp_policy","object":"response","model":"` + model + `","status":"completed","output":[{"type":"message","id":"msg_policy","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":3,"output_tokens":2}}}` + "\n\ndata: [DONE]\n\n"
	} else if stream {
		contentType = "text/event-stream"
		responseBody = "data: " + `{"id":"chatcmpl_policy","model":"` + model + `","choices":[{"index":0,"delta":{"role":"assistant","content":"ok"},"finish_reason":null}]}` + "\n\ndata: " + `{"id":"chatcmpl_policy","model":"` + model + `","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2}}` + "\n\ndata: [DONE]\n\n"
	}
	upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{contentType}}, Body: io.NopCloser(strings.NewReader(responseBody))}}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
	recorder := httptest.NewRecorder()
	ginContext, _ := gin.CreateTestContext(recorder)
	ginContext.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(string(body))).WithContext(ctx)
	result, err := svc.ForwardAsAnthropic(ctx, ginContext, account, body, "", "")
	if err != nil {
		require.Nil(t, result)
		require.Contains(t, []int{http.StatusBadRequest, http.StatusForbidden}, recorder.Code, "unexpected forwarding error: %v", err)
	} else {
		require.Equal(t, http.StatusOK, recorder.Code)
		require.Equal(t, stream, result.Stream)
	}
	return result, upstream, recorder
}

func assertConvertedMessagesPolicyForward(t *testing.T, result *OpenAIForwardResult, upstream *httpUpstreamRecorder, protocol, model, effort string) {
	t.Helper()
	require.NotNil(t, result)
	require.Len(t, upstream.requests, 1)
	path, effortPath := "/chat/completions", "reasoning_effort"
	if protocol == APIProtocolResponses {
		path, effortPath = "/responses", "reasoning.effort"
	}
	require.True(t, strings.HasSuffix(upstream.lastReq.URL.Path, path), upstream.lastReq.URL.String())
	require.Equal(t, model, gjson.GetBytes(upstream.lastBody, "model").String())
	require.Equal(t, effort, gjson.GetBytes(upstream.lastBody, effortPath).String())
	require.NotNil(t, result.ReasoningEffort)
	require.Equal(t, effort, *result.ReasoningEffort)
}
