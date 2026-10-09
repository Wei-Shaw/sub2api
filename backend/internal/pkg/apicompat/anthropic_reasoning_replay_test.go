package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGPTReasoningSurvivesAnthropicToolRoundTrip(t *testing.T) {
	for _, model := range []string{"gpt-6-luna", "gpt-6.1-sol", "gpt-5.3-codex"} {
		t.Run(model, func(t *testing.T) {
			original := &ResponsesResponse{ID: "resp_tool", Model: model, Status: "completed", Output: []ResponsesOutput{{Type: "reasoning", EncryptedContent: "gAAAA_PROVIDER_CIPHERTEXT", Summary: []ResponsesSummary{{Type: "summary_text", Text: "Keep the original requirement."}}}, {Type: "function_call", CallID: "call_read", Name: "Read", Arguments: `{"file_path":"main.go"}`}}}
			reply := ResponsesToAnthropic(original, model)
			assistant, err := json.Marshal(reply.Content)
			require.NoError(t, err)
			next, err := AnthropicToResponses(&AnthropicRequest{Model: model, MaxTokens: 128, Messages: []AnthropicMessage{{Role: "user", Content: json.RawMessage(`"original requirement"`)}, {Role: "assistant", Content: assistant}, {Role: "user", Content: json.RawMessage(`[{"type":"tool_result","tool_use_id":"call_read","content":"package main"}]`)}}})
			require.NoError(t, err)
			var items []ResponsesInputItem
			require.NoError(t, json.Unmarshal(next.Input, &items))
			require.Len(t, items, 4)
			require.Equal(t, "reasoning", items[1].Type)
			require.Equal(t, original.Output[0].EncryptedContent, items[1].EncryptedContent)
			require.Equal(t, "function_call", items[2].Type)
			require.Equal(t, "function_call_output", items[3].Type)
			require.Equal(t, items[2].CallID, items[3].CallID)
			require.NotNil(t, items[1].Summary)
			require.Equal(t, original.Output[0].Summary, *items[1].Summary)
			var wire []map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(next.Input, &wire))
			require.Contains(t, wire[1], "summary")
			for _, i := range []int{0, 2, 3} {
				require.NotContains(t, wire[i], "summary")
			}
		})
	}
}

func TestGrokReasoningReplayFiltersOnlyForeignGPTCiphertext(t *testing.T) {
	for _, model := range []string{"grok-4.5", "xai/grok-4.5"} {
		t.Run(model, func(t *testing.T) {
			next, err := AnthropicToResponses(&AnthropicRequest{Model: model, Messages: []AnthropicMessage{{Role: "assistant", Content: json.RawMessage(`[{"type":"thinking","thinking":"foreign summary","signature":"gAAAA_FOREIGN"},{"type":"thinking","thinking":"local summary","signature":"enc-grok-local"},{"type":"thinking","thinking":"unsigned summary","signature":""},{"type":"text","text":"ok"}]`)}}})
			require.NoError(t, err)
			var items []ResponsesInputItem
			require.NoError(t, json.Unmarshal(next.Input, &items))
			require.Len(t, items, 2)
			require.Equal(t, "reasoning", items[0].Type)
			require.Equal(t, "enc-grok-local", items[0].EncryptedContent)
			require.Equal(t, "message", items[1].Type)
			require.NotContains(t, string(next.Input), "foreign summary")
			require.NotContains(t, string(next.Input), "unsigned summary")
		})
	}
}

func TestAnthropicToResponses_SignatureOnlyReasoningIncludesEmptySummary(t *testing.T) {
	next, err := AnthropicToResponses(&AnthropicRequest{Model: "gpt-6-luna", Messages: []AnthropicMessage{{Role: "assistant", Content: json.RawMessage(`[{"type":"thinking","thinking":"","signature":"gAAAA_CIPHERTEXT"}]`)}}})
	require.NoError(t, err)
	var wire []map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(next.Input, &wire))
	require.Len(t, wire, 1)
	require.Equal(t, `[]`, string(wire[0]["summary"]))
	require.Equal(t, `"gAAAA_CIPHERTEXT"`, string(wire[0]["encrypted_content"]))
}
