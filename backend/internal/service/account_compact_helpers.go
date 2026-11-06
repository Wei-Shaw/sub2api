package service

import (
	"strings"
)

const (
	// OpenAICompactModeAuto follows compact-probe results when deciding compact eligibility.
	OpenAICompactModeAuto = "auto"
	// OpenAICompactModeForceOn always treats the account as compact-supported.
	OpenAICompactModeForceOn = "force_on"
	// OpenAICompactModeForceOff always treats the account as compact-unsupported.
	OpenAICompactModeForceOff = "force_off"
)

func normalizeOpenAICompactMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case OpenAICompactModeForceOn:
		return OpenAICompactModeForceOn
	case OpenAICompactModeForceOff:
		return OpenAICompactModeForceOff
	default:
		return OpenAICompactModeAuto
	}
}

func stringMappingFromRaw(raw any) map[string]string {
	switch mapping := raw.(type) {
	case map[string]any:
		if len(mapping) == 0 {
			return nil
		}
		result := make(map[string]string, len(mapping))
		for key, value := range mapping {
			if str, ok := value.(string); ok {
				result[key] = str
			}
		}
		if len(result) == 0 {
			return nil
		}
		return result
	case map[string]string:
		if len(mapping) == 0 {
			return nil
		}
		result := make(map[string]string, len(mapping))
		for key, value := range mapping {
			result[key] = value
		}
		return result
	default:
		return nil
	}
}
