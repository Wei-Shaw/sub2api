//go:build unit

package antigravity

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// 工具进度回退（SetToolProgressFallback）默认关闭：只有调用方显式启用后，
// 才会在“整条响应在工具调用前没有任何可见正文”的情况下补一条普通 text 提示。
const toolProgressTextPrefix = "【工具进度】"

// toolProgressLine 构造一条包含指定 parts 的 v1internal 流式数据行。
func toolProgressLine(parts string) string {
	return `data: {"response":{"candidates":[{"content":{"parts":[` + parts + `]},"finishReason":"STOP"}]}}`
}

// 关闭（默认）时，只有工具调用的响应依旧不产生任何正文。
func TestToolProgressFallback_DisabledByDefaultKeepsToolOnlyWithoutText(t *testing.T) {
	p := NewStreamingProcessor("gemini-3.8-pro-preview")
	require.False(t, p.toolProgressFallback, "默认应关闭")

	out := string(p.ProcessLine(toolProgressLine(`{"functionCall":{"name":"read_file","args":{"path":"a.txt"}}}`)))
	require.Contains(t, out, `"type":"tool_use"`)
	require.Contains(t, out, `"name":"read_file"`)
	require.NotContains(t, out, toolProgressTextPrefix)
	require.NotContains(t, out, "text_delta", "关闭时 tool-only 响应不应出现正文")
	require.False(t, p.hasVisibleText)

	// 显式关闭同样保持原有行为。
	p2 := NewStreamingProcessor("gemini-3.8-pro-preview")
	p2.SetToolProgressFallback(false)
	out2 := string(p2.ProcessLine(toolProgressLine(`{"functionCall":{"name":"read_file","args":{"path":"a.txt"}}}`)))
	require.NotContains(t, out2, toolProgressTextPrefix)
	require.NotContains(t, out2, "text_delta")
}

// 启用后，tool-only 响应先补一条正文，再输出工具块。
func TestToolProgressFallback_EnabledEmitsTextBeforeTool(t *testing.T) {
	p := NewStreamingProcessor("gemini-3.8-pro-preview")
	p.SetToolProgressFallback(true)

	out := string(p.ProcessLine(toolProgressLine(`{"functionCall":{"name":"read_file","args":{"path":"a.txt"}}}`)))
	require.True(t, p.hasVisibleText)

	textIdx := strings.Index(out, "text_delta")
	toolIdx := strings.Index(out, `"type":"tool_use"`)
	require.NotEqual(t, -1, textIdx, "启用后应补一条正文")
	require.NotEqual(t, -1, toolIdx)
	require.Less(t, textIdx, toolIdx, "进度正文必须出现在 tool_use 之前")

	require.Contains(t, out, `"delta":{"text":"【工具进度】接下来调用 read_file，继续处理当前任务。","type":"text_delta"}`)
	require.Equal(t, 1, strings.Count(out, "text_delta"))
	require.Contains(t, out, `"name":"read_file"`)
	require.Contains(t, out, `"partial_json"`)
	require.Contains(t, out, `{\"path\":\"a.txt\"}`, "原工具参数不应被改动")
}

// 已经有可见正文时不再追加进度提示。
func TestToolProgressFallback_SkipsWhenVisibleTextExists(t *testing.T) {
	p := NewStreamingProcessor("gemini-3.8-pro-preview")
	p.SetToolProgressFallback(true)

	out := string(p.ProcessLine(toolProgressLine(`{"text":"先看一下目录结构。"},{"functionCall":{"name":"grep","args":{"q":"main"}}}`)))
	require.True(t, p.hasVisibleText)
	require.Contains(t, out, "先看一下目录结构。")
	require.NotContains(t, out, toolProgressTextPrefix, "已有正文时不应追加工具进度")
	require.Equal(t, 1, strings.Count(out, "text_delta"))
	require.Contains(t, out, `"name":"grep"`)
	require.Contains(t, out, `{\"q\":\"main\"}`)
}

// thinking 不算可见正文：只有思考再调用工具时仍补进度提示，且思考内容不外泄到正文。
func TestToolProgressFallback_ThinkingOnlyStillEmits(t *testing.T) {
	p := NewStreamingProcessor("gemini-3.8-pro-preview")
	p.SetToolProgressFallback(true)

	thinkOut := string(p.ProcessLine(toolProgressLine(`{"text":"内部推理内容不应外泄","thought":true}`)))
	require.Contains(t, thinkOut, "thinking_delta")
	require.Contains(t, thinkOut, "内部推理内容不应外泄")
	require.False(t, p.hasVisibleText, "thinking 不算可见正文")
	require.NotContains(t, thinkOut, toolProgressTextPrefix)

	toolOut := string(p.ProcessLine(toolProgressLine(`{"functionCall":{"name":"run_bash","args":{"cmd":"ls -la"}}}`)))
	require.Contains(t, toolOut, `"delta":{"text":"【工具进度】接下来调用 run_bash，继续处理当前任务。","type":"text_delta"}`)
	require.Equal(t, 1, strings.Count(toolOut, "text_delta"))
	require.NotContains(t, toolOut, "内部推理内容不应外泄", "进度提示不应携带 thinking 内容")
	require.Contains(t, toolOut, `"name":"run_bash"`)
	require.Contains(t, toolOut, `{\"cmd\":\"ls -la\"}`)
}

// 同一条响应里有多个工具时只补一次，两个工具的块和参数都保留。
func TestToolProgressFallback_OnlyOnceAcrossMultipleTools(t *testing.T) {
	p := NewStreamingProcessor("gemini-3.8-pro-preview")
	p.SetToolProgressFallback(true)

	out := string(p.ProcessLine(toolProgressLine(`{"functionCall":{"name":"tool_alpha","args":{"a":1}}},{"functionCall":{"name":"tool_beta","args":{"b":2}}}`)))
	require.Equal(t, 1, strings.Count(out, toolProgressTextPrefix), "同一条响应里只应补一次工具进度")
	require.Equal(t, 1, strings.Count(out, "text_delta"))
	require.Contains(t, out, "接下来调用 tool_alpha")
	require.NotContains(t, out, "接下来调用 tool_beta")

	require.Equal(t, 2, strings.Count(out, `"type":"tool_use"`), "两个工具块都应保留")
	require.Contains(t, out, `"name":"tool_alpha"`)
	require.Contains(t, out, `"name":"tool_beta"`)
	require.Equal(t, 2, strings.Count(out, "input_json_delta"))
	require.Contains(t, out, `{\"a\":1}`)
	require.Contains(t, out, `{\"b\":2}`)
}
