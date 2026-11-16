package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	codexReservedPythonToolName = "python"
	codexPythonToolAlias        = "python__sub2api"
	codexToolNameReverseKey     = "openai_codex_tool_name_reverse"
	codexToolNameSessionKey     = "openai_codex_tool_name_session_reverse"
)

type codexToolNameField struct {
	object map[string]any
	key    string
	name   string
}

// aliasOpenAIOAuthReservedToolNames avoids names reserved by the ChatGPT
// Codex backend. It validates every declaration/reference before mutating so
// collisions cannot leave a partially rewritten request.
func aliasOpenAIOAuthReservedToolNames(reqBody map[string]any) (map[string]string, bool, error) {
	if reqBody == nil {
		return nil, false, nil
	}

	fields := collectOpenAIResponsesToolNameFields(reqBody)
	owners := make(map[string]string)
	reverse := make(map[string]string)
	for _, field := range fields {
		normalized := aliasOpenAIOAuthReservedToolName(field.name)
		original := field.name
		if normalized != field.name {
			original = strings.TrimSpace(field.name)
		}
		if previous, exists := owners[normalized]; exists && previous != original {
			return nil, false, fmt.Errorf("tool names %q and %q both normalize to %q", previous, original, normalized)
		}
		owners[normalized] = original
		if normalized != field.name {
			reverse[normalized] = original
		}
	}
	if len(reverse) == 0 {
		return nil, false, nil
	}
	for _, field := range fields {
		if aliased := aliasOpenAIOAuthReservedToolName(field.name); aliased != field.name {
			field.object[field.key] = aliased
		}
	}
	return reverse, true, nil
}

func aliasOpenAIOAuthReservedToolName(name string) string {
	trimmed := strings.TrimSpace(name)
	if strings.EqualFold(trimmed, codexReservedPythonToolName) {
		return codexPythonToolAlias
	}
	return name
}

func collectOpenAIResponsesToolNameFields(reqBody map[string]any) []codexToolNameField {
	fields := make([]codexToolNameField, 0, 8)
	appendName := func(object map[string]any, key string) {
		if object == nil {
			return
		}
		name, ok := object[key].(string)
		if !ok || strings.TrimSpace(name) == "" {
			return
		}
		fields = append(fields, codexToolNameField{object: object, key: key, name: name})
	}
	var collectTools func(any)
	collectTools = func(rawTools any) {
		tools, ok := rawTools.([]any)
		if !ok {
			return
		}
		for _, raw := range tools {
			tool, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			toolType := strings.ToLower(strings.TrimSpace(firstNonEmptyString(tool["type"])))
			if toolType == "function" {
				appendName(tool, "name")
				if function, ok := tool["function"].(map[string]any); ok {
					appendName(function, "name")
				}
			}
			if toolType == "namespace" {
				collectTools(tool["tools"])
			}
		}
	}
	collectTools(reqBody["tools"])
	if functions, ok := reqBody["functions"].([]any); ok {
		for _, raw := range functions {
			function, _ := raw.(map[string]any)
			appendName(function, "name")
		}
	}
	if strings.EqualFold(strings.TrimSpace(firstNonEmptyString(reqBody["type"])), "session.update") {
		if session, ok := reqBody["session"].(map[string]any); ok {
			collectTools(session["tools"])
		}
	}
	if choice, ok := reqBody["tool_choice"].(map[string]any); ok {
		if strings.EqualFold(strings.TrimSpace(firstNonEmptyString(choice["type"])), "allowed_tools") {
			collectTools(choice["tools"])
		}
		if strings.EqualFold(strings.TrimSpace(firstNonEmptyString(choice["type"])), "function") {
			appendName(choice, "name")
			if function, ok := choice["function"].(map[string]any); ok {
				appendName(function, "name")
			}
		}
	}
	if input, ok := reqBody["input"].([]any); ok {
		for _, raw := range input {
			item, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			typ := strings.ToLower(strings.TrimSpace(firstNonEmptyString(item["type"])))
			if typ == "additional_tools" {
				collectTools(item["tools"])
			}
			if typ == "function_call" {
				appendName(item, "name")
				if function, ok := item["function"].(map[string]any); ok {
					appendName(function, "name")
				}
			}
		}
	}
	return fields
}

func aliasOpenAIOAuthReservedToolNamesBody(body []byte) ([]byte, map[string]string, bool, error) {
	if len(body) == 0 || !containsASCIIFold(body, []byte(codexReservedPythonToolName)) {
		return body, nil, false, nil
	}
	var reqBody map[string]any
	if err := decodeOpenAIJSONUseNumber(body, &reqBody); err != nil {
		return body, nil, false, fmt.Errorf("decode OAuth reserved tool names: %w", err)
	}
	reverse, changed, err := aliasOpenAIOAuthReservedToolNames(reqBody)
	if err != nil || !changed {
		return body, reverse, false, err
	}
	normalized, err := json.Marshal(reqBody)
	if err != nil {
		return body, nil, false, fmt.Errorf("encode OAuth reserved tool names: %w", err)
	}
	return normalized, reverse, true, nil
}

// containsASCIIFold reports whether haystack contains needle under ASCII case
// folding.
//
// 逐字节比较在 1MiB 级请求体上等于整段扫描；只按首字节 memchr 又会为每个候选
// 重新探测「大小写两种首字节」各自的整段尾部，候选一多就退化成平方级。这里给
// 大小写两条候选游标，每条只在自身未消费的区间上前进，整体保持线性，
// 结果与逐字节折叠比较完全一致。
func containsASCIIFold(haystack, needle []byte) bool {
	if len(needle) == 0 || len(haystack) < len(needle) {
		return false
	}
	first := needle[0]
	upper := first
	if 'a' <= first && first <= 'z' {
		upper = first - ('a' - 'A')
	}
	last := len(haystack) - len(needle)

	lowerPos := bytes.IndexByte(haystack, first)
	upperPos := -1
	if upper != first {
		upperPos = bytes.IndexByte(haystack, upper)
	}
	for lowerPos >= 0 || upperPos >= 0 {
		pos := lowerPos
		if pos < 0 || (upperPos >= 0 && upperPos < pos) {
			pos = upperPos
		}
		if pos > last {
			return false
		}
		if bytes.EqualFold(haystack[pos:pos+len(needle)], needle) {
			return true
		}
		// 只推进刚刚消费掉的那条游标。
		if pos == lowerPos {
			lowerPos = -1
			if next := bytes.IndexByte(haystack[pos+1:], first); next >= 0 {
				lowerPos = pos + 1 + next
			}
			continue
		}
		upperPos = -1
		if next := bytes.IndexByte(haystack[pos+1:], upper); next >= 0 {
			upperPos = pos + 1 + next
		}
	}
	return false
}

func setCodexToolNameReverse(c *gin.Context, reverse map[string]string) {
	if c == nil {
		return
	}
	storeCodexToolNameReverse(c, codexToolNameReverseKey, reverse)
	storeCodexToolNameReverse(c, codexToolNameSessionKey, nil)
}

func storeCodexToolNameReverse(c *gin.Context, key string, reverse map[string]string) {
	if c == nil {
		return
	}
	copyMap := make(map[string]string, len(reverse))
	for aliased, original := range reverse {
		copyMap[aliased] = original
	}
	c.Set(key, copyMap)
}

func mergeCodexToolNameReverse(c *gin.Context, reverse map[string]string) {
	if c == nil || len(reverse) == 0 {
		return
	}
	merged := make(map[string]string, len(reverse)+len(codexToolNameReverseFromContext(c)))
	for aliased, original := range codexToolNameReverseFromContext(c) {
		merged[aliased] = original
	}
	for aliased, original := range reverse {
		merged[aliased] = original
	}
	storeCodexToolNameReverse(c, codexToolNameReverseKey, merged)
}

func codexToolNameReverseFromContext(c *gin.Context) map[string]string {
	return codexToolNameReverseForKey(c, codexToolNameReverseKey)
}

func codexToolNameReverseForKey(c *gin.Context, key string) map[string]string {
	if c == nil {
		return nil
	}
	raw, ok := c.Get(key)
	if !ok {
		return nil
	}
	reverse, _ := raw.(map[string]string)
	return reverse
}

// updateCodexToolNameReverseForWSFrame keeps the active turn isolated from
// session updates that may arrive while that turn is still streaming.
func updateCodexToolNameReverseForWSFrame(c *gin.Context, frame []byte, reverse map[string]string) {
	if c == nil {
		return
	}
	eventType := strings.TrimSpace(gjson.GetBytes(frame, "type").String())
	switch eventType {
	case "session.update":
		if gjson.GetBytes(frame, "session.tools").Exists() {
			storeCodexToolNameReverse(c, codexToolNameSessionKey, reverse)
		}
	case "response.create", "":
		active := reverse
		if !openAIWSFrameHasExplicitToolDeclarations(frame) {
			active = mergeCodexToolNameReverseMaps(
				codexToolNameReverseForKey(c, codexToolNameSessionKey),
				reverse,
			)
		}
		storeCodexToolNameReverse(c, codexToolNameReverseKey, active)
	}
}

func openAIWSFrameHasExplicitToolDeclarations(frame []byte) bool {
	if gjson.GetBytes(frame, "tools").Exists() {
		return true
	}
	for _, item := range gjson.GetBytes(frame, "input").Array() {
		if strings.EqualFold(strings.TrimSpace(item.Get("type").String()), "additional_tools") && item.Get("tools").Exists() {
			return true
		}
	}
	return false
}

func mergeCodexToolNameReverseMaps(base, overlay map[string]string) map[string]string {
	merged := make(map[string]string, len(base)+len(overlay))
	for aliased, original := range base {
		merged[aliased] = original
	}
	for aliased, original := range overlay {
		merged[aliased] = original
	}
	return merged
}

func restoreCodexToolNamesInJSON(data []byte, reverse map[string]string) []byte {
	if len(data) == 0 || len(reverse) == 0 || !json.Valid(data) {
		return data
	}
	var decoded any
	if err := decodeOpenAIJSONUseNumber(data, &decoded); err != nil {
		return data
	}
	if !restoreCodexToolNameFields(decoded, reverse) {
		return data
	}
	restored, err := json.Marshal(decoded)
	if err != nil {
		return data
	}
	return restored
}

func restoreCodexToolNamesFromContext(c *gin.Context, data []byte) []byte {
	reverse := codexToolNameReverseFromContext(c)
	switch strings.TrimSpace(gjson.GetBytes(data, "type").String()) {
	case "session.created", "session.updated":
		reverse = codexToolNameReverseForKey(c, codexToolNameSessionKey)
	}
	return restoreCodexToolNamesInJSON(data, reverse)
}

func restoreCodexToolNamesFromSSEContext(c *gin.Context, data []byte, eventType string) []byte {
	if strings.TrimSpace(gjson.GetBytes(data, "type").String()) != "" || strings.TrimSpace(eventType) == "" {
		return restoreCodexToolNamesFromContext(c, data)
	}
	compat := []byte(openAICompatPayloadWithEventType(string(data), eventType))
	restored := restoreCodexToolNamesFromContext(c, compat)
	if string(restored) == string(compat) {
		return data
	}
	withoutSyntheticType, err := sjson.DeleteBytes(restored, "type")
	if err != nil {
		return restored
	}
	return withoutSyntheticType
}

func restoreCodexToolNameFields(value any, reverse map[string]string) bool {
	root, ok := value.(map[string]any)
	if !ok {
		return false
	}
	changed := false
	restoreItem := func(raw any) {
		item, ok := raw.(map[string]any)
		if !ok || !strings.EqualFold(strings.TrimSpace(firstNonEmptyString(item["type"])), "function_call") {
			return
		}
		name, _ := item["name"].(string)
		if original, exists := reverse[name]; exists {
			item["name"] = original
			changed = true
		}
	}
	restoreOutput := func(raw any) {
		output, _ := raw.([]any)
		for _, item := range output {
			restoreItem(item)
		}
	}
	restoreResponse := func(raw any) {
		response, ok := raw.(map[string]any)
		if ok {
			restoreOutput(response["output"])
		}
	}
	restoreFunction := func(raw any) {
		function, ok := raw.(map[string]any)
		if !ok {
			return
		}
		name, _ := function["name"].(string)
		if original, exists := reverse[name]; exists {
			function["name"] = original
			changed = true
		}
	}
	restoreChatToolCalls := func(raw any) {
		toolCalls, _ := raw.([]any)
		for _, rawCall := range toolCalls {
			call, ok := rawCall.(map[string]any)
			if !ok {
				continue
			}
			callType := strings.ToLower(strings.TrimSpace(firstNonEmptyString(call["type"])))
			if callType == "" || callType == "function" {
				restoreFunction(call["function"])
			}
		}
	}
	restoreMessageContent := func(raw any) {
		content, _ := raw.([]any)
		for _, rawBlock := range content {
			block, ok := rawBlock.(map[string]any)
			if !ok || !strings.EqualFold(strings.TrimSpace(firstNonEmptyString(block["type"])), "tool_use") {
				continue
			}
			name, _ := block["name"].(string)
			if original, exists := reverse[name]; exists {
				block["name"] = original
				changed = true
			}
		}
	}
	var restoreTools func(any)
	restoreTools = func(raw any) {
		tools, _ := raw.([]any)
		for _, rawTool := range tools {
			tool, ok := rawTool.(map[string]any)
			if !ok {
				continue
			}
			toolType := strings.ToLower(strings.TrimSpace(firstNonEmptyString(tool["type"])))
			if toolType == "function" {
				name, _ := tool["name"].(string)
				if original, exists := reverse[name]; exists {
					tool["name"] = original
					changed = true
				}
			}
			if toolType == "namespace" {
				restoreTools(tool["tools"])
			}
		}
	}

	eventType := strings.ToLower(strings.TrimSpace(firstNonEmptyString(root["type"])))
	switch eventType {
	case "response.output_item.added", "response.output_item.done":
		restoreItem(root["item"])
	case "response.created", "response.in_progress", "response.completed", "response.done", "response.failed", "response.incomplete", "response.cancelled", "response.canceled":
		restoreResponse(root["response"])
	case "session.created", "session.updated":
		if session, ok := root["session"].(map[string]any); ok {
			restoreTools(session["tools"])
		}
	}
	if _, hasOutput := root["output"]; hasOutput {
		restoreOutput(root["output"])
	}
	if choices, ok := root["choices"].([]any); ok {
		for _, rawChoice := range choices {
			choice, ok := rawChoice.(map[string]any)
			if !ok {
				continue
			}
			for _, key := range []string{"message", "delta"} {
				if message, ok := choice[key].(map[string]any); ok {
					restoreChatToolCalls(message["tool_calls"])
				}
			}
		}
	}
	restoreMessageContent(root["content"])
	if block, ok := root["content_block"].(map[string]any); ok && strings.EqualFold(strings.TrimSpace(firstNonEmptyString(block["type"])), "tool_use") {
		name, _ := block["name"].(string)
		if original, exists := reverse[name]; exists {
			block["name"] = original
			changed = true
		}
	}
	return changed
}
