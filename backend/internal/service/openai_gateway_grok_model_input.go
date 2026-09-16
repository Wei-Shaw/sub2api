package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// sanitizeGrokResponsesModelInput converts replay items from OpenAI and Chat
// shapes into the subset accepted by xAI's ModelInput decoder. Call IDs are
// assigned in a separate pass so an output can safely appear before its call.
func sanitizeGrokResponsesModelInput(body []byte) ([]byte, error) {
	input := gjson.GetBytes(body, "input")
	if !input.Exists() || input.Type == gjson.String {
		return body, nil
	}
	if input.IsObject() {
		var err error
		body, err = sjson.SetRawBytes(body, "input", []byte("["+input.Raw+"]"))
		if err != nil {
			return nil, fmt.Errorf("wrap Grok Responses model input: %w", err)
		}
		input = gjson.GetBytes(body, "input")
	}
	if !input.IsArray() {
		return body, nil
	}

	var items []any
	decoder := json.NewDecoder(bytes.NewReader([]byte(input.Raw)))
	decoder.UseNumber()
	if err := decoder.Decode(&items); err != nil {
		return nil, fmt.Errorf("parse Grok Responses model input: %w", err)
	}
	callIDs, outputIDs := pairGrokReplayCallIDs(items)
	filtered := make([]any, 0, len(items))
	pendingOutputImages := make([]grokToolOutputImage, 0)
	for index, rawItem := range items {
		item, ok := rawItem.(map[string]any)
		if !ok {
			filtered = appendGrokToolOutputImageMessage(filtered, pendingOutputImages)
			pendingOutputImages = pendingOutputImages[:0]
			if text, ok := rawItem.(string); ok && strings.TrimSpace(text) != "" {
				filtered = append(filtered, map[string]any{"type": "message", "role": "user", "content": text})
			}
			continue
		}

		itemType := strings.ToLower(strings.TrimSpace(grokStringValue(item["type"])))
		role := strings.ToLower(strings.TrimSpace(grokStringValue(item["role"])))
		if role == "tool" || role == "function" || isGrokReplayOutputType(itemType) {
			callID := outputIDs[index]
			rawOutput := firstNonNilGrokJSONValue(item["output"], item["content"], item["results"])
			output, nestedImages := normalizeGrokToolOutput(rawOutput, callID)
			filtered = append(filtered, map[string]any{
				"type":    "function_call_output",
				"call_id": callID,
				"output":  output,
			})
			pendingOutputImages = append(pendingOutputImages, extractGrokToolOutputImages(item, callID)...)
			pendingOutputImages = append(pendingOutputImages, nestedImages...)
			continue
		}
		filtered = appendGrokToolOutputImageMessage(filtered, pendingOutputImages)
		pendingOutputImages = pendingOutputImages[:0]

		switch itemType {
		case "text", "input_text", "output_text":
			text := strings.TrimSpace(grokStringValue(item["text"]))
			if text == "" {
				continue
			}
			if role == "" {
				role = "user"
			}
			item = map[string]any{"type": "message", "role": role, "content": text}
			itemType = "message"
		case "custom_tool_call", "tool_search_call":
			originalType := itemType
			item["type"] = "function_call"
			itemType = "function_call"
			if strings.TrimSpace(grokStringValue(item["name"])) == "" {
				if originalType == "tool_search_call" {
					item["name"] = "tool_search"
				}
			}
		}

		if itemType == "" && role != "" {
			itemType = "message"
			item["type"] = itemType
		}
		if itemType == "message" {
			if role == "" {
				role = "user"
				item["role"] = role
			}
			content, keep := sanitizeGrokMessageContent(item["content"])
			if !keep {
				continue
			}
			item["content"] = content
			if role == "assistant" && !grokIsCompleteOutputMessage(item) {
				if text, ok := collapseGrokAssistantOutputText(content); ok {
					item["content"] = text
				}
				delete(item, "id")
				delete(item, "status")
			}
		} else if itemType == "reasoning" {
			delete(item, "status")
			if content, exists := item["content"]; exists && content == nil {
				delete(item, "content")
			}
		} else if itemType == "function_call" {
			name := strings.TrimSpace(grokStringValue(item["name"]))
			arguments := item["arguments"]
			if function, ok := item["function"].(map[string]any); ok {
				if name == "" {
					name = strings.TrimSpace(grokStringValue(function["name"]))
				}
				if arguments == nil {
					arguments = function["arguments"]
				}
			}
			if arguments == nil {
				arguments = firstNonNilGrokJSONValue(item["input"], item["query"])
				if _, custom := item["input"]; custom {
					arguments = map[string]any{"input": arguments}
				}
			}
			if name == "" {
				name = "unknown_tool"
			}
			item["call_id"] = callIDs[index]
			item["name"] = name
			item["arguments"] = grokModelInputString(arguments, "{}")
			for _, field := range []string{"id", "status", "tool_call_id", "function", "input", "query", "execution"} {
				delete(item, field)
			}
		}
		if shouldStripOpenAIResponsesNonPairCallID(itemType) {
			delete(item, "call_id")
		}
		filtered = append(filtered, item)
	}
	filtered = appendGrokToolOutputImageMessage(filtered, pendingOutputImages)

	encoded, err := json.Marshal(filtered)
	if err != nil {
		return nil, fmt.Errorf("serialize Grok Responses model input: %w", err)
	}
	updated, err := sjson.SetRawBytes(body, "input", encoded)
	if err != nil {
		return nil, fmt.Errorf("set Grok Responses model input: %w", err)
	}
	return updated, nil
}

type grokToolOutputImage struct {
	callID string
	url    string
}

// Grok 1.0.5 returns read_file media as structured output parts rather than
// top-level images. xAI requires function_call_output.output to be a string,
// so remove those media parts before stringifying and lift them separately.
func normalizeGrokToolOutput(value any, callID string) (string, []grokToolOutputImage) {
	stripped, images, keep := stripGrokToolOutputImages(value, callID)
	if len(images) == 0 {
		return grokModelInputString(value, "(empty)"), nil
	}
	if !keep {
		return "(empty)", images
	}
	return grokStructuredToolOutputString(stripped, "(empty)"), images
}

func stripGrokToolOutputImages(value any, callID string) (any, []grokToolOutputImage, bool) {
	switch typed := value.(type) {
	case []any:
		filtered := make([]any, 0, len(typed))
		images := make([]grokToolOutputImage, 0)
		for _, item := range typed {
			stripped, nested, keep := stripGrokToolOutputImages(item, callID)
			images = append(images, nested...)
			if keep {
				filtered = append(filtered, stripped)
			}
		}
		return filtered, images, len(filtered) > 0
	case map[string]any:
		partType := strings.ToLower(strings.TrimSpace(grokStringValue(typed["type"])))
		if partType == "image" || partType == "image_url" || partType == "input_image" {
			url := grokToolOutputImageURL(typed)
			if url == "" || isEmptyBase64DataURI(url) {
				return nil, nil, false
			}
			return nil, []grokToolOutputImage{{callID: callID, url: url}}, false
		}

		filtered := make(map[string]any, len(typed))
		for key, item := range typed {
			filtered[key] = item
		}
		images := make([]grokToolOutputImage, 0)
		if rawImages, exists := filtered["images"]; exists {
			if values, ok := rawImages.([]any); ok {
				for _, rawImage := range values {
					url := grokToolOutputImageURL(rawImage)
					if url != "" && !isEmptyBase64DataURI(url) {
						images = append(images, grokToolOutputImage{callID: callID, url: url})
					}
				}
				delete(filtered, "images")
			}
		}
		for _, field := range []string{"content", "output", "results"} {
			item, exists := filtered[field]
			if !exists {
				continue
			}
			stripped, nested, keep := stripGrokToolOutputImages(item, callID)
			images = append(images, nested...)
			if keep {
				filtered[field] = stripped
			} else {
				delete(filtered, field)
			}
		}
		return filtered, images, len(filtered) > 0
	default:
		return value, nil, value != nil
	}
}

func grokStructuredToolOutputString(value any, fallback string) string {
	parts, ok := value.([]any)
	if !ok || len(parts) == 0 {
		return grokModelInputString(value, fallback)
	}

	texts := make([]string, 0, len(parts))
	for _, rawPart := range parts {
		part, ok := rawPart.(map[string]any)
		if !ok {
			return grokModelInputString(value, fallback)
		}
		partType := strings.ToLower(strings.TrimSpace(grokStringValue(part["type"])))
		if partType != "text" && partType != "input_text" && partType != "output_text" {
			return grokModelInputString(value, fallback)
		}
		if text := strings.TrimSpace(grokStringValue(part["text"])); text != "" {
			texts = append(texts, text)
		}
	}
	if len(texts) == 0 {
		return fallback
	}
	return strings.Join(texts, "\n")
}

// Grok Shell attaches local image results to function_call_output.images.
// xAI's ModelInput accepts only a string output, so keep the tool replies
// consecutive and lift their media into one following user input_image turn.
func extractGrokToolOutputImages(item map[string]any, callID string) []grokToolOutputImage {
	rawImages, ok := item["images"].([]any)
	if !ok || len(rawImages) == 0 {
		return nil
	}

	images := make([]grokToolOutputImage, 0, len(rawImages))
	for _, rawImage := range rawImages {
		url := grokToolOutputImageURL(rawImage)
		if url == "" || isEmptyBase64DataURI(url) {
			continue
		}
		images = append(images, grokToolOutputImage{callID: callID, url: url})
	}
	return images
}

func grokToolOutputImageURL(value any) string {
	switch image := value.(type) {
	case string:
		return strings.TrimSpace(image)
	case map[string]any:
		for _, field := range []string{"url", "image_url", "file_url"} {
			raw := image[field]
			switch typed := raw.(type) {
			case string:
				if url := strings.TrimSpace(typed); url != "" {
					return url
				}
			case map[string]any:
				if url := strings.TrimSpace(grokStringValue(typed["url"])); url != "" {
					return url
				}
			}
		}
	}
	return ""
}

func appendGrokToolOutputImageMessage(items []any, images []grokToolOutputImage) []any {
	if len(images) == 0 {
		return items
	}

	content := make([]any, 0, len(images)*2)
	lastCallID := ""
	for _, image := range images {
		if image.callID != lastCallID {
			content = append(content, map[string]any{
				"type": "input_text",
				"text": fmt.Sprintf("[Tool output media for call %s]", image.callID),
			})
			lastCallID = image.callID
		}
		content = append(content, map[string]any{
			"type":      "input_image",
			"image_url": image.url,
		})
	}
	return append(items, map[string]any{
		"type":    "message",
		"role":    "user",
		"content": content,
	})
}

func pairGrokReplayCallIDs(items []any) (map[int]string, map[int]string) {
	callIDs := make(map[int]string)
	outputIDs := make(map[int]string)
	aliases := make(map[string]string)
	conflictingAliases := make(map[string]struct{})
	pendingCalls := make([]string, 0)
	nextID := 0
	synthetic := func() string {
		nextID++
		return fmt.Sprintf("grok_replay_call_%d", nextID)
	}
	registerAlias := func(alias, canonical string) {
		alias = strings.TrimSpace(alias)
		if alias == "" {
			return
		}
		if _, conflict := conflictingAliases[alias]; conflict {
			return
		}
		if existing, exists := aliases[alias]; exists && existing != canonical {
			delete(aliases, alias)
			conflictingAliases[alias] = struct{}{}
			return
		}
		aliases[alias] = canonical
	}

	for index, rawItem := range items {
		item, ok := rawItem.(map[string]any)
		if !ok {
			continue
		}
		itemType := strings.ToLower(strings.TrimSpace(grokStringValue(item["type"])))
		if !isGrokReplayCallType(itemType) {
			continue
		}
		callID := firstNonEmptyGrokString(item["call_id"], item["tool_call_id"])
		if callID == "" {
			callID = synthetic()
		}
		callIDs[index] = callID
		pendingCalls = append(pendingCalls, callID)
		registerAlias(callID, callID)
		registerAlias(grokStringValue(item["call_id"]), callID)
		registerAlias(grokStringValue(item["tool_call_id"]), callID)
		registerAlias(grokStringValue(item["id"]), callID)
	}

	consumedCalls := make(map[string]struct{})
	consumeNextCall := func() string {
		for _, callID := range pendingCalls {
			if _, consumed := consumedCalls[callID]; consumed {
				continue
			}
			consumedCalls[callID] = struct{}{}
			return callID
		}
		return synthetic()
	}
	for index, rawItem := range items {
		item, ok := rawItem.(map[string]any)
		if !ok {
			continue
		}
		itemType := strings.ToLower(strings.TrimSpace(grokStringValue(item["type"])))
		role := strings.ToLower(strings.TrimSpace(grokStringValue(item["role"])))
		if role != "tool" && role != "function" && !isGrokReplayOutputType(itemType) {
			continue
		}
		alias := firstNonEmptyGrokString(item["call_id"], item["tool_call_id"], item["id"])
		_, conflict := conflictingAliases[alias]
		if canonical := aliases[alias]; canonical != "" && !conflict {
			outputIDs[index] = canonical
			consumedCalls[canonical] = struct{}{}
		} else if firstNonEmptyGrokString(item["call_id"], item["tool_call_id"]) != "" {
			// Explicit call identifiers remain authoritative even when an invalid
			// transcript reuses the same identifier for more than one call.
			outputIDs[index] = alias
		} else if conflict {
			// An item ID shared by multiple calls cannot safely identify either
			// one. Keep it out of the call_id namespace and leave the output orphaned.
			outputIDs[index] = synthetic()
		} else {
			outputIDs[index] = consumeNextCall()
		}
	}
	return callIDs, outputIDs
}

func isGrokReplayCallType(itemType string) bool {
	switch itemType {
	case "function_call", "custom_tool_call", "tool_search_call":
		return true
	default:
		return false
	}
}

func isGrokReplayOutputType(itemType string) bool {
	switch itemType {
	case "function_call_output", "custom_tool_call_output", "tool_search_output", "tool_search_call_output":
		return true
	default:
		return false
	}
}

func firstNonEmptyGrokString(values ...any) string {
	for _, value := range values {
		if text := strings.TrimSpace(grokStringValue(value)); text != "" {
			return text
		}
	}
	return ""
}

func firstNonNilGrokJSONValue(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func sanitizeGrokMessageContent(content any) (any, bool) {
	switch value := content.(type) {
	case nil:
		return nil, false
	case string:
		return value, strings.TrimSpace(value) != ""
	case []any:
		filtered := make([]any, 0, len(value))
		for _, rawPart := range value {
			part, ok := rawPart.(map[string]any)
			if !ok {
				if text, isText := rawPart.(string); !isText || strings.TrimSpace(text) != "" {
					filtered = append(filtered, rawPart)
				}
				continue
			}
			switch strings.ToLower(strings.TrimSpace(grokStringValue(part["type"]))) {
			case "text", "input_text", "output_text":
				if strings.TrimSpace(grokStringValue(part["text"])) == "" {
					continue
				}
			case "image_url", "input_image":
				if !grokContentPartHasImageURL(part) {
					continue
				}
			}
			filtered = append(filtered, part)
		}
		return filtered, len(filtered) > 0
	default:
		return content, true
	}
}

func grokContentPartHasImageURL(part map[string]any) bool {
	for _, field := range []string{"file_id", "file_data"} {
		if strings.TrimSpace(grokStringValue(part[field])) != "" {
			return true
		}
	}
	raw := firstNonNilGrokJSONValue(part["image_url"], part["file_url"])
	switch value := raw.(type) {
	case string:
		value = strings.TrimSpace(value)
		return value != "" && !isEmptyBase64DataURI(value)
	case map[string]any:
		url := strings.TrimSpace(grokStringValue(value["url"]))
		return url != "" && !isEmptyBase64DataURI(url)
	default:
		return false
	}
}

func grokStringValue(value any) string {
	text, _ := value.(string)
	return text
}

func grokModelInputString(value any, fallback string) string {
	if text, ok := value.(string); ok {
		if strings.TrimSpace(text) == "" {
			return fallback
		}
		return text
	}
	if value == nil {
		return fallback
	}
	encoded, err := json.Marshal(value)
	if err != nil || len(encoded) == 0 || string(encoded) == "null" {
		return fallback
	}
	return string(encoded)
}

func grokIsCompleteOutputMessage(item map[string]any) bool {
	return strings.TrimSpace(grokStringValue(item["id"])) != "" &&
		strings.TrimSpace(grokStringValue(item["status"])) != ""
}

func collapseGrokAssistantOutputText(content any) (string, bool) {
	parts, ok := content.([]any)
	if !ok || len(parts) == 0 {
		return "", false
	}
	var text strings.Builder
	for _, rawPart := range parts {
		part, ok := rawPart.(map[string]any)
		if !ok || strings.ToLower(strings.TrimSpace(grokStringValue(part["type"]))) != "output_text" {
			return "", false
		}
		value, ok := part["text"].(string)
		if !ok {
			return "", false
		}
		_, _ = text.WriteString(value)
	}
	if strings.TrimSpace(text.String()) == "" {
		return "", false
	}
	return text.String(), true
}
