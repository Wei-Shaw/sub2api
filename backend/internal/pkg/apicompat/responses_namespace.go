package apicompat

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// ResponsesNamespaceName identifies a function child in a Responses namespace.
// It aliases the chat bridge mapping so both native and bridged paths share one
// namespace identity contract.
type ResponsesNamespaceName = NamespacedToolName

// FlattenResponsesNamespaces converts Codex private namespace declarations into
// public Responses tools and rewrites namespace-qualified request calls.
// function 与 custom 子工具都会被摊平（见 isFlattenableNamespaceChild）。
func FlattenResponsesNamespaces(req map[string]any) (map[string]ResponsesNamespaceName, bool, error) {
	return FlattenResponsesNamespacesExcept(req, nil)
}

// FlattenResponsesNamespacesExcept is FlattenResponsesNamespaces with a set of
// service-owned namespace names that must remain native in the request.
func FlattenResponsesNamespacesExcept(req map[string]any, preserved map[string]bool) (map[string]ResponsesNamespaceName, bool, error) {
	if req == nil {
		return nil, false, nil
	}
	tools, ok := req["tools"].([]any)
	if !ok || len(tools) == 0 {
		return nil, false, nil
	}

	topLevel := make(map[string]bool)
	for _, raw := range tools {
		tool, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		typ := strings.TrimSpace(stringValue(tool["type"]))
		name := strings.TrimSpace(stringValue(tool["name"]))
		if (typ == "function" || typ == "custom") && name != "" {
			topLevel[name] = true
		}
	}

	names := make(map[string]ResponsesNamespaceName)
	bareNames := make(map[string]string)
	ambiguousBareNames := make(map[string]bool)
	for _, raw := range tools {
		tool, ok := raw.(map[string]any)
		if !ok || strings.TrimSpace(stringValue(tool["type"])) != "namespace" {
			continue
		}
		namespace := strings.TrimSpace(stringValue(tool["name"]))
		if namespace == "" || preserved[namespace] {
			continue
		}
		for _, rawChild := range namespaceChildren(tool) {
			child, ok := rawChild.(map[string]any)
			if !ok || !isFlattenableNamespaceChild(child) {
				continue
			}
			name := strings.TrimSpace(stringValue(child["name"]))
			if name == "" {
				continue
			}
			flat := flattenNamespaceToolName(namespace, name)
			entry := ResponsesNamespaceName{Namespace: namespace, Name: name}
			if topLevel[flat] {
				return nil, false, fmt.Errorf("namespace tool %q/%q flattens to %q which conflicts with a top-level tool of the same name; this upstream cannot disambiguate them, rename one of the tools", namespace, name, flat)
			}
			if prev, exists := names[flat]; exists && prev != entry {
				return nil, false, fmt.Errorf("namespace tools %q/%q and %q/%q both flatten to %q; this upstream cannot disambiguate them, rename one of the tools", prev.Namespace, prev.Name, namespace, name, flat)
			}
			names[flat] = entry
			// Older Codex turns can replay namespace calls without a namespace.
			// Only infer the flat identity when the bare child name is unique
			// and cannot refer to a top-level function/custom tool.
			if topLevel[name] || ambiguousBareNames[name] {
				delete(bareNames, name)
				continue
			}
			if previous, exists := bareNames[name]; exists && previous != flat {
				delete(bareNames, name)
				ambiguousBareNames[name] = true
				continue
			}
			bareNames[name] = flat
		}
	}
	if len(names) == 0 {
		return nil, false, nil
	}

	flattened := make([]any, 0, len(tools)+len(names))
	seen := make(map[string]bool)
	for _, raw := range tools {
		tool, ok := raw.(map[string]any)
		if !ok || strings.TrimSpace(stringValue(tool["type"])) != "namespace" {
			flattened = append(flattened, raw)
			continue
		}
		namespace := strings.TrimSpace(stringValue(tool["name"]))
		if preserved[namespace] {
			flattened = append(flattened, raw)
			continue
		}
		for _, rawChild := range namespaceChildren(tool) {
			child, ok := rawChild.(map[string]any)
			if !ok || !isFlattenableNamespaceChild(child) {
				continue
			}
			name := strings.TrimSpace(stringValue(child["name"]))
			flat := flattenNamespaceToolName(namespace, name)
			if name == "" || seen[flat] {
				continue
			}
			seen[flat] = true
			flatChild := make(map[string]any, len(child))
			for key, value := range child {
				flatChild[key] = value
			}
			flatChild["name"] = flat
			flattened = append(flattened, flatChild)
		}
	}
	req["tools"] = flattened
	rewriteNamespaceCalls(req["input"], names, bareNames)
	if choice, ok := req["tool_choice"].(map[string]any); ok {
		choiceNamespace := strings.TrimSpace(stringValue(choice["name"]))
		if strings.TrimSpace(stringValue(choice["type"])) == "namespace" && !preserved[choiceNamespace] {
			req["tool_choice"] = "auto"
		} else {
			rewriteNamespaceCall(choice, names, bareNames)
		}
	}
	return names, true, nil
}

// RestoreResponsesNamespaceCalls restores flattened tool calls in a JSON
// Responses payload to the namespace/name identity expected by Codex.
// function_call 与 custom_tool_call 都参与还原（见 isNamespaceQualifiedCallType）。
func RestoreResponsesNamespaceCalls(payload []byte, names map[string]ResponsesNamespaceName) ([]byte, bool, error) {
	if len(payload) == 0 || len(names) == 0 {
		return payload, false, nil
	}
	var value any
	if err := json.Unmarshal(payload, &value); err != nil {
		return payload, false, err
	}
	changed := restoreResponsesNamespaceValue(value, names)
	if !changed {
		return payload, false, nil
	}
	var rebuilt bytes.Buffer
	encoder := json.NewEncoder(&rebuilt)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return payload, false, err
	}
	return bytes.TrimSuffix(rebuilt.Bytes(), []byte("\n")), true, nil
}

func namespaceChildren(tool map[string]any) []any {
	if children, ok := tool["tools"].([]any); ok && len(children) > 0 {
		return children
	}
	children, _ := tool["children"].([]any)
	return children
}

// isFlattenableNamespaceChild 判断 namespace 子工具是否参与摊平。
//
// function 与 custom 都必须摊平：Codex CLI 0.147+ 把 exec（唯一能跑命令/读文件的
// 编排工具，输入是自由文本 JavaScript）声明为 namespace 内的 custom 子工具。只放行
// function 会把它静默丢弃，模型于是收到一组碰不到文件系统的工具，表现为"没有提供
// 文件读取或终端执行工具"式的拒绝。
//
// 摊平时保留子工具原本的 type：custom 的降级（type→function + 自由文本 schema）
// 由 AdaptResponsesClientTools 的降级循环统一处理，它跑在摊平之后，会按摊平名把
// 工具登记进 CustomTools，回程才能还原成 custom_tool_call。此处提前降级会让那份
// 登记落空。
func isFlattenableNamespaceChild(child map[string]any) bool {
	switch strings.TrimSpace(stringValue(child["type"])) {
	case "function", "custom":
		return true
	default:
		return false
	}
}

func rewriteNamespaceQualifiedCalls(value any, names map[string]ResponsesNamespaceName) {
	rewriteNamespaceCalls(value, names, nil)
}

func rewriteNamespaceCalls(value any, names map[string]ResponsesNamespaceName, bareNames map[string]string) {
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			rewriteNamespaceCalls(item, names, bareNames)
		}
	case map[string]any:
		if isNamespaceQualifiedCallType(stringValue(typed["type"])) {
			rewriteNamespaceCall(typed, names, bareNames)
		}
		for _, child := range typed {
			rewriteNamespaceCalls(child, names, bareNames)
		}
	}
}

// isNamespaceQualifiedCallType 指示某个历史项类型是否带 namespace+name 身份。
// custom_tool_call 与 function_call 同样按 namespace 路由（namespace 内的 custom
// 子工具，如 Codex 的 exec），漏掉它会让历史里的 exec 调用保持 namespace 限定名，
// 与摊平后的工具声明对不上，上游按未声明工具处理。
func isNamespaceQualifiedCallType(typ string) bool {
	switch strings.TrimSpace(typ) {
	case "function_call", "custom_tool_call":
		return true
	default:
		return false
	}
}

func rewriteNamespaceQualifiedCall(item map[string]any, names map[string]ResponsesNamespaceName) bool {
	return rewriteNamespaceCall(item, names, nil)
}

func rewriteNamespaceCall(item map[string]any, names map[string]ResponsesNamespaceName, bareNames map[string]string) bool {
	namespace := strings.TrimSpace(stringValue(item["namespace"]))
	name := strings.TrimSpace(stringValue(item["name"]))
	if name == "" {
		return false
	}
	if namespace == "" {
		flat, ok := bareNames[name]
		if !ok {
			return false
		}
		item["name"] = flat
		return true
	}
	flat := flattenNamespaceToolName(namespace, name)
	entry, ok := names[flat]
	if !ok || entry.Namespace != namespace || entry.Name != name {
		return false
	}
	item["name"] = flat
	delete(item, "namespace")
	return true
}

func restoreResponsesNamespaceValue(value any, names map[string]ResponsesNamespaceName) bool {
	changed := false
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			changed = restoreResponsesNamespaceValue(item, names) || changed
		}
	case map[string]any:
		if isNamespaceQualifiedCallType(stringValue(typed["type"])) {
			if entry, ok := names[strings.TrimSpace(stringValue(typed["name"]))]; ok {
				typed["name"] = entry.Name
				typed["namespace"] = entry.Namespace
				changed = true
			}
		}
		for _, child := range typed {
			changed = restoreResponsesNamespaceValue(child, names) || changed
		}
	}
	return changed
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}
