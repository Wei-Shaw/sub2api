package apicompat

import "strings"

// NormalizeResponsesAgentMessages converts Codex-private multi-agent history
// items into standard Responses user messages for compatible upstreams.
func NormalizeResponsesAgentMessages(request map[string]any) bool {
	items, ok := request["input"].([]any)
	if !ok {
		return false
	}
	normalized := make([]any, 0, len(items))
	changed := false
	for _, raw := range items {
		item, isObject := raw.(map[string]any)
		if !isObject || strings.TrimSpace(stringValue(item["type"])) != "agent_message" {
			normalized = append(normalized, raw)
			continue
		}
		changed = true
		text := agentMessageTextValue(item["content"])
		if text == "" {
			continue
		}
		normalized = append(normalized, map[string]any{
			"type":    "message",
			"role":    "user",
			"content": []any{map[string]any{"type": "input_text", "text": text}},
		})
	}
	if changed {
		request["input"] = normalized
	}
	return changed
}

func agentMessageTextValue(content any) string {
	if text, ok := content.(string); ok {
		return strings.TrimSpace(text)
	}
	parts, ok := content.([]any)
	if !ok {
		return ""
	}
	var out strings.Builder
	for _, raw := range parts {
		part, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		var value string
		switch strings.TrimSpace(stringValue(part["type"])) {
		case "input_text", "output_text", "text":
			value = stringValue(part["text"])
		case "encrypted_content":
			value = stringValue(part["encrypted_content"])
		}
		if value == "" {
			continue
		}
		if out.Len() > 0 && !strings.HasSuffix(out.String(), "\n") {
			_ = out.WriteByte('\n')
		}
		_, _ = out.WriteString(value)
	}
	return strings.TrimSpace(out.String())
}
