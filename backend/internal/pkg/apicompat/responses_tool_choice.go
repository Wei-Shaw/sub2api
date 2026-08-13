package apicompat

import (
	"encoding/json"
	"strings"
)

// NormalizeResponsesFunctionToolChoice converts the Chat Completions function
// choice shape into the Responses API shape:
//
//	{"type":"function","function":{"name":"get_weather"}}
//
// becomes:
//
//	{"type":"function","name":"get_weather"}
//
// Responses-native choices and non-function choices are returned byte-for-byte
// unchanged. Unknown top-level fields are preserved when a legacy choice is
// normalized so provider-specific extensions are not discarded.
func NormalizeResponsesFunctionToolChoice(raw json.RawMessage) (json.RawMessage, bool) {
	var choice map[string]json.RawMessage
	if err := json.Unmarshal(raw, &choice); err != nil {
		return raw, false
	}

	choiceType, ok := responsesToolChoiceString(choice["type"])
	if !ok || choiceType != "function" {
		return raw, false
	}
	legacyFunction, hasLegacyFunction := choice["function"]
	if !hasLegacyFunction {
		return raw, false
	}

	name, hasName := responsesToolChoiceString(choice["name"])
	if !hasName || strings.TrimSpace(name) == "" {
		var function map[string]json.RawMessage
		if err := json.Unmarshal(legacyFunction, &function); err != nil {
			return raw, false
		}
		name, hasName = responsesToolChoiceString(function["name"])
		if !hasName || strings.TrimSpace(name) == "" {
			return raw, false
		}
	}

	encodedName, err := json.Marshal(name)
	if err != nil {
		return raw, false
	}
	choice["name"] = encodedName
	delete(choice, "function")

	normalized, err := json.Marshal(choice)
	if err != nil {
		return raw, false
	}
	return normalized, true
}

func responsesToolChoiceString(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 {
		return "", false
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", false
	}
	return value, true
}
