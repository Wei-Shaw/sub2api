package securityaudit

import "strings"

const promptRecordMaxMessages = 30

// truncatePromptRecordMessages keeps only the newest messages in the retained
// request document. The request itself may contain much more history, but a
// prompt record only needs the latest conversation context for review.
func truncatePromptRecordMessages(value any, maxMessages int) any {
	if maxMessages < 1 {
		return value
	}
	return truncatePromptRecordMessageValue(value, "", maxMessages)
}

func truncatePromptRecordMessageValue(value any, key string, maxMessages int) any {
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for childKey, child := range typed {
			result[childKey] = truncatePromptRecordMessageValue(child, childKey, maxMessages)
		}
		return result
	case []any:
		if promptRecordMessageCollectionKey(key) && len(typed) > maxMessages {
			result := make([]any, maxMessages)
			for index, item := range typed[len(typed)-maxMessages:] {
				result[index] = truncatePromptRecordMessageValue(item, "", maxMessages)
			}
			return result
		}
		result := make([]any, len(typed))
		for index, item := range typed {
			result[index] = truncatePromptRecordMessageValue(item, "", maxMessages)
		}
		return result
	}
	return value
}

func promptRecordMessageCollectionKey(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "messages", "input", "contents":
		return true
	default:
		return false
	}
}

// sanitizePromptRecordDocument builds a new retained tree. The input tree stays
// untouched so its prompt identity can be calculated before filtering.
func sanitizePromptRecordDocument(protocol string, document any, options promptRecordFilterOptions) any {
	stripped, keep := stripMultimodalDocument(document, "")
	if !keep {
		stripped = map[string]any{}
	}
	return filterPresetDocument(protocol, stripped, options)
}

func stripMultimodalDocument(value any, key string) (any, bool) {
	if isMultimodalPayloadKey(key) {
		return nil, false
	}
	switch typed := value.(type) {
	case map[string]any:
		kind, _ := documentString(typed["type"])
		if mediaContentType(strings.ToLower(strings.TrimSpace(kind))) || documentHasMediaMIME(typed) {
			return nil, false
		}
		filtered := make(map[string]any, len(typed))
		for childKey, child := range typed {
			if retained, keep := stripMultimodalDocument(child, childKey); keep {
				filtered[childKey] = retained
			}
		}
		return filtered, true
	case []any:
		filtered := make([]any, 0, len(typed))
		for _, item := range typed {
			if retained, keep := stripMultimodalDocument(item, ""); keep {
				filtered = append(filtered, retained)
			}
		}
		return filtered, true
	case string:
		if looksLikeInlineMedia(typed) {
			return nil, false
		}
	}
	return value, true
}

func documentHasMediaMIME(object map[string]any) bool {
	for _, key := range []string{"mime_type", "mimeType", "media_type", "content_type"} {
		value, ok := documentString(object[key])
		if !ok {
			continue
		}
		value = strings.ToLower(strings.TrimSpace(value))
		if strings.HasPrefix(value, "image/") || strings.HasPrefix(value, "audio/") || strings.HasPrefix(value, "video/") ||
			value == "application/octet-stream" || value == "application/pdf" {
			return true
		}
	}
	return false
}

func filterPresetDocument(protocol string, document any, options promptRecordFilterOptions) any {
	if !options.Enabled {
		return document
	}
	root, ok := document.(map[string]any)
	if !ok || root == nil {
		return document
	}
	rules := make([]agentPresetRule, 0, len(agentPresetRules))
	for _, rule := range agentPresetRules {
		if documentHasIdentity(root, rule.identity) {
			rules = append(rules, rule)
		}
	}
	return compactInteractionDocument(root, rules, isMediaProtocol(protocol), options.AgentPreset, options.Skills)
}

func documentHasIdentity(root map[string]any, identity string) bool {
	for _, key := range []string{"system", "instructions", "systemInstruction", "system_instruction"} {
		if documentContainsString(root[key], identity) {
			return true
		}
	}
	for _, key := range []string{"messages", "input"} {
		items, _ := root[key].([]any)
		for _, item := range items {
			message, ok := item.(map[string]any)
			if ok && documentPresetRole(message) && documentContainsString(message["content"], identity) {
				return true
			}
		}
	}
	if response, ok := root["response"].(map[string]any); ok && documentHasIdentity(response, identity) {
		return true
	}
	if requests, ok := root["requests"].([]any); ok {
		for _, item := range requests {
			if request, ok := item.(map[string]any); ok && documentHasIdentity(request, identity) {
				return true
			}
		}
	}
	return false
}

func compactInteractionDocument(root map[string]any, rules []agentPresetRule, media, filterAgentPreset, filterSkills bool) map[string]any {
	interactionRules := rules
	if !filterAgentPreset {
		interactionRules = nil
	}
	result := make(map[string]any)
	for _, key := range []string{"messages", "input", "contents", "content"} {
		if value, exists := root[key]; exists {
			if compacted, keep := compactInteractionDocumentValue(value, interactionRules, ""); keep {
				result[key] = compacted
			}
		}
	}

	if response, ok := root["response"].(map[string]any); ok {
		if compacted := compactInteractionDocument(response, rules, media, filterAgentPreset, filterSkills); len(compacted) > 0 {
			result["response"] = compacted
		}
	}
	if requests, ok := root["requests"].([]any); ok {
		compacted := make([]any, 0, len(requests))
		for _, item := range requests {
			request, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if retained := compactInteractionDocument(request, rules, media, filterAgentPreset, filterSkills); len(retained) > 0 {
				compacted = append(compacted, retained)
			}
		}
		if len(compacted) > 0 {
			result["requests"] = compacted
		}
	}

	if frameType, _ := documentString(root["type"]); frameType == "response.create" && (result["input"] != nil || result["response"] != nil) {
		result["type"] = frameType
	}
	if media || len(result) == 0 {
		for _, key := range mediaInteractionRootKeys {
			value, exists := root[key]
			if !exists || result[key] != nil {
				continue
			}
			if compacted, keep := compactMediaDocumentValue(value, key); keep {
				result[key] = compacted
			}
		}
	}
	if !filterAgentPreset {
		retainAgentPresetDocumentBlocks(root, result, rules)
	}
	if !filterSkills {
		retainRegisteredSkillDocumentBlocks(root, result)
	}
	return result
}

func compactInteractionDocumentValue(value any, rules []agentPresetRule, role string) (any, bool) {
	switch typed := value.(type) {
	case string:
		text := typed
		if role == "" || role == "user" {
			text = stripAgentPresetPrefix(text, rules)
		}
		if strings.TrimSpace(text) == "" {
			return nil, false
		}
		return text, true
	case []any:
		kept := make([]any, 0, len(typed))
		for _, item := range typed {
			if compacted, keep := compactInteractionDocumentItem(item, rules, role); keep {
				kept = append(kept, compacted)
			}
		}
		return kept, len(kept) > 0
	case map[string]any:
		return compactInteractionDocumentObject(typed, rules, role, false)
	default:
		return nil, false
	}
}

func compactInteractionDocumentItem(value any, rules []agentPresetRule, parentRole string) (any, bool) {
	if text, ok := value.(string); ok {
		return compactInteractionDocumentValue(text, rules, parentRole)
	}
	item, ok := value.(map[string]any)
	if !ok {
		return nil, false
	}
	return compactInteractionDocumentObject(item, rules, parentRole, parentRole != "")
}

func compactInteractionDocumentObject(item map[string]any, rules []agentPresetRule, parentRole string, contentPart bool) (any, bool) {
	role, hasRole := documentString(item["role"])
	role = strings.ToLower(strings.TrimSpace(role))
	if hasRole && !interactionRole(role) {
		return nil, false
	}
	if hasRole {
		return compactDocumentMessage(item, rules, role)
	}

	kind, _ := documentString(item["type"])
	kind = strings.ToLower(strings.TrimSpace(kind))
	if nonInteractionType(kind) {
		return nil, false
	}
	if contentPart || textContentType(kind) || mediaContentType(kind) {
		return compactDocumentContentPart(item, rules, parentRole)
	}
	if kind != "" && kind != "message" {
		return nil, false
	}

	implicitRole := parentRole
	if implicitRole == "" {
		implicitRole = "user"
	}
	result := make(map[string]any)
	for _, key := range []string{"content", "parts", "text"} {
		if value, exists := item[key]; exists {
			if compacted, keep := compactInteractionDocumentValue(value, rules, implicitRole); keep {
				result[key] = compacted
			}
		}
	}
	return result, len(result) > 0
}

func compactDocumentMessage(item map[string]any, rules []agentPresetRule, role string) (any, bool) {
	result := map[string]any{"role": role}
	for _, key := range []string{"content", "parts"} {
		if value, exists := item[key]; exists {
			if compacted, keep := compactDocumentContentValue(value, rules, role); keep {
				result[key] = compacted
			}
		}
	}
	if value, exists := item["text"]; exists {
		if compacted, keep := compactInteractionDocumentValue(value, rules, role); keep {
			result["text"] = compacted
		}
	}
	return result, len(result) > 1
}

func compactDocumentContentValue(value any, rules []agentPresetRule, role string) (any, bool) {
	if _, ok := value.(string); ok {
		return compactInteractionDocumentValue(value, rules, role)
	}
	if parts, ok := value.([]any); ok {
		kept := make([]any, 0, len(parts))
		for _, part := range parts {
			if object, ok := part.(map[string]any); ok {
				if compacted, keep := compactDocumentContentPart(object, rules, role); keep {
					kept = append(kept, compacted)
				}
				continue
			}
			if compacted, keep := compactInteractionDocumentValue(part, rules, role); keep {
				kept = append(kept, compacted)
			}
		}
		return kept, len(kept) > 0
	}
	if part, ok := value.(map[string]any); ok {
		return compactDocumentContentPart(part, rules, role)
	}
	return nil, false
}

func compactDocumentContentPart(part map[string]any, rules []agentPresetRule, role string) (any, bool) {
	kind, _ := documentString(part["type"])
	kind = strings.ToLower(strings.TrimSpace(kind))
	if nonInteractionType(kind) || mediaContentType(kind) {
		return nil, false
	}
	if textContentType(kind) || (kind == "" && part["text"] != nil) {
		text, ok := documentString(part["text"])
		if !ok {
			return nil, false
		}
		if role == "" || role == "user" {
			text = stripAgentPresetPrefix(text, rules)
		}
		if strings.TrimSpace(text) == "" {
			return nil, false
		}
		result := map[string]any{"text": text}
		if kind != "" {
			result["type"] = kind
		}
		return result, true
	}
	if kind == "refusal" {
		if refusal, ok := documentString(part["refusal"]); ok && strings.TrimSpace(refusal) != "" {
			return map[string]any{"type": kind, "refusal": refusal}, true
		}
		return nil, false
	}
	if kind != "" {
		return part, true
	}
	return nil, false
}

func compactMediaDocumentValue(value any, key string) (any, bool) {
	switch typed := value.(type) {
	case string:
		if !isMediaPromptKey(key) || looksLikeMediaPayload(typed) || strings.TrimSpace(typed) == "" {
			return nil, false
		}
		return typed, true
	case map[string]any:
		result := make(map[string]any)
		for childKey, child := range typed {
			if compacted, keep := compactMediaDocumentValue(child, childKey); keep {
				result[childKey] = compacted
			}
		}
		return result, len(result) > 0
	case []any:
		result := make([]any, 0, len(typed))
		for _, item := range typed {
			if compacted, keep := compactMediaDocumentValue(item, key); keep {
				result = append(result, compacted)
			}
		}
		return result, len(result) > 0
	default:
		return nil, false
	}
}

func retainAgentPresetDocumentBlocks(root, result map[string]any, rules []agentPresetRule) {
	if len(rules) == 0 {
		return
	}
	seen := map[string]struct{}{}
	blocks := make([]string, 0, 1)
	collect := func(value any) {
		walkDocumentStrings(value, func(text string) {
			for _, block := range extractAgentPresetBlocks(text, rules) {
				if _, exists := seen[block]; exists {
					continue
				}
				seen[block] = struct{}{}
				blocks = append(blocks, block)
			}
		})
	}
	for _, key := range []string{"system", "instructions", "systemInstruction", "system_instruction"} {
		collect(root[key])
	}
	for _, key := range []string{"messages", "input"} {
		items, _ := root[key].([]any)
		for _, item := range items {
			if message, ok := item.(map[string]any); ok && documentPresetRole(message) {
				collect(message["content"])
			}
		}
	}
	appendRetainedDocumentBlocks(root, result, blocks)
}

func retainRegisteredSkillDocumentBlocks(root, result map[string]any) {
	seen := map[string]struct{}{}
	blocks := make([]string, 0, 1)
	collect := func(value any) {
		walkDocumentStrings(value, func(text string) {
			for _, block := range extractTaggedBlocks(text, "<skills_instructions>", "</skills_instructions>") {
				if _, exists := seen[block]; exists {
					continue
				}
				seen[block] = struct{}{}
				blocks = append(blocks, block)
			}
		})
	}
	for _, key := range []string{"system", "instructions", "systemInstruction", "system_instruction"} {
		collect(root[key])
	}
	for _, key := range []string{"messages", "input"} {
		items, _ := root[key].([]any)
		for _, item := range items {
			if message, ok := item.(map[string]any); ok && documentPresetRole(message) {
				collect(message["content"])
			}
		}
	}
	appendRetainedDocumentBlocks(root, result, blocks)
}

func appendRetainedDocumentBlocks(root, result map[string]any, blocks []string) {
	if len(blocks) == 0 {
		return
	}
	key := ""
	for _, candidate := range []string{"input", "messages"} {
		if _, exists := root[candidate]; exists {
			key = candidate
			break
		}
	}
	if key == "" {
		return
	}
	message := map[string]any{"role": "developer", "content": strings.Join(blocks, "\n\n")}
	items, ok := result[key].([]any)
	if !ok && result[key] != nil {
		items = []any{map[string]any{"role": "user", "content": result[key]}}
	}
	result[key] = append([]any{message}, items...)
}

func walkDocumentStrings(value any, visit func(string)) {
	switch typed := value.(type) {
	case string:
		visit(typed)
	case []any:
		for _, item := range typed {
			walkDocumentStrings(item, visit)
		}
	case map[string]any:
		for _, item := range typed {
			walkDocumentStrings(item, visit)
		}
	}
}

func documentContainsString(value any, fragment string) bool {
	found := false
	walkDocumentStrings(value, func(text string) {
		if strings.Contains(text, fragment) {
			found = true
		}
	})
	return found
}

func documentPresetRole(item map[string]any) bool {
	role, _ := documentString(item["role"])
	return strings.EqualFold(role, "system") || strings.EqualFold(role, "developer")
}

func documentString(value any) (string, bool) {
	text, ok := value.(string)
	return text, ok
}
