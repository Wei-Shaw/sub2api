package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func anthropicPassthroughProtocolFixture(toolName string, stopReason string, includeStructuredTool bool) string {
	var builder strings.Builder
	builder.WriteString("event: message_start\n")
	builder.WriteString(`data: {"type":"message_start","message":{"id":"msg_fixture","type":"message","role":"assistant","content":[],"usage":{"input_tokens":5,"output_tokens":0}}}`)
	builder.WriteString("\n\n")
	builder.WriteString("event: content_block_start\n")
	builder.WriteString(`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
	builder.WriteString("\n\n")
	builder.WriteString("event: content_block_delta\n")
	builder.WriteString(`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Running.\n<inv"}}`)
	builder.WriteString("\n\n")
	builder.WriteString("event: content_block_delta\n")
	builder.WriteString(`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"oke name=\"Bash\"><parameter name=\"command\">printf test</parameter></invoke>"}}`)
	builder.WriteString("\n\n")
	builder.WriteString("event: content_block_stop\n")
	builder.WriteString(`data: {"type":"content_block_stop","index":0}`)
	builder.WriteString("\n\n")
	if includeStructuredTool {
		builder.WriteString("event: content_block_start\n")
		builder.WriteString(`data: {"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_1","name":"`)
		builder.WriteString(toolName)
		builder.WriteString(`","input":{}}}`)
		builder.WriteString("\n\n")
		builder.WriteString("event: content_block_delta\n")
		builder.WriteString(`data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{}"}}`)
		builder.WriteString("\n\n")
		builder.WriteString("event: content_block_stop\n")
		builder.WriteString(`data: {"type":"content_block_stop","index":1}`)
		builder.WriteString("\n\n")
	}
	builder.WriteString("event: message_delta\n")
	builder.WriteString(`data: {"type":"message_delta","delta":{"stop_reason":"`)
	builder.WriteString(stopReason)
	builder.WriteString(`","stop_sequence":null},"usage":{"output_tokens":3}}`)
	builder.WriteString("\n\n")
	builder.WriteString("event: message_stop\n")
	builder.WriteString(`data: {"type":"message_stop"}`)
	builder.WriteString("\n\n")
	return builder.String()
}

func runAnthropicPassthroughProtocolFixture(t *testing.T, fixture string) (*streamingResult, error, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(fixture)),
	}
	svc := &GatewayService{
		cfg: &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}},
	}
	result, err := svc.handleStreamingResponseAnthropicAPIKeyPassthrough(
		context.Background(),
		resp,
		c,
		&Account{ID: 2},
		time.Now(),
		"test-model",
	)
	return result, err, rec.Body.String()
}

func anthropicPassthroughTextDeltas(body string) string {
	var text strings.Builder
	for _, line := range strings.Split(body, "\n") {
		data, ok := extractAnthropicSSEDataLine(line)
		if !ok || !gjson.Valid(data) || gjson.Get(data, "type").String() != "content_block_delta" {
			continue
		}
		if gjson.Get(data, "delta.type").String() == "text_delta" {
			text.WriteString(gjson.Get(data, "delta.text").String())
		}
	}
	return text.String()
}

func TestAnthropicPassthroughProtocolGuard_StripsDuplicatedRawToolCall(t *testing.T) {
	result, err, body := runAnthropicPassthroughProtocolFixture(
		t,
		anthropicPassthroughProtocolFixture("Bash", "tool_use", true),
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "Running.\n", anthropicPassthroughTextDeltas(body))
	require.Contains(t, body, `"type":"tool_use"`)
	require.Contains(t, body, `"name":"Bash"`)
}

func TestAnthropicPassthroughProtocolGuard_RejectsRawOnlyEndTurn(t *testing.T) {
	result, err, body := runAnthropicPassthroughProtocolFixture(
		t,
		anthropicPassthroughProtocolFixture("", "end_turn", false),
	)

	require.NotNil(t, result)
	var protocolErr *anthropicToolCallProtocolError
	require.True(t, errors.As(err, &protocolErr))
	require.NotContains(t, body, "<invoke")
	require.Contains(t, body, "event: error")
	require.Contains(t, body, `"type":"protocol_error"`)
}

func TestAnthropicPassthroughProtocolGuard_RejectsMismatchedToolNames(t *testing.T) {
	result, err, body := runAnthropicPassthroughProtocolFixture(
		t,
		anthropicPassthroughProtocolFixture("Read", "tool_use", true),
	)

	require.NotNil(t, result)
	var protocolErr *anthropicToolCallProtocolError
	require.True(t, errors.As(err, &protocolErr))
	require.NotContains(t, body, "<invoke")
	require.Contains(t, body, "event: error")
}

func TestAnthropicPassthroughProtocolGuard_RejectsIncompleteRawToolCall(t *testing.T) {
	fixture := strings.Replace(
		anthropicPassthroughProtocolFixture("", "end_turn", false),
		"</invoke>",
		"",
		1,
	)
	result, err, body := runAnthropicPassthroughProtocolFixture(t, fixture)

	require.NotNil(t, result)
	var protocolErr *anthropicToolCallProtocolError
	require.True(t, errors.As(err, &protocolErr))
	require.NotContains(t, body, "<invoke")
	require.Contains(t, body, "event: error")
}
