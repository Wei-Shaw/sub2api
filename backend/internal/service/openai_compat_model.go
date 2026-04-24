package service

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
)

func NormalizeOpenAICompatRequestedModel(model string) string {
	trimmed := strings.TrimSpace(model)
	if trimmed == "" {
		return ""
	}

	normalized, _, ok := splitOpenAICompatReasoningModel(trimmed)
	if !ok || normalized == "" {
		return trimmed
	}
	return normalized
}

func NormalizeOpenAIProjectionModelKey(model string) string {
	trimmed := strings.TrimSpace(StripSysSuffix(model))
	if trimmed == "" {
		return ""
	}

	modelID := openAICompatModelID(trimmed)
	if modelID == "" {
		return ""
	}
	if mapped := getNormalizedCodexModel(modelID); mapped != "" {
		return mapped
	}
	if baseModel, _, ok := splitOpenAICompatReasoningBaseModel(modelID); ok {
		if mapped := getNormalizedCodexModel(baseModel); mapped != "" {
			return mapped
		}
	}
	return strings.ToLower(strings.TrimSpace(modelID))
}

func applyOpenAICompatModelNormalization(req *apicompat.AnthropicRequest) {
	if req == nil {
		return
	}

	originalModel := strings.TrimSpace(req.Model)
	if originalModel == "" {
		return
	}

	normalizedModel, derivedEffort, hasReasoningSuffix := splitOpenAICompatReasoningModel(originalModel)
	if hasReasoningSuffix && normalizedModel != "" {
		req.Model = normalizedModel
	}

	if req.OutputConfig != nil && strings.TrimSpace(req.OutputConfig.Effort) != "" {
		return
	}

	claudeEffort := openAIReasoningEffortToClaudeOutputEffort(derivedEffort)
	if claudeEffort == "" {
		return
	}

	if req.OutputConfig == nil {
		req.OutputConfig = &apicompat.AnthropicOutputConfig{}
	}
	req.OutputConfig.Effort = claudeEffort
}

func splitOpenAICompatReasoningModel(model string) (normalizedModel string, reasoningEffort string, ok bool) {
	baseModel, reasoningEffort, ok := splitOpenAICompatReasoningBaseModel(model)
	if !ok {
		return strings.TrimSpace(model), "", false
	}
	return normalizeCodexModel(baseModel), reasoningEffort, true
}

func splitOpenAICompatReasoningBaseModel(model string) (baseModel string, reasoningEffort string, ok bool) {
	trimmed := strings.TrimSpace(model)
	if trimmed == "" {
		return "", "", false
	}

	modelID := openAICompatModelID(trimmed)
	if !strings.HasPrefix(strings.ToLower(modelID), "gpt-") {
		return trimmed, "", false
	}

	lowerModelID := strings.ToLower(modelID)
	for _, suffix := range []struct {
		match  string
		effort string
	}{
		{match: "-extrahigh", effort: "xhigh"},
		{match: "_extrahigh", effort: "xhigh"},
		{match: " extrahigh", effort: "xhigh"},
		{match: "-xhigh", effort: "xhigh"},
		{match: "_xhigh", effort: "xhigh"},
		{match: " xhigh", effort: "xhigh"},
		{match: "-minimal", effort: ""},
		{match: "_minimal", effort: ""},
		{match: " minimal", effort: ""},
		{match: "-none", effort: ""},
		{match: "_none", effort: ""},
		{match: " none", effort: ""},
		{match: "-medium", effort: "medium"},
		{match: "_medium", effort: "medium"},
		{match: " medium", effort: "medium"},
		{match: "-high", effort: "high"},
		{match: "_high", effort: "high"},
		{match: " high", effort: "high"},
		{match: "-low", effort: "low"},
		{match: "_low", effort: "low"},
		{match: " low", effort: "low"},
	} {
		if !strings.HasSuffix(lowerModelID, suffix.match) {
			continue
		}
		baseModel = strings.TrimSpace(modelID[:len(modelID)-len(suffix.match)])
		if baseModel == "" {
			return trimmed, "", false
		}
		return baseModel, suffix.effort, true
	}

	return trimmed, "", false
}

func openAICompatModelID(model string) string {
	modelID := strings.TrimSpace(model)
	if modelID == "" {
		return ""
	}
	if strings.Contains(modelID, "/") {
		parts := strings.Split(modelID, "/")
		modelID = parts[len(parts)-1]
	}
	return strings.TrimSpace(modelID)
}

func openAIReasoningEffortToClaudeOutputEffort(effort string) string {
	switch strings.TrimSpace(effort) {
	case "low", "medium", "high":
		return effort
	case "xhigh":
		return "max"
	default:
		return ""
	}
}
