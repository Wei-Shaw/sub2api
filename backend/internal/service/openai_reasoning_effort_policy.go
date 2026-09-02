package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	maxReasoningEffortMappings = 64
	maxReasoningEffortValueLen = 64

	// ReasoningEffortOverLimitDowngrade rewrites values above the ceiling.
	ReasoningEffortOverLimitDowngrade = "downgrade"
	// ReasoningEffortOverLimitDeny rejects the request when the ceiling is exceeded.
	ReasoningEffortOverLimitDeny = "deny"
)

var openAIReasoningEffortValues = []string{"minimal", "low", "medium", "high", "xhigh", "max"}

type openAIReasoningEffortPolicyContextKey struct{}
type requestedReasoningEffortContextKey struct{}

type openAIReasoningEffortPolicy struct {
	maxEffort string
	overLimit string
	mappings  []ReasoningEffortMapping
}

// ReasoningEffortOverLimitError is returned when a group policy is set to deny
// requests whose explicit reasoning effort exceeds the ceiling.
type ReasoningEffortOverLimitError struct {
	Requested string
	Max       string
}

func (e *ReasoningEffortOverLimitError) Error() string {
	if e == nil {
		return "reasoning effort exceeds this group's limit"
	}
	requested := strings.TrimSpace(e.Requested)
	max := strings.TrimSpace(e.Max)
	if requested == "" && max == "" {
		return "reasoning effort exceeds this group's limit"
	}
	if requested == "" {
		return fmt.Sprintf("reasoning effort exceeds this group's limit of %q", max)
	}
	if max == "" {
		return fmt.Sprintf("reasoning effort %q exceeds this group's limit", requested)
	}
	return fmt.Sprintf("reasoning effort %q exceeds this group's limit of %q", requested, max)
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

// NormalizeMaxReasoningEffortOverLimit canonicalizes the over-limit action.
// Empty means the historical default: automatically downgrade to the ceiling.
func NormalizeMaxReasoningEffortOverLimit(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", ReasoningEffortOverLimitDowngrade:
		return ReasoningEffortOverLimitDowngrade
	case ReasoningEffortOverLimitDeny:
		return ReasoningEffortOverLimitDeny
	default:
		return ""
	}
}

func normalizeMaxReasoningEffortOverLimitForPlatform(platform, raw string) (string, error) {
	value := NormalizeMaxReasoningEffortOverLimit(raw)
	if value == "" {
		return "", fmt.Errorf(
			"reasoning effort over-limit action %q is not supported; allowed values: %s, %s",
			raw,
			ReasoningEffortOverLimitDowngrade,
			ReasoningEffortOverLimitDeny,
		)
	}
	if value == ReasoningEffortOverLimitDowngrade {
		return value, nil
	}
	if len(reasoningEffortValuesForPlatform(platform)) == 0 {
		return "", fmt.Errorf(
			"reasoning effort policy is only supported for platforms %q and %q",
			PlatformOpenAI,
			PlatformComposite,
		)
	}
	return value, nil
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
// fixed effort values supported by OpenAI routes.
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

// WithRequestedReasoningEffort stores the client-requested effort captured from
// the inbound body before group policy or model-family remapping.
func WithRequestedReasoningEffort(ctx context.Context, effort string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	effort = strings.TrimSpace(effort)
	if effort == "" {
		return ctx
	}
	return context.WithValue(ctx, requestedReasoningEffortContextKey{}, effort)
}

// RequestedReasoningEffortFromContext returns the inbound requested effort bound
// to ctx, or nil when none was captured.
func RequestedReasoningEffortFromContext(ctx context.Context) *string {
	if ctx == nil {
		return nil
	}
	value, ok := ctx.Value(requestedReasoningEffortContextKey{}).(string)
	if !ok {
		return nil
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

// WithOpenAIReasoningEffortPolicy binds a group policy to a request after its
// concrete target platform has been resolved to OpenAI. The policy is copied so
// retries and asynchronous forwarding cannot observe later slice mutations.
func WithOpenAIReasoningEffortPolicy(ctx context.Context, maxEffort string, mappings []ReasoningEffortMapping, overLimit string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	policy := openAIReasoningEffortPolicy{
		maxEffort: maxEffort,
		overLimit: overLimit,
		mappings:  append([]ReasoningEffortMapping(nil), mappings...),
	}
	return context.WithValue(ctx, openAIReasoningEffortPolicyContextKey{}, policy)
}

// ApplyOpenAIReasoningEffortPolicyFromContext applies a policy previously bound
// to the request. An unbound request is returned byte-for-byte unchanged.
func ApplyOpenAIReasoningEffortPolicyFromContext(ctx context.Context, body []byte) ([]byte, bool, error) {
	if ctx == nil {
		return body, false, nil
	}
	policy, ok := ctx.Value(openAIReasoningEffortPolicyContextKey{}).(openAIReasoningEffortPolicy)
	if !ok {
		return body, false, nil
	}
	return ApplyOpenAIReasoningEffortPolicy(body, policy.maxEffort, policy.mappings, policy.overLimit)
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
	overLimit := NormalizeMaxReasoningEffortOverLimit(group.MaxReasoningEffortOverLimit)
	if maxErr != nil {
		maxEffort = ""
	}
	if mappingsErr != nil {
		mappings = []ReasoningEffortMapping{}
	}
	if overLimit == "" || (group.Platform != PlatformOpenAI && group.Platform != PlatformComposite) {
		overLimit = ReasoningEffortOverLimitDowngrade
	}
	group.MaxReasoningEffort = maxEffort
	group.MaxReasoningEffortOverLimit = overLimit
	group.ReasoningEffortMappings = mappings
}

// ApplyOpenAIReasoningEffortPolicy applies one exact mapping and then either
// caps known effort levels or rejects the request when the group is configured
// to deny values above the ceiling. Omitted values remain untouched so
// upstream defaults stay in control.
func ApplyOpenAIReasoningEffortPolicy(body []byte, maxEffort string, mappings []ReasoningEffortMapping, overLimit string) ([]byte, bool, error) {
	maxRank, hasMax := reasoningEffortRank(maxEffort)
	if len(body) == 0 || (!hasMax && len(mappings) == 0) {
		return body, false, nil
	}
	deny := hasMax && NormalizeMaxReasoningEffortOverLimit(overLimit) == ReasoningEffortOverLimitDeny
	canonicalMax := NormalizeMaxReasoningEffort(maxEffort)

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
				if deny {
					return body, false, &ReasoningEffortOverLimitError{Requested: effective, Max: canonicalMax}
				}
				effective = canonicalMax
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
	return result, changed, nil
}

func applyOpenAIWSReasoningEffortPolicy(payload []byte, hooks *OpenAIWSIngressHooks) ([]byte, error) {
	if hooks == nil || (hooks.MaxReasoningEffort == "" && len(hooks.ReasoningEffortMappings) == 0) {
		return payload, nil
	}
	capped, changed, err := ApplyOpenAIReasoningEffortPolicy(payload, hooks.MaxReasoningEffort, hooks.ReasoningEffortMappings, hooks.MaxReasoningEffortOverLimit)
	if err != nil {
		return payload, err
	}
	if changed {
		return capped, nil
	}
	return payload, nil
}
