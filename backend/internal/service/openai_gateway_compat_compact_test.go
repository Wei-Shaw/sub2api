package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
)

func TestIsCompatCompactionRequest(t *testing.T) {
	compact := []byte(`{"model":"m","input":[{"type":"message","role":"user","content":[]},{"type":"compaction_trigger"}]}`)
	require.True(t, isCompatCompactionRequest(compact))

	normal := []byte(`{"model":"m","input":[{"type":"message","role":"user","content":[]}]}`)
	require.False(t, isCompatCompactionRequest(normal))
}

// compaction_trigger 必须变成一条模型看得懂的总结指令，否则上游收不到任何
// 「请压缩」的信号，会按普通一轮对话回复。
func TestRewriteCompatCompactRequestBody_TriggerBecomesInstruction(t *testing.T) {
	body := []byte(`{"model":"m","stream":true,"tools":[{"type":"function","name":"exec"}],"input":[
		{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"}]},
		{"type":"compaction_trigger"}
	]}`)

	out, err := rewriteCompatCompactRequestBody(body)
	require.NoError(t, err)

	input := gjson.GetBytes(out, "input").Array()
	require.Len(t, input, 2)
	require.Equal(t, "message", input[1].Get("type").String())
	require.Equal(t, "user", input[1].Get("role").String())
	require.Equal(t, grokCompactSummaryPrompt, input[1].Get("content.0.text").String())

	// 压缩是纯总结调用：上游强制非流式，且不允许模型改去调工具。
	require.False(t, gjson.GetBytes(out, "stream").Bool())
	require.Equal(t, "none", gjson.GetBytes(out, "tool_choice").String())

	// 原有的对话消息必须原样保留，否则压缩结果会漏掉历史。
	require.Equal(t, "hello", input[0].Get("content.0.text").String())
}

// 上一轮压缩产生的 compaction item 回传时不能被丢弃，否则压缩后的历史整段丢失。
func TestRewriteCompatCompactRequestBody_ReplaysPriorCompaction(t *testing.T) {
	body := []byte(`{"model":"m","input":[
		{"type":"compaction","summary":[{"type":"summary_text","text":"earlier work"}]},
		{"type":"message","role":"user","content":[{"type":"input_text","text":"next"}]},
		{"type":"compaction_trigger"}
	]}`)

	out, err := rewriteCompatCompactRequestBody(body)
	require.NoError(t, err)

	input := gjson.GetBytes(out, "input").Array()
	require.Len(t, input, 3)
	require.Equal(t, "message", input[0].Get("type").String())
	summary := input[0].Get("content.0.text").String()
	require.Contains(t, summary, "<"+compatCompactSummaryTag+">")
	require.Contains(t, summary, "earlier work")
}

// 没有 compaction_trigger 的请求必须原样返回，避免影响普通对话。
func TestRewriteCompatCompactRequestBody_NoTriggerIsUnchanged(t *testing.T) {
	body := []byte(`{"model":"m","stream":true,"input":[{"type":"message","role":"user","content":[]}]}`)
	out, err := rewriteCompatCompactRequestBody(body)
	require.NoError(t, err)
	require.JSONEq(t, string(body), string(out))
}

func TestBuildCompatCompactResponse_SingleCompactionItem(t *testing.T) {
	content, err := json.Marshal("summary text")
	require.NoError(t, err)
	resp := &apicompat.ChatCompletionsResponse{
		ID: "chatcmpl-1",
		Choices: []apicompat.ChatChoice{{
			Message: apicompat.ChatMessage{Role: "assistant", Content: content},
		}},
		Usage: &apicompat.ChatUsage{PromptTokens: 10, CompletionTokens: 4, TotalTokens: 14},
	}

	out, err := buildCompatCompactResponse(resp, "gpt-5.6-sol")
	require.NoError(t, err)
	require.Equal(t, "gpt-5.6-sol", out.Model)
	require.Equal(t, "completed", out.Status)
	require.Len(t, out.Output, 1, "Codex 要求 output 里恰好一个 compaction item")
	require.Equal(t, "compaction", out.Output[0].Type)
	require.True(t, strings.HasPrefix(out.Output[0].ID, "cmp_"))
	require.Len(t, out.Output[0].Summary, 1)
	require.Equal(t, "summary text", out.Output[0].Summary[0].Text)
	require.NotNil(t, out.Usage)
	require.Equal(t, 10, out.Usage.InputTokens)
}

// 只回推理、不回正文的上游（部分国产 thinking 模型）用推理文本兜底，
// 好过交回一个空摘要。
func TestBuildCompatCompactResponse_FallsBackToReasoning(t *testing.T) {
	resp := &apicompat.ChatCompletionsResponse{
		Choices: []apicompat.ChatChoice{{
			Message: apicompat.ChatMessage{Role: "assistant", ReasoningContent: "thought summary"},
		}},
	}
	out, err := buildCompatCompactResponse(resp, "m")
	require.NoError(t, err)
	require.Equal(t, "thought summary", out.Output[0].Summary[0].Text)
	require.True(t, strings.HasPrefix(out.ID, "resp_"), "缺少上游 id 时必须自行补一个，Codex 要求 response.id 必填")
}

func TestBuildCompatCompactResponse_EmptySummaryIsError(t *testing.T) {
	resp := &apicompat.ChatCompletionsResponse{
		Choices: []apicompat.ChatChoice{{Message: apicompat.ChatMessage{Role: "assistant"}}},
	}
	_, err := buildCompatCompactResponse(resp, "m")
	require.Error(t, err)
}

// 合成出来的响应必须能被 compact SSE 桥转成 Codex 认得的事件序列。
func TestBuildCompatCompactResponse_FeedsCompactSSEBridge(t *testing.T) {
	content, err := json.Marshal("summary text")
	require.NoError(t, err)
	resp := &apicompat.ChatCompletionsResponse{
		ID:      "chatcmpl-1",
		Choices: []apicompat.ChatChoice{{Message: apicompat.ChatMessage{Role: "assistant", Content: content}}},
		Usage:   &apicompat.ChatUsage{PromptTokens: 10, CompletionTokens: 4, TotalTokens: 14},
	}
	compactResp, err := buildCompatCompactResponse(resp, "m")
	require.NoError(t, err)
	encoded, err := json.Marshal(compactResp)
	require.NoError(t, err)

	payload, ok := buildOpenAICompactSSEPayload(encoded)
	require.True(t, ok)

	text := string(payload)
	require.Equal(t, 1, strings.Count(text, "event: response.output_item.done"))
	require.Contains(t, text, `"type":"compaction"`)
	require.Contains(t, text, "event: response.completed")
}

func TestChatMessagePlainText(t *testing.T) {
	stringContent, err := json.Marshal("plain")
	require.NoError(t, err)
	require.Equal(t, "plain", chatMessagePlainText(stringContent))

	partsContent := json.RawMessage(`[{"type":"text","text":"a"},{"type":"text","text":"b"}]`)
	require.Equal(t, "a\nb", chatMessagePlainText(partsContent))

	require.Equal(t, "", chatMessagePlainText(nil))
}
