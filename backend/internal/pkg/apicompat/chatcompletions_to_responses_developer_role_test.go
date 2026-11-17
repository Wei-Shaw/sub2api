package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// The Responses API accepts role "developer" in input messages. Converting it
// to "user" silently demotes the caller's instructions to user turns.
func TestChatCompletionsToResponses_DeveloperRolePreserved(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		wantContent string
	}{
		{name: "string content", content: `"Answer in French."`, wantContent: `"Answer in French."`},
		{name: "text parts", content: `[{"type":"text","text":"Answer in French."}]`, wantContent: `[{"type":"input_text","text":"Answer in French."}]`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &ChatCompletionsRequest{
				Model: "gpt-4o",
				Messages: []ChatMessage{
					{Role: "developer", Content: json.RawMessage(tt.content)},
					{Role: "user", Content: json.RawMessage(`"Hello"`)},
				},
			}

			resp, err := ChatCompletionsToResponses(req)
			require.NoError(t, err)

			var items []ResponsesInputItem
			require.NoError(t, json.Unmarshal(resp.Input, &items))
			require.Len(t, items, 2)
			require.Equal(t, "message", items[0].Type)
			require.Equal(t, "developer", items[0].Role)
			require.JSONEq(t, tt.wantContent, string(items[0].Content))
			require.Equal(t, "user", items[1].Role)
		})
	}
}
