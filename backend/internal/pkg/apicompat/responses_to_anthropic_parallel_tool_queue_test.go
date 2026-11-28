package apicompat

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// assertStrictAnthropicBlockLifecycle validates strict Anthropic SSE block
// invariants:
//  1. Content block indices start sequentially (0, 1, 2, ...).
//  2. At most one content block is open at any point in the stream.
//  3. Every content_block_delta and content_block_stop targets the currently
//     open block — never an unstarted block and never an already-closed block.
//  4. Every started block is closed before message_delta / message_stop.
func assertStrictAnthropicBlockLifecycle(t *testing.T, events []AnthropicStreamEvent) {
	t.Helper()

	openIdx := -1
	nextIdx := 0
	closed := make(map[int]bool)
	sawMessageStart := false
	sawMessageStop := false

	for step, evt := range events {
		switch evt.Type {
		case "message_start":
			require.Falsef(t, sawMessageStart, "step %d: duplicate message_start", step)
			sawMessageStart = true
		case "content_block_start":
			require.Truef(t, sawMessageStart, "step %d: content_block_start before message_start", step)
			require.Falsef(t, sawMessageStop, "step %d: content_block_start after message_stop", step)
			require.NotNilf(t, evt.Index, "step %d: content_block_start missing index", step)
			require.Equalf(t, -1, openIdx, "step %d: content_block_start(%d) while block %d is still open", step, *evt.Index, openIdx)
			require.Equalf(t, nextIdx, *evt.Index, "step %d: content_block_start expected sequential index %d, got %d", step, nextIdx, *evt.Index)
			openIdx = *evt.Index
			nextIdx++
		case "content_block_delta":
			require.NotNilf(t, evt.Index, "step %d: content_block_delta missing index", step)
			require.Falsef(t, closed[*evt.Index], "step %d: content_block_delta on already-closed block %d", step, *evt.Index)
			require.Equalf(t, openIdx, *evt.Index, "step %d: content_block_delta on index %d while open block is %d", step, *evt.Index, openIdx)
		case "content_block_stop":
			require.NotNilf(t, evt.Index, "step %d: content_block_stop missing index", step)
			require.Falsef(t, closed[*evt.Index], "step %d: duplicate content_block_stop on block %d", step, *evt.Index)
			require.Equalf(t, openIdx, *evt.Index, "step %d: content_block_stop on index %d while open block is %d", step, *evt.Index, openIdx)
			closed[*evt.Index] = true
			openIdx = -1
		case "message_delta":
			require.Equalf(t, -1, openIdx, "step %d: message_delta while block %d is still open", step, openIdx)
		case "message_stop":
			require.Equalf(t, -1, openIdx, "step %d: message_stop while block %d is still open", step, openIdx)
			sawMessageStop = true
		}
	}
	require.Equal(t, -1, openIdx, "stream ended with unclosed content block")
}

// TestResponsesParallelToolUse_UpfrontAddedThenInterleavedDeltas verifies that
// when a Responses upstream announces multiple function_call items upfront
// before streaming interleaved argument deltas, each Anthropic tool_use block
// opens, receives its full input_json_delta payload, and closes sequentially.
func TestResponsesParallelToolUse_UpfrontAddedThenInterleavedDeltas(t *testing.T) {
	state := NewResponsesEventToAnthropicState()
	state.Model = "glm-5.2"

	stream := []ResponsesStreamEvent{
		{
			Type:     "response.created",
			Response: &ResponsesResponse{ID: "resp_par_stream", Model: "glm-5.2"},
		},
		{
			Type:        "response.output_item.added",
			OutputIndex: 0,
			Item:        &ResponsesOutput{Type: "reasoning", ID: "rs_1"},
		},
		{
			Type:        "response.reasoning_summary_text.delta",
			OutputIndex: 0,
			Delta:       "Run two Bash checks in parallel.",
		},
		{
			Type:        "response.output_item.done",
			OutputIndex: 0,
			Item:        &ResponsesOutput{Type: "reasoning", ID: "rs_1", EncryptedContent: "sig_enc_123"},
		},
		{
			Type:        "response.output_item.added",
			OutputIndex: 1,
			Item:        &ResponsesOutput{Type: "function_call", CallID: "call_00_bash", Name: "Bash"},
		},
		{
			Type:        "response.output_item.added",
			OutputIndex: 2,
			Item:        &ResponsesOutput{Type: "function_call", CallID: "call_01_bash", Name: "Bash"},
		},
		{
			Type:        "response.function_call_arguments.delta",
			OutputIndex: 1,
			Delta:       `{"command":`,
		},
		{
			Type:        "response.function_call_arguments.delta",
			OutputIndex: 2,
			Delta:       `{"command":`,
		},
		{
			Type:        "response.function_call_arguments.delta",
			OutputIndex: 1,
			Delta:       `"uname -a"}`,
		},
		{
			Type:        "response.function_call_arguments.delta",
			OutputIndex: 2,
			Delta:       `"whoami"}`,
		},
		{
			Type:        "response.function_call_arguments.done",
			OutputIndex: 1,
			Arguments:   `{"command":"uname -a"}`,
		},
		{
			Type:        "response.function_call_arguments.done",
			OutputIndex: 2,
			Arguments:   `{"command":"whoami"}`,
		},
		{
			Type: "response.completed",
			Response: &ResponsesResponse{
				ID:     "resp_par_stream",
				Status: "completed",
				Usage:  &ResponsesUsage{InputTokens: 120, OutputTokens: 45},
			},
		},
	}

	var events []AnthropicStreamEvent
	for i := range stream {
		events = append(events, ResponsesEventToAnthropicEvents(&stream[i], state)...)
	}

	assertStrictAnthropicBlockLifecycle(t, events)

	tools := assembleToolUseBlocks(events)
	require.Len(t, tools, 2)
	require.Equal(t, "call_00_bash", tools[0].ID)
	require.Equal(t, "Bash", tools[0].Name)
	require.JSONEq(t, `{"command":"uname -a"}`, tools[0].Input)
	require.Equal(t, "call_01_bash", tools[1].ID)
	require.Equal(t, "Bash", tools[1].Name)
	require.JSONEq(t, `{"command":"whoami"}`, tools[1].Input)
}

// TestResponsesParallelToolUse_SecondToolFinishesBeforeFirst verifies that when
// a queued sibling tool receives its .done before the active tool finishes, it
// stays buffered without corrupting the active block and flushes once the
// active tool completes, preserving Read pages:"" sanitization.
func TestResponsesParallelToolUse_SecondToolFinishesBeforeFirst(t *testing.T) {
	state := NewResponsesEventToAnthropicState()

	stream := []ResponsesStreamEvent{
		{
			Type:     "response.created",
			Response: &ResponsesResponse{ID: "resp_par_ooo", Model: "glm-5.2"},
		},
		{
			Type:        "response.output_item.added",
			OutputIndex: 0,
			Item:        &ResponsesOutput{Type: "function_call", CallID: "call_read", Name: "Read"},
		},
		{
			Type:        "response.output_item.added",
			OutputIndex: 1,
			Item:        &ResponsesOutput{Type: "function_call", CallID: "call_bash", Name: "Bash"},
		},
		{
			Type:        "response.function_call_arguments.delta",
			OutputIndex: 1,
			Delta:       `{"command":"ls -la"}`,
		},
		{
			Type:        "response.function_call_arguments.done",
			OutputIndex: 1,
			Arguments:   `{"command":"ls -la"}`,
		},
		{
			Type:        "response.function_call_arguments.delta",
			OutputIndex: 0,
			Delta:       `{"file_path":"/etc/hosts",`,
		},
		{
			Type:        "response.function_call_arguments.delta",
			OutputIndex: 0,
			Delta:       `"pages":""}`,
		},
		{
			Type:        "response.function_call_arguments.done",
			OutputIndex: 0,
			Arguments:   `{"file_path":"/etc/hosts","pages":""}`,
		},
		{
			Type:     "response.completed",
			Response: &ResponsesResponse{ID: "resp_par_ooo", Status: "completed"},
		},
	}

	var events []AnthropicStreamEvent
	for i := range stream {
		events = append(events, ResponsesEventToAnthropicEvents(&stream[i], state)...)
	}

	assertStrictAnthropicBlockLifecycle(t, events)

	tools := assembleToolUseBlocks(events)
	require.Len(t, tools, 2)
	require.Equal(t, "Read", tools[0].Name)
	require.JSONEq(t, `{"file_path":"/etc/hosts"}`, tools[0].Input)
	require.Equal(t, "Bash", tools[1].Name)
	require.JSONEq(t, `{"command":"ls -la"}`, tools[1].Input)
}

// TestChatCompletionsParallelToolUse_UpfrontAnnouncementAndInterleavedDeltas
// verifies the direct Chat Completions -> Anthropic streaming bridge under
// upfront multi-tool announcement and interleaved argument deltas.
func TestChatCompletionsParallelToolUse_UpfrontAnnouncementAndInterleavedDeltas(t *testing.T) {
	events := collectAnthropicStreamEvents(t, []string{
		`{"id":"chatcmpl-par","model":"deepseek-v4-pro","choices":[{"index":0,"delta":{"reasoning_content":"Checking two commands."}}]}`,
		`{"id":"chatcmpl-par","model":"deepseek-v4-pro","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_00","type":"function","function":{"name":"Bash","arguments":""}},{"index":1,"id":"call_01","type":"function","function":{"name":"Bash","arguments":""}}]}}]}`,
		`{"id":"chatcmpl-par","model":"deepseek-v4-pro","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"command\":"}},{"index":1,"function":{"arguments":"{\"command\":"}}]}}]}`,
		`{"id":"chatcmpl-par","model":"deepseek-v4-pro","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"pwd\"}"}},{"index":1,"function":{"arguments":"\"id\"}"}}]}}]}`,
		`{"id":"chatcmpl-par","model":"deepseek-v4-pro","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":10,"completion_tokens":20,"total_tokens":30}}`,
	})

	assertStrictAnthropicBlockLifecycle(t, events)

	tools := assembleToolUseBlocks(events)
	require.Len(t, tools, 2)
	require.Equal(t, "call_00", tools[0].ID)
	require.Equal(t, "Bash", tools[0].Name)
	require.JSONEq(t, `{"command":"pwd"}`, tools[0].Input)
	require.Equal(t, "call_01", tools[1].ID)
	require.Equal(t, "Bash", tools[1].Name)
	require.JSONEq(t, `{"command":"id"}`, tools[1].Input)
}

// TestChatCompletionsParallelToolUse_LateNamePreservesOrder verifies that if
// tool 0's name arrives after tool 1's name, tool 1 stays queued behind tool 0
// so tools are emitted sequentially in [0, 1] order with intact inputs.
func TestChatCompletionsParallelToolUse_LateNamePreservesOrder(t *testing.T) {
	events := collectAnthropicStreamEvents(t, []string{
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_0"}]}}]}`,
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":1,"id":"call_1","type":"function","function":{"name":"tool_b","arguments":"{\"b\":2}"}}]}}]}`,
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"type":"function","function":{"name":"tool_a","arguments":"{\"a\":1}"}}]}}]}`,
		`{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
	})

	assertStrictAnthropicBlockLifecycle(t, events)

	tools := assembleToolUseBlocks(events)
	require.Len(t, tools, 2)
	require.Equal(t, "tool_a", tools[0].Name)
	require.JSONEq(t, `{"a":1}`, tools[0].Input)
	require.Equal(t, "tool_b", tools[1].Name)
	require.JSONEq(t, `{"b":2}`, tools[1].Input)
}
