// Package openai_compat 提供 OpenAI 协议族在不同上游间的能力差异判定工具。
//
// 背景：sub2api 的 OpenAI APIKey 账号通过 base_url 接入多种第三方 OpenAI 兼容上游
// （DeepSeek、Kimi、GLM、Qwen 等）。这些上游普遍只支持 /v1/chat/completions，
// 不存在 /v1/responses 端点。但网关历史代码无差别走 CC→Responses 转换并打到
// /v1/responses，导致兼容上游 404。
//
// 本包提供基于"账号探测标记"的能力判定，配合
// internal/service/openai_apikey_responses_probe.go 在创建/修改账号时一次性
// 探测并落标。
//
// 设计取舍：
//   - 不维护静态 host 白名单——避免新增厂商时必须改代码（讨论沉淀于
//     pensieve/short-term/knowledge/upstream-capability-detection-design-tradeoffs）
//   - 标记缺失时默认 true（即"走 Responses"），保持与重构前老代码完全一致的存量
//     账号行为（"现状即证据"原则；详见
//     pensieve/short-term/maxims/preserve-existing-runtime-behavior-when-replacing-logic-in-stateful-systems）
//
// 路由判定：网关分流以 ResolveOpenAITextRoute 为准（三态：Responses / Chat
// Completions / Native）。当上游同时支持两个端点时按入站类型原生透传，避免对不
// 匹配的入站类型做有损跨协议转换——探测确认支持（auto+supported）时自动启用，
// 也可用账号级 native 模式手动强制。ShouldUseResponsesAPI 仅保留给非分流场景。
package openai_compat

// AccountResponsesSupport 描述账号上游对 OpenAI Responses API 的有效支持状态。
//
// 仅用于 platform=openai + type=apikey 的账号；其他账号类型不应调用本包判定。
type AccountResponsesSupport int

const (
	// ResponsesSupportUnknown 表示账号尚未完成能力探测（extra 字段缺失）。
	// 上游路由层应按"现状即证据"原则默认走 Responses，保持与重构前一致。
	ResponsesSupportUnknown AccountResponsesSupport = iota

	// ResponsesSupportYes 探测确认上游支持 /v1/responses。
	ResponsesSupportYes

	// ResponsesSupportNo 探测确认上游不支持 /v1/responses，应走
	// /v1/chat/completions 直转路径。
	ResponsesSupportNo
)

// ResponsesSupportMode 描述账号级 Responses API 路由覆盖模式。
type ResponsesSupportMode string

const (
	// ResponsesSupportModeAuto 表示跟随自动探测结果。
	ResponsesSupportModeAuto ResponsesSupportMode = "auto"

	// ResponsesSupportModeForceResponses 强制使用 /v1/responses。
	ResponsesSupportModeForceResponses ResponsesSupportMode = "force_responses"

	// ResponsesSupportModeForceChatCompletions 强制使用 /v1/chat/completions。
	ResponsesSupportModeForceChatCompletions ResponsesSupportMode = "force_chat_completions"

	// ResponsesSupportModeNative 表示上游同时支持两个端点，按入站类型原生透传：
	// Responses 入站 → /v1/responses，Chat Completions 入站 → /v1/chat/completions，
	// 不做任何 CC<->Responses 跨协议转换（详见 ResolveOpenAITextRoute）。
	ResponsesSupportModeNative ResponsesSupportMode = "native"
)

// ExtraKeyResponsesMode 是 accounts.extra JSON 中存储手动覆盖模式的键名。
// 值类型为 string：auto=跟随探测，force_responses=强制 Responses，
// force_chat_completions=强制 Chat Completions。
const ExtraKeyResponsesMode = "openai_responses_mode"

// ExtraKeyResponsesSupported 是 accounts.extra JSON 中存储自动探测结果的键名。
// 值类型为 bool：true=支持、false=不支持、键缺失=未探测。
const ExtraKeyResponsesSupported = "openai_responses_supported"

// NormalizeResponsesSupportMode 归一化账号级 Responses API 路由覆盖模式。
// 缺失或非法值按 auto 处理，以保持存量行为。
func NormalizeResponsesSupportMode(mode string) ResponsesSupportMode {
	switch ResponsesSupportMode(mode) {
	case ResponsesSupportModeForceResponses:
		return ResponsesSupportModeForceResponses
	case ResponsesSupportModeForceChatCompletions:
		return ResponsesSupportModeForceChatCompletions
	case ResponsesSupportModeNative:
		return ResponsesSupportModeNative
	default:
		return ResponsesSupportModeAuto
	}
}

// ResolveResponsesSupport 从账号的 extra map 中读取手动覆盖模式与探测标记。
//
// 标记缺失或类型不匹配时返回 ResponsesSupportUnknown——调用方应按
// "未探测=保留旧行为=走 Responses" 处理（参见 ShouldUseResponsesAPI）。
func ResolveResponsesSupport(extra map[string]any) AccountResponsesSupport {
	if extra == nil {
		return ResponsesSupportUnknown
	}
	if mode, ok := extra[ExtraKeyResponsesMode].(string); ok {
		switch NormalizeResponsesSupportMode(mode) {
		case ResponsesSupportModeForceResponses:
			return ResponsesSupportYes
		case ResponsesSupportModeForceChatCompletions:
			return ResponsesSupportNo
		}
	}
	v, ok := extra[ExtraKeyResponsesSupported]
	if !ok {
		return ResponsesSupportUnknown
	}
	supported, ok := v.(bool)
	if !ok {
		return ResponsesSupportUnknown
	}
	if supported {
		return ResponsesSupportYes
	}
	return ResponsesSupportNo
}

// ShouldUseResponsesAPI 是一个保留的兼容判定，仅用于非分流场景（如账号连通性
// 测试、ops 端点标签的兜底）。网关分流请改用 ResolveOpenAITextRoute——它按入站
// 类型区分原生透传，能力更强。
//
// 返回 true 的两种情况：
//  1. 账号已探测确认支持 Responses
//  2. 账号未探测（标记缺失）——按"现状即证据"原则保留旧行为
//
// 仅当账号已探测且确认不支持时返回 false。
func ShouldUseResponsesAPI(extra map[string]any) bool {
	return ResolveResponsesSupport(extra) != ResponsesSupportNo
}

// OpenAITextRoute 描述 OpenAI APIKey 账号文本补全请求（/v1/responses 与
// /v1/chat/completions）的上游路由策略。
//
// 它取代了早期用单个布尔 ShouldUseResponsesAPI 在两个入站入口（Responses 与
// Chat Completions）以相反语义分流的做法——单布尔只能把账号的全部流量归一到一个
// 端点，对不匹配的入站类型强制有损跨协议转换。三态路由让"上游双支持"成为一等
// 公民：按入站类型原生透传，零转换。
type OpenAITextRoute int

const (
	// RouteResponses 所有文本请求走上游 /v1/responses：Responses 入站原生直转，
	// Chat Completions 入站上转为 Responses。
	RouteResponses OpenAITextRoute = iota

	// RouteChatCompletions 所有文本请求走上游 /v1/chat/completions：Chat
	// Completions 入站原生直转，Responses 入站下转为 Chat Completions。
	RouteChatCompletions

	// RouteNative 上游同时支持两个端点，按入站类型原生透传，零跨协议转换。
	RouteNative
)

// ResolveOpenAITextRoute 决定 OpenAI APIKey 账号文本请求的上游路由。
//
// 判定顺序（mode 优先于探测标记，与 ResolveResponsesSupport 一致）：
//   - mode=native                  → RouteNative（手动覆盖：双支持原生透传）
//   - mode=force_responses         → RouteResponses
//   - mode=force_chat_completions  → RouteChatCompletions
//   - mode=auto/缺失：跟随探测标记
//   - 探测支持（Yes）          → RouteNative（智能自动：双支持原生透传）
//   - 探测不支持（No）         → RouteChatCompletions
//   - 未探测（Unknown）        → RouteResponses（"现状即证据"保留存量行为）
//
// 仅 platform=openai + type=apikey 的账号应调用本函数；OAuth 账号只支持
// Responses，不经过本路由判定。
func ResolveOpenAITextRoute(extra map[string]any) OpenAITextRoute {
	if extra != nil {
		if mode, ok := extra[ExtraKeyResponsesMode].(string); ok {
			switch NormalizeResponsesSupportMode(mode) {
			case ResponsesSupportModeNative:
				return RouteNative
			case ResponsesSupportModeForceResponses:
				return RouteResponses
			case ResponsesSupportModeForceChatCompletions:
				return RouteChatCompletions
			}
		}
	}
	switch ResolveResponsesSupport(extra) {
	case ResponsesSupportYes:
		return RouteNative
	case ResponsesSupportNo:
		return RouteChatCompletions
	default:
		return RouteResponses
	}
}
