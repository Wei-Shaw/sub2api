package apicompat

import "testing"

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

// TestAnthropicEventToResponses_ThinkingAfterTextKeepsMessageOutput pins that a
// thinking block arriving after a text block closes the open message item
// instead of silently replacing it.
//
// Why: a message item is deliberately left open when its text block stops
// (more text blocks may follow in the same item). content_block_start therefore
// has to close whatever is open before starting a new item — tool_use already
// did, thinking did not, so it overwrote CurrentItemType/CurrentItemID and the
// accumulated CurrentContent never reached state.Outputs. response.completed
// then carried only the reasoning item and the client saw a successful response
// with no assistant text. Interleaved thinking (anthropic-beta
// interleaved-thinking-2025-05-14, forwarded by this gateway) produces exactly
// this text → thinking ordering.
func TestAnthropicEventToResponses_ThinkingAfterTextKeepsMessageOutput(t *testing.T) {
	state := NewAnthropicEventToResponsesState()
	state.Model = "claude-sonnet-4-5"

	var events []ResponsesStreamEvent
	feed := func(evt *AnthropicStreamEvent) {
		events = append(events, AnthropicEventToResponsesEvents(evt, state)...)
	}

	i0, i1 := 0, 1
	feed(&AnthropicStreamEvent{Type: "message_start", Message: &AnthropicResponse{ID: "msg_1"}})
	feed(&AnthropicStreamEvent{Type: "content_block_start", Index: &i0, ContentBlock: &AnthropicContentBlock{Type: "text"}})
	feed(&AnthropicStreamEvent{Type: "content_block_delta", Index: &i0, Delta: &AnthropicDelta{Type: "text_delta", Text: "answer"}})
	feed(&AnthropicStreamEvent{Type: "content_block_stop", Index: &i0})
	feed(&AnthropicStreamEvent{Type: "content_block_start", Index: &i1, ContentBlock: &AnthropicContentBlock{Type: "thinking"}})
	feed(&AnthropicStreamEvent{Type: "content_block_delta", Index: &i1, Delta: &AnthropicDelta{Type: "thinking_delta", Thinking: "hmm"}})
	feed(&AnthropicStreamEvent{Type: "content_block_stop", Index: &i1})
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
	outputs := completed.Response.Output
	if len(outputs) != 2 {
		t.Fatalf("response.completed carries %d output items, want 2 (message + reasoning): %+v", len(outputs), outputs)
	}
	if outputs[0].Type != "message" {
		t.Fatalf("output[0].type = %q, want message", outputs[0].Type)
	}
	if len(outputs[0].Content) != 1 || outputs[0].Content[0].Text != "answer" {
		t.Errorf("assistant text lost: output[0].content = %+v", outputs[0].Content)
	}
	if outputs[1].Type != "reasoning" {
		t.Errorf("output[1].type = %q, want reasoning", outputs[1].Type)
	}

	// The two items must also occupy distinct output_index values; sharing one
	// index is how the reasoning item used to land on top of the message item.
	var addedIndexes []int
	for _, evt := range events {
		if evt.Type == "response.output_item.added" {
			addedIndexes = append(addedIndexes, evt.OutputIndex)
		}
	}
	if len(addedIndexes) != 2 || addedIndexes[0] == addedIndexes[1] {
		t.Errorf("output_item.added indexes = %v, want two distinct values", addedIndexes)
	}
}

// TestAnthropicEventToResponses_MultipleTextBlocksAdvanceContentIndex pins that
// each text block within one message item gets its own content_index.
//
// Why: content_index was assigned when the item opened and never advanced, so a
// second text block re-emitted content_part.added at index 0. The OpenAI SDK's
// accumulating stream helper writes parts at output.content[content_index], so
// the second part overwrote the first and the earlier text disappeared from the
// accumulated response.
func TestAnthropicEventToResponses_MultipleTextBlocksAdvanceContentIndex(t *testing.T) {
	state := NewAnthropicEventToResponsesState()
	state.Model = "claude-sonnet-4-5"

	var events []ResponsesStreamEvent
	feed := func(evt *AnthropicStreamEvent) {
		events = append(events, AnthropicEventToResponsesEvents(evt, state)...)
	}

	i0, i1 := 0, 1
	feed(&AnthropicStreamEvent{Type: "message_start", Message: &AnthropicResponse{ID: "msg_1"}})
	feed(&AnthropicStreamEvent{Type: "content_block_start", Index: &i0, ContentBlock: &AnthropicContentBlock{Type: "text"}})
	feed(&AnthropicStreamEvent{Type: "content_block_delta", Index: &i0, Delta: &AnthropicDelta{Type: "text_delta", Text: "first"}})
	feed(&AnthropicStreamEvent{Type: "content_block_stop", Index: &i0})
	feed(&AnthropicStreamEvent{Type: "content_block_start", Index: &i1, ContentBlock: &AnthropicContentBlock{Type: "text"}})
	feed(&AnthropicStreamEvent{Type: "content_block_delta", Index: &i1, Delta: &AnthropicDelta{Type: "text_delta", Text: "second"}})
	feed(&AnthropicStreamEvent{Type: "content_block_stop", Index: &i1})
	feed(&AnthropicStreamEvent{Type: "message_stop"})

	var partAdded []int
	for _, evt := range events {
		if evt.Type == "response.content_part.added" {
			partAdded = append(partAdded, evt.ContentIndex)
		}
	}
	if len(partAdded) != 2 {
		t.Fatalf("content_part.added emitted %d times, want 2", len(partAdded))
	}
	if partAdded[0] != 0 || partAdded[1] != 1 {
		t.Errorf("content_part.added indexes = %v, want [0 1]", partAdded)
	}

	// Every delta/done for a part must carry that part's index, otherwise the
	// SDK appends the text to the wrong slot.
	byIndex := map[int]string{}
	for _, evt := range events {
		if evt.Type == "response.output_text.delta" {
			byIndex[evt.ContentIndex] += evt.Delta
		}
	}
	if byIndex[0] != "first" || byIndex[1] != "second" {
		t.Errorf("output_text.delta grouped by content_index = %v, want {0:first 1:second}", byIndex)
	}
}

// TestAnthropicEventToResponses_ItemLifecycleIsBalanced is the invariant behind
// both regressions above: the converter must never leave an item that was
// announced with response.output_item.added without a matching
// response.output_item.done, and the terminal event must carry one output entry
// per announced item. Any future block type that forgets to close the open item
// breaks this, whatever the symptom looks like.
func TestAnthropicEventToResponses_ItemLifecycleIsBalanced(t *testing.T) {
	for _, tc := range []struct {
		name   string
		blocks []*AnthropicContentBlock
	}{
		{"text then thinking", []*AnthropicContentBlock{{Type: "text"}, {Type: "thinking"}}},
		{"thinking then text", []*AnthropicContentBlock{{Type: "thinking"}, {Type: "text"}}},
		{"text then tool_use", []*AnthropicContentBlock{{Type: "text"}, {Type: "tool_use", ID: "toolu_1", Name: "t"}}},
		{"text thinking text", []*AnthropicContentBlock{{Type: "text"}, {Type: "thinking"}, {Type: "text"}}},
		{"thinking text tool_use", []*AnthropicContentBlock{{Type: "thinking"}, {Type: "text"}, {Type: "tool_use", ID: "toolu_2", Name: "t"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := NewAnthropicEventToResponsesState()
			state.Model = "claude-sonnet-4-5"

			var events []ResponsesStreamEvent
			feed := func(evt *AnthropicStreamEvent) {
				events = append(events, AnthropicEventToResponsesEvents(evt, state)...)
			}

			feed(&AnthropicStreamEvent{Type: "message_start", Message: &AnthropicResponse{ID: "msg_1"}})
			for i, block := range tc.blocks {
				idx := i
				feed(&AnthropicStreamEvent{Type: "content_block_start", Index: &idx, ContentBlock: block})
				switch block.Type {
				case "text":
					feed(&AnthropicStreamEvent{Type: "content_block_delta", Index: &idx, Delta: &AnthropicDelta{Type: "text_delta", Text: "t"}})
				case "thinking":
					feed(&AnthropicStreamEvent{Type: "content_block_delta", Index: &idx, Delta: &AnthropicDelta{Type: "thinking_delta", Thinking: "r"}})
				case "tool_use":
					feed(&AnthropicStreamEvent{Type: "content_block_delta", Index: &idx, Delta: &AnthropicDelta{Type: "input_json_delta", PartialJSON: "{}"}})
				}
				feed(&AnthropicStreamEvent{Type: "content_block_stop", Index: &idx})
			}
			feed(&AnthropicStreamEvent{Type: "message_stop"})

			openIDs := map[string]int{}
			var order []string
			for _, evt := range events {
				switch evt.Type {
				case "response.output_item.added":
					if evt.Item == nil {
						t.Fatalf("output_item.added without item")
					}
					openIDs[evt.Item.ID]++
					order = append(order, evt.Item.ID)
				case "response.output_item.done":
					if evt.Item == nil {
						t.Fatalf("output_item.done without item")
					}
					openIDs[evt.Item.ID]--
				}
			}
			for id, n := range openIDs {
				if n != 0 {
					t.Errorf("item %s: output_item.added/done imbalance %+d", id, n)
				}
			}

			var completed *ResponsesStreamEvent
			for i := range events {
				if events[i].Type == "response.completed" {
					completed = &events[i]
				}
			}
			if completed == nil || completed.Response == nil {
				t.Fatalf("response.completed was not emitted")
			}
			if len(completed.Response.Output) != len(order) {
				t.Fatalf("response.completed carries %d outputs, want %d (one per announced item)",
					len(completed.Response.Output), len(order))
			}
			for i, id := range order {
				if completed.Response.Output[i].ID != id {
					t.Errorf("output[%d].id = %q, want %q (announcement order must be preserved)",
						i, completed.Response.Output[i].ID, id)
				}
			}
		})
	}
}
