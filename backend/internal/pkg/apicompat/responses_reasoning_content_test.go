package apicompat

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResponsesReasoningContentFallback(t *testing.T) {
	tests := []struct {
		name      string
		reasoning string
		want      string
		signature string
	}{
		{
			name:      "empty_summary",
			reasoning: `{"type":"reasoning","summary":[],"content":[{"type":"reasoning_text","text":"think "},{"type":"reasoning_text","text":"more"}],"encrypted_content":null}`,
			want:      "think more",
		},
		{
			name:      "missing_summary",
			reasoning: `{"type":"reasoning","content":[{"type":"reasoning_text","text":"raw reasoning"}]}`,
			want:      "raw reasoning",
		},
		{
			name:      "empty_summary_text",
			reasoning: `{"type":"reasoning","summary":[{"type":"summary_text","text":""}],"content":[{"type":"reasoning_text","text":"raw reasoning"}]}`,
			want:      "raw reasoning",
		},
		{
			name:      "summary_takes_precedence",
			reasoning: `{"type":"reasoning","summary":[{"type":"summary_text","text":"short "},{"type":"summary_text","text":"summary"}],"content":[{"type":"reasoning_text","text":"full reasoning"}],"encrypted_content":"opaque-signature"}`,
			want:      "short summary",
			signature: "opaque-signature",
		},
		{
			name:      "only_reasoning_text_is_used",
			reasoning: `{"type":"reasoning","summary":null,"content":[{"type":"output_text","text":"not reasoning"},{"type":"reasoning_text","text":"reasoning"}]}`,
			want:      "reasoning",
		},
		{
			name:      "signature_only",
			reasoning: `{"type":"reasoning","content":[{"type":"output_text","text":"not reasoning"}],"encrypted_content":"opaque-signature"}`,
			signature: "opaque-signature",
		},
		{
			name:      "no_plaintext_or_signature",
			reasoning: `{"type":"reasoning","content":[{"type":"output_text","text":"not reasoning"}]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var resp ResponsesResponse
			require.NoError(t, json.Unmarshal([]byte(`{"id":"resp_reasoning","status":"completed","output":[`+tt.reasoning+`,{"type":"message","role":"assistant","content":[{"type":"output_text","text":"answer"}]}],"usage":{"input_tokens":20,"output_tokens":15,"total_tokens":35,"output_tokens_details":{"reasoning_tokens":10}}}`), &resp))

			chat := ResponsesToChatCompletions(&resp, "reasoning-model")
			require.Len(t, chat.Choices, 1)
			require.Equal(t, tt.want, chat.Choices[0].Message.ReasoningContent)
			require.JSONEq(t, `"answer"`, string(chat.Choices[0].Message.Content))
			require.Equal(t, "stop", chat.Choices[0].FinishReason)
			require.Equal(t, 15, chat.Usage.CompletionTokens)
			require.Equal(t, 10, chat.Usage.CompletionTokensDetails.ReasoningTokens)

			anthropic := ResponsesToAnthropic(&resp, "reasoning-model")
			if tt.want != "" || tt.signature != "" {
				require.Len(t, anthropic.Content, 2)
				require.Equal(t, AnthropicContentBlock{Type: "thinking", Thinking: tt.want, Signature: tt.signature}, anthropic.Content[0])
			} else {
				require.Len(t, anthropic.Content, 1)
			}
			require.Equal(t, AnthropicContentBlock{Type: "text", Text: "answer"}, anthropic.Content[len(anthropic.Content)-1])
			require.Equal(t, "end_turn", *anthropic.StopReason)
			require.Equal(t, 15, anthropic.Usage.OutputTokens)
		})
	}
}

func TestResponsesReasoningContentFallbackPerItem(t *testing.T) {
	var resp ResponsesResponse
	require.NoError(t, json.Unmarshal([]byte(`{"status":"completed","output":[
		{"type":"reasoning","summary":[{"type":"summary_text","text":"summary "}],"content":[{"type":"reasoning_text","text":"ignored"}]},
		{"type":"reasoning","summary":[],"content":[{"type":"reasoning_text","text":"raw reasoning"}],"encrypted_content":"signature"},
		{"type":"function_call","call_id":"call_tool","name":"lookup","arguments":"{}"}
	]}`), &resp))

	chat := ResponsesToChatCompletions(&resp, "reasoning-model")
	require.Equal(t, "summary raw reasoning", chat.Choices[0].Message.ReasoningContent)
	require.Equal(t, "tool_calls", chat.Choices[0].FinishReason)
	require.Len(t, chat.Choices[0].Message.ToolCalls, 1)
	require.Equal(t, "call_tool", chat.Choices[0].Message.ToolCalls[0].ID)

	anthropic := ResponsesToAnthropic(&resp, "reasoning-model")
	require.Len(t, anthropic.Content, 3)
	require.Equal(t, "summary ", anthropic.Content[0].Thinking)
	require.Equal(t, "raw reasoning", anthropic.Content[1].Thinking)
	require.Equal(t, "signature", anthropic.Content[1].Signature)
	require.Equal(t, "tool_use", anthropic.Content[2].Type)
	require.Equal(t, "lookup", anthropic.Content[2].Name)
	require.Equal(t, "tool_use", *anthropic.StopReason)

	// The fallback must preserve the same bytes as the existing streamed deltas.
	state := NewResponsesEventToChatState()
	var streamed strings.Builder
	for _, delta := range []string{"summary ", "raw reasoning"} {
		for _, chunk := range ResponsesEventToChatChunks(&ResponsesStreamEvent{Type: "response.reasoning_text.delta", Delta: delta}, state) {
			for _, choice := range chunk.Choices {
				if choice.Delta.ReasoningContent != nil {
					_, _ = streamed.WriteString(*choice.Delta.ReasoningContent)
				}
			}
		}
	}
	require.Equal(t, streamed.String(), chat.Choices[0].Message.ReasoningContent)
}
