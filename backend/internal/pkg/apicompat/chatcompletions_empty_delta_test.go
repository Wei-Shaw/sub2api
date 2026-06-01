package apicompat

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// strptr is a local helper for *string fields.
func strptr(s string) *string { return &s }

// Reproduces the mimo "thinking done, nothing shown" bug: the upstream emits a
// leading {"content":""} chunk (non-nil, empty). The bridge must NOT emit a
// response.output_text.delta for it (the delta would serialize empty and a
// premature message item would be created), and must still stream the real
// content that follows.
func TestChatChunkToResponses_SkipsEmptyContentDelta(t *testing.T) {
	state := NewChatCompletionsToResponsesStreamState("mimo-v2.5")

	// chunk 1: empty content (some upstreams send a leading empty chunk) — no text delta
	c1 := &ChatCompletionsChunk{
		ID:      "c1",
		Choices: []ChatChunkChoice{{Delta: ChatDelta{Role: "assistant", Content: strptr("")}}},
	}
	ev1 := ChatCompletionsChunkToResponsesEvents(c1, state)
	for _, e := range ev1 {
		assert.NotEqual(t, "response.output_text.delta", e.Type,
			"empty content must not emit an output_text delta")
	}

	// chunk 2: real content — must emit a delta carrying the text
	c2 := &ChatCompletionsChunk{
		ID:      "c1",
		Choices: []ChatChunkChoice{{Delta: ChatDelta{Content: strptr("Hello")}}},
	}
	ev2 := ChatCompletionsChunkToResponsesEvents(c2, state)
	var sawDelta bool
	for _, e := range ev2 {
		if e.Type == "response.output_text.delta" {
			sawDelta = true
			assert.Equal(t, "Hello", e.Delta)
		}
	}
	assert.True(t, sawDelta, "real content must emit an output_text delta")
}

func TestChatChunkToResponses_SkipsEmptyReasoningDelta(t *testing.T) {
	state := NewChatCompletionsToResponsesStreamState("mimo-v2.5")
	c := &ChatCompletionsChunk{
		ID:      "c1",
		Choices: []ChatChunkChoice{{Delta: ChatDelta{ReasoningContent: strptr("")}}},
	}
	ev := ChatCompletionsChunkToResponsesEvents(c, state)
	for _, e := range ev {
		assert.NotEqual(t, "response.reasoning_summary_text.delta", e.Type,
			"empty reasoning_content must not emit a reasoning delta")
	}
}

// Full mimo-shaped stream: empty content → reasoning → real content. The final
// visible text must be exactly the real content, and at least one non-empty
// output_text delta must reach the client.
func TestChatChunkToResponses_MimoShapedStream(t *testing.T) {
	state := NewChatCompletionsToResponsesStreamState("mimo-v2.5")
	chunks := []*ChatCompletionsChunk{
		{ID: "x", Choices: []ChatChunkChoice{{Delta: ChatDelta{Role: "assistant", Content: strptr("")}}}},
		{ID: "x", Choices: []ChatChunkChoice{{Delta: ChatDelta{ReasoningContent: strptr("thinking...")}}}},
		{ID: "x", Choices: []ChatChunkChoice{{Delta: ChatDelta{Content: strptr("Hi")}}}},
		{ID: "x", Choices: []ChatChunkChoice{{Delta: ChatDelta{Content: strptr("!")}}}},
	}
	var textDeltas []string
	for _, c := range chunks {
		for _, e := range ChatCompletionsChunkToResponsesEvents(c, state) {
			if e.Type == "response.output_text.delta" {
				textDeltas = append(textDeltas, e.Delta)
			}
		}
	}
	// every emitted text delta is non-empty
	for _, d := range textDeltas {
		assert.NotEqual(t, "", d)
	}
	assert.Equal(t, "Hi!", strings.Join(textDeltas, ""))
}

// codex requires response.content_part.added before output_text deltas and
// content_part.done at the end; without them it renders nothing.
func TestChatChunkToResponses_EmitsContentPartEvents(t *testing.T) {
	state := NewChatCompletionsToResponsesStreamState("mimo-v2.5")
	var types []string
	for _, c := range []*ChatCompletionsChunk{
		{ID: "x", Choices: []ChatChunkChoice{{Delta: ChatDelta{Content: strptr("Hi")}}}},
	} {
		for _, e := range ChatCompletionsChunkToResponsesEvents(c, state) {
			types = append(types, e.Type)
		}
	}
	for _, e := range FinalizeChatCompletionsResponsesStream(state) {
		types = append(types, e.Type)
	}
	assert.Contains(t, types, "response.content_part.added")
	assert.Contains(t, types, "response.content_part.done")
	// content_part.added must come before the first output_text.delta
	iAdded, iDelta := -1, -1
	for i, ty := range types {
		if ty == "response.content_part.added" && iAdded < 0 {
			iAdded = i
		}
		if ty == "response.output_text.delta" && iDelta < 0 {
			iDelta = i
		}
	}
	assert.GreaterOrEqual(t, iDelta, 0)
	assert.GreaterOrEqual(t, iAdded, 0)
	assert.Less(t, iAdded, iDelta, "content_part.added must precede output_text.delta")
}

// codex collects final text from OutputItemDone items, so the message item in
// response.output_item.done must carry content with the accumulated text.
func TestChatChunkToResponses_OutputItemDoneCarriesContent(t *testing.T) {
	state := NewChatCompletionsToResponsesStreamState("mimo-v2.5")
	for _, c := range []*ChatCompletionsChunk{
		{ID: "x", Choices: []ChatChunkChoice{{Delta: ChatDelta{Content: strptr("Hello world")}}}},
	} {
		ChatCompletionsChunkToResponsesEvents(c, state)
	}
	var found bool
	for _, e := range FinalizeChatCompletionsResponsesStream(state) {
		if e.Type == "response.output_item.done" && e.Item != nil && e.Item.Type == "message" {
			found = true
			require.Len(t, e.Item.Content, 1)
			assert.Equal(t, "output_text", e.Item.Content[0].Type)
			assert.Equal(t, "Hello world", e.Item.Content[0].Text)
		}
	}
	assert.True(t, found, "must emit message output_item.done with content")
}

// Some chat/completions upstreams reject reasoning_effort "xhigh"
// (only low/medium/high allowed). It must be normalized to high.
func TestResponsesToChatCompletions_XhighReasoningNormalized(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5","reasoning":{"effort":"xhigh"},"input":[{"role":"user","content":[{"type":"input_text","text":"hi"}]}]}`)
	var req ResponsesRequest
	require.NoError(t, json.Unmarshal(body, &req))
	cc, err := ResponsesToChatCompletionsRequest(&req)
	require.NoError(t, err)
	assert.Equal(t, "high", cc.ReasoningEffort, "xhigh must be normalized to high for chat/completions")
}

func TestNormalizeChatReasoningEffort(t *testing.T) {
	assert.Equal(t, "high", normalizeChatReasoningEffort("xhigh"))
	assert.Equal(t, "high", normalizeChatReasoningEffort("high"))
	assert.Equal(t, "high", normalizeChatReasoningEffort("max"))
	assert.Equal(t, "medium", normalizeChatReasoningEffort("medium"))
	assert.Equal(t, "low", normalizeChatReasoningEffort("low"))
	assert.Equal(t, "low", normalizeChatReasoningEffort("minimal"))
	assert.Equal(t, "", normalizeChatReasoningEffort(""))
	assert.Equal(t, "", normalizeChatReasoningEffort("bogus"))
}

// mimo and other chat/completions upstreams stream tool calls; the bridge must
// emit terminal function_call_arguments.done + output_item.done (with
// call_id/name/arguments) at stream end, or codex receives an unterminated
// tool call and stalls/renders blank.
func TestChatChunkToResponses_StreamedToolCallFinalized(t *testing.T) {
	state := NewChatCompletionsToResponsesStreamState("test-reasoning-model")
	idx := 0
	chunk := &ChatCompletionsChunk{
		ID: "x",
		Choices: []ChatChunkChoice{{Delta: ChatDelta{ToolCalls: []ChatToolCall{{
			Index:    &idx,
			ID:       "call_abc",
			Type:     "function",
			Function: ChatFunctionCall{Name: "open_browser", Arguments: `{"url":"google.com"}`},
		}}}}},
	}
	ChatCompletionsChunkToResponsesEvents(chunk, state)
	final := FinalizeChatCompletionsResponsesStream(state)

	var argsDone, itemDone *ResponsesStreamEvent
	for i := range final {
		switch final[i].Type {
		case "response.function_call_arguments.done":
			argsDone = &final[i]
		case "response.output_item.done":
			if final[i].Item != nil && final[i].Item.Type == "function_call" {
				itemDone = &final[i]
			}
		}
	}
	require.NotNil(t, argsDone, "must emit function_call_arguments.done")
	assert.Equal(t, "call_abc", argsDone.CallID)
	assert.JSONEq(t, `{"url":"google.com"}`, argsDone.Arguments)
	require.NotNil(t, itemDone, "must emit function_call output_item.done")
	assert.Equal(t, "call_abc", itemDone.Item.CallID)
	assert.Equal(t, "open_browser", itemDone.Item.Name)
	assert.JSONEq(t, `{"url":"google.com"}`, itemDone.Item.Arguments)
}
