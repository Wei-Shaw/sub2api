package service

import (
	"strings"

	"github.com/gin-gonic/gin"
)

// 智力检测（鹈鹕测试）需要比连通性测试长得多的题面与输出上限
// （官方 Claude 测试 payload 的 max_tokens 只有 1024，装不下一份完整 HTML 作品）。
//
// 官方测试路径（账号连通性测试、定时连通性测试、用量采样）必须逐字节保持原样，
// 因此这里不改动任何 payload 构造函数与它们的调用语义，而是：
//  1. 把覆盖值挂在本次 gin.Context 上；
//  2. 由 *ForIntelligenceCheck 包装函数先调官方原函数，再就地改写返回的 map。
//
// 没有挂覆盖值时，包装函数与官方原函数完全等价。
const intelligenceCheckOverrideContextKey = "sub2api_intelligence_check_override"

// intelligenceCheckOverride 承载一次智力检测跑测的题面、输出上限、思考强度与流式开关。
type intelligenceCheckOverride struct {
	Prompt    string
	MaxTokens int
	// ReasoningEffort 是思考强度（low / medium / high / xhigh / max 等，走上游自己的取值）。
	// 留空表示不干预，payload 保持官方原样——绝不能凭空塞一个上游不认的字段。
	ReasoningEffort string
	// DisableStream 为 true 时把请求体改成非流式（stream=false）。
	// 智力检测需要一次性拿到完整作品，不依赖增量推送；
	// 官方连通性测试保持流式（该字段保持 false），行为逐字不变。
	DisableStream bool
}

// withIntelligenceCheckOverride 把覆盖值挂到本次请求上，仅智力检测跑测会调用。
func withIntelligenceCheckOverride(c *gin.Context, override intelligenceCheckOverride) {
	if c == nil {
		return
	}
	clean := intelligenceCheckOverride{
		Prompt:          strings.TrimSpace(override.Prompt),
		MaxTokens:       override.MaxTokens,
		ReasoningEffort: strings.TrimSpace(override.ReasoningEffort),
		DisableStream:   override.DisableStream,
	}
	if clean.Prompt == "" && clean.MaxTokens <= 0 && clean.ReasoningEffort == "" && !clean.DisableStream {
		return
	}
	c.Set(intelligenceCheckOverrideContextKey, clean)
}

// intelligenceCheckOverrideFrom 取出覆盖值；第二个返回值为 false 表示走官方原行为。
func intelligenceCheckOverrideFrom(c *gin.Context) (intelligenceCheckOverride, bool) {
	if c == nil {
		return intelligenceCheckOverride{}, false
	}
	value, ok := c.Get(intelligenceCheckOverrideContextKey)
	if !ok {
		return intelligenceCheckOverride{}, false
	}
	override, ok := value.(intelligenceCheckOverride)
	if !ok {
		return intelligenceCheckOverride{}, false
	}
	if override.Prompt == "" && override.MaxTokens <= 0 && override.ReasoningEffort == "" && !override.DisableStream {
		return intelligenceCheckOverride{}, false
	}
	return override, true
}

// createClaudeTestPayloadForIntelligenceCheck 包装官方 Claude 测试 payload。
func createClaudeTestPayloadForIntelligenceCheck(c *gin.Context, modelID string) (map[string]any, error) {
	payload, err := createTestPayload(modelID)
	if err != nil {
		return nil, err
	}
	if override, ok := intelligenceCheckOverrideFrom(c); ok {
		applyNestedTextOverride(payload, "messages", override)
		applyMaxTokensOverride(payload, "max_tokens", override.MaxTokens)
		applyClaudeThinkingOverride(payload, override)
	}
	return payload, nil
}

// createOpenAIResponsesPayloadForIntelligenceCheck 包装官方 OpenAI Responses 测试 payload。
// 输出上限字段名按 Responses API 规范使用 max_output_tokens，仅在显式配置时才写入，
// 避免给官方请求体引入上游可能不接受的字段。
// 智力检测同时把 stream 关掉（非流式），官方测试路径仍保持流式。
func createOpenAIResponsesPayloadForIntelligenceCheck(c *gin.Context, modelID string, isOAuth bool) map[string]any {
	payload := createOpenAITestPayload(modelID, isOAuth)
	if override, ok := intelligenceCheckOverrideFrom(c); ok {
		applyNestedTextOverride(payload, "input", override)
		applyMaxTokensOverride(payload, "max_output_tokens", override.MaxTokens)
		applyOpenAIResponsesReasoningOverride(payload, override)
		applyStreamOverride(payload, override)
	}
	return payload
}

// createOpenAIChatPayloadForIntelligenceCheck 包装官方 Chat Completions 测试 payload。
// 该原函数本身就接收题面，这里只补输出上限与思考强度。
func createOpenAIChatPayloadForIntelligenceCheck(c *gin.Context, modelID string, prompt string) map[string]any {
	payload := createOpenAIChatCompletionsTestPayload(modelID, prompt)
	if override, ok := intelligenceCheckOverrideFrom(c); ok {
		applyMaxTokensOverride(payload, "max_tokens", override.MaxTokens)
		applyStringOverride(payload, "reasoning_effort", override.ReasoningEffort)
	}
	return payload
}

// applyNestedTextOverride 把 payload[listKey][0].content[0].text 换成覆盖题面。
// 只在目标结构符合预期时才改写，避免污染其他形态的请求体。
func applyNestedTextOverride(payload map[string]any, listKey string, override intelligenceCheckOverride) {
	if override.Prompt == "" || payload == nil {
		return
	}
	list, ok := payload[listKey].([]map[string]any)
	if !ok || len(list) == 0 {
		return
	}
	content, ok := list[0]["content"].([]map[string]any)
	if !ok || len(content) == 0 {
		return
	}
	if _, hasText := content[0]["text"]; !hasText {
		return
	}
	content[0]["text"] = override.Prompt
}

// applyMaxTokensOverride 写入输出上限；maxTokens <= 0 时保持官方原值。
func applyMaxTokensOverride(payload map[string]any, field string, maxTokens int) {
	if payload == nil || maxTokens <= 0 || strings.TrimSpace(field) == "" {
		return
	}
	payload[field] = maxTokens
}

// applyStringOverride 写入一个非空的字符串字段；空值保持官方原样。
func applyStringOverride(payload map[string]any, field string, value string) {
	trimmed := strings.TrimSpace(value)
	if payload == nil || trimmed == "" || strings.TrimSpace(field) == "" {
		return
	}
	payload[field] = trimmed
}

// applyStreamOverride 把请求体切成非流式。
// DisableStream 为 false 时完全不动 payload —— 官方测试路径必须保持 stream:true。
func applyStreamOverride(payload map[string]any, override intelligenceCheckOverride) {
	if payload == nil || !override.DisableStream {
		return
	}
	payload["stream"] = false
}

// applyOpenAIResponsesReasoningOverride 按 Responses API 规范写入思考强度。
// Responses 用嵌套的 reasoning.effort，与 Chat Completions 的顶层 reasoning_effort 不同名，
// 写错字段上游会直接忽略，表现为「配了但没效果」——所以才要按协议分开处理。
func applyOpenAIResponsesReasoningOverride(payload map[string]any, override intelligenceCheckOverride) {
	effort := strings.TrimSpace(override.ReasoningEffort)
	if payload == nil || effort == "" {
		return
	}
	payload["reasoning"] = map[string]any{"effort": effort}
}

// Claude 思考预算的取值边界。
const (
	// claudeThinkingMinBudgetTokens 是 Claude 允许的最小思考预算。
	claudeThinkingMinBudgetTokens = 1024
	// claudeThinkingMinOutputReserve 是必须给正文留出的余量：
	// budget_tokens 必须严格小于 max_tokens，否则请求非法；
	// 而作品是一整份 HTML，正文空间被思考吃光就只剩半张图。
	claudeThinkingMinOutputReserve = 4096
)

// applyClaudeThinkingOverride 按 Claude 协议写入扩展思考配置。
// Claude 没有 reasoning_effort 字段，思考深度由 thinking.budget_tokens 表达，
// 所以这里把通用级别映射成预算，并保证预算严格小于输出上限。
func applyClaudeThinkingOverride(payload map[string]any, override intelligenceCheckOverride) {
	if payload == nil || strings.TrimSpace(override.ReasoningEffort) == "" {
		return
	}
	budget := claudeThinkingBudgetForEffort(override.ReasoningEffort)
	if budget <= 0 {
		// 未知取值一律不干预：宁可走上游默认，也不要发一个猜出来的预算。
		return
	}
	maxTokens := override.MaxTokens
	if maxTokens <= 0 {
		if existing, ok := payload["max_tokens"].(int); ok {
			maxTokens = existing
		}
	}
	if maxTokens > 0 {
		if limit := maxTokens - claudeThinkingMinOutputReserve; budget > limit {
			budget = limit
		}
	}
	if budget < claudeThinkingMinBudgetTokens {
		// 输出上限太小，塞不进合法的思考预算，保持官方原样。
		return
	}
	payload["thinking"] = map[string]any{
		"type":          "enabled",
		"budget_tokens": budget,
	}
}

// claudeThinkingBudgetForEffort 把通用思考级别映射成 Claude 的思考预算（tokens）。
// 返回 0 表示「不认识这个取值」，调用方应放弃干预。
func claudeThinkingBudgetForEffort(effort string) int {
	switch strings.ToLower(strings.TrimSpace(effort)) {
	case "minimal", "low":
		return 4096
	case "medium":
		return 8192
	case "high":
		return 16384
	case "xhigh", "max":
		return 24576
	default:
		return 0
	}
}
