package kiro

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/stretchr/testify/require"
)

func rawJSON(t *testing.T, s string) json.RawMessage {
	t.Helper()
	require.True(t, json.Valid([]byte(s)), "测试用例里的 JSON 无效: %s", s)
	return json.RawMessage(s)
}

func TestFlattenSystemStringAndBlocks(t *testing.T) {
	t.Parallel()

	got, err := FlattenSystem(rawJSON(t, `"you are helpful"`))
	require.NoError(t, err)
	require.Equal(t, "you are helpful", got)

	got, err = FlattenSystem(rawJSON(t, `[{"type":"text","text":"a"},{"type":"text","text":"b"}]`))
	require.NoError(t, err)
	require.Equal(t, "a\n\nb", got)

	got, err = FlattenSystem(nil)
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestFromAnthropicTextAndImage(t *testing.T) {
	t.Parallel()

	req := &apicompat.AnthropicRequest{
		Messages: []apicompat.AnthropicMessage{{
			Role: "user",
			Content: rawJSON(t, `[
				{"type":"text","text":"look"},
				{"type":"image","source":{"type":"base64","media_type":"image/png","data":"QUJD"}}
			]`),
		}},
	}

	msgs, err := FromAnthropic(req)
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	require.Equal(t, "user", msgs[0].Role)
	require.Equal(t, "look", msgs[0].Text)
	require.Len(t, msgs[0].Images, 1)
	require.Equal(t, "png", msgs[0].Images[0].Format)
	require.Equal(t, "QUJD", msgs[0].Images[0].Data)
}

func TestFromAnthropicRejectsNonBase64Image(t *testing.T) {
	t.Parallel()

	req := &apicompat.AnthropicRequest{
		Messages: []apicompat.AnthropicMessage{{
			Role:    "user",
			Content: rawJSON(t, `[{"type":"image","source":{"type":"url","url":"https://example.com/a.png"}}]`),
		}},
	}

	_, err := FromAnthropic(req)
	require.ErrorIs(t, err, ErrUnsupportedImageSource)
}

// TestFromAnthropicAggregatesParallelToolResults 是本任务最重要的测试。
// Claude Code 并行调用工具时，多个 tool_result 会出现在同一条 user message 里。
// 丢任何一个都会让上游看到不完整的工具轮次。
func TestFromAnthropicAggregatesParallelToolResults(t *testing.T) {
	t.Parallel()

	req := &apicompat.AnthropicRequest{
		Messages: []apicompat.AnthropicMessage{{
			Role: "user",
			Content: rawJSON(t, `[
				{"type":"tool_result","tool_use_id":"tu_1","content":"first"},
				{"type":"tool_result","tool_use_id":"tu_2","content":[{"type":"text","text":"second"}]},
				{"type":"tool_result","tool_use_id":"tu_3","content":"boom","is_error":true}
			]`),
		}},
	}

	msgs, err := FromAnthropic(req)
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	require.Len(t, msgs[0].ToolResults, 3, "三个 tool_result 必须全部保留")
	require.Equal(t, "tu_1", msgs[0].ToolResults[0].ToolUseID)
	require.Equal(t, "first", msgs[0].ToolResults[0].Text)
	require.Equal(t, "second", msgs[0].ToolResults[1].Text)
	require.True(t, msgs[0].ToolResults[2].IsError)
}

func TestFromAnthropicToolUse(t *testing.T) {
	t.Parallel()

	req := &apicompat.AnthropicRequest{
		Messages: []apicompat.AnthropicMessage{{
			Role: "assistant",
			Content: rawJSON(t, `[
				{"type":"text","text":"calling"},
				{"type":"tool_use","id":"tu_1","name":"Read","input":{"path":"/a"}}
			]`),
		}},
	}

	msgs, err := FromAnthropic(req)
	require.NoError(t, err)
	require.Len(t, msgs[0].ToolCalls, 1)
	require.Equal(t, "tu_1", msgs[0].ToolCalls[0].ID)
	require.Equal(t, "Read", msgs[0].ToolCalls[0].Name)
	require.JSONEq(t, `{"path":"/a"}`, string(msgs[0].ToolCalls[0].Input))
}

func TestMergeAdjacentJoinsSameRole(t *testing.T) {
	t.Parallel()

	in := []Msg{
		{Role: "user", Text: "a"},
		{Role: "user", Text: "b", ToolResults: []ToolResult{{ToolUseID: "t1", Text: "r"}}},
		{Role: "assistant", Text: "c"},
	}

	out := MergeAdjacent(in)
	require.Len(t, out, 2)
	require.Equal(t, "a\n\nb", out[0].Text)
	require.Len(t, out[0].ToolResults, 1, "合并时 toolResults 不得丢失")
	require.Equal(t, "assistant", out[1].Role)
}

func TestMergeAdjacentCarriesAllThreeSlices(t *testing.T) {
	t.Parallel()

	in := []Msg{
		{
			Role:        "user",
			Text:        "first",
			Images:      []Image{{Format: "png", Data: "AAA"}},
			ToolCalls:   []ToolCall{{ID: "tc1", Name: "Foo", Input: []byte("{}")}},
			ToolResults: []ToolResult{{ToolUseID: "tr1", Text: "result1"}},
		},
		{
			Role:        "user",
			Text:        "second",
			Images:      []Image{{Format: "jpg", Data: "BBB"}},
			ToolCalls:   []ToolCall{{ID: "tc2", Name: "Bar", Input: []byte("{}")}},
			ToolResults: []ToolResult{{ToolUseID: "tr2", Text: "result2"}},
		},
	}

	out := MergeAdjacent(in)
	require.Len(t, out, 1)
	require.Len(t, out[0].Images, 2, "合并时 Images 不得丢失")
	require.Len(t, out[0].ToolCalls, 2, "合并时 ToolCalls 不得丢失")
	require.Len(t, out[0].ToolResults, 2, "合并时 ToolResults 不得丢失")
}

func TestEnsureFirstIsUserDropsLeadingAssistant(t *testing.T) {
	t.Parallel()

	out := EnsureFirstIsUser([]Msg{
		{Role: "assistant", Text: "stray"},
		{Role: "user", Text: "hi"},
	})
	require.Len(t, out, 1)
	require.Equal(t, "user", out[0].Role)

	// 全是 assistant 时返回空，由上层决定如何兜底。
	require.Empty(t, EnsureFirstIsUser([]Msg{{Role: "assistant", Text: "x"}}))
}

func TestEnsureAlternatingInsertsFiller(t *testing.T) {
	t.Parallel()

	// MergeAdjacent 之后理论上不会有连续同角色，但防御性地保证不变式。
	out := EnsureAlternating([]Msg{
		{Role: "user", Text: "a"},
		{Role: "user", Text: "b"},
	})

	require.Len(t, out, 3)
	require.Equal(t, "user", out[0].Role)
	require.Equal(t, "assistant", out[1].Role)
	require.Equal(t, "user", out[2].Role)
}

func TestStripToolContentRemovesCallsAndResults(t *testing.T) {
	t.Parallel()

	out := StripToolContent([]Msg{{
		Role:        "user",
		Text:        "keep",
		ToolCalls:   []ToolCall{{ID: "a"}},
		ToolResults: []ToolResult{{ToolUseID: "b"}},
	}})

	require.Equal(t, "keep", out[0].Text)
	require.Empty(t, out[0].ToolCalls)
	require.Empty(t, out[0].ToolResults)
}

func TestStripToolContentDoesNotMutateInput(t *testing.T) {
	t.Parallel()

	in := []Msg{{
		Role:        "user",
		Text:        "keep",
		ToolCalls:   []ToolCall{{ID: "a"}},
		ToolResults: []ToolResult{{ToolUseID: "b"}},
	}}

	out := StripToolContent(in)

	// 输入必须保持不变
	require.Len(t, in[0].ToolCalls, 1, "输入消息的 ToolCalls 不得被修改")
	require.Len(t, in[0].ToolResults, 1, "输入消息的 ToolResults 不得被修改")

	// 返回值必须被清空
	require.Empty(t, out[0].ToolCalls)
	require.Empty(t, out[0].ToolResults)
}
