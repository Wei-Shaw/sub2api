// 本文件验证 Anthropic Messages 最终 JSON 和 SSE 事件编码。
package messages

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/devin/llm"
)

// TestStreamEncoderEmitsMessageStartAndText 验证流式文本产生 Anthropic 标准事件。
func TestStreamEncoderEmitsMessageStartAndText(t *testing.T) {
	encoder := NewStreamEncoder("claude-test")
	text := llm.TextContent{Text: "hello"}
	partial := &llm.AssistantMessage{Content: []llm.Content{text}, StopReason: llm.StopReasonPending}
	final := &llm.AssistantMessage{Content: []llm.Content{text}, StopReason: llm.StopReasonStop}
	events := []llm.ResponseEvent{
		{Type: llm.ResponseEventStart, Partial: &llm.AssistantMessage{ResponseID: "msg-1", StopReason: llm.StopReasonPending}},
		{Type: llm.ResponseEventTextStart, ContentIndex: 0, Partial: partial},
		{Type: llm.ResponseEventTextDelta, ContentIndex: 0, Delta: "hello", Partial: partial},
		{Type: llm.ResponseEventTextEnd, ContentIndex: 0, Content: "hello", Partial: partial},
		{Type: llm.ResponseEventDone, Reason: llm.StopReasonStop, Message: final},
	}
	encoded := encodeStreamEvents(t, encoder, events)
	if len(encoded) != 6 { // message_start, content_block_start, content_block_delta, content_block_stop, message_delta, message_stop
		t.Fatalf("event count = %d, want 6", len(encoded))
	}
	if encoded[0].Name != "message_start" || encoded[5].Name != "message_stop" {
		t.Fatalf("events = %v", encoded)
	}
}

// TestStreamEncoderEmitsToolUse 验证流式工具调用按 Anthropic 增量格式输出。
func TestStreamEncoderEmitsToolUse(t *testing.T) {
	encoder := NewStreamEncoder("claude-test")
	call := llm.ToolCall{ID: "call-1", Name: "lookup", Arguments: json.RawMessage(`{"city":"Shanghai"}`)}
	partial := &llm.AssistantMessage{Content: []llm.Content{call}, StopReason: llm.StopReasonPending}
	final := &llm.AssistantMessage{Content: []llm.Content{call}, StopReason: llm.StopReasonToolUse}
	events := []llm.ResponseEvent{
		{Type: llm.ResponseEventStart, Partial: &llm.AssistantMessage{StopReason: llm.StopReasonPending}},
		{Type: llm.ResponseEventToolCallStart, ContentIndex: 0, ToolCallID: "call-1", ToolName: "lookup", Partial: partial},
		{Type: llm.ResponseEventToolCallDelta, ContentIndex: 0, ToolCallID: "call-1", Delta: `{"city":"`, Partial: partial},
		{Type: llm.ResponseEventToolCallDelta, ContentIndex: 0, ToolCallID: "call-1", Delta: `Shanghai"}`, Partial: partial},
		{Type: llm.ResponseEventToolCallEnd, ContentIndex: 0, ToolCall: &call, Partial: partial},
		{Type: llm.ResponseEventDone, Reason: llm.StopReasonToolUse, Message: final},
	}
	encoded := encodeStreamEvents(t, encoder, events)
	if len(encoded) != 7 {
		t.Fatalf("event count = %d, want 7", len(encoded))
	}
	if encoded[0].Name != "message_start" || encoded[6].Name != "message_stop" {
		t.Fatalf("events = %v", encoded)
	}
	startBlock := decodeEventData(t, encoded[1])
	content, ok := startBlock["content_block"].(map[string]any)
	if !ok || content["type"] != "tool_use" || content["name"] != "lookup" {
		t.Fatalf("content_block = %#v", content)
	}
	stopBlock := decodeEventData(t, encoded[4])
	if stopBlock["type"] != "content_block_stop" {
		t.Fatalf("stop block type = %s", stopBlock["type"])
	}
}

// TestStreamEncoderHoldsThinkingForLateSignature 的测试动机是上游实测帧序
// thinking_end → toolcall_* → thinking_signature：thinking 块必须挂起等待
// 隔块的尾随签名，签名到达时补发 signature_delta 再收尾。
func TestStreamEncoderHoldsThinkingForLateSignature(t *testing.T) {
	encoder := NewStreamEncoder("claude-test")
	call := llm.ToolCall{ID: "call-1", Name: "lookup", Arguments: json.RawMessage(`{"city":"Shanghai"}`)}
	partial := &llm.AssistantMessage{
		Content:    []llm.Content{llm.ThinkingContent{Thinking: "inspect"}, call},
		StopReason: llm.StopReasonPending,
	}
	final := &llm.AssistantMessage{
		Content:    []llm.Content{llm.ThinkingContent{Thinking: "inspect", ThinkingSignature: "sig"}, call},
		StopReason: llm.StopReasonToolUse,
	}
	encoded := encodeStreamEvents(t, encoder, []llm.ResponseEvent{
		{Type: llm.ResponseEventStart, Partial: &llm.AssistantMessage{StopReason: llm.StopReasonPending}},
		{Type: llm.ResponseEventThinkingStart, ContentIndex: 0, Partial: partial},
		{Type: llm.ResponseEventThinkingDelta, ContentIndex: 0, Delta: "inspect", Partial: partial},
		{Type: llm.ResponseEventThinkingEnd, ContentIndex: 0, Content: "inspect", Partial: partial},
		{Type: llm.ResponseEventToolCallStart, ContentIndex: 1, ToolCallID: "call-1", ToolName: "lookup", Partial: partial},
		{Type: llm.ResponseEventToolCallDelta, ContentIndex: 1, ToolCallID: "call-1", Delta: `{"city":"Shanghai"}`, Partial: partial},
	})
	// thinking 块挂起期间 tool_use 正常推进，此刻共 5 个事件且没有块收尾。
	if len(encoded) != 5 {
		t.Fatalf("events before signature = %d, want 5", len(encoded))
	}
	partial.Content[0] = llm.ThinkingContent{Thinking: "inspect", ThinkingSignature: "sig"}
	encoded = append(encoded, encodeStreamEvents(t, encoder, []llm.ResponseEvent{
		{Type: llm.ResponseEventThinkingSignature, ContentIndex: 0, Delta: "sig", Partial: partial},
		{Type: llm.ResponseEventToolCallEnd, ContentIndex: 1, ToolCall: &call, Partial: partial},
		{Type: llm.ResponseEventDone, Reason: llm.StopReasonToolUse, Message: final},
	})...)
	names := make([]string, 0, len(encoded))
	for _, event := range encoded {
		names = append(names, event.Name)
	}
	want := []string{
		"message_start", "content_block_start", "content_block_delta",
		"content_block_start", "content_block_delta",
		"content_block_delta", "content_block_stop",
		"content_block_stop", "message_delta", "message_stop",
	}
	if len(names) != len(want) {
		t.Fatalf("event names = %v, want %v", names, want)
	}
	for index, name := range want {
		if names[index] != name {
			t.Fatalf("event[%d] = %q, want %q (all: %v)", index, names[index], name, names)
		}
	}
	signatureDelta := decodeEventData(t, encoded[5])
	delta, ok := signatureDelta["delta"].(map[string]any)
	if !ok || delta["type"] != "signature_delta" || delta["signature"] != "sig" || signatureDelta["index"] != float64(0) {
		t.Fatalf("signature delta = %#v", signatureDelta)
	}
	stopBlock := decodeEventData(t, encoded[6])
	block, ok := stopBlock["content_block"].(map[string]any)
	if !ok || stopBlock["index"] != float64(0) || block["type"] != "thinking" || block["signature"] != "sig" {
		t.Fatalf("thinking stop block = %#v", stopBlock)
	}
}

// TestEncodeResponseFinal 验证非流式最终 JSON 结构和缓存用量字段。
func TestEncodeResponseFinal(t *testing.T) {
	final := &llm.AssistantMessage{
		ResponseID:    "msg-1",
		ResponseModel: "claude-test",
		Content:       []llm.Content{llm.TextContent{Text: "hello"}},
		StopReason:    llm.StopReasonStop,
		Usage:         llm.Usage{Input: 10, Output: 5, CacheRead: 3, CacheWrite: 2},
	}
	body, err := EncodeResponse(final)
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed["type"] != "message" || parsed["model"] != "claude-test" {
		t.Fatalf("response = %#v", parsed)
	}
	usage, ok := parsed["usage"].(map[string]any)
	if !ok || usage["cache_read_input_tokens"] != float64(3) || usage["cache_creation_input_tokens"] != float64(2) {
		t.Fatalf("usage = %#v", usage)
	}
}

func encodeStreamEvents(t *testing.T, encoder *StreamEncoder, events []llm.ResponseEvent) []SSEEvent {
	t.Helper()
	var encoded []SSEEvent
	for _, event := range events {
		batch, err := encoder.Encode(event)
		if err != nil {
			t.Fatal(err)
		}
		encoded = append(encoded, batch...)
	}
	return encoded
}

func decodeEventData(t *testing.T, event SSEEvent) map[string]any {
	t.Helper()
	var data map[string]any
	if err := json.Unmarshal(event.Data, &data); err != nil {
		t.Fatal(err)
	}
	return data
}

// TestStreamEncoderEmitsError 验证流式错误生成 event: error。
func TestStreamEncoderEmitsError(t *testing.T) {
	encoder := NewStreamEncoder("claude-test")
	failed := &llm.AssistantMessage{Provider: "devin", StopReason: llm.StopReasonError, ErrorMessage: "permission_denied: not allowed"}
	event := llm.ResponseEvent{Type: llm.ResponseEventError, Reason: llm.StopReasonError, Error: failed}
	encoded, err := encoder.Encode(event)
	if err != nil {
		t.Fatalf("encode error: %v", err)
	}
	if len(encoded) != 1 {
		t.Fatalf("event count = %d, want 1", len(encoded))
	}
	if encoded[0].Name != "error" {
		t.Fatalf("event name = %q, want error", encoded[0].Name)
	}
	data := decodeEventData(t, encoded[0])
	if data["type"] != "error" {
		t.Fatalf("type = %v", data["type"])
	}
	errObj, ok := data["error"].(map[string]any)
	if !ok {
		t.Fatalf("missing error object: %v", data)
	}
	if errObj["message"] != "permission_denied: not allowed" {
		t.Fatalf("error.message = %v", errObj["message"])
	}
	if errObj["type"] != "invalid_request_error" {
		t.Fatalf("error.type = %v, want invalid_request_error", errObj["type"])
	}
	// 错误后再次编码应因流已结束而失败。
	if _, err := encoder.Encode(event); err == nil {
		t.Fatal("encoding after error should fail")
	}
}
