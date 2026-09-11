package securityaudit

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strings"
	"unicode/utf8"
)

const PromptResponseTextLimit = 256 * 1024

type ResponseTextExtraction struct {
	Text       string
	Length     int
	Recognized bool
	Truncated  bool
}

// ExtractResponseText extracts generated assistant text from supported JSON and
// SSE response shapes. Raw response envelopes are never returned for storage.
func ExtractResponseText(body []byte, captureTruncated bool) ResponseTextExtraction {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return ResponseTextExtraction{}
	}
	var text string
	var recognized bool
	if bytes.HasPrefix(trimmed, []byte("data:")) || bytes.Contains(trimmed, []byte("\ndata:")) {
		text, recognized = extractSSEResponseText(trimmed)
	} else {
		var payload any
		if json.Unmarshal(trimmed, &payload) == nil {
			text, recognized = extractJSONResponseText(payload)
		}
	}
	originalLength := utf8.RuneCountInString(text)
	text, textTruncated := truncateUTF8(text, PromptResponseTextLimit)
	return ResponseTextExtraction{
		Text: text, Length: originalLength, Recognized: recognized,
		Truncated: captureTruncated || textTruncated,
	}
}

func extractSSEResponseText(body []byte) (string, bool) {
	scanner := bufio.NewScanner(bytes.NewReader(body))
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	var deltas, final []string
	recognized := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}
		var payload any
		if json.Unmarshal([]byte(data), &payload) != nil {
			continue
		}
		parts, ok := extractStreamingDelta(payload)
		if ok {
			recognized = true
			deltas = append(deltas, parts...)
			continue
		}
		finalText, finalOK := extractJSONResponseText(payload)
		if finalOK {
			recognized = true
			final = partsIfText(finalText)
		}
	}
	if len(deltas) > 0 {
		return strings.Join(deltas, ""), true
	}
	return strings.Join(final, ""), recognized
}

func extractStreamingDelta(payload any) ([]string, bool) {
	root, ok := payload.(map[string]any)
	if !ok {
		return nil, false
	}
	typeName, _ := root["type"].(string)
	switch typeName {
	case "response.output_text.delta":
		value, _ := root["delta"].(string)
		return []string{value}, true
	case "content_block_delta":
		if delta, ok := root["delta"].(map[string]any); ok {
			value, _ := delta["text"].(string)
			return []string{value}, true
		}
		return nil, true
	}
	if choices, exists := root["choices"]; exists {
		parts := extractChoiceText(choices, true)
		return parts, true
	}
	if candidates, exists := root["candidates"]; exists {
		return extractCandidateText(candidates), true
	}
	return nil, false
}

func extractJSONResponseText(payload any) (string, bool) {
	root, ok := payload.(map[string]any)
	if !ok {
		if values, isArray := payload.([]any); isArray {
			parts := make([]string, 0)
			recognized := false
			for _, value := range values {
				text, found := extractJSONResponseText(value)
				if found {
					recognized = true
					parts = append(parts, text)
				}
			}
			return strings.Join(parts, ""), recognized
		}
		return "", false
	}
	if value, exists := root["output_text"]; exists {
		text, _ := value.(string)
		return text, true
	}
	if choices, exists := root["choices"]; exists {
		return strings.Join(extractChoiceText(choices, false), ""), true
	}
	if content, exists := root["content"]; exists {
		return strings.Join(extractContentText(content), ""), true
	}
	if output, exists := root["output"]; exists {
		return strings.Join(extractOutputText(output), ""), true
	}
	if candidates, exists := root["candidates"]; exists {
		return strings.Join(extractCandidateText(candidates), ""), true
	}
	return "", false
}

func extractChoiceText(value any, deltaOnly bool) []string {
	choices, _ := value.([]any)
	parts := make([]string, 0, len(choices))
	for _, rawChoice := range choices {
		choice, _ := rawChoice.(map[string]any)
		if delta, ok := choice["delta"].(map[string]any); ok {
			parts = append(parts, extractTextValue(delta["content"])...)
			continue
		}
		if deltaOnly {
			continue
		}
		if message, ok := choice["message"].(map[string]any); ok {
			parts = append(parts, extractTextValue(message["content"])...)
			continue
		}
		if text, ok := choice["text"].(string); ok {
			parts = append(parts, text)
		}
	}
	return parts
}

func extractContentText(value any) []string {
	return extractTextValue(value)
}

func extractOutputText(value any) []string {
	items, _ := value.([]any)
	parts := make([]string, 0)
	for _, rawItem := range items {
		item, _ := rawItem.(map[string]any)
		parts = append(parts, extractTextValue(item["content"])...)
	}
	return parts
}

func extractCandidateText(value any) []string {
	candidates, _ := value.([]any)
	parts := make([]string, 0)
	for _, rawCandidate := range candidates {
		candidate, _ := rawCandidate.(map[string]any)
		content, _ := candidate["content"].(map[string]any)
		parts = append(parts, extractTextValue(content["parts"])...)
	}
	return parts
}

func extractTextValue(value any) []string {
	switch typed := value.(type) {
	case string:
		return []string{typed}
	case []any:
		parts := make([]string, 0)
		for _, item := range typed {
			parts = append(parts, extractTextValue(item)...)
		}
		return parts
	case map[string]any:
		if text, ok := typed["text"].(string); ok {
			return []string{text}
		}
	}
	return nil
}

func partsIfText(text string) []string {
	if text == "" {
		return nil
	}
	return []string{text}
}

func truncateUTF8(value string, maxBytes int) (string, bool) {
	if maxBytes <= 0 || len(value) <= maxBytes {
		return value, false
	}
	value = value[:maxBytes]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value, true
}
