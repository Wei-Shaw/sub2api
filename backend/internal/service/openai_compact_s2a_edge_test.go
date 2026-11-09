package service

// 「由 sub2api 压缩」的边界行为：回放还原、历史摊平、异常载荷处理。

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestRehydrateS2AMultipleItems(t *testing.T) {
	first := compactS2ABuildItem("first summary", "deepseek-flash")
	second := compactS2ABuildItem("second summary", "deepseek-flash")
	body := []byte(`{"input":[` +
		`{"type":"compaction_summary","id":"` + first.ID + `","encrypted_content":"` + first.EncryptedContent + `"},` +
		`{"type":"message","role":"user","content":"continue"},` +
		`{"type":"compaction_summary","id":"` + second.ID + `","encrypted_content":"` + second.EncryptedContent + `"}` +
		`]}`)

	out, changed := rehydrateS2ACompactionItems(body)
	require.True(t, changed)
	require.Equal(t, "message", gjson.GetBytes(out, "input.0.type").String())
	require.Equal(t, "message", gjson.GetBytes(out, "input.2.type").String())
	require.Contains(t, gjson.GetBytes(out, "input.0.content.0.text").String(), "first summary")
	require.Contains(t, gjson.GetBytes(out, "input.2.content.0.text").String(), "second summary")
	require.Equal(t, "continue", gjson.GetBytes(out, "input.1.content").String())
}

func TestRehydrateS2ALeavesForeignAndMalformedItems(t *testing.T) {
	good := compactS2ABuildItem("kept", "deepseek-flash")
	body := []byte(`{"input":[` +
		// 第三方 compaction item：保持原样
		`{"type":"compaction","id":"cmp_foreign","encrypted_content":"zzz"},` +
		// 自家前缀但载荷被破坏：不应吞掉、也不应改变
		`{"type":"compaction_summary","id":"` + compactS2AIDPrefix + `broken","encrypted_content":"` + compactS2APayloadPrefix + `not-base64!!"},` +
		`{"type":"compaction_summary","id":"` + good.ID + `","encrypted_content":"` + good.EncryptedContent + `"}` +
		`]}`)

	out, changed := rehydrateS2ACompactionItems(body)
	require.True(t, changed)
	require.Equal(t, "compaction", gjson.GetBytes(out, "input.0.type").String(), "第三方 item 不得改动")
	require.Equal(t, compactS2AIDPrefix+"broken", gjson.GetBytes(out, "input.1.id").String(), "损坏载荷保持原样")
	require.Equal(t, "message", gjson.GetBytes(out, "input.2.type").String(), "可识别 item 正常还原")
}

func TestFlattenS2AInputEdgeShapes(t *testing.T) {
	t.Run("字符串 input", func(t *testing.T) {
		text := compactS2AFlattenInput(gjson.Parse(`"直接一段文本"`))
		require.Equal(t, "直接一段文本", text)
	})
	t.Run("内容数组与工具结果", func(t *testing.T) {
		input := gjson.Parse(`[
			{"type":"message","role":"user","content":[{"type":"input_text","text":"第一段"},{"type":"input_text","text":"第二段"}]},
			{"type":"function_call_output","output":"ok"},
			{"type":"unknown_item","payload":"ignored"}
		]`)
		text := compactS2AFlattenInput(input)
		require.Contains(t, text, "第一段")
		require.Contains(t, text, "第二段")
		require.Contains(t, text, "[tool result] ok")
		require.NotContains(t, text, "ignored")
	})
	t.Run("空 input", func(t *testing.T) {
		require.Empty(t, compactS2AFlattenInput(gjson.Parse(`[]`)))
	})
}

func TestTruncateCompactS2ATextKeepsHeadAndTail(t *testing.T) {
	long := make([]byte, compactS2ASummaryLimit+1000)
	for i := range long {
		long[i] = 'a'
	}
	copy(long, "HEAD")
	copy(long[len(long)-4:], "TAIL")

	got := truncateCompactS2AText(string(long))
	require.LessOrEqual(t, len(got), compactS2ASummaryLimit+64, "省略标记的额外开销之外不应超限")
	require.Contains(t, got, "HEAD")
	require.Contains(t, got, "TAIL")
	require.Contains(t, got, "middle of conversation omitted")
}

func TestCompactS2AItemRoundTrip(t *testing.T) {
	item := compactS2ABuildItem("摘要内容", "deepseek-flash")
	parsed := gjson.Parse(`{"id":"` + item.ID + `","encrypted_content":"` + item.EncryptedContent + `"}`)

	summary, ok := compactS2ADecodeItem(parsed)
	require.True(t, ok)
	require.Equal(t, "摘要内容", summary)

	// 前缀不对的 item 不应被识别
	_, ok = compactS2ADecodeItem(gjson.Parse(`{"id":"cmp_other","encrypted_content":"` + item.EncryptedContent + `"}`))
	require.False(t, ok)
}
