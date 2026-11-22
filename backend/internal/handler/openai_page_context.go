package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/tidwall/gjson"
)

var codexSelectedPageName = regexp.MustCompile(`^codex_apps_selected_page_part_([1-9][0-9]*)_of_([1-9][0-9]*)$`)

// Codex injects external context as standalone outputs, without a model tool call.
// Use the legacy user-message representation for HTTP Responses interoperability.
func normalizeCodexUntrustedContext(body []byte) ([]byte, bool) {
	if !mayContainCodexStandaloneContext(body) || !hasUniqueJSONMembers(body) {
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
		name := readString("name")
		var output *string
		if json.Unmarshal(item["output"], &output) != nil || output == nil || !validCodexUntrustedContext(name, *output) {
			continue
		}
		text := "<external_" + name + ">" + *output + "</external_" + name + ">"
		if validCodexPageContext(name, *output) {
			text = "<external_" + name + ">\n" + *output + "\n</external_" + name + ">"
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

// Codex emits every additional context source (terminal_info, browser_tabs, ...)
// as an agent-namespace output without call_id, named from the source key with
// only ASCII letters, digits and underscores (at most 128). Accept that shape
// instead of chasing an allowlist of names clients keep extending.
var codexContextName = regexp.MustCompile(`^[A-Za-z0-9_]{1,128}$`)

// mayContainCodexStandaloneContext is a zero-copy probe for an input item of type
// function_call_output in the agent namespace without a call_id, so ordinary tool
// history never pays for the full uniqueness scan and decode.
func mayContainCodexStandaloneContext(body []byte) bool {
	if !service.JSONMayEscapePlainASCII(body) &&
		(!bytes.Contains(body, []byte("function_call_output")) || !bytes.Contains(body, []byte("agent"))) {
		return false
	}
	input := gjson.GetBytes(body, "input")
	if !input.IsArray() {
		return false
	}
	found := false
	input.ForEach(func(_, item gjson.Result) bool {
		if !item.IsObject() || item.Get("type").String() != "function_call_output" || item.Get("namespace").String() != "agent" {
			return true
		}
		callID := item.Get("call_id")
		if !callID.Exists() || (callID.Type == gjson.String && strings.TrimSpace(callID.String()) == "") {
			found = true
			return false
		}
		return true
	})
	return found
}

func validCodexUntrustedContext(name, output string) bool {
	switch name {
	case "untrusted_content":
		// Codex's fallback for an unnamed context source can carry any text, including an empty value.
		return true
	case "proposed_plan":
		// Codex splits an approved plan by byte length, so a chunk can be all whitespace.
		return output != ""
	case "codex_apps_open_page":
		return validCodexPageContext(name, output)
	default:
		if codexSelectedPageName.MatchString(name) {
			return validCodexPageContext(name, output)
		}
		return codexContextName.MatchString(name) && strings.TrimSpace(output) != ""
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
