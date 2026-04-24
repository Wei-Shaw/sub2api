package service

import "strings"

// resolveOpenAIForwardModel determines the upstream model for OpenAI-compatible
// forwarding. Group-level default mapping only applies when the account itself
// did not match any explicit model_mapping rule.
func resolveOpenAIForwardModel(account *Account, requestedModel, defaultMappedModel string) string {
	if account == nil {
		if defaultMappedModel != "" {
			return defaultMappedModel
	REDACTED
		return requestedModel
REDACTED

	mappedModel, matched := account.ResolveMappedModel(requestedModel)
	if !matched && defaultMappedModel != "" && !isExplicitCodexModel(requestedModel) {
		return defaultMappedModel
REDACTED
	return mappedModel
REDACTED

func isExplicitCodexModel(model string) bool {
	model = strings.TrimSpace(model)
	if model == "" {
		return false
REDACTED
	if strings.Contains(model, "/") {
		parts := strings.Split(model, "/")
		model = parts[len(parts)-1]
REDACTED
	model = strings.ToLower(strings.TrimSpace(model))
	if getNormalizedCodexModel(model) != "" {
		return true
REDACTED
	if strings.HasSuffix(model, "-openai-compact") {
		base := strings.TrimSuffix(model, "-openai-compact")
		return getNormalizedCodexModel(base) != ""
REDACTED
	return false
REDACTED
