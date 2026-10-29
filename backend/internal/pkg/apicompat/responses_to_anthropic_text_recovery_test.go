package apicompat

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// feedResponsesEvents converts a whole event sequence with a single state.
func feedResponsesEvents(events ...*ResponsesStreamEvent) []AnthropicStreamEvent {
	state := NewResponsesEventToAnthropicState()
	var out []AnthropicStreamEvent
	for _, evt := range events {
		out = append(out, ResponsesEventToAnthropicEvents(evt, state)...)
	}
	return out
}

// collectAnthropicText accumulates the visible assistant text a Claude client
// reconstructs from a converted event stream.
func collectAnthropicText(events []AnthropicStreamEvent) string {
	text := ""
	for _, event := range events {
		if event.Type == "content_block_delta" && event.Delta != nil && event.Delta.Type == "text_delta" {
			text += event.Delta.Text
		}
	}
	return text
}

// requireAnthropicBlockLifecycle checks the block framing a strict client
// relies on: indices start at zero and increase by one, every block is closed,
// and no delta arrives outside an open block.
func requireAnthropicBlockLifecycle(t *testing.T, events []AnthropicStreamEvent) {
	t.Helper()

	open := false
	next := 0
	for _, event := range events {
		switch event.Type {
		case "content_block_start":
			require.False(t, open, "content_block_start while a block is open")
			require.NotNil(t, event.Index)
			require.Equal(t, next, *event.Index, "content block indices must increase by one")
			open = true
		case "content_block_delta":
			require.True(t, open, "content_block_delta outside an open block")
			require.NotNil(t, event.Index)
			require.Equal(t, next, *event.Index)
		case "content_block_stop":
			require.True(t, open, "content_block_stop without an open block")
			require.NotNil(t, event.Index)
			require.Equal(t, next, *event.Index)
			open = false
			next++
		}
	}
	require.False(t, open, "stream ended with an unclosed content block")
}

func responsesCreated() *ResponsesStreamEvent {
	return &ResponsesStreamEvent{
		Type:     "response.created",
		Response: &ResponsesResponse{ID: "resp_text_recovery", Model: "gpt-5.2"},
	}
}

func responsesMessageOutput(texts ...string) []ResponsesOutput {
	content := make([]ResponsesContentPart, 0, len(texts))
	for _, text := range texts {
		content = append(content, ResponsesContentPart{Type: "output_text", Text: text})
	}
	return []ResponsesOutput{{Type: "message", Role: "assistant", Content: content}}
}

// Some streams carry the finished text only on response.output_text.done and
// send no output_text.delta at all. Dropping it leaves a protocol-valid message
// with no text, which clients report as an empty response.
func TestResponsesEventToAnthropicEvents_RecoversTextFromDoneEvent(t *testing.T) {
	const answer = "recovered from done"

	events := feedResponsesEvents(
		responsesCreated(),
		&ResponsesStreamEvent{Type: "response.output_item.added", Item: &ResponsesOutput{Type: "message"}},
		&ResponsesStreamEvent{Type: "response.output_text.done", Text: answer},
		&ResponsesStreamEvent{Type: "response.completed", Response: &ResponsesResponse{Status: "completed"}},
	)

	assert.Equal(t, answer, collectAnthropicText(events))
	requireAnthropicBlockLifecycle(t, events)
	require.NotEmpty(t, events)
	assert.Equal(t, "message_stop", events[len(events)-1].Type)
}

// When nothing streamed at all, the terminal event still carries the finished
// output. Dropping it produces the same empty turn.
func TestResponsesEventToAnthropicEvents_RecoversTextFromTerminalOutput(t *testing.T) {
	const answer = "recovered from terminal"

	events := feedResponsesEvents(
		responsesCreated(),
		&ResponsesStreamEvent{Type: "response.completed", Response: &ResponsesResponse{
			Status: "completed",
			Output: responsesMessageOutput(answer),
		}},
	)

	assert.Equal(t, answer, collectAnthropicText(events))
	requireAnthropicBlockLifecycle(t, events)
	require.NotEmpty(t, events)
	assert.Equal(t, "message_stop", events[len(events)-1].Type)
}

// The normal path delivers text through deltas. Neither the done payload nor
// the terminal payload may repeat it.
func TestResponsesEventToAnthropicEvents_DoesNotDuplicateTextDeliveredByDeltas(t *testing.T) {
	events := feedResponsesEvents(
		responsesCreated(),
		&ResponsesStreamEvent{Type: "response.output_text.delta", Delta: "Hel"},
		&ResponsesStreamEvent{Type: "response.output_text.delta", Delta: "lo"},
		&ResponsesStreamEvent{Type: "response.output_text.done", Text: "Hello"},
		&ResponsesStreamEvent{Type: "response.completed", Response: &ResponsesResponse{
			Status: "completed",
			Output: responsesMessageOutput("Hello"),
		}},
	)

	assert.Equal(t, "Hello", collectAnthropicText(events))
	requireAnthropicBlockLifecycle(t, events)
}

// Text recovered from a done event must not be repeated by the terminal
// payload that also carries it.
func TestResponsesEventToAnthropicEvents_DoesNotDuplicateTextRecoveredFromDoneEvent(t *testing.T) {
	const answer = "recovered once"

	events := feedResponsesEvents(
		responsesCreated(),
		&ResponsesStreamEvent{Type: "response.output_text.done", Text: answer},
		&ResponsesStreamEvent{Type: "response.completed", Response: &ResponsesResponse{
			Status: "completed",
			Output: responsesMessageOutput(answer),
		}},
	)

	assert.Equal(t, answer, collectAnthropicText(events))
	requireAnthropicBlockLifecycle(t, events)
}

// Deduplication is per output_text part, not per content block: an event that
// closes the block between the deltas and the matching done must not make the
// done payload look undelivered.
func TestResponsesEventToAnthropicEvents_DoesNotDuplicateTextWhenBlockClosedBeforeDone(t *testing.T) {
	events := feedResponsesEvents(
		responsesCreated(),
		&ResponsesStreamEvent{Type: "response.output_text.delta", Delta: "Hello"},
		&ResponsesStreamEvent{
			Type:        "response.output_item.added",
			OutputIndex: 1,
			Item:        &ResponsesOutput{Type: "function_call", CallID: "call_1", Name: "lookup"},
		},
		&ResponsesStreamEvent{Type: "response.output_text.done", Text: "Hello"},
		&ResponsesStreamEvent{Type: "response.completed", Response: &ResponsesResponse{Status: "completed"}},
	)

	assert.Equal(t, "Hello", collectAnthropicText(events))
	requireAnthropicBlockLifecycle(t, events)
}

// When only part of the text streamed, the done payload is authoritative and
// the missing suffix must still reach the client.
func TestResponsesEventToAnthropicEvents_AppendsOnlySuffixMissingFromDeltas(t *testing.T) {
	events := feedResponsesEvents(
		responsesCreated(),
		&ResponsesStreamEvent{Type: "response.output_text.delta", Delta: "Hel"},
		&ResponsesStreamEvent{Type: "response.output_text.done", Text: "Hello world"},
		&ResponsesStreamEvent{Type: "response.completed", Response: &ResponsesResponse{
			Status: "completed",
			Output: responsesMessageOutput("Hello world"),
		}},
	)

	assert.Equal(t, "Hello world", collectAnthropicText(events))
	requireAnthropicBlockLifecycle(t, events)
}

// Streamed text cannot be recalled, so a payload that contradicts what was
// already delivered is left alone rather than appended.
func TestResponsesEventToAnthropicEvents_IgnoresDonePayloadDivergingFromDeltas(t *testing.T) {
	events := feedResponsesEvents(
		responsesCreated(),
		&ResponsesStreamEvent{Type: "response.output_text.delta", Delta: "Hello"},
		&ResponsesStreamEvent{Type: "response.output_text.done", Text: "Goodbye"},
		&ResponsesStreamEvent{Type: "response.completed", Response: &ResponsesResponse{Status: "completed"}},
	)

	assert.Equal(t, "Hello", collectAnthropicText(events))
	requireAnthropicBlockLifecycle(t, events)
}

// Streamed events and the terminal output array share no guaranteed identity,
// so once any text has been delivered the terminal payload is left alone. An
// upstream that numbers its streamed items differently from its terminal array
// must not make a delivered answer look undelivered.
func TestResponsesEventToAnthropicEvents_DoesNotDuplicateWhenTerminalOutputIsIndexedDifferently(t *testing.T) {
	events := feedResponsesEvents(
		responsesCreated(),
		&ResponsesStreamEvent{
			Type:        "response.output_item.added",
			OutputIndex: 0,
			Item:        &ResponsesOutput{Type: "reasoning", ID: "rs_1"},
		},
		&ResponsesStreamEvent{Type: "response.output_text.delta", OutputIndex: 1, Delta: "Hello"},
		&ResponsesStreamEvent{Type: "response.completed", Response: &ResponsesResponse{
			Status: "completed",
			// The reasoning item is absent here, so the message sits at array
			// position 0 while it streamed at output_index 1.
			Output: responsesMessageOutput("Hello"),
		}},
	)

	assert.Equal(t, "Hello", collectAnthropicText(events))
	requireAnthropicBlockLifecycle(t, events)
}

// Partially streamed responses keep exactly what streamed. Recovering the rest
// would mean matching two index spaces that upstreams do not keep aligned.
func TestResponsesEventToAnthropicEvents_LeavesTerminalOutputAloneOnceTextStreamed(t *testing.T) {
	events := feedResponsesEvents(
		responsesCreated(),
		&ResponsesStreamEvent{Type: "response.output_text.delta", ContentIndex: 0, Delta: "first"},
		&ResponsesStreamEvent{Type: "response.output_text.done", ContentIndex: 0, Text: "first"},
		&ResponsesStreamEvent{Type: "response.completed", Response: &ResponsesResponse{
			Status: "completed",
			Output: responsesMessageOutput("first", "second"),
		}},
	)

	assert.Equal(t, "first", collectAnthropicText(events))
	requireAnthropicBlockLifecycle(t, events)
}

// When nothing streamed, every output_text part of the terminal payload is
// recovered, merged into one block the way contiguous streamed text is.
func TestResponsesEventToAnthropicEvents_RecoversEveryTerminalPartWhenNothingStreamed(t *testing.T) {
	events := feedResponsesEvents(
		responsesCreated(),
		&ResponsesStreamEvent{Type: "response.completed", Response: &ResponsesResponse{
			Status: "completed",
			Output: responsesMessageOutput("first", "second"),
		}},
	)

	assert.Equal(t, "firstsecond", collectAnthropicText(events))
	requireAnthropicBlockLifecycle(t, events)

	starts := 0
	for _, event := range events {
		if event.Type == "content_block_start" {
			starts++
		}
	}
	assert.Equal(t, 1, starts, "contiguous recovered text belongs in one block")
}

// A turn that produced only a tool call must stay text-free: the terminal
// payload carries no message item and nothing may be synthesised.
func TestResponsesEventToAnthropicEvents_LeavesToolOnlyTurnTextFree(t *testing.T) {
	events := feedResponsesEvents(
		responsesCreated(),
		&ResponsesStreamEvent{
			Type:        "response.output_item.added",
			OutputIndex: 0,
			Item:        &ResponsesOutput{Type: "function_call", CallID: "call_1", Name: "lookup"},
		},
		&ResponsesStreamEvent{
			Type:        "response.function_call_arguments.done",
			OutputIndex: 0,
			Arguments:   `{"id":1}`,
		},
		&ResponsesStreamEvent{Type: "response.completed", Response: &ResponsesResponse{
			Status: "completed",
			Output: []ResponsesOutput{{Type: "function_call", CallID: "call_1", Name: "lookup", Arguments: `{"id":1}`}},
		}},
	)

	assert.Empty(t, collectAnthropicText(events))
	requireAnthropicBlockLifecycle(t, events)
}

// Recovery must behave the same on every terminal alias the converter accepts,
// not only on response.completed.
func TestResponsesEventToAnthropicEvents_RecoversTextOnEveryTerminalAlias(t *testing.T) {
	for _, terminal := range []string{
		"response.completed",
		"response.done",
		"response.incomplete",
		"response.failed",
	} {
		t.Run(terminal, func(t *testing.T) {
			const answer = "recovered on a terminal alias"

			events := feedResponsesEvents(
				responsesCreated(),
				&ResponsesStreamEvent{Type: terminal, Response: &ResponsesResponse{
					Status: "completed",
					Output: responsesMessageOutput(answer),
				}},
			)

			assert.Equal(t, answer, collectAnthropicText(events))
			requireAnthropicBlockLifecycle(t, events)
			require.NotEmpty(t, events)
			assert.Equal(t, "message_stop", events[len(events)-1].Type)
		})
	}
}

// A done payload indexed differently from every delta seen so far cannot be
// matched to a part. Emitting it anyway would repeat an answer the client
// already has, which is the failure the terminal guard also exists to prevent.
func TestResponsesEventToAnthropicEvents_DoesNotDuplicateWhenDoneIsIndexedDifferently(t *testing.T) {
	for _, tc := range []struct {
		name string
		done *ResponsesStreamEvent
	}{
		{"content index diverges", &ResponsesStreamEvent{Type: "response.output_text.done", ContentIndex: 1, Text: "Hello world"}},
		{"output index diverges", &ResponsesStreamEvent{Type: "response.output_text.done", OutputIndex: 1, Text: "Hello world"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			events := feedResponsesEvents(
				responsesCreated(),
				&ResponsesStreamEvent{Type: "response.output_text.delta", Delta: "Hello world"},
				tc.done,
				&ResponsesStreamEvent{Type: "response.completed", Response: &ResponsesResponse{Status: "completed"}},
			)

			assert.Equal(t, "Hello world", collectAnthropicText(events))
			requireAnthropicBlockLifecycle(t, events)
		})
	}
}

// The same rule holds when only part of the text streamed: an unmatchable
// payload is left alone rather than appended on top of what was delivered.
func TestResponsesEventToAnthropicEvents_LeavesUnmatchableDoneAloneAfterPartialDeltas(t *testing.T) {
	events := feedResponsesEvents(
		responsesCreated(),
		&ResponsesStreamEvent{Type: "response.output_text.delta", ContentIndex: 0, Delta: "Hello"},
		&ResponsesStreamEvent{Type: "response.output_text.done", ContentIndex: 1, Text: "Hello world"},
		&ResponsesStreamEvent{Type: "response.completed", Response: &ResponsesResponse{Status: "completed"}},
	)

	assert.Equal(t, "Hello", collectAnthropicText(events))
	requireAnthropicBlockLifecycle(t, events)
}

// Delivered text is keyed by output index and content index together. Keying on
// either alone would make one of these parts look already delivered.
func TestResponsesEventToAnthropicEvents_KeysDeliveredTextByBothIndices(t *testing.T) {
	events := feedResponsesEvents(
		responsesCreated(),
		&ResponsesStreamEvent{Type: "response.output_text.delta", OutputIndex: 0, ContentIndex: 0, Delta: "first"},
		&ResponsesStreamEvent{Type: "response.output_text.done", OutputIndex: 0, ContentIndex: 0, Text: "first"},
		&ResponsesStreamEvent{Type: "response.output_text.delta", OutputIndex: 0, ContentIndex: 1, Delta: "second"},
		&ResponsesStreamEvent{Type: "response.output_text.done", OutputIndex: 0, ContentIndex: 1, Text: "second-tail"},
		&ResponsesStreamEvent{Type: "response.completed", Response: &ResponsesResponse{Status: "completed"}},
	)

	assert.Equal(t, "firstsecond-tail", collectAnthropicText(events))
	requireAnthropicBlockLifecycle(t, events)
}

// A stray done event after the terminal one must not resurrect a stopped
// message. The baseline dropped it outright; recovery has to keep that, or a
// client sees a content block after message_stop.
func TestResponsesEventToAnthropicEvents_IgnoresDoneEventAfterMessageStop(t *testing.T) {
	events := feedResponsesEvents(
		responsesCreated(),
		&ResponsesStreamEvent{Type: "response.completed", Response: &ResponsesResponse{
			Status: "completed",
			Output: responsesMessageOutput("the answer"),
		}},
		&ResponsesStreamEvent{Type: "response.output_text.done", OutputIndex: 7, Text: "late text"},
	)

	assert.Equal(t, "the answer", collectAnthropicText(events))
	requireAnthropicBlockLifecycle(t, events)
	require.NotEmpty(t, events)
	assert.Equal(t, "message_stop", events[len(events)-1].Type, "no content may follow message_stop")
}

// The tool path reads arguments, never text. A tool done event carrying a
// stray text field must not produce a text block.
func TestResponsesEventToAnthropicEvents_LeavesToolArgumentsDoneTextFree(t *testing.T) {
	events := feedResponsesEvents(
		responsesCreated(),
		&ResponsesStreamEvent{
			Type:        "response.output_item.added",
			OutputIndex: 0,
			Item:        &ResponsesOutput{Type: "function_call", CallID: "call_1", Name: "lookup"},
		},
		&ResponsesStreamEvent{
			Type:        "response.function_call_arguments.done",
			OutputIndex: 0,
			Arguments:   `{"id":1}`,
			Text:        "must not be emitted",
		},
		&ResponsesStreamEvent{Type: "response.completed", Response: &ResponsesResponse{Status: "completed"}},
	)

	assert.Empty(t, collectAnthropicText(events))
	requireAnthropicBlockLifecycle(t, events)
}

// Recovery closes whatever block is open through the shared helper, which owns
// signature emission. A thinking block must still get its signature_delta
// before its stop.
func TestResponsesEventToAnthropicEvents_PreservesThinkingSignatureWhenRecovering(t *testing.T) {
	events := feedResponsesEvents(
		responsesCreated(),
		&ResponsesStreamEvent{
			Type:        "response.output_item.added",
			OutputIndex: 0,
			Item:        &ResponsesOutput{Type: "reasoning", EncryptedContent: "sig-abc"},
		},
		&ResponsesStreamEvent{Type: "response.output_text.done", OutputIndex: 1, Text: "recovered"},
		&ResponsesStreamEvent{Type: "response.completed", Response: &ResponsesResponse{Status: "completed"}},
	)

	assert.Equal(t, "recovered", collectAnthropicText(events))
	requireAnthropicBlockLifecycle(t, events)

	var signatures, stopsBefore int
	for _, event := range events {
		if event.Type == "content_block_delta" && event.Delta != nil && event.Delta.Type == "signature_delta" {
			assert.Equal(t, "sig-abc", event.Delta.Signature)
			signatures++
			assert.Equal(t, 0, stopsBefore, "signature_delta must precede the thinking block stop")
		}
		if event.Type == "content_block_stop" {
			stopsBefore++
		}
	}
	assert.Equal(t, 1, signatures, "the thinking signature must survive recovery")
}

// A stream that ends without any terminal event exits through the synthetic
// finalizer, which must still close a block recovery opened.
func TestResponsesEventToAnthropicEvents_RecoveredTextSurvivesTheSyntheticFinalizer(t *testing.T) {
	state := NewResponsesEventToAnthropicState()
	var events []AnthropicStreamEvent
	for _, evt := range []*ResponsesStreamEvent{
		responsesCreated(),
		{Type: "response.output_text.done", Text: "recovered before the stream died"},
	} {
		events = append(events, ResponsesEventToAnthropicEvents(evt, state)...)
	}
	events = append(events, FinalizeResponsesAnthropicStream(state)...)

	assert.Equal(t, "recovered before the stream died", collectAnthropicText(events))
	requireAnthropicBlockLifecycle(t, events)
	require.NotEmpty(t, events)
	assert.Equal(t, "message_stop", events[len(events)-1].Type)
}

// The terminal walk only takes output_text parts of message items.
func TestResponsesEventToAnthropicEvents_DeclinesTerminalRecoveryWithoutMessageText(t *testing.T) {
	for _, tc := range []struct {
		name     string
		response *ResponsesResponse
	}{
		{"no response payload", nil},
		{"no message item", &ResponsesResponse{Status: "completed", Output: []ResponsesOutput{{Type: "reasoning"}}}},
		{"message without output_text", &ResponsesResponse{Status: "completed", Output: []ResponsesOutput{{
			Type:    "message",
			Role:    "assistant",
			Content: []ResponsesContentPart{{Type: "refusal", Text: "refused"}},
		}}}},
		{"empty output_text", &ResponsesResponse{Status: "completed", Output: responsesMessageOutput("")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			events := feedResponsesEvents(
				responsesCreated(),
				&ResponsesStreamEvent{Type: "response.completed", Response: tc.response},
			)

			assert.Empty(t, collectAnthropicText(events))
			requireAnthropicBlockLifecycle(t, events)
		})
	}
}

// Every message item of the terminal payload is recovered, and non-message
// items between them are skipped without breaking the merged block.
func TestResponsesEventToAnthropicEvents_RecoversEveryTerminalMessageItem(t *testing.T) {
	events := feedResponsesEvents(
		responsesCreated(),
		&ResponsesStreamEvent{Type: "response.completed", Response: &ResponsesResponse{
			Status: "completed",
			Output: []ResponsesOutput{
				{Type: "reasoning"},
				{Type: "message", Role: "assistant", Content: []ResponsesContentPart{{Type: "output_text", Text: "first"}}},
				{Type: "function_call", CallID: "call_1", Name: "lookup"},
				{Type: "message", Role: "assistant", Content: []ResponsesContentPart{{Type: "output_text", Text: "second"}}},
			},
		}},
	)

	assert.Equal(t, "firstsecond", collectAnthropicText(events))
	requireAnthropicBlockLifecycle(t, events)
}

// Text that upstream delivers only on a done event still reaches the client
// when a tool block is open, which the baseline dropped. The tool block is
// closed first, so later block indices shift by one.
func TestResponsesEventToAnthropicEvents_EmitsDoneTextWhileAToolBlockIsOpen(t *testing.T) {
	events := feedResponsesEvents(
		responsesCreated(),
		&ResponsesStreamEvent{
			Type:        "response.output_item.added",
			OutputIndex: 0,
			Item:        &ResponsesOutput{Type: "function_call", CallID: "call_1", Name: "lookup"},
		},
		&ResponsesStreamEvent{Type: "response.output_text.done", OutputIndex: 1, Text: "spoken after the call"},
		&ResponsesStreamEvent{Type: "response.completed", Response: &ResponsesResponse{Status: "completed"}},
	)

	assert.Equal(t, "spoken after the call", collectAnthropicText(events))
	requireAnthropicBlockLifecycle(t, events)
}
