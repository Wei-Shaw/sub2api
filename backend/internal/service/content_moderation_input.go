package service

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"

	"github.com/tidwall/gjson"
)

const (
	contentModerationInputClassText                = "user_text"
	contentModerationInputClassImage               = "user_image"
	contentModerationInputClassTextImage           = "user_text_image"
	contentModerationInputClassToolContinuation    = "tool_continuation"
	contentModerationInputClassNonUserContinuation = "non_user_continuation"
	contentModerationInputClassEmpty               = "empty"
	contentModerationInputClassInvalidJSON         = "invalid_json"
	contentModerationInputClassUnsupported         = "unsupported_nonempty"

	contentModerationInputShapeMissing = "missing"
	contentModerationInputShapeString  = "string"
	contentModerationInputShapeObject  = "object"
	contentModerationInputShapeArray   = "array"
	contentModerationInputShapeOther   = "other"

	responsesItemKindNone       = "none"
	responsesItemKindUser       = "user"
	responsesItemKindAssistant  = "assistant"
	responsesItemKindToolCall   = "tool_call"
	responsesItemKindToolOutput = "tool_output"
	responsesItemKindReasoning  = "reasoning"
	responsesItemKindOther      = "other"
)

func ExtractContentModerationText(protocol string, body []byte) string {
	return ExtractContentModerationInput(protocol, body).Text
}

func ExtractContentModerationInput(protocol string, body []byte) ContentModerationInput {
	if len(body) == 0 {
		return newContentModerationInput(nil, nil, contentModerationInputClassEmpty, contentModerationInputShapeMissing, responsesItemKindNone, 0, false)
	}
	if !gjson.ValidBytes(body) {
		return newContentModerationInput(nil, nil, contentModerationInputClassInvalidJSON, contentModerationInputShapeOther, responsesItemKindOther, 0, protocol == ContentModerationProtocolOpenAIResponses)
	}
	var parts []string
	var images []string
	classification := ""
	inputShape := contentModerationInputShapeMissing
	tailItemKind := responsesItemKindNone
	itemCount := 0
	failClosed := false
	switch protocol {
	case ContentModerationProtocolAnthropicMessages:
		collectLastAnthropicUserMessage(gjson.GetBytes(body, "messages"), &parts, &images)
	case ContentModerationProtocolOpenAIChat:
		collectLastRoleMessage(gjson.GetBytes(body, "messages"), "user", &parts, &images)
	case ContentModerationProtocolOpenAIResponses:
		return extractResponsesContentModerationInput(gjson.GetBytes(body, "input"))
	case ContentModerationProtocolGemini:
		collectLastGeminiContent(gjson.GetBytes(body, "contents"), &parts, &images)
	case ContentModerationProtocolOpenAIImages:
		addModerationText(&parts, gjson.GetBytes(body, "prompt").String())
		collectContentValue(gjson.GetBytes(body, "images"), &parts, &images)
	default:
		collectLastResponsesInput(gjson.GetBytes(body, "input"), &parts, &images)
		collectLastRoleMessage(gjson.GetBytes(body, "messages"), "user", &parts, &images)
		collectLastGeminiContent(gjson.GetBytes(body, "contents"), &parts, &images)
	}
	return newContentModerationInput(parts, images, classification, inputShape, tailItemKind, itemCount, failClosed)
}

func newContentModerationInput(parts []string, images []string, classification string, inputShape string, tailItemKind string, itemCount int, failClosed bool) ContentModerationInput {
	out := ContentModerationInput{
		Text:           normalizeContentModerationText(strings.Join(parts, "\n")),
		Images:         normalizeModerationImages(images),
		classification: classification,
		inputShape:     inputShape,
		tailItemKind:   tailItemKind,
		itemCount:      itemCount,
		failClosed:     failClosed,
	}
	out.Normalize()
	if out.classification == "" {
		out.classification = classifyExtractedModerationInput(out)
	}
	return out
}

func classifyExtractedModerationInput(input ContentModerationInput) string {
	hasText := strings.TrimSpace(input.Text) != ""
	hasImages := len(input.Images) > 0
	switch {
	case hasText && hasImages:
		return contentModerationInputClassTextImage
	case hasText:
		return contentModerationInputClassText
	case hasImages:
		return contentModerationInputClassImage
	default:
		return contentModerationInputClassEmpty
	}
}

func collectLastRoleMessage(messages gjson.Result, role string, parts *[]string, images *[]string) {
	if !messages.IsArray() {
		return
	}
	array := messages.Array()
	if len(array) == 0 {
		return
	}
	last := array[len(array)-1]
	if strings.ToLower(strings.TrimSpace(last.Get("role").String())) != role {
		return
	}
	var candidate []string
	var candidateImages []string
	collectContentValue(last.Get("content"), &candidate, &candidateImages)
	if normalizeContentModerationText(strings.Join(candidate, "\n")) == "" && len(candidateImages) == 0 {
		return
	}
	*parts = append(*parts, candidate...)
	*images = append(*images, candidateImages...)
}

func collectLastAnthropicUserMessage(messages gjson.Result, parts *[]string, images *[]string) {
	if !messages.IsArray() {
		return
	}
	array := messages.Array()
	if len(array) == 0 {
		return
	}
	last := array[len(array)-1]
	if strings.ToLower(strings.TrimSpace(last.Get("role").String())) != "user" {
		return
	}
	var candidate []string
	var candidateImages []string
	collectAnthropicUserContentValue(last.Get("content"), &candidate, &candidateImages)
	if normalizeContentModerationText(strings.Join(candidate, "\n")) == "" && len(candidateImages) == 0 {
		return
	}
	*parts = append(*parts, candidate...)
	*images = append(*images, candidateImages...)
}

func collectAnthropicUserContentValue(value gjson.Result, parts *[]string, images *[]string) {
	switch {
	case !value.Exists():
		return
	case value.Type == gjson.String:
		if !isAnthropicSystemReminderText(value.String()) {
			addModerationText(parts, value.String())
		}
	case value.IsArray():
		value.ForEach(func(_, item gjson.Result) bool {
			collectAnthropicUserContentValue(item, parts, images)
			return true
		})
	case value.IsObject():
		typ := strings.ToLower(strings.TrimSpace(value.Get("type").String()))
		switch typ {
		case "", "text", "input_text", "message":
			if value.Get("text").Exists() && !isAnthropicSystemReminderText(value.Get("text").String()) {
				addModerationText(parts, value.Get("text").String())
			}
			if value.Get("content").Exists() {
				collectAnthropicUserContentValue(value.Get("content"), parts, images)
			}
		case "image_url", "input_image", "image":
			collectContentValue(value, parts, images)
		}
	}
}

func isAnthropicSystemReminderText(text string) bool {
	return strings.HasPrefix(strings.TrimSpace(text), "<system-reminder>")
}

func extractResponsesContentModerationInput(input gjson.Result) ContentModerationInput {
	switch {
	case !input.Exists():
		return newContentModerationInput(nil, nil, contentModerationInputClassEmpty, contentModerationInputShapeMissing, responsesItemKindNone, 0, false)
	case input.Type == gjson.String:
		var parts []string
		addModerationText(&parts, input.String())
		return newContentModerationInput(parts, nil, "", contentModerationInputShapeString, responsesItemKindUser, 1, false)
	case input.IsArray():
		array := input.Array()
		if len(array) == 0 {
			return newContentModerationInput(nil, nil, contentModerationInputClassEmpty, contentModerationInputShapeArray, responsesItemKindNone, 0, false)
		}
		tailKind := classifyResponsesItem(array[len(array)-1])
		switch tailKind {
		case responsesItemKindToolCall, responsesItemKindToolOutput:
			return newContentModerationInput(nil, nil, contentModerationInputClassToolContinuation, contentModerationInputShapeArray, tailKind, len(array), false)
		case responsesItemKindAssistant, responsesItemKindReasoning:
			return newContentModerationInput(nil, nil, contentModerationInputClassNonUserContinuation, contentModerationInputShapeArray, tailKind, len(array), false)
		case responsesItemKindOther:
			return newContentModerationInput(nil, nil, contentModerationInputClassUnsupported, contentModerationInputShapeArray, tailKind, len(array), true)
		}
		var parts []string
		var images []string
		failClosed := false
		start := len(array) - 1
		for start > 0 && classifyResponsesItem(array[start-1]) == responsesItemKindUser {
			start--
		}
		for idx := start; idx < len(array); idx++ {
			item := array[idx]
			beforeText := len(parts)
			beforeImages := len(images)
			collectResponsesUserItem(item, &parts, &images)
			if len(parts) == beforeText && len(images) == beforeImages && responsesUserItemHasNonEmptyContent(item) {
				failClosed = true
			}
		}
		classification := ""
		if failClosed && normalizeContentModerationText(strings.Join(parts, "\n")) == "" && len(images) == 0 {
			classification = contentModerationInputClassUnsupported
		}
		return newContentModerationInput(parts, images, classification, contentModerationInputShapeArray, tailKind, len(array), failClosed)
	case input.IsObject():
		kind := classifyResponsesItem(input)
		if kind != responsesItemKindUser {
			classification := contentModerationInputClassUnsupported
			failClosed := true
			if kind == responsesItemKindToolCall || kind == responsesItemKindToolOutput {
				classification = contentModerationInputClassToolContinuation
				failClosed = false
			} else if kind == responsesItemKindAssistant || kind == responsesItemKindReasoning {
				classification = contentModerationInputClassNonUserContinuation
				failClosed = false
			}
			return newContentModerationInput(nil, nil, classification, contentModerationInputShapeObject, kind, 1, failClosed)
		}
		var parts []string
		var images []string
		collectResponsesUserItem(input, &parts, &images)
		failClosed := len(parts) == 0 && len(images) == 0 && responsesUserItemHasNonEmptyContent(input)
		classification := ""
		if failClosed {
			classification = contentModerationInputClassUnsupported
		}
		return newContentModerationInput(parts, images, classification, contentModerationInputShapeObject, kind, 1, failClosed)
	default:
		return newContentModerationInput(nil, nil, contentModerationInputClassUnsupported, contentModerationInputShapeOther, responsesItemKindOther, 1, true)
	}
}

func collectLastResponsesInput(input gjson.Result, parts *[]string, images *[]string) {
	extracted := extractResponsesContentModerationInput(input)
	if extracted.IsEmpty() {
		return
	}
	if extracted.Text != "" {
		*parts = append(*parts, extracted.Text)
	}
	*images = append(*images, extracted.Images...)
}

func classifyResponsesItem(item gjson.Result) string {
	role := strings.ToLower(strings.TrimSpace(item.Get("role").String()))
	switch role {
	case "user":
		return responsesItemKindUser
	case "assistant", "developer", "system":
		return responsesItemKindAssistant
	case "tool", "function":
		return responsesItemKindToolOutput
	}
	if role != "" {
		return responsesItemKindOther
	}
	typ := strings.ToLower(strings.TrimSpace(item.Get("type").String()))
	switch typ {
	case "input_text", "input_image", "text", "image", "image_url":
		return responsesItemKindUser
	case "function_call_output", "computer_call_output", "local_shell_call_output", "mcp_call_output", "custom_tool_call_output":
		return responsesItemKindToolOutput
	case "function_call", "computer_call", "local_shell_call", "web_search_call", "file_search_call", "mcp_call", "custom_tool_call":
		return responsesItemKindToolCall
	case "reasoning":
		return responsesItemKindReasoning
	case "message":
		return responsesItemKindOther
	case "":
		if item.Get("content").Exists() || item.Get("text").Exists() || item.Get("image_url").Exists() {
			return responsesItemKindUser
		}
	}
	return responsesItemKindOther
}

func collectResponsesUserItem(item gjson.Result, parts *[]string, images *[]string) {
	collectContentValue(item.Get("content"), parts, images)
	typ := strings.ToLower(strings.TrimSpace(item.Get("type").String()))
	if typ == "input_text" || typ == "input_image" || typ == "text" || typ == "image" || typ == "image_url" || item.Get("text").Exists() || item.Get("image_url").Exists() {
		collectContentValue(item, parts, images)
	}
}

func responsesUserItemHasNonEmptyContent(item gjson.Result) bool {
	for _, path := range []string{"content", "text", "image_url", "url", "file_id"} {
		value := item.Get(path)
		if !value.Exists() || value.Type == gjson.Null {
			continue
		}
		if value.Type == gjson.String && strings.TrimSpace(value.String()) == "" {
			continue
		}
		if value.IsArray() && len(value.Array()) == 0 {
			continue
		}
		return true
	}
	return false
}

func collectLastGeminiContent(contents gjson.Result, parts *[]string, images *[]string) {
	if !contents.IsArray() {
		return
	}
	array := contents.Array()
	if len(array) == 0 {
		return
	}
	last := array[len(array)-1]
	role := strings.ToLower(strings.TrimSpace(last.Get("role").String()))
	if role != "" && role != "user" {
		return
	}
	var candidate []string
	var candidateImages []string
	if arr := last.Get("parts"); arr.IsArray() {
		arr.ForEach(func(_, part gjson.Result) bool {
			addModerationText(&candidate, part.Get("text").String())
			addGeminiModerationImage(&candidateImages, part)
			return true
		})
	}
	if normalizeContentModerationText(strings.Join(candidate, "\n")) == "" && len(candidateImages) == 0 {
		return
	}
	*parts = append(*parts, candidate...)
	*images = append(*images, candidateImages...)
}

func collectContentValue(value gjson.Result, parts *[]string, images *[]string) {
	switch {
	case !value.Exists():
		return
	case value.Type == gjson.String:
		addModerationText(parts, value.String())
	case value.IsArray():
		value.ForEach(func(_, item gjson.Result) bool {
			collectContentValue(item, parts, images)
			return true
		})
	case value.IsObject():
		typ := strings.ToLower(strings.TrimSpace(value.Get("type").String()))
		addModerationImage(images, value.Get("image_url.url").String())
		addModerationImage(images, value.Get("image_url").String())
		addModerationImage(images, value.Get("url").String())
		addModerationImageData(images, value.Get("source.media_type").String(), value.Get("source.data").String())
		addModerationImageData(images, value.Get("source.mediaType").String(), value.Get("source.data").String())
		addModerationImageData(images, value.Get("media_type").String(), value.Get("data").String())
		addModerationImageData(images, value.Get("mime_type").String(), value.Get("data").String())
		addModerationImageData(images, value.Get("mimeType").String(), value.Get("data").String())
		addModerationImage(images, value.Get("source.data").String())
		addModerationImage(images, value.Get("data").String())
		addModerationImage(images, value.Get("base64").String())
		switch typ {
		case "", "text", "input_text", "message":
			if value.Get("text").Exists() {
				addModerationText(parts, value.Get("text").String())
			}
			if value.Get("content").Exists() {
				collectContentValue(value.Get("content"), parts, images)
			}
		case "image_url", "input_image", "image":
		}
	}
}

func addGeminiModerationImage(images *[]string, part gjson.Result) {
	if inlineData := part.Get("inline_data"); inlineData.IsObject() {
		mimeType := strings.TrimSpace(inlineData.Get("mime_type").String())
		data := strings.TrimSpace(inlineData.Get("data").String())
		if mimeType != "" && data != "" {
			addModerationImage(images, fmt.Sprintf("data:%s;base64,%s", mimeType, data))
		}
	}
	if inlineData := part.Get("inlineData"); inlineData.IsObject() {
		mimeType := strings.TrimSpace(inlineData.Get("mimeType").String())
		data := strings.TrimSpace(inlineData.Get("data").String())
		if mimeType != "" && data != "" {
			addModerationImage(images, fmt.Sprintf("data:%s;base64,%s", mimeType, data))
		}
	}
	addModerationImage(images, part.Get("file_data.file_uri").String())
	addModerationImage(images, part.Get("fileData.fileUri").String())
}

func addModerationImageData(images *[]string, mimeType string, data string) {
	mimeType = strings.TrimSpace(mimeType)
	data = strings.TrimSpace(data)
	if mimeType == "" || data == "" {
		return
	}
	addModerationImage(images, fmt.Sprintf("data:%s;base64,%s", mimeType, data))
}

func addModerationImage(images *[]string, image string) {
	image = strings.TrimSpace(image)
	if image == "" {
		return
	}
	if strings.HasPrefix(image, "data:") || strings.HasPrefix(image, "http://") || strings.HasPrefix(image, "https://") {
		*images = append(*images, image)
	}
}

func normalizeModerationImages(images []string) []string {
	out := make([]string, 0, len(images))
	seen := make(map[string]struct{}, len(images))
	for _, image := range images {
		image = strings.TrimSpace(image)
		if image == "" {
			continue
		}
		if _, ok := seen[image]; ok {
			continue
		}
		seen[image] = struct{}{}
		out = append(out, image)
	}
	return out
}

func limitContentModerationImages(images []string) []string {
	if len(images) <= maxContentModerationInputImages {
		return images
	}
	idx, err := rand.Int(rand.Reader, big.NewInt(int64(len(images))))
	if err != nil {
		return images[:maxContentModerationInputImages]
	}
	return []string{images[int(idx.Int64())]}
}

func addModerationText(parts *[]string, text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	if strings.Contains(text, "<system-reminder>") {
		return
	}
	*parts = append(*parts, text)
}

func normalizeContentModerationText(text string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
}
