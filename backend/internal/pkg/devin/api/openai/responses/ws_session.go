// 本文件实现 OpenAI Responses WebSocket 连接的会话状态机。
// 移植自 devin2api internal/app/websocket_session.go（仅包名/类型名适配）。
//
// Codex WS v2 的多轮语义：同一连接上反复发送 {"type":"response.create"|"response.append"}，
// 续轮用 previous_response_id + 增量 input（只含本轮新项）。我们的上游没有
// previous_response_id 语义，所以每个后续请求都被展开成完整 transcript 再交给
// 常规 /v1/responses 流水线：
//
//	merged_input = lastRequest.input + lastResponseOutput + 新 input
//
// 合并时按 item id / tool call call_id 去重（参照 CLIProxyAPI 的两轮 dedupe：
// call_id 保首个，id 默认保最后但被 output 引用的 call 项不被顶掉），并做
// function_call ↔ function_call_output 配对校验——上游对孤儿 output 会硬报
// invalid_argument，提前拦截避免把客户端的坏请求算成上游故障。
package responses

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// WSMaxTranscriptBytes 与 HTTP body 的 32MiB 上限对齐：合并后的 transcript
// 逐轮增长，必须有一道闸门防止单连接无限膨胀内存与上游负载。
const WSMaxTranscriptBytes = 32 << 20

var (
	ErrWSPreviousResponseNotFound = errors.New("previous response is not available on this websocket; resend the full conversation input without previous_response_id")
	ErrWSUnsupportedRequestType   = errors.New("unsupported websocket request type")
)

// wsItem 是 transcript item 的一次解析结果：raw 是原文（回放时重放进数组），
// fields 是已知字段的探针解码。数组元素必为合法 JSON；非 object 项的
// fields 为零值，谓词结果与「无字段」一致。
type wsItem struct {
	raw    json.RawMessage
	fields wsItemFields
}

// wsItemFields 是 transcript item 的已知字段：合并/去重/配对/回放判定
// 只读这几个键，struct 探针替代 map 解树，每条省一半分配。
type wsItemFields struct {
	Type      string          `json:"type"`
	ID        string          `json:"id"`
	CallID    string          `json:"call_id"`
	Name      string          `json:"name"`
	Role      string          `json:"role"`
	Content   json.RawMessage `json:"content"`
	Arguments json.RawMessage `json:"arguments"`
	Input     json.RawMessage `json:"input"`
}

// WSSession 保存一条 WebSocket 连接的对话回放状态。连接级生命周期：
// 断开后客户端按协议重放完整 transcript，不需要跨连接存储。
//
// lastTop/lastItems/lastOutputItems 是 commit 时解析好的上一轮请求顶层
// 字段、input 项和 output 项——续轮合并直接复用，历史项每轮零解析。
type WSSession struct {
	lastTop                 map[string]json.RawMessage
	lastItems               []wsItem
	lastOutputItems         []wsItem
	lastResponseID          string
	pendingToolCallIDs      []string
	replacementReplayNeeded bool
	// stagedTop/stagedItems 是本轮 normalize 的产出，commit 时才转正；
	// 规范化失败或被丢弃的轮次不推进会话状态。
	stagedTop   map[string]json.RawMessage
	stagedItems []wsItem
}

func NewWSSession() *WSSession {
	return &WSSession{}
}

// WSTurnResult 是单轮结束后的提交内容；由 WS 写出层在流结束时产出。
type WSTurnResult struct {
	CompletedOutput     json.RawMessage
	CompletedResponseID string
	PendingToolCallIDs  []string
}

// Commit 把上一轮结果记入会话；只有成功终结的轮次才推进状态。
// 调用前必须先成功执行 NormalizeRequest（它产出 staged 字段）。
func (s *WSSession) Commit(result WSTurnResult) {
	s.lastTop = s.stagedTop
	s.lastItems = s.stagedItems
	s.stagedTop = nil
	s.stagedItems = nil
	s.lastOutputItems = wsParseItems(result.CompletedOutput)
	s.lastResponseID = strings.TrimSpace(result.CompletedResponseID)
	s.pendingToolCallIDs = append([]string(nil), result.PendingToolCallIDs...)
}

// RequireReplacementReplay 标记下一次无 previous_response_id 的 create 为全量
// 替换（上游中断后客户端重放完整 transcript 的情形）。
func (s *WSSession) RequireReplacementReplay() {
	s.replacementReplayNeeded = true
}

// NormalizeRequest 把一条客户端 WS 帧规范化成可交给 /v1/responses 的完整请求体。
// 守卫顺序参照同类 Responses WS 会话实现：先结构性校验，再处理
// 替换/续链/合并三种形态。
//
// payload 顶层只解析一次成字段 map：后续的类型判定、续链、规范化改写全部在
// map 上读写，出口一次 marshal。续轮的增量 input 项也只解析一次，供
// pending 校验、完整回放判定、合并去重共享。
func (s *WSSession) NormalizeRequest(payload []byte) (json.RawMessage, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(payload, &top); err != nil {
		if !json.Valid(payload) {
			return nil, errors.New("invalid websocket request JSON")
		}
		// 合法但非 object 的 JSON：top 为 nil，type 读为空串，
		// 落到 unsupported type 分支——与原先逐字段解析的行为一致。
	}
	requestType := wsMapString(top, "type")
	if requestType != "response.create" && requestType != "response.append" {
		return nil, fmt.Errorf("%w: %q", ErrWSUnsupportedRequestType, requestType)
	}
	previousID := strings.TrimSpace(wsMapString(top, "previous_response_id"))
	// previous_response_id 只认 resp_* 形态——msg_*/item_*/chatcmpl_* 是
	// 别的协议的标识，续链必然找不到，按坏请求报。
	if previousID != "" && !strings.HasPrefix(previousID, "resp_") {
		return nil, fmt.Errorf("previous_response_id %q is not a response id", previousID)
	}

	if s.lastTop == nil {
		if requestType == "response.append" {
			return nil, errors.New("response.append received before response.create")
		}
		if previousID != "" {
			return nil, ErrWSPreviousResponseNotFound
		}
		return s.normalizeInitialRequest(top)
	}

	// 续轮的 input 必须是数组（增量项列表）；缺省按空增量处理（客户端可能
	// 只带 previous_response_id 触发续轮），字符串形态只在首轮合法。
	nextInput := top["input"]
	if len(nextInput) > 0 && !wsIsJSONArray(nextInput) {
		return nil, errors.New("websocket request requires array field: input")
	}
	nextItems := wsParseItems(nextInput)
	if previousID != "" && previousID != s.lastResponseID {
		return nil, fmt.Errorf("%w: %q", ErrWSPreviousResponseNotFound, previousID)
	}
	if s.replacementReplayNeeded && requestType == "response.create" && previousID == "" {
		s.replacementReplayNeeded = false
		return s.finalizeReplacement(top)
	}
	if len(s.pendingToolCallIDs) > 0 && !wsItemsSatisfyToolCalls(nextItems, s.pendingToolCallIDs) {
		if previousID != "" || requestType == "response.append" {
			return nil, errors.New("incremental websocket request is missing output for a pending tool call")
		}
		return s.finalizeReplacement(top)
	}
	if previousID == "" && wsItemsContainCompletedTranscript(nextItems) {
		// 客户端自带的 input 已是完整回放（含历史 model 产出），与它合并只会
		// 得到重复/乱序的 transcript——直接当替换处理。
		return s.finalizeReplacement(top)
	}

	merged := dedupeWSItems(mergeWSItems(s.lastItems, s.lastOutputItems, nextItems))
	if err := inheritWSFields(top, s.lastTop); err != nil {
		return nil, err
	}
	top["input"] = marshalWSItems(merged)
	return s.finishNormalize(top, merged)
}

// inheritWSFields 是续轮请求（合并与替换共用）的字段规范化：剥掉 WS 信封
// 字段，继承上一轮的 model/instructions（增量帧常省略这两个字段），
// 强制 stream=true。
func inheritWSFields(top, last map[string]json.RawMessage) error {
	delete(top, "type")
	delete(top, "previous_response_id")
	delete(top, "generate")
	if strings.TrimSpace(wsMapString(top, "model")) == "" {
		if model := strings.TrimSpace(wsMapString(last, "model")); model != "" {
			encoded, err := json.Marshal(model)
			if err != nil {
				return err
			}
			top["model"] = encoded
		}
	}
	if _, has := top["instructions"]; !has {
		if instructions, has := last["instructions"]; has {
			top["instructions"] = instructions
		}
	}
	top["stream"] = json.RawMessage("true")
	return nil
}

// finishNormalize 是规范化请求的统一出口：配对校验 → marshal → 字节上限，
// 通过后才暂存 staged 字段供 commit 转正。
func (s *WSSession) finishNormalize(top map[string]json.RawMessage, items []wsItem) (json.RawMessage, error) {
	if err := wsValidateItemPairing(items); err != nil {
		return nil, err
	}
	normalized, err := json.Marshal(top)
	if err != nil {
		return nil, err
	}
	if len(normalized) > WSMaxTranscriptBytes {
		return nil, fmt.Errorf("websocket transcript exceeds %d byte limit; compact and replay the conversation", WSMaxTranscriptBytes)
	}
	s.stagedTop = top
	s.stagedItems = items
	return normalized, nil
}

// finalizeReplacement 走替换语义规范化：不合并历史，input 自带完整
// transcript（或上次中断后客户端的全量重放）。增量合并路径不走这里——
// 它先把客户端增量合并进历史再统一校验，提前对未合并的增量做配对校验
// 会误报孤儿 output。
func (s *WSSession) finalizeReplacement(top map[string]json.RawMessage) (json.RawMessage, error) {
	if err := inheritWSFields(top, s.lastTop); err != nil {
		return nil, err
	}
	var items []wsItem
	if input := top["input"]; wsIsJSONArray(input) {
		items = wsParseItems(input)
	}
	return s.finishNormalize(top, items)
}

// normalizeInitialRequest 处理首轮 response.create：剥离 WS 信封字段，
// 强制 stream=true，input 缺省补空数组，model 必填。
// 首轮 input 允许字符串形态——只对数组形态做解析与配对校验。
func (s *WSSession) normalizeInitialRequest(top map[string]json.RawMessage) (json.RawMessage, error) {
	if strings.TrimSpace(wsMapString(top, "model")) == "" {
		return nil, errors.New("missing model in response.create request")
	}
	delete(top, "type")
	delete(top, "previous_response_id")
	delete(top, "generate")
	var items []wsItem
	if input, hasInput := top["input"]; !hasInput {
		top["input"] = json.RawMessage("[]")
	} else if wsIsJSONArray(input) {
		items = wsParseItems(input)
	}
	top["stream"] = json.RawMessage("true")
	return s.finishNormalize(top, items)
}

// WSGenerateDisabled 判定 Codex 的预热帧：{"type":"response.create","generate":false,...}
// 预热不打上游，本地合成 created+completed，但 input 要计入 transcript（客户端
// 下一轮会 previous_response_id 指向它）。
func WSGenerateDisabled(payload []byte) bool {
	raw, has, _ := wsJSONField(payload, "generate")
	if !has {
		return false
	}
	var enabled bool
	if json.Unmarshal(raw, &enabled) != nil {
		return false
	}
	return !enabled
}

// wsParseItems 把合法的 input/output 数组逐条解为 wsItem；raw 不是数组
// 时返回 nil（调用方按无项处理，首轮字符串 input 即这种形态）。
func wsParseItems(raw json.RawMessage) []wsItem {
	var raws []json.RawMessage
	if json.Unmarshal(raw, &raws) != nil {
		return nil
	}
	items := make([]wsItem, 0, len(raws))
	for _, itemRaw := range raws {
		itemRaw = bytes.TrimSpace(itemRaw)
		var fields wsItemFields
		_ = json.Unmarshal(itemRaw, &fields)
		items = append(items, wsItem{raw: itemRaw, fields: fields})
	}
	return items
}

// wsIsJSONArray 判定 raw 是否为 JSON 数组形态（忽略前导空白）。
func wsIsJSONArray(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) > 0 && trimmed[0] == '['
}

// mergeWSItems 拼接 lastItems + lastOutputItems + 增量项。
// 第一轮 dedupe：tool call 项按 call_id 保首个——重复 call 只留第一次出现。
func mergeWSItems(lastItems, lastOutputItems, nextItems []wsItem) []wsItem {
	var items []wsItem
	seenCallIDs := make(map[string]struct{})
	for _, part := range [][]wsItem{lastItems, lastOutputItems, nextItems} {
		for _, item := range part {
			if wsFieldsAreToolCall(item.fields) {
				if callID := strings.TrimSpace(item.fields.CallID); callID != "" {
					if _, dup := seenCallIDs[callID]; dup {
						continue
					}
					seenCallIDs[callID] = struct{}{}
				}
			}
			items = append(items, item)
		}
	}
	return items
}

// marshalWSItems 手工拼接 item raw 为 JSON 数组：raw 已是合法 JSON，直接
// 拼接免去 json.Marshal 对每条 RawMessage 的 compact 校验。同时把每项的
// raw 重指到新缓冲——旧 transcript/上一帧缓冲不再被 item 引用滞留。
func marshalWSItems(items []wsItem) json.RawMessage {
	size := 2
	for _, item := range items {
		size += len(item.raw) + 1
	}
	out := make([]byte, 0, size)
	out = append(out, '[')
	starts := make([]int, 0, len(items))
	for i, item := range items {
		if i > 0 {
			out = append(out, ',')
		}
		starts = append(starts, len(out))
		out = append(out, item.raw...)
	}
	out = append(out, ']')
	for i, start := range starts {
		items[i].raw = out[start : start+len(items[i].raw)]
	}
	return out
}

// dedupeWSItems 第二轮去重：按 item id 默认保留最后出现（客户端可能
// 在增量里修正已发项），但被某个 output 项 call_id 引用的 call 项不会被
// 未引用项顶掉（保 function_call ↔ output 的配对相邻性）。
// 每条 item 的字段树来自合并前的单次解析，三轮扫描全部读树。
func dedupeWSItems(items []wsItem) []wsItem {
	referencedCallIDs := make(map[string]struct{})
	for _, item := range items {
		if wsFieldsAreToolCallOutput(item.fields) {
			if callID := strings.TrimSpace(item.fields.CallID); callID != "" {
				referencedCallIDs[callID] = struct{}{}
			}
		}
	}
	// 反向扫描决定每个 id 保留哪个下标：默认最后一个；更早但 call_id 被引用的
	// 项优先级高于未被引用的重复项。
	keepAt := make(map[string]int)
	callIDReferenced := func(i int) bool {
		_, ok := referencedCallIDs[strings.TrimSpace(items[i].fields.CallID)]
		return ok
	}
	for i := len(items) - 1; i >= 0; i-- {
		id := strings.TrimSpace(items[i].fields.ID)
		if id == "" {
			continue
		}
		if existing, seen := keepAt[id]; seen {
			// existing 是更靠后的重复项。若 existing 未被引用而当前项被引用，
			// 改保留当前项（更早但配对着）。
			if !callIDReferenced(existing) && callIDReferenced(i) {
				keepAt[id] = i
			}
			continue
		}
		keepAt[id] = i
	}
	out := items[:0]
	for i, item := range items {
		id := strings.TrimSpace(items[i].fields.ID)
		if id != "" && keepAt[id] != i {
			continue
		}
		out = append(out, item)
	}
	return out
}

// wsValidateItemPairing 拒绝「有 output 无 call」的 transcript——上游对
// 这种形态硬报 invalid_argument，提前拦截以免把客户端坏请求算成上游故障。
// call 只需出现在数组任意位置（不要求在 output 之前）。
func wsValidateItemPairing(items []wsItem) error {
	calls := make(map[string]struct{})
	var outputs []string
	for _, item := range items {
		switch {
		case wsFieldsAreToolCall(item.fields):
			if callID := strings.TrimSpace(item.fields.CallID); callID != "" {
				calls[callID] = struct{}{}
			}
		case wsFieldsAreToolCallOutput(item.fields):
			if callID := strings.TrimSpace(item.fields.CallID); callID != "" {
				outputs = append(outputs, callID)
			}
		}
	}
	for _, callID := range outputs {
		if _, ok := calls[callID]; !ok {
			return fmt.Errorf("websocket transcript has tool call output for unknown call_id %q", callID)
		}
	}
	return nil
}

// wsItemsSatisfyToolCalls 检查增量项是否包含所有 pending call 的 output。
func wsItemsSatisfyToolCalls(items []wsItem, pending []string) bool {
	outputs := make(map[string]struct{}, len(pending))
	for _, item := range items {
		if wsFieldsAreToolCallOutput(item.fields) {
			if callID := strings.TrimSpace(item.fields.CallID); callID != "" {
				outputs[callID] = struct{}{}
			}
		}
	}
	for _, callID := range pending {
		if _, ok := outputs[callID]; !ok {
			return false
		}
	}
	return true
}

// wsItemsContainCompletedTranscript 判定增量 input 实际已含完整回放历史。
// 出现以下任一项即为完整形态：compaction 标记、model 产出项（function_call、
// assistant message）、或 Codex 本地压缩摘要前缀的 user 消息。
func wsItemsContainCompletedTranscript(items []wsItem) bool {
	for _, item := range items {
		switch strings.TrimSpace(item.fields.Type) {
		case "compaction", "compaction_summary", "function_call", "custom_tool_call":
			return true
		}
		role := strings.TrimSpace(item.fields.Role)
		if role == "assistant" {
			return true
		}
		if role == "user" && strings.HasPrefix(wsMessageText(item.fields), wsCodexCompactionSummaryPrefix+"\n") {
			return true
		}
	}
	return false
}

// wsCodexCompactionSummaryPrefix 是 Codex 本地压缩产出的摘要前缀；命中它说明
// input 是压缩后的完整回放，不该再和历史合并。
const wsCodexCompactionSummaryPrefix = "Another language model started to solve this problem and produced a summary of its thinking process. You also have access to the state of the tools that were used by that language model. Use this to build on the work that has already been done and avoid duplicating work. Here is the summary produced by the other language model, use the information in this summary to assist with your own analysis:"

// wsMessageText 提取 message 项的纯文本（content 为字符串或 input_text/text 块数组）。
// fields 是调用方已解析的 item 字段探针。
func wsMessageText(fields wsItemFields) string {
	if len(fields.Content) == 0 {
		return ""
	}
	var text string
	if json.Unmarshal(fields.Content, &text) == nil {
		return text
	}
	var parts []json.RawMessage
	if json.Unmarshal(fields.Content, &parts) != nil {
		return ""
	}
	var builder strings.Builder
	for _, part := range parts {
		partType := wsJSONString(part, "type")
		if partType == "input_text" || partType == "text" {
			builder.WriteString(wsJSONString(part, "text"))
		}
	}
	return builder.String()
}

// WSPendingToolCallIDs 从 output 数组里收集「完整」tool call 的 call_id：
// call_id/name 非空且 arguments（function_call）或 input（custom_tool_call）
// 为字符串。这些 call 等待客户端回送 output，是下一轮 input 的配对义务。
func WSPendingToolCallIDs(output json.RawMessage) []string {
	seen := make(map[string]struct{})
	var callIDs []string
	for _, item := range wsParseItems(output) {
		fields := item.fields
		if !wsFieldsAreCompleteToolCall(fields) {
			continue
		}
		callID := strings.TrimSpace(fields.CallID)
		if callID == "" {
			continue
		}
		if _, dup := seen[callID]; dup {
			continue
		}
		seen[callID] = struct{}{}
		callIDs = append(callIDs, callID)
	}
	return callIDs
}

func wsFieldsAreToolCall(fields wsItemFields) bool {
	t := strings.TrimSpace(fields.Type)
	return t == "function_call" || t == "custom_tool_call"
}

func wsFieldsAreToolCallOutput(fields wsItemFields) bool {
	t := strings.TrimSpace(fields.Type)
	return t == "function_call_output" || t == "custom_tool_call_output"
}

func wsFieldsAreCompleteToolCall(fields wsItemFields) bool {
	if !wsFieldsAreToolCall(fields) {
		return false
	}
	if strings.TrimSpace(fields.CallID) == "" || strings.TrimSpace(fields.Name) == "" {
		return false
	}
	body := fields.Arguments
	if strings.TrimSpace(fields.Type) == "custom_tool_call" {
		body = fields.Input
	}
	var s string
	return json.Unmarshal(body, &s) == nil
}

// WSCompletedOutputFromEvent 提取终结事件的 response.output 数组。
func WSCompletedOutputFromEvent(payload json.RawMessage) json.RawMessage {
	response, has, _ := wsJSONField(payload, "response")
	if !has {
		return nil
	}
	output, has, isArray := wsJSONField(response, "output")
	if !has || !isArray {
		return nil
	}
	return output
}

// --- 以下为最小 JSON 手术工具：不依赖 gjson/sjson，这几个 helper
// 用 encoding/json 覆盖本文件需要的全部读取/改写。 ---

// wsJSONField 返回指定字段的原始 JSON；isArray 单独标出便于数组形态校验。
func wsJSONField(payload json.RawMessage, key string) (value json.RawMessage, has bool, isArray bool) {
	var object map[string]json.RawMessage
	if json.Unmarshal(payload, &object) != nil {
		return nil, false, false
	}
	raw, ok := object[key]
	if !ok {
		return nil, false, false
	}
	trimmed := bytes.TrimSpace(raw)
	return raw, true, len(trimmed) > 0 && trimmed[0] == '['
}

func wsJSONString(payload json.RawMessage, key string) string {
	raw, has, _ := wsJSONField(payload, key)
	if !has {
		return ""
	}
	return wsRawString(raw)
}

// wsRawString 把单个 RawMessage 解为字符串；非字符串或空值返回 ""。
func wsRawString(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) != nil {
		return ""
	}
	return s
}

// wsMapString 读已解析顶层 map 的字符串字段；字段缺失/非字符串返回 ""。
func wsMapString(fields map[string]json.RawMessage, key string) string {
	return wsRawString(fields[key])
}
