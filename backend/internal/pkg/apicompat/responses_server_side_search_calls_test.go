package apicompat

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// serverSideSearchStream is the event sequence of one custom_tool_call item
// followed by a text answer, in the shape xAI documents for X Search.
func serverSideSearchStream(callID, name string) []*ResponsesStreamEvent {
	item := ResponsesOutput{Type: "custom_tool_call", ID: "ctc_1", CallID: callID, Name: name, Input: `{"query":"q"}`, Status: "completed"}
	added := ResponsesOutput{Type: "custom_tool_call", ID: "ctc_1", CallID: callID, Name: name}
	message := ResponsesOutput{Type: "message", ID: "msg_1", Role: "assistant", Content: []ResponsesContentPart{{Type: "output_text", Text: "found it"}}}
	return []*ResponsesStreamEvent{
		{Type: "response.created", Response: &ResponsesResponse{ID: "resp_1"}},
		{Type: "response.output_item.added", OutputIndex: 0, Item: &added},
		{Type: "response.custom_tool_call_input.delta", OutputIndex: 0, ItemID: "ctc_1", Delta: `{"query":"q"}`},
		{Type: "response.custom_tool_call_input.done", OutputIndex: 0, ItemID: "ctc_1", Input: `{"query":"q"}`},
		{Type: "response.output_item.done", OutputIndex: 0, Item: &item},
		{Type: "response.output_text.delta", OutputIndex: 1, Delta: "found it"},
		{Type: "response.output_item.done", OutputIndex: 1, Item: &message},
		{Type: "response.completed", Response: &ResponsesResponse{ID: "resp_1", Status: "completed", Output: []ResponsesOutput{item, message}}},
	}
}

func anthropicEventsOf(events []*ResponsesStreamEvent, xai bool) []AnthropicStreamEvent {
	state := NewResponsesEventToAnthropicState()
	state.XAIServerSideSearch = xai
	var out []AnthropicStreamEvent
	for _, event := range events {
		out = append(out, ResponsesEventToAnthropicEvents(event, state)...)
	}
	return out
}

func chatChunksOf(events []*ResponsesStreamEvent, xai bool) []ChatCompletionsChunk {
	state := NewResponsesEventToChatState()
	state.XAIServerSideSearch = xai
	var out []ChatCompletionsChunk
	for _, event := range events {
		out = append(out, ResponsesEventToChatChunks(event, state)...)
	}
	return out
}

func anthropicToolUseStarts(events []AnthropicStreamEvent) int {
	count := 0
	for _, event := range events {
		if event.Type == "content_block_start" && event.ContentBlock != nil && event.ContentBlock.Type == "tool_use" {
			count++
		}
	}
	return count
}

func anthropicStopReason(events []AnthropicStreamEvent) string {
	for _, event := range events {
		if event.Type == "message_delta" && event.Delta != nil {
			return event.Delta.StopReason
		}
	}
	return ""
}

func chatToolCallChunks(chunks []ChatCompletionsChunk) int {
	count := 0
	for _, chunk := range chunks {
		for _, choice := range chunk.Choices {
			count += len(choice.Delta.ToolCalls)
		}
	}
	return count
}

func chatFinishReason(chunks []ChatCompletionsChunk) string {
	for _, chunk := range chunks {
		for _, choice := range chunk.Choices {
			if choice.FinishReason != nil {
				return *choice.FinishReason
			}
		}
	}
	return ""
}

func TestResponsesEventToAnthropicEvents_ServerSideSearchIsNotToolUse(t *testing.T) {
	for _, tt := range []struct {
		name, callID, toolName string
		xai                    bool
		wantToolUse            int
		wantStop               string
	}{
		{"x search", "xs_123", "x_keyword_search", true, 0, "end_turn"},
		{"attachment search", "as_456", "attachment_search", true, 0, "end_turn"},
		{"client custom tool", "call_9", "exec", true, 1, "tool_use"},
		{"x search call id on a non-xAI upstream", "xs_123", "x_keyword_search", false, 1, "tool_use"},
		{"attachment search call id on a non-xAI upstream", "as_456", "attachment_search", false, 1, "tool_use"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			events := anthropicEventsOf(serverSideSearchStream(tt.callID, tt.toolName), tt.xai)
			require.Equal(t, tt.wantToolUse, anthropicToolUseStarts(events))
			require.Equal(t, tt.wantStop, anthropicStopReason(events))
		})
	}
}

func TestResponsesEventToAnthropicEvents_ServerSideSearchKeepsClientToolCall(t *testing.T) {
	stream := serverSideSearchStream("xs_123", "x_keyword_search")
	client := ResponsesOutput{Type: "custom_tool_call", ID: "ctc_2", CallID: "call_9", Name: "exec", Input: "dir"}
	clientAdded := ResponsesOutput{Type: "custom_tool_call", ID: "ctc_2", CallID: "call_9", Name: "exec"}
	mixed := append([]*ResponsesStreamEvent{}, stream[:len(stream)-1]...)
	mixed = append(mixed,
		&ResponsesStreamEvent{Type: "response.output_item.added", OutputIndex: 2, Item: &clientAdded},
		&ResponsesStreamEvent{Type: "response.custom_tool_call_input.delta", OutputIndex: 2, ItemID: "ctc_2", Delta: "dir"},
		&ResponsesStreamEvent{Type: "response.output_item.done", OutputIndex: 2, Item: &client},
		&ResponsesStreamEvent{Type: "response.completed", Response: &ResponsesResponse{ID: "resp_1", Status: "completed"}},
	)

	events := anthropicEventsOf(mixed, true)
	require.Equal(t, 1, anthropicToolUseStarts(events))
	require.Equal(t, "tool_use", anthropicStopReason(events))
}

func TestResponsesEventToChatChunks_ServerSideSearchIsNotToolCall(t *testing.T) {
	for _, tt := range []struct {
		name, callID, toolName string
		xai                    bool
		wantToolCalls          int
		wantFinish             string
	}{
		{"x search", "xs_123", "x_keyword_search", true, 0, "stop"},
		{"attachment search", "as_456", "attachment_search", true, 0, "stop"},
		{"client custom tool", "call_9", "exec", true, 2, "tool_calls"},
		{"x search call id on a non-xAI upstream", "xs_123", "x_keyword_search", false, 2, "tool_calls"},
		{"attachment search call id on a non-xAI upstream", "as_456", "attachment_search", false, 2, "tool_calls"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			chunks := chatChunksOf(serverSideSearchStream(tt.callID, tt.toolName), tt.xai)
			// A client call is announced once and then carries its input delta.
			require.Equal(t, tt.wantToolCalls, chatToolCallChunks(chunks))
			require.Equal(t, tt.wantFinish, chatFinishReason(chunks))
		})
	}
}

func TestBufferedResponseAccumulator_ServerSideSearchIsNotRebuiltAsFunctionCall(t *testing.T) {
	rebuild := func(callID, name string, xai bool) []ResponsesOutput {
		acc := NewBufferedResponseAccumulator()
		acc.XAIServerSideSearch = xai
		for _, event := range serverSideSearchStream(callID, name) {
			if event.Type == "response.completed" {
				continue
			}
			acc.ProcessEvent(event)
		}
		resp := &ResponsesResponse{ID: "resp_1", Status: "completed"}
		acc.SupplementResponseOutput(resp)
		return resp.Output
	}

	client := rebuild("call_9", "exec", true)
	require.Equal(t, "function_call", client[len(client)-1].Type)
	require.Equal(t, "call_9", client[len(client)-1].CallID)

	for _, callID := range []string{"xs_123", "as_456"} {
		notXAI := rebuild(callID, "x_keyword_search", false)
		require.Equal(t, "function_call", notXAI[len(notXAI)-1].Type, "without the xAI switch the item is rebuilt as before")

		output := rebuild(callID, "x_keyword_search", true)
		for _, item := range output {
			require.NotEqual(t, "function_call", item.Type, "a server-side search is not a client function call")
		}
		last := output[len(output)-1]
		require.Equal(t, "custom_tool_call", last.Type)
		require.Equal(t, callID, last.CallID)
		require.Equal(t, `{"query":"q"}`, last.Input)

		resp := &ResponsesResponse{ID: "resp_1", Status: "completed", Output: output}
		message := ResponsesToAnthropic(resp, "m")
		require.Equal(t, "end_turn", *message.StopReason)
		for _, block := range message.Content {
			require.NotEqual(t, "tool_use", block.Type)
		}
		chat := ResponsesToChatCompletions(resp, "m")
		require.Empty(t, chat.Choices[0].Message.ToolCalls)
		require.Equal(t, "stop", chat.Choices[0].FinishReason)
	}
}
