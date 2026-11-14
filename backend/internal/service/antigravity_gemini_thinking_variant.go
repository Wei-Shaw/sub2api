package service

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/tidwall/gjson"
)

type antigravityThinkingLevelContextKey struct{}

// WithAntigravityThinkingLevelFromClaudeBody 从策略改写后的最终 Claude 请求体提取
// Antigravity 思考档位写入 ctx，确保混合分组调度冷却预检与实际转发命中同一变体。
func WithAntigravityThinkingLevelFromClaudeBody(ctx context.Context, body []byte) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	var probe struct {
		Thinking     *antigravity.ThinkingConfig     `json:"thinking"`
		OutputConfig *antigravity.ClaudeOutputConfig `json:"output_config"`
	}
	if len(body) > 0 && json.Unmarshal(body, &probe) == nil {
		level := geminiThinkingLevelFromClaudeRequest(&antigravity.ClaudeRequest{
			Thinking:     probe.Thinking,
			OutputConfig: probe.OutputConfig,
		})
		return context.WithValue(ctx, antigravityThinkingLevelContextKey{}, level)
	}
	return ctx
}

// WithAntigravityThinkingLevelFromGeminiBody 从 Gemini 原生请求体提取思考档位写入 ctx。
func WithAntigravityThinkingLevelFromGeminiBody(ctx context.Context, body []byte) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, antigravityThinkingLevelContextKey{}, geminiThinkingLevelFromBody(body))
}

// AntigravityThinkingLevelFromContext 读取请求体解析出的有效 Antigravity 思考档位。
func AntigravityThinkingLevelFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	level, ok := ctx.Value(antigravityThinkingLevelContextKey{}).(string)
	return level, ok && level != ""
}

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

var geminiThinkingVariantSuffixes = []string{"-low", "-medium", "-high", "-xhigh", "-tiered"}

const (
	geminiThinkingBudgetLowMax    = 1024
	geminiThinkingBudgetMediumMax = 8192
)

type geminiThinkingConfigProbe struct {
	GenerationConfig      *geminiThinkingGenerationConfigProbe `json:"generationConfig"`
	GenerationConfigSnake *geminiThinkingGenerationConfigProbe `json:"generation_config"`
}

type geminiThinkingGenerationConfigProbe struct {
	ThinkingConfig      *geminiThinkingInnerProbe `json:"thinkingConfig"`
	ThinkingConfigSnake *geminiThinkingInnerProbe `json:"thinking_config"`
}

type geminiThinkingInnerProbe struct {
	ThinkingBudget      *json.Number `json:"thinkingBudget"`
	ThinkingBudgetSnake *json.Number `json:"thinking_budget"`
	ThinkingLevel       string       `json:"thinkingLevel"`
	ThinkingLevelSnake  string       `json:"thinking_level"`
}

// isAntigravityThinkingVariantFamily 判断模型是否属于 Antigravity 按思考深度拆分的模型族
// （gemini-* 以及 claude-opus-5-5* / claude-sonnet-5-5*）。
func isAntigravityThinkingVariantFamily(model string) bool {
	lower := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(model)), "models/")
	return strings.HasPrefix(lower, "gemini-") ||
		lower == "claude-opus-5-5" ||
		lower == "claude-sonnet-5-5" ||
		strings.HasPrefix(lower, "claude-opus-5-5-") ||
		strings.HasPrefix(lower, "claude-sonnet-5-5-")
}

// hasGeminiThinkingVariantSuffix 判断模型名是否已经带了思考深度后缀。
func hasGeminiThinkingVariantSuffix(model string) bool {
	lower := strings.ToLower(strings.TrimSpace(model))
	for _, suffix := range geminiThinkingVariantSuffixes {
		if strings.HasSuffix(lower, suffix) {
			return true
		}
	}
	return false
}

// antigravityRateLimitModelKey 在裸 Gemini 模型按请求思考档位落到自映射变体时返回实际转发变体，
// 避免模型级冷却将低档位失败误记到默认高档位，同时对无自映射目标的显式别名保留原名以免二次映射置空。
func antigravityRateLimitModelKey(account *Account, originalModel, mappedModel string) string {
	if mappedModel != "" && mappedModel != originalModel && hasGeminiThinkingVariantSuffix(mappedModel) && mapAntigravityModel(account, mappedModel) == mappedModel {
		return mappedModel
	}
	return originalModel
}

// effectiveAntigravityGeminiReasoningEffort 提取转发到 Antigravity 分档模型族时实际生效的思考档位，
// 仅在客户端显式指定了 effort 且目标属于分档模型族时返回（不给未传 effort 的请求虚构计费档位）。
func effectiveAntigravityGeminiReasoningEffort(mappedModel string, wrappedGeminiBody []byte, hadExplicitEffort bool) *string {
	if !hadExplicitEffort {
		return nil
	}
	actualModel := strings.TrimSpace(gjson.GetBytes(wrappedGeminiBody, "model").String())
	if actualModel == "" {
		actualModel = mappedModel
	}
	lowerModel := strings.TrimPrefix(strings.ToLower(actualModel), "models/")
	if !isAntigravityThinkingVariantFamily(lowerModel) {
		return nil
	}
	requestRaw := gjson.GetBytes(wrappedGeminiBody, "request").Raw
	if requestRaw == "" {
		requestRaw = string(wrappedGeminiBody)
	}
	if effort := extractGeminiReasoningEffortFromBody([]byte(requestRaw)); effort != nil {
		return effort
	}
	for _, level := range []string{"xhigh", "high", "medium", "low"} {
		if strings.HasSuffix(lowerModel, "-"+level) {
			resolved := level
			return &resolved
		}
	}
	return nil
}

// geminiThinkingLevelFromBody 从 Gemini 原生请求体推导期望的思考档位（low/medium/high/xhigh）。
// 解析失败或未携带 thinkingConfig 时返回 "high"。
func geminiThinkingLevelFromBody(body []byte) string {
	if len(body) == 0 {
		return "high"
	}
	var probe geminiThinkingConfigProbe
	if err := json.Unmarshal(body, &probe); err != nil {
		return "high"
	}
	gc := probe.GenerationConfig
	if gc == nil {
		gc = probe.GenerationConfigSnake
	}
	if gc == nil {
		return "high"
	}
	tc := gc.ThinkingConfig
	if tc == nil {
		tc = gc.ThinkingConfigSnake
	}
	if tc == nil {
		return "high"
	}
	rawLevel := strings.TrimSpace(tc.ThinkingLevel)
	if rawLevel == "" {
		rawLevel = strings.TrimSpace(tc.ThinkingLevelSnake)
	}
	switch strings.ToLower(rawLevel) {
	case "minimal", "low":
		return "low"
	case "medium":
		return "medium"
	case "high":
		return "high"
	case "xhigh":
		return "xhigh"
	}
	rawBudget := tc.ThinkingBudget
	if rawBudget == nil {
		rawBudget = tc.ThinkingBudgetSnake
	}
	if rawBudget == nil {
		return "high"
	}
	budget, err := rawBudget.Float64()
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

// geminiThinkingLevelFromClaudeRequest 用 Claude Messages 协议的 thinking 与 output_config.effort
// 推导档位（disabled > effort > budget_tokens），与 Carbon/Hydrogen 思考配置及 Gemini 原生路径保持一致。
func geminiThinkingLevelFromClaudeRequest(req *antigravity.ClaudeRequest) string {
	if level := antigravity.ResolveClaudeThinkingLevel(req); level != "" {
		return level
	}
	return "high"
}

// resolveGeminiThinkingVariant 为裸 Gemini 模型名挑选账号映射表里存在的思考深度变体。
// 返回 (映射后的上游模型名, 是否命中)。未命中时调用方应回退到常规 getMappedModel 流程。
func resolveGeminiThinkingVariant(account *Account, requestedModel string, body []byte) (string, bool) {
	return resolveGeminiThinkingVariantForLevel(account, requestedModel, geminiThinkingLevelFromBody(body))
}

func resolveGeminiThinkingVariantForLevel(account *Account, requestedModel string, preferred string) (string, bool) {
	if account == nil {
		return "", false
	}
	model := strings.TrimPrefix(strings.TrimSpace(requestedModel), "models/")
	if !isAntigravityThinkingVariantFamily(model) || hasGeminiThinkingVariantSuffix(model) {
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
	for _, level := range []string{"high", "medium", "low", "xhigh", "tiered"} {
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
	// 当账号显式添加了裸动态分档自映射（如前端点击 Opus 5.5 / Sonnet 5.5 / 4 Hydrogen 预设
	// 产生 base -> base），但未单独逐条录入后缀变体时，回退到官方默认分档目录避免把裸名直接发往上游 404。
	if modelMatched && strings.TrimSpace(mapped) == model {
		for _, level := range order {
			if defaultTarget, ok := domain.DefaultAntigravityModelMapping[model+"-"+level]; ok && strings.TrimSpace(defaultTarget) != "" {
				return defaultTarget, true
			}
		}
	}
	return "", false
}
