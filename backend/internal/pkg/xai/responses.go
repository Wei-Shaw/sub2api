package xai

import (
	"bytes"
	"encoding/json"
	"sort"
	"strings"
)

func NormalizeResponsesBody(body []byte, model string, stream bool, promptCacheKey string) ([]byte, error) {
	var req map[string]any
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}
	NormalizeResponsesBodyMap(req, model, stream, promptCacheKey)
	return json.Marshal(req)
}

func NormalizeResponsesBodyMap(req map[string]any, model string, stream bool, promptCacheKey string) {
	if req == nil {
		return
	}
	if strings.TrimSpace(model) != "" {
		req["model"] = strings.TrimSpace(model)
	}
	req["stream"] = stream
	delete(req, "previous_response_id")
	delete(req, "prompt_cache_retention")
	delete(req, "safety_identifier")
	delete(req, "stream_options")
	if strings.TrimSpace(promptCacheKey) != "" {
		req["prompt_cache_key"] = strings.TrimSpace(promptCacheKey)
	}
	normalizeTools(req)
	normalizeReasoning(req)
}

func normalizeTools(req map[string]any) {
	rawTools, ok := req["tools"].([]any)
	if !ok || len(rawTools) == 0 {
		delete(req, "tools")
		delete(req, "tool_choice")
		delete(req, "parallel_tool_calls")
		return
	}
	tools := make([]any, 0, len(rawTools))
	for _, raw := range rawTools {
		tools = append(tools, normalizeTool(raw)...)
	}
	if len(tools) == 0 {
		delete(req, "tools")
		delete(req, "tool_choice")
		delete(req, "parallel_tool_calls")
		return
	}
	req["tools"] = tools
}

func normalizeTool(raw any) []any {
	tool, ok := raw.(map[string]any)
	if !ok || tool == nil {
		return nil
	}
	clone := cloneMap(tool)
	toolType := strings.ToLower(strings.TrimSpace(stringValue(clone["type"])))
	switch toolType {
	case "tool_search", "image_generation":
		return nil
	case "namespace":
		nested, _ := clone["tools"].([]any)
		out := make([]any, 0, len(nested))
		for _, item := range nested {
			out = append(out, normalizeTool(item)...)
		}
		return out
	case "custom":
		if strings.TrimSpace(stringValue(clone["name"])) == "apply_patch" {
			return nil
		}
		clone["type"] = "function"
		ensureToolParameters(clone)
	case "function":
		ensureToolParameters(clone)
	case "web_search":
		delete(clone, "external_web_access")
	default:
		if toolType == "" {
			clone["type"] = "function"
			ensureToolParameters(clone)
		}
	}
	return []any{clone}
}

func ensureToolParameters(tool map[string]any) {
	if _, ok := tool["parameters"]; !ok {
		tool["parameters"] = map[string]any{}
	}
}

func normalizeReasoning(req map[string]any) {
	model := strings.TrimSpace(stringValue(req["model"]))
	if !supportsReasoningEffort(model) {
		delete(req, "reasoning")
	}

	if include, ok := req["include"].([]any); ok {
		filtered := make([]any, 0, len(include))
		for _, item := range include {
			if strings.TrimSpace(stringValue(item)) == "reasoning.encrypted_content" {
				continue
			}
			filtered = append(filtered, item)
		}
		if len(filtered) == 0 {
			delete(req, "include")
		} else {
			req["include"] = filtered
		}
	}

	input, ok := req["input"].([]any)
	if !ok || len(input) == 0 {
		return
	}
	normalized := make([]any, 0, len(input))
	for _, raw := range input {
		item, ok := raw.(map[string]any)
		if !ok || item == nil || strings.TrimSpace(stringValue(item["type"])) != "reasoning" {
			normalized = append(normalized, raw)
			continue
		}
		clone := cloneMap(item)
		if clone["content"] == nil {
			delete(clone, "content")
		}
		if clone["encrypted_content"] == nil {
			delete(clone, "encrypted_content")
		}
		if isSummaryOnlyReasoning(clone) && len(normalized) > 0 {
			if prev, ok := normalized[len(normalized)-1].(map[string]any); ok && isSummaryOnlyReasoning(prev) {
				prev["summary"] = append(anySlice(prev["summary"]), anySlice(clone["summary"])...)
				continue
			}
		}
		normalized = append(normalized, clone)
	}
	req["input"] = normalized
}

func supportsReasoningEffort(model string) bool {
	model = strings.TrimSpace(model)
	return strings.HasPrefix(model, "grok-3-mini") ||
		strings.HasPrefix(model, "grok-4.20-multi-agent") ||
		strings.HasPrefix(model, "grok-4.3")
}

func isSummaryOnlyReasoning(item map[string]any) bool {
	if item == nil || strings.TrimSpace(stringValue(item["type"])) != "reasoning" {
		return false
	}
	if _, ok := item["summary"].([]any); !ok {
		return false
	}
	for key := range item {
		if key != "type" && key != "summary" {
			return false
		}
	}
	return true
}

func anySlice(value any) []any {
	if value == nil {
		return nil
	}
	if arr, ok := value.([]any); ok {
		return arr
	}
	return nil
}

func cloneMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func stringValue(value any) string {
	s, _ := value.(string)
	return s
}

type OutputItemCollector struct {
	items []collectedOutputItem
	order int
}

type collectedOutputItem struct {
	index int
	order int
	item  any
}

func (c *OutputItemCollector) ProcessData(data []byte) ([]byte, bool) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("[DONE]")) {
		return data, false
	}
	var event map[string]any
	if err := json.Unmarshal(trimmed, &event); err != nil {
		return data, false
	}
	eventType := strings.TrimSpace(stringValue(event["type"]))
	switch eventType {
	case "response.output_item.done":
		item, ok := event["item"].(map[string]any)
		if !ok || item == nil {
			return data, false
		}
		idx := intFromAny(event["output_index"], c.order)
		c.items = append(c.items, collectedOutputItem{index: idx, order: c.order, item: item})
		c.order++
		return data, false
	case "response.completed", "response.done":
		if len(c.items) == 0 {
			return data, false
		}
		response, ok := event["response"].(map[string]any)
		if !ok || response == nil {
			return data, false
		}
		if output, ok := response["output"].([]any); ok && len(output) > 0 {
			return data, false
		}
		response["output"] = c.sortedItems()
		patched, err := json.Marshal(event)
		if err != nil {
			return data, false
		}
		return patched, true
	default:
		return data, false
	}
}

func (c *OutputItemCollector) sortedItems() []any {
	items := append([]collectedOutputItem(nil), c.items...)
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].index != items[j].index {
			return items[i].index < items[j].index
		}
		return items[i].order < items[j].order
	})
	out := make([]any, 0, len(items))
	for _, item := range items {
		out = append(out, item.item)
	}
	return out
}

func intFromAny(value any, fallback int) int {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case json.Number:
		if i, err := v.Int64(); err == nil {
			return int(i)
		}
	}
	return fallback
}

func PatchSSEBodyWithOutputItems(body []byte) []byte {
	if len(body) == 0 {
		return body
	}
	collector := &OutputItemCollector{}
	lines := bytes.Split(body, []byte("\n"))
	changed := false
	for i, line := range lines {
		data, ok := extractSSEData(line)
		if !ok {
			continue
		}
		if patched, patchedOK := collector.ProcessData(data); patchedOK {
			lines[i] = append([]byte("data: "), patched...)
			changed = true
		}
	}
	if !changed {
		return body
	}
	return bytes.Join(lines, []byte("\n"))
}

func extractSSEData(line []byte) ([]byte, bool) {
	if !bytes.HasPrefix(line, []byte("data:")) {
		return nil, false
	}
	data := line[len("data:"):]
	data = bytes.TrimLeft(data, " \t")
	return data, true
}
