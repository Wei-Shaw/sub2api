package kiro

import (
	"strings"
	"testing"
)

func TestAnthropicSSEWriter_EmitsMessageStartImmediately(t *testing.T) {
	var sb strings.Builder
	_ = NewAnthropicSSEWriter(&sb, nil, "claude-sonnet-4.6")
	out := sb.String()
	if !strings.Contains(out, "event: message_start") {
		t.Fatalf("missing message_start: %s", out)
	}
	if !strings.Contains(out, "\"role\":\"assistant\"") {
		t.Fatalf("message_start missing role: %s", out)
	}
}

func TestAnthropicSSEWriter_TextDeltaFlow(t *testing.T) {
	var sb strings.Builder
	w := NewAnthropicSSEWriter(&sb, nil, "m")
	w.WriteText("hello", false)
	w.WriteText(" world", false)
	w.WriteFinal(10, 5)

	out := sb.String()
	if !strings.Contains(out, "event: content_block_start") {
		t.Fatal("missing content_block_start")
	}
	if !strings.Contains(out, "\"text\":\"hello\"") {
		t.Fatalf("first text delta missing: %s", out)
	}
	if !strings.Contains(out, "\"text\":\" world\"") {
		t.Fatalf("second text delta missing: %s", out)
	}
	if !strings.Contains(out, "event: message_delta") {
		t.Fatal("missing message_delta")
	}
	if !strings.Contains(out, "\"stop_reason\":\"end_turn\"") {
		t.Fatalf("stop_reason missing: %s", out)
	}
	if !strings.Contains(out, "event: message_stop") {
		t.Fatal("missing message_stop")
	}
}

func TestAnthropicSSEWriter_ThinkingThenText(t *testing.T) {
	var sb strings.Builder
	w := NewAnthropicSSEWriter(&sb, nil, "m")
	w.WriteText("step 1", true)
	w.WriteText("answer", false)
	w.WriteFinal(0, 0)

	out := sb.String()
	if !strings.Contains(out, "\"type\":\"thinking_delta\"") {
		t.Fatalf("thinking_delta missing: %s", out)
	}
	if !strings.Contains(out, "\"type\":\"text_delta\"") {
		t.Fatalf("text_delta missing: %s", out)
	}
	// thinking block must close before text opens
	thinkClose := strings.Index(out, "content_block_stop")
	textStart := strings.Index(out, "\"text\":\"\"")
	if thinkClose == -1 || textStart == -1 || thinkClose > textStart {
		t.Fatalf("thinking didn't close before text opened: %s", out)
	}
}

func TestAnthropicSSEWriter_ToolUseBlock(t *testing.T) {
	var sb strings.Builder
	w := NewAnthropicSSEWriter(&sb, nil, "m")
	w.WriteText("calling tool", false)
	w.WriteToolUse(ToolUse{
		ToolUseID: "tu_1",
		Name:      "list_files",
		Input:     map[string]any{"path": "/tmp"},
	})
	w.WriteFinal(1, 2)

	out := sb.String()
	if !strings.Contains(out, "\"type\":\"tool_use\"") {
		t.Fatalf("tool_use block missing: %s", out)
	}
	if !strings.Contains(out, "\"name\":\"list_files\"") {
		t.Fatalf("tool name missing: %s", out)
	}
	if !strings.Contains(out, "\"type\":\"input_json_delta\"") {
		t.Fatalf("input_json_delta missing: %s", out)
	}
	if !strings.Contains(out, `\"path\":\"/tmp\"`) {
		t.Fatalf("tool input missing: %s", out)
	}
	if !strings.Contains(out, "\"stop_reason\":\"tool_use\"") {
		t.Fatalf("stop_reason should be tool_use: %s", out)
	}
}

func TestAnthropicSSEWriter_FlusherInvokedPerEvent(t *testing.T) {
	var sb strings.Builder
	flushes := 0
	w := NewAnthropicSSEWriter(&sb, func() { flushes++ }, "m")
	w.WriteText("hi", false)
	w.WriteFinal(0, 0)
	// message_start (1) + content_block_start (1) + 1 delta + content_block_stop +
	// message_delta + message_stop = 6
	if flushes < 5 {
		t.Fatalf("flushes = %d, expected >= 5", flushes)
	}
}

func TestBuildAnthropicNonStreamingResponse_AggregatesContent(t *testing.T) {
	events := []Event{
		{Type: "assistantResponseEvent", Payload: map[string]any{"content": "hello"}},
		{Type: "assistantResponseEvent", Payload: map[string]any{"content": "hello world"}},
		{Type: "toolUseEvent", Payload: map[string]any{
			"toolUseId": "tu",
			"name":      "f",
			"input":     `{"x":1}`,
			"stop":      true,
		}},
		{Type: "meteringEvent", Payload: map[string]any{
			"usage": map[string]any{
				"inputTokens":  3.0,
				"outputTokens": 4.0,
			},
		}},
	}
	resp := BuildAnthropicNonStreamingResponse(events, "claude-sonnet-4.6", nil)
	if resp.StopReason != "tool_use" {
		t.Fatalf("stop = %q", resp.StopReason)
	}
	if resp.Model != "claude-sonnet-4.6" {
		t.Fatal("model not set")
	}
	if resp.Usage["input_tokens"] != 3 || resp.Usage["output_tokens"] != 4 {
		t.Fatalf("usage = %+v", resp.Usage)
	}
	// blocks: text + tool_use
	if len(resp.Content) != 2 {
		t.Fatalf("len(blocks) = %d", len(resp.Content))
	}
	if resp.Content[0]["type"] != "text" || resp.Content[0]["text"] != "hello world" {
		t.Fatalf("text block wrong: %+v", resp.Content[0])
	}
	if resp.Content[1]["type"] != "tool_use" {
		t.Fatalf("tool_use block wrong: %+v", resp.Content[1])
	}
}

func TestHTTPError_FormatsCleanly(t *testing.T) {
	err := &HTTPError{StatusCode: 429, Body: []byte("limit")}
	if !strings.Contains(err.Error(), "429") {
		t.Fatalf("error msg: %v", err)
	}
}
