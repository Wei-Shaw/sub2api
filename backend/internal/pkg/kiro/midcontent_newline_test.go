package kiro

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// 回归: Kiro 把标题后的空行作为独立纯空白 chunk 发送时, 中段空行必须保留,
// 否则 markdown 标题与正文粘连, 下游(codex)无法渲染。
func TestStreamEventStreamPreservesMidContentBlankLines(t *testing.T) {
	chunks := []string{"## 标题一", "\n\n", "这是正文第一段。", "\n\n", "### 小节", "\n\n", "这是正文第二段。"}
	stream := bytes.NewBuffer(nil)
	for _, c := range chunks {
		_, _ = stream.Write(buildEventStreamFrame(t, "assistantResponseEvent", map[string]any{
			"assistantResponseEvent": map[string]any{"content": c},
		}))
	}
	var out bytes.Buffer
	_, err := StreamEventStreamAsAnthropicWithContext(context.Background(), stream, &out, "gpt-5.6-sol", 9, KiroRequestContext{})
	require.NoError(t, err)
	output := out.String()
	// 标题后的 \n\n 必须存活
	require.Contains(t, output, `"text":"\n\n这是正文第一段。"`)
	// 且首段前不应有前导空白(本例首 chunk 就是标题, 无前导)
	require.Contains(t, output, `"text":"## 标题一"`)
}
