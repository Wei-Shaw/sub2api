package kiro

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
)

// thinkingOpenTag / thinkingCloseTag 是假思考模式下模型被要求使用的标签。
const (
	thinkingOpenTag  = "<thinking>"
	thinkingCloseTag = "</thinking>"
)

// UpstreamError 是 Kiro 返回的异常帧。
type UpstreamError struct {
	Type    string
	Code    string
	Message string
}

func (e *UpstreamError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("kiro upstream %s (%s): %s", e.Type, e.Code, e.Message)
	}
	return fmt.Sprintf("kiro upstream %s: %s", e.Type, e.Message)
}

// blockKind 标识当前打开的 content block 类型。
type blockKind int

const (
	blockNone blockKind = iota
	blockThinking
	blockText
	blockToolUse
)

// thinkingPhase 是假思考剥离的状态机阶段。
type thinkingPhase int

const (
	// phasePending：还在判断响应是否以 <thinking> 开头。
	phasePending thinkingPhase = iota
	// phaseInThinking：正在思考块内部。
	phaseInThinking
	// phaseText：已确定后续全部是正文。
	phaseText
)

// StreamTranslator 把 Kiro 的 event-stream 增量翻译成 Anthropic SSE 事件。
// 非并发安全，每个上游响应一个实例。
type StreamTranslator struct {
	model        string
	messageID    string
	fakeThinking bool
	dec          *Decoder

	started   bool
	finalized bool

	nextIndex int
	openKind  blockKind
	openIndex int

	phase   thinkingPhase
	gateBuf string
	// closeBuf 是 phaseInThinking 阶段末尾可能是 </thinking> 前缀的一段
	// 尾巴——上游按任意字节边界切块，闭标签完全可能被切成两半分别落在
	// 相邻两个 chunk 里（真实账号测试发现的真实场景，不是假设性边界情况：
	// 同一道题对 Claude 和某个非 Claude 模型分别测，前者闭标签落在一个
	// chunk 内、后者被切开——只有前者被正确识别）。跟 gateBuf 处理开标签
	// 被截断是同一个思路，这里对称地处理闭标签。
	closeBuf string

	curToolID string
	toolCount int

	outputText  strings.Builder
	stopReason  string
	credits     float64
	cacheRead   int
	cacheWrite  int
	inputTokens int
	sawContent  bool

	// fallbackCache* 是本地模拟出的 prompt cache 用量（见 prompt_cache.go 顶部
	// 文档），只在上游 meteringEvent 全程从未给出任何非零 cache_read/
	// cache_creation 时才会被 Usage() 采用——上游真实值永远优先，与
	// TestKiroForwardUpstreamSuccessStreamingWithRealCacheReadTokens 已经锁定
	// 的行为完全兼容。
	fallbackCacheCreation int
	fallbackCacheRead     int
	fallbackCache5m       int
	fallbackCache1h       int
	hasFallbackCache      bool
}

// NewStreamTranslator 创建翻译器。messageID 由调用方生成，便于与请求日志关联。
// fakeThinking 与请求侧的注入开关必须一致，否则思考标签会漏进正文。
func NewStreamTranslator(model, messageID string, fakeThinking bool) *StreamTranslator {
	t := &StreamTranslator{
		model:        model,
		messageID:    messageID,
		fakeThinking: fakeThinking,
		dec:          NewDecoder(),
		phase:        phasePending,
	}
	if !fakeThinking {
		t.phase = phaseText
	}
	return t
}

// SawContent 返回是否已经向客户端吐出过内容。
// 上层据此决定失败是否可重试：首字节前可重试，已出内容不可重试。
func (t *StreamTranslator) SawContent() bool { return t.sawContent }

// Credits 返回本次请求消耗的 Kiro credits（来自 meteringEvent）。
func (t *StreamTranslator) Credits() float64 { return t.credits }

// SetInputTokens 填充本次请求的 input token 估算值（调用方通常用
// EstimateRequestInput 算出）。必须在 Finalize 之前调用，否则
// message_delta 里的 usage.input_tokens 会保持默认的 0——Kiro 本身不提供
// input token，这一项完全依赖调用方主动写入。
func (t *StreamTranslator) SetInputTokens(n int) { t.inputTokens = n }

// SetPromptCacheFallback 提供本地模拟的 prompt cache 用量（调用方通常用
// PromptCacheTracker.Compute 算出），仅在上游 meteringEvent 全程一次都没有
// 给出任何非零 cache_read/cache_creation 时才会被 Usage() 采用——见 Usage()
// 的判定逻辑。必须在 Finalize 之前调用，通常紧跟在 SetInputTokens 之后
// （调用方此时还没开始 Feed 上游字节，天然满足这个时序要求）。
func (t *StreamTranslator) SetPromptCacheFallback(creation, read, creation5m, creation1h int) {
	t.fallbackCacheCreation = creation
	t.fallbackCacheRead = read
	t.fallbackCache5m = creation5m
	t.fallbackCache1h = creation1h
	t.hasFallbackCache = true
}

// PromptCacheCreationSplit 返回 Usage() 里 cache_creation 的 5m/1h TTL 拆分——
// 只有采用了本地模拟兜底时才非零；Kiro 协议本身不区分 TTL 档位，上游真实值
// 走这里永远是 0/0（计费口径见 billing_service.go 对未拆分场景的默认处理）。
func (t *StreamTranslator) PromptCacheCreationSplit() (fiveMin, oneHour int) {
	if t.cacheRead == 0 && t.cacheWrite == 0 && t.hasFallbackCache {
		return t.fallbackCache5m, t.fallbackCache1h
	}
	return 0, 0
}

// Usage 返回计费用量。cache token 是上游真实值；output token 是估算值 ——
// Kiro 不提供 input/output token，这是既定计费口径。
// output token 统计的是正文文本、假思考剥离出的 thinking 文本、以及工具调用的
// input JSON 三者累加：Anthropic 原生的 usage.output_tokens 本身就包含
// thinking token 和 tool_use.input，本网关对外呈现 Anthropic 兼容接口，口径要
// 与之一致；而且这三者都确实作为对应的 content block 发给了客户端，并非未展示
// 给用户的隐藏消耗。
// InputTokens 由调用方通过 SetInputTokens 填充（通常用 EstimateRequestInput
// 算出），不在这里估算——StreamTranslator 只看到上游流式响应，看不到原始
// 请求内容，没有能力自行估算 input token。cache_read/cache_creation 优先用
// 上游真实值；两者都是 0 时（真实调查证实的常态，见 prompt_cache.go 顶部
// 文档）改用 SetPromptCacheFallback 提供的本地模拟值兜底。返回的
// InputTokens 已经按 Anthropic 真实语义扣掉了 cache 部分——input_tokens 从不
// 包含 cache_read/cache_creation。
func (t *StreamTranslator) Usage() apicompat.AnthropicUsage {
	cacheRead, cacheCreation := t.cacheRead, t.cacheWrite
	if cacheRead == 0 && cacheCreation == 0 && t.hasFallbackCache {
		cacheRead, cacheCreation = t.fallbackCacheRead, t.fallbackCacheCreation
	}
	inputTokens := max(t.inputTokens-cacheRead-cacheCreation, 0)
	return apicompat.AnthropicUsage{
		InputTokens:              inputTokens,
		OutputTokens:             EstimateText(t.outputText.String()),
		CacheReadInputTokens:     cacheRead,
		CacheCreationInputTokens: cacheCreation,
	}
}

// Feed 消费一段上游字节，返回应当下发给客户端的 Anthropic 事件。
// Finalize 之后再调用 Feed 是非法用法，直接返回 (nil, nil)——
// 否则会在已经发出的 message_stop 之后又吐出 content_block 事件，破坏协议顺序。
func (t *StreamTranslator) Feed(chunk []byte) ([]apicompat.AnthropicStreamEvent, error) {
	if t.finalized {
		return nil, nil
	}

	frames, err := t.dec.Feed(chunk)
	if err != nil {
		return nil, err
	}

	var out []apicompat.AnthropicStreamEvent
	for _, f := range frames {
		ev, perr := ParseEvent(f)
		if perr != nil {
			return out, perr
		}

		events, herr := t.handle(ev)
		out = append(out, events...)
		if herr != nil {
			return out, herr
		}
	}
	return out, nil
}

func (t *StreamTranslator) handle(ev Event) ([]apicompat.AnthropicStreamEvent, error) {
	var out []apicompat.AnthropicStreamEvent

	switch ev.Kind {
	case EventAssistantResponse:
		if ev.Assistant == nil || ev.Assistant.Content == "" {
			return nil, nil
		}
		out = append(out, t.ensureStarted()...)
		out = append(out, t.routeContent(ev.Assistant.Content)...)

	case EventToolUse:
		if ev.ToolUse == nil {
			return nil, nil
		}
		out = append(out, t.ensureStarted()...)
		out = append(out, t.handleToolUse(ev.ToolUse)...)

	case EventMetadata:
		if ev.Metadata != nil && ev.Metadata.StopReason != "" {
			t.stopReason = ev.Metadata.StopReason
		}

	case EventMetering:
		if ev.Metering != nil {
			t.credits += ev.Metering.Usage
			t.cacheRead += ev.Metering.CacheReadInputTokens
			t.cacheWrite += ev.Metering.CacheCreationInputTokens
		}

	case EventException:
		ex := ev.Exception
		if ex == nil {
			ex = &Exception{Type: "UnknownException"}
		}
		return out, &UpstreamError{Type: ex.Type, Code: ex.Code, Message: ex.Message}

	case EventContextUsage, EventCodeReference, EventUnknown:
		// 无需下发给客户端。
	}

	return out, nil
}

// ensureStarted 首次产出前发出 message_start。
func (t *StreamTranslator) ensureStarted() []apicompat.AnthropicStreamEvent {
	if t.started {
		return nil
	}
	t.started = true

	return []apicompat.AnthropicStreamEvent{{
		Type: "message_start",
		Message: &apicompat.AnthropicResponse{
			ID:      t.messageID,
			Type:    "message",
			Role:    "assistant",
			Content: []apicompat.AnthropicContentBlock{},
			Model:   t.model,
			Usage:   apicompat.AnthropicUsage{},
		},
	}}
}

// routeContent 把一段文本按假思考状态机分流到 thinking 块或 text 块。
func (t *StreamTranslator) routeContent(s string) []apicompat.AnthropicStreamEvent {
	var out []apicompat.AnthropicStreamEvent

	for s != "" {
		switch t.phase {
		case phasePending:
			t.gateBuf += s
			s = ""

			trimmed := strings.TrimLeft(t.gateBuf, " \t\r\n")
			switch {
			case strings.HasPrefix(trimmed, thinkingOpenTag):
				t.phase = phaseInThinking
				s = strings.TrimPrefix(trimmed, thinkingOpenTag)
				t.gateBuf = ""
			case strings.HasPrefix(thinkingOpenTag, trimmed):
				// 仍可能是 <thinking> 的前缀，继续等待更多字节。
			default:
				t.phase = phaseText
				s = t.gateBuf
				t.gateBuf = ""
			}

		case phaseInThinking:
			combined := t.closeBuf + s
			t.closeBuf = ""
			s = ""

			idx := strings.Index(combined, thinkingCloseTag)
			if idx >= 0 {
				out = append(out, t.emitThinking(combined[:idx])...)
				s = combined[idx+len(thinkingCloseTag):]
				t.phase = phaseText
				break
			}

			// 没找到完整闭标签：结尾可能恰好是闭标签的前缀（比如这个 chunk
			// 以 "</thin" 结尾，下一个 chunk 才补上 "king>"）——扣下这条
			// 可能的前缀，留到下次 Feed 时跟新字节拼起来再判断，不能现在
			// 就当正文/思考内容发出去，否则闭标签一旦真被切断，标签本身
			// 和标签之后的真实回答会整段被错误吞进思考块（真实账号复现过
			// 的 bug，不是理论风险）。
			safeLen := len(combined)
			for i := len(thinkingCloseTag) - 1; i >= 1; i-- {
				if i > len(combined) {
					continue
				}
				if strings.HasSuffix(combined, thinkingCloseTag[:i]) {
					safeLen = len(combined) - i
					break
				}
			}
			t.closeBuf = combined[safeLen:]
			out = append(out, t.emitThinking(combined[:safeLen])...)

		case phaseText:
			out = append(out, t.emitText(s)...)
			s = ""
		}
	}

	return out
}

func (t *StreamTranslator) emitThinking(s string) []apicompat.AnthropicStreamEvent {
	if s == "" {
		return nil
	}
	t.sawContent = true
	_, _ = t.outputText.WriteString(s)

	var out []apicompat.AnthropicStreamEvent
	if t.openKind != blockThinking {
		out = append(out, t.closeBlock()...)
		out = append(out, t.openBlockOf(blockThinking, &apicompat.AnthropicContentBlock{Type: "thinking"}))
	}
	out = append(out, apicompat.AnthropicStreamEvent{
		Type:  "content_block_delta",
		Index: intPtr(t.openIndex),
		Delta: &apicompat.AnthropicDelta{Type: "thinking_delta", Thinking: s},
	})
	return out
}

func (t *StreamTranslator) emitText(s string) []apicompat.AnthropicStreamEvent {
	if s == "" {
		return nil
	}
	t.sawContent = true
	_, _ = t.outputText.WriteString(s)

	var out []apicompat.AnthropicStreamEvent
	if t.openKind != blockText {
		out = append(out, t.closeBlock()...)
		out = append(out, t.openBlockOf(blockText, &apicompat.AnthropicContentBlock{Type: "text"}))
	}
	out = append(out, apicompat.AnthropicStreamEvent{
		Type:  "content_block_delta",
		Index: intPtr(t.openIndex),
		Delta: &apicompat.AnthropicDelta{Type: "text_delta", Text: s},
	})
	return out
}

func (t *StreamTranslator) handleToolUse(tu *ToolUse) []apicompat.AnthropicStreamEvent {
	var out []apicompat.AnthropicStreamEvent

	if tu.ToolUseID != t.curToolID || t.openKind != blockToolUse {
		out = append(out, t.closeBlock()...)
		out = append(out, t.openBlockOf(blockToolUse, &apicompat.AnthropicContentBlock{
			Type:  "tool_use",
			ID:    tu.ToolUseID,
			Name:  tu.Name,
			Input: json.RawMessage("{}"),
		}))
		t.curToolID = tu.ToolUseID
		t.toolCount++
		t.sawContent = true
	}

	if tu.Input != "" {
		// 工具调用的输入 JSON 片段也要计入 output token——Anthropic 原生
		// usage.output_tokens 本身包含 tool_use.input，本网关对外呈现
		// Anthropic 兼容接口，口径要与之一致（I2）。
		_, _ = t.outputText.WriteString(tu.Input)
		out = append(out, apicompat.AnthropicStreamEvent{
			Type:  "content_block_delta",
			Index: intPtr(t.openIndex),
			Delta: &apicompat.AnthropicDelta{Type: "input_json_delta", PartialJSON: tu.Input},
		})
	}

	if tu.Stop {
		out = append(out, t.closeBlock()...)
		t.curToolID = ""
	}

	return out
}

func (t *StreamTranslator) openBlockOf(kind blockKind, block *apicompat.AnthropicContentBlock) apicompat.AnthropicStreamEvent {
	t.openKind = kind
	t.openIndex = t.nextIndex
	t.nextIndex++

	return apicompat.AnthropicStreamEvent{
		Type:         "content_block_start",
		Index:        intPtr(t.openIndex),
		ContentBlock: block,
	}
}

func (t *StreamTranslator) closeBlock() []apicompat.AnthropicStreamEvent {
	if t.openKind == blockNone {
		return nil
	}
	idx := t.openIndex
	t.openKind = blockNone
	return []apicompat.AnthropicStreamEvent{{Type: "content_block_stop", Index: intPtr(idx)}}
}

// Finalize 冲刷缓冲、关闭未完块，并发出收尾事件。重复调用返回空。
func (t *StreamTranslator) Finalize() []apicompat.AnthropicStreamEvent {
	if t.finalized {
		return nil
	}
	t.finalized = true

	var out []apicompat.AnthropicStreamEvent

	// message_start 必须无条件先发——哪怕上游整段响应被静默截断、一个字节的
	// 正文都没有（gateBuf 也是空的，从未触发过 handle 里任何一条 ensureStarted
	// 分支），客户端也必须收到结构完整的 message_start -> message_delta ->
	// message_stop，不能只有后两者。ensureStarted 内部有 started 标记，
	// 重复调用是安全的空操作。
	out = append(out, t.ensureStarted()...)

	// 门控缓冲里可能还压着未判定的内容（响应太短、始终像 <thinking> 的前缀）。
	if t.gateBuf != "" {
		t.phase = phaseText
		buffered := t.gateBuf
		t.gateBuf = ""
		out = append(out, t.emitText(buffered)...)
	}

	// closeBuf 里可能还压着一段一直没等到后续字节确认的疑似闭标签前缀
	// （流在思考块内部被截断，比如撞到 max_tokens）——这段内容确实是
	// 思考块的一部分（还没等到 </thinking> 就没了），必须当思考内容
	// 吐出去，不能悄悄丢弃。
	if t.closeBuf != "" {
		buffered := t.closeBuf
		t.closeBuf = ""
		out = append(out, t.emitThinking(buffered)...)
	}

	out = append(out, t.closeBlock()...)

	usage := t.Usage()
	out = append(out, apicompat.AnthropicStreamEvent{
		Type: "message_delta",
		Delta: &apicompat.AnthropicDelta{
			StopReason: mapStopReason(t.stopReason, t.toolCount),
		},
		Usage: &usage,
	})
	out = append(out, apicompat.AnthropicStreamEvent{Type: "message_stop"})

	return out
}

// mapStopReason 把 Kiro 的 stopReason 映射为 Anthropic 的 stop_reason。
// 有工具调用时恒为 tool_use —— 否则客户端不会继续工具轮次。
func mapStopReason(reason string, toolCount int) string {
	if toolCount > 0 {
		return "tool_use"
	}

	switch strings.ToLower(strings.TrimSpace(reason)) {
	case "max_tokens", "max_output_tokens", "length":
		return "max_tokens"
	case "model_context_window_exceeded", "context_window_exceeded":
		return "model_context_window_exceeded"
	case "refusal", "content_filter", "content_filtered", "guardrail_intervened":
		return "refusal"
	case "stop_sequence":
		return "stop_sequence"
	case "pause_turn":
		return "pause_turn"
	default:
		return "end_turn"
	}
}

func intPtr(v int) *int { return &v }
