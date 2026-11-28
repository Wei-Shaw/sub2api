//go:build unit

package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
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

// Anthropic upstream turn: signed thinking (signature split across deltas),
// text, redacted_thinking and tool_use, interleaved so ordering is observable.
func thinkingToolTurnSSE() string {
	return strings.Join([]string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"id":"msg_rd","type":"message","role":"assistant","content":[],"model":"claude-opus-5-5","usage":{"input_tokens":10}}}`,
		``,
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":"","signature":""}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"plan the lookup"}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"sig-part-A/"}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"sig-part-B+=="}}`,
		``,
		`event: content_block_stop`,
		`data: {"type":"content_block_stop","index":0}`,
		``,
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"checking"}}`,
		``,
		`event: content_block_stop`,
		`data: {"type":"content_block_stop","index":1}`,
		``,
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":2,"content_block":{"type":"redacted_thinking","data":"redacted-blob=="}}`,
		``,
		`event: content_block_stop`,
		`data: {"type":"content_block_stop","index":2}`,
		``,
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":3,"content_block":{"type":"tool_use","id":"toolu_1","name":"lookup","input":{}}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":3,"delta":{"type":"input_json_delta","partial_json":"{\"q\":\"x\"}"}}`,
		``,
		`event: content_block_stop`,
		`data: {"type":"content_block_stop","index":3}`,
		``,
		`event: message_delta`,
		`data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":5}}`,
		``,
		`event: message_stop`,
		`data: {"type":"message_stop"}`,
		``,
	}, "\n")
}

var (
	upstreamThinkingBlock = apicompat.AnthropicContentBlock{Type: "thinking", Thinking: "plan the lookup", Signature: "sig-part-A/sig-part-B+=="}
	upstreamRedactedBlock = apicompat.AnthropicContentBlock{Type: "redacted_thinking", Data: "redacted-blob=="}
)

// thinkingEnvelope builds the opaque anthropic-thinking-v1 envelope the Responses layer carries.
func thinkingEnvelope(blockType, thinking, signature, data string) string {
	payload, _ := json.Marshal(struct {
		Type      string `json:"type"`
		Thinking  string `json:"thinking"`
		Signature string `json:"signature,omitempty"`
		Data      string `json:"data,omitempty"`
	}{blockType, thinking, signature, data})
	return "anthropic-thinking-v1:" + base64.RawStdEncoding.EncodeToString(payload)
}

var (
	thinkingDetailJSON = fmt.Sprintf(`{"type":"reasoning.encrypted","data":%q,"format":"anthropic-claude-v1","index":0}`,
		thinkingEnvelope("thinking", "plan the lookup", "sig-part-A/sig-part-B+==", ""))
	redactedDetailJSON = fmt.Sprintf(`{"type":"reasoning.encrypted","data":%q,"format":"anthropic-claude-v1","index":1}`,
		thinkingEnvelope("redacted_thinking", "", "", "redacted-blob=="))
)

func runThinkingToolTurnBuffered(t *testing.T) []byte {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	resp := &http.Response{Header: http.Header{}, Body: io.NopCloser(strings.NewReader(thinkingToolTurnSSE()))}
	_, err := (&GatewayService{}).handleCCBufferedFromAnthropic(resp, c, "claude-opus-5-5", "claude-opus-5-5", nil, time.Now())
	require.NoError(t, err)
	return rec.Body.Bytes()
}

func TestHandleCCBufferedFromAnthropic_ReasoningDetails(t *testing.T) {
	body := runThinkingToolTurnBuffered(t)

	msg := gjson.GetBytes(body, "choices.0.message")
	require.True(t, msg.Get("reasoning_details").IsArray(), "message.reasoning_details missing: %s", msg.Raw)
	require.JSONEq(t, "["+thinkingDetailJSON+","+redactedDetailJSON+"]", msg.Get("reasoning_details").Raw)
	require.Equal(t, "plan the lookup", msg.Get("reasoning_content").String(), "reasoning_content unchanged")
	require.Equal(t, "checking", msg.Get("content").String())
	require.Equal(t, "toolu_1", msg.Get("tool_calls.0.id").String())
}

func TestHandleCCStreamingFromAnthropic_ReasoningDetails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	resp := &http.Response{Header: http.Header{}, Body: io.NopCloser(strings.NewReader(thinkingToolTurnSSE()))}
	_, err := (&GatewayService{}).handleCCStreamingFromAnthropic(resp, c, "claude-opus-5-5", "claude-opus-5-5", nil, time.Now())
	require.NoError(t, err)

	body := rec.Body.String()
	require.NotContains(t, body, `"signature":null`)

	// Sequence of observable delta kinds, in wire order.
	var seq []string
	var details []string
	for _, line := range strings.Split(body, "\n") {
		payload, ok := strings.CutPrefix(line, "data: ")
		if !ok || payload == "[DONE]" {
			continue
		}
		delta := gjson.Get(payload, "choices.0.delta")
		if rd := delta.Get("reasoning_details"); rd.Exists() {
			require.Len(t, rd.Array(), 1, "one chunk per block")
			details = append(details, rd.Array()[0].Raw)
			seq = append(seq, "detail-"+rd.Array()[0].Get("index").String())
		}
		if delta.Get("content").Exists() {
			seq = append(seq, "content")
		}
		if delta.Get("tool_calls").Exists() {
			seq = append(seq, "tool_calls")
		}
	}
	require.Len(t, details, 2)
	require.JSONEq(t, thinkingDetailJSON, details[0])
	require.JSONEq(t, redactedDetailJSON, details[1])

	first := func(kind string) int {
		for i, k := range seq {
			if k == kind {
				return i
			}
		}
		return -1
	}
	require.Less(t, first("detail-0"), first("content"), "seq=%v", seq)
	require.Less(t, first("content"), first("detail-1"), "seq=%v", seq)
	require.Less(t, first("detail-1"), first("tool_calls"), "seq=%v", seq)
}

// Conversation history shared with an OpenAI-family model must not carry the
// Anthropic envelope as encrypted_content to that upstream.
func TestChatReasoningDetails_NonAnthropicPathKeepsLegacyConversion(t *testing.T) {
	body := []byte(`{"model":"gpt-5.1","messages":[
		{"role":"user","content":"look it up"},
		{"role":"assistant","content":"checking","reasoning_content":"summary",
		 "reasoning_details":[` + thinkingDetailJSON + `,` + redactedDetailJSON + `]}]}`)

	converted, err := convertLegacyResponsesMessages(nil, body)
	require.NoError(t, err)
	input, err := json.Marshal(converted.input)
	require.NoError(t, err)

	require.NotContains(t, string(input), "anthropic-thinking-v1:", "input: %s", input)
	require.Len(t, gjson.ParseBytes(input).Array(), 2, "input: %s", input)
	require.Equal(t, "<thinking>summary</thinking>\nchecking", gjson.GetBytes(input, "1.content.0.text").String())
}

func TestChatReasoningDetails_RoundTripRestoresUpstreamThinkingBlocks(t *testing.T) {
	assistant := gjson.GetBytes(runThinkingToolTurnBuffered(t), "choices.0.message").Raw
	callID := gjson.Get(assistant, "tool_calls.0.id").String()

	reqJSON := `{"model":"claude-opus-5-5","messages":[` +
		`{"role":"user","content":"look it up"},` + assistant + `,` +
		`{"role":"tool","tool_call_id":"` + callID + `","content":"found"}]}`
	var ccReq apicompat.ChatCompletionsRequest
	require.NoError(t, json.Unmarshal([]byte(reqJSON), &ccReq))
	responsesReq, err := apicompat.ChatCompletionsToResponsesWithOptions(&ccReq, apicompat.ChatCompletionsToResponsesOptions{ReplayAnthropicThinking: true})
	require.NoError(t, err)
	anthropicReq, err := apicompat.ResponsesToAnthropicRequest(responsesReq)
	require.NoError(t, err)

	require.Len(t, anthropicReq.Messages, 3)
	require.Equal(t, "assistant", anthropicReq.Messages[1].Role)
	var blocks []json.RawMessage
	require.NoError(t, json.Unmarshal(anthropicReq.Messages[1].Content, &blocks))
	require.GreaterOrEqual(t, len(blocks), 4, "assistant content: %s", anthropicReq.Messages[1].Content)

	wantThinking, _ := json.Marshal(upstreamThinkingBlock)
	wantRedacted, _ := json.Marshal(upstreamRedactedBlock)
	require.Equal(t, string(wantThinking), string(blocks[0]))
	require.Equal(t, string(wantRedacted), string(blocks[1]))
	require.Equal(t, "text", gjson.GetBytes(blocks[2], "type").String())
	require.Equal(t, "tool_use", gjson.GetBytes(blocks[3], "type").String())
	require.NotContains(t, string(anthropicReq.Messages[1].Content), "<thinking>")
}

func forwardChatToolRoundWithReasoningDetails(t *testing.T, model string) []byte {
	t.Helper()
	gin.SetMode(gin.TestMode)
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(thinkingToolTurnSSE())),
	}}
	body := []byte(`{"model":"` + model + `","messages":[
		{"role":"user","content":"look it up"},
		{"role":"assistant","content":"checking","reasoning_content":"summary",
		 "tool_calls":[{"id":"toolu_1","type":"function","function":{"name":"lookup","arguments":"{}"}}],
		 "reasoning_details":[` + thinkingDetailJSON + `,` + redactedDetailJSON + `]},
		{"role":"tool","tool_call_id":"toolu_1","content":"found"}]}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(string(body)))
	svc := &GatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
	account := &Account{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "sk-test"}}
	_, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, nil)
	require.NoError(t, err)
	return upstream.lastBody
}

func TestForwardAsChatCompletions_ReplaysReasoningDetailsForSignedThinkingModel(t *testing.T) {
	sent := forwardChatToolRoundWithReasoningDetails(t, "claude-opus-5-5")

	content := gjson.GetBytes(sent, "messages.1.content")
	require.Equal(t, "thinking", content.Get("0.type").String(), "content: %s", content.Raw)
	require.Equal(t, "sig-part-A/sig-part-B+==", content.Get("0.signature").String())
	require.Equal(t, "redacted_thinking", content.Get("1.type").String())
	require.NotContains(t, content.Raw, "<thinking>")
}

// A model that ResponsesToAnthropicRequest does not keep signed thinking for
// must still get the <thinking> text copy of reasoning_content.
func TestForwardAsChatCompletions_KeepsThinkingTextOutsideSignedThinkingModels(t *testing.T) {
	sent := forwardChatToolRoundWithReasoningDetails(t, "claude-sonnet-4-5")

	content := gjson.GetBytes(sent, "messages.1.content")
	require.Equal(t, "text", content.Get("0.type").String(), "content: %s", content.Raw)
	require.Equal(t, "<thinking>summary</thinking>\nchecking", content.Get("0.text").String())
	require.NotContains(t, content.Raw, "signature")
	require.NotContains(t, content.Raw, "redacted_thinking")
}

func echoedModelThinkingToolTurnSSE(t *testing.T) string {
	t.Helper()
	sse := strings.Replace(thinkingToolTurnSSE(), `"model":"claude-opus-5-5"`, `"model":"echoed-model"`, 1)
	require.NotEqual(t, thinkingToolTurnSSE(), sse)
	return sse
}

// The signature gate follows the mapped upstream model, not the model name the
// upstream echoes in message_start.
func TestHandleCCStreamingFromAnthropic_ReasoningDetailsFollowMappedModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	resp := &http.Response{Header: http.Header{}, Body: io.NopCloser(strings.NewReader(echoedModelThinkingToolTurnSSE(t)))}
	_, err := (&GatewayService{}).handleCCStreamingFromAnthropic(resp, c, "claude-opus-5-5", "claude-opus-5-5", nil, time.Now())
	require.NoError(t, err)

	require.Contains(t, rec.Body.String(), `"format":"anthropic-claude-v1"`)
}

func TestHandleCCBufferedFromAnthropic_ReasoningDetailsFollowMappedModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	resp := &http.Response{Header: http.Header{}, Body: io.NopCloser(strings.NewReader(echoedModelThinkingToolTurnSSE(t)))}
	_, err := (&GatewayService{}).handleCCBufferedFromAnthropic(resp, c, "claude-opus-5-5", "claude-opus-5-5", nil, time.Now())
	require.NoError(t, err)

	details := gjson.GetBytes(rec.Body.Bytes(), "choices.0.message.reasoning_details")
	require.JSONEq(t, "["+thinkingDetailJSON+","+redactedDetailJSON+"]", details.Raw)
}

// collectChatStreamReasoningDetails returns the reasoning_details entries of a Chat SSE body, in order.
func collectChatStreamReasoningDetails(body string) []string {
	var entries []string
	for _, line := range strings.Split(body, "\n") {
		payload, ok := strings.CutPrefix(line, "data: ")
		if !ok || payload == "[DONE]" {
			continue
		}
		for _, d := range gjson.Get(payload, "choices.0.delta.reasoning_details").Array() {
			entries = append(entries, d.Raw)
		}
	}
	return entries
}

func replayedAssistantBlocks(t *testing.T, assistantJSON string) []gjson.Result {
	t.Helper()
	reqJSON := `{"model":"claude-opus-5-5","messages":[` +
		`{"role":"user","content":"look it up"},` + assistantJSON + `,` +
		`{"role":"tool","tool_call_id":"toolu_1","content":"found"}]}`
	var ccReq apicompat.ChatCompletionsRequest
	require.NoError(t, json.Unmarshal([]byte(reqJSON), &ccReq))
	responsesReq, err := apicompat.ChatCompletionsToResponsesWithOptions(&ccReq, apicompat.ChatCompletionsToResponsesOptions{ReplayAnthropicThinking: true})
	require.NoError(t, err)
	anthropicReq, err := apicompat.ResponsesToAnthropicRequest(responsesReq)
	require.NoError(t, err)
	return gjson.ParseBytes(anthropicReq.Messages[1].Content).Array()
}

// Response bytes go through tool name reversal on the way out. The signed
// thinking text must reach the upstream again exactly as the upstream signed it.
func TestChatReasoningDetails_ReplayedThinkingSurvivesToolNameReversal(t *testing.T) {
	signedText := "call cc_ses_lookup next"
	sse := strings.ReplaceAll(thinkingToolTurnSSE(), "plan the lookup", signedText)
	gin.SetMode(gin.TestMode)

	t.Run("buffered", func(t *testing.T) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		resp := &http.Response{Header: http.Header{}, Body: io.NopCloser(strings.NewReader(sse))}
		_, err := (&GatewayService{}).handleCCBufferedFromAnthropic(resp, c, "claude-opus-5-5", "claude-opus-5-5", nil, time.Now())
		require.NoError(t, err)

		blocks := replayedAssistantBlocks(t, gjson.GetBytes(rec.Body.Bytes(), "choices.0.message").Raw)
		require.Equal(t, signedText, blocks[0].Get("thinking").String(), "blocks: %v", blocks)
		require.Equal(t, "sig-part-A/sig-part-B+==", blocks[0].Get("signature").String())
	})

	t.Run("streaming", func(t *testing.T) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		resp := &http.Response{Header: http.Header{}, Body: io.NopCloser(strings.NewReader(sse))}
		_, err := (&GatewayService{}).handleCCStreamingFromAnthropic(resp, c, "claude-opus-5-5", "claude-opus-5-5", nil, time.Now())
		require.NoError(t, err)

		details := collectChatStreamReasoningDetails(rec.Body.String())
		require.Len(t, details, 2)
		assistant := `{"role":"assistant","content":"checking","reasoning_details":[` + strings.Join(details, ",") + `],` +
			`"tool_calls":[{"id":"toolu_1","type":"function","function":{"name":"lookup","arguments":"{}"}}]}`
		blocks := replayedAssistantBlocks(t, assistant)
		require.Equal(t, signedText, blocks[0].Get("thinking").String(), "blocks: %v", blocks)
		require.Equal(t, "sig-part-A/sig-part-B+==", blocks[0].Get("signature").String())
	})
}

// Responses are converted without reasoning_details unless the request side
// replays them for the same model, so any other model's output stays as before.
func TestChatReasoningDetails_NotEmittedOutsideReplayGate(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("buffered", func(t *testing.T) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		resp := &http.Response{Header: http.Header{}, Body: io.NopCloser(strings.NewReader(thinkingToolTurnSSE()))}
		_, err := (&GatewayService{}).handleCCBufferedFromAnthropic(resp, c, "claude-sonnet-4-5", "claude-sonnet-4-5", nil, time.Now())
		require.NoError(t, err)

		require.False(t, gjson.GetBytes(rec.Body.Bytes(), "choices.0.message.reasoning_details").Exists(), "body: %s", rec.Body.String())
	})

	t.Run("streaming", func(t *testing.T) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		resp := &http.Response{Header: http.Header{}, Body: io.NopCloser(strings.NewReader(thinkingToolTurnSSE()))}
		_, err := (&GatewayService{}).handleCCStreamingFromAnthropic(resp, c, "claude-sonnet-4-5", "claude-sonnet-4-5", nil, time.Now())
		require.NoError(t, err)

		require.NotContains(t, rec.Body.String(), "reasoning_details")
	})
}

// Event and field layout of a real Anthropic stream with a signed thinking block and a tool call,
// with placeholder values. The thinking text carries a tool name as the model sees it after tool name rewriting.
func renamedToolTurnSSE(thinking, signature string) string {
	data := func(event, payload string) string { return "event: " + event + "\ndata: " + payload + "\n\n" }
	return data("message_start", `{"type":"message_start","message":{"id":"msg_stub","type":"message","role":"assistant","model":"claude-haiku-5-5","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":10,"output_tokens":1}}}`) +
		data("content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":"","signature":""}}`) +
		data("ping", `{"type":"ping"}`) +
		data("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":`+fmt.Sprintf("%q", thinking)+`}}`) +
		data("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":`+fmt.Sprintf("%q", signature)+`}}`) +
		data("content_block_stop", `{"type":"content_block_stop","index":0}`) +
		data("content_block_start", `{"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}`) +
		data("content_block_delta", `{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"checking"}}`) +
		data("content_block_stop", `{"type":"content_block_stop","index":1}`) +
		data("content_block_start", `{"type":"content_block_start","index":2,"content_block":{"type":"tool_use","id":"toolu_stub_1","name":"cc_ses_lookup","input":{}}}`) +
		data("content_block_delta", `{"type":"content_block_delta","index":2,"delta":{"type":"input_json_delta","partial_json":"{\"query\":\"x\"}"}}`) +
		data("content_block_stop", `{"type":"content_block_stop","index":2}`) +
		data("message_delta", `{"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":null},"usage":{"input_tokens":10,"output_tokens":5}}`) +
		data("message_stop", `{"type":"message_stop"}`)
}

func assembleChatStreamMessage(t *testing.T, body string) string {
	t.Helper()
	var details []json.RawMessage
	var content, reasoning, toolID, toolName, toolArgs strings.Builder
	for _, line := range strings.Split(body, "\n") {
		payload, ok := strings.CutPrefix(line, "data: ")
		if !ok || payload == "[DONE]" {
			continue
		}
		delta := gjson.Get(payload, "choices.0.delta")
		content.WriteString(delta.Get("content").String())
		reasoning.WriteString(delta.Get("reasoning_content").String())
		for _, d := range delta.Get("reasoning_details").Array() {
			details = append(details, json.RawMessage(d.Raw))
		}
		for _, call := range delta.Get("tool_calls").Array() {
			toolID.WriteString(call.Get("id").String())
			toolName.WriteString(call.Get("function.name").String())
			toolArgs.WriteString(call.Get("function.arguments").String())
		}
	}
	msg := map[string]any{
		"role": "assistant", "content": content.String(), "reasoning_content": reasoning.String(), "reasoning_details": details,
		"tool_calls": []any{map[string]any{"id": toolID.String(), "type": "function", "function": map[string]any{"name": toolName.String(), "arguments": toolArgs.String()}}},
	}
	raw, err := json.Marshal(msg)
	require.NoError(t, err)
	return string(raw)
}

// Two full rounds through ForwardAsChatCompletions on an OAuth account whose tool names are
// rewritten. The thinking text of round 1 contains the rewritten tool name, which the response
// side maps back for the client; round 2 must still send the text the upstream signed.
func TestForwardAsChatCompletions_TwoRoundsReplaySignedThinkingWithToolNameRewrite(t *testing.T) {
	const signedThinking = "I will call cc_ses_lookup next"
	const signature = "sig-stub-A/B+=="
	tools := `[{"type":"function","function":{"name":"session_lookup","description":"Look up a fact.","parameters":{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}}}]`
	sseResponse := func(body string) *http.Response {
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(body))}
	}
	finalTurn := "event: message_start\ndata: " + `{"type":"message_start","message":{"id":"msg_stub2","type":"message","role":"assistant","model":"claude-haiku-5-5","content":[],"usage":{"input_tokens":10,"output_tokens":1}}}` + "\n\n" +
		"event: content_block_start\ndata: " + `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}` + "\n\n" +
		"event: content_block_delta\ndata: " + `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"done"}}` + "\n\n" +
		"event: content_block_stop\ndata: " + `{"type":"content_block_stop","index":0}` + "\n\n" +
		"event: message_delta\ndata: " + `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":2}}` + "\n\n" +
		"event: message_stop\ndata: " + `{"type":"message_stop"}` + "\n\n"

	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream=%v", stream), func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			upstream := &httpUpstreamRecorder{responses: []*http.Response{sseResponse(renamedToolTurnSSE(signedThinking, signature)), sseResponse(finalTurn)}}
			svc := &GatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
			account := &Account{ID: 2, Platform: PlatformAnthropic, Type: AccountTypeOAuth, Concurrency: 1, Credentials: map[string]any{"access_token": "sk-test"}}
			forward := func(body string) *httptest.ResponseRecorder {
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
				_, err := svc.ForwardAsChatCompletions(context.Background(), c, account, []byte(body), nil)
				require.NoError(t, err)
				return rec
			}

			round1 := forward(fmt.Sprintf(`{"model":"claude-haiku-5-5","stream":%v,"tools":%s,"messages":[{"role":"user","content":"look it up"}]}`, stream, tools))
			var assistant string
			if stream {
				assistant = assembleChatStreamMessage(t, round1.Body.String())
			} else {
				assistant = gjson.GetBytes(round1.Body.Bytes(), "choices.0.message").Raw
			}
			require.Equal(t, "session_lookup", gjson.Get(assistant, "tool_calls.0.function.name").String(), "tool name is mapped back for the client")
			require.NotEmpty(t, gjson.Get(assistant, "reasoning_details").Array(), "assistant: %s", assistant)

			forward(fmt.Sprintf(`{"model":"claude-haiku-5-5","stream":%v,"tools":%s,"messages":[{"role":"user","content":"look it up"},%s,{"role":"tool","tool_call_id":"toolu_stub_1","content":"found"}]}`, stream, tools, assistant))

			require.Len(t, upstream.bodies, 2)
			sent := upstream.bodies[1]
			require.Equal(t, "cc_ses_lookup", gjson.GetBytes(sent, "tools.0.name").String(), "tool name rewriting is active on the request: %s", sent)
			thinking := gjson.GetBytes(sent, "messages.1.content.0")
			require.Equal(t, "thinking", thinking.Get("type").String(), "content: %s", gjson.GetBytes(sent, "messages.1.content").Raw)
			require.Equal(t, signedThinking, thinking.Get("thinking").String())
			require.Equal(t, signature, thinking.Get("signature").String())
		})
	}
}
