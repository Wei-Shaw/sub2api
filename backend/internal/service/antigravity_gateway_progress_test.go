package service

import (
	"context"
	"encoding/json"
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

func TestAntigravityCompatResponsesToolProgressFallback(t *testing.T) {
	for _, tc := range []struct {
		name    string
		enabled bool
		text    string
	}{
		{"silent_model", true, ""},
		{"native_progress", true, "I will inspect the project."},
		{"caller_did_not_request_progress", false, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			svc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSize}, nil)
			c, recorder := newAntigravityCompatContext(http.MethodPost, "/v1/responses", nil)
			c.Set("antigravity_tool_progress_fallback", tc.enabled)
			parts := `{"functionCall":{"id":"call_progress","name":"inspect_project","args":{"path":"."}}}`
			if tc.text != "" {
				parts = `{"text":"` + tc.text + `"},` + parts
			}
			body := `data: {"response":{"candidates":[{"content":{"parts":[` + parts + `]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":8,"candidatesTokenCount":12}}}` + "\n\n"
			resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(body))}
			result, err := svc.handleResponsesStreamingFromAntigravity(c, resp, time.Now(), "gemini-3.8-flash-high")
			require.NoError(t, err)
			require.NotNil(t, result)
			var text strings.Builder
			var order []string
			for _, line := range strings.Split(recorder.Body.String(), "\n") {
				if !strings.HasPrefix(line, "data: {") {
					continue
				}
				frame := gjson.Parse(strings.TrimPrefix(line, "data: "))
				if frame.Get("type").String() == "response.output_text.delta" {
					_, _ = text.WriteString(frame.Get("delta").String())
					order = append(order, "text")
				}
				if frame.Get("type").String() == "response.output_item.added" && frame.Get("item.type").String() == "function_call" {
					order = append(order, "tool")
					require.Equal(t, "inspect_project", frame.Get("item.name").String())
				}
			}
			if tc.enabled {
				require.NotEmpty(t, text.String(), "silent tool rounds must still carry visible tool progress")
				require.Equal(t, "text", order[0])
				if tc.text == "" {
					require.Contains(t, text.String(), "工具进度")
					require.Contains(t, text.String(), "inspect_project")
				} else {
					require.Equal(t, tc.text, text.String(), "native progress must not get a duplicate notice")
				}
			} else {
				require.Empty(t, text.String())
			}
			require.Equal(t, "tool", order[len(order)-1])
			require.Equal(t, 12, result.usage.OutputTokens)
		})
	}
}

func TestAntigravityCompatPrepareToolProgressScope(t *testing.T) {
	for _, tc := range []struct {
		name, instruction string
		choice            any
		want              bool
	}{
		{"requested_progress", "Before each tool batch, briefly state its purpose.", "auto", true},
		{"unrequested_progress", "You are a coding assistant.", "auto", false},
		{"forced_required", "Before each tool batch, briefly state its purpose.", "required", false},
		{"forced_named_function", "Before each tool batch, briefly state its purpose.", map[string]any{"type": "function", "name": "inspect_project"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			const model = "gemini-3.8-flash-high"
			original, err := json.Marshal(map[string]any{"model": model, "tool_choice": tc.choice})
			require.NoError(t, err)
			claude, err := json.Marshal(map[string]any{
				"model": model, "max_tokens": 1024, "stream": true, "system": tc.instruction,
				"messages": []any{map[string]any{"role": "user", "content": "Inspect the project."}},
				"tools":    []any{map[string]any{"name": "inspect_project", "input_schema": map[string]any{"type": "object", "properties": map[string]any{}}}},
			})
			require.NoError(t, err)
			account := newAntigravityCompatAccount(AccountTypeOAuth)
			account.Credentials["model_mapping"] = map[string]any{model: model}
			svc := newAntigravityCompatService(config.GatewayConfig{}, nil)
			c, _ := newAntigravityCompatContext(http.MethodPost, "/v1/responses", original)
			call, err := svc.prepareAntigravityCompatCall(context.Background(), c, account, antigravityCompatRequest{
				originalModel: model, originalBody: original, claudeBody: claude,
			})
			require.NoError(t, err)
			require.NotNil(t, call)
			require.Equal(t, tc.want, c.GetBool("antigravity_tool_progress_fallback"))
		})
	}
}
