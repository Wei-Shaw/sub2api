package service

import (
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

var openAIModelEffortSuffixes = []string{"minimal", "medium", "xhigh", "none", "high", "low", "max"}

// Models whose terminal token is part of the catalog ID, not an effort alias.
// Keep this closed list for IDs not exposed by the package catalogs below.
var openAIModelEffortSuffixExemptions = map[string]struct{}{
	"gemini-3.6-flash-high":   {},
	"gemini-3.6-flash-low":    {},
	"gemini-3.6-flash-medium": {},
}

// normalizeOpenAIModelEffortSuffixForUpstream rewrites only the payload sent to
// the already-selected account. Routing and billing continue to use the client model.
func normalizeOpenAIModelEffortSuffixForUpstream(body []byte, account *Account, responsesAPI bool) ([]byte, string, bool, error) {
	modelResult := gjson.GetBytes(body, "model")
	if !modelResult.Exists() || modelResult.Type != gjson.String {
		return body, "", false, nil
	}
	model := strings.TrimSpace(modelResult.String())
	base, effort, ok := splitOpenAIModelEffortSuffix(model)
	if !ok || isExplicitOpenAIModelID(account, model) {
		return body, model, false, nil
	}

	upstreamModel := base
	if account != nil {
		upstreamModel = account.GetMappedModel(base)
	}
	normalized, err := sjson.SetBytes(body, "model", upstreamModel)
	if err != nil {
		return body, model, false, fmt.Errorf("strip OpenAI model effort suffix: %w", err)
	}

	path := "reasoning_effort"
	if responsesAPI {
		path = "reasoning.effort"
	}
	if !hasExplicitOpenAIEffort(body, path) {
		normalized, err = sjson.SetBytes(normalized, path, effort)
		if err != nil {
			return body, model, false, fmt.Errorf("set OpenAI model suffix effort: %w", err)
		}
	}
	return normalized, upstreamModel, true, nil
}

func splitOpenAIModelEffortSuffix(model string) (string, string, bool) {
	lower := strings.ToLower(model)
	for _, effort := range openAIModelEffortSuffixes {
		suffix := "-" + effort
		if strings.HasSuffix(lower, suffix) && len(model) > len(suffix) {
			return model[:len(model)-len(suffix)], effort, true
		}
	}
	return model, "", false
}

func isExplicitOpenAIModelID(account *Account, model string) bool {
	if account != nil {
		if _, matched := account.ResolveMappedModel(model); matched {
			return true
		}
	}
	if _, ok := codexModelMap[model]; ok {
		return true
	}
	if _, ok := domain.DefaultAntigravityModelMapping[model]; ok {
		return true
	}
	if _, ok := openAIModelEffortSuffixExemptions[model]; ok {
		return true
	}
	for _, catalogModel := range antigravity.DefaultModels() {
		if catalogModel.ID == model {
			return true
		}
	}
	return false
}

func hasExplicitOpenAIEffort(body []byte, path string) bool {
	effort := gjson.GetBytes(body, path)
	return effort.Type == gjson.String && strings.TrimSpace(effort.String()) != ""
}
