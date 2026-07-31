package apicompat

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func requireReasoningItemID(t *testing.T, id string) {
	t.Helper()
	require.True(t, strings.HasPrefix(id, "rs_"), "reasoning item id %q must use the rs_ prefix", id)
	require.False(t, strings.HasPrefix(id, "item_"), "reasoning item id must not use the generic item_ prefix")
}

func TestAnthropicToResponsesResponse_ReasoningIDPrefix(t *testing.T) {
	resp := AnthropicToResponsesResponse(&AnthropicResponse{
		Model: "claude-sonnet-4-5",
		Content: []AnthropicContentBlock{{
			Type:     "thinking",
			Thinking: "consider the options",
		}},
	})

	require.Len(t, resp.Output, 1)
	require.Equal(t, "reasoning", resp.Output[0].Type)
	requireReasoningItemID(t, resp.Output[0].ID)
}

func TestAnthropicEventToResponses_ReasoningIDPrefixAndLifecycle(t *testing.T) {
	state := NewAnthropicEventToResponsesState()
	idx := 0
	var events []ResponsesStreamEvent
	feed := func(evt *AnthropicStreamEvent) {
		events = append(events, AnthropicEventToResponsesEvents(evt, state)...)
	}

	feed(&AnthropicStreamEvent{Type: "message_start", Message: &AnthropicResponse{ID: "msg_1", Model: "claude-sonnet-4-5"}})
	feed(&AnthropicStreamEvent{Type: "content_block_start", Index: &idx, ContentBlock: &AnthropicContentBlock{Type: "thinking"}})
	feed(&AnthropicStreamEvent{Type: "content_block_delta", Index: &idx, Delta: &AnthropicDelta{Type: "thinking_delta", Thinking: "plan"}})
	feed(&AnthropicStreamEvent{Type: "content_block_stop", Index: &idx})
	feed(&AnthropicStreamEvent{Type: "message_stop"})

	var reasoningID string
	for _, event := range events {
		switch event.Type {
		case "response.output_item.added":
			if event.Item != nil && event.Item.Type == "reasoning" {
				reasoningID = event.Item.ID
				requireReasoningItemID(t, reasoningID)
			}
		case "response.reasoning_summary_text.delta", "response.reasoning_summary_text.done":
			require.NotEmpty(t, reasoningID)
			require.Equal(t, reasoningID, event.ItemID)
		case "response.output_item.done":
			if event.Item != nil && event.Item.Type == "reasoning" {
				require.Equal(t, reasoningID, event.Item.ID)
			}
		case "response.completed":
			require.NotNil(t, event.Response)
			require.Len(t, event.Response.Output, 1)
			require.Equal(t, reasoningID, event.Response.Output[0].ID)
		}
	}
	require.NotEmpty(t, reasoningID)
}

func TestChatCompletionsResponseToResponses_ReasoningIDPrefix(t *testing.T) {
	resp := ChatCompletionsResponseToResponses(&ChatCompletionsResponse{
		Choices: []ChatChoice{{
			Message: ChatMessage{
				Role:             "assistant",
				Content:          json.RawMessage(`"answer"`),
				ReasoningContent: "plan",
			},
		}},
	}, "deepseek-v4-pro", nil, false, nil)

	require.NotEmpty(t, resp.Output)
	require.Equal(t, "reasoning", resp.Output[0].Type)
	requireReasoningItemID(t, resp.Output[0].ID)
}

func TestChatCompletionsChunkToResponses_ReasoningIDPrefixAndLifecycle(t *testing.T) {
	events := collectStreamEvents(t, []string{
		`{"choices":[{"index":0,"delta":{"reasoning_content":"plan"}}]}`,
		`{"choices":[{"index":0,"delta":{"content":"answer"}}]}`,
		`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
	})

	var reasoningID string
	for _, event := range events {
		switch event.Type {
		case "response.output_item.added":
			if event.Item != nil && event.Item.Type == "reasoning" {
				reasoningID = event.Item.ID
				requireReasoningItemID(t, reasoningID)
			}
		case "response.reasoning_summary_part.added",
			"response.reasoning_summary_text.delta",
			"response.reasoning_summary_text.done",
			"response.reasoning_summary_part.done":
			require.NotEmpty(t, reasoningID)
			require.Equal(t, reasoningID, event.ItemID)
		case "response.output_item.done":
			if event.Item != nil && event.Item.Type == "reasoning" {
				require.Equal(t, reasoningID, event.Item.ID)
			}
		case "response.completed":
			require.NotNil(t, event.Response)
			require.NotEmpty(t, event.Response.Output)
			require.Equal(t, "reasoning", event.Response.Output[0].Type)
			require.Equal(t, reasoningID, event.Response.Output[0].ID)
		}
	}
	require.NotEmpty(t, reasoningID)
}
