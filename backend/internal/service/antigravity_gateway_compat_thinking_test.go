package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestApplyAntigravityCompatGeminiThinkingConfig(t *testing.T) {
	budget := func(v int64) *int64 { return &v }
	const baseReq = `{"contents":[{"role":"user","parts":[{"text":"hi"}]}],"generationConfig":{"maxOutputTokens":8192}}`
	tests := []struct {
		name         string
		model        string
		claudeBody   string
		wantLevel    string
		wantBudget   *int64
		wantThoughts bool
		wantNoConfig bool
	}{
		{name: "tier suffix model still gets explicit level", model: "gemini-4-argon-high", claudeBody: `{"output_config":{"effort":"high"},"thinking":{"type":"enabled","budget_tokens":10240}}`, wantLevel: "high", wantThoughts: true},
		{name: "tier suffix model follows requested low effort", model: "gemini-4-argon-high", claudeBody: `{"output_config":{"effort":"low"}}`, wantLevel: "low", wantThoughts: true},
		{name: "bare gemini 3 model", model: "gemini-3.5-flash", claudeBody: `{"output_config":{"effort":"high"},"thinking":{"type":"enabled","budget_tokens":10240}}`, wantLevel: "high", wantThoughts: true},
		{name: "medium on flash", model: "gemini-3.8-flash-medium", claudeBody: `{"output_config":{"effort":"medium"},"thinking":{"type":"enabled","budget_tokens":4096}}`, wantLevel: "medium", wantThoughts: true},
		{name: "budget only", model: "gemini-3.8-flash-low", claudeBody: `{"thinking":{"type":"enabled","budget_tokens":31999}}`, wantLevel: "high", wantThoughts: true},
		{name: "adaptive without effort uses name tier", model: "gemini-3.8-flash-low", claudeBody: `{"thinking":{"type":"adaptive"}}`, wantLevel: "low", wantThoughts: true},
		{name: "adaptive without effort defaults to high", model: "gemini-4-argon", claudeBody: `{"thinking":{"type":"adaptive"}}`, wantLevel: "high", wantThoughts: true},
		{name: "effort none lowers level without thoughts", model: "gemini-4-argon-high", claudeBody: `{"output_config":{"effort":"none"},"thinking":{"type":"enabled","budget_tokens":10240}}`, wantLevel: "low"},
		{name: "thinking disabled on flash", model: "gemini-3.8-flash-high", claudeBody: `{"thinking":{"type":"disabled"}}`, wantLevel: "minimal"},
		{name: "2.5 flash high", model: "gemini-2.5-flash", claudeBody: `{"output_config":{"effort":"high"},"thinking":{"type":"enabled","budget_tokens":10240}}`, wantBudget: budget(24576), wantThoughts: true},
		{name: "2.5 adaptive uses dynamic budget", model: "gemini-2.5-flash", claudeBody: `{"thinking":{"type":"adaptive"}}`, wantBudget: budget(-1), wantThoughts: true},
		{name: "2.5 flash none disables thinking", model: "gemini-2.5-flash", claudeBody: `{"output_config":{"effort":"none"}}`, wantBudget: budget(0)},
		{name: "2.5 pro cannot disable thinking", model: "gemini-2.5-pro", claudeBody: `{"thinking":{"type":"disabled"}}`, wantBudget: budget(128)},
		{name: "no thinking intent keeps request untouched", model: "gemini-4-argon-high", claudeBody: `{}`, wantNoConfig: true},
		{name: "unknown effort keeps request untouched", model: "gemini-4-argon-high", claudeBody: `{"output_config":{"effort":"turbo"}}`, wantNoConfig: true},
		{name: "image model untouched", model: "gemini-3-pro-image-preview", claudeBody: `{"output_config":{"effort":"high"}}`, wantNoConfig: true},
		{name: "non gemini model untouched", model: "claude-sonnet-4-5", claudeBody: `{"output_config":{"effort":"high"}}`, wantNoConfig: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := applyAntigravityCompatGeminiThinkingConfig([]byte(baseReq), []byte(tt.claudeBody), tt.model)
			require.Equal(t, int64(8192), gjson.GetBytes(out, "generationConfig.maxOutputTokens").Int())
			cfg := gjson.GetBytes(out, "generationConfig.thinkingConfig")
			if tt.wantNoConfig {
				require.False(t, cfg.Exists(), cfg.Raw)
				return
			}
			require.Equal(t, tt.wantLevel, cfg.Get("thinkingLevel").String(), cfg.Raw)
			if tt.wantBudget != nil {
				require.True(t, cfg.Get("thinkingBudget").Exists(), cfg.Raw)
				require.Equal(t, *tt.wantBudget, cfg.Get("thinkingBudget").Int())
			} else {
				require.False(t, cfg.Get("thinkingBudget").Exists(), cfg.Raw)
			}
			require.Equal(t, tt.wantThoughts, cfg.Get("includeThoughts").Bool(), cfg.Raw)
		})
	}
}

func newAntigravityCompatThinkingAccount() *Account {
	account := newAntigravityCompatAccount(AccountTypeOAuth)
	mapping, ok := account.Credentials["model_mapping"].(map[string]any)
	if !ok {
		panic("antigravity compat test account has no model_mapping")
	}
	mapping["gemini-4-argon-high"] = "gemini-4-argon-high"
	mapping["gemini-3.5-flash"] = "gemini-3.5-flash"
	mapping["gemini-2.5-flash"] = "gemini-2.5-flash"
	return account
}

func TestAntigravityCompatGeminiForwardsThinkingConfig(t *testing.T) {
	gin.SetMode(gin.TestMode)
	budget := func(v int64) *int64 { return &v }
	tests := []struct {
		name         string
		path         string
		body         string
		wantModel    string
		wantLevel    string
		wantBudget   *int64
		wantThoughts bool
		wantNoConfig bool
		wantEffort   string
	}{
		{
			name:         "chat reasoning_effort high on tier suffix model",
			path:         "/v1/chat/completions",
			body:         `{"model":"gemini-4-argon-high","messages":[{"role":"user","content":"hi"}],"reasoning_effort":"high"}`,
			wantModel:    "gemini-4-argon-high",
			wantLevel:    "high",
			wantThoughts: true,
			wantEffort:   "high",
		},
		{
			name:         "chat reasoning_effort high on bare gemini 3 model",
			path:         "/v1/chat/completions",
			body:         `{"model":"gemini-3.5-flash","messages":[{"role":"user","content":"hi"}],"reasoning_effort":"high"}`,
			wantModel:    "gemini-3.5-flash",
			wantLevel:    "high",
			wantThoughts: true,
			wantEffort:   "high",
		},
		{
			name:         "chat reasoning_effort high on gemini 2.5 uses budget",
			path:         "/v1/chat/completions",
			body:         `{"model":"gemini-2.5-flash","messages":[{"role":"user","content":"hi"}],"reasoning_effort":"high"}`,
			wantModel:    "gemini-2.5-flash",
			wantBudget:   budget(24576),
			wantThoughts: true,
			wantEffort:   "high",
		},
		{
			name:         "chat without reasoning keeps existing request",
			path:         "/v1/chat/completions",
			body:         `{"model":"gemini-4-argon-high","messages":[{"role":"user","content":"hi"}]}`,
			wantModel:    "gemini-4-argon-high",
			wantNoConfig: true,
		},
		{
			name:         "responses reasoning effort high on tier suffix model",
			path:         "/v1/responses",
			body:         `{"model":"gemini-4-argon-high","input":"hi","reasoning":{"effort":"high"}}`,
			wantModel:    "gemini-4-argon-high",
			wantLevel:    "high",
			wantThoughts: true,
			wantEffort:   "high",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstream := &queuedHTTPUpstreamStub{responses: []*http.Response{antigravityCompatSuccessResponse()}}
			svc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSize}, upstream)
			body := []byte(tt.body)
			c, _ := newAntigravityCompatContext(http.MethodPost, tt.path, body)
			account := newAntigravityCompatThinkingAccount()

			var (
				result *ForwardResult
				err    error
			)
			if tt.path == "/v1/responses" {
				result, err = svc.ForwardAsResponses(context.Background(), c, account, body, nil)
			} else {
				result, err = svc.ForwardAsChatCompletions(context.Background(), c, account, body, nil)
			}

			require.NoError(t, err)
			require.NotNil(t, result)
			require.Len(t, upstream.requestBodies, 1)
			sent := upstream.requestBodies[0]
			require.Equal(t, tt.wantModel, gjson.GetBytes(sent, "model").String())
			require.Equal(t, tt.wantModel, result.UpstreamModel)
			if tt.wantEffort != "" {
				require.NotNil(t, result.ReasoningEffort)
				require.Equal(t, tt.wantEffort, *result.ReasoningEffort)
			}
			cfg := gjson.GetBytes(sent, "request.generationConfig.thinkingConfig")
			if tt.wantNoConfig {
				require.False(t, cfg.Exists(), cfg.Raw)
				return
			}
			require.Equal(t, tt.wantLevel, cfg.Get("thinkingLevel").String(), cfg.Raw)
			if tt.wantBudget != nil {
				require.Equal(t, *tt.wantBudget, cfg.Get("thinkingBudget").Int(), cfg.Raw)
			} else {
				require.False(t, cfg.Get("thinkingBudget").Exists(), cfg.Raw)
			}
			require.Equal(t, tt.wantThoughts, cfg.Get("includeThoughts").Bool(), cfg.Raw)
		})
	}
}

func antigravityCompatThoughtStreamResponse() *http.Response {
	body := `data: {"response":{"responseId":"resp_thought","candidates":[{"content":{"parts":[{"text":"plan it","thought":true}]}}]}}` + "\n\n" +
		`data: {"response":{"responseId":"resp_thought","candidates":[{"content":{"parts":[{"text":"answer"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":8,"candidatesTokenCount":3,"thoughtsTokenCount":5}}}` + "\n\n"
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestAntigravityCompatChatMapsGeminiThoughtsToReasoningContent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSize}, nil)

	t.Run("stream", func(t *testing.T) {
		c, recorder := newAntigravityCompatContext(http.MethodPost, "/v1/chat/completions", nil)
		result, err := svc.handleChatCompletionsStreamingFromAntigravity(c, antigravityCompatThoughtStreamResponse(), time.Now(), "gemini-4-argon-high", true)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Contains(t, recorder.Body.String(), `"reasoning_content":"plan it"`)
		require.Contains(t, recorder.Body.String(), `"content":"answer"`)
	})

	t.Run("non-stream", func(t *testing.T) {
		c, recorder := newAntigravityCompatContext(http.MethodPost, "/v1/chat/completions", nil)
		result, err := svc.handleChatCompletionsNonStreamingFromAntigravity(c, antigravityCompatThoughtStreamResponse(), time.Now(), "gemini-4-argon-high")
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, "plan it", gjson.Get(recorder.Body.String(), "choices.0.message.reasoning_content").String(), recorder.Body.String())
		require.Equal(t, "answer", gjson.Get(recorder.Body.String(), "choices.0.message.content").String(), recorder.Body.String())
	})
}
