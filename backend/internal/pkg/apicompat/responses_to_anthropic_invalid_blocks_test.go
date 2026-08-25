package apicompat

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// anthropicInboundBlockTypes 是 Anthropic Messages 请求体里合法的 content block
// 类型。转换结果只能落在这个集合内——发出集合外的类型，上游一律回
// 400 "Request body format invalid"（见 issue #5329）。
var anthropicInboundBlockTypes = map[string]bool{
	"text":              true,
	"image":             true,
	"document":          true,
	"tool_use":          true,
	"tool_result":       true,
	"thinking":          true,
	"redacted_thinking": true,
}

func responsesToAnthropicMessages(t *testing.T, input string) []AnthropicMessage {
	t.Helper()
	var req ResponsesRequest
	require.NoError(t, json.Unmarshal([]byte(`{"model":"glm-5.2","input":`+input+`}`), &req))
	out, err := ResponsesToAnthropicRequest(&req)
	require.NoError(t, err)
	return out.Messages
}

// requireAnthropicMessagesAreSendable 断言消息序列不含 Anthropic 会拒收的形态：
// 未知 block 类型、空内容消息、纯空白 text 块。
func requireAnthropicMessagesAreSendable(t *testing.T, messages []AnthropicMessage) {
	t.Helper()
	for i, m := range messages {
		raw := strings.TrimSpace(string(m.Content))
		require.NotContains(t, []string{"", "null", `""`, "[]"}, raw,
			"messages[%d] 内容为空，Anthropic 拒收空内容消息", i)

		var s string
		if err := json.Unmarshal(m.Content, &s); err == nil {
			require.NotEmpty(t, strings.TrimSpace(s), "messages[%d] 字符串内容不能全为空白", i)
			continue
		}
		blocks := parseContentBlocks(m.Content)
		require.NotEmpty(t, blocks, "messages[%d] 解析不出任何 block", i)
		for j, b := range blocks {
			require.True(t, anthropicInboundBlockTypes[b.Type],
				"messages[%d].content[%d] 是 Anthropic 不认识的 block 类型 %q", i, j, b.Type)
			if b.Type == "text" {
				require.NotEmpty(t, strings.TrimSpace(b.Text),
					"messages[%d].content[%d] 是空白 text 块，Anthropic 拒收", i, j)
			}
		}
	}
}

// issue #5329：工具执行后的下一轮，Codex 会把 reasoning item 一起回放。
// 该 item 带 content 数组时，reasoning_text 块以前会被原样塞进 Anthropic 请求体。
func TestResponsesToAnthropic_ReasoningItemWithContentIsDropped(t *testing.T) {
	messages := responsesToAnthropicMessages(t, `[
		{"type":"message","role":"user","content":[{"type":"input_text","text":"run a shell command"}]},
		{"type":"reasoning","id":"rs_1","summary":[],"content":[{"type":"reasoning_text","text":"let me think"}]}
	]`)

	requireAnthropicMessagesAreSendable(t, messages)
	require.Len(t, messages, 1)
	require.NotContains(t, string(messages[0].Content), "reasoning_text")
	require.NotContains(t, string(messages[0].Content), "let me think")
}

// Codex 的常见 reasoning 形态（只有 summary + encrypted_content）本来就会被丢弃，
// 这条守卫确保行为没有被改变。
func TestResponsesToAnthropic_ReasoningItemSummaryOnlyStillDropped(t *testing.T) {
	messages := responsesToAnthropicMessages(t, `[
		{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]},
		{"type":"reasoning","id":"rs_1","summary":[{"type":"summary_text","text":"s"}],"encrypted_content":"gAAAA"}
	]`)

	requireAnthropicMessagesAreSendable(t, messages)
	require.Len(t, messages, 1)
	require.NotContains(t, string(messages[0].Content), "gAAAA")
}

// 未知 item type 的 content 以前会被逐字透传，把 Responses 专有分片带进上游请求。
func TestResponsesToAnthropic_UnknownItemTypeContentIsSanitized(t *testing.T) {
	messages := responsesToAnthropicMessages(t, `[
		{"type":"web_search_call","id":"ws_1","content":[{"type":"web_search_result","text":"payload"}]}
	]`)

	requireAnthropicMessagesAreSendable(t, messages)
	require.Empty(t, messages, "整条内容都无法映射时不应发出消息")
}

// 未知 item type 里夹带的可识别文本仍然保留，不做无谓丢弃。
func TestResponsesToAnthropic_UnknownItemTypeKeepsRecognizableText(t *testing.T) {
	messages := responsesToAnthropicMessages(t, `[
		{"type":"some_future_item","content":[
			{"type":"input_text","text":"keep me"},
			{"type":"reasoning_text","text":"drop me"}
		]}
	]`)

	requireAnthropicMessagesAreSendable(t, messages)
	require.Len(t, messages, 1)
	require.Contains(t, string(messages[0].Content), "keep me")
	require.NotContains(t, string(messages[0].Content), "drop me")
}

// user 消息的分片全部不可识别时，以前会退化成 content:""，Anthropic 拒收空内容消息。
func TestResponsesToAnthropic_UserMessageWithOnlyUnknownPartsIsDropped(t *testing.T) {
	messages := responsesToAnthropicMessages(t, `[
		{"type":"message","role":"user","content":[{"type":"input_file","file_id":"file_1"}]}
	]`)

	requireAnthropicMessagesAreSendable(t, messages)
	require.Empty(t, messages)
}

// assistant 侧同理：以前会退化成单个空 text 块，Anthropic 同样拒收。
func TestResponsesToAnthropic_AssistantMessageWithOnlyUnknownPartsIsDropped(t *testing.T) {
	messages := responsesToAnthropicMessages(t, `[
		{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]},
		{"type":"message","role":"assistant","content":[{"type":"refusal","refusal":"no"}]}
	]`)

	requireAnthropicMessagesAreSendable(t, messages)
	require.Len(t, messages, 1)
	require.Equal(t, "user", messages[0].Role)
}

// 完整的 Codex 工具续接回放：tool_use / tool_result 配对必须保持不变，
// 同时整个序列满足可发送不变式。
func TestResponsesToAnthropic_CodexToolRoundStaysIntactAndSendable(t *testing.T) {
	messages := responsesToAnthropicMessages(t, `[
		{"type":"message","role":"user","content":[{"type":"input_text","text":"run ls"}]},
		{"type":"reasoning","id":"rs_1","summary":[],"content":[{"type":"reasoning_text","text":"plan"}],"encrypted_content":"gAAAA"},
		{"type":"function_call","id":"fc_1","call_id":"call_1","name":"shell","arguments":"{\"cmd\":\"ls\"}"},
		{"type":"function_call_output","call_id":"call_1","output":"file1"},
		{"type":"message","role":"user","content":[{"type":"input_text","text":"continue"}]}
	]`)

	requireAnthropicMessagesAreSendable(t, messages)

	var sawToolUse, sawToolResult bool
	for _, m := range messages {
		for _, b := range parseContentBlocks(m.Content) {
			switch b.Type {
			case "tool_use":
				sawToolUse = true
				require.Equal(t, "call_1", b.ID)
				require.Equal(t, "shell", b.Name)
			case "tool_result":
				sawToolResult = true
				require.Equal(t, "call_1", b.ToolUseID)
			}
		}
	}
	require.True(t, sawToolUse, "function_call 必须转成 tool_use")
	require.True(t, sawToolResult, "function_call_output 必须转成 tool_result")
	require.NotContains(t, string(mustMarshal(t, messages)), "reasoning_text")
	require.NotContains(t, string(mustMarshal(t, messages)), "gAAAA")
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}

func TestAnthropicContentIsEmpty(t *testing.T) {
	cases := []struct {
		raw  string
		want bool
	}{
		{``, true},
		{`""`, true},
		{`null`, true},
		{`[]`, true},
		{`  []  `, true},
		{`"hi"`, false},
		{`[{"type":"text","text":"hi"}]`, false},
	}
	for _, tc := range cases {
		require.Equal(t, tc.want, anthropicContentIsEmpty(json.RawMessage(tc.raw)), "raw=%q", tc.raw)
	}
}

func TestAnthropicContentIsOnlyBlankText(t *testing.T) {
	cases := []struct {
		raw  string
		want bool
	}{
		{`[{"type":"text","text":""}]`, true},
		{`[{"type":"text","text":"   "}]`, true},
		{`[{"type":"text","text":""},{"type":"text","text":" "}]`, true},
		{`[{"type":"text","text":"hi"}]`, false},
		{`[{"type":"text","text":""},{"type":"image","source":{}}]`, false},
		{`[]`, false},
	}
	for _, tc := range cases {
		require.Equal(t, tc.want, anthropicContentIsOnlyBlankText(json.RawMessage(tc.raw)), "raw=%q", tc.raw)
	}
}
