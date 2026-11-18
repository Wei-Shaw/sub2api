package service

import (
	"context"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

type reasoningEffortStepdownContextKey struct{}

// WithReasoningEffortStepdown 把分组自适应降档开关绑到请求上下文，供转发时
// 按最终上游模型元数据降档。默认关闭（零值 false 即原样透传）。
func WithReasoningEffortStepdown(ctx context.Context, enabled bool) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, reasoningEffortStepdownContextKey{}, enabled)
}

// reasoningEffortStepdownEnabled 从请求上下文读开关；gin.Context 优先用其
// Request.Context，保证 handler 绑定的值在转发路径可见。
func reasoningEffortStepdownEnabled(ctx context.Context, c *gin.Context) bool {
	if c != nil && c.Request != nil {
		if enabled, ok := c.Request.Context().Value(reasoningEffortStepdownContextKey{}).(bool); ok {
			return enabled
		}
	}
	if ctx == nil {
		return false
	}
	enabled, _ := ctx.Value(reasoningEffortStepdownContextKey{}).(bool)
	return enabled
}

// applyReasoningEffortStepdownForAccount 在转发组装上游请求时应用自适应降档：
// 开关关闭、无上游元数据、模型不支持推理时一律原样返回。upstreamModel 应为
// 全部映射解析后的最终上游模型名。
func applyReasoningEffortStepdownForAccount(ctx context.Context, c *gin.Context, account *Account, upstreamModel string, body []byte) ([]byte, bool) {
	if !reasoningEffortStepdownEnabled(ctx, c) {
		return body, false
	}
	if account == nil || len(body) == 0 {
		return body, false
	}
	model := strings.TrimSpace(upstreamModel)
	if model == "" {
		model = strings.TrimSpace(gjson.GetBytes(body, "model").String())
	}
	if model == "" {
		return body, false
	}
	metadata, ok := account.GetUpstreamModelMetadata(model)
	if !ok {
		return body, false
	}
	if metadata.Reasoning != nil && !*metadata.Reasoning {
		return body, false
	}
	levels := normalizeReasoningLevels(metadata.SupportedReasoningLevels)
	if len(levels) == 0 {
		return body, false
	}
	stepped, changed, err := ApplyReasoningEffortStepdown(body, levels)
	if err != nil || !changed {
		return body, false
	}
	return stepped, true
}

// reasoningEffortStepdownRank 给推理强度定序，ultra 为最高档（7）。
// "none" 与未知值不参与排序，调用方保持原样透传。
func reasoningEffortStepdownRank(raw string) (int, string, bool) {
	value := strings.ToLower(strings.TrimSpace(raw))
	value = reasoningEffortSeparatorRemover.Replace(value)
	switch value {
	case "minimal":
		return 1, "minimal", true
	case "low":
		return 2, "low", true
	case "medium":
		return 3, "medium", true
	case "high":
		return 4, "high", true
	case "xhigh", "extrahigh":
		return 5, "xhigh", true
	case "max":
		return 6, "max", true
	case "ultra":
		return 7, "ultra", true
	default:
		return 0, "", false
	}
}

// stepdownReasoningEffort 按上游模型的支持列表降档：返回支持列表里不超过请求
// 的最高档。请求档位已支持、低于下限、无支持列表时返回 ok=false（只降不升，
// 未知情况一律原样透传）。
func stepdownReasoningEffort(requested string, supported []string) (string, bool) {
	if strings.EqualFold(strings.TrimSpace(requested), "none") {
		return "", false
	}
	reqRank, reqCanonical, ok := reasoningEffortStepdownRank(requested)
	if !ok {
		return "", false
	}
	bestRank := 0
	best := ""
	for _, level := range supported {
		rank, canonical, ok := reasoningEffortStepdownRank(level)
		if !ok || rank > reqRank || rank <= bestRank {
			continue
		}
		bestRank, best = rank, canonical
	}
	if bestRank == 0 || best == reqCanonical {
		return "", false
	}
	return best, true
}

// ApplyReasoningEffortStepdown 把 body 里的推理强度按 supported 自适应降档。
// supported 为空或请求无需降档时原样返回。理解 OpenAI 与 Anthropic 两种字段
// 形态（reasoning.effort / reasoning_effort / output_config.effort）。
func ApplyReasoningEffortStepdown(body []byte, supported []string) ([]byte, bool, error) {
	if len(body) == 0 || len(supported) == 0 {
		return body, false, nil
	}
	result := body
	changed := false
	for _, path := range []string{"reasoning.effort", "reasoning_effort", "output_config.effort"} {
		field := gjson.GetBytes(result, path)
		if !field.Exists() || field.Type != gjson.String {
			continue
		}
		original := strings.TrimSpace(field.String())
		if original == "" {
			continue
		}
		effective, ok := stepdownReasoningEffort(original, supported)
		if !ok || effective == original {
			continue
		}
		updated, err := sjson.SetBytes(result, path, effective)
		if err != nil {
			continue
		}
		result = updated
		changed = true
	}
	return result, changed, nil
}
