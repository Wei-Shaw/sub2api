// 本文件验证 OpenAI Chat Completions 最终 JSON 和 SSE chunk 编码。
package chat

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/devin/llm"
)

// TestStreamEncoderEmitsRoleAndText 验证流式文本产生 role chunk 和 content delta。
func TestStreamEncoderEmitsRoleAndText(t *testing.T) {
	encoder := NewStreamEncoder("gpt-test", false)
	text := llm.TextContent{Text: "final answer"}
	partial := &llm.AssistantMessage{Content: []llm.Content{text}, StopReason: llm.StopReasonPending}
	final := &llm.AssistantMessage{Content: []llm.Content{text}, StopReason: llm.StopReasonStop}
	events := []llm.ResponseEvent{
		{Type: llm.ResponseEventStart, Partial: &llm.AssistantMessage{StopReason: llm.StopReasonPending}},
		{Type: llm.ResponseEventTextStart, ContentIndex: 0, Partial: partial},
		{Type: llm.ResponseEventTextDelta, ContentIndex: 0, Delta: "final ", Partial: partial},
		{Type: llm.ResponseEventTextDelta, ContentIndex: 0, Delta: "answer", Partial: partial},
		{Type: llm.ResponseEventTextEnd, ContentIndex: 0, Content: "final answer", Partial: partial},
		{Type: llm.ResponseEventDone, Reason: llm.StopReasonStop, Message: final},
	}
	encoded := encodeStreamEvents(t, encoder, events)
	if len(encoded) != 5 { // role, "final ", "answer", finish, [DONE]
		t.Fatalf("event count = %d, want 5", len(encoded))
	}
	first := decodeEventData(t, encoded[0])
	if choices, _ := first["choices"].([]any); len(choices) == 0 {
		t.Fatal("first event missing choices")
	} else if choice, _ := choices[0].(map[string]any); choice == nil {
		t.Fatal("first event choice malformed")
	} else if delta, _ := choice["delta"].(map[string]any); delta["role"] != "assistant" {
		t.Fatalf("first event role missing: %v", first)
	}
	if strings.Contains(string(encoded[0].Data), `"content":`) {
		t.Fatalf("first chunk should not contain content: %s", encoded[0].Data)
	}
	if mustMap(t, mustMap(t, mustSlice(t, decodeEventData(t, encoded[1])["choices"])[0])["delta"])["content"] != "final " {
		t.Fatalf("second chunk content wrong")
	}
}

// TestStreamEncoderEmitsToolCalls 验证流式工具调用按 OpenAI Chat 增量格式输出。
func TestStreamEncoderEmitsToolCalls(t *testing.T) {
	encoder := NewStreamEncoder("gpt-test", false)
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
	if len(encoded) != 6 { // role, tool start, arg delta x2, finish, [DONE]
		t.Fatalf("event count = %d, want 6", len(encoded))
	}
	firstTool := decodeEventData(t, encoded[1])
	toolCall, _ := firstTool["choices"].([]any)[0].(map[string]any)["delta"].(map[string]any)["tool_calls"].([]any)[0].(map[string]any)
	if mustMap(t, toolCall["function"])["name"] != "lookup" {
		t.Fatalf("tool call name = %v", toolCall["function"])
	}
}

// TestStreamEncoderToolCallAfterOtherBlocks 钉住工具查找语义：ContentIndex 是
// partial.Content 的全局块下标（thinking/text 占 0/1，工具在 2），不能与
// tool_calls 输出序号混比。delta 的 id 与 start 不一致时（上游再键控），
// 参数增量必须仍归属正确的工具而不是被丢弃或错挂到 index=0 的块上。
func TestStreamEncoderToolCallAfterOtherBlocks(t *testing.T) {
	encoder := NewStreamEncoder("gpt-test", false)
	call := llm.ToolCall{ID: "call-1", Name: "lookup", Arguments: json.RawMessage(`{"city":"Shanghai"}`)}
	partial := &llm.AssistantMessage{Content: []llm.Content{
		llm.ThinkingContent{Thinking: "t"}, llm.TextContent{Text: "x"}, call,
	}, StopReason: llm.StopReasonPending}
	final := &llm.AssistantMessage{Content: partial.Content, StopReason: llm.StopReasonToolUse}
	events := []llm.ResponseEvent{
		{Type: llm.ResponseEventStart, Partial: &llm.AssistantMessage{StopReason: llm.StopReasonPending}},
		{Type: llm.ResponseEventToolCallStart, ContentIndex: 2, ToolCallID: "call-1", ToolName: "lookup", Partial: partial},
		{Type: llm.ResponseEventToolCallDelta, ContentIndex: 2, ToolCallID: "call-rekeyed", Delta: `{"city":"`, Partial: partial},
		{Type: llm.ResponseEventToolCallDelta, ContentIndex: 2, ToolCallID: "call-rekeyed", Delta: `Shanghai"}`, Partial: partial},
		{Type: llm.ResponseEventToolCallEnd, ContentIndex: 2, ToolCall: &call, Partial: partial},
		{Type: llm.ResponseEventDone, Reason: llm.StopReasonToolUse, Message: final},
	}
	encoded := encodeStreamEvents(t, encoder, events)
	// role, tool start, arg delta x2, finish, [DONE]——两个 delta 都必须下发。
	if len(encoded) != 6 {
		t.Fatalf("event count = %d, want 6 (arg deltas must not be dropped)", len(encoded))
	}
	delta := decodeEventData(t, encoded[2])
	toolCall, _ := delta["choices"].([]any)[0].(map[string]any)["delta"].(map[string]any)["tool_calls"].([]any)[0].(map[string]any)
	if toolCall["index"] != float64(0) {
		t.Fatalf("tool_calls index = %v, want 0 (output ordinal, not content index)", toolCall["index"])
	}
	if mustMap(t, toolCall["function"])["arguments"] != `{"city":"` {
		t.Fatalf("arguments delta = %v", toolCall["function"])
	}
}

// TestEncodeResponseFinal 验证非流式最终 JSON 结构和用量缓存字段。
func TestEncodeResponseFinal(t *testing.T) {
	final := &llm.AssistantMessage{
		ResponseID:    "chatcmpl-1",
		ResponseModel: "gpt-test",
		Content:       []llm.Content{llm.TextContent{Text: "hello"}},
		StopReason:    llm.StopReasonStop,
		Usage:         llm.Usage{Input: 10, Output: 5, CacheRead: 3, TotalTokens: 18},
	}
	body, err := EncodeResponse(final)
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed["object"] != "chat.completion" || parsed["model"] != "gpt-test" {
		t.Fatalf("response = %#v", parsed)
	}
	usage, _ := parsed["usage"].(map[string]any)
	details, _ := usage["prompt_tokens_details"].(map[string]any)
	if details["cached_tokens"] != float64(3) {
		t.Fatalf("cached_tokens = %v", details["cached_tokens"])
	}
}

// TestEncodeResponseFinalWithReasoning 验证非流式下 thinking 与 text 分离开。
func TestEncodeResponseFinalWithReasoning(t *testing.T) {
	final := &llm.AssistantMessage{
		ResponseID:    "chatcmpl-2",
		ResponseModel: "gpt-test",
		Content: []llm.Content{
			llm.ThinkingContent{Thinking: "think"},
			llm.TextContent{Text: "hello"},
		},
		StopReason: llm.StopReasonStop,
	}
	body, err := EncodeResponse(final)
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatal(err)
	}
	message, _ := parsed["choices"].([]any)[0].(map[string]any)["message"].(map[string]any)
	if message["content"] != "hello" {
		t.Fatalf("content = %v, want hello", message["content"])
	}
	if message["reasoning_content"] != "think" {
		t.Fatalf("reasoning_content = %v, want think", message["reasoning_content"])
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

// mustMap 断言 v 为 map 并返回（测试失败即终止）。
func mustMap(t *testing.T, v any) map[string]any {
	t.Helper()
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", v)
	}
	return m
}

// mustSlice 断言 v 为切片并返回。
func mustSlice(t *testing.T, v any) []any {
	t.Helper()
	s, ok := v.([]any)
	if !ok {
		t.Fatalf("expected []any, got %T", v)
	}
	return s
}

func decodeEventData(t *testing.T, event SSEEvent) map[string]any {
	t.Helper()
	var data map[string]any
	if err := json.Unmarshal(event.Data, &data); err != nil {
		t.Fatal(err)
	}
	return data
}

// TestStreamEncoderEmitsError 验证流式错误生成带 error 字段的 chat.completion.chunk。
func TestStreamEncoderEmitsError(t *testing.T) {
	encoder := NewStreamEncoder("gpt-test", false)
	failed := &llm.AssistantMessage{Provider: "devin", StopReason: llm.StopReasonError, ErrorMessage: "resource_exhausted: rate limit exceeded"}
	event := llm.ResponseEvent{Type: llm.ResponseEventError, Reason: llm.StopReasonError, Error: failed}
	encoded, err := encoder.Encode(event)
	if err != nil {
		t.Fatalf("encode error: %v", err)
	}
	if len(encoded) != 1 {
		t.Fatalf("event count = %d, want 1", len(encoded))
	}
	if encoded[0].Name != "" {
		t.Fatalf("error chunk should be data-only, got event name %q", encoded[0].Name)
	}
	data := decodeEventData(t, encoded[0])
	if data["object"] != "chat.completion.chunk" {
		t.Fatalf("object = %v", data["object"])
	}
	choices, ok := data["choices"].([]any)
	if !ok || len(choices) != 0 {
		t.Fatalf("choices should be empty, got %v", data["choices"])
	}
	errObj, ok := data["error"].(map[string]any)
	if !ok {
		t.Fatalf("missing error object: %v", data)
	}
	if errObj["message"] != "resource_exhausted: rate limit exceeded" {
		t.Fatalf("error.message = %v", errObj["message"])
	}
	if errObj["type"] != "rate_limit_error" {
		t.Fatalf("error.type = %v, want rate_limit_error", errObj["type"])
	}
	// 错误后再次编码应因流已结束而失败。
	if _, err := encoder.Encode(event); err == nil {
		t.Fatal("encoding after error should fail")
	}
}

// TestStreamEncoderEmitsThinkingAsReasoningContent 验证 OpenAI Chat 流式下思考走 reasoning_content、正文走 content。
func TestStreamEncoderEmitsThinkingAsReasoningContent(t *testing.T) {
	encoder := NewStreamEncoder("gpt-test", false)
	thinking := llm.ThinkingContent{Thinking: "think"}
	text := llm.TextContent{Text: "hello"}
	partial := &llm.AssistantMessage{Content: []llm.Content{thinking, text}, StopReason: llm.StopReasonPending}
	final := &llm.AssistantMessage{Content: []llm.Content{thinking, text}, StopReason: llm.StopReasonStop}
	events := []llm.ResponseEvent{
		{Type: llm.ResponseEventStart, Partial: &llm.AssistantMessage{StopReason: llm.StopReasonPending}},
		{Type: llm.ResponseEventThinkingStart, ContentIndex: 0, Partial: partial},
		{Type: llm.ResponseEventThinkingDelta, ContentIndex: 0, Delta: "think", Partial: partial},
		{Type: llm.ResponseEventThinkingEnd, ContentIndex: 0, Content: "think", Partial: partial},
		{Type: llm.ResponseEventTextStart, ContentIndex: 1, Partial: partial},
		{Type: llm.ResponseEventTextDelta, ContentIndex: 1, Delta: "hello", Partial: partial},
		{Type: llm.ResponseEventTextEnd, ContentIndex: 1, Content: "hello", Partial: partial},
		{Type: llm.ResponseEventDone, Reason: llm.StopReasonStop, Message: final},
	}
	encoded := encodeStreamEvents(t, encoder, events)
	// role, reasoning "think", text "hello", finish, [DONE]
	if len(encoded) != 5 {
		t.Fatalf("event count = %d, want 5", len(encoded))
	}
	second := decodeEventData(t, encoded[1])
	if mustMap(t, mustMap(t, mustSlice(t, second["choices"])[0])["delta"])["reasoning_content"] != "think" {
		t.Fatalf("second chunk should be thinking as reasoning_content: %v", second)
	}
	third := decodeEventData(t, encoded[2])
	if mustMap(t, mustMap(t, mustSlice(t, third["choices"])[0])["delta"])["content"] != "hello" {
		t.Fatalf("third chunk should be text content: %v", third)
	}
}

// TestMessageToChatKeepsTextWithToolCalls 钉住 content 与 tool_calls 共存：
// 模型同轮给正文+调用时正文必须下发（上游实测会这么回），纯调用轮才置 nil。
func TestMessageToChatKeepsTextWithToolCalls(t *testing.T) {
	message := &llm.AssistantMessage{Content: []llm.Content{
		llm.TextContent{Text: "let me check"},
		llm.ToolCall{ID: "c1", Name: "read_file", Arguments: json.RawMessage(`{"path":"a"}`)},
	}}
	object, calls := messageToChat(message)
	if object["content"] != "let me check" || len(calls) != 1 {
		t.Fatalf("message = %#v, calls = %#v", object, calls)
	}
	onlyCall, _ := messageToChat(&llm.AssistantMessage{Content: []llm.Content{
		llm.ToolCall{ID: "c1", Name: "read_file", Arguments: json.RawMessage(`{"path":"a"}`)},
	}})
	if content, ok := onlyCall["content"]; !ok || content != nil {
		t.Fatalf("pure tool call turn should emit content=null, got %#v", onlyCall)
	}
}
