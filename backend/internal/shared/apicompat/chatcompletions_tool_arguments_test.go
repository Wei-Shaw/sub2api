package apicompat

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChatToolArgumentBuffersKeepParallelCallSnapshots(t *testing.T) {
	state := NewChatCompletionsToResponsesStreamState("test")
	prefixes := []string{`{"text":"你好`, `{"text":"🙂`}
	suffixes := []string{` world"}`, ` other"}`}
	var earlier []string
	var events []ResponsesStreamEvent
	for turn := range 2 {
		for idx := range 2 {
			call := ChatToolCall{Index: &idx, ID: "call_" + string(rune('a'+idx)), Type: "function"}
			call.Function.Name = "lookup"
			if turn == 0 {
				call.Function.Arguments = prefixes[idx]
			} else {
				call.Function.Arguments = suffixes[idx]
			}
			events = append(events, ChatCompletionsChunkToResponsesEvents(&ChatCompletionsChunk{Choices: []ChatChunkChoice{{Delta: ChatDelta{ToolCalls: []ChatToolCall{call}}}}}, state)...)
			if turn == 0 {
				earlier = append(earlier, state.ToolCalls[idx].Function.Arguments)
			}
		}
	}
	require.Equal(t, prefixes, earlier, "subsequent fragments cannot mutate earlier snapshots")
	require.NoError(t, state.ValidateToolCallArguments())
	events = append(events, FinalizeChatCompletionsResponsesStream(state)...)
	for idx := range 2 {
		var deltas strings.Builder
		var done string
		for _, event := range events {
			if event.OutputIndex == state.ToolOutputIndex[idx] {
				if event.Type == "response.function_call_arguments.delta" {
					_, _ = deltas.WriteString(event.Delta)
				}
				if event.Type == "response.function_call_arguments.done" {
					done = event.Arguments
				}
			}
		}
		require.Equal(t, prefixes[idx]+suffixes[idx], done)
		require.Equal(t, done, deltas.String())
		require.Equal(t, done, events[len(events)-1].Response.Output[idx].Arguments)
	}
}

func TestChatToolArgumentBufferPreservesSingleChunkAndReplacement(t *testing.T) {
	state := NewChatCompletionsToResponsesStreamState("test")
	call := &ChatToolCall{}
	appendChatToolArguments(state, 0, call, "first")
	require.Nil(t, state.toolArgumentBuffers, "single-fragment calls need no builder")
	appendChatToolArguments(state, 0, call, " second")
	snapshot := call.Function.Arguments
	call.Function.Arguments = "replaced"
	appendChatToolArguments(state, 0, call, " tail")
	require.Equal(t, "first second", snapshot)
	require.Equal(t, "replaced tail", call.Function.Arguments)
}
