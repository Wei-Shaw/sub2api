//go:build unit

package service

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestAntigravityCompatPrepareThinkingMappingBoundaries(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, testCase := range []struct {
		name       string
		config     string
		wantLevel  string
		wantClaude string
	}{
		{name: "absent", wantLevel: "high", wantClaude: "claude-sonnet-4-5"},
		{name: "effort_none", config: `,"output_config":{"effort":"none"}`, wantLevel: "low", wantClaude: "claude-sonnet-4-5"},
		{name: "effort_minimal", config: `,"output_config":{"effort":"minimal"}`, wantLevel: "low", wantClaude: "claude-sonnet-4-5"},
		{name: "effort_low", config: `,"output_config":{"effort":"low"}`, wantLevel: "low", wantClaude: "claude-sonnet-4-5"},
		{name: "disabled_overrides_effort_and_budget", config: `,"thinking":{"type":"disabled","budget_tokens":8193},"output_config":{"effort":"high"}`, wantLevel: "low", wantClaude: "claude-sonnet-4-5"},
		{name: "adaptive_default", config: `,"thinking":{"type":"adaptive"}`, wantLevel: "high", wantClaude: "claude-sonnet-4-5-thinking"},
		{name: "adaptive_low", config: `,"thinking":{"type":"adaptive"},"output_config":{"effort":"low"}`, wantLevel: "low", wantClaude: "claude-sonnet-4-5-thinking"},
		{name: "adaptive_medium", config: `,"thinking":{"type":"adaptive"},"output_config":{"effort":"medium"}`, wantLevel: "medium", wantClaude: "claude-sonnet-4-5-thinking"},
		{name: "enabled_no_budget", config: `,"thinking":{"type":"enabled"}`, wantLevel: "high", wantClaude: "claude-sonnet-4-5-thinking"},
		{name: "negative_budget", config: `,"thinking":{"type":"enabled","budget_tokens":-1}`, wantLevel: "high", wantClaude: "claude-sonnet-4-5-thinking"},
		{name: "zero_budget", config: `,"thinking":{"type":"enabled","budget_tokens":0}`, wantLevel: "high", wantClaude: "claude-sonnet-4-5-thinking"},
		{name: "budget_one", config: `,"thinking":{"type":"enabled","budget_tokens":1}`, wantLevel: "low", wantClaude: "claude-sonnet-4-5-thinking"},
		{name: "budget_low_max", config: `,"thinking":{"type":"enabled","budget_tokens":1024}`, wantLevel: "low", wantClaude: "claude-sonnet-4-5-thinking"},
		{name: "budget_medium_min", config: `,"thinking":{"type":"enabled","budget_tokens":1025}`, wantLevel: "medium", wantClaude: "claude-sonnet-4-5-thinking"},
		{name: "budget_medium_max", config: `,"thinking":{"type":"enabled","budget_tokens":8192}`, wantLevel: "medium", wantClaude: "claude-sonnet-4-5-thinking"},
		{name: "budget_high_min", config: `,"thinking":{"type":"enabled","budget_tokens":8193}`, wantLevel: "high", wantClaude: "claude-sonnet-4-5-thinking"},
		{name: "effort_overrides_budget", config: `,"thinking":{"type":"enabled","budget_tokens":8193},"output_config":{"effort":"low"}`, wantLevel: "low", wantClaude: "claude-sonnet-4-5-thinking"},
	} {
		for _, family := range []string{"gemini", "claude"} {
			t.Run(testCase.name+"/"+family, func(t *testing.T) {
				upstream := &queuedHTTPUpstreamStub{}
				svc := newAntigravityCompatService(config.GatewayConfig{}, upstream)
				account := newAntigravityCompatAccount(AccountTypeOAuth)
				mapping := map[string]any{}
				for _, level := range []string{"low", "medium", "high"} {
					target := "gemini-3.8-flash-" + level
					if family == "claude" {
						target = "claude-sonnet-4-5"
					}
					mapping["gemini-3.8-flash-"+level] = target
				}
				account.Credentials["model_mapping"] = mapping
				body := []byte(`{"model":"gemini-3.8-flash","max_tokens":20000,"messages":[{"role":"user","content":"hello"}]` + testCase.config + `}`)
				ginContext, _ := newAntigravityCompatContext(http.MethodPost, "/v1/chat/completions", body)
				call, err := svc.prepareAntigravityCompatCall(context.Background(), ginContext, account, antigravityCompatRequest{
					claudeBody:    body,
					originalBody:  body,
					originalModel: "gemini-3.8-flash",
				})
				require.NoError(t, err)
				require.NotNil(t, call)
				wantModel := "gemini-3.8-flash-" + testCase.wantLevel
				if family == "claude" {
					wantModel = testCase.wantClaude
				}
				require.Equal(t, wantModel, call.billingModel)
				require.Equal(t, wantModel, gjson.GetBytes(call.geminiBody, "model").String())
				require.Empty(t, upstream.requestBodies)
			})
		}
	}
}

func TestAntigravityChatInputAudioEffortMappingBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, testCase := range []struct {
		effort string
		level  string
	}{
		{effort: "low", level: "low"},
		{effort: "minimal", level: "low"},
		{effort: "none", level: "low"},
		{effort: "medium", level: "medium"},
	} {
		for _, allowAudio := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/allow_audio=%t", testCase.effort, allowAudio), func(t *testing.T) {
				body := []byte(fmt.Sprintf(`{"model":"gemini-3.8-flash","reasoning_effort":%q,"messages":[{"role":"user","content":[{"type":"input_audio","input_audio":{"data":"YXVkaW8=","format":"wav"}}]}]}`, testCase.effort))
				ginContext, recorder := newAntigravityCompatContext(http.MethodPost, "/v1/chat/completions", body)
				upstream := &queuedHTTPUpstreamStub{responses: []*http.Response{antigravityCompatSuccessResponse()}}
				svc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSize}, upstream)
				account := newAntigravityCompatAccount(AccountTypeOAuth)
				defaultModel := "gemini-3.8-flash-high"
				selectedModel := "claude-sonnet-4-5"
				if allowAudio {
					defaultModel = "claude-sonnet-4-5"
					selectedModel = "gemini-3.8-flash-" + testCase.level
				}
				account.Credentials["model_mapping"] = map[string]any{
					"gemini-3.8-flash-high":              defaultModel,
					"gemini-3.8-flash-" + testCase.level: selectedModel,
				}
				result, err := svc.ForwardAsChatCompletions(context.Background(), ginContext, account, body, nil)
				if !allowAudio {
					require.Error(t, err)
					require.Nil(t, result)
					var validationErr *apicompat.InputAudioValidationError
					require.ErrorAs(t, err, &validationErr)
					require.Equal(t, http.StatusBadRequest, recorder.Code)
					require.Equal(t, "invalid_request_error", gjson.Get(recorder.Body.String(), "error.type").String())
					require.Empty(t, upstream.requestBodies)
					return
				}
				require.NoError(t, err)
				require.NotNil(t, result)
				require.Equal(t, http.StatusOK, recorder.Code)
				require.Len(t, upstream.requestBodies, 1)
				sent := upstream.requestBodies[0]
				require.Equal(t, selectedModel, gjson.GetBytes(sent, "model").String())
				require.Equal(t, "audio/wav", gjson.GetBytes(sent, "request.contents.0.parts.0.inlineData.mimeType").String())
				require.Equal(t, "YXVkaW8=", gjson.GetBytes(sent, "request.contents.0.parts.0.inlineData.data").String())
			})
		}
	}
}
