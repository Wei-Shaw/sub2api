package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func replayTestRequest() *ChatCompletionsRequest {
	return &ChatCompletionsRequest{
		Model: "gpt-5.6",
		Messages: []ChatMessage{
			{Role: "user", Content: json.RawMessage(`"list files"`)},
			{
				Role:             "assistant",
				ReasoningContent: "I should list the directory first",
				ToolCalls: []ChatToolCall{
					{ID: "call_a", Type: "function", Function: ChatFunctionCall{Name: "ls", Arguments: `{"path":"."}`}},
					{ID: "call_b", Type: "function", Function: ChatFunctionCall{Name: "cat", Arguments: `{"path":"README"}`}},
				},
			},
			{Role: "tool", ToolCallID: "call_a", Content: json.RawMessage(`"a.txt"`)},
			{Role: "tool", ToolCallID: "call_b", Content: json.RawMessage(`"hello"`)},
		},
	}
}

func decodeReplayInput(t *testing.T, req *ResponsesRequest) []map[string]any {
	t.Helper()
	var items []map[string]any
	require.NoError(t, json.Unmarshal(req.Input, &items))
	return items
}

// A cache hit inserts the encrypted reasoning items directly before the tool
// call they produced and stops echoing the plaintext summary as <thinking>.
func TestChatCompletionsToResponses_ReasoningReplayInsertsItemsBeforeToolCall(t *testing.T) {
	lookups := []string{}
	out, err := ChatCompletionsToResponsesWithOptions(replayTestRequest(), &ChatToResponsesOptions{
		ReasoningByToolCallID: func(callID string) []string {
			lookups = append(lookups, callID)
			switch callID {
			case "call_a":
				return []string{"enc-1", "enc-2"}
			case "call_b":
				return []string{"enc-3"}
			}
			return nil
		},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"call_a", "call_b"}, lookups)

	items := decodeReplayInput(t, out)
	types := make([]string, 0, len(items))
	for _, item := range items {
		typ, _ := item["type"].(string)
		if typ == "" {
			role, _ := item["role"].(string)
			typ = "message:" + role
		}
		types = append(types, typ)
	}
	assert.Equal(t, []string{
		"message:user",
		"reasoning", "reasoning", "function_call",
		"reasoning", "function_call",
		"function_call_output", "function_call_output",
	}, types)

	assert.Equal(t, "enc-1", items[1]["encrypted_content"])
	assert.Equal(t, []any{}, items[1]["summary"], "replayed reasoning must carry an explicit empty summary")
	_, hasID := items[1]["id"]
	assert.False(t, hasID, "replayed reasoning must not carry an rs_ id (store=false upstream 404s on it)")
	assert.Equal(t, "enc-2", items[2]["encrypted_content"])
	assert.Equal(t, "call_a", items[3]["call_id"])
	assert.Equal(t, "enc-3", items[4]["encrypted_content"])
	assert.Equal(t, "call_b", items[5]["call_id"])

	raw := string(out.Input)
	assert.NotContains(t, raw, "<thinking>", "plaintext reasoning summary must not be echoed once real reasoning is restored")
}

// A partial hit still restores what is cached; the assistant text (if any)
// is kept, and only the <thinking> echo is dropped.
func TestChatCompletionsToResponses_ReasoningReplayPartialHitKeepsAssistantText(t *testing.T) {
	req := replayTestRequest()
	req.Messages[1].Content = json.RawMessage(`"Let me look."`)
	out, err := ChatCompletionsToResponsesWithOptions(req, &ChatToResponsesOptions{
		ReasoningByToolCallID: func(callID string) []string {
			if callID == "call_b" {
				return []string{"enc-b"}
			}
			return nil
		},
	})
	require.NoError(t, err)
	items := decodeReplayInput(t, out)

	require.Equal(t, "assistant", items[1]["role"])
	var parts []ResponsesContentPart
	partsRaw, _ := json.Marshal(items[1]["content"])
	require.NoError(t, json.Unmarshal(partsRaw, &parts))
	require.Len(t, parts, 1)
	assert.Equal(t, "Let me look.", parts[0].Text)

	assert.Equal(t, "function_call", items[2]["type"])
	assert.Equal(t, "call_a", items[2]["call_id"])
	assert.Equal(t, "reasoning", items[3]["type"])
	assert.Equal(t, "enc-b", items[3]["encrypted_content"])
	assert.Equal(t, "function_call", items[4]["type"])
	assert.Equal(t, "call_b", items[4]["call_id"])
}

// A miss (or no hook) keeps the pre-existing behavior byte for byte: the
// reasoning_content is echoed as <thinking> and no reasoning item appears.
func TestChatCompletionsToResponses_ReasoningReplayMissKeepsLegacyShape(t *testing.T) {
	legacy, err := ChatCompletionsToResponses(replayTestRequest())
	require.NoError(t, err)

	missed, err := ChatCompletionsToResponsesWithOptions(replayTestRequest(), &ChatToResponsesOptions{
		ReasoningByToolCallID: func(string) []string { return nil },
	})
	require.NoError(t, err)
	assert.JSONEq(t, string(legacy.Input), string(missed.Input))

	blank, err := ChatCompletionsToResponsesWithOptions(replayTestRequest(), &ChatToResponsesOptions{
		ReasoningByToolCallID: func(string) []string { return []string{"", "  "} },
	})
	require.NoError(t, err)
	assert.JSONEq(t, string(legacy.Input), string(blank.Input), "blank payloads must count as a miss")

	items := decodeReplayInput(t, legacy)
	partsRaw, _ := json.Marshal(items[1]["content"])
	var parts []ResponsesContentPart
	require.NoError(t, json.Unmarshal(partsRaw, &parts))
	require.Len(t, parts, 1)
	assert.Equal(t, "<thinking>I should list the directory first</thinking>", parts[0].Text)
	for _, item := range items {
		assert.NotEqual(t, "reasoning", item["type"])
	}
}

// Tool calls without an id are never looked up.
func TestChatCompletionsToResponses_ReasoningReplaySkipsBlankToolCallIDs(t *testing.T) {
	req := replayTestRequest()
	req.Messages[1].ToolCalls[0].ID = " "
	called := 0
	_, err := ChatCompletionsToResponsesWithOptions(req, &ChatToResponsesOptions{
		ReasoningByToolCallID: func(callID string) []string {
			called++
			assert.Equal(t, "call_b", callID)
			return nil
		},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, called)
}
