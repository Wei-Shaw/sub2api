package kiro

import (
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/stretchr/testify/require"
)

func TestEstimateTextEmpty(t *testing.T) {
	t.Parallel()
	require.Zero(t, EstimateText(""))
}

func TestEstimateTextShortStrings(t *testing.T) {
	t.Parallel()

	// 长度 < 5 走 ceil(n/3)，下限 1。
	require.Equal(t, 1, EstimateText("a"))
	require.Equal(t, 1, EstimateText("abc"))
	require.Equal(t, 2, EstimateText("abcd"))
}

func TestEstimateTextAsciiProse(t *testing.T) {
	t.Parallel()

	// 36 个普通 ascii 字符（含空格）→ ceil(36/4.5) = 8
	got := EstimateText("the quick brown fox jumps over lazyy")
	require.Equal(t, 8, got)
}

func TestEstimateTextCJKExactValue(t *testing.T) {
	t.Parallel()

	// 9 个 rune（非 ASCII）→ ceil(9/1.5) = 6。
	// 此测试防护 rune 计数 vs 字节计数回归：字节计数会得 ceil(27/1.5) = 18。
	require.Equal(t, 6, EstimateText("中文字符测试一二三"))
}

func TestEstimateTextSymbolsAndDigits(t *testing.T) {
	t.Parallel()

	// 12 个符号 → ceil(12/1.5) = 8
	require.Equal(t, 8, EstimateText("{}[]()<>!@#$"))
	// 12 个数字 → ceil(12/2) = 6
	require.Equal(t, 6, EstimateText("123456789012"))
}

func TestEstimateTextSymbolBoundaries(t *testing.T) {
	t.Parallel()

	// 9 个字符的表格驱动测试，防护符号范围边界和字符分类错误。
	// 每个输入都选择 9 个字符，使得误分类会改变结果（digit→5, ascii→2, symbol→6 互不相同）。
	tests := []struct {
		input    string
		expected int
		desc     string
	}{
		{strings.Repeat(" ", 9), 2, "space 是 ascii，不是 symbol"},
		{strings.Repeat("!", 9), 6, "! 是 symbol 的下界 ('!'..'/')"},
		{strings.Repeat("/", 9), 6, "/ 是 symbol 范围 ('!'..'/')的上界"},
		{strings.Repeat(":", 9), 6, ": 是 symbol 范围 (':'..'@')的下界"},
		{strings.Repeat("@", 9), 6, "@ 是 symbol 范围 (':'..'@')的上界"},
		{strings.Repeat("`", 9), 6, "` 是 symbol 范围 ('['..'`')的上界"},
		{strings.Repeat("{", 9), 6, "{ 是 symbol 范围 ('{'..'~')的下界"},
		{strings.Repeat("~", 9), 6, "~ 是 symbol 范围 ('{'..'~')的上界"},
		{strings.Repeat("5", 9), 5, "digit 5，验证与 symbol 的分离"},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			require.Equal(t, tt.expected, EstimateText(tt.input), "input: %q", tt.input)
		})
	}
}

func TestEstimateTextNeverZeroForNonEmpty(t *testing.T) {
	t.Parallel()
	require.GreaterOrEqual(t, EstimateText(" "), 1)
}

func TestEstimateRequestInputCoversSystemMessagesTools(t *testing.T) {
	t.Parallel()

	base := &apicompat.AnthropicRequest{
		Messages: []apicompat.AnthropicMessage{
			{Role: "user", Content: rawJSON(t, `"hello world this is a message"`)},
		},
	}
	withSystem := &apicompat.AnthropicRequest{
		System:   rawJSON(t, `"a fairly long system prompt goes here"`),
		Messages: base.Messages,
	}
	withTools := &apicompat.AnthropicRequest{
		Messages: base.Messages,
		Tools: []apicompat.AnthropicTool{{
			Name:        "Read",
			Description: "reads a file from disk and returns its contents",
			InputSchema: rawJSON(t, `{"type":"object","properties":{"path":{"type":"string"}}}`),
		}},
	}

	require.Greater(t, EstimateRequestInput(withSystem), EstimateRequestInput(base),
		"system 必须计入 input token")
	require.Greater(t, EstimateRequestInput(withTools), EstimateRequestInput(base),
		"工具声明必须计入 input token")
}

func TestEstimateRequestInputNil(t *testing.T) {
	t.Parallel()
	require.Zero(t, EstimateRequestInput(nil))
}

// TestEstimateRequestInputImageDoesNotScaleWithBase64Size 覆盖 I3：图片的
// base64 payload 之前被整段字符串化后交给 EstimateText，字节数越大估算的
// input token 越大——一张几 MB 的截图能把估算顶到几十万。修复后，图片只按
// imageTokenEstimate 记一个固定近似值，与 base64 字节数无关。
func TestEstimateRequestInputImageDoesNotScaleWithBase64Size(t *testing.T) {
	t.Parallel()

	smallImage := &apicompat.AnthropicRequest{
		Messages: []apicompat.AnthropicMessage{
			{Role: "user", Content: rawJSON(t, `[
				{"type":"text","text":"look at this"},
				{"type":"image","source":{"type":"base64","media_type":"image/png","data":"`+strings.Repeat("A", 100)+`"}}
			]`)},
		},
	}
	largeImage := &apicompat.AnthropicRequest{
		Messages: []apicompat.AnthropicMessage{
			{Role: "user", Content: rawJSON(t, `[
				{"type":"text","text":"look at this"},
				{"type":"image","source":{"type":"base64","media_type":"image/png","data":"`+strings.Repeat("A", 2_000_000)+`"}}
			]`)},
		},
	}

	small := EstimateRequestInput(smallImage)
	large := EstimateRequestInput(largeImage)

	require.Equal(t, small, large,
		"图片体积从 100 字节涨到 200 万字节，估算的 input token 不应该变化")
	require.Less(t, large, 10_000,
		"修复前一张 2MB 的 base64 图片会把估算顶到几十万 token，实际得到 %d", large)
}

// TestEstimateRequestInputImageCountsFixedConstant 验证单张图片确实计入了
// imageTokenEstimate 这个固定近似值，而不是被直接丢弃估算为 0——I3 的修复
// 目标是"排除 base64 payload"，不是"图片完全不计费"。
func TestEstimateRequestInputImageCountsFixedConstant(t *testing.T) {
	t.Parallel()

	withoutImage := &apicompat.AnthropicRequest{
		Messages: []apicompat.AnthropicMessage{
			{Role: "user", Content: rawJSON(t, `[{"type":"text","text":"look at this"}]`)},
		},
	}
	withImage := &apicompat.AnthropicRequest{
		Messages: []apicompat.AnthropicMessage{
			{Role: "user", Content: rawJSON(t, `[
				{"type":"text","text":"look at this"},
				{"type":"image","source":{"type":"base64","media_type":"image/png","data":"`+strings.Repeat("A", 100)+`"}}
			]`)},
		},
	}

	diff := EstimateRequestInput(withImage) - EstimateRequestInput(withoutImage)
	require.Equal(t, imageTokenEstimate, diff,
		"一张图片必须恰好贡献 imageTokenEstimate 个 token")
}

// TestEstimateRequestInputToolResultImageDoesNotScale 覆盖 tool_result 内容
// 里夹带 image 块的情况（工具把图片结果传回模型）——这条路径必须走同一套
// 递归估算，同样不能把 base64 payload 计入。
func TestEstimateRequestInputToolResultImageDoesNotScale(t *testing.T) {
	t.Parallel()

	small := &apicompat.AnthropicRequest{
		Messages: []apicompat.AnthropicMessage{
			{Role: "user", Content: rawJSON(t, `[
				{"type":"tool_result","tool_use_id":"tu_1","content":[
					{"type":"image","source":{"type":"base64","media_type":"image/png","data":"`+strings.Repeat("B", 100)+`"}}
				]}
			]`)},
		},
	}
	large := &apicompat.AnthropicRequest{
		Messages: []apicompat.AnthropicMessage{
			{Role: "user", Content: rawJSON(t, `[
				{"type":"tool_result","tool_use_id":"tu_1","content":[
					{"type":"image","source":{"type":"base64","media_type":"image/png","data":"`+strings.Repeat("B", 2_000_000)+`"}}
				]}
			]`)},
		},
	}

	require.Equal(t, EstimateRequestInput(small), EstimateRequestInput(large),
		"tool_result 里的图片同样不能按 base64 字节数计费")
}

// TestEstimateBlockTokensThinkingIncludesSignature 覆盖整分支复核 Finding：
// 多轮对话里客户端回传的 thinking 块同时带 thinking 正文和 signature 字段，
// 只算正文会系统性少算——signature 必须计入。
func TestEstimateBlockTokensThinkingIncludesSignature(t *testing.T) {
	t.Parallel()

	withoutSignature := &apicompat.AnthropicRequest{
		Messages: []apicompat.AnthropicMessage{
			{Role: "assistant", Content: rawJSON(t, `[{"type":"thinking","thinking":"let me think about this"}]`)},
		},
	}
	withSignature := &apicompat.AnthropicRequest{
		Messages: []apicompat.AnthropicMessage{
			{Role: "assistant", Content: rawJSON(t, `[{"type":"thinking","thinking":"let me think about this","signature":"`+strings.Repeat("s", 200)+`"}]`)},
		},
	}

	require.Greater(t, EstimateRequestInput(withSignature), EstimateRequestInput(withoutSignature),
		"signature 字段必须计入 thinking 块的估算")
}

// TestEstimateBlockTokensUnmodeledTypesFallBackToRawJSON 覆盖整分支复核
// Finding：redacted_thinking 的实际载荷在 apicompat.AnthropicContentBlock
// 未建模的 "data" 字段，document/web_search_tool_result 等新块类型同样未
// 建模。这些类型解析进结构体后关心的字段是零值，若直接按 Text 字段估算会
// 得到 0；必须退回该块自身的原始 JSON 字符串估算。
func TestEstimateBlockTokensUnmodeledTypesFallBackToRawJSON(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		raw  string
	}{
		{"redacted_thinking", `[{"type":"redacted_thinking","data":"` + strings.Repeat("r", 200) + `"}]`},
		{"document", `[{"type":"document","source":{"type":"base64","media_type":"application/pdf","data":"x"},"title":"` + strings.Repeat("d", 200) + `"}]`},
		{"unknown future type", `[{"type":"server_tool_use","name":"web_search","input":{"query":"` + strings.Repeat("q", 200) + `"}}]`},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			req := &apicompat.AnthropicRequest{
				Messages: []apicompat.AnthropicMessage{
					{Role: "assistant", Content: rawJSON(t, c.raw)},
				},
			}
			require.NotZero(t, EstimateRequestInput(req), "未建模类型不应估算为 0")
		})
	}
}

// TestEstimateBlockTokensImageStillFixedAmongUnmodeledFallback 是
// TestEstimateRequestInputImageCountsFixedConstant 的兄弟测试：确认给
// default 分支加原始 JSON 回退，不会让 image 块也退回到按 base64 字节数
// 估算——image 分支必须仍在 switch 里显式排在 default 之前拦截。
func TestEstimateBlockTokensImageStillFixedAmongUnmodeledFallback(t *testing.T) {
	t.Parallel()

	small := &apicompat.AnthropicRequest{
		Messages: []apicompat.AnthropicMessage{
			{Role: "user", Content: rawJSON(t, `[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"`+strings.Repeat("A", 100)+`"}}]`)},
		},
	}
	large := &apicompat.AnthropicRequest{
		Messages: []apicompat.AnthropicMessage{
			{Role: "user", Content: rawJSON(t, `[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"`+strings.Repeat("A", 2_000_000)+`"}}]`)},
		},
	}

	require.Equal(t, EstimateRequestInput(small), EstimateRequestInput(large))
	require.Equal(t, imageTokenEstimate, EstimateRequestInput(small))
}
