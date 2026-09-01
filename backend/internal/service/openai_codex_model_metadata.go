package service

import "strings"

func groupCodexModelMetadata(
	platform string,
	modelID string,
	accounts []Account,
	compositeRoutes []CompositeModelRoute,
	compositeRoutesAvailable bool,
) (codexModelMetadataOverride, bool) {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return codexModelMetadataOverride{}, false
	}
	upstreamModel := modelID
	if platform == PlatformComposite {
		var resolved bool
		platform, upstreamModel, resolved = resolveCodexCompositeModelTarget(
			modelID,
			accounts,
			compositeRoutes,
			compositeRoutesAvailable,
		)
		if !resolved {
			if codexExplicitModelTargetsConflict(accounts, modelID) {
				return codexModelMetadataOverride{
					reasoningConflict:       true,
					inputModalitiesConflict: true,
				}, true
			}
			return codexModelMetadataOverride{}, false
		}
	}
	if !isConcreteRequestPlatform(platform) {
		return codexModelMetadataOverride{}, false
	}

	explicitClaims := false
	if upstreamModel == modelID {
		for _, account := range accounts {
			if account.Platform == platform && codexExplicitModelMappingClaims(account, modelID) {
				explicitClaims = true
				break
			}
		}
	}
	explicitTargetsConflict := explicitClaims && codexExplicitModelTargetsConflictForPlatform(accounts, platform, modelID)
	publicAlias := upstreamModel != modelID
	candidates := make([]UpstreamModelMetadata, 0)
	for i := range accounts {
		account := &accounts[i]
		if account.Platform != platform {
			continue
		}
		var lookupModel string
		if explicitClaims {
			if !codexExplicitModelMappingClaims(*account, modelID) {
				continue
			}
			lookupModel = account.GetMappedModel(modelID)
		} else {
			if !account.IsModelSupported(upstreamModel) {
				continue
			}
			lookupModel = account.GetMappedModel(upstreamModel)
		}
		if strings.TrimSpace(lookupModel) != modelID {
			publicAlias = true
		}
		metadata, ok := account.GetUpstreamModelMetadata(lookupModel)
		if !ok {
			if explicitTargetsConflict {
				return codexModelMetadataOverride{
					reasoningConflict:       true,
					inputModalitiesConflict: true,
				}, true
			}
			return codexModelMetadataOverride{}, false
		}
		candidates = append(candidates, metadata)
	}
	if len(candidates) == 0 {
		return codexModelMetadataOverride{}, false
	}
	metadata := intersectUpstreamModelMetadata(modelID, candidates)
	if publicAlias {
		metadata.DisplayName = modelID
		metadata.Description = configuredCodexCustomDescription
	}
	return metadata, true
}

func codexExplicitModelTargetsConflict(accounts []Account, modelID string) bool {
	targets := make(map[string]struct{})
	for i := range accounts {
		account := &accounts[i]
		mappedModel, matched := account.ResolveMappedModel(modelID)
		mappedModel = strings.TrimSpace(mappedModel)
		if !matched || mappedModel == "" {
			continue
		}
		targets[strings.TrimSpace(account.Platform)+"\x00"+mappedModel] = struct{}{}
	}
	return len(targets) > 1
}

func codexExplicitModelTargetsConflictForPlatform(accounts []Account, platform, modelID string) bool {
	targets := make(map[string]struct{})
	for i := range accounts {
		account := &accounts[i]
		if account.Platform != platform {
			continue
		}
		mappedModel, matched := account.ResolveMappedModel(modelID)
		mappedModel = strings.TrimSpace(mappedModel)
		if !matched || mappedModel == "" {
			continue
		}
		targets[mappedModel] = struct{}{}
	}
	return len(targets) > 1
}

func intersectUpstreamModelMetadata(modelID string, candidates []UpstreamModelMetadata) codexModelMetadataOverride {
	result := codexModelMetadataOverride{UpstreamModelMetadata: UpstreamModelMetadata{ID: strings.TrimSpace(modelID)}}
	for _, candidate := range candidates {
		if result.DisplayName == "" && strings.TrimSpace(candidate.DisplayName) != "" {
			result.DisplayName = strings.TrimSpace(candidate.DisplayName)
		}
		if result.Description == "" && strings.TrimSpace(candidate.Description) != "" {
			result.Description = strings.TrimSpace(candidate.Description)
		}
	}

	reasoningKnown := true
	reasoningValue := false
	for i, candidate := range candidates {
		if candidate.Reasoning == nil {
			reasoningKnown = false
			break
		}
		if i == 0 {
			reasoningValue = *candidate.Reasoning
			continue
		}
		if reasoningValue != *candidate.Reasoning {
			reasoningKnown = false
			result.reasoningConflict = true
			break
		}
	}
	if reasoningKnown {
		result.Reasoning = &reasoningValue
		if reasoningValue {
			levels := normalizeReasoningLevels(candidates[0].SupportedReasoningLevels)
			for _, candidate := range candidates[1:] {
				levels = intersectOrderedStrings(levels, normalizeReasoningLevels(candidate.SupportedReasoningLevels))
			}
			result.SupportedReasoningLevels = levels
			if len(levels) == 0 {
				result.reasoningConflict = true
			} else {
				sharedDefault := normalizeReasoningLevel(candidates[0].DefaultReasoningLevel)
				for _, candidate := range candidates[1:] {
					if normalizeReasoningLevel(candidate.DefaultReasoningLevel) != sharedDefault {
						sharedDefault = ""
						break
					}
				}
				if !stringSliceContains(levels, sharedDefault) {
					sharedDefault = levels[0]
				}
				result.DefaultReasoningLevel = sharedDefault
			}
		}
	}

	modalitiesKnown := true
	modalities := normalizeCodexInputModalities(candidates[0].InputModalities)
	if len(modalities) == 0 {
		modalitiesKnown = false
	}
	for _, candidate := range candidates[1:] {
		candidateModalities := normalizeCodexInputModalities(candidate.InputModalities)
		if len(candidateModalities) == 0 {
			modalitiesKnown = false
			break
		}
		modalities = intersectOrderedStrings(modalities, candidateModalities)
	}
	if modalitiesKnown && len(modalities) > 0 {
		result.InputModalities = modalities
	} else if modalitiesKnown {
		result.inputModalitiesConflict = true
	}

	contextKnown := true
	for i, candidate := range candidates {
		if candidate.ContextWindow <= 0 {
			contextKnown = false
			break
		}
		if i == 0 || candidate.ContextWindow < result.ContextWindow {
			result.ContextWindow = candidate.ContextWindow
		}
	}
	if !contextKnown {
		result.ContextWindow = 0
	}
	return result
}

func applyUpstreamModelMetadataToCodexDescriptor(
	descriptor *configuredCodexModelDescriptor,
	metadata codexModelMetadataOverride,
) {
	if descriptor == nil {
		return
	}
	if strings.TrimSpace(metadata.DisplayName) != "" {
		descriptor.DisplayName = strings.TrimSpace(metadata.DisplayName)
	}
	if strings.TrimSpace(metadata.Description) != "" {
		descriptor.Description = strings.TrimSpace(metadata.Description)
	}
	if metadata.reasoningConflict {
		descriptor.DefaultReasoningLevel = nil
		descriptor.SupportedReasoningLevels = []configuredCodexReasoningLevel{}
	} else if metadata.Reasoning != nil && !*metadata.Reasoning {
		none := "none"
		descriptor.DefaultReasoningLevel = &none
		descriptor.SupportedReasoningLevels = []configuredCodexReasoningLevel{{
			Effort:      "none",
			Description: configuredCodexReasoningLevelDescription("none"),
		}}
	} else if metadata.Reasoning != nil && *metadata.Reasoning {
		levels := normalizeReasoningLevels(metadata.SupportedReasoningLevels)
		if len(levels) == 0 {
			descriptor.DefaultReasoningLevel = nil
			descriptor.SupportedReasoningLevels = []configuredCodexReasoningLevel{}
		} else {
			defaultLevel := normalizeReasoningLevel(metadata.DefaultReasoningLevel)
			if !stringSliceContains(levels, defaultLevel) {
				defaultLevel = levels[0]
			}
			descriptor.DefaultReasoningLevel = &defaultLevel
			descriptor.SupportedReasoningLevels = make([]configuredCodexReasoningLevel, 0, len(levels))
			for _, level := range levels {
				descriptor.SupportedReasoningLevels = append(descriptor.SupportedReasoningLevels, configuredCodexReasoningLevel{
					Effort:      level,
					Description: configuredCodexReasoningLevelDescription(level),
				})
			}
		}
	}
	if metadata.inputModalitiesConflict {
		descriptor.InputModalities = []string{"text"}
	} else if modalities := normalizeCodexInputModalities(metadata.InputModalities); len(modalities) > 0 {
		descriptor.InputModalities = modalities
	}
	if metadata.ContextWindow > 0 {
		descriptor.ContextWindow = metadata.ContextWindow
		descriptor.MaxContextWindow = metadata.ContextWindow
	}
}

func configuredCodexReasoningLevelDescription(level string) string {
	switch level {
	case "none":
		return "Use the model's default behavior without configurable reasoning"
	case "minimal":
		return "Minimal reasoning for the fastest responses"
	case "low":
		return "Fast responses with lighter reasoning"
	case "medium":
		return "Balanced reasoning for most coding tasks"
	case "high":
		return "Greater reasoning depth for coding and agent tasks"
	case "xhigh":
		return "Extra-high reasoning depth for difficult tasks"
	case "max":
		return "Maximum reasoning depth for complex tasks"
	default:
		return "Reasoning effort supported by the upstream model"
	}
}

func intersectOrderedStrings(left, right []string) []string {
	rightSet := make(map[string]struct{}, len(right))
	for _, value := range right {
		rightSet[value] = struct{}{}
	}
	intersection := make([]string, 0, len(left))
	for _, value := range left {
		if _, ok := rightSet[value]; ok {
			intersection = append(intersection, value)
		}
	}
	return intersection
}

func stringSliceContains(values []string, target string) bool {
	if target == "" {
		return false
	}
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
