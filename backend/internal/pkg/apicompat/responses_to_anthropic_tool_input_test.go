package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAnthropicToolUseInput(t *testing.T) {
	tests := []struct {
		name     string
		itemType string
		raw      string
		want     string
	}{
		{"object kept", "function_call", "{\"a\":1}", "{\"a\":1}"},
		{"object with padding kept", "function_call", " {\"a\": 1}\n", " {\"a\": 1}\n"},
		{"truncated function arguments", "function_call", "{\"a\":1", "{}"},
		{"empty function arguments", "function_call", "", "{}"},
		{"non-object function arguments", "function_call", "[1]", "{}"},
		{"custom input wrapped", "custom_tool_call", "dir /b", "{\"input\":\"dir /b\"}"},
		{"empty custom input is an empty object", "custom_tool_call", "", "{}"},
		{"custom input that is an object kept", "custom_tool_call", "{\"a\":1}", "{\"a\":1}"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := anthropicToolUseInput(tc.itemType, "exec", tc.raw)
			require.Equal(t, tc.want, string(got))
			require.True(t, json.Valid(got))
		})
	}
}

// The wrapper must round-trip through the helper that undoes the custom tool
// downgrade, including text that is itself valid JSON or has outer whitespace.
func TestAnthropicToolUseInputCustomRoundTrip(t *testing.T) {
	for _, text := range []string{"dir", "  padded text\n", "", "\"quoted\"", "[1,2]", "line1\nline2"} {
		wrapped := anthropicToolUseInput("custom_tool_call", "exec", text)
		require.Equal(t, text, extractCustomToolCallInput(string(wrapped)), text)
	}
}

func TestResponsesToAnthropicInvalidToolInputStillMarshals(t *testing.T) {
	resp := &ResponsesResponse{ID: "resp_1", Status: "incomplete", IncompleteDetails: &ResponsesIncompleteDetails{Reason: "max_output_tokens"}, Output: []ResponsesOutput{
		{Type: "reasoning", Summary: []ResponsesSummary{{Type: "summary_text", Text: "plan"}}},
		{Type: "function_call", CallID: "call_1", Name: "get", Arguments: "{\"a\":1"},
		{Type: "custom_tool_call", CallID: "call_2", Name: "exec", Input: "dir"},
	}, RenderCustomToolCalls: true}
	out := ResponsesToAnthropic(resp, "m")
	body, err := json.Marshal(out)
	require.NoError(t, err)
	require.Len(t, out.Content, 3)
	require.Equal(t, "thinking", out.Content[0].Type)
	require.Equal(t, "{}", string(out.Content[1].Input))
	require.Equal(t, "{\"input\":\"dir\"}", string(out.Content[2].Input))
	require.Equal(t, "max_tokens", *out.StopReason, string(body))
}

func TestResponsesToChatCompletionsRendersCustomToolCall(t *testing.T) {
	resp := &ResponsesResponse{ID: "resp_1", Status: "completed", Output: []ResponsesOutput{
		{Type: "custom_tool_call", CallID: "call_1", Name: "exec", Input: "dir"},
	}, RenderCustomToolCalls: true}
	out := ResponsesToChatCompletions(resp, "m")
	require.Len(t, out.Choices[0].Message.ToolCalls, 1)
	require.Equal(t, "dir", out.Choices[0].Message.ToolCalls[0].Function.Arguments)
	require.Equal(t, "tool_calls", out.Choices[0].FinishReason)
}

// An output the upstream delivered keeps custom_tool_call items unrendered.
func TestResponsesToAnthropicAndChatSkipDeliveredCustomToolCall(t *testing.T) {
	resp := &ResponsesResponse{ID: "resp_1", Status: "completed", Output: []ResponsesOutput{
		{Type: "custom_tool_call", CallID: "xs_123", Name: "x_keyword_search", Input: "{}"},
	}}
	require.Len(t, ResponsesToAnthropic(resp, "m").Content, 1)
	require.Equal(t, "text", ResponsesToAnthropic(resp, "m").Content[0].Type)
	chat := ResponsesToChatCompletions(resp, "m")
	require.Empty(t, chat.Choices[0].Message.ToolCalls)
	require.Equal(t, "stop", chat.Choices[0].FinishReason)
}
