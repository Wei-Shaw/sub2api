// 性能基准：长 function_call 历史的 Responses 请求解码。
package responses

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// BenchmarkDecodeRequestToolHistory 构造 N 组 function_call +
// function_call_output 的 input 数组；每条 output 触发一次 findToolName
// 全历史回扫，整体 O(N²)。
func BenchmarkDecodeRequestToolHistory(b *testing.B) {
	const rounds = 200
	items := make([]map[string]any, 0, rounds*3)
	for i := 0; i < rounds; i++ {
		items = append(items,
			map[string]any{"type": "message", "role": "user",
				"content": []map[string]any{{"type": "input_text", "text": fmt.Sprintf("task %d %s", i, strings.Repeat("ctx ", 40))}}},
			map[string]any{"type": "function_call", "id": fmt.Sprintf("fc_%d", i),
				"call_id": fmt.Sprintf("call_%d", i), "name": "exec", "arguments": `{"command":"ls"}`},
			map[string]any{"type": "function_call_output",
				"call_id": fmt.Sprintf("call_%d", i), "output": strings.Repeat("out ", 80)},
		)
	}
	body, err := json.Marshal(map[string]any{"model": "fake", "input": items})
	if err != nil {
		b.Fatal(err)
	}
	b.Logf("body bytes = %d, items = %d", len(body), len(items))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := DecodeRequest(body); err != nil {
			b.Fatal(err)
		}
	}
}
