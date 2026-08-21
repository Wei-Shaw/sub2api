package service

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// DSML 泄漏防护：deepseek-v4 间歇性把 DSML 工具调用漏成 `delta.content` 正文
// （开标签被上游 serving 吃掉，参数体+闭合标签当正文流出，finish_reason:"stop"，
// 全程无 tool_calls delta）。客户端视为回合正常结束，表现为「跑到一半自己停了」。
//
// 本文件提供：
//
//   - openAIDSMLLeakGuard：流式响应侧的观察者+扣流状态机（仿 openai_silent_refusal.go，
//     纯状态机不碰 IO；扣流/放行由调用方按返回值执行）。与 silent-refusal 的
//     pendingLines 门并存、不合并：refusal 门管「零字节 failover 资格」，本门管
//     「文本通道扣留与原地重试」。
//   - applyDSMLHistoryScrub：请求侧历史清洗（治级联——历史里带泄漏文本时模型会模仿，
//     不清洗则重试大概率复漏）。仅动 assistant 角色的字符串 content，user/tool 一律不碰。
//
// 失败路径全部「放行降级」：误判或重试失败的最坏代价是延迟，绝不向客户端造错、
// 绝不丢数据。
const (
	// dsmlLeakMarker 是泄漏文本的检测信号：全角竖线（U+FF5C）包裹的 DSML 标记。
	// 正常模型输出不含此序列；用户「讨论」DSML 的文本走 user 角色，不在响应文本
	// 通道的判定范围内。
	dsmlLeakMarker = "｜DSML｜"

	// dsmlGuardSniffReleaseBytes：累计正文超过该字节数且无标记、无可疑前缀时，
	// 判为正常答案，冲洗放行并转直通（后续只扫不扣）。
	dsmlGuardSniffReleaseBytes = 128

	// 扣留上限：超限原样冲洗转直通（兜底，防止极长输出被无限扣留）。
	dsmlGuardHoldMaxBytes    = 1 << 20 // 1 MiB
	dsmlGuardHoldMaxDuration = 5 * time.Minute

	// content-hold 生效期间向客户端写的 SSE 注释保活行（规范允许、解析器必须忽略）。
	// reasoning 结束后正文被扣住，零字节间隔可能触发客户端/中间层空闲超时，注释行
	// 在不产生语义输出的前提下维持连接。
	dsmlGuardKeepaliveComment  = ": dsml-guard-hold"
	dsmlGuardKeepaliveInterval = 15 * time.Second

	// ops_error_logs 事件类别（appendOpsUpstreamError 的 Kind）。
	dsmlGuardOpsKind = "dsml_leak"

	// 前缀判定与标记跨 chunk 拼接所需的缓冲长度。
	dsmlGuardPrefixProbeBytes = 64
)

// dsmlGuardHoldPrefixes：正文以这些前缀开头时视为疑似泄漏，持续扣到终态
// （泄漏形态为 content 以 `<thinking>` 或残留的 `<｜` 开标签片段起头）。
var dsmlGuardHoldPrefixes = []string{"<thinking>", "<｜"}

var (
	// 完整块：<｜DSML｜xxx> … </｜DSML｜xxx>（兼容单/双全角竖线两种拼写）。
	dsmlLeakBlockRe = regexp.MustCompile(`(?s)<｜{1,2}DSML｜{1,2}[^>]*>.*?</｜{1,2}DSML｜{1,2}[^>]*>`)
	// 孤儿闭合标签（chat 直转形态里开标签常被上游吃掉，只剩闭合簇）。
	dsmlLeakCloseTagRe = regexp.MustCompile(`</｜{1,2}DSML｜{1,2}[^>]*>`)
	// 任意残留的 DSML 开/闭标签。
	dsmlLeakTagRe = regexp.MustCompile(`</?｜{1,2}DSML｜{1,2}[^>]*>`)
)

type dsmlGuardState int

const (
	// 正文尚未出现：一切照常直播。
	dsmlGuardWatching dsmlGuardState = iota
	// 正文通道扣留中：所有后续行进入 heldLines（含空行分隔符），保持顺序。
	dsmlGuardHolding
	// 已放行转直通：只扫不扣（尾部再现标记只计 ops，不可回收）。
	dsmlGuardPassthrough
	// 终态判定为泄漏：持续吞掉本 attempt 的剩余行（usage、[DONE]），等调用方重试或冲洗。
	dsmlGuardLeakLatched
)

// openAIDSMLLeakGuard 是单个上游 attempt 流的 DSML 泄漏检测/扣流状态机。
// 非并发安全（除 Holding()，供保活 goroutine 读取）；重试换流后用 ResetForRetry 复位。
type openAIDSMLLeakGuard struct {
	observeOnly bool
	now         func() time.Time

	state         dsmlGuardState
	holding       atomic.Bool
	heldLines     []string
	heldBytes     int
	holdStartedAt time.Time

	contentLen    int
	contentPrefix string // 左侧去空白后的正文前缀（前 dsmlGuardPrefixProbeBytes 字节）
	markerTail    string // 跨 chunk 标记拼接探针（保留末尾若干字节）

	markerSeen     bool
	sawToolCall    bool
	terminalSeen   bool
	lateMarkerSeen bool
	capReleased    bool
}

func newOpenAIDSMLLeakGuard(observeOnly bool) *openAIDSMLLeakGuard {
	return &openAIDSMLLeakGuard{
		observeOnly: observeOnly,
		now:         time.Now,
	}
}

// dsmlChunkView 是单条 SSE 行中与判定相关的字段快照。
// 同时支持 chat completions chunk（delta.content / delta.tool_calls / finish_reason）
// 与 Responses 事件（response.output_text.delta / function_call 事件 / 终态事件）。
type dsmlChunkView struct {
	contentText string
	hasToolCall bool
	terminal    bool // finish_reason 非空、[DONE] 或 Responses 终态事件
}

func parseDSMLChunkView(line string) dsmlChunkView {
	var v dsmlChunkView
	payload, ok := extractOpenAISSEDataLine(line)
	if !ok {
		return v
	}
	p := strings.TrimSpace(payload)
	if p == "" {
		return v
	}
	if p == "[DONE]" {
		v.terminal = true
		return v
	}
	if !gjson.Valid(p) {
		return v
	}
	root := gjson.Parse(p)
	if eventType := strings.TrimSpace(root.Get("type").String()); eventType != "" && strings.HasPrefix(eventType, "response.") {
		switch eventType {
		case "response.output_text.delta":
			v.contentText = root.Get("delta").String()
		case "response.output_item.added", "response.output_item.done":
			if strings.TrimSpace(root.Get("item.type").String()) == "function_call" {
				v.hasToolCall = true
			}
		case "response.function_call_arguments.delta", "response.function_call_arguments.done":
			v.hasToolCall = true
		case "response.completed", "response.done", "response.incomplete", "response.failed":
			v.terminal = true
		}
		return v
	}
	choices := root.Get("choices")
	if !choices.Exists() || !choices.IsArray() {
		return v
	}
	for _, choice := range choices.Array() {
		if finish := choice.Get("finish_reason"); finish.Exists() && strings.TrimSpace(finish.String()) != "" {
			v.terminal = true
		}
		delta := choice.Get("delta")
		if !delta.Exists() {
			continue
		}
		if content := delta.Get("content"); content.Type == gjson.String && content.String() != "" {
			v.contentText += content.String()
		}
		if delta.Get("tool_calls").Exists() || delta.Get("function_call").Exists() {
			v.hasToolCall = true
		}
	}
	return v
}

// HandleLine 消费一条上游 SSE 行，返回「现在就该写给客户端」的行（按序，可能包含
// 之前被扣的积压）。返回空切片表示该行被扣留。nil guard 恒为直通。
func (g *openAIDSMLLeakGuard) HandleLine(line string) []string {
	if g == nil {
		return []string{line}
	}
	v := parseDSMLChunkView(line)

	if g.observeOnly {
		g.observeVirtual(v)
		return []string{line}
	}

	switch g.state {
	case dsmlGuardPassthrough:
		if v.contentText != "" && strings.Contains(v.contentText, dsmlLeakMarker) {
			g.lateMarkerSeen = true
		}
		if v.hasToolCall {
			g.sawToolCall = true
		}
		return []string{line}

	case dsmlGuardLeakLatched:
		// 泄漏已判定：吞掉本 attempt 的剩余行（usage-only chunk、[DONE]），
		// 否则重试后客户端会收到两份 usage（求和型客户端双计）。
		g.hold(line)
		return nil

	case dsmlGuardWatching:
		if v.hasToolCall {
			g.sawToolCall = true
		}
		if v.terminal {
			g.terminalSeen = true
		}
		if v.contentText == "" {
			// reasoning / role / usage-only 等非正文行照常直播。
			return []string{line}
		}
		// 首个非空正文 chunk：进入 content-hold。
		g.trackContent(v.contentText)
		g.state = dsmlGuardHolding
		g.holding.Store(true)
		g.holdStartedAt = g.now()
		g.hold(line)
		if v.terminal {
			return g.decideAtTerminal()
		}
		return g.maybeRelease()

	case dsmlGuardHolding:
		if v.contentText != "" {
			g.trackContent(v.contentText)
		}
		g.hold(line)
		if v.hasToolCall {
			// 工具轮：前导文本无害。含标记的行剔除标记片段后冲洗放行（shape-2 清理）。
			g.sawToolCall = true
			g.state = dsmlGuardPassthrough
			g.holding.Store(false)
			return g.releaseHeld(g.markerSeen)
		}
		if v.terminal {
			g.terminalSeen = true
			return g.decideAtTerminal()
		}
		return g.maybeRelease()
	}
	return []string{line}
}

// observeVirtual 在 observe 模式下推进虚拟状态机：只做判定所需的记账，字节零改动。
func (g *openAIDSMLLeakGuard) observeVirtual(v dsmlChunkView) {
	if v.contentText != "" {
		g.trackContent(v.contentText)
	}
	if v.hasToolCall {
		g.sawToolCall = true
	}
	if v.terminal {
		g.terminalSeen = true
	}
}

func (g *openAIDSMLLeakGuard) trackContent(text string) {
	g.contentLen += len(text)
	if len(g.contentPrefix) < dsmlGuardPrefixProbeBytes {
		candidate := g.contentPrefix + text
		if g.contentPrefix == "" {
			candidate = strings.TrimLeft(candidate, " \t\r\n")
		}
		if len(candidate) > dsmlGuardPrefixProbeBytes {
			candidate = candidate[:dsmlGuardPrefixProbeBytes]
		}
		g.contentPrefix = candidate
	}
	if !g.markerSeen {
		probe := g.markerTail + text
		if strings.Contains(probe, dsmlLeakMarker) {
			g.markerSeen = true
			g.markerTail = ""
			return
		}
		if len(probe) > dsmlGuardPrefixProbeBytes {
			probe = probe[len(probe)-dsmlGuardPrefixProbeBytes:]
		}
		g.markerTail = probe
	}
}

func (g *openAIDSMLLeakGuard) hold(line string) {
	g.heldLines = append(g.heldLines, line)
	g.heldBytes += len(line)
}

// decideAtTerminal 在终态帧（finish/[DONE]，已入 heldLines、尚未写出）处做终局判定。
func (g *openAIDSMLLeakGuard) decideAtTerminal() []string {
	if g.markerSeen && !g.sawToolCall {
		g.state = dsmlGuardLeakLatched
		return nil
	}
	g.state = dsmlGuardPassthrough
	g.holding.Store(false)
	return g.releaseHeld(g.markerSeen)
}

// maybeRelease 检查扣留中的提前放行条件（sniff 窗口 / 扣留上限）。
func (g *openAIDSMLLeakGuard) maybeRelease() []string {
	if !g.markerSeen && g.contentLen >= dsmlGuardSniffReleaseBytes && !dsmlGuardPrefixSuspicious(g.contentPrefix) {
		// 正常答案：冲洗后转直通，后续只扫不扣。
		g.state = dsmlGuardPassthrough
		g.holding.Store(false)
		return g.releaseHeld(false)
	}
	if g.heldBytes > dsmlGuardHoldMaxBytes || g.now().Sub(g.holdStartedAt) > dsmlGuardHoldMaxDuration {
		// 超限兜底：原样冲洗转直通（降级 = 今天的行为，绝不报错）。
		g.capReleased = true
		g.state = dsmlGuardPassthrough
		g.holding.Store(false)
		return g.releaseHeld(false)
	}
	return nil
}

// releaseHeld 交出全部被扣行。scrub=true 时做 shape-2 清理
// （只剔标记片段，不动其余内容）。
func (g *openAIDSMLLeakGuard) releaseHeld(scrub bool) []string {
	held := g.heldLines
	g.heldLines = nil
	g.heldBytes = 0
	if !scrub {
		return held
	}
	return scrubDSMLFromHeldSSELines(held)
}

// scrubDSMLFromHeldSSELines 对被扣行做 shape-2 清理（工具轮前导文本含标记）。
// 标记可能跨 chunk 断裂，单行 Contains 检不出：把全部纯正文行的 delta.content
// 按序拼接后整体清洗，结果并入第一条正文行，其余纯正文行（及其空行分隔符）
// 删除——客户端本就按序拼接 content，chunk 粒度无语义。带 tool_calls 或终态的
// 行不参与合并（不能删），原样保序、就地单行清理。任何重写失败都退回逐行清理
// （放行降级）。
func scrubDSMLFromHeldSSELines(held []string) []string {
	mergeable := make([]bool, len(held))
	firstContent := -1
	var total strings.Builder
	for i, line := range held {
		v := parseDSMLChunkView(line)
		if v.contentText == "" || v.hasToolCall || v.terminal {
			continue
		}
		mergeable[i] = true
		if firstContent < 0 {
			firstContent = i
		}
		total.WriteString(v.contentText)
	}
	if firstContent < 0 {
		return scrubDSMLHeldLinesInPlace(held)
	}
	clean, changed := scrubDSMLLeakFragments(total.String())
	if !changed {
		return scrubDSMLHeldLinesInPlace(held)
	}
	out := make([]string, 0, len(held))
	skipBlank := false
	for i, line := range held {
		if mergeable[i] {
			if i == firstContent {
				rewritten, ok := dsmlRewriteSSELineContent(line, clean)
				if !ok {
					return scrubDSMLHeldLinesInPlace(held)
				}
				out = append(out, rewritten)
			} else {
				skipBlank = true
			}
			continue
		}
		if skipBlank && line == "" {
			skipBlank = false
			continue
		}
		skipBlank = false
		out = append(out, scrubDSMLFromSSELine(line))
	}
	return out
}

func scrubDSMLHeldLinesInPlace(held []string) []string {
	out := make([]string, 0, len(held))
	for _, line := range held {
		out = append(out, scrubDSMLFromSSELine(line))
	}
	return out
}

// dsmlRewriteSSELineContent 把一条正文 SSE 行的 choices.0.delta.content 重写为 text。
func dsmlRewriteSSELineContent(line, text string) (string, bool) {
	payload, ok := extractOpenAISSEDataLine(line)
	if !ok {
		return "", false
	}
	updated, err := sjson.Set(strings.TrimSpace(payload), "choices.0.delta.content", text)
	if err != nil {
		return "", false
	}
	return "data: " + updated, true
}

// TakeHeldLines 交出残留的被扣行（上游中途断流的 fail-open 冲洗、或重试耗尽的原样放行）。
func (g *openAIDSMLLeakGuard) TakeHeldLines() []string {
	if g == nil {
		return nil
	}
	g.state = dsmlGuardPassthrough
	g.holding.Store(false)
	return g.releaseHeld(false)
}

// ResetForRetry 丢弃本 attempt 的全部被扣行并复位状态机，返回丢弃量（供 ops 记录）。
func (g *openAIDSMLLeakGuard) ResetForRetry() (discardedLines int, discardedBytes int) {
	discardedLines = len(g.heldLines)
	discardedBytes = g.heldBytes
	g.heldLines = nil
	g.heldBytes = 0
	g.state = dsmlGuardWatching
	g.holding.Store(false)
	g.contentLen = 0
	g.contentPrefix = ""
	g.markerTail = ""
	g.markerSeen = false
	g.sawToolCall = false
	g.terminalSeen = false
	g.capReleased = false
	return discardedLines, discardedBytes
}

// LeakVerdict 报告终局判定：被扣正文含标记且全程无结构化工具调用。
func (g *openAIDSMLLeakGuard) LeakVerdict() bool {
	if g == nil {
		return false
	}
	if g.observeOnly {
		return g.terminalSeen && g.markerSeen && !g.sawToolCall
	}
	return g.state == dsmlGuardLeakLatched
}

// Holding 供保活 goroutine 并发读取：当前是否处于扣流（含泄漏已判定、等待重试）状态。
func (g *openAIDSMLLeakGuard) Holding() bool {
	return g != nil && g.holding.Load()
}

// LateMarkerSeen 报告直通后才出现标记（sniff 放行的尾部风险，不可回收，只计 ops）。
func (g *openAIDSMLLeakGuard) LateMarkerSeen() bool {
	return g != nil && g.lateMarkerSeen
}

// CapReleased 报告扣留因超限被原样放行。
func (g *openAIDSMLLeakGuard) CapReleased() bool {
	return g != nil && g.capReleased
}

func dsmlGuardPrefixSuspicious(prefix string) bool {
	// 空前缀 = 目前只见过空白字符。按不可疑处理，否则纯空白开头的正常答案会被
	// 「部分前缀匹配」规则一路扣到上限。
	if prefix == "" {
		return false
	}
	for _, p := range dsmlGuardHoldPrefixes {
		if strings.HasPrefix(prefix, p) {
			return true
		}
		// 前缀尚不足以判定（如首 chunk 只有 "<thi"）时按可疑处理，继续扣留。
		if len(prefix) < len(p) && strings.HasPrefix(p, prefix) {
			return true
		}
	}
	return false
}

// scrubDSMLFromSSELine 对单条 SSE 行做 shape-2 清理：剔除 delta.content 中的
// DSML 标记片段，其余字段原样。非 data 行或解析失败时原样返回（放行降级）。
func scrubDSMLFromSSELine(line string) string {
	if !strings.Contains(line, dsmlLeakMarker) {
		return line
	}
	payload, ok := extractOpenAISSEDataLine(line)
	if !ok {
		return line
	}
	p := strings.TrimSpace(payload)
	if p == "" || p == "[DONE]" || !gjson.Valid(p) {
		return line
	}
	choices := gjson.Get(p, "choices")
	if !choices.Exists() || !choices.IsArray() {
		return line
	}
	out := p
	for i, choice := range choices.Array() {
		content := choice.Get("delta.content")
		if content.Type != gjson.String || !strings.Contains(content.String(), dsmlLeakMarker) {
			continue
		}
		clean, _ := scrubDSMLLeakFragments(content.String())
		updated, err := sjson.Set(out, fmt.Sprintf("choices.%d.delta.content", i), clean)
		if err != nil {
			return line
		}
		out = updated
	}
	return "data: " + out
}

// scrubDSMLLeakFragments 从一段文本中剥离泄漏的 DSML 碎片：
//
//  1. 完整块 <｜DSML｜x>…</｜DSML｜x>（Responses 侧泄漏块常完整保留）；
//  2. chat 直转主形态：开标签被上游吃掉，剩 `<thinking>…参数体…</｜DSML｜x>` ——
//     从 <thinking> 起剥到最后一个闭合标签为止（参数体一并剥除）;
//  3. 残留的孤儿开/闭标签。
//
// 返回清理后的文本与是否发生改动。剥后为空是合法结果（调用方保留空串，不删消息）。
func scrubDSMLLeakFragments(s string) (string, bool) {
	if !strings.Contains(s, dsmlLeakMarker) {
		return s, false
	}
	out := dsmlLeakBlockRe.ReplaceAllString(s, "")
	if strings.Contains(out, dsmlLeakMarker) {
		if end := dsmlLastCloseTagEnd(out); end > 0 {
			// 取最后一个闭合标签之前「最近」的 <thinking> 起剥：早段合法的
			// <thinking> 思考块与正文不受牵连（评审加固）。
			if head := strings.LastIndex(out[:end], "<thinking>"); head >= 0 {
				out = out[:head] + out[end:]
			}
		}
	}
	out = dsmlLeakTagRe.ReplaceAllString(out, "")
	return out, out != s
}

func dsmlLastCloseTagEnd(s string) int {
	locs := dsmlLeakCloseTagRe.FindAllStringIndex(s, -1)
	if len(locs) == 0 {
		return -1
	}
	return locs[len(locs)-1][1]
}

// --- 服务级门控与请求侧清洗 ---

// dsmlGuardModelMatches 报告 upstreamModel 是否命中配置的前缀列表。
func (s *OpenAIGatewayService) dsmlGuardModelMatches(upstreamModel string) bool {
	if s == nil || s.cfg == nil || !s.cfg.Gateway.DSMLGuardEnabled {
		return false
	}
	for _, prefix := range s.cfg.Gateway.DSMLGuardModels {
		prefix = strings.TrimSpace(prefix)
		if prefix != "" && strings.HasPrefix(upstreamModel, prefix) {
			return true
		}
	}
	return false
}

// dsmlGuardActiveForBody 报告响应侧检测器是否激活：配置开启 + 模型命中 + 请求带 tools。
func (s *OpenAIGatewayService) dsmlGuardActiveForBody(upstreamModel string, body []byte) bool {
	if !s.dsmlGuardModelMatches(upstreamModel) {
		return false
	}
	tools := gjson.GetBytes(body, "tools")
	return tools.Exists() && tools.IsArray() && len(tools.Array()) > 0
}

// applyDSMLHistoryScrub 从请求历史的 assistant 消息里剥离已泄漏的 DSML 碎片（治级联）。
// 仅动 assistant 角色的字符串 content；user/tool 角色一律不碰（用户可能正在讨论这个 bug）。
// 剥后为空的消息保留为空串（不删消息、不动结构）。observe 模式只记录不改写。
func (s *OpenAIGatewayService) applyDSMLHistoryScrub(c *gin.Context, account *Account, upstreamModel string, body []byte) []byte {
	if !s.dsmlGuardModelMatches(upstreamModel) {
		return body
	}
	if !bytes.Contains(body, []byte(dsmlLeakMarker)) {
		return body
	}
	messages := gjson.GetBytes(body, "messages")
	if !messages.Exists() || !messages.IsArray() {
		return body
	}
	observe := s.cfg.Gateway.DSMLGuardObserve
	out := body
	scrubbedCount := 0
	for i, msg := range messages.Array() {
		if msg.Get("role").String() != "assistant" {
			continue
		}
		content := msg.Get("content")
		if content.Type != gjson.String || !strings.Contains(content.String(), dsmlLeakMarker) {
			continue
		}
		clean, changed := scrubDSMLLeakFragments(content.String())
		if !changed {
			continue
		}
		scrubbedCount++
		if observe {
			continue
		}
		updated, err := sjson.SetBytes(out, fmt.Sprintf("messages.%d.content", i), clean)
		if err != nil {
			continue
		}
		out = updated
	}
	if scrubbedCount > 0 {
		message := fmt.Sprintf("dsml history scrub: stripped leaked fragments from %d assistant message(s)", scrubbedCount)
		if observe {
			message += " (observe mode, request unchanged)"
		}
		appendDSMLGuardOpsEvent(c, account, "", message, "")
	}
	if observe {
		return body
	}
	return out
}

// appendDSMLGuardOpsEvent 落一条 Kind=dsml_leak 的 ops 事件。成功响应（<400）里的
// 事件也会被 OpsErrorLoggerMiddleware 持久化，恢复动作因此可观测、零新表。
func appendDSMLGuardOpsEvent(c *gin.Context, account *Account, upstreamRequestID, message, detail string) {
	ev := OpsUpstreamErrorEvent{
		Kind:              dsmlGuardOpsKind,
		Message:           message,
		Detail:            detail,
		UpstreamRequestID: upstreamRequestID,
	}
	if account != nil {
		ev.Platform = account.Platform
		ev.AccountID = account.ID
		ev.AccountName = account.Name
	}
	appendOpsUpstreamError(c, ev)
}
