package service

import (
	"bufio"
	"encoding/json"
	"fmt"
	"strings"
)

type openAICodexResponsesStreamNormalizer struct {
	seq int

	responseID string

	sawCreated    bool
	sawInProgress bool

	messageItemID     string
	messageOutputIndex int
	messageItemAdded  bool
	messageItemDone   bool

	contentPartAdded bool
	contentPartDone  bool

	text strings.Builder
}

func newOpenAICodexResponsesStreamNormalizer() *openAICodexResponsesStreamNormalizer {
	return &openAICodexResponsesStreamNormalizer{
		messageOutputIndex: 0,
	}
}

func normalizeOpenAICodexResponsesSSEBlock(n *openAICodexResponsesStreamNormalizer, sse string) string {
	if n == nil || strings.TrimSpace(sse) == "" {
		return ""
	}
	var out strings.Builder
	scanner := bufio.NewScanner(strings.NewReader(sse))
	for scanner.Scan() {
		line := scanner.Text()
		if data, ok := extractOpenAISSEDataLine(line); ok {
			out.WriteString(n.NormalizeData(data))
		}
	}
	return out.String()
}

func (n *openAICodexResponsesStreamNormalizer) NormalizeData(data string) string {
	trimmed := strings.TrimSpace(data)
	if trimmed == "" {
		return ""
	}
	if trimmed == "[DONE]" {
		return "data: [DONE]\n\n"
	}

	var event map[string]any
	if err := json.Unmarshal([]byte(trimmed), &event); err != nil {
		return renderOpenAICodexSSEDataOnly(trimmed)
	}

	eventType, _ := event["type"].(string)
	if eventType == "" {
		return renderOpenAICodexSSEEvent(n.withSequence(event))
	}

	var out []map[string]any
	switch eventType {
	case "response.created":
		n.sawCreated = true
		n.captureResponseID(event)
		out = append(out, n.withSequence(event))
		if !n.sawInProgress {
			out = append(out, n.synthesizeResponseEvent(event, "response.in_progress", "in_progress"))
			n.sawInProgress = true
		}
	case "response.in_progress":
		n.sawInProgress = true
		n.captureResponseID(event)
		out = append(out, n.withSequence(event))
	case "response.output_item.added":
		if n.isReasoningItemEvent(event) {
			return ""
		}
		if n.isMessageItemEvent(event) {
			n.captureMessageItem(event)
			n.ensureMessageItemShape(event)
			out = append(out, n.withSequence(event))
			n.messageItemAdded = true
			n.messageItemDone = false
		} else {
			out = append(out, n.withSequence(event))
		}
	case "response.output_text.delta":
		delta, _ := event["delta"].(string)
		if delta == "" {
			return ""
		}
		n.text.WriteString(delta)
		out = append(out, n.ensureMessageAndContent()...)
		n.patchTextEvent(event)
		out = append(out, n.withSequence(event))
	case "response.output_text.done":
		out = append(out, n.ensureMessageAndContent()...)
		if _, ok := event["text"].(string); !ok {
			event["text"] = n.text.String()
		}
		n.patchTextEvent(event)
		out = append(out, n.withSequence(event))
		out = append(out, n.ensureContentDone()...)
	case "response.content_part.added":
		n.captureContentPart(event)
		n.patchContentPartEvent(event)
		out = append(out, n.ensureMessageItemAdded()...)
		out = append(out, n.withSequence(event))
		n.contentPartAdded = true
		n.contentPartDone = false
	case "response.content_part.done":
		if n.contentPartDone {
			return ""
		}
		n.patchContentPartEvent(event)
		out = append(out, n.ensureMessageAndContent()...)
		out = append(out, n.withSequence(event))
		n.contentPartDone = true
	case "response.output_item.done":
		if n.isReasoningItemEvent(event) {
			return ""
		}
		if n.isMessageItemEvent(event) || n.messageItemAdded {
			out = append(out, n.ensureContentDone()...)
			n.patchOutputItemEvent(event, "completed")
			out = append(out, n.withSequence(event))
			n.messageItemDone = true
		} else {
			out = append(out, n.withSequence(event))
		}
	case "response.completed", "response.done", "response.incomplete", "response.failed", "response.cancelled", "response.canceled":
		out = append(out, n.ensureContentDone()...)
		out = append(out, n.ensureMessageItemDone()...)
		out = append(out, n.withSequence(event))
	case "response.reasoning_summary_text.delta", "response.reasoning_summary_text.done",
		"response.reasoning_summary_part.added", "response.reasoning_summary_part.done":
		return ""
	default:
		out = append(out, n.withSequence(event))
	}

	return renderOpenAICodexSSEEvents(out)
}

func (n *openAICodexResponsesStreamNormalizer) captureResponseID(event map[string]any) {
	if n.responseID != "" {
		return
	}
	if response, ok := event["response"].(map[string]any); ok {
		if id, _ := response["id"].(string); id != "" {
			n.responseID = id
			return
		}
	}
	if id, _ := event["response_id"].(string); id != "" {
		n.responseID = id
	}
}

func (n *openAICodexResponsesStreamNormalizer) captureMessageItem(event map[string]any) {
	if id, _ := event["item_id"].(string); id != "" {
		n.messageItemID = id
	}
	if idx, ok := numberToInt(event["output_index"]); ok {
		n.messageOutputIndex = idx
	}
	item, _ := event["item"].(map[string]any)
	if item == nil {
		return
	}
	if id, _ := item["id"].(string); id != "" {
		n.messageItemID = id
	}
	if idx, ok := numberToInt(item["output_index"]); ok {
		n.messageOutputIndex = idx
	}
}

func (n *openAICodexResponsesStreamNormalizer) captureContentPart(event map[string]any) {
	if id, _ := event["item_id"].(string); id != "" {
		n.messageItemID = id
	}
	if idx, ok := numberToInt(event["output_index"]); ok {
		n.messageOutputIndex = idx
	}
}

func (n *openAICodexResponsesStreamNormalizer) isMessageItemEvent(event map[string]any) bool {
	item, _ := event["item"].(map[string]any)
	if item == nil {
		return n.messageItemAdded
	}
	itemType, _ := item["type"].(string)
	return itemType == "" || itemType == "message"
}

func (n *openAICodexResponsesStreamNormalizer) isReasoningItemEvent(event map[string]any) bool {
	item, _ := event["item"].(map[string]any)
	if item == nil {
		return false
	}
	itemType, _ := item["type"].(string)
	return itemType == "reasoning"
}

func (n *openAICodexResponsesStreamNormalizer) ensureMessageAndContent() []map[string]any {
	out := n.ensureMessageItemAdded()
	out = append(out, n.ensureContentPartAdded()...)
	return out
}

func (n *openAICodexResponsesStreamNormalizer) ensureMessageItemAdded() []map[string]any {
	if n.messageItemAdded {
		return nil
	}
	itemID := n.ensureMessageItemID()
	event := map[string]any{
		"type":         "response.output_item.added",
		"output_index": n.messageOutputIndex,
		"item": map[string]any{
			"id":      itemID,
			"type":    "message",
			"status":  "in_progress",
			"role":    "assistant",
			"content": []any{},
		},
	}
	n.messageItemAdded = true
	n.messageItemDone = false
	return []map[string]any{n.withSequence(event)}
}

func (n *openAICodexResponsesStreamNormalizer) ensureContentPartAdded() []map[string]any {
	if n.contentPartAdded {
		return nil
	}
	event := map[string]any{
		"type":          "response.content_part.added",
		"item_id":       n.ensureMessageItemID(),
		"output_index":  n.messageOutputIndex,
		"content_index": 0,
		"part": map[string]any{
			"type": "output_text",
			"text": "",
		},
	}
	n.contentPartAdded = true
	n.contentPartDone = false
	return []map[string]any{n.withSequence(event)}
}

func (n *openAICodexResponsesStreamNormalizer) ensureContentDone() []map[string]any {
	if !n.contentPartAdded || n.contentPartDone {
		return nil
	}
	event := map[string]any{
		"type":          "response.content_part.done",
		"item_id":       n.ensureMessageItemID(),
		"output_index":  n.messageOutputIndex,
		"content_index": 0,
		"part": map[string]any{
			"type": "output_text",
			"text": n.text.String(),
		},
	}
	n.contentPartDone = true
	return []map[string]any{n.withSequence(event)}
}

func (n *openAICodexResponsesStreamNormalizer) ensureMessageItemDone() []map[string]any {
	if !n.messageItemAdded || n.messageItemDone {
		return nil
	}
	event := map[string]any{
		"type":         "response.output_item.done",
		"output_index": n.messageOutputIndex,
		"item": map[string]any{
			"id":     n.ensureMessageItemID(),
			"type":   "message",
			"status": "completed",
			"role":   "assistant",
			"content": []any{
				map[string]any{
					"type": "output_text",
					"text": n.text.String(),
				},
			},
		},
	}
	n.messageItemDone = true
	return []map[string]any{n.withSequence(event)}
}

func (n *openAICodexResponsesStreamNormalizer) ensureMessageItemShape(event map[string]any) {
	item, _ := event["item"].(map[string]any)
	if item == nil {
		item = map[string]any{}
		event["item"] = item
	}
	if _, ok := event["output_index"]; !ok {
		event["output_index"] = n.messageOutputIndex
	}
	if _, ok := item["id"]; !ok {
		item["id"] = n.ensureMessageItemID()
	}
	if _, ok := item["type"]; !ok {
		item["type"] = "message"
	}
	if _, ok := item["role"]; !ok {
		item["role"] = "assistant"
	}
	if _, ok := item["status"]; !ok {
		item["status"] = "in_progress"
	}
	if _, ok := item["content"]; !ok {
		item["content"] = []any{}
	}
}

func (n *openAICodexResponsesStreamNormalizer) patchTextEvent(event map[string]any) {
	event["item_id"] = n.ensureMessageItemID()
	event["output_index"] = n.messageOutputIndex
	event["content_index"] = 0
}

func (n *openAICodexResponsesStreamNormalizer) patchContentPartEvent(event map[string]any) {
	event["item_id"] = n.ensureMessageItemID()
	event["output_index"] = n.messageOutputIndex
	event["content_index"] = 0
}

func (n *openAICodexResponsesStreamNormalizer) patchOutputItemEvent(event map[string]any, status string) {
	if _, ok := event["output_index"]; !ok {
		event["output_index"] = n.messageOutputIndex
	}
	item, _ := event["item"].(map[string]any)
	if item == nil {
		item = map[string]any{}
		event["item"] = item
	}
	item["id"] = n.ensureMessageItemID()
	if _, ok := item["type"]; !ok {
		item["type"] = "message"
	}
	if _, ok := item["role"]; !ok {
		item["role"] = "assistant"
	}
	if status != "" {
		item["status"] = status
	}
	if _, ok := item["content"]; !ok {
		item["content"] = []any{
			map[string]any{
				"type": "output_text",
				"text": n.text.String(),
			},
		}
	}
}

func (n *openAICodexResponsesStreamNormalizer) synthesizeResponseEvent(src map[string]any, eventType, status string) map[string]any {
	event := cloneOpenAICodexMap(src)
	event["type"] = eventType
	if response, ok := event["response"].(map[string]any); ok && status != "" {
		response["status"] = status
	}
	return n.withSequence(event)
}

func (n *openAICodexResponsesStreamNormalizer) withSequence(event map[string]any) map[string]any {
	event["sequence_number"] = n.seq
	n.seq++
	return event
}

func (n *openAICodexResponsesStreamNormalizer) ensureMessageItemID() string {
	if n.messageItemID == "" {
		n.messageItemID = fmt.Sprintf("msg_codex_compat_%d", n.messageOutputIndex)
	}
	return n.messageItemID
}

func cloneOpenAICodexMap(src map[string]any) map[string]any {
	clone := make(map[string]any, len(src))
	for k, v := range src {
		if nested, ok := v.(map[string]any); ok {
			clone[k] = cloneOpenAICodexMap(nested)
			continue
		}
		clone[k] = v
	}
	return clone
}

func numberToInt(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	case int64:
		return int(n), true
	default:
		return 0, false
	}
}

func renderOpenAICodexSSEEvents(events []map[string]any) string {
	if len(events) == 0 {
		return ""
	}
	var b strings.Builder
	for _, event := range events {
		b.WriteString(renderOpenAICodexSSEEvent(event))
	}
	return b.String()
}

func renderOpenAICodexSSEEvent(event map[string]any) string {
	eventType, _ := event["type"].(string)
	payload, err := json.Marshal(event)
	if err != nil {
		return ""
	}
	if eventType == "" {
		return renderOpenAICodexSSEDataOnly(string(payload))
	}
	return "event: " + eventType + "\n" + "data: " + string(payload) + "\n\n"
}

func renderOpenAICodexSSEDataOnly(data string) string {
	return "data: " + data + "\n\n"
}
