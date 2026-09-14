// response_decoder.go 把 Devin protobuf 响应帧解释为有序的中间响应事件。
// 逐段移植 devin2api response_decoder.go；帧来源由生成类型换成
// pkg/devin 的手写解码结构（字段语义一一对应）。
package adapter

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/devin"
	"github.com/Wei-Shaw/sub2api/internal/pkg/devin/llm"
)

// responseDecoder 保存一次 Devin 请求内的响应累计状态和内容映射。
type responseDecoder struct {
	// model 是请求使用的 Devin 模型标识。
	model string
	// partial 是当前累计形成的助手消息。
	partial llm.AssistantMessage
	// text 是当前累计的文字块。
	text *llm.TextContent
	// textBuilder 累计文字增量，避免 O(n²) 字符串拼接。
	textBuilder strings.Builder
	// textIdx 是文字块在 partial.Content 中的位置。
	textIdx int
	// textOpen 表示文字块已经开始但尚未结束。
	textOpen bool
	// thinking 是当前累计的思考块。
	thinking *llm.ThinkingContent
	// thinkingBuilder 累计思考正文。
	thinkingBuilder strings.Builder
	// thinkingSigBuilder 累计思考签名。
	thinkingSigBuilder strings.Builder
	// thinkIdx 是思考块在 partial.Content 中的位置。
	thinkIdx int
	// thinkingOpen 表示思考块已经开始但尚未结束。
	thinkingOpen bool
	// tools 保存正在累计参数的工具调用。
	tools []*toolState
	// started 表示已经生成 start 事件。
	started bool
	// finished 表示已经生成 done 或 error 事件。
	finished bool
	// hasStopReason 表示 Devin 已经显式返回停止原因。
	hasStopReason bool
	// stopReason 保存 Devin 声明的最终停止原因，等待上游 EOF 后用于完成响应。
	stopReason llm.StopReason
	// stopPatterns 是客户端请求的停止序列。上游 stopPatterns 实测不生效
	//（模型越过 pattern 继续输出），由解码层对累计文本做本地截断。
	stopPatterns []string
	// maxPatternLen 是最长停止序列的字节长度，决定流式下发时保留的尾部窗口。
	maxPatternLen int
	// textEmitted 是 textBuilder 中已作为 delta 下发的字节数。
	textEmitted int
	// stoppedByPattern 表示生成已被停止序列截断；后续帧只更新用量元数据。
	stoppedByPattern bool
	// stopSequence 是命中的停止序列文本。
	stopSequence string
	// providerLogged 表示已把 provider 侧追踪信息写入 diagnostics。
	providerLogged bool
	// providerRefusal 表示上游声明 provider 拒绝了本次请求（usage.provider_refusal）。
	providerRefusal bool
	// customTools 是本次请求按 freeform/custom 语义声明的工具名集合；
	// 这些工具在 wire 上是单参数 function 包装形态，响应要解包回原文。
	customTools map[string]bool
}

// toolState 保存一次 Devin 工具调用的累计状态。
type toolState struct {
	// call 是当前累计形成的工具调用。
	call llm.ToolCall
	// contentIdx 是调用在 partial.Content 中的位置。
	contentIdx int
	// arguments 累计 Devin 返回的工具参数 JSON 片段。
	arguments strings.Builder
	// emitted 表示原始工具调用已经加入 partial 并产生 start 事件。
	emitted bool
	// wrapped 表示该调用是 custom 声明工具的 function 包装形态：参数片段
	// 是 {"input":"<原文>"} 的 JSON 包装，不是给客户端的参数体本身。
	wrapped bool
}

func newResponseDecoder(model string, stopPatterns []string, customTools map[string]bool) *responseDecoder {
	patterns := make([]string, 0, len(stopPatterns))
	maxLen := 0
	for _, pattern := range stopPatterns {
		if pattern == "" {
			continue
		}
		patterns = append(patterns, pattern)
		if len(pattern) > maxLen {
			maxLen = len(pattern)
		}
	}
	return &responseDecoder{model: model, stopPatterns: patterns, maxPatternLen: maxLen, customTools: customTools}
}

// customToolNames 返回请求里按 freeform/custom 语义声明的工具名集合，
// 供解码器识别包装 function 调用并解包回原文。
func customToolNames(tools []llm.ToolDefinition) map[string]bool {
	var names map[string]bool
	for _, tool := range tools {
		if tool.Custom {
			if names == nil {
				names = make(map[string]bool)
			}
			names[tool.Name] = true
		}
	}
	return names
}

func (decoder *responseDecoder) start() []llm.ResponseEvent {
	if decoder.started || decoder.finished {
		return nil
	}
	decoder.started = true
	decoder.partial = llm.AssistantMessage{
		API: "connect", Provider: "devin", Model: decoder.model,
		StopReason: llm.StopReasonPending, TimestampMS: time.Now().UnixMilli(),
	}
	return []llm.ResponseEvent{{Type: llm.ResponseEventStart, Partial: &decoder.partial}}
}

func (decoder *responseDecoder) decode(frame *devin.ChatResponseFrame) []llm.ResponseEvent {
	if frame == nil || decoder.finished {
		return nil
	}
	decoder.updateMetadata(frame)
	if decoder.stoppedByPattern {
		// 停止序列已截断对外输出；继续消费上游帧仅为 usage 统计完整。
		return nil
	}
	// 子解码器共享同一个 events 切片追加事件；多数帧只产 1-2 个事件，
	// 共享切片把每帧的 2-3 次小分配压到接近一次。
	events := make([]llm.ResponseEvent, 0, 4)
	// 上游把签名作为全部正文之后的尾随帧发送；思考块已关闭时
	// 不能新开思考块，要把签名合并回上一个思考块。
	if frame.DeltaSignature != "" && frame.DeltaThinking == "" && !decoder.thinkingOpen {
		events = decoder.decodeLateSignature(events, frame)
	} else if frame.DeltaThinking != "" || frame.DeltaSignature != "" || frame.ThinkingRedacted {
		events = decoder.endText(events)
		events = decoder.decodeThinking(events, frame)
	}
	if frame.DeltaText != "" {
		events = decoder.endThinking(events)
		events = decoder.decodeText(events, frame.DeltaText)
	}
	for _, delta := range frame.ToolDeltas {
		events = decoder.endThinking(events)
		events = decoder.endText(events)
		events = decoder.decodeTool(events, delta)
	}
	if frame.StopReason != devin.StopReasonUnspecified {
		decoder.hasStopReason = true
		decoder.stopReason = llm.StopReason(devin.MapStopReason(frame.StopReason))
	}
	return events
}

func (decoder *responseDecoder) finish(upstreamErr error) []llm.ResponseEvent {
	if decoder.finished {
		return nil
	}
	if upstreamErr != nil {
		// 流中途/结束时的 Connect 错误同样透传原文。
		return decoder.fail(errors.New(connectErrorText(upstreamErr)))
	}
	if !decoder.hasStopReason && len(decoder.partial.Content) == 0 && len(decoder.tools) == 0 {
		return decoder.fail(errors.New("devin stream ended without generated content"))
	}
	reason := decoder.stopReason
	if decoder.stoppedByPattern {
		reason = llm.StopReasonStopSequence
		decoder.partial.StopSequence = decoder.stopSequence
	} else if !decoder.hasStopReason {
		// Devin 正常收尾必带 stopReason 帧（实测帧序 delta → stopReason →
		// usage → responseDimensionGroups）。干净 EOF 却没有停止原因说明
		// 流在应用层被截断；此处合成 Stop 会把截断伪装成 end_turn，
		// 下游 agent 会把半完成的任务当作完成。
		return decoder.fail(errors.New("devin stream ended without stop reason"))
	}
	if reason == llm.StopReasonError {
		if decoder.providerRefusal {
			return decoder.fail(errors.New("upstream provider refused the request (provider_refusal)"))
		}
		return decoder.fail(errors.New("devin stopped with an error"))
	}
	return decoder.complete(reason)
}

func (decoder *responseDecoder) updateMetadata(frame *devin.ChatResponseFrame) {
	if frame.MessageID != "" {
		decoder.partial.ResponseID = frame.MessageID
	}
	if frame.OutputID != "" {
		decoder.partial.OutputID = frame.OutputID
	}
	if frame.RequestID != "" && decoder.partial.UpstreamRequestID == "" {
		decoder.partial.UpstreamRequestID = frame.RequestID
	}
	if frame.ActualModelUID != "" {
		decoder.partial.ResponseModel = frame.ActualModelUID
	}
	if frame.TimestampUnixMS != 0 {
		decoder.partial.TimestampMS = frame.TimestampUnixMS
	}
	if usage := frame.Usage; usage != nil {
		if decoder.partial.ResponseModel == "" && usage.ModelUID != "" {
			decoder.partial.ResponseModel = usage.ModelUID
		}
		decoder.partial.Usage.Input = usage.Input
		decoder.partial.Usage.Output = usage.Output
		decoder.partial.Usage.CacheRead = usage.CacheRead
		decoder.partial.Usage.CacheWrite = usage.CacheWrite
		decoder.partial.Usage.TotalTokens = usage.Input + usage.Output + usage.CacheRead + usage.CacheWrite
		if usage.ProviderRefusal {
			decoder.providerRefusal = true
		}
		// provider 侧追踪信息（api_provider + 供应商 HTTP 请求号）只记一次，
		// 排障时可直接把 x-request-id 报给上游/供应商。
		if !decoder.providerLogged {
			apiProvider := strings.TrimPrefix(modelProviderEnumName(int(usage.APIProvider)), "API_PROVIDER_")
			if usage.ProviderRequestID != "" || (apiProvider != "" && apiProvider != "UNSPECIFIED") {
				details, _ := json.Marshal(map[string]string{
					"api_provider":        apiProvider,
					"provider_request_id": usage.ProviderRequestID,
					"provider_message_id": usage.ProviderMessageID,
					"billing_model_uid":   usage.BillingModelUID,
				})
				decoder.partial.Diagnostics = append(decoder.partial.Diagnostics, llm.AssistantMessageDiagnostic{
					Type:        "upstream_provider",
					TimestampMS: time.Now().UnixMilli(),
					Details:     details,
				})
				decoder.providerLogged = true
			}
		}
	}
}

func (decoder *responseDecoder) decodeThinking(events []llm.ResponseEvent, frame *devin.ChatResponseFrame) []llm.ResponseEvent {
	if !decoder.thinkingOpen {
		decoder.thinking = &llm.ThinkingContent{Redacted: frame.ThinkingRedacted}
		decoder.thinkingBuilder.Reset()
		decoder.thinkingSigBuilder.Reset()
		decoder.partial.Content = append(decoder.partial.Content, *decoder.thinking)
		decoder.thinkIdx = len(decoder.partial.Content) - 1
		decoder.thinkingOpen = true
		events = append(events, llm.ResponseEvent{Type: llm.ResponseEventThinkingStart, ContentIndex: decoder.thinkIdx, Partial: &decoder.partial})
	}
	if delta := frame.DeltaThinking; delta != "" {
		decoder.thinkingBuilder.WriteString(delta)
	}
	if sig := frame.DeltaSignature; sig != "" {
		decoder.thinkingSigBuilder.WriteString(sig)
	}
	if sigType := frame.DeltaSignatureType; sigType != "" {
		// signature_type 决定签名载荷的格式（sealed/anthropic/openai），
		// 重放时必须原样回传——实测错配触发上游 invalid_argument。
		decoder.thinking.SignatureType = sigType
	}
	decoder.thinking.Redacted = decoder.thinking.Redacted || frame.ThinkingRedacted
	// 思考正文在 endThinking 再 materialize；签名通常较短，每帧同步签名避免 startReasoning 拿不到。
	decoder.thinking.ThinkingSignature = decoder.thinkingSigBuilder.String()
	decoder.partial.Content[decoder.thinkIdx] = *decoder.thinking
	if delta := frame.DeltaThinking; delta != "" {
		events = append(events, llm.ResponseEvent{Type: llm.ResponseEventThinkingDelta, ContentIndex: decoder.thinkIdx, Delta: delta, Partial: &decoder.partial})
	}
	return events
}

func (decoder *responseDecoder) decodeText(events []llm.ResponseEvent, delta string) []llm.ResponseEvent {
	if !decoder.textOpen {
		decoder.text = &llm.TextContent{}
		decoder.textBuilder.Reset()
		decoder.textEmitted = 0
		decoder.partial.Content = append(decoder.partial.Content, *decoder.text)
		decoder.textIdx = len(decoder.partial.Content) - 1
		decoder.textOpen = true
		events = append(events, llm.ResponseEvent{Type: llm.ResponseEventTextStart, ContentIndex: decoder.textIdx, Partial: &decoder.partial})
	}
	// 用 Builder 累加，避免每帧产生越来越大的新字符串。
	decoder.textBuilder.WriteString(delta)
	if len(decoder.stopPatterns) == 0 {
		decoder.textEmitted += len(delta)
		events = append(events, llm.ResponseEvent{Type: llm.ResponseEventTextDelta, ContentIndex: decoder.textIdx, Delta: delta, Partial: &decoder.partial})
		return events
	}
	return decoder.scanTextForStops(events)
}

// scanTextForStops 在累计文本中查找停止序列并决定本次可下发的 delta。
// 已下发前缀保证不含任何匹配起点（见 emitTextDelta 的安全窗口推导），
// 因此只需从 textEmitted 起搜索。命中时截断文本、关闭文字块并标记
// stoppedByPattern；未命中时保留尾部 maxPatternLen-1 字节不下发——
// 它们可能是某个序列跨帧的不完整前缀。
func (decoder *responseDecoder) scanTextForStops(events []llm.ResponseEvent) []llm.ResponseEvent {
	text := decoder.textBuilder.String()
	earliest := -1
	for _, pattern := range decoder.stopPatterns {
		if idx := strings.Index(text[decoder.textEmitted:], pattern); idx >= 0 {
			pos := decoder.textEmitted + idx
			if earliest < 0 || pos < earliest {
				earliest = pos
				decoder.stopSequence = pattern
			}
		}
	}
	if earliest >= 0 {
		if earliest > decoder.textEmitted {
			events = append(events, decoder.emitTextDelta(text[decoder.textEmitted:earliest]))
		}
		decoder.stoppedByPattern = true
		decoder.textBuilder.Reset()
		decoder.textBuilder.WriteString(text[:earliest])
		return decoder.endText(events)
	}
	if safe := len(text) - decoder.maxPatternLen + 1; safe > decoder.textEmitted {
		events = append(events, decoder.emitTextDelta(text[decoder.textEmitted:safe]))
	}
	return events
}

// emitTextDelta 下发一段文本增量并推进 textEmitted 计数。
func (decoder *responseDecoder) emitTextDelta(delta string) llm.ResponseEvent {
	decoder.textEmitted += len(delta)
	return llm.ResponseEvent{Type: llm.ResponseEventTextDelta, ContentIndex: decoder.textIdx, Delta: delta, Partial: &decoder.partial}
}

func (decoder *responseDecoder) decodeTool(events []llm.ResponseEvent, delta devin.ChatToolDelta) []llm.ResponseEvent {
	state := decoder.findTool(delta.ID)
	if state == nil {
		id := delta.ID
		if id == "" {
			// 上游偶发首帧不带 id：合成稳定占位，保证 ToolCallStart
			// 事件过得了 Validate，后续按位置续接参数增量。
			id = fmt.Sprintf("call_%d", len(decoder.tools))
		}
		state = &toolState{
			call:       llm.ToolCall{ID: id, Name: delta.Name, Arguments: json.RawMessage(`{}`)},
			contentIdx: -1,
		}
		decoder.tools = append(decoder.tools, state)
	}
	if delta.ID != "" {
		state.call.ID = delta.ID
	}
	if delta.Name != "" {
		state.call.Name = delta.Name
		if decoder.customTools[delta.Name] {
			// custom 声明工具的 wire 形态是包装 function：按 custom_tool_call
			// 语义标记，参数片段在 complete 统一解包。
			state.call.Custom = true
			state.wrapped = true
		}
	}
	if delta.Custom {
		state.call.Custom = true
	}
	fragment := delta.Args
	hasFragment := delta.HasArgs
	if delta.Custom && fragment != "" {
		// custom/freeform 工具的参数体本来就不是 JSON（如补丁文本），
		// 原样透传给客户端而不是吞成 {}。
		state.call.Custom = true
	}
	if hasFragment {
		state.arguments.WriteString(fragment)
	}
	return decoder.decodeNativeTool(events, state, fragment, hasFragment)
}

// decodeNativeTool 保留 Devin 原生工具名称和参数增量语义。
func (decoder *responseDecoder) decodeNativeTool(events []llm.ResponseEvent, state *toolState, fragment string, hasFragment bool) []llm.ResponseEvent {
	if !state.emitted {
		state.contentIdx = len(decoder.partial.Content)
		state.emitted = true
		decoder.partial.Content = append(decoder.partial.Content, state.call)
		events = append(events, llm.ResponseEvent{
			Type: llm.ResponseEventToolCallStart, ContentIndex: state.contentIdx,
			ToolCallID: state.call.ID, ToolName: state.call.Name, Partial: &decoder.partial,
		})
	}
	// 工具参数在 complete 中一次性解析并写入，避免每帧 O(n) 拷贝/校验。
	// wrapped 调用的片段是 JSON 包装碎片而非参数原文，不下发增量——
	// 解包后的完整 input 在 complete 以单条 delta 补发。
	if hasFragment && !state.wrapped {
		events = append(events, llm.ResponseEvent{
			Type: llm.ResponseEventToolCallDelta, ContentIndex: state.contentIdx,
			ToolCallID: state.call.ID, Delta: fragment, Partial: &decoder.partial,
		})
	}
	return events
}

// decodeLateSignature 把思考块关闭后才到达的签名帧合并回上一个思考块。
// 没有思考块可挂时合成一个空块：openai 体制下签名是唯一思考产物
// （无 deltaThinking，推理内容密封在签名的 reasoning item 里），
// 丢弃它会让 /v1/responses 下游永远拿不到 reasoning item。
func (decoder *responseDecoder) decodeLateSignature(events []llm.ResponseEvent, frame *devin.ChatResponseFrame) []llm.ResponseEvent {
	signature := frame.DeltaSignature
	for index := len(decoder.partial.Content) - 1; index >= 0; index-- {
		thinking, ok := decoder.partial.Content[index].(llm.ThinkingContent)
		if !ok {
			continue
		}
		thinking.ThinkingSignature += signature
		if sigType := frame.DeltaSignatureType; sigType != "" {
			thinking.SignatureType = sigType
		}
		thinking.Redacted = thinking.Redacted || frame.ThinkingRedacted
		decoder.partial.Content[index] = thinking
		return append(events, llm.ResponseEvent{
			Type: llm.ResponseEventThinkingSignature, ContentIndex: index,
			Delta: signature, Partial: &decoder.partial,
		})
	}
	thinking := llm.ThinkingContent{
		ThinkingSignature: signature,
		SignatureType:     frame.DeltaSignatureType,
		Redacted:          frame.ThinkingRedacted,
	}
	decoder.partial.Content = append(decoder.partial.Content, thinking)
	index := len(decoder.partial.Content) - 1
	// 事件共享同一 Partial 指针，编码器在 start/end 边界就会读到块内签名；
	// 再发 thinking_signature 会与之叠加翻倍，且合成块永远没有 thinking_end，
	// 只发签名事件会让编码器侧的 item 悬挂到流终止报错。
	return append(events,
		llm.ResponseEvent{Type: llm.ResponseEventThinkingStart, ContentIndex: index, Partial: &decoder.partial},
		llm.ResponseEvent{Type: llm.ResponseEventThinkingEnd, ContentIndex: index, Partial: &decoder.partial},
	)
}

func (decoder *responseDecoder) endThinking(events []llm.ResponseEvent) []llm.ResponseEvent {
	if !decoder.thinkingOpen || decoder.thinking == nil {
		return events
	}
	decoder.thinkingOpen = false
	// 只在思考块结束时一次性生成完整思考与签名。
	decoder.thinking.Thinking = decoder.thinkingBuilder.String()
	decoder.thinking.ThinkingSignature = decoder.thinkingSigBuilder.String()
	decoder.partial.Content[decoder.thinkIdx] = *decoder.thinking
	return append(events, llm.ResponseEvent{
		Type: llm.ResponseEventThinkingEnd, ContentIndex: decoder.thinkIdx,
		Content: decoder.thinking.Thinking, Partial: &decoder.partial,
	})
}

func (decoder *responseDecoder) endText(events []llm.ResponseEvent) []llm.ResponseEvent {
	if !decoder.textOpen || decoder.text == nil {
		return events
	}
	decoder.textOpen = false
	// 有停止序列时尾部窗口可能还有未下发内容；截断时 builder 已被重置
	// 为截断文本且 textEmitted 不超过其长度，不会多发。
	if pending := decoder.textBuilder.String(); decoder.textEmitted < len(pending) {
		events = append(events, decoder.emitTextDelta(pending[decoder.textEmitted:]))
	}
	// 只在内容块结束时一次性生成完整文字，避免 O(n²) 拷贝。
	decoder.text.Text = decoder.textBuilder.String()
	decoder.partial.Content[decoder.textIdx] = *decoder.text
	return append(events, llm.ResponseEvent{
		Type: llm.ResponseEventTextEnd, ContentIndex: decoder.textIdx,
		Content: decoder.text.Text, Partial: &decoder.partial,
	})
}

func (decoder *responseDecoder) findTool(id string) *toolState {
	for _, state := range decoder.tools {
		if id != "" && state.call.ID == id {
			return state
		}
	}
	if id == "" && len(decoder.tools) > 0 {
		return decoder.tools[len(decoder.tools)-1]
	}
	return nil
}

func (decoder *responseDecoder) complete(reason llm.StopReason) []llm.ResponseEvent {
	if decoder.finished {
		return nil
	}
	events := make([]llm.ResponseEvent, 0, len(decoder.tools)*3+3)
	decoder.partial.StopReason = reason
	events = decoder.endThinking(events)
	events = decoder.endText(events)
	for _, state := range decoder.tools {
		if !state.emitted {
			continue
		}
		// 在结束时一次性把 Builder 中的完整参数转成 JSON，避免中间反复解析/拷贝。
		state.call.Arguments = json.RawMessage(state.arguments.String())
		if state.wrapped {
			// custom 声明工具的包装参数体：解出 input 原文下发；
			// 模型偏离包装 schema 时（裸文本/多键）整体按 freeform 原文透传。
			state.call.Custom = true
			state.call.Arguments = unwrapCustomToolArguments(state.arguments.String())
			// 补发单条完整 delta，让按增量累计输入的下游状态收敛到一致。
			events = append(events, llm.ResponseEvent{
				Type: llm.ResponseEventToolCallDelta, ContentIndex: state.contentIdx,
				ToolCallID: state.call.ID, Delta: string(state.call.Arguments), Partial: &decoder.partial,
			})
		} else if state.call.Custom {
			// 原文即参数体（invalid_json_str 通道或客户端回灌的畸形 JSON），
			// 不走 JSON 校验与 XML 修复。
		} else if !llm.IsJSONObject(state.call.Arguments) {
			// swe 系模型偶尔把 XML 参数语法泄漏进 arguments_json（CLI 实测），
			// 先尝试把 <parameter name="X">v</parameter> 解回 JSON 再兜底 {}。
			if repaired, ok := repairLeakedXMLArguments(state.arguments.String()); ok {
				state.call.Arguments = repaired
			} else {
				state.call.Arguments = json.RawMessage(`{}`)
			}
		}
		decoder.partial.Content[state.contentIdx] = state.call
		events = append(events, llm.ResponseEvent{Type: llm.ResponseEventToolCallEnd, ContentIndex: state.contentIdx, ToolCall: &state.call, Partial: &decoder.partial})
	}
	events = append(events, llm.ResponseEvent{Type: llm.ResponseEventDone, Reason: reason, Message: &decoder.partial})
	decoder.finished = true
	return events
}

func (decoder *responseDecoder) fail(err error) []llm.ResponseEvent {
	if decoder.finished {
		return nil
	}
	decoder.partial.StopReason = llm.StopReasonError
	decoder.partial.ErrorMessage = err.Error()
	decoder.finished = true
	return []llm.ResponseEvent{{Type: llm.ResponseEventError, Reason: llm.StopReasonError, Error: &decoder.partial}}
}

// modelProviderEnumName 把 ModelProvider 枚举值映射回 proto 枚举名
// （API_PROVIDER_ 前缀形态，供 diagnostics 展示）。
func modelProviderEnumName(v int) string {
	switch v {
	case 1:
		return "API_PROVIDER_WINDSURF"
	case 2:
		return "API_PROVIDER_OPENAI"
	case 3:
		return "API_PROVIDER_ANTHROPIC"
	case 4:
		return "API_PROVIDER_GOOGLE"
	case 5:
		return "API_PROVIDER_AZURE_OPENAI"
	case 6:
		return "API_PROVIDER_DEEPSEEK"
	case 7:
		return "API_PROVIDER_XAI"
	case 8:
		return "API_PROVIDER_UNRECOGNIZED"
	case 9:
		return "API_PROVIDER_QWEN"
	case 10:
		return "API_PROVIDER_ZAI"
	case 11:
		return "API_PROVIDER_KIMI"
	case 12:
		return "API_PROVIDER_THINKING_MACHINES"
	default:
		return "API_PROVIDER_UNSPECIFIED"
	}
}

// unwrapCustomToolArguments 解出包装 schema {"input":"<原文>"} 中的原文；
// 模型偏离包装（裸文本或 input 非字符串）时按 freeform 语义整体透传。
func unwrapCustomToolArguments(raw string) json.RawMessage {
	var object map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &object); err != nil {
		return json.RawMessage(raw)
	}
	if input, ok := object["input"]; ok {
		var text string
		if err := json.Unmarshal(input, &text); err == nil {
			return json.RawMessage(text)
		}
	}
	return json.RawMessage(raw)
}

// leakedXMLParameterPattern 匹配泄漏进 arguments_json 的 XML 参数片段（CLI 实测格式）。
var leakedXMLParameterPattern = regexp.MustCompile(`<(?:antml:)?parameter\s+name="([A-Za-z_][\w-]*)"[^>]*>([\s\S]*?)</(?:antml:)?parameter>`)

// repairLeakedXMLArguments 把混进 arguments 的 XML 参数标签提取成 JSON 对象；
// 不含参数标签时返回 false，调用方走原有兜底路径。
func repairLeakedXMLArguments(raw string) (json.RawMessage, bool) {
	matches := leakedXMLParameterPattern.FindAllStringSubmatch(raw, -1)
	if len(matches) == 0 {
		return nil, false
	}
	object := make(map[string]string, len(matches))
	for _, match := range matches {
		object[match[1]] = strings.TrimSpace(match[2])
	}
	data, err := json.Marshal(object)
	if err != nil {
		return nil, false
	}
	return data, true
}
