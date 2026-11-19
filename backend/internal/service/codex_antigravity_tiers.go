package service

import "strings"

var codexAntigravityGeminiTiers = [...]string{"low", "medium", "high", "xhigh"}

func codexAntigravityGeminiTier(model string) (base, tier string, ok bool) {
	if !strings.HasPrefix(model, "gemini-") {
		return "", "", false
	}
	for _, suffix := range []string{"tiered-xhigh", "tiered-high", "low", "medium", "high", "xhigh"} {
		if base, ok := strings.CutSuffix(model, "-"+suffix); ok {
			return base, strings.TrimPrefix(suffix, "tiered-"), true
		}
	}
	return "", "", false
}

func hasConfiguredAntigravitySelfMapping(account *Account, model string) bool {
	if account.Platform != PlatformAntigravity {
		return false
	}
	mapping, _ := account.Credentials["model_mapping"].(map[string]any)
	target, _ := mapping[model].(string)
	return strings.TrimSpace(target) == model && account.GetModelMapping()[model] == model
}

// codexAntigravityTierMetadata describes a self-mapped base. An explicit
// mapping to another model still describes that single target.
func codexAntigravityTierMetadata(account *Account, base string) (UpstreamModelMetadata, bool) {
	if !strings.HasPrefix(base, "gemini-") || !hasConfiguredAntigravitySelfMapping(account, base) {
		return UpstreamModelMetadata{}, false
	}
	levels := make([]string, 0, len(codexAntigravityGeminiTiers))
	var candidates []UpstreamModelMetadata
	for _, tier := range codexAntigravityGeminiTiers {
		target, ok := resolveGeminiThinkingVariantForLevel(account, base, tier)
		targetBase, targetTier, isTier := codexAntigravityGeminiTier(target)
		if !ok || !isTier || targetBase != base || targetTier != tier {
			continue
		}
		levels = append(levels, tier)
		metadata, _ := account.GetUpstreamModelMetadata(target)
		candidates = append(candidates, metadata)
	}
	if len(levels) == 0 {
		return UpstreamModelMetadata{}, false
	}

	metadata := intersectUpstreamModelMetadata(base, candidates).UpstreamModelMetadata
	configuredBase, _ := account.GetUpstreamModelMetadata(base)
	metadata.DisplayName = configuredBase.DisplayName
	metadata.Description = configuredBase.Description
	reasoning := true
	metadata.Reasoning = &reasoning
	metadata.SupportedReasoningLevels = levels
	metadata.DefaultReasoningLevel = levels[0]
	if target, ok := resolveGeminiThinkingVariantForLevel(account, base, ""); ok {
		if _, tier, _ := codexAntigravityGeminiTier(target); stringSliceContains(levels, tier) {
			metadata.DefaultReasoningLevel = tier
		}
	}
	return metadata, true
}

func foldCodexAntigravityGeminiTiers(platform string, modelIDs []string, accounts []Account, group *Group, routes []CompositeModelRoute) []string {
	present := make(map[string]bool, len(modelIDs))
	for _, model := range modelIDs {
		present[strings.TrimSpace(model)] = true
	}
	out := make([]string, 0, len(modelIDs))
	for _, model := range modelIDs {
		modelID := strings.TrimSpace(model)
		base, tier, ok := codexAntigravityGeminiTier(modelID)
		if !ok || !present[base] || !canFoldCodexAntigravityTier(platform, modelID, base, tier, accounts, group, routes) {
			out = append(out, model)
		}
	}
	return out
}

func canFoldCodexAntigravityTier(platform, model, base, tier string, accounts []Account, group *Group, routes []CompositeModelRoute) bool {
	for _, id := range []string{model, base} {
		if len(codexModelRoutingAccountIDs(group, id)) > 0 {
			return false
		}
		if codexCompositeRouteMatchesModel(routes, id) {
			return false
		}
		if platform == PlatformComposite {
			targetPlatform, _, resolved := resolveCodexCompositeModelTarget(id, accounts, routes, true)
			if !resolved || targetPlatform != PlatformAntigravity {
				return false
			}
		}
	}
	if platform == PlatformComposite {
		platform = PlatformAntigravity
	}
	matched := false
	for i := range accounts {
		account := &accounts[i]
		if !codexAccountServesPlatformModel(account, platform, model) {
			continue
		}
		if !account.IsModelSupported(base) && !account.IsModelSupported(model) {
			continue
		}
		if !hasConfiguredAntigravitySelfMapping(account, base) || !hasConfiguredAntigravitySelfMapping(account, model) {
			return false
		}
		if target, ok := resolveGeminiThinkingVariantForLevel(account, base, tier); !ok || target != model {
			return false
		}
		matched = true
	}
	return matched
}
