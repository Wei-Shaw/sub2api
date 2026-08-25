//go:build unit

package service

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestAdaptResponsesClientToolsForAnthropic_FlattensNamespace(t *testing.T) {
	t.Parallel()

	body := []byte(`{
		"model":"claude-fable-5",
		"input":[{"type":"function_call","call_id":"call_1","namespace":"codex_app","name":"read_thread","arguments":"{}"}],
		"tools":[{"type":"namespace","name":"codex_app","tools":[{"type":"function","name":"read_thread","description":"Read a task","parameters":{"type":"object","properties":{}}}]}]
	}`)

	adapted, mapping, err := adaptResponsesClientToolsForAnthropic(body)
	require.NoError(t, err)
	require.Equal(t, apicompat.ResponsesNamespaceName{Namespace: "codex_app", Name: "read_thread"}, mapping.NamespaceTools["codex_app__read_thread"])

	var request map[string]any
	require.NoError(t, json.Unmarshal(adapted, &request))
	tools := request["tools"].([]any)
	require.Len(t, tools, 1)
	tool := tools[0].(map[string]any)
	require.Equal(t, "function", tool["type"])
	require.Equal(t, "codex_app__read_thread", tool["name"])

	input := request["input"].([]any)
	call := input[0].(map[string]any)
	require.Equal(t, "codex_app__read_thread", call["name"])
	require.NotContains(t, call, "namespace")
}

func TestAdaptResponsesClientToolsForAnthropic_LiftsAdditionalTools(t *testing.T) {
	body := []byte(`{
		"model":"claude-fable-5",
		"input":[
			{"type":"additional_tools","tools":[
				{"type":"custom","name":"exec","description":"Run a command"},
				{"type":"namespace","name":"codex_app","tools":[
					{"type":"function","name":"read_thread","parameters":{"type":"object"}}
				]}
			]},
			{"type":"message","role":"user","content":[{"type":"input_text","text":"inspect"}]}
		]
	}`)

	adapted, mapping, err := adaptResponsesClientToolsForAnthropic(body)
	require.NoError(t, err)
	require.True(t, mapping.CustomTools["exec"])
	require.Equal(t, apicompat.ResponsesNamespaceName{Namespace: "codex_app", Name: "read_thread"}, mapping.NamespaceTools["codex_app__read_thread"])

	var request map[string]any
	require.NoError(t, json.Unmarshal(adapted, &request))
	tools := request["tools"].([]any)
	require.Len(t, tools, 2)
	require.Equal(t, "function", tools[0].(map[string]any)["type"])
	require.Equal(t, "codex_app__read_thread", tools[1].(map[string]any)["name"])

	input := request["input"].([]any)
	require.Len(t, input, 1)
	require.Equal(t, "message", input[0].(map[string]any)["type"])
}

func namespaceToolAnthropicStream() string {
	return strings.Join([]string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"id":"msg_namespace","type":"message","role":"assistant","content":[],"model":"claude-fable-5","stop_reason":"","usage":{"input_tokens":10}}}`,
		``,
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_namespace","name":"codex_app__read_thread","input":{"thread_id":"123"}}}`,
		``,
		`event: content_block_stop`,
		`data: {"type":"content_block_stop","index":0}`,
		``,
		`event: message_delta`,
		`data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":5}}`,
		``,
		`event: message_stop`,
		`data: {"type":"message_stop"}`,
		``,
	}, "\n")
}

func namespaceToolMapping() apicompat.ResponsesClientToolMapping {
	return apicompat.ResponsesClientToolMapping{NamespaceTools: map[string]apicompat.ResponsesNamespaceName{
		"codex_app__read_thread": {Namespace: "codex_app", Name: "read_thread"},
	}}
}

func TestHandleResponsesBufferedStreamingResponse_RestoresNamespaceTool(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	resp := &http.Response{Body: io.NopCloser(strings.NewReader(namespaceToolAnthropicStream()))}

	svc := &GatewayService{}
	_, err := svc.handleResponsesBufferedStreamingResponse(resp, c, "claude-fable-5", "claude-fable-5", nil, time.Now(), namespaceToolMapping())
	require.NoError(t, err)
	require.Contains(t, rec.Body.String(), `"type":"function_call"`)
	require.Contains(t, rec.Body.String(), `"name":"read_thread"`)
	require.Contains(t, rec.Body.String(), `"namespace":"codex_app"`)
	require.NotContains(t, rec.Body.String(), `"name":"codex_app__read_thread"`)
}

func TestHandleResponsesBufferedStreamingResponse_NormalizesClaudeExecDialect(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	resp := &http.Response{Body: io.NopCloser(strings.NewReader(anthropicExecToolSSE(t, `console.log(process.cwd());`)))}
	mapping := apicompat.ResponsesClientToolMapping{CustomTools: map[string]bool{"exec": true}}

	svc := &GatewayService{}
	_, err := svc.handleResponsesBufferedStreamingResponse(
		resp, c, "claude-sonnet-5", "claude-sonnet-5", nil, time.Now(), mapping,
	)

	require.NoError(t, err)
	require.Equal(t, "custom_tool_call", gjson.Get(rec.Body.String(), "output.0.type").String())
	require.Equal(t,
		"const result = await tools.exec_command({cmd: \"pwd\"});\ntext(result.output);",
		gjson.Get(rec.Body.String(), "output.0.input").String(),
	)
}

func TestHandleResponsesStreamingResponse_RestoresNamespaceTool(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	resp := &http.Response{Body: io.NopCloser(strings.NewReader(namespaceToolAnthropicStream()))}

	svc := &GatewayService{}
	_, err := svc.handleResponsesStreamingResponse(resp, c, "claude-fable-5", "claude-fable-5", nil, time.Now(), namespaceToolMapping())
	require.NoError(t, err)
	require.Contains(t, rec.Body.String(), `response.output_item.added`)
	require.Contains(t, rec.Body.String(), `"name":"read_thread"`)
	require.Contains(t, rec.Body.String(), `"namespace":"codex_app"`)
	require.NotContains(t, rec.Body.String(), `"name":"codex_app__read_thread"`)
}

func TestHandleResponsesStreamingResponse_RestoresSparseCustomToolOnTruncatedStream(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	// The upstream closes before message_stop/content_block_stop. The gateway
	// must still restore the synthetic terminal events to the client-side
	// custom_tool_call shape instead of leaking the lowered function alias.
	stream := strings.Join([]string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"id":"msg_truncated","type":"message","role":"assistant","content":[],"model":"deepseek-v4-flash","stop_reason":"","usage":{"input_tokens":10}}}`,
		``,
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":2,"content_block":{"type":"tool_use","id":"toolu_sparse","name":"functions__exec","input":{"input":"pwd"}}}`,
		``,
	}, "\n")
	resp := &http.Response{Body: io.NopCloser(strings.NewReader(stream))}
	mapping := apicompat.ResponsesClientToolMapping{
		CustomTools: map[string]bool{"exec": true},
		NamespaceTools: map[string]apicompat.ResponsesNamespaceName{
			"functions__wait": {Namespace: "functions", Name: "wait"},
		},
	}

	svc := &GatewayService{}
	_, err := svc.handleResponsesStreamingResponse(resp, c, "deepseek-v4-flash", "deepseek-v4-flash", nil, time.Now(), mapping)
	require.NoError(t, err)
	body := rec.Body.String()
	require.Contains(t, body, `"type":"custom_tool_call"`)
	require.Contains(t, body, `"name":"exec"`)
	require.Contains(t, body, `"type":"response.completed"`)
	require.NotContains(t, body, `"name":"functions__exec"`)
	require.NotContains(t, body, `"type":"function_call"`)
}

func TestExtractResponsesReasoningEffortFromBody(t *testing.T) {
	t.Parallel()

	got := ExtractResponsesReasoningEffortFromBody([]byte(`{"model":"claude-sonnet-4.5","reasoning":{"effort":"HIGH"}}`))
	require.NotNil(t, got)
	require.Equal(t, "high", *got)

	maxGot := ExtractResponsesReasoningEffortFromBody([]byte(`{"model":"deepseek-v4-pro","reasoning":{"effort":"max"}}`))
	require.NotNil(t, maxGot)
	require.Equal(t, "max", *maxGot)

	mappedMax := ExtractResponsesReasoningEffortFromBody(
		[]byte(`{"model":"public-alias","reasoning":{"effort":"max"}}`),
		"provider/glm-5.2",
		"public-alias",
	)
	require.NotNil(t, mappedMax)
	require.Equal(t, "max", *mappedMax)

	legacyMax := ExtractResponsesReasoningEffortFromBody([]byte(`{"model":"gpt-5.5","reasoning":{"effort":"max"}}`))
	require.NotNil(t, legacyMax)
	require.Equal(t, "xhigh", *legacyMax)

	require.Nil(t, ExtractResponsesReasoningEffortFromBody([]byte(`{"model":"claude-sonnet-4.5"}`)))
}

func TestHandleResponsesBufferedStreamingResponse_PreservesMessageStartCacheUsage(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	resp := &http.Response{
		Header: http.Header{"x-request-id": []string{"rid_buffered"}},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`event: message_start`,
			`data: {"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4.5","stop_reason":"","usage":{"input_tokens":12,"cache_read_input_tokens":9,"cache_creation_input_tokens":3}}}`,
			``,
			`event: content_block_start`,
			`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":"hello"}}`,
			``,
			`event: message_delta`,
			`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":7}}`,
			``,
		}, "\n"))),
	}

	svc := &GatewayService{}
	result, err := svc.handleResponsesBufferedStreamingResponse(resp, c, "claude-sonnet-4.5", "claude-sonnet-4.5", nil, time.Now(), apicompat.ResponsesClientToolMapping{})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 12, result.Usage.InputTokens)
	require.Equal(t, 7, result.Usage.OutputTokens)
	require.Equal(t, 9, result.Usage.CacheReadInputTokens)
	require.Equal(t, 3, result.Usage.CacheCreationInputTokens)
	require.Contains(t, rec.Body.String(), `"cached_tokens":9`)
}

func TestHandleResponsesStreamingResponse_PreservesMessageStartCacheUsage(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	resp := &http.Response{
		Header: http.Header{"x-request-id": []string{"rid_stream"}},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`event: message_start`,
			`data: {"type":"message_start","message":{"id":"msg_2","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4.5","stop_reason":"","usage":{"input_tokens":20,"cache_read_input_tokens":11,"cache_creation_input_tokens":4}}}`,
			``,
			`event: content_block_start`,
			`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":"hello"}}`,
			``,
			`event: message_delta`,
			`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":8}}`,
			``,
			`event: message_stop`,
			`data: {"type":"message_stop"}`,
			``,
		}, "\n"))),
	}

	svc := &GatewayService{}
	result, err := svc.handleResponsesStreamingResponse(resp, c, "claude-sonnet-4.5", "claude-sonnet-4.5", nil, time.Now(), apicompat.ResponsesClientToolMapping{})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 20, result.Usage.InputTokens)
	require.Equal(t, 8, result.Usage.OutputTokens)
	require.Equal(t, 11, result.Usage.CacheReadInputTokens)
	require.Equal(t, 4, result.Usage.CacheCreationInputTokens)
	require.Contains(t, rec.Body.String(), `response.completed`)
}

func TestParseAnthropicSSEField(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		line      string
		field     string
		wantValue string
		wantOK    bool
	}{
		{
			name:      "standard format with space",
			line:      "event: message_start",
			field:     "event",
			wantValue: "message_start",
			wantOK:    true,
		},
		{
			name:      "compact format without space",
			line:      "event:message_start",
			field:     "event",
			wantValue: "message_start",
			wantOK:    true,
		},
		{
			name:      "data field with space",
			line:      "data: {\"type\":\"message_start\"}",
			field:     "data",
			wantValue: "{\"type\":\"message_start\"}",
			wantOK:    true,
		},
		{
			name:      "data field without space",
			line:      "data:{\"type\":\"message_start\"}",
			field:     "data",
			wantValue: "{\"type\":\"message_start\"}",
			wantOK:    true,
		},
		{
			name:      "field with multiple spaces after colon",
			line:      "event:  message_delta",
			field:     "event",
			wantValue: "message_delta",
			wantOK:    true,
		},
		{
			name:      "wrong field name",
			line:      "event: message_start",
			field:     "data",
			wantValue: "",
			wantOK:    false,
		},
		{
			name:      "empty line",
			line:      "",
			field:     "event",
			wantValue: "",
			wantOK:    false,
		},
		{
			name:      "line without colon",
			line:      "invalid line",
			field:     "event",
			wantValue: "",
			wantOK:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotValue, gotOK := parseAnthropicSSEField(tt.line, tt.field)
			require.Equal(t, tt.wantOK, gotOK, "parseAnthropicSSEField() ok")
			require.Equal(t, tt.wantValue, gotValue, "parseAnthropicSSEField() value")
		})
	}
}

func TestHandleResponsesBufferedStreamingResponse_CompactSSEFormat(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	// Simulate compact SSE format without spaces after colons (e.g. Kimi API)
	resp := &http.Response{
		Header: http.Header{"x-request-id": []string{"rid_compact"}},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`event:message_start`,
			`data:{"type":"message_start","message":{"id":"msg_compact","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4.5","stop_reason":"","usage":{"input_tokens":10}}}`,
			``,
			`event:content_block_start`,
			`data:{"type":"content_block_start","index":0,"content_block":{"type":"text","text":"OK"}}`,
			``,
			`event:message_delta`,
			`data:{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":5}}`,
			``,
		}, "\n"))),
	}

	svc := &GatewayService{}
	result, err := svc.handleResponsesBufferedStreamingResponse(resp, c, "claude-sonnet-4.5", "claude-sonnet-4.5", nil, time.Now(), apicompat.ResponsesClientToolMapping{})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 10, result.Usage.InputTokens)
	require.Equal(t, 5, result.Usage.OutputTokens)
}

func TestHandleResponsesStreamingResponse_CompactSSEFormat(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	// Simulate compact SSE format without spaces after colons (e.g. Kimi API)
	resp := &http.Response{
		Header: http.Header{"x-request-id": []string{"rid_compact_stream"}},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`event:message_start`,
			`data:{"type":"message_start","message":{"id":"msg_compact_stream","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4.5","stop_reason":"","usage":{"input_tokens":15}}}`,
			``,
			`event:content_block_start`,
			`data:{"type":"content_block_start","index":0,"content_block":{"type":"text","text":"OK"}}`,
			``,
			`event:message_delta`,
			`data:{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":6}}`,
			``,
			`event:message_stop`,
			`data:{"type":"message_stop"}`,
			``,
		}, "\n"))),
	}

	svc := &GatewayService{}
	result, err := svc.handleResponsesStreamingResponse(resp, c, "claude-sonnet-4.5", "claude-sonnet-4.5", nil, time.Now(), apicompat.ResponsesClientToolMapping{})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 15, result.Usage.InputTokens)
	require.Equal(t, 6, result.Usage.OutputTokens)
	require.Contains(t, rec.Body.String(), `response.completed`)
}
