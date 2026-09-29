package service

import (
	"encoding/json"
	"regexp"
	"strings"
)

const (
	MaxStoredUserPromptRunes = 4096
	userPromptTruncationMark = "...[truncated]"
)

var (
	codexLocalImageBlockPattern = regexp.MustCompile(`(?is)<image\s+[^>]*name=\[Image\s+#\d+\][^>]*path="[^"\r\n]+"[^>]*>\s*</image>`)
	codexImageReferencePattern  = regexp.MustCompile(`(?i)\[Image\s+#\d+\]`)
)

// ExtractUserPrompt returns the latest user-authored text in a supported API
// request. It intentionally ignores system/developer/assistant/tool content.
func ExtractUserPrompt(body []byte, protocol string) *string {
	var root map[string]any
	if len(body) == 0 || json.Unmarshal(body, &root) != nil {
		return nil
	}
	var prompt string
	switch strings.ToLower(strings.TrimSpace(protocol)) {
	case "responses", "openai_responses":
		prompt = latestUserFromResponsesInput(root["input"])
	case "openai_embeddings", "embeddings":
		prompt = latestUserFromValue(root["input"])
	case "gemini", "gemini_generate_content":
		prompt = latestUserFromArray(root["contents"])
	case "anthropic", "messages", "chat_completions", "openai_chat":
		prompt = latestUserFromArray(root["messages"])
	default:
		prompt = latestUserFromArray(root["messages"])
		if prompt == "" {
			prompt = latestUserFromArray(root["contents"])
		}
	}
	if prompt == "" {
		// Image/media APIs use a top-level prompt and have no message history.
		if value, ok := root["prompt"].(string); ok {
			prompt = value
		}
	}
	return NormalizeUserPrompt(prompt)
}

func latestUserFromResponsesInput(value any) string {
	if prompt, ok := value.(string); ok {
		return prompt
	}
	return latestUserFromArray(value)
}

// NormalizeUserPrompt trims whitespace and caps stored data by Unicode code
// points so UTF-8 text is never split in the middle of a character.
func NormalizeUserPrompt(prompt string) *string {
	prompt = removeCodexImageAttachmentMetadata(prompt)
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return nil
	}
	runes := []rune(prompt)
	if len(runes) > MaxStoredUserPromptRunes {
		mark := []rune(userPromptTruncationMark)
		limit := MaxStoredUserPromptRunes - len(mark)
		if limit < 0 {
			limit = 0
		}
		prompt = string(runes[:limit]) + userPromptTruncationMark
	}
	return &prompt
}

func removeCodexImageAttachmentMetadata(prompt string) string {
	if !codexLocalImageBlockPattern.MatchString(prompt) {
		return prompt
	}
	prompt = codexLocalImageBlockPattern.ReplaceAllString(prompt, "")
	return codexImageReferencePattern.ReplaceAllString(prompt, "")
}

func latestUserFromArray(value any) string {
	items, ok := value.([]any)
	if !ok {
		return ""
	}
	for i := len(items) - 1; i >= 0; i-- {
		item, ok := items[i].(map[string]any)
		if !ok {
			continue
		}
		role, _ := item["role"].(string)
		if strings.EqualFold(strings.TrimSpace(role), "user") {
			return latestUserFromValue(item["content"])
		}
	}
	return ""
}

func latestUserFromValue(value any) string {
	switch value := value.(type) {
	case string:
		return value
	case []any:
		parts := make([]string, 0, len(value))
		for _, item := range value {
			if text := textPart(item); text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "\n")
	case map[string]any:
		if role, _ := value["role"].(string); strings.EqualFold(strings.TrimSpace(role), "user") {
			return latestUserFromValue(value["content"])
		}
		return textPart(value)
	default:
		return ""
	}
}

func textPart(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	item, ok := value.(map[string]any)
	if !ok {
		return ""
	}
	for _, key := range []string{"text", "input_text", "value"} {
		if text, ok := item[key].(string); ok {
			return text
		}
	}
	if nested, ok := item["content"]; ok {
		return latestUserFromValue(nested)
	}
	return ""
}
