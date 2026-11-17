//go:build unit

package service

import (
	"fmt"
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

func TestMessagesEffortPolicyHTTP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	noneToLow := []ReasoningEffortMapping{{From: "none", To: "low"}}
	for _, chat := range []bool{false, true} {
		for _, auth := range []string{AccountTypeAPIKey, AccountTypeOAuth} {
			for _, stream := range []bool{false, true} {
				for _, test := range []struct {
					name       string
					preference string
					mappings   []ReasoningEffortMapping
					ceiling    string
					overLimit  string
					model      string
					terminal   bool
					want       string
					status     int
				}{
					{name: "omitted unchanged", want: "medium"},
					{name: "omitted unrelated mapping unchanged", mappings: []ReasoningEffortMapping{{From: "medium", To: "low"}}, want: "medium"},
					{name: "omitted ceiling unchanged", ceiling: "low", want: "medium"},
					{name: "omitted deny ceiling unchanged", ceiling: "low", overLimit: ReasoningEffortOverLimitDeny, want: "medium"},
					{name: "omitted none mapping and ceiling unchanged", mappings: noneToLow, ceiling: "low", want: "medium"},
					{name: "omitted above ceiling none mapping leaves default", mappings: []ReasoningEffortMapping{{From: "none", To: "xhigh"}}, ceiling: "low", overLimit: ReasoningEffortOverLimitDeny, want: "medium"},
					{name: "omitted none mapping leaves default", mappings: noneToLow, want: "medium"},
					{name: "disabled omitted mapped", preference: `,"thinking":{"type":"disabled"}`, mappings: noneToLow, want: "low"},
					{name: "disabled synthesized ceiling downgrade", preference: `,"thinking":{"type":"disabled"}`, mappings: []ReasoningEffortMapping{{From: "none", To: "high"}}, ceiling: "low", want: "low"},
					{name: "disabled synthesized ceiling deny", preference: `,"thinking":{"type":"disabled"}`, mappings: []ReasoningEffortMapping{{From: "none", To: "high"}}, ceiling: "low", overLimit: ReasoningEffortOverLimitDeny, status: 403},
					{name: "disabled synthesized xhigh capped", preference: `,"thinking":{"type":"disabled"}`, mappings: []ReasoningEffortMapping{{From: "none", To: "xhigh"}}, ceiling: "low", want: "low"},
					{name: "disabled synthesized xhigh denied", preference: `,"thinking":{"type":"disabled"}`, mappings: []ReasoningEffortMapping{{From: "none", To: "xhigh"}}, ceiling: "low", overLimit: ReasoningEffortOverLimitDeny, status: 403},
					{name: "mapped claude sampling removed", model: "claude-sonnet-4-5", preference: `,"thinking":{"type":"disabled"},"temperature":0.5,"top_p":0.9`, mappings: noneToLow, want: "low"},
					{name: "disabled low mapped", preference: `,"thinking":{"type":"disabled"},"output_config":{"effort":"low"}`, mappings: noneToLow, want: "low"},
					{name: "disabled high mapped", preference: `,"thinking":{"type":"disabled"},"output_config":{"effort":"high"}`, mappings: noneToLow, want: "low"},
					{name: "disabled max mapped", preference: `,"thinking":{"type":"disabled"},"output_config":{"effort":"max"}`, mappings: noneToLow, want: "low"},
					{name: "disabled no mapping rejected", preference: `,"thinking":{"type":"disabled"},"output_config":{"effort":"high"}`, status: 400},
					{name: "disabled omitted no mapping rejected", preference: `,"thinking":{"type":"disabled"}`, status: 400},
					{name: "disabled unrelated mapping rejected", preference: `,"thinking":{"type":"disabled"},"output_config":{"effort":"high"}`, mappings: []ReasoningEffortMapping{{From: "high", To: "low"}}, status: 400},
					{name: "disabled wrong model scope rejected", preference: `,"thinking":{"type":"disabled"}`, mappings: []ReasoningEffortMapping{{From: "none", To: "low", Model: "gpt-6-sol", MatchType: "exact"}}, status: 400},
					{name: "raw none cannot be mapped away", preference: `,"output_config":{"effort":"none"}`, mappings: noneToLow, status: 400},
					{name: "raw none disabled cannot be hidden", preference: `,"thinking":{"type":"disabled"},"output_config":{"effort":"none"}`, mappings: noneToLow, status: 400},
					{name: "raw minimal disabled cannot be hidden", preference: `,"thinking":{"type":"disabled"},"output_config":{"effort":"minimal"}`, mappings: append(append([]ReasoningEffortMapping{}, noneToLow...), ReasoningEffortMapping{From: "minimal", To: "low"}), status: 400},
					{name: "raw minimal cannot be mapped away", preference: `,"output_config":{"effort":"minimal"}`, mappings: []ReasoningEffortMapping{{From: "minimal", To: "low"}}, status: 400},
					{name: "raw reasoning none cannot be hidden", preference: `,"thinking":{"type":"disabled"},"output_config":{"effort":"high"},"reasoning":{"effort":"none"}`, mappings: noneToLow, status: 400},
					{name: "raw chat minimal cannot be hidden", preference: `,"thinking":{"type":"disabled"},"output_config":{"effort":"high"},"reasoning_effort":"minimal"`, mappings: noneToLow, status: 400},
					{name: "raw alias cannot be hidden", model: "gpt-6.1-sol-minimal", preference: `,"thinking":{"type":"disabled"}`, mappings: noneToLow, status: 400},
					{name: "underscore minimal cannot be hidden", model: "gpt-6.1-sol_minimal", preference: `,"thinking":{"type":"disabled"}`, mappings: noneToLow, status: 400},
					{name: "underscore none cannot be hidden", model: "gpt-6.1-sol_none", preference: `,"thinking":{"type":"disabled"}`, mappings: noneToLow, status: 400},
					{name: "one hop mapping", preference: `,"thinking":{"type":"disabled"},"output_config":{"effort":"high"}`, mappings: []ReasoningEffortMapping{{From: "none", To: "low"}, {From: "low", To: "high"}}, want: "low"},
					{name: "mapping final unsupported rejected", preference: `,"thinking":{"type":"disabled"}`, mappings: []ReasoningEffortMapping{{From: "none", To: "minimal"}}, status: 400},
					{name: "mapping deny preserved", preference: `,"thinking":{"type":"disabled"}`, mappings: []ReasoningEffortMapping{{From: "none", To: "deny"}}, status: 403},
					{name: "adaptive explicit unchanged", preference: `,"thinking":{"type":"adaptive"},"output_config":{"effort":"high"}`, mappings: noneToLow, want: "high"},
					{name: "canonical explicit low preserved", preference: `,"output_config":{"effort":"low"}`, mappings: noneToLow, want: "low"},
					{name: "canonical explicit high preserved", preference: `,"output_config":{"effort":"high"}`, mappings: noneToLow, want: "high"},
					{name: "adaptive explicit mapped", preference: `,"thinking":{"type":"adaptive"},"output_config":{"effort":"high"}`, mappings: []ReasoningEffortMapping{{From: "high", To: "low"}, {From: "low", To: "medium"}}, want: "low"},
					{name: "explicit ceiling preserved", preference: `,"output_config":{"effort":"high"}`, ceiling: "medium", want: "medium"},
					{name: "terminal override not mapped twice", terminal: true, preference: `,"thinking":{"type":"disabled"},"output_config":{"effort":"high"}`, mappings: []ReasoningEffortMapping{{From: "none", To: "low", Model: "gpt-6.1-sol", MatchType: "exact"}}, want: "low"},
					{name: "terminal override raw rejected", terminal: true, preference: `,"thinking":{"type":"disabled"},"output_config":{"effort":"none"}`, mappings: noneToLow, status: 400},
				} {
					t.Run(fmt.Sprintf("chat=%t/%s/stream=%t/%s", chat, auth, stream, test.name), func(t *testing.T) {
						model := test.model
						if model == "" {
							model = "gpt-6.1-sol"
						}
						account := rawGPT56ResponsesAPIKeyAccount(model, "gpt-6.1-sol")
						if auth == AccountTypeOAuth {
							account = rawGPT56ResponsesOAuthAccount(model, "gpt-6.1-sol")
						}
						if chat {
							account.Extra = map[string]any{openai_compat.ExtraKeyResponsesMode: string(openai_compat.ResponsesSupportModeForceChatCompletions)}
						}
						defaultMapped := ""
						if test.terminal {
							model = "public-alias"
							defaultMapped = "gpt-6.1-sol"
							account.Credentials["model_mapping"] = map[string]any{"gpt-6.1-sol": "gpt-6-sol"}
						}
						usesChat := chat && auth == AccountTypeAPIKey
						response := openAICompatSSECompletedResponse("resp_effort", "gpt-6.1-sol")
						if usesChat {
							payload := `{"id":"chat_effort","model":"gpt-6.1-sol","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":2}}`
							contentType := "application/json"
							if stream {
								payload = "data: " + `{"id":"chat_effort","model":"gpt-6.1-sol","choices":[{"index":0,"delta":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":2}}` + "\n\ndata: [DONE]\n\n"
								contentType = "text/event-stream"
							}
							response = &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{contentType}}, Body: io.NopCloser(strings.NewReader(payload))}
						}
						upstream := &httpUpstreamRecorder{resp: response}
						svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
						body := fmt.Sprintf(`{"model":%q,"max_tokens":1024,"messages":[{"role":"user","content":"Hello"}],"stream":%t%s}`, model, stream, test.preference)
						var forwardErr error
						var result *OpenAIForwardResult
						router := gin.New()
						router.POST("/v1/messages", func(c *gin.Context) {
							ctx := WithOpenAIReasoningEffortPolicyForMessages(c.Request.Context(), []byte(body), test.ceiling, test.mappings, test.overLimit)
							result, forwardErr = svc.ForwardAsAnthropic(ctx, c, account, []byte(body), "", defaultMapped)
						})
						recorder := httptest.NewRecorder()
						router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body)))
						if test.status != 0 {
							require.Error(t, forwardErr)
							require.Equal(t, test.status, recorder.Code, recorder.Body.String())
							require.Empty(t, upstream.requests)
							return
						}
						preferredEffortPath := "reasoning.effort"
						endpoint := "/responses"
						if usesChat {
							preferredEffortPath = "reasoning_effort"
							endpoint = "/chat/completions"
						}
						require.NoError(t, forwardErr)
						require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
						require.Len(t, upstream.requests, 1)
						require.True(t, strings.HasSuffix(upstream.lastReq.URL.Path, endpoint), upstream.lastReq.URL.String())
						require.Equal(t, "gpt-6.1-sol", gjson.GetBytes(upstream.lastBody, "model").String())
						require.Equal(t, test.want, gjson.GetBytes(upstream.lastBody, preferredEffortPath).String())
						require.False(t, gjson.GetBytes(upstream.lastBody, "temperature").Exists())
						require.False(t, gjson.GetBytes(upstream.lastBody, "top_p").Exists())
						require.NotNil(t, result)
						require.NotNil(t, result.ReasoningEffort)
						require.Equal(t, test.want, *result.ReasoningEffort)
						require.Equal(t, stream, result.Stream)
						require.Equal(t, stream || !usesChat, gjson.GetBytes(upstream.lastBody, "stream").Bool())
						if stream {
							require.Contains(t, recorder.Header().Get("Content-Type"), "text/event-stream")
							require.Contains(t, recorder.Body.String(), "message_stop")
						} else {
							require.Contains(t, recorder.Header().Get("Content-Type"), "application/json")
							require.Equal(t, "ok", gjson.GetBytes(recorder.Body.Bytes(), "content.0.text").String())
						}
					})
				}
			}
		}
	}
}

func TestMessagesRawEffortValidation(t *testing.T) {
	for _, test := range []struct {
		body string
		deny bool
	}{
		{body: `{"output_config":{"effort":"high"},"thinking":{"type":"disabled"}}`},
		{body: `{"output_config":{"effort":"none"},"thinking":{"type":"disabled"}}`, deny: true},
		{body: `{"reasoning_effort":"none"}`, deny: true},
		{body: `{"reasoning":{"effort":"minimal"}}`, deny: true},
		{body: `{"model":"gpt-6.1-sol-none"}`, deny: true},
		{body: `{"model":"gpt-6.1-sol_none"}`, deny: true},
		{body: `{"model":"gpt-6.1-sol_minimal"}`, deny: true},
		{body: `{"model":"gpt-6.1-sol_low","output_config":{"effort":"low"}}`},
		{body: `{"model":"gpt-6.1-sol","output_config":{"effort":"high"}}`},
	} {
		require.Equal(t, test.deny, validateGPT61RawEffort([]byte(test.body), "gpt-6.1-sol") != nil, test.body)
		require.NoError(t, validateGPT61RawEffort([]byte(test.body), "gpt-6-sol"))
	}
}

func TestMessagesGLMMappedEffortPolicyHTTP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		name      string
		model     string
		target    string
		ceiling   string
		overLimit string
		want      string
		status    int
		explicit  bool
	}{
		{name: "mapped low preserved after normalization", target: "low", want: "low"},
		{name: "unmapped none below low ceiling", ceiling: "low", overLimit: ReasoningEffortOverLimitDeny, want: "none"},
		{name: "unmapped none below minimal ceiling", ceiling: "minimal", overLimit: ReasoningEffortOverLimitDeny, want: "none"},
		{name: "glm53 unmapped none below low ceiling", model: "glm-5.3", ceiling: "low", overLimit: ReasoningEffortOverLimitDeny, want: "none"},
		{name: "mapped minimal preserved", target: "minimal", want: "minimal"},
		{name: "mapped max preserved", target: "max", want: "max"},
		{name: "mapped low meets low ceiling", target: "low", ceiling: "low", want: "low"},
		{name: "mapped minimal below low ceiling", target: "minimal", ceiling: "low", want: "minimal"},
		{name: "mapped minimal meets minimal ceiling", target: "minimal", ceiling: "minimal", want: "minimal"},
		{name: "mapped low below medium ceiling", target: "low", ceiling: "medium", want: "low"},
		{name: "max capped to xhigh", target: "max", ceiling: "xhigh", want: "xhigh"},
		{name: "glm53 low preserved", model: "glm-5.3", target: "low", ceiling: "low", want: "low"},
		{name: "glm53 minimal preserved", model: "glm-5.3", target: "minimal", want: "minimal"},
		{name: "glm53 minimal below low ceiling", model: "glm-5.3", target: "minimal", ceiling: "low", want: "minimal"},
		{name: "glm53 max capped to medium", model: "glm-5.3", target: "max", ceiling: "medium", want: "medium"},
		{name: "glm53 minimal meets minimal ceiling", model: "glm-5.3", target: "minimal", ceiling: "minimal", want: "minimal"},
		{name: "mapped effort denied", target: "high", ceiling: "low", overLimit: ReasoningEffortOverLimitDeny, status: 403},
		{name: "mapping denied", target: "deny", status: 403},
		{name: "mapping applied once", target: "low", ceiling: "high", want: "low", explicit: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			model := test.model
			if model == "" {
				model = "glm-5.2"
			}
			account := rawGPT56ResponsesAPIKeyAccount("claude-sonnet-4-5", model)
			account.Extra = map[string]any{openai_compat.ExtraKeyResponsesMode: string(openai_compat.ResponsesSupportModeForceChatCompletions)}
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"id":"chat_glm","model":"` + model + `","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":2}}`)),
			}}
			svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
			body := []byte(`{"model":"claude-sonnet-4-5","max_tokens":1024,"thinking":{"type":"disabled"},"messages":[{"role":"user","content":"Hello"}]}`)
			if test.explicit {
				body = []byte(`{"model":"claude-sonnet-4-5","max_tokens":1024,"thinking":{"type":"disabled"},"output_config":{"effort":"high"},"messages":[{"role":"user","content":"Hello"}]}`)
			}
			recorder := httptest.NewRecorder()
			requestContext, _ := gin.CreateTestContext(recorder)
			requestContext.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(string(body)))
			mappings := []ReasoningEffortMapping{{From: "high", To: "max"}}
			if test.target != "" {
				mappings = append(mappings, ReasoningEffortMapping{From: "none", To: test.target})
			}
			ctx := WithOpenAIReasoningEffortPolicyForMessages(requestContext.Request.Context(), body, test.ceiling, mappings, test.overLimit)
			result, err := svc.ForwardAsAnthropic(ctx, requestContext, account, body, "", "")
			if test.status != 0 {
				require.Error(t, err)
				require.Equal(t, test.status, recorder.Code)
				require.Empty(t, upstream.requests)
				return
			}
			require.NoError(t, err)
			require.Len(t, upstream.requests, 1)
			require.Equal(t, model, gjson.GetBytes(upstream.lastBody, "model").String())
			require.Equal(t, test.want, gjson.GetBytes(upstream.lastBody, "reasoning_effort").String())
			require.NotNil(t, result.ReasoningEffort)
			require.Equal(t, test.want, *result.ReasoningEffort)
		})
	}
}

func TestMessagesDisabledThinkingCountTokensHTTP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-6.1-sol","thinking":{"type":"disabled"},"messages":[{"role":"user","content":"hello"}]}`)
	recorder := httptest.NewRecorder()
	requestContext, _ := gin.CreateTestContext(recorder)
	requestContext.Request = httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", strings.NewReader(string(body)))
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"object":"response.input_tokens","input_tokens":42}`)),
	}}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
	account := rawGPT56ResponsesAPIKeyAccount("gpt-6.1-sol", "gpt-6.1-sol")
	ctx := WithOpenAIReasoningEffortPolicyForMessages(requestContext.Request.Context(), body, "", []ReasoningEffortMapping{{From: "none", To: "low"}}, "")
	require.NoError(t, svc.ForwardCountTokensAsAnthropic(ctx, requestContext, account, body, ""))
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, int64(42), gjson.GetBytes(recorder.Body.Bytes(), "input_tokens").Int())
	require.Len(t, upstream.requests, 1)
	require.True(t, strings.HasSuffix(upstream.lastReq.URL.Path, "/responses/input_tokens"))
	require.False(t, gjson.GetBytes(upstream.lastBody, "reasoning").Exists())
	require.False(t, gjson.GetBytes(upstream.lastBody, "reasoning_effort").Exists())
}
