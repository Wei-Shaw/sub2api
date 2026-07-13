package apicompat

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResToAnthFuncArgsDelta_ReadToolStreamsDeltas(t *testing.T) {
	state := NewResponsesEventToAnthropicState()
	state.MessageStartSent = true
	state.CurrentBlockType = "tool_use"
	state.CurrentToolName = "Read"
	state.OutputIndexToBlockIdx = map[int]int{0: 0}

	evt := &ResponsesStreamEvent{
		Type:        "response.function_call_arguments.delta",
		OutputIndex: 0,
		Delta:       `{"file_path":"/tmp/test.go"}`,
	}

	events := ResponsesEventToAnthropicEvents(evt, state)

	require.Len(t, events, 1, "Read tool delta must produce content_block_delta")
	assert.Equal(t, "content_block_delta", events[0].Type)
	assert.Equal(t, "input_json_delta", events[0].Delta.Type)
	assert.Equal(t, `{"file_path":"/tmp/test.go"}`, events[0].Delta.PartialJSON)
	assert.True(t, state.CurrentToolHadDelta, "Read deltas should set CurrentToolHadDelta")
}

func TestResToAnthFuncArgsDelta_ReadToolWithoutDone(t *testing.T) {
	state := NewResponsesEventToAnthropicState()
	state.MessageStartSent = true
	state.ContentBlockIndex = 0
	state.ContentBlockOpen = true
	state.CurrentBlockType = "tool_use"
	state.CurrentToolName = "Read"
	state.OutputIndexToBlockIdx = map[int]int{0: 0}

	delta := &ResponsesStreamEvent{
		Type:        "response.function_call_arguments.delta",
		OutputIndex: 0,
		Delta:       `{"file_path":"/tmp/test.go"}`,
	}
	events := ResponsesEventToAnthropicEvents(delta, state)
	require.Len(t, events, 1, "delta should be streamed")

	completed := &ResponsesStreamEvent{
		Type: "response.completed",
		Response: &ResponsesResponse{
			Status: "completed",
		},
	}
	events = ResponsesEventToAnthropicEvents(completed, state)

	hasStop := false
	for _, e := range events {
		if e.Type == "content_block_stop" {
			hasStop = true
		}
	}
	assert.True(t, hasStop, "block should be closed even without .done event")
}

func TestResToAnthFuncArgsDelta_NonReadToolUnchanged(t *testing.T) {
	state := NewResponsesEventToAnthropicState()
	state.MessageStartSent = true
	state.CurrentBlockType = "tool_use"
	state.CurrentToolName = "Write"
	state.OutputIndexToBlockIdx = map[int]int{0: 0}

	evt := &ResponsesStreamEvent{
		Type:        "response.function_call_arguments.delta",
		OutputIndex: 0,
		Delta:       `{"file_path":"/tmp/out.txt","content":"hello"}`,
	}

	events := ResponsesEventToAnthropicEvents(evt, state)

	require.Len(t, events, 1)
	assert.Equal(t, "content_block_delta", events[0].Type)
	assert.True(t, state.CurrentToolHadDelta)
}
