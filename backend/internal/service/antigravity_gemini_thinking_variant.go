package service

import (
	"encoding/json"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
)

// Gemini 原生请求（/v1beta/models/{model}:generateContent 等）经 Antigravity 账号转发时，
// 客户端（如 Antigravity CLI / go-genai）习惯发"裸"模型名（gemini-3.8-flash）并用
// generationConfig.thinkingConfig 表达思考深度；而 Antigravity 上游的模型目录只有
// gemini-3.8-flash-low / -medium / -high / -tiered 这类带后缀的变体，裸名直接转发会被上游
// 以 404 "Requested entity was not found." 拒绝。
//
// 这里在账号 model_mapping 没有为裸名配置显式条目、但配置了它的后缀变体时，
// 按 thinkingConfig 自动挑一个变体：
//   - thinkingLevel: "low" / "medium" / "high" → 同名后缀
//   - thinkingBudget: -1（动态）→ high；0 → low；1..1024 → low；1025..8192 → medium；>8192 → high
//   - 未携带 thinkingConfig → high（与 Gemini 3 系列默认开启动态思考一致）
// 选中的后缀在映射表里不存在时按 high → medium → low → tiered 的顺序降级到存在的变体。
// 显式映射（裸名本身在 model_mapping 里）始终优先，行为与现在完全一致。

var geminiThinkingVariantSuffixes = []string{"-low", "-medium", "-high", "-tiered"}

const (
	geminiThinkingBudgetLowMax    = 1024
	geminiThinkingBudgetMediumMax = 8192
)

type geminiThinkingConfigProbe struct {
	GenerationConfig struct {
		ThinkingConfig *struct {
			ThinkingBudget *json.Number `json:"thinkingBudget"`
			ThinkingLevel  string       `json:"thinkingLevel"`
		} `json:"thinkingConfig"`
	} `json:"generationConfig"`
}

// hasGeminiThinkingVariantSuffix 判断模型名是否已经带了思考深度后缀。
func hasGeminiThinkingVariantSuffix(model string) bool {
	for _, suffix := range geminiThinkingVariantSuffixes {
		if strings.HasSuffix(model, suffix) {
			return true
		}
	}
	return false
}

// geminiThinkingLevelFromBody 从 Gemini 原生请求体推导期望的思考档位（low/medium/high）。
// 解析失败或未携带 thinkingConfig 时返回 "high"。
func geminiThinkingLevelFromBody(body []byte) string {
	if len(body) == 0 {
		return "high"
	}
	var probe geminiThinkingConfigProbe
	if err := json.Unmarshal(body, &probe); err != nil || probe.GenerationConfig.ThinkingConfig == nil {
		return "high"
	}
	tc := probe.GenerationConfig.ThinkingConfig
	switch strings.ToLower(strings.TrimSpace(tc.ThinkingLevel)) {
	case "low":
		return "low"
	case "medium":
		return "medium"
	case "high":
		return "high"
	}
	if tc.ThinkingBudget == nil {
		return "high"
	}
	budget, err := tc.ThinkingBudget.Float64()
	if err != nil {
		return "high"
	}
	switch {
	case budget < 0:
		return "high" // -1 = 动态思考
	case budget <= geminiThinkingBudgetLowMax:
		return "low" // 含 0（关闭思考）：上游没有"无思考"变体，取最浅档，budget 本身仍原样透传
	case budget <= geminiThinkingBudgetMediumMax:
		return "medium"
	default:
		return "high"
	}
}

// geminiThinkingLevelFromClaudeThinking 用 Claude Messages 协议的 thinking 配置推导档位，
// 阈值与 geminiThinkingLevelFromBody 保持一致，使同一请求无论走 Gemini 原生还是
// Chat Completions / Messages 兼容层都落到同一个上游变体。
func geminiThinkingLevelFromClaudeThinking(thinking *antigravity.ThinkingConfig) string {
	if thinking == nil {
		return "high"
	}
	if strings.EqualFold(strings.TrimSpace(thinking.Type), "disabled") {
		return "low"
	}
	budget := thinking.BudgetTokens
	switch {
	case budget <= 0:
		return "high" // 动态思考 / 未指定预算
	case budget <= geminiThinkingBudgetLowMax:
		return "low"
	case budget <= geminiThinkingBudgetMediumMax:
		return "medium"
	default:
		return "high"
	}
}

// resolveGeminiThinkingVariant 为裸 Gemini 模型名挑选账号映射表里存在的思考深度变体。
// 返回 (映射后的上游模型名, 是否命中)。未命中时调用方应回退到常规 getMappedModel 流程。
func resolveGeminiThinkingVariant(account *Account, requestedModel string, body []byte) (string, bool) {
	return resolveGeminiThinkingVariantForLevel(account, requestedModel, geminiThinkingLevelFromBody(body))
}

// resolveGeminiThinkingVariantForLevel 是 resolveGeminiThinkingVariant 的协议无关内核：
// 调用方负责按自身协议推导 preferred 档位（Gemini 原生读 generationConfig.thinkingConfig，
// Claude/OpenAI 兼容层读 thinking.budget_tokens），此处只做映射查找与降级。
func resolveGeminiThinkingVariantForLevel(account *Account, requestedModel string, preferred string) (string, bool) {
	if account == nil {
		return "", false
	}
	model := strings.TrimSpace(strings.TrimPrefix(requestedModel, "models/"))
	if !strings.HasPrefix(model, "gemini-") || hasGeminiThinkingVariantSuffix(model) {
		return "", false
	}
	mapping := account.GetModelMapping()
	if len(mapping) == 0 {
		return "", false
	}
	// 裸名本身若已映射到其他具体模型（如 gemini-3.8-flash → gemini-3.8-flash-tiered、
	// gemini-3.1-pro → gemini-pro-agent），尊重显式别名配置。
	// 若裸名映射目标是自己（如前端从 DefaultAntigravityModelMapping 拉取并持久化的
	// gemini-3.8-flash → gemini-3.8-flash，或 ensureAntigravityDefaultPassthroughs 注入的条目），
	// 当映射表中存在该模型的精确思考变体（-high/-medium/-low/-tiered）时继续解析变体，
	// 避免把上游不存在的裸名直接发出导致 404；而对已在映射表中的真实裸模型（如 gemini-2.5-flash），
	// 仅查找精确变体键以避免被通配符误匹配。
	mapped, modelMatched := resolveRequestedModelInMapping(mapping, model)
	if modelMatched && strings.TrimSpace(mapped) != model {
		return "", false
	}

	if preferred == "" {
		preferred = "high"
	}
	order := []string{preferred}
	for _, level := range []string{"high", "medium", "low", "tiered"} {
		if level != preferred {
			order = append(order, level)
		}
	}
	for _, level := range order {
		candidate := model + "-" + level
		var (
			variantMapped  string
			variantMatched bool
		)
		if modelMatched {
			variantMapped, variantMatched = mapping[candidate]
		} else {
			variantMapped, variantMatched = resolveRequestedModelInMapping(mapping, candidate)
		}
		if variantMatched && strings.TrimSpace(variantMapped) != "" {
			return variantMapped, true
		}
	}
	return "", false
}
