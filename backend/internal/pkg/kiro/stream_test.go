package kiro

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/stretchr/testify/require"
)

// eventFrame 复用 eventstream_test.go 的 buildFrame 构造一个事件帧。
func eventFrame(t *testing.T, eventType, payload string) []byte {
	t.Helper()
	return buildFrame(t, [][2]string{
		{":message-type", "event"},
		{":event-type", eventType},
	}, []byte(payload))
}

// collectTypes 提取事件类型序列，便于断言整体流形状。
func collectTypes(events []apicompat.AnthropicStreamEvent) []string {
	out := make([]string, 0, len(events))
	for _, e := range events {
		out = append(out, e.Type)
	}
	return out
}

func TestStreamTextOnly(t *testing.T) {
	t.Parallel()

	tr := NewStreamTranslator("claude-sonnet-4.6", "msg_1", false)

	got, err := tr.Feed(eventFrame(t, "assistantResponseEvent", `{"content":"Hel"}`))
	require.NoError(t, err)
	require.Equal(t, []string{"message_start", "content_block_start", "content_block_delta"}, collectTypes(got))
	require.Equal(t, "msg_1", got[0].Message.ID)
	require.Equal(t, "claude-sonnet-4.6", got[0].Message.Model)
	require.Equal(t, "text", got[1].ContentBlock.Type)
	require.Equal(t, "text_delta", got[2].Delta.Type)
	require.Equal(t, "Hel", got[2].Delta.Text)

	got, err = tr.Feed(eventFrame(t, "assistantResponseEvent", `{"content":"lo"}`))
	require.NoError(t, err)
	require.Equal(t, []string{"content_block_delta"}, collectTypes(got))
	require.Equal(t, "lo", got[0].Delta.Text)

	final := tr.Finalize()
	require.Equal(t, []string{"content_block_stop", "message_delta", "message_stop"}, collectTypes(final))
	require.Equal(t, "end_turn", final[1].Delta.StopReason)
	require.True(t, tr.SawContent())
}

func TestStreamNoContentSawContentFalse(t *testing.T) {
	t.Parallel()

	tr := NewStreamTranslator("m", "msg_1", false)
	require.False(t, tr.SawContent(), "首字节前失败可重试，SawContent 必须为 false")
}

// TestStreamToolUseAccumulatesPartialJSON 覆盖 toolUseEvent 的分片语义。
func TestStreamToolUseAccumulatesPartialJSON(t *testing.T) {
	t.Parallel()

	tr := NewStreamTranslator("m", "msg_1", false)

	_, err := tr.Feed(eventFrame(t, "assistantResponseEvent", `{"content":"working"}`))
	require.NoError(t, err)

	got, err := tr.Feed(eventFrame(t, "toolUseEvent",
		`{"name":"Read","toolUseId":"tu_1","input":"{\"pa","stop":false}`))
	require.NoError(t, err)
	// 文本块要先关闭，再开工具块。
	require.Equal(t, []string{"content_block_stop", "content_block_start", "content_block_delta"}, collectTypes(got))
	require.Equal(t, "tool_use", got[1].ContentBlock.Type)
	require.Equal(t, "tu_1", got[1].ContentBlock.ID)
	require.Equal(t, "Read", got[1].ContentBlock.Name)
	require.Equal(t, "input_json_delta", got[2].Delta.Type)
	require.Equal(t, `{"pa`, got[2].Delta.PartialJSON)

	got, err = tr.Feed(eventFrame(t, "toolUseEvent",
		`{"name":"Read","toolUseId":"tu_1","input":"th\":1}","stop":true}`))
	require.NoError(t, err)
	require.Equal(t, []string{"content_block_delta", "content_block_stop"}, collectTypes(got))
	require.Equal(t, `th":1}`, got[0].Delta.PartialJSON)

	final := tr.Finalize()
	require.Equal(t, []string{"message_delta", "message_stop"}, collectTypes(final))
	require.Equal(t, "tool_use", final[0].Delta.StopReason, "有工具调用时 stop_reason 必须是 tool_use")
}

func TestStreamTwoToolCallsGetDistinctIndices(t *testing.T) {
	t.Parallel()

	tr := NewStreamTranslator("m", "msg_1", false)

	got, err := tr.Feed(eventFrame(t, "toolUseEvent", `{"name":"A","toolUseId":"tu_1","input":"{}","stop":true}`))
	require.NoError(t, err)
	firstIdx := *got[1].Index

	got, err = tr.Feed(eventFrame(t, "toolUseEvent", `{"name":"B","toolUseId":"tu_2","input":"{}","stop":true}`))
	require.NoError(t, err)
	secondIdx := *got[0].Index

	require.Equal(t, firstIdx+1, secondIdx, "第二个工具块的 index 必须递增")
}

// TestStreamMetadataStopReason 是 metadataEvent 的端到端回归：
// 漏掉这个事件会让 stop_reason 永远退化为 end_turn。
func TestStreamMetadataStopReason(t *testing.T) {
	t.Parallel()

	tr := NewStreamTranslator("m", "msg_1", false)
	_, err := tr.Feed(eventFrame(t, "assistantResponseEvent", `{"content":"x"}`))
	require.NoError(t, err)

	got, err := tr.Feed(eventFrame(t, "metadataEvent", `{"stopReason":"max_tokens"}`))
	require.NoError(t, err)
	require.Empty(t, got, "metadataEvent 本身不产出 SSE 事件")

	final := tr.Finalize()
	require.Equal(t, "max_tokens", final[1].Delta.StopReason)
}

func TestStreamMeteringFillsUsageAndCredits(t *testing.T) {
	t.Parallel()

	tr := NewStreamTranslator("m", "msg_1", false)
	_, err := tr.Feed(eventFrame(t, "assistantResponseEvent", `{"content":"hello world"}`))
	require.NoError(t, err)

	_, err = tr.Feed(eventFrame(t, "meteringEvent",
		`{"unit":"credit","usage":2.5,"cacheReadInputTokens":100,"cacheCreationInputTokens":20}`))
	require.NoError(t, err)

	usage := tr.Usage()
	require.Equal(t, 100, usage.CacheReadInputTokens, "cache token 必须用上游真实值")
	require.Equal(t, 20, usage.CacheCreationInputTokens)
	require.Positive(t, usage.OutputTokens, "output token 由估算得出")
	require.InDelta(t, 2.5, tr.Credits(), 1e-9)
}

// TestStreamSetInputTokensReachesMessageDeltaUsage 覆盖 I4——SetInputTokens
// 写入的值必须能在 message_delta 事件的 usage.input_tokens 里读到，也必须能
// 从 Usage() 里读到。修复前没有任何写入入口，input_tokens 恒为 0。
func TestStreamSetInputTokensReachesMessageDeltaUsage(t *testing.T) {
	t.Parallel()

	tr := NewStreamTranslator("m", "msg_1", false)
	_, err := tr.Feed(eventFrame(t, "assistantResponseEvent", `{"content":"hello"}`))
	require.NoError(t, err)

	tr.SetInputTokens(123)

	final := tr.Finalize()
	var deltaUsage *apicompat.AnthropicUsage
	for _, e := range final {
		if e.Type == "message_delta" {
			deltaUsage = e.Usage
		}
	}
	require.NotNil(t, deltaUsage, "message_delta 必须带 usage")
	require.Equal(t, 123, deltaUsage.InputTokens,
		"SetInputTokens 写入的值必须出现在 message_delta 的 usage.input_tokens 里")
	require.Equal(t, 123, tr.Usage().InputTokens)
}

// TestStreamWithoutSetInputTokensDefaultsToZero 验证不调用 SetInputTokens 时
// input_tokens 保持默认 0，而不是 panic 或产生别的意外值——覆盖"调用方忘了调用"
// 这个边界。
func TestStreamWithoutSetInputTokensDefaultsToZero(t *testing.T) {
	t.Parallel()

	tr := NewStreamTranslator("m", "msg_1", false)
	_, err := tr.Feed(eventFrame(t, "assistantResponseEvent", `{"content":"hello"}`))
	require.NoError(t, err)
	tr.Finalize()

	require.Zero(t, tr.Usage().InputTokens)
}

// TestStreamPromptCacheFallbackUsedWhenMeteringAbsent 覆盖 Usage() 的兜底
// 分支：上游全程没有给出任何非零 cache_read/cache_creation 时（真实调查证实
// 的常态），SetPromptCacheFallback 提供的本地模拟值必须被采用，且
// InputTokens 要相应扣减。
func TestStreamPromptCacheFallbackUsedWhenMeteringAbsent(t *testing.T) {
	t.Parallel()

	tr := NewStreamTranslator("m", "msg_1", false)
	_, err := tr.Feed(eventFrame(t, "assistantResponseEvent", `{"content":"hello"}`))
	require.NoError(t, err)

	tr.SetInputTokens(1000)
	tr.SetPromptCacheFallback(200, 300, 100, 200)

	usage := tr.Usage()
	require.Equal(t, 200, usage.CacheCreationInputTokens)
	require.Equal(t, 300, usage.CacheReadInputTokens)
	require.Equal(t, 500, usage.InputTokens, "input_tokens 必须扣掉 cache_read+cache_creation")

	fiveMin, oneHour := tr.PromptCacheCreationSplit()
	require.Equal(t, 100, fiveMin)
	require.Equal(t, 200, oneHour)
}

// TestStreamPromptCacheFallbackIgnoredWhenMeteringPresent 覆盖分层优先级：
// 上游 meteringEvent 一旦给出任何非零值，必须原样使用，本地模拟值即便被设置
// 也不能覆盖——不能破坏 TestStreamMeteringFillsUsageAndCredits 已经锁定的行为。
func TestStreamPromptCacheFallbackIgnoredWhenMeteringPresent(t *testing.T) {
	t.Parallel()

	tr := NewStreamTranslator("m", "msg_1", false)
	_, err := tr.Feed(eventFrame(t, "assistantResponseEvent", `{"content":"hello"}`))
	require.NoError(t, err)

	tr.SetInputTokens(1000)
	_, err = tr.Feed(eventFrame(t, "meteringEvent",
		`{"unit":"credit","usage":2.5,"cacheReadInputTokens":100,"cacheCreationInputTokens":20}`))
	require.NoError(t, err)
	// 即便之后（或之前）设置了本地兜底值，真实值也必须优先——调用顺序不该
	// 影响这条优先级规则。
	tr.SetPromptCacheFallback(999, 999, 999, 999)

	usage := tr.Usage()
	require.Equal(t, 20, usage.CacheCreationInputTokens, "上游真实值必须优先于本地模拟值")
	require.Equal(t, 100, usage.CacheReadInputTokens)
	require.Equal(t, 880, usage.InputTokens)

	fiveMin, oneHour := tr.PromptCacheCreationSplit()
	require.Zero(t, fiveMin, "采用真实值时不做 5m/1h 拆分")
	require.Zero(t, oneHour)
}

func TestStreamFakeThinkingStripsBlock(t *testing.T) {
	t.Parallel()

	tr := NewStreamTranslator("m", "msg_1", true)

	var all []apicompat.AnthropicStreamEvent
	for _, chunk := range []string{"<thinking>let me ", "reason</thinking>", "final answer"} {
		got, err := tr.Feed(eventFrame(t, "assistantResponseEvent", `{"content":`+quoteJSON(chunk)+`}`))
		require.NoError(t, err)
		all = append(all, got...)
	}
	all = append(all, tr.Finalize()...)

	var thinking, text string
	var sawThinkingBlock, sawTextBlock bool
	for _, e := range all {
		if e.Type == "content_block_start" && e.ContentBlock != nil {
			switch e.ContentBlock.Type {
			case "thinking":
				sawThinkingBlock = true
			case "text":
				sawTextBlock = true
			}
		}
		if e.Type == "content_block_delta" && e.Delta != nil {
			thinking += e.Delta.Thinking
			text += e.Delta.Text
		}
	}

	require.True(t, sawThinkingBlock)
	require.True(t, sawTextBlock)
	require.Equal(t, "let me reason", thinking)
	require.Equal(t, "final answer", text)
}

func TestStreamFakeThinkingWithoutTagIsAllText(t *testing.T) {
	t.Parallel()

	tr := NewStreamTranslator("m", "msg_1", true)

	var all []apicompat.AnthropicStreamEvent
	got, err := tr.Feed(eventFrame(t, "assistantResponseEvent", `{"content":"just a plain answer"}`))
	require.NoError(t, err)
	all = append(all, got...)
	all = append(all, tr.Finalize()...)

	var text string
	for _, e := range all {
		if e.Type == "content_block_delta" && e.Delta != nil {
			require.Empty(t, e.Delta.Thinking)
			text += e.Delta.Text
		}
	}
	require.Equal(t, "just a plain answer", text)
}

// TestStreamFakeThinkingOpenTagSplitAcrossFrames 覆盖开标签本身被拆到多个帧的情况——
// 之前的测试只拆了闭标签和正文，开标签总是整段到达，从未真正走到
// routeContent 里"缓冲区还只是 <thinking> 前缀，继续等待更多字节"这一分支。
func TestStreamFakeThinkingOpenTagSplitAcrossFrames(t *testing.T) {
	t.Parallel()

	tr := NewStreamTranslator("m", "msg_1", true)

	var all []apicompat.AnthropicStreamEvent
	for _, chunk := range []string{"<thi", "nking>let me ", "reason</thinking>", "final answer"} {
		got, err := tr.Feed(eventFrame(t, "assistantResponseEvent", `{"content":`+quoteJSON(chunk)+`}`))
		require.NoError(t, err)
		all = append(all, got...)
	}
	all = append(all, tr.Finalize()...)

	var thinking, text string
	var sawThinkingBlock, sawTextBlock bool
	for _, e := range all {
		if e.Type == "content_block_start" && e.ContentBlock != nil {
			switch e.ContentBlock.Type {
			case "thinking":
				sawThinkingBlock = true
			case "text":
				sawTextBlock = true
			}
		}
		if e.Type == "content_block_delta" && e.Delta != nil {
			thinking += e.Delta.Thinking
			text += e.Delta.Text
		}
	}

	require.True(t, sawThinkingBlock)
	require.True(t, sawTextBlock)
	require.Equal(t, "let me reason", thinking)
	require.Equal(t, "final answer", text)
	require.NotContains(t, thinking, "<thinking>")
	require.NotContains(t, thinking, "</thinking>")
	require.NotContains(t, text, "<thinking>")
	require.NotContains(t, text, "</thinking>")
}

// TestStreamFakeThinkingLookAlikeTagIsAllText 覆盖"像标签但不是标签"的情况——
// 开头是 "<think" 但后面接的不是 "ing>"，必须整段落回正文，不能被误判为思考标签。
func TestStreamFakeThinkingLookAlikeTagIsAllText(t *testing.T) {
	t.Parallel()

	tr := NewStreamTranslator("m", "msg_1", true)

	got, err := tr.Feed(eventFrame(t, "assistantResponseEvent", `{"content":"<think about it>"}`))
	require.NoError(t, err)
	final := tr.Finalize()

	var thinking, text string
	var sawThinkingBlock bool
	for _, e := range append(got, final...) {
		if e.Type == "content_block_start" && e.ContentBlock != nil && e.ContentBlock.Type == "thinking" {
			sawThinkingBlock = true
		}
		if e.Type == "content_block_delta" && e.Delta != nil {
			thinking += e.Delta.Thinking
			text += e.Delta.Text
		}
	}

	require.False(t, sawThinkingBlock, "\"<think about it>\" 不是思考标签，不应开出 thinking block")
	require.Empty(t, thinking)
	require.Equal(t, "<think about it>", text)
}

// TestStreamFakeThinkingShortPrefixFlushedOnFinalize 覆盖流在标签判定完成前就结束的情况——
// 内容比 "<thinking>" 本身还短，门控缓冲必须在 Finalize 时冲刷成正文，不能被吞掉。
func TestStreamFakeThinkingShortPrefixFlushedOnFinalize(t *testing.T) {
	t.Parallel()

	tr := NewStreamTranslator("m", "msg_1", true)

	got, err := tr.Feed(eventFrame(t, "assistantResponseEvent", `{"content":"<th"}`))
	require.NoError(t, err)
	// message_start 由 ensureStarted 无条件先发；标签判定未完成前不应有任何内容块事件。
	require.Equal(t, []string{"message_start"}, collectTypes(got))

	final := tr.Finalize()

	var text string
	for _, e := range final {
		if e.Type == "content_block_delta" && e.Delta != nil {
			text += e.Delta.Text
			require.Empty(t, e.Delta.Thinking)
		}
	}
	require.Equal(t, "<th", text, "门控缓冲里的内容必须在 Finalize 时冲刷为正文")
	require.True(t, tr.SawContent(), "冲刷出的内容也算已吐出内容")
}

// TestStreamEmptyResponseStillEmitsMessageStart 覆盖响应被静默截断、完全没有
// 正文/工具调用内容的情况（I1）——gateBuf 从未被写入过，handle() 里任何一条
// ensureStarted 分支都不会被触发，之前的实现会导致 Finalize 只发
// message_delta + message_stop，缺失 message_start，破坏协议顺序。
func TestStreamEmptyResponseStillEmitsMessageStart(t *testing.T) {
	t.Parallel()

	tr := NewStreamTranslator("m", "msg_1", false)

	// 只喂一个 metadataEvent，不带任何正文/工具调用内容。
	got, err := tr.Feed(eventFrame(t, "metadataEvent", `{"stopReason":"end_turn"}`))
	require.NoError(t, err)
	require.Empty(t, got, "metadataEvent 本身不产出 SSE 事件")

	final := tr.Finalize()
	require.Equal(t, []string{"message_start", "message_delta", "message_stop"}, collectTypes(final),
		"空响应也必须先发 message_start，保持协议顺序完整")
	require.Equal(t, "msg_1", final[0].Message.ID)
	require.False(t, tr.SawContent())
}

func TestStreamExceptionFrameReturnsError(t *testing.T) {
	t.Parallel()

	raw := buildFrame(t, [][2]string{
		{":message-type", "exception"},
		{":exception-type", "ThrottlingException"},
	}, []byte(`{"message":"slow down"}`))

	tr := NewStreamTranslator("m", "msg_1", false)
	_, err := tr.Feed(raw)
	require.Error(t, err)

	var upstream *UpstreamError
	require.ErrorAs(t, err, &upstream)
	require.Equal(t, "ThrottlingException", upstream.Type)
	require.Contains(t, upstream.Message, "slow down")
}

func TestStreamFinalizeIsIdempotent(t *testing.T) {
	t.Parallel()

	tr := NewStreamTranslator("m", "msg_1", false)
	_, err := tr.Feed(eventFrame(t, "assistantResponseEvent", `{"content":"x"}`))
	require.NoError(t, err)

	first := tr.Finalize()
	require.NotEmpty(t, first)
	require.Empty(t, tr.Finalize(), "重复 Finalize 不得重复产出事件")
}

// TestStreamToolInterleaveWithoutStopClosesFirstBeforeSecondOpens 覆盖两个
// toolUseId 在没有中间 stop:true 的情况下直接切换的场景——curToolID 的判定必须
// 独立生效，否则第二个工具的分片会污染进第一个还开着的工具块里。
func TestStreamToolInterleaveWithoutStopClosesFirstBeforeSecondOpens(t *testing.T) {
	t.Parallel()

	tr := NewStreamTranslator("m", "msg_1", false)

	_, err := tr.Feed(eventFrame(t, "toolUseEvent",
		`{"name":"A","toolUseId":"tu_1","input":"{\"a\":1","stop":false}`))
	require.NoError(t, err)

	got, err := tr.Feed(eventFrame(t, "toolUseEvent",
		`{"name":"B","toolUseId":"tu_2","input":"{\"b\":2}","stop":false}`))
	require.NoError(t, err)

	require.Equal(t, []string{"content_block_stop", "content_block_start", "content_block_delta"}, collectTypes(got),
		"tu_1 的块必须先关闭，再开 tu_2 的块")
	require.Equal(t, "tool_use", got[1].ContentBlock.Type)
	require.Equal(t, "tu_2", got[1].ContentBlock.ID)
	require.Equal(t, *got[0].Index+1, *got[1].Index, "两个工具块的 index 必须递增且不同")
	require.Equal(t, `{"b":2}`, got[2].Delta.PartialJSON)
	require.NotContains(t, got[2].Delta.PartialJSON, `{"a":1`, "tu_2 的分片不能混进 tu_1 的内容")
}

// TestMapStopReasonTable 逐一覆盖 mapStopReason 的每条映射分支，
// 并验证有工具调用时 toolCount>0 必须压制上游给出的任何 stopReason。
func TestMapStopReasonTable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		reason string
		want   string
		desc   string
	}{
		{"", "end_turn", "空字符串落到默认分支"},
		{"max_tokens", "max_tokens", "max_tokens 原生值"},
		{"max_output_tokens", "max_tokens", "max_output_tokens 别名"},
		{"length", "max_tokens", "length 别名"},
		{"model_context_window_exceeded", "model_context_window_exceeded", "model_context_window_exceeded 原生值"},
		{"context_window_exceeded", "model_context_window_exceeded", "context_window_exceeded 别名"},
		{"refusal", "refusal", "refusal 原生值"},
		{"content_filter", "refusal", "content_filter 别名"},
		{"content_filtered", "refusal", "content_filtered 别名"},
		{"guardrail_intervened", "refusal", "guardrail_intervened 别名"},
		{"stop_sequence", "stop_sequence", "stop_sequence 原生值"},
		{"pause_turn", "pause_turn", "pause_turn 原生值"},
		{"some_unrecognized_reason", "end_turn", "未知 reason 落到默认分支"},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			require.Equal(t, tt.want, mapStopReason(tt.reason, 0), "reason=%q toolCount=0", tt.reason)
			require.Equal(t, "tool_use", mapStopReason(tt.reason, 1), "reason=%q toolCount=1 必须被 tool_use 覆盖", tt.reason)
		})
	}
}

// TestStreamFakeThinkingIncludesThinkingInOutputTokens 覆盖假思考剥离出的 thinking
// 文本也必须计入 output token——Anthropic 原生 usage.output_tokens 本身包含
// thinking token，本网关对外呈现 Anthropic 兼容接口，口径要与之一致。
func TestStreamFakeThinkingIncludesThinkingInOutputTokens(t *testing.T) {
	t.Parallel()

	tr := NewStreamTranslator("m", "msg_1", true)

	_, err := tr.Feed(eventFrame(t, "assistantResponseEvent",
		`{"content":"<thinking>let me reason</thinking>final answer"}`))
	require.NoError(t, err)
	tr.Finalize()

	want := EstimateText("let me reason" + "final answer")
	require.Equal(t, want, tr.Usage().OutputTokens, "thinking 文本必须和正文一起计入 output token")
}

// TestStreamToolOnlyResponseCountsInputTowardOutputTokens 覆盖只有工具调用、
// 没有任何正文的响应（I2）——工具调用的 input JSON 片段之前从未累加进
// outputText，导致纯工具调用响应的 OutputTokens 恒为 0，计费漏计。
func TestStreamToolOnlyResponseCountsInputTowardOutputTokens(t *testing.T) {
	t.Parallel()

	tr := NewStreamTranslator("m", "msg_1", false)

	_, err := tr.Feed(eventFrame(t, "toolUseEvent",
		`{"name":"Read","toolUseId":"tu_1","input":"{\"pa","stop":false}`))
	require.NoError(t, err)
	_, err = tr.Feed(eventFrame(t, "toolUseEvent",
		`{"name":"Read","toolUseId":"tu_1","input":"th\":1}","stop":true}`))
	require.NoError(t, err)
	tr.Finalize()

	want := EstimateText(`{"path":1}`)
	require.Equal(t, want, tr.Usage().OutputTokens,
		"纯工具调用响应的 OutputTokens 必须等于累计 input JSON 的估算值，不能是 0")
	require.Positive(t, tr.Usage().OutputTokens)
}

// TestStreamFeedReturnsPartialOutputAlongsideException 覆盖同一次 Feed 调用里
// 先有正文帧、后有异常帧的情况——已经产出的事件必须随错误一起返回，不能被吞掉，
// 否则客户端明明收到过的内容会在重试判定里凭空消失。
func TestStreamFeedReturnsPartialOutputAlongsideException(t *testing.T) {
	t.Parallel()

	textFrame := eventFrame(t, "assistantResponseEvent", `{"content":"partial output"}`)
	excFrame := buildFrame(t, [][2]string{
		{":message-type", "exception"},
		{":exception-type", "ThrottlingException"},
	}, []byte(`{"message":"slow down"}`))
	combined := append(append([]byte{}, textFrame...), excFrame...)

	tr := NewStreamTranslator("m", "msg_1", false)
	got, err := tr.Feed(combined)
	require.Error(t, err)
	require.NotEmpty(t, got, "已经产出的事件必须随错误一起返回")

	var text string
	for _, e := range got {
		if e.Type == "content_block_delta" && e.Delta != nil {
			text += e.Delta.Text
		}
	}
	require.Equal(t, "partial output", text)

	var upstream *UpstreamError
	require.ErrorAs(t, err, &upstream)
	require.Equal(t, "ThrottlingException", upstream.Type)
}

// TestStreamFeedAfterFinalizeIsNoop 覆盖 Finalize 之后再调用 Feed 的越界用法——
// 必须直接返回空，不能在已经发出的 message_stop 之后又吐出 content_block 事件。
func TestStreamFeedAfterFinalizeIsNoop(t *testing.T) {
	t.Parallel()

	tr := NewStreamTranslator("m", "msg_1", false)
	_, err := tr.Feed(eventFrame(t, "assistantResponseEvent", `{"content":"x"}`))
	require.NoError(t, err)
	tr.Finalize()

	got, err := tr.Feed(eventFrame(t, "assistantResponseEvent", `{"content":"y"}`))
	require.NoError(t, err)
	require.Empty(t, got, "Finalize 之后的 Feed 必须是空操作")
}

// quoteJSON 把字符串编码为 JSON 字符串字面量（含引号）。
func quoteJSON(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// TestStreamFakeThinkingCloseTagSplitAcrossFrames 覆盖真实账号测试发现的
// 真实 bug：闭标签 </thinking> 被上游切成两个 chunk 分别到达时，旧实现
// 只在单个 chunk 内用 strings.Index 找闭标签，两个 chunk 各自都找不到，
// 导致闭标签本身连同它之后的真实回答整段被错误吞进思考块——同一道题分别
// 测 Claude 与某个非 Claude 模型时，只有前者的闭标签恰好没被切断，后者
// 被切断后触发了这个 bug。跟 TestStreamFakeThinkingOpenTagSplitAcrossFrames
// 对称，那个测的是开标签被切断，这个测闭标签被切断。
func TestStreamFakeThinkingCloseTagSplitAcrossFrames(t *testing.T) {
	t.Parallel()

	tr := NewStreamTranslator("m", "msg_1", true)

	var all []apicompat.AnthropicStreamEvent
	for _, chunk := range []string{"<thinking>let me reason", "</thin", "king>", "final answer"} {
		got, err := tr.Feed(eventFrame(t, "assistantResponseEvent", `{"content":`+quoteJSON(chunk)+`}`))
		require.NoError(t, err)
		all = append(all, got...)
	}
	all = append(all, tr.Finalize()...)

	var thinking, text string
	var sawThinkingBlock, sawTextBlock bool
	for _, e := range all {
		if e.Type == "content_block_start" && e.ContentBlock != nil {
			switch e.ContentBlock.Type {
			case "thinking":
				sawThinkingBlock = true
			case "text":
				sawTextBlock = true
			}
		}
		if e.Type == "content_block_delta" && e.Delta != nil {
			thinking += e.Delta.Thinking
			text += e.Delta.Text
		}
	}

	require.True(t, sawThinkingBlock)
	require.True(t, sawTextBlock)
	require.Equal(t, "let me reason", thinking, "闭标签被切断不应该把标签片段或之后的正文一起吞进思考块")
	require.Equal(t, "final answer", text, "闭标签之后的真实回答必须完整落进正文块，不能丢失或混进思考块")
	require.NotContains(t, thinking, "<", "不应该把闭标签的任何片段残留在思考内容里")
}

// TestStreamFakeThinkingCloseTagPrefixFlushedOnFinalize 覆盖流在思考块内部
// 被截断（比如撞到 max_tokens）、且截断点恰好卡在疑似闭标签前缀上的情况：
// 缓冲区里那段"看起来像 </thinking> 前缀但一直没等到后续字节确认"的内容
// 必须在 Finalize 时当作思考内容冲刷出去，不能悄悄丢弃——跟 gateBuf 在
// Finalize 时冲刷成正文是同一个"不丢内容"的约定，只是这里冲刷的目标是
// 思考块而不是正文块。
func TestStreamFakeThinkingCloseTagPrefixFlushedOnFinalize(t *testing.T) {
	t.Parallel()

	tr := NewStreamTranslator("m", "msg_1", true)

	got, err := tr.Feed(eventFrame(t, "assistantResponseEvent", `{"content":"<thinking>partial reasoning</th"}`))
	require.NoError(t, err)
	var thinkingBeforeFinalize string
	for _, e := range got {
		if e.Type == "content_block_delta" && e.Delta != nil {
			thinkingBeforeFinalize += e.Delta.Thinking
		}
	}
	require.Equal(t, "partial reasoning", thinkingBeforeFinalize,
		"疑似闭标签前缀 </th 在确认完整闭标签之前不应该被当作思考内容提前发出")

	final := tr.Finalize()

	var thinkingAfterFinalize, textAfterFinalize string
	for _, e := range final {
		if e.Type == "content_block_delta" && e.Delta != nil {
			thinkingAfterFinalize += e.Delta.Thinking
			textAfterFinalize += e.Delta.Text
		}
	}
	require.Equal(t, "</th", thinkingAfterFinalize, "流截断时挂起的疑似闭标签前缀必须当思考内容冲刷，不能丢弃")
	require.Empty(t, textAfterFinalize, "这段内容属于思考块，不应该被冲刷成正文")
}
