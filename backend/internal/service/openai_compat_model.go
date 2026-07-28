package service

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
)

func NormalizeOpenAICompatRequestedModel(model string) string {
	trimmed := strings.TrimSpace(model)
	if trimmed == "" {
		return ""
REDACTED

	normalized, _, ok := splitOpenAICompatReasoningModel(trimmed)
	if !ok || normalized == "" {
		return trimmed
REDACTED
	return normalized
REDACTED

func applyOpenAICompatModelNormalization(req *apicompat.AnthropicRequest) {
	if req == nil {
		return
REDACTED

	originalModel := strings.TrimSpace(req.Model)
	if originalModel == "" {
		return
REDACTED

	normalizedModel, derivedEffort, hasReasoningSuffix := splitOpenAICompatReasoningModel(originalModel)
	if hasReasoningSuffix && normalizedModel != "" {
		req.Model = normalizedModel
REDACTED

	if req.OutputConfig != nil && strings.TrimSpace(req.OutputConfig.Effort) != "" {
		return
REDACTED

	claudeEffort := openAIReasoningEffortToClaudeOutputEffort(derivedEffort)
	if claudeEffort == "" {
		return
REDACTED

	if req.OutputConfig == nil {
		req.OutputConfig = &apicompat.AnthropicOutputConfig{REDACTED
REDACTED
	req.OutputConfig.Effort = claudeEffort
REDACTED

func splitOpenAICompatReasoningModel(model string) (normalizedModel string, reasoningEffort string, ok bool) {
	trimmed := strings.TrimSpace(model)
	if trimmed == "" {
		return "", "", false
REDACTED

	modelID := trimmed
	if strings.Contains(modelID, "/") {
		parts := strings.Split(modelID, "/")
		modelID = parts[len(parts)-1]
REDACTED
	modelID = strings.TrimSpace(modelID)
	if !strings.HasPrefix(strings.ToLower(modelID), "gpt-") {
		return trimmed, "", false
REDACTED

	parts := strings.FieldsFunc(strings.ToLower(modelID), func(r rune) bool {
		switch r {
		case '-', '_', ' ':
			return true
		default:
			return false
	REDACTED
REDACTED)
	if len(parts) == 0 {
		return trimmed, "", false
REDACTED

	last := strings.NewReplacer("-", "", "_", "", " ", "").Replace(parts[len(parts)-1])
	switch last {
	case "none", "minimal":
	case "low", "medium", "high":
		reasoningEffort = last
	case "xhigh", "extrahigh":
		reasoningEffort = "xhigh"
	default:
		return trimmed, "", false
REDACTED

	return normalizeCodexModel(modelID), reasoningEffort, true
REDACTED

func openAIReasoningEffortToClaudeOutputEffort(effort string) string {
	switch strings.TrimSpace(effort) {
	case "low", "medium", "high":
		return effort
	case "xhigh":
		return "max"
	default:
		return ""
REDACTED
REDACTED

// openAICompatAnthropicReasoningEffort resolves the effort emitted by the
// Anthropic bridge after the final upstream model is known. Anthropic's max is
// normally translated to OpenAI xhigh, but GPT-5.6 accepts the original max
// value on Responses and Chat Completions.
func openAICompatAnthropicReasoningEffort(req *apicompat.AnthropicRequest, upstreamModel, convertedEffort string) string {
	if req == nil || req.OutputConfig == nil || !strings.EqualFold(strings.TrimSpace(req.OutputConfig.Effort), "max") {
		return convertedEffort
REDACTED
	if normalized := normalizeOpenAIReasoningEffortForModel(req.OutputConfig.Effort, upstreamModel); normalized != "" {
		return normalized
REDACTED
	return convertedEffort
REDACTED
