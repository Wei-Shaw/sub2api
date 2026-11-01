package handler

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type chatReasoningPolicyUpstream struct {
	service.HTTPUpstream
	bodies [][]byte
}

func (upstream *chatReasoningPolicyUpstream) Do(request *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	body, err := io.ReadAll(request.Body)
	if err != nil {
		return nil, err
	}
	upstream.bodies = append(upstream.bodies, body)
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
		{name: "xhigh ceiling rejects GLM max", effort: "xhigh", ceiling: "xhigh", wantDenied: true},
		{name: "xhigh ceiling denies GLM max", effort: "xhigh", ceiling: "xhigh", overLimit: "deny", wantDenied: true},
		{name: "max input cannot bypass xhigh ceiling", effort: "max", ceiling: "xhigh", wantDenied: true},
		{name: "max downgraded to high", effort: "xhigh", ceiling: "high", wantEffort: "high"},
		{name: "max ceiling permits max", effort: "xhigh", ceiling: "max", wantEffort: "max"},
		{name: "explicit max remains max", effort: "max", ceiling: "max", wantEffort: "max"},
		{name: "deny max on ingress", effort: "max", ceiling: "high", overLimit: "deny", wantDenied: true},
		{name: "low ceiling incompatible with older GLM", effort: "medium", ceiling: "low", wantDenied: true},
		{name: "GLM 5.3 supports low ceiling", finalModel: "glm-5.3", effort: "medium", ceiling: "low", wantEffort: "low"},
		{name: "account alias resolves GLM", model: "deepseek-flash", platform: service.PlatformDeepseek, effort: "medium", ceiling: "medium", wantDenied: true},
		{name: "Kimi account resolves GLM", model: "kimi-k2-thinking", platform: service.PlatformKimi, effort: "xhigh", ceiling: "xhigh", wantDenied: true},
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
				require.True(t, openAICompatibleTextTargetAllowed(ginContext, apiKey, model))
				upstream := &chatReasoningPolicyUpstream{}
				cfg := &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{AllowInsecureHTTP: true}}}
				gateway := service.NewOpenAIGatewayService(nil, nil, nil, nil, nil, nil, nil, cfg, nil, nil, nil, nil, nil, upstream, nil, nil, nil, nil, nil, nil, nil, nil)
				account := &service.Account{ID: 101, Platform: platform, Type: service.AccountTypeAPIKey, Credentials: map[string]any{
					"api_key": "sk-test", "base_url": "http://upstream.example", "api_protocol": service.APIProtocolChatCompletions,
					"model_mapping": map[string]any{model: finalModel},
				}}
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
				require.Equal(t, finalModel, gjson.GetBytes(upstream.bodies[0], "model").String())
				require.Equal(t, test.wantEffort, gjson.GetBytes(upstream.bodies[0], effortPath).String())
				require.Equal(t, test.effort, *service.RequestedReasoningEffortFromContext(ginContext.Request.Context()))
			})
		}
	}
}
