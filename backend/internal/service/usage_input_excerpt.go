package service

import (
	"encoding/json"
	"strings"

	"github.com/tidwall/gjson"
)

const maxUsageInputExcerptRunes = 240

func BuildUsageInputExcerpt(body []byte) string {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return ""
	}

	candidates := []string{
		extractUsageExcerptFromMessages(body),
		extractUsageExcerptFromResponsesInput(body),
		extractUsageExcerptFromStringOrArray(body, "prompt"),
		extractUsageExcerptFromStringOrArray(body, "input"),
		extractUsageExcerptFromContents(body),
	}
	for _, candidate := range candidates {
		if excerpt := sanitizeUsageInputExcerpt(candidate); excerpt != "" {
			return excerpt
		}
	}
	return ""
}

func sanitizeUsageInputExcerpt(text string) string {
	text = normalizeUsageExcerptWhitespace(text)
	if text == "" {
		return ""
	}
	return trimRunes(redactContentModerationSecrets(text), maxUsageInputExcerptRunes)
}

func normalizeUsageExcerptWhitespace(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	return strings.Join(strings.Fields(text), " ")
}

func extractUsageExcerptFromMessages(body []byte) string {
	messages := gjson.GetBytes(body, "messages")
	if !messages.Exists() || !messages.IsArray() {
		return ""
	}
	for i := len(messages.Array()) - 1; i >= 0; i-- {
		msg := messages.Array()[i]
		role := strings.TrimSpace(msg.Get("role").String())
		if role != "" && role != "user" {
			continue
		}
		if text := collectUsageText(msg.Get("content")); text != "" {
			return text
		}
	}
	for i := len(messages.Array()) - 1; i >= 0; i-- {
		if text := collectUsageText(messages.Array()[i].Get("content")); text != "" {
			return text
		}
	}
	return ""
}

func extractUsageExcerptFromResponsesInput(body []byte) string {
	input := gjson.GetBytes(body, "input")
	if !input.Exists() {
		return ""
	}
	if input.Type == gjson.String {
		return input.String()
	}
	if input.IsArray() {
		items := input.Array()
		for i := len(items) - 1; i >= 0; i-- {
			item := items[i]
			role := strings.TrimSpace(item.Get("role").String())
			if role != "" && role != "user" {
				continue
			}
			if text := collectUsageText(item.Get("content")); text != "" {
				return text
			}
			if text := collectUsageText(item); text != "" {
				return text
			}
		}
		for i := len(items) - 1; i >= 0; i-- {
			if text := collectUsageText(items[i]); text != "" {
				return text
			}
		}
	}
	return collectUsageText(input)
}

func extractUsageExcerptFromContents(body []byte) string {
	contents := gjson.GetBytes(body, "contents")
	if !contents.Exists() || !contents.IsArray() {
		return ""
	}
	items := contents.Array()
	for i := len(items) - 1; i >= 0; i-- {
		item := items[i]
		role := strings.TrimSpace(item.Get("role").String())
		if role != "" && role != "user" {
			continue
		}
		if text := collectUsageText(item.Get("parts")); text != "" {
			return text
		}
	}
	for i := len(items) - 1; i >= 0; i-- {
		if text := collectUsageText(items[i].Get("parts")); text != "" {
			return text
		}
	}
	return ""
}

func extractUsageExcerptFromStringOrArray(body []byte, path string) string {
	value := gjson.GetBytes(body, path)
	if !value.Exists() {
		return ""
	}
	return collectUsageText(value)
}

func collectUsageText(value gjson.Result) string {
	if !value.Exists() {
		return ""
	}
	switch value.Type {
	case gjson.String:
		return value.String()
	case gjson.JSON:
		if value.IsArray() {
			var parts []string
			for _, item := range value.Array() {
				if text := collectUsageText(item); text != "" {
					parts = append(parts, text)
				}
			}
			return strings.Join(parts, " ")
		}
		for _, key := range []string{"text", "input_text", "content", "prompt", "query"} {
			if text := collectUsageText(value.Get(key)); text != "" {
				return text
			}
		}
		if kind := strings.TrimSpace(value.Get("type").String()); kind != "" {
			switch kind {
			case "text", "input_text":
				if text := strings.TrimSpace(value.Get("text").String()); text != "" {
					return text
				}
			case "message":
				if text := collectUsageText(value.Get("content")); text != "" {
					return text
				}
			}
		}
		if raw := strings.TrimSpace(value.Raw); raw != "" && raw != "{}" && raw != "[]" {
			var generic any
			if err := json.Unmarshal([]byte(raw), &generic); err == nil {
				switch typed := generic.(type) {
				case string:
					return typed
				}
			}
		}
	}
	return ""
}
