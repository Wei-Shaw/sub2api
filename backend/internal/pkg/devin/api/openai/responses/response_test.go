// 本文件验证中间响应事件会展开为可供 agent loop 重放的完整 Responses SSE 生命周期。
package responses

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/devin/llm"
)

// TestStreamEncoderEncodesReasoningAndToolItems 的测试动机是保证思考和工具调用作为独立 output item 完整结束并进入最终 output。
func TestStreamEncoderEncodesReasoningAndToolItems(t *testing.T) {
	encoder := NewStreamEncoder("gpt-test")
	thinking := llm.ThinkingContent{Thinking: "inspect", ThinkingSignature: "encrypted"}
	call := llm.ToolCall{ID: "call-1", Name: "lookup", Arguments: json.RawMessage(`{"city":"Shanghai"}`)}
	partial := &llm.AssistantMessage{Content: []llm.Content{thinking, call}, StopReason: llm.StopReasonPending}
	final := &llm.AssistantMessage{
		Content: []llm.Content{thinking, call}, StopReason: llm.StopReasonToolUse,
		Usage: llm.Usage{Input: 10, Output: 5, CacheRead: 2, TotalTokens: 17},
	}
	events := []llm.ResponseEvent{
		{Type: llm.ResponseEventStart, Partial: &llm.AssistantMessage{StopReason: llm.StopReasonPending}},
		{Type: llm.ResponseEventThinkingStart, ContentIndex: 0, Partial: partial},
		{Type: llm.ResponseEventThinkingDelta, ContentIndex: 0, Delta: "inspect", Partial: partial},
		{Type: llm.ResponseEventThinkingEnd, ContentIndex: 0, Content: "inspect", Partial: partial},
		{Type: llm.ResponseEventToolCallStart, ContentIndex: 1, ToolCallID: "call-1", ToolName: "lookup", Partial: partial},
		{Type: llm.ResponseEventToolCallDelta, ContentIndex: 1, ToolCallID: "call-1", Delta: `{"city":"`, Partial: partial},
		{Type: llm.ResponseEventToolCallDelta, ContentIndex: 1, ToolCallID: "call-1", Delta: `Shanghai"}`, Partial: partial},
		{Type: llm.ResponseEventToolCallEnd, ContentIndex: 1, ToolCall: &call, Partial: partial},
		{Type: llm.ResponseEventDone, Reason: llm.StopReasonToolUse, Message: final},
	}
	encoded := encodeStreamEvents(t, encoder, events)
	wantNames := []string{
		"response.created", "response.in_progress",
		"response.output_item.added", "response.reasoning_summary_part.added",
		"response.reasoning_summary_text.delta", "response.reasoning_summary_text.done",
		"response.reasoning_summary_part.done", "response.output_item.done",
		"response.output_item.added", "response.function_call_arguments.delta",
		"response.function_call_arguments.delta", "response.function_call_arguments.done",
		"response.output_item.done", "response.completed",
	}
	assertEventNames(t, encoded, wantNames)
	assertSequenceNumbers(t, encoded)

	created := decodeEventData(t, encoded[0])
	responseID := nestedString(t, created, "response", "id")
	if !strings.HasPrefix(responseID, "resp_") {
		t.Fatalf("response id = %q, want resp_ prefix", responseID)
	}
	reasoningAdded := decodeEventData(t, encoded[2])
	reasoningID := nestedString(t, reasoningAdded, "item", "id")
	if !strings.HasPrefix(reasoningID, "rs_") {
		t.Fatalf("reasoning id = %q, want rs_ prefix", reasoningID)
	}
	if itemID := decodeEventData(t, encoded[4])["item_id"]; itemID != reasoningID {
		t.Fatalf("reasoning delta item_id = %v, want %q", itemID, reasoningID)
	}
	toolAdded := decodeEventData(t, encoded[8])
	toolItemID := nestedString(t, toolAdded, "item", "id")
	if !strings.HasPrefix(toolItemID, "fc_") || toolItemID == "call-1" {
		t.Fatalf("tool item id = %q, want distinct fc_ id", toolItemID)
	}
	if callID := nestedString(t, toolAdded, "item", "call_id"); callID != "call-1" {
		t.Fatalf("call_id = %q, want call-1", callID)
	}
	if itemID := decodeEventData(t, encoded[9])["item_id"]; itemID != toolItemID {
		t.Fatalf("tool delta item_id = %v, want %q", itemID, toolItemID)
	}

	completed := decodeEventData(t, encoded[len(encoded)-1])
	if completedResponseID := nestedString(t, completed, "response", "id"); completedResponseID != responseID {
		t.Fatalf("completed response id = %q, want %q", completedResponseID, responseID)
	}
	response := completed["response"].(map[string]any)
	output := response["output"].([]any)
	if len(output) != 2 {
		t.Fatalf("completed output count = %d, want 2", len(output))
	}
	if output[0].(map[string]any)["type"] != "reasoning" || output[1].(map[string]any)["type"] != "function_call" {
		t.Fatalf("completed output = %#v", output)
	}
	if output[1].(map[string]any)["call_id"] != "call-1" {
		t.Fatalf("completed function call = %#v", output[1])
	}
}

// TestStreamEncoderEncodesFinalTextMessage 的测试动机是保证 final answer 同时具备 message item 和 output_text content part 生命周期。
func TestStreamEncoderEncodesFinalTextMessage(t *testing.T) {
	encoder := NewStreamEncoder("gpt-test")
	text := llm.TextContent{Text: "final answer"}
	partial := &llm.AssistantMessage{Content: []llm.Content{text}, StopReason: llm.StopReasonPending}
	final := &llm.AssistantMessage{Content: []llm.Content{text}, StopReason: llm.StopReasonStop}
	encoded := encodeStreamEvents(t, encoder, []llm.ResponseEvent{
		{Type: llm.ResponseEventStart, Partial: &llm.AssistantMessage{StopReason: llm.StopReasonPending}},
		{Type: llm.ResponseEventTextStart, ContentIndex: 0, Partial: partial},
		{Type: llm.ResponseEventTextDelta, ContentIndex: 0, Delta: "final ", Partial: partial},
		{Type: llm.ResponseEventTextDelta, ContentIndex: 0, Delta: "answer", Partial: partial},
		{Type: llm.ResponseEventTextEnd, ContentIndex: 0, Content: "final answer", Partial: partial},
		{Type: llm.ResponseEventDone, Reason: llm.StopReasonStop, Message: final},
	})
	assertEventNames(t, encoded, []string{
		"response.created", "response.in_progress", "response.output_item.added",
		"response.content_part.added", "response.output_text.delta", "response.output_text.delta",
		"response.output_text.done", "response.content_part.done", "response.output_item.done",
		"response.completed",
	})
	assertSequenceNumbers(t, encoded)
	itemAdded := decodeEventData(t, encoded[2])
	messageID := nestedString(t, itemAdded, "item", "id")
	if !strings.HasPrefix(messageID, "msg_") {
		t.Fatalf("message id = %q, want msg_ prefix", messageID)
	}
	partAdded := decodeEventData(t, encoded[3])
	if partAdded["item_id"] != messageID || partAdded["content_index"] != float64(0) {
		t.Fatalf("content part added = %#v", partAdded)
	}
	completed := decodeEventData(t, encoded[len(encoded)-1])
	output := completed["response"].(map[string]any)["output"].([]any)
	message := output[0].(map[string]any)
	content := message["content"].([]any)[0].(map[string]any)
	if message["type"] != "message" || content["text"] != "final answer" {
		t.Fatalf("completed message = %#v", message)
	}
}

// TestStreamEncoderHoldsReasoningForLateSignature 的测试动机是上游实测帧序
// thinking_end → toolcall_* → thinking_signature：reasoning item 必须挂起等待
// 隔块的尾随签名，而不是提前关闭把签名撞成 already-closed 错误。
func TestStreamEncoderHoldsReasoningForLateSignature(t *testing.T) {
	encoder := NewStreamEncoder("gpt-test")
	call := llm.ToolCall{ID: "call-1", Name: "lookup", Arguments: json.RawMessage(`{"city":"Shanghai"}`)}
	partial := &llm.AssistantMessage{
		Content:    []llm.Content{llm.ThinkingContent{Thinking: "inspect"}, call},
		StopReason: llm.StopReasonPending,
	}
	final := &llm.AssistantMessage{
		Content:    []llm.Content{llm.ThinkingContent{Thinking: "inspect", ThinkingSignature: "sig"}, call},
		StopReason: llm.StopReasonToolUse,
	}
	events := []llm.ResponseEvent{
		{Type: llm.ResponseEventStart, Partial: &llm.AssistantMessage{StopReason: llm.StopReasonPending}},
		{Type: llm.ResponseEventThinkingStart, ContentIndex: 0, Partial: partial},
		{Type: llm.ResponseEventThinkingDelta, ContentIndex: 0, Delta: "inspect", Partial: partial},
		{Type: llm.ResponseEventThinkingEnd, ContentIndex: 0, Content: "inspect", Partial: partial},
		{Type: llm.ResponseEventToolCallStart, ContentIndex: 1, ToolCallID: "call-1", ToolName: "lookup", Partial: partial},
		{Type: llm.ResponseEventToolCallDelta, ContentIndex: 1, ToolCallID: "call-1", Delta: `{"city":"Shanghai"}`, Partial: partial},
	}
	encoded := encodeStreamEvents(t, encoder, events)
	// 签名帧到达时 Partial 中的思考块已带上签名（与解码器共享指针语义一致）。
	partial.Content[0] = llm.ThinkingContent{Thinking: "inspect", ThinkingSignature: "sig"}
	late := []llm.ResponseEvent{
		{Type: llm.ResponseEventThinkingSignature, ContentIndex: 0, Delta: "sig", Partial: partial},
		{Type: llm.ResponseEventToolCallEnd, ContentIndex: 1, ToolCall: &call, Partial: partial},
		{Type: llm.ResponseEventDone, Reason: llm.StopReasonToolUse, Message: final},
	}
	encoded = append(encoded, encodeStreamEvents(t, encoder, late)...)
	assertEventNames(t, encoded, []string{
		"response.created", "response.in_progress",
		"response.output_item.added", "response.reasoning_summary_part.added",
		"response.reasoning_summary_text.delta",
		"response.output_item.added", "response.function_call_arguments.delta",
		"response.reasoning_summary_text.done", "response.reasoning_summary_part.done",
		"response.output_item.done",
		"response.function_call_arguments.done", "response.output_item.done",
		"response.completed",
	})
	assertSequenceNumbers(t, encoded)
	reasoningDone := decodeEventData(t, encoded[9])
	if got := nestedString(t, reasoningDone, "item", "encrypted_content"); got != "sig" {
		t.Fatalf("reasoning encrypted_content = %q, want sig", got)
	}
	completed := decodeEventData(t, encoded[len(encoded)-1])
	output := completed["response"].(map[string]any)["output"].([]any)
	if got := output[0].(map[string]any)["encrypted_content"]; got != "sig" {
		t.Fatalf("completed reasoning output = %#v", output[0])
	}
}

// TestStreamEncoderEncodesSignatureOnlyReasoning 的测试动机是 openai 体制上游
// 可能只发签名没有思考正文：解码器合成 thinking_start/end 后，reasoning item
// 必须正常关闭且签名不翻倍。
func TestStreamEncoderEncodesSignatureOnlyReasoning(t *testing.T) {
	encoder := NewStreamEncoder("gpt-test")
	thinking := llm.ThinkingContent{ThinkingSignature: "sig"}
	partial := &llm.AssistantMessage{Content: []llm.Content{thinking}, StopReason: llm.StopReasonPending}
	final := &llm.AssistantMessage{Content: []llm.Content{thinking}, StopReason: llm.StopReasonStop}
	encoded := encodeStreamEvents(t, encoder, []llm.ResponseEvent{
		{Type: llm.ResponseEventStart, Partial: &llm.AssistantMessage{StopReason: llm.StopReasonPending}},
		{Type: llm.ResponseEventThinkingStart, ContentIndex: 0, Partial: partial},
		{Type: llm.ResponseEventThinkingEnd, ContentIndex: 0, Partial: partial},
		{Type: llm.ResponseEventDone, Reason: llm.StopReasonStop, Message: final},
	})
	assertEventNames(t, encoded, []string{
		"response.created", "response.in_progress",
		"response.output_item.added", "response.reasoning_summary_part.added",
		"response.reasoning_summary_text.done", "response.reasoning_summary_part.done",
		"response.output_item.done", "response.completed",
	})
	completed := decodeEventData(t, encoded[len(encoded)-1])
	output := completed["response"].(map[string]any)["output"].([]any)
	if got := output[0].(map[string]any)["encrypted_content"]; got != "sig" {
		t.Fatalf("signature-only reasoning encrypted_content = %v, want sig", got)
	}
}

// TestStreamEncoderRejectsUnknownEvent 的测试动机是避免未知核心事件被静默丢弃并产生不完整 SSE。
func TestStreamEncoderRejectsUnknownEvent(t *testing.T) {
	encoder := NewStreamEncoder("gpt-test")
	if _, err := encoder.Encode(llm.ResponseEvent{Type: "unknown"}); err == nil {
		t.Fatal("Encode() error = nil, want unknown event error")
	}
}

// TestStreamEncoderRejectsDoneWithOpenItem 的测试动机是防止未产生 item done 的残缺 output 被包装成成功响应。
func TestStreamEncoderRejectsDoneWithOpenItem(t *testing.T) {
	encoder := NewStreamEncoder("gpt-test")
	partial := &llm.AssistantMessage{Content: []llm.Content{llm.TextContent{Text: "partial"}}, StopReason: llm.StopReasonPending}
	if _, err := encoder.Encode(llm.ResponseEvent{Type: llm.ResponseEventTextStart, ContentIndex: 0, Partial: partial}); err != nil {
		t.Fatal(err)
	}
	_, err := encoder.Encode(llm.ResponseEvent{
		Type: llm.ResponseEventDone, Reason: llm.StopReasonStop,
		Message: &llm.AssistantMessage{Content: partial.Content, StopReason: llm.StopReasonStop},
	})
	if err == nil || !strings.Contains(err.Error(), "open message item") {
		t.Fatalf("Encode(done) error = %v, want open item error", err)
	}
}

// TestStreamEncoderEncodesLengthAsIncomplete 的测试动机是避免达到 token 上限时向调用方谎报 completed。
func TestStreamEncoderEncodesLengthAsIncomplete(t *testing.T) {
	encoder := NewStreamEncoder("gpt-test")
	encoded := encodeStreamEvents(t, encoder, []llm.ResponseEvent{
		{Type: llm.ResponseEventStart, Partial: &llm.AssistantMessage{StopReason: llm.StopReasonPending}},
		{Type: llm.ResponseEventDone, Reason: llm.StopReasonLength, Message: &llm.AssistantMessage{StopReason: llm.StopReasonLength}},
	})
	assertEventNames(t, encoded, []string{"response.created", "response.in_progress", "response.incomplete"})
	response := decodeEventData(t, encoded[2])["response"].(map[string]any)
	if response["status"] != "incomplete" || response["completed_at"] != nil {
		t.Fatalf("incomplete response = %#v", response)
	}
}

// TestResponseUsageIncludesCachedTokensInInputTotal 的测试动机是把互斥的中间用量正确还原为 OpenAI 的输入总量和缓存子集。
func TestResponseUsageIncludesCachedTokensInInputTotal(t *testing.T) {
	encoded := responseUsage(llm.Usage{Input: 167, Output: 61, CacheRead: 12195, TotalTokens: 12423})
	if encoded["input_tokens"] != int64(12362) {
		t.Fatalf("input_tokens = %v, want 12362", encoded["input_tokens"])
	}
	details := encoded["input_tokens_details"].(map[string]any)
	if details["cached_tokens"] != int64(12195) {
		t.Fatalf("cached_tokens = %v, want 12195", details["cached_tokens"])
	}
	if encoded["output_tokens"] != int64(61) || encoded["total_tokens"] != int64(12423) {
		t.Fatalf("encoded usage = %#v", encoded)
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

func assertEventNames(t *testing.T, events []SSEEvent, want []string) {
	t.Helper()
	if len(events) != len(want) {
		t.Fatalf("event count = %d, want %d: %#v", len(events), len(want), events)
	}
	for index, name := range want {
		if events[index].Name != name {
			t.Fatalf("event[%d] = %q, want %q", index, events[index].Name, name)
		}
	}
}

func assertSequenceNumbers(t *testing.T, events []SSEEvent) {
	t.Helper()
	for index, event := range events {
		data := decodeEventData(t, event)
		if data["sequence_number"] != float64(index) {
			t.Fatalf("event[%d] sequence_number = %v", index, data["sequence_number"])
		}
	}
}

func decodeEventData(t *testing.T, event SSEEvent) map[string]any {
	t.Helper()
	var data map[string]any
	if err := json.Unmarshal(event.Data, &data); err != nil {
		t.Fatal(err)
	}
	return data
}

func nestedString(t *testing.T, value map[string]any, parent string, field string) string {
	t.Helper()
	nested, ok := value[parent].(map[string]any)
	if !ok {
		t.Fatalf("%s = %#v, want object", parent, value[parent])
	}
	result, ok := nested[field].(string)
	if !ok {
		t.Fatalf("%s.%s = %#v, want string", parent, field, nested[field])
	}
	return result
}

// TestStreamEncoderOpenAISignatureRestoresItemID 验证 openai 型签名
// （序列化 reasoning item 数组）下行时 item id 还原为内层真实 rs_*，
// encrypted_content 携带签名原文 blob 供下一轮回放识别。
func TestStreamEncoderOpenAISignatureRestoresItemID(t *testing.T) {
	encoder := NewStreamEncoder("gpt-test")
	blob := `[{"id":"rs_real1","type":"reasoning","encrypted_content":"gAAA","summary":[],"content":[],"status":""}]`
	thinking := llm.ThinkingContent{ThinkingSignature: blob, SignatureType: "openai", Redacted: true}
	partial := &llm.AssistantMessage{Content: []llm.Content{thinking}, StopReason: llm.StopReasonPending}
	final := &llm.AssistantMessage{Content: []llm.Content{thinking}, StopReason: llm.StopReasonStop}
	encoded := encodeStreamEvents(t, encoder, []llm.ResponseEvent{
		{Type: llm.ResponseEventStart, Partial: &llm.AssistantMessage{StopReason: llm.StopReasonPending}},
		{Type: llm.ResponseEventThinkingStart, ContentIndex: 0, Partial: partial},
		{Type: llm.ResponseEventThinkingEnd, ContentIndex: 0, Partial: partial},
		{Type: llm.ResponseEventDone, Reason: llm.StopReasonStop, Message: final},
	})
	added := decodeEventData(t, encoded[2])
	if got := nestedString(t, added, "item", "id"); got != "rs_real1" {
		t.Fatalf("reasoning item id = %q, want rs_real1", got)
	}
	if got := nestedString(t, added, "item", "encrypted_content"); got != blob {
		t.Fatalf("encrypted_content = %q, want raw signature blob", got)
	}
}

// TestStreamEncoderMessageItemUsesOutputID 验证上游 output_id（msg_*）
// 直接作为 message item id 下发，与上游记录对齐。
func TestStreamEncoderMessageItemUsesOutputID(t *testing.T) {
	encoder := NewStreamEncoder("gpt-test")
	partial := &llm.AssistantMessage{OutputID: "msg_up1", Content: []llm.Content{llm.TextContent{}}, StopReason: llm.StopReasonPending}
	final := &llm.AssistantMessage{OutputID: "msg_up1", Content: []llm.Content{llm.TextContent{Text: "hi"}}, StopReason: llm.StopReasonStop}
	encoded := encodeStreamEvents(t, encoder, []llm.ResponseEvent{
		{Type: llm.ResponseEventStart, Partial: &llm.AssistantMessage{StopReason: llm.StopReasonPending}},
		{Type: llm.ResponseEventTextStart, ContentIndex: 0, Partial: partial},
		{Type: llm.ResponseEventTextDelta, ContentIndex: 0, Delta: "hi", Partial: partial},
		{Type: llm.ResponseEventTextEnd, ContentIndex: 0, Content: "hi", Partial: partial},
		{Type: llm.ResponseEventDone, Reason: llm.StopReasonStop, Message: final},
	})
	added := decodeEventData(t, encoded[2])
	if got := nestedString(t, added, "item", "id"); got != "msg_up1" {
		t.Fatalf("message item id = %q, want msg_up1", got)
	}
}

// TestStreamEncoderCustomToolCall 验证 Custom 调用按 custom_tool_call item
// 下发：input 字段携带非 JSON 原文而非 arguments。
func TestStreamEncoderCustomToolCall(t *testing.T) {
	encoder := NewStreamEncoder("gpt-test")
	call := llm.ToolCall{ID: "c1", Name: "apply_patch", Arguments: json.RawMessage("*** Begin Patch"), Custom: true}
	partial := &llm.AssistantMessage{Content: []llm.Content{call}, StopReason: llm.StopReasonPending}
	final := &llm.AssistantMessage{Content: []llm.Content{call}, StopReason: llm.StopReasonToolUse}
	encoded := encodeStreamEvents(t, encoder, []llm.ResponseEvent{
		{Type: llm.ResponseEventStart, Partial: &llm.AssistantMessage{StopReason: llm.StopReasonPending}},
		{Type: llm.ResponseEventToolCallStart, ContentIndex: 0, ToolCallID: "c1", ToolName: "apply_patch", Partial: partial},
		{Type: llm.ResponseEventToolCallDelta, ContentIndex: 0, ToolCallID: "c1", Delta: "*** Begin Patch", Partial: partial},
		{Type: llm.ResponseEventToolCallEnd, ContentIndex: 0, ToolCall: &call, Partial: partial},
		{Type: llm.ResponseEventDone, Reason: llm.StopReasonToolUse, Message: final},
	})
	added := decodeEventData(t, encoded[2])
	if got := nestedString(t, added, "item", "type"); got != "custom_tool_call" {
		t.Fatalf("item type = %q, want custom_tool_call", got)
	}
	if name := decodeEventData(t, encoded[3])["type"]; name != "response.custom_tool_call_input.delta" {
		t.Fatalf("delta event = %v, want response.custom_tool_call_input.delta", name)
	}
	done := decodeEventData(t, encoded[5])
	if got := nestedString(t, done, "item", "input"); got != "*** Begin Patch" {
		t.Fatalf("custom input = %q", got)
	}
	if _, hasArguments := done["item"].(map[string]any)["arguments"]; hasArguments {
		t.Fatal("custom_tool_call must not carry arguments field")
	}
}
