package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func feedChatChunks(t *testing.T, state *ChatCompletionsToResponsesStreamState, chunks []string) []ResponsesStreamEvent {
	t.Helper()
	var events []ResponsesStreamEvent
	for _, payload := range chunks {
		var chunk ChatCompletionsChunk
		require.NoError(t, json.Unmarshal([]byte(payload), &chunk))
		events = append(events, ChatCompletionsChunkToResponsesEvents(&chunk, state)...)
	}
	return events
}

func doneFunctionCalls(events []ResponsesStreamEvent) []*ResponsesOutput {
	var calls []*ResponsesOutput
	for _, e := range events {
		if e.Type == "response.output_item.done" && e.Item != nil && e.Item.Type == "function_call" {
			calls = append(calls, e.Item)
		}
	}
	return calls
}

// Google AI Studio's OpenAI-compatible endpoint streams parallel tool calls
// without an index: each call arrives whole in its own chunk with its own id.
// Treating a missing index as 0 merged both calls into one item whose
// arguments were {"cmd":"ls /tmp"}{"cmd":"date"}, so the stream failed.
func TestStream_ParallelToolCallsWithoutIndexStaySeparate(t *testing.T) {
	state := NewChatCompletionsToResponsesStreamState("gemini-4-neon")
	events := feedChatChunks(t, state, []string{
		`{"choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"id":"call_a","type":"function","function":{"name":"exec_command","arguments":"{\"cmd\":\"ls /tmp\"}"}}]}}]}`,
		`{"choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"id":"call_b","type":"function","function":{"name":"exec_command","arguments":"{\"cmd\":\"date\"}"}}]}}]}`,
		`{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}`,
	})

	require.NoError(t, state.ValidateToolCallArguments())
	events = append(events, FinalizeChatCompletionsResponsesStream(state)...)

	calls := doneFunctionCalls(events)
	require.Len(t, calls, 2)
	require.Equal(t, "call_a", calls[0].CallID)
	require.JSONEq(t, `{"cmd":"ls /tmp"}`, calls[0].Arguments)
	require.Equal(t, "call_b", calls[1].CallID)
	require.JSONEq(t, `{"cmd":"date"}`, calls[1].Arguments)
}

// Upstreams that omit the index but send the id only on the first chunk (or
// repeat the same id) must still accumulate later fragments into that call.
func TestStream_ToolCallFragmentsWithoutIndexContinueLastCall(t *testing.T) {
	state := NewChatCompletionsToResponsesStreamState("some-model")
	events := feedChatChunks(t, state, []string{
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"id":"call_a","type":"function","function":{"name":"exec_command","arguments":"{\"cmd\":"}}]}}]}`,
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"function":{"arguments":"\"ls /tmp\""}}]}}]}`,
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"id":"call_a","function":{"arguments":"}"}}]}}]}`,
		`{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
	})

	require.NoError(t, state.ValidateToolCallArguments())
	events = append(events, FinalizeChatCompletionsResponsesStream(state)...)

	calls := doneFunctionCalls(events)
	require.Len(t, calls, 1)
	require.Equal(t, "call_a", calls[0].CallID)
	require.JSONEq(t, `{"cmd":"ls /tmp"}`, calls[0].Arguments)
}

// Indexed streams (OpenAI, Unity LiteLLM) keep their exact behavior, including
// interleaved fragments for different indices.
func TestStream_IndexedParallelToolCallsUnchanged(t *testing.T) {
	state := NewChatCompletionsToResponsesStreamState("gpt")
	events := feedChatChunks(t, state, []string{
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_a","type":"function","function":{"name":"exec_command","arguments":"{\"cmd\":"}}]}}]}`,
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":1,"id":"call_b","type":"function","function":{"name":"exec_command","arguments":"{\"cmd\":\"date\"}"}}]}}]}`,
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"ls\"}"}}]}}]}`,
		`{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
	})

	require.NoError(t, state.ValidateToolCallArguments())
	events = append(events, FinalizeChatCompletionsResponsesStream(state)...)

	calls := doneFunctionCalls(events)
	require.Len(t, calls, 2)
	byID := map[string]string{}
	for _, c := range calls {
		byID[c.CallID] = c.Arguments
	}
	require.JSONEq(t, `{"cmd":"ls"}`, byID["call_a"])
	require.JSONEq(t, `{"cmd":"date"}`, byID["call_b"])
}
