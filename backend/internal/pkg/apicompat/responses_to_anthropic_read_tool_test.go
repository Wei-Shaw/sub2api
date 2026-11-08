package apicompat

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResToAnthFuncArgsDelta_ReadToolWaitsForCompleteJSON(t *testing.T) {
	state := NewResponsesEventToAnthropicState()
	state.MessageStartSent = true
	state.ContentBlockOpen = true
	state.CurrentBlockType = "tool_use"
	state.CurrentToolName = "Read"
	state.OutputIndexToBlockIdx = map[int]int{0: 0}

	events := ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{
		Type:        "response.function_call_arguments.delta",
		OutputIndex: 0,
		Delta:       `{"file_path":"/tmp/te`,
	}, state)
	assert.Empty(t, events, "partial Read JSON must wait for sanitization")
	assert.False(t, state.CurrentToolHadDelta)

	events = ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{
		Type:        "response.function_call_arguments.delta",
		OutputIndex: 0,
		Delta:       `st.go","pages":""}`,
	}, state)
	require.Len(t, events, 1)
	assert.Equal(t, "content_block_delta", events[0].Type)
	assert.Equal(t, "input_json_delta", events[0].Delta.Type)
	assert.JSONEq(t, `{"file_path":"/tmp/test.go"}`, events[0].Delta.PartialJSON)
	assert.Equal(t, `{"file_path":"/tmp/test.go","pages":""}`, state.CurrentToolArgs)
	assert.True(t, state.CurrentToolHadDelta)

	events = ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{
		Type:        "response.function_call_arguments.done",
		OutputIndex: 0,
		Arguments:   `{"file_path":"/tmp/test.go","pages":""}`,
	}, state)
	require.Len(t, events, 1)
	assert.Equal(t, "content_block_stop", events[0].Type)

	events = ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{
		Type:        "response.function_call_arguments.done",
		OutputIndex: 0,
		Arguments:   `{"file_path":"/tmp/test.go","pages":""}`,
	}, state)
	assert.Empty(t, events, "duplicate done must be idempotent")
}

func TestResponsesEventToAnthropicEvents_ReadToolWithoutArgumentsDoneClosesOnCompleted(t *testing.T) {
	state := NewResponsesEventToAnthropicState()

	events := ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{
		Type:     "response.created",
		Response: &ResponsesResponse{ID: "resp_read", Model: "gpt-5.5"},
	}, state)
	require.Len(t, events, 1)
	assert.Equal(t, "message_start", events[0].Type)

	events = ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{
		Type:        "response.output_item.added",
		OutputIndex: 0,
		Item:        &ResponsesOutput{Type: "function_call", CallID: "call_read", Name: "Read"},
	}, state)
	require.Len(t, events, 1)
	assert.Equal(t, "content_block_start", events[0].Type)

	events = ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{
		Type:        "response.function_call_arguments.delta",
		OutputIndex: 0,
		Delta:       `{"file_path":"/tmp/test.go","pages":""}`,
	}, state)
	require.Len(t, events, 1)
	assert.Equal(t, "content_block_delta", events[0].Type)
	assert.Equal(t, "input_json_delta", events[0].Delta.Type)
	assert.JSONEq(t, `{"file_path":"/tmp/test.go"}`, events[0].Delta.PartialJSON)

	events = ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{
		Type: "response.completed",
		Response: &ResponsesResponse{
			Status: "completed",
		},
	}, state)
	require.Len(t, events, 3)
	assert.Equal(t, "content_block_stop", events[0].Type)
	assert.Equal(t, "message_delta", events[1].Type)
	assert.Equal(t, "tool_use", events[1].Delta.StopReason)
	assert.Equal(t, "message_stop", events[2].Type)
	assert.Empty(t, FinalizeResponsesAnthropicStream(state), "terminal event already finalized the stream")
}

func TestResToAnthFuncArgsDelta_NonReadToolStreamsPartialJSONImmediately(t *testing.T) {
	state := NewResponsesEventToAnthropicState()
	state.MessageStartSent = true
	state.CurrentBlockType = "tool_use"
	state.CurrentToolName = "Write"
	state.OutputIndexToBlockIdx = map[int]int{0: 0}

	evt := &ResponsesStreamEvent{
		Type:        "response.function_call_arguments.delta",
		OutputIndex: 0,
		Delta:       `{"file_path":"/tmp/out`,
	}

	events := ResponsesEventToAnthropicEvents(evt, state)

	require.Len(t, events, 1)
	assert.Equal(t, "content_block_delta", events[0].Type)
	assert.Equal(t, `{"file_path":"/tmp/out`, events[0].Delta.PartialJSON)
	assert.True(t, state.CurrentToolHadDelta)
}
