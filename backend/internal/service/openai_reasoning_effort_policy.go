package service

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	maxReasoningEffortMappings = 64
	maxReasoningEffortValueLen = 64
)

var openAIReasoningEffortValues = []string{"minimal", "low", "medium", "high", "xhigh", "max"}

type openAIReasoningEffortPolicyContextKey struct{}

type reasoningEffortMappingKey struct {
	model string
	from  string
}

type openAIReasoningEffortPolicy struct {
	maxEffort string
	mappings  []ReasoningEffortMapping
	model     string
	// sourceEffort retains the explicit Anthropic Messages value from
	// output_config.effort. The Messages bridge can translate that value before
	// this policy reaches the resulting OpenAI request (notably max -> xhigh),
	// so matching against only the converted body would lose the configured
	// source rule.
	sourceEffort string
}

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
	if platform != PlatformOpenAI && platform != PlatformComposite {
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
		return "", fmt.Errorf(
			"reasoning effort policy is only supported for platforms %q and %q",
			PlatformOpenAI,
			PlatformComposite,
		)
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

// normalizeReasoningEffortMappingSource canonicalizes known OpenAI effort
// aliases while retaining forward-compatible custom client values. Internal
// whitespace is preserved because it can be meaningful to an upstream; only
// leading/trailing whitespace and ASCII case are normalized.
func normalizeReasoningEffortMappingSource(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	if canonical := NormalizeMaxReasoningEffort(value); canonical != "" {
		return canonical
	}
	return value
}

func normalizeReasoningEffortMappingModel(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

// NormalizeReasoningEffortMappings validates group mapping rules. Sources may
// be custom client values; targets remain fixed values supported by OpenAI
// routes.
func NormalizeReasoningEffortMappings(platform string, raw []ReasoningEffortMapping) ([]ReasoningEffortMapping, error) {
	if len(raw) > maxReasoningEffortMappings {
		return nil, fmt.Errorf("reasoning effort mappings cannot exceed %d entries", maxReasoningEffortMappings)
	}

	normalized := make([]ReasoningEffortMapping, 0, len(raw))
	seen := make(map[reasoningEffortMappingKey]struct{}, len(raw))
	for i, mapping := range raw {
		model := normalizeReasoningEffortMappingModel(mapping.Model)
		from := normalizeReasoningEffortMappingSource(mapping.From)
		to := NormalizeMaxReasoningEffort(mapping.To)
		if from == "" || from == "none" {
			return nil, fmt.Errorf("reasoning effort mapping %d contains an empty or unknown value", i+1)
		}
		if to == "" {
			return nil, fmt.Errorf("reasoning effort mapping %d target contains an empty or unknown value", i+1)
		}
		if utf8.RuneCountInString(model) > maxReasoningEffortValueLen || utf8.RuneCountInString(from) > maxReasoningEffortValueLen || utf8.RuneCountInString(to) > maxReasoningEffortValueLen {
			return nil, fmt.Errorf("reasoning effort mapping %d values cannot exceed %d characters", i+1, maxReasoningEffortValueLen)
		}
		// Mapping sources intentionally allow custom future/upstream values. The
		// target remains restricted to values the OpenAI routes understand.
		if len(reasoningEffortValuesForPlatform(platform)) == 0 {
			return nil, fmt.Errorf("reasoning effort mapping %d source: reasoning effort policy is only supported for platforms %q and %q", i+1, PlatformOpenAI, PlatformComposite)
		}
		if _, err := normalizeMaxReasoningEffortForPlatform(platform, to); err != nil {
			return nil, fmt.Errorf("reasoning effort mapping %d target: %w", i+1, err)
		}
		key := reasoningEffortMappingKey{model: model, from: from}
		if _, exists := seen[key]; exists {
			return nil, fmt.Errorf("duplicate reasoning effort mapping source %q for model %q", from, model)
		}
		seen[key] = struct{}{}
		normalized = append(normalized, ReasoningEffortMapping{Model: model, From: from, To: to})
	}
	return normalized, nil
}

// WithOpenAIReasoningEffortPolicy binds a group policy to a request after its
// concrete target platform has been resolved to OpenAI. The policy is copied so
// retries and asynchronous forwarding cannot observe later slice mutations.
func WithOpenAIReasoningEffortPolicy(ctx context.Context, maxEffort string, mappings []ReasoningEffortMapping, model ...string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	policy := openAIReasoningEffortPolicy{
		maxEffort: maxEffort,
		mappings:  append([]ReasoningEffortMapping(nil), mappings...),
	}
	if len(model) > 0 {
		policy.model = strings.TrimSpace(model[0])
	}
	return context.WithValue(ctx, openAIReasoningEffortPolicyContextKey{}, policy)
}

// WithOpenAIMessagesReasoningEffortPolicy binds a policy for an Anthropic
// Messages request with an explicit output_config.effort value. Keep this
// separate from WithOpenAIReasoningEffortPolicy so ordinary OpenAI requests
// continue to take their mapping source from their own body.
func WithOpenAIMessagesReasoningEffortPolicy(ctx context.Context, maxEffort string, mappings []ReasoningEffortMapping, model, sourceEffort string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	policy := openAIReasoningEffortPolicy{
		maxEffort:    maxEffort,
		mappings:     append([]ReasoningEffortMapping(nil), mappings...),
		model:        strings.TrimSpace(model),
		sourceEffort: strings.TrimSpace(sourceEffort),
	}
	return context.WithValue(ctx, openAIReasoningEffortPolicyContextKey{}, policy)
}

// ApplyOpenAIReasoningEffortPolicyFromContext applies a policy previously bound
// to the request. An unbound request is returned byte-for-byte unchanged.
func ApplyOpenAIReasoningEffortPolicyFromContext(ctx context.Context, body []byte) ([]byte, bool) {
	if ctx == nil {
		return body, false
	}
	policy, ok := ctx.Value(openAIReasoningEffortPolicyContextKey{}).(openAIReasoningEffortPolicy)
	if !ok {
		return body, false
	}
	model := policy.model
	if strings.TrimSpace(model) == "" {
		model = strings.TrimSpace(gjson.GetBytes(body, "model").String())
	}
	return applyOpenAIReasoningEffortPolicyForModelAndSource(body, model, policy.sourceEffort, policy.maxEffort, policy.mappings)
}

func mapReasoningEffort(raw, model string, mappings []ReasoningEffortMapping) (string, bool) {
	value := strings.TrimSpace(raw)
	canonical := normalizeReasoningEffortMappingSource(value)
	model = normalizeReasoningEffortMappingModel(model)
	for _, scoped := range []bool{true, false} {
		for _, mapping := range mappings {
			mappingModel := normalizeReasoningEffortMappingModel(mapping.Model)
			if scoped {
				if model == "" || mappingModel == "" || mappingModel != model {
					continue
				}
			} else if mappingModel != "" {
				continue
			}
			if canonical == normalizeReasoningEffortMappingSource(mapping.From) {
				return strings.TrimSpace(mapping.To), true
			}
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
	if maxErr != nil {
		maxEffort = ""
	}
	if mappingsErr != nil {
		mappings = []ReasoningEffortMapping{}
	}
	group.MaxReasoningEffort = maxEffort
	group.ReasoningEffortMappings = mappings
}

// ApplyOpenAIReasoningEffortPolicy applies one exact mapping and then caps
// known effort levels. Omitted values remain untouched so upstream defaults
// stay in control.
func ApplyOpenAIReasoningEffortPolicy(body []byte, maxEffort string, mappings []ReasoningEffortMapping) ([]byte, bool) {
	return ApplyOpenAIReasoningEffortPolicyForModel(body, strings.TrimSpace(gjson.GetBytes(body, "model").String()), maxEffort, mappings)
}

// ApplyOpenAIReasoningEffortPolicyForModel applies the group policy using the
// client-requested model. Supplying it separately is required for converted
// Messages requests and WebSocket follow-up turns that omit the model field.
func ApplyOpenAIReasoningEffortPolicyForModel(body []byte, model, maxEffort string, mappings []ReasoningEffortMapping) ([]byte, bool) {
	return applyOpenAIReasoningEffortPolicyForModelAndSource(body, model, "", maxEffort, mappings)
}

func applyOpenAIReasoningEffortPolicyForModelAndSource(body []byte, model, sourceEffort, maxEffort string, mappings []ReasoningEffortMapping) ([]byte, bool) {
	maxRank, hasMax := reasoningEffortRank(maxEffort)
	if len(body) == 0 || (!hasMax && len(mappings) == 0) {
		return body, false
	}
	if strings.TrimSpace(model) == "" {
		model = strings.TrimSpace(gjson.GetBytes(body, "model").String())
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

		mappingSource := original
		if strings.TrimSpace(sourceEffort) != "" {
			mappingSource = sourceEffort
		}
		effective, mapped := mapReasoningEffort(mappingSource, model, mappings)
		if !mapped {
			// A source captured from a Messages request is only used to select a
			// configured mapping. If it does not match, preserve the effort that
			// the protocol bridge already produced for this upstream model.
			effective = original
		}
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
