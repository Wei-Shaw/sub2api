package securityaudit

import (
	"sort"
	"strings"
)

// ---- Gemini -------------------------------------------------------------------

func extractGemini(value any) []AuditSegment {
	var contents []any
	switch typed := value.(type) {
	case []any:
		contents = typed
	case map[string]any:
		contents = []any{typed}
	default:
		return nil
	}
	result := make([]AuditSegment, 0, len(contents))
	for msgIndex, item := range contents {
		content, ok := item.(map[string]any)
		if !ok {
			continue
		}
		role := strings.ToLower(stringValue(content["role"]))
		if role != "" && !isClientInstructionRole(role) {
			continue
		}
		parts, _ := content["parts"].([]any)
		for blockIndex, part := range parts {
			object, ok := part.(map[string]any)
			if !ok {
				continue
			}
			result = append(result, geminiPartSegment(object, role, msgIndex, blockIndex)...)
		}
	}
	return result
}

func geminiPartSegment(object map[string]any, role string, msgIndex, blockIndex int) []AuditSegment {
	if text := stringValue(object["text"]); text != "" {
		seg := textSegment(text, responsesRole(role), "gemini")
		seg.MessageIndex, seg.BlockIndex = msgIndex, blockIndex
		return []AuditSegment{seg}
	}
	if call, ok := object["functionCall"].(map[string]any); ok {
		name := stringValue(call["name"])
		args := toolArgumentsText(call["args"])
		text := strings.TrimSpace(name + " " + args)
		if text != "" {
			return []AuditSegment{{
				Kind: SegmentToolCall, Role: "assistant", ToolName: name, Text: text,
				MessageIndex: msgIndex, BlockIndex: blockIndex, SourceType: "gemini_tool_call",
			}}
		}
	}
	if resp, ok := object["functionResponse"].(map[string]any); ok {
		name := stringValue(resp["name"])
		text := toolArgumentsText(resp["response"])
		if strings.TrimSpace(text) != "" {
			return []AuditSegment{{
				Kind: SegmentToolResult, Role: "tool", ToolName: name, Text: text,
				MessageIndex: msgIndex, BlockIndex: blockIndex, SourceType: "gemini_tool_result",
			}}
		}
	}
	// executableCode is model-generated code: audit its language + source.
	if code, ok := object["executableCode"].(map[string]any); ok {
		text := strings.TrimSpace(stringValue(code["language"]) + "\n" + stringValue(code["code"]))
		if text != "" {
			return []AuditSegment{{
				Kind: SegmentUnknown, Role: "assistant", Text: text,
				MessageIndex: msgIndex, BlockIndex: blockIndex, SourceType: "gemini_executable_code",
			}}
		}
	}
	// codeExecutionResult is a tool-like result: audit its output text.
	if res, ok := object["codeExecutionResult"].(map[string]any); ok {
		text := toolArgumentsText(res)
		if strings.TrimSpace(text) != "" {
			return []AuditSegment{{
				Kind: SegmentToolResult, Role: "tool", Text: text,
				MessageIndex: msgIndex, BlockIndex: blockIndex, SourceType: "gemini_code_result",
			}}
		}
	}
	// inlineData / fileData are known-ignored binary parts.
	for _, key := range []string{"inlineData", "inline_data", "fileData", "file_data"} {
		if _, ok := object[key]; ok {
			return nil
		}
	}
	// Any other non-empty part is audited as an unknown block.
	if len(object) > 0 {
		if seg, ok := unknownObjectSegment(object, responsesRole(role), msgIndex, blockIndex, "gemini_unknown"); ok {
			return []AuditSegment{seg}
		}
	}
	return nil
}

func extractGeminiRoot(root map[string]any) []AuditSegment {
	if root == nil {
		return nil
	}
	result := extractGeminiSystemInstruction(root["systemInstruction"])
	result = append(result, extractGeminiSystemInstruction(root["system_instruction"])...)
	result = append(result, extractGemini(root["contents"])...)
	result = append(result, extractGemini(root["content"])...)
	result = append(result, extractGeminiInstances(root["instances"])...)
	if requests, ok := root["requests"].([]any); ok {
		for _, item := range requests {
			request, ok := item.(map[string]any)
			if !ok {
				continue
			}
			result = append(result, extractGeminiSystemInstruction(request["systemInstruction"])...)
			result = append(result, extractGeminiSystemInstruction(request["system_instruction"])...)
			result = append(result, extractGemini(request["contents"])...)
			result = append(result, extractGemini(request["content"])...)
			result = append(result, extractGeminiInstances(request["instances"])...)
		}
	}
	return result
}

func extractGeminiSystemInstruction(value any) []AuditSegment {
	switch typed := value.(type) {
	case string:
		if text := strings.TrimSpace(typed); text != "" {
			return []AuditSegment{textSegment(text, "system", "gemini_system")}
		}
	case map[string]any:
		if parts, ok := typed["parts"].([]any); ok {
			result := make([]AuditSegment, 0, len(parts))
			for _, part := range parts {
				if object, ok := part.(map[string]any); ok {
					if text := stringValue(object["text"]); text != "" {
						result = append(result, textSegment(text, "system", "gemini_system"))
					}
				}
			}
			return result
		}
		return systemPromptSegments(contentTexts(typed))
	case []any:
		segments := extractGemini(typed)
		for index := range segments {
			segments[index].Kind = SegmentSystemText
			segments[index].Role = "system"
		}
		return segments
	}
	return nil
}

func extractGeminiInstances(value any) []AuditSegment {
	instances, ok := value.([]any)
	if !ok {
		return nil
	}
	result := make([]AuditSegment, 0, len(instances))
	for _, item := range instances {
		if instance, ok := item.(map[string]any); ok {
			if prompt := stringValue(instance["prompt"]); prompt != "" {
				result = append(result, textSegment(prompt, "user", "gemini_instance"))
			}
		}
	}
	return result
}

// ---- Media / images -----------------------------------------------------------

func extractMediaPrompts(root map[string]any) []string {
	if root == nil {
		return nil
	}
	result := make([]string, 0, 4)
	seen := map[string]struct{}{}
	var walk func(any, string)
	walk = func(value any, key string) {
		switch typed := value.(type) {
		case map[string]any:
			keys := make([]string, 0, len(typed))
			for childKey := range typed {
				keys = append(keys, childKey)
			}
			sort.Strings(keys)
			for _, childKey := range keys {
				walk(typed[childKey], childKey)
			}
		case []any:
			for _, item := range typed {
				walk(item, key)
			}
		case string:
			if !isMediaPromptKey(key) || looksLikeMediaPayload(typed) {
				return
			}
			text := strings.TrimSpace(typed)
			if text == "" {
				return
			}
			if _, duplicate := seen[text]; duplicate {
				return
			}
			seen[text] = struct{}{}
			result = append(result, text)
		}
	}
	walk(root, "")
	return result
}

func isMediaPromptKey(key string) bool {
	normalized := strings.NewReplacer("_", "", "-", "").Replace(strings.ToLower(strings.TrimSpace(key)))
	switch normalized {
	case "prompt", "inputprompt", "textprompt", "description", "query", "lyrics", "negativeprompt",
		"positiveprompt", "gptdescriptionprompt", "prompten", "finalprompt", "finalzhprompt",
		"origprompt", "actualprompt", "imageprompt", "input":
		return true
	default:
		return false
	}
}

func looksLikeMediaPayload(value string) bool {
	trimmed := strings.TrimSpace(value)
	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "data:image/") || strings.HasPrefix(lower, "data:video/") ||
		strings.HasPrefix(lower, "data:audio/") ||
		strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return true
	}
	if len(trimmed) >= 256 {
		for _, r := range trimmed {
			alphaNumeric := (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
			if !alphaNumeric && r != '+' && r != '/' && r != '=' {
				return false
			}
		}
		return true
	}
	return false
}
