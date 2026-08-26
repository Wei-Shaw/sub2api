package apicompat

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCompactionEnvelopeRoundTrip(t *testing.T) {
	summary := "第一节：用户要求修复 compaction\n第二节：涉及 <tags> 与 \"引号\" 和 emoji 🙂"

	envelope := EncodeCompactionEnvelope(summary)
	require.NotEmpty(t, envelope)
	require.True(t, strings.HasPrefix(envelope, "sub2api-compaction-v1."))

	decoded, ok := DecodeCompactionEnvelope(envelope)
	require.True(t, ok)
	require.Equal(t, summary, decoded)
}

func TestEncodeCompactionEnvelopeRejectsBlank(t *testing.T) {
	// 空摘要必须返回空串：Codex 拒收空 encrypted_content，调用方要据此判失败。
	require.Empty(t, EncodeCompactionEnvelope(""))
	require.Empty(t, EncodeCompactionEnvelope("   \n\t "))
}

func TestDecodeCompactionEnvelopeRejectsForeignPayload(t *testing.T) {
	// 上游原生的不透明密文不是本网关的信封，必须返回 ok=false 让调用方回落，
	// 而不是报错或 panic。
	for _, input := range []string{
		"",
		"gAAAAABn0pQ9opaque",
		"sub2api-compaction-v1.!!!not-base64!!!",
		"sub2api-compaction-v1." + rawURLBase64(t, `{"v":1}`),
		"sub2api-compaction-v1." + rawURLBase64(t, `not json`),
		"sub2api-compaction-v1." + rawURLBase64(t, `{"v":1,"summary":"  "}`),
	} {
		decoded, ok := DecodeCompactionEnvelope(input)
		require.False(t, ok, "input=%q", input)
		require.Empty(t, decoded, "input=%q", input)
	}
}

func TestCompactionSummaryFromItemPrefersPlainSummary(t *testing.T) {
	item := &ResponsesInputItem{
		Type:             CompactionItemType,
		EncryptedContent: EncodeCompactionEnvelope("来自信封"),
		Summary: []ResponsesSummary{
			{Type: "summary_text", Text: "第一段"},
			{Type: "summary_text", Text: "第二段"},
		},
	}
	require.Equal(t, "第一段\n第二段", CompactionSummaryFromItem(item))
}

func TestCompactionSummaryFromItemFallsBackToEnvelope(t *testing.T) {
	item := &ResponsesInputItem{
		Type:             CompactionItemType,
		EncryptedContent: EncodeCompactionEnvelope("仅信封可用"),
	}
	require.Equal(t, "仅信封可用", CompactionSummaryFromItem(item))

	// 两者都拿不到时返回空串，调用方据此跳过该 item。
	require.Empty(t, CompactionSummaryFromItem(&ResponsesInputItem{Type: CompactionItemType}))
	require.Empty(t, CompactionSummaryFromItem(nil))
}

func TestHasCompactionTrigger(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"触发器在末尾", `[{"type":"message","role":"user","content":"hi"},{"type":"compaction_trigger"}]`, true},
		{"触发器在中间", `[{"type":"compaction_trigger"},{"type":"message","role":"user","content":"hi"}]`, true},
		{"普通请求", `[{"type":"message","role":"user","content":"hi"}]`, false},
		{"空数组", `[]`, false},
		{"纯字符串 input", `"hello"`, false},
		{"非法 JSON 不 panic", `{oops`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &ResponsesRequest{Input: json.RawMessage(tt.input)}
			require.Equal(t, tt.want, HasCompactionTrigger(req))
		})
	}
	require.False(t, HasCompactionTrigger(nil))
}

// compaction_trigger 必须变成携带摘要指令的 user message。若落到 default 分支
// 会因 Content==nil 被静默丢弃，上游当成普通对话继续干活，Codex 随后报
// "expected exactly one compaction output item"。
func TestResponsesToAnthropicRequest_CompactionTriggerBecomesSummaryPrompt(t *testing.T) {
	req := &ResponsesRequest{
		Model: "gpt-5.6-sol",
		Input: json.RawMessage(`[
			{"type":"message","role":"user","content":[{"type":"input_text","text":"帮我改代码"}]},
			{"type":"compaction_trigger"}
		]`),
	}

	out, err := ResponsesToAnthropicRequest(req)
	require.NoError(t, err)
	require.NotEmpty(t, out.Messages)

	last := out.Messages[len(out.Messages)-1]
	require.Equal(t, "user", last.Role)
	// 原 user 消息与摘要指令都是 user 角色，会被合并成一条（两个文本块）。
	text := anthropicMessageText(t, last)
	require.Contains(t, text, "帮我改代码")
	require.Contains(t, text, CompactionSummaryPrompt)
}

// 回放路径：Codex 下一轮把 compaction item 原样放回 input，代表被丢弃的全部前文。
// 必须还原成 <conversation_summary> user message，否则压缩"成功"后上下文全丢。
func TestResponsesToAnthropicRequest_CompactionItemReplaysSummary(t *testing.T) {
	req := &ResponsesRequest{
		Model: "gpt-5.6-sol",
		Input: json.RawMessage(`[
			{"type":"compaction","status":"completed","encrypted_content":"` +
			EncodeCompactionEnvelope("前文摘要正文") + `"},
			{"type":"message","role":"user","content":[{"type":"input_text","text":"继续"}]}
		]`),
	}

	out, err := ResponsesToAnthropicRequest(req)
	require.NoError(t, err)
	require.NotEmpty(t, out.Messages)

	first := anthropicMessageText(t, out.Messages[0])
	require.Contains(t, first, "<conversation_summary>")
	require.Contains(t, first, "前文摘要正文")
	require.Contains(t, first, "</conversation_summary>")
	require.Contains(t, first, "继续")
}

// 明文 summary 字段可用时优先读它，无需解码信封。
func TestResponsesToAnthropicRequest_CompactionItemUsesPlainSummary(t *testing.T) {
	req := &ResponsesRequest{
		Model: "gpt-5.6-sol",
		Input: json.RawMessage(`[
			{"type":"compaction","summary":[{"type":"summary_text","text":"明文摘要"}],
			 "encrypted_content":"gAAAAABopaque-from-other-upstream"},
			{"type":"message","role":"user","content":[{"type":"input_text","text":"继续"}]}
		]`),
	}

	out, err := ResponsesToAnthropicRequest(req)
	require.NoError(t, err)
	require.NotEmpty(t, out.Messages)

	first := anthropicMessageText(t, out.Messages[0])
	require.Contains(t, first, "明文摘要")
	// 上游原生的不透明密文不是本网关信封，不得被当作摘要正文泄漏进请求体。
	require.NotContains(t, first, "opaque-from-other-upstream")
}

// 摘要完全取不到的 compaction item 应被跳过，而不是发出空内容消息
// （Anthropic 拒收空文本块）。
func TestResponsesToAnthropicRequest_CompactionItemWithoutSummarySkipped(t *testing.T) {
	req := &ResponsesRequest{
		Model: "gpt-5.6-sol",
		Input: json.RawMessage(`[
			{"type":"compaction","encrypted_content":"gAAAAABopaque"},
			{"type":"message","role":"user","content":[{"type":"input_text","text":"继续"}]}
		]`),
	}

	out, err := ResponsesToAnthropicRequest(req)
	require.NoError(t, err)
	require.Len(t, out.Messages, 1)

	only := anthropicMessageText(t, out.Messages[0])
	require.Equal(t, "继续", only)
	require.NotContains(t, only, "<conversation_summary>")
}

// 新增的 Summary/Status 字段带 omitempty：不含 compaction 的 item 序列化后
// 不得多出这两个键，否则 CC→Responses 桥接的上游请求体会被改变。
func TestResponsesInputItemMarshalOmitsCompactionFields(t *testing.T) {
	encoded, err := json.Marshal(ResponsesInputItem{Role: "user", Content: json.RawMessage(`"hi"`)})
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "summary")
	require.NotContains(t, string(encoded), "status")
}

func TestIsCompactionItemType(t *testing.T) {
	require.True(t, IsCompactionItemType("compaction"))
	require.True(t, IsCompactionItemType("compaction_summary"))
	require.True(t, IsCompactionItemType("  compaction  "))
	require.False(t, IsCompactionItemType("compaction_trigger"))
	require.False(t, IsCompactionItemType("message"))
	require.False(t, IsCompactionItemType(""))
}

func rawURLBase64(t *testing.T, payload string) string {
	t.Helper()
	return base64.RawURLEncoding.EncodeToString([]byte(payload))
}

// anthropicMessageText 取出消息的全部文本。Content 是多态的：单条消息为纯字符串，
// 而 mergeConsecutiveMessages 合并同角色消息后会变成内容块数组（Anthropic 要求
// 角色交替，所以合并是预期行为）。断言必须同时兼容两种形态。
func anthropicMessageText(t *testing.T, msg AnthropicMessage) string {
	t.Helper()

	var plain string
	if err := json.Unmarshal(msg.Content, &plain); err == nil {
		return plain
	}

	var blocks []AnthropicContentBlock
	require.NoError(t, json.Unmarshal(msg.Content, &blocks))
	parts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if block.Text != "" {
			parts = append(parts, block.Text)
		}
	}
	return strings.Join(parts, "\n")
}
