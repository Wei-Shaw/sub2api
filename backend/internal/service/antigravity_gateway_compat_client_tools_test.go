package service

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const antigravityCustomToolRequest = `{
  "model": "gemini-3.8-flash-high",
  "stream": true,
  "input": "create hello.txt",
  "tools": [
    {"type": "custom", "name": "apply_patch", "description": "Edit files with a patch.",
     "format": {"type": "grammar", "syntax": "lark", "definition": "start: /(.|\\n)+/"}},
    {"type": "function", "name": "exec_command", "description": "Run a command.",
     "parameters": {"type": "object", "properties": {"cmd": {"type": "string"}}, "required": ["cmd"]}}
  ]
}`

const antigravityApplyPatchInput = "*** Begin Patch\n*** Add File: hello.txt\n+hi\n*** End Patch"

func antigravityCustomToolMapping(t *testing.T) apicompat.ResponsesClientToolMapping {
	t.Helper()
	_, mapping, err := adaptResponsesClientToolsForAnthropic([]byte(antigravityCustomToolRequest))
	require.NoError(t, err)
	require.True(t, mapping.CustomTools["apply_patch"])
	return mapping
}

func antigravityApplyPatchFrame(t *testing.T) string {
	t.Helper()
	args, err := json.Marshal(map[string]string{"input": antigravityApplyPatchInput})
	require.NoError(t, err)
	return `data: {"response":{"responseId":"resp_custom","candidates":[{"content":{"parts":[{"functionCall":{"id":"call_patch","name":"apply_patch","args":` +
		string(args) + `}}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":8,"candidatesTokenCount":3}}}` + "\n\n"
}

// The request side must lower the freeform tool to a function that carries an
// "input" string, instead of an empty-object schema the model cannot fill.
func TestAntigravityResponsesLowersCustomToolsBeforeClaudeConversion(t *testing.T) {
	adapted, _, err := adaptResponsesClientToolsForAnthropic([]byte(antigravityCustomToolRequest))
	require.NoError(t, err)

	var request apicompat.ResponsesRequest
	require.NoError(t, json.Unmarshal(adapted, &request))
	claudeRequest, err := apicompat.ResponsesToAnthropicRequest(&request)
	require.NoError(t, err)

	claudeBody, err := json.Marshal(claudeRequest)
	require.NoError(t, err)
	tool := gjson.GetBytes(claudeBody, `tools.#(name=="apply_patch")`)
	require.True(t, tool.Exists(), string(claudeBody))
	require.Equal(t, "string", tool.Get("input_schema.properties.input.type").String(), tool.Raw)
	require.Contains(t, tool.Get("input_schema.required").Raw, "input")

	other := gjson.GetBytes(claudeBody, `tools.#(name=="exec_command")`)
	require.Equal(t, "string", other.Get("input_schema.properties.cmd.type").String())
}

func TestAntigravityResponsesStreamRestoresCustomToolCall(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSize}, nil)
	c, recorder := newAntigravityCompatContext(http.MethodPost, "/v1/responses", nil)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(antigravityApplyPatchFrame(t))),
	}

	_, err := svc.handleResponsesStreamingFromAntigravityWithClientTools(c, resp, time.Now(), "gemini-3.8-flash-high", antigravityCustomToolMapping(t))
	require.NoError(t, err)

	out := recorder.Body.String()
	require.Contains(t, out, `"type":"custom_tool_call"`)
	require.Contains(t, out, "event: response.custom_tool_call_input.done")
	require.NotContains(t, out, "event: response.function_call_arguments.delta")

	var doneInput string
	for _, line := range strings.Split(out, "\n") {
		payload, ok := strings.CutPrefix(line, "data: ")
		if !ok {
			continue
		}
		if gjson.Get(payload, "type").String() == "response.custom_tool_call_input.done" {
			doneInput = gjson.Get(payload, "input").String()
		}
	}
	require.Equal(t, antigravityApplyPatchInput, doneInput)
}

func TestAntigravityResponsesNonStreamRestoresCustomToolCall(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSize}, nil)
	c, recorder := newAntigravityCompatContext(http.MethodPost, "/v1/responses", nil)
	resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(antigravityApplyPatchFrame(t)))}

	_, err := svc.handleResponsesNonStreamingFromAntigravityWithClientTools(c, resp, time.Now(), "gemini-3.8-flash-high", antigravityCustomToolMapping(t))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, recorder.Code)

	item := gjson.GetBytes(recorder.Body.Bytes(), `output.#(name=="apply_patch")`)
	require.True(t, item.Exists(), recorder.Body.String())
	require.Equal(t, "custom_tool_call", item.Get("type").String())
	require.Equal(t, antigravityApplyPatchInput, item.Get("input").String())
}

// Without client tools the output must keep the plain function_call shape.
func TestAntigravityResponsesStreamWithoutClientToolsKeepsFunctionCall(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSize}, nil)
	c, recorder := newAntigravityCompatContext(http.MethodPost, "/v1/responses", nil)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(antigravityApplyPatchFrame(t))),
	}

	_, err := svc.handleResponsesStreamingFromAntigravity(c, resp, time.Now(), "gemini-3.8-flash-high")
	require.NoError(t, err)
	out := recorder.Body.String()
	require.Contains(t, out, `"type":"function_call"`)
	require.NotContains(t, out, "custom_tool_call")
}
