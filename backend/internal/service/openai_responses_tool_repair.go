package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	responsesChatToolRepairMaxAttempts = 1
	responsesChatToolRepairMaxNames    = 128
)

var responsesChatToolRepairNamePattern = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,64}$`)

type responsesChatToolRepairSender func(unknownNames []string) (*http.Response, error)

func buildResponsesChatToolRepairBody(body []byte, unknownNames []string) ([]byte, error) {
	if !gjson.ValidBytes(body) {
		return nil, fmt.Errorf("invalid chat fallback request")
	}
	allowedNames := responsesChatDeclaredToolNames(body)
	if len(allowedNames) == 0 {
		return nil, fmt.Errorf("chat fallback request has no declared tools")
	}
	unknownNames = sanitizeResponsesChatToolNames(unknownNames)

	contract := "Tool protocol correction: retry the original task now. "
	if len(unknownNames) > 0 {
		contract += "The previous attempt called undeclared tool(s): " + strings.Join(unknownNames, ", ") + ". "
	} else {
		contract += "The previous attempt called an undeclared tool. "
	}
	contract += "Call only a tool whose name exactly matches one of the declared tools. Allowed tool names: " + strings.Join(allowedNames, ", ") + ". Do not invent, prefix, namespace, or rename tools."
	if containsStringValue(allowedNames, "exec") {
		contract += " For filesystem or command work, use exec with its declared schema."
	}
	contract += " Return the corrected tool call without explaining this correction."

	repairMessage, err := json.Marshal(map[string]string{"role": "user", "content": contract})
	if err != nil {
		return nil, fmt.Errorf("marshal tool repair message: %w", err)
	}

	var messages []json.RawMessage
	messagesJSON := gjson.GetBytes(body, "messages")
	if messagesJSON.Exists() {
		if !messagesJSON.IsArray() {
			return nil, fmt.Errorf("chat fallback messages must be an array")
		}
		if err := json.Unmarshal([]byte(messagesJSON.Raw), &messages); err != nil {
			return nil, fmt.Errorf("decode chat fallback messages: %w", err)
		}
	}
	// Append the correction so the original conversation remains an unchanged
	// prompt prefix and can still benefit from upstream prefix caching.
	messages = append(messages, repairMessage)

	messagesBytes, err := json.Marshal(messages)
	if err != nil {
		return nil, fmt.Errorf("encode repaired chat fallback messages: %w", err)
	}
	repaired, err := sjson.SetRawBytes(body, "messages", messagesBytes)
	if err != nil {
		return nil, fmt.Errorf("set repaired chat fallback messages: %w", err)
	}
	return repaired, nil
}

func responsesChatDeclaredToolNames(body []byte) []string {
	var names []string
	for _, tool := range gjson.GetBytes(body, "tools").Array() {
		names = append(names, tool.Get("function.name").String())
	}
	for _, function := range gjson.GetBytes(body, "functions").Array() {
		names = append(names, function.Get("name").String())
	}
	return sanitizeResponsesChatToolNames(names)
}

func sanitizeResponsesChatToolNames(names []string) []string {
	unique := make(map[string]struct{})
	for _, name := range names {
		name = strings.TrimSpace(name)
		if !responsesChatToolRepairNamePattern.MatchString(name) {
			continue
		}
		unique[name] = struct{}{}
	}
	out := make([]string, 0, min(len(unique), responsesChatToolRepairMaxNames))
	for name := range unique {
		out = append(out, name)
	}
	sort.Strings(out)
	if len(out) > responsesChatToolRepairMaxNames {
		out = out[:responsesChatToolRepairMaxNames]
	}
	return out
}

func containsStringValue(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
