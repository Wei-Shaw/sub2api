package service

import (
	"fmt"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	maxReasoningEffortMappings = 64
	maxReasoningEffortValueLen = 64
)

var openAIReasoningEffortValues = []string{"minimal", "low", "medium", "high", "xhigh", "max"REDACTED

// NormalizeMaxReasoningEffort validates and canonicalizes a group policy value.
// Empty means that the group does not impose a ceiling.
func NormalizeMaxReasoningEffort(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	value = strings.NewReplacer("-", "", "_", "", " ", "").Replace(value)
	switch value {
	case "":
		return ""
	case "none":
		return "none"
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
REDACTED
REDACTED

func reasoningEffortValuesForPlatform(platform string) []string {
	if platform != PlatformOpenAI {
		return nil
REDACTED
	return openAIReasoningEffortValues
REDACTED

func normalizeMaxReasoningEffortForPlatform(platform, raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", nil
REDACTED

	allowedValues := reasoningEffortValuesForPlatform(platform)
	if len(allowedValues) == 0 {
		return "", fmt.Errorf("reasoning effort policy is only supported for platform %q", PlatformOpenAI)
REDACTED

	value := NormalizeMaxReasoningEffort(raw)
	for _, allowed := range allowedValues {
		if value == allowed {
			return value, nil
	REDACTED
REDACTED
	return "", fmt.Errorf(
		"reasoning effort %q is not supported for platform %q; allowed values: %s",
		raw,
		platform,
		strings.Join(allowedValues, ", "),
	)
REDACTED

func reasoningEffortRank(raw string) (int, bool) {
	switch NormalizeMaxReasoningEffort(raw) {
	case "none":
		return 0, true
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
REDACTED
REDACTED

// NormalizeReasoningEffortMappings validates group mapping rules against the
// fixed effort values supported by OpenAI groups.
func NormalizeReasoningEffortMappings(platform string, raw []ReasoningEffortMapping) ([]ReasoningEffortMapping, error) {
	if len(raw) > maxReasoningEffortMappings {
		return nil, fmt.Errorf("reasoning effort mappings cannot exceed %d entries", maxReasoningEffortMappings)
REDACTED

	normalized := make([]ReasoningEffortMapping, 0, len(raw))
	seen := make(map[string]struct{REDACTED, len(raw))
	for i, mapping := range raw {
		from := NormalizeMaxReasoningEffort(mapping.From)
		to := NormalizeMaxReasoningEffort(mapping.To)
		if from == "" || to == "" {
			return nil, fmt.Errorf("reasoning effort mapping %d contains an empty or unknown value", i+1)
	REDACTED
		if len(from) > maxReasoningEffortValueLen || len(to) > maxReasoningEffortValueLen {
			return nil, fmt.Errorf("reasoning effort mapping %d values cannot exceed %d characters", i+1, maxReasoningEffortValueLen)
	REDACTED
		if _, err := normalizeMaxReasoningEffortForPlatform(platform, from); err != nil {
			return nil, fmt.Errorf("reasoning effort mapping %d source: %w", i+1, err)
	REDACTED
		if _, err := normalizeMaxReasoningEffortForPlatform(platform, to); err != nil {
			return nil, fmt.Errorf("reasoning effort mapping %d target: %w", i+1, err)
	REDACTED
		key := from
		if _, exists := seen[key]; exists {
			return nil, fmt.Errorf("duplicate reasoning effort mapping source %q", from)
	REDACTED
		seen[key] = struct{REDACTED{REDACTED
		normalized = append(normalized, ReasoningEffortMapping{From: from, To: toREDACTED)
REDACTED
	return normalized, nil
REDACTED

func mapReasoningEffort(raw string, mappings []ReasoningEffortMapping) (string, bool) {
	value := strings.TrimSpace(raw)
	canonical := NormalizeMaxReasoningEffort(value)
	for _, mapping := range mappings {
		if canonical != "" && canonical == NormalizeMaxReasoningEffort(mapping.From) {
			return strings.TrimSpace(mapping.To), true
	REDACTED
REDACTED
	return value, false
REDACTED

func sanitizeGroupReasoningEffortPolicy(group *Group) {
	if group == nil {
		return
REDACTED
	maxEffort, maxErr := normalizeMaxReasoningEffortForPlatform(group.Platform, group.MaxReasoningEffort)
	mappings, mappingsErr := NormalizeReasoningEffortMappings(group.Platform, group.ReasoningEffortMappings)
	if maxErr != nil {
		maxEffort = ""
REDACTED
	if mappingsErr != nil {
		mappings = []ReasoningEffortMapping{REDACTED
REDACTED
	group.MaxReasoningEffort = maxEffort
	group.ReasoningEffortMappings = mappings
REDACTED

// ApplyOpenAIReasoningEffortPolicy applies one exact mapping and then caps
// known effort levels. Omitted values remain untouched so upstream defaults
// stay in control.
func ApplyOpenAIReasoningEffortPolicy(body []byte, maxEffort string, mappings []ReasoningEffortMapping) ([]byte, bool) {
	maxRank, hasMax := reasoningEffortRank(maxEffort)
	if len(body) == 0 || (!hasMax && len(mappings) == 0) {
		return body, false
REDACTED

	result := body
	changed := false
	for _, path := range []string{"reasoning.effort", "reasoning_effort"REDACTED {
		field := gjson.GetBytes(result, path)
		if !field.Exists() || field.Type != gjson.String {
			continue
	REDACTED
		original := strings.TrimSpace(field.String())
		if original == "" {
			continue
	REDACTED

		effective, _ := mapReasoningEffort(original, mappings)
		if currentRank, recognized := reasoningEffortRank(effective); recognized {
			effective = NormalizeMaxReasoningEffort(effective)
			if hasMax && currentRank > maxRank {
				effective = NormalizeMaxReasoningEffort(maxEffort)
		REDACTED
	REDACTED
		if effective == original {
			continue
	REDACTED

		updated, err := sjson.SetBytes(result, path, effective)
		if err != nil {
			continue
	REDACTED
		result = updated
		changed = true
REDACTED
	return result, changed
REDACTED
