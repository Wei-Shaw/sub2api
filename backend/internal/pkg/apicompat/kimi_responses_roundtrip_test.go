package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResponsesToAnthropicRequest_ReplaysReasoningSignature(t *testing.T) {
	req := &ResponsesRequest{
		Model: "k3-256k",
		Input: json.RawMessage(`[
			{"role":"user","content":"inspect"},
			{"type":"reasoning","summary":[{"type":"summary_text","text":"private thought"}],"encrypted_content":"signed-thinking"},
			{"type":"function_call","call_id":"toolu_1","name":"Read","arguments":"{\"file_path\":\"README.md\"}"},
			{"type":"function_call_output","call_id":"toolu_1","output":"ok"}
		]`),
	}

	got, err := ResponsesToAnthropicRequest(req)
	require.NoError(t, err)
	require.Len(t, got.Messages, 3)
	require.Equal(t, "assistant", got.Messages[1].Role)
	require.NotNil(t, got.Thinking)
	require.Equal(t, "enabled", got.Thinking.Type)

	blocks := parseContentBlocks(got.Messages[1].Content)
	require.Len(t, blocks, 2)
	require.Equal(t, "thinking", blocks[0].Type)
	require.Equal(t, "private thought", blocks[0].Thinking)
	require.Equal(t, "signed-thinking", blocks[0].Signature)
	require.Equal(t, "tool_use", blocks[1].Type)
}

func TestResponsesToAnthropicRequest_DropsForeignAndSummaryOnlyReasoning(t *testing.T) {
	req := &ResponsesRequest{
		Model: "claude-sonnet-4-6",
		Input: json.RawMessage(`[
			{"role":"user","content":"inspect"},
			{"type":"reasoning","summary":[{"type":"summary_text","text":"openai private"}],"encrypted_content":"gAAAA-openai-blob"},
			{"type":"reasoning","summary":[{"type":"summary_text","text":"summary only"}]},
			{"type":"function_call","call_id":"call_1","name":"Read","arguments":"{\"file_path\":\"README.md\"}"},
			{"type":"function_call_output","call_id":"call_1","output":"ok"}
		]`),
	}

	got, err := ResponsesToAnthropicRequest(req)
	require.NoError(t, err)
	require.Len(t, got.Messages, 3)
	require.Nil(t, got.Thinking)

	blocks := parseContentBlocks(got.Messages[1].Content)
	require.Len(t, blocks, 1)
	require.Equal(t, "tool_use", blocks[0].Type)
}

func TestResponsesToAnthropicRequest_EnablesThinkingWhenReplayingSignedHistory(t *testing.T) {
	req := &ResponsesRequest{
		Model: "k3-256k",
		Reasoning: &ResponsesReasoning{
			Effort: "low",
		},
		Input: json.RawMessage(`[
			{"role":"user","content":"hi"},
			{"type":"reasoning","summary":[{"type":"summary_text","text":"plan"}],"encrypted_content":"sig-kimi"},
			{"role":"assistant","content":[{"type":"output_text","text":"hello"}]}
		]`),
	}

	got, err := ResponsesToAnthropicRequest(req)
	require.NoError(t, err)
	require.NotNil(t, got.Thinking)
	require.Equal(t, "enabled", got.Thinking.Type)

	blocks := parseContentBlocks(got.Messages[1].Content)
	require.Equal(t, "thinking", blocks[0].Type)
	require.Equal(t, "sig-kimi", blocks[0].Signature)
	require.Equal(t, "text", blocks[1].Type)
}

func TestAnthropicToResponsesResponse_PreservesThinkingSignature(t *testing.T) {
	resp := AnthropicToResponsesResponse(&AnthropicResponse{
		ID:    "msg_1",
		Model: "k3-256k",
		Content: []AnthropicContentBlock{{
			Type:      "thinking",
			Thinking:  "private thought",
			Signature: "signed-thinking",
		}},
	})

	require.Len(t, resp.Output, 1)
	require.Equal(t, "reasoning", resp.Output[0].Type)
	require.Equal(t, "signed-thinking", resp.Output[0].EncryptedContent)
	require.Equal(t, "private thought", resp.Output[0].Summary[0].Text)
}

func TestAnthropicEventToResponses_PreservesThinkingSignature(t *testing.T) {
	state := NewAnthropicEventToResponsesState()
	var events []ResponsesStreamEvent
	feed := func(evt *AnthropicStreamEvent) {
		events = append(events, AnthropicEventToResponsesEvents(evt, state)...)
	}

	idx := 0
	feed(&AnthropicStreamEvent{Type: "message_start", Message: &AnthropicResponse{ID: "msg_1", Model: "k3-256k"}})
	feed(&AnthropicStreamEvent{Type: "content_block_start", Index: &idx, ContentBlock: &AnthropicContentBlock{Type: "thinking", Signature: "sig-"}})
	feed(&AnthropicStreamEvent{Type: "content_block_delta", Index: &idx, Delta: &AnthropicDelta{Type: "thinking_delta", Thinking: "private thought"}})
	feed(&AnthropicStreamEvent{Type: "content_block_delta", Index: &idx, Delta: &AnthropicDelta{Type: "signature_delta", Signature: "tail"}})
	feed(&AnthropicStreamEvent{Type: "content_block_stop", Index: &idx})
	feed(&AnthropicStreamEvent{Type: "message_delta", Delta: &AnthropicDelta{StopReason: "end_turn"}})
	feed(&AnthropicStreamEvent{Type: "message_stop"})

	var doneItem, terminalItem *ResponsesOutput
	for i := range events {
		if events[i].Type == "response.output_item.done" && events[i].Item != nil {
			doneItem = events[i].Item
		}
		if events[i].Type == "response.completed" && events[i].Response != nil {
			require.Len(t, events[i].Response.Output, 1)
			terminalItem = &events[i].Response.Output[0]
		}
	}

	require.NotNil(t, doneItem)
	require.NotNil(t, terminalItem)
	require.Equal(t, "sig-tail", doneItem.EncryptedContent)
	require.Equal(t, "sig-tail", terminalItem.EncryptedContent)
	require.Equal(t, "private thought", terminalItem.Summary[0].Text)
}

func TestAnthropicEventToResponses_MultipleTextBlocksUseDistinctIndexesAndSkipEmpty(t *testing.T) {
	state := NewAnthropicEventToResponsesState()
	var events []ResponsesStreamEvent
	feed := func(evt *AnthropicStreamEvent) {
		events = append(events, AnthropicEventToResponsesEvents(evt, state)...)
	}

	idx := 0
	feed(&AnthropicStreamEvent{Type: "message_start", Message: &AnthropicResponse{ID: "msg_1", Model: "k3-256k"}})
	feed(&AnthropicStreamEvent{Type: "content_block_start", Index: &idx, ContentBlock: &AnthropicContentBlock{Type: "text"}})
	feed(&AnthropicStreamEvent{Type: "content_block_stop", Index: &idx})
	feed(&AnthropicStreamEvent{Type: "content_block_start", Index: &idx, ContentBlock: &AnthropicContentBlock{Type: "text"}})
	feed(&AnthropicStreamEvent{Type: "content_block_delta", Index: &idx, Delta: &AnthropicDelta{Type: "text_delta", Text: "first"}})
	feed(&AnthropicStreamEvent{Type: "content_block_stop", Index: &idx})
	feed(&AnthropicStreamEvent{Type: "content_block_start", Index: &idx, ContentBlock: &AnthropicContentBlock{Type: "text"}})
	feed(&AnthropicStreamEvent{Type: "content_block_stop", Index: &idx})
	feed(&AnthropicStreamEvent{Type: "content_block_start", Index: &idx, ContentBlock: &AnthropicContentBlock{Type: "text"}})
	feed(&AnthropicStreamEvent{Type: "content_block_delta", Index: &idx, Delta: &AnthropicDelta{Type: "text_delta", Text: "second"}})
	feed(&AnthropicStreamEvent{Type: "content_block_stop", Index: &idx})
	feed(&AnthropicStreamEvent{Type: "message_delta", Delta: &AnthropicDelta{StopReason: "end_turn"}})
	feed(&AnthropicStreamEvent{Type: "message_stop"})

	var addedIndexes []int
	var completed *ResponsesResponse
	for i := range events {
		if events[i].Type == "response.content_part.added" {
			addedIndexes = append(addedIndexes, events[i].ContentIndex)
		}
		if events[i].Type == "response.completed" {
			completed = events[i].Response
		}
	}

	require.Equal(t, []int{0, 1}, addedIndexes)
	require.NotNil(t, completed)
	require.Len(t, completed.Output, 1)
	require.Equal(t, []ResponsesContentPart{
		{Type: "output_text", Text: "first"},
		{Type: "output_text", Text: "second"},
	}, completed.Output[0].Content)
}

func TestFinalizeAnthropicResponsesStream_WithoutStopReasonFails(t *testing.T) {
	state := NewAnthropicEventToResponsesState()
	idx := 0
	AnthropicEventToResponsesEvents(&AnthropicStreamEvent{
		Type: "message_start", Message: &AnthropicResponse{ID: "msg_1", Model: "k3-256k"},
	}, state)
	AnthropicEventToResponsesEvents(&AnthropicStreamEvent{
		Type: "content_block_start", Index: &idx, ContentBlock: &AnthropicContentBlock{Type: "text"},
	}, state)
	AnthropicEventToResponsesEvents(&AnthropicStreamEvent{
		Type: "content_block_delta", Index: &idx, Delta: &AnthropicDelta{Type: "text_delta", Text: "partial"},
	}, state)

	events := FinalizeAnthropicResponsesStream(state)
	require.Len(t, events, 4)
	require.Equal(t, "response.output_text.done", events[0].Type)
	require.Equal(t, "response.content_part.done", events[1].Type)
	require.Equal(t, "response.output_item.done", events[2].Type)
	require.Equal(t, "response.failed", events[3].Type)
	require.NotNil(t, events[3].Response)
	require.Equal(t, "failed", events[3].Response.Status)
	require.Equal(t, "server_error", events[3].Response.Error.Code)
	require.Equal(t, "partial", events[3].Response.Output[0].Content[0].Text)
}
