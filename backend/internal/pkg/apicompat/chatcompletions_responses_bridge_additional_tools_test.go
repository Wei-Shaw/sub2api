package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// #6653：Codex 把运行时工具放在 input[].additional_tools 里，force_chat_completions
// 下必须仍降级成标准 chat 工具随 /v1/chat/completions 发出，而不是因无法转换退回 Responses。
func TestResponsesToChatCompletionsRequest_LowersAdditionalToolsForChatOnlyUpstream(t *testing.T) {
	body := `{
		"model":"gpt-5.6-luna","stream":true,"store":false,
		"reasoning":{"effort":"medium","context":"all_turns"},
		"tool_choice":"auto","parallel_tool_calls":false,
		"include":["reasoning.encrypted_content"],
		"input":[
			{"type":"additional_tools","role":"developer","tools":[
				{"type":"custom","name":"exec","description":"run a shell command","format":{"type":"grammar","syntax":"lark","definition":"start: /.*/"}},
				{"type":"function","name":"wait","strict":false,"parameters":{"type":"object"}},
				{"type":"namespace","name":"collaboration","tools":[{"type":"function","name":"spawn_agent","parameters":{"type":"object"}}]}
			]},
			{"type":"message","role":"user","content":[{"type":"input_text","text":"reply ok"}]}
		]
	}`
	var req ResponsesRequest
	require.NoError(t, json.Unmarshal([]byte(body), &req))

	tools, err := EffectiveResponsesTools(&req)
	require.NoError(t, err)
	require.Len(t, tools, 3, "additional_tools 里的 custom / function / namespace 都要被提升")
	require.True(t, CustomToolNames(tools)["exec"])
	require.Equal(t, "collaboration", NamespaceToolNames(tools)["collaboration__spawn_agent"].Namespace)

	chat, err := ResponsesToChatCompletionsRequest(&req)
	require.NoError(t, err)
	require.Len(t, chat.Messages, 1, "additional_tools 不能变成 chat 消息")
	require.Equal(t, "user", chat.Messages[0].Role)

	names := make([]string, 0, len(chat.Tools))
	for _, tool := range chat.Tools {
		require.Equal(t, "function", tool.Type, "chat 上游只认 function 工具")
		require.NotNil(t, tool.Function)
		names = append(names, tool.Function.Name)
	}
	require.Equal(t, []string{"exec", "wait", "collaboration__spawn_agent"}, names)
	require.JSONEq(t, customToolInputSchema, string(chat.Tools[0].Function.Parameters), "custom 工具降级为单一 input 字符串参数")
	require.NotNil(t, chat.ParallelToolCalls)
	require.False(t, *chat.ParallelToolCalls)
	require.Equal(t, "medium", chat.ReasoningEffort)
}
