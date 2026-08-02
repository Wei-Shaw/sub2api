package apicompat

import (
	"encoding/json"
	"testing"
)

// ---------------------------------------------------------------------------
// Streaming: server_tool_use(web_search) → web_search_call
// ---------------------------------------------------------------------------

func TestStreaming_ServerToolUse_WebSearch_EmitsWebSearchCall(t *testing.T) {
	state := NewAnthropicEventToResponsesState()
	state.Model = "k3-256k"

	var events []ResponsesStreamEvent
	feed := func(evt *AnthropicStreamEvent) {
		events = append(events, AnthropicEventToResponsesEvents(evt, state)...)
	}

	idx0, idx1, idx2, idx3 := 0, 1, 2, 3
	feed(&AnthropicStreamEvent{Type: "message_start", Message: &AnthropicResponse{ID: "msg_ws1", Model: "k3-256k"}})

	// Block 0: status text "Search results for query: ..."
	feed(&AnthropicStreamEvent{Type: "content_block_start", Index: &idx0, ContentBlock: &AnthropicContentBlock{Type: "text"}})
	feed(&AnthropicStreamEvent{Type: "content_block_delta", Index: &idx0, Delta: &AnthropicDelta{Type: "text_delta", Text: "Search results for query: 北京天气"}})
	feed(&AnthropicStreamEvent{Type: "content_block_stop", Index: &idx0})

	// Block 1: server_tool_use
	feed(&AnthropicStreamEvent{Type: "content_block_start", Index: &idx1, ContentBlock: &AnthropicContentBlock{
		Type:  "server_tool_use",
		ID:    "srvtoolu_abc",
		Name:  "web_search",
		Input: json.RawMessage(`{"query":"北京天气"}`),
	}})
	feed(&AnthropicStreamEvent{Type: "content_block_stop", Index: &idx1})

	// Block 2: web_search_tool_result
	feed(&AnthropicStreamEvent{Type: "content_block_start", Index: &idx2, ContentBlock: &AnthropicContentBlock{
		Type:      "web_search_tool_result",
		ToolUseID: "srvtoolu_abc",
		Content:   json.RawMessage(`[{"type":"web_search_result","url":"https://weather.com","title":"天气预报"}]`),
	}})
	feed(&AnthropicStreamEvent{Type: "content_block_stop", Index: &idx2})

	// Block 3: final text answer
	feed(&AnthropicStreamEvent{Type: "content_block_start", Index: &idx3, ContentBlock: &AnthropicContentBlock{Type: "text"}})
	feed(&AnthropicStreamEvent{Type: "content_block_delta", Index: &idx3, Delta: &AnthropicDelta{Type: "text_delta", Text: "北京今天晴朗"}})
	feed(&AnthropicStreamEvent{Type: "content_block_stop", Index: &idx3})
	feed(&AnthropicStreamEvent{Type: "message_stop"})

	var types []string
	for _, e := range events {
		types = append(types, e.Type)
	}

	// Must contain web_search_call events
	hasItemAdded := false
	hasItemDone := false
	for _, e := range events {
		if e.Type == "response.output_item.added" && e.Item != nil && e.Item.Type == "web_search_call" {
			hasItemAdded = true
			if e.Item.Status != "in_progress" {
				t.Errorf("web_search_call added status = %q, want in_progress", e.Item.Status)
			}
			if e.Item.Action == nil || e.Item.Action.Query != "北京天气" {
				t.Errorf("web_search_call query mismatch, got %+v", e.Item.Action)
			}
		}
		if e.Type == "response.output_item.done" && e.Item != nil && e.Item.Type == "web_search_call" {
			hasItemDone = true
			if e.Item.Status != "completed" {
				t.Errorf("web_search_call done status = %q, want completed", e.Item.Status)
			}
		}
	}
	if !hasItemAdded {
		t.Errorf("missing response.output_item.added for web_search_call; events: %v", types)
	}
	if !hasItemDone {
		t.Errorf("missing response.output_item.done for web_search_call; events: %v", types)
	}

	// Status text must NOT appear as output_text.delta
	for _, e := range events {
		if e.Type == "response.output_text.delta" && e.Delta == "Search results for query: 北京天气" {
			t.Error("search status text leaked to client as output_text.delta")
		}
	}

	// Final answer must appear
	hasFinalText := false
	for _, e := range events {
		if e.Type == "response.output_text.delta" && e.Delta == "北京今天晴朗" {
			hasFinalText = true
		}
	}
	if !hasFinalText {
		t.Error("final answer text not emitted")
	}

	// Verify completed output list
	var completed *ResponsesStreamEvent
	for i := range events {
		if events[i].Type == "response.completed" {
			completed = &events[i]
		}
	}
	if completed == nil || completed.Response == nil {
		t.Fatal("response.completed not emitted")
	}
	foundWS := false
	foundMsg := false
	for _, o := range completed.Response.Output {
		if o.Type == "web_search_call" {
			foundWS = true
		}
		if o.Type == "message" {
			foundMsg = true
		}
	}
	if !foundWS {
		t.Error("response.completed missing web_search_call in output")
	}
	if !foundMsg {
		t.Error("response.completed missing message in output")
	}
}

// TestStreaming_StatusTextNotFollowedBySearch_FlushesNormally verifies that
// text starting with "Search results for query:" is NOT suppressed if the
// next block is not server_tool_use(web_search).
func TestStreaming_StatusTextNotFollowedBySearch_FlushesNormally(t *testing.T) {
	state := NewAnthropicEventToResponsesState()
	state.Model = "k3-256k"

	var events []ResponsesStreamEvent
	feed := func(evt *AnthropicStreamEvent) {
		events = append(events, AnthropicEventToResponsesEvents(evt, state)...)
	}

	idx0, idx1 := 0, 1
	feed(&AnthropicStreamEvent{Type: "message_start", Message: &AnthropicResponse{ID: "msg_ws2"}})

	// Text with "Search results for query:" prefix
	feed(&AnthropicStreamEvent{Type: "content_block_start", Index: &idx0, ContentBlock: &AnthropicContentBlock{Type: "text"}})
	feed(&AnthropicStreamEvent{Type: "content_block_delta", Index: &idx0, Delta: &AnthropicDelta{Type: "text_delta", Text: "Search results for query: test"}})
	feed(&AnthropicStreamEvent{Type: "content_block_stop", Index: &idx0})

	// Next block is normal text, NOT server_tool_use
	feed(&AnthropicStreamEvent{Type: "content_block_start", Index: &idx1, ContentBlock: &AnthropicContentBlock{Type: "text"}})
	feed(&AnthropicStreamEvent{Type: "content_block_delta", Index: &idx1, Delta: &AnthropicDelta{Type: "text_delta", Text: "answer"}})
	feed(&AnthropicStreamEvent{Type: "content_block_stop", Index: &idx1})
	feed(&AnthropicStreamEvent{Type: "message_stop"})

	// The "Search results for query: test" text should be flushed as normal output
	hasStatusText := false
	for _, e := range events {
		if e.Type == "response.output_text.delta" && e.Delta == "Search results for query: test" {
			hasStatusText = true
		}
	}
	if !hasStatusText {
		t.Error("text matching search prefix should be flushed as normal text when not followed by server_tool_use")
	}
}

// TestStreaming_IncrementalStatusTextPrefix verifies that text arriving in
// incremental deltas matching the prefix pattern is correctly buffered.
func TestStreaming_IncrementalStatusTextPrefix(t *testing.T) {
	state := NewAnthropicEventToResponsesState()
	state.Model = "k3-256k"

	var events []ResponsesStreamEvent
	feed := func(evt *AnthropicStreamEvent) {
		events = append(events, AnthropicEventToResponsesEvents(evt, state)...)
	}

	idx0, idx1 := 0, 1
	feed(&AnthropicStreamEvent{Type: "message_start", Message: &AnthropicResponse{ID: "msg_ws3"}})

	// Status text arriving in fragments
	feed(&AnthropicStreamEvent{Type: "content_block_start", Index: &idx0, ContentBlock: &AnthropicContentBlock{Type: "text"}})
	feed(&AnthropicStreamEvent{Type: "content_block_delta", Index: &idx0, Delta: &AnthropicDelta{Type: "text_delta", Text: "Search "}})
	feed(&AnthropicStreamEvent{Type: "content_block_delta", Index: &idx0, Delta: &AnthropicDelta{Type: "text_delta", Text: "results for query: foo"}})
	feed(&AnthropicStreamEvent{Type: "content_block_stop", Index: &idx0})

	// Followed by server_tool_use → should discard
	feed(&AnthropicStreamEvent{Type: "content_block_start", Index: &idx1, ContentBlock: &AnthropicContentBlock{
		Type:  "server_tool_use",
		ID:    "srvtoolu_xyz",
		Name:  "web_search",
		Input: json.RawMessage(`{"query":"foo"}`),
	}})
	feed(&AnthropicStreamEvent{Type: "content_block_stop", Index: &idx1})

	// Status text should NOT be emitted
	for _, e := range events {
		if e.Type == "response.output_text.delta" {
			t.Errorf("incremental status text should be suppressed, got delta: %q", e.Delta)
		}
	}
}

// TestStreaming_NonSearchTextNotBuffered verifies that normal text not matching
// the search prefix pattern passes through immediately.
func TestStreaming_NonSearchTextNotBuffered(t *testing.T) {
	state := NewAnthropicEventToResponsesState()
	state.Model = "k3-256k"

	var events []ResponsesStreamEvent
	feed := func(evt *AnthropicStreamEvent) {
		events = append(events, AnthropicEventToResponsesEvents(evt, state)...)
	}

	idx0 := 0
	feed(&AnthropicStreamEvent{Type: "message_start", Message: &AnthropicResponse{ID: "msg_ws4"}})

	feed(&AnthropicStreamEvent{Type: "content_block_start", Index: &idx0, ContentBlock: &AnthropicContentBlock{Type: "text"}})
	feed(&AnthropicStreamEvent{Type: "content_block_delta", Index: &idx0, Delta: &AnthropicDelta{Type: "text_delta", Text: "Hello world"}})

	// After first delta that doesn't match prefix, should immediately emit
	hasDelta := false
	for _, e := range events {
		if e.Type == "response.output_text.delta" && e.Delta == "Hello world" {
			hasDelta = true
		}
	}
	if !hasDelta {
		var types []string
		for _, e := range events {
			types = append(types, e.Type)
		}
		t.Errorf("normal text should be emitted immediately; events: %v", types)
	}
}

// ---------------------------------------------------------------------------
// Non-streaming: AnthropicToResponsesResponse with web search blocks
// ---------------------------------------------------------------------------

func TestNonStreaming_WebSearch_ConvertsToWebSearchCall(t *testing.T) {
	resp := &AnthropicResponse{
		ID:    "msg_ns1",
		Model: "k3-256k",
		Content: []AnthropicContentBlock{
			{Type: "text", Text: "Search results for query: golang"},
			{Type: "server_tool_use", ID: "srvtoolu_1", Name: "web_search", Input: json.RawMessage(`{"query":"golang"}`)},
			{Type: "web_search_tool_result", ToolUseID: "srvtoolu_1", Content: json.RawMessage(`[{"type":"web_search_result","url":"https://go.dev","title":"Go"}]`)},
			{Type: "text", Text: "Go is a programming language."},
		},
		StopReason: AnthropicStopReasonPtr("end_turn"),
		Usage:      AnthropicUsage{InputTokens: 10, OutputTokens: 20},
	}

	result := AnthropicToResponsesResponse(resp)

	foundWS := false
	foundMsg := false
	for _, o := range result.Output {
		if o.Type == "web_search_call" {
			foundWS = true
			if o.Status != "completed" {
				t.Errorf("web_search_call status = %q, want completed", o.Status)
			}
			if o.Action == nil || o.Action.Query != "golang" {
				t.Errorf("web_search_call query mismatch: %+v", o.Action)
			}
		}
		if o.Type == "message" {
			foundMsg = true
			// Status text should be filtered out
			for _, part := range o.Content {
				if part.Type == "output_text" && part.Text == "Search results for query: golang" {
					t.Error("search status text should be filtered from message content")
				}
			}
			// Final answer should be present
			hasAnswer := false
			for _, part := range o.Content {
				if part.Type == "output_text" && part.Text == "Go is a programming language." {
					hasAnswer = true
				}
			}
			if !hasAnswer {
				t.Error("final answer text not found in message content")
			}
		}
	}
	if !foundWS {
		t.Error("web_search_call not found in output")
	}
	if !foundMsg {
		t.Error("message not found in output")
	}
}

func TestNonStreaming_StatusTextNotAdjacentToSearch_Preserved(t *testing.T) {
	resp := &AnthropicResponse{
		ID:    "msg_ns2",
		Model: "k3-256k",
		Content: []AnthropicContentBlock{
			{Type: "text", Text: "Search results for query: something"},
			{Type: "text", Text: "Normal answer"},
		},
		StopReason: AnthropicStopReasonPtr("end_turn"),
		Usage:      AnthropicUsage{InputTokens: 10, OutputTokens: 20},
	}

	result := AnthropicToResponsesResponse(resp)

	var texts []string
	for _, o := range result.Output {
		if o.Type == "message" {
			for _, part := range o.Content {
				if part.Type == "output_text" {
					texts = append(texts, part.Text)
				}
			}
		}
	}

	// "Search results for query:" text is NOT followed by server_tool_use(web_search)
	// so it should be preserved
	found := false
	for _, text := range texts {
		if text == "Search results for query: something" {
			found = true
		}
	}
	if !found {
		t.Errorf("text not adjacent to web_search should be preserved; got texts: %v", texts)
	}
}

func TestNonStreaming_MultipleWebSearches(t *testing.T) {
	resp := &AnthropicResponse{
		ID:    "msg_ns3",
		Model: "k3-256k",
		Content: []AnthropicContentBlock{
			{Type: "text", Text: "Search results for query: q1"},
			{Type: "server_tool_use", ID: "srvtoolu_a", Name: "web_search", Input: json.RawMessage(`{"query":"q1"}`)},
			{Type: "web_search_tool_result", ToolUseID: "srvtoolu_a"},
			{Type: "text", Text: "Search results for query: q2"},
			{Type: "server_tool_use", ID: "srvtoolu_b", Name: "web_search", Input: json.RawMessage(`{"query":"q2"}`)},
			{Type: "web_search_tool_result", ToolUseID: "srvtoolu_b"},
			{Type: "text", Text: "Final combined answer."},
		},
		StopReason: AnthropicStopReasonPtr("end_turn"),
		Usage:      AnthropicUsage{InputTokens: 10, OutputTokens: 20},
	}

	result := AnthropicToResponsesResponse(resp)

	wsCount := 0
	for _, o := range result.Output {
		if o.Type == "web_search_call" {
			wsCount++
		}
	}
	if wsCount != 2 {
		t.Errorf("expected 2 web_search_call items, got %d", wsCount)
	}

	// Status texts should be filtered
	for _, o := range result.Output {
		if o.Type == "message" {
			for _, part := range o.Content {
				if part.Type == "output_text" && part.Text != "Final combined answer." {
					t.Errorf("unexpected text in message: %q", part.Text)
				}
			}
		}
	}
}

// TestStreaming_MessageStopFlushesHeldText verifies that pending status text
// is flushed on normal message_stop (issue: text was lost when no further
// content_block_start arrives to trigger flush decision).
func TestStreaming_MessageStopFlushesHeldText(t *testing.T) {
	state := NewAnthropicEventToResponsesState()
	state.Model = "k3-256k"

	var events []ResponsesStreamEvent
	feed := func(evt *AnthropicStreamEvent) {
		events = append(events, AnthropicEventToResponsesEvents(evt, state)...)
	}

	idx0 := 0
	feed(&AnthropicStreamEvent{Type: "message_start", Message: &AnthropicResponse{ID: "msg_ms1"}})
	feed(&AnthropicStreamEvent{Type: "content_block_start", Index: &idx0, ContentBlock: &AnthropicContentBlock{Type: "text"}})
	feed(&AnthropicStreamEvent{Type: "content_block_delta", Index: &idx0, Delta: &AnthropicDelta{Type: "text_delta", Text: "Search results for query: last block"}})
	feed(&AnthropicStreamEvent{Type: "content_block_stop", Index: &idx0})
	// Normal message_delta + message_stop without any subsequent block
	feed(&AnthropicStreamEvent{Type: "message_delta", Delta: &AnthropicDelta{StopReason: "end_turn"}})
	feed(&AnthropicStreamEvent{Type: "message_stop"})

	hasText := false
	for _, e := range events {
		if e.Type == "response.output_text.delta" && e.Delta == "Search results for query: last block" {
			hasText = true
		}
	}
	if !hasText {
		t.Error("pending status text must be flushed on message_stop when no following block exists")
	}

	// Should also appear in completed output
	var completed *ResponsesStreamEvent
	for i := range events {
		if events[i].Type == "response.completed" {
			completed = &events[i]
		}
	}
	if completed == nil || completed.Response == nil {
		t.Fatal("response.completed not emitted")
	}
	if len(completed.Response.Output) == 0 {
		t.Fatal("completed output is empty")
	}
	msg := completed.Response.Output[0]
	if msg.Type != "message" || len(msg.Content) == 0 || msg.Content[0].Text != "Search results for query: last block" {
		t.Errorf("completed output message content mismatch: %+v", msg)
	}
}

// TestStreaming_WebSearchInputJsonDelta verifies that input_json_delta during
// a web_search_call does NOT emit function_call_arguments.delta events.
func TestStreaming_WebSearchInputJsonDelta(t *testing.T) {
	state := NewAnthropicEventToResponsesState()
	state.Model = "k3-256k"

	var events []ResponsesStreamEvent
	feed := func(evt *AnthropicStreamEvent) {
		events = append(events, AnthropicEventToResponsesEvents(evt, state)...)
	}

	idx0, idx1 := 0, 1
	feed(&AnthropicStreamEvent{Type: "message_start", Message: &AnthropicResponse{ID: "msg_ijd"}})

	// server_tool_use starts with empty input (will be streamed)
	feed(&AnthropicStreamEvent{Type: "content_block_start", Index: &idx0, ContentBlock: &AnthropicContentBlock{
		Type: "server_tool_use", ID: "srvtoolu_ijd", Name: "web_search",
	}})
	// Input arrives via input_json_delta
	feed(&AnthropicStreamEvent{Type: "content_block_delta", Index: &idx0, Delta: &AnthropicDelta{Type: "input_json_delta", PartialJSON: `{"query":`}})
	feed(&AnthropicStreamEvent{Type: "content_block_delta", Index: &idx0, Delta: &AnthropicDelta{Type: "input_json_delta", PartialJSON: `"streamed query"}`}})
	feed(&AnthropicStreamEvent{Type: "content_block_stop", Index: &idx0})

	// web_search_tool_result
	feed(&AnthropicStreamEvent{Type: "content_block_start", Index: &idx1, ContentBlock: &AnthropicContentBlock{
		Type: "web_search_tool_result", ToolUseID: "srvtoolu_ijd",
	}})
	feed(&AnthropicStreamEvent{Type: "content_block_stop", Index: &idx1})
	feed(&AnthropicStreamEvent{Type: "message_stop"})

	// Must NOT have any function_call_arguments.delta
	for _, e := range events {
		if e.Type == "response.function_call_arguments.delta" {
			t.Errorf("input_json_delta during web_search_call must not emit function_call_arguments.delta; got: %q", e.Delta)
		}
	}

	// Query should be extracted from accumulated input
	var completed *ResponsesStreamEvent
	for i := range events {
		if events[i].Type == "response.completed" {
			completed = &events[i]
		}
	}
	if completed == nil || completed.Response == nil {
		t.Fatal("response.completed not emitted")
	}
	foundWS := false
	for _, o := range completed.Response.Output {
		if o.Type == "web_search_call" {
			foundWS = true
			if o.Action == nil || o.Action.Query != "streamed query" {
				t.Errorf("web_search_call query should be extracted from streamed input; got: %+v", o.Action)
			}
		}
	}
	if !foundWS {
		t.Error("web_search_call not found in completed output")
	}
}

// TestStreaming_FinalizeFlushesHeldText verifies that if the stream ends
// abruptly after a pending status text without a following block, the text
// is flushed (not lost).
func TestStreaming_FinalizeFlushesHeldText(t *testing.T) {
	state := NewAnthropicEventToResponsesState()
	state.Model = "k3-256k"

	var events []ResponsesStreamEvent
	feed := func(evt *AnthropicStreamEvent) {
		events = append(events, AnthropicEventToResponsesEvents(evt, state)...)
	}

	idx0 := 0
	feed(&AnthropicStreamEvent{Type: "message_start", Message: &AnthropicResponse{ID: "msg_fin"}})
	feed(&AnthropicStreamEvent{Type: "content_block_start", Index: &idx0, ContentBlock: &AnthropicContentBlock{Type: "text"}})
	feed(&AnthropicStreamEvent{Type: "content_block_delta", Index: &idx0, Delta: &AnthropicDelta{Type: "text_delta", Text: "Search results for query: test"}})
	feed(&AnthropicStreamEvent{Type: "content_block_stop", Index: &idx0})

	// Stream ends abruptly (no more blocks, no message_stop)
	finalEvents := FinalizeAnthropicResponsesStream(state)
	events = append(events, finalEvents...)

	hasText := false
	for _, e := range events {
		if e.Type == "response.output_text.delta" && e.Delta == "Search results for query: test" {
			hasText = true
		}
	}
	if !hasText {
		t.Error("pending status text should be flushed when stream finalizes without a following web_search block")
	}
}

// TestStreaming_WebSearchWithThinking simulates the full Kimi response pattern:
// status_text → server_tool_use → web_search_tool_result → thinking → final_text
func TestStreaming_WebSearchWithThinking(t *testing.T) {
	state := NewAnthropicEventToResponsesState()
	state.Model = "k3-256k"

	var events []ResponsesStreamEvent
	feed := func(evt *AnthropicStreamEvent) {
		events = append(events, AnthropicEventToResponsesEvents(evt, state)...)
	}

	idx0, idx1, idx2, idx3, idx4 := 0, 1, 2, 3, 4
	feed(&AnthropicStreamEvent{Type: "message_start", Message: &AnthropicResponse{ID: "msg_full", Model: "k3-256k"}})

	// 0: status text
	feed(&AnthropicStreamEvent{Type: "content_block_start", Index: &idx0, ContentBlock: &AnthropicContentBlock{Type: "text"}})
	feed(&AnthropicStreamEvent{Type: "content_block_delta", Index: &idx0, Delta: &AnthropicDelta{Type: "text_delta", Text: "Search results for query: React hooks"}})
	feed(&AnthropicStreamEvent{Type: "content_block_stop", Index: &idx0})

	// 1: server_tool_use
	feed(&AnthropicStreamEvent{Type: "content_block_start", Index: &idx1, ContentBlock: &AnthropicContentBlock{
		Type: "server_tool_use", ID: "srvtoolu_react", Name: "web_search",
		Input: json.RawMessage(`{"query":"React hooks"}`),
	}})
	feed(&AnthropicStreamEvent{Type: "content_block_stop", Index: &idx1})

	// 2: web_search_tool_result
	feed(&AnthropicStreamEvent{Type: "content_block_start", Index: &idx2, ContentBlock: &AnthropicContentBlock{
		Type: "web_search_tool_result", ToolUseID: "srvtoolu_react",
	}})
	feed(&AnthropicStreamEvent{Type: "content_block_stop", Index: &idx2})

	// 3: thinking
	feed(&AnthropicStreamEvent{Type: "content_block_start", Index: &idx3, ContentBlock: &AnthropicContentBlock{Type: "thinking", Signature: "sig123"}})
	feed(&AnthropicStreamEvent{Type: "content_block_delta", Index: &idx3, Delta: &AnthropicDelta{Type: "thinking_delta", Thinking: "Let me explain hooks..."}})
	feed(&AnthropicStreamEvent{Type: "content_block_stop", Index: &idx3})

	// 4: final answer
	feed(&AnthropicStreamEvent{Type: "content_block_start", Index: &idx4, ContentBlock: &AnthropicContentBlock{Type: "text"}})
	feed(&AnthropicStreamEvent{Type: "content_block_delta", Index: &idx4, Delta: &AnthropicDelta{Type: "text_delta", Text: "React Hooks are functions..."}})
	feed(&AnthropicStreamEvent{Type: "content_block_stop", Index: &idx4})
	feed(&AnthropicStreamEvent{Type: "message_stop"})

	// Verify output structure in completed event
	var completed *ResponsesStreamEvent
	for i := range events {
		if events[i].Type == "response.completed" {
			completed = &events[i]
		}
	}
	if completed == nil || completed.Response == nil {
		t.Fatal("response.completed not emitted")
	}

	outputTypes := make(map[string]int)
	for _, o := range completed.Response.Output {
		outputTypes[o.Type]++
	}

	if outputTypes["web_search_call"] != 1 {
		t.Errorf("expected 1 web_search_call, got %d", outputTypes["web_search_call"])
	}
	if outputTypes["reasoning"] != 1 {
		t.Errorf("expected 1 reasoning, got %d", outputTypes["reasoning"])
	}
	if outputTypes["message"] != 1 {
		t.Errorf("expected 1 message, got %d", outputTypes["message"])
	}

	// Verify no status text leaked
	for _, e := range events {
		if e.Type == "response.output_text.delta" && e.Delta == "Search results for query: React hooks" {
			t.Error("search status text should not be emitted")
		}
	}
}

// TestStreaming_WebSearchMissingResult_ClosesFailed verifies that when the
// stream ends (message_stop) without a web_search_tool_result, the in-progress
// web_search_call is closed as failed with a consistent ID — never as an
// empty-ID completed item.
func TestStreaming_WebSearchMissingResult_ClosesFailed(t *testing.T) {
	state := NewAnthropicEventToResponsesState()
	state.Model = "k3-256k"

	var events []ResponsesStreamEvent
	feed := func(evt *AnthropicStreamEvent) {
		events = append(events, AnthropicEventToResponsesEvents(evt, state)...)
	}

	idx0 := 0
	feed(&AnthropicStreamEvent{Type: "message_start", Message: &AnthropicResponse{ID: "msg_miss"}})
	feed(&AnthropicStreamEvent{Type: "content_block_start", Index: &idx0, ContentBlock: &AnthropicContentBlock{
		Type: "server_tool_use", ID: "srvtoolu_miss", Name: "web_search",
		Input: json.RawMessage(`{"query":"lost query"}`),
	}})
	feed(&AnthropicStreamEvent{Type: "content_block_stop", Index: &idx0})
	// No web_search_tool_result; stream ends normally.
	feed(&AnthropicStreamEvent{Type: "message_delta", Delta: &AnthropicDelta{StopReason: "end_turn"}})
	feed(&AnthropicStreamEvent{Type: "message_stop"})

	var addedID string
	var doneItem *ResponsesOutput
	for i := range events {
		e := &events[i]
		if e.Type == "response.output_item.added" && e.Item != nil && e.Item.Type == "web_search_call" {
			addedID = e.Item.ID
		}
		if e.Type == "response.output_item.done" && e.Item != nil && e.Item.Type == "web_search_call" {
			doneItem = e.Item
		}
	}
	if addedID == "" {
		t.Fatal("web_search_call output_item.added missing")
	}
	if doneItem == nil {
		t.Fatal("web_search_call output_item.done missing on message_stop")
	}
	if doneItem.ID == "" {
		t.Error("web_search_call done must not have an empty ID")
	}
	if doneItem.ID != addedID {
		t.Errorf("added/done ID mismatch: added=%q done=%q", addedID, doneItem.ID)
	}
	if doneItem.Status == "completed" {
		t.Error("unfinished web_search_call must not be marked completed")
	}
	if doneItem.Status != "failed" {
		t.Errorf("unfinished web_search_call status = %q, want failed", doneItem.Status)
	}
	if doneItem.Action == nil || doneItem.Action.Query != "lost query" {
		t.Errorf("done item should carry the query, got %+v", doneItem.Action)
	}

	// Terminal output must contain the same coherent item.
	var completed *ResponsesStreamEvent
	for i := range events {
		if events[i].Type == "response.completed" {
			completed = &events[i]
		}
	}
	if completed == nil || completed.Response == nil {
		t.Fatal("response.completed not emitted")
	}
	foundWS := false
	for _, o := range completed.Response.Output {
		if o.Type == "web_search_call" {
			foundWS = true
			if o.ID != addedID || o.Status != "failed" || o.Action == nil {
				t.Errorf("terminal web_search_call item incoherent: %+v", o)
			}
		}
	}
	if !foundWS {
		t.Error("terminal output missing web_search_call")
	}
}

// TestStreaming_WebSearchMismatchedResult_DoesNotComplete verifies that a
// web_search_tool_result with a different tool_use_id neither completes the
// active search nor produces a second phantom item; the original search is
// closed as failed at message_stop.
func TestStreaming_WebSearchMismatchedResult_DoesNotComplete(t *testing.T) {
	state := NewAnthropicEventToResponsesState()
	state.Model = "k3-256k"

	var events []ResponsesStreamEvent
	feed := func(evt *AnthropicStreamEvent) {
		events = append(events, AnthropicEventToResponsesEvents(evt, state)...)
	}

	idx0, idx1 := 0, 1
	feed(&AnthropicStreamEvent{Type: "message_start", Message: &AnthropicResponse{ID: "msg_mism"}})
	feed(&AnthropicStreamEvent{Type: "content_block_start", Index: &idx0, ContentBlock: &AnthropicContentBlock{
		Type: "server_tool_use", ID: "srvtoolu_a", Name: "web_search",
		Input: json.RawMessage(`{"query":"query a"}`),
	}})
	feed(&AnthropicStreamEvent{Type: "content_block_stop", Index: &idx0})
	// Result for a DIFFERENT tool_use_id — must not complete srvtoolu_a.
	feed(&AnthropicStreamEvent{Type: "content_block_start", Index: &idx1, ContentBlock: &AnthropicContentBlock{
		Type: "web_search_tool_result", ToolUseID: "srvtoolu_b",
	}})
	feed(&AnthropicStreamEvent{Type: "content_block_stop", Index: &idx1})
	feed(&AnthropicStreamEvent{Type: "message_delta", Delta: &AnthropicDelta{StopReason: "end_turn"}})
	feed(&AnthropicStreamEvent{Type: "message_stop"})

	var addedID string
	var doneItems []*ResponsesOutput
	for i := range events {
		e := &events[i]
		if e.Type == "response.output_item.added" && e.Item != nil && e.Item.Type == "web_search_call" {
			addedID = e.Item.ID
		}
		if e.Type == "response.output_item.done" && e.Item != nil && e.Item.Type == "web_search_call" {
			doneItems = append(doneItems, e.Item)
		}
	}
	if addedID == "" {
		t.Fatal("web_search_call output_item.added missing")
	}
	if len(doneItems) != 1 {
		t.Fatalf("expected exactly 1 web_search_call done event, got %d", len(doneItems))
	}
	done := doneItems[0]
	if done.ID == "" || done.ID != addedID {
		t.Errorf("done ID mismatch: added=%q done=%q", addedID, done.ID)
	}
	if done.Status != "failed" {
		t.Errorf("mismatched result must not complete the search; status = %q, want failed", done.Status)
	}

	// Terminal output must have exactly one web_search_call, coherent with the stream.
	var completed *ResponsesStreamEvent
	for i := range events {
		if events[i].Type == "response.completed" {
			completed = &events[i]
		}
	}
	if completed == nil || completed.Response == nil {
		t.Fatal("response.completed not emitted")
	}
	wsCount := 0
	for _, o := range completed.Response.Output {
		if o.Type == "web_search_call" {
			wsCount++
			if o.ID != addedID || o.Status != "failed" {
				t.Errorf("terminal web_search_call item incoherent: %+v", o)
			}
		}
	}
	if wsCount != 1 {
		t.Errorf("expected exactly 1 web_search_call in terminal output, got %d", wsCount)
	}
}
