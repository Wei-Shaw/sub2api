package apicompat

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// ResponsesClientToolMapping records the reversible lowering applied before a
// native Responses request is sent to an upstream that only understands
// function tools.
type ResponsesClientToolMapping struct {
	CustomTools    map[string]bool
	ToolSearch     bool
	NamespaceTools map[string]ResponsesNamespaceName
REDACTED

// AdaptResponsesClientTools lowers Codex client-only tools in req to
// ordinary function tools. It mutates req and returns the mapping required to
// restore the upstream response.
func AdaptResponsesClientTools(req map[string]any) (ResponsesClientToolMapping, bool, error) {
	if req == nil {
		return ResponsesClientToolMapping{REDACTED, false, nil
REDACTED
	tools, ok := req["tools"].([]any)
	if !ok || len(tools) == 0 {
		return ResponsesClientToolMapping{REDACTED, false, nil
REDACTED

	adapter := ResponsesClientToolMapping{CustomTools: make(map[string]bool)REDACTED
	functionNames := make(map[string]bool)
	customNames := make(map[string]bool)
	for _, raw := range tools {
		tool, ok := raw.(map[string]any)
		if !ok {
			continue
	REDACTED
		name := strings.TrimSpace(stringValue(tool["name"]))
		switch strings.TrimSpace(stringValue(tool["type"])) {
		case "function":
			if name != "" {
				functionNames[name] = true
		REDACTED
		case "custom":
			if name != "" {
				customNames[name] = true
		REDACTED
		case "tool_search":
			adapter.ToolSearch = true
	REDACTED
REDACTED
	for name := range customNames {
		if functionNames[name] {
			return ResponsesClientToolMapping{REDACTED, false, fmt.Errorf("custom tool %q conflicts with a function tool of the same name; this upstream cannot disambiguate them, rename one of the tools", name)
	REDACTED
REDACTED
	if adapter.ToolSearch && (functionNames[toolSearchProxyName] || customNames[toolSearchProxyName]) {
		return ResponsesClientToolMapping{REDACTED, false, fmt.Errorf("built-in tool_search conflicts with a declared tool named %q; this upstream cannot disambiguate them, rename the tool", toolSearchProxyName)
REDACTED

	// Namespace flattening also rewrites namespace-qualified history and choice.
	names, flattened, err := FlattenResponsesNamespaces(req)
	if err != nil {
		return ResponsesClientToolMapping{REDACTED, false, err
REDACTED
	adapter.NamespaceTools = names
	if adapter.ToolSearch {
		if _, exists := names[toolSearchProxyName]; exists {
			return ResponsesClientToolMapping{REDACTED, false, fmt.Errorf("built-in tool_search conflicts with namespace tool flattened as %q; this upstream cannot disambiguate them, rename the tool", toolSearchProxyName)
	REDACTED
REDACTED

	tools, _ = req["tools"].([]any)
	lowered := make([]any, 0, len(tools))
	changed := flattened
	seenSearch := false
	for _, raw := range tools {
		tool, ok := raw.(map[string]any)
		if !ok {
			lowered = append(lowered, raw)
			continue
	REDACTED
		typ := strings.TrimSpace(stringValue(tool["type"]))
		name := strings.TrimSpace(stringValue(tool["name"]))
		switch typ {
		case "custom":
			if name == "" {
				lowered = append(lowered, raw)
				continue
		REDACTED
			copy := copyClientTool(tool)
			copy["type"] = "function"
			copy["parameters"] = json.RawMessage(customToolInputSchema)
			delete(copy, "format")
			adapter.CustomTools[name] = true
			lowered = append(lowered, copy)
			changed = true
		case "tool_search":
			if seenSearch {
				changed = true
				continue
		REDACTED
			seenSearch = true
			lowered = append(lowered, map[string]any{
				"type": "function", "name": toolSearchProxyName,
				"description": "Search and load Codex tools, plugins, connectors, and MCP namespaces for the current task.",
				"parameters":  json.RawMessage(toolSearchProxySchema),
		REDACTED)
			changed = true
		default:
			lowered = append(lowered, raw)
	REDACTED
REDACTED
	if changed {
		req["tools"] = lowered
REDACTED
	if rewriteClientToolHistory(req["input"], &adapter) {
		changed = true
REDACTED
	if rewriteClientToolChoice(req, &adapter) {
		changed = true
REDACTED
	if len(adapter.CustomTools) == 0 {
		adapter.CustomTools = nil
REDACTED
	if len(adapter.NamespaceTools) == 0 {
		adapter.NamespaceTools = nil
REDACTED
	return adapter, changed, nil
REDACTED

func copyClientTool(tool map[string]any) map[string]any {
	copy := make(map[string]any, len(tool))
	for key, value := range tool {
		copy[key] = value
REDACTED
	return copy
REDACTED

func rewriteClientToolHistory(value any, adapter *ResponsesClientToolMapping) bool {
	changed := false
	var visit func(any)
	visit = func(value any) {
		switch typed := value.(type) {
		case []any:
			for _, item := range typed {
				visit(item)
		REDACTED
		case map[string]any:
			typ := strings.TrimSpace(stringValue(typed["type"]))
			switch typ {
			case "custom_tool_call":
				if adapter.CustomTools[strings.TrimSpace(stringValue(typed["name"]))] {
					typed["type"] = "function_call"
					typed["arguments"] = customToolCallArguments(stringValue(typed["input"]))
					delete(typed, "input")
					changed = true
			REDACTED
			case "custom_tool_call_output":
				typed["type"] = "function_call_output"
				normalizeClientToolOutput(typed)
				changed = true
			case "tool_search_call":
				if adapter.ToolSearch {
					typed["type"] = "function_call"
					typed["name"] = toolSearchProxyName
					typed["arguments"] = rawObjectString(typed["arguments"])
					delete(typed, "execution")
					changed = true
			REDACTED
			case "tool_search_output":
				if adapter.ToolSearch {
					typed["type"] = "function_call_output"
					normalizeClientToolOutput(typed)
					changed = true
			REDACTED
		REDACTED
			for _, child := range typed {
				visit(child)
		REDACTED
	REDACTED
REDACTED
	visit(value)
	return changed
REDACTED

func normalizeClientToolOutput(item map[string]any) {
	output, exists := item["output"]
	if !exists {
		return
REDACTED
	if _, ok := output.(string); ok {
		return
REDACTED
	if output == nil {
		item["output"] = ""
		return
REDACTED
	encoded, err := json.Marshal(output)
	if err != nil {
		item["output"] = ""
		return
REDACTED
	item["output"] = string(encoded)
REDACTED

func rewriteClientToolChoice(req map[string]any, adapter *ResponsesClientToolMapping) bool {
	choice, ok := req["tool_choice"].(map[string]any)
	if !ok {
		return false
REDACTED
	typ := strings.TrimSpace(stringValue(choice["type"]))
	name := strings.TrimSpace(stringValue(choice["name"]))
	if typ == "custom" && adapter.CustomTools[name] {
		choice["type"] = "function"
		return true
REDACTED
	if typ == "tool_search" && adapter.ToolSearch {
		req["tool_choice"] = map[string]any{"type": "function", "name": toolSearchProxyNameREDACTED
		return true
REDACTED
	return false
REDACTED

func customToolCallArguments(input string) string {
	encoded, _ := json.Marshal(map[string]string{"input": inputREDACTED)
	return string(encoded)
REDACTED

func rawObjectString(value any) string {
	if text, ok := value.(string); ok {
		return text
REDACTED
	encoded, err := json.Marshal(value)
	if err != nil {
		return "{REDACTED"
REDACTED
	return string(encoded)
REDACTED

// RestoreResponsesClientToolPayload restores client tool calls in a non-stream
// native Responses JSON payload.
func RestoreResponsesClientToolPayload(payload []byte, mapping ResponsesClientToolMapping) ([]byte, bool, error) {
	if len(payload) == 0 {
		return payload, false, nil
REDACTED
	var value any
	if err := json.Unmarshal(payload, &value); err != nil {
		return payload, false, err
REDACTED
	changed := restoreClientToolValue(value, &mapping)
	if !changed {
		if len(mapping.NamespaceTools) == 0 {
			return payload, false, nil
	REDACTED
		return RestoreResponsesNamespaceCalls(payload, mapping.NamespaceTools)
REDACTED
	var rebuilt bytes.Buffer
	encoder := json.NewEncoder(&rebuilt)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return payload, false, err
REDACTED
	rebuiltPayload := bytes.TrimSuffix(rebuilt.Bytes(), []byte("\n"))
	if len(mapping.NamespaceTools) == 0 {
		return rebuiltPayload, true, nil
REDACTED
	restored, _, err := RestoreResponsesNamespaceCalls(rebuiltPayload, mapping.NamespaceTools)
	if err != nil {
		return payload, false, err
REDACTED
	return restored, true, nil
REDACTED

func restoreClientToolValue(value any, adapter *ResponsesClientToolMapping) bool {
	changed := false
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			changed = restoreClientToolValue(item, adapter) || changed
	REDACTED
	case map[string]any:
		if strings.TrimSpace(stringValue(typed["type"])) == "function_call" {
			name := strings.TrimSpace(stringValue(typed["name"]))
			if adapter.CustomTools[name] {
				typed["type"] = "custom_tool_call"
				typed["input"] = extractCustomToolCallInput(rawObjectString(typed["arguments"]))
				delete(typed, "arguments")
				delete(typed, "namespace")
				changed = true
		REDACTED else if adapter.ToolSearch && name == toolSearchProxyName {
				typed["type"] = "tool_search_call"
				typed["execution"] = "client"
				typed["arguments"] = json.RawMessage(toolSearchCallArgumentsJSON(rawObjectString(typed["arguments"])))
				delete(typed, "name")
				delete(typed, "namespace")
				changed = true
		REDACTED
	REDACTED
		for _, child := range typed {
			changed = restoreClientToolValue(child, adapter) || changed
	REDACTED
REDACTED
	return changed
REDACTED

// ResponsesClientToolStreamRestorer restores client tool stream lifecycles.
// It is intentionally stateful because custom tools need their function
// arguments buffered until the upstream signals the call is complete.
type ResponsesClientToolStreamRestorer struct {
	adapter  ResponsesClientToolMapping
	nextSeq  int
	seenSeq  bool
	calls    map[string]*responsesClientToolStreamCall
	byOutput map[int]*responsesClientToolStreamCall
REDACTED

type responsesClientToolStreamCall struct {
	kind      string
	name      string
	callID    string
	itemID    string
	outputIdx int
	arguments strings.Builder
REDACTED

func NewResponsesClientToolStreamRestorer(mapping ResponsesClientToolMapping) *ResponsesClientToolStreamRestorer {
	return &ResponsesClientToolStreamRestorer{adapter: mapping, calls: make(map[string]*responsesClientToolStreamCall), byOutput: make(map[int]*responsesClientToolStreamCall)REDACTED
REDACTED

// Restore transforms one upstream SSE event into zero or more client events.
// Returned sequence numbers are continuous even when function argument events
// are suppressed or a custom completion expands into two events.
func (r *ResponsesClientToolStreamRestorer) Restore(event ResponsesStreamEvent) []ResponsesStreamEvent {
	if r == nil {
		return []ResponsesStreamEvent{eventREDACTED
REDACTED
	if !r.seenSeq {
		r.nextSeq = event.SequenceNumber
		r.seenSeq = true
REDACTED
	var out []ResponsesStreamEvent
	emit := func(event ResponsesStreamEvent) {
		event.SequenceNumber = r.nextSeq
		r.nextSeq++
		out = append(out, event)
REDACTED

	switch event.Type {
	case "response.output_item.added":
		if call := r.recordItem(event); call != nil {
			if call.kind == "custom" {
				event.Item.Type = "custom_tool_call"
				event.Item.Input = ""
				event.Item.Arguments = ""
				event.Item.Namespace = ""
		REDACTED else {
				event.Item.Type = "tool_search_call"
				event.Item.Name = ""
				event.Item.Arguments = "{REDACTED"
				event.Item.Namespace = ""
		REDACTED
	REDACTED
		emit(r.restoreNamespaceEvent(event))
	case "response.function_call_arguments.delta":
		if call := r.callFor(event); call != nil {
			_, _ = call.arguments.WriteString(event.Delta)
			return nil
	REDACTED
		emit(r.restoreNamespaceEvent(event))
	case "response.function_call_arguments.done":
		if call := r.callFor(event); call != nil {
			if event.Arguments != "" {
				call.arguments.Reset()
				_, _ = call.arguments.WriteString(event.Arguments)
		REDACTED
			if call.kind == "custom" {
				input := extractCustomToolCallInput(call.arguments.String())
				if input != "" {
					emit(ResponsesStreamEvent{Type: "response.custom_tool_call_input.delta", OutputIndex: call.outputIdx, ItemID: call.itemID, Delta: inputREDACTED)
			REDACTED
				emit(ResponsesStreamEvent{Type: "response.custom_tool_call_input.done", OutputIndex: call.outputIdx, ItemID: call.itemID, CallID: call.callID, Name: call.name, Input: inputREDACTED)
		REDACTED
			return out
	REDACTED
		emit(r.restoreNamespaceEvent(event))
	case "response.output_item.done":
		if call := r.recordItem(event); call != nil {
			if call.kind == "custom" {
				event.Item.Type = "custom_tool_call"
				event.Item.Input = extractCustomToolCallInput(call.arguments.String())
				event.Item.Arguments = ""
				event.Item.Namespace = ""
		REDACTED else {
				event.Item.Type = "tool_search_call"
				event.Item.Name = ""
				event.Item.Arguments = call.arguments.String()
				if strings.TrimSpace(event.Item.Arguments) == "" {
					event.Item.Arguments = "{REDACTED"
			REDACTED
				event.Item.Namespace = ""
		REDACTED
			delete(r.calls, call.itemID)
			delete(r.calls, call.callID)
			delete(r.byOutput, call.outputIdx)
	REDACTED
		emit(r.restoreNamespaceEvent(event))
	default:
		// response.completed carries the non-stream representation.
		if event.Response != nil {
			restoreResponsesOutputClientTools(event.Response.Output, &r.adapter)
	REDACTED
		emit(r.restoreNamespaceEvent(event))
REDACTED
	return out
REDACTED

// RestoreEvent restores one Responses SSE JSON data payload. Custom tool
// completions can expand to multiple payloads and proxy argument deltas can be
// intentionally dropped, hence the slice return value.
func (r *ResponsesClientToolStreamRestorer) RestoreEvent(payload []byte) ([][]byte, bool, error) {
	if len(payload) == 0 {
		return nil, false, nil
REDACTED
	var wire struct {
		Type     string `json:"type"`
		Sequence int    `json:"sequence_number"`
REDACTED
	if err := json.Unmarshal(payload, &wire); err != nil {
		return nil, false, err
REDACTED
	if wire.Type == "response.completed" || wire.Type == "response.incomplete" || wire.Type == "response.failed" {
		restored, changed, err := RestoreResponsesClientToolPayload(payload, r.adapter)
		if err != nil {
			return nil, false, err
	REDACTED
		return r.resequenceRaw(restored, wire.Sequence, changed)
REDACTED
	if !clientToolLifecycleEvent(wire.Type) {
		return r.resequenceRaw(payload, wire.Sequence, false)
REDACTED
	if !r.clientToolEventPayload(payload) {
		return r.resequenceRaw(payload, wire.Sequence, false)
REDACTED
	var event ResponsesStreamEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return nil, false, err
REDACTED
	events := r.Restore(event)
	if len(events) == 1 {
		unchanged, err := json.Marshal(events[0])
		if err == nil && bytes.Equal(bytes.TrimSpace(unchanged), bytes.TrimSpace(payload)) {
			return [][]byte{payloadREDACTED, false, nil
	REDACTED
REDACTED
	result := make([][]byte, 0, len(events))
	for _, restored := range events {
		encoded, err := json.Marshal(restored)
		if err != nil {
			return nil, false, err
	REDACTED
		result = append(result, encoded)
REDACTED
	return result, true, nil
REDACTED

func (r *ResponsesClientToolStreamRestorer) clientToolEventPayload(payload []byte) bool {
	var raw struct {
		ItemID      string `json:"item_id"`
		CallID      string `json:"call_id"`
		Name        string `json:"name"`
		OutputIndex int    `json:"output_index"`
		Item        *struct {
			Type   string `json:"type"`
			ID     string `json:"id"`
			CallID string `json:"call_id"`
			Name   string `json:"name"`
	REDACTED `json:"item"`
REDACTED
	if err := json.Unmarshal(payload, &raw); err != nil {
		return false
REDACTED
	if raw.Item != nil {
		if raw.Item.Type != "function_call" {
			return false
	REDACTED
		_, namespaceTool := r.adapter.NamespaceTools[raw.Item.Name]
		return r.adapter.CustomTools[raw.Item.Name] || (r.adapter.ToolSearch && raw.Item.Name == toolSearchProxyName) || namespaceTool || r.calls[raw.Item.ID] != nil || r.calls[raw.Item.CallID] != nil
REDACTED
	if _, namespaceTool := r.adapter.NamespaceTools[raw.Name]; namespaceTool {
		return true
REDACTED
	if r.calls[raw.ItemID] != nil || r.calls[raw.CallID] != nil || r.byOutput[raw.OutputIndex] != nil {
		return true
REDACTED
	return false
REDACTED

func clientToolLifecycleEvent(typ string) bool {
	switch typ {
	case "response.output_item.added", "response.output_item.done", "response.function_call_arguments.delta", "response.function_call_arguments.done":
		return true
	default:
		return false
REDACTED
REDACTED

// resequenceRaw deliberately keeps opaque upstream event fields untouched.
func (r *ResponsesClientToolStreamRestorer) resequenceRaw(payload []byte, sequence int, changed bool) ([][]byte, bool, error) {
	if !r.seenSeq {
		r.nextSeq, r.seenSeq = sequence, true
REDACTED
	if r.nextSeq == sequence && !changed {
		r.nextSeq++
		return [][]byte{payloadREDACTED, false, nil
REDACTED
	var raw map[string]any
	if err := json.Unmarshal(payload, &raw); err != nil {
		return nil, false, err
REDACTED
	raw["sequence_number"] = r.nextSeq
	r.nextSeq++
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil, false, err
REDACTED
	return [][]byte{encodedREDACTED, true, nil
REDACTED

func (r *ResponsesClientToolStreamRestorer) recordItem(event ResponsesStreamEvent) *responsesClientToolStreamCall {
	if event.Item == nil || event.Item.Type != "function_call" {
		return nil
REDACTED
	name := event.Item.Name
	kind := ""
	if r.adapter.CustomTools[name] {
		kind = "custom"
REDACTED else if r.adapter.ToolSearch && name == toolSearchProxyName {
		kind = "tool_search"
REDACTED
	if kind == "" {
		return nil
REDACTED
	key := event.Item.ID
	if key == "" {
		key = event.Item.CallID
REDACTED
	call := r.calls[key]
	if call == nil {
		call = &responsesClientToolStreamCall{kind: kind, name: name, callID: event.Item.CallID, itemID: event.Item.ID, outputIdx: event.OutputIndexREDACTED
		r.calls[key] = call
		if call.callID != "" {
			r.calls[call.callID] = call
	REDACTED
		r.byOutput[call.outputIdx] = call
REDACTED
	if event.Item.Arguments != "" {
		call.arguments.Reset()
		_, _ = call.arguments.WriteString(event.Item.Arguments)
REDACTED
	return call
REDACTED

func (r *ResponsesClientToolStreamRestorer) callFor(event ResponsesStreamEvent) *responsesClientToolStreamCall {
	if call := r.calls[event.ItemID]; call != nil {
		return call
REDACTED
	if call := r.byOutput[event.OutputIndex]; call != nil {
		return call
REDACTED
	for _, call := range r.calls {
		if (event.CallID != "" && call.callID == event.CallID) || (event.ItemID == "" && event.Name != "" && call.name == event.Name) {
			return call
	REDACTED
REDACTED
	return nil
REDACTED

func (r *ResponsesClientToolStreamRestorer) restoreNamespaceEvent(event ResponsesStreamEvent) ResponsesStreamEvent {
	if len(r.adapter.NamespaceTools) == 0 {
		return event
REDACTED
	if event.Item != nil && event.Item.Type == "function_call" {
		if name, ok := r.adapter.NamespaceTools[event.Item.Name]; ok {
			event.Item.Name, event.Item.Namespace = name.Name, name.Namespace
	REDACTED
REDACTED
	if event.Type == "response.function_call_arguments.done" {
		if name, ok := r.adapter.NamespaceTools[event.Name]; ok {
			event.Name = name.Name
	REDACTED
REDACTED
	return event
REDACTED

func restoreResponsesOutputClientTools(outputs []ResponsesOutput, adapter *ResponsesClientToolMapping) {
	for index := range outputs {
		output := &outputs[index]
		if output.Type != "function_call" {
			continue
	REDACTED
		if adapter.CustomTools[output.Name] {
			output.Type = "custom_tool_call"
			output.Input = extractCustomToolCallInput(output.Arguments)
			output.Arguments = ""
			output.Namespace = ""
	REDACTED else if adapter.ToolSearch && output.Name == toolSearchProxyName {
			output.Type = "tool_search_call"
			output.Name = ""
			output.Namespace = ""
	REDACTED
		if name, ok := adapter.NamespaceTools[output.Name]; ok && output.Type == "function_call" {
			output.Name, output.Namespace = name.Name, name.Namespace
	REDACTED
REDACTED
REDACTED
