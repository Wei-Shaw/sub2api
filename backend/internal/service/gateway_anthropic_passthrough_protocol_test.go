package service

import (
	"context"
	"fmt"
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
	structuredToolEvents := ""
	if includeStructuredTool {
		structuredToolEvents = fmt.Sprintf(
			"event: content_block_start\n"+
				`data: {"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_1","name":"%s","input":{}}}`+"\n\n"+
				"event: content_block_delta\n"+
				`data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{}"}}`+"\n\n"+
				"event: content_block_stop\n"+
				`data: {"type":"content_block_stop","index":1}`+"\n\n",
			toolName,
		)
	}
	return fmt.Sprintf(
		"event: message_start\n"+
			`data: {"type":"message_start","message":{"id":"msg_fixture","type":"message","role":"assistant","content":[],"usage":{"input_tokens":5,"output_tokens":0}}}`+"\n\n"+
			"event: content_block_start\n"+
			`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`+"\n\n"+
			"event: content_block_delta\n"+
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Running.\n<inv"}}`+"\n\n"+
			"event: content_block_delta\n"+
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"oke name=\"Bash\"><parameter name=\"command\">printf test</parameter></invoke>"}}`+"\n\n"+
			"event: content_block_stop\n"+
			`data: {"type":"content_block_stop","index":0}`+"\n\n"+
			"%s"+
			"event: message_delta\n"+
			`data: {"type":"message_delta","delta":{"stop_reason":"%s","stop_sequence":null},"usage":{"output_tokens":3}}`+"\n\n"+
			"event: message_stop\n"+
			`data: {"type":"message_stop"}`+"\n\n",
		structuredToolEvents,
		stopReason,
	)
}

func runAnthropicPassthroughProtocolFixture(t *testing.T, fixture string) (*streamingResult, string, error) {
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
	return result, rec.Body.String(), err
}

func anthropicPassthroughTextDeltas(body string) string {
	var text strings.Builder
	for _, line := range strings.Split(body, "\n") {
		data, ok := extractAnthropicSSEDataLine(line)
		if !ok || !gjson.Valid(data) || gjson.Get(data, "type").String() != "content_block_delta" {
			continue
		}
		if gjson.Get(data, "delta.type").String() == "text_delta" {
			_, _ = text.WriteString(gjson.Get(data, "delta.text").String())
		}
	}
	return text.String()
}

func TestAnthropicPassthroughProtocolGuard_StripsDuplicatedRawToolCall(t *testing.T) {
	result, body, err := runAnthropicPassthroughProtocolFixture(
		t,
		anthropicPassthroughProtocolFixture("Bash", "tool_use", true),
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "Running.\n", anthropicPassthroughTextDeltas(body))
	require.Contains(t, body, `"type":"tool_use"`)
	require.Contains(t, body, `"name":"Bash"`)
}

func TestAnthropicPassthroughProtocolGuard_PreservesRawOnlyEndTurn(t *testing.T) {
	result, body, err := runAnthropicPassthroughProtocolFixture(
		t,
		anthropicPassthroughProtocolFixture("", "end_turn", false),
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "Running.\n<invoke name=\"Bash\"><parameter name=\"command\">printf test</parameter></invoke>", anthropicPassthroughTextDeltas(body))
	require.NotContains(t, body, "event: error")
}

func TestAnthropicPassthroughProtocolGuard_PreservesMismatchedToolNames(t *testing.T) {
	result, body, err := runAnthropicPassthroughProtocolFixture(
		t,
		anthropicPassthroughProtocolFixture("Read", "tool_use", true),
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "Running.\n<invoke name=\"Bash\"><parameter name=\"command\">printf test</parameter></invoke>", anthropicPassthroughTextDeltas(body))
	require.Contains(t, body, `"name":"Read"`)
	require.NotContains(t, body, "event: error")
}

func TestAnthropicPassthroughProtocolGuard_PreservesIncompleteRawToolCall(t *testing.T) {
	fixture := strings.Replace(
		anthropicPassthroughProtocolFixture("", "end_turn", false),
		"</invoke>",
		"",
		1,
	)
	result, body, err := runAnthropicPassthroughProtocolFixture(t, fixture)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "Running.\n<invoke name=\"Bash\"><parameter name=\"command\">printf test</parameter>", anthropicPassthroughTextDeltas(body))
	require.NotContains(t, body, "event: error")
}

func TestAnthropicPassthroughProtocolGuard_PreservesOverlongIncompleteRawToolCall(t *testing.T) {
	overlongCandidate := `<invoke name="Bash">` + strings.Repeat("x", maxAnthropicToolCallCandidateBytes+1)
	fixture := strings.Replace(
		anthropicPassthroughProtocolFixture("", "end_turn", false),
		`oke name=\"Bash\"><parameter name=\"command\">printf test</parameter></invoke>`,
		`oke name=\"Bash\">`+strings.Repeat("x", maxAnthropicToolCallCandidateBytes+1),
		1,
	)
	result, body, err := runAnthropicPassthroughProtocolFixture(t, fixture)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "Running.\n"+overlongCandidate, anthropicPassthroughTextDeltas(body))
	require.NotContains(t, body, "event: error")
}

func TestAnthropicPassthroughProtocolGuard_FallsBackWhenDeferredEventsExceedLimit(t *testing.T) {
	fixture := strings.Replace(
		anthropicPassthroughProtocolFixture("Bash", "tool_use", true),
		`"partial_json":"{}"`,
		`"partial_json":"`+strings.Repeat("x", maxAnthropicToolCallDeferredEventBytes+1)+`"`,
		1,
	)
	result, body, err := runAnthropicPassthroughProtocolFixture(t, fixture)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "Running.\n<invoke name=\"Bash\"><parameter name=\"command\">printf test</parameter></invoke>", anthropicPassthroughTextDeltas(body))
	require.Contains(t, body, strings.Repeat("x", 1024))
	require.NotContains(t, body, "event: error")
}

func TestAnthropicPassthroughProtocolGuard_PreservesCandidateWithTrailingText(t *testing.T) {
	fixture := strings.Replace(
		anthropicPassthroughProtocolFixture("Bash", "tool_use", true),
		`</invoke>"}}`,
		`</invoke>\nLiteral XML example."}}`,
		1,
	)
	result, body, err := runAnthropicPassthroughProtocolFixture(t, fixture)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "Running.\n<invoke name=\"Bash\"><parameter name=\"command\">printf test</parameter></invoke>\nLiteral XML example.", anthropicPassthroughTextDeltas(body))
	require.Contains(t, body, `"name":"Bash"`)
	require.NotContains(t, body, "event: error")
}

func TestNormalizeAnthropicPassthroughResponseBody_PreservesUnverifiedLiteralMarkup(t *testing.T) {
	tests := []string{
		`{"content":[{"type":"text","text":"<invoke name=\"Bash\">example</invoke>\nLiteral XML example."},{"type":"tool_use","name":"Bash"}],"stop_reason":"tool_use"}`,
		`{"content":[{"type":"text","text":"<invoke name=\"Bash\">example</invoke><note>Literal XML example.</note>"},{"type":"tool_use","name":"Bash"}],"stop_reason":"tool_use"}`,
	}

	for _, body := range tests {
		normalized, err := normalizeAnthropicPassthroughResponseBody([]byte(body))

		require.NoError(t, err)
		require.JSONEq(t, body, string(normalized))
	}
}

func TestNormalizeAnthropicPassthroughResponseBody_UsesConsistentXMLGrammar(t *testing.T) {
	body := `{"content":[{"type":"text","text":"<invoke  name=\"Bash\"><parameter name=\"command\">printf test</parameter></invoke>"},{"type":"tool_use","name":"Bash"}],"stop_reason":"tool_use"}`

	normalized, err := normalizeAnthropicPassthroughResponseBody([]byte(body))

	require.NoError(t, err)
	require.Len(t, gjson.GetBytes(normalized, "content").Array(), 1)
	require.Equal(t, "tool_use", gjson.GetBytes(normalized, "content.0.type").String())
}
