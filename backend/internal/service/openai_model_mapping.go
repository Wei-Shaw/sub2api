package service

import "strings"

// resolveOpenAIForwardModel 解析 OpenAI 兼容转发使用的模型。
// defaultMappedModel 只服务于 /v1/messages 的 Claude 系列显式调度映射，
// 不作为普通 OpenAI 请求的未知模型兜底。
func resolveOpenAIForwardModel(account *Account, requestedModel, defaultMappedModel string) string {
	if account == nil {
		if defaultMappedModel != "" && claudeMessagesDispatchFamily(requestedModel) != "" {
			return defaultMappedModel
		}
		return requestedModel
	}

	mappedModel, matched := account.ResolveMappedModel(requestedModel)
	if !matched && defaultMappedModel != "" && claudeMessagesDispatchFamily(requestedModel) != "" {
		return defaultMappedModel
	}
	return mappedModel
}
func resolveOpenAIUpstreamModel(model string) string {
	if isBareGPT53CodexSparkModel(model) {
		return "gpt-5.3-codex-spark"
	}
	return normalizeCodexModel(strings.TrimSpace(model))
}

func isBareGPT53CodexSparkModel(model string) bool {
	modelID := strings.TrimSpace(model)
	if modelID == "" {
		return false
	}
	if strings.Contains(modelID, "/") {
		parts := strings.Split(modelID, "/")
		modelID = parts[len(parts)-1]
	}
	normalized := strings.ToLower(strings.TrimSpace(modelID))
	return normalized == "gpt-5.3-codex-spark" || normalized == "gpt 5.3 codex spark"
}

func IsSysModel(model string) bool {
	modelID := strings.TrimSpace(model)
	if modelID == "" || len(modelID) < len("-sys") {
		return false
	}
	return strings.EqualFold(modelID[len(modelID)-len("-sys"):], "-sys")
}

func StripSysSuffix(model string) string {
	modelID := strings.TrimSpace(model)
	if !IsSysModel(modelID) {
		return modelID
	}
	return strings.TrimSpace(modelID[:len(modelID)-len("-sys")])
}

// resolveOpenAICompactForwardModel determines the compact-only upstream model
// for /responses/compact requests. It never affects normal /responses traffic.
// When no compact-specific mapping matches, the input model is returned as-is.
func resolveOpenAICompactForwardModel(account *Account, model string) string {
	trimmedModel := strings.TrimSpace(model)
	if trimmedModel == "" || account == nil {
		return trimmedModel
	}

	mappedModel, matched := account.ResolveCompactMappedModel(trimmedModel)
	if !matched {
		return trimmedModel
	}
	if trimmedMapped := strings.TrimSpace(mappedModel); trimmedMapped != "" {
		return trimmedMapped
	}
	return trimmedModel
}
