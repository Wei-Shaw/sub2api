package apicompat

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Strict Responses clients track reasoning summary parts by summary_index.
// Text events must refer to an announced part, and terminal reasoning items
// must retain the summary array even when thinking has no visible text.

func feedAnthropic(t *testing.T, model string, events ...*AnthropicStreamEvent) []ResponsesStreamEvent {
	t.Helper()
	state := NewAnthropicEventToResponsesState()
	state.Model = model
	var out []ResponsesStreamEvent
	for _, evt := range events {
		out = append(out, AnthropicEventToResponsesEvents(evt, state)...)
	}
	return out
}

func eventTypes(events []ResponsesStreamEvent) []string {
	types := make([]string, 0, len(events))
	for _, evt := range events {
		types = append(types, evt.Type)
	}
	return types
}

func indexOfType(types []string, target string) int {
	for i, ty := range types {
		if ty == target {
			return i
		}
	}
	return -1
}

func TestAnthropicEventToResponses_ThinkingTextIsInAnAnnouncedSummaryPart(t *testing.T) {
	i0 := 0
	events := feedAnthropic(t, "claude-sonnet-4-5",
		&AnthropicStreamEvent{Type: "message_start", Message: &AnthropicResponse{ID: "msg_1"}},
		&AnthropicStreamEvent{Type: "content_block_start", Index: &i0, ContentBlock: &AnthropicContentBlock{Type: "thinking"}},
		&AnthropicStreamEvent{Type: "content_block_delta", Index: &i0, Delta: &AnthropicDelta{Type: "thinking_delta", Thinking: "think"}},
		&AnthropicStreamEvent{Type: "content_block_delta", Index: &i0, Delta: &AnthropicDelta{Type: "thinking_delta", Thinking: "ing"}},
		&AnthropicStreamEvent{Type: "content_block_stop", Index: &i0},
		&AnthropicStreamEvent{Type: "message_stop"},
	)
	types := eventTypes(events)
	added := indexOfType(types, "response.reasoning_summary_part.added")
	firstDelta := indexOfType(types, "response.reasoning_summary_text.delta")
	textDone := indexOfType(types, "response.reasoning_summary_text.done")
	partDone := indexOfType(types, "response.reasoning_summary_part.done")
	itemDone := indexOfType(types, "response.output_item.done")
	if added < 0 || firstDelta < 0 || textDone < 0 || partDone < 0 || itemDone < 0 {
		t.Fatalf("missing reasoning summary events: %v", types)
	}
	if added > firstDelta || firstDelta > textDone || textDone > partDone || partDone > itemDone {
		t.Fatalf("reasoning summary events out of order: %v", types)
	}
	if got := events[textDone].Text; got != "thinking" {
		t.Errorf("reasoning_summary_text.done text = %q, want the whole summary %q", got, "thinking")
	}
	if events[partDone].Part == nil || events[partDone].Part.Text != "thinking" {
		t.Errorf("reasoning_summary_part.done part = %+v, want the whole summary", events[partDone].Part)
	}
	if summary := events[itemDone].Item.Summary; len(summary) != 1 || summary[0].Text != "thinking" {
		t.Errorf("output_item.done summary = %+v, want one part with the whole summary", summary)
	}
}

// Gemini's thinking often arrives with no text at all (only a signature): no summary part is announced, none is said done,
// and the item's summary is an empty list.
func TestAnthropicEventToResponses_EmptyThinkingHasNoSummaryEvents(t *testing.T) {
	i0, i1 := 0, 1
	events := feedAnthropic(t, "gemini-2.5-flash",
		&AnthropicStreamEvent{Type: "message_start", Message: &AnthropicResponse{ID: "msg_1", Model: "gemini-2.5-flash"}},
		&AnthropicStreamEvent{Type: "content_block_start", Index: &i0, ContentBlock: &AnthropicContentBlock{Type: "thinking"}},
		&AnthropicStreamEvent{Type: "content_block_stop", Index: &i0},
		&AnthropicStreamEvent{Type: "content_block_start", Index: &i1, ContentBlock: &AnthropicContentBlock{Type: "tool_use", ID: "toolu_1", Name: "workspace_read"}},
		&AnthropicStreamEvent{Type: "content_block_delta", Index: &i1, Delta: &AnthropicDelta{Type: "input_json_delta", PartialJSON: `{"path":"README.md"}`}},
		&AnthropicStreamEvent{Type: "content_block_stop", Index: &i1},
		&AnthropicStreamEvent{Type: "message_stop"},
	)
	for _, evt := range events {
		switch evt.Type {
		case "response.reasoning_summary_part.added", "response.reasoning_summary_text.delta",
			"response.reasoning_summary_text.done", "response.reasoning_summary_part.done":
			t.Fatalf("an empty thinking block must not emit %s: %v", evt.Type, eventTypes(events))
		}
	}
}

// The completed response lists every reasoning item with its summary, an empty list when there is none: a strict client checks
// the final output against what was streamed and needs the field.
func TestResponsesOutput_ReasoningAlwaysCarriesSummary(t *testing.T) {
	raw, err := json.Marshal(ResponsesOutput{Type: "reasoning", ID: "rs_1", Status: "completed"})
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	summary, ok := decoded["summary"].([]any)
	if !ok || len(summary) != 0 {
		t.Fatalf("reasoning output %s: summary = %#v, want []", raw, decoded["summary"])
	}
	if decoded["id"] != "rs_1" || decoded["status"] != "completed" {
		t.Errorf("other fields lost: %s", raw)
	}

	withText, _ := json.Marshal(ResponsesOutput{Type: "reasoning", ID: "rs_2", Summary: []ResponsesSummary{{Type: "summary_text", Text: "t"}}})
	if err := json.Unmarshal(withText, &decoded); err != nil {
		t.Fatal(err)
	}
	if s, _ := decoded["summary"].([]any); len(s) != 1 {
		t.Errorf("summary with text lost: %s", withText)
	}

	// Non-reasoning output keeps its existing wire shape.
	message, err := json.Marshal(ResponsesOutput{Type: "message", ID: "m", Role: "assistant"})
	require.NoError(t, err)
	require.JSONEq(t, `{"type":"message","id":"m","role":"assistant"}`, string(message))
}

func TestAnthropicEventToResponses_ReasoningSummaryClosesWithItem(t *testing.T) {
	i0, i1 := 0, 1
	for _, tc := range []struct {
		name string
		end  []*AnthropicStreamEvent
	}{
		{"block stop", []*AnthropicStreamEvent{{Type: "content_block_stop", Index: &i0}, {Type: "message_stop"}}},
		{"message stop without block stop", []*AnthropicStreamEvent{{Type: "message_stop"}}},
		{"next block without block stop", []*AnthropicStreamEvent{
			{Type: "content_block_start", Index: &i1, ContentBlock: &AnthropicContentBlock{Type: "tool_use", ID: "toolu_1", Name: "read"}},
			{Type: "content_block_stop", Index: &i1}, {Type: "message_stop"},
		}},
		{"stream finalization", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := NewAnthropicEventToResponsesState()
			state.Model = "gemini-2.5-flash"
			input := []*AnthropicStreamEvent{
				{Type: "message_start", Message: &AnthropicResponse{ID: "msg_1"}},
				{Type: "content_block_start", Index: &i0, ContentBlock: &AnthropicContentBlock{Type: "thinking", Thinking: "先"}},
				{Type: "content_block_delta", Index: &i0, Delta: &AnthropicDelta{Type: "thinking_delta", Thinking: "检查"}},
			}
			var events []ResponsesStreamEvent
			for _, evt := range append(input, tc.end...) {
				events = append(events, AnthropicEventToResponsesEvents(evt, state)...)
			}
			events = append(events, FinalizeAnthropicResponsesStream(state)...)
			require.Empty(t, FinalizeAnthropicResponsesStream(state), "finalizing again must not duplicate events")

			var types []string
			var text string
			lastSequence := -1
			for _, evt := range events {
				require.Greater(t, evt.SequenceNumber, lastSequence)
				lastSequence = evt.SequenceNumber
				if strings.HasPrefix(evt.Type, "response.reasoning_summary_") {
					types = append(types, evt.Type)
					require.Equal(t, 0, evt.OutputIndex)
					require.Equal(t, 0, evt.SummaryIndex)
					raw, err := json.Marshal(evt)
					require.NoError(t, err)
					var wire map[string]json.RawMessage
					require.NoError(t, json.Unmarshal(raw, &wire))
					require.JSONEq(t, "0", string(wire["output_index"]))
					require.JSONEq(t, "0", string(wire["summary_index"]))
					require.NotEmpty(t, evt.ItemID)
					require.Contains(t, wire, "item_id")
					require.Contains(t, wire, "sequence_number")
				}
				switch evt.Type {
				case "response.reasoning_summary_text.delta":
					text += evt.Delta
				case "response.reasoning_summary_text.done":
					require.Equal(t, "先检查", evt.Text)
				case "response.reasoning_summary_part.done":
					require.NotNil(t, evt.Part)
					require.Equal(t, "先检查", evt.Part.Text)
				case "response.output_item.done":
					if evt.Item.Type == "reasoning" {
						types = append(types, evt.Type)
						require.Equal(t, []ResponsesSummary{{Type: "summary_text", Text: "先检查"}}, evt.Item.Summary)
					}
				}
			}
			require.Equal(t, "先检查", text, "block-start text must be streamed as well as deltas")
			require.Equal(t, []string{
				"response.reasoning_summary_part.added", "response.reasoning_summary_text.delta", "response.reasoning_summary_text.delta",
				"response.reasoning_summary_text.done", "response.reasoning_summary_part.done", "response.output_item.done",
			}, types)
		})
	}
}

func TestAnthropicEventToResponses_SeparateThinkingSummaries(t *testing.T) {
	i0, i1, i2 := 0, 1, 2
	events := feedAnthropic(t, "gemini-2.5-flash",
		&AnthropicStreamEvent{Type: "message_start", Message: &AnthropicResponse{ID: "msg_1"}},
		&AnthropicStreamEvent{Type: "content_block_start", Index: &i0, ContentBlock: &AnthropicContentBlock{Type: "thinking", Thinking: "first"}},
		&AnthropicStreamEvent{Type: "content_block_stop", Index: &i0},
		&AnthropicStreamEvent{Type: "content_block_start", Index: &i1, ContentBlock: &AnthropicContentBlock{Type: "thinking"}},
		&AnthropicStreamEvent{Type: "content_block_delta", Index: &i1, Delta: &AnthropicDelta{Type: "thinking_delta"}},
		&AnthropicStreamEvent{Type: "content_block_stop", Index: &i1},
		&AnthropicStreamEvent{Type: "content_block_start", Index: &i2, ContentBlock: &AnthropicContentBlock{Type: "thinking"}},
		&AnthropicStreamEvent{Type: "content_block_delta", Index: &i2, Delta: &AnthropicDelta{Type: "thinking_delta", Thinking: "second"}},
		&AnthropicStreamEvent{Type: "content_block_stop", Index: &i2},
		&AnthropicStreamEvent{Type: "message_stop"},
	)
	var indexes []int
	var texts, itemIDs []string
	for _, evt := range events {
		switch evt.Type {
		case "response.reasoning_summary_part.added":
			indexes = append(indexes, evt.OutputIndex)
			itemIDs = append(itemIDs, evt.ItemID)
			require.Equal(t, 0, evt.SummaryIndex)
		case "response.reasoning_summary_text.done":
			texts = append(texts, evt.Text)
		case "response.completed":
			require.Len(t, evt.Response.Output, 3)
			require.Equal(t, []ResponsesSummary{{Type: "summary_text", Text: "first"}}, evt.Response.Output[0].Summary)
			require.Empty(t, evt.Response.Output[1].Summary)
			require.Equal(t, []ResponsesSummary{{Type: "summary_text", Text: "second"}}, evt.Response.Output[2].Summary)
		}
	}
	require.Equal(t, []int{0, 2}, indexes, "an empty thinking block must not inherit its predecessor's summary part")
	require.Equal(t, []string{"first", "second"}, texts)
	require.Len(t, itemIDs, 2)
	require.NotEmpty(t, itemIDs[0])
	require.NotEqual(t, itemIDs[0], itemIDs[1])
}

func TestAnthropicEventToResponses_SignatureOnlyThinkingRoundTrip(t *testing.T) {
	i0, i1 := 0, 1
	const model = "claude-opus-5-5"
	events := feedAnthropic(t, model,
		&AnthropicStreamEvent{Type: "message_start", Message: &AnthropicResponse{ID: "msg_1", Model: model}},
		&AnthropicStreamEvent{Type: "content_block_start", Index: &i0, ContentBlock: &AnthropicContentBlock{Type: "thinking"}},
		&AnthropicStreamEvent{Type: "content_block_delta", Index: &i0, Delta: &AnthropicDelta{Type: "signature_delta", Signature: "opaque-"}},
		&AnthropicStreamEvent{Type: "content_block_delta", Index: &i0, Delta: &AnthropicDelta{Type: "signature_delta", Signature: "signature"}},
		&AnthropicStreamEvent{Type: "content_block_stop", Index: &i0},
		&AnthropicStreamEvent{Type: "content_block_start", Index: &i1, ContentBlock: &AnthropicContentBlock{Type: "tool_use", ID: "toolu_1", Name: "lookup", Input: json.RawMessage(`{}`)}},
		&AnthropicStreamEvent{Type: "content_block_stop", Index: &i1},
		&AnthropicStreamEvent{Type: "message_stop"},
	)
	var completed *ResponsesResponse
	for _, evt := range events {
		require.False(t, strings.HasPrefix(evt.Type, "response.reasoning_summary_"), "signature-only thinking has no visible summary")
		if evt.Type == "response.completed" {
			completed = evt.Response
		}
	}
	require.NotNil(t, completed)
	require.Len(t, completed.Output, 2)
	require.NotEmpty(t, completed.Output[0].EncryptedContent)

	// Inspect the actual terminal wire form, where omitempty used to lose [].
	raw, err := json.Marshal(completed)
	require.NoError(t, err)
	var wire struct {
		Output []map[string]json.RawMessage `json:"output"`
	}
	require.NoError(t, json.Unmarshal(raw, &wire))
	require.JSONEq(t, "[]", string(wire.Output[0]["summary"]))
	require.NotContains(t, string(raw), "opaque-signature", "the signature must not become visible summary text")

	// A following tool-result request must recover the original signed block.
	raw, err = json.Marshal(completed.Output)
	require.NoError(t, err)
	var items []ResponsesInputItem
	require.NoError(t, json.Unmarshal(raw, &items))
	items = append(items, ResponsesInputItem{Type: "function_call_output", CallID: completed.Output[1].CallID, Output: "ok"})
	raw, err = json.Marshal(items)
	require.NoError(t, err)
	converted, err := ResponsesToAnthropicRequest(&ResponsesRequest{Model: model, Input: raw})
	require.NoError(t, err)
	require.Len(t, converted.Messages, 2)
	var blocks []AnthropicContentBlock
	require.NoError(t, json.Unmarshal(converted.Messages[0].Content, &blocks))
	require.Len(t, blocks, 2)
	require.Equal(t, AnthropicContentBlock{Type: "thinking", Signature: "opaque-signature"}, blocks[0])
	require.Equal(t, "lookup", blocks[1].Name)
}
