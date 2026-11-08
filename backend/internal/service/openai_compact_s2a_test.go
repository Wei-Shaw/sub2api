package service

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestRehydrateS2ACompactionItems(t *testing.T) {
	item := compactS2ABuildItem("handover: did X, next Y", "deepseek-flash")
	body := []byte(`{"model":"gpt-6.1-sol","input":[` +
		`{"type":"compaction_summary","id":"` + item.ID + `","encrypted_content":"` + item.EncryptedContent + `"},` +
		`{"type":"message","role":"user","content":"continue"}]}`)

	out, changed := rehydrateS2ACompactionItems(body)
	require.True(t, changed)

	first := gjson.GetBytes(out, "input.0")
	require.Equal(t, "message", first.Get("type").String())
	require.Equal(t, "developer", first.Get("role").String())
	require.Contains(t, first.Get("content.0.text").String(), "handover: did X, next Y")
	require.Equal(t, "continue", gjson.GetBytes(out, "input.1.content").String())
}

func TestRehydrateS2ACompactionItemsIgnoresForeignItems(t *testing.T) {
	body := []byte(`{"input":[{"type":"compaction","id":"cmp_abc","encrypted_content":"zzz"}]}`)
	out, changed := rehydrateS2ACompactionItems(body)
	require.False(t, changed)
	require.Equal(t, string(body), string(out))
}

func TestCompactS2AFlattenInput(t *testing.T) {
	input := []byte(`[
		{"type":"message","role":"user","content":[{"type":"input_text","text":"修一下登录 bug"}]},
		{"type":"function_call","name":"exec","arguments":"{\"command\":\"ls\"}"},
		{"type":"function_call_output","output":"a.go b.go"},
		{"type":"reasoning","summary":[]},
		{"type":"message","role":"assistant","content":"已定位到 a.go"}
	]`)
	text := compactS2AFlattenInput(gjson.ParseBytes(input))
	require.Contains(t, text, "修一下登录 bug")
	require.Contains(t, text, "[tool call] exec")
	require.Contains(t, text, "[tool result] a.go b.go")
	require.Contains(t, text, "已定位到 a.go")
	require.NotContains(t, text, "reasoning")
}
