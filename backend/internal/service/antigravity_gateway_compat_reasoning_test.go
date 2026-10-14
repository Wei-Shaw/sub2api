package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func antigravityCompatReasoningAccount(t *testing.T) *Account {
	t.Helper()

	account := newAntigravityCompatAccount(AccountTypeOAuth)
	mapping, ok := account.Credentials["model_mapping"].(map[string]any)
	require.True(t, ok, "model_mapping should be map[string]any")
	mapping["gemini-3.8-flash-low"] = "gemini-3.8-flash-low"
	mapping["gemini-3.8-flash-medium"] = "gemini-3.8-flash-medium"
	mapping["gemini-3.8-flash-high"] = "gemini-3.8-flash-high"
	return account
}

func antigravityCompatThinkingResponse() *http.Response {
	body := strings.Join([]string{
		`data: {"response":{"responseId":"resp_reasoning","candidates":[{"content":{"parts":[{"text":"provider thought","thought":true,"thoughtSignature":"sig_reasoning"}]}}]}}`,
		`data: {"response":{"candidates":[{"content":{"parts":[{"text":"final answer"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":13,"candidatesTokenCount":7}}}`,
	}, "\n\n") + "\n\n"
	return &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"text/event-stream"},
			"X-Request-Id": []string{"request-reasoning-test"},
		},
		Body: io.NopCloser(strings.NewReader(body)),
	}
}

func TestAntigravityCompatChatCompletionsAppliesExplicitGeminiThinking(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		effort    string
		wantModel string
		wantLevel string
	}{
		{effort: "low", wantModel: "gemini-3.8-flash-low", wantLevel: "LOW"},
		{effort: "medium", wantModel: "gemini-3.8-flash-medium", wantLevel: "MEDIUM"},
		{effort: "high", wantModel: "gemini-3.8-flash-high", wantLevel: "HIGH"},
		{effort: "xhigh", wantModel: "gemini-3.8-flash-high", wantLevel: "HIGH"},
	}

	for _, tt := range tests {
		t.Run(tt.effort, func(t *testing.T) {
			upstream := &queuedHTTPUpstreamStub{
				responses: []*http.Response{antigravityCompatThinkingResponse()},
			}
			svc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSize}, upstream)
			body := []byte(`{"model":"gemini-3.8-flash","messages":[{"role":"user","content":"think"}],"reasoning_effort":"` + tt.effort + `","max_tokens":4096,"stream":true}`)
			c, recorder := newAntigravityCompatContext(http.MethodPost, "/v1/chat/completions", body)

			result, err := svc.ForwardAsChatCompletions(
				context.Background(),
				c,
				antigravityCompatReasoningAccount(t),
				body,
				nil,
			)

			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, http.StatusOK, recorder.Code)
			require.Equal(t, "text/event-stream", recorder.Header().Get("Content-Type"))
			require.Len(t, upstream.requestBodies, 1)
			require.Equal(t, tt.wantModel, gjson.GetBytes(upstream.requestBodies[0], "model").String())
			require.Equal(t, tt.wantLevel, gjson.GetBytes(upstream.requestBodies[0], "request.generationConfig.thinkingConfig.thinkingLevel").String())
			require.True(t, gjson.GetBytes(upstream.requestBodies[0], "request.generationConfig.thinkingConfig.includeThoughts").Bool())
			require.Contains(t, recorder.Body.String(), `"reasoning_content":"provider thought"`)
			require.Contains(t, recorder.Body.String(), `"content":"final answer"`)
			require.Contains(t, recorder.Body.String(), "data: [DONE]")
		})
	}
}

func TestAntigravityCompatChatCompletionsWithoutEffortKeepsExistingBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &queuedHTTPUpstreamStub{
		responses: []*http.Response{antigravityCompatSuccessResponse()},
	}
	svc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSize}, upstream)
	body := []byte(`{"model":"gemini-3.8-flash","messages":[{"role":"user","content":"answer"}],"stream":true}`)
	c, _ := newAntigravityCompatContext(http.MethodPost, "/v1/chat/completions", body)

	_, err := svc.ForwardAsChatCompletions(
		context.Background(),
		c,
		antigravityCompatReasoningAccount(t),
		body,
		nil,
	)

	require.NoError(t, err)
	require.Len(t, upstream.requestBodies, 1)
	require.Equal(t, "gemini-3.8-flash-high", gjson.GetBytes(upstream.requestBodies[0], "model").String())
	require.False(t, gjson.GetBytes(upstream.requestBodies[0], "request.generationConfig.thinkingConfig").Exists())
}
