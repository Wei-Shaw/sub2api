package service

import (
	"fmt"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

var openAIModelEffortSuffixes = []string{"minimal", "medium", "xhigh", "none", "high", "low", "max"}

// NormalizeOpenAIModelEffortSuffix strips a recognized terminal effort suffix.
// Explicit request effort remains authoritative.
func NormalizeOpenAIModelEffortSuffix(body []byte, responsesAPI bool) ([]byte, bool, error) {
	modelResult := gjson.GetBytes(body, "model")
	if !modelResult.Exists() || modelResult.Type != gjson.String {
		return body, false, nil
	}
	model := modelResult.String()
	lower := strings.ToLower(model)
	base, effort := "", ""
	for _, candidate := range openAIModelEffortSuffixes {
		suffix := "-" + candidate
		if strings.HasSuffix(lower, suffix) && len(model) > len(suffix) {
			base, effort = model[:len(model)-len(suffix)], candidate
			break
		}
	}
	if base == "" {
		return body, false, nil
	}
	normalized, err := sjson.SetBytes(body, "model", base)
	if err != nil {
		return body, false, fmt.Errorf("strip OpenAI model effort suffix: %w", err)
	}
	path := "reasoning_effort"
	if responsesAPI {
		path = "reasoning.effort"
	} else if hasExplicitOpenAIEffort(body, "reasoning.effort") {
		return normalized, true, nil
	}
	if hasExplicitOpenAIEffort(body, path) {
		return normalized, true, nil
	}
	normalized, err = sjson.SetBytes(normalized, path, effort)
	if err != nil {
		return body, false, fmt.Errorf("set OpenAI model suffix effort: %w", err)
	}
	return normalized, true, nil
}

func hasExplicitOpenAIEffort(body []byte, path string) bool {
	effort := gjson.GetBytes(body, path)
	return effort.Type == gjson.String && strings.TrimSpace(effort.String()) != ""
}
