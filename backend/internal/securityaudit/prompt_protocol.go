package securityaudit

import (
	"encoding/json"
	"strings"
)

// clientInstructionRoles are roles a client may freely populate. Attackers can
// place jailbreak/PII text in assistant/tool turns, so blocking audit must scan
// them too—not only user/system/developer instructions.
var clientInstructionRoles = []string{"user", "system", "developer", "assistant", "tool"}

func isClientInstructionRole(role string) bool {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "user", "system", "developer", "assistant", "tool", "model":
		return true
	default:
		return false
	}
}

// kindForRole maps a message role to a segment kind for plain text content.
func kindForRole(role string) SegmentKind {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "user":
		return SegmentHumanText
	case "assistant", "model":
		return SegmentAssistantText
	case "system", "developer":
		return SegmentSystemText
	case "tool":
		return SegmentToolResult
	default:
		return SegmentHumanText
	}
}

// textSegment builds a standalone text segment. MessageIndex -1 marks it as not
// belonging to a numbered message array (system/instructions/media), so the
// latest-turn selector never groups it with a real message turn.
func textSegment(text, role, source string) AuditSegment {
	return AuditSegment{Kind: kindForRole(role), Role: role, Text: text, SourceType: source, MessageIndex: -1}
}

// messageTextSegment builds a text segment that belongs to a real message array
// entry, carrying that message's index so the latest-turn selector groups a
// message's parts together and never merges distinct messages that happen to be
// plain strings.
func messageTextSegment(text, role, source string, msgIndex int) AuditSegment {
	seg := textSegment(text, role, source)
	seg.MessageIndex = msgIndex
	return seg
}

func promptSegmentsForRole(texts []string, role string) []AuditSegment {
	// Empty role in responses/gemini/media contexts means the user turn.
	effectiveRole := role
	if effectiveRole == "" {
		effectiveRole = "user"
	}
	result := make([]AuditSegment, 0, len(texts))
	for _, text := range texts {
		result = append(result, textSegment(text, effectiveRole, "message"))
	}
	return result
}

func userPromptSegments(texts []string) []AuditSegment { return promptSegmentsForRole(texts, "user") }
func systemPromptSegments(texts []string) []AuditSegment {
	return promptSegmentsForRole(texts, "system")
}

func extractProtocolSegments(protocol string, document any) []AuditSegment {
	root, _ := document.(map[string]any)
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	switch protocol {
	case "openai_chat_completions", "openai_chat", "chat_completions":
		return extractChatLikeSegments(root)
	case "anthropic_messages", "claude_messages", "messages":
		return append(extractAnthropicSystem(root["system"]), extractAnthropicMessages(root["messages"])...)
	case "gemini", "gemini_generate_content":
		return extractGeminiRoot(root)
	case "openai_responses", "responses", "responses_websocket":
		if frameType := stringValue(root["type"]); frameType != "" || protocol == "responses_websocket" {
			if frameType != "response.create" {
				return nil
			}
			if input, exists := root["input"]; exists && input != nil {
				return append(extractInstructions(root["instructions"]), extractResponses(input)...)
			}
			if response, ok := root["response"].(map[string]any); ok {
				return append(extractInstructions(response["instructions"]), extractResponses(response["input"])...)
			}
			return extractInstructions(root["instructions"])
		}
		return append(extractInstructions(root["instructions"]), extractResponses(root["input"])...)
	case "openai_images", "grok_media", "media", "images":
		return userPromptSegments(extractMediaPrompts(root))
	default:
		if segments := extractChatLikeSegments(root); len(segments) > 0 {
			return segments
		}
		if responses := append(extractInstructions(root["instructions"]), extractResponses(root["input"])...); len(responses) > 0 {
			return responses
		}
		if gemini := extractGeminiRoot(root); len(gemini) > 0 {
			return gemini
		}
		return userPromptSegments(extractMediaPrompts(root))
	}
}

// ---- OpenAI Chat Completions --------------------------------------------------

func extractChatLikeSegments(root map[string]any) []AuditSegment {
	if root == nil {
		return nil
	}
	items, ok := root["messages"].([]any)
	if !ok {
		return nil
	}
	wanted := make(map[string]struct{}, len(clientInstructionRoles))
	for _, role := range clientInstructionRoles {
		wanted[role] = struct{}{}
	}
	result := make([]AuditSegment, 0, len(items))
	for msgIndex, item := range items {
		message, ok := item.(map[string]any)
		if !ok {
			continue
		}
		role := strings.ToLower(stringValue(message["role"]))
		if _, match := wanted[role]; !match {
			continue
		}
		if role == "tool" {
			// A tool result is opaque data: marshal it (stripping binary and
			// escaping angle brackets) instead of scanning it as raw text.
			if text := toolArgumentsText(message["content"]); text != "" {
				result = append(result, AuditSegment{
					Kind: SegmentToolResult, Role: "tool", ToolCallID: stringValue(message["tool_call_id"]),
					Text: text, MessageIndex: msgIndex, SourceType: "chat_tool_result",
				})
			}
			continue
		}
		for blockIndex, text := range contentTexts(message["content"]) {
			result = append(result, AuditSegment{
				Kind: kindForRole(role), Text: text, Role: role,
				MessageIndex: msgIndex, BlockIndex: blockIndex, SourceType: "chat",
			})
		}
		// Non-text, non-ignored content blocks are audited as unknown blocks.
		result = append(result, unknownContentBlocks(message["content"], role, msgIndex, "chat_unknown")...)
		// assistant.tool_calls[].function.{name,arguments} were previously dropped;
		// they carry the model's actual tool intent and must be audited.
		if role == "assistant" {
			result = append(result, chatToolCallSegments(message["tool_calls"], msgIndex)...)
		}
	}
	return result
}

func chatToolCallSegments(value any, msgIndex int) []AuditSegment {
	calls, ok := value.([]any)
	if !ok {
		return nil
	}
	result := make([]AuditSegment, 0, len(calls))
	for blockIndex, item := range calls {
		call, ok := item.(map[string]any)
		if !ok {
			continue
		}
		function, _ := call["function"].(map[string]any)
		name := stringValue(function["name"])
		args := toolArgumentsText(function["arguments"])
		text := strings.TrimSpace(name + " " + args)
		if text == "" {
			continue
		}
		result = append(result, AuditSegment{
			Kind: SegmentToolCall, Role: "assistant", ToolName: name, ToolCallID: stringValue(call["id"]),
			Text: text, MessageIndex: msgIndex, BlockIndex: blockIndex, SourceType: "chat_tool_call",
		})
	}
	return result
}

// ---- OpenAI Responses ---------------------------------------------------------

func extractInstructions(value any) []AuditSegment {
	switch typed := value.(type) {
	case string:
		if text := strings.TrimSpace(typed); text != "" {
			return []AuditSegment{textSegment(text, "system", "instructions")}
		}
	case []any:
		return systemPromptSegments(contentTexts(typed))
	case map[string]any:
		return systemPromptSegments(contentTexts(typed))
	}
	return nil
}

func extractResponses(value any) []AuditSegment {
	switch typed := value.(type) {
	case string:
		return []AuditSegment{textSegment(typed, "user", "responses")}
	case []any:
		result := make([]AuditSegment, 0, len(typed))
		for msgIndex, item := range typed {
			switch entry := item.(type) {
			case string:
				result = append(result, messageTextSegment(entry, "user", "responses", msgIndex))
			case map[string]any:
				if segs := responsesToolSegments(entry, msgIndex); segs != nil {
					result = append(result, segs...)
					continue
				}
				role := strings.ToLower(stringValue(entry["role"]))
				if role != "" && !isClientInstructionRole(role) {
					continue
				}
				emitted := 0
				if content, exists := entry["content"]; exists {
					for blockIndex, text := range contentTexts(content) {
						seg := textSegment(text, responsesRole(role), "responses")
						seg.MessageIndex, seg.BlockIndex = msgIndex, blockIndex
						result = append(result, seg)
						emitted++
					}
					// Unknown blocks nested inside a message's content array are
					// audited too, so a message whose content is only unknown blocks
					// no longer forms an empty audit.
					for _, seg := range unknownContentBlocks(content, responsesRole(role), msgIndex, "responses_unknown") {
						result = append(result, seg)
						emitted++
					}
				} else if text := stringValue(entry["text"]); text != "" {
					seg := textSegment(text, responsesRole(role), "responses")
					seg.MessageIndex = msgIndex
					result = append(result, seg)
					emitted++
				}
				// A typed non-message item that yielded no text and is not
				// known-ignored is audited as an unknown block rather than dropped.
				if itemType := strings.ToLower(stringValue(entry["type"])); emitted == 0 && itemType != "" && itemType != "message" && !isKnownIgnoredContentType(itemType) {
					if seg, ok := unknownObjectSegment(entry, responsesRole(role), msgIndex, 0, "responses_unknown"); ok {
						result = append(result, seg)
					}
				}
			}
		}
		return result
	case map[string]any:
		if segs := responsesToolSegments(typed, 0); segs != nil {
			return segs
		}
		role := strings.ToLower(stringValue(typed["role"]))
		if role != "" && !isClientInstructionRole(role) {
			return nil
		}
		// A single top-level message object is message index 0. Also audit any
		// unknown blocks nested in its content.
		segs := promptSegmentsForRole(contentTexts(typed["content"]), role)
		for i := range segs {
			segs[i].MessageIndex = 0
		}
		segs = append(segs, unknownContentBlocks(typed["content"], responsesRole(role), 0, "responses_unknown")...)
		return segs
	}
	return nil
}

func responsesRole(role string) string {
	if role == "" {
		return "user"
	}
	return role
}

// responsesToolSegments handles Responses function_call / function_call_output
// items, which are typed items rather than role-bearing messages and were
// previously discarded entirely (design §8.2). Returns nil for non-tool items.
func responsesToolSegments(entry map[string]any, msgIndex int) []AuditSegment {
	switch strings.ToLower(stringValue(entry["type"])) {
	case "function_call":
		name := stringValue(entry["name"])
		args := toolArgumentsText(entry["arguments"])
		text := strings.TrimSpace(name + " " + args)
		if text == "" {
			return []AuditSegment{}
		}
		return []AuditSegment{{
			Kind: SegmentToolCall, Role: "assistant", ToolName: name, ToolCallID: stringValue(entry["call_id"]),
			Text: text, MessageIndex: msgIndex, SourceType: "responses_tool_call",
		}}
	case "function_call_output":
		text := toolArgumentsText(entry["output"])
		if strings.TrimSpace(text) == "" {
			return []AuditSegment{}
		}
		return []AuditSegment{{
			Kind: SegmentToolResult, Role: "tool", ToolCallID: stringValue(entry["call_id"]),
			Text: text, MessageIndex: msgIndex, SourceType: "responses_tool_result",
		}}
	default:
		return nil
	}
}

// ---- Anthropic Messages -------------------------------------------------------

func extractAnthropicSystem(value any) []AuditSegment {
	switch typed := value.(type) {
	case string:
		if text := strings.TrimSpace(typed); text != "" {
			return []AuditSegment{textSegment(text, "system", "anthropic_system")}
		}
	case []any:
		return systemPromptSegments(contentTexts(typed))
	case map[string]any:
		return systemPromptSegments(contentTexts(typed))
	}
	return nil
}

func extractAnthropicMessages(value any) []AuditSegment {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	result := make([]AuditSegment, 0, len(items))
	for msgIndex, item := range items {
		message, ok := item.(map[string]any)
		if !ok {
			continue
		}
		role := strings.ToLower(stringValue(message["role"]))
		if role != "" && !isClientInstructionRole(role) {
			continue
		}
		result = append(result, anthropicContentSegments(message["content"], role, msgIndex)...)
	}
	return result
}

func anthropicContentSegments(content any, role string, msgIndex int) []AuditSegment {
	switch typed := content.(type) {
	case string:
		if text := strings.TrimSpace(typed); text != "" {
			return []AuditSegment{messageTextSegment(text, role, "anthropic", msgIndex)}
		}
		return nil
	case []any:
		result := make([]AuditSegment, 0, len(typed))
		for blockIndex, item := range typed {
			block, ok := item.(map[string]any)
			if !ok {
				continue
			}
			blockType := strings.ToLower(stringValue(block["type"]))
			switch blockType {
			case "", "text", "input_text", "output_text":
				if text := stringValue(block["text"]); text != "" {
					seg := textSegment(text, role, "anthropic")
					seg.MessageIndex, seg.BlockIndex = msgIndex, blockIndex
					result = append(result, seg)
				}
			case "tool_use":
				name := stringValue(block["name"])
				args := toolArgumentsText(block["input"])
				text := strings.TrimSpace(name + " " + args)
				if text != "" {
					result = append(result, AuditSegment{
						Kind: SegmentToolCall, Role: "assistant", ToolName: name, ToolCallID: stringValue(block["id"]),
						Text: text, MessageIndex: msgIndex, BlockIndex: blockIndex, SourceType: "anthropic_tool_use",
					})
				}
			case "tool_result":
				text := toolArgumentsText(block["content"])
				if strings.TrimSpace(text) != "" {
					result = append(result, AuditSegment{
						Kind: SegmentToolResult, Role: "tool", ToolCallID: stringValue(block["tool_use_id"]),
						Text: text, MessageIndex: msgIndex, BlockIndex: blockIndex, SourceType: "anthropic_tool_result",
					})
				}
			default:
				// thinking / redacted_thinking / image stay silent; any other block
				// is audited as an unknown block rather than dropped.
				if !isKnownIgnoredContentType(blockType) {
					if seg, ok := unknownObjectSegment(block, role, msgIndex, blockIndex, "anthropic_unknown"); ok {
						result = append(result, seg)
					}
				}
			}
		}
		return result
	case map[string]any:
		if text := stringValue(typed["text"]); text != "" {
			return []AuditSegment{messageTextSegment(text, role, "anthropic", msgIndex)}
		}
	}
	return nil
}

// ---- Shared content helpers ---------------------------------------------------

// contentTexts extracts plain text parts. It intentionally recognizes only
// text/input_text/output_text so binary/tool blocks are handled by the dedicated
// tool extractors above rather than being scanned as if they were text.
func contentTexts(value any) []string {
	switch typed := value.(type) {
	case string:
		return []string{typed}
	case []any:
		result := make([]string, 0, len(typed))
		for _, part := range typed {
			object, ok := part.(map[string]any)
			if !ok {
				continue
			}
			typeName := strings.ToLower(stringValue(object["type"]))
			if typeName != "" && typeName != "text" && typeName != "input_text" && typeName != "output_text" {
				continue
			}
			if text := stringValue(object["text"]); text != "" {
				result = append(result, text)
			}
		}
		return result
	case map[string]any:
		if text := stringValue(typed["text"]); text != "" {
			return []string{text}
		}
	}
	return nil
}

// unknownContentBlocks audits non-text, non-known-ignored blocks in a content
// array (used by Chat) so a novel/executable block is not silently dropped.
func unknownContentBlocks(content any, role string, msgIndex int, source string) []AuditSegment {
	blocks, ok := content.([]any)
	if !ok {
		return nil
	}
	result := make([]AuditSegment, 0)
	for blockIndex, item := range blocks {
		object, ok := item.(map[string]any)
		if !ok {
			continue
		}
		switch strings.ToLower(stringValue(object["type"])) {
		case "", "text", "input_text", "output_text":
			continue // plain text is handled by contentTexts
		}
		if isKnownIgnoredContentType(strings.ToLower(stringValue(object["type"]))) {
			continue
		}
		if seg, ok := unknownObjectSegment(object, role, msgIndex, blockIndex, source); ok {
			result = append(result, seg)
		}
	}
	return result
}

// toolArgumentsText normalizes a tool argument/result value to a safe string.
// JSON-string arguments are parsed and re-marshaled so nested binary is stripped
// and angle brackets are escaped; non-JSON strings are marshaled directly.
func toolArgumentsText(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return ""
		}
		var parsed any
		if json.Unmarshal([]byte(trimmed), &parsed) == nil {
			return marshalToolContent(parsed)
		}
		return marshalToolContent(trimmed)
	default:
		return marshalToolContent(typed)
	}
}
