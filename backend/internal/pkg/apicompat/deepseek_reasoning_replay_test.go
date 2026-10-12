package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDeepSeekChatResponsesReasoningRoundTrip(t *testing.T) {
	req := &ChatCompletionsRequest{Model: "deepseek-v4-flash", Messages: []ChatMessage{
		{Role: "user", Content: json.RawMessage(`"hello"`)},
		{Role: "assistant", ReasoningContent: "first plan", Content: json.RawMessage(`"answer"`)},
		{Role: "user", Content: json.RawMessage(`"check"`)},
		{Role: "assistant", ReasoningContent: "tool plan", ToolCalls: []ChatToolCall{{ID: "call_one", Type: "function", Function: ChatFunctionCall{Name: "lookup", Arguments: "{}"}}}},
		{Role: "tool", ToolCallID: "call_one", Content: json.RawMessage(`"done"`)},
	}}
	converted, err := ChatCompletionsToResponsesWithReasoningReplay(req, true)
	require.NoError(t, err)
	require.Contains(t, string(converted.Input), `"type":"reasoning_text"`)
	require.NotContains(t, string(converted.Input), "<thinking>")
	back, err := ResponsesToChatCompletionsRequest(converted)
	require.NoError(t, err)
	require.Equal(t, req.Messages, back.Messages)
	legacy, err := ChatCompletionsToResponses(req)
	require.NoError(t, err)
	require.Contains(t, string(legacy.Input), `\u003cthinking\u003e`)
	require.Equal(t, "first plan", req.Messages[1].ReasoningContent)
}
