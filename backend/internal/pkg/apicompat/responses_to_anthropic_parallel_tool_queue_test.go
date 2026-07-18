package apicompat

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type capturedAnthropicToolBlock struct {
	ID        string
	Name      string
	Arguments string
}

func assertAnthropicBlockLifecycle(
	t *testing.T,
	events []AnthropicStreamEvent,
) []capturedAnthropicToolBlock {
	t.Helper()

	openBlocks := make(map[int]string)
	toolsByBlockIndex := make(map[int]*capturedAnthropicToolBlock)
	var toolOrder []int
	sawMessageStop := false

	for eventPosition, event := range events {
		switch event.Type {
		case "content_block_start":
			require.NotNil(t, event.Index, "start event %d has no index", eventPosition)
			require.NotNil(t, event.ContentBlock, "start event %d has no content block", eventPosition)
			idx := *event.Index
			_, alreadyOpen := openBlocks[idx]
			require.False(t, alreadyOpen, "content block %d was started twice", idx)
			openBlocks[idx] = event.ContentBlock.Type

			if event.ContentBlock.Type == "tool_use" {
				toolsByBlockIndex[idx] = &capturedAnthropicToolBlock{
					ID:   event.ContentBlock.ID,
					Name: event.ContentBlock.Name,
				}
				toolOrder = append(toolOrder, idx)
			}

		case "content_block_delta":
			require.NotNil(t, event.Index, "delta event %d has no index", eventPosition)
			idx := *event.Index
			_, isOpen := openBlocks[idx]
			require.True(t, isOpen, "delta event %d referenced closed or missing block %d", eventPosition, idx)

			if event.Delta != nil && event.Delta.Type == "input_json_delta" {
				tool := toolsByBlockIndex[idx]
				require.NotNil(t, tool, "tool delta event %d referenced non-tool block %d", eventPosition, idx)
				tool.Arguments += event.Delta.PartialJSON
			}

		case "content_block_stop":
			require.NotNil(t, event.Index, "stop event %d has no index", eventPosition)
			idx := *event.Index
			_, isOpen := openBlocks[idx]
			require.True(t, isOpen, "stop event %d referenced closed or missing block %d", eventPosition, idx)
			delete(openBlocks, idx)

		case "message_stop":
			require.Empty(t, openBlocks, "message_stop emitted while content blocks remain open")
			sawMessageStop = true
		}
	}

	require.True(t, sawMessageStop, "stream did not emit message_stop")
	require.Empty(t, openBlocks, "stream ended with content blocks still open")

	tools := make([]capturedAnthropicToolBlock, 0, len(toolOrder))
	for _, idx := range toolOrder {
		tools = append(tools, *toolsByBlockIndex[idx])
	}
	return tools
}

func TestStreamingParallelToolsAreSerializedInAnthropicOrder(t *testing.T) {
	state := NewResponsesEventToAnthropicState()
	var allEvents []AnthropicStreamEvent
	emit := func(event *ResponsesStreamEvent) []AnthropicStreamEvent {
		events := ResponsesEventToAnthropicEvents(event, state)
		allEvents = append(allEvents, events...)
		return events
	}

	emit(&ResponsesStreamEvent{
		Type:     "response.created",
		Response: &ResponsesResponse{ID: "resp_parallel", Model: "grok-4.5"},
	})

	events := emit(&ResponsesStreamEvent{
		Type:        "response.output_item.added",
		OutputIndex: 0,
		Item:        &ResponsesOutput{Type: "reasoning"},
	})
	require.Len(t, events, 1)
	assert.Equal(t, "content_block_start", events[0].Type)

	events = emit(&ResponsesStreamEvent{
		Type:        "response.reasoning_summary_text.delta",
		OutputIndex: 0,
		Delta:       "Checking the workspace.",
	})
	require.Len(t, events, 1, "thinking must remain real-time")
	assert.Equal(t, "thinking_delta", events[0].Delta.Type)

	events = emit(&ResponsesStreamEvent{
		Type:        "response.reasoning_summary_text.done",
		OutputIndex: 0,
	})
	require.Len(t, events, 1)
	assert.Equal(t, "content_block_stop", events[0].Type)

	events = emit(&ResponsesStreamEvent{
		Type:        "response.output_text.delta",
		OutputIndex: 1,
		Delta:       "I will run two tools.",
	})
	require.Len(t, events, 2, "text must remain real-time")
	assert.Equal(t, "text_delta", events[1].Delta.Type)

	events = emit(&ResponsesStreamEvent{
		Type:        "response.output_text.done",
		OutputIndex: 1,
	})
	require.Len(t, events, 1)
	assert.Equal(t, "content_block_stop", events[0].Type)

	events = emit(&ResponsesStreamEvent{
		Type:        "response.output_item.added",
		OutputIndex: 2,
		Item: &ResponsesOutput{
			Type:   "function_call",
			CallID: "call_first",
			Name:   "TaskCreate",
		},
	})
	require.Len(t, events, 1)
	assert.Equal(t, "content_block_start", events[0].Type)
	assert.Equal(t, "call_first", events[0].ContentBlock.ID)

	events = emit(&ResponsesStreamEvent{
		Type:        "response.output_item.added",
		OutputIndex: 3,
		Item: &ResponsesOutput{
			Type:   "function_call",
			CallID: "call_second",
			Name:   "Bash",
		},
	})
	require.Empty(t, events, "a queued tool must not close or start over the active tool")

	events = emit(&ResponsesStreamEvent{
		Type:        "response.function_call_arguments.delta",
		OutputIndex: 2,
		CallID:      "call_first",
		Delta:       `{"subject":"first"}`,
	})
	require.Len(t, events, 1)
	assert.Equal(t, "content_block_delta", events[0].Type)

	events = emit(&ResponsesStreamEvent{
		Type:        "response.function_call_arguments.done",
		OutputIndex: 2,
		CallID:      "call_first",
		Arguments:   `{"subject":"first"}`,
	})
	require.Len(t, events, 2)
	assert.Equal(t, "content_block_stop", events[0].Type)
	assert.Equal(t, "content_block_start", events[1].Type)
	assert.Equal(t, "call_second", events[1].ContentBlock.ID)

	events = emit(&ResponsesStreamEvent{
		Type:        "response.function_call_arguments.delta",
		OutputIndex: 3,
		CallID:      "call_second",
		Delta:       `{"command":"echo second"}`,
	})
	require.Len(t, events, 1)
	assert.Equal(t, "content_block_delta", events[0].Type)

	events = emit(&ResponsesStreamEvent{
		Type:        "response.function_call_arguments.done",
		OutputIndex: 3,
		CallID:      "call_second",
		Arguments:   `{"command":"echo second"}`,
	})
	require.Len(t, events, 1)
	assert.Equal(t, "content_block_stop", events[0].Type)

	events = emit(&ResponsesStreamEvent{
		Type: "response.completed",
		Response: &ResponsesResponse{
			Status: "completed",
			Usage:  &ResponsesUsage{InputTokens: 20, OutputTokens: 10},
		},
	})
	require.Len(t, events, 2)
	assert.Equal(t, "tool_use", events[0].Delta.StopReason)
	assert.Equal(t, "message_stop", events[1].Type)

	tools := assertAnthropicBlockLifecycle(t, allEvents)
	require.Len(t, tools, 2)
	assert.Equal(t, "call_first", tools[0].ID)
	assert.Equal(t, "TaskCreate", tools[0].Name)
	assert.JSONEq(t, `{"subject":"first"}`, tools[0].Arguments)
	assert.Equal(t, "call_second", tools[1].ID)
	assert.Equal(t, "Bash", tools[1].Name)
	assert.JSONEq(t, `{"command":"echo second"}`, tools[1].Arguments)
}

func TestStreamingQueuedToolBuffersArgsAndIgnoresLateBlockDone(t *testing.T) {
	state := NewResponsesEventToAnthropicState()
	var allEvents []AnthropicStreamEvent
	emit := func(event *ResponsesStreamEvent) []AnthropicStreamEvent {
		events := ResponsesEventToAnthropicEvents(event, state)
		allEvents = append(allEvents, events...)
		return events
	}

	emit(&ResponsesStreamEvent{
		Type:     "response.created",
		Response: &ResponsesResponse{ID: "resp_interleaved", Model: "grok-4.5"},
	})
	emit(&ResponsesStreamEvent{
		Type:        "response.output_item.added",
		OutputIndex: 0,
		Item:        &ResponsesOutput{Type: "reasoning"},
	})
	emit(&ResponsesStreamEvent{
		Type:        "response.reasoning_summary_text.delta",
		OutputIndex: 0,
		Delta:       "Planning.",
	})
	emit(&ResponsesStreamEvent{
		Type:        "response.output_text.delta",
		OutputIndex: 1,
		Delta:       "Starting tools.",
	})

	events := emit(&ResponsesStreamEvent{
		Type:        "response.output_item.added",
		OutputIndex: 2,
		Item: &ResponsesOutput{
			Type:   "function_call",
			CallID: "call_active",
			Name:   "TaskCreate",
		},
	})
	require.Len(t, events, 2)
	assert.Equal(t, "content_block_stop", events[0].Type)
	assert.Equal(t, "content_block_start", events[1].Type)

	events = emit(&ResponsesStreamEvent{
		Type:        "response.output_item.added",
		OutputIndex: 3,
		Item: &ResponsesOutput{
			Type:   "function_call",
			CallID: "call_queued",
			Name:   "Bash",
		},
	})
	require.Empty(t, events)

	events = emit(&ResponsesStreamEvent{
		Type:        "response.function_call_arguments.delta",
		OutputIndex: 3,
		CallID:      "call_queued",
		Delta:       `{"command":"queued"}`,
	})
	require.Empty(t, events, "queued tool arguments must stay in the gateway")

	events = emit(&ResponsesStreamEvent{
		Type:        "response.function_call_arguments.done",
		OutputIndex: 3,
		CallID:      "call_queued",
		Arguments:   `{"command":"queued"}`,
	})
	require.Empty(t, events, "a completed queued tool must wait for the active tool")

	for _, lateDone := range []*ResponsesStreamEvent{
		{
			Type:        "response.reasoning_summary_text.done",
			OutputIndex: 0,
		},
		{
			Type:        "response.output_text.done",
			OutputIndex: 1,
		},
		{
			Type:        "response.output_item.done",
			OutputIndex: 0,
			Item:        &ResponsesOutput{Type: "reasoning"},
		},
		{
			Type:        "response.output_item.done",
			OutputIndex: 1,
			Item:        &ResponsesOutput{Type: "message"},
		},
	} {
		require.Empty(t, emit(lateDone), "late thinking/text done must not close the active tool")
		require.True(t, state.ContentBlockOpen)
		assert.Equal(t, "tool_use", state.CurrentBlockType)
		require.NotNil(t, state.ActiveToolOutputIndex)
		assert.Equal(t, 2, *state.ActiveToolOutputIndex)
	}

	events = emit(&ResponsesStreamEvent{
		Type:        "response.function_call_arguments.delta",
		OutputIndex: 2,
		CallID:      "call_active",
		Delta:       `{"subject":"`,
	})
	require.Len(t, events, 1)
	assert.Equal(t, `{"subject":"`, events[0].Delta.PartialJSON)

	events = emit(&ResponsesStreamEvent{
		Type:        "response.function_call_arguments.done",
		OutputIndex: 2,
		CallID:      "call_active",
		Arguments:   `{"subject":"active"}`,
	})
	require.Len(t, events, 5)
	assert.Equal(t, "content_block_delta", events[0].Type, "final arguments should fill the active prefix")
	assert.Equal(t, `active"}`, events[0].Delta.PartialJSON)
	assert.Equal(t, "content_block_stop", events[1].Type)
	assert.Equal(t, "content_block_start", events[2].Type)
	assert.Equal(t, "content_block_delta", events[3].Type)
	assert.Equal(t, `{"command":"queued"}`, events[3].Delta.PartialJSON)
	assert.Equal(t, "content_block_stop", events[4].Type)

	emit(&ResponsesStreamEvent{
		Type: "response.completed",
		Response: &ResponsesResponse{
			Status: "completed",
		},
	})

	tools := assertAnthropicBlockLifecycle(t, allEvents)
	require.Len(t, tools, 2)
	assert.Equal(t, "call_active", tools[0].ID)
	assert.Equal(t, "TaskCreate", tools[0].Name)
	assert.JSONEq(t, `{"subject":"active"}`, tools[0].Arguments)
	assert.Equal(t, "call_queued", tools[1].ID)
	assert.Equal(t, "Bash", tools[1].Name)
	assert.JSONEq(t, `{"command":"queued"}`, tools[1].Arguments)
}

func TestStreamingCompletedFlushesFinalArgumentsForPendingTools(t *testing.T) {
	state := NewResponsesEventToAnthropicState()
	var allEvents []AnthropicStreamEvent
	emit := func(event *ResponsesStreamEvent) []AnthropicStreamEvent {
		events := ResponsesEventToAnthropicEvents(event, state)
		allEvents = append(allEvents, events...)
		return events
	}

	emit(&ResponsesStreamEvent{
		Type:     "response.created",
		Response: &ResponsesResponse{ID: "resp_completed_fallback", Model: "grok-4.5"},
	})
	emit(&ResponsesStreamEvent{
		Type:        "response.output_item.added",
		OutputIndex: 0,
		Item: &ResponsesOutput{
			Type:   "function_call",
			CallID: "call_read",
			Name:   "Read",
		},
	})
	emit(&ResponsesStreamEvent{
		Type:        "response.output_item.added",
		OutputIndex: 1,
		Item: &ResponsesOutput{
			Type:   "function_call",
			CallID: "call_bash",
			Name:   "Bash",
		},
	})

	events := emit(&ResponsesStreamEvent{
		Type:        "response.function_call_arguments.delta",
		OutputIndex: 0,
		CallID:      "call_read",
		Delta:       `{"file_path":"`,
	})
	require.Empty(t, events, "partial Read JSON must wait for terminal argument recovery")

	events = emit(&ResponsesStreamEvent{
		Type:        "response.function_call_arguments.delta",
		OutputIndex: 1,
		CallID:      "call_bash",
		Delta:       `{"command":"`,
	})
	require.Empty(t, events)

	events = emit(&ResponsesStreamEvent{
		Type: "response.completed",
		Response: &ResponsesResponse{
			Status: "completed",
			Output: []ResponsesOutput{
				{
					Type:      "function_call",
					CallID:    "call_read",
					Name:      "Read",
					Arguments: `{"file_path":"README.md"}`,
				},
				{
					Type:      "function_call",
					CallID:    "call_bash",
					Name:      "Bash",
					Arguments: `{"command":"go test ./..."}`,
				},
			},
		},
	})
	require.Len(t, events, 7)
	assert.Equal(t, "content_block_delta", events[0].Type)
	assert.JSONEq(t, `{"file_path":"README.md"}`, events[0].Delta.PartialJSON)
	assert.Equal(t, "content_block_stop", events[1].Type)
	assert.Equal(t, "content_block_start", events[2].Type)
	assert.Equal(t, "content_block_delta", events[3].Type)
	assert.Equal(t, "content_block_stop", events[4].Type)
	assert.Equal(t, "message_delta", events[5].Type)
	assert.Equal(t, "message_stop", events[6].Type)

	tools := assertAnthropicBlockLifecycle(t, allEvents)
	require.Len(t, tools, 2)
	assert.Equal(t, "call_read", tools[0].ID)
	assert.Equal(t, "Read", tools[0].Name)
	assert.JSONEq(t, `{"file_path":"README.md"}`, tools[0].Arguments)
	assert.Equal(t, "call_bash", tools[1].ID)
	assert.Equal(t, "Bash", tools[1].Name)
	assert.JSONEq(t, `{"command":"go test ./..."}`, tools[1].Arguments)
}
