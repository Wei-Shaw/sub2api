package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// buildLargeAnthropicBody 构造一个体量接近真实长会话的 /v1/messages 请求体：
// messages 数组占绝大部分字节，顶层小字段排在它后面，正是让 gjson 顶层查找退化成
// 全量扫描的形状。
func buildLargeAnthropicBody(tb testing.TB, turns int, blockSize int) []byte {
	tb.Helper()

	filler := strings.Repeat("x", blockSize)
	messages := make([]map[string]any, 0, turns)
	for i := 0; i < turns; i++ {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		messages = append(messages, map[string]any{
			"role": role,
			"content": []map[string]any{
				{"type": "text", "text": fmt.Sprintf("turn %d %s", i, filler)},
			},
		})
	}

	body, err := json.Marshal(map[string]any{
		"model":      "claude-sonnet-4-5-20250929",
		"messages":   messages,
		"stream":     true,
		"max_tokens": 8192,
		"metadata":   map[string]any{"user_id": "bench-user"},
	})
	require.NoError(tb, err)
	return body
}

// TestReplaceBodyUnchangedKeepsDerivedState 固定住身份短路的语义：原样传回当前 body
// 不改变任何派生状态，真正换掉 body 仍然重新解析。
func TestReplaceBodyUnchangedKeepsDerivedState(t *testing.T) {
	body := buildLargeAnthropicBody(t, 4, 16)
	parsed, err := ParseGatewayRequest(NewRequestBodyRef(body), "anthropic")
	require.NoError(t, err)

	before := parsed.Body.Bytes()
	require.Equal(t, "claude-sonnet-4-5-20250929", parsed.Model)
	require.True(t, parsed.Stream)
	require.Equal(t, "bench-user", parsed.MetadataUserID)
	require.Equal(t, 8192, parsed.MaxTokens)

	require.NoError(t, parsed.ReplaceBody(before))
	require.Equal(t, "claude-sonnet-4-5-20250929", parsed.Model)
	require.True(t, parsed.Stream)
	require.Equal(t, "bench-user", parsed.MetadataUserID)
	require.Equal(t, 8192, parsed.MaxTokens)
	require.Equal(t, string(before), string(parsed.Body.Bytes()))

	// 内容相同但底层数组不同：不走短路，派生状态照样正确。
	copied := append([]byte(nil), before...)
	require.NoError(t, parsed.ReplaceBody(copied))
	require.Equal(t, "bench-user", parsed.MetadataUserID)

	// 真正的改写必须被看到。
	rewritten := []byte(`{"model":"claude-opus-4-5","stream":false,"max_tokens":16}`)
	require.NoError(t, parsed.ReplaceBody(rewritten))
	require.Equal(t, "claude-opus-4-5", parsed.Model)
	require.False(t, parsed.Stream)
	require.Equal(t, 16, parsed.MaxTokens)
	require.Empty(t, parsed.MetadataUserID)
}

// BenchmarkReplaceBodyUnchanged 模拟 Forward 转发路径上 helper 全部原样返回时的
// ReplaceBody 链（Anthropic 路径约 10 次）。
func BenchmarkReplaceBodyUnchanged(b *testing.B) {
	for _, tc := range []struct {
		name      string
		turns     int
		blockSize int
	}{
		{name: "small_8KB", turns: 16, blockSize: 512},
		{name: "medium_256KB", turns: 64, blockSize: 4096},
		{name: "large_2MB", turns: 128, blockSize: 16384},
	} {
		body := buildLargeAnthropicBody(b, tc.turns, tc.blockSize)
		b.Run(fmt.Sprintf("%s_bytes_%d", tc.name, len(body)), func(b *testing.B) {
			parsed, err := ParseGatewayRequest(NewRequestBodyRef(body), "anthropic")
			require.NoError(b, err)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				current := parsed.Body.Bytes()
				for j := 0; j < 10; j++ {
					if err := parsed.ReplaceBody(current); err != nil {
						b.Fatal(err)
					}
					current = parsed.Body.Bytes()
				}
			}
		})
	}
}
