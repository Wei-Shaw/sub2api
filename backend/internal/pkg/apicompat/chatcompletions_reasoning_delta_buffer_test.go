package apicompat

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// GLM 等上游把 reasoning_content 拆成大量小碎片逐条下发，桥接层 1:1 转发时
// 每个碎片都会变成一个 reasoning_summary_text.delta 事件，Codex UI 被几千个
// 小事件刷到卡死。桥接必须先把碎片攒成大块再下发。
func TestStream_BuffersFragmentedReasoningDeltas(t *testing.T) {
	chunks := make([]string, 0, 16)
	chunks = append(chunks, `{"choices":[{"index":0,"delta":{"role":"assistant","content":null,"reasoning_content":""}}]}`)
	for i := 0; i < 12; i++ {
		chunks = append(chunks, `{"choices":[{"index":0,"delta":{"reasoning_content":"`+strings.Repeat("a", 100)+`"}}]}`)
	}
	chunks = append(chunks, `{"choices":[{"index":0,"delta":{"content":"done"},"finish_reason":"stop"}]}`)

	state := NewChatCompletionsToResponsesStreamState("glm-5.3-flash")
	var deltaCount int
	var deltaBytes strings.Builder
	for _, payload := range chunks {
		var chunk ChatCompletionsChunk
		require.NoError(t, json.Unmarshal([]byte(payload), &chunk))
		for _, e := range ChatCompletionsChunkToResponsesEvents(&chunk, state) {
			if e.Type == "response.reasoning_summary_text.delta" {
				deltaCount++
				_, _ = deltaBytes.WriteString(e.Delta)
			}
		}
	}
	for _, e := range FinalizeChatCompletionsResponsesStream(state) {
		if e.Type == "response.reasoning_summary_text.delta" {
			deltaCount++
			_, _ = deltaBytes.WriteString(e.Delta)
		}
	}

	// 1200 字符碎片按 256 字符缓冲应合并成 ~5 个 delta，而不是 12 个。
	require.LessOrEqual(t, deltaCount, 6)
	require.Equal(t, strings.Repeat("a", 1200), deltaBytes.String())
}

// 短思考（不足一个缓冲块）也必须在思考段结束时一次性 flush，不丢内容。
func TestStream_FlushesShortReasoningOnItemClose(t *testing.T) {
	events := collectStreamEvents(t, []string{
		`{"choices":[{"index":0,"delta":{"role":"assistant","content":null,"reasoning_content":""}}]}`,
		`{"choices":[{"index":0,"delta":{"reasoning_content":"brief"}}]}`,
		`{"choices":[{"index":0,"delta":{"content":"answer"},"finish_reason":"stop"}]}`,
	})

	var deltas []string
	for _, e := range events {
		if e.Type == "response.reasoning_summary_text.delta" {
			deltas = append(deltas, e.Delta)
		}
	}
	require.Len(t, deltas, 1)
	require.Equal(t, "brief", deltas[0])
}
