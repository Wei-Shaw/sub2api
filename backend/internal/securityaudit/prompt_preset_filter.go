package securityaudit

import (
	"encoding/json"
	"strings"
)

// Protocol envelopes and agent-injected text are separate extension points.
// Never walk arbitrary quoted text: it can resemble a preset while still being
// part of the user's interaction.
type agentPresetRule struct {
	identity string
	prefixes []string
}

var agentPresetRules = []agentPresetRule{
	{identity: "You are Codex", prefixes: []string{"# AGENTS.md instructions\n", "<environment_context>"}},
	{identity: "You are Claude Code", prefixes: []string{"<system-reminder>"}},
}

func filterPresetRequestBody(protocol string, body []byte, enabled bool) []byte {
	if !enabled {
		return body
	}
	// RawMessage preserves retained multimodal payloads without a float64 round trip.
	var root map[string]json.RawMessage
	if json.Unmarshal(body, &root) != nil || root == nil {
		return body
	}
	var rules []agentPresetRule
	for _, rule := range agentPresetRules {
		if envelopeHasIdentity(root, rule.identity) {
			rules = append(rules, rule)
		}
	}
	filtered := compactInteractionEnvelope(root, rules, isMediaProtocol(protocol))
	result, err := json.Marshal(filtered)
	if err != nil {
		return body
	}
	return result
}

func envelopeHasIdentity(root map[string]json.RawMessage, identity string) bool {
	for _, key := range []string{"system", "instructions", "systemInstruction", "system_instruction"} {
		if strings.Contains(string(root[key]), identity) {
			return true
		}
	}
	for _, key := range []string{"messages", "input"} {
		var items []map[string]json.RawMessage
		if json.Unmarshal(root[key], &items) == nil {
			for _, item := range items {
				if presetRole(item) && strings.Contains(string(item["content"]), identity) {
					return true
				}
			}
		}
	}
	var nested map[string]json.RawMessage
	if json.Unmarshal(root["response"], &nested) == nil && nested != nil && envelopeHasIdentity(nested, identity) {
		return true
	}
	var requests []map[string]json.RawMessage
	if json.Unmarshal(root["requests"], &requests) == nil {
		for _, request := range requests {
			if envelopeHasIdentity(request, identity) {
				return true
			}
		}
	}
	return false
}

func presetRole(item map[string]json.RawMessage) bool {
	var role string
	_ = json.Unmarshal(item["role"], &role)
	return strings.EqualFold(role, "system") || strings.EqualFold(role, "developer")
}

// compactInteractionEnvelope uses an allowlist because production agent
// requests contain large, evolving collections of transport and runtime fields.
// The stored request needs conversation content, not a replayable API request.
func compactInteractionEnvelope(root map[string]json.RawMessage, rules []agentPresetRule, media bool) map[string]json.RawMessage {
	result := make(map[string]json.RawMessage)
	for _, key := range []string{"messages", "input", "contents", "content"} {
		if raw, ok := root[key]; ok {
			if compacted, keep := compactInteractionValue(raw, rules, ""); keep {
				result[key] = compacted
			}
		}
	}

	var response map[string]json.RawMessage
	if json.Unmarshal(root["response"], &response) == nil && response != nil {
		if compacted := compactInteractionEnvelope(response, rules, media); len(compacted) > 0 {
			result["response"] = marshalRaw(compacted)
		}
	}

	var requests []map[string]json.RawMessage
	if json.Unmarshal(root["requests"], &requests) == nil {
		compacted := make([]map[string]json.RawMessage, 0, len(requests))
		for _, request := range requests {
			if item := compactInteractionEnvelope(request, rules, media); len(item) > 0 {
				compacted = append(compacted, item)
			}
		}
		if len(compacted) > 0 {
			result["requests"] = marshalRaw(compacted)
		}
	}

	// Responses WebSocket extraction requires this discriminator. It is the only
	// transport field retained, and only for the frame that carries interaction.
	var frameType string
	if json.Unmarshal(root["type"], &frameType) == nil && frameType == "response.create" && (result["input"] != nil || result["response"] != nil) {
		result["type"] = marshalRaw(frameType)
	}

	if media || len(result) == 0 {
		for _, key := range mediaInteractionRootKeys {
			raw, exists := root[key]
			if !exists {
				continue
			}
			if _, exists := result[key]; exists {
				continue
			}
			if compacted, keep := compactMediaValue(raw, key); keep {
				result[key] = compacted
			}
		}
	}
	return result
}

func compactInteractionValue(raw json.RawMessage, rules []agentPresetRule, role string) (json.RawMessage, bool) {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		if role == "" || role == "user" {
			text = stripAgentPresetPrefix(text, rules)
		}
		if strings.TrimSpace(text) == "" {
			return nil, false
		}
		return marshalRaw(text), true
	}

	var items []json.RawMessage
	if json.Unmarshal(raw, &items) == nil {
		kept := make([]json.RawMessage, 0, len(items))
		for _, item := range items {
			if compacted, keep := compactInteractionItem(item, rules, role); keep {
				kept = append(kept, compacted)
			}
		}
		if len(kept) == 0 {
			return nil, false
		}
		return marshalRaw(kept), true
	}

	var item map[string]json.RawMessage
	if json.Unmarshal(raw, &item) == nil && item != nil {
		return compactInteractionObject(item, rules, role, false)
	}
	return nil, false
}

func compactInteractionItem(raw json.RawMessage, rules []agentPresetRule, parentRole string) (json.RawMessage, bool) {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return compactInteractionValue(raw, rules, parentRole)
	}
	var item map[string]json.RawMessage
	if json.Unmarshal(raw, &item) != nil || item == nil {
		return nil, false
	}
	return compactInteractionObject(item, rules, parentRole, parentRole != "")
}

func compactInteractionObject(item map[string]json.RawMessage, rules []agentPresetRule, parentRole string, contentPart bool) (json.RawMessage, bool) {
	role, hasRole := rawString(item["role"])
	role = strings.ToLower(strings.TrimSpace(role))
	if hasRole && !interactionRole(role) {
		return nil, false
	}
	if hasRole {
		return compactMessage(item, rules, role)
	}

	kind, _ := rawString(item["type"])
	kind = strings.ToLower(strings.TrimSpace(kind))
	if nonInteractionType(kind) {
		return nil, false
	}
	if contentPart || textContentType(kind) || mediaContentType(kind) {
		return compactContentPart(item, rules, parentRole)
	}
	if kind != "" && kind != "message" {
		return nil, false
	}

	// Role-less Responses input objects represent user content.
	implicitRole := parentRole
	if implicitRole == "" {
		implicitRole = "user"
	}
	result := make(map[string]json.RawMessage)
	for _, key := range []string{"content", "parts", "text"} {
		if raw, exists := item[key]; exists {
			if compacted, keep := compactInteractionValue(raw, rules, implicitRole); keep {
				result[key] = compacted
			}
		}
	}
	if len(result) == 0 {
		return nil, false
	}
	return marshalRaw(result), true
}

func compactMessage(item map[string]json.RawMessage, rules []agentPresetRule, role string) (json.RawMessage, bool) {
	result := map[string]json.RawMessage{"role": marshalRaw(role)}
	for _, key := range []string{"content", "parts"} {
		if raw, exists := item[key]; exists {
			if compacted, keep := compactContentValue(raw, rules, role); keep {
				result[key] = compacted
			}
		}
	}
	if raw, exists := item["text"]; exists {
		if compacted, keep := compactInteractionValue(raw, rules, role); keep {
			result["text"] = compacted
		}
	}
	if len(result) == 1 {
		return nil, false
	}
	return marshalRaw(result), true
}

func compactContentValue(raw json.RawMessage, rules []agentPresetRule, role string) (json.RawMessage, bool) {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return compactInteractionValue(raw, rules, role)
	}
	var parts []json.RawMessage
	if json.Unmarshal(raw, &parts) == nil {
		kept := make([]json.RawMessage, 0, len(parts))
		for _, part := range parts {
			var object map[string]json.RawMessage
			if json.Unmarshal(part, &object) == nil && object != nil {
				if compacted, keep := compactContentPart(object, rules, role); keep {
					kept = append(kept, compacted)
				}
				continue
			}
			if compacted, keep := compactInteractionValue(part, rules, role); keep {
				kept = append(kept, compacted)
			}
		}
		if len(kept) == 0 {
			return nil, false
		}
		return marshalRaw(kept), true
	}
	var part map[string]json.RawMessage
	if json.Unmarshal(raw, &part) == nil && part != nil {
		return compactContentPart(part, rules, role)
	}
	return nil, false
}

func compactContentPart(part map[string]json.RawMessage, rules []agentPresetRule, role string) (json.RawMessage, bool) {
	kind, _ := rawString(part["type"])
	kind = strings.ToLower(strings.TrimSpace(kind))
	if nonInteractionType(kind) {
		return nil, false
	}
	if textContentType(kind) || (kind == "" && part["text"] != nil) {
		text, ok := rawString(part["text"])
		if !ok {
			return nil, false
		}
		if role == "" || role == "user" {
			text = stripAgentPresetPrefix(text, rules)
		}
		if strings.TrimSpace(text) == "" {
			return nil, false
		}
		result := map[string]json.RawMessage{"text": marshalRaw(text)}
		if kind != "" {
			result["type"] = marshalRaw(kind)
		}
		return marshalRaw(result), true
	}
	if kind == "refusal" {
		if refusal, ok := rawString(part["refusal"]); ok && strings.TrimSpace(refusal) != "" {
			return marshalRaw(map[string]json.RawMessage{"type": marshalRaw(kind), "refusal": marshalRaw(refusal)}), true
		}
		return nil, false
	}
	// Images, audio, files, and future content parts inside a retained user or
	// assistant message are interaction data. Preserve their complete value.
	if kind != "" {
		return marshalRaw(part), true
	}
	return nil, false
}

func interactionRole(role string) bool {
	return role == "user" || role == "assistant" || role == "model"
}

func textContentType(kind string) bool {
	return kind == "text" || kind == "input_text" || kind == "output_text"
}

func mediaContentType(kind string) bool {
	switch kind {
	case "image", "image_url", "input_image", "output_image", "audio", "input_audio", "output_audio", "file", "input_file", "document":
		return true
	default:
		return false
	}
}

func nonInteractionType(kind string) bool {
	if kind == "" {
		return false
	}
	switch kind {
	case "additional_tools", "reasoning", "item_reference", "computer_initialize_state", "computer_screenshot":
		return true
	}
	return strings.Contains(kind, "tool") || strings.HasSuffix(kind, "_call") || strings.HasSuffix(kind, "_call_output")
}

var mediaInteractionRootKeys = []string{
	"prompt", "input_prompt", "text_prompt", "description", "query", "lyrics",
	"negative_prompt", "positive_prompt", "gpt_description_prompt", "prompt_en",
	"final_prompt", "final_zh_prompt", "orig_prompt", "actual_prompt", "image_prompt",
	"input", "request", "image", "images",
}

func compactMediaValue(raw json.RawMessage, key string) (json.RawMessage, bool) {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		if !isMediaPromptKey(key) || looksLikeMediaPayload(text) || strings.TrimSpace(text) == "" {
			return nil, false
		}
		return marshalRaw(text), true
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) == nil && object != nil {
		result := make(map[string]json.RawMessage)
		for childKey, child := range object {
			if compacted, keep := compactMediaValue(child, childKey); keep {
				result[childKey] = compacted
			}
		}
		if len(result) == 0 {
			return nil, false
		}
		return marshalRaw(result), true
	}
	var items []json.RawMessage
	if json.Unmarshal(raw, &items) == nil {
		result := make([]json.RawMessage, 0, len(items))
		for _, item := range items {
			if compacted, keep := compactMediaValue(item, key); keep {
				result = append(result, compacted)
			}
		}
		if len(result) == 0 {
			return nil, false
		}
		return marshalRaw(result), true
	}
	return nil, false
}

func isMediaProtocol(protocol string) bool {
	switch strings.ToLower(strings.TrimSpace(protocol)) {
	case "openai_images", "grok_media", "media", "images":
		return true
	default:
		return false
	}
}

func rawString(raw json.RawMessage) (string, bool) {
	var value string
	if len(raw) == 0 || json.Unmarshal(raw, &value) != nil {
		return "", false
	}
	return value, true
}

func marshalRaw(value any) json.RawMessage {
	result, _ := json.Marshal(value)
	return result
}

func stripAgentPresetPrefix(text string, rules []agentPresetRule) string {
	remaining := text
	for {
		trimmed := strings.TrimSpace(strings.ReplaceAll(remaining, "\r\n", "\n"))
		end := -1
		for _, rule := range rules {
			for _, prefix := range rule.prefixes {
				if !strings.HasPrefix(trimmed, prefix) {
					continue
				}
				closing := "</environment_context>"
				if prefix == "<system-reminder>" {
					closing = "</system-reminder>"
				}
				if strings.HasPrefix(prefix, "# AGENTS.md") {
					if !strings.HasPrefix(strings.TrimSpace(strings.TrimPrefix(trimmed, prefix)), "<INSTRUCTIONS>") {
						continue
					}
					closing = "</INSTRUCTIONS>"
				}
				if index := strings.Index(trimmed, closing); index >= 0 {
					end = index + len(closing)
				}
			}
		}
		if end < 0 {
			return remaining
		}
		remaining = strings.TrimSpace(trimmed[end:])
	}
}
