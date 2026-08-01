package service

import (
	"fmt"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	maxReasoningEffortMappings   = 64
	maxReasoningEffortValueLen   = 64
	maxModelReasoningEffortRules = 64
	maxReasoningEffortModelLen   = 200
)

var openAIReasoningEffortValues = []string{"minimal", "low", "medium", "high", "xhigh", "max"}

// NormalizeMaxReasoningEffort validates and canonicalizes a group policy value.
// Empty means that the group does not impose a ceiling.
func NormalizeMaxReasoningEffort(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	value = strings.NewReplacer("-", "", "_", "", " ", "").Replace(value)
	switch value {
	case "":
		return ""
	case "minimal":
		return "minimal"
	case "low":
		return "low"
	case "medium":
		return "medium"
	case "high":
		return "high"
	case "xhigh", "extrahigh":
		return "xhigh"
	case "max":
		return "max"
	default:
		return ""
	}
}

func reasoningEffortValuesForPlatform(platform string) []string {
	if platform != PlatformOpenAI {
		return nil
	}
	return openAIReasoningEffortValues
}

func normalizeMaxReasoningEffortForPlatform(platform, raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", nil
	}

	allowedValues := reasoningEffortValuesForPlatform(platform)
	if len(allowedValues) == 0 {
		return "", fmt.Errorf("reasoning effort policy is only supported for platform %q", PlatformOpenAI)
	}

	value := NormalizeMaxReasoningEffort(raw)
	for _, allowed := range allowedValues {
		if value == allowed {
			return value, nil
		}
	}
	return "", fmt.Errorf(
		"reasoning effort %q is not supported for platform %q; allowed values: %s",
		raw,
		platform,
		strings.Join(allowedValues, ", "),
	)
}

func reasoningEffortRank(raw string) (int, bool) {
	switch NormalizeMaxReasoningEffort(raw) {
	case "minimal":
		return 1, true
	case "low":
		return 2, true
	case "medium":
		return 3, true
	case "high":
		return 4, true
	case "xhigh":
		return 5, true
	case "max":
		return 6, true
	default:
		return 0, false
	}
}

// NormalizeReasoningEffortMappings validates group mapping rules against the
// fixed effort values supported by OpenAI groups.
func NormalizeReasoningEffortMappings(platform string, raw []ReasoningEffortMapping) ([]ReasoningEffortMapping, error) {
	if len(raw) > maxReasoningEffortMappings {
		return nil, fmt.Errorf("reasoning effort mappings cannot exceed %d entries", maxReasoningEffortMappings)
	}

	normalized := make([]ReasoningEffortMapping, 0, len(raw))
	seen := make(map[string]struct{}, len(raw))
	for i, mapping := range raw {
		from := NormalizeMaxReasoningEffort(mapping.From)
		to := NormalizeMaxReasoningEffort(mapping.To)
		if from == "" || to == "" {
			return nil, fmt.Errorf("reasoning effort mapping %d contains an empty or unknown value", i+1)
		}
		if len(from) > maxReasoningEffortValueLen || len(to) > maxReasoningEffortValueLen {
			return nil, fmt.Errorf("reasoning effort mapping %d values cannot exceed %d characters", i+1, maxReasoningEffortValueLen)
		}
		if _, err := normalizeMaxReasoningEffortForPlatform(platform, from); err != nil {
			return nil, fmt.Errorf("reasoning effort mapping %d source: %w", i+1, err)
		}
		if _, err := normalizeMaxReasoningEffortForPlatform(platform, to); err != nil {
			return nil, fmt.Errorf("reasoning effort mapping %d target: %w", i+1, err)
		}
		key := from
		if _, exists := seen[key]; exists {
			return nil, fmt.Errorf("duplicate reasoning effort mapping source %q", from)
		}
		seen[key] = struct{}{}
		normalized = append(normalized, ReasoningEffortMapping{From: from, To: to})
	}
	return normalized, nil
}

// NormalizeModelReasoningEffortRules validates exact model overrides. A rule
// with an empty ceiling and empty mappings is intentional: it disables the
// group default policy for that model.
func NormalizeModelReasoningEffortRules(platform string, raw []ModelReasoningEffortRule) ([]ModelReasoningEffortRule, error) {
	if len(raw) > maxModelReasoningEffortRules {
		return nil, fmt.Errorf("model reasoning effort rules cannot exceed %d entries", maxModelReasoningEffortRules)
	}
	if len(raw) > 0 && platform != PlatformOpenAI {
		return nil, fmt.Errorf("model reasoning effort rules are only supported for platform %q", PlatformOpenAI)
	}

	normalized := make([]ModelReasoningEffortRule, 0, len(raw))
	seen := make(map[string]struct{}, len(raw))
	for i, rule := range raw {
		model := strings.TrimSpace(rule.Model)
		if model == "" {
			return nil, fmt.Errorf("model reasoning effort rule %d has an empty model", i+1)
		}
		if len(model) > maxReasoningEffortModelLen {
			return nil, fmt.Errorf("model reasoning effort rule %d model cannot exceed %d characters", i+1, maxReasoningEffortModelLen)
		}
		if _, exists := seen[model]; exists {
			return nil, fmt.Errorf("duplicate model reasoning effort rule for model %q", model)
		}
		seen[model] = struct{}{}

		maxEffort, err := normalizeMaxReasoningEffortForPlatform(platform, rule.MaxReasoningEffort)
		if err != nil {
			return nil, fmt.Errorf("model reasoning effort rule %d ceiling: %w", i+1, err)
		}
		mappings, err := NormalizeReasoningEffortMappings(platform, rule.ReasoningEffortMappings)
		if err != nil {
			return nil, fmt.Errorf("model reasoning effort rule %d mappings: %w", i+1, err)
		}
		normalized = append(normalized, ModelReasoningEffortRule{
			Model:                   model,
			MaxReasoningEffort:      maxEffort,
			ReasoningEffortMappings: mappings,
		})
	}
	return normalized, nil
}

func mapReasoningEffort(raw string, mappings []ReasoningEffortMapping) (string, bool) {
	value := strings.TrimSpace(raw)
	canonical := NormalizeMaxReasoningEffort(value)
	for _, mapping := range mappings {
		if canonical != "" && canonical == NormalizeMaxReasoningEffort(mapping.From) {
			return strings.TrimSpace(mapping.To), true
		}
	}
	return value, false
}

func sanitizeGroupReasoningEffortPolicy(group *Group) {
	if group == nil {
		return
	}
	maxEffort, maxErr := normalizeMaxReasoningEffortForPlatform(group.Platform, group.MaxReasoningEffort)
	mappings, mappingsErr := NormalizeReasoningEffortMappings(group.Platform, group.ReasoningEffortMappings)
	modelRules, modelRulesErr := NormalizeModelReasoningEffortRules(group.Platform, group.ModelReasoningEffortRules)
	if maxErr != nil {
		maxEffort = ""
	}
	if mappingsErr != nil {
		mappings = []ReasoningEffortMapping{}
	}
	if modelRulesErr != nil {
		modelRules = []ModelReasoningEffortRule{}
	}
	group.MaxReasoningEffort = maxEffort
	group.ReasoningEffortMappings = mappings
	group.ModelReasoningEffortRules = modelRules
}

func reasoningEffortPolicyForModel(
	model string,
	defaultMaxEffort string,
	defaultMappings []ReasoningEffortMapping,
	modelRules []ModelReasoningEffortRule,
) (string, []ReasoningEffortMapping) {
	model = strings.TrimSpace(model)
	for _, rule := range modelRules {
		if strings.TrimSpace(rule.Model) == model {
			return rule.MaxReasoningEffort, rule.ReasoningEffortMappings
		}
	}
	return defaultMaxEffort, defaultMappings
}

// ApplyOpenAIReasoningEffortPolicyForModel selects an exact model override
// before applying the policy. The model is the client-requested model before
// account or upstream model mapping.
func ApplyOpenAIReasoningEffortPolicyForModel(
	body []byte,
	model string,
	defaultMaxEffort string,
	defaultMappings []ReasoningEffortMapping,
	modelRules []ModelReasoningEffortRule,
) ([]byte, bool) {
	maxEffort, mappings := reasoningEffortPolicyForModel(model, defaultMaxEffort, defaultMappings, modelRules)
	return ApplyOpenAIReasoningEffortPolicy(body, maxEffort, mappings)
}

// ApplyOpenAIReasoningEffortPolicy applies one exact mapping and then caps
// known effort levels. Omitted values remain untouched so upstream defaults
// stay in control.
func ApplyOpenAIReasoningEffortPolicy(body []byte, maxEffort string, mappings []ReasoningEffortMapping) ([]byte, bool) {
	maxRank, hasMax := reasoningEffortRank(maxEffort)
	if len(body) == 0 || (!hasMax && len(mappings) == 0) {
		return body, false
	}

	result := body
	changed := false
	for _, path := range []string{"reasoning.effort", "reasoning_effort"} {
		field := gjson.GetBytes(result, path)
		if !field.Exists() || field.Type != gjson.String {
			continue
		}
		original := strings.TrimSpace(field.String())
		if original == "" {
			continue
		}

		effective, _ := mapReasoningEffort(original, mappings)
		if currentRank, recognized := reasoningEffortRank(effective); recognized {
			effective = NormalizeMaxReasoningEffort(effective)
			if hasMax && currentRank > maxRank {
				effective = NormalizeMaxReasoningEffort(maxEffort)
			}
		}
		if effective == original {
			continue
		}

		updated, err := sjson.SetBytes(result, path, effective)
		if err != nil {
			continue
		}
		result = updated
		changed = true
	}
	return result, changed
}
