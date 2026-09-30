package basispoints

import (
	"container/list"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

type tool struct {
	Name       string
	Namespace  string
	Kind       string
	Definition string
	Parameters object
	Catalog    object
	Schema     *jsonschema.Schema
}

const (
	replayCacheMaxEntries = 1024
	replayCacheMaxBytes   = 16 << 20
	replayCacheEntryBytes = 1 << 20
	replayCacheIdleTTL    = 2 * time.Hour
)

type replayEntry struct {
	key             string
	raw             []byte
	callFingerprint string
	used            time.Time
	weight          int
}

// ReplayCache retains native tool identities without mixing accounts or sessions.
// Both entry count and bytes are bounded because tool arguments can be large.
// Idle entries expire on the next cache operation. The byte budget includes
// payloads, keys, signatures and estimated metadata; it is not an RSS limit.
type ReplayCache struct {
	mu      sync.Mutex
	entries map[string]*list.Element
	order   list.List
	bytes   int
	now     func() time.Time
}

func (c *ReplayCache) currentTime() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
}

func (c *ReplayCache) removeLocked(element *list.Element) {
	entry, _ := element.Value.(replayEntry)
	delete(c.entries, entry.key)
	c.bytes -= entry.weight
	c.order.Remove(element)
}

func (c *ReplayCache) expireLocked(now time.Time) {
	for oldest := c.order.Front(); oldest != nil; oldest = c.order.Front() {
		entry, _ := oldest.Value.(replayEntry)
		if now.Sub(entry.used) < replayCacheIdleTTL {
			return
		}
		c.removeLocked(oldest)
	}
}

func (c *ReplayCache) put(scope, id string, item object, clientCall ...object) {
	if c == nil || id == "" {
		return
	}
	raw, err := json.Marshal(item)
	var signature string
	if err == nil && len(raw) <= replayCacheEntryBytes && len(clientCall) == 1 {
		signature = historyCallFingerprint(clientCall[0])
	}
	key := scope + "\x00" + id
	weight := len(raw) + len(key) + len(signature) + 128
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = make(map[string]*list.Element)
	}
	now := c.currentTime()
	c.expireLocked(now)
	if old := c.entries[key]; old != nil {
		c.removeLocked(old)
	}
	// An uncacheable replacement must not leave an older native identity
	// available to a later output-only replay with the same call ID.
	if err != nil || len(raw) > replayCacheEntryBytes || weight > replayCacheMaxBytes {
		return
	}
	c.entries[key] = c.order.PushBack(replayEntry{key: key, raw: raw, callFingerprint: signature, used: now, weight: weight})
	c.bytes += weight
	for len(c.entries) > replayCacheMaxEntries || c.bytes > replayCacheMaxBytes {
		c.removeLocked(c.order.Front())
	}
}

func (c *ReplayCache) get(scope, id string) object {
	return c.getMatching(scope, id, "", false)
}

// A complete client call is stronger evidence than a reused call ID. Matching
// ignores wire-only item IDs/status and JSON object order, but retains the tool
// kind, namespace, argument values and exact custom input.
func historyCallFingerprint(item object) string {
	kind, id, name := text(item["type"]), text(item["call_id"]), text(item["name"])
	if id == "" || name == "" || strings.TrimSpace(id) != id || strings.TrimSpace(name) != name {
		return ""
	}
	namespace := ""
	if raw, exists := item["namespace"]; exists {
		var ok bool
		namespace, ok = raw.(string)
		if !ok || strings.TrimSpace(namespace) != namespace {
			return ""
		}
	}
	canonical := object{"type": kind, "call_id": id, "name": name, "namespace": namespace}
	switch kind {
	case "function_call":
		arguments := item["arguments"]
		if raw, ok := arguments.(string); ok {
			if decode([]byte(raw), &arguments) != nil {
				return ""
			}
		}
		if args, ok := arguments.(object); !ok || args == nil {
			return ""
		}
		canonical["arguments"] = arguments
	case "custom_tool_call":
		input, ok := item["input"].(string)
		if !ok {
			return ""
		}
		canonical["input"] = input
	default:
		return ""
	}
	return fingerprint(canonical)
}

func (c *ReplayCache) getForCall(scope, id string, clientCall object) object {
	signature := historyCallFingerprint(clientCall)
	if signature == "" {
		return nil
	}
	return c.getMatching(scope, id, signature, true)
}

func (c *ReplayCache) getMatching(scope, id, signature string, requireSignature bool) object {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	now := c.currentTime()
	c.expireLocked(now)
	entry := c.entries[scope+"\x00"+id]
	if entry == nil {
		c.mu.Unlock()
		return nil
	}
	cached, ok := entry.Value.(replayEntry)
	if !ok || (requireSignature && cached.callFingerprint != signature) {
		c.mu.Unlock()
		return nil
	}
	cached.used = now
	entry.Value = cached
	c.order.MoveToBack(entry)
	c.mu.Unlock()
	// Stored JSON is immutable. Decode an independent caller-owned tree
	// outside the global cache lock, including after eviction/replacement.
	var item object
	if decode(cached.raw, &item) != nil {
		return nil
	}
	return item
}

func (b *Bridge) collectTools(value any, namespace string) ([]any, error) {
	var catalog []any
	items, ok := value.([]any)
	if value != nil && !ok {
		return nil, fmt.Errorf("basispoints client tools must be an array")
	}
	for _, raw := range items {
		item, ok := raw.(object)
		if !ok {
			return nil, fmt.Errorf("invalid Basispoints client tool")
		}
		kind, name := text(item["type"]), text(item["name"])
		if kind == "namespace" {
			if name == "" {
				return nil, fmt.Errorf("basispoints client namespaces require a name")
			}
			nestedNamespace := name
			if namespace != "" {
				nestedNamespace = namespace + "." + name
			}
			nested, err := b.collectTools(item["tools"], nestedNamespace)
			if err != nil {
				return nil, err
			}
			catalog = append(catalog, nested...)
			continue
		}
		if isUnsupportedHostedTool(kind) {
			if b.unsupportedTools == nil {
				b.unsupportedTools = make(map[string]bool)
			}
			b.unsupportedTools[kind] = true
			continue
		}
		if kind != "function" && kind != "custom" {
			return nil, fmt.Errorf("basispoints does not support hosted tool %q; use client function or custom tools", kind)
		}
		if name == "" {
			return nil, fmt.Errorf("basispoints client tools require a name")
		}
		key := name
		if namespace != "" {
			key = namespace + "." + name
		}
		entry := object{"type": kind, "name": key}
		for _, field := range []string{"description", "format", "parameters"} {
			if v, exists := item[field]; exists {
				entry[field] = v
			}
		}
		if kind == "function" && entry["parameters"] == nil {
			entry["parameters"] = item["inputSchema"]
			if entry["parameters"] == nil {
				entry["parameters"] = item["input_schema"]
			}
		}
		definition := toolDefinitionFingerprint(item)
		if previous, exists := b.tools[key]; exists {
			if previous.Definition != definition || previous.Namespace != namespace || previous.Name != name {
				return nil, fmt.Errorf("conflicting duplicate Basispoints client tool %q", key)
			}
			continue
		}
		parameters, ok := entry["parameters"].(object)
		if entry["parameters"] != nil && !ok {
			return nil, fmt.Errorf("basispoints function parameters must be a schema object")
		}
		schema, err := compileToolSchema(parameters)
		if err != nil {
			return nil, err
		}
		b.tools[key] = tool{Name: name, Namespace: namespace, Kind: kind, Definition: definition, Parameters: parameters, Schema: schema, Catalog: item}
		catalog = append(catalog, entry)
	}
	return catalog, nil
}

// Tool descriptions and discovery state can change as Codex replays or lazily
// loads its catalog. They do not change how a call is decoded. Keep the first
// declaration (the explicit/inherited catalog precedes historical additions),
// but compare its call contract rather than rejecting annotation-only changes.
// Unknown fields remain part of the signature so new execution constraints
// cannot silently disappear.
func toolDefinitionFingerprint(item object) string {
	definition := make(object, len(item))
	function := text(item["type"]) == "function"
	for field, value := range item {
		switch field {
		case "description", "defer_loading":
			continue
		case "parameters", "inputSchema", "input_schema":
			if function {
				continue
			}
		}
		definition[field] = value
	}
	if function {
		// Match collectTools' existing schema alias precedence exactly.
		parameters := item["parameters"]
		if parameters == nil {
			parameters = item["inputSchema"]
		}
		if parameters == nil {
			parameters = item["input_schema"]
		}
		if parameters != nil {
			definition["parameters"] = parameters
		}
	}
	return fingerprint(definition)
}

// Hosted capabilities cannot be relayed as client function calls. Ignore known
// declarations in automatic mode; forced selections are rejected by Prepare.
func isUnsupportedHostedTool(kind string) bool {
	switch kind {
	case "web_search", "web_search_preview", "web_search_preview_2025_03_11", "web_search_2025_08_26",
		"tool_search", "image_generation", "file_search", "code_interpreter", "computer", "computer_use_preview", "mcp":
		return true
	default:
		return false
	}
}

// rebuildNativeHistoryCall uses only the complete call supplied by the client.
// It does not execute a tool or require that an old tool remain in today's
// catalog. Cached native items remain authoritative when available.
func (b *Bridge) rebuildNativeHistoryCall(item object) (object, error) {
	id, name := text(item["call_id"]), text(item["name"])
	if id == "" || strings.TrimSpace(id) != id || name == "" || strings.TrimSpace(name) != name {
		return nil, fmt.Errorf("basispoints history recovery requires a complete tool call with nonempty call_id and name")
	}
	if value, exists := item["namespace"]; exists {
		namespace, ok := value.(string)
		if !ok || strings.TrimSpace(namespace) != namespace {
			return nil, fmt.Errorf("basispoints history tool namespace must be a string")
		}
		if namespace != "" {
			name = namespace + "." + name
		}
	}
	envelope := object{"name": name}
	switch text(item["type"]) {
	case "function_call":
		arguments := item["arguments"]
		if encoded, ok := arguments.(string); ok {
			if decode([]byte(encoded), &arguments) != nil {
				return nil, fmt.Errorf("basispoints history function arguments must contain one valid JSON object")
			}
		}
		if args, ok := arguments.(object); !ok || args == nil {
			return nil, fmt.Errorf("basispoints history function arguments must be a JSON object")
		}
		envelope["arguments"] = arguments
	case "custom_tool_call":
		input, ok := item["input"].(string)
		if !ok {
			return nil, fmt.Errorf("basispoints history custom tool input must be a string")
		}
		envelope["input"] = input
	default:
		return nil, fmt.Errorf("basispoints history recovery requires a function or custom tool call")
	}
	code, err := json.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("basispoints history tool arguments cannot be serialized")
	}
	outer := object{
		"code": string(code), "summary": "Replay a previously requested client tool",
		"extended_summary": "The supplied client history contains this tool call; consume its recorded result without repeating it.",
		"destructive":      false, "references": []any{},
	}
	// Rebuilt calls are examples for subsequent model turns. Use the same raw
	// transport advertised by today's catalog instead of teaching CUSTOM tools
	// to use the ordinary FUNCTION envelope. Cached native calls stay verbatim.
	if info, ok := b.tools[name]; ok && info.Kind == "custom" && text(item["type"]) == "custom_tool_call" {
		outer["summary"] = customTransportPrefix + name
		outer["code"] = envelope["input"]
		if _, _, err := customTransportEnvelope(outer); err != nil {
			return nil, err
		}
	}
	if info, ok := b.tools[name]; ok && text(item["type"]) == "function_call" && supportsFunctionCodeTransport(name, info.Kind, info.Parameters) {
		args, _ := envelope["arguments"].(object)
		if _, hasCode := args["code"].(string); hasCode {
			outer, err = encodeFunctionCodeTransport(name, args)
			if err != nil {
				return nil, err
			}
		}
	}
	if info, ok := b.tools[name]; ok && text(item["type"]) == "function_call" && supportsFunctionCmdTransport(name, info.Kind, info.Parameters) {
		args, _ := envelope["arguments"].(object)
		if _, hasCmd := args["cmd"].(string); hasCmd {
			outer, err = encodeFunctionCmdTransport(name, args)
			if err != nil {
				return nil, err
			}
		}
	}
	arguments, err := json.Marshal(outer)
	if err != nil {
		return nil, fmt.Errorf("basispoints history transport cannot be serialized")
	}
	itemID := text(item["id"])
	if !strings.HasPrefix(itemID, "fc_") || len(itemID) > 64 {
		itemID = "fc_" + fingerprint(id)
	}
	return object{
		"type": "function_call", "id": itemID, "call_id": id, "name": "run_officejs",
		"arguments": string(arguments), "status": "completed",
	}, nil
}

func (b *Bridge) translateHistory(input []any) ([]any, error) {
	result := make([]any, 0, len(input))
	seenCalls := make(map[string]bool)
	var trigger any
	for index, raw := range input {
		var toolImages object
		item, ok := raw.(object)
		if !ok {
			return nil, fmt.Errorf("invalid Basispoints input item")
		}
		delete(item, "internal_chat_message_metadata_passthrough")
		switch text(item["type"]) {
		case "additional_tools":
			continue
		case "item_reference":
			return nil, fmt.Errorf("basispoints requires full history; item_reference is unsupported")
		case "compaction_trigger":
			trigger = item
			continue
		case "reasoning":
			if encrypted := text(item["encrypted_content"]); encrypted != "" {
				result = append(result, object{"type": "reasoning", "summary": []any{}, "encrypted_content": encrypted})
			}
			continue
		case "function_call", "custom_tool_call":
			id := text(item["call_id"])
			if native := b.replay.getForCall(b.scope, id, item); native != nil {
				item = native
			} else {
				native, err := b.rebuildNativeHistoryCall(item)
				if err != nil {
					return nil, err
				}
				b.replay.put(b.scope, id, native, item)
				item = native
			}
			seenCalls[id] = true
		case "function_call_output", "custom_tool_call_output":
			id := text(item["call_id"])
			if !seenCalls[id] {
				native := b.replay.get(b.scope, id)
				if native == nil {
					return nil, fmt.Errorf("basispoints original tool item is unavailable for this tool result (path=input[%d]); resend the matching complete tool call with its result, or start a new conversation", index)
				}
				result = append(result, native)
				seenCalls[id] = true
			}
			item["type"] = "function_call_output"
			if err := b.validateHistoryContent(item["output"], index, "output"); err != nil {
				return nil, err
			}
			// Codex custom results carry ctco_ IDs. After lowering to a function
			// result, BPS requires an fc_ item ID even when the client supplied one.
			itemID := text(item["id"])
			if itemID == "" {
				itemID = "fc_" + id
			}
			if !strings.HasPrefix(itemID, "fc_") || len(itemID) > 64 {
				itemID = "fc_" + fingerprint(itemID)
			}
			item["id"] = itemID
			toolImages = separateToolImages(item)
		case "configuration_update":
			return nil, fmt.Errorf("basispoints does not support configuration_update; start a new request with the desired effort")
		}
		if err := b.validateHistoryContent(item["content"], index, "content"); err != nil {
			return nil, err
		}
		var err error
		item, err = normalizeHistoryMessage(item, index)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
		if toolImages != nil {
			result = append(result, toolImages)
		}
	}
	if trigger != nil {
		result = append(result, trigger)
	}
	return result, nil
}

func isTool(item object) bool {
	return text(item["type"]) == "function_call" || text(item["type"]) == "custom_tool_call"
}

// translateCall accepts only the declared relay transport and a caller-declared tool.
// It does not evaluate code or dispatch any Excel operation.
func (b *Bridge) translateCall(native object) (object, error) {
	name := text(native["name"])
	if text(native["type"]) == "function_call" && (name == "update_plan" || name == "functions.update_plan") {
		return b.translateNativePlan(native)
	}
	if name != "run_officejs" && name != "functions.run_officejs" {
		return b.translateDirectCatalogCall(native)
	}
	var arguments object
	if value, ok := native["arguments"].(object); ok {
		arguments = value
	} else if err := decode([]byte(text(native["arguments"])), &arguments); err != nil {
		return nil, fmt.Errorf("basispoints returned invalid tool transport arguments")
	}
	if arguments == nil {
		return nil, fmt.Errorf("basispoints returned empty tool transport arguments")
	}
	envelope, marked, err := customTransportEnvelope(arguments)
	rawCustom := marked
	rawCmd := false
	if !marked && err == nil {
		envelope, marked, err = b.functionCodeTransportEnvelope(arguments)
	}
	if !marked && err == nil {
		envelope, marked, err = b.functionCmdTransportEnvelope(arguments)
		rawCmd = marked
	}
	if !marked && err == nil {
		envelope, err = decodeTransportEnvelope(arguments["code"])
		if err != nil {
			if recovered, ok := recoverTransportEnvelope(arguments["code"], b.tools); ok {
				envelope, err = recovered, nil
			} else {
				err = fmt.Errorf("%w; raw CUSTOM input requires summary=codex2api.custom/CATALOG_NAME; raw FUNCTION_CODE requires summary=codex2api.function_code/CATALOG_NAME for an eligible catalog function", err)
			}
		}
	}
	if err != nil {
		return nil, err
	}
	toolName, err := envelopeName(envelope)
	if err != nil {
		return nil, err
	}
	info, allowed := b.tools[toolName]
	if !allowed {
		return nil, unknownClientToolError{}
	}
	result, err := b.finishClientToolCall(native, info, envelope, rawCustom, !rawCmd)
	if err != nil {
		return nil, err
	}
	// run_officejs is a real BPS-native tool, so its item replays upstream verbatim.
	b.rememberReplay(text(native["call_id"]), native, result)
	return result, nil
}

// translateDirectCatalogCall recovers a native tool call the model addressed by the
// client tool's own name instead of through the run_officejs transport. Some turns
// skip the wrapper and call the catalog tool directly; the reference plugins accept
// this rather than failing the whole response. It relays the client's declared tool
// call to the client unchanged and never executes any code. Only exact catalog names
// (optionally carrying a host "functions." display prefix) are accepted; any other
// native tool remains an unsupported-native-tool error.
func (b *Bridge) translateDirectCatalogCall(native object) (object, error) {
	name := text(native["name"])
	info, ok := b.tools[name]
	if !ok {
		if trimmed := strings.TrimPrefix(name, "functions."); trimmed != name {
			info, ok = b.tools[trimmed]
		}
	}
	if !ok {
		return nil, fmt.Errorf("basispoints returned an unsupported native tool; no tool was executed")
	}
	kind := text(native["type"])
	var envelope object
	switch info.Kind {
	case "function":
		if kind != "function_call" {
			return nil, fmt.Errorf("basispoints returned client function tool %q as a %q; no tool was executed", info.Name, kind)
		}
		envelope = object{"name": info.Name, "arguments": native["arguments"]}
	case "custom":
		if kind != "custom_tool_call" {
			return nil, fmt.Errorf("basispoints returned client custom tool %q as a %q; no tool was executed", info.Name, kind)
		}
		input, ok := native["input"].(string)
		if !ok {
			return nil, fmt.Errorf("basispoints direct custom tool input must be a string")
		}
		envelope = object{"name": info.Name, "input": input}
	default:
		return nil, fmt.Errorf("basispoints returned an unsupported native tool; no tool was executed")
	}
	result, err := b.finishClientToolCall(native, info, envelope, false, true)
	if err != nil {
		return nil, err
	}
	// Direct calls can carry real upstream encryption metadata. Only relay
	// that metadata when the arguments still belong to the same client tool.
	if encrypted := native["encrypted_function_args"]; encrypted != nil && info.Kind == "function" {
		result["encrypted_function_args"] = encrypted
	}
	// The model bypassed run_officejs, so the bare native name is not a BPS tool.
	// Cache a transport-wrapped replay so the next turn presents a BPS-known
	// run_officejs item, matching how absent history is rebuilt.
	wrapped, err := b.rebuildNativeHistoryCall(result)
	if err != nil {
		return nil, err
	}
	b.rememberReplay(text(native["call_id"]), wrapped, result)
	return result, nil
}

// finishClientToolCall builds the client-facing tool item from a resolved catalog
// tool and its envelope. It performs no caching and executes nothing; callers decide
// how the call replays upstream.
func (b *Bridge) finishClientToolCall(native object, info tool, envelope object, marked, validateSchema bool) (object, error) {
	if marked && info.Kind != "custom" {
		return nil, fmt.Errorf("basispoints raw transport requires a declared custom tool")
	}
	id := text(native["call_id"])
	if id == "" {
		return nil, fmt.Errorf("basispoints tool call is missing call_id")
	}
	itemID := text(native["id"])
	if itemID == "" {
		itemID = "fc_" + fingerprint(id)
	}
	result := object{"type": info.Kind + "_call", "id": itemID, "call_id": id, "name": info.Name, "status": "completed"}
	if info.Namespace != "" {
		result["namespace"] = info.Namespace
	}
	if info.Kind == "custom" {
		value, hasInput := envelope["input"]
		if alias, hasAlias := envelope["args"]; hasAlias {
			if hasInput {
				return nil, fmt.Errorf("basispoints custom tool envelope contains conflicting input fields")
			}
			value = alias
		}
		if _, exists := envelope["arguments"]; exists {
			return nil, fmt.Errorf("basispoints custom tools require input text, not arguments")
		}
		input, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("basispoints custom tool input must be a string")
		}
		result["type"] = "custom_tool_call"
		result["id"] = "ctc_" + fingerprint(id)
		result["input"] = input
	} else {
		args, err := envelopeArguments(envelope)
		if err != nil {
			return nil, err
		}
		if raw, ok := args.(string); ok {
			if decode([]byte(raw), &args) != nil {
				return nil, fmt.Errorf("basispoints function arguments are invalid JSON")
			}
		}
		if _, ok := args.(object); !ok {
			return nil, fmt.Errorf("basispoints function arguments must be an object")
		}
		if info.Schema != nil {
			if err := validateToolNumberBudget(args); err != nil {
				return nil, toolArgumentsSchemaError{}
			}
		}
		// FUNCTION_CMD preserves client-validated metadata verbatim. Ordinary
		// function envelopes still enforce their complete declared schema.
		if validateSchema && info.Schema != nil && info.Schema.Validate(args) != nil {
			return nil, toolArgumentsSchemaError{}
		}
		encoded, _ := json.Marshal(args)
		result["arguments"] = string(encoded)
		// The relay envelope contains plaintext, even when a catalog parameter
		// declares encrypted:true. Codex collaboration tools distinguish an
		// explicit empty list from a missing field: without it, they incorrectly
		// package plaintext messages as encrypted_content for the child agent.
		result["encrypted_function_args"] = []string{}
	}
	return result, nil
}

type replayWrite struct {
	id             string
	native, client object
}

func (b *Bridge) rememberReplay(id string, native, client object) {
	if b.stagedReplays != nil {
		*b.stagedReplays = append(*b.stagedReplays, replayWrite{id, native, client})
		return
	}
	b.replay.put(b.scope, id, native, client)
}

// Validate the whole batch before changing output or committing any replay entry.
func (b *Bridge) translateResponse(response object) error {
	if response == nil {
		return nil
	}
	// Validate the whole batch before mutating output or committing replay items.
	if err := b.validateToolResponse(response); err != nil {
		return err
	}
	output, _ := response["output"].([]any)
	translated := make([]any, len(output))
	var writes []replayWrite
	staged := *b
	staged.stagedReplays = &writes
	ids := make(map[string]bool)
	count := 0
	for i, raw := range output {
		item, _ := raw.(object)
		if isTool(item) {
			count++
			if count > 1024 || (b.disallowParallel && count > 1) {
				return fmt.Errorf("basispoints response violates the tool call count limit")
			}
			id := text(item["call_id"])
			if id == "" || ids[id] {
				return fmt.Errorf("basispoints response contains a missing or duplicate tool call_id")
			}
			ids[id] = true
			call, err := staged.translateCall(item)
			if err != nil {
				return err
			}
			translated[i] = call
		} else {
			translated[i] = raw
		}
	}
	for _, write := range writes {
		b.replay.put(b.scope, write.id, write.native, write.client)
	}
	response["output"] = translated
	response["reasoning"] = object{"effort": b.Effort}
	response["parallel_tool_calls"] = false
	return nil
}

func isToolEvent(kind string) bool {
	return strings.HasPrefix(kind, "response.function_call_arguments.") || strings.HasPrefix(kind, "response.custom_tool_call_input.")
}
