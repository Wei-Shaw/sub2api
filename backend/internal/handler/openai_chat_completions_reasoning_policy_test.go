package handler

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

type chatReasoningPolicyUpstream struct {
	service.HTTPUpstream
	bodies [][]byte
	paths  []string
}

func (upstream *chatReasoningPolicyUpstream) Do(request *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	body, err := io.ReadAll(request.Body)
	if err != nil {
		return nil, err
	}
	upstream.bodies = append(upstream.bodies, body)
	upstream.paths = append(upstream.paths, request.URL.Path)
	if strings.HasSuffix(request.URL.Path, "/responses") {
		responseBody := fmt.Sprintf(`data: {"type":"response.completed","response":{"id":"resp_policy","object":"response","model":%q,"status":"completed","output":[{"type":"message","id":"msg_policy","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}}`+"\n\n", gjson.GetBytes(body, "model").String())
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader(responseBody)),
		}, nil
	}
	if gjson.GetBytes(body, "stream").Bool() {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader("data: {\"id\":\"chatcmpl_policy\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"delta\":{\"role\":\"assistant\",\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")),
		}, nil
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"id":"chatcmpl_policy","object":"chat.completion","choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`)),
	}, nil
}

func TestChatCompletionsReasoningPolicyForwardedBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		name       string
		platform   string
		model      string
		finalModel string
		effort     string
		ceiling    string
		overLimit  string
		mappings   []service.ReasoningEffortMapping
		wantEffort string
		wantDenied bool
	}{
		{name: "medium ceiling rejects GLM upgrade", effort: "medium", ceiling: "medium", wantDenied: true},
		{name: "medium ceiling denies GLM upgrade", effort: "medium", ceiling: "medium", overLimit: "deny", wantDenied: true},
		{name: "xhigh ceiling downgrades GLM max", effort: "xhigh", ceiling: "xhigh", wantEffort: "high"},
		{name: "xhigh ceiling denies GLM max", effort: "xhigh", ceiling: "xhigh", overLimit: "deny", wantDenied: true},
		{name: "max input respects xhigh ceiling", effort: "max", ceiling: "xhigh", wantEffort: "high"},
		{name: "max downgraded to high", effort: "xhigh", ceiling: "high", wantEffort: "high"},
		{name: "max ceiling permits max", effort: "xhigh", ceiling: "max", wantEffort: "max"},
		{name: "explicit max remains max", effort: "max", ceiling: "max", wantEffort: "max"},
		{name: "deny max on ingress", effort: "max", ceiling: "high", overLimit: "deny", wantDenied: true},
		{name: "low ceiling incompatible with older GLM", effort: "medium", ceiling: "low", wantDenied: true},
		{name: "GLM 5.3 supports low ceiling", finalModel: "glm-5.3", effort: "medium", ceiling: "low", wantEffort: "low"},
		{name: "account alias resolves GLM", model: "deepseek-flash", platform: service.PlatformDeepseek, effort: "medium", ceiling: "medium", wantDenied: true},
		{name: "Kimi account resolves GLM", model: "kimi-k2-thinking", platform: service.PlatformKimi, effort: "xhigh", ceiling: "xhigh", wantEffort: "high"},
		{name: "GLM53 medium ceiling selects low", finalModel: "glm-5.3", effort: "medium", ceiling: "medium", wantEffort: "low"},
		{name: "CN none bypasses native tiers", effort: "none", ceiling: "low", wantEffort: "none"},
		{name: "CN minimal bypasses native tiers", effort: "minimal", ceiling: "low", wantEffort: "minimal"},
		{name: "OpenAI GLM low legacy", platform: service.PlatformOpenAI, effort: "low", ceiling: "medium", wantEffort: "high"},
		{name: "OpenAI GLM medium legacy", platform: service.PlatformOpenAI, effort: "medium", ceiling: "medium", wantEffort: "high"},
		{name: "OpenAI GLM high legacy", platform: service.PlatformOpenAI, effort: "high", ceiling: "medium", wantEffort: "high"},
		{name: "OpenAI GLM none legacy", platform: service.PlatformOpenAI, effort: "none", ceiling: "medium", wantEffort: "none"},
		{name: "OpenAI GLM minimal legacy", platform: service.PlatformOpenAI, effort: "minimal", ceiling: "medium", wantEffort: "minimal"},
		{name: "OpenAI GLM xhigh legacy", platform: service.PlatformOpenAI, effort: "xhigh", ceiling: "xhigh", wantEffort: "max"},
		{name: "OpenAI GLM max legacy", platform: service.PlatformOpenAI, effort: "max", ceiling: "xhigh", wantEffort: "max"},
		{name: "MiniMax account resolves GLM", model: "MiniMax-M3", platform: service.PlatformMiniMax, effort: "medium", ceiling: "medium", wantDenied: true},
		{name: "GLM alias resolves non GLM", finalModel: "deepseek-flash", effort: "medium", ceiling: "medium", wantEffort: "medium"},
		{name: "mapping runs once before normalization", effort: "low", ceiling: "max", mappings: []service.ReasoningEffortMapping{
			{From: "low", To: "medium", MatchType: "exact", Model: "glm-5.2"},
			{From: "medium", To: "max", MatchType: "exact", Model: "glm-5.2"},
			{From: "high", To: "deny"},
		}, wantEffort: "high"},
		{name: "mapping matches client not final model", effort: "low", ceiling: "max", mappings: []service.ReasoningEffortMapping{
			{From: "low", To: "medium", MatchType: "exact", Model: "glm-5.2"},
			{From: "medium", To: "deny", MatchType: "exact", Model: "glm-5.3"},
			{From: "high", To: "deny", MatchType: "exact", Model: "glm-5.3"},
		}, finalModel: "glm-5.3", wantEffort: "high"},
		{name: "mapping deny is local", effort: "medium", mappings: []service.ReasoningEffortMapping{{From: "medium", To: "deny"}}, wantDenied: true},
		{name: "no ceiling keeps provider normalization", effort: "medium", wantEffort: "high"},
	} {
		for _, field := range []string{"reasoning_effort", "reasoning"} {
			t.Run(test.name+"/"+field, func(t *testing.T) {
				model, finalModel, platform := test.model, test.finalModel, test.platform
				if model == "" {
					model = "glm-5.2"
				}
				if finalModel == "" {
					finalModel = "glm-5.2"
				}
				if platform == "" {
					platform = service.PlatformZhipu
				}
				effortJSON := fmt.Sprintf(`%q`, test.effort)
				effortPath := field
				if field == "reasoning" {
					effortJSON = fmt.Sprintf(`{"effort":%q}`, test.effort)
					effortPath = "reasoning.effort"
				}
				body := []byte(fmt.Sprintf(`{"model":%q,%q:%s,"messages":[{"role":"user","content":"hello"}]}`, model, field, effortJSON))
				recorder := httptest.NewRecorder()
				ginContext, _ := gin.CreateTestContext(recorder)
				ginContext.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(string(body)))
				apiKey := &service.APIKey{Group: &service.Group{
					Platform: service.PlatformComposite, MaxReasoningEffort: test.ceiling,
					MaxReasoningEffortOverLimit: test.overLimit, ReasoningEffortMappings: test.mappings,
				}}
				if platform == service.PlatformOpenAI {
					apiKey.Group.Platform = service.PlatformOpenAI
				}
				require.True(t, openAICompatibleTextTargetAllowed(ginContext, apiKey, model))
				upstream := &chatReasoningPolicyUpstream{}
				cfg := &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{AllowInsecureHTTP: true}}}
				gateway := service.NewOpenAIGatewayService(nil, nil, nil, nil, nil, nil, nil, cfg, nil, nil, nil, nil, nil, upstream, nil, nil, nil, nil, nil, nil, nil, nil)
				account := &service.Account{ID: 101, Platform: platform, Type: service.AccountTypeAPIKey, Credentials: map[string]any{
					"api_key": "sk-test", "base_url": "http://upstream.example", "api_protocol": service.APIProtocolChatCompletions,
					"model_mapping": map[string]any{model: finalModel},
				}}
				if platform == service.PlatformOpenAI {
					account.Extra = map[string]any{openai_compat.ExtraKeyResponsesSupported: false}
				}
				forwardBody, _, err := applyOpenAIChatCompletionsReasoningEffortPolicyForRequest(ginContext, apiKey, body)
				if err != nil {
					respondOpenAIReasoningEffortPolicyError(ginContext, err, (&OpenAIGatewayHandler{}).errorResponse)
				} else {
					_, err = gateway.ForwardAsChatCompletions(ginContext.Request.Context(), ginContext, account, forwardBody, "", "")
				}
				if test.wantDenied {
					require.True(t, service.IsReasoningEffortPolicyDenied(err), "error: %v", err)
					require.Equal(t, http.StatusForbidden, recorder.Code)
					require.Equal(t, "permission_error", gjson.GetBytes(recorder.Body.Bytes(), "error.type").String())
					require.Empty(t, upstream.bodies)
					return
				}
				require.NoError(t, err)
				require.Equal(t, http.StatusOK, recorder.Code)
				require.Len(t, upstream.bodies, 1)
				require.Equal(t, []string{"/v1/chat/completions"}, upstream.paths)
				require.Equal(t, finalModel, gjson.GetBytes(upstream.bodies[0], "model").String())
				require.Equal(t, test.wantEffort, gjson.GetBytes(upstream.bodies[0], effortPath).String())
				requested := service.RequestedReasoningEffortFromContext(ginContext.Request.Context())
				if test.effort == "none" {
					require.Nil(t, requested)
				} else {
					require.NotNil(t, requested)
					require.Equal(t, test.effort, *requested)
				}
			})
		}
	}
}

func TestCNReasoningPolicyOriginalModelRewrites(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, rewrite := range []string{"composite", "channel"} {
		for _, endpoint := range []string{"messages", "chat", "responses"} {
			for _, denied := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/deny=%t", rewrite, endpoint, denied), func(t *testing.T) {
					c, _ := gin.CreateTestContext(httptest.NewRecorder())
					c.Request = httptest.NewRequest(http.MethodPost, "/v1/"+endpoint, nil)
					c.Request = c.Request.WithContext(service.WithResolvedTargetPlatform(c.Request.Context(), service.PlatformDeepseek))
					model := "public-alias"
					if rewrite == "composite" {
						c.Request = c.Request.WithContext(service.WithCompositeRouteDecision(c.Request.Context(), service.CompositeRouteDecision{
							Matched: true, PublicModel: model, TargetPlatform: service.PlatformDeepseek, UpstreamModel: "deepseek-flash",
						}))
						model = "deepseek-flash"
					}
					target := "medium"
					if denied {
						target = "deny"
					}
					apiKey := &service.APIKey{Group: &service.Group{Platform: service.PlatformComposite, ReasoningEffortMappings: []service.ReasoningEffortMapping{
						{From: "low", To: target, MatchType: "exact", Model: "public-alias"},
						{From: "medium", To: "deny"},
					}}}
					field := "reasoning_effort"
					switch endpoint {
					case "messages":
						field = "output_config.effort"
					case "responses":
						field = "reasoning.effort"
					}
					body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"hello"}]}`, model))
					body, err := sjson.SetBytes(body, field, "low")
					require.NoError(t, err)
					if endpoint == "messages" {
						bindOpenAIReasoningEffortPolicyForMessagesRequest(c, apiKey, body)
					} else {
						apply := applyOpenAIReasoningEffortPolicyForRequest
						if endpoint == "chat" {
							apply = applyOpenAIChatCompletionsReasoningEffortPolicyForRequest
						}
						body, _, err = apply(c, apiKey, body)
						if denied {
							require.True(t, service.IsReasoningEffortPolicyDenied(err), "error: %v", err)
							return
						}
						require.NoError(t, err)
					}
					body, err = sjson.SetBytes(body, "model", "deepseek-flash")
					require.NoError(t, err)
					for retry := 0; retry < 2; retry++ {
						got, _, err := service.ApplyOpenAIReasoningEffortPolicyFromContext(c.Request.Context(), body)
						if denied {
							require.True(t, service.IsReasoningEffortPolicyDenied(err), "error: %v", err)
							continue
						}
						require.NoError(t, err)
						require.Equal(t, "medium", gjson.GetBytes(got, field).String())
						require.Equal(t, "deepseek-flash", gjson.GetBytes(got, "model").String())
						if endpoint == "chat" {
							upstream := &chatReasoningPolicyUpstream{}
							cfg := &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{AllowInsecureHTTP: true}}}
							gateway := service.NewOpenAIGatewayService(nil, nil, nil, nil, nil, nil, nil, cfg, nil, nil, nil, nil, nil, upstream, nil, nil, nil, nil, nil, nil, nil, nil)
							account := &service.Account{ID: 101, Platform: service.PlatformDeepseek, Type: service.AccountTypeAPIKey, Credentials: map[string]any{
								"api_key": "sk-test", "base_url": "http://upstream.example", "api_protocol": service.APIProtocolChatCompletions,
							}}
							_, err = gateway.ForwardAsChatCompletions(c.Request.Context(), c, account, got, "", "")
							require.NoError(t, err)
							require.Len(t, upstream.bodies, 1)
							require.Equal(t, "medium", gjson.GetBytes(upstream.bodies[0], field).String())
							require.Equal(t, "deepseek-flash", gjson.GetBytes(upstream.bodies[0], "model").String())
						}
					}
				})
			}
		}
	}
}

func TestOpenAIReasoningPolicyBindsFinalCeiling(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, endpoint := range []string{"chat", "responses"} {
		for _, overLimit := range []string{"downgrade", "deny"} {
			t.Run(endpoint+"/"+overLimit, func(t *testing.T) {
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/"+endpoint, nil)
				c.Request = c.Request.WithContext(service.WithResolvedTargetPlatform(c.Request.Context(), service.PlatformKimi))
				apiKey := &service.APIKey{Group: &service.Group{
					Platform: service.PlatformComposite, MaxReasoningEffort: "xhigh", MaxReasoningEffortOverLimit: overLimit,
					ReasoningEffortMappings: []service.ReasoningEffortMapping{{From: "low", To: "xhigh"}, {From: "max", To: "low"}},
				}}
				body := []byte(`{"model":"k3","reasoning":{"effort":"low"}}`)
				apply := applyOpenAIReasoningEffortPolicyForRequest
				if endpoint == "chat" {
					apply = applyOpenAIChatCompletionsReasoningEffortPolicyForRequest
				}
				got, _, err := apply(c, apiKey, body)
				require.NoError(t, err)
				require.Equal(t, "xhigh", gjson.GetBytes(got, "reasoning.effort").String())
				got, _, err = service.ApplyOpenAIReasoningEffortPolicyFromContext(c.Request.Context(), []byte(`{"model":"rewritten","output_config":{"effort":"max"}}`))
				if overLimit == "deny" {
					require.True(t, service.IsReasoningEffortPolicyDenied(err), "error: %v", err)
					return
				}
				require.NoError(t, err)
				require.Equal(t, "xhigh", gjson.GetBytes(got, "output_config.effort").String())
			})
		}
	}
}

func TestCNMessagesOmittedEffortDoesNotBindSynthesizedDefault(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, platform := range []string{service.PlatformDeepseek, service.PlatformKimi, service.PlatformZhipu, service.PlatformMiniMax} {
		t.Run(platform, func(t *testing.T) {
			ginContext, _ := gin.CreateTestContext(httptest.NewRecorder())
			ginContext.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
			ginContext.Request = ginContext.Request.WithContext(service.WithResolvedTargetPlatform(ginContext.Request.Context(), platform))
			apiKey := &service.APIKey{Group: &service.Group{
				Platform: service.PlatformComposite, MaxReasoningEffort: "low", MaxReasoningEffortOverLimit: "deny",
				ReasoningEffortMappings: []service.ReasoningEffortMapping{{From: "medium", To: "deny"}},
			}}
			bindOpenAIReasoningEffortPolicyForMessagesRequest(ginContext, apiKey, []byte(`{"model":"public-alias","thinking":{"type":"enabled"}}`))
			converted := []byte(`{"model":"glm-5.3","reasoning":{"effort":"medium"}}`)
			got, changed, err := service.ApplyOpenAIReasoningEffortPolicyFromContext(ginContext.Request.Context(), converted)
			require.NoError(t, err)
			require.False(t, changed)
			require.Equal(t, converted, got)
			require.Nil(t, service.RequestedReasoningEffortFromContext(ginContext.Request.Context()))
		})
	}
}

func TestCNReasoningDiagnosticPreservesIngressToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, endpoint := range []string{"chat/completions", "responses"} {
		for _, protocol := range []string{service.APIProtocolChatCompletions, service.APIProtocolAnthropic} {
			for _, token := range []string{"none", "NoNe", " HIGH "} {
				t.Run(endpoint+"/"+protocol+"/"+token, func(t *testing.T) {
					body := []byte(fmt.Sprintf(`{"model":"glm-4.6","messages":[{"role":"user","content":"hello"}],"reasoning_effort":%q}`, token))
					field := "reasoning_effort"
					if endpoint == "responses" {
						field = "reasoning.effort"
						body = []byte(fmt.Sprintf(`{"model":"glm-4.6","input":"hello","reasoning":{"effort":%q}}`, token))
					}
					recorder := httptest.NewRecorder()
					ginContext, _ := gin.CreateTestContext(recorder)
					ginContext.Request = httptest.NewRequest(http.MethodPost, "/v1/"+endpoint, strings.NewReader(string(body)))
					ginContext.Request = ginContext.Request.WithContext(service.WithResolvedTargetPlatform(ginContext.Request.Context(), service.PlatformZhipu))
					apiKey := &service.APIKey{Group: &service.Group{
						Platform: service.PlatformComposite, MaxReasoningEffort: "xhigh", MaxReasoningEffortOverLimit: "deny",
						ReasoningEffortMappings: []service.ReasoningEffortMapping{{From: strings.ToLower(strings.TrimSpace(token)), To: "xhigh"}},
					}}
					forwardBody, changed, err := applyOpenAIReasoningEffortPolicyForRequest(ginContext, apiKey, body)
					require.NoError(t, err)
					require.True(t, changed)
					require.Equal(t, "xhigh", gjson.GetBytes(forwardBody, field).String())
					require.Equal(t, service.CanonicalRequestedReasoningEffort(body), service.RequestedReasoningEffortFromContext(ginContext.Request.Context()))
					upstream := &chatReasoningPolicyUpstream{}
					cfg := &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{AllowInsecureHTTP: true}}}
					gateway := service.NewOpenAIGatewayService(nil, nil, nil, nil, nil, nil, nil, cfg, nil, nil, nil, nil, nil, upstream, nil, nil, nil, nil, nil, nil, nil, nil)
					account := &service.Account{ID: 101, Platform: service.PlatformZhipu, Type: service.AccountTypeAPIKey, Credentials: map[string]any{
						"api_key": "sk-test", "base_url": "http://upstream.example", "api_protocol": protocol,
					}}
					for retry := 0; retry < 2; retry++ {
						var result *service.OpenAIForwardResult
						if endpoint == "responses" {
							result, err = gateway.Forward(ginContext.Request.Context(), ginContext, account, forwardBody)
						} else {
							result, err = gateway.ForwardAsChatCompletions(ginContext.Request.Context(), ginContext, account, forwardBody, "", "")
						}
						var denial *service.ReasoningEffortOverLimitError
						require.ErrorAs(t, err, &denial)
						require.Equal(t, token, denial.Requested)
						require.Equal(t, "xhigh", denial.Max)
						require.Nil(t, result)
						require.Equal(t, http.StatusForbidden, recorder.Code)
						require.Empty(t, upstream.bodies)
					}
				})
			}
		}
	}
}
