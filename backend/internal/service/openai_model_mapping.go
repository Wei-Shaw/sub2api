package service

import "strings"

// resolveOpenAIForwardModel resolves the account/group mapping result for
// OpenAI-compatible forwarding. Group-level default mapping only applies when
// the account itself did not match any explicit model_mapping rule.
func resolveOpenAIForwardModel(account *Account, requestedModel, defaultMappedModel string) string {
	if account == nil {
		if defaultMappedModel != "" {
			return defaultMappedModel
		}
		return requestedModel
	}

	mappedModel, matched := account.ResolveMappedModel(requestedModel)
	if !matched && defaultMappedModel != "" {
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
