package service

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func filterOpenCodeResponsesSSEFrameWithImages(ctx context.Context, lines []string, store *OpenAIGeneratedImageStore, opts openCodeImageRewriteOptions) (frame string, data string, hasData bool, keep bool) {
	if len(lines) == 0 {
		return "", "", false, false
	}
	sseEventType := openCodeSSEFrameEventType(lines)
	dataLines := make([]string, 0, 1)
	for _, line := range lines {
		if extracted, ok := extractOpenAISSEDataLine(line); ok {
			dataLines = append(dataLines, extracted)
		}
	}
	if len(dataLines) == 0 {
		if strings.HasPrefix(sseEventType, "response.image_generation_call.") {
			return "", "", false, false
		}
		return rebuildSSEFrame(lines), "", false, true
	}
	joined := strings.Join(dataLines, "\n")
	if !gjson.Valid(joined) && containsMalformedOpenCodeImageSSEMarker(joined) {
		return "", joined, true, false
	}
	dataEventType := strings.TrimSpace(gjson.Get(joined, "type").String())
	eventType := dataEventType
	if eventType == "" {
		eventType = sseEventType
	}
	if gjson.Get(joined, "partial_image_b64").Exists() {
		return "", joined, true, false
	}
	if strings.HasPrefix(dataEventType, "response.image_generation_call.") || strings.HasPrefix(sseEventType, "response.image_generation_call.") {
		return "", joined, true, false
	}

	outputItemAdded := dataEventType == "response.output_item.added" || sseEventType == "response.output_item.added"
	outputItemDone := dataEventType == "response.output_item.done" || sseEventType == "response.output_item.done"
	if outputItemAdded {
		if gjson.Get(joined, "item.type").String() == "image_generation_call" {
			return "", joined, true, false
		}
		if !gjson.Valid(joined) && strings.Contains(joined, "image_generation_call") {
			return "", joined, true, false
		}
	}
	if outputItemDone {
		if gjson.Get(joined, "item.type").String() == "image_generation_call" {
			frameBody, eventData := buildOpenCodeImageDoneSSEFrames(ctx, joined, store, opts)
			return frameBody, eventData, true, frameBody != ""
		}
		if !gjson.Valid(joined) && strings.Contains(joined, "image_generation_call") {
			return "", joined, true, false
		}
	}
	if !outputItemAdded && !outputItemDone && gjson.Get(joined, "item.type").String() == "image_generation_call" {
		if strings.TrimSpace(gjson.Get(joined, "item.result").String()) == "" && strings.TrimSpace(gjson.Get(joined, "item.status").String()) != "completed" {
			return "", joined, true, false
		}
		frameBody, eventData := buildOpenCodeImageDoneSSEFrames(ctx, joined, store, opts)
		return frameBody, eventData, true, frameBody != ""
	}
	if eventType == "" {
		for _, item := range gjson.Get(joined, "response.output").Array() {
			if item.Get("type").String() == "image_generation_call" {
				return "", joined, true, false
			}
		}
	}

	filteredData, keep := filterOpenCodeResponsesSSEData(joined)
	if !keep {
		return "", joined, true, false
	}
	if isOpenAITerminalResponseEventType(eventType) {
		patchedData, syntheticFrames, changed := rewriteOpenCodeTerminalSSEDataWithImages(ctx, filteredData, store, opts)
		if changed {
			return syntheticFrames + rebuildSSEFrameWithData(lines, patchedData), patchedData, true, true
		}
	}
	return rebuildSSEFrameWithData(lines, filteredData), filteredData, true, true
}

func containsMalformedOpenCodeImageSSEMarker(data string) bool {
	raw := strings.ToLower(data)
	if strings.Contains(raw, "partial_image_b64") {
		return true
	}
	if !strings.Contains(raw, "image_generation_call") {
		return false
	}
	return strings.Contains(raw, "result") || strings.Contains(raw, "output_format") || strings.Contains(raw, "status")
}

func isOpenAITerminalResponseEventType(eventType string) bool {
	switch strings.TrimSpace(eventType) {
	case "response.completed", "response.done", "response.failed", "response.incomplete", "response.cancelled", "response.canceled":
		return true
	default:
		return false
	}
}

func openCodeSSEFrameEventType(lines []string) string {
	for _, line := range lines {
		if eventType, ok := extractSSEEventLine(line); ok {
			return eventType
		}
	}
	return ""
}

func extractSSEEventLine(line string) (string, bool) {
	if !strings.HasPrefix(line, "event:") {
		return "", false
	}
	start := len("event:")
	for start < len(line) {
		if line[start] != ' ' && line[start] != '\t' {
			break
		}
		start++
	}
	return strings.TrimSpace(line[start:]), true
}

func buildOpenCodeImageDoneSSEFrames(ctx context.Context, data string, store *OpenAIGeneratedImageStore, opts openCodeImageRewriteOptions) (string, string) {
	itemRaw := gjson.Get(data, "item").Raw
	if itemRaw == "" {
		return "", ""
	}
	sourceItemID := strings.TrimSpace(gjson.Get(data, "item.id").String())
	if sourceItemID != "" && opts.RewrittenImageMessages != nil {
		if message, ok := opts.RewrittenImageMessages[sourceItemID]; ok {
			return buildOpenCodeMessageSSEFrames(message, int(gjson.Get(data, "output_index").Int()))
		}
	}
	message, _, err := buildOpenCodeImageGenerationMessageForRawItem(ctx, json.RawMessage(itemRaw), store, opts, true)
	if err != nil {
		message = buildOpenCodeImageNoResultMessage(gjson.Get(data, "item.id").String())
	}
	if sourceItemID != "" && opts.RewrittenImageMessages != nil {
		opts.RewrittenImageMessages[sourceItemID] = message
	}
	return buildOpenCodeMessageSSEFrames(message, int(gjson.Get(data, "output_index").Int()))
}

func buildOpenCodeGeneratedImageSSEFrames(rec OpenAIGeneratedImageRecord, outputIndex int, opts openCodeImageRewriteOptions) string {
	frames, _ := buildOpenCodeMessageSSEFrames(buildOpenCodeGeneratedImageMessage(rec, opts), outputIndex)
	return frames
}

func buildOpenCodeMessageSSEFrames(message map[string]any, outputIndex int) (string, string) {
	messageJSON, err := json.Marshal(message)
	if err != nil {
		return "", ""
	}
	messageID := gjson.GetBytes(messageJSON, "id").String()
	text := gjson.GetBytes(messageJSON, "content.0.text").String()
	part := map[string]any{
		"type":        "output_text",
		"text":        text,
		"annotations": []any{},
	}
	addedItem := map[string]any{
		"id":      messageID,
		"type":    "message",
		"status":  "in_progress",
		"role":    "assistant",
		"content": []any{},
	}
	events := []struct {
		typeName string
		payload  map[string]any
	}{
		{
			typeName: "response.output_item.added",
			payload: map[string]any{
				"type":         "response.output_item.added",
				"output_index": outputIndex,
				"item":         addedItem,
			},
		},
		{
			typeName: "response.content_part.added",
			payload: map[string]any{
				"type":          "response.content_part.added",
				"output_index":  outputIndex,
				"content_index": 0,
				"item_id":       messageID,
				"part":          part,
			},
		},
		{
			typeName: "response.output_text.delta",
			payload: map[string]any{
				"type":          "response.output_text.delta",
				"output_index":  outputIndex,
				"content_index": 0,
				"item_id":       messageID,
				"delta":         text,
			},
		},
		{
			typeName: "response.output_text.done",
			payload: map[string]any{
				"type":          "response.output_text.done",
				"output_index":  outputIndex,
				"content_index": 0,
				"item_id":       messageID,
				"text":          text,
			},
		},
		{
			typeName: "response.content_part.done",
			payload: map[string]any{
				"type":          "response.content_part.done",
				"output_index":  outputIndex,
				"content_index": 0,
				"item_id":       messageID,
				"part":          part,
			},
		},
		{
			typeName: "response.output_item.done",
			payload: map[string]any{
				"type":         "response.output_item.done",
				"output_index": outputIndex,
				"item":         message,
			},
		},
	}

	var builder strings.Builder
	lastData := ""
	for _, event := range events {
		payload, err := json.Marshal(event.payload)
		if err != nil {
			return "", ""
		}
		lastData = string(payload)
		builder.WriteString("event: ")
		builder.WriteString(event.typeName)
		builder.WriteString("\n")
		builder.WriteString("data: ")
		builder.Write(payload)
		builder.WriteString("\n\n")
	}
	return builder.String(), lastData
}

func rewriteOpenCodeTerminalSSEDataWithImages(ctx context.Context, data string, store *OpenAIGeneratedImageStore, opts openCodeImageRewriteOptions) (string, string, bool) {
	responseRaw := gjson.Get(data, "response").Raw
	if responseRaw == "" {
		return data, "", false
	}
	output := gjson.Get(responseRaw, "output")
	if !output.Exists() || !output.IsArray() {
		return data, "", false
	}
	rewritten, generated, changed, err := rewriteOpenCodeImageGenerationOutputItems(ctx, []byte(output.Raw), store, opts, true)
	if err != nil || !changed {
		return data, "", false
	}
	outputJSON, err := json.Marshal(rewritten)
	if err != nil {
		return data, "", false
	}
	patchedResponse, err := sjson.SetRaw(responseRaw, "output", string(outputJSON))
	if err != nil {
		return data, "", false
	}
	patchedData, err := sjson.SetRaw(data, "response", patchedResponse)
	if err != nil {
		return data, "", false
	}

	var synthetic strings.Builder
	for _, generatedMessage := range generated {
		frames, _ := buildOpenCodeMessageSSEFrames(generatedMessage.Message, generatedMessage.OutputIndex)
		synthetic.WriteString(frames)
	}
	return patchedData, synthetic.String(), true
}

func replaceSSEFrameDataPayload(frameBody string, oldData string, newData string, fallbackLines []string) string {
	if oldData == newData {
		return frameBody
	}
	if frameBody != "" && oldData != "" {
		oldBlock := buildSSEDataBlock(oldData)
		if oldBlock != "" {
			newBlock := buildSSEDataBlock(newData)
			if strings.Contains(frameBody, oldBlock) {
				return strings.Replace(frameBody, oldBlock, newBlock, 1)
			}
		}
	}
	if strings.Count(frameBody, "\n\n") > 1 {
		if replaced, ok := replaceLastSSEFrameDataPayload(frameBody, newData); ok {
			return replaced
		}
	}
	return rebuildSSEFrameWithData(fallbackLines, newData)
}

func buildSSEDataBlock(data string) string {
	parts := strings.Split(data, "\n")
	lines := make([]string, 0, len(parts))
	for _, part := range parts {
		lines = append(lines, "data: "+part)
	}
	return strings.Join(lines, "\n")
}

func replaceLastSSEFrameDataPayload(frameBody string, newData string) (string, bool) {
	frames := strings.Split(frameBody, "\n\n")
	for i := len(frames) - 1; i >= 0; i-- {
		if strings.TrimSpace(frames[i]) == "" {
			continue
		}
		updated := strings.TrimSuffix(rebuildSSEFrameWithData(strings.Split(frames[i], "\n"), newData), "\n\n")
		if updated == "" {
			return "", false
		}
		frames[i] = updated
		return strings.Join(frames, "\n\n"), true
	}
	return "", false
}
