package apicompat

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// responsesCollaborationNamespace is the reserved Codex MultiAgentV2 namespace
// whose message-bearing tools may declare their message parameter as
// encrypted:true. Upstreams that cannot perform that client-side encryption
// still receive the call in plaintext after the marked tools are lowered to
// ordinary function tools.
const responsesCollaborationNamespace = "collaboration"

// responsesCollaborationMessageTools lists the namespace children this adapter
// is allowed to lower. Control tools in the same namespace (wait/list style)
// and every other namespace are never touched.
var responsesCollaborationMessageTools = map[string]bool{
	"spawn_agent":   true,
	"send_message":  true,
	"followup_task": true,
}

// ResponsesCollaborationPlaintextMapping records the reversible lowering
// applied to one request. Calls maps each deterministic alias
// (collaboration__<tool>) to the original namespace/name pair the upstream's
// function_call must be restored to. It is per-request state and must not be
// shared across requests.
type ResponsesCollaborationPlaintextMapping struct {
	Calls map[string]ResponsesNamespaceName
}

// AdaptResponsesCollaborationPlaintext lowers marked collaboration namespace
// message tools in req to ordinary top-level function tools so an upstream
// that cannot honour the encrypted:true message annotation still receives the
// call in plaintext. The tools stay callable under a deterministic alias; the
// returned mapping restores upstream function_calls to the namespace identity
// Codex expects.
//
// A target child is lowered only when its parameters.properties.message schema
// is a string property annotated encrypted:true; only the encrypted annotation
// is removed. Children without the marker stay inside the namespace untouched.
// Malformed or unsupported target declarations abort with an explicit error
// instead of being guessed at, as do alias collisions with any declared
// function/custom tool or flattened namespace child. Both the top-level tools
// array and input[] items of type additional_tools are processed; arbitrary
// user JSON and tool argument payloads are never rewritten recursively.
func AdaptResponsesCollaborationPlaintext(req map[string]any) (ResponsesCollaborationPlaintextMapping, bool, error) {
	mapping := ResponsesCollaborationPlaintextMapping{}
	if req == nil {
		return mapping, false, nil
	}
	carriers := collaborationPlaintextCarriers(req)
	if len(carriers) == 0 {
		return mapping, false, nil
	}

	// Collect every declared identity so minted aliases cannot collide with an
	// existing top-level function/custom tool or another flattened namespace
	// child across either carrier.
	declared := make(map[string]bool)
	flattened := make(map[string]ResponsesNamespaceName)
	for _, carrier := range carriers {
		for _, raw := range carrier.tools {
			tool, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			name := strings.TrimSpace(stringValue(tool["name"]))
			switch strings.TrimSpace(stringValue(tool["type"])) {
			case "function", "custom":
				if name != "" {
					declared[name] = true
				}
			case "namespace":
				if name == "" {
					continue
				}
				for _, rawChild := range namespaceChildren(tool) {
					child, ok := rawChild.(map[string]any)
					if !ok || strings.TrimSpace(stringValue(child["type"])) != "function" {
						continue
					}
					childName := strings.TrimSpace(stringValue(child["name"]))
					if childName == "" {
						continue
					}
					flattened[flattenNamespaceToolName(name, childName)] = ResponsesNamespaceName{Namespace: name, Name: childName}
				}
			}
		}
	}

	// Classify every target child before mutating: a malformed declaration, an
	// ambiguous namespace, a duplicate with a different schema, or an alias
	// conflict must abort without a partially rewritten request. The scan is
	// read-only: string/RawMessage schemas are decoded into temporaries and the
	// originals are normalized only when a child is actually converted.
	aliases := make(map[string]ResponsesNamespaceName)
	aliasDecl := make(map[string]string)
	for _, carrier := range carriers {
		for _, raw := range carrier.tools {
			tool, ok := raw.(map[string]any)
			if !ok || strings.TrimSpace(stringValue(tool["type"])) != "namespace" || strings.TrimSpace(stringValue(tool["name"])) != responsesCollaborationNamespace {
				continue
			}
			children, skip, err := collaborationNamespaceTargetChildren(tool)
			if err != nil {
				return ResponsesCollaborationPlaintextMapping{}, false, err
			}
			if skip {
				continue
			}
			for _, rawChild := range children {
				child, ok := rawChild.(map[string]any)
				if !ok || strings.TrimSpace(stringValue(child["type"])) != "function" {
					continue
				}
				name := strings.TrimSpace(stringValue(child["name"]))
				if !responsesCollaborationMessageTools[name] {
					continue
				}
				convert, err := collaborationMessageMarkedEncrypted(child)
				if err != nil {
					return ResponsesCollaborationPlaintextMapping{}, false, err
				}
				if !convert {
					continue
				}
				alias := flattenNamespaceToolName(responsesCollaborationNamespace, name)
				entry := ResponsesNamespaceName{Namespace: responsesCollaborationNamespace, Name: name}
				if declared[alias] {
					return ResponsesCollaborationPlaintextMapping{}, false, fmt.Errorf("collaboration tool %q lowers to %q which conflicts with a declared tool of the same name; this upstream cannot disambiguate them, rename one of the tools", name, alias)
				}
				if prev, exists := flattened[alias]; exists && prev != entry {
					return ResponsesCollaborationPlaintextMapping{}, false, fmt.Errorf("collaboration tool %q lowers to %q which also flattens namespace tool %q/%q; this upstream cannot disambiguate them, rename one of the tools", name, alias, prev.Namespace, prev.Name)
				}
				encoded, err := json.Marshal(child)
				if err != nil {
					return ResponsesCollaborationPlaintextMapping{}, false, fmt.Errorf("collaboration tool %q declaration cannot be encoded for duplicate comparison: %w", name, err)
				}
				if prev, exists := aliasDecl[alias]; exists && prev != string(encoded) {
					return ResponsesCollaborationPlaintextMapping{}, false, fmt.Errorf("collaboration tool %q is declared more than once with different schemas; this upstream cannot disambiguate them, keep a single declaration", name)
				}
				aliasDecl[alias] = string(encoded)
				aliases[alias] = entry
			}
		}
	}
	if len(aliases) == 0 {
		return mapping, false, nil
	}
	mapping.Calls = aliases

	// A tool_choice selecting the whole collaboration namespace can no longer
	// be expressed once its message tools became plain functions. Validate the
	// entire choice tree before any mutation so this error never leaves a
	// partially rewritten request behind.
	if err := rejectCollaborationNamespaceChoice(req["tool_choice"]); err != nil {
		return ResponsesCollaborationPlaintextMapping{}, false, err
	}

	changed := false
	choiceChanged, err := rewriteCollaborationPlaintextChoice(req["tool_choice"], mapping)
	if err != nil {
		return ResponsesCollaborationPlaintextMapping{}, false, err
	}
	changed = choiceChanged || changed
	for _, carrier := range carriers {
		lowered, carrierChanged := lowerCollaborationCarrierTools(carrier.tools, mapping)
		if carrierChanged {
			carrier.commit(lowered)
			changed = true
		}
	}
	if rewriteCollaborationPlaintextInputCalls(req["input"], mapping) {
		changed = true
	}
	return mapping, changed, nil
}

// collaborationToolCarrier is one mutable tool declaration list: either the
// top-level tools field or the tools array of an input[] additional_tools item.
type collaborationToolCarrier struct {
	tools  []any
	commit func([]any)
}

func collaborationPlaintextCarriers(req map[string]any) []collaborationToolCarrier {
	var carriers []collaborationToolCarrier
	if tools, ok := req["tools"].([]any); ok && len(tools) > 0 {
		carriers = append(carriers, collaborationToolCarrier{
			tools:  tools,
			commit: func(updated []any) { req["tools"] = updated },
		})
	}
	input, ok := req["input"].([]any)
	if !ok {
		return carriers
	}
	for _, rawItem := range input {
		item, ok := rawItem.(map[string]any)
		if !ok || strings.TrimSpace(stringValue(item["type"])) != "additional_tools" {
			continue
		}
		tools, ok := item["tools"].([]any)
		if !ok || len(tools) == 0 {
			continue
		}
		carriers = append(carriers, collaborationToolCarrier{
			tools:  tools,
			commit: func(updated []any) { item["tools"] = updated },
		})
	}
	return carriers
}

// lowerCollaborationCarrierTools rebuilds one carrier's declaration list:
// converted children leave the collaboration namespace and are appended as
// flat function tools at the namespace's position. The namespace keeps its
// remaining children verbatim and is dropped only when nothing remains.
func lowerCollaborationCarrierTools(tools []any, mapping ResponsesCollaborationPlaintextMapping) ([]any, bool) {
	seen := make(map[string]bool)
	out := make([]any, 0, len(tools)+len(mapping.Calls))
	changed := false
	for _, raw := range tools {
		tool, ok := raw.(map[string]any)
		if !ok || strings.TrimSpace(stringValue(tool["type"])) != "namespace" || strings.TrimSpace(stringValue(tool["name"])) != responsesCollaborationNamespace {
			out = append(out, raw)
			continue
		}
		// A namespace carrying non-empty tools and children keys at once was
		// classified as ambiguous upstream: never partially lower it here.
		toolsList, _ := tool["tools"].([]any)
		childrenList, _ := tool["children"].([]any)
		if len(toolsList) > 0 && len(childrenList) > 0 {
			out = append(out, raw)
			continue
		}
		key, children := collaborationNamespaceChildList(tool)
		remaining := make([]any, 0, len(children))
		var lowered []any
		nsChanged := false
		for _, rawChild := range children {
			child, ok := rawChild.(map[string]any)
			name := ""
			if ok {
				name = strings.TrimSpace(stringValue(child["name"]))
			}
			alias := flattenNamespaceToolName(responsesCollaborationNamespace, name)
			// A child converts only when this very declaration carries the
			// encrypted marker — a minted alias alone is never a license to
			// convert an unmarked or skipped sibling declaration.
			convert := ok && strings.TrimSpace(stringValue(child["type"])) == "function" &&
				responsesCollaborationMessageTools[name] &&
				mapping.Calls[alias] == (ResponsesNamespaceName{Namespace: responsesCollaborationNamespace, Name: name})
			if convert {
				marked, err := collaborationMessageMarkedEncrypted(child)
				convert = err == nil && marked
			}
			if !convert {
				remaining = append(remaining, rawChild)
				continue
			}
			nsChanged = true
			if seen[alias] {
				continue
			}
			seen[alias] = true
			child["name"] = alias
			stripCollaborationMessageEncryption(child)
			lowered = append(lowered, child)
		}
		if !nsChanged {
			out = append(out, raw)
			continue
		}
		changed = true
		if len(remaining) > 0 {
			tool[key] = remaining
		} else {
			delete(tool, key)
		}
		if len(remaining) > 0 || collaborationNamespaceSiblingChildren(tool, key) {
			out = append(out, tool)
		}
		out = append(out, lowered...)
	}
	if !changed {
		return tools, false
	}
	return out, true
}

// collaborationNamespaceTargetChildren returns the child list to inspect for
// targets. A namespace that carries non-empty tools and children keys at once
// is ambiguous: it is an explicit error when either list holds a convertible
// target, and is skipped entirely (left untouched) when none does.
func collaborationNamespaceTargetChildren(tool map[string]any) ([]any, bool, error) {
	toolsList, _ := tool["tools"].([]any)
	childrenList, _ := tool["children"].([]any)
	if len(toolsList) == 0 || len(childrenList) == 0 {
		return namespaceChildren(tool), false, nil
	}
	for _, list := range [][]any{toolsList, childrenList} {
		for _, rawChild := range list {
			child, ok := rawChild.(map[string]any)
			if !ok || strings.TrimSpace(stringValue(child["type"])) != "function" {
				continue
			}
			if !responsesCollaborationMessageTools[strings.TrimSpace(stringValue(child["name"]))] {
				continue
			}
			convert, err := collaborationMessageMarkedEncrypted(child)
			if err != nil {
				return nil, false, err
			}
			if convert {
				return nil, false, fmt.Errorf("collaboration namespace declares children under both \"tools\" and \"children\"; the declaration is ambiguous and cannot be lowered safely")
			}
		}
	}
	return nil, true, nil
}

// collaborationNamespaceChildList returns the child-list key actually used by
// a namespace declaration (tools preferred, children accepted like
// namespaceChildren) together with the list itself.
func collaborationNamespaceChildList(tool map[string]any) (string, []any) {
	if children, ok := tool["tools"].([]any); ok && len(children) > 0 {
		return "tools", children
	}
	children, _ := tool["children"].([]any)
	return "children", children
}

// collaborationNamespaceSiblingChildren reports whether the namespace keeps
// children under the key that was not processed, so an emptied processed list
// alone does not drop the declaration.
func collaborationNamespaceSiblingChildren(tool map[string]any, processedKey string) bool {
	sibling := "children"
	if processedKey == "children" {
		sibling = "tools"
	}
	children, _ := tool[sibling].([]any)
	return len(children) > 0
}

// collaborationMessageMarkedEncrypted reports whether a target child's message
// parameter is declared as a string with encrypted:true. Missing or unmarked
// schemas return false; malformed declarations and encrypted non-string
// message parameters return an explicit error.
func collaborationMessageMarkedEncrypted(child map[string]any) (bool, error) {
	msg, err := collaborationMessageSchema(child)
	if err != nil {
		return false, fmt.Errorf("collaboration tool %q: %w", strings.TrimSpace(stringValue(child["name"])), err)
	}
	if msg == nil {
		return false, nil
	}
	raw, present := msg["encrypted"]
	if !present || raw == nil {
		return false, nil
	}
	encrypted, ok := raw.(bool)
	if !ok {
		return false, fmt.Errorf("collaboration tool %q declares a non-boolean encrypted marker on its message parameter; refusing to guess", strings.TrimSpace(stringValue(child["name"])))
	}
	if !encrypted {
		return false, nil
	}
	if strings.TrimSpace(stringValue(msg["type"])) != "string" {
		return false, fmt.Errorf("collaboration tool %q marks a non-string message parameter as encrypted; only string message parameters can be lowered to plaintext", strings.TrimSpace(stringValue(child["name"])))
	}
	return true, nil
}

// collaborationMessageSchema returns the parameters.properties.message schema
// map of a function child, normalizing string/RawMessage parameter
// declarations in place. A nil map means the child carries no inspectable
// message schema; an error means a present declaration is malformed.
func collaborationMessageSchema(child map[string]any) (map[string]any, error) {
	params, err := collaborationSchemaField(child, "parameters")
	if err != nil || params == nil {
		return nil, err
	}
	props, err := collaborationSchemaField(params, "properties")
	if err != nil || props == nil {
		return nil, err
	}
	return collaborationSchemaField(props, "message")
}

// collaborationSchemaField reads a schema sub-object without mutating the
// declaration: string/RawMessage forms are decoded into a temporary map so a
// no-op or error path leaves the original representation untouched.
func collaborationSchemaField(container map[string]any, key string) (map[string]any, error) {
	raw, present := container[key]
	if !present || raw == nil {
		return nil, nil
	}
	switch typed := raw.(type) {
	case map[string]any:
		return typed, nil
	case json.RawMessage:
		return collaborationDecodeSchema(key, typed)
	case string:
		return collaborationDecodeSchema(key, []byte(typed))
	default:
		return nil, fmt.Errorf("declaration has a malformed %s field; refusing to guess its message schema", key)
	}
}

func collaborationDecodeSchema(key string, raw []byte) (map[string]any, error) {
	var decoded any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&decoded); err != nil {
		return nil, fmt.Errorf("declaration has a malformed %s field; refusing to guess its message schema", key)
	}
	obj, ok := decoded.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("declaration has a malformed %s field; refusing to guess its message schema", key)
	}
	return obj, nil
}

// stripCollaborationMessageEncryption removes the encrypted annotation from a
// converted child's message schema. String/RawMessage schema fields are
// normalized into decoded objects only here, after the child is committed to
// conversion; the classification scan never writes back.
func stripCollaborationMessageEncryption(child map[string]any) {
	params := collaborationWritableSchemaField(child, "parameters")
	props := collaborationWritableSchemaField(params, "properties")
	if msg := collaborationWritableSchemaField(props, "message"); msg != nil {
		delete(msg, "encrypted")
	}
}

func collaborationWritableSchemaField(container map[string]any, key string) map[string]any {
	if container == nil {
		return nil
	}
	switch typed := container[key].(type) {
	case map[string]any:
		return typed
	case json.RawMessage:
		obj, err := collaborationDecodeSchema(key, typed)
		if err == nil {
			container[key] = obj
		}
		return obj
	case string:
		obj, err := collaborationDecodeSchema(key, []byte(typed))
		if err == nil {
			container[key] = obj
		}
		return obj
	default:
		return nil
	}
}

// rewriteCollaborationPlaintextInputCalls rewrites direct input[] function_call
// items that address a lowered tool by namespace+name to the alias form.
// Nested user JSON and argument payloads are never descended into.
func rewriteCollaborationPlaintextInputCalls(input any, mapping ResponsesCollaborationPlaintextMapping) bool {
	items, ok := input.([]any)
	if !ok {
		return false
	}
	changed := false
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok || strings.TrimSpace(stringValue(item["type"])) != "function_call" {
			continue
		}
		changed = rewriteCollaborationPlaintextCall(item, mapping) || changed
	}
	return changed
}

func rewriteCollaborationPlaintextCall(item map[string]any, mapping ResponsesCollaborationPlaintextMapping) bool {
	namespace := strings.TrimSpace(stringValue(item["namespace"]))
	name := strings.TrimSpace(stringValue(item["name"]))
	if namespace != responsesCollaborationNamespace || name == "" {
		return false
	}
	alias := flattenNamespaceToolName(namespace, name)
	entry, ok := mapping.Calls[alias]
	if !ok || entry.Name != name || entry.Namespace != namespace {
		return false
	}
	item["name"] = alias
	delete(item, "namespace")
	return true
}

// rejectCollaborationNamespaceChoice fails when tool_choice selects the whole
// collaboration namespace at a known reference position — a top-level choice
// entry or an entry of an allowed_tools choice's tools list: after lowering,
// that selection cannot be expressed upstream, and silently degrading it to
// auto would change client intent. It runs before any mutation so the error
// path leaves the request untouched. Namespace-shaped objects inside meta or
// unknown wrappers are not real selections: they are neither rejected nor
// rewritten, mirroring rewriteCollaborationPlaintextChoice's traversal.
func rejectCollaborationNamespaceChoice(value any) error {
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			if err := rejectCollaborationNamespaceChoice(item); err != nil {
				return err
			}
		}
	case map[string]any:
		switch strings.TrimSpace(stringValue(typed["type"])) {
		case "namespace":
			if strings.TrimSpace(stringValue(typed["name"])) == responsesCollaborationNamespace {
				return fmt.Errorf("tool_choice selects reserved namespace %q whose message tools were lowered to plaintext functions; this upstream cannot express that selection, choose a specific tool instead", responsesCollaborationNamespace)
			}
		case "allowed_tools":
			return rejectCollaborationNamespaceChoice(typed["tools"])
		}
	}
	return nil
}

// rewriteCollaborationPlaintextChoice rewrites tool_choice references to
// lowered tools to their alias. Only known reference shapes are touched —
// function/custom entries carrying namespace:"collaboration" plus a target
// name, and the tools list of an allowed_tools choice — matching the
// response-side restorer exactly, so every rewritten reference round-trips.
// Namespace-wide selections are rejected beforehand by
// rejectCollaborationNamespaceChoice.
func rewriteCollaborationPlaintextChoice(value any, mapping ResponsesCollaborationPlaintextMapping) (bool, error) {
	changed := false
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			itemChanged, err := rewriteCollaborationPlaintextChoice(item, mapping)
			if err != nil {
				return false, err
			}
			changed = itemChanged || changed
		}
	case map[string]any:
		switch strings.TrimSpace(stringValue(typed["type"])) {
		case "function", "custom":
			if rewriteCollaborationPlaintextCall(typed, mapping) {
				changed = true
			}
		case "allowed_tools":
			toolsChanged, err := rewriteCollaborationPlaintextChoice(typed["tools"], mapping)
			if err != nil {
				return false, err
			}
			changed = toolsChanged || changed
		}
	}
	return changed, nil
}

// RestoreResponsesCollaborationPlaintextPayload restores one upstream JSON
// payload (a non-stream response body or a single SSE/WS event payload) to the
// namespace identity Codex expects. Only known protocol positions are touched:
// response.output[] and top-level output[] items, the item of output_item
// added/done events, the name of function_call_arguments delta/done events,
// and echoed tools/tool_choice declarations. Restored function_calls gain an
// explicit encrypted_function_args:[] marker; a non-empty upstream
// encrypted_function_args is never overwritten and aborts with an error that
// does not expose the payload. Unknown aliases and unrelated user data are
// left untouched, and an unchanged payload is returned byte-for-byte.
func RestoreResponsesCollaborationPlaintextPayload(payload []byte, mapping ResponsesCollaborationPlaintextMapping) ([]byte, bool, error) {
	if len(payload) == 0 || len(mapping.Calls) == 0 {
		return payload, false, nil
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return payload, false, err
	}
	changed, err := restoreCollaborationPlaintextRoot(value, mapping)
	if err != nil {
		return payload, false, err
	}
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

func restoreCollaborationPlaintextRoot(value any, mapping ResponsesCollaborationPlaintextMapping) (bool, error) {
	root, ok := value.(map[string]any)
	if !ok {
		return false, nil
	}
	changed := false
	// A non-stream response body carries output/tools/tool_choice at the root;
	// terminal stream events wrap the same object under response.
	containers := []map[string]any{root}
	if response, ok := root["response"].(map[string]any); ok {
		containers = append(containers, response)
	}
	for _, container := range containers {
		outputChanged, err := restoreCollaborationPlaintextOutput(container["output"], mapping)
		if err != nil {
			return false, err
		}
		if toolsRestored, toolsChanged := restoreCollaborationPlaintextTools(container["tools"], mapping); toolsChanged {
			container["tools"] = toolsRestored
			changed = true
		}
		choiceChanged, err := restoreCollaborationPlaintextChoice(container["tool_choice"], mapping)
		if err != nil {
			return false, err
		}
		changed = outputChanged || choiceChanged || changed
	}
	if item, ok := root["item"].(map[string]any); ok {
		itemChanged, err := restoreCollaborationPlaintextCallItem(item, mapping)
		if err != nil {
			return false, err
		}
		changed = itemChanged || changed
	}
	switch strings.TrimSpace(stringValue(root["type"])) {
	case "response.function_call_arguments.delta", "response.function_call_arguments.done":
		if entry, ok := mapping.Calls[strings.TrimSpace(stringValue(root["name"]))]; ok {
			root["name"] = entry.Name
			changed = true
		}
	}
	return changed, nil
}

func restoreCollaborationPlaintextOutput(output any, mapping ResponsesCollaborationPlaintextMapping) (bool, error) {
	items, ok := output.([]any)
	if !ok {
		return false, nil
	}
	changed := false
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		itemChanged, err := restoreCollaborationPlaintextCallItem(item, mapping)
		if err != nil {
			return false, err
		}
		changed = itemChanged || changed
	}
	return changed, nil
}

// restoreCollaborationPlaintextCallItem restores one function_call item that
// names a lowered alias back to its namespace/name pair and marks
// encrypted_function_args explicitly empty.
func restoreCollaborationPlaintextCallItem(item map[string]any, mapping ResponsesCollaborationPlaintextMapping) (bool, error) {
	if strings.TrimSpace(stringValue(item["type"])) != "function_call" {
		return false, nil
	}
	alias := strings.TrimSpace(stringValue(item["name"]))
	entry, ok := mapping.Calls[alias]
	if !ok {
		return false, nil
	}
	if raw, present := item["encrypted_function_args"]; present {
		switch typed := raw.(type) {
		case nil:
		case []any:
			if len(typed) > 0 {
				return false, fmt.Errorf("collaboration call %q returned a non-empty encrypted_function_args; refusing to forward opaque ciphertext as plaintext", alias)
			}
		default:
			return false, fmt.Errorf("collaboration call %q returned a malformed encrypted_function_args; refusing to forward it", alias)
		}
	}
	item["name"] = entry.Name
	item["namespace"] = entry.Namespace
	item["encrypted_function_args"] = []any{}
	return true, nil
}

// restoreCollaborationPlaintextTools restores echoed tool declarations: a flat
// function naming an alias becomes a child of the collaboration namespace
// again, merged into an existing echoed namespace declaration when present,
// otherwise re-wrapped at the position of the first aliased function.
func restoreCollaborationPlaintextTools(tools any, mapping ResponsesCollaborationPlaintextMapping) ([]any, bool) {
	list, ok := tools.([]any)
	if !ok {
		return nil, false
	}
	var restored []any
	hasNamespace := false
	for _, raw := range list {
		tool, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		typ := strings.TrimSpace(stringValue(tool["type"]))
		name := strings.TrimSpace(stringValue(tool["name"]))
		if typ == "namespace" && name == responsesCollaborationNamespace {
			hasNamespace = true
			continue
		}
		if typ != "function" {
			continue
		}
		entry, ok := mapping.Calls[name]
		if !ok {
			continue
		}
		child := copyClientTool(tool)
		child["name"] = entry.Name
		restoreCollaborationMessageMarker(child)
		restored = append(restored, child)
	}
	if len(restored) == 0 {
		return nil, false
	}
	out := make([]any, 0, len(list))
	merged := false
	inserted := false
	for _, raw := range list {
		tool, ok := raw.(map[string]any)
		if !ok {
			out = append(out, raw)
			continue
		}
		typ := strings.TrimSpace(stringValue(tool["type"]))
		name := strings.TrimSpace(stringValue(tool["name"]))
		if typ == "namespace" && name == responsesCollaborationNamespace {
			if !merged {
				key := collaborationNamespaceWriteKey(tool)
				children, _ := tool[key].([]any)
				combined := make([]any, 0, len(children)+len(restored))
				combined = append(combined, children...)
				combined = append(combined, restored...)
				tool[key] = combined
				merged = true
			}
			out = append(out, tool)
			continue
		}
		if typ == "function" {
			if _, isAlias := mapping.Calls[name]; isAlias {
				if !hasNamespace && !inserted {
					out = append(out, map[string]any{
						"type": "namespace", "name": responsesCollaborationNamespace, "tools": restored,
					})
					inserted = true
				}
				continue
			}
		}
		out = append(out, raw)
	}
	return out, true
}

// collaborationNamespaceWriteKey picks the child-list key an existing
// namespace declaration already uses, defaulting to tools.
func collaborationNamespaceWriteKey(tool map[string]any) string {
	if _, ok := tool["tools"].([]any); ok {
		return "tools"
	}
	if _, ok := tool["children"].([]any); ok {
		return "children"
	}
	return "tools"
}

// restoreCollaborationMessageMarker re-adds the encrypted:true annotation the
// adapter stripped, so an echoed declaration matches what the client sent.
// Each map along the path is copied before the write so the source payload
// node is never mutated.
func restoreCollaborationMessageMarker(child map[string]any) {
	params, ok := child["parameters"].(map[string]any)
	if !ok {
		return
	}
	props, ok := params["properties"].(map[string]any)
	if !ok {
		return
	}
	msg, ok := props["message"].(map[string]any)
	if !ok {
		return
	}
	msgCopy := copyClientTool(msg)
	msgCopy["encrypted"] = true
	propsCopy := copyClientTool(props)
	propsCopy["message"] = msgCopy
	paramsCopy := copyClientTool(params)
	paramsCopy["properties"] = propsCopy
	child["parameters"] = paramsCopy
}

// restoreCollaborationPlaintextChoice restores alias references inside an
// echoed tool_choice subtree back to the namespace/name addressing the client
// used. Only known reference shapes are touched: function/custom entries whose
// name is a mapped alias, and the tools list of an allowed_tools choice. Maps
// of any other type and name fields nested inside unknown structures are left
// exactly as the upstream sent them.
func restoreCollaborationPlaintextChoice(value any, mapping ResponsesCollaborationPlaintextMapping) (bool, error) {
	changed := false
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			itemChanged, err := restoreCollaborationPlaintextChoice(item, mapping)
			if err != nil {
				return false, err
			}
			changed = itemChanged || changed
		}
	case map[string]any:
		switch strings.TrimSpace(stringValue(typed["type"])) {
		case "function", "custom":
			if entry, ok := mapping.Calls[strings.TrimSpace(stringValue(typed["name"]))]; ok {
				typed["name"] = entry.Name
				typed["namespace"] = entry.Namespace
				changed = true
			}
		case "allowed_tools":
			toolsChanged, err := restoreCollaborationPlaintextChoice(typed["tools"], mapping)
			if err != nil {
				return false, err
			}
			changed = toolsChanged || changed
		}
	}
	return changed, nil
}
