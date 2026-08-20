package apicompat

import (
	"encoding/json"
	"fmt"
	"strings"
)

type responsesDiscoveredToolIdentity struct {
	typ       string
	name      string
	namespace string
	encoded   string
	ambiguous bool
REDACTED

// promoteResponsesToolSearchDiscoveries makes successfully discovered client
// tools callable for function-only upstreams. The original tool_search_output
// remains request history and is normalized separately; declarations are
// appended after static tools so the client's declaration order stays stable.
func promoteResponsesToolSearchDiscoveries(req map[string]any) (bool, error) {
	tools, ok := req["tools"].([]any)
	if !ok || len(tools) == 0 || !hasResponsesToolSearchDeclaration(tools) {
		return false, nil
REDACTED
	input, ok := req["input"].([]any)
	if !ok || len(input) == 0 {
		return false, nil
REDACTED

	known := make(map[string]responsesDiscoveredToolIdentity)
	for _, raw := range tools {
		registerExistingResponsesToolIdentity(known, raw)
REDACTED

	promoted := make([]any, 0)
	for _, rawItem := range input {
		item, ok := rawItem.(map[string]any)
		if !ok || strings.TrimSpace(stringValue(item["type"])) != "tool_search_output" || !usableResponsesToolSearchOutput(item) {
			continue
	REDACTED
		discoveries, ok := item["tools"].([]any)
		if !ok || len(discoveries) == 0 {
			continue
	REDACTED
		if _, err := json.Marshal(discoveries); err != nil {
			continue
	REDACTED
		for _, rawDiscovery := range discoveries {
			discovery, ok := rawDiscovery.(map[string]any)
			if !ok {
				continue
		REDACTED
			typ := strings.TrimSpace(stringValue(discovery["type"]))
			switch typ {
			case "function", "custom":
				copy, identity, ok := responsesDirectToolDiscovery(discovery, typ)
				if !ok {
					continue
			REDACTED
				appendTool, err := admitResponsesDiscoveredTool(known, identity.name, identity)
				if err != nil {
					return false, err
			REDACTED
				if appendTool {
					promoted = append(promoted, copy)
			REDACTED
			case "namespace":
				copy, identities, ok := responsesNamespaceToolDiscovery(discovery)
				if !ok {
					continue
			REDACTED
				children := make([]any, 0, len(identities))
				for _, candidate := range identities {
					appendTool, err := admitResponsesDiscoveredTool(known, candidate.flat, candidate.identity)
					if err != nil {
						return false, err
				REDACTED
					if appendTool {
						children = append(children, candidate.child)
				REDACTED
			REDACTED
				if len(children) > 0 {
					copy["tools"] = children
					delete(copy, "children")
					promoted = append(promoted, copy)
			REDACTED
		REDACTED
	REDACTED
REDACTED
	if len(promoted) == 0 {
		return false, nil
REDACTED
	req["tools"] = append(tools, promoted...)
	return true, nil
REDACTED

func hasResponsesToolSearchDeclaration(tools []any) bool {
	for _, raw := range tools {
		tool, ok := raw.(map[string]any)
		if ok && strings.TrimSpace(stringValue(tool["type"])) == "tool_search" {
			return true
	REDACTED
REDACTED
	return false
REDACTED

func usableResponsesToolSearchOutput(item map[string]any) bool {
	status, present := item["status"]
	if !present {
		return true
REDACTED
	text, ok := status.(string)
	return ok && strings.TrimSpace(text) == "completed"
REDACTED

func registerExistingResponsesToolIdentity(known map[string]responsesDiscoveredToolIdentity, raw any) {
	tool, ok := raw.(map[string]any)
	if !ok {
		return
REDACTED
	typ := strings.TrimSpace(stringValue(tool["type"]))
	switch typ {
	case "function", "custom":
		_, identity, ok := responsesDirectToolDiscovery(tool, typ)
		if ok {
			registerExistingResponsesIdentity(known, identity.name, identity)
	REDACTED
	case "namespace":
		_, identities, ok := responsesNamespaceToolDiscovery(tool)
		if !ok {
			return
	REDACTED
		for _, candidate := range identities {
			registerExistingResponsesIdentity(known, candidate.flat, candidate.identity)
	REDACTED
REDACTED
REDACTED

func registerExistingResponsesIdentity(known map[string]responsesDiscoveredToolIdentity, key string, identity responsesDiscoveredToolIdentity) {
	previous, exists := known[key]
	if !exists {
		known[key] = identity
		return
REDACTED
	if !sameResponsesDiscoveredTool(previous, identity) {
		previous.ambiguous = true
		known[key] = previous
REDACTED
REDACTED

func admitResponsesDiscoveredTool(known map[string]responsesDiscoveredToolIdentity, key string, identity responsesDiscoveredToolIdentity) (bool, error) {
	previous, exists := known[key]
	if !exists {
		known[key] = identity
		return true, nil
REDACTED
	if !previous.ambiguous && sameResponsesDiscoveredTool(previous, identity) {
		return false, nil
REDACTED
	return false, fmt.Errorf("discovered tool %q conflicts with an existing declaration; this upstream cannot safely disambiguate different names, namespaces, or schemas", key)
REDACTED

func sameResponsesDiscoveredTool(left, right responsesDiscoveredToolIdentity) bool {
	return left.typ == right.typ && left.name == right.name && left.namespace == right.namespace && left.encoded == right.encoded
REDACTED

func responsesDirectToolDiscovery(tool map[string]any, typ string) (map[string]any, responsesDiscoveredToolIdentity, bool) {
	name := strings.TrimSpace(stringValue(tool["name"]))
	if name == "" {
		return nil, responsesDiscoveredToolIdentity{REDACTED, false
REDACTED
	copy := copyClientTool(tool)
	copy["type"] = typ
	copy["name"] = name
	identityCopy := copy
	if typ == "custom" {
		identityCopy = copyClientTool(copy)
		identityCopy["type"] = "function"
		identityCopy["parameters"] = json.RawMessage(customToolInputSchema)
		delete(identityCopy, "format")
REDACTED
	encoded, err := json.Marshal(identityCopy)
	if err != nil {
		return nil, responsesDiscoveredToolIdentity{REDACTED, false
REDACTED
	return copy, responsesDiscoveredToolIdentity{typ: typ, name: name, encoded: string(encoded)REDACTED, true
REDACTED

// restoreInheritedResponsesClientToolDeclarations reverses only the declaration
// identities recorded by ResponsesClientToolMapping. It is used when a WS
// continuation omits tools but the HTTP function upstream still needs the
// effective session declarations on every request.
func restoreInheritedResponsesClientToolDeclarations(lowered []any, mapping ResponsesClientToolMapping) []any {
	restored := make([]any, 0, len(lowered))
	for _, raw := range lowered {
		tool, ok := raw.(map[string]any)
		if !ok {
			restored = append(restored, raw)
			continue
	REDACTED
		name := strings.TrimSpace(stringValue(tool["name"]))
		switch {
		case mapping.ToolSearch && name == toolSearchProxyName:
			restored = append(restored, map[string]any{"type": "tool_search"REDACTED)
		case mapping.CustomTools[name]:
			copy := copyClientTool(tool)
			copy["type"] = "custom"
			restored = append(restored, copy)
		case mapping.NamespaceTools[name].Namespace != "":
			identity := mapping.NamespaceTools[name]
			child := copyClientTool(tool)
			child["type"] = "function"
			child["name"] = identity.Name
			restored = append(restored, map[string]any{
				"type": "namespace", "name": identity.Namespace, "tools": []any{childREDACTED,
		REDACTED)
		default:
			restored = append(restored, copyClientTool(tool))
	REDACTED
REDACTED
	return restored
REDACTED

type responsesNamespaceToolCandidate struct {
	flat     string
	child    map[string]any
	identity responsesDiscoveredToolIdentity
REDACTED

func responsesNamespaceToolDiscovery(tool map[string]any) (map[string]any, []responsesNamespaceToolCandidate, bool) {
	namespace := strings.TrimSpace(stringValue(tool["name"]))
	children := namespaceChildren(tool)
	if namespace == "" || len(children) == 0 {
		return nil, nil, false
REDACTED
	copy := copyClientTool(tool)
	copy["type"] = "namespace"
	copy["name"] = namespace
	identities := make([]responsesNamespaceToolCandidate, 0, len(children))
	for _, rawChild := range children {
		child, ok := rawChild.(map[string]any)
		if !ok || strings.TrimSpace(stringValue(child["type"])) != "function" {
			continue
	REDACTED
		childCopy, direct, ok := responsesDirectToolDiscovery(child, "function")
		if !ok {
			continue
	REDACTED
		flat := flattenNamespaceToolName(namespace, direct.name)
		identities = append(identities, responsesNamespaceToolCandidate{
			flat:  flat,
			child: childCopy,
			identity: responsesDiscoveredToolIdentity{
				typ: "namespace", name: direct.name, namespace: namespace, encoded: direct.encoded,
		REDACTED,
	REDACTED)
REDACTED
	if len(identities) == 0 {
		return nil, nil, false
REDACTED
	return copy, identities, true
REDACTED
