package apicompat

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type streamAccumulationFieldCase struct {
	name      string
	blockType string
	field     func(*AnthropicEventToResponsesState) *string
	delta     func(string) *AnthropicDelta
}

func streamAccumulationFieldCases() []streamAccumulationFieldCase {
	return []streamAccumulationFieldCase{
		{"text", "text", func(s *AnthropicEventToResponsesState) *string { return &s.TextAccum }, func(v string) *AnthropicDelta { return &AnthropicDelta{Type: "text_delta", Text: v} }},
		{"thinking", "thinking", func(s *AnthropicEventToResponsesState) *string { return &s.CurrentSummary }, func(v string) *AnthropicDelta { return &AnthropicDelta{Type: "thinking_delta", Thinking: v} }},
		{"arguments", "tool_use", func(s *AnthropicEventToResponsesState) *string { return &s.CurrentArgs }, func(v string) *AnthropicDelta { return &AnthropicDelta{Type: "input_json_delta", PartialJSON: v} }},
		{"signature", "thinking", func(s *AnthropicEventToResponsesState) *string { return &s.CurrentThinking.Signature }, func(v string) *AnthropicDelta { return &AnthropicDelta{Type: "signature_delta", Signature: v} }},
	}
}

func TestStreamAccumulationAnthropicPublicStringsAndReset(t *testing.T) {
	for _, tc := range streamAccumulationFieldCases() {
		t.Run(tc.name, func(t *testing.T) {
			state := NewAnthropicEventToResponsesState()
			state.PreserveThinkingSignatures = true
			block := &AnthropicContentBlock{Type: tc.blockType, ID: "call_seed", Name: "tool"}
			if tc.name == "thinking" {
				block.Thinking = "seed-"
			}
			if tc.name == "signature" {
				block.Signature = "seed-"
			}
			AnthropicEventToResponsesEvents(&AnthropicStreamEvent{Type: "content_block_start", ContentBlock: block}, state)
			if tc.name == "thinking" || tc.name == "signature" {
				require.Equal(t, "seed-", *tc.field(state), "content block seeds remain visible")
			} else {
				*tc.field(state) = "seed-"
			}
			appendDelta := func(value string) {
				AnthropicEventToResponsesEvents(&AnthropicStreamEvent{Type: "content_block_delta", Delta: tc.delta(value)}, state)
			}

			appendDelta("one")
			require.Equal(t, "seed-one", *tc.field(state))
			firstSnapshot := *tc.field(state)
			appendDelta("-two")
			require.Equal(t, "seed-one-two", *tc.field(state))
			require.Equal(t, "seed-one", firstSnapshot, "later deltas must not mutate borrowed strings")
			secondSnapshot := *tc.field(state)

			// An external assignment with the same length must replace the old
			// accumulator, not be mistaken for a synchronized public field.
			replacement := strings.Repeat("r", len(secondSnapshot))
			*tc.field(state) = replacement
			appendDelta("!")
			require.Equal(t, replacement+"!", *tc.field(state))
			require.Equal(t, "seed-one-two", secondSnapshot)

			*tc.field(state) = ""
			appendDelta("after-reset")
			require.Equal(t, "after-reset", *tc.field(state))
			closedSnapshot := *tc.field(state)
			closedEvents := AnthropicEventToResponsesEvents(&AnthropicStreamEvent{Type: "content_block_stop"}, state)
			require.Empty(t, *tc.field(state))

			AnthropicEventToResponsesEvents(&AnthropicStreamEvent{Type: "content_block_start", ContentBlock: &AnthropicContentBlock{Type: tc.blockType, ID: "call_next", Name: "tool"}}, state)
			appendDelta(strings.Repeat("next", 128))
			require.Equal(t, strings.Repeat("next", 128), *tc.field(state), "a new block must not reuse the previous prefix")
			require.Equal(t, "after-reset", closedSnapshot)
			require.Equal(t, "seed-one", firstSnapshot)
			if tc.name == "text" {
				require.Len(t, closedEvents, 2)
				require.Equal(t, "after-reset", closedEvents[0].Text)
				require.Equal(t, "after-reset", closedEvents[1].Part.Text)
			}
		})
	}
}

func TestStreamAccumulationAnthropicCopiedStateBranches(t *testing.T) {
	for _, tc := range streamAccumulationFieldCases() {
		t.Run(tc.name, func(t *testing.T) {
			original := NewAnthropicEventToResponsesState()
			original.PreserveThinkingSignatures = true
			appendDelta := func(state *AnthropicEventToResponsesState, value string) {
				AnthropicEventToResponsesEvents(&AnthropicStreamEvent{Type: "content_block_delta", Delta: tc.delta(value)}, state)
			}
			appendDelta(original, "base")
			baseSnapshot := *tc.field(original)
			copied := *original

			// Sequential copies historically had independent string values.
			// This does not promise concurrent mutation of shared state is safe.
			appendDelta(original, "/left")
			appendDelta(&copied, "/right")
			require.Equal(t, "base/left", *tc.field(original))
			require.Equal(t, "base/right", *tc.field(&copied))
			appendDelta(original, "/again")
			require.Equal(t, "base/left/again", *tc.field(original))
			require.Equal(t, "base/right", *tc.field(&copied))
			require.Equal(t, "base", baseSnapshot)
		})
	}
}

func TestStreamAccumulationResponsesChatToolIsolation(t *testing.T) {
	state := NewResponsesEventToChatState()
	for _, idx := range []int{2, 7} {
		ResponsesEventToChatChunks(&ResponsesStreamEvent{Type: "response.output_item.added", OutputIndex: idx, Item: &ResponsesOutput{Type: "function_call", CallID: fmt.Sprintf("call_%d", idx), Name: "tool"}}, state)
	}
	appendDelta := func(idx int, delta, want string) {
		t.Helper()
		ResponsesEventToChatChunks(&ResponsesStreamEvent{Type: "response.function_call_arguments.delta", OutputIndex: idx, Delta: delta}, state)
		require.Equal(t, want, state.OutputIndexToArguments[idx])
	}
	state.OutputIndexToArguments[2] = `{"a":`
	appendDelta(2, "1", `{"a":1`)
	firstSnapshot := state.OutputIndexToArguments[2]
	appendDelta(7, `{"b":`, `{"b":`)
	appendDelta(2, "}", `{"a":1}`)
	appendDelta(7, "2}", `{"b":2}`)
	require.Equal(t, `{"a":1`, firstSnapshot)
	require.Equal(t, `{"a":1}`, state.OutputIndexToArguments[2])

	ResponsesEventToChatChunks(&ResponsesStreamEvent{Type: "response.function_call_arguments.done", OutputIndex: 2, Arguments: `{"a":1} `}, state)
	require.Equal(t, `{"a":1} `, state.OutputIndexToArguments[2])
	appendDelta(2, "\n", "{\"a\":1} \n")
	state.OutputIndexToArguments[2] = "external"
	appendDelta(2, "!", "external!")
	delete(state.OutputIndexToArguments, 2)
	appendDelta(2, "fresh", "fresh")
	require.Equal(t, `{"b":2}`, state.OutputIndexToArguments[7])
}

func streamAccumulationChatToolChunk(idx int, name, arguments string) *ChatCompletionsChunk {
	return &ChatCompletionsChunk{Choices: []ChatChunkChoice{{Delta: ChatDelta{ToolCalls: []ChatToolCall{{Index: &idx, ID: fmt.Sprintf("call_%d", idx), Type: "function", Function: ChatFunctionCall{Name: name, Arguments: arguments}}}}}}}
}

func TestStreamAccumulationChatResponsesToolIsolation(t *testing.T) {
	state := NewChatCompletionsToResponsesStreamState("model")
	appendDelta := func(idx int, name, delta, want string) {
		t.Helper()
		ChatCompletionsChunkToResponsesEvents(streamAccumulationChatToolChunk(idx, name, delta), state)
		require.Equal(t, want, state.ToolCalls[idx].Function.Arguments)
	}
	appendDelta(0, "first", `{"a":`, `{"a":`)
	firstSnapshot := state.ToolCalls[0].Function.Arguments
	appendDelta(1, "second", `{"b":`, `{"b":`)
	appendDelta(0, "", "1}", `{"a":1}`)
	appendDelta(1, "", "2}", `{"b":2}`)
	require.Equal(t, `{"a":`, firstSnapshot)
	require.Equal(t, `{"a":1}`, state.ToolCalls[0].Function.Arguments)

	state.ToolCalls[0].Function.Arguments = `{"c":3`
	appendDelta(0, "", "}", `{"c":3}`)
	state.ToolCalls[0].Function.Arguments = ""
	appendDelta(0, "", "{}", "{}")
	oldCall := state.ToolCalls[0]
	delete(state.ToolCalls, 0)
	appendDelta(0, "replacement", `{"new":`, `{"new":`)
	appendDelta(0, "", "4}", `{"new":4}`)
	require.Equal(t, "{}", oldCall.Function.Arguments, "replacing an indexed call must leave the old call immutable")
	require.Equal(t, `{"b":2}`, state.ToolCalls[1].Function.Arguments)
	require.NoError(t, state.ValidateToolCallArguments())
}

func TestStreamAccumulationReadRawArgumentsAndReset(t *testing.T) {
	state := NewResponsesEventToAnthropicState()
	startTool := func(idx int) {
		ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{Type: "response.output_item.added", OutputIndex: idx, Item: &ResponsesOutput{Type: "function_call", CallID: fmt.Sprintf("call_%d", idx), Name: "Read"}}, state)
	}
	startTool(0)
	state.CurrentToolArgs = `{"file_path":"`
	ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{Type: "response.function_call_arguments.delta", OutputIndex: 0, Delta: "/tmp/one"}, state)
	require.Equal(t, `{"file_path":"/tmp/one`, state.CurrentToolArgs)
	require.NotNil(t, state.currentToolArgsValidation)
	require.Equal(t, state.CurrentToolArgs, string(state.currentToolArgsValidation.data))
	firstSnapshot := state.CurrentToolArgs
	firstValidationSnapshot := state.currentToolArgsValidation.data
	// Same-length caller replacement must also be respected by this special
	// path, whose externally visible raw JSON differs from sanitized output.
	state.CurrentToolArgs = `{"file_path":"/tmp/two`
	events := ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{Type: "response.function_call_arguments.delta", OutputIndex: 0, Delta: `","pages":""}`}, state)
	require.Len(t, events, 1)
	require.JSONEq(t, `{"file_path":"/tmp/two"}`, events[0].Delta.PartialJSON)
	require.Equal(t, `{"file_path":"/tmp/two","pages":""}`, state.CurrentToolArgs)
	require.Equal(t, state.CurrentToolArgs, string(state.currentToolArgsValidation.data))
	require.Equal(t, firstSnapshot, string(firstValidationSnapshot), "external replacement must not overwrite a borrowed validation prefix")
	rawSnapshot := state.CurrentToolArgs
	require.Equal(t, `{"file_path":"/tmp/one`, firstSnapshot)

	ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{Type: "response.function_call_arguments.done", OutputIndex: 0}, state)
	require.Empty(t, state.CurrentToolArgs)
	require.Nil(t, state.currentToolArgsValidation)
	startTool(1)
	require.Nil(t, state.currentToolArgsValidation)
	ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{Type: "response.function_call_arguments.delta", OutputIndex: 1, Delta: `{"file_path":"/tmp/new"}`}, state)
	require.Equal(t, `{"file_path":"/tmp/new"}`, state.CurrentToolArgs)
	require.Equal(t, state.CurrentToolArgs, string(state.currentToolArgsValidation.data))
	require.Equal(t, `{"file_path":"/tmp/two","pages":""}`, rawSnapshot)
	require.JSONEq(t, `{"file_path":"/tmp/two"}`, events[0].Delta.PartialJSON)
}

func TestStreamAccumulationReadValidationCopiedStateBranches(t *testing.T) {
	original := NewResponsesEventToAnthropicState()
	ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{Type: "response.output_item.added", OutputIndex: 0, Item: &ResponsesOutput{Type: "function_call", CallID: "call_read", Name: "Read"}}, original)
	appendDelta := func(state *ResponsesEventToAnthropicState, value string) {
		t.Helper()
		events := ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{Type: "response.function_call_arguments.delta", OutputIndex: 0, Delta: value}, state)
		require.Empty(t, events, "unfinished strings must not pass the complete-JSON gate")
		require.NotNil(t, state.currentToolArgsValidation)
		require.Equal(t, state.CurrentToolArgs, string(state.currentToolArgsValidation.data))
		require.Equal(t, state.CurrentToolArgs, state.currentToolArgsValidation.snapshot)
	}
	original.CurrentToolArgs = `{"file_path":"`
	appendDelta(original, "base")
	baseSnapshot := original.CurrentToolArgs
	baseValidationSnapshot := original.currentToolArgsValidation.data
	copied := *original

	appendDelta(original, "/left")
	leftValidationSnapshot := original.currentToolArgsValidation.data
	appendDelta(&copied, "/right")
	require.Equal(t, `{"file_path":"base/left`, original.CurrentToolArgs)
	require.Equal(t, original.CurrentToolArgs, string(original.currentToolArgsValidation.data))
	require.Equal(t, `{"file_path":"base/right`, copied.CurrentToolArgs)
	appendDelta(original, "/again")
	require.Equal(t, `{"file_path":"base/left/again`, original.CurrentToolArgs)
	require.Equal(t, `{"file_path":"base/right`, string(copied.currentToolArgsValidation.data))
	require.Equal(t, baseSnapshot, string(baseValidationSnapshot))
	require.Equal(t, `{"file_path":"base/left`, string(leftValidationSnapshot))

	beforeReset := original.currentToolArgsValidation.data
	original.CurrentToolArgs = ""
	appendDelta(original, `{"file_path":"reset`)
	require.Equal(t, `{"file_path":"reset`, original.CurrentToolArgs)
	require.Equal(t, `{"file_path":"base/left/again`, string(beforeReset))
	require.Equal(t, `{"file_path":"base/right`, string(copied.currentToolArgsValidation.data))
}

func TestStreamAccumulationReadValidationStopsAfterComplete(t *testing.T) {
	state := NewResponsesEventToAnthropicState()
	ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{Type: "response.output_item.added", OutputIndex: 0, Item: &ResponsesOutput{Type: "function_call", CallID: "call_read", Name: "Read"}}, state)
	const raw = `{"file_path":"/tmp/file","pages":""}`
	events := ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{Type: "response.function_call_arguments.delta", OutputIndex: 0, Delta: raw}, state)
	require.Len(t, events, 1)
	require.True(t, state.CurrentToolHadDelta)
	validation := state.currentToolArgsValidation
	require.NotNil(t, validation)
	require.Equal(t, raw, string(validation.data))
	validationCapacity := cap(validation.data)

	for _, trailing := range []string{" ", strings.Repeat("x", 1024)} {
		previous := state.CurrentToolArgs
		events := ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{Type: "response.function_call_arguments.delta", OutputIndex: 0, Delta: trailing}, state)
		require.Empty(t, events, "a completed Read tool must not emit another sanitized delta")
		require.Equal(t, previous+trailing, state.CurrentToolArgs, "the existing public string still accumulates late deltas")
		require.Same(t, validation, state.currentToolArgsValidation)
		require.Equal(t, raw, string(validation.data))
		require.Equal(t, raw, validation.snapshot)
		require.Equal(t, validationCapacity, cap(validation.data))
	}
	require.JSONEq(t, `{"file_path":"/tmp/file"}`, events[0].Delta.PartialJSON)
	ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{Type: "response.function_call_arguments.done", OutputIndex: 0}, state)
	require.Nil(t, state.currentToolArgsValidation)
	require.Empty(t, state.CurrentToolArgs)
}

func TestStreamAccumulationDeferredAnthropicTools(t *testing.T) {
	state := NewChatCompletionsToAnthropicStreamState("model")
	var events []AnthropicStreamEvent
	feed := func(idx int, name, args string) {
		events = append(events, ChatCompletionsChunkToAnthropicEvents(streamAccumulationChatToolChunk(idx, name, args), state)...)
	}
	feed(0, "", `{"a":`)
	feed(1, "", `{"b":`)
	feed(0, "", "1")
	feed(1, "", "2")
	feed(0, "first", "}")
	feed(1, "second", "}")
	events = append(events, FinalizeChatCompletionsAnthropicStream(state)...)
	blocks := assembleToolUseBlocks(events)
	require.Len(t, blocks, 2)
	require.Equal(t, "first", blocks[0].Name)
	require.Equal(t, `{"a":1}`, blocks[0].Input)
	require.Equal(t, "second", blocks[1].Name)
	require.Equal(t, `{"b":2}`, blocks[1].Input)
	require.Empty(t, FinalizeChatCompletionsAnthropicStream(state))
}

var streamAccumulationBenchmarkSink string

// ReportAllocs measures cumulative allocation traffic, not retained heap/RSS.
// A fixed 64-byte delta makes total-size scaling expose repeated prefix copies.
func BenchmarkStreamAccumulationAnthropic64ByteDeltas(b *testing.B) {
	for _, tc := range streamAccumulationFieldCases() {
		for _, size := range []int{32 << 10, 128 << 10, 512 << 10} {
			b.Run(fmt.Sprintf("%s/%dKiB", tc.name, size>>10), func(b *testing.B) {
				payload := strings.Repeat("x", 64)
				event := &AnthropicStreamEvent{Type: "content_block_delta", Delta: tc.delta(payload)}
				b.ReportAllocs()
				b.SetBytes(int64(size))
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					state := NewAnthropicEventToResponsesState()
					state.PreserveThinkingSignatures = true
					for received := 0; received < size; received += len(payload) {
						AnthropicEventToResponsesEvents(event, state)
					}
					streamAccumulationBenchmarkSink = *tc.field(state)
					if len(streamAccumulationBenchmarkSink) != size {
						b.Fatalf("accumulated %d bytes, want %d", len(streamAccumulationBenchmarkSink), size)
					}
				}
			})
		}
	}
}

// Read's existing complete-JSON gate inspects the accumulated arguments on
// every delta. Keep the file_path string unfinished to measure that independent
// cost without changing when the converter emits sanitized tool input.
func BenchmarkStreamAccumulationRead64ByteDeltas(b *testing.B) {
	for _, size := range []int{32 << 10, 128 << 10, 512 << 10} {
		b.Run(fmt.Sprintf("%dKiB", size>>10), func(b *testing.B) {
			const prefix = `{"file_path":"`
			firstEvent := &ResponsesStreamEvent{Type: "response.function_call_arguments.delta", OutputIndex: 0, Delta: prefix + strings.Repeat("x", 64-len(prefix))}
			nextEvent := &ResponsesStreamEvent{Type: "response.function_call_arguments.delta", OutputIndex: 0, Delta: strings.Repeat("x", 64)}
			b.ReportAllocs()
			b.SetBytes(int64(size))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				state := NewResponsesEventToAnthropicState()
				state.MessageStartSent = true
				state.ContentBlockOpen = true
				state.CurrentBlockType = "tool_use"
				state.CurrentToolName = "Read"
				state.OutputIndexToBlockIdx[0] = 0
				ResponsesEventToAnthropicEvents(firstEvent, state)
				for received := 64; received < size; received += 64 {
					ResponsesEventToAnthropicEvents(nextEvent, state)
				}
				streamAccumulationBenchmarkSink = state.CurrentToolArgs
				if len(streamAccumulationBenchmarkSink) != size || state.CurrentToolHadDelta {
					b.Fatalf("unfinished Read arguments: bytes=%d, emitted=%v", len(streamAccumulationBenchmarkSink), state.CurrentToolHadDelta)
				}
			}
		})
	}
}
