package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAnthropicToResponsesDestinationReasoningReplay(t *testing.T) {
	for _, tc := range []struct {
		name   string
		policy ReasoningReplayPolicy
		want   []string
	}{
		{"disabled", ReasoningReplayDisabled, nil},
		{"openai", ReasoningReplayOpenAILegacy, []string{"gAAAA-openai"}},
		{"grok", ReasoningReplayGrokLegacy, []string{"enc-rs-1", "opaque-unknown"}},
		{"unknown policy", ReasoningReplayPolicy(255), nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Deliberately a Claude model: destination options, not spelling, decide.
			req := &AnthropicRequest{Model: "claude-opus-5-5", MaxTokens: 32, Messages: []AnthropicMessage{
				{Role: "assistant", Content: json.RawMessage(`[{"type":"thinking","thinking":"native","signature":"CAQS-native"},{"type":"thinking","signature":"gAAAA-openai"},{"type":"thinking","signature":"anthropic-thinking-v1:payload"},{"type":"thinking","signature":"enc-rs-1"},{"type":"thinking","signature":"opaque-unknown"},{"type":"thinking","signature":"  "},{"type":"redacted_thinking","data":"private"},{"type":"text","text":"visible"},{"type":"tool_use","id":"toolu_original","name":"lookup","input":{"n":9007199254740993}}]`)},
				{Role: "user", Content: json.RawMessage(`[{"type":"tool_result","tool_use_id":"toolu_original","content":"result"}]`)},
			}}
			resp, err := AnthropicToResponsesWithOptions(req, AnthropicToResponsesOptions{ReasoningReplayPolicy: tc.policy})
			require.NoError(t, err)
			var items []ResponsesInputItem
			require.NoError(t, json.Unmarshal(resp.Input, &items))
			var signatures []string
			for _, item := range items {
				if item.Type == "reasoning" {
					signatures = append(signatures, item.EncryptedContent)
					require.Equal(t, `[]`, string(item.Summary), "replayed reasoning requires an explicit empty summary array")
				} else {
					require.Empty(t, item.Summary, "non-reasoning items must not acquire summary fields")
				}
			}
			require.Equal(t, tc.want, signatures)
			n := len(tc.want)
			require.Len(t, items, n+3)
			require.Equal(t, "message", items[n].Type)
			require.Contains(t, string(items[n].Content), "visible")
			require.Equal(t, "function_call", items[n+1].Type)
			require.Equal(t, "toolu_original", items[n+1].CallID)
			require.JSONEq(t, `{"n":9007199254740993}`, items[n+1].Arguments)
			require.Equal(t, "function_call_output", items[n+2].Type)
			require.Equal(t, "toolu_original", items[n+2].CallID)
			require.Equal(t, "result", items[n+2].Output)
		})
	}
}

func TestResponsesReasoningOutputSignaturesRemainRaw(t *testing.T) {
	for _, sig := range []string{"gAAAA-output", "enc-rs-1", "CAQS-output"} {
		out := ResponsesToAnthropic(&ResponsesResponse{Output: []ResponsesOutput{{Type: "reasoning", EncryptedContent: sig}}}, "claude-opus-5-5")
		require.Len(t, out.Content, 1)
		require.Equal(t, sig, out.Content[0].Signature)
		state := NewResponsesEventToAnthropicState()
		ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{Type: "response.output_item.added", OutputIndex: 0, Item: &ResponsesOutput{Type: "reasoning", ID: "rs_1"}}, state)
		events := ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{Type: "response.output_item.done", OutputIndex: 0, Item: &ResponsesOutput{Type: "reasoning", ID: "rs_1", EncryptedContent: sig}}, state)
		found := false
		for _, event := range events {
			b, _ := json.Marshal(event)
			if string(b) != "" {
				var v map[string]any
				_ = json.Unmarshal(b, &v)
				if delta, ok := v["delta"].(map[string]any); ok && delta["signature"] == sig {
					found = true
				}
			}
		}
		require.True(t, found, "raw signature_delta must remain unchanged")
	}
}

func TestAnthropicToResponsesReasoningReplayPreservesInterleavedTextTools(t *testing.T) {
	req := &AnthropicRequest{Model: "gpt-6.1-sol", Messages: []AnthropicMessage{{Role: "assistant", Content: json.RawMessage(`[{"type":"text","text":"before"},{"type":"tool_use","id":"call_1","name":"first","input":{}},{"type":"text","text":"between"},{"type":"thinking","signature":"CAQS-native"},{"type":"tool_use","id":"call_2","name":"second","input":{}},{"type":"text","text":"after"}]`)}}}
	resp, err := AnthropicToResponsesWithOptions(req, AnthropicToResponsesOptions{ReasoningReplayPolicy: ReasoningReplayOpenAILegacy})
	require.NoError(t, err)
	var items []ResponsesInputItem
	require.NoError(t, json.Unmarshal(resp.Input, &items))
	require.Len(t, items, 5)
	require.Contains(t, string(items[0].Content), "before")
	require.Equal(t, "call_1", items[1].CallID)
	require.Contains(t, string(items[2].Content), "between")
	require.Equal(t, "call_2", items[3].CallID)
	require.Contains(t, string(items[4].Content), "after")
}
