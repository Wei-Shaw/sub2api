package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAnthropicToResponsesResponse_PreservesServerWebSearchCall(t *testing.T) {
	resp := &AnthropicResponse{
		ID:    "msg_search",
		Model: "claude-sonnet-5",
		Content: []AnthropicContentBlock{
			{
				Type:  "server_tool_use",
				ID:    "srv_1",
				Name:  "web_search",
				Input: json.RawMessage(`{"query":"OpenCode Go pricing"}`),
			},
			{
				Type:      "web_search_tool_result",
				ToolUseID: "srv_1",
				Content:   json.RawMessage(`[]`),
			},
			{Type: "text", Text: "The current price is $10/month."},
		},
		StopReason: AnthropicStopReasonPtr("end_turn"),
	}

	out := AnthropicToResponsesResponse(resp)
	if len(out.Output) < 2 {
		t.Fatalf("output = %+v, want web_search_call and message", out.Output)
	}

	search := out.Output[0]
	if search.Type != "web_search_call" {
		t.Fatalf("output[0].type = %q, want web_search_call", search.Type)
	}
	if search.Status != "completed" {
		t.Errorf("search status = %q, want completed", search.Status)
	}
	if search.Action == nil || search.Action.Type != "search" || search.Action.Query != "OpenCode Go pricing" {
		t.Errorf("search action = %+v, want search query", search.Action)
	}
}

func TestAnthropicEventToResponses_ServerSearchDoesNotEmitFunctionArguments(t *testing.T) {
	state := NewAnthropicEventToResponsesState()
	var events []ResponsesStreamEvent
	feed := func(evt *AnthropicStreamEvent) {
		events = append(events, AnthropicEventToResponsesEvents(evt, state)...)
	}

	start := 0
	feed(&AnthropicStreamEvent{Type: "message_start", Message: &AnthropicResponse{ID: "msg_search", Model: "claude-sonnet-5"}})
	feed(&AnthropicStreamEvent{
		Type: "content_block_start", Index: &start,
		ContentBlock: &AnthropicContentBlock{
			Type: "server_tool_use", ID: "srv_1", Name: "web_search",
			Input: json.RawMessage(`{}`),
		},
	})
	feed(&AnthropicStreamEvent{
		Type: "content_block_delta", Index: &start,
		Delta: &AnthropicDelta{Type: "input_json_delta", PartialJSON: `{"query":"OpenCode `},
	})
	feed(&AnthropicStreamEvent{
		Type: "content_block_delta", Index: &start,
		Delta: &AnthropicDelta{Type: "input_json_delta", PartialJSON: `Go pricing"}`},
	})
	feed(&AnthropicStreamEvent{Type: "content_block_stop", Index: &start})

	result := 1
	feed(&AnthropicStreamEvent{
		Type: "content_block_start", Index: &result,
		ContentBlock: &AnthropicContentBlock{Type: "web_search_tool_result", ToolUseID: "srv_1", Content: json.RawMessage(`[]`)},
	})
	feed(&AnthropicStreamEvent{Type: "content_block_stop", Index: &result})
	feed(&AnthropicStreamEvent{Type: "message_stop"})

	var sawAdded, sawDone, sawCompleted, sawFunctionDelta bool
	for _, evt := range events {
		switch evt.Type {
		case "response.output_item.added":
			if evt.Item != nil && evt.Item.Type == "web_search_call" {
				sawAdded = true
			}
		case "response.output_item.done":
			if evt.Item != nil && evt.Item.Type == "web_search_call" {
				sawDone = true
				if evt.Item.Action == nil || evt.Item.Action.Query != "OpenCode Go pricing" {
					t.Errorf("completed search item = %+v, want query", evt.Item)
				}
			}
		case "response.function_call_arguments.delta":
			sawFunctionDelta = true
		case "response.completed":
			if evt.Response != nil && len(evt.Response.Output) == 1 && evt.Response.Output[0].Type == "web_search_call" {
				sawCompleted = true
			}
		}
	}
	if !sawAdded || !sawDone {
		t.Fatalf("events = %+v, want added/done web_search_call lifecycle", events)
	}
	if sawFunctionDelta {
		t.Fatalf("events = %+v, server search must not emit function_call_arguments.delta", events)
	}
	if !sawCompleted {
		t.Fatalf("events = %+v, response.completed must carry the web_search_call", events)
	}
}

func TestAnthropicEventToResponses_ServerSearchWithoutResultClosesBeforeNextItem(t *testing.T) {
	tests := []struct {
		name         string
		nextType     string
		wantNextType string
		delta        *AnthropicDelta
	}{
		{name: "text", nextType: "text", wantNextType: "message", delta: &AnthropicDelta{Type: "text_delta", Text: "answer"}},
		{name: "thinking", nextType: "thinking", wantNextType: "reasoning", delta: &AnthropicDelta{Type: "thinking_delta", Thinking: "reason"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := NewAnthropicEventToResponsesState()
			var events []ResponsesStreamEvent
			feed := func(evt *AnthropicStreamEvent) {
				events = append(events, AnthropicEventToResponsesEvents(evt, state)...)
			}

			searchIndex := 0
			feed(&AnthropicStreamEvent{Type: "message_start", Message: &AnthropicResponse{ID: "msg_missing_result", Model: "claude-sonnet-5"}})
			feed(&AnthropicStreamEvent{Type: "content_block_start", Index: &searchIndex, ContentBlock: &AnthropicContentBlock{
				Type: "server_tool_use", ID: "srv_missing", Name: "web_search", Input: json.RawMessage(`{}`),
			}})
			feed(&AnthropicStreamEvent{Type: "content_block_delta", Index: &searchIndex, Delta: &AnthropicDelta{
				Type: "input_json_delta", PartialJSON: `{"query":"missing result"}`,
			}})
			feed(&AnthropicStreamEvent{Type: "content_block_stop", Index: &searchIndex})

			nextIndex := 1
			feed(&AnthropicStreamEvent{Type: "content_block_start", Index: &nextIndex, ContentBlock: &AnthropicContentBlock{Type: tt.nextType}})
			feed(&AnthropicStreamEvent{Type: "content_block_delta", Index: &nextIndex, Delta: tt.delta})
			feed(&AnthropicStreamEvent{Type: "content_block_stop", Index: &nextIndex})
			feed(&AnthropicStreamEvent{Type: "message_stop"})

			var done []ResponsesStreamEvent
			var completed *ResponsesResponse
			for _, evt := range events {
				if evt.Type == "response.output_item.done" {
					done = append(done, evt)
				}
				if evt.Type == "response.completed" {
					completed = evt.Response
				}
			}
			require.Len(t, done, 2)
			require.Equal(t, 0, done[0].OutputIndex)
			require.Equal(t, "web_search_call", done[0].Item.Type)
			require.Equal(t, "missing result", done[0].Item.Action.Query)
			require.Equal(t, 1, done[1].OutputIndex)
			require.Equal(t, tt.wantNextType, done[1].Item.Type)
			require.NotNil(t, completed)
			require.Len(t, completed.Output, 2)
			require.Equal(t, "web_search_call", completed.Output[0].Type)
			require.Equal(t, tt.wantNextType, completed.Output[1].Type)
		})
	}
}

func TestAnthropicEventToResponses_LateServerSearchResultDoesNotDuplicateClosedItem(t *testing.T) {
	state := NewAnthropicEventToResponsesState()
	var events []ResponsesStreamEvent
	feed := func(evt *AnthropicStreamEvent) {
		events = append(events, AnthropicEventToResponsesEvents(evt, state)...)
	}

	searchIndex := 0
	feed(&AnthropicStreamEvent{Type: "message_start", Message: &AnthropicResponse{ID: "msg_late_result", Model: "claude-sonnet-5"}})
	feed(&AnthropicStreamEvent{Type: "content_block_start", Index: &searchIndex, ContentBlock: &AnthropicContentBlock{
		Type: "server_tool_use", ID: "srv_late", Name: "web_search", Input: json.RawMessage(`{}`),
	}})
	feed(&AnthropicStreamEvent{Type: "content_block_delta", Index: &searchIndex, Delta: &AnthropicDelta{
		Type: "input_json_delta", PartialJSON: `{"query":"late result"}`,
	}})
	feed(&AnthropicStreamEvent{Type: "content_block_stop", Index: &searchIndex})

	// Some Anthropic-compatible upstreams interleave a thinking block before the
	// matching web_search_tool_result. Opening that block closes the pending
	// Responses search item so the stream can continue.
	thinkingIndex := 1
	feed(&AnthropicStreamEvent{Type: "content_block_start", Index: &thinkingIndex, ContentBlock: &AnthropicContentBlock{Type: "thinking"}})
	feed(&AnthropicStreamEvent{Type: "content_block_delta", Index: &thinkingIndex, Delta: &AnthropicDelta{Type: "thinking_delta", Thinking: "checking sources"}})
	feed(&AnthropicStreamEvent{Type: "content_block_stop", Index: &thinkingIndex})

	// The late result belongs to the already-closed search. It must not create a
	// second web_search_call with the same id and an empty action.
	resultIndex := 2
	feed(&AnthropicStreamEvent{Type: "content_block_start", Index: &resultIndex, ContentBlock: &AnthropicContentBlock{
		Type: "web_search_tool_result", ToolUseID: "srv_late", Content: json.RawMessage(`[]`),
	}})
	feed(&AnthropicStreamEvent{Type: "content_block_stop", Index: &resultIndex})

	textIndex := 3
	feed(&AnthropicStreamEvent{Type: "content_block_start", Index: &textIndex, ContentBlock: &AnthropicContentBlock{Type: "text"}})
	feed(&AnthropicStreamEvent{Type: "content_block_delta", Index: &textIndex, Delta: &AnthropicDelta{Type: "text_delta", Text: "answer"}})
	feed(&AnthropicStreamEvent{Type: "content_block_stop", Index: &textIndex})
	feed(&AnthropicStreamEvent{Type: "message_stop"})

	searchDone := 0
	var completed *ResponsesResponse
	for _, evt := range events {
		if evt.Type == "response.output_item.done" && evt.Item != nil && evt.Item.Type == "web_search_call" {
			searchDone++
			require.NotNil(t, evt.Item.Action)
			require.Equal(t, "late result", evt.Item.Action.Query)
		}
		if evt.Type == "response.completed" {
			completed = evt.Response
		}
	}
	require.Equal(t, 1, searchDone, "late result must not emit a duplicate web_search_call")
	require.NotNil(t, completed)
	completedSearches := 0
	for _, output := range completed.Output {
		if output.Type == "web_search_call" {
			completedSearches++
		}
	}
	require.Equal(t, 1, completedSearches)
}

func TestAnthropicEventToResponses_LateServerSearchResultDoesNotStopOpenMessage(t *testing.T) {
	state := NewAnthropicEventToResponsesState()
	var events []ResponsesStreamEvent
	feed := func(evt *AnthropicStreamEvent) {
		events = append(events, AnthropicEventToResponsesEvents(evt, state)...)
	}

	searchIndex := 0
	feed(&AnthropicStreamEvent{Type: "message_start", Message: &AnthropicResponse{ID: "msg_open_message", Model: "claude-sonnet-5"}})
	feed(&AnthropicStreamEvent{Type: "content_block_start", Index: &searchIndex, ContentBlock: &AnthropicContentBlock{
		Type: "server_tool_use", ID: "srv_open_message", Name: "web_search", Input: json.RawMessage(`{}`),
	}})
	feed(&AnthropicStreamEvent{Type: "content_block_delta", Index: &searchIndex, Delta: &AnthropicDelta{
		Type: "input_json_delta", PartialJSON: `{"query":"open message"}`,
	}})
	feed(&AnthropicStreamEvent{Type: "content_block_stop", Index: &searchIndex})

	textIndex := 1
	feed(&AnthropicStreamEvent{Type: "content_block_start", Index: &textIndex, ContentBlock: &AnthropicContentBlock{Type: "text"}})
	feed(&AnthropicStreamEvent{Type: "content_block_delta", Index: &textIndex, Delta: &AnthropicDelta{Type: "text_delta", Text: "answer"}})
	feed(&AnthropicStreamEvent{Type: "content_block_stop", Index: &textIndex})

	// The message item remains open after a text block stop. A late duplicate
	// search result must consume only its own stop event, not close the message.
	resultIndex := 2
	feed(&AnthropicStreamEvent{Type: "content_block_start", Index: &resultIndex, ContentBlock: &AnthropicContentBlock{
		Type: "web_search_tool_result", ToolUseID: "srv_open_message", Content: json.RawMessage(`[]`),
	}})
	feed(&AnthropicStreamEvent{Type: "content_block_stop", Index: &resultIndex})
	feed(&AnthropicStreamEvent{Type: "message_stop"})

	var textDone []ResponsesStreamEvent
	var completed *ResponsesResponse
	for _, evt := range events {
		if evt.Type == "response.output_text.done" {
			textDone = append(textDone, evt)
		}
		if evt.Type == "response.completed" {
			completed = evt.Response
		}
	}
	require.Len(t, textDone, 1, "late result must not emit a second output_text.done")
	require.Equal(t, "answer", textDone[0].Text)
	require.NotNil(t, completed)
	require.Len(t, completed.Output, 2)
	require.Equal(t, "web_search_call", completed.Output[0].Type)
	require.Equal(t, "message", completed.Output[1].Type)
}

func TestAnthropicEventToResponses_LateSearchStopMatchesItsBlockIndex(t *testing.T) {
	state := NewAnthropicEventToResponsesState()
	var events []ResponsesStreamEvent
	feed := func(evt *AnthropicStreamEvent) {
		events = append(events, AnthropicEventToResponsesEvents(evt, state)...)
	}

	searchIndex := 0
	feed(&AnthropicStreamEvent{Type: "message_start", Message: &AnthropicResponse{ID: "msg_indexed_late", Model: "claude-sonnet-5"}})
	feed(&AnthropicStreamEvent{Type: "content_block_start", Index: &searchIndex, ContentBlock: &AnthropicContentBlock{
		Type: "server_tool_use", ID: "srv_indexed_late", Name: "web_search", Input: json.RawMessage(`{"query":"indexed late"}`),
	}})
	feed(&AnthropicStreamEvent{Type: "content_block_stop", Index: &searchIndex})

	textIndex := 1
	feed(&AnthropicStreamEvent{Type: "content_block_start", Index: &textIndex, ContentBlock: &AnthropicContentBlock{Type: "text"}})
	feed(&AnthropicStreamEvent{Type: "content_block_delta", Index: &textIndex, Delta: &AnthropicDelta{Type: "text_delta", Text: "answer"}})
	feed(&AnthropicStreamEvent{Type: "content_block_stop", Index: &textIndex})

	lateIndex := 2
	feed(&AnthropicStreamEvent{Type: "content_block_start", Index: &lateIndex, ContentBlock: &AnthropicContentBlock{
		Type: "web_search_tool_result", ToolUseID: "srv_indexed_late", Content: json.RawMessage(`[]`),
	}})
	// An unrelated stop arrives first. It must not consume the late-result
	// marker because its block index differs.
	feed(&AnthropicStreamEvent{Type: "content_block_stop", Index: &textIndex})
	feed(&AnthropicStreamEvent{Type: "content_block_stop", Index: &lateIndex})
	feed(&AnthropicStreamEvent{Type: "message_stop"})

	var textDone []ResponsesStreamEvent
	for _, evt := range events {
		if evt.Type == "response.output_text.done" {
			textDone = append(textDone, evt)
		}
	}
	require.Len(t, textDone, 2)
	require.Equal(t, "answer", textDone[0].Text)
	require.Equal(t, "", textDone[1].Text)
}

// TestAnthropicEventToResponses_TextEmitsContentPart pins that a message text
// stream emits response.content_part.added, and that it precedes the first
// output_text.delta for that part.
//
// Why: the OpenAI SDK's accumulating stream helper (client.responses.stream)
// only appends a content part to the message item when it sees
// content_part.added. The item is added with content: [], so a missing event
// makes the following output_text.delta index output.content[content_index] and
// raise IndexError. Raw event iteration does not accumulate, so a regression
// here is easy to miss.
func TestAnthropicEventToResponses_TextEmitsContentPart(t *testing.T) {
	state := NewAnthropicEventToResponsesState()
	state.Model = "claude-sonnet-4-5"

	var types []string
	feed := func(evt *AnthropicStreamEvent) {
		for _, out := range AnthropicEventToResponsesEvents(evt, state) {
			types = append(types, out.Type)
		}
	}

	idx := 0
	feed(&AnthropicStreamEvent{Type: "message_start", Message: &AnthropicResponse{ID: "msg_1", Model: "claude-sonnet-4-5"}})
	feed(&AnthropicStreamEvent{Type: "content_block_start", Index: &idx, ContentBlock: &AnthropicContentBlock{Type: "text"}})
	feed(&AnthropicStreamEvent{Type: "content_block_delta", Index: &idx, Delta: &AnthropicDelta{Type: "text_delta", Text: "Hel"}})
	feed(&AnthropicStreamEvent{Type: "content_block_delta", Index: &idx, Delta: &AnthropicDelta{Type: "text_delta", Text: "lo"}})
	feed(&AnthropicStreamEvent{Type: "content_block_stop", Index: &idx})
	feed(&AnthropicStreamEvent{Type: "message_stop"})

	posOf := func(target string) int {
		for i, ty := range types {
			if ty == target {
				return i
			}
		}
		return -1
	}

	partAdded := posOf("response.content_part.added")
	firstDelta := posOf("response.output_text.delta")

	if partAdded < 0 {
		t.Fatalf("response.content_part.added was not emitted; got %v", types)
	}
	if firstDelta < 0 {
		t.Fatalf("response.output_text.delta was not emitted; got %v", types)
	}
	if partAdded > firstDelta {
		t.Errorf("content_part.added must precede the first output_text.delta; got %v", types)
	}
	if posOf("response.content_part.done") < 0 {
		t.Errorf("response.content_part.done was not emitted; got %v", types)
	}
}

// TestAnthropicEventToResponses_DoneEventsCarryFullText pins that done events
// carry the part's full text (deltas carry increments only).
func TestAnthropicEventToResponses_DoneEventsCarryFullText(t *testing.T) {
	state := NewAnthropicEventToResponsesState()
	state.Model = "claude-sonnet-4-5"

	var events []ResponsesStreamEvent
	feed := func(evt *AnthropicStreamEvent) {
		events = append(events, AnthropicEventToResponsesEvents(evt, state)...)
	}

	idx := 0
	feed(&AnthropicStreamEvent{Type: "message_start", Message: &AnthropicResponse{ID: "msg_1"}})
	feed(&AnthropicStreamEvent{Type: "content_block_start", Index: &idx, ContentBlock: &AnthropicContentBlock{Type: "text"}})
	feed(&AnthropicStreamEvent{Type: "content_block_delta", Index: &idx, Delta: &AnthropicDelta{Type: "text_delta", Text: "Hello "}})
	feed(&AnthropicStreamEvent{Type: "content_block_delta", Index: &idx, Delta: &AnthropicDelta{Type: "text_delta", Text: "world"}})
	feed(&AnthropicStreamEvent{Type: "content_block_stop", Index: &idx})

	const want = "Hello world"
	var sawTextDone, sawPartDone bool
	for _, e := range events {
		switch e.Type {
		case "response.output_text.done":
			sawTextDone = true
			if e.Text != want {
				t.Errorf("output_text.done text = %q, want %q", e.Text, want)
			}
		case "response.content_part.done":
			sawPartDone = true
			if e.Part == nil || e.Part.Text != want {
				t.Errorf("content_part.done part = %+v, want text %q", e.Part, want)
			}
		}
	}
	if !sawTextDone || !sawPartDone {
		t.Errorf("missing done events: output_text.done=%v content_part.done=%v", sawTextDone, sawPartDone)
	}
}

// TestAnthropicEventToResponses_CompletedCarriesOutput pins that
// response.completed carries the full output list. The SDK's
// get_final_response() and tracing integrations parse the terminal event's
// response directly; an empty output leaves them with nothing (the text still
// renders from deltas, which is why this is invisible when only watching the
// stream).
func TestAnthropicEventToResponses_CompletedCarriesOutput(t *testing.T) {
	state := NewAnthropicEventToResponsesState()
	state.Model = "claude-sonnet-4-5"

	var events []ResponsesStreamEvent
	feed := func(evt *AnthropicStreamEvent) {
		events = append(events, AnthropicEventToResponsesEvents(evt, state)...)
	}

	idx := 0
	feed(&AnthropicStreamEvent{Type: "message_start", Message: &AnthropicResponse{ID: "msg_1"}})
	feed(&AnthropicStreamEvent{Type: "content_block_start", Index: &idx, ContentBlock: &AnthropicContentBlock{Type: "text"}})
	feed(&AnthropicStreamEvent{Type: "content_block_delta", Index: &idx, Delta: &AnthropicDelta{Type: "text_delta", Text: "4826"}})
	feed(&AnthropicStreamEvent{Type: "content_block_stop", Index: &idx})
	feed(&AnthropicStreamEvent{Type: "message_stop"})

	var completed *ResponsesStreamEvent
	for i := range events {
		if events[i].Type == "response.completed" {
			completed = &events[i]
		}
	}
	if completed == nil || completed.Response == nil {
		t.Fatalf("response.completed was not emitted")
	}
	if len(completed.Response.Output) == 0 {
		t.Fatalf("response.completed carries an empty output; clients would see no result")
	}
	msg := completed.Response.Output[0]
	if msg.Type != "message" || len(msg.Content) == 0 {
		t.Fatalf("output[0] = %+v, want a message with content", msg)
	}
	if msg.Content[0].Text != "4826" {
		t.Errorf("output[0].content[0].text = %q, want %q", msg.Content[0].Text, "4826")
	}
}

// TestAnthropicEventToResponses_ToolCallCompletedCarriesArguments pins that a
// function call's accumulated arguments survive into output_item.done and
// response.completed.
func TestAnthropicEventToResponses_ToolCallCompletedCarriesArguments(t *testing.T) {
	state := NewAnthropicEventToResponsesState()
	state.Model = "claude-sonnet-4-5"

	var events []ResponsesStreamEvent
	feed := func(evt *AnthropicStreamEvent) {
		events = append(events, AnthropicEventToResponsesEvents(evt, state)...)
	}

	idx := 0
	feed(&AnthropicStreamEvent{Type: "message_start", Message: &AnthropicResponse{ID: "msg_1"}})
	feed(&AnthropicStreamEvent{Type: "content_block_start", Index: &idx, ContentBlock: &AnthropicContentBlock{
		Type: "tool_use", ID: "toolu_1", Name: "get_weather",
	}})
	feed(&AnthropicStreamEvent{Type: "content_block_delta", Index: &idx, Delta: &AnthropicDelta{
		Type: "input_json_delta", PartialJSON: `{"city":`,
	}})
	feed(&AnthropicStreamEvent{Type: "content_block_delta", Index: &idx, Delta: &AnthropicDelta{
		Type: "input_json_delta", PartialJSON: `"SH"}`,
	}})
	feed(&AnthropicStreamEvent{Type: "content_block_stop", Index: &idx})
	feed(&AnthropicStreamEvent{Type: "message_stop"})

	var completed *ResponsesStreamEvent
	for i := range events {
		if events[i].Type == "response.completed" {
			completed = &events[i]
		}
	}
	if completed == nil || completed.Response == nil || len(completed.Response.Output) == 0 {
		t.Fatalf("response.completed carries no output")
	}
	fc := completed.Response.Output[0]
	if fc.Type != "function_call" {
		t.Fatalf("output[0].type = %q, want function_call", fc.Type)
	}
	if fc.Arguments != `{"city":"SH"}` {
		t.Errorf("arguments = %q, want %q", fc.Arguments, `{"city":"SH"}`)
	}
	if fc.Name != "get_weather" {
		t.Errorf("name = %q, want get_weather", fc.Name)
	}
}
