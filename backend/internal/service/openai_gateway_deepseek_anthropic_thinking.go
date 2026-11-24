package service

// DeepSeek 官方 Anthropic 原生端点（/v1/messages）的 thinking 历史补位。
//
// DeepSeek 在 thinking 模式下要求工具续接请求把 assistant 历史里的 thinking
// 块完整回传；缺失时上游会按 tool-call ID 尝试自动恢复，恢复失败返回 400
// "The content[].thinking in the thinking mode must be passed back to the API"。
// 会话历史中由其他后端（如 Ollama 中转）生成的 tool_use 轮没有 thinking 块，
// 被 file-owner 固定到 DeepSeek 官方账号后就会触发该 400。
//
// Chat Completions 链已有同构先例 ensureDeepSeekChatReasoningPlaceholders
// （见 openai_gateway_responses_chat_fallback.go），这里为 Anthropic 原生直通链
// 做对应补位。

import (
	"strconv"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// deepSeekAnthropicThinkingPlaceholderBlock 是插到 assistant 历史 content 首位的
// thinking 占位块。thinking 填单个空格而非空串：DeepSeek 可能把空串按缺失处理
// （同 Chat 链 ensureDeepSeekChatReasoningPlaceholders 的结论：DeepSeek 拒绝
// 空串，非空即可通过，LiteLLM 同样注入单个空格）。signature 可以省略：
// DeepSeek 官方文档说明 input 侧的 signature 不做校验。
const deepSeekAnthropicThinkingPlaceholderBlock = `{"type":"thinking","thinking":" "}`

// anthropicBodyThinkingEnabled 报告 Anthropic Messages 请求体是否处于 thinking
// 模式。thinking.type 为 enabled/adaptive 视为启用、disabled 视为关闭；没有
// type 时看 output_config.effort，none 关闭、其余（含缺失）视为启用——DeepSeek
// 对 deepseek-flash 默认启用 thinking。
func anthropicBodyThinkingEnabled(body []byte) bool {
	switch strings.ToLower(strings.TrimSpace(gjson.GetBytes(body, "thinking.type").String())) {
	case "enabled", "adaptive":
		return true
	case "disabled":
		return false
	}
	effort := strings.TrimSpace(gjson.GetBytes(body, "output_config.effort").String())
	return !strings.EqualFold(effort, "none")
}

// ensureDeepSeekAnthropicThinkingBlocks 给带 tool_use 但缺 thinking 的 assistant
// 历史消息补单个空格占位的 thinking 块（DeepSeek 可能把空串按缺失处理），插在
// content 数组最前面。已有 thinking / redacted_thinking 的消息、只有文本的消息
// 都不改写。
//
// 用 gjson 定位、sjson.SetRawBytes 只重写 messages.<i>.content，保留请求体其余
// 字节。非法 JSON、messages 非数组、content 非数组（如字符串）时原样返回；
// 合法 JSON 但无需改动时返回同一份 body。
func ensureDeepSeekAnthropicThinkingBlocks(body []byte) []byte {
	if !gjson.ValidBytes(body) {
		return body
	}
	messages := gjson.GetBytes(body, "messages")
	if !messages.IsArray() {
		return body
	}
	updated := body
	changed := false
	for i, msg := range messages.Array() {
		if strings.TrimSpace(msg.Get("role").String()) != "assistant" {
			continue
		}
		content := msg.Get("content")
		if !content.IsArray() {
			continue
		}
		hasToolUse := false
		hasThinking := false
		for _, block := range content.Array() {
			switch block.Get("type").String() {
			case "tool_use":
				hasToolUse = true
			case "thinking", "redacted_thinking":
				hasThinking = true
			}
		}
		if !hasToolUse || hasThinking {
			continue
		}
		raw := content.Raw
		if len(raw) < 2 || raw[0] != '[' {
			continue
		}
		prefixed := "[" + deepSeekAnthropicThinkingPlaceholderBlock + "," + raw[1:]
		next, err := sjson.SetRawBytes(updated, "messages."+strconv.Itoa(i)+".content", []byte(prefixed))
		if err != nil {
			return body
		}
		updated = next
		changed = true
	}
	if !changed {
		return body
	}
	return updated
}

// shouldRepairDeepSeekAnthropicThinking 报告该请求是否需要做 DeepSeek Anthropic
// thinking 补位：仅 DeepSeek 官方上游账号（targetsDeepSeekAPIHost）且请求启用
// thinking 时生效，其他上游保持字节不变。
func shouldRepairDeepSeekAnthropicThinking(account *Account, body []byte) bool {
	if account == nil {
		return false
	}
	return targetsDeepSeekAPIHost(account) && anthropicBodyThinkingEnabled(body)
}
