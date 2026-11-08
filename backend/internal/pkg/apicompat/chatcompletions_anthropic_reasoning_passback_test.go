package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// issue #5528：/v1/messages 客户端(Claude Code 等)打到只会 Chat Completions 的
// OpenAI 兼容上游时，历史 assistant 消息里的 thinking 块被整块丢弃。DeepSeek 的
// thinking mode 要求产生工具调用的 reasoning_content 随该 assistant 消息回传，
// 于是「单轮正常、一进多轮工具对话必现 400」。

func anthropicAssistantMsg(t *testing.T, blocks string) *AnthropicRequest {
	t.Helper()
	return &AnthropicRequest{
		Model:     "deepseek-v4-flash",
		MaxTokens: 256,
		Messages: []AnthropicMessage{
			{Role: "user", Content: json.RawMessage(`"what's the weather?"`)},
			{Role: "assistant", Content: json.RawMessage(blocks)},
			{Role: "user", Content: json.RawMessage(`[{"type":"tool_result","tool_use_id":"toolu_1","content":"sunny"}]`)},
		},
	}
}

const anthropicThinkingToolTurn = `[
	{"type":"thinking","thinking":"user wants weather, call the tool"},
	{"type":"text","text":"checking"},
	{"type":"tool_use","id":"toolu_1","name":"get_weather","input":{"city":"SF"}}
]`

func TestAnthropicToChatCompletionsRequest_ThinkingBecomesReasoningContentOnToolTurn(t *testing.T) {
	out, err := AnthropicToChatCompletionsRequest(anthropicAssistantMsg(t, anthropicThinkingToolTurn))
	require.NoError(t, err)

	var assistant *ChatMessage
	for i := range out.Messages {
		if out.Messages[i].Role == "assistant" {
			assistant = &out.Messages[i]
			break
		}
	}
	require.NotNil(t, assistant, "assistant message must survive the bridge")
	require.Equal(t, "user wants weather, call the tool", assistant.ReasoningContent,
		"产生工具调用的 thinking 必须作为 reasoning_content 回传，否则 DeepSeek 400")
	require.Len(t, assistant.ToolCalls, 1)
	require.Equal(t, `"checking"`, string(assistant.Content), "text/tool_use 处理保持不变")
}

// 上游线格式才是上游看到的东西：字段没序列化出去，等于没修。
func TestAnthropicToChatCompletionsRequest_ReasoningContentSerializesOnWire(t *testing.T) {
	out, err := AnthropicToChatCompletionsRequest(anthropicAssistantMsg(t, anthropicThinkingToolTurn))
	require.NoError(t, err)

	payload, err := json.Marshal(out)
	require.NoError(t, err)
	require.Contains(t, string(payload), `"reasoning_content":"user wants weather, call the tool"`)
}

// 闭环不变式：thinking 块本来就是本桥出站时用上游 reasoning_content 生成的
// (chatMessageToAnthropicBlocks)，客户端只是原样回传。出站造、入站丢 = 自己丢自己的东西。
func TestAnthropicChatBridge_ReasoningSurvivesOutboundInboundRoundTrip(t *testing.T) {
	upstream := ChatMessage{
		Role:             "assistant",
		ReasoningContent: "step 1: need the weather tool",
		Content:          json.RawMessage(`"checking"`),
		ToolCalls: []ChatToolCall{{
			ID:       "call_1",
			Type:     "function",
			Function: ChatFunctionCall{Name: "get_weather", Arguments: `{"city":"SF"}`},
		}},
	}

	// 出站：Chat 响应 → Anthropic content blocks
	blocks := chatMessageToAnthropicBlocks(upstream)
	require.Equal(t, "thinking", blocks[0].Type)
	require.Equal(t, upstream.ReasoningContent, blocks[0].Thinking)

	// 客户端下一轮把同一组 blocks 原样回传
	raw, err := json.Marshal(blocks)
	require.NoError(t, err)

	// 入站：Anthropic content blocks → Chat 请求
	back, err := anthropicAssistantToChatMessages(raw)
	require.NoError(t, err)
	require.Len(t, back, 1)
	require.Equal(t, upstream.ReasoningContent, back[0].ReasoningContent,
		"出站生成的 thinking 必须能原样还原回 reasoning_content")
	require.Len(t, back[0].ToolCalls, 1)
}

// 兄弟不变式：Responses→Chat 桥(buildChatMessagesFromItems 的 pendingReasoning)
// 早就把 reasoning 挂到带 tool_calls 的 assistant 消息上了。等价历史下两条桥必须一致。
func TestAnthropicChatBridge_MatchesResponsesChatBridgeReasoningPlacement(t *testing.T) {
	responsesReq := &ResponsesRequest{
		Model: "deepseek-v4-flash",
		Input: json.RawMessage(`[
			{"type":"message","role":"user","content":[{"type":"input_text","text":"what's the weather?"}]},
			{"type":"reasoning","summary":[{"type":"summary_text","text":"call the tool"}]},
			{"type":"function_call","call_id":"call_1","name":"get_weather","arguments":"{\"city\":\"SF\"}"},
			{"type":"function_call_output","call_id":"call_1","output":"sunny"}
		]`),
	}
	viaResponses, err := ResponsesToChatCompletionsRequest(responsesReq)
	require.NoError(t, err)

	viaAnthropic, err := AnthropicToChatCompletionsRequest(anthropicAssistantMsg(t, `[
		{"type":"thinking","thinking":"call the tool"},
		{"type":"tool_use","id":"toolu_1","name":"get_weather","input":{"city":"SF"}}
	]`))
	require.NoError(t, err)

	reasoningOnToolCallMessage := func(msgs []ChatMessage) string {
		for _, m := range msgs {
			if m.Role == "assistant" && len(m.ToolCalls) > 0 {
				return m.ReasoningContent
			}
		}
		return ""
	}
	require.Equal(t, "call the tool", reasoningOnToolCallMessage(viaResponses.Messages),
		"前置条件：兄弟桥本来就带 reasoning_content")
	require.Equal(t, reasoningOnToolCallMessage(viaResponses.Messages),
		reasoningOnToolCallMessage(viaAnthropic.Messages),
		"两条桥对等价历史必须产出同样的 reasoning_content 位置")
}

// 作用域守卫：不带工具调用的纯文本轮次维持现状(与兄弟桥一致 —— reasoning 只随
// 工具调用回传)，避免把 reasoning_content 撒到不需要它的上游请求上。
func TestAnthropicToChatCompletionsRequest_ThinkingWithoutToolCallsStaysDropped(t *testing.T) {
	req := &AnthropicRequest{
		Model:     "deepseek-v4-flash",
		MaxTokens: 100,
		Messages: []AnthropicMessage{
			{Role: "assistant", Content: json.RawMessage(
				`[{"type":"thinking","thinking":"secret thoughts"},{"type":"text","text":"answer"}]`)},
		},
	}

	out, err := AnthropicToChatCompletionsRequest(req)
	require.NoError(t, err)
	require.Len(t, out.Messages, 1)
	require.Empty(t, out.Messages[0].ReasoningContent)
	require.Equal(t, `"answer"`, string(out.Messages[0].Content))

	payload, err := json.Marshal(out)
	require.NoError(t, err)
	require.NotContains(t, string(payload), "reasoning_content")
}

func TestAnthropicThinkingToReasoningContent(t *testing.T) {
	blocksOf := func(t *testing.T, raw string) []AnthropicContentBlock {
		t.Helper()
		var blocks []AnthropicContentBlock
		require.NoError(t, json.Unmarshal([]byte(raw), &blocks))
		return blocks
	}

	cases := []struct {
		name         string
		raw          string
		hasToolCalls bool
		want         string
	}{
		{
			name:         "single_thinking_block",
			raw:          `[{"type":"thinking","thinking":"a"}]`,
			hasToolCalls: true,
			want:         "a",
		},
		{
			// 多个 thinking 块用 "\n" 连接，与 extractResponsesReasoningText 一致。
			name:         "multiple_blocks_join_with_newline",
			raw:          `[{"type":"thinking","thinking":"a"},{"type":"text","text":"x"},{"type":"thinking","thinking":"b"}]`,
			hasToolCalls: true,
			want:         "a\nb",
		},
		{
			// redacted_thinking 没有明文可回传。
			name:         "redacted_thinking_has_no_plaintext",
			raw:          `[{"type":"redacted_thinking","signature":"abc"}]`,
			hasToolCalls: true,
			want:         "",
		},
		{
			// 只带 signature 的 thinking 占位块(xAI/Codex 密文回放形态)同样无明文。
			name:         "signature_only_thinking",
			raw:          `[{"type":"thinking","thinking":"","signature":"gAAAAxxx"}]`,
			hasToolCalls: true,
			want:         "",
		},
		{
			name:         "no_tool_calls_returns_empty",
			raw:          `[{"type":"thinking","thinking":"a"}]`,
			hasToolCalls: false,
			want:         "",
		},
		{
			name:         "no_thinking_blocks",
			raw:          `[{"type":"text","text":"x"}]`,
			hasToolCalls: true,
			want:         "",
		},
		{
			name:         "empty_blocks",
			raw:          `[]`,
			hasToolCalls: true,
			want:         "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want,
				anthropicThinkingToReasoningContent(blocksOf(t, tc.raw), tc.hasToolCalls))
		})
	}
}

// 纯字符串形态的 assistant content 没有 blocks 可读，走早返回分支，不得 panic。
func TestAnthropicAssistantToChatMessages_PlainStringContentUnaffected(t *testing.T) {
	msgs, err := anthropicAssistantToChatMessages(json.RawMessage(`"just text"`))
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	require.Empty(t, msgs[0].ReasoningContent)
	require.Equal(t, `"just text"`, string(msgs[0].Content))
}
