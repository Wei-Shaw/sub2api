package service

import "strings"

// ThinkingProtocol 描述上游对 thinking block 的处理契约。
// 不同上游对历史 thinking block 的语义要求是相反的：
//   - Anthropic 官方：要求 thinking block 携带有效 signature，否则 400
//     "thinking.signature: Field required"
//   - DeepSeek `/anthropic`、Kimi `/coding` 等第三方 Anthropic 兼容上游：
//     要求历史 thinking block 原样回传，否则 400
//     "The content[].thinking in the thinking mode must be passed back to the API"
//
// 见 .pensieve/short-term/knowledge/thinking-block-filter-third-party-upstream-inversion/
type ThinkingProtocol int

const (
	// ThinkingProtocolUnknown 表示无法识别协议族（默认保守不剥离）。
	ThinkingProtocolUnknown ThinkingProtocol = iota

	// ThinkingProtocolAnthropicStrict 表示 Anthropic 官方语义：
	// 历史 thinking block 必须携带有效 signature，缺失/非法签名应剥离。
	ThinkingProtocolAnthropicStrict

	// ThinkingProtocolPassbackRequired 表示第三方兼容上游语义：
	// 所有历史 thinking block 必须原样回传，预过滤会破坏契约。
	ThinkingProtocolPassbackRequired
)

// ResolveThinkingProtocol 根据「实际发给上游的模型 ID」推断 thinking 协议族。
// 必须传入映射后的模型 ID（mappedModel），不要传客户端原始请求模型 ID（reqModel），
// 否则用户配置「claude-sonnet-4-6 → deepseek-v4-pro」时会判错。
//
// 匹配规则按厂商前缀硬编码：
//   - anthropic-strict: claude-* / opus-* / sonnet-* / haiku-*
//   - passback-required: deepseek-* / kimi-* / moonshot-* / glm-* /
//     minimax-* / minimax-m* / (qwen-|qwen2-|qwen3-|qwen4-)*-thinking
//   - unknown: 其他模型（保守不剥离）
//
// 已知局限：前缀贪婪匹配（如 `claudette-`、`claude-foreign-relay-` 也会被分类为
// strict）。当遇到伪装命名时改成显式名单匹配，但现实场景几乎不会出现。
//
// 不覆盖的厂商（截至 2026-04）：
//   - Doubao / Seed (ByteDance)：走 Volcano Engine OpenAI 协议，非 Anthropic 路径
//   - Hunyuan T1 (Tencent)：未提供 Anthropic 兼容端点
//   - 若未来出现这些厂商的 Anthropic 兼容代理，需扩展前缀列表
func ResolveThinkingProtocol(modelID string) ThinkingProtocol {
	if modelID == "" {
		return ThinkingProtocolUnknown
	}
	id := strings.ToLower(modelID)

	// Passback-required 优先匹配（特定厂商前缀），避免误判 claude-* 时也命中。
	switch {
	case strings.HasPrefix(id, "deepseek-"),
		strings.HasPrefix(id, "kimi-"),
		strings.HasPrefix(id, "moonshot-"),
		strings.HasPrefix(id, "glm-"):
		return ThinkingProtocolPassbackRequired
	}
	// MiniMax M 系列：走 https://api.minimax.io/anthropic 端点，
	// 官方明文要求 thinking block round-trip（interleaved thinking 协议）。
	// 实例：MiniMax-M2、MiniMax-M2.1、MiniMax-M2.5、MiniMax-M2.7、MiniMax-M2.7-highspeed
	// 大小写在 ToLower 后统一为 minimax-。
	if strings.HasPrefix(id, "minimax-m") {
		return ThinkingProtocolPassbackRequired
	}
	// Qwen thinking 变体：覆盖 qwen-/qwen2-/qwen3-/qwen4- 前缀 + 包含 -thinking
	// 实例：qwen3-235b-a22b-thinking-2507、qwen3-next-80b-a3b-thinking、qwen-3-72b-thinking
	if (strings.HasPrefix(id, "qwen-") ||
		strings.HasPrefix(id, "qwen2-") ||
		strings.HasPrefix(id, "qwen3-") ||
		strings.HasPrefix(id, "qwen4-")) && strings.Contains(id, "-thinking") {
		return ThinkingProtocolPassbackRequired
	}

	switch {
	case strings.HasPrefix(id, "claude-"),
		strings.HasPrefix(id, "opus-"),
		strings.HasPrefix(id, "sonnet-"),
		strings.HasPrefix(id, "haiku-"):
		return ThinkingProtocolAnthropicStrict
	}

	return ThinkingProtocolUnknown
}

// ShouldPreFilterThinkingBlocks 判断是否应在转发前剥离无效 thinking block。
// 仅 anthropic-strict 协议族需要预过滤；passback-required/unknown 都跳过，
// 因为「保留 thinking block」对 anthropic-strict 之外的上游一律更安全。
func ShouldPreFilterThinkingBlocks(modelID string) bool {
	return ResolveThinkingProtocol(modelID) == ThinkingProtocolAnthropicStrict
}

// ShouldRectifyThinkingSignatureError 判断是否应在 400 后触发 thinking 签名整流 retry。
// 仅 anthropic-strict 触发；passback-required 路径的 400 一般不是签名缺失问题，
// retry 任何 thinking 变形都不会修好，反而会破坏契约。unknown 同理保守不 retry。
func ShouldRectifyThinkingSignatureError(modelID string) bool {
	return ResolveThinkingProtocol(modelID) == ThinkingProtocolAnthropicStrict
}

// ShouldApplyRetryFilters 判断是否应执行 retry 路径的 thinking/tool block 整流。
// 与预过滤保持对称：仅 anthropic-strict 走变形；passback-required 与 unknown
// 一律返回原 body 不变形——避免在不熟悉的上游上做出可能破坏契约的猜测。
func ShouldApplyRetryFilters(modelID string) bool {
	return ResolveThinkingProtocol(modelID) == ThinkingProtocolAnthropicStrict
}
