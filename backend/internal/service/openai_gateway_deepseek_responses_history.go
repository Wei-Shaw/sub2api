package service

import (
	"bytes"
	"strconv"
	"strings"

	"github.com/tidwall/gjson"
)

const (
	// responsesReasoningPlaceholderText 是占位 reasoning 的明文。必须是单个空格：
	// DeepSeek 实测拒绝空串，非空即可通过；LiteLLM 对 Chat Completions 侧
	// reasoning_content 的兜底同样注入单个空格。
	responsesReasoningPlaceholderText = " "

	// responsesReasoningPlaceholderIDPrefix 区分网关补出来的占位 item 与上游真实
	// 返回的 reasoning item（DeepSeek 真实 item 的 id 是裸 UUID）。
	responsesReasoningPlaceholderIDPrefix = "rs_ph_"
)

// isDeepSeekResponsesUpstream 报告这次 /v1/responses 实际上会打到 DeepSeek 官方 API。
//
// 覆盖两类账号：
//   - platform=deepseek 且走原生 Responses；
//   - platform=openai 但 base_url 主机是 api.deepseek.com（Codex 把 GPT 模型名
//     映射到 DeepSeek 的常见接法，#7313 的 Chat 回退已经认过这条混合路径）。
//
// 上次合入的 Chat 占位（#7313）和尚未合入的 #7283 都没有在这条混合 Responses
// 路径上补 reasoning_text，也没有拦截 compact。
func isDeepSeekResponsesUpstream(account *Account) bool {
	if account == nil {
		return false
	}
	if account.Platform == PlatformDeepseek {
		return account.UsesNativeCNResponses()
	}
	return targetsDeepSeekAPIHost(account)
}

// responsesReasoningTextFromJSON 返回 reasoning item 的非空明文思维链，无明文时返回空串。
//
// 判定必须是「非空字符串」而不是「非空白」：占位值恰好是一个空格，用 TrimSpace
// 判定会让占位被误判为空，导致每次请求都重复插入。
func responsesReasoningTextFromJSON(item gjson.Result) string {
	for _, part := range item.Get("content").Array() {
		if strings.TrimSpace(part.Get("type").String()) != "reasoning_text" {
			continue
		}
		if text := part.Get("text").String(); text != "" {
			return text
		}
	}
	return ""
}

// responsesInputNeedsReasoningReplay 报告 input 是否需要 reasoning 明文补齐：存在
// 「无明文的 reasoning item」，或存在「前面没有非空明文 reasoning 的 assistant 消息」。
// 只用于决定是否需要整体解码改写；判定口径必须与 ensureDeepSeekResponsesReasoningPlaceholders
// 的改写循环保持一致。
func responsesInputNeedsReasoningReplay(input gjson.Result) bool {
	guarded := false
	found := false
	input.ForEach(func(_, item gjson.Result) bool {
		switch responsesInputItemTypeFromJSON(item) {
		case "reasoning":
			if responsesReasoningTextFromJSON(item) != "" {
				guarded = true
				return true
			}
			// 无明文的 reasoning item 本身就要补明文：DeepSeek 要求每条 reasoning item
			// 都带 reasoning_text，仅靠给后续 assistant 消息插占位覆盖不到
			// 「reasoning → function_call」这种没有 assistant 消息的形态。
			found = true
			return false
		case "message":
			if strings.TrimSpace(item.Get("role").String()) == "assistant" {
				if !guarded {
					found = true
					return false
				}
				guarded = false
				return true
			}
			guarded = false
		}
		return true
	})
	return found
}

// responsesInputItemTypeFromJSON 归一化 input item 类型：Responses 允许 message 省略 type
// 字段（只带 role），DeepSeek 同样按 role 识别这类 item。
func responsesInputItemTypeFromJSON(item gjson.Result) string {
	itemType := strings.TrimSpace(item.Get("type").String())
	if itemType != "" {
		return itemType
	}
	if strings.TrimSpace(item.Get("role").String()) != "" {
		return "message"
	}
	return ""
}

// responsesInputItemType 是 responsesInputItemTypeFromJSON 的解码版本，规则必须保持一致。
func responsesInputItemType(item map[string]any) string {
	itemType := strings.TrimSpace(firstNonEmptyString(item["type"]))
	if itemType != "" {
		return itemType
	}
	if strings.TrimSpace(firstNonEmptyString(item["role"])) != "" {
		return "message"
	}
	return ""
}

// applyDeepSeekResponsesHistoryGuards 在出站到 DeepSeek /responses 前重排工具调用块
// 并补 reasoning_text 占位。openai 平台但主机为 api.deepseek.com 的映射账号与原生
// DeepSeek 共用此函数：前者走通用 Forward，不能只挂在 IsDeepSeek 早返上。
func applyDeepSeekResponsesHistoryGuards(account *Account, body []byte, compactPath bool) ([]byte, bool) {
	if account == nil || compactPath || !isDeepSeekResponsesUpstream(account) {
		return body, false
	}
	changed := false
	if reorderedBody, reordered := normalizeDeepSeekResponsesToolCallBlocks(body); reordered {
		body = reorderedBody
		changed = true
	}
	if replayedBody, replayed := ensureDeepSeekResponsesReasoningPlaceholders(body, openAIRequestBodyHasTools(body)); replayed {
		body = replayedBody
		changed = true
	}
	return body, changed
}

// ensureDeepSeekResponsesReasoningPlaceholders 为历史里缺少 reasoning 明文的 assistant
// 消息前置插入一条非空明文占位 reasoning item。
//
// DeepSeek 原生 Responses 端点在请求携带 tools 时要求每条 assistant 消息都回传 reasoning
// 明文，否则整轮 400 "The `reasoning_text` in the thinking mode must be passed back to the
// API."。以下形态都会触发：reasoning item 被中间层整条剔除、明文为空串、只有 summary、
// 只有 encrypted_content（后两者 DeepSeek 都不认）。
//
// 占位只保证会话不中断，模型看不到该轮真实推理；保真版（回灌缓存里的真实明文）不在本次范围。
func ensureDeepSeekResponsesReasoningPlaceholders(body []byte, knownHasTools bool) ([]byte, bool) {
	if !knownHasTools || len(bytes.TrimSpace(body)) == 0 {
		return body, false
	}
	input := parseRawJSONView(body).Get("input")
	if !input.IsArray() || !responsesInputNeedsReasoningReplay(input) {
		return body, false
	}

	var reqBody map[string]any
	if err := decodeOpenAIJSONUseNumber(body, &reqBody); err != nil {
		return body, false
	}
	items, ok := reqBody["input"].([]any)
	if !ok {
		return body, false
	}

	rewritten := make([]any, 0, len(items)+1)
	guarded := false
	changed := false
	for index, rawItem := range items {
		item, ok := rawItem.(map[string]any)
		if !ok {
			rewritten = append(rewritten, rawItem)
			continue
		}
		switch responsesInputItemType(item) {
		case "reasoning":
			if !responsesReasoningItemHasPlaintext(item) {
				// 补全 no-plain 的 reasoning item：DeepSeek 只认 content[].reasoning_text，
				// 仅有 summary / encrypted_content 或空串都会被 400 拒绝
				// （"The `reasoning_text` in the thinking mode must be passed back to the API."）。
				//
				// 跨模型切换时 xAI 等上游留下的是 summary-only 形态，summary 里就是真实推理，
				// 直接提升为明文可以保住上下文；没有 summary 时退回空格占位。summary 一并移除，
				// 因为该字段对 DeepSeek 无意义，留着只会让请求体翻倍。
				if text := responsesReasoningSummaryText(item); text != "" {
					item["content"] = []any{
						map[string]any{"type": "reasoning_text", "text": text},
					}
					delete(item, "summary")
				} else {
					item["content"] = []any{
						map[string]any{"type": "reasoning_text", "text": responsesReasoningPlaceholderText},
					}
				}
				changed = true
			}
			guarded = true
		case "message":
			if strings.TrimSpace(firstNonEmptyString(item["role"])) == "assistant" {
				if guarded {
					guarded = false
				} else {
					rewritten = append(rewritten, responsesReasoningPlaceholderItem(item, index))
					changed = true
				}
			} else {
				// user / system / developer 开启新段：上一段的 reasoning 不再复用。
				guarded = false
			}
		}
		rewritten = append(rewritten, item)
	}
	if !changed {
		return body, false
	}
	reqBody["input"] = rewritten
	normalized, err := marshalOpenAIUpstreamJSON(reqBody)
	if err != nil {
		return body, false
	}
	return normalized, true
}

// responsesReasoningSummaryText 拼接 reasoning item 里 summary_text 的正文。
// 跨模型切换后（xAI 等上游）推理正文只存在于 summary，DeepSeek 不认该字段，
// 需要把它提升成 reasoning_text 才能满足上游校验并保住上下文。
func responsesReasoningSummaryText(item map[string]any) string {
	summary, ok := item["summary"].([]any)
	if !ok {
		return ""
	}
	parts := make([]string, 0, len(summary))
	for _, rawPart := range summary {
		part, ok := rawPart.(map[string]any)
		if !ok {
			continue
		}
		if strings.TrimSpace(firstNonEmptyString(part["type"])) != "summary_text" {
			continue
		}
		if text, ok := part["text"].(string); ok && strings.TrimSpace(text) != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n")
}

// responsesReasoningItemHasPlaintext 判定解码后的 reasoning item 是否携带非空明文思维链。
func responsesReasoningItemHasPlaintext(item map[string]any) bool {
	content, ok := item["content"].([]any)
	if !ok {
		return false
	}
	for _, rawPart := range content {
		part, ok := rawPart.(map[string]any)
		if !ok {
			continue
		}
		if strings.TrimSpace(firstNonEmptyString(part["type"])) != "reasoning_text" {
			continue
		}
		if text, ok := part["text"].(string); ok && text != "" {
			return true
		}
	}
	return false
}

// responsesReasoningPlaceholderItem 构造占位 reasoning item。
//
// id 取被保护消息的 id（跨轮次稳定）；消息没有 id 时回落到它在改写后输入里的下标。
// 这里刻意不用随机 UUID：随机 id 每轮都变，会让 DeepSeek 的上下文缓存前缀失效。
func responsesReasoningPlaceholderItem(message map[string]any, index int) map[string]any {
	id := strings.TrimSpace(firstNonEmptyString(message["id"]))
	if id != "" {
		id = responsesReasoningPlaceholderIDPrefix + id
	} else {
		id = responsesReasoningPlaceholderIDPrefix + strconv.Itoa(index)
	}
	return map[string]any{
		"type":    "reasoning",
		"id":      id,
		"summary": []any{},
		"content": []any{
			map[string]any{"type": "reasoning_text", "text": responsesReasoningPlaceholderText},
		},
	}
}

// isOpenAIResponsesToolCallType / isOpenAIResponsesToolOutputType 判定 Responses input 里
// 的调用与输出 item 类型（含 custom / tool_search 三种形态）。
func isOpenAIResponsesToolCallType(itemType string) bool {
	switch itemType {
	case "function_call", "custom_tool_call", "tool_search_call":
		return true
	default:
		return false
	}
}

func isOpenAIResponsesToolOutputType(itemType string) bool {
	switch itemType {
	case "function_call_output", "custom_tool_call_output", "tool_search_output":
		return true
	default:
		return false
	}
}

// normalizeDeepSeekResponsesToolCallBlocks 把插在工具调用块内部的非调用/输出 item
// （assistant message、reasoning 等）移到该块之前，使调用块连续、其输出紧随其后。
//
// DeepSeek 原生 Responses 端点按「连续调用块 + 输出块」校验历史。跨模型切换后会出现
// `call → message → call → output → output` 这类形态（上一个模型允许在并行调用之间插入
// 说明），DeepSeek 会以 "No tool output found for tool call ..." 拒绝整轮——即使调用与
// 输出在数量上完全配对。实测把插入项移到块前即可通过。
func normalizeDeepSeekResponsesToolCallBlocks(body []byte) ([]byte, bool) {
	input := parseRawJSONView(body).Get("input")
	if !input.IsArray() || !responsesToolBlocksNeedNormalization(input) {
		return body, false
	}

	var reqBody map[string]any
	if err := decodeOpenAIJSONUseNumber(body, &reqBody); err != nil {
		return body, false
	}
	items, ok := reqBody["input"].([]any)
	if !ok {
		return body, false
	}
	rewritten := reorderResponsesToolCallBlocks(items)
	if len(rewritten) != len(items) {
		// 只做重排、不做增删；长度变化说明判定与改写不一致，保持原样更安全。
		return body, false
	}
	reqBody["input"] = rewritten
	normalized, err := marshalOpenAIUpstreamJSON(reqBody)
	if err != nil {
		return body, false
	}
	return normalized, true
}

// responsesToolBlocksNeedNormalization 快判：是否存在「调用块夹着其它 item」的形态。
// 判定规则必须与 reorderResponsesToolCallBlocks 保持一致。
func responsesToolBlocksNeedNormalization(input gjson.Result) bool {
	items := input.Array()
	for index := 0; index < len(items); {
		if !isOpenAIResponsesToolCallType(strings.TrimSpace(items[index].Get("type").String())) {
			index++
			continue
		}
		callIDs := make(map[string]struct{}, 4)
		pending := make(map[string]struct{}, 4)
		advanced := false
		for index < len(items) {
			itemType := strings.TrimSpace(items[index].Get("type").String())
			callID := strings.TrimSpace(items[index].Get("call_id").String())
			switch {
			case isOpenAIResponsesToolCallType(itemType):
				if callID != "" {
					callIDs[callID] = struct{}{}
					pending[callID] = struct{}{}
				}
				index++
				advanced = true
			case isOpenAIResponsesToolOutputType(itemType):
				if _, ok := callIDs[callID]; !ok {
					// 孤儿输出：本块到此结束。
					return false
				}
				delete(pending, callID)
				index++
				advanced = true
				if len(pending) == 0 {
					goto blockDone
				}
			case len(pending) > 0:
				// 调用块内部夹了其它 item。
				return true
			default:
				goto blockDone
			}
		}
	blockDone:
		if !advanced {
			index++
		}
	}
	return false
}

// reorderResponsesToolCallBlocks 逐个收集调用块，把块内的插入项放到块前，其余 item 顺序不变。
func reorderResponsesToolCallBlocks(items []any) []any {
	out := make([]any, 0, len(items))
	for index := 0; index < len(items); {
		if !isOpenAIResponsesToolCallItem(items[index]) {
			out = append(out, items[index])
			index++
			continue
		}
		block, inserts, next := collectResponsesToolCallBlock(items, index)
		out = append(out, inserts...)
		out = append(out, block...)
		if next <= index {
			out = append(out, items[index])
			index++
			continue
		}
		index = next
	}
	return out
}

// collectResponsesToolCallBlock 从 start 开始收集一个调用块。返回块内 item（调用与匹配的
// 输出，保持原有相对顺序）、被夹在块中的插入项，以及下一个待处理下标。
func collectResponsesToolCallBlock(items []any, start int) (block []any, inserts []any, next int) {
	callIDs := make(map[string]struct{}, 4)
	pending := make(map[string]struct{}, 4)
	index := start
	for index < len(items) {
		item := items[index]
		itemType := responsesInputItemTypeOfValue(item)
		callID := responsesToolItemCallID(item)
		switch {
		case isOpenAIResponsesToolCallType(itemType):
			if callID != "" {
				callIDs[callID] = struct{}{}
				pending[callID] = struct{}{}
			}
			block = append(block, item)
			index++
		case isOpenAIResponsesToolOutputType(itemType):
			if _, ok := callIDs[callID]; !ok {
				return block, inserts, index
			}
			delete(pending, callID)
			block = append(block, item)
			index++
			if len(pending) == 0 {
				return block, inserts, index
			}
		case len(pending) > 0:
			inserts = append(inserts, item)
			index++
		default:
			return block, inserts, index
		}
	}
	return block, inserts, index
}

func isOpenAIResponsesToolCallItem(item any) bool {
	return isOpenAIResponsesToolCallType(responsesInputItemTypeOfValue(item))
}

func responsesInputItemTypeOfValue(item any) string {
	typed, ok := item.(map[string]any)
	if !ok {
		return ""
	}
	return responsesInputItemType(typed)
}

func responsesToolItemCallID(item any) string {
	typed, ok := item.(map[string]any)
	if !ok {
		return ""
	}
	return strings.TrimSpace(firstNonEmptyString(typed["call_id"]))
}
