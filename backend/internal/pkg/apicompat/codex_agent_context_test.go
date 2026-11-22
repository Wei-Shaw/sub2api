package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// Chat Completions and Anthropic have no standalone tool output: an agent
// context output must reach them as user text in place, never as a tool
// message or tool_result without a call to pair with.
const codexAgentContextInput = `[
	{"type":"message","role":"user","content":[{"type":"input_text","text":"open the second tab"}]},
	{"type":"function_call_output","id":"fco_tabs","namespace":"agent","name":"browser_tabs","output":"<in-app-browser-context>2 tabs</in-app-browser-context>"},
	{"type":"function_call","call_id":"call_1","name":"exec_command","arguments":"{}"},
	{"type":"function_call_output","call_id":"call_1","output":"done"}
]`

const wantCodexAgentContextText = "<external_browser_tabs><in-app-browser-context>2 tabs</in-app-browser-context></external_browser_tabs>"

func TestResponsesToChatCompletionsRendersCodexAgentContextAsUserText(t *testing.T) {
	chat, err := ResponsesToChatCompletionsRequest(&ResponsesRequest{Model: "m", Input: json.RawMessage(codexAgentContextInput)})
	require.NoError(t, err)
	var texts []string
	for _, message := range chat.Messages {
		require.False(t, message.Role == "tool" && message.ToolCallID == "", "tool message without tool_call_id")
		var text string
		if message.Role == "user" && json.Unmarshal(message.Content, &text) == nil {
			texts = append(texts, text)
		}
	}
	require.Contains(t, texts, wantCodexAgentContextText)
	require.Equal(t, "tool", chat.Messages[len(chat.Messages)-1].Role)
	require.Equal(t, "call_1", chat.Messages[len(chat.Messages)-1].ToolCallID)
}

func TestResponsesToAnthropicRendersCodexAgentContextAsUserText(t *testing.T) {
	anthropic, err := ResponsesToAnthropicRequest(&ResponsesRequest{Model: "m", Input: json.RawMessage(codexAgentContextInput)})
	require.NoError(t, err)
	foundContext := false
	for _, message := range anthropic.Messages {
		var blocks []AnthropicContentBlock
		if json.Unmarshal(message.Content, &blocks) != nil {
			continue
		}
		for _, block := range blocks {
			require.False(t, block.Type == "tool_result" && block.ToolUseID == "", "tool_result without tool_use_id")
			if block.Type == "text" && block.Text == wantCodexAgentContextText {
				require.Equal(t, "user", message.Role)
				foundContext = true
			}
		}
	}
	require.True(t, foundContext)
}
