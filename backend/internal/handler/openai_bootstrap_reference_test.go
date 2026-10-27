package handler

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
)

// Frozen implementation from upstream 5fc0e486c, kept only as a differential
// oracle and allocation benchmark baseline.
func referenceCodexCallOutputBootstrap(body []byte, isCandidate func(map[string]any) bool, allowHistoricalContext bool) ([]byte, bool) {
	if !referenceHasUniqueJSONMembers(body) {
		return body, false
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var request map[string]any
	if err := decoder.Decode(&request); err != nil {
		return body, false
	}
	if previousResponseID, exists := request["previous_response_id"]; exists {
		value, ok := previousResponseID.(string)
		if !ok || (!allowHistoricalContext && strings.TrimSpace(value) != "") {
			return body, false
		}
	}
	input, ok := request["input"].([]any)
	if !ok {
		return body, false
	}

	// Responses built-ins follow the *_call / *_call_output naming convention,
	// so classify by the wire type shape instead of maintaining an incomplete
	// allowlist. Delegation may coexist with historical anchors only when their
	// IDs make them unambiguous; automation retains the bootstrap-only boundary.
	for _, raw := range input {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		typ := stringField(item, "type")
		if isCandidate(item) {
			callIDValue, exists := item["call_id"]
			callID, isString := callIDValue.(string)
			if exists && (!isString || strings.TrimSpace(callID) != "") {
				return body, false
			}
			continue
		}
		if typ == "item_reference" {
			if allowHistoricalContext && strings.TrimSpace(stringField(item, "id")) != "" {
				continue
			}
			return body, false
		}
		if strings.HasSuffix(typ, "_call") || isResponsesCallOutputType(typ) {
			if allowHistoricalContext && strings.TrimSpace(stringField(item, "call_id")) != "" {
				continue
			}
			return body, false
		}
	}

	changed := false
	for i, raw := range input {
		item, ok := raw.(map[string]any)
		if !ok || !isCandidate(item) {
			continue
		}
		output, ok := item["output"].(string)
		if !ok {
			continue
		}
		input[i] = map[string]any{
			"type": "message",
			"role": "user",
			"content": []any{map[string]any{
				"type": "input_text",
				"text": output,
			}},
		}
		changed = true
	}
	if !changed {
		return body, false
	}
	normalized, err := json.Marshal(request)
	if err != nil {
		return body, false
	}
	return normalized, true
}

func referenceHasUniqueJSONMembers(body []byte) bool {
	decoder := json.NewDecoder(bytes.NewReader(body))
	if !referenceConsumeUniqueJSONValue(decoder) {
		return false
	}
	_, err := decoder.Token()
	return err == io.EOF
}

func referenceConsumeUniqueJSONValue(decoder *json.Decoder) bool {
	token, err := decoder.Token()
	if err != nil {
		return false
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return true
	}

	switch delim {
	case '{':
		members := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return false
			}
			key, ok := keyToken.(string)
			if !ok {
				return false
			}
			if _, duplicate := members[key]; duplicate {
				return false
			}
			members[key] = struct{}{}
			if !referenceConsumeUniqueJSONValue(decoder) {
				return false
			}
		}
		end, err := decoder.Token()
		return err == nil && end == json.Delim('}')
	case '[':
		for decoder.More() {
			if !referenceConsumeUniqueJSONValue(decoder) {
				return false
			}
		}
		end, err := decoder.Token()
		return err == nil && end == json.Delim(']')
	default:
		return false
	}
}

func isCodexDelegationCandidate(item map[string]any) bool {
	if stringField(item, "type") != "function_call_output" ||
		!isCodexDelegationTool(stringField(item, "namespace"), stringField(item, "name")) {
		return false
	}
	output, ok := item["output"].(string)
	return ok && validCodexDelegationEnvelope(output)
}

func isCodexAutomationCandidate(item map[string]any) bool {
	if stringField(item, "type") != "function_call_output" ||
		stringField(item, "namespace") != "codex_app" ||
		stringField(item, "name") != "automation_update" {
		return false
	}
	output, ok := item["output"].(string)
	return ok && (validCodexAutomationBootstrap(output) || validCodexAutomationHeartbeat(output))
}

func stringField(item map[string]any, key string) string {
	value, _ := item[key].(string)
	return value
}
