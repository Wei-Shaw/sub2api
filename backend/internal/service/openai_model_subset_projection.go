package service

import (
	"sort"
	"strconv"
	"strings"
)

const (
	openAICapabilityExplicitModelsExtraKey = "openai_capability_explicit_models"
	openAICapabilityWildcardRulesExtraKey  = "openai_capability_wildcard_rules"
	openAICapabilityDefaultAllowExtraKey   = "openai_capability_default_allow"
)

type OpenAIModelCapabilitySnapshot struct {
	ExplicitModels map[string]struct{}
	WildcardRules  []string
	DefaultAllow   bool
}

func BuildOpenAICanonicalModelCatalog(accounts []Account, explicitCapabilityModels []string, configuredModels []string) []string {
	catalog := make(map[string]struct{})
	addOpenAICanonicalModels(catalog, explicitCapabilityModels...)
	addOpenAICanonicalModels(catalog, configuredModels...)

	for _, account := range accounts {
		snapshot := buildOpenAIModelCapabilitySnapshot(account)
		for model := range snapshot.ExplicitModels {
			catalog[model] = struct{}{}
		}
		addOpenAICanonicalModelsFromMapping(catalog, account.GetModelMapping())
		addOpenAICanonicalModelsFromGroups(catalog, account.Groups)
	}

	models := make([]string, 0, len(catalog))
	for model := range catalog {
		models = append(models, model)
	}
	sort.Strings(models)
	return models
}

func accountSupportsProjectionModel(account Account, model string) bool {
	snapshot := buildOpenAIModelCapabilitySnapshot(account)
	catalog := stringSliceToSet(BuildOpenAICanonicalModelCatalog([]Account{account}, nil, nil))
	return account.SupportsProjectionModel(model, snapshot, catalog)
}

func buildOpenAIModelCapabilitySnapshot(account Account) OpenAIModelCapabilitySnapshot {
	snapshot := OpenAIModelCapabilitySnapshot{
		ExplicitModels: make(map[string]struct{}),
	}
	if account.Extra == nil {
		return snapshot
	}

	for _, model := range parseOpenAIProjectionStringSlice(account.Extra[openAICapabilityExplicitModelsExtraKey]) {
		if canonical := NormalizeOpenAIProjectionModelKey(model); canonical != "" {
			snapshot.ExplicitModels[canonical] = struct{}{}
		}
	}
	for _, rule := range parseOpenAIProjectionStringSlice(account.Extra[openAICapabilityWildcardRulesExtraKey]) {
		if normalized := normalizeOpenAIProjectionPattern(rule); normalized != "" {
			snapshot.WildcardRules = append(snapshot.WildcardRules, normalized)
		}
	}
	snapshot.DefaultAllow = parseOpenAIProjectionBool(account.Extra[openAICapabilityDefaultAllowExtraKey])
	return snapshot
}

func addOpenAICanonicalModels(catalog map[string]struct{}, models ...string) {
	for _, model := range models {
		if strings.Contains(model, "*") {
			continue
		}
		if canonical := NormalizeOpenAIProjectionModelKey(model); canonical != "" {
			catalog[canonical] = struct{}{}
		}
	}
}

func addOpenAICanonicalModelsFromMapping(catalog map[string]struct{}, mapping map[string]string) {
	for requestedModel, mappedModel := range mapping {
		if !strings.Contains(requestedModel, "*") {
			addOpenAICanonicalModels(catalog, requestedModel)
		}
		addOpenAICanonicalModels(catalog, mappedModel)
	}
}

func addOpenAICanonicalModelsFromGroups(catalog map[string]struct{}, groups []*Group) {
	for _, group := range groups {
		if group == nil {
			continue
		}
		addOpenAICanonicalModels(catalog, group.DefaultMappedModel)
		if !group.AllowMessagesDispatch {
			continue
		}

		cfg := normalizeOpenAIMessagesDispatchModelConfig(group.MessagesDispatchModelConfig)
		addOpenAICanonicalModels(catalog, cfg.OpusMappedModel, cfg.SonnetMappedModel, cfg.HaikuMappedModel)
		for _, mappedModel := range cfg.ExactModelMappings {
			addOpenAICanonicalModels(catalog, mappedModel)
		}
	}
}

func mappingSupportsProjectionModel(mapping map[string]string, canonicalModel string) bool {
	if canonicalModel == "" || len(mapping) == 0 {
		return false
	}
	for requestedModel, mappedModel := range mapping {
		if NormalizeOpenAIProjectionModelKey(mappedModel) == canonicalModel {
			return true
		}
		if projectionPatternMatches(requestedModel, canonicalModel) {
			return true
		}
	}
	return false
}

func wildcardRulesSupportProjectionModel(rules []string, canonicalModel string) bool {
	for _, rule := range rules {
		if projectionPatternMatches(rule, canonicalModel) {
			return true
		}
	}
	return false
}

func projectionPatternMatches(pattern, canonicalModel string) bool {
	normalizedPattern := normalizeOpenAIProjectionPattern(pattern)
	if normalizedPattern == "" || canonicalModel == "" {
		return false
	}
	if strings.HasSuffix(normalizedPattern, "*") {
		return strings.HasPrefix(canonicalModel, strings.TrimSuffix(normalizedPattern, "*"))
	}
	return normalizedPattern == canonicalModel
}

func normalizeOpenAIProjectionPattern(pattern string) string {
	trimmed := strings.TrimSpace(StripSysSuffix(pattern))
	if trimmed == "" {
		return ""
	}
	if !strings.Contains(trimmed, "*") {
		return NormalizeOpenAIProjectionModelKey(trimmed)
	}
	if !strings.HasSuffix(trimmed, "*") {
		return strings.ToLower(trimmed)
	}
	prefix := strings.TrimSpace(strings.TrimSuffix(trimmed, "*"))
	if prefix == "" {
		return ""
	}
	normalizedPrefix := NormalizeOpenAIProjectionModelKey(prefix)
	if normalizedPrefix == "" {
		normalizedPrefix = strings.ToLower(openAICompatModelID(prefix))
	}
	if normalizedPrefix == "" {
		return ""
	}
	return normalizedPrefix + "*"
}

func parseOpenAIProjectionStringSlice(value any) []string {
	if value == nil {
		return nil
	}

	var raw []string
	switch v := value.(type) {
	case []string:
		raw = v
	case []any:
		raw = make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				raw = append(raw, s)
			}
		}
	default:
		return nil
	}

	out := make([]string, 0, len(raw))
	for _, item := range raw {
		trimmed := strings.TrimSpace(item)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func parseOpenAIProjectionBool(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(v))
		return err == nil && parsed
	default:
		return false
	}
}

func stringSliceToSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		set[value] = struct{}{}
	}
	return set
}
