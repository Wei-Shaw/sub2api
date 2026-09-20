// 性能基准：长工具调用历史的请求解码（findToolName 回溯扫描）。
package chat

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// BenchmarkDecodeRequestToolHistory 构造 N 轮 assistant tool_call + tool 结果
// 交替的请求体；每条 tool 消息触发一次 findToolName 全历史回扫，整体 O(N²)。
func BenchmarkDecodeRequestToolHistory(b *testing.B) {
	const rounds = 200
	messages := make([]map[string]any, 0, rounds*3)
	for i := 0; i < rounds; i++ {
		messages = append(messages,
			map[string]any{"role": "user", "content": fmt.Sprintf("task %d %s", i, strings.Repeat("ctx ", 40))},
			map[string]any{"role": "assistant", "tool_calls": []map[string]any{{
				"id": fmt.Sprintf("call_%d", i), "type": "function",
				"function": map[string]any{"name": "exec", "arguments": `{"command":"ls"}`},
			}}},
			map[string]any{"role": "tool", "tool_call_id": fmt.Sprintf("call_%d", i), "content": strings.Repeat("out ", 80)},
		)
	}
	body, err := json.Marshal(map[string]any{"model": "fake", "messages": messages})
	if err != nil {
		b.Fatal(err)
	}
	b.Logf("body bytes = %d, messages = %d", len(body), len(messages))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := DecodeRequest(body); err != nil {
			b.Fatal(err)
		}
	}
}
