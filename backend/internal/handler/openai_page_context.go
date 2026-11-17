package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var codexSelectedPageName = regexp.MustCompile(`^codex_apps_selected_page_part_([1-9][0-9]*)_of_([1-9][0-9]*)$`)

// Codex injects external context as standalone outputs, without a model tool call.
// Use the legacy user-message representation for HTTP Responses interoperability.
func normalizeCodexUntrustedContext(body []byte) ([]byte, bool) {
	if !mayContainCodexCallOutputCandidate(body, "codex_apps_open_page", "codex_apps_selected_page_part_", "browser_info", "goal_objective", "untrusted_context") || !hasUniqueJSONMembers(body) {
		return body, false
	}
	var request map[string]json.RawMessage
	if json.Unmarshal(body, &request) != nil {
		return body, false
	}
	var input []json.RawMessage
	if json.Unmarshal(request["input"], &input) != nil {
		return body, false
	}
	changed := false
	for i, raw := range input {
		var item map[string]json.RawMessage
		if json.Unmarshal(raw, &item) != nil {
			continue
		}
		readString := func(key string) string {
			var value string
			_ = json.Unmarshal(item[key], &value)
			return value
		}
		if readString("type") != "function_call_output" || readString("namespace") != "agent" {
			continue
		}
		if callID, exists := item["call_id"]; exists {
			var value string
			if bytes.Equal(bytes.TrimSpace(callID), []byte("null")) || json.Unmarshal(callID, &value) != nil || strings.TrimSpace(value) != "" {
				continue
			}
		}
		name, output := readString("name"), readString("output")
		if !validCodexUntrustedContext(name, output) {
			continue
		}
		text := "<external_" + name + ">" + output + "</external_" + name + ">"
		if validCodexPageContext(name, output) {
			text = "<external_" + name + ">\n" + output + "\n</external_" + name + ">"
		}
		message := map[string]any{
			"type": "message",
			"role": "user",
			"content": []map[string]string{{
				"type": "input_text",
				"text": text,
			}},
		}
		encoded, err := json.Marshal(message)
		if err != nil {
			return body, false
		}
		input[i] = encoded
		changed = true
	}
	if !changed {
		return body, false
	}
	encodedInput, err := json.Marshal(input)
	if err != nil {
		return body, false
	}
	request["input"] = encodedInput
	encoded, err := json.Marshal(request)
	if err != nil {
		return body, false
	}
	return encoded, true
}

func validCodexUntrustedContext(name, output string) bool {
	switch name {
	case "browser_info", "goal_objective", "untrusted_context":
		return strings.TrimSpace(output) != ""
	default:
		return validCodexPageContext(name, output)
	}
}

func validCodexPageContext(name, output string) bool {
	if name == "codex_apps_open_page" {
		return validCodexPageSnapshot(output)
	}
	parts := codexSelectedPageName.FindStringSubmatch(name)
	if len(parts) != 3 {
		return false
	}
	part, err := strconv.Atoi(parts[1])
	if err != nil {
		return false
	}
	total, err := strconv.Atoi(parts[2])
	if err != nil || part > total {
		return false
	}
	header, snapshot, ok := strings.Cut(output, "\n")
	if !ok || strings.TrimSuffix(header, "\r") != fmt.Sprintf("Companion Page snapshot part %d of %d:", part, total) || strings.TrimSpace(snapshot) == "" {
		return false
	}
	// A multi-part snapshot may split inside a JSON string; preserve each chunk verbatim.
	return total > 1 || validCodexPageSnapshot(snapshot)
}

func validCodexPageSnapshot(text string) bool {
	if !hasUniqueJSONMembers([]byte(text)) {
		return false
	}
	var snapshot map[string]json.RawMessage
	if json.Unmarshal([]byte(text), &snapshot) != nil {
		return false
	}
	id, exists := snapshot["page_id"]
	if !exists {
		return false
	}
	if bytes.Equal(bytes.TrimSpace(id), []byte("null")) {
		return true
	}
	var value string
	return json.Unmarshal(id, &value) == nil && strings.TrimSpace(value) != ""
}
