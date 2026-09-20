package service

import (
	"fmt"
	"strings"
)

const (
	openAIToolLoopGuardScanLimit = 64
	openAIToolLoopGuardThreshold = 2
	openAIToolLoopGuardMarker    = "[tool_loop_guard]"
)

type openAIToolLoopGuardResult struct {
	ToolName      string
	ToolNamespace string
	ErrorClass    string
	FailureCount  int
}

type openAIToolIdentity struct {
	Name      string
	Namespace string
}

// applyOpenAIResponsesToolLoopGuard temporarily suppresses a tool after the
// current user turn has produced the same deterministic failure repeatedly.
// It is intentionally reconstructed from the replay tail so it works across
// HTTP, WebSocket reconnects, failover, and gateway replicas without storage.
func applyOpenAIResponsesToolLoopGuard(body []byte) ([]byte, openAIToolLoopGuardResult, bool, error) {
	var result openAIToolLoopGuardResult
	if len(body) == 0 {
		return body, result, false, nil
	}

	var requestBody map[string]any
	if err := decodeOpenAIJSONUseNumber(body, &requestBody); err != nil {
		return body, result, false, nil
	}
	input, ok := requestBody["input"].([]any)
	if !ok || len(input) == 0 {
		return body, result, false, nil
	}

	tool, errorClass, failures := detectOpenAIResponsesToolFailureLoop(input)
	if failures < openAIToolLoopGuardThreshold {
		return body, result, false, nil
	}
	result = openAIToolLoopGuardResult{
		ToolName:      tool.Name,
		ToolNamespace: tool.Namespace,
		ErrorClass:    errorClass,
		FailureCount:  failures,
	}

	removed := removeOpenAIResponsesTool(requestBody, tool)
	if !removed {
		return body, result, false, nil
	}
	appendOpenAIToolLoopGuardInstruction(requestBody, result)
	normalizeOpenAIToolChoiceAfterSuppression(requestBody, tool)

	normalized, err := marshalOpenAIUpstreamJSON(requestBody)
	if err != nil {
		return body, openAIToolLoopGuardResult{}, false, fmt.Errorf("encode tool loop guard body: %w", err)
	}
	return normalized, result, true, nil
}

func detectOpenAIResponsesToolFailureLoop(input []any) (openAIToolIdentity, string, int) {
	start := 0
	if len(input) > openAIToolLoopGuardScanLimit {
		start = len(input) - openAIToolLoopGuardScanLimit
	}
	for i := len(input) - 1; i >= start; i-- {
		item, ok := input[i].(map[string]any)
		if !ok {
			continue
		}
		if strings.EqualFold(firstNonEmptyString(item["type"]), "message") &&
			strings.EqualFold(firstNonEmptyString(item["role"]), "user") {
			start = i + 1
			break
		}
	}

	callTools := make(map[string]openAIToolIdentity)
	for i := start; i < len(input); i++ {
		item, ok := input[i].(map[string]any)
		if !ok || !isOpenAIToolCallItem(item) {
			continue
		}
		callID := firstNonEmptyString(item["call_id"], item["id"])
		tool := openAIToolItemIdentity(item)
		if callID != "" && tool.Name != "" {
			callTools[callID] = tool
		}
	}

	var targetTool openAIToolIdentity
	var targetClass string
	failures := 0
	for i := len(input) - 1; i >= start; i-- {
		item, ok := input[i].(map[string]any)
		if !ok || !isOpenAIToolOutputItem(item) {
			continue
		}
		tool := openAIToolItemIdentity(item)
		if tool.Name == "" {
			tool = callTools[firstNonEmptyString(item["call_id"], item["id"])]
		}
		errorClass := classifyOpenAIToolFailure(flattenOpenAIToolOutputText(item["output"]))
		if tool.Name == "" || errorClass == "" {
			break
		}
		if failures == 0 {
			targetTool, targetClass = tool, errorClass
		}
		if !sameOpenAIToolIdentity(tool, targetTool) || errorClass != targetClass {
			break
		}
		failures++
	}
	return targetTool, targetClass, failures
}

func isOpenAIToolCallItem(item map[string]any) bool {
	switch strings.ToLower(firstNonEmptyString(item["type"])) {
	case "function_call", "custom_tool_call":
		return true
	default:
		return false
	}
}

func isOpenAIToolOutputItem(item map[string]any) bool {
	switch strings.ToLower(firstNonEmptyString(item["type"])) {
	case "function_call_output", "custom_tool_call_output":
		return true
	default:
		return false
	}
}

func openAIToolItemName(item map[string]any) string {
	return openAIToolItemIdentity(item).Name
}

func openAIToolItemIdentity(item map[string]any) openAIToolIdentity {
	identity := openAIToolIdentity{
		Name:      firstNonEmptyString(item["name"]),
		Namespace: firstNonEmptyString(item["namespace"]),
	}
	if identity.Name != "" {
		return identity
	}
	if function, ok := item["function"].(map[string]any); ok {
		identity.Name = firstNonEmptyString(function["name"])
		identity.Namespace = firstNonEmptyString(function["namespace"], item["namespace"])
	}
	return identity
}

func sameOpenAIToolIdentity(left, right openAIToolIdentity) bool {
	return strings.EqualFold(strings.TrimSpace(left.Name), strings.TrimSpace(right.Name)) &&
		strings.EqualFold(strings.TrimSpace(left.Namespace), strings.TrimSpace(right.Namespace))
}

func flattenOpenAIToolOutputText(value any) string {
	var parts []string
	var walk func(any)
	walk = func(node any) {
		switch typed := node.(type) {
		case string:
			if trimmed := strings.TrimSpace(typed); trimmed != "" {
				parts = append(parts, trimmed)
			}
		case []any:
			for _, child := range typed {
				walk(child)
			}
		case map[string]any:
			for _, key := range []string{"text", "content", "output", "error", "message"} {
				if child, exists := typed[key]; exists {
					walk(child)
				}
			}
		}
	}
	walk(value)
	return strings.Join(parts, "\n")
}

func classifyOpenAIToolFailure(output string) string {
	normalized := strings.ToLower(strings.TrimSpace(output))
	if normalized == "" {
		return ""
	}
	if (strings.Contains(normalized, "exec cell ") ||
		strings.Contains(normalized, "process handle ") ||
		strings.Contains(normalized, "session handle ")) &&
		(strings.Contains(normalized, " not found") || strings.Contains(normalized, " invalid")) {
		return "invalid_handle"
	}
	if strings.Contains(normalized, "unknown tool") ||
		strings.Contains(normalized, "tool not found") ||
		strings.Contains(normalized, "tool is unavailable") {
		return "tool_unavailable"
	}
	if strings.Contains(normalized, "permission denied") ||
		strings.Contains(normalized, "access denied") ||
		strings.Contains(normalized, "unauthorized") ||
		strings.Contains(normalized, "forbidden") {
		return "permission_denied"
	}
	if strings.Contains(normalized, "invalid argument") ||
		strings.Contains(normalized, "invalid parameter") ||
		strings.Contains(normalized, "missing required") {
		return "invalid_arguments"
	}
	return ""
}

func removeOpenAIResponsesTool(requestBody map[string]any, target openAIToolIdentity) bool {
	removed := false
	if tools, ok := requestBody["tools"].([]any); ok {
		filtered, changed := filterOpenAITools(tools, target, "")
		if changed {
			removed = true
			if len(filtered) == 0 {
				delete(requestBody, "tools")
			} else {
				requestBody["tools"] = filtered
			}
		}
	}
	if input, ok := requestBody["input"].([]any); ok {
		for _, rawItem := range input {
			item, ok := rawItem.(map[string]any)
			if !ok || !strings.EqualFold(firstNonEmptyString(item["type"]), "additional_tools") {
				continue
			}
			tools, ok := item["tools"].([]any)
			if !ok {
				continue
			}
			filtered, changed := filterOpenAITools(tools, target, "")
			if !changed {
				continue
			}
			removed = true
			item["tools"] = filtered
		}
	}
	return removed
}

func filterOpenAITools(tools []any, target openAIToolIdentity, parentNamespace string) ([]any, bool) {
	filtered := make([]any, 0, len(tools))
	changed := false
	for _, rawTool := range tools {
		tool, ok := rawTool.(map[string]any)
		if !ok {
			filtered = append(filtered, rawTool)
			continue
		}
		identity := openAIToolItemIdentity(tool)
		if identity.Namespace == "" {
			identity.Namespace = parentNamespace
		}
		if openAIToolIdentityMatches(identity, target) {
			changed = true
			continue
		}
		if strings.EqualFold(firstNonEmptyString(tool["type"]), "namespace") {
			namespace := firstNonEmptyString(tool["name"])
			namespaceChanged := false
			for _, childField := range []string{"tools", "children"} {
				children, childrenOK := tool[childField].([]any)
				if !childrenOK {
					continue
				}
				filteredChildren, childChanged := filterOpenAITools(children, target, namespace)
				if !childChanged {
					continue
				}
				changed = true
				namespaceChanged = true
				tool[childField] = filteredChildren
			}
			if namespaceChanged && namespaceChildrenEmpty(tool) {
				continue
			}
		}
		filtered = append(filtered, rawTool)
	}
	return filtered, changed
}

func openAIToolIdentityMatches(candidate, target openAIToolIdentity) bool {
	if sameOpenAIToolIdentity(candidate, target) {
		return true
	}
	if target.Namespace == "" || candidate.Namespace != "" {
		return false
	}
	flattened := target.Namespace + "__" + target.Name
	return strings.EqualFold(strings.TrimSpace(candidate.Name), flattened)
}

func namespaceChildrenEmpty(tool map[string]any) bool {
	for _, childField := range []string{"tools", "children"} {
		if children, ok := tool[childField].([]any); ok && len(children) > 0 {
			return false
		}
	}
	return true
}

func appendOpenAIToolLoopGuardInstruction(requestBody map[string]any, result openAIToolLoopGuardResult) {
	toolName := result.ToolName
	if result.ToolNamespace != "" {
		toolName = result.ToolNamespace + "." + result.ToolName
	}
	note := fmt.Sprintf(
		"%s Tool %q is temporarily unavailable because recent calls repeatedly failed with %s. Continue without it or start a new operation that returns a valid capability.",
		openAIToolLoopGuardMarker,
		toolName,
		result.ErrorClass,
	)
	existing, exists := requestBody["instructions"]
	if !exists {
		requestBody["instructions"] = note
		return
	}
	instructions, ok := existing.(string)
	if !ok || strings.Contains(instructions, openAIToolLoopGuardMarker) {
		return
	}
	requestBody["instructions"] = strings.TrimSpace(instructions) + "\n\n" + note
}

func normalizeOpenAIToolChoiceAfterSuppression(requestBody map[string]any, target openAIToolIdentity) {
	if !hasOpenAIToolDeclarations(requestBody) {
		delete(requestBody, "tool_choice")
		delete(requestBody, "parallel_tool_calls")
		return
	}
	choice, ok := requestBody["tool_choice"].(map[string]any)
	if !ok {
		return
	}
	if openAIToolIdentityMatches(openAIToolItemIdentity(choice), target) {
		requestBody["tool_choice"] = "auto"
	}
}

func hasOpenAIToolDeclarations(requestBody map[string]any) bool {
	if tools, ok := requestBody["tools"].([]any); ok && len(tools) > 0 {
		return true
	}
	input, _ := requestBody["input"].([]any)
	for _, rawItem := range input {
		item, ok := rawItem.(map[string]any)
		if !ok || !strings.EqualFold(firstNonEmptyString(item["type"]), "additional_tools") {
			continue
		}
		if tools, ok := item["tools"].([]any); ok && len(tools) > 0 {
			return true
		}
	}
	return false
}
