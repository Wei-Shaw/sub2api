package service

import (
	"encoding/json"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/domain"
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
// 显式映射（裸名映射到其他模型）始终优先；默认 tiered 家族的裸名自映射
// （无论 runtime 默认表还是 UI 保存的默认表）都会继续推导，其余自映射保持透传。

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

// accountRawModelMappingHasKey 判断 key 是否由用户写在账号 credentials.model_mapping 里
// （而不是 resolveModelMapping 运行时补进来的默认透传条目）。
func accountRawModelMappingHasKey(account *Account, key string) bool {
	if account == nil || account.Credentials == nil {
		return false
	}
	raw, _ := account.Credentials["model_mapping"].(map[string]any)
	_, ok := raw[key]
	return ok
}

// resolveGeminiThinkingVariant 为裸 Gemini 模型名挑选账号映射表里存在的思考深度变体。
// 返回 (映射后的上游模型名, 是否命中)。未命中时调用方应回退到常规 getMappedModel 流程。
func resolveGeminiThinkingVariant(account *Account, requestedModel string, body []byte) (string, bool) {
	return resolveGeminiThinkingVariantForLevel(account, requestedModel, geminiThinkingLevelFromBody(body))
}

// isDefaultAntigravityTieredBareModel 判断裸模型名是否属于默认目录中登记了
// tier 变体家族的模型（当前为 gemini-3.6/3.7/3.8-flash）。这类裸名在
// domain.DefaultAntigravityModelMapping 中是自映射，而上游目录只登记带
// -low/-medium/-high/-tiered 后缀的变体，裸名透传会被 404 拒绝。
func isDefaultAntigravityTieredBareModel(model string) bool {
	if domain.DefaultAntigravityModelMapping[model] != model {
		return false
	}
	for _, suffix := range geminiThinkingVariantSuffixes {
		if _, ok := domain.DefaultAntigravityModelMapping[model+suffix]; ok {
			return true
		}
	}
	return false
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
	// 裸名本身已有"真正的"映射 → 尊重现有配置，不做推导。判定分两层：
	//   1. 映射目标不是自己（如 gemini-3.8-flash → gemini-3.8-flash-tiered）：用户明确指定了目标；
	//   2. 映射目标是自己（原样透传）：仅当这条自映射表达的是用户意图时才尊重。
	//      默认 tiered 家族（isDefaultAntigravityTieredBareModel）的裸名自映射
	//      有两种来源，都不算用户意图：
	//      a) 运行时默认表（空映射时 GetModelMapping 整表返回
	//         domain.DefaultAntigravityModelMapping；ensureAntigravityDefaultPassthroughs
	//         也会补带后缀的变体，但不含裸名）；
	//      b) 新建账号 UI 把 domain.DefaultAntigravityModelMapping 整表保存进
	//         credentials.model_mapping —— 裸名自映射只是目录登记，不是手写意图。
	//      家族外模型（如 gemini-2.5-flash、未知自定义裸名）的上游目录存在裸名
	//      或行为未知，用户显式写成自映射仍视为意图、原样尊重。
	if mapped, matched := resolveRequestedModelInMapping(mapping, model); matched {
		if strings.TrimSpace(mapped) != model {
			return "", false
		}
		if accountRawModelMappingHasKey(account, model) && !isDefaultAntigravityTieredBareModel(model) {
			return "", false
		}
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
		if mapped, matched := resolveRequestedModelInMapping(mapping, candidate); matched && strings.TrimSpace(mapped) != "" {
			return mapped, true
		}
	}
	return "", false
}
