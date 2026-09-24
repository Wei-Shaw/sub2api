package apicompat

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeResponsesAgentMessages(t *testing.T) {
	request := map[string]any{
		"input": []any{
			map[string]any{
				"type":      "agent_message",
				"author":    "worker",
				"recipient": "root",
				"content": []any{
					map[string]any{"type": "output_text", "text": "done"},
					map[string]any{"type": "encrypted_content", "encrypted_content": "cipher"},
				},
			},
			map[string]any{"type": "function_call", "name": "exec"},
		},
	}

	require.True(t, NormalizeResponsesAgentMessages(request))
	items, ok := request["input"].([]any)
	require.True(t, ok)
	require.Len(t, items, 2)
	message, ok := items[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "message", message["type"])
	require.Equal(t, "user", message["role"])
	require.NotContains(t, message, "author")
	require.NotContains(t, message, "recipient")
	content, ok := message["content"].([]any)
	require.True(t, ok)
	part, ok := content[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "done\ncipher", part["text"])
}

func TestNormalizeResponsesAgentMessagesDropsEmptyItems(t *testing.T) {
	request := map[string]any{"input": []any{
		map[string]any{"type": "agent_message", "content": "  "},
		map[string]any{"type": "message", "role": "user", "content": "keep"},
	}}

	require.True(t, NormalizeResponsesAgentMessages(request))
	require.Equal(t, []any{map[string]any{"type": "message", "role": "user", "content": "keep"}}, request["input"])
}
