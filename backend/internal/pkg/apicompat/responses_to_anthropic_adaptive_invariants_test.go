package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// Regression test for upstream 400:
// "This model does not support assistant message prefill. The conversation must end with a user message."
// Happens when Codex reconnects/continues after an assistant commentary message
// (or when a trailing unanswered function_call is stripped by tool pairing,
// leaving the preceding assistant text/commentary as the final message).
func TestResponsesToAnthropic_AdaptiveModelNeverEndsWithAssistantPrefill(t *testing.T) {
	thinkingBlock := AnthropicContentBlock{Type: "thinking", Thinking: "let me check", Signature: "sig_1"}
	resp := AnthropicToResponsesResponse(&AnthropicResponse{
		Model: "claude-opus-5-5",
		Content: []AnthropicContentBlock{
			thinkingBlock,
			{Type: "text", Text: "Red. Now the implementation and the log call before the upstream send."},
		},
	})
	rawOut, err := json.Marshal(resp.Output)
	require.NoError(t, err)

	var items []ResponsesInputItem
	require.NoError(t, json.Unmarshal(rawOut, &items))
	items = append([]ResponsesInputItem{{
		Type:    "message",
		Role:    "user",
		Content: json.RawMessage(`[{"type":"input_text","text":"ok 2"}]`),
	}}, items...)
	inputRaw, err := json.Marshal(items)
	require.NoError(t, err)

	converted, err := ResponsesToAnthropicRequest(&ResponsesRequest{
		Model: "claude-opus-5-5",
		Input: inputRaw,
	})
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(converted.Messages), 3)
	last := converted.Messages[len(converted.Messages)-1]
	require.Equal(t, "user", last.Role, "adaptive Claude models reject trailing assistant messages as prefill")
	requireAnthropicMessagesAreSendable(t, converted.Messages)
	assertAnthropicPairing(t, converted.Messages)
}

// Regression test for upstream 400:
// "messages.N: The final block in an assistant message cannot be `thinking`."
// Happens when an assistant turn had signed thinking + function_call, and the
// function_call was unanswered (cancelled/interrupted) so normalizeAnthropicToolPairing
// dropped the tool_use block, leaving an assistant message whose final (or only)
// block is thinking/redacted_thinking.
func TestResponsesToAnthropic_StripsTrailingThinkingFromAssistantMessages(t *testing.T) {
	thinkingBlock := AnthropicContentBlock{Type: "thinking", Thinking: "plan", Signature: "sig_1"}
	resp := AnthropicToResponsesResponse(&AnthropicResponse{
		Model: "claude-opus-5-5",
		Content: []AnthropicContentBlock{
			thinkingBlock,
			{Type: "tool_use", ID: "call_unanswered", Name: "exec_command", Input: json.RawMessage(`{"cmd":"ls"}`)},
		},
	})
	rawOut, err := json.Marshal(resp.Output)
	require.NoError(t, err)

	var turnItems []ResponsesInputItem
	require.NoError(t, json.Unmarshal(rawOut, &turnItems))

	// Case 1: thinking-only assistant turn (after unanswered tool_use is dropped)
	// between two user messages -> the thinking-only assistant message is dropped
	// and the surrounding user messages merge cleanly.
	items := []ResponsesInputItem{
		{Type: "message", Role: "user", Content: json.RawMessage(`[{"type":"input_text","text":"first"}]`)},
	}
	items = append(items, turnItems...)
	items = append(items, ResponsesInputItem{
		Type: "message", Role: "user", Content: json.RawMessage(`[{"type":"input_text","text":"second"}]`),
	})
	inputRaw, err := json.Marshal(items)
	require.NoError(t, err)

	converted, err := ResponsesToAnthropicRequest(&ResponsesRequest{
		Model: "claude-opus-5-5",
		Input: inputRaw,
	})
	require.NoError(t, err)
	requireAnthropicMessagesAreSendable(t, converted.Messages)
	assertAnthropicPairing(t, converted.Messages)
	for i, m := range converted.Messages {
		if m.Role != "assistant" {
			continue
		}
		blocks := parseContentBlocks(m.Content)
		require.NotEmpty(t, blocks)
		lastBlock := blocks[len(blocks)-1]
		require.NotContainsf(t, []string{"thinking", "redacted_thinking"}, lastBlock.Type,
			"messages[%d] ends with %s block", i, lastBlock.Type)
	}
	require.Equal(t, "user", converted.Messages[len(converted.Messages)-1].Role)

	// Case 2: assistant turn has text followed by thinking + unanswered tool_use ->
	// trailing thinking is stripped, the text block remains.
	respWithText := AnthropicToResponsesResponse(&AnthropicResponse{
		Model: "claude-opus-5-5",
		Content: []AnthropicContentBlock{
			{Type: "text", Text: "I will check."},
			thinkingBlock,
			{Type: "tool_use", ID: "call_unanswered_2", Name: "exec_command", Input: json.RawMessage(`{"cmd":"pwd"}`)},
		},
	})
	rawWithText, err := json.Marshal(respWithText.Output)
	require.NoError(t, err)
	var textTurnItems []ResponsesInputItem
	require.NoError(t, json.Unmarshal(rawWithText, &textTurnItems))

	items2 := []ResponsesInputItem{
		{Type: "message", Role: "user", Content: json.RawMessage(`[{"type":"input_text","text":"start"}]`)},
	}
	items2 = append(items2, textTurnItems...)
	items2 = append(items2, ResponsesInputItem{
		Type: "message", Role: "user", Content: json.RawMessage(`[{"type":"input_text","text":"go on"}]`),
	})
	inputRaw2, err := json.Marshal(items2)
	require.NoError(t, err)

	converted2, err := ResponsesToAnthropicRequest(&ResponsesRequest{
		Model: "claude-opus-5-5",
		Input: inputRaw2,
	})
	require.NoError(t, err)
	requireAnthropicMessagesAreSendable(t, converted2.Messages)
	assertAnthropicPairing(t, converted2.Messages)
	require.Len(t, converted2.Messages, 3)
	asstBlocks := parseContentBlocks(converted2.Messages[1].Content)
	require.Len(t, asstBlocks, 1)
	require.Equal(t, "text", asstBlocks[0].Type)
}
