package apicompat

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// chainCCToAnthropic 把一组 CC SSE 帧 → Responses events → Anthropic events 后
// 拼接为 Anthropic SSE 文本，方便 assert。
func chainCCToAnthropic(t *testing.T, lines []string) (string, []AnthropicStreamEvent, *ResponsesUsage) {
	t.Helper()
	ccState := NewCCStreamState()
	anthState := NewResponsesEventToAnthropicState()
	var sseOut strings.Builder
	var anthEvents []AnthropicStreamEvent

	pumpFrame := func(frame []byte) {
		// Each frame is "event: <type>\ndata: <json>\n\n" or "data: [DONE]\n\n".
		for _, raw := range strings.Split(string(frame), "\n") {
			line := strings.TrimSpace(raw)
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			payload := strings.TrimSpace(line[len("data:"):])
			if payload == "" || payload == "[DONE]" {
				continue
			}
			var evt ResponsesStreamEvent
			if err := json.Unmarshal([]byte(payload), &evt); err != nil {
				continue
			}
			for _, anth := range ResponsesEventToAnthropicEvents(&evt, anthState) {
				anthEvents = append(anthEvents, anth)
				sse, err := ResponsesAnthropicEventToSSE(anth)
				require.NoError(t, err)
				sseOut.WriteString(sse)
			}
		}
	}

	for _, line := range lines {
		frames, err := ConvertChatCompletionsSSEChunkToResponsesEvents([]byte(line), ccState)
		if err != nil && !strings.Contains(err.Error(), "parse chat completions chunk") {
			require.NoError(t, err)
		}
		for _, f := range frames {
			pumpFrame(f)
		}
	}
	if !ccState.CompletedSent {
		for _, f := range FinalizeCCStream(ccState) {
			pumpFrame(f)
		}
	}
	for _, anth := range FinalizeResponsesAnthropicStream(anthState) {
		anthEvents = append(anthEvents, anth)
		sse, err := ResponsesAnthropicEventToSSE(anth)
		require.NoError(t, err)
		sseOut.WriteString(sse)
	}
	return sseOut.String(), anthEvents, ccState.Usage
}

// TestCCChunksToAnthropicChain_TextDelta 验证 CC 的 text delta 链路转换为
// Anthropic 的 message_start / content_block_start / content_block_delta 序列。
func TestCCChunksToAnthropicChain_TextDelta(t *testing.T) {
	lines := []string{
		`data: {"id":"chk_1","object":"chat.completion.chunk","model":"deepseek-chat","choices":[{"index":0,"delta":{"role":"assistant"}}]}`,
		`data: {"id":"chk_1","object":"chat.completion.chunk","model":"deepseek-chat","choices":[{"index":0,"delta":{"content":"Hi"}}]}`,
		`data: {"id":"chk_1","object":"chat.completion.chunk","model":"deepseek-chat","choices":[{"index":0,"delta":{"content":" there"}}]}`,
		`data: {"id":"chk_1","object":"chat.completion.chunk","model":"deepseek-chat","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`,
		"data: [DONE]",
	}
	sse, events, usage := chainCCToAnthropic(t, lines)
	assert.Contains(t, sse, "event: message_start")
	assert.Contains(t, sse, "event: content_block_start")
	assert.Contains(t, sse, "event: content_block_delta")
	assert.Contains(t, sse, "event: content_block_stop")
	assert.Contains(t, sse, "event: message_delta")
	assert.Contains(t, sse, "event: message_stop")
	require.NotNil(t, usage)
	assert.Equal(t, 3, usage.InputTokens)
	assert.Equal(t, 2, usage.OutputTokens)
	assert.NotEmpty(t, events)
}

// TestCCChunksToAnthropicChain_ToolCall 验证 CC 的 tool_calls.arguments 增量被
// 翻译为 Anthropic input_json_delta + tool_use 块。
func TestCCChunksToAnthropicChain_ToolCall(t *testing.T) {
	lines := []string{
		`data: {"id":"chk","object":"chat.completion.chunk","model":"m","choices":[{"index":0,"delta":{"role":"assistant"}}]}`,
		`data: {"id":"chk","object":"chat.completion.chunk","model":"m","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"lookup"}}]}}]}`,
		`data: {"id":"chk","object":"chat.completion.chunk","model":"m","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"q\":"}}]}}]}`,
		`data: {"id":"chk","object":"chat.completion.chunk","model":"m","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"x\"}"}}]}}]}`,
		`data: {"id":"chk","object":"chat.completion.chunk","model":"m","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
		"data: [DONE]",
	}
	sse, _, _ := chainCCToAnthropic(t, lines)
	assert.Contains(t, sse, "tool_use")
	assert.Contains(t, sse, `"name":"lookup"`)
	assert.Contains(t, sse, "input_json_delta")
	assert.Contains(t, sse, "tool_use")
}

// TestCCChunksToAnthropicChain_FinishLength 验证 finish_reason=length 在 Anthropic 侧映射为
// stop_reason=max_tokens（与 Responses incomplete + max_output_tokens 对应）。
func TestCCChunksToAnthropicChain_FinishLength(t *testing.T) {
	lines := []string{
		`data: {"id":"c","object":"chat.completion.chunk","model":"m","choices":[{"index":0,"delta":{"role":"assistant","content":"x"}}]}`,
		`data: {"id":"c","object":"chat.completion.chunk","model":"m","choices":[{"index":0,"delta":{},"finish_reason":"length"}]}`,
		"data: [DONE]",
	}
	sse, _, _ := chainCCToAnthropic(t, lines)
	assert.Contains(t, sse, "max_tokens")
}
