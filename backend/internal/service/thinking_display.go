package service

import (
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// ThinkingDisplayMode 取值：思考摘要注入的三档强度。
const (
	// ThinkingDisplayModeOff 不做任何 thinking 归一化。
	ThinkingDisplayModeOff = "off"
	// ThinkingDisplayModeDisplayOnly 只处理「已经在思考、但摘要被隐藏」的请求。
	// 不改变是否思考，因此不增加 token、不影响 messages 层缓存。
	ThinkingDisplayModeDisplayOnly = "display_only"
	// ThinkingDisplayModeForce 额外为完全未携带 thinking 的请求注入思考。
	// 这会真正开启思考：增加 token 消耗，并使 messages 层 prompt 缓存失效。
	ThinkingDisplayModeForce = "force"
)

const (
	// thinkingForceMaxTokens 是 force 模式为流式请求保底的 max_tokens。
	thinkingForceMaxTokens = 32000
	// thinkingForceMaxTokensNonStream 是非流式请求的保底值。非流式请求的 max_tokens
	// 过高会拉长单次响应时间并撞上 HTTP 超时，因此比流式保守。
	thinkingForceMaxTokensNonStream = 16000
)

// thinkingDisplayOptInPrefixes 列出 thinking.display 默认为 "omitted" 的模型族。
// 这些模型上：
//   - thinking 的唯一开启写法是 {type:"adaptive"}；{type:"enabled"} 与 budget_tokens 一律 400。
//   - display 不填则思考块文本为空字符串（思考仍然发生并计费，只是不可见）。
//
// Opus 4.6 / Sonnet 4.6 的 display 默认就是 "summarized"，无需补齐，故不在此列。
// 更老的模型（Sonnet 4.5 / Haiku 4.5 / Opus 4.5 等）不认识 adaptive，更不能碰。
var thinkingDisplayOptInPrefixes = []string{
	"claude-opus-4-8",
	"claude-opus-4-7",
	"claude-sonnet-5",
	"claude-fable-5",
	"claude-mythos-5",
}

// thinkingDisplayNeedsOptIn 报告 model 是否属于需要显式 display 才能看到思考摘要的模型族。
// 必须精确到 minor 版本：claude-opus-4-6 与 claude-opus-4-8 行为不同，
// 任何 "claude-opus-4" 级别的前缀匹配都会把两者混为一谈。
func thinkingDisplayNeedsOptIn(model string) bool {
	m := strings.ToLower(strings.TrimSpace(model))
	m = strings.TrimPrefix(m, "anthropic.") // Bedrock 风格的 provider 前缀
	for _, prefix := range thinkingDisplayOptInPrefixes {
		if strings.HasPrefix(m, prefix) {
			return true
		}
	}
	return false
}

// ensureMaxTokensForThinking 在注入思考时抬高 max_tokens。
//
// max_tokens 是「思考 + 正文」的总上限，不是正文的上限。在一个按无思考调校过
// max_tokens 的请求上强开思考，思考会挤占正文预算，正文被截断并以
// stop_reason="max_tokens" 结束 —— 上游不报错，只有终端用户看得见。
//
// 已有先例：RectifyThinkingBudget（gateway_request.go）与 Antigravity 的
// ensureMaxTokensGreaterThanBudget 都用同样的手法。
func ensureMaxTokensForThinking(body []byte, stream bool) ([]byte, bool) {
	target := int64(thinkingForceMaxTokens)
	if !stream {
		target = thinkingForceMaxTokensNonStream
	}
	if gjson.GetBytes(body, "max_tokens").Int() >= target {
		return body, false
	}
	out, err := sjson.SetBytes(body, "max_tokens", target)
	if err != nil {
		return body, false
	}
	return out, true
}

// NormalizeAnthropicThinkingDisplay 归一化新模型族的 thinking 参数，让思考摘要对客户端可见。
//
// 按输入分三种情形处理：
//
//  1. type=="adaptive" 且未设置 display —— 补 display="summarized"。模型本来就在思考、
//     本来就在计费，只是文本被默认值 "omitted" 隐藏；补齐不增加任何 token，也不动缓存。
//  2. type=="enabled"（Anthropic SDK 的老写法）—— 这些模型上必定 400。改写为 adaptive
//     并删除 budget_tokens，属于纯修复：原样转发本来就是失败的。
//  3. 缺少 thinking 字段 —— 仅 force 模式注入。这会真正开启思考，故同时抬高 max_tokens。
//
// 客户端显式设置的 display 一律尊重；显式 {type:"disabled"} 也不覆盖 —— 那是用户的明确意图。
// 任何一步出错都返回原 body（fail-safe，与本链路其余整流器一致）。
//
// model 必须是映射后的上游模型 ID —— 决定 display 行为的是真正执行请求的那个模型。
func NormalizeAnthropicThinkingDisplay(body []byte, model, mode string, stream bool) ([]byte, bool) {
	if mode != ThinkingDisplayModeDisplayOnly && mode != ThinkingDisplayModeForce {
		return body, false
	}
	if !thinkingDisplayNeedsOptIn(model) {
		return body, false
	}

	thinking := gjson.GetBytes(body, "thinking")

	switch {
	case !thinking.Exists():
		if mode != ThinkingDisplayModeForce {
			return body, false
		}
		// 逐字段 set 而非一次性写入 map：Go 的 map 迭代顺序不确定，会让相同请求
		// 每次产生不同的 JSON 字节序，破坏本链路对 body 字节序的既有假设。
		modified, err := sjson.SetBytes(body, "thinking.type", "adaptive")
		if err != nil {
			return body, false
		}
		modified, err = sjson.SetBytes(modified, "thinking.display", "summarized")
		if err != nil {
			return body, false
		}
		if bumped, ok := ensureMaxTokensForThinking(modified, stream); ok {
			modified = bumped
		}
		return modified, true

	case thinking.Get("type").String() == "enabled":
		// 这些模型既不接受 enabled 也不接受 budget_tokens，原样转发必 400。
		// 注意 RectifyThinkingBudget 的 400 后重试在此无效：它只调整 budget_tokens 的
		// 取值，而这里的模型是整个拒绝该字段，重试照样 400。
		modified, err := sjson.SetBytes(body, "thinking.type", "adaptive")
		if err != nil {
			return body, false
		}
		if next, err := sjson.DeleteBytes(modified, "thinking.budget_tokens"); err == nil {
			modified = next
		}
		if !thinking.Get("display").Exists() {
			if next, err := sjson.SetBytes(modified, "thinking.display", "summarized"); err == nil {
				modified = next
			}
		}
		// 老写法的 max_tokens 本就为思考留过余量（budget_tokens < max_tokens 是硬约束），
		// 不需要抬高。
		return modified, true

	case thinking.Get("type").String() == "adaptive":
		if thinking.Get("display").Exists() {
			return body, false // 客户端显式设过，尊重之
		}
		modified, err := sjson.SetBytes(body, "thinking.display", "summarized")
		if err != nil {
			return body, false
		}
		return modified, true

	default:
		// disabled 或未知取值：不干预。
		return body, false
	}
}
