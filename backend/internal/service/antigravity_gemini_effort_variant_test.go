package service

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestGeminiThinkingLevelFromClaudeRequestPrecedence(t *testing.T) {
	for _, tc := range []struct {
		name, body, effort, want string
	}{
		{"disabled wins", `{"thinking":{"type":"disabled","budget_tokens":20000},"output_config":{"effort":"high"}}`, "high", "low"},
		{"budget low wins over effort high", `{"thinking":{"type":"enabled","budget_tokens":1024},"output_config":{"effort":"high"}}`, "high", "low"},
		{"budget medium wins over effort low", `{"thinking":{"type":"adaptive","budget_tokens":4000},"output_config":{"effort":"low"}}`, "low", "medium"},
		{"budget high wins over effort low", `{"thinking":{"type":"enabled","budget_tokens":20000},"output_config":{"effort":"low"}}`, "low", "high"},
		{"zero budget retains existing high semantics", `{"thinking":{"type":"enabled","budget_tokens":0},"output_config":{"effort":"low"}}`, "low", "high"},
		{"dynamic budget retains existing high semantics", `{"thinking":{"type":"enabled","budget_tokens":-1},"output_config":{"effort":"low"}}`, "low", "high"},
		{"adaptive output effort", `{"thinking":{"type":"adaptive"},"output_config":{"effort":"low"}}`, "high", "low"},
		{"output effort without thinking", `{"output_config":{"effort":"medium"}}`, "low", "medium"},
		{"enabled without budget can use effort", `{"thinking":{"type":"enabled"}}`, "low", "low"},
		{"none", `{}`, "none", "low"},
		{"minimal", `{}`, "minimal", "low"},
		{"xhigh", `{}`, "xhigh", "high"},
		{"max", `{}`, "max", "high"},
		{"unknown output effort falls back to reasoning effort", `{"output_config":{"effort":"unknown"}}`, "low", "low"},
		{"unknown reasoning effort defaults high", `{}`, "unknown", "high"},
		{"missing effort defaults high", `{}`, "", "high"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, geminiThinkingLevelFromClaudeRequest([]byte(tc.body), &tc.effort))
		})
	}
	require.Equal(t, "high", geminiThinkingLevelFromClaudeRequest([]byte(`{}`), nil))
}

func forwardGeminiEffortVariantFixture(t *testing.T, route, model, extra string, mapping map[string]any) (*ForwardResult, []byte) {
	t.Helper()
	upstream := &queuedHTTPUpstreamStub{responses: []*http.Response{antigravityCompatSuccessResponse()}}
	svc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSize}, upstream)
	account := newAntigravityCompatAccount(AccountTypeOAuth)
	account.Credentials["model_mapping"] = mapping
	content := `"messages":[{"role":"user","content":"hello"}]`
	switch route {
	case "responses":
		content = `"input":"hello"`
	case "gemini":
		content = `"contents":[{"role":"user","parts":[{"text":"hello"}]}]`
	}
	body := []byte(fmt.Sprintf(`{"model":%q,%s,"max_tokens":8192%s}`, model, content, extra))
	c, recorder := newAntigravityCompatContext(http.MethodPost, "/v1/"+route, body)
	var result *ForwardResult
	var err error
	switch route {
	case "chat":
		result, err = svc.ForwardAsChatCompletions(context.Background(), c, account, body, nil)
	case "responses":
		result, err = svc.ForwardAsResponses(context.Background(), c, account, body, nil)
	case "messages":
		result, err = svc.Forward(context.Background(), c, account, body, false)
	case "gemini":
		result, err = svc.ForwardGemini(context.Background(), c, account, model, "generateContent", false, body, false)
	default:
		t.Fatalf("unknown fixture route %q", route)
	}
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Len(t, upstream.requestBodies, 1)
	return result, upstream.requestBodies[0]
}

func TestAntigravityGeminiEffortVariantsAcrossProtocols(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, model := range []string{"gemini-4-argon", "gemini-3.8-flash"} {
		mapping := map[string]any{model: model}
		for _, level := range []string{"low", "medium", "high"} {
			mapping[model+"-"+level] = model + "-" + level
		}
		for _, tc := range []struct{ effort, level string }{
			{"low", "low"}, {"medium", "medium"}, {"high", "high"},
			{"xhigh", "high"}, {"max", "high"}, {"minimal", "low"}, {"none", "low"}, {"", "high"},
		} {
			for _, route := range []string{"chat", "responses", "messages", "gemini"} {
				t.Run(model+"/"+route+"/effort="+tc.effort, func(t *testing.T) {
					extra := ""
					if tc.effort != "" {
						switch route {
						case "chat":
							extra = fmt.Sprintf(`,"reasoning_effort":%q`, tc.effort)
						case "responses":
							extra = fmt.Sprintf(`,"reasoning":{"effort":%q}`, tc.effort)
						case "messages":
							extra = fmt.Sprintf(`,"thinking":{"type":"adaptive"},"output_config":{"effort":%q}`, tc.effort)
						case "gemini":
							extra = fmt.Sprintf(`,"generationConfig":{"thinkingConfig":{"thinkingLevel":%q}}`, tc.level)
						}
					}
					result, upstream := forwardGeminiEffortVariantFixture(t, route, model, extra, mapping)
					require.Equal(t, model+"-"+tc.level, result.UpstreamModel)
					require.Equal(t, model+"-"+tc.level, gjson.GetBytes(upstream, "model").String())
				})
			}
		}
	}
}

func TestAntigravityGeminiEffortVariantBudgetsSuffixAndFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const model = "gemini-4-argon"
	for _, tc := range []struct {
		name, route, requested, extra, want string
		omitLow                             bool
	}{
		{"explicit budget overrides output effort", "messages", model, `,"thinking":{"type":"enabled","budget_tokens":4000},"output_config":{"effort":"low"}`, model + "-medium", false},
		{"disabled overrides effort", "messages", model, `,"thinking":{"type":"disabled"},"output_config":{"effort":"high"}`, model + "-low", false},
		{"chat explicit client budget overrides effort", "chat", model, `,"thinking":{"type":"enabled","budget_tokens":4000},"reasoning_effort":"low"`, model + "-medium", false},
		{"responses explicit client budget overrides effort", "responses", model, `,"thinking":{"type":"enabled","budget_tokens":4000},"reasoning":{"effort":"low"}`, model + "-medium", false},
		{"suffixed chat model stays high", "chat", model + "-high", `,"reasoning_effort":"low"`, model + "-high", false},
		{"suffixed responses model stays high", "responses", model + "-high", `,"reasoning":{"effort":"low"}`, model + "-high", false},
		{"missing low falls back to high", "chat", model, `,"reasoning_effort":"low"`, model + "-high", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mapping := map[string]any{model: model, model + "-medium": model + "-medium", model + "-high": model + "-high"}
			if !tc.omitLow {
				mapping[model+"-low"] = model + "-low"
			}
			result, upstream := forwardGeminiEffortVariantFixture(t, tc.route, tc.requested, tc.extra, mapping)
			require.Equal(t, tc.want, result.UpstreamModel)
			require.Equal(t, tc.want, gjson.GetBytes(upstream, "model").String())
		})
	}
}
