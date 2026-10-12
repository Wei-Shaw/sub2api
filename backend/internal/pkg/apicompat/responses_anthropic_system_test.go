package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResponsesInlineSystemPlacement(t *testing.T) {
	for _, tc := range []struct {
		name, input string
		roles       []string
	}{
		{"before user", `[{"role":"user","content":"q1"},{"role":"assistant","content":"a1"},{"role":"developer","content":"policy2"},{"role":"user","content":"q2"}]`, []string{"user", "assistant", "user", "system"}},
		{"after user", `[{"role":"user","content":"q1"},{"role":"developer","content":"policy2"}]`, []string{"user", "system"}},
		{"tool interruption", `[{"role":"user","content":"q1"},{"type":"function_call","call_id":"call_1","name":"lookup","arguments":"{}"},{"role":"developer","content":"policy2"},{"type":"function_call_output","call_id":"call_1","output":"result"}]`, []string{"user", "assistant", "user", "system"}},
		{"history", `[{"role":"user","content":"q1"},{"role":"developer","content":"policy2"},{"role":"assistant","content":"a1"},{"role":"user","content":"q2"}]`, []string{"user", "system", "assistant", "user"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := &ResponsesRequest{Model: "claude-opus-5-5", Instructions: "initial policy", Input: json.RawMessage(tc.input)}
			out, err := ResponsesToAnthropicRequestWithOptions(req, ResponsesAnthropicOptions{MidConversationSystem: true})
			require.NoError(t, err)
			require.JSONEq(t, `"initial policy"`, string(out.System))
			var roles []string
			found := 0
			for _, m := range out.Messages {
				roles = append(roles, m.Role)
				if m.Role == "system" {
					found++
					require.Equal(t, "policy2", parseContentBlocks(m.Content)[0].Text)
				}
			}
			require.Equal(t, tc.roles, roles)
			require.Equal(t, 1, found)
		})
	}
}

func TestResponsesInlineSystemKeepsHistoricalPrefix(t *testing.T) {
	input := []ResponsesInputItem{{Role: "developer", Content: json.RawMessage(`"initial"`)}, {Role: "user", Content: json.RawMessage(`"q1"`)}}
	var previous *AnthropicRequest
	for i := 0; i < 5; i++ {
		raw, err := json.Marshal(input)
		require.NoError(t, err)
		out, err := ResponsesToAnthropicRequestWithOptions(&ResponsesRequest{Model: "claude-opus-5-5", Input: raw}, ResponsesAnthropicOptions{MidConversationSystem: true})
		require.NoError(t, err)
		if previous != nil {
			require.Equal(t, previous.System, out.System)
			require.Equal(t, previous.Messages, out.Messages[:len(previous.Messages)])
		}
		previous = out
		input = append(input, ResponsesInputItem{Role: "assistant", Content: json.RawMessage(`"answer"`)}, ResponsesInputItem{Role: "developer", Content: json.RawMessage(`"new instruction"`)}, ResponsesInputItem{Role: "user", Content: json.RawMessage(`"next"`)})
	}
}

func TestResponsesInlineSystemRejectsUnrepresentablePlacement(t *testing.T) {
	req := &ResponsesRequest{Model: "claude-opus-5-5", Input: json.RawMessage(`[{"role":"user","content":"q"},{"role":"assistant","content":"a"},{"role":"developer","content":"new instruction"}]`)}
	_, err := ResponsesToAnthropicRequestWithOptions(req, ResponsesAnthropicOptions{MidConversationSystem: true})
	require.ErrorContains(t, err, "requires user input or tool results")
	legacy, err := ResponsesToAnthropicRequest(req)
	require.NoError(t, err)
	require.JSONEq(t, `"new instruction"`, string(legacy.System))
}
